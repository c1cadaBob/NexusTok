package upstreamaccount

import (
	"context"
	"fmt"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
)

// ChannelBalanceRefreshResult 表示同步渠道余额刷新后已经写入数据库并返回给前端的字段。
type ChannelBalanceRefreshResult struct {
	Balance            float64  `json:"balance"`
	UsedQuota          int64    `json:"used_quota"`
	BalanceUpdatedTime int64    `json:"balance_updated_time"`
	UpstreamBalanceUSD *float64 `json:"upstream_balance_usd,omitempty"`
	UpstreamUsedUSD    *float64 `json:"upstream_used_usd,omitempty"`
	UpstreamUsedQuota  *int64   `json:"upstream_used_quota,omitempty"`
	UpstreamFactor     *float64 `json:"upstream_conversion_factor,omitempty"`
	UpstreamPartial    bool     `json:"upstream_partial,omitempty"`
}

// RefreshChannelBalance 使用已加密保存的上游账号凭据刷新同步渠道的真实账号余额。
//
// 上游账号同步渠道的 Channel.Key 只是 account_pool 占位值，不能用于普通 provider
// balance API。该方法只重新登录上游平台并读取账号维度的余额，然后更新
// Channel.balance / balance_updated_time / settings，不改动任何 key、模型、密钥分组、
// 优先级或权重，也不覆盖 Channel.used_quota。渠道级 used_quota 是 NexusTok 本地
// 经该渠道产生的消费累计，上游账号 used_usd 可能包含站外消耗，不能混写到同一字段。
func RefreshChannelBalance(ctx context.Context, channel *model.Channel) (*ChannelBalanceRefreshResult, error) {
	if channel == nil {
		return nil, fmt.Errorf("渠道不存在")
	}
	credential, ok, err := ReadChannelSyncCredential(channel.OtherSettings)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("当前同步渠道没有保存上游账号凭据，请先在编辑渠道中重新同步上游账号")
	}
	if refreshShouldUsePasswordLogin(credential) {
		// 保存凭据同时包含密码和旧登录态时，余额刷新也必须优先重新登录。
		// 否则即使账号密码已经失效，旧 AT/RT 仍可能短暂成功，掩盖真实凭据问题。
		credential.Session = nil
		credential.AccessToken = ""
		credential.RefreshToken = ""
		credential.ExpiresAt = 0
	}
	client, err := NewPlatformClient(credential.Platform)
	if err != nil {
		return nil, err
	}
	snapshot, err := client.FetchSnapshot(ctx, credential)
	if err != nil {
		return nil, err
	}
	var existingAccounts []model.ChannelAccount
	if err := model.DB.Where("channel_id = ?", channel.Id).Find(&existingAccounts).Error; err != nil {
		return nil, err
	}
	// 余额刷新不会携带前端换算表单；先恢复渠道或密钥 metadata 中的历史配置，
	// 避免一次仅刷新余额的操作把管理员保存的资产换算因子丢掉。
	applySnapshotRatioConversionForRefresh(
		snapshot,
		RatioConversionConfig{},
		channel.OtherSettings,
		existingAccounts,
	)
	attachStoredCredential(snapshot, credential)
	balance := balanceValue(snapshot.Balance)
	updatedTime := common.GetTimestamp()
	mergedSettings := mergeChannelSyncMetadataWithCredential(channel.OtherSettings, snapshot, credential)
	updates := map[string]any{
		"balance":              balance,
		"balance_updated_time": updatedTime,
		"settings":             mergedSettings,
	}
	if baseURL, ok := syncedChannelBaseURLUpdate(*channel, snapshot); ok {
		updates["base_url"] = baseURL
	}
	if err := model.DB.Model(channel).Updates(updates).Error; err != nil {
		return nil, err
	}
	model.InitChannelCache()
	displayChannel := *channel
	displayChannel.OtherSettings = mergedSettings
	displayChannel.Balance = balance
	display := BuildChannelAssetDisplay(&displayChannel, existingAccounts)
	return &ChannelBalanceRefreshResult{
		Balance:            balance,
		UsedQuota:          channel.UsedQuota,
		BalanceUpdatedTime: updatedTime,
		UpstreamBalanceUSD: display.BalanceUSD,
		UpstreamUsedUSD:    display.UsedUSD,
		UpstreamUsedQuota:  display.UsedQuota,
		UpstreamFactor:     pointerFromFloat(display.ConversionFactor),
		UpstreamPartial:    display.Partial,
	}, nil
}

func pointerFromFloat(value float64) *float64 {
	return &value
}
