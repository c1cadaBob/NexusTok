// account_pool_cliproxy.go 实现了与 CLIProxyAPI (CPAMC) Sidecar 的集成。
// 负责将 CLIProxyAPI 管理的账号分组同步为 NexusTok 内部的账号池组镜像，
// 查询 Sidecar 的分组统计信息，以及构造发往 Sidecar 的请求头。
// CLIProxyAPI 是一个外部的账号管理服务，本模块将其账号分组映射到 NexusTok 的账号池体系中。
package service

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"gorm.io/gorm"
)

const (
	// AccountPoolCLIProxyGroupHeader 是发往 CLIProxyAPI Sidecar 时用于指定账号分组的 HTTP 请求头名称。
	AccountPoolCLIProxyGroupHeader = "X-NexusTok-Account-Pool-Group"

	defaultAccountPoolCLIProxyURL           = "http://127.0.0.1:8317"                    // CLIProxyAPI Sidecar 的默认地址
	defaultAccountPoolCLIProxyManagementKey = "nexustok-account-pool-local"               // 默认的管理 API 密钥
	defaultAccountPoolCLIProxyRelayKey      = "nexustok-account-pool-relay-local"         // 默认的 Relay API 密钥
)

// cliProxyAuthFilesResponse 表示 CLIProxyAPI /v0/management/auth-files 接口的响应结构。
type cliProxyAuthFilesResponse struct {
	Files []cliProxyAuthFileEntry `json:"files"` // 认证文件条目列表
}

// cliProxyAuthFileEntry 表示 CLIProxyAPI 中单个认证文件的信息。
// 每个条目对应一个外部账号的配置文件。
type cliProxyAuthFileEntry struct {
	Name          string   `json:"name"`           // 认证文件名
	Type          string   `json:"type"`           // 账号类型（如 codex）
	Provider      string   `json:"provider"`       // 提供者名称
	Disabled      bool     `json:"disabled"`       // 是否已禁用
	Unavailable   bool     `json:"unavailable"`    // 是否不可用
	AccountGroup  string   `json:"account_group"`  // 单一分组名（旧格式）
	AccountGroups []string `json:"account_groups"` // 多分组名列表（新格式）
}

// cliproxyGroupAggregate 表示 CLIProxyAPI 账号分组的聚合统计信息。
// 用于将多个认证文件条目按分组名进行汇总。
type cliproxyGroupAggregate struct {
	name        string         // 分组名称
	platforms   map[string]int // 各平台的账号数量统计
	total       int64          // 账号总数
	enabled     int64          // 已启用的账号数
	disabled    int64          // 已禁用的账号数
	unavailable int64          // 不可用的账号数
}

// AccountPoolCLIProxyURL 返回 NexusTok 访问内部 CLIProxyAPI sidecar 的地址。
func AccountPoolCLIProxyURL() string {
	value := strings.TrimRight(strings.TrimSpace(os.Getenv("ACCOUNT_POOL_CLI_PROXY_URL")), "/")
	if value == "" {
		value = defaultAccountPoolCLIProxyURL
	}
	return value
}

// AccountPoolCLIProxyRelayKey 返回只在后端和容器网络内使用的 relay key。
func AccountPoolCLIProxyRelayKey() string {
	value := strings.TrimSpace(os.Getenv("ACCOUNT_POOL_CLI_PROXY_RELAY_KEY"))
	if value == "" {
		value = defaultAccountPoolCLIProxyRelayKey
	}
	return value
}

// AccountPoolCLIProxyManagementKey 返回只在后端管理代理中使用的 management key。
func AccountPoolCLIProxyManagementKey() string {
	key := strings.TrimSpace(os.Getenv("ACCOUNT_POOL_CLI_PROXY_MANAGEMENT_KEY"))
	if key == "" {
		key = strings.TrimSpace(os.Getenv("ACCOUNT_POOL_MANAGEMENT_KEY"))
	}
	if key == "" {
		key = defaultAccountPoolCLIProxyManagementKey
	}
	return key
}

// SyncCLIProxyAccountGroups 将 CPAMC/CLIProxyAPI 管理分组同步为 NexusTok 可引用的账号池组镜像。
func SyncCLIProxyAccountGroups(ctx context.Context) error {
	entries, err := fetchCLIProxyAuthFiles(ctx)
	if err != nil {
		return err
	}
	aggregates := aggregateCLIProxyGroups(entries)
	return upsertCLIProxyAccountGroups(aggregates)
}

// fetchCLIProxyAuthFiles 从 CLIProxyAPI Sidecar 获取认证文件列表。
// 向 /v0/management/auth-files 端点发送 GET 请求，使用 management key 鉴权。
//
// 参数：
//   - ctx: 请求上下文
//
// 返回：
//   - []cliProxyAuthFileEntry: 认证文件条目列表
//   - error: 请求或解析错误
func fetchCLIProxyAuthFiles(ctx context.Context) ([]cliProxyAuthFileEntry, error) {
	targetURL, err := url.JoinPath(AccountPoolCLIProxyURL(), "/v0/management/auth-files")
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+AccountPoolCLIProxyManagementKey())

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("CLIProxyAPI auth-files 返回状态码 %d", resp.StatusCode)
	}

	var payload cliProxyAuthFilesResponse
	if err := common.DecodeJson(resp.Body, &payload); err != nil {
		return nil, err
	}
	return payload.Files, nil
}

