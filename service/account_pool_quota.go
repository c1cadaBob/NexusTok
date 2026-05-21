package service

import (
	"github.com/c1cada/NexusTok/model"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
)

// AddSelectedAccountUsedQuota 兼容渠道内账号池和全局账号池的账号维度统计。
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
func AddRelayAccountUsedQuota(relayInfo *relaycommon.RelayInfo, quota int64) {
	if relayInfo == nil {
		return
	}
	AddSelectedAccountUsedQuota(relayInfo.ChannelAccountId, relayInfo.PoolAccountId, quota)
}
