package upstreamaccount

import (
	"context"
	"fmt"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/setting/operation_setting"
	"gorm.io/gorm"
)

// RefreshRequest 表示刷新已有账号同步渠道的请求。
type RefreshRequest struct {
	Credential
	ChannelID         int                   `json:"channel_id"`
	PreviewID         string                `json:"preview_id,omitempty"`
	Accounts          []AccountCreateConfig `json:"accounts"`
	ApplySuggested    bool                  `json:"apply_suggested"`
	DisableMissingKey bool                  `json:"disable_missing_key"`
	RatioConversion   RatioConversionConfig `json:"ratio_conversion,omitempty"`
}

// RefreshResult 表示刷新已有同步渠道后的变更统计。
type RefreshResult struct {
	ChannelID int `json:"channel_id"`
	Created   int `json:"created"`
	Updated   int `json:"updated"`
	Disabled  int `json:"disabled"`
}

// RefreshChannelFromCredential 使用管理员临时输入或已保存的上游账号凭据刷新已有同步渠道。
//
// 账号密码只用于本次请求。刷新时优先按账号 settings 中的同步元数据匹配上游 key，
// 旧账号没有元数据时再按完整 key 的 SHA-256 digest 匹配，避免把明文 key 写入匹配索引。
func RefreshChannelFromCredential(ctx context.Context, req RefreshRequest) (*RefreshResult, error) {
	if req.ChannelID <= 0 {
		return nil, fmt.Errorf("渠道 ID 不能为空")
	}
	if strings.TrimSpace(req.PreviewID) != "" {
		record, err := ConsumePreviewRecord(req.PreviewID)
		if err != nil {
			return nil, err
		}
		if record.Snapshot == nil {
			return nil, fmt.Errorf("预览快照为空，请重新同步")
		}
		applySnapshotRatioConversionForRequest(record.Snapshot, req.RatioConversion)
		syncSnapshotKeyModels(ctx, req.ChannelID, record.Snapshot, req.Accounts)
		return RefreshChannelFromSnapshot(req.ChannelID, record.Snapshot, req)
	}
	req.AuthMode = NormalizeAuthMode(req.AuthMode)
	if req.AuthMode == AuthModePassword && strings.TrimSpace(req.Password) == "" {
		credential, ok, err := loadChannelSyncCredential(req.ChannelID)
		if err != nil {
			return nil, err
		}
		if ok {
			req.Credential = credential
		}
	}
	var err error
	req.Credential, err = PrepareImportedCredential(req.Credential)
	if err != nil {
		return nil, err
	}
	req.Platform = NormalizePlatform(req.Platform)
	if strings.TrimSpace(req.Platform) == "" {
		return nil, fmt.Errorf("上游平台不能为空")
	}
	if strings.TrimSpace(req.BaseURL) == "" {
		return nil, fmt.Errorf("上游平台地址不能为空")
	}
	if credentialNeedsPassword(req.Credential) {
		return nil, fmt.Errorf("上游平台密码不能为空")
	}
	if refreshShouldUsePasswordLogin(req.Credential) {
		// 刷新同步渠道时，账号密码比已保存的 session / token 更可靠。这里仅清空
		// 本次内存凭据中的登录态，成功登录后平台客户端仍会把新的登录态重新挂到快照，
		// 后续无密码场景可以继续复用。
		req.Credential.Session = nil
		req.Credential.AccessToken = ""
		req.Credential.RefreshToken = ""
		req.Credential.ExpiresAt = 0
	}
	client, err := NewPlatformClient(req.Platform)
	if err != nil {
		return nil, err
	}
	snapshot, err := client.FetchSnapshot(ctx, req.Credential)
	if err != nil {
		return nil, err
	}
	ApplyRatioConversion(snapshot, req.RatioConversion)
	ApplySuggestions(snapshot)
	attachStoredCredential(snapshot, req.Credential)
	syncSnapshotKeyModels(ctx, req.ChannelID, snapshot, req.Accounts)
	return RefreshChannelFromSnapshot(req.ChannelID, snapshot, req)
}

