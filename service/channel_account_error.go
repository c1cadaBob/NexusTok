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
	"github.com/c1cada/NexusTok/setting/operation_setting"
	"github.com/c1cada/NexusTok/types"

	// 第三方库：Gin Web 框架
	"github.com/gin-gonic/gin"
)

// 默认的冷却时间常量
const (
	defaultChannelAccountRateLimitCooldown          = 5 * time.Minute  // 速率限制（429）的默认冷却时间
	defaultChannelAccountOverloadCooldown           = 10 * time.Minute // 过载（529）的默认冷却时间
	defaultChannelAccountServiceUnavailableCooldown = time.Minute      // 服务暂不可用（503）的默认冷却时间
)

// ProcessChannelAccountError 处理渠道账户请求错误
// 根据错误类型更新账户状态，并将当前账户从请求上下文中排除
// 处理策略：
// - 认证失败（401/403）或无效 Key：自动禁用账户（如果 AutoBan 为 true）
// - 速率限制（429）：设置 rate_limited_until 冷却时间
// - 过载（529、503 或包含 "overload"/"temporarily unavailable"）：设置 overload_until 冷却时间
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

	// 账号错误会持久化并在后台展示。通用脱敏无法识别所有自定义 Key 格式，
	// 因此还要按当前账号的实际凭据精确替换，避免上游正文意外回显密钥。
	reason := sanitizeChannelAccountErrorReason(channelError.ChannelId, channelError.ChannelAccountId, err)
	if processSyncedChannelAccountError(c, channelError, err, reason) {
		return
	}
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
		// 上游暂不可用：优先尊重 Retry-After。503 的默认冷却应短于 529，
		// 让单个临时异常账号快速退出调度，也能在短时间后自动恢复探测。
		updates["overload_until"] = retryAfterUntil(err.RetryAfter, channelAccountOverloadCooldown(err))
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

// MarkSelectedChannelAccountRequestSuccess 在同步密钥真实请求成功后清理连续失败状态。
//
// 普通账号池账号不使用 upstream_account_sync 元数据，保持旧行为不写库。同步密钥只有在
// 之前存在失败计数、错误或自动禁用标记时才更新，避免每次成功请求都产生额外数据库写入。
func MarkSelectedChannelAccountRequestSuccess(c *gin.Context) {
	if c == nil || !common.GetContextKeyBool(c, constant.ContextKeyChannelAccountPool) {
		return
	}
	channelID := common.GetContextKeyInt(c, constant.ContextKeyChannelId)
	accountID := common.GetContextKeyInt(c, constant.ContextKeyChannelAccountId)
	if channelID <= 0 || accountID <= 0 {
		return
	}
	account, err := model.GetChannelAccountById(channelID, accountID)
	if err != nil || account == nil || !channelAccountHasUpstreamSyncMetadata(account.OtherSettings) {
		return
	}
	metadata := readSyncedChannelAccountAutoCheckMetadata(account.OtherSettings)
	if metadata.FailureCount <= 0 &&
		strings.TrimSpace(metadata.LastError) == "" &&
		strings.TrimSpace(account.LastError) == "" &&
		metadata.LastStatus != "failed" &&
		!metadata.DisabledByAutoCheck {
		return
	}
	settings := applySyncedChannelAccountAutoCheckSuccess(account.OtherSettings)
	updates := map[string]interface{}{
		"settings":   settings,
		"last_error": "",
	}
	if err := model.DB.Model(&model.ChannelAccount{}).Where("channel_id = ? AND id = ?", channelID, accountID).Updates(updates).Error; err != nil {
		common.SysLog("failed to mark synced channel account request success: " + err.Error())
	}
}

func processSyncedChannelAccountError(c *gin.Context, channelError types.ChannelError, err *types.NexusTokError, reason string) bool {
	account, loadErr := model.GetChannelAccountById(channelError.ChannelId, channelError.ChannelAccountId)
	if loadErr != nil || account == nil || !channelAccountHasUpstreamSyncMetadata(account.OtherSettings) {
		return false
	}
	errText := sanitizeSyncedChannelAccountError(reason, account)
	if !shouldCountSyncedChannelAccountFailure(err) {
		updates := map[string]interface{}{"last_error": errText}
		if updateErr := model.DB.Model(&model.ChannelAccount{}).Where("channel_id = ? AND id = ?", channelError.ChannelId, channelError.ChannelAccountId).Updates(updates).Error; updateErr != nil {
			common.SysLog("failed to update synced channel account non-countable error: " + updateErr.Error())
		}
		return true
	}

	metadata := readSyncedChannelAccountAutoCheckMetadata(account.OtherSettings)
	failureCount := metadata.FailureCount + 1
	threshold := operation_setting.GetUpstreamAccountKeyCheckSetting().NormalizedFailureThreshold()
	shouldDisable := account.Status == common.ChannelStatusEnabled && failureCount >= threshold
	disabledByAutoCheck := metadata.DisabledByAutoCheck || shouldDisable
	settings := applySyncedChannelAccountAutoCheckFailure(account.OtherSettings, failureCount, errText, disabledByAutoCheck)
	updates := map[string]interface{}{
		"settings":   settings,
		"last_error": errText,
	}
	if shouldDisable {
		updates["status"] = common.ChannelStatusAutoDisabled
		updates["disabled_reason"] = errText
		updates["temp_disabled_until"] = 0
		updates["rate_limited_until"] = 0
		updates["overload_until"] = 0
	}
	if updateErr := model.DB.Model(&model.ChannelAccount{}).Where("channel_id = ? AND id = ?", channelError.ChannelId, channelError.ChannelAccountId).Updates(updates).Error; updateErr != nil {
		common.SysLog("failed to update synced channel account failure state: " + updateErr.Error())
		return true
	}
	if shouldDisable {
		common.SetContextKey(c, constant.ContextKeySyncedChannelAccountAutoDisabledRetry, true)
	}
	if channelAccountErrorUpdatesAffectCapabilities(updates) {
		if syncErr := model.SyncChannelAccountPoolCapabilities(channelError.ChannelId, nil); syncErr != nil {
			common.SysLog("failed to sync channel account capabilities after synced key failure: " + syncErr.Error())
			return true
		}
		model.InitChannelCache()
		ResetProxyClientCache()
	}
	return true
}

