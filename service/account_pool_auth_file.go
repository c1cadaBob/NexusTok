// account_pool_auth_file.go 实现原生账号池认证文件的解析与导入。
//
// 认证文件是 NexusTok 原生账号池的一等管理对象：管理员导入一份 JSON 文件后，
// 系统会保存加密原文、解析文件级分组/代理/优先级，并生成一个 PoolAccount 参与
// 现有调度热路径。认证文件负责保留凭据来源和可恢复入口，PoolAccount 负责实际调度、
// 并发控制和错误冷却逻辑。
package service

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"gorm.io/gorm"
)

// AccountPoolAuthFileImportOptions 描述导入认证文件时可由管理员覆盖的文件级配置。
type AccountPoolAuthFileImportOptions struct {
	Name           string   // 文件显示名称
	Content        string   // JSON 认证文件原文
	PoolGroupID    int      // 显式指定账号池分组；为空时按 provider 自动创建或复用
	GroupName      string   // 自动创建分组时使用的名称
	Provider       string   // 覆盖解析出的 provider
	Platform       string   // 覆盖解析出的本地平台
	AuthType       string   // 覆盖解析出的认证类型
	AccountGroups  []string // 文件级调用分组
	Models         string   // 文件级模型限制
	Proxy          string   // 文件级代理
	BaseURL        *string  // 文件级基础 URL
	Priority       *int64   // 文件级优先级
	Weight         *int     // 文件级权重
	MaxConcurrency *int     // 文件级最大并发数
	Status         *int     // 文件状态，同时同步到生成账号
}

// AccountPoolAuthFileBatchImportOptions 描述批量导入认证文件时的行为。
// 当前主要用于兼容 Sub2api 导出的 sub2api-data/sub2api-bundle 包；普通单个
// JSON 对象也会被包装成批量结果，方便前端统一展示导入结果。
type AccountPoolAuthFileBatchImportOptions struct {
	AccountPoolAuthFileImportOptions
	SkipDuplicates bool // 遇到重复文件摘要时跳过而不是中断整个批次
}

// AccountPoolAuthFileUpdateOptions 描述认证文件编辑请求。
// Content 为空时只更新文件级调度字段；Content 非空时会重新解析并替换生成账号的凭据。
type AccountPoolAuthFileUpdateOptions struct {
	Name           *string
	Content        *string
	PoolGroupID    *int
	GroupName      *string
	Provider       *string
	Platform       *string
	AuthType       *string
	AccountGroups  *[]string
	Models         *string
	Proxy          *string
	BaseURL        *string
	Priority       *int64
	Weight         *int
	MaxConcurrency *int
	Status         *int
}

// ParsedAccountPoolAuthFile 是认证文件解析后的规范化结果。
type ParsedAccountPoolAuthFile struct {
	Name              string
	SourcePlatform    string
	Format            string
	Provider          string
	Platform          string
	AuthType          string
	CredentialJSON    string
	FileDigest        string
	CredentialSummary string
	Metadata          map[string]any
	Attributes        map[string]string
	AccountGroups     []string
	Models            string
	Proxy             string
	BaseURL           *string
	Priority          int64
	Weight            int
	MaxConcurrency    int
	Status            int
}

// AccountPoolAuthFileImportResult 是导入或更新认证文件后的完整结果。
type AccountPoolAuthFileImportResult struct {
	AuthFile *model.AccountPoolAuthFile
	Account  *model.PoolAccount
	Group    *model.AccountPoolGroup
}

// AccountPoolAuthFileBatchImportResult 汇总批量导入结果。
type AccountPoolAuthFileBatchImportResult struct {
	Created int                                `json:"created"`
	Skipped int                                `json:"skipped"`
	Failed  int                                `json:"failed"`
	Items   []*AccountPoolAuthFileImportResult `json:"items"`
	Errors  []AccountPoolAuthFileImportError   `json:"errors,omitempty"`
}

// AccountPoolAuthFileImportError 描述批量导入中某个账号或代理映射的失败原因。
type AccountPoolAuthFileImportError struct {
	Index   int    `json:"index"`
	Name    string `json:"name,omitempty"`
	Message string `json:"message"`
}

type accountPoolAuthFileCandidate struct {
	name   string
	format string
	value  map[string]any
}

type accountPoolAuthFileBatchItem struct {
	index int
	name  string
	opts  AccountPoolAuthFileImportOptions
}

type sub2APIDataPayload struct {
	Type       string               `json:"type,omitempty"`
	Version    int                  `json:"version,omitempty"`
	ExportedAt string               `json:"exported_at"`
	Proxies    []sub2APIDataProxy   `json:"proxies"`
	Accounts   []sub2APIDataAccount `json:"accounts"`
}

