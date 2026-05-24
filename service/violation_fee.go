// 本文件 (violation_fee.go) 提供内容违规收费服务。
// 当用户的请求触发内容安全违规（如 CSAM 检测、违反使用准则等）时，
// 系统会在正常计费流程之外额外扣除一笔"违规罚款"。
// 该机制目前主要针对 Grok 模型的内容安全检查。
// 处理流程：检测违规标记 -> 标准化错误 -> 判断是否需要收费 -> 计算罚款金额 -> 扣除配额 -> 记录日志。
package service

import (
	"fmt"       // 格式化输出
	"strings"   // 字符串操作（前缀匹配、内容检测）
	"time"      // 时间计算（请求耗时）

	"github.com/c1cada/NexusTok/common"                    // 项目公共工具包
	"github.com/c1cada/NexusTok/logger"                     // 日志记录
	"github.com/c1cada/NexusTok/model"                      // 数据模型层
	relaycommon "github.com/c1cada/NexusTok/relay/common"   // Relay 通用类型
	"github.com/c1cada/NexusTok/setting/model_setting"      // 模型设置（Grok 配置）
	"github.com/c1cada/NexusTok/types"                      // 类型定义（错误码等）

	"github.com/shopspring/decimal" // 高精度十进制运算库（避免浮点精度问题）

	"github.com/gin-gonic/gin" // Gin Web 框架
)

// 违规收费相关常量
const (
	ViolationFeeCodePrefix     = "violation_fee."                        // 违规费用错误码前缀
	CSAMViolationMarker        = "Failed check: SAFETY_CHECK_TYPE"      // CSAM（儿童性虐待材料）违规标记
	ContentViolatesUsageMarker = "Content violates usage guidelines"     // 内容违反使用准则标记
)

// IsViolationFeeCode 判断给定的错误码是否为违规费用错误码。
// 通过检查错误码是否以 "violation_fee." 前缀开头来判断。
// 参数:
//   - code: 错误码
// 返回值:
//   - bool: true 表示是违规费用错误码
func IsViolationFeeCode(code types.ErrorCode) bool {
	return strings.HasPrefix(string(code), ViolationFeeCodePrefix)
}

// HasCSAMViolationMarker 检查错误是否包含 CSAM 或内容违规标记。
// 通过在错误消息和 OpenAI 格式的错误消息中搜索特定标记字符串来判断。
// 参数:
//   - err: NexusTok 错误对象，可为 nil
// 返回值:
//   - bool: true 表示包含违规标记
func HasCSAMViolationMarker(err *types.NexusTokError) bool {
	if err == nil {
		return false
	}
	// 在原始错误消息中搜索标记
	if strings.Contains(err.Error(), CSAMViolationMarker) || strings.Contains(err.Error(), ContentViolatesUsageMarker) {
		return true
	}
	// 在 OpenAI 格式的错误消息中搜索标记
	msg := err.ToOpenAIError().Message
	return strings.Contains(msg, CSAMViolationMarker) || strings.Contains(err.Error(), ContentViolatesUsageMarker)
}

// WrapAsViolationFeeGrokCSAM 将错误包装为 Grok CSAM 违规费用错误。
// 将错误码和类型设置为 ErrorCodeViolationFeeGrokCSAM，并启用跳过重试标志。
// 参数:
//   - err: 原始 NexusTok 错误对象，可为 nil
// 返回值:
//   - *types.NexusTokError: 包装后的违规费用错误，nil 输入返回 nil
func WrapAsViolationFeeGrokCSAM(err *types.NexusTokError) *types.NexusTokError {
	if err == nil {
		return nil
	}
	oai := err.ToOpenAIError()
	oai.Type = string(types.ErrorCodeViolationFeeGrokCSAM)
	oai.Code = string(types.ErrorCodeViolationFeeGrokCSAM)
	return types.WithOpenAIError(oai, err.StatusCode, types.ErrOptionWithSkipRetry())
}

// NormalizeViolationFeeError 标准化违规费用错误。
// 确保：
// - 如果错误包含 CSAM 标记，将 error.code 设置为稳定的违规费用错误码，并启用跳过重试。
// - 如果 error.code 已经有违规费用前缀，启用跳过重试。
// 该函数必须在重试决策逻辑之前调用。
// 参数:
//   - err: 原始 NexusTok 错误对象，可为 nil
// 返回值:
//   - *types.NexusTokError: 标准化后的错误，nil 输入返回 nil
func NormalizeViolationFeeError(err *types.NexusTokError) *types.NexusTokError {
	if err == nil {
		return nil
	}

	// 包含 CSAM 标记的错误，包装为 Grok CSAM 违规费用错误
	if HasCSAMViolationMarker(err) {
		return WrapAsViolationFeeGrokCSAM(err)
	}

	// 已经是违规费用错误码的，确保启用跳过重试
	if IsViolationFeeCode(err.GetErrorCode()) {
		oai := err.ToOpenAIError()
		return types.WithOpenAIError(oai, err.StatusCode, types.ErrOptionWithSkipRetry())
	}

	return err
}

