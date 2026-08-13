package upstreamaccount

import (
	"math"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
)

// UpstreamAssetDisplay 是同步渠道或同步密钥的运行时资产展示值。
//
// 数据库存储仍保留原始上游 USD 快照和 NexusTok 本地 used_quota。这个结构只在
// API 响应阶段生成，统一应用实付金额 / 上游平台到账额度，避免重复换算或把
// 上游账单写进本地消费字段。
type UpstreamAssetDisplay struct {
	BalanceUSD       *float64
	UsedUSD          *float64
	UsedQuota        *int64
	RemainingUSD     *float64
	RemainingQuota   *int64
	ConversionFactor float64
	Partial          bool
	HasBalance       bool
	HasUsed          bool
	HasRemaining     bool
}

// BuildChannelAssetDisplay 计算同步渠道的上游资产展示值。
//
// 账号级快照优先；旧数据缺少账号级已用量时，使用同步密钥的本地 quota 快照
// 做近似回退并标记 partial。普通渠道不应调用这个函数。
func BuildChannelAssetDisplay(channel *model.Channel, accounts []model.ChannelAccount) UpstreamAssetDisplay {
	display := UpstreamAssetDisplay{ConversionFactor: 1}
	if channel == nil || !channel.HasUpstreamAccountSyncMetadata() {
		return display
	}

	channelMetadata := readChannelSyncMetadata(channel.OtherSettings)
	config := ratioConversionConfigFromSnapshot(channelMetadata.RatioConversionConfig)
	if !config.Enabled() {
		for i := range accounts {
			accountMetadata := readAccountSyncMetadata(accounts[i].OtherSettings)
			if !syncMetadataHasIdentity(accountMetadata) {
				continue
			}
			config = ratioConversionConfigFromSnapshot(accountMetadata.RatioConversionConfig)
			if config.Enabled() {
				break
			}
		}
	}
	display.ConversionFactor = config.AssetConversionFactor()

	snapshot := channelMetadata.BalanceSnapshot
	if snapshot != nil {
		if finiteNonNegativeFloatPtr(snapshot.BalanceUSD) {
			value := *snapshot.BalanceUSD * display.ConversionFactor
			display.BalanceUSD = &value
			display.HasBalance = true
		}
		if finiteNonNegativeFloatPtr(snapshot.UsedUSD) {
			value := *snapshot.UsedUSD * display.ConversionFactor
			display.UsedUSD = &value
			display.HasUsed = true
		}
		display.Partial = snapshot.Partial || snapshot.MissingUsedValue || snapshot.MissingBalanceValue
	}

	if !display.HasBalance && finiteNonNegativeFloat(channel.Balance) {
		value := channel.Balance * display.ConversionFactor
		display.BalanceUSD = &value
		display.HasBalance = true
		display.Partial = true
	}

	if !display.HasUsed {
		if rawUSD, ok := syncedAccountsRawUsedUSD(accounts); ok {
			convertedUSD := rawUSD * display.ConversionFactor
			display.UsedUSD = &convertedUSD
			display.UsedQuota = quotaPointerFromUSD(convertedUSD)
			display.HasUsed = true
			display.Partial = true
		}
	}
	if display.HasUsed && display.UsedQuota == nil {
		display.UsedQuota = quotaPointerFromUSD(*display.UsedUSD)
	}
	return display
}

// BuildAccountAssetDisplay 计算同步密钥的换算后已用量和剩余量。
//
// 新同步账号从 settings 中读取原始 USD 快照；旧账号缺少快照时仅能把
// ChannelAccount.used_quota 作为已用量近似值，剩余额度保持缺失而不是伪造。
func BuildAccountAssetDisplay(settings string, localUsedQuota int64) UpstreamAssetDisplay {
	metadata := readAccountSyncMetadata(settings)
	if !syncMetadataHasIdentity(metadata) {
		return UpstreamAssetDisplay{}
	}
	config := ratioConversionConfigFromSnapshot(metadata.RatioConversionConfig)
	display := UpstreamAssetDisplay{
		ConversionFactor: config.AssetConversionFactor(),
	}

	usedUSD := metadata.QuotaUsedUSD
	if usedUSD == nil && localUsedQuota >= 0 {
		value := float64(localUsedQuota) / common.QuotaPerUnit
		usedUSD = &value
		display.Partial = true
	}
	if finiteNonNegativeFloatPtr(usedUSD) {
		value := *usedUSD * display.ConversionFactor
		display.UsedUSD = &value
		display.UsedQuota = quotaPointerFromUSD(value)
		display.HasUsed = true
	}

	remainingUSD := metadata.QuotaRemainingUSD
	if remainingUSD == nil && metadata.QuotaLimitUSD != nil && usedUSD != nil {
		value := *metadata.QuotaLimitUSD - *usedUSD
		if value < 0 {
			value = 0
		}
		remainingUSD = &value
		display.Partial = true
	}
	if finiteNonNegativeFloatPtr(remainingUSD) {
		value := *remainingUSD * display.ConversionFactor
		display.RemainingUSD = &value
		display.RemainingQuota = quotaPointerFromUSD(value)
		display.HasRemaining = true
	}
	return display
}

