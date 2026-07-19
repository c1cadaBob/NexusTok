package upstreamaccount

import (
	"fmt"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
	"gorm.io/gorm"
)

// CreateRequest 表示根据预览快照创建渠道及渠道账号的请求。
type CreateRequest struct {
	PreviewID         string                `json:"preview_id"`
	Channel           ChannelCreateConfig   `json:"channel"`
	Accounts          []AccountCreateConfig `json:"accounts"`
	ApplySuggested    bool                  `json:"apply_suggested"`
	DisableMissingKey bool                  `json:"disable_missing_key"`
}

// ChannelCreateConfig 是预览创建时允许用户配置的渠道字段。
type ChannelCreateConfig struct {
	Name              string  `json:"name"`
	Type              int     `json:"type"`
	BaseURL           *string `json:"base_url"`
	Models            string  `json:"models"`
	Group             string  `json:"group"`
	Priority          *int64  `json:"priority"`
	Weight            *uint   `json:"weight"`
	TestModel         *string `json:"test_model"`
	AutoBan           *int    `json:"auto_ban"`
	Status            int     `json:"status"`
	Tag               *string `json:"tag"`
	Remark            *string `json:"remark"`
	Setting           *string `json:"setting"`
	ParamOverride     *string `json:"param_override"`
	HeaderOverride    *string `json:"header_override"`
	StatusCodeMapping *string `json:"status_code_mapping"`
	Other             string  `json:"other"`
	OtherSettings     string  `json:"settings"`
}

// AccountCreateConfig 是单个同步密钥在 NexusTok 中的配置覆盖。
type AccountCreateConfig struct {
	SyncID             string  `json:"sync_id"`
	ExternalID         string  `json:"external_id"`
	Name               string  `json:"name"`
	Enabled            *bool   `json:"enabled"`
	Models             string  `json:"models"`
	Group              string  `json:"group"`
	Priority           *int64  `json:"priority"`
	Weight             *int    `json:"weight"`
	BaseURL            *string `json:"base_url"`
	OpenAIOrganization *string `json:"openai_organization"`
	Other              string  `json:"other"`
	Setting            *string `json:"setting"`
	OtherSettings      string  `json:"settings"`
	ModelMapping       *string `json:"model_mapping"`
	ParamOverride      *string `json:"param_override"`
	HeaderOverride     *string `json:"header_override"`
	StatusCodeMapping  *string `json:"status_code_mapping"`
	MaxConcurrency     int     `json:"max_concurrency"`
}

// CreateResult 表示创建结果。
type CreateResult struct {
	ChannelID int `json:"channel_id"`
	Created   int `json:"created"`
	Skipped   int `json:"skipped"`
}

// CreateFromPreview 根据短期预览快照创建渠道和渠道内账号池。
func CreateFromPreview(req CreateRequest) (*CreateResult, error) {
	record, err := ConsumePreviewRecord(req.PreviewID)
	if err != nil {
		return nil, err
	}
	if record.Snapshot == nil {
		return nil, fmt.Errorf("预览快照为空，请重新同步")
	}
	channel, accounts, err := buildChannelAndAccounts(record.Snapshot, req)
	if err != nil {
		return nil, err
	}
	created := 0
	skipped := 0
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(channel).Error; err != nil {
			return err
		}
		for i := range accounts {
			accounts[i].ChannelId = channel.Id
		}
		if len(accounts) > 0 {
			if err := tx.Create(&accounts).Error; err != nil {
				return err
			}
			created = len(accounts)
		}
		if err := model.SyncChannelAccountPoolCapabilities(channel.Id, tx); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}
	model.InitChannelCache()
	return &CreateResult{ChannelID: channel.Id, Created: created, Skipped: skipped}, nil
}

