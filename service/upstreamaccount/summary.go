package upstreamaccount

import (
	"math"

	"github.com/c1cada/NexusTok/model"
)

// UpstreamAccountSummary 是管理员钱包页展示的上游账号资产汇总。
//
// 这里汇总的是上游平台账号维度的余额和总用量，和 NexusTok 本地用户余额、
// Channel.used_quota、账单统计互不影响。禁用渠道也会计入，因为它们仍然代表
// 管理员曾同步过的上游账号资产或历史账单快照。
type UpstreamAccountSummary struct {
	UpstreamBalanceUSD float64 `json:"upstream_balance_usd"`
	UpstreamUsedUSD    float64 `json:"upstream_used_usd"`
	UpstreamUsedQuota  int64   `json:"upstream_used_quota"`
	SyncedChannelCount int     `json:"synced_channel_count"`
	Partial            bool    `json:"partial"`
	UpdatedAt          int64   `json:"updated_at"`
}

// SummarizeUpstreamAccounts 汇总所有同步渠道最近一次上游账号快照。
//
// 新同步/刷新后的渠道会在 settings.upstream_account_sync.balance_snapshot 中保存
// 账号级 used_usd；旧渠道缺少该字段时只能用已同步密钥的 used_quota 求和近似回退。
// 这个回退无法覆盖未同步或已删除的上游密钥，因此会把 partial 标记为 true，提醒
// 管理员该总用量不是完整账号级数据。
func SummarizeUpstreamAccounts() (UpstreamAccountSummary, error) {
	var channels []model.Channel
	if err := model.DB.
		Omit("key").
		Where("settings LIKE ?", "%upstream_account_sync%").
		Find(&channels).Error; err != nil {
		return UpstreamAccountSummary{}, err
	}

	syncedChannels := make([]model.Channel, 0, len(channels))
	channelIDs := make([]int, 0)
	for i := range channels {
		if !channels[i].HasUpstreamAccountSyncMetadata() {
			continue
		}
		syncedChannels = append(syncedChannels, channels[i])
		channelIDs = append(channelIDs, channels[i].Id)
	}
	accountsByChannel, err := loadChannelAccountsByChannel(channelIDs)
	if err != nil {
		return UpstreamAccountSummary{}, err
	}

	summary := UpstreamAccountSummary{
		SyncedChannelCount: len(syncedChannels),
	}
	for i := range syncedChannels {
		channel := syncedChannels[i]
		snapshot, hasSnapshot := ReadChannelAccountBalanceSnapshot(channel.OtherSettings)
		factor := BuildChannelAssetDisplay(&channel, accountsByChannel[channel.Id]).ConversionFactor
		if hasSnapshot && snapshot.SyncedAt > summary.UpdatedAt {
			summary.UpdatedAt = snapshot.SyncedAt
		}
		if channel.BalanceUpdatedTime > summary.UpdatedAt {
			summary.UpdatedAt = channel.BalanceUpdatedTime
		}

		if hasSnapshot && snapshot.BalanceUSD != nil && finiteNonNegativeFloat(*snapshot.BalanceUSD) {
			summary.UpstreamBalanceUSD += *snapshot.BalanceUSD * factor
		} else if finiteNonNegativeFloat(channel.Balance) {
			summary.UpstreamBalanceUSD += channel.Balance * factor
			// 没有账号级余额或快照中的余额无效时，只能使用渠道余额作为近似值。
			summary.Partial = true
		} else {
			summary.Partial = true
		}

		if hasSnapshot && snapshot.UsedUSD != nil && finiteNonNegativeFloat(*snapshot.UsedUSD) {
			convertedUsedUSD := *snapshot.UsedUSD * factor
			summary.UpstreamUsedUSD += convertedUsedUSD
			summary.UpstreamUsedQuota = saturatingAddInt64(
				summary.UpstreamUsedQuota,
				snapshotUSDToQuotaInt64(&convertedUsedUSD),
			)
		} else {
			if rawUsedUSD, ok := syncedAccountsRawUsedUSD(accountsByChannel[channel.Id]); ok {
				convertedUsedUSD := rawUsedUSD * factor
				summary.UpstreamUsedQuota = saturatingAddInt64(
					summary.UpstreamUsedQuota,
					snapshotUSDToQuotaInt64(&convertedUsedUSD),
				)
				summary.UpstreamUsedUSD += convertedUsedUSD
			}
			summary.Partial = true
		}

		if hasSnapshot && (snapshot.Partial || snapshot.MissingUsedValue || snapshot.MissingBalanceValue) {
			summary.Partial = true
		}
	}

	return summary, nil
}

func loadChannelAccountsByChannel(channelIDs []int) (map[int][]model.ChannelAccount, error) {
	accountsByChannel := make(map[int][]model.ChannelAccount, len(channelIDs))
	if len(channelIDs) == 0 {
		return accountsByChannel, nil
	}
	var accounts []model.ChannelAccount
	if err := model.DB.Where("channel_id IN ?", channelIDs).Find(&accounts).Error; err != nil {
		return nil, err
	}
	for i := range accounts {
		accountsByChannel[accounts[i].ChannelId] = append(accountsByChannel[accounts[i].ChannelId], accounts[i])
	}
	return accountsByChannel, nil
}

func finiteNonNegativeFloat(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
}

func saturatingAddInt64(a int64, b int64) int64 {
	if b <= 0 {
		return a
	}
	if a > math.MaxInt64-b {
		return math.MaxInt64
	}
	return a + b
}
