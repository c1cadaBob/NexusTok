// account_pool_quota.go 实现了账号池和渠道账号的用量统计功能。
// 提供统一的接口来累加已选中账号的使用配额，兼容渠道内账号池和全局账号池两种模式。
// 当请求使用全局账号池时，统计到 PoolAccount 维度；否则统计到 ChannelAccount 维度。
package service

import (
	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
	relaycommon "github.com/c1cada/NexusTok/relay/common"

	"github.com/gin-gonic/gin"
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
	AddSelectedAccountUsedQuotaWithGroup(channelAccountID, 0, poolAccountID, quota)
}

// AddSelectedAccountUsedQuotaWithGroup 累加已选中账号及其账号池分组的使用配额。
// poolGroupID 由 RelayInfo 直接传入时优先使用；异步任务等旧路径只持有账号 ID 时，
// 会回查 PoolAccount 获得分组 ID，保证组级配额统计尽量不漏记。
func AddSelectedAccountUsedQuotaWithGroup(channelAccountID int, poolGroupID int, poolAccountID int, quota int64) {
	if poolAccountID > 0 {
		recordPoolAccountRequestRuntime(poolAccountID, true)
		if quota > 0 {
			model.AddPoolAccountUsedQuota(poolAccountID, quota)
			if poolGroupID <= 0 {
				if account, err := model.GetPoolAccountById(poolAccountID); err == nil && account != nil {
					poolGroupID = account.PoolGroupId
				}
			}
			if poolGroupID > 0 {
				model.AddAccountPoolGroupUsedQuota(poolGroupID, quota)
			}
		}
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
	AddSelectedAccountUsedQuotaWithGroup(relayInfo.ChannelAccountId, relayInfo.PoolGroupId, relayInfo.PoolAccountId, quota)
}

// poolAccountUsageLogRecordFromContext 从请求上下文构造账号池使用日志快照。
// 成功和失败路径共用该函数，确保账号池组、账号、渠道、用户、请求 ID 等展示字段保持一致。
func poolAccountUsageLogRecordFromContext(c *gin.Context, relayInfo *relaycommon.RelayInfo, success bool) model.PoolAccountUsageLogRecord {
	record := model.PoolAccountUsageLogRecord{Success: success}
	if relayInfo != nil {
		if relayInfo.ChannelMeta != nil {
			record.PoolGroupId = relayInfo.PoolGroupId
			record.PoolGroupName = relayInfo.PoolGroupName
			record.PoolAccountId = relayInfo.PoolAccountId
			record.PoolAccountName = relayInfo.PoolAccountName
			record.PoolAccountAuthType = relayInfo.PoolAccountAuthType
			record.ChannelId = relayInfo.ChannelId
		}
		record.ModelName = relayInfo.OriginModelName
		record.UserId = relayInfo.UserId
		record.TokenId = relayInfo.TokenId
		record.Group = relayInfo.UsingGroup
		record.IsStream = relayInfo.IsStream
		record.RequestId = relayInfo.RequestId
		record.RetryIndex = relayInfo.RetryIndex
		if relayInfo.IsModelMapped && relayInfo.UpstreamModelName != "" {
			record.ModelName = relayInfo.UpstreamModelName
		}
	}
	if c == nil {
		return record
	}
	if record.PoolGroupId <= 0 {
		record.PoolGroupId = common.GetContextKeyInt(c, constant.ContextKeyPoolGroupId)
	}
	if record.PoolGroupName == "" {
		record.PoolGroupName = common.GetContextKeyString(c, constant.ContextKeyPoolGroupName)
	}
	if record.PoolAccountId <= 0 {
		record.PoolAccountId = common.GetContextKeyInt(c, constant.ContextKeyPoolAccountId)
	}
	if record.PoolAccountName == "" {
		record.PoolAccountName = common.GetContextKeyString(c, constant.ContextKeyPoolAccountName)
	}
	if record.PoolAccountAuthType == "" {
		record.PoolAccountAuthType = common.GetContextKeyString(c, constant.ContextKeyPoolAccountAuthType)
	}
	if record.ChannelId <= 0 {
		record.ChannelId = common.GetContextKeyInt(c, constant.ContextKeyChannelId)
	}
	record.ChannelName = common.GetContextKeyString(c, constant.ContextKeyChannelName)
	if record.ModelName == "" {
		record.ModelName = common.GetContextKeyString(c, constant.ContextKeyOriginalModel)
	}
	if record.UserId <= 0 {
		record.UserId = common.GetContextKeyInt(c, constant.ContextKeyUserId)
	}
	record.Username = c.GetString(string(constant.ContextKeyUserName))
	if record.TokenId <= 0 {
		record.TokenId = common.GetContextKeyInt(c, constant.ContextKeyTokenId)
	}
	record.TokenName = c.GetString("token_name")
	if record.Group == "" {
		record.Group = common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	}
	if !record.IsStream {
		record.IsStream = common.GetContextKeyBool(c, constant.ContextKeyIsStream)
	}
	if record.RequestId == "" {
		record.RequestId = c.GetString(common.RequestIdKey)
	}
	record.UpstreamRequestId = c.GetString(common.UpstreamRequestIdKey)
	return record
}

// RecordRelayPoolAccountUsageSuccess 记录原生账号池账号的一次成功使用日志。
// 该日志是账号池管理页的可观测性数据源，使用请求上下文和 RelayInfo 中的快照信息，避免日志写入再反查主库。
func RecordRelayPoolAccountUsageSuccess(c *gin.Context, relayInfo *relaycommon.RelayInfo, promptTokens int, completionTokens int, quota int, useTimeSeconds int) {
	if relayInfo == nil || relayInfo.ChannelMeta == nil || relayInfo.PoolAccountId <= 0 {
		return
	}
	record := poolAccountUsageLogRecordFromContext(c, relayInfo, true)
	record.Quota = quota
	record.PromptTokens = promptTokens
	record.CompletionTokens = completionTokens
	record.UseTime = useTimeSeconds
	model.RecordPoolAccountUsageLog(record)
}

// RecordSelectedPoolAccountUsageSuccess 从 Gin 上下文记录异步任务等场景中的成功日志。
// 任务提交路径不会总是持有完整的 RelayInfo 计费明细，因此该函数只记录可确定的快照字段。
func RecordSelectedPoolAccountUsageSuccess(c *gin.Context, relayInfo *relaycommon.RelayInfo, quota int) {
	if c == nil && relayInfo == nil {
		return
	}
	poolAccountID := 0
	if relayInfo != nil && relayInfo.ChannelMeta != nil {
		poolAccountID = relayInfo.PoolAccountId
	}
	if poolAccountID <= 0 && c != nil {
		poolAccountID = common.GetContextKeyInt(c, constant.ContextKeyPoolAccountId)
	}
	if poolAccountID <= 0 {
		return
	}
	record := poolAccountUsageLogRecordFromContext(c, relayInfo, true)
	record.Quota = quota
	model.RecordPoolAccountUsageLog(record)
}