func buildChannelAndAccounts(snapshot *Snapshot, req CreateRequest) (*model.Channel, []model.ChannelAccount, error) {
	if strings.TrimSpace(req.Channel.Name) == "" {
		return nil, nil, fmt.Errorf("渠道名称不能为空")
	}
	ApplySyncIDs(snapshot)
	channelType := req.Channel.Type
	if channelType <= 0 {
		// new-api 和 sub2api 都暴露 OpenAI 兼容接口。账号同步创建允许管理员先不选择类型，
		// 后端默认按 OpenAI 兼容渠道保存，避免空类型渠道进入后续调度路径。
		channelType = constant.ChannelTypeOpenAI
	}
	models := strings.TrimSpace(req.Channel.Models)
	if models == "" {
		models = inferModelsFromKeys(snapshot.Keys)
	}
	group := strings.TrimSpace(req.Channel.Group)
	if group == "" {
		group = inferGroupFromKeys(snapshot.Keys)
	}
	if group == "" {
		group = "default"
	}
	status := req.Channel.Status
	if status <= 0 {
		status = common.ChannelStatusEnabled
	}
	priority := req.Channel.Priority
	if priority == nil {
		value := int64(0)
		priority = &value
	}
	weight := req.Channel.Weight
	if weight == nil {
		value := uint(0)
		weight = &value
	}
	autoBan := req.Channel.AutoBan
	if autoBan == nil {
		value := 1
		autoBan = &value
	}
	channel := &model.Channel{
		Type:               channelType,
		Key:                constant.ChannelCredentialModeAccountPool,
		Name:               strings.TrimSpace(req.Channel.Name),
		Weight:             weight,
		CreatedTime:        common.GetTimestamp(),
		BaseURL:            normalizeSyncedChannelBaseURL(req.Channel.BaseURL, snapshot),
		Balance:            balanceValue(snapshot.Balance),
		BalanceUpdatedTime: common.GetTimestamp(),
		Models:             models,
		Group:              group,
		UsedQuota:          usedQuotaValue(snapshot.Balance),
		Priority:           priority,
		AutoBan:            autoBan,
		Status:             status,
		TestModel:          req.Channel.TestModel,
		Tag:                req.Channel.Tag,
		Remark:             req.Channel.Remark,
		Setting:            req.Channel.Setting,
		ParamOverride:      req.Channel.ParamOverride,
		HeaderOverride:     req.Channel.HeaderOverride,
		StatusCodeMapping:  req.Channel.StatusCodeMapping,
		Other:              req.Channel.Other,
		OtherSettings:      mergeChannelSyncMetadata(req.Channel.OtherSettings, snapshot),
		ChannelInfo: model.ChannelInfo{
			CredentialMode:      constant.ChannelCredentialModeAccountPool,
			AccountPoolEnabled:  true,
			AccountPoolMode:     constant.ChannelAccountPoolModePolling,
			AccountPoolFallback: false,
			IsMultiKey:          false,
		},
	}
	accounts, err := buildAccounts(snapshot, req, models, group)
	if err != nil {
		return nil, nil, err
	}
	// 渠道顶层 models/group 是路由能力表的来源，只能由最终启用的同步账号贡献。
	// 如果把已跳过或已禁用的 key 也汇总进去，能力表会暴露实际没有可用账号的模型，
	// 请求进入 Relay 后才因账号不可用而失败，造成“保存成功但调度异常”的误判。
	models, group = summarizeEnabledAccountCapabilities(accounts, models, group)
	channel.Models = models
	channel.Group = group
	return channel, accounts, nil
}

// normalizeSyncedChannelBaseURL 统一选择同步渠道的真实调用地址。
//
// 上游账号同步的预览阶段会通过 newHTTPClient/平台适配器拿到规范后的站点根地址：
// new-api 会去掉尾部 `/`，sub2api 会把常见前端路由 `/login`、`/dashboard` 等剥离。
// 创建渠道时优先使用该快照地址，可以避免前端仍把用户粘贴的页面地址写入渠道
// `base_url`，导致后续 Relay 拼出 `//v1/...` 或 `/login/v1/...` 这类错误上游路径。
func normalizeSyncedChannelBaseURL(value *string, snapshot *Snapshot) *string {
	if snapshot != nil {
		baseURL := normalizeSyncMetadataBaseURL(snapshot.Platform, snapshot.BaseURL)
		if baseURL != "" {
			return &baseURL
		}
	}
	if value == nil {
		return nil
	}
	baseURL := strings.TrimRight(strings.TrimSpace(*value), "/")
	if baseURL == "" {
		return nil
	}
	return &baseURL
}

// normalizeAccountConfigBaseURL 规整单个同步 key 的本地 API 地址覆盖。
//
// 账号级 BaseURL 优先级高于渠道级 BaseURL，因此这里也需要去掉尾部 `/`。空字符串按
// 未设置处理，继续回退到渠道级调用地址，避免把空指针以外的空值写入数据库后造成误判。
func normalizeAccountConfigBaseURL(value *string) *string {
	if value == nil {
		return nil
	}
	baseURL := strings.TrimRight(strings.TrimSpace(*value), "/")
	if baseURL == "" {
		return nil
	}
	return &baseURL
}