// ConsumeSyncedChannelAccountAutoDisabledRetrySignal 消费同步密钥自动禁用后的额外重试信号。
//
// 全局 RetryTimes 默认为 0，但同步密钥达到连续失败阈值时，当前请求应该继续尝试
// 同渠道内下一个启用密钥。该信号只在 ProcessChannelAccountError 判定真实上游失败
// 并自动禁用同步密钥时写入；消费后立即清空，保证额外重试机会是一次性的。
func ConsumeSyncedChannelAccountAutoDisabledRetrySignal(c *gin.Context) bool {
	if c == nil {
		return false
	}
	if !common.GetContextKeyBool(c, constant.ContextKeySyncedChannelAccountAutoDisabledRetry) {
		return false
	}
	common.SetContextKey(c, constant.ContextKeySyncedChannelAccountAutoDisabledRetry, false)
	return true
}

// sanitizeChannelAccountErrorReason 为渠道账号状态持久化生成脱敏错误描述。
//
// 上游可能把 Authorization、query 参数或账号 Key 原样拼进错误正文。先使用统一掩码，
// 再用数据库中当前账号的完整 Key 精确替换，最后限制长度，避免后台状态字段保存敏感
// 信息或异常长的 HTML/JSON 错误页。
func sanitizeChannelAccountErrorReason(channelID int, accountID int, err *types.NexusTokError) string {
	if err == nil {
		return ""
	}
	reason := strings.TrimSpace(err.MaskSensitiveErrorWithStatusCode())
	if accountID > 0 {
		if account, loadErr := model.GetChannelAccountById(channelID, accountID); loadErr == nil && account != nil {
			if key := strings.TrimSpace(account.Key); key != "" {
				reason = strings.ReplaceAll(reason, key, "[redacted-key]")
			}
		}
	}
	if reason == "" {
		reason = "渠道账号请求失败"
	}
	if len([]rune(reason)) > 240 {
		return string([]rune(reason)[:240]) + "..."
	}
	return reason
}

func shouldCountSyncedChannelAccountFailure(err *types.NexusTokError) bool {
	if err == nil {
		return false
	}
	lowerMessage := strings.ToLower(err.Error())
	if strings.Contains(lowerMessage, "client_gone") ||
		strings.Contains(lowerMessage, "client disconnected") ||
		strings.Contains(lowerMessage, "context canceled") ||
		strings.Contains(lowerMessage, "request canceled") {
		return false
	}
	switch err.GetErrorCode() {
	case types.ErrorCodeDoRequestFailed,
		types.ErrorCodeBadResponseStatusCode,
		types.ErrorCodeBadResponse,
		types.ErrorCodeReadResponseBodyFailed,
		types.ErrorCodeBadResponseBody,
		types.ErrorCodeEmptyResponse,
		types.ErrorCodeAwsInvokeError,
		types.ErrorCodeModelNotFound,
		types.ErrorCodeChannelInvalidKey,
		types.ErrorCodeChannelResponseTimeExceeded:
		return true
	default:
		return false
	}
}

func sanitizeSyncedChannelAccountError(errText string, account *model.ChannelAccount) string {
	errText = strings.TrimSpace(errText)
	if errText == "" {
		errText = "同步密钥请求失败"
	}
	errText = common.MaskSensitiveInfo(errText)
	if account != nil {
		if key := strings.TrimSpace(account.Key); key != "" {
			errText = strings.ReplaceAll(errText, key, "[redacted-key]")
		}
	}
	if len([]rune(errText)) > 240 {
		errText = string([]rune(errText)[:240]) + "..."
	}
	return errText
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
// - HTTP 503 或明确的服务暂不可用错误
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
	if isChannelAccountServiceUnavailableError(err) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "overload")
}

// isChannelAccountServiceUnavailableError 判断错误是否表示上游服务暂不可用。
//
// 部分兼容网关会丢失 HTTP 状态码，仅保留错误正文，因此同时识别 503 和文本特征。
func isChannelAccountServiceUnavailableError(err *types.NexusTokError) bool {
	if err == nil {
		return false
	}
	if err.StatusCode == http.StatusServiceUnavailable {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "service temporarily unavailable") ||
		strings.Contains(message, "service unavailable")
}

// channelAccountOverloadCooldown 为过载类错误选择默认冷却时长。
//
// 503 通常表示短暂不可用，不应复用 529 的十分钟冷却；其它过载错误仍保持原有策略。
func channelAccountOverloadCooldown(err *types.NexusTokError) time.Duration {
	if isChannelAccountServiceUnavailableError(err) {
		return defaultChannelAccountServiceUnavailableCooldown
	}
	return defaultChannelAccountOverloadCooldown
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
