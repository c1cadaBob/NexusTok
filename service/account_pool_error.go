// account_pool_error.go 实现了账号池账号的错误处理逻辑。
// 当全局账号池中的账号在请求过程中发生错误时，根据错误类型采取不同的处理策略：
// - 认证失败（401/403）或无效 Key：自动禁用账号并标记为不可调度
// - 速率限制（429）：设置临时冷却时间
// - 过载错误（529）：设置较短的冷却时间
// 同时记录请求统计（成功/失败）到滑动窗口。
package service

import (
	"net/http"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/service/accountauth"
	"github.com/c1cada/NexusTok/types"

	"github.com/gin-gonic/gin"
)

// ProcessPoolAccountError 处理账号池账号的请求错误。
// 只更新全局账号池账号的状态，不自动禁用整个渠道。
// 处理策略：
// - 认证失败或无效 Key：自动禁用账号，标记为不可调度
// - 速率限制（429）：设置 rate_limited_until 冷却时间
// - 过载（529）：设置 overload_until 冷却时间
// 同时将当前账号从请求上下文中排除，以便重试时选择其他账号。
//
// 参数：
//   - c: Gin 请求上下文
//   - channelError: 渠道错误信息，包含渠道 ID 和账号 ID
//   - err: 具体的错误详情
func ProcessPoolAccountError(c *gin.Context, channelError types.ChannelError, err *types.NexusTokError) {
	if err == nil || channelError.PoolAccountId <= 0 {
		return
	}
	ExcludePoolAccountForRequest(c, channelError.PoolAccountId)
	common.SetContextKey(c, constant.ContextKeyChannelAccountRetryChannelId, channelError.ChannelId)

	reason := err.ErrorWithStatusCode()
	updates := map[string]interface{}{
		"last_error":     reason,
		"status_message": reason,
		"unavailable":    true,
	}
	recordPoolAccountRequestRuntime(channelError.PoolAccountId, false)

	if shouldDisableChannelAccount(err) && channelError.AutoBan {
		updates["status"] = common.ChannelStatusAutoDisabled
		updates["schedulable"] = false
		updates["disabled_reason"] = reason
		updates["temp_disabled_until"] = 0
		updates["rate_limited_until"] = 0
		updates["overload_until"] = 0
		updates["next_retry_time"] = 0
	} else if err.StatusCode == http.StatusTooManyRequests {
		updates["rate_limited_until"] = retryAfterUntil(err.RetryAfter, defaultChannelAccountRateLimitCooldown)
		updates["disabled_reason"] = reason
		updates["next_retry_time"] = updates["rate_limited_until"]
	} else if isChannelAccountOverloadError(err) {
		updates["overload_until"] = common.GetTimestamp() + int64(defaultChannelAccountOverloadCooldown.Seconds())
		updates["disabled_reason"] = reason
		updates["next_retry_time"] = updates["overload_until"]
	}

	if updateErr := model.UpdatePoolAccountErrorState(channelError.PoolAccountId, updates); updateErr != nil {
		common.SysLog("failed to update pool account error state: " + updateErr.Error())
	}
}

// recordPoolAccountRequestRuntime 记录账号池账号的请求统计。
// 更新最近请求的滑动窗口统计和成功/失败计数。
//
// 参数：
//   - accountID: 账号 ID
//   - success: 请求是否成功
func recordPoolAccountRequestRuntime(accountID int, success bool) {
	if accountID <= 0 {
		return
	}
	account, err := model.GetPoolAccountById(accountID)
	if err != nil || account == nil {
		return
	}
	recentRequests := accountauth.RecordRecentRequest(account.RecentRequests, time.Now(), success)
	model.RecordPoolAccountRequest(accountID, success, recentRequests)
}