func buildAccounts(snapshot *Snapshot, req CreateRequest, defaultModels string, defaultGroup string) ([]model.ChannelAccount, error) {
	configs := accountConfigBySyncID(req.Accounts)
	accounts := make([]model.ChannelAccount, 0, len(snapshot.Keys))
	for _, key := range snapshot.Keys {
		if strings.TrimSpace(key.Key) == "" {
			return nil, fmt.Errorf("预览快照中的密钥 %s 缺少完整 key，请重新同步", key.Name)
		}
		config := configs[accountConfigLookupID(key)]
		if config.Enabled != nil && !*config.Enabled {
			continue
		}
		status := common.ChannelStatusEnabled
		if key.Status > 0 && key.Status != common.ChannelStatusEnabled {
			status = key.Status
		}
		name := strings.TrimSpace(config.Name)
		if name == "" {
			name = strings.TrimSpace(key.Name)
		}
		if name == "" {
			name = key.MaskedKey
		}
		models, hasModels := explicitSyncValue(config.Models)
		if !hasModels {
			models = strings.Join(key.Models, ",")
		}
		if models == "" {
			models = defaultModels
		}
		group, hasGroup := explicitSyncValue(config.Group)
		if !hasGroup {
			group = firstNonEmpty(key.GroupName, key.GroupID, defaultGroup)
		}
		priority := int64(0)
		if req.ApplySuggested {
			priority = key.SuggestedPriority
		}
		if config.Priority != nil {
			priority = *config.Priority
		}
		weight := 0
		if req.ApplySuggested {
			weight = key.SuggestedWeight
		}
		if config.Weight != nil {
			weight = *config.Weight
		}
		accounts = append(accounts, model.ChannelAccount{
			Name:               name,
			Key:                key.Key,
			Status:             status,
			Models:             models,
			Group:              group,
			Priority:           priority,
			Weight:             weight,
			UsedQuota:          usdToQuotaInt64(key.QuotaUsedUSD),
			BaseURL:            normalizeAccountConfigBaseURL(config.BaseURL),
			OpenAIOrganization: config.OpenAIOrganization,
			Other:              config.Other,
			Setting:            config.Setting,
			OtherSettings:      mergeAccountSyncMetadata(config.OtherSettings, snapshot, key),
			ModelMapping:       config.ModelMapping,
			ParamOverride:      config.ParamOverride,
			HeaderOverride:     config.HeaderOverride,
			StatusCodeMapping:  config.StatusCodeMapping,
			MaxConcurrency:     config.MaxConcurrency,
		})
	}
	if len(accounts) == 0 {
		return nil, fmt.Errorf("没有可创建的同步密钥")
	}
	return accounts, nil
}

// explicitSyncValue 解析同步表单里可覆盖快照的非空字符串值。
//
// 上游账号同步的创建/刷新请求目前使用 string 字段，Go 反序列化后无法区分
// “字段缺失”和“字段显式传入空串”。因此创建和刷新语义保持保守：空串代表
// 未覆盖，继续回退到上游快照或渠道级默认值；已同步账号的本地编辑保存则走
// ChannelAccount 更新接口，该接口可根据原始 JSON 字段集合支持显式清空。
func explicitSyncValue(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", false
	}
	return trimmed, true
}

func inferModelsFromKeys(keys []SyncedKey) string {
	seen := map[string]struct{}{}
	result := make([]string, 0)
	for _, key := range keys {
		for _, modelName := range key.Models {
			modelName = strings.TrimSpace(modelName)
			if modelName == "" {
				continue
			}
			if _, ok := seen[modelName]; ok {
				continue
			}
			seen[modelName] = struct{}{}
			result = append(result, modelName)
		}
	}
	return strings.Join(result, ",")
}

func inferGroupFromKeys(keys []SyncedKey) string {
	seen := map[string]struct{}{}
	result := make([]string, 0)
	for _, key := range keys {
		group := strings.TrimSpace(firstNonEmpty(key.GroupName, key.GroupID))
		if group == "" {
			continue
		}
		if _, ok := seen[group]; ok {
			continue
		}
		seen[group] = struct{}{}
		result = append(result, group)
	}
	return strings.Join(result, ",")
}

func summarizeEnabledAccountCapabilities(accounts []model.ChannelAccount, fallbackModels string, fallbackGroup string) (string, string) {
	hasEnabledAccount := false
	for _, account := range accounts {
		if account.Status == common.ChannelStatusEnabled {
			hasEnabledAccount = true
			break
		}
	}
	if !hasEnabledAccount {
		return "", firstNonEmpty(strings.TrimSpace(fallbackGroup), "default")
	}
	models := uniqueAccountCSV(accounts, func(account model.ChannelAccount) string {
		if account.Status != common.ChannelStatusEnabled {
			return ""
		}
		return account.Models
	})
	groups := uniqueAccountCSV(accounts, func(account model.ChannelAccount) string {
		if account.Status != common.ChannelStatusEnabled {
			return ""
		}
		return account.Group
	})
	if strings.TrimSpace(models) == "" {
		models = strings.TrimSpace(fallbackModels)
	}
	if strings.TrimSpace(groups) == "" {
		groups = strings.TrimSpace(fallbackGroup)
	}
	if strings.TrimSpace(groups) == "" {
		groups = "default"
	}
	return models, groups
}

func uniqueAccountCSV(accounts []model.ChannelAccount, valueFn func(model.ChannelAccount) string) string {
	seen := map[string]struct{}{}
	values := make([]string, 0)
	for _, account := range accounts {
		for _, part := range strings.Split(valueFn(account), ",") {
			value := strings.TrimSpace(part)
			if value == "" {
				continue
			}
			if _, exists := seen[value]; exists {
				continue
			}
			seen[value] = struct{}{}
			values = append(values, value)
		}
	}
	return strings.Join(values, ",")
}

func balanceValue(balance *BalanceSnapshot) float64 {
	if balance != nil && balance.BalanceUSD != nil {
		return *balance.BalanceUSD
	}
	return 0
}

func usedQuotaValue(balance *BalanceSnapshot) int64 {
	if balance != nil && balance.UsedUSD != nil {
		return int64(common.QuotaRound(*balance.UsedUSD * common.QuotaPerUnit))
	}
	return 0
}

func usdToQuotaInt64(value *float64) int64 {
	if value == nil {
		return 0
	}
	return int64(common.QuotaRound(*value * common.QuotaPerUnit))
}