type sub2APIDataProxy struct {
	ProxyKey string `json:"proxy_key"`
	Name     string `json:"name"`
	Protocol string `json:"protocol"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Status   string `json:"status"`
}

type sub2APIDataAccount struct {
	Name               string         `json:"name"`
	Notes              *string        `json:"notes,omitempty"`
	Platform           string         `json:"platform"`
	Type               string         `json:"type"`
	Credentials        map[string]any `json:"credentials"`
	Extra              map[string]any `json:"extra,omitempty"`
	ProxyKey           *string        `json:"proxy_key,omitempty"`
	Concurrency        int            `json:"concurrency"`
	Priority           int            `json:"priority"`
	RateMultiplier     *float64       `json:"rate_multiplier,omitempty"`
	ExpiresAt          *int64         `json:"expires_at,omitempty"`
	AutoPauseOnExpired *bool          `json:"auto_pause_on_expired,omitempty"`
}

var ErrAccountPoolAuthFileDuplicate = errors.New("auth file already exists")

const (
	sub2APIDataType       = "sub2api-data"
	sub2APILegacyDataType = "sub2api-bundle"
	sub2APIDataVersion    = 1
)

var accountPoolAuthWrapperKeys = []string{
	"auth",
	"auth_file",
	"authFile",
	"credential",
	"credentials",
	"data",
	"account",
	"token",
	"token_data",
	"tokenData",
}

var accountPoolKnownSourcePlatforms = map[string]string{
	"native":  model.AccountPoolAuthFileFormatNative,
	"sub2":    model.AccountPoolAuthFileFormatSub2,
	"newapi":  model.AccountPoolAuthFileFormatNewAPI,
	"new-api": model.AccountPoolAuthFileFormatNewAPI,
	"oneapi":  "oneapi",
	"one-api": "oneapi",
}

// ParseAccountPoolAuthFile 将多来源 JSON 认证文件解析为本地统一结构。
// 支持两类输入：
// 1. 原生凭据对象：顶层包含 type、access_token/api_key、proxy_url、account_groups；
// 2. sub2/newapi 等包装：顶层描述来源平台，auth/credential/data/account 中放真实凭据。
func ParseAccountPoolAuthFile(raw string, opts AccountPoolAuthFileImportOptions) (*ParsedAccountPoolAuthFile, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("auth file content is required")
	}
	var root map[string]any
	if err := common.UnmarshalJsonStr(trimmed, &root); err != nil {
		return nil, fmt.Errorf("auth file must be a JSON object: %w", err)
	}
	if len(root) == 0 {
		return nil, fmt.Errorf("auth file JSON object is empty")
	}

	candidate := selectAccountPoolAuthFileCandidate(root)
	if candidate.value == nil {
		return nil, fmt.Errorf("auth file does not contain a recognizable credential object")
	}
	if !hasAuthFileCredentialFields(candidate.value) {
		return nil, fmt.Errorf("auth file does not contain credential fields")
	}

	sourcePlatform := detectAccountPoolAuthSourcePlatform(root)
	format := sourcePlatform
	if format == "" {
		format = candidate.format
	}
	if format == "" {
		format = model.AccountPoolAuthFileFormatNative
	}

	metadata := cloneAuthFileMetadata(candidate.value)
	provider := normalizeAccountPoolAuthProvider(firstNonEmpty(
		opts.Provider,
		readAuthFileString(metadata, "type", "provider", "platform", "vendor", "service", "model_provider", "modelProvider"),
		readAuthFileString(root, "provider", "type", "vendor", "service", "model_provider", "modelProvider"),
		inferProviderFromAuthFileCredential(metadata),
	))
	if provider == "" {
		return nil, fmt.Errorf("auth file provider is required")
	}
	if provider == "codex" {
		metadata = normalizeCodexAuthFileMetadata(metadata)
	}
	metadata["type"] = provider

	platform := normalizeAccountPoolAuthProvider(firstNonEmpty(opts.Platform, provider))
	authType := normalizeAccountPoolAuthType(firstNonEmpty(opts.AuthType, readAuthFileString(metadata, "auth_type", "authType", "credential_type", "credentialType")))
	if authType == "" {
		authType = inferAccountPoolAuthType(provider, metadata)
	}

	accountGroups := normalizeAccountPoolAuthGroups(
		opts.AccountGroups,
		readAuthFileGroups(metadata, "account_groups", "accountGroups", "account_group", "accountGroup", "group"),
		readAuthFileGroups(root, "account_groups", "accountGroups", "account_group", "accountGroup", "group"),
	)
	models := firstNonEmpty(strings.TrimSpace(opts.Models), readAuthFileCSV(metadata, "models", "model"), readAuthFileCSV(root, "models", "model"))
	proxy := firstNonEmpty(strings.TrimSpace(opts.Proxy), readAuthFileString(metadata, "proxy_url", "proxyUrl", "proxy-url", "proxy"), readAuthFileString(root, "proxy_url", "proxyUrl", "proxy-url", "proxy"))
	baseURL := opts.BaseURL
	if baseURL == nil {
		baseURL = optionalAuthFileString(metadata, "base_url", "baseURL", "api_base", "apiBase", "endpoint", "upstream_url", "upstreamUrl")
	}

	priority := readAuthFileInt64(metadata, 0, "priority")
	if opts.Priority != nil {
		priority = *opts.Priority
	}
	weight := int(readAuthFileInt64(metadata, 1, "weight"))
	if opts.Weight != nil {
		weight = *opts.Weight
	}
	if weight <= 0 {
		weight = 1
	}
	maxConcurrency := int(readAuthFileInt64(metadata, 0, "max_concurrency", "maxConcurrency", "concurrency"))
	if opts.MaxConcurrency != nil {
		maxConcurrency = *opts.MaxConcurrency
	}
	if maxConcurrency < 0 {
		maxConcurrency = 0
	}
	status := inferAuthFileStatus(metadata)
	if opts.Status != nil && *opts.Status > 0 {
		status = *opts.Status
	}

	name := firstNonEmpty(strings.TrimSpace(opts.Name), readAuthFileString(metadata, "name", "label", "email", "account_id", "accountId"), provider+" auth file")
	attrs := buildAccountPoolAuthFileAttributes(sourcePlatform, format, accountGroups, proxy, priority, weight, maxConcurrency, metadata)
	credentialJSON, err := marshalStableAuthFileJSON(metadata)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256([]byte(trimmed))

	return &ParsedAccountPoolAuthFile{
		Name:              name,
		SourcePlatform:    sourcePlatform,
		Format:            format,
		Provider:          provider,
		Platform:          platform,
		AuthType:          authType,
		CredentialJSON:    credentialJSON,
		FileDigest:        hex.EncodeToString(digest[:]),
		CredentialSummary: model.NormalizeAccountPoolCredentialSummary(credentialJSON),
		Metadata:          metadata,
		Attributes:        attrs,
		AccountGroups:     accountGroups,
		Models:            normalizeAccountPoolAuthCSV(models),
		Proxy:             strings.TrimSpace(proxy),
		BaseURL:           baseURL,
		Priority:          priority,
		Weight:            weight,
		MaxConcurrency:    maxConcurrency,
		Status:            status,
	}, nil
}

// ImportAccountPoolAuthFile 解析 JSON 文件并创建认证文件记录及其对应 PoolAccount。
func ImportAccountPoolAuthFile(opts AccountPoolAuthFileImportOptions) (*AccountPoolAuthFileImportResult, error) {
	parsed, err := ParseAccountPoolAuthFile(opts.Content, opts)
	if err != nil {
		return nil, err
	}
	encryptedContent, err := common.EncryptSensitiveString(strings.TrimSpace(opts.Content))
	if err != nil {
		return nil, err
	}
	encryptedCredential, err := common.EncryptSensitiveString(parsed.CredentialJSON)
	if err != nil {
		return nil, err
	}
	metadataJSON, attrsJSON, err := encodeAuthFileMetadata(parsed.Metadata, parsed.Attributes)
	if err != nil {
		return nil, err
	}

	result := &AccountPoolAuthFileImportResult{}
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		var duplicate model.AccountPoolAuthFile
		if err := tx.Where("file_digest = ?", parsed.FileDigest).First(&duplicate).Error; err == nil {
			return fmt.Errorf("%w: %s", ErrAccountPoolAuthFileDuplicate, duplicate.Name)
		} else if err != gorm.ErrRecordNotFound {
			return err
		}

		group, err := resolveAccountPoolAuthFileGroup(tx, parsed, opts)
		if err != nil {
			return err
		}
		account := buildPoolAccountFromParsedAuthFile(group, parsed, encryptedCredential, metadataJSON, attrsJSON)
		if err := tx.Create(account).Error; err != nil {
			return err
		}
		authFile := buildAccountPoolAuthFileRecord(group, account, parsed, encryptedContent, metadataJSON, attrsJSON)
		if err := tx.Create(authFile).Error; err != nil {
			return err
		}
		result.AuthFile = authFile
		result.Account = account
		result.Group = group
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ImportAccountPoolAuthFiles 自动识别单个认证对象或 Sub2api 导出包并执行导入。
// 普通单对象导入仍会复用 ImportAccountPoolAuthFile 的事务和去重逻辑；批量导入逐条
// 提交，避免某个坏账号阻断整包迁移，结果中会返回每条失败原因。
func ImportAccountPoolAuthFiles(opts AccountPoolAuthFileBatchImportOptions) (*AccountPoolAuthFileBatchImportResult, error) {
	items, buildErrors, isBatch, err := buildAccountPoolAuthFileBatchItems(opts)
	if err != nil {
		return nil, err
	}
	if !isBatch {
		result, err := ImportAccountPoolAuthFile(opts.AccountPoolAuthFileImportOptions)
		if err != nil {
			if opts.SkipDuplicates && errors.Is(err, ErrAccountPoolAuthFileDuplicate) {
				return &AccountPoolAuthFileBatchImportResult{Skipped: 1}, nil
			}
			return nil, err
		}
		return &AccountPoolAuthFileBatchImportResult{
			Created: 1,
			Items:   []*AccountPoolAuthFileImportResult{result},
		}, nil
	}

	result := &AccountPoolAuthFileBatchImportResult{
		Items:  make([]*AccountPoolAuthFileImportResult, 0, len(items)),
		Errors: append([]AccountPoolAuthFileImportError{}, buildErrors...),
	}
	result.Failed = len(buildErrors)
	for _, item := range items {
		imported, importErr := ImportAccountPoolAuthFile(item.opts)
		if importErr != nil {
			if opts.SkipDuplicates && errors.Is(importErr, ErrAccountPoolAuthFileDuplicate) {
				result.Skipped++
				continue
			}
			result.Failed++
			result.Errors = append(result.Errors, AccountPoolAuthFileImportError{
				Index:   item.index,
				Name:    item.name,
				Message: importErr.Error(),
			})
			continue
		}
		result.Created++
		result.Items = append(result.Items, imported)
	}
	return result, nil
}

// UpdateAccountPoolAuthFile 更新认证文件记录，并把文件级调度字段同步到关联账号。
func UpdateAccountPoolAuthFile(authFileID int, opts AccountPoolAuthFileUpdateOptions) (*AccountPoolAuthFileImportResult, error) {
	if authFileID <= 0 {
		return nil, fmt.Errorf("auth file id is required")
	}
	result := &AccountPoolAuthFileImportResult{}
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var authFile model.AccountPoolAuthFile
		if err := tx.Where("id = ?", authFileID).First(&authFile).Error; err != nil {
			return err
		}
		var account model.PoolAccount
		if authFile.PoolAccountId > 0 {
			if err := tx.Where("id = ?", authFile.PoolAccountId).First(&account).Error; err != nil && err != gorm.ErrRecordNotFound {
				return err
			}
		}

		group, err := resolveAccountPoolAuthFileUpdateGroup(tx, &authFile, opts)
		if err != nil {
			return err
		}
		updates := buildAuthFilePatchUpdates(&authFile, opts)
		accountUpdates := buildLinkedPoolAccountPatchUpdates(opts)

		if opts.Content != nil && strings.TrimSpace(*opts.Content) != "" {
			importOpts := authFileImportOptionsFromUpdate(&authFile, opts)
			importOpts.Content = *opts.Content
			parsed, parseErr := ParseAccountPoolAuthFile(*opts.Content, importOpts)
			if parseErr != nil {
				return parseErr
			}
			if parsed.FileDigest != authFile.FileDigest {
				var duplicate model.AccountPoolAuthFile
				if dupErr := tx.Where("file_digest = ? AND id <> ?", parsed.FileDigest, authFileID).First(&duplicate).Error; dupErr == nil {
					return fmt.Errorf("%w: %s", ErrAccountPoolAuthFileDuplicate, duplicate.Name)
				} else if dupErr != gorm.ErrRecordNotFound {
					return dupErr
				}
			}
			encryptedContent, encErr := common.EncryptSensitiveString(strings.TrimSpace(*opts.Content))
			if encErr != nil {
				return encErr
			}
			encryptedCredential, encErr := common.EncryptSensitiveString(parsed.CredentialJSON)
			if encErr != nil {
				return encErr
			}
			metadataJSON, attrsJSON, encErr := encodeAuthFileMetadata(parsed.Metadata, parsed.Attributes)
			if encErr != nil {
				return encErr
			}
			group, err = resolveAccountPoolAuthFileGroup(tx, parsed, importOpts)
			if err != nil {
				return err
			}
			updates = mergeStringAnyMaps(updates, map[string]interface{}{
				"name":                  parsed.Name,
				"source_platform":       parsed.SourcePlatform,
				"format":                parsed.Format,
				"provider":              parsed.Provider,
				"platform":              parsed.Platform,
				"auth_type":             parsed.AuthType,
				"pool_group_id":         group.Id,
				"status":                parsed.Status,
				"file_digest":           parsed.FileDigest,
				"encrypted_content":     encryptedContent,
				"credential_summary":    parsed.CredentialSummary,
				"credential_metadata":   metadataJSON,
				"credential_attributes": attrsJSON,
				"account_groups":        strings.Join(parsed.AccountGroups, ","),
				"models":                parsed.Models,
				"proxy":                 parsed.Proxy,
				"priority":              parsed.Priority,
				"weight":                parsed.Weight,
				"max_concurrency":       parsed.MaxConcurrency,
				"last_imported_time":    common.GetTimestamp(),
			})
			if parsed.BaseURL != nil {
				updates["base_url"] = *parsed.BaseURL
			}
			if account.Id == 0 {
				// 认证文件可能因为管理员删除池账号而保留为“无关联账号”的恢复记录。
				// 只有在本次请求重新提供 JSON 内容并成功解析出凭据时，才创建新的 PoolAccount；
				// 普通字段补丁不凭空生成账号，避免把缺少明文凭据的记录错误放回调度层。
				newAccount := buildPoolAccountFromParsedAuthFile(group, parsed, encryptedCredential, metadataJSON, attrsJSON)
				if err := tx.Create(newAccount).Error; err != nil {
					return err
				}
				account = *newAccount
				updates["pool_account_id"] = newAccount.Id
			}
			accountUpdates = mergeStringAnyMaps(accountUpdates, map[string]interface{}{
				"pool_group_id":         group.Id,
				"name":                  parsed.Name,
				"platform":              parsed.Platform,
				"auth_type":             parsed.AuthType,
				"credentials":           encryptedCredential,
				"credential_summary":    parsed.CredentialSummary,
				"credential_provider":   parsed.Provider,
				"credential_label":      parsed.Name,
				"credential_metadata":   metadataJSON,
				"credential_attributes": attrsJSON,
				"status":                parsed.Status,
				"schedulable":           parsed.Status == common.ChannelStatusEnabled,
				"models":                parsed.Models,
				"group":                 strings.Join(parsed.AccountGroups, ","),
				"proxy":                 parsed.Proxy,
				"priority":              parsed.Priority,
				"weight":                parsed.Weight,
				"max_concurrency":       parsed.MaxConcurrency,
				"unavailable":           false,
				"last_error":            "",
				"status_message":        "",
				"last_refreshed_time":   common.GetTimestamp(),
				"next_retry_time":       0,
				"rate_limited_until":    0,
				"overload_until":        0,
				"temp_disabled_until":   0,
			})
			if parsed.BaseURL != nil {
				accountUpdates["base_url"] = *parsed.BaseURL
			}
		}

		if group != nil {
			updates["pool_group_id"] = group.Id
			accountUpdates["pool_group_id"] = group.Id
		}
		if len(updates) > 0 {
			if err := tx.Model(&authFile).Updates(updates).Error; err != nil {
				return err
			}
		}
		if account.Id > 0 && len(accountUpdates) > 0 {
			if err := tx.Model(&account).Updates(accountUpdates).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("id = ?", authFileID).First(&authFile).Error; err != nil {
			return err
		}
		if authFile.PoolAccountId > 0 {
			_ = tx.Where("id = ?", authFile.PoolAccountId).First(&account).Error
		}
		result.AuthFile = &authFile
		if account.Id > 0 {
			result.Account = &account
		}
		if group == nil && authFile.PoolGroupId > 0 {
			group = &model.AccountPoolGroup{}
			_ = tx.Where("id = ?", authFile.PoolGroupId).First(group).Error
		}
		result.Group = group
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func buildAccountPoolAuthFileBatchItems(opts AccountPoolAuthFileBatchImportOptions) ([]accountPoolAuthFileBatchItem, []AccountPoolAuthFileImportError, bool, error) {
	trimmed := strings.TrimSpace(opts.Content)
	if trimmed == "" {
		return nil, nil, false, fmt.Errorf("auth file content is required")
	}
	var root map[string]any
	if err := common.UnmarshalJsonStr(trimmed, &root); err != nil {
		return nil, nil, false, fmt.Errorf("auth file must be a JSON object: %w", err)
	}
	if !looksLikeSub2APIDataPayload(root) {
		return nil, nil, false, nil
	}
	payloadBytes, err := common.Marshal(root)
	if err != nil {
		return nil, nil, true, err
	}
	var payload sub2APIDataPayload
	if err := common.Unmarshal(payloadBytes, &payload); err != nil {
		return nil, nil, true, err
	}
	if err := validateSub2APIDataPayload(payload); err != nil {
		return nil, nil, true, err
	}
	proxyURLByKey := buildSub2APIProxyURLMap(payload.Proxies)
	items := make([]accountPoolAuthFileBatchItem, 0, len(payload.Accounts))
	buildErrors := make([]AccountPoolAuthFileImportError, 0)
	for index, account := range payload.Accounts {
		itemOpts, err := buildSub2APIAccountImportOptions(account, proxyURLByKey, opts.AccountPoolAuthFileImportOptions)
		if err != nil {
			buildErrors = append(buildErrors, AccountPoolAuthFileImportError{
				Index:   index,
				Name:    account.Name,
				Message: err.Error(),
			})
			continue
		}
		items = append(items, accountPoolAuthFileBatchItem{
			index: index,
			name:  account.Name,
			opts:  itemOpts,
		})
	}
	return items, buildErrors, true, nil
}

func looksLikeSub2APIDataPayload(root map[string]any) bool {
	if root == nil {
		return false
	}
	payloadType := strings.ToLower(strings.TrimSpace(readAuthFileString(root, "type")))
	if payloadType == sub2APIDataType || payloadType == sub2APILegacyDataType {
		return true
	}
	_, hasAccounts := root["accounts"].([]any)
	_, hasProxies := root["proxies"].([]any)
	return hasAccounts && hasProxies
}

func validateSub2APIDataPayload(payload sub2APIDataPayload) error {
	payloadType := strings.ToLower(strings.TrimSpace(payload.Type))
	if payloadType != "" && payloadType != sub2APIDataType && payloadType != sub2APILegacyDataType {
		return fmt.Errorf("unsupported sub2 data type: %s", payload.Type)
	}
	if payload.Version != 0 && payload.Version != sub2APIDataVersion {
		return fmt.Errorf("unsupported sub2 data version: %d", payload.Version)
	}
	if len(payload.Accounts) == 0 {
		return fmt.Errorf("sub2 data accounts is required")
	}
	return nil
}

func buildSub2APIProxyURLMap(proxies []sub2APIDataProxy) map[string]string {
	result := make(map[string]string, len(proxies))
	for _, proxy := range proxies {
		proxyURL := buildSub2APIProxyURL(proxy)
		if proxyURL == "" {
			continue
		}
		key := strings.TrimSpace(proxy.ProxyKey)
		if key == "" {
			key = buildSub2APIProxyKey(proxy.Protocol, proxy.Host, proxy.Port, proxy.Username, proxy.Password)
		}
		if key != "" {
			result[key] = proxyURL
		}
	}
	return result
}

func buildSub2APIProxyKey(protocol string, host string, port int, username string, password string) string {
	return fmt.Sprintf("%s|%s|%d|%s|%s", strings.TrimSpace(protocol), strings.TrimSpace(host), port, strings.TrimSpace(username), strings.TrimSpace(password))
}

func buildSub2APIProxyURL(proxy sub2APIDataProxy) string {
	protocol := strings.ToLower(strings.TrimSpace(proxy.Protocol))
	host := strings.TrimSpace(proxy.Host)
	if protocol == "" || host == "" || proxy.Port <= 0 {
		return ""
	}
	u := &url.URL{
		Scheme: protocol,
		Host:   net.JoinHostPort(host, strconv.Itoa(proxy.Port)),
	}
	if strings.TrimSpace(proxy.Username) != "" {
		if strings.TrimSpace(proxy.Password) != "" {
			u.User = url.UserPassword(strings.TrimSpace(proxy.Username), strings.TrimSpace(proxy.Password))
		} else {
			u.User = url.User(strings.TrimSpace(proxy.Username))
		}
	}
	return u.String()
}

func buildSub2APIAccountImportOptions(account sub2APIDataAccount, proxyURLByKey map[string]string, base AccountPoolAuthFileImportOptions) (AccountPoolAuthFileImportOptions, error) {
	if strings.TrimSpace(account.Name) == "" {
		return AccountPoolAuthFileImportOptions{}, fmt.Errorf("sub2 account name is required")
	}
	if strings.TrimSpace(account.Platform) == "" {
		return AccountPoolAuthFileImportOptions{}, fmt.Errorf("sub2 account platform is required")
	}
	if len(account.Credentials) == 0 {
		return AccountPoolAuthFileImportOptions{}, fmt.Errorf("sub2 account credentials is required")
	}
	metadata := cloneAuthFileMetadata(account.Credentials)
	if account.Extra != nil {
		metadata["extra"] = cloneAuthFileMetadata(account.Extra)
	}
	if account.Notes != nil && strings.TrimSpace(*account.Notes) != "" {
		metadata["note"] = strings.TrimSpace(*account.Notes)
	}
	if account.ExpiresAt != nil && *account.ExpiresAt > 0 {
		metadata["expires_at"] = *account.ExpiresAt
	}
	if account.AutoPauseOnExpired != nil {
		metadata["auto_pause_on_expired"] = *account.AutoPauseOnExpired
	}
	if account.RateMultiplier != nil {
		metadata["rate_multiplier"] = *account.RateMultiplier
	}

	provider := normalizeSub2APIProvider(account.Platform, account.Type, metadata)
	authType := normalizeSub2APIAuthType(account.Type, metadata)
	metadata["type"] = provider
	metadata["provider"] = provider
	metadata["platform"] = provider
	metadata["auth_type"] = authType

	root := map[string]any{
		"source_platform": "sub2",
		"format":          model.AccountPoolAuthFileFormatSub2,
		"name":            account.Name,
		"provider":        provider,
		"platform":        provider,
		"auth_type":       authType,
		"credentials":     metadata,
	}
	if base.Proxy == "" && account.ProxyKey != nil {
		if proxyURL := proxyURLByKey[strings.TrimSpace(*account.ProxyKey)]; proxyURL != "" {
			root["proxy"] = proxyURL
		}
	}
	if account.Concurrency > 0 {
		root["max_concurrency"] = account.Concurrency
	}
	if account.Priority > 0 {
		// Sub2api 的 priority 是数值越小越优先；NexusTok 是数值越大越优先。
		// 这里取负数以保留 Sub2api 内部相对顺序，并避免和管理员手动设置的正优先级混淆。
		root["priority"] = -int64(account.Priority)
	}
	if base.Models == "" {
		if models := readAuthFileCSV(account.Extra, "models", "model"); models != "" {
			root["models"] = models
		}
	}
	if base.BaseURL == nil {
		if baseURL := optionalAuthFileString(account.Extra, "base_url", "baseURL", "api_base", "apiBase", "endpoint", "upstream_url", "upstreamUrl"); baseURL != nil {
			root["base_url"] = *baseURL
		}
	}
	contentBytes, err := common.Marshal(root)
	if err != nil {
		return AccountPoolAuthFileImportOptions{}, err
	}

	opts := base
	opts.Content = string(contentBytes)
	if strings.TrimSpace(opts.Name) == "" {
		opts.Name = account.Name
	}
	if strings.TrimSpace(opts.Provider) == "" {
		opts.Provider = provider
	}
	if strings.TrimSpace(opts.Platform) == "" {
		opts.Platform = provider
	}
	if strings.TrimSpace(opts.AuthType) == "" {
		opts.AuthType = authType
	}
	if len(opts.AccountGroups) == 0 {
		opts.AccountGroups = readAuthFileGroups(account.Extra, "account_groups", "accountGroups", "account_group", "accountGroup", "group", "groups")
	}
	if strings.TrimSpace(opts.Models) == "" {
		opts.Models = readAuthFileCSV(account.Extra, "models", "model")
	}
	if strings.TrimSpace(opts.Proxy) == "" {
		if proxy, ok := root["proxy"].(string); ok {
			opts.Proxy = proxy
		}
	}
	if opts.Priority == nil {
		priority := int64(0)
		if account.Priority > 0 {
			priority = -int64(account.Priority)
		}
		opts.Priority = &priority
	}
	if opts.MaxConcurrency == nil && account.Concurrency > 0 {
		maxConcurrency := account.Concurrency
		opts.MaxConcurrency = &maxConcurrency
	}
	return opts, nil
}

func normalizeSub2APIProvider(platform string, accountType string, metadata map[string]any) string {
	provider := normalizeAccountPoolAuthProvider(firstNonEmpty(
		readAuthFileString(metadata, "provider", "platform", "service", "vendor"),
		platform,
	))
	if provider == "openai" && strings.EqualFold(strings.TrimSpace(accountType), "oauth") {
		return "codex"
	}
	if provider == "" {
		provider = normalizeAccountPoolAuthProvider(platform)
	}
	return provider
}

func normalizeSub2APIAuthType(accountType string, metadata map[string]any) string {
	switch strings.ToLower(strings.TrimSpace(accountType)) {
	case "oauth":
		return model.AccountPoolAuthTypeOfficialOAuth
	case "apikey", "api-key":
		return model.AccountPoolAuthTypeAPIKey
	case "service_account", "service-account", "bedrock":
		return model.AccountPoolAuthTypeServiceAccount
	case "setup-token", "setup_token", "upstream":
		return model.AccountPoolAuthTypeCustomJSON
	default:
		if authType := normalizeAccountPoolAuthType(readAuthFileString(metadata, "auth_type", "authType", "credential_type", "credentialType")); authType != "" {
			return authType
		}
		return inferAccountPoolAuthType(normalizeAccountPoolAuthProvider(readAuthFileString(metadata, "provider", "type", "platform")), metadata)
	}
}

func selectAccountPoolAuthFileCandidate(root map[string]any) accountPoolAuthFileCandidate {
	candidates := []accountPoolAuthFileCandidate{{name: "root", format: detectAccountPoolAuthSourcePlatform(root), value: root}}
	for _, key := range accountPoolAuthWrapperKeys {
		if nested, ok := root[key].(map[string]any); ok && len(nested) > 0 {
			format := detectAccountPoolAuthSourcePlatform(root)
			candidates = append(candidates, accountPoolAuthFileCandidate{name: key, format: format, value: nested})
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return scoreAuthFileCandidate(candidates[i], root) > scoreAuthFileCandidate(candidates[j], root)
	})
	for _, candidate := range candidates {
		if scoreAuthFileCandidate(candidate, root) > 0 {
			return candidate
		}
	}
	return accountPoolAuthFileCandidate{}
}

func scoreAuthFileCandidate(candidate accountPoolAuthFileCandidate, root map[string]any) int {
	if candidate.value == nil {
		return 0
	}
	score := 0
	if hasAuthFileCredentialFields(candidate.value) {
		score += 16
	}
	if readAuthFileString(candidate.value, "type", "provider", "platform") != "" {
		score += 8
	}
	if candidate.name != "root" {
		score += 4
	}
	if readAuthFileString(root, "provider", "type") != "" {
		score += 2
	}
	return score
}

func hasAuthFileCredentialFields(value map[string]any) bool {
	for _, key := range []string{
		"api_key", "apiKey", "key", "access_token", "accessToken", "refresh_token", "refreshToken",
		"id_token", "idToken", "account_id", "accountId", "client_email", "private_key", "project_id",
		"cookie", "jwt", "token_data", "tokenData",
	} {
		if raw, ok := value[key]; ok && raw != nil {
			if text, ok := raw.(string); !ok || strings.TrimSpace(text) != "" {
				return true
			}
		}
	}
	return false
}

func detectAccountPoolAuthSourcePlatform(root map[string]any) string {
	for _, key := range []string{"source_platform", "sourcePlatform", "source", "platform", "format"} {
		value := strings.ToLower(strings.TrimSpace(readAuthFileString(root, key)))
		if canonical, ok := accountPoolKnownSourcePlatforms[value]; ok {
			return canonical
		}
	}
	return ""
}

func inferProviderFromAuthFileCredential(metadata map[string]any) string {
	if readAuthFileString(metadata, "access_token", "accessToken", "refresh_token", "refreshToken", "id_token", "idToken") != "" {
		return "codex"
	}
	if readAuthFileString(metadata, "api_key", "apiKey", "key") != "" {
		return readAuthFileString(metadata, "provider", "platform")
	}
	return ""
}

func normalizeAccountPoolAuthProvider(value string) string {
	provider := strings.ToLower(strings.TrimSpace(value))
	provider = strings.ReplaceAll(provider, "_", "-")
	switch provider {
	case "grok", "x.ai", "x-ai":
		return "xai"
	case "codex-cli", "openai-codex", "chatgpt":
		return "codex"
	case "newapi", "new-api", "sub2":
		return ""
	default:
		return provider
	}
}

func normalizeAccountPoolAuthType(value string) string {
	authType := strings.ToLower(strings.TrimSpace(value))
	authType = strings.ReplaceAll(authType, "-", "_")
	switch authType {
	case "oauth", "official-oauth":
		return model.AccountPoolAuthTypeOfficialOAuth
	case "apikey", "api-key":
		return model.AccountPoolAuthTypeAPIKey
	default:
		return authType
	}
}

func inferAccountPoolAuthType(provider string, metadata map[string]any) string {
	if provider == "codex" && readAuthFileString(metadata, "access_token", "accessToken", "refresh_token", "refreshToken", "id_token", "idToken") != "" {
		return model.AccountPoolAuthTypeOfficialOAuth
	}
	if readAuthFileString(metadata, "api_key", "apiKey", "key") != "" {
		return model.AccountPoolAuthTypeAPIKey
	}
	if readAuthFileString(metadata, "client_email", "private_key") != "" {
		return model.AccountPoolAuthTypeServiceAccount
	}
	return model.AccountPoolAuthTypeCustomJSON
}

func normalizeCodexAuthFileMetadata(metadata map[string]any) map[string]any {
	normalized := cloneAuthFileMetadata(metadata)
	copyAuthFileStringField(normalized, "access_token", []string{"accessToken"}, []string{"token_data", "tokenData", "token"}, "access_token", "accessToken")
	copyAuthFileStringField(normalized, "refresh_token", []string{"refreshToken"}, []string{"token_data", "tokenData", "token"}, "refresh_token", "refreshToken")
	copyAuthFileStringField(normalized, "id_token", []string{"idToken"}, []string{"token_data", "tokenData", "token"}, "id_token", "idToken")
	copyAuthFileStringField(normalized, "email", nil, []string{"token_data", "tokenData", "token"}, "email")
	copyAuthFileStringField(normalized, "account_id", []string{"accountId"}, []string{"token_data", "tokenData", "token"}, "account_id", "accountId")
	copyAuthFileAnyField(normalized, "last_refresh", []string{"lastRefresh", "last_refreshed_at", "lastRefreshedAt"}, []string{"token_data", "tokenData", "token"}, "last_refresh", "lastRefresh", "last_refreshed_at", "lastRefreshedAt")
	copyAuthFileAnyField(normalized, "expired", []string{"expire", "expires_at", "expiresAt", "expiry", "expires"}, []string{"token_data", "tokenData", "token"}, "expired", "expire", "expires_at", "expiresAt", "expiry", "expires")

	if strings.TrimSpace(readAuthFileString(normalized, "account_id")) == "" {
		for _, token := range []string{readAuthFileString(normalized, "access_token"), readAuthFileString(normalized, "id_token")} {
			if accountID, ok := ExtractCodexAccountIDFromJWT(token); ok {
				normalized["account_id"] = accountID
				break
			}
		}
	}
	if strings.TrimSpace(readAuthFileString(normalized, "email")) == "" {
		for _, token := range []string{readAuthFileString(normalized, "id_token"), readAuthFileString(normalized, "access_token")} {
			if email, ok := ExtractEmailFromJWT(token); ok {
				normalized["email"] = email
				break
			}
		}
	}
	if readAuthFileString(normalized, "access_token") != "" && readAuthFileString(normalized, "refresh_token") == "" {
		normalized["credential_mode"] = "access_token_only"
		normalized["access_token_only"] = true
		normalized["refreshable"] = false
	} else if readAuthFileString(normalized, "refresh_token") != "" {
		normalized["refreshable"] = true
		delete(normalized, "credential_mode")
		delete(normalized, "access_token_only")
	}
	return normalized
}

func copyAuthFileStringField(metadata map[string]any, canonical string, aliases []string, containerKeys []string, nestedKeys ...string) {
	if value := readAuthFileString(metadata, append([]string{canonical}, aliases...)...); value != "" {
		metadata[canonical] = value
		return
	}
	if value := readAuthFileNestedString(metadata, containerKeys, nestedKeys...); value != "" {
		metadata[canonical] = value
	}
}

func copyAuthFileAnyField(metadata map[string]any, canonical string, aliases []string, containerKeys []string, nestedKeys ...string) {
	for _, key := range append([]string{canonical}, aliases...) {
		if value, ok := metadata[key]; ok && value != nil {
			metadata[canonical] = value
			return
		}
	}
	for _, containerKey := range containerKeys {
		container, ok := metadata[containerKey].(map[string]any)
		if !ok {
			continue
		}
		for _, nestedKey := range nestedKeys {
			if value, ok := container[nestedKey]; ok && value != nil {
				metadata[canonical] = value
				return
			}
		}
	}
}

func readAuthFileNestedString(metadata map[string]any, containerKeys []string, nestedKeys ...string) string {
	for _, containerKey := range containerKeys {
		container, ok := metadata[containerKey].(map[string]any)
		if !ok {
			continue
		}
		if value := readAuthFileString(container, nestedKeys...); value != "" {
			return value
		}
	}
	return ""
}

func inferAuthFileStatus(metadata map[string]any) int {
	if disabled, ok := metadata["disabled"].(bool); ok && disabled {
		return common.ChannelStatusManuallyDisabled
	}
	if value := readAuthFileString(metadata, "status"); value != "" {
		switch strings.ToLower(value) {
		case "disabled", "disable", "inactive", "off":
			return common.ChannelStatusManuallyDisabled
		case "enabled", "enable", "active", "on":
			return common.ChannelStatusEnabled
		}
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			return parsed
		}
	}
	return common.ChannelStatusEnabled
}

func buildAccountPoolAuthFileAttributes(sourcePlatform string, format string, groups []string, proxy string, priority int64, weight int, maxConcurrency int, metadata map[string]any) map[string]string {
	attrs := map[string]string{
		"source": "native_auth_file",
		"format": format,
	}
	if sourcePlatform != "" {
		attrs["source_platform"] = sourcePlatform
	}
	if len(groups) > 0 {
		attrs["account_groups"] = strings.Join(groups, "\n")
		attrs["account_group"] = groups[0]
	}
	if strings.TrimSpace(proxy) != "" {
		attrs["proxy_url"] = strings.TrimSpace(proxy)
	}
	if priority != 0 {
		attrs["priority"] = strconv.FormatInt(priority, 10)
	}
	if weight > 0 && weight != 1 {
		attrs["weight"] = strconv.Itoa(weight)
	}
	if maxConcurrency > 0 {
		attrs["max_concurrency"] = strconv.Itoa(maxConcurrency)
	}
	if value := readAuthFileString(metadata, "credential_mode"); value != "" {
		attrs["credential_mode"] = value
	}
	if refreshable, ok := metadata["refreshable"].(bool); ok {
		attrs["refreshable"] = strconv.FormatBool(refreshable)
	}
	if note := readAuthFileString(metadata, "note"); note != "" {
		attrs["note"] = note
	}
	return attrs
}

func resolveAccountPoolAuthFileGroup(tx *gorm.DB, parsed *ParsedAccountPoolAuthFile, opts AccountPoolAuthFileImportOptions) (*model.AccountPoolGroup, error) {
	if tx == nil || parsed == nil {
		return nil, fmt.Errorf("account pool auth file group context is required")
	}
	if opts.PoolGroupID > 0 {
		group := &model.AccountPoolGroup{}
		if err := tx.Where("id = ?", opts.PoolGroupID).First(group).Error; err != nil {
			return nil, err
		}
		return group, nil
	}
	name := strings.TrimSpace(opts.GroupName)
	if name == "" {
		name = fmt.Sprintf("%s 认证文件", strings.ToUpper(parsed.Provider))
	}
	var group model.AccountPoolGroup
	err := tx.Where("source = ? AND name = ? AND platform = ? AND auth_type = ?", model.AccountPoolGroupSourceNative, name, parsed.Platform, parsed.AuthType).First(&group).Error
	if err == nil {
		return &group, nil
	}
	if err != gorm.ErrRecordNotFound {
		return nil, err
	}
	group = model.AccountPoolGroup{
		Name:     name,
		Platform: parsed.Platform,
		AuthType: parsed.AuthType,
		Source:   model.AccountPoolGroupSourceNative,
		Status:   common.ChannelStatusEnabled,
		Strategy: model.AccountPoolStrategyRoundRobin,
	}
	if err := tx.Create(&group).Error; err != nil {
		return nil, err
	}
	return &group, nil
}

func resolveAccountPoolAuthFileUpdateGroup(tx *gorm.DB, authFile *model.AccountPoolAuthFile, opts AccountPoolAuthFileUpdateOptions) (*model.AccountPoolGroup, error) {
	if opts.PoolGroupID != nil && *opts.PoolGroupID > 0 {
		group := &model.AccountPoolGroup{}
		if err := tx.Where("id = ?", *opts.PoolGroupID).First(group).Error; err != nil {
			return nil, err
		}
		return group, nil
	}
	if opts.GroupName != nil && strings.TrimSpace(*opts.GroupName) != "" && authFile != nil {
		parsed := &ParsedAccountPoolAuthFile{Provider: authFile.Provider, Platform: authFile.Platform, AuthType: authFile.AuthType}
		return resolveAccountPoolAuthFileGroup(tx, parsed, AccountPoolAuthFileImportOptions{GroupName: *opts.GroupName})
	}
	return nil, nil
}

func buildPoolAccountFromParsedAuthFile(group *model.AccountPoolGroup, parsed *ParsedAccountPoolAuthFile, encryptedCredential string, metadataJSON string, attrsJSON string) *model.PoolAccount {
	return &model.PoolAccount{
		PoolGroupId:        group.Id,
		Name:               parsed.Name,
		Platform:           parsed.Platform,
		AuthType:           parsed.AuthType,
		Credentials:        encryptedCredential,
		CredentialSummary:  parsed.CredentialSummary,
		CredentialProvider: parsed.Provider,
		CredentialLabel:    parsed.Name,
		CredentialMetadata: metadataJSON,
		CredentialAttrs:    attrsJSON,
		Status:             parsed.Status,
		Schedulable:        parsed.Status == common.ChannelStatusEnabled,
		Models:             parsed.Models,
		Group:              strings.Join(parsed.AccountGroups, ","),
		Priority:           parsed.Priority,
		Weight:             parsed.Weight,
		MaxConcurrency:     parsed.MaxConcurrency,
		Proxy:              parsed.Proxy,
		BaseURL:            parsed.BaseURL,
		LastRefreshedTime:  common.GetTimestamp(),
	}
}

func buildAccountPoolAuthFileRecord(group *model.AccountPoolGroup, account *model.PoolAccount, parsed *ParsedAccountPoolAuthFile, encryptedContent string, metadataJSON string, attrsJSON string) *model.AccountPoolAuthFile {
	return &model.AccountPoolAuthFile{
		Name:               parsed.Name,
		SourcePlatform:     parsed.SourcePlatform,
		Format:             parsed.Format,
		Provider:           parsed.Provider,
		Platform:           parsed.Platform,
		AuthType:           parsed.AuthType,
		PoolGroupId:        group.Id,
		PoolAccountId:      account.Id,
		Status:             parsed.Status,
		FileDigest:         parsed.FileDigest,
		EncryptedContent:   encryptedContent,
		CredentialSummary:  parsed.CredentialSummary,
		CredentialMetadata: metadataJSON,
		CredentialAttrs:    attrsJSON,
		AccountGroups:      strings.Join(parsed.AccountGroups, ","),
		Models:             parsed.Models,
		Proxy:              parsed.Proxy,
		BaseURL:            parsed.BaseURL,
		Priority:           parsed.Priority,
		Weight:             parsed.Weight,
		MaxConcurrency:     parsed.MaxConcurrency,
		LastImportedTime:   common.GetTimestamp(),
	}
}

func buildAuthFilePatchUpdates(authFile *model.AccountPoolAuthFile, opts AccountPoolAuthFileUpdateOptions) map[string]interface{} {
	updates := map[string]interface{}{}
	if opts.Name != nil && strings.TrimSpace(*opts.Name) != "" {
		updates["name"] = strings.TrimSpace(*opts.Name)
	}
	if opts.Provider != nil && strings.TrimSpace(*opts.Provider) != "" {
		updates["provider"] = normalizeAccountPoolAuthProvider(*opts.Provider)
	}
	if opts.Platform != nil && strings.TrimSpace(*opts.Platform) != "" {
		updates["platform"] = normalizeAccountPoolAuthProvider(*opts.Platform)
	}
	if opts.AuthType != nil && strings.TrimSpace(*opts.AuthType) != "" {
		updates["auth_type"] = normalizeAccountPoolAuthType(*opts.AuthType)
	}
	if opts.AccountGroups != nil {
		updates["account_groups"] = strings.Join(normalizeAccountPoolAuthGroups(*opts.AccountGroups), ",")
	}
	if opts.Models != nil {
		updates["models"] = normalizeAccountPoolAuthCSV(*opts.Models)
	}
	if opts.Proxy != nil {
		updates["proxy"] = strings.TrimSpace(*opts.Proxy)
	}
	if opts.BaseURL != nil {
		updates["base_url"] = strings.TrimSpace(*opts.BaseURL)
	}
	if opts.Priority != nil {
		updates["priority"] = *opts.Priority
	}
	if opts.Weight != nil {
		weight := *opts.Weight
		if weight <= 0 {
			weight = 1
		}
		updates["weight"] = weight
	}
	if opts.MaxConcurrency != nil {
		maxConcurrency := *opts.MaxConcurrency
		if maxConcurrency < 0 {
			maxConcurrency = 0
		}
		updates["max_concurrency"] = maxConcurrency
	}
	if opts.Status != nil && *opts.Status > 0 {
		updates["status"] = *opts.Status
	}
	_ = authFile
	return updates
}

func buildLinkedPoolAccountPatchUpdates(opts AccountPoolAuthFileUpdateOptions) map[string]interface{} {
	updates := map[string]interface{}{}
	if opts.Name != nil && strings.TrimSpace(*opts.Name) != "" {
		updates["name"] = strings.TrimSpace(*opts.Name)
		updates["credential_label"] = strings.TrimSpace(*opts.Name)
	}
	if opts.Provider != nil && strings.TrimSpace(*opts.Provider) != "" {
		updates["credential_provider"] = normalizeAccountPoolAuthProvider(*opts.Provider)
	}
	if opts.Platform != nil && strings.TrimSpace(*opts.Platform) != "" {
		updates["platform"] = normalizeAccountPoolAuthProvider(*opts.Platform)
	}
	if opts.AuthType != nil && strings.TrimSpace(*opts.AuthType) != "" {
		updates["auth_type"] = normalizeAccountPoolAuthType(*opts.AuthType)
	}
	if opts.AccountGroups != nil {
		updates["group"] = strings.Join(normalizeAccountPoolAuthGroups(*opts.AccountGroups), ",")
	}
	if opts.Models != nil {
		updates["models"] = normalizeAccountPoolAuthCSV(*opts.Models)
	}
	if opts.Proxy != nil {
		updates["proxy"] = strings.TrimSpace(*opts.Proxy)
	}
	if opts.BaseURL != nil {
		updates["base_url"] = strings.TrimSpace(*opts.BaseURL)
	}
	if opts.Priority != nil {
		updates["priority"] = *opts.Priority
	}
	if opts.Weight != nil {
		weight := *opts.Weight
		if weight <= 0 {
			weight = 1
		}
		updates["weight"] = weight
	}
	if opts.MaxConcurrency != nil {
		maxConcurrency := *opts.MaxConcurrency
		if maxConcurrency < 0 {
			maxConcurrency = 0
		}
		updates["max_concurrency"] = maxConcurrency
	}
	if opts.Status != nil && *opts.Status > 0 {
		updates["status"] = *opts.Status
		updates["schedulable"] = *opts.Status == common.ChannelStatusEnabled
		if *opts.Status == common.ChannelStatusEnabled {
			updates["unavailable"] = false
			updates["status_message"] = ""
			updates["last_error"] = ""
			updates["rate_limited_until"] = 0
			updates["overload_until"] = 0
			updates["temp_disabled_until"] = 0
			updates["next_retry_time"] = 0
		}
	}
	return updates
}

func authFileImportOptionsFromUpdate(authFile *model.AccountPoolAuthFile, opts AccountPoolAuthFileUpdateOptions) AccountPoolAuthFileImportOptions {
	result := AccountPoolAuthFileImportOptions{
		Name:        authFile.Name,
		PoolGroupID: authFile.PoolGroupId,
		Provider:    authFile.Provider,
		Platform:    authFile.Platform,
		AuthType:    authFile.AuthType,
		Models:      authFile.Models,
		Proxy:       authFile.Proxy,
		BaseURL:     authFile.BaseURL,
	}
	if authFile.MaxConcurrency > 0 {
		maxConcurrency := authFile.MaxConcurrency
		result.MaxConcurrency = &maxConcurrency
	}
	if authFile.AccountGroups != "" {
		result.AccountGroups = normalizeAccountPoolAuthGroups(authFile.AccountGroups)
	}
	if opts.Name != nil {
		result.Name = *opts.Name
	}
	if opts.PoolGroupID != nil {
		result.PoolGroupID = *opts.PoolGroupID
	}
	if opts.GroupName != nil {
		result.GroupName = *opts.GroupName
	}
	if opts.Provider != nil {
		result.Provider = *opts.Provider
	}
	if opts.Platform != nil {
		result.Platform = *opts.Platform
	}
	if opts.AuthType != nil {
		result.AuthType = *opts.AuthType
	}
	if opts.AccountGroups != nil {
		result.AccountGroups = *opts.AccountGroups
	}
	if opts.Models != nil {
		result.Models = *opts.Models
	}
	if opts.Proxy != nil {
		result.Proxy = *opts.Proxy
	}
	if opts.BaseURL != nil {
		result.BaseURL = opts.BaseURL
	}
	result.Priority = opts.Priority
	result.Weight = opts.Weight
	result.MaxConcurrency = opts.MaxConcurrency
	result.Status = opts.Status
	return result
}

func encodeAuthFileMetadata(metadata map[string]any, attrs map[string]string) (string, string, error) {
	metadataBytes, err := common.Marshal(metadata)
	if err != nil {
		return "", "", err
	}
	attrsBytes, err := common.Marshal(attrs)
	if err != nil {
		return "", "", err
	}
	return string(metadataBytes), string(attrsBytes), nil
}

func marshalStableAuthFileJSON(metadata map[string]any) (string, error) {
	data, err := common.Marshal(metadata)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func cloneAuthFileMetadata(metadata map[string]any) map[string]any {
	cloned := make(map[string]any, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}

func readAuthFileString(metadata map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := metadata[key]; ok && value != nil {
			switch v := value.(type) {
			case string:
				if trimmed := strings.TrimSpace(v); trimmed != "" {
					return trimmed
				}
			case fmt.Stringer:
				if trimmed := strings.TrimSpace(v.String()); trimmed != "" {
					return trimmed
				}
			}
		}
	}
	return ""
}

func optionalAuthFileString(metadata map[string]any, keys ...string) *string {
	value := readAuthFileString(metadata, keys...)
	if value == "" {
		return nil
	}
	return &value
}

func readAuthFileCSV(metadata map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := metadata[key]; ok {
			groups := normalizeAccountPoolAuthGroups(value)
			if len(groups) > 0 {
				return strings.Join(groups, ",")
			}
		}
	}
	return ""
}

func readAuthFileGroups(metadata map[string]any, keys ...string) []string {
	values := make([]any, 0, len(keys))
	for _, key := range keys {
		if value, ok := metadata[key]; ok {
			values = append(values, value)
		}
	}
	return normalizeAccountPoolAuthGroups(values...)
}

func normalizeAccountPoolAuthGroups(values ...any) []string {
	seen := map[string]struct{}{}
	groups := make([]string, 0, len(values))
	add := func(value string) {
		for _, part := range splitAccountPoolAuthGroupString(value) {
			if _, ok := seen[part]; ok {
				continue
			}
			seen[part] = struct{}{}
			groups = append(groups, part)
		}
	}
	for _, value := range values {
		switch v := value.(type) {
		case string:
			add(v)
		case []string:
			for _, item := range v {
				add(item)
			}
		case []any:
			for _, item := range v {
				if text, ok := item.(string); ok {
					add(text)
				}
			}
		}
	}
	return groups
}

func splitAccountPoolAuthGroupString(value string) []string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	if strings.HasPrefix(trimmed, "[") {
		var decoded []string
		if err := common.UnmarshalJsonStr(trimmed, &decoded); err == nil {
			return normalizeAccountPoolAuthGroups(decoded)
		}
	}
	parts := strings.FieldsFunc(trimmed, func(r rune) bool {
		return r == ',' || r == '\n' || r == ';'
	})
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.Join(strings.Fields(strings.TrimSpace(part)), " ")
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}

func normalizeAccountPoolAuthCSV(value string) string {
	return strings.Join(normalizeAccountPoolAuthGroups(value), ",")
}

func readAuthFileInt64(metadata map[string]any, fallback int64, keys ...string) int64 {
	for _, key := range keys {
		value, ok := metadata[key]
		if !ok || value == nil {
			continue
		}
		switch v := value.(type) {
		case float64:
			return int64(v)
		case int64:
			return v
		case int:
			return int64(v)
		case string:
			if parsed, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
				return parsed
			}
		}
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func mergeStringAnyMaps(left map[string]interface{}, right map[string]interface{}) map[string]interface{} {
	if left == nil {
		left = map[string]interface{}{}
	}
	for key, value := range right {
		left[key] = value
	}
	return left
}

// authFileNowString 返回统一的时间字符串，用于补齐缺失的 last_refresh。
// 当前未强制写入所有凭据，只在后续 provider 需要时保留扩展入口。
func authFileNowString() string {
	return time.Now().Format(time.RFC3339)
}
