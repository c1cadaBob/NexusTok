// account_pool_quota.go 实现了账号池和渠道账号的用量统计功能。
// 提供统一的接口来累加已选中账号的使用配额，兼容渠道内账号池和全局账号池两种模式。
// 当请求使用全局账号池时，统计到 PoolAccount 维度；否则统计到 ChannelAccount 维度。
package service

import (
	"github.com/c1cada/NexusTok/model"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
)

// AddSelectedAccountUsedQuota 累加已选中账号的使用配额。
// 兼容渠道内账号池和全局账号池的账号维度统计。
// 优先使用全局账号池 ID（poolAccountID），其次使用渠道账号 ID（channelAccountID）。
//
// 参数：
//   - channelAccountID: 渠道账号 ID（渠道内账号池模式）
//   - poolAccountID: 全局账号池账号 ID（全局账号池模式）
//   - quota: 本次消耗的配额值
func AddSelectedAccountUsedQuota(channelAccountID int, poolAccountID int, quota int64) {
	if poolAccountID > 0 {
		if quota > 0 {
			model.AddPoolAccountUsedQuota(poolAccountID, quota)
		}
		recordPoolAccountRequestRuntime(poolAccountID, true)
		return
	}
	if quota == 0 {
		return
	}
	if channelAccountID > 0 {
		model.AddChannelAccountUsedQuota(channelAccountID, quota)
	}
}

// AddRelayAccountUsedQuota 从 RelayInfo 中读取本次实际使用的账号并累加用量。
// 这是一个便捷函数，从 RelayInfo 中提取账号 ID 后委托给 AddSelectedAccountUsedQuota。
//
// 参数：
//   - relayInfo: Relay 信息，包含渠道账号 ID 和全局账号池账号 ID
//   - quota: 本次消耗的配额值
func AddRelayAccountUsedQuota(relayInfo *relaycommon.RelayInfo, quota int64) {
	if relayInfo == nil {
		return
	}
	AddSelectedAccountUsedQuota(relayInfo.ChannelAccountId, relayInfo.PoolAccountId, quota)
}