// RefreshChannelFromSnapshot 将已获取的上游快照应用到现有渠道。
func RefreshChannelFromSnapshot(channelID int, snapshot *Snapshot, req RefreshRequest) (*RefreshResult, error) {
	if snapshot == nil {
		return nil, fmt.Errorf("上游账号快照为空")
	}
	result := &RefreshResult{ChannelID: channelID}
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		var channel model.Channel
		if err := tx.Where("id = ?", channelID).First(&channel).Error; err != nil {
			return err
		}
		if channel.GetCredentialMode() != constant.ChannelCredentialModeAccountPool {
			return fmt.Errorf("当前渠道不是账号同步渠道，无法刷新")
		}
		var existing []model.ChannelAccount
		if err := tx.Where("channel_id = ?", channelID).Find(&existing).Error; err != nil {
			return err
		}

		applySnapshotRatioConversionForRefresh(
			snapshot,
			req.RatioConversion,
			channel.OtherSettings,
			existing,
		)
		ApplySyncIDs(snapshot)

		defaultModels := strings.TrimSpace(channel.Models)
		if inferred := inferModelsFromKeys(snapshot.Keys); inferred != "" {
			defaultModels = inferred
		}
		defaultGroup := strings.TrimSpace(channel.Group)
		if defaultGroup == "" {
			defaultGroup = "default"
		}

		updates := map[string]any{
			"balance":              balanceValue(snapshot.Balance),
			"balance_updated_time": common.GetTimestamp(),
			"settings":             mergeChannelSyncMetadataWithCredential(channel.OtherSettings, snapshot, req.Credential),
		}
		if baseURL, ok := syncedChannelBaseURLUpdate(channel, snapshot); ok {
			updates["base_url"] = baseURL
		}
		if syncedChannelType := resolveSyncedChannelType(snapshot, channel.Type); syncedChannelType > 0 && channel.Type != syncedChannelType {
			// 刷新快照时以后端识别出的上游平台为准修正渠道类型，避免历史 OpenAI
			// 同步渠道或外部 API 调用刷新后继续显示成普通 OpenAI 渠道。
			updates["type"] = syncedChannelType
		}
		if err := tx.Model(&channel).Updates(updates).Error; err != nil {
			return err
		}
		channel.OtherSettings = updates["settings"].(string)

		configs := accountConfigBySyncID(req.Accounts)
		byIdentity, byDigest := indexExistingAccounts(existing)
		seenExistingIDs := map[int]struct{}{}
		for _, key := range snapshot.Keys {
			if strings.TrimSpace(key.Key) == "" {
				return fmt.Errorf("上游密钥 %s 缺少完整 key，无法刷新", key.Name)
			}
			config := configs[accountConfigLookupID(key)]
			enabled := true
			if config.Enabled != nil {
				enabled = *config.Enabled
			}
			identity := syncIdentityKey(snapshot.Platform, snapshotManagementBaseURL(snapshot), key.ExternalID)
			digest := keyDigest(key.Key)
			account := byIdentity[identity]
			if account == nil && NormalizePlatform(snapshot.Platform) == PlatformSub2API {
				account = byIdentity[syncIdentityKey(snapshot.Platform, snapshotRelayBaseURL(snapshot), key.ExternalID)]
			}
			if account == nil && digest != "" {
				account = byDigest[digest]
			}
			if account == nil {
				if !enabled {
					continue
				}
				accountToCreate := buildAccountFromSyncedKey(snapshot, key, config, req.ApplySuggested, defaultModels, defaultGroup)
				accountToCreate.ChannelId = channelID
				if err := validateEnabledSyncedAccountCapability(accountToCreate); err != nil {
					return err
				}
				if err := tx.Create(&accountToCreate).Error; err != nil {
					return err
				}
				result.Created++
				continue
			}
			seenExistingIDs[account.Id] = struct{}{}
			updates, err := buildAccountRefreshUpdates(account, snapshot, key, config, req.ApplySuggested, defaultModels, defaultGroup)
			if err != nil {
				return err
			}
			if !enabled {
				updates["status"] = common.ChannelStatusManuallyDisabled
				updates["disabled_reason"] = "upstream account sync disabled"
			}
			if err := tx.Model(account).Updates(updates).Error; err != nil {
				return err
			}
			result.Updated++
		}

		if req.DisableMissingKey {
			for i := range existing {
				account := &existing[i]
				if _, ok := seenExistingIDs[account.Id]; ok {
					continue
				}
				metadata := readAccountSyncMetadata(account.OtherSettings)
				if !sameSyncSource(metadata, snapshot) {
					continue
				}
				if account.Status != common.ChannelStatusManuallyDisabled {
					err := tx.Model(account).Updates(map[string]any{
						"status":          common.ChannelStatusManuallyDisabled,
						"disabled_reason": "upstream key missing after account sync refresh",
					}).Error
					if err != nil {
						return err
					}
					result.Disabled++
				}
			}
		}
		if err := model.SyncChannelAccountPoolCapabilities(channelID, tx); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}
	model.InitChannelCache()
	return result, nil
}

