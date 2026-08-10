// 本文件 (log_info_generate.go) 提供消费日志附加信息的生成功能。
// 在 AI API 请求完成后，需要记录详细的计费和调试信息到消费日志中。
// 本文件为不同类型的请求（文本、音频、WebSocket、Claude、Midjourney 等）
// 生成对应的附加信息 (other) 映射，包含：
// - 计费参数（模型比率、分组比率、补全比率、缓存比率等）
// - 调试信息（请求路径、首次响应时间、模型映射等）
// - 管理员信息（使用的渠道、凭证模式、多 Key 信息、账号池信息等）
// - 计费模式信息（订阅/钱包、分层计费表达式等）
// - 流式状态信息（是否正常结束、错误计数等）
package service

import (
	"encoding/base64" // Base64 编码（用于编码计费表达式）
	"fmt"
	"math"
	"strings" // 字符串操作

	"github.com/c1cada/NexusTok/common"                   // 项目公共工具包
	"github.com/c1cada/NexusTok/constant"                 // 常量定义（上下文键、Relay 格式等）
	"github.com/c1cada/NexusTok/dto"                      // 数据传输对象（Usage、RealtimeUsage 等）
	"github.com/c1cada/NexusTok/logger"                   // 日志
	"github.com/c1cada/NexusTok/pkg/billingexpr"          // 计费表达式引擎
	relaycommon "github.com/c1cada/NexusTok/relay/common" // Relay 通用类型（RelayInfo 等）
	"github.com/c1cada/NexusTok/types"                    // 类型定义（Relay 格式、PriceData 等）

	"github.com/gin-gonic/gin" // Gin Web 框架
)

// AttachQuotaSaturationToOther 将配额饱和事件写入 other.admin_info.quota_saturation。
// admin_info 会在普通用户日志视图中被整体移除，因此这里适合存放计费异常、
// 配置错误或攻击流量相关的管理员审计信息。clamp 为空表示本次请求没有饱和事件。
func AttachQuotaSaturationToOther(other map[string]interface{}, clamp *common.QuotaClamp) {
	if other == nil || clamp == nil {
		return
	}
	adminInfo, ok := other["admin_info"].(map[string]interface{})
	if !ok || adminInfo == nil {
		adminInfo = map[string]interface{}{}
		other["admin_info"] = adminInfo
	}
	adminInfo["quota_saturation"] = clamp.AuditMap()
}

// AttachQuotaSaturation 将 RelayInfo 上记录的首个配额饱和事件附加到消费日志。
// 调用点应位于 RecordConsumeLog/RecordTaskBillingLog 之前，确保日志和后端警告
// 同时包含 op、kind、原始值、饱和值以及请求所属用户/模型。
func AttachQuotaSaturation(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, other map[string]interface{}) {
	if relayInfo == nil || relayInfo.QuotaClamp == nil {
		return
	}
	clamp := relayInfo.QuotaClamp
	AttachQuotaSaturationToOther(other, clamp)
	message := fmt.Sprintf("配额换算触发饱和保护：op=%s kind=%s original=%g clamped=%d user=%d model=%s",
		clamp.Op, clamp.Kind, clamp.Original, clamp.Clamped, relayInfo.UserId, relayInfo.OriginModelName)
	if ctx != nil {
		logger.LogWarn(ctx, message)
		return
	}
	common.SysLog(message)
}

// appendRequestPath 将请求路径追加到附加信息映射中。
// 优先从 Gin 上下文中获取请求路径，如果不可用则从 RelayInfo 中获取。
// RelayInfo 中的路径可能包含查询参数，会被截断。
// 参数:
//   - ctx: Gin 上下文，可为 nil
//   - relayInfo: Relay 请求信息，可为 nil
//   - other: 附加信息映射，可为 nil
func appendRequestPath(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, other map[string]interface{}) {
	if other == nil {
		return
	}
	// 优先从 Gin 上下文中获取请求路径
	if ctx != nil && ctx.Request != nil && ctx.Request.URL != nil {
		if path := ctx.Request.URL.Path; path != "" {
			other["request_path"] = path
			return
		}
	}
	// 从 RelayInfo 中获取（可能包含查询参数，需要截断）
	if relayInfo != nil && relayInfo.RequestURLPath != "" {
		path := relayInfo.RequestURLPath
		if idx := strings.Index(path, "?"); idx != -1 {
			path = path[:idx]
		}
		other["request_path"] = path
	}
}