// aggregateCLIProxyGroups 将认证文件条目按分组名聚合统计。
// 每个条目可能属于多个分组（通过 AccountGroups 或 AccountGroup 指定）。
// 统计每个分组的总数、启用数、禁用数、不可用数和平台分布。
//
// 参数：
//   - entries: 认证文件条目列表
//
// 返回：
//   - map[string]*cliproxyGroupAggregate: 分组名 -> 聚合统计信息
func aggregateCLIProxyGroups(entries []cliProxyAuthFileEntry) map[string]*cliproxyGroupAggregate {
	aggregates := map[string]*cliproxyGroupAggregate{}
	for _, entry := range entries {
		groups := normalizeCLIProxyGroupValues(entry.AccountGroup, strings.Join(entry.AccountGroups, "\n"))
		for _, groupName := range groups {
			aggregate := aggregates[groupName]
			if aggregate == nil {
				aggregate = &cliproxyGroupAggregate{
					name:      groupName,
					platforms: map[string]int{},
				}
				aggregates[groupName] = aggregate
			}
			aggregate.total++
			if entry.Disabled {
				aggregate.disabled++
			} else {
				aggregate.enabled++
			}
			if entry.Unavailable {
				aggregate.unavailable++
			}
			if platform := normalizeCLIProxyPlatform(entry.Provider, entry.Type); platform != "" {
				aggregate.platforms[platform]++
			}
		}
	}
	return aggregates
}

// upsertCLIProxyAccountGroups 将聚合后的分组信息同步到 NexusTok 数据库。
// 在事务中执行：新增或更新活跃分组，然后禁用不在同步列表中的旧镜像分组。
//
// 参数：
//   - aggregates: 分组名 -> 聚合统计信息
//
// 返回：
//   - error: 数据库操作错误
func upsertCLIProxyAccountGroups(aggregates map[string]*cliproxyGroupAggregate) error {
	now := common.GetTimestamp()
	return model.DB.Transaction(func(tx *gorm.DB) error {
		names := make([]string, 0, len(aggregates))
		for name := range aggregates {
			names = append(names, name)
		}
		sort.Strings(names)
		seen := make(map[string]struct{}, len(names))
		for _, name := range names {
			aggregate := aggregates[name]
			if aggregate == nil {
				continue
			}
			seen[name] = struct{}{}
			group := &model.AccountPoolGroup{
				Name:        aggregate.name,
				Platform:    aggregate.primaryPlatform(),
				AuthType:    model.AccountPoolAuthTypeOfficialOAuth,
				Source:      model.AccountPoolGroupSourceCLIProxyAPI,
				ExternalKey: aggregate.name,
				Status:      common.ChannelStatusEnabled,
				Strategy:    model.AccountPoolStrategyRoundRobin,
				UpdatedTime: now,
			}
			var existing model.AccountPoolGroup
			err := tx.Where("source = ? AND external_group_key = ?", model.AccountPoolGroupSourceCLIProxyAPI, aggregate.name).First(&existing).Error
			if err == nil {
				if updateErr := tx.Model(&existing).Updates(map[string]interface{}{
					"name":               group.Name,
					"platform":           group.Platform,
					"auth_type":          group.AuthType,
					"status":             group.Status,
					"strategy":           group.Strategy,
					"updated_time":       group.UpdatedTime,
					"external_group_key": group.ExternalKey,
				}).Error; updateErr != nil {
					return updateErr
				}
				continue
			}
			if err != gorm.ErrRecordNotFound {
				return err
			}
			if createErr := tx.Create(group).Error; createErr != nil {
				return createErr
			}
		}
		return disableMissingCLIProxyAccountGroups(tx, seen, now)
	})
}

// disableMissingCLIProxyAccountGroups 禁用不在当前同步列表中的旧镜像分组。
// 遍历数据库中所有来源为 CLIProxyAPI 的分组，将不在 activeGroups 中的标记为手动禁用。
//
// 参数：
//   - tx: 数据库事务
//   - activeGroups: 当前活跃的分组名集合
//   - now: 当前时间戳
//
// 返回：
//   - error: 数据库操作错误
func disableMissingCLIProxyAccountGroups(tx *gorm.DB, activeGroups map[string]struct{}, now int64) error {
	var groups []model.AccountPoolGroup
	if err := tx.Where("source = ?", model.AccountPoolGroupSourceCLIProxyAPI).Find(&groups).Error; err != nil {
		return err
	}
	missingIds := make([]int, 0, len(groups))
	for _, group := range groups {
		groupKey := strings.TrimSpace(group.ExternalKey)
		if groupKey == "" {
			groupKey = strings.TrimSpace(group.Name)
		}
		if groupKey == "" {
			continue
		}
		if _, ok := activeGroups[groupKey]; ok {
			continue
		}
		missingIds = append(missingIds, group.Id)
	}
	if len(missingIds) == 0 {
		return nil
	}
	return tx.Model(&model.AccountPoolGroup{}).Where("id IN ?", missingIds).Updates(map[string]interface{}{
		"status":       common.ChannelStatusManuallyDisabled,
		"updated_time": now,
	}).Error
}