// refreshShouldUsePasswordLogin 判断刷新同步渠道时是否应强制走账号密码登录。
//
// 只有认证模式为 password、账号标识和密码都存在时才启用。调用方会清空本次内存
// 中的 session / access_token / refresh_token，避免旧登录态短暂可用时掩盖账号密码
// 已失效的问题；登录失败也不会回退旧 token。
func refreshShouldUsePasswordLogin(credential Credential) bool {
	if NormalizeAuthMode(credential.AuthMode) != AuthModePassword {
		return false
	}
	if strings.TrimSpace(credential.Password) == "" {
		return false
	}
	return strings.TrimSpace(firstNonEmpty(credential.Username, credential.Email)) != ""
}

// applySnapshotRatioConversionForRefresh 确定刷新落库时使用的成本换算配置。
//
// 优先级固定为：本次请求显式配置、快照已有配置、当前渠道账号 metadata 中保留的
// 历史配置。最后一项用于系统自动同步和外部 API 刷新，避免未传 ratio_conversion
// 时把管理员已保存的“实付金额 / 上游到账额度”重置为默认值。
func applySnapshotRatioConversionForRefresh(
	snapshot *Snapshot,
	config RatioConversionConfig,
	channelSettings string,
	existing []model.ChannelAccount,
) {
	if config.Enabled() || (snapshot != nil && snapshot.RatioConversion != nil) {
		applySnapshotRatioConversionForRequest(snapshot, config)
		return
	}
	if preserved, ok := preservedRatioConversionConfigFromChannelSettings(channelSettings); ok {
		ApplyRatioConversion(snapshot, preserved)
		ApplySuggestions(snapshot)
		return
	}
	if preserved, ok := preservedRatioConversionConfig(existing); ok {
		ApplyRatioConversion(snapshot, preserved)
		ApplySuggestions(snapshot)
		return
	}
	applySnapshotRatioConversionForRequest(snapshot, config)
}

// preservedRatioConversionConfigFromChannelSettings 读取渠道级历史换算配置。
//
// 新版同步渠道会把“实付金额 / 上游平台到账额度”同时保存到渠道和密钥
// metadata；旧版或部分导入路径可能只保存了渠道级配置。余额刷新、系统同步
// 不携带前端比例表单，因此必须先从渠道级配置恢复，避免密钥 metadata 被
// 空配置覆盖后，前端显示突然退回未换算值。
func preservedRatioConversionConfigFromChannelSettings(
	settings string,
) (RatioConversionConfig, bool) {
	metadata := readChannelSyncMetadata(settings)
	config := ratioConversionConfigFromSnapshot(metadata.RatioConversionConfig)
	if !config.Enabled() {
		return RatioConversionConfig{}, false
	}
	return config, true
}

