package upstreamaccount

import (
	"fmt"
	"math"
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
	RatioConversion   RatioConversionConfig `json:"ratio_conversion,omitempty"`
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
	Models             *string `json:"models,omitempty"`
	Group              string  `json:"group"`
	AccessGroups       *string `json:"access_groups,omitempty"`
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
	applySnapshotRatioConversionForRequest(record.Snapshot, req.RatioConversion)
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
	channelType := resolveSyncedChannelType(snapshot, req.Channel.Type)
	models := strings.TrimSpace(req.Channel.Models)
	if models == "" {
		models = inferModelsFromKeys(snapshot.Keys)
	}
	group := strings.TrimSpace(req.Channel.Group)
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
		// 渠道级 used_quota 表示 NexusTok 本地经该渠道产生的消费累计。
		// 上游账号快照中的账号总用量可能包含站外消耗，只能展示在密钥明细或未来
		// 独立的上游用量字段中，不能写入渠道本地累计值。
		UsedQuota:         0,
		Priority:          priority,
		AutoBan:           autoBan,
		Status:            status,
		TestModel:         req.Channel.TestModel,
		Tag:               req.Channel.Tag,
		Remark:            req.Channel.Remark,
		Setting:           req.Channel.Setting,
		ParamOverride:     req.Channel.ParamOverride,
		HeaderOverride:    req.Channel.HeaderOverride,
		StatusCodeMapping: req.Channel.StatusCodeMapping,
		Other:             req.Channel.Other,
		OtherSettings:     mergeChannelSyncMetadataWithCredential(req.Channel.OtherSettings, snapshot, Credential{}),
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
	models, group = summarizeEnabledAccountCapabilities(accounts, models, group, true)
	channel.Models = models
	channel.Group = group
	return channel, accounts, nil
}

func resolveSyncedChannelType(snapshot *Snapshot, requestedType int) int {
	// 上游账号同步渠道的“渠道类型”应表达接入平台本身：new-api 保存为 new-api，
	// sub2api 保存为 sub2api。两者在 Relay 层仍映射到 OpenAI 兼容 API 类型，
	// 因此这里改变展示和管理语义，不改变真实请求协议。
	if snapshot != nil {
		switch NormalizePlatform(snapshot.Platform) {
		case PlatformNewAPI:
			return constant.ChannelTypeNewAPI
		case PlatformSub2API:
			return constant.ChannelTypeSub2API
		}
	}
	if requestedType > 0 {
		return requestedType
	}
	return constant.ChannelTypeOpenAI
}

// normalizeSyncedChannelBaseURL 统一选择同步渠道的真实调用地址。
//
// 上游账号同步的预览阶段会通过 newHTTPClient/平台适配器拿到规范后的站点根地址：
// new-api 会去掉尾部 `/`，sub2api 会把常见前端路由 `/login`、`/dashboard` 等剥离。
// 创建渠道时优先使用该快照地址，可以避免前端仍把用户粘贴的页面地址写入渠道
// `base_url`，导致后续 Relay 拼出 `//v1/...` 或 `/login/v1/...` 这类错误上游路径。
func normalizeSyncedChannelBaseURL(value *string, snapshot *Snapshot) *string {
	if snapshot != nil {
		baseURL := normalizeSyncMetadataBaseURL(snapshot.Platform, firstNonEmpty(snapshotRelayBaseURL(snapshot), snapshot.BaseURL))
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
		models := syncedAccountModelsValue(config.Models, key.Models, defaultModels)
		group, hasGroup := explicitSyncValue(config.Group)
		if !hasGroup {
			group = firstNonEmpty(key.GroupName, key.GroupID, defaultGroup)
		}
		settings := mergeAccountSyncMetadata(config.OtherSettings, snapshot, key)
		settings = applyAccountKeyModelsSyncMetadata(settings, key, config.Models != nil, models)
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
		account := model.ChannelAccount{
			Name:               name,
			Key:                key.Key,
			Status:             status,
			Models:             models,
			Group:              group,
			AccessGroups:       normalizeSyncedAccessGroups(config.AccessGroups, "default"),
			Priority:           priority,
			Weight:             weight,
			UsedQuota:          usdToQuotaInt64(key.QuotaUsedUSD),
			BaseURL:            normalizeAccountConfigBaseURL(config.BaseURL),
			OpenAIOrganization: config.OpenAIOrganization,
			Other:              config.Other,
			Setting:            config.Setting,
			OtherSettings:      settings,
			ModelMapping:       config.ModelMapping,
			ParamOverride:      config.ParamOverride,
			HeaderOverride:     config.HeaderOverride,
			StatusCodeMapping:  config.StatusCodeMapping,
			MaxConcurrency:     config.MaxConcurrency,
		}
		applySyncedKeyModelFailureFallback(&account, key)
		if err := validateEnabledSyncedAccountCapability(account); err != nil {
			return nil, err
		}
		accounts = append(accounts, account)
	}
	if len(accounts) == 0 {
		return nil, fmt.Errorf("没有可创建的同步密钥")
	}
	return accounts, nil
}