// GenerateTextOtherInfo 生成文本请求的附加信息。
// 这是最基础的附加信息生成函数，其他类型（音频、WebSocket 等）会在此基础上扩展。
// 生成的信息包括：
// - 计费参数：模型比率、分组比率、补全比率、缓存 Token 数/比率、模型价格、用户分组比率
// - 性能指标：首次响应时间 (FRT)
// - 模型映射信息
// - 系统提示词覆盖标记
// - 管理员信息（渠道、凭证模式、多 Key、账号池等）
// - 请求路径、请求格式转换链、最终请求格式
// - 计费模式信息（订阅/钱包）
// - 参数覆盖审计信息
// - 流式状态信息
// 参数:
//   - ctx: Gin 上下文
//   - relayInfo: Relay 请求信息
//   - modelRatio: 模型比率
//   - groupRatio: 分组比率
//   - completionRatio: 补全比率（输出 Token 相对输入 Token 的价格倍数）
//   - cacheTokens: 缓存命中的 Token 数
//   - cacheRatio: 缓存 Token 的价格比率
//   - modelPrice: 模型固定价格
//   - userGroupRatio: 用户分组比率
//
// 返回值:
//   - map[string]interface{}: 附加信息映射
func GenerateTextOtherInfo(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, modelRatio, groupRatio, completionRatio float64,
	cacheTokens int, cacheRatio float64, modelPrice float64, userGroupRatio float64) map[string]interface{} {
	other := make(map[string]interface{})
	// 基础计费参数
	other["model_ratio"] = modelRatio
	other["group_ratio"] = groupRatio
	other["completion_ratio"] = completionRatio
	other["cache_tokens"] = cacheTokens
	other["cache_ratio"] = cacheRatio
	other["model_price"] = modelPrice
	other["user_group_ratio"] = userGroupRatio
	// 首次响应时间（毫秒），用于性能监控
	other["frt"] = float64(relayInfo.FirstResponseTime.UnixMilli() - relayInfo.StartTime.UnixMilli())
	// 推理努力程度（如 o1 模型的 reasoning_effort 参数）
	if relayInfo.ReasoningEffort != "" {
		other["reasoning_effort"] = relayInfo.ReasoningEffort
	}
	// 模型映射信息（当请求模型被映射到上游实际模型时）
	if relayInfo.IsModelMapped {
		other["is_model_mapped"] = true
		other["upstream_model_name"] = relayInfo.UpstreamModelName
	}

	// 系统提示词覆盖标记
	isSystemPromptOverwritten := common.GetContextKeyBool(ctx, constant.ContextKeySystemPromptOverride)
	if isSystemPromptOverwritten {
		other["is_system_prompt_overwritten"] = true
	}

	// 构建管理员信息（仅供管理员查看的调试信息）
	adminInfo := make(map[string]interface{})
	adminInfo["use_channel"] = ctx.GetStringSlice("use_channel")
	if credentialMode := common.GetContextKeyString(ctx, constant.ContextKeyChannelCredentialMode); credentialMode != "" {
		adminInfo["credential_mode"] = credentialMode
	}
	// 上游成本倍率仅供管理员日志展示“费用 × 上游换算倍率”的估算成本。
	// 倍率来自本次实际命中的同步账号元数据；普通用户日志会在 model 层移除整个 admin_info。
	if ratioConversion, ok := common.GetContextKeyType[float64](ctx, constant.ContextKeyUpstreamRatioConversion); ok &&
		ratioConversion > 0 &&
		!math.IsNaN(ratioConversion) &&
		!math.IsInf(ratioConversion, 0) {
		adminInfo["ratio_conversion"] = ratioConversion
	}
	// 多 Key 模式信息
	isMultiKey := common.GetContextKeyBool(ctx, constant.ContextKeyChannelIsMultiKey)
	if isMultiKey {
		adminInfo["is_multi_key"] = true
		adminInfo["multi_key_index"] = common.GetContextKeyInt(ctx, constant.ContextKeyChannelMultiKeyIndex)
	}
	// 账号池信息
	if common.GetContextKeyBool(ctx, constant.ContextKeyChannelAccountPool) {
		adminInfo["account_pool"] = true
		adminInfo["channel_account_id"] = common.GetContextKeyInt(ctx, constant.ContextKeyChannelAccountId)
		adminInfo["channel_account_name"] = common.GetContextKeyString(ctx, constant.ContextKeyChannelAccountName)
		// 账号池分组信息
		if poolGroupID := common.GetContextKeyInt(ctx, constant.ContextKeyPoolGroupId); poolGroupID > 0 {
			adminInfo["pool_group_id"] = poolGroupID
			adminInfo["pool_group_name"] = common.GetContextKeyString(ctx, constant.ContextKeyPoolGroupName)
		}
		// 账号池账号信息
		if poolAccountID := common.GetContextKeyInt(ctx, constant.ContextKeyPoolAccountId); poolAccountID > 0 {
			adminInfo["pool_account_id"] = poolAccountID
			adminInfo["pool_account_name"] = common.GetContextKeyString(ctx, constant.ContextKeyPoolAccountName)
			adminInfo["pool_account_auth_type"] = common.GetContextKeyString(ctx, constant.ContextKeyPoolAccountAuthType)
		} else if authType := common.GetContextKeyString(ctx, constant.ContextKeyPoolAccountAuthType); authType != "" {
			adminInfo["pool_account_name"] = common.GetContextKeyString(ctx, constant.ContextKeyPoolAccountName)
			adminInfo["pool_account_auth_type"] = authType
		}
	}

	// 本地 Token 计数标记
	isLocalCountTokens := common.GetContextKeyBool(ctx, constant.ContextKeyLocalCountTokens)
	if isLocalCountTokens {
		adminInfo["local_count_tokens"] = isLocalCountTokens
	}

	// 渠道亲和性信息
	AppendChannelAffinityAdminInfo(ctx, adminInfo)

	other["admin_info"] = adminInfo
	// 追加各类附加信息
	appendRequestPath(ctx, relayInfo, other)       // 请求路径
	appendRequestConversionChain(relayInfo, other) // 请求格式转换链
	appendFinalRequestFormat(relayInfo, other)     // 最终请求格式
	appendBillingInfo(relayInfo, other)            // 计费模式信息
	appendParamOverrideInfo(relayInfo, other)      // 参数覆盖审计
	appendStreamStatus(relayInfo, other)           // 流式状态
	return other
}

