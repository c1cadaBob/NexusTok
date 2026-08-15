package upstreamaccount

import (
	"context"
	"fmt"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/service/upstreammodel"
	"github.com/c1cada/NexusTok/setting/operation_setting"
)

const (
	keyModelSyncSourceSnapshot    = "snapshot"
	keyModelSyncSourceFetchModels = "fetch_models"
)

// syncSnapshotKeyModels 在账号同步落库前补齐每个同步 key 的模型列表。
//
// 上游管理接口如果已经返回 key.Models，则直接记录来源为 snapshot；如果缺失，则用该
// key 自己的凭据构造一次性渠道拉取模型列表。单个 key 拉取失败只写入脱敏错误并保留
// 原模型，不会让整次账号同步失败。
func syncSnapshotKeyModels(ctx context.Context, channelID int, snapshot *Snapshot, configs []AccountCreateConfig) {
	setting := operation_setting.GetUpstreamAccountSyncSetting()
	if setting == nil || !setting.SyncKeyModelsEnabled || snapshot == nil {
		return
	}
	ApplySyncIDs(snapshot)
	for index := range snapshot.Keys {
		if len(snapshot.Keys[index].Models) > 0 {
			snapshot.Keys[index].KeyModelSyncSource = keyModelSyncSourceSnapshot
		}
	}

	channel, err := model.GetChannelById(channelID, true)
	if err != nil {
		markMissingSnapshotKeyModelSyncError(snapshot, fmt.Errorf("读取同步渠道失败：%w", err))
		return
	}

	var existing []model.ChannelAccount
	if err := model.DB.Where("channel_id = ?", channelID).Find(&existing).Error; err != nil {
		markMissingSnapshotKeyModelSyncError(snapshot, fmt.Errorf("读取同步密钥失败：%w", err))
		return
	}
	byIdentity, byDigest := indexExistingAccounts(existing)
	configsByID := accountConfigBySyncID(configs)

	for index := range snapshot.Keys {
		if err := ctx.Err(); err != nil {
			markMissingSnapshotKeyModelSyncError(snapshot, err)
			return
		}
		key := &snapshot.Keys[index]
		if len(key.Models) > 0 {
			continue
		}
		if strings.TrimSpace(key.Key) == "" {
			key.KeyModelSyncSource = keyModelSyncSourceFetchModels
			key.KeyModelSyncError = "上游密钥缺少完整 key，无法同步模型列表"
			continue
		}
		config := configsByID[accountConfigLookupID(*key)]
		if config.Models != nil && !setting.KeyModelSyncOverwriteManualEnabled {
			continue
		}
		existingAccount := findExistingAccountForSnapshotKey(snapshot, *key, byIdentity, byDigest)
		if existingAccount != nil && shouldPreserveExistingAccountModels(existingAccount) {
			continue
		}
		tempChannel := buildChannelForSnapshotKeyModelFetch(channel, existingAccount, snapshot, *key)
		models, fetchErr := upstreammodel.FetchChannelModelIDs(tempChannel)
		key.KeyModelSyncSource = keyModelSyncSourceFetchModels
		if fetchErr != nil {
			key.KeyModelSyncError = common.MaskSensitiveInfo(fetchErr.Error())
			continue
		}
		key.Models = models
		key.KeyModelSyncError = ""
	}
}

func markMissingSnapshotKeyModelSyncError(snapshot *Snapshot, err error) {
	if snapshot == nil || err == nil {
		return
	}
	errText := common.MaskSensitiveInfo(err.Error())
	for index := range snapshot.Keys {
		if len(snapshot.Keys[index].Models) > 0 {
			continue
		}
		snapshot.Keys[index].KeyModelSyncSource = keyModelSyncSourceFetchModels
		snapshot.Keys[index].KeyModelSyncError = errText
	}
}

func findExistingAccountForSnapshotKey(
	snapshot *Snapshot,
	key SyncedKey,
	byIdentity map[string]*model.ChannelAccount,
	byDigest map[string]*model.ChannelAccount,
) *model.ChannelAccount {
	identity := syncIdentityKey(snapshot.Platform, snapshotManagementBaseURL(snapshot), key.ExternalID)
	account := byIdentity[identity]
	if account == nil && NormalizePlatform(snapshot.Platform) == PlatformSub2API {
		account = byIdentity[syncIdentityKey(snapshot.Platform, snapshotRelayBaseURL(snapshot), key.ExternalID)]
	}
	if account == nil {
		account = byDigest[keyDigest(key.Key)]
	}
	return account
}

func buildChannelForSnapshotKeyModelFetch(
	channel *model.Channel,
	existingAccount *model.ChannelAccount,
	snapshot *Snapshot,
	key SyncedKey,
) *model.Channel {
	temp := model.Channel{
		Type:   resolveSyncedChannelType(snapshot, 0),
		Key:    strings.TrimSpace(key.Key),
		Name:   firstNonEmpty(strings.TrimSpace(key.Name), strings.TrimSpace(key.MaskedKey), "synced-key-model-fetch"),
		Status: common.ChannelStatusEnabled,
		Group:  "default",
		Models: strings.Join(key.Models, ","),
		ChannelInfo: model.ChannelInfo{
			CredentialMode:     "",
			AccountPoolEnabled: false,
			IsMultiKey:         false,
		},
	}
	if channel != nil {
		temp = *channel
		temp.Type = resolveSyncedChannelType(snapshot, channel.Type)
		temp.Key = strings.TrimSpace(key.Key)
		temp.ChannelInfo.IsMultiKey = false
		temp.ChannelInfo.AccountPoolEnabled = false
		temp.ChannelInfo.CredentialMode = ""
	}
	baseURL := normalizeSyncMetadataBaseURL(snapshot.Platform, firstNonEmpty(snapshotRelayBaseURL(snapshot), snapshot.BaseURL))
	if baseURL != "" {
		temp.BaseURL = common.GetPointer(baseURL)
	} else if temp.GetBaseURL() == "" {
		if fallback := constant.ChannelBaseURLs[temp.Type]; strings.TrimSpace(fallback) != "" {
			temp.BaseURL = common.GetPointer(fallback)
		}
	}
	if existingAccount == nil {
		return &temp
	}
	account := *existingAccount
	account.Key = strings.TrimSpace(key.Key)
	return upstreammodel.ChannelWithAccountCredential(&temp, &account)
}