// applySyncedKeyModelFailureFallback 将“模型同步失败且模型为空”的同步密钥降级为禁用保存。
//
// 生产上游经常会因为余额不足、权限不足或模型接口差异导致单把 key 拉不到模型。
// 如果继续让启用态校验报错，整次账号同步都会失败，管理员也看不到其它成功 key。
// 因此只有在已经有明确 key_models_sync_error 且最终模型仍为空时，才把该 key 标记为
// 手动禁用并写入脱敏原因；普通启用 key 没配置模型时仍由后续校验阻止保存。
func applySyncedKeyModelFailureFallback(account *model.ChannelAccount, key SyncedKey) {
	if account == nil || account.Status != common.ChannelStatusEnabled || strings.TrimSpace(account.Models) != "" {
		return
	}
	errText := strings.TrimSpace(common.MaskSensitiveInfo(key.KeyModelSyncError))
	if errText == "" {
		return
	}
	if key := strings.TrimSpace(key.Key); key != "" {
		errText = strings.ReplaceAll(errText, key, "[redacted-key]")
	}
	reason := "同步密钥模型列表获取失败：" + errText
	account.Status = common.ChannelStatusManuallyDisabled
	account.DisabledReason = reason
	account.LastError = reason
}

// validateEnabledSyncedAccountCapability 校验启用中的同步密钥是否具备可路由能力。
//
// 上游同步渠道的真实调度能力由每个 ChannelAccount 的 models 与 access_groups
// 共同决定。启用状态下如果任一字段为空，渠道看似保存成功但能力表不会生成可命中
// 的模型/用户组组合，因此在服务层兜底拒绝；禁用密钥可以保留空值，供管理员先保存
// 草稿配置，后续补齐能力后再启用。
func validateEnabledSyncedAccountCapability(account model.ChannelAccount) error {
	if account.Status != common.ChannelStatusEnabled {
		return nil
	}
	name := strings.TrimSpace(account.Name)
	if name == "" {
		name = "未命名同步密钥"
	}
	if strings.TrimSpace(account.Models) == "" {
		return fmt.Errorf("同步密钥 %s 必须配置至少一个模型", name)
	}
	if strings.TrimSpace(account.AccessGroups) == "" {
		return fmt.Errorf("同步密钥 %s 必须配置至少一个 NexusTok 可访问用户组", name)
	}
	return nil
}

// normalizeSyncedAccessGroups 规范化同步密钥可访问的 NexusTok 用户组。
//
// 新同步密钥默认只允许 default；管理员显式提交空字符串时仍保留空值，但启用密钥会在
// validateEnabledSyncedAccountCapability 中被拒绝。指针参数用于区分“刷新时未提交，继续
// 保留旧配置”和“明确清空”，禁用密钥可先保存空值作为草稿。
func normalizeSyncedAccessGroups(value *string, fallback string) string {
	raw := fallback
	if value != nil {
		raw = *value
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	seen := map[string]struct{}{}
	groups := make([]string, 0, 3)
	for _, item := range strings.Split(raw, ",") {
		group := strings.TrimSpace(item)
		if group == "" {
			continue
		}
		if _, exists := seen[group]; exists {
			continue
		}
		seen[group] = struct{}{}
		groups = append(groups, group)
	}
	return strings.Join(groups, ",")
}

// syncedAccountModelsValue 解析同步密钥的模型白名单。
//
// 上游同步渠道的路由能力完全由 ChannelAccount.models 和 access_groups 决定。
// models 使用指针是为了区分两种业务语义：
//   - nil：本次没有覆盖，继续使用上游快照模型；快照缺失时才回退渠道级兼容模型。
//   - 非 nil：管理员明确提交模型白名单，空字符串也要保留为空；启用密钥随后会被校验
//     拒绝，禁用密钥则允许作为未完成配置保存。
func syncedAccountModelsValue(configModels *string, keyModels []string, fallbackModels string) string {
	if configModels != nil {
		return strings.TrimSpace(*configModels)
	}
	if models := strings.TrimSpace(strings.Join(keyModels, ",")); models != "" {
		return models
	}
	return strings.TrimSpace(fallbackModels)
}

// explicitSyncValue 解析同步表单里可覆盖快照的非空字符串值。
//
// group 仍然是 string，因为它只表示上游密钥分组，空值继续回退到上游快照或渠道级默认值。
// NexusTok 下游用户组由 access_groups 指针字段控制，支持显式清空。
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

func summarizeEnabledAccountCapabilities(accounts []model.ChannelAccount, fallbackModels string, fallbackGroup string, includeAccountGroups bool) (string, string) {
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
		if !includeAccountGroups {
			return ""
		}
		if account.Status != common.ChannelStatusEnabled {
			return ""
		}
		if strings.TrimSpace(account.AccessGroups) != "" {
			return account.AccessGroups
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

func usdToQuotaInt64(value *float64) int64 {
	return snapshotUSDToQuotaInt64(value)
}

// snapshotUSDToQuotaInt64 将上游平台返回的美元快照换算成 NexusTok 内部 quota。
//
// 该函数只用于账号同步和余额刷新这类“展示上游累计值”的路径。上游账号的累计用量
// 可能远大于单次计费额度，如果复用 common.QuotaRound 会被 int32 计费边界截断，
// 导致渠道列表最多只能显示约 $4,294.97。真实扣费、预扣费和审计日志仍应继续使用
// common.QuotaRound / QuotaFromDecimal 等全局计费入口。
func snapshotUSDToQuotaInt64(value *float64) int64 {
	if value == nil {
		return 0
	}
	raw := *value
	if math.IsNaN(raw) || raw <= 0 {
		return 0
	}
	quota := math.Round(raw * common.QuotaPerUnit)
	if math.IsNaN(quota) || quota <= 0 {
		return 0
	}
	if quota >= float64(math.MaxInt64) {
		return math.MaxInt64
	}
	return int64(quota)
}
