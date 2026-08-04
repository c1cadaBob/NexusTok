package upstreamaccount

import (
	"context"
	"fmt"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
)

// ChannelBalanceRefreshResult 表示同步渠道余额刷新后已经写入数据库并返回给前端的字段。
type ChannelBalanceRefreshResult struct {
	Balance            float64 `json:"balance"`
	UsedQuota          int64   `json:"used_quota"`
	BalanceUpdatedTime int64   `json:"balance_updated_time"`
}

// RefreshChannelBalance 使用已加密保存的上游账号凭据刷新同步渠道的真实账号余额。
//
// 上游账号同步渠道的 Channel.Key 只是 account_pool 占位值，不能用于普通 provider
// balance API。该方法只重新登录上游平台并读取账号维度的余额、已用额度，然后更新
// Channel.balance / used_quota / balance_updated_time / settings，不改动任何 key、
// 模型、密钥分组、优先级或权重，避免余额刷新误触发同步配置变更。
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
	client, err := NewPlatformClient(credential.Platform)
	if err != nil {
		return nil, err
	}
	snapshot, err := client.FetchSnapshot(ctx, credential)
	if err != nil {
		return nil, err
	}
	attachStoredCredential(snapshot, credential)
	balance := balanceValue(snapshot.Balance)
	usedQuota := usedQuotaValue(snapshot.Balance)
	updatedTime := common.GetTimestamp()
	updates := map[string]any{
		"balance":              balance,
		"balance_updated_time": updatedTime,
		"used_quota":           usedQuota,
		"settings":             mergeChannelSyncMetadataWithCredential(channel.OtherSettings, snapshot, credential),
	}
	if baseURL, ok := syncedChannelBaseURLUpdate(*channel, snapshot); ok {
		updates["base_url"] = baseURL
	}
	if err := model.DB.Model(channel).Updates(updates).Error; err != nil {
		return nil, err
	}
	model.InitChannelCache()
	return &ChannelBalanceRefreshResult{
		Balance:            balance,
		UsedQuota:          usedQuota,
		BalanceUpdatedTime: updatedTime,
	}, nil
}