// syncedAccountsRawUsedUSD 汇总同步密钥的原始上游已用量。
//
// 同步渠道允许管理员在同一个渠道下额外维护本地账号，这些本地账号的 used_quota
// 是 NexusTok 本地消费累计，不能混入上游账号账单。这里必须先确认账号带有
// upstream_account_sync 身份，再优先使用 metadata 中保存的原始 USD 快照；旧同步
// 账号没有原始快照时，才把 ChannelAccount.used_quota 当作旧版上游已用快照回退。
// 如果整个渠道下没有任何账号带同步身份，则说明可能是早期版本数据，只能退回旧逻辑
// 统计全部账号并标记 partial，由管理员据此判断数据完整性。
func syncedAccountsRawUsedUSD(accounts []model.ChannelAccount) (float64, bool) {
	var total float64
	hasSyncedMetadata := false
	legacyTotal := 0.0
	legacyFound := false
	for i := range accounts {
		metadata := readAccountSyncMetadata(accounts[i].OtherSettings)
		if !syncMetadataHasIdentity(metadata) {
			if accounts[i].UsedQuota >= 0 {
				legacyTotal += float64(accounts[i].UsedQuota) / common.QuotaPerUnit
				legacyFound = true
			}
			continue
		}
		hasSyncedMetadata = true
		if finiteNonNegativeFloatPtr(metadata.QuotaUsedUSD) {
			total += *metadata.QuotaUsedUSD
			continue
		}
		if accounts[i].UsedQuota >= 0 {
			total += float64(accounts[i].UsedQuota) / common.QuotaPerUnit
		}
	}
	if hasSyncedMetadata {
		return total, true
	}
	return legacyTotal, legacyFound
}

func quotaPointerFromUSD(value float64) *int64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return nil
	}
	quota := snapshotUSDToQuotaInt64(&value)
	return &quota
}

func finiteNonNegativeFloatPtr(value *float64) bool {
	return value != nil && finiteNonNegativeFloat(*value)
}

// AttachChannelAssetDisplays 为渠道列表附加同步渠道的换算后上游资产字段。
func AttachChannelAssetDisplays(channels []*model.Channel) error {
	ids := make([]int, 0)
	for _, channel := range channels {
		if channel != nil && channel.HasUpstreamAccountSyncMetadata() {
			ids = append(ids, channel.Id)
		}
	}
	if len(ids) == 0 {
		return nil
	}

	var accounts []model.ChannelAccount
	if err := model.DB.Where("channel_id IN ?", ids).Find(&accounts).Error; err != nil {
		return err
	}
	accountsByChannel := make(map[int][]model.ChannelAccount, len(ids))
	for i := range accounts {
		accountsByChannel[accounts[i].ChannelId] = append(accountsByChannel[accounts[i].ChannelId], accounts[i])
	}
	for _, channel := range channels {
		if channel == nil || !channel.HasUpstreamAccountSyncMetadata() {
			continue
		}
		display := BuildChannelAssetDisplay(channel, accountsByChannel[channel.Id])
		attachChannelAssetDisplay(channel, display)
	}
	return nil
}

func attachChannelAssetDisplay(channel *model.Channel, display UpstreamAssetDisplay) {
	if channel == nil || !display.HasBalance && !display.HasUsed {
		return
	}
	channel.UpstreamBalanceUSD = display.BalanceUSD
	channel.UpstreamUsedUSD = display.UsedUSD
	channel.UpstreamUsedQuota = display.UsedQuota
	channel.UpstreamConversionFactor = &display.ConversionFactor
	channel.UpstreamPartial = display.Partial
}