// appendParamOverrideInfo 将参数覆盖审计信息追加到附加信息映射中。
// 当请求中存在参数覆盖（如温度、top_p 等被管理员强制设置）时，
// 记录覆盖详情以便审计。
// 参数:
//   - relayInfo: Relay 请求信息
//   - other: 附加信息映射
func appendParamOverrideInfo(relayInfo *relaycommon.RelayInfo, other map[string]interface{}) {
	if relayInfo == nil || other == nil || len(relayInfo.ParamOverrideAudit) == 0 {
		return
	}
	other["po"] = relayInfo.ParamOverrideAudit
}

// appendStreamStatus 将流式响应状态信息追加到附加信息映射中。
// 仅对流式请求生效，记录流式传输是否正常结束、结束原因、错误信息等。
// 状态值："ok" 表示正常结束，"error" 表示异常结束或有错误。
// 参数:
//   - relayInfo: Relay 请求信息
//   - other: 附加信息映射
func appendStreamStatus(relayInfo *relaycommon.RelayInfo, other map[string]interface{}) {
	if relayInfo == nil || other == nil || !relayInfo.IsStream || relayInfo.StreamStatus == nil {
		return
	}
	ss := relayInfo.StreamStatus
	// 判断流式状态：正常结束且无错误为 "ok"，否则为 "error"
	status := "ok"
	if !ss.IsNormalEnd() || ss.HasErrors() {
		status = "error"
	}
	streamInfo := map[string]interface{}{
		"status":     status,
		"end_reason": string(ss.EndReason),
	}
	if ss.EndError != nil {
		streamInfo["end_error"] = ss.EndError.Error()
	}
	// 记录错误详情
	if ss.ErrorCount > 0 {
		streamInfo["error_count"] = ss.ErrorCount
		messages := make([]string, 0, len(ss.Errors))
		for _, e := range ss.Errors {
			messages = append(messages, e.Message)
		}
		streamInfo["errors"] = messages
	}
	other["stream_status"] = streamInfo
}