// shouldChargeViolationFee 判断是否应该收取违规费用。
// 当错误码为 Grok CSAM 违规费用码，或错误包含 CSAM 违规标记时返回 true。
// 参数:
//   - err: NexusTok 错误对象，可为 nil
// 返回值:
//   - bool: true 表示应收取违规费用
func shouldChargeViolationFee(err *types.NexusTokError) bool {
	if err == nil {
		return false
	}
	if err.GetErrorCode() == types.ErrorCodeViolationFeeGrokCSAM {
		return true
	}
	// 安全兜底：调用方可能未调用标准化函数
	return HasCSAMViolationMarker(err)
}

// calcViolationFeeQuota 计算违规费用的配额值。
// 使用高精度十进制运算避免浮点精度问题。
// 公式：金额 * 每单位配额数 * 分组比率，四舍五入取整。
// 参数:
//   - amount: 违规罚款基础金额（美元）
//   - groupRatio: 用户分组的配额比率
// 返回值:
//   - int: 应扣除的配额数量（负数或零时不扣除）
func calcViolationFeeQuota(amount, groupRatio float64) int {
	if amount <= 0 {
		return 0
	}
	if groupRatio <= 0 {
		return 0
	}
	// 使用 decimal 库进行高精度计算，避免浮点数精度丢失
	quota := decimal.NewFromFloat(amount).
		Mul(decimal.NewFromFloat(common.QuotaPerUnit)).
		Mul(decimal.NewFromFloat(groupRatio)).
		Round(0).
		IntPart()
	if quota <= 0 {
		return 0
	}
	return int(quota)
}

// ChargeViolationFeeIfNeeded 在正常流程完成后（包括退款）收取额外的违规费用。
// 使用 Grok 费用设置作为收费策略。
// 处理流程：
// 1. 检查是否需要收取违规费用
// 2. 获取 Grok 违规扣费配置
// 3. 计算违规费用配额
// 4. 扣除配额并更新用户和渠道的使用量
// 5. 记录违规费用消费日志
// 参数:
//   - ctx: Gin 上下文
//   - relayInfo: Relay 请求信息（包含用户、渠道、分组等信息）
//   - apiErr: 上游 API 返回的错误
// 返回值:
//   - bool: true 表示成功收取了违规费用
func ChargeViolationFeeIfNeeded(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, apiErr *types.NexusTokError) bool {
	if ctx == nil || relayInfo == nil || apiErr == nil {
		return false
	}
	// 检查是否需要收取违规费用
	if !shouldChargeViolationFee(apiErr) {
		return false
	}

	// 获取 Grok 违规扣费配置
	settings := model_setting.GetGrokSettings()
	if settings == nil || !settings.ViolationDeductionEnabled {
		return false
	}

	// 计算违规费用配额
	groupRatio := relayInfo.PriceData.GroupRatioInfo.GroupRatio
	feeQuota := calcViolationFeeQuota(settings.ViolationDeductionAmount, groupRatio)
	if feeQuota <= 0 {
		return false
	}

	// 扣除配额
	if err := PostConsumeQuota(relayInfo, feeQuota, 0, true); err != nil {
		logger.LogError(ctx, fmt.Sprintf("failed to charge violation fee: %s", err.Error()))
		return false
	}

	// 更新用户和渠道的使用量统计
	model.UpdateUserUsedQuotaAndRequestCount(relayInfo.UserId, feeQuota)
	model.UpdateChannelUsedQuota(relayInfo.ChannelId, feeQuota)
	AddRelayAccountUsedQuota(relayInfo, int64(feeQuota))

	// 构建违规费用的详细信息，用于日志记录
	useTimeSeconds := time.Now().Unix() - relayInfo.StartTime.Unix()
	tokenName := ctx.GetString("token_name")
	oai := apiErr.ToOpenAIError()

	other := map[string]any{
		"violation_fee":        true,                                               // 标记为违规费用
		"violation_fee_code":   string(types.ErrorCodeViolationFeeGrokCSAM),        // 违规费用错误码
		"fee_quota":            feeQuota,                                           // 实际扣除的配额
		"base_amount":          settings.ViolationDeductionAmount,                  // 基础罚款金额
		"group_ratio":          groupRatio,                                         // 分组比率
		"status_code":          apiErr.StatusCode,                                  // HTTP 状态码
		"upstream_error_type":  oai.Type,                                           // 上游错误类型
		"upstream_error_code":  fmt.Sprintf("%v", oai.Code),                        // 上游错误码
		"violation_fee_marker": CSAMViolationMarker,                                // 违规标记
	}

	// 记录消费日志
	model.RecordConsumeLog(ctx, relayInfo.UserId, model.RecordConsumeLogParams{
		ChannelId:      relayInfo.ChannelId,
		ModelName:      relayInfo.OriginModelName,
		TokenName:      tokenName,
		Quota:          feeQuota,
		Content:        "Violation fee charged",
		TokenId:        relayInfo.TokenId,
		UseTimeSeconds: int(useTimeSeconds),
		IsStream:       relayInfo.IsStream,
		Group:          relayInfo.UsingGroup,
		Other:          other,
	})

	return true
}
