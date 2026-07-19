// channel_account_error.go
// 本文件实现了渠道账户错误处理逻辑。
// 当渠道账户在请求过程中发生错误时，根据错误类型（401/403 认证失败、
// 429 速率限制、529 过载等）采取不同的处理策略：
// - 认证失败：自动禁用账户
// - 速率限制：设置临时冷却时间
// - 过载错误：设置较短的冷却时间
// 同时将当前账户从请求上下文中排除，以便重试时选择其他账户。

package service

import (
	// 标准库
	"net/http"
	"strconv"
	"strings"
	"time"

	// 项目内部包
	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/types"

	// 第三方库：Gin Web 框架
	"github.com/gin-gonic/gin"
)

// 默认的冷却时间常量
const (
	defaultChannelAccountRateLimitCooldown = 5 * time.Minute  // 速率限制（429）的默认冷却时间
	defaultChannelAccountOverloadCooldown  = 10 * time.Minute // 过载（529）的默认冷却时间
)

// ProcessChannelAccountError 处理渠道账户请求错误
// 根据错误类型更新账户状态，并将当前账户从请求上下文中排除
// 处理策略：
// - 认证失败（401/403）或无效 Key：自动禁用账户（如果 AutoBan 为 true）
// - 速率限制（429）：设置 rate_limited_until 冷却时间
// - 过载（529 或包含 "overload"）：设置 overload_until 冷却时间
// 参数:
//   - c: Gin 请求上下文
//   - channelError: 渠道错误信息，包含渠道 ID 和账户 ID
//   - err: 具体的错误详情
func ProcessChannelAccountError(c *gin.Context, channelError types.ChannelError, err *types.NexusTokError) {
	if err == nil || channelError.ChannelAccountId <= 0 {
		return
	}
	// 将当前账户从请求上下文中排除，避免重试时再次使用
	ExcludeChannelAccountForRequest(c, channelError.ChannelAccountId)
	// 设置重试渠道 ID，便于后续重试逻辑
	common.SetContextKey(c, constant.ContextKeyChannelAccountRetryChannelId, channelError.ChannelId)

	reason := err.ErrorWithStatusCode()
	updates := map[string]interface{}{
		"last_error": reason, // 记录最后一次错误信息
	}

	// 根据错误类型决定处理策略
	if shouldDisableChannelAccount(err) && channelError.AutoBan {
		// 认证失败：自动禁用账户，清除所有临时状态
		updates["status"] = common.ChannelStatusAutoDisabled
		updates["disabled_reason"] = reason
		updates["temp_disabled_until"] = 0
		updates["rate_limited_until"] = 0
		updates["overload_until"] = 0
	} else if err.StatusCode == http.StatusTooManyRequests {
		// 速率限制：设置冷却时间
		updates["rate_limited_until"] = retryAfterUntil(err.RetryAfter, defaultChannelAccountRateLimitCooldown)
		updates["disabled_reason"] = reason
	} else if isChannelAccountOverloadError(err) {
		// 过载：设置较短的冷却时间
		updates["overload_until"] = common.GetTimestamp() + int64(defaultChannelAccountOverloadCooldown.Seconds())
		updates["disabled_reason"] = reason
	}

	if updateErr := model.UpdateChannelAccountErrorState(channelError.ChannelId, channelError.ChannelAccountId, updates); updateErr != nil {
		common.SysLog("failed to update channel account error state: " + updateErr.Error())
		return
	}
	if channelAccountErrorUpdatesAffectCapabilities(updates) {
		if syncErr := model.SyncChannelAccountPoolCapabilities(channelError.ChannelId, nil); syncErr != nil {
			common.SysLog("failed to sync channel account capabilities after error: " + syncErr.Error())
			return
		}
		model.InitChannelCache()
		ResetProxyClientCache()
	}
}

// channelAccountErrorUpdatesAffectCapabilities 判断账号错误状态是否会影响渠道可用能力。
//
// 自动禁用会让该账号从账号池候选中移除；限流、过载或临时禁用会让账号进入冷却，
// 同样不应继续参与后续请求调度。只记录 last_error 时不重建能力，避免普通错误日志
// 造成不必要的全量渠道缓存刷新。
func channelAccountErrorUpdatesAffectCapabilities(updates map[string]interface{}) bool {
	for field := range updates {
		switch field {
		case "status", "rate_limited_until", "overload_until", "temp_disabled_until":
			return true
		}
	}
	return false
}

// shouldDisableChannelAccount 判断错误是否应该导致账户被自动禁用
// 以下情况应自动禁用：
// - HTTP 401 Unauthorized（认证失败）
// - HTTP 403 Forbidden（权限不足）
// - 错误码为 ErrorCodeChannelInvalidKey（无效 Key）
// - ShouldDisableChannel 返回 true（其他需要禁用的情况）
// 参数:
//   - err: 错误详情
//
// 返回值:
//   - bool: 是否应该禁用账户
func shouldDisableChannelAccount(err *types.NexusTokError) bool {
	if err == nil {
		return false
	}
	if err.StatusCode == http.StatusUnauthorized || err.StatusCode == http.StatusForbidden {
		return true
	}
	if err.GetErrorCode() == types.ErrorCodeChannelInvalidKey {
		return true
	}
	return ShouldDisableChannel(err)
}

// isChannelAccountOverloadError 判断错误是否为过载错误
// 以下情况视为过载：
// - HTTP 529 状态码（服务过载）
// - 错误信息中包含 "overload"（不区分大小写）
// 参数:
//   - err: 错误详情
//
// 返回值:
//   - bool: 是否为过载错误
func isChannelAccountOverloadError(err *types.NexusTokError) bool {
	if err == nil {
		return false
	}
	if err.StatusCode == 529 {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "overload")
}

// retryAfterUntil 根据 Retry-After 响应头计算冷却结束时间
// 支持两种 Retry-After 格式：
// 1. 秒数格式（如 "120"）：表示多少秒后可重试
// 2. HTTP 日期格式（如 "Fri, 31 Dec 2025 23:59:59 GMT"）：表示具体重试时间
// 如果解析失败，使用默认冷却时间
// 参数:
//   - retryAfter: Retry-After 响应头的值
//   - defaultCooldown: 默认冷却时间
//
// 返回值:
//   - int64: 冷却结束的 Unix 时间戳
func retryAfterUntil(retryAfter string, defaultCooldown time.Duration) int64 {
	now := time.Now()
	retryAfter = strings.TrimSpace(retryAfter)
	if retryAfter == "" {
		return now.Add(defaultCooldown).Unix() // 无 Retry-After 头，使用默认冷却时间
	}
	// 尝试解析为秒数格式
	if seconds, err := strconv.Atoi(retryAfter); err == nil && seconds > 0 {
		return now.Add(time.Duration(seconds) * time.Second).Unix()
	}
	// 尝试解析为 HTTP 日期格式
	if retryTime, err := http.ParseTime(retryAfter); err == nil && retryTime.After(now) {
		return retryTime.Unix()
	}
	return now.Add(defaultCooldown).Unix() // 解析失败，使用默认冷却时间
}