// appendBillingInfo 将计费模式信息追加到附加信息映射中。
// 记录用户使用的是钱包余额还是订阅套餐进行计费，
// 以及订阅相关的详细信息（订阅 ID、预扣金额、结算差额、套餐信息等）。
// 参数:
//   - relayInfo: Relay 请求信息
//   - other: 附加信息映射
func appendBillingInfo(relayInfo *relaycommon.RelayInfo, other map[string]interface{}) {
	if relayInfo == nil || other == nil {
		return
	}
	// 计费来源："wallet"（钱包）或 "subscription"（订阅）
	if relayInfo.BillingSource != "" {
		other["billing_source"] = relayInfo.BillingSource
	}
	if relayInfo.UserSetting.BillingPreference != "" {
		other["billing_preference"] = relayInfo.UserSetting.BillingPreference
	}
	// 订阅模式的详细信息
	if relayInfo.BillingSource == "subscription" {
		if relayInfo.SubscriptionId != 0 {
			other["subscription_id"] = relayInfo.SubscriptionId
		}
		if relayInfo.SubscriptionPreConsumed > 0 {
			other["subscription_pre_consumed"] = relayInfo.SubscriptionPreConsumed
		}
		// 结算差额：实际使用量确定后的调整值（负数表示退款）
		if relayInfo.SubscriptionPostDelta != 0 {
			other["subscription_post_delta"] = relayInfo.SubscriptionPostDelta
		}
		if relayInfo.SubscriptionPlanId != 0 {
			other["subscription_plan_id"] = relayInfo.SubscriptionPlanId
		}
		if relayInfo.SubscriptionPlanTitle != "" {
			other["subscription_plan_title"] = relayInfo.SubscriptionPlanTitle
		}
		// 计算本次请求的订阅消耗量和剩余量
		consumed := relayInfo.SubscriptionPreConsumed + relayInfo.SubscriptionPostDelta
		usedFinal := relayInfo.SubscriptionAmountUsedAfterPreConsume + relayInfo.SubscriptionPostDelta
		if consumed < 0 {
			consumed = 0
		}
		if usedFinal < 0 {
			usedFinal = 0
		}
		if relayInfo.SubscriptionAmountTotal > 0 {
			remain := relayInfo.SubscriptionAmountTotal - usedFinal
			if remain < 0 {
				remain = 0
			}
			other["subscription_total"] = relayInfo.SubscriptionAmountTotal
			other["subscription_used"] = usedFinal
			other["subscription_remain"] = remain
		}
		if consumed > 0 {
			other["subscription_consumed"] = consumed
		}
		// 订阅模式下不扣除钱包配额
		other["wallet_quota_deducted"] = 0
	}
}

// appendRequestConversionChain 将请求格式转换链追加到附加信息映射中。
// 当请求在 Relay 过程中经历了格式转换（如 OpenAI -> Claude -> Gemini），
// 记录转换链以便调试和排查问题。
// 参数:
//   - relayInfo: Relay 请求信息
//   - other: 附加信息映射
func appendRequestConversionChain(relayInfo *relaycommon.RelayInfo, other map[string]interface{}) {
	if relayInfo == nil || other == nil {
		return
	}
	if len(relayInfo.RequestConversionChain) == 0 {
		return
	}
	// 将内部格式标识转换为可读的格式名称
	chain := make([]string, 0, len(relayInfo.RequestConversionChain))
	for _, f := range relayInfo.RequestConversionChain {
		switch f {
		case types.RelayFormatOpenAI:
			chain = append(chain, "OpenAI Compatible")
		case types.RelayFormatClaude:
			chain = append(chain, "Claude Messages")
		case types.RelayFormatGemini:
			chain = append(chain, "Google Gemini")
		case types.RelayFormatOpenAIResponses:
			chain = append(chain, "OpenAI Responses")
		default:
			chain = append(chain, string(f))
		}
	}
	if len(chain) == 0 {
		return
	}
	other["request_conversion"] = chain
}