// preservedRatioConversionConfig 从已有同步账号 metadata 中读取首个有效换算配置。
//
// 同一个同步渠道的账号共享渠道级成本换算配置，因此首个有效配置即可代表本渠道的
// 历史设置；无效、关闭或不完整的旧数据会被跳过，不做 1/1 或 1/10 推断。
func preservedRatioConversionConfig(accounts []model.ChannelAccount) (RatioConversionConfig, bool) {
	for i := range accounts {
		metadata := readAccountSyncMetadata(accounts[i].OtherSettings)
		config := metadata.RatioConversionConfig
		if config == nil || !config.Enabled || config.PaidCNY <= 0 || config.PlatformUSDCredit <= 0 {
			continue
		}
		return RatioConversionConfig{
			PaidCNY:           config.PaidCNY,
			PlatformUSDCredit: config.PlatformUSDCredit,
		}, true
	}
	return RatioConversionConfig{}, false
}

// syncedChannelBaseURLUpdate 判断刷新成功后是否可以把渠道调用地址迁移到快照地址。
//
// Sub2API 面板域名和 API 域名分离时，预览快照会携带实际 API 地址。只有当前渠道
// base_url 仍等于旧同步元数据地址或为空时才自动更新；如果管理员已经在渠道页改成
// 其他地址，说明这是本地覆盖，刷新流程不能替管理员覆盖掉。
func syncedChannelBaseURLUpdate(channel model.Channel, snapshot *Snapshot) (string, bool) {
	if snapshot == nil {
		return "", false
	}
	nextBaseURL := normalizeSyncMetadataBaseURL(snapshot.Platform, firstNonEmpty(snapshotRelayBaseURL(snapshot), snapshot.BaseURL))
	if nextBaseURL == "" {
		return "", false
	}
	currentBaseURL := ""
	if channel.BaseURL != nil {
		currentBaseURL = strings.TrimRight(strings.TrimSpace(*channel.BaseURL), "/")
	}
	if currentBaseURL == nextBaseURL {
		return "", false
	}
	metadata := readChannelSyncMetadata(channel.OtherSettings)
	metadataPlatform := firstNonEmpty(metadata.Platform, snapshot.Platform)
	oldRelayBaseURL := normalizeSyncMetadataBaseURL(metadataPlatform, firstNonEmpty(metadata.RelayBaseURL, metadata.BaseURL))
	oldManagementBaseURL := normalizeSyncMetadataBaseURL(metadataPlatform, firstNonEmpty(metadata.ManagementBaseURL, metadata.BaseURL))
	if currentBaseURL == "" ||
		(oldRelayBaseURL != "" && currentBaseURL == oldRelayBaseURL) ||
		(oldManagementBaseURL != "" && currentBaseURL == oldManagementBaseURL) {
		return nextBaseURL, true
	}
	return "", false
}

func accountConfigBySyncID(configs []AccountCreateConfig) map[string]AccountCreateConfig {
	result := map[string]AccountCreateConfig{}
	for _, config := range configs {
		syncID := strings.TrimSpace(config.SyncID)
		if syncID != "" {
			result[syncID] = config
		}
		externalID := strings.TrimSpace(config.ExternalID)
		if externalID != "" {
			if _, exists := result[externalID]; !exists {
				result[externalID] = config
			}
		}
	}
	return result
}

func accountConfigLookupID(key SyncedKey) string {
	if strings.TrimSpace(key.SyncID) != "" {
		return strings.TrimSpace(key.SyncID)
	}
	if strings.TrimSpace(key.ExternalID) != "" {
		return strings.TrimSpace(key.ExternalID)
	}
	if strings.TrimSpace(key.MaskedKey) != "" {
		return strings.TrimSpace(key.MaskedKey)
	}
	return keyDigest(key.Key)
}

func indexExistingAccounts(accounts []model.ChannelAccount) (map[string]*model.ChannelAccount, map[string]*model.ChannelAccount) {
	byIdentity := map[string]*model.ChannelAccount{}
	byDigest := map[string]*model.ChannelAccount{}
	for i := range accounts {
		account := &accounts[i]
		metadata := readAccountSyncMetadata(account.OtherSettings)
		baseURLs := []string{metadata.BaseURL}
		if NormalizePlatform(metadata.Platform) == PlatformSub2API {
			baseURLs = append(baseURLs, metadata.ManagementBaseURL, metadata.RelayBaseURL)
		}
		for _, baseURL := range baseURLs {
			if key := syncIdentityKey(metadata.Platform, baseURL, metadata.ExternalID); key != "" {
				byIdentity[key] = account
			}
		}
		if metadata.KeyDigest != "" {
			byDigest[metadata.KeyDigest] = account
			continue
		}
		if digest := keyDigest(account.Key); digest != "" {
			byDigest[digest] = account
		}
	}
	return byIdentity, byDigest
}

