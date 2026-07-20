package upstreamaccount

import (
	"context"
	"fmt"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
)

// RefreshChannelBalance 使用已加密保存的上游账号凭据刷新同步渠道的真实账号余额。
//
// 上游账号同步渠道的 Channel.Key 只是 account_pool 占位值，不能用于普通 provider
// balance API。该方法只重新登录上游平台并读取账号维度的余额、已用额度，然后更新
// Channel.balance / used_quota / balance_updated_time / settings，不改动任何 key、
// 模型、密钥分组、优先级或权重，避免余额刷新误触发同步配置变更。
func RefreshChannelBalance(ctx context.Context, channel *model.Channel) (float64, error) {
	if channel == nil {
		return 0, fmt.Errorf("渠道不存在")
	}
	credential, ok, err := ReadChannelSyncCredential(channel.OtherSettings)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, fmt.Errorf("当前同步渠道没有保存上游账号凭据，请先在编辑渠道中重新同步上游账号")
	}
	client, err := NewPlatformClient(credential.Platform)
	if err != nil {
		return 0, err
	}
	snapshot, err := client.FetchSnapshot(ctx, credential)
	if err != nil {
		return 0, err
	}
	attachStoredCredential(snapshot, credential)
	balance := balanceValue(snapshot.Balance)
	updates := map[string]any{
		"balance":              balance,
		"balance_updated_time": common.GetTimestamp(),
		"used_quota":           usedQuotaValue(snapshot.Balance),
		"settings":             mergeChannelSyncMetadataWithCredential(channel.OtherSettings, snapshot, credential),
	}
	if err := model.DB.Model(channel).Updates(updates).Error; err != nil {
		return 0, err
	}
	model.InitChannelCache()
	return balance, nil
}
