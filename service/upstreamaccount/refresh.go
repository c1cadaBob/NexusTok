package upstreamaccount

import (
	"context"
	"fmt"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
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
	return RefreshChannelFromSnapshot(req.ChannelID, snapshot, req)
}

// RefreshChannelFromSnapshot 将已获取的上游快照应用到现有渠道。
func RefreshChannelFromSnapshot(channelID int, snapshot *Snapshot, req RefreshRequest) (*RefreshResult, error) {
	if snapshot == nil {
		return nil, fmt.Errorf("上游账号快照为空")
	}
	applySnapshotRatioConversionForRequest(snapshot, req.RatioConversion)
	ApplySyncIDs(snapshot)
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
			"used_quota":           usedQuotaValue(snapshot.Balance),
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
				if err := tx.Create(&accountToCreate).Error; err != nil {
					return err
				}
				result.Created++
				continue
			}
			seenExistingIDs[account.Id] = struct{}{}
			updates := buildAccountRefreshUpdates(account, snapshot, key, config, req.ApplySuggested, defaultModels, defaultGroup)
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
	if applySuggested {
		priority = key.SuggestedPriority
	}
	if config.Priority != nil {
		priority = *config.Priority
	}
	weight := 0
	if applySuggested {
		weight = key.SuggestedWeight
	}
	if config.Weight != nil {
		weight = *config.Weight
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
	return model.ChannelAccount{
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
		OtherSettings:      mergeAccountSyncMetadata(config.OtherSettings, snapshot, key),
		ModelMapping:       config.ModelMapping,
		ParamOverride:      config.ParamOverride,
		HeaderOverride:     config.HeaderOverride,
		StatusCodeMapping:  config.StatusCodeMapping,
		MaxConcurrency:     config.MaxConcurrency,
	}
}

func buildAccountRefreshUpdates(existing *model.ChannelAccount, snapshot *Snapshot, key SyncedKey, config AccountCreateConfig, applySuggested bool, defaultModels string, defaultGroup string) map[string]any {
	account := buildAccountFromSyncedKey(snapshot, key, config, applySuggested, defaultModels, defaultGroup)
	settings := account.OtherSettings
	if existing != nil {
		settings = mergeAccountSyncMetadata(existing.OtherSettings, snapshot, key)
		if !applySuggested && config.Priority == nil {
			account.Priority = existing.Priority
		}
		if !applySuggested && config.Weight == nil {
			account.Weight = existing.Weight
		}
		if config.Enabled == nil {
			account.Status = existing.Status
		}
		if config.Models == nil && strings.TrimSpace(existing.Models) != "" {
			account.Models = existing.Models
		}
		if strings.TrimSpace(config.Group) == "" && strings.TrimSpace(existing.Group) != "" {
			account.Group = existing.Group
		}
		if config.AccessGroups == nil {
			account.AccessGroups = existing.AccessGroups
		}
	}
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
		"disabled_reason":     "",
		"rate_limited_until":  0,
		"overload_until":      0,
		"temp_disabled_until": 0,
		"last_error":          "",
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
	return updates
}

func sameSyncSource(metadata syncMetadata, snapshot *Snapshot) bool {
	metadataBaseURL := firstNonEmpty(metadata.ManagementBaseURL, metadata.BaseURL)
	snapshotBaseURL := snapshotManagementBaseURL(snapshot)
	return syncIdentityKey(metadata.Platform, metadataBaseURL, metadata.ExternalID) != "" &&
		NormalizePlatform(metadata.Platform) == NormalizePlatform(snapshot.Platform) &&
		sameSyncSourceBaseURL(snapshot.Platform, metadataBaseURL, snapshotBaseURL)
}