// appendFinalRequestFormat 将最终请求格式信息追加到附加信息映射中。
// 当最终上游请求格式为 Claude Messages 时，设置 "claude" 标记为 true。
// 前端日志渲染使用此标记来保持原始 Claude 输入的显示方式。
// 参数:
//   - relayInfo: Relay 请求信息
//   - other: 附加信息映射
func appendFinalRequestFormat(relayInfo *relaycommon.RelayInfo, other map[string]interface{}) {
	if relayInfo == nil || other == nil {
		return
	}
	if relayInfo.GetFinalRequestRelayFormat() == types.RelayFormatClaude {
		other["claude"] = true
	}
}

// GenerateWssOtherInfo 生成 WebSocket (实时音频) 请求的附加信息。
// 在文本请求附加信息的基础上，增加音频相关的 Token 信息和比率。
// 参数:
//   - ctx: Gin 上下文
//   - relayInfo: Relay 请求信息
//   - usage: 实时音频使用量（包含音频/文本的输入输出 Token 数）
//   - modelRatio: 模型比率
//   - groupRatio: 分组比率
//   - completionRatio: 补全比率
//   - audioRatio: 音频输入比率
//   - audioCompletionRatio: 音频补全比率
//   - modelPrice: 模型价格
//   - userGroupRatio: 用户分组比率
//
// 返回值:
//   - map[string]interface{}: 附加信息映射
func GenerateWssOtherInfo(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.RealtimeUsage, modelRatio, groupRatio, completionRatio, audioRatio, audioCompletionRatio, modelPrice, userGroupRatio float64) map[string]interface{} {
	// 复用文本请求的基础信息
	info := GenerateTextOtherInfo(ctx, relayInfo, modelRatio, groupRatio, completionRatio, 0, 0.0, modelPrice, userGroupRatio)
	info["ws"] = true // 标记为 WebSocket 请求
	// 音频和文本的 Token 详情
	info["audio_input"] = usage.InputTokenDetails.AudioTokens
	info["audio_output"] = usage.OutputTokenDetails.AudioTokens
	info["text_input"] = usage.InputTokenDetails.TextTokens
	info["text_output"] = usage.OutputTokenDetails.TextTokens
	info["audio_ratio"] = audioRatio
	info["audio_completion_ratio"] = audioCompletionRatio
	return info
}

// GenerateAudioOtherInfo 生成音频请求的附加信息。
// 在文本请求附加信息的基础上，增加音频相关的 Token 信息和比率。
// 与 WebSocket 不同，此函数处理的是非实时的音频请求（如 TTS、语音转文字等）。
// 参数:
//   - ctx: Gin 上下文
//   - relayInfo: Relay 请求信息
//   - usage: 标准使用量（包含音频/文本的输入输出 Token 详情）
//   - modelRatio: 模型比率
//   - groupRatio: 分组比率
//   - completionRatio: 补全比率
//   - audioRatio: 音频输入比率
//   - audioCompletionRatio: 音频补全比率
//   - modelPrice: 模型价格
//   - userGroupRatio: 用户分组比率
//
// 返回值:
//   - map[string]interface{}: 附加信息映射
func GenerateAudioOtherInfo(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.Usage, modelRatio, groupRatio, completionRatio, audioRatio, audioCompletionRatio, modelPrice, userGroupRatio float64) map[string]interface{} {
	info := GenerateTextOtherInfo(ctx, relayInfo, modelRatio, groupRatio, completionRatio, 0, 0.0, modelPrice, userGroupRatio)
	info["audio"] = true // 标记为音频请求
	info["audio_input"] = usage.PromptTokensDetails.AudioTokens
	info["audio_output"] = usage.CompletionTokenDetails.AudioTokens
	info["text_input"] = usage.PromptTokensDetails.TextTokens
	info["text_output"] = usage.CompletionTokenDetails.TextTokens
	info["audio_ratio"] = audioRatio
	info["audio_completion_ratio"] = audioCompletionRatio
	return info
}