func buildAccountFromSyncedKey(snapshot *Snapshot, key SyncedKey, config AccountCreateConfig, applySuggested bool, defaultModels string, defaultGroup string) model.ChannelAccount {
	priority := int64(0)
	if config.Priority != nil {
		priority = *config.Priority
	}
	weight := ManagedWeightForSyncedKey(key)
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
	settings := mergeAccountSyncMetadata(config.OtherSettings, snapshot, key)
	settings = applyAccountKeyModelsSyncMetadata(settings, key, config.Models != nil, models)
	group, hasGroup := explicitSyncValue(config.Group)
	if !hasGroup {
		group = firstNonEmpty(key.GroupName, key.GroupID, defaultGroup)
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
	return account
}

func buildAccountRefreshUpdates(existing *model.ChannelAccount, snapshot *Snapshot, key SyncedKey, config AccountCreateConfig, applySuggested bool, defaultModels string, defaultGroup string) (map[string]any, error) {
	account := buildAccountFromSyncedKey(snapshot, key, config, applySuggested, defaultModels, defaultGroup)
	settings := account.OtherSettings
	explicitDisable := config.Enabled != nil && !*config.Enabled
	if existing != nil {
		settings = mergeAccountSyncMetadata(existing.OtherSettings, snapshot, key)
		if config.Priority == nil {
			account.Priority = existing.Priority
		}
		if explicitDisable {
			settings = applyExplicitSyncedAccountRefreshDisable(&account, settings)
		} else if existing.Status != common.ChannelStatusEnabled {
			// 刷新同步账号只能更新上游快照带来的托管字段，不能把已经禁用的密钥
			// 偷偷恢复成启用。真正恢复必须来自手动测试成功、自动测试连续快速成功，
			// 或管理员在账号管理中显式点击启用。
			account.Status = existing.Status
			account.DisabledReason = existing.DisabledReason
			account.LastError = existing.LastError
			account.RateLimitedUntil = existing.RateLimitedUntil
			account.OverloadUntil = existing.OverloadUntil
			account.TempDisabledUntil = existing.TempDisabledUntil
		}
		modelsManuallyOverridden := config.Models != nil
		if config.Models == nil && shouldPreserveExistingAccountModels(existing) {
			account.Models = existing.Models
			modelsManuallyOverridden = true
		}
		if strings.TrimSpace(config.Group) == "" && strings.TrimSpace(existing.Group) != "" {
			account.Group = existing.Group
		}
		if config.AccessGroups == nil {
			account.AccessGroups = existing.AccessGroups
		}
		settings = applyAccountKeyModelsSyncMetadata(settings, key, modelsManuallyOverridden, account.Models)
		account.OtherSettings = settings
	}
	// 显式关闭的同步 key 可以保留空模型或空访问组作为草稿；这里先写入最终状态，
	// 再执行启用态校验，避免刷新禁用 key 时被误判为能力缺失。
	if explicitDisable {
		account.Status = common.ChannelStatusManuallyDisabled
		if strings.TrimSpace(account.DisabledReason) == "" {
			account.DisabledReason = "upstream account sync disabled"
		}
	}
	applySyncedKeyModelFailureFallback(&account, key)
	if err := validateEnabledSyncedAccountCapability(account); err != nil {
		return nil, err
	}
	disabledReason := account.DisabledReason
	lastError := account.LastError
	updates := map[string]any{
		"name":                account.Name,
		"key":                 account.Key,
		"status":              account.Status,
		"models":              account.Models,
		"group":               account.Group,
		"access_groups":       account.AccessGroups,
		"priority":            account.Priority,
		"weight":              account.Weight,
		"used_quota":          account.UsedQuota,
		"settings":            settings,
		"disabled_reason":     disabledReason,
		"rate_limited_until":  account.RateLimitedUntil,
		"overload_until":      account.OverloadUntil,
		"temp_disabled_until": account.TempDisabledUntil,
		"last_error":          lastError,
	}
	// 已有账号可能被管理员在 NexusTok 中做过本地覆盖。刷新默认只更新上游同步字段；
	// 只有请求显式传入覆盖配置时才写入这些字段，避免一次刷新清空手工配置。
	if config.MaxConcurrency > 0 {
		updates["max_concurrency"] = account.MaxConcurrency
	}
	if config.BaseURL != nil {
		updates["base_url"] = account.BaseURL
	}
	if config.OpenAIOrganization != nil {
		// ChannelAccount 未显式声明 gorm column，GORM 会映射为 open_ai_organization。
		updates["open_ai_organization"] = account.OpenAIOrganization
	}
	if strings.TrimSpace(config.Other) != "" {
		updates["other"] = account.Other
	}
	if config.Setting != nil {
		updates["setting"] = account.Setting
	}
	if strings.TrimSpace(config.OtherSettings) != "" {
		// 显式提交 settings 时仍要把新的同步身份写回，避免管理员的本地配置覆盖掉
		// `platform/base_url/external_id/key_digest`，否则下次刷新只能退回 key digest 匹配。
		updates["settings"] = account.OtherSettings
	}
	if config.ModelMapping != nil {
		updates["model_mapping"] = account.ModelMapping
	}
	if config.ParamOverride != nil {
		updates["param_override"] = account.ParamOverride
	}
	if config.HeaderOverride != nil {
		updates["header_override"] = account.HeaderOverride
	}
	if config.StatusCodeMapping != nil {
		updates["status_code_mapping"] = account.StatusCodeMapping
	}
	return updates, nil
}

func applyExplicitSyncedAccountRefreshDisable(account *model.ChannelAccount, settings string) string {
	if account != nil {
		account.Status = common.ChannelStatusManuallyDisabled
		account.DisabledReason = "upstream account sync disabled"
		account.LastError = ""
		account.RateLimitedUntil = 0
		account.OverloadUntil = 0
		account.TempDisabledUntil = 0
	}
	// 刷新面板显式关闭密钥等同于管理员人工禁用：必须清掉自动恢复标记，
	// 否则后续后台自动测试可能把这个人工状态误当成可自动恢复。
	return ClearAccountAutoCheckDisableMarker(settings)
}

func shouldPreserveExistingAccountModels(existing *model.ChannelAccount) bool {
	if existing == nil || strings.TrimSpace(existing.Models) == "" {
		return false
	}
	setting := operation_setting.GetUpstreamAccountSyncSetting()
	if setting == nil || !setting.SyncKeyModelsEnabled {
		return true
	}
	if setting.KeyModelSyncOverwriteManualEnabled {
		return false
	}
	metadata := readAccountSyncMetadata(existing.OtherSettings)
	if metadata.KeyModelsManualOverride {
		return true
	}
	// 旧同步账号没有 key_models_synced_at/source，无法可靠区分“上游同步写入”和
	// “管理员本地编辑”。默认不覆盖这类历史白名单，避免升级后第一次后台同步把本地
	// 治理模型冲掉；需要强制跟随上游时可打开覆盖开关。
	return metadata.KeyModelsSyncedAt <= 0 && strings.TrimSpace(metadata.KeyModelsSyncSource) == ""
}

func sameSyncSource(metadata syncMetadata, snapshot *Snapshot) bool {
	metadataBaseURL := firstNonEmpty(metadata.ManagementBaseURL, metadata.BaseURL)
	snapshotBaseURL := snapshotManagementBaseURL(snapshot)
	return syncIdentityKey(metadata.Platform, metadataBaseURL, metadata.ExternalID) != "" &&
		NormalizePlatform(metadata.Platform) == NormalizePlatform(snapshot.Platform) &&
		sameSyncSourceBaseURL(snapshot.Platform, metadataBaseURL, snapshotBaseURL)
}