// primaryPlatform 推断分组的主要平台。
// 如果分组中只有一个平台，返回该平台名称；
// 如果有多个平台，返回 "cliproxyapi"（混合平台标识）；
// 如果没有平台信息，也返回 "cliproxyapi"。
func (aggregate *cliproxyGroupAggregate) primaryPlatform() string {
	if aggregate == nil || len(aggregate.platforms) == 0 {
		return "cliproxyapi"
	}
	type platformCount struct {
		platform string
		count    int
	}
	items := make([]platformCount, 0, len(aggregate.platforms))
	for platform, count := range aggregate.platforms {
		items = append(items, platformCount{platform: platform, count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].count != items[j].count {
			return items[i].count > items[j].count
		}
		return items[i].platform < items[j].platform
	})
	if len(items) > 1 {
		return "cliproxyapi"
	}
	return items[0].platform
}

// normalizeCLIProxyPlatform 从多个候选值中提取第一个非空的平台名称。
// 所有值会被标准化为小写并去除首尾空格。
func normalizeCLIProxyPlatform(values ...string) string {
	for _, value := range values {
		platform := strings.ToLower(strings.TrimSpace(value))
		if platform != "" {
			return platform
		}
	}
	return ""
}

// normalizeCLIProxyGroupValues 从多个输入值中解析并去重分组名列表。
// 支持的分隔符：换行符、逗号、分号。
// 支持 JSON 数组格式输入（如 ["group1","group2"]）。
// 分组名会被规范化为单空格分隔的形式。
func normalizeCLIProxyGroupValues(values ...string) []string {
	seen := map[string]struct{}{}
	groups := make([]string, 0, len(values))
	add := func(value string) {
		for _, part := range strings.FieldsFunc(value, func(r rune) bool {
			return r == '\n' || r == '\r' || r == ',' || r == ';'
		}) {
			groupName := strings.Join(strings.Fields(strings.TrimSpace(part)), " ")
			if groupName == "" {
				continue
			}
			if _, ok := seen[groupName]; ok {
				continue
			}
			seen[groupName] = struct{}{}
			groups = append(groups, groupName)
		}
	}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "[") {
			var decoded []string
			if err := common.UnmarshalJsonStr(trimmed, &decoded); err == nil {
				for _, item := range decoded {
					add(item)
				}
				continue
			}
		}
		add(trimmed)
	}
	return groups
}

// CLIProxyGroupStats 读取当前 sidecar 分组统计，列表接口用于展示镜像组实时状态。
func CLIProxyGroupStats(ctx context.Context) (map[string]map[string]int64, error) {
	entries, err := fetchCLIProxyAuthFiles(ctx)
	if err != nil {
		return nil, err
	}
	aggregates := aggregateCLIProxyGroups(entries)
	stats := make(map[string]map[string]int64, len(aggregates))
	for name, aggregate := range aggregates {
		if aggregate == nil {
			continue
		}
		stats[name] = map[string]int64{
			"total":       aggregate.total,
			"enabled":     aggregate.enabled,
			"disabled":    aggregate.disabled,
			"cooldown":    0,
			"unavailable": aggregate.unavailable,
		}
	}
	return stats, nil
}

// BuildCLIProxyGroupHeaderOverride 构造发往 sidecar 的组过滤请求头。
func BuildCLIProxyGroupHeaderOverride(group *model.AccountPoolGroup) map[string]interface{} {
	groupKey := ""
	if group != nil {
		groupKey = strings.TrimSpace(group.ExternalKey)
		if groupKey == "" {
			groupKey = strings.TrimSpace(group.Name)
		}
	}
	if groupKey == "" {
		return map[string]interface{}{}
	}
	return map[string]interface{}{
		AccountPoolCLIProxyGroupHeader: groupKey,
	}
}

// MergeHeaderOverrides 以账号池内部调度头覆盖渠道侧同名配置，避免用户误覆盖组过滤。
func MergeHeaderOverrides(base map[string]interface{}, overrides map[string]interface{}) map[string]interface{} {
	merged := make(map[string]interface{}, len(base)+len(overrides))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range overrides {
		merged[key] = value
	}
	return merged
}

// IsCLIProxyAccountPoolGroup 判断账号池分组是否来源于 CLIProxyAPI。
// 通过比较分组的 Source 字段与 CLIProxyAPI 标识实现。
func IsCLIProxyAccountPoolGroup(group *model.AccountPoolGroup) bool {
	return group != nil && strings.EqualFold(strings.TrimSpace(group.Source), model.AccountPoolGroupSourceCLIProxyAPI)
}

// AccountPoolSidecarUnavailableError 将错误包装为 Sidecar 不可用的错误信息。
// 用于统一 CLIProxyAPI 同步失败的错误格式。
func AccountPoolSidecarUnavailableError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("同步 CLIProxyAPI 账号组失败: %w", err)
}