// GenerateClaudeOtherInfo 生成 Claude 模型请求的附加信息。
// 在文本请求附加信息的基础上，增加 Claude 特有的缓存创建 Token 信息。
// Claude 支持多级缓存：标准缓存、5 分钟缓存、1 小时缓存，各有不同的价格比率。
// 参数:
//   - ctx: Gin 上下文
//   - relayInfo: Relay 请求信息
//   - modelRatio: 模型比率
//   - groupRatio: 分组比率
//   - completionRatio: 补全比率
//   - cacheTokens: 标准缓存命中的 Token 数
//   - cacheRatio: 标准缓存价格比率
//   - cacheCreationTokens: 缓存创建 Token 数
//   - cacheCreationRatio: 缓存创建价格比率
//   - cacheCreationTokens5m: 5 分钟缓存创建 Token 数
//   - cacheCreationRatio5m: 5 分钟缓存创建价格比率
//   - cacheCreationTokens1h: 1 小时缓存创建 Token 数
//   - cacheCreationRatio1h: 1 小时缓存创建价格比率
//   - modelPrice: 模型价格
//   - userGroupRatio: 用户分组比率
//
// 返回值:
//   - map[string]interface{}: 附加信息映射
func GenerateClaudeOtherInfo(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, modelRatio, groupRatio, completionRatio float64,
	cacheTokens int, cacheRatio float64,
	cacheCreationTokens int, cacheCreationRatio float64,
	cacheCreationTokens5m int, cacheCreationRatio5m float64,
	cacheCreationTokens1h int, cacheCreationRatio1h float64,
	modelPrice float64, userGroupRatio float64) map[string]interface{} {
	// 复用文本请求的基础信息
	info := GenerateTextOtherInfo(ctx, relayInfo, modelRatio, groupRatio, completionRatio, cacheTokens, cacheRatio, modelPrice, userGroupRatio)
	info["claude"] = true // 标记为 Claude 模型
	// 标准缓存创建信息
	info["cache_creation_tokens"] = cacheCreationTokens
	info["cache_creation_ratio"] = cacheCreationRatio
	// 5 分钟缓存创建信息（仅在有数据时记录）
	if cacheCreationTokens5m != 0 {
		info["cache_creation_tokens_5m"] = cacheCreationTokens5m
		info["cache_creation_ratio_5m"] = cacheCreationRatio5m
	}
	// 1 小时缓存创建信息（仅在有数据时记录）
	if cacheCreationTokens1h != 0 {
		info["cache_creation_tokens_1h"] = cacheCreationTokens1h
		info["cache_creation_ratio_1h"] = cacheCreationRatio1h
	}
	return info
}

// GenerateMjOtherInfo 生成 Midjourney 请求的附加信息。
// Midjourney 使用固定价格计费（而非 Token 计费），因此信息较简单。
// 参数:
//   - relayInfo: Relay 请求信息
//   - priceData: 价格数据（包含模型价格和分组比率信息）
//
// 返回值:
//   - map[string]interface{}: 附加信息映射
func GenerateMjOtherInfo(relayInfo *relaycommon.RelayInfo, priceData types.PriceData) map[string]interface{} {
	other := make(map[string]interface{})
	other["model_price"] = priceData.ModelPrice
	other["group_ratio"] = priceData.GroupRatioInfo.GroupRatio
	// 如果有特殊分组比率，记录用户分组比率
	if priceData.GroupRatioInfo.HasSpecialRatio {
		other["user_group_ratio"] = priceData.GroupRatioInfo.GroupSpecialRatio
	}
	appendRequestPath(nil, relayInfo, other)
	return other
}

// InjectTieredBillingInfo 将分层计费信息注入到已有的附加信息映射中。
// 当请求使用 tiered_expr（分层表达式）计费模式时，在标准信息之上叠加分层计费字段。
// 必须在 GenerateTextOtherInfo / GenerateClaudeOtherInfo 等函数之后调用。
// 参数:
//   - other: 已有的附加信息映射（会被原地修改）
//   - relayInfo: Relay 请求信息（包含分层计费快照）
//   - result: 分层计费计算结果（包含匹配的层级信息）
func InjectTieredBillingInfo(other map[string]interface{}, relayInfo *relaycommon.RelayInfo, result *billingexpr.TieredResult) {
	if relayInfo == nil || other == nil {
		return
	}
	snap := relayInfo.TieredBillingSnapshot
	if snap == nil {
		return
	}
	other["billing_mode"] = "tiered_expr" // 标记为分层表达式计费模式
	// 将计费表达式 Base64 编码后存储（避免 JSON 转义问题）
	other["expr_b64"] = base64.StdEncoding.EncodeToString([]byte(snap.ExprString))
	if result != nil {
		other["matched_tier"] = result.MatchedTier
	}
}
