// Package service - text_quota.go
// 该文件实现了文本请求的配额计算和结算逻辑
//
// 文本配额计算的核心逻辑：
// 1. 区分不同类型的 Token：普通文本、缓存、缓存创建、图像、音频
// 2. 每种 Token 类型有不同的倍率
// 3. 支持倍率模式和价格模式两种计费方式
// 4. 支持工具调用附加费（Web Search、File Search、Image Generation 等）
// 5. 支持阶梯计费（Tiered Billing）
//
// 配额计算公式（倍率模式）：
// 配额 = (基础Token + 缓存Token*缓存倍率 + 图像Token*图像倍率 + 缓存创建Token*缓存创建倍率) * 模型倍率 * 分组倍率 + 输出Token*补全倍率*模型倍率*分组倍率 + 工具调用附加费
//
// 配额计算公式（价格模式）：
// 配额 = 模型价格 * 配额单位 * 分组倍率 + 工具调用附加费
package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common"                       // 公共工具包
	"github.com/c1cada/NexusTok/constant"                     // 常量定义
	"github.com/c1cada/NexusTok/dto"                          // 数据传输对象
	"github.com/c1cada/NexusTok/logger"                       // 日志
	"github.com/c1cada/NexusTok/model"                        // 数据模型
	"github.com/c1cada/NexusTok/pkg/billingexpr"              // 计费表达式
	perfmetrics "github.com/c1cada/NexusTok/pkg/perf_metrics" // 性能指标
	relaycommon "github.com/c1cada/NexusTok/relay/common"     // 中继公共
	"github.com/c1cada/NexusTok/setting/operation_setting"    // 运营设置
	"github.com/c1cada/NexusTok/types"                        // 类型定义

	"github.com/bytedance/gopkg/util/gopool" // 协程池
	"github.com/gin-gonic/gin"               // Gin 框架
	"github.com/shopspring/decimal"          // 高精度十进制运算
)

// textQuotaSummary 文本配额汇总结构体
// 包含文本请求配额计算所需的所有信息
type textQuotaSummary struct {
	PromptTokens             int             // 输入 Token 总数
	CompletionTokens         int             // 输出 Token 总数
	TotalTokens              int             // Token 总数
	CacheTokens              int             // 缓存 Token 数
	CacheCreationTokens      int             // 缓存创建 Token 总数
	CacheCreationTokens5m    int             // 5分钟缓存创建 Token 数
	CacheCreationTokens1h    int             // 1小时缓存创建 Token 数
	ImageTokens              int             // 图像 Token 数
	AudioTokens              int             // 音频 Token 数
	ModelName                string          // 模型名称
	TokenName                string          // Token 名称
	UseTimeSeconds           int64           // 使用时长（秒）
	CompletionRatio          float64         // 补全倍率
	CacheRatio               float64         // 缓存倍率
	ImageRatio               float64         // 图像倍率
	ModelRatio               float64         // 模型倍率
	GroupRatio               float64         // 分组倍率
	ModelPrice               float64         // 模型价格（价格模式使用）
	CacheCreationRatio       float64         // 缓存创建倍率
	CacheCreationRatio5m     float64         // 5分钟缓存创建倍率
	CacheCreationRatio1h     float64         // 1小时缓存创建倍率
	Quota                    int             // 计算得到的配额值
	IsClaudeUsageSemantic    bool            // 是否使用 Claude 语义的使用量
	UsageSemantic            string          // 使用量语义类型（openai/anthropic）
	WebSearchPrice           float64         // Web Search 单价
	WebSearchCallCount       int             // Web Search 调用次数
	ClaudeWebSearchPrice     float64         // Claude Web Search 单价
	ClaudeWebSearchCallCount int             // Claude Web Search 调用次数
	FileSearchPrice          float64         // File Search 单价
	FileSearchCallCount      int             // File Search 调用次数
	AudioInputPrice          float64         // 音频输入单价
	ImageGenerationCallPrice float64         // 图像生成调用单价
	ToolCallSurchargeQuota   decimal.Decimal // 工具调用附加费配额
	StandardToolSurcharge    decimal.Decimal // 排除下游分组倍率后的工具调用附加费
	StandardBillingQuota     int             // 排除下游分组倍率后的标准计费基准
}

// cacheWriteTokensTotal 计算缓写 Token 总数
// 如果存在 5分钟/1小时 分离的缓存创建 Token，则返回它们的和
// 否则返回缓存创建 Token 总数
//
// 参数：
//   - summary: 文本配额汇总
//
// 返回值：
//   - int: 缓写 Token 总数
func cacheWriteTokensTotal(summary textQuotaSummary) int {
	if summary.CacheCreationTokens5m > 0 || summary.CacheCreationTokens1h > 0 {
		splitCacheWriteTokens := summary.CacheCreationTokens5m + summary.CacheCreationTokens1h
		if summary.CacheCreationTokens > splitCacheWriteTokens {
			return summary.CacheCreationTokens
		}
		return splitCacheWriteTokens
	}
	return summary.CacheCreationTokens
}

// isLegacyClaudeDerivedOpenAIUsage 判断是否为旧版 Claude 派生的 OpenAI 使用量
// 旧版格式中 Claude 的缓存创建 Token 可能以 OpenAI 格式返回
//
// 参数：
//   - relayInfo: 中继信息
//   - usage: 使用量信息
//
// 返回值：
//   - bool: 是否为旧版 Claude 派生格式
func isLegacyClaudeDerivedOpenAIUsage(relayInfo *relaycommon.RelayInfo, usage *dto.Usage) bool {
	if relayInfo == nil || usage == nil {
		return false
	}
	if relayInfo.GetFinalRequestRelayFormat() == types.RelayFormatClaude {
		return false
	}
	if usage.UsageSource != "" || usage.UsageSemantic != "" {
		return false
	}
	return usage.ClaudeCacheCreation5mTokens > 0 || usage.ClaudeCacheCreation1hTokens > 0
}

// calculateTextToolCallSurcharge 计算文本请求的工具调用附加费
// 支持的工具调用：
// 1. Web Search: 网页搜索（OpenAI 和 Claude 两种格式）
// 2. File Search: 文件搜索
// 3. Image Generation: 图像生成
//
// 参数：
//   - ctx: Gin 上下文
//   - relayInfo: 中继信息
//   - summary: 文本配额汇总（会被修改）
//
// 返回值：
//   - decimal.Decimal: 工具调用附加费配额
func calculateTextToolCallSurcharge(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, summary *textQuotaSummary) decimal.Decimal {
	return calculateTextToolCallSurchargeWithGroupRatio(ctx, relayInfo, summary, decimal.NewFromFloat(summary.GroupRatio), true)
}

// calculateTextToolCallSurchargeWithGroupRatio 按指定分组倍率计算文本工具调用附加费。
//
// 真实扣费路径使用当前下游有效 group_ratio；标准计费基准使用 1。两者共享同一套
// 工具价格和调用次数来源，避免上游成本字段漏掉 Web Search、File Search、图像生成等
// 请求自身产生的成本。recordDetails=true 时会把调用次数和单价写回 summary，用于日志展示。
func calculateTextToolCallSurchargeWithGroupRatio(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, summary *textQuotaSummary, dGroupRatio decimal.Decimal, recordDetails bool) decimal.Decimal {
	dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)

	var surcharge decimal.Decimal

	if relayInfo.ResponsesUsageInfo != nil {
		if webSearchTool, exists := relayInfo.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearchPreview]; exists && webSearchTool.CallCount > 0 {
			webSearchPrice := operation_setting.GetToolPriceForModel("web_search_preview", summary.ModelName)
			if recordDetails {
				summary.WebSearchCallCount = webSearchTool.CallCount
				summary.WebSearchPrice = webSearchPrice
			}
			surcharge = surcharge.Add(decimal.NewFromFloat(webSearchPrice).
				Mul(decimal.NewFromInt(int64(webSearchTool.CallCount))).
				Div(decimal.NewFromInt(1000)).
				Mul(dGroupRatio).
				Mul(dQuotaPerUnit))
		}
	} else if strings.HasSuffix(summary.ModelName, "search-preview") {
		webSearchPrice := operation_setting.GetToolPriceForModel("web_search_preview", summary.ModelName)
		if recordDetails {
			summary.WebSearchCallCount = 1
			summary.WebSearchPrice = webSearchPrice
		}
		surcharge = surcharge.Add(decimal.NewFromFloat(webSearchPrice).
			Div(decimal.NewFromInt(1000)).
			Mul(dGroupRatio).
			Mul(dQuotaPerUnit))
	}

	claudeWebSearchCallCount := ctx.GetInt("claude_web_search_requests")
	if recordDetails {
		summary.ClaudeWebSearchCallCount = claudeWebSearchCallCount
	}
	if claudeWebSearchCallCount > 0 {
		claudeWebSearchPrice := operation_setting.GetToolPrice("web_search")
		if recordDetails {
			summary.ClaudeWebSearchPrice = claudeWebSearchPrice
		}
		surcharge = surcharge.Add(decimal.NewFromFloat(claudeWebSearchPrice).
			Div(decimal.NewFromInt(1000)).
			Mul(dGroupRatio).
			Mul(dQuotaPerUnit).
			Mul(decimal.NewFromInt(int64(claudeWebSearchCallCount))))
	}

	if relayInfo.ResponsesUsageInfo != nil {
		if fileSearchTool, exists := relayInfo.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolFileSearch]; exists && fileSearchTool.CallCount > 0 {
			fileSearchPrice := operation_setting.GetToolPrice("file_search")
			if recordDetails {
				summary.FileSearchCallCount = fileSearchTool.CallCount
				summary.FileSearchPrice = fileSearchPrice
			}
			surcharge = surcharge.Add(decimal.NewFromFloat(fileSearchPrice).
				Mul(decimal.NewFromInt(int64(fileSearchTool.CallCount))).
				Div(decimal.NewFromInt(1000)).
				Mul(dGroupRatio).
				Mul(dQuotaPerUnit))
		}
	}

	if ctx.GetBool("image_generation_call") {
		imageGenerationCallPrice := operation_setting.GetGPTImage1PriceOnceCall(ctx.GetString("image_generation_call_quality"), ctx.GetString("image_generation_call_size"))
		if recordDetails {
			summary.ImageGenerationCallPrice = imageGenerationCallPrice
		}
		surcharge = surcharge.Add(decimal.NewFromFloat(imageGenerationCallPrice).
			Mul(dGroupRatio).
			Mul(dQuotaPerUnit))
	}

	return surcharge
}

// composeTieredTextQuota 组合阶梯计费的文本配额
// 将阶梯计费结果与工具调用附加费组合
//
// 参数：
//   - relayInfo: 中继信息
//   - summary: 文本配额汇总
//   - tieredQuota: 阶梯计费配额
//   - tieredResult: 阶梯计费结果
//
// 返回值：
//   - int: 组合后的配额值
func composeTieredTextQuota(relayInfo *relaycommon.RelayInfo, summary textQuotaSummary, tieredQuota int, tieredResult *billingexpr.TieredResult) int {
	if summary.ToolCallSurchargeQuota.IsZero() {
		return tieredQuota
	}

	if tieredResult != nil {
		if snap := relayInfo.TieredBillingSnapshot; snap != nil {
			quota, clamp := common.QuotaFromDecimalChecked(decimal.NewFromFloat(tieredResult.ActualQuotaBeforeGroup).
				Mul(decimal.NewFromFloat(snap.GroupRatio)).
				Add(summary.ToolCallSurchargeQuota))
			relayInfo.NoteQuotaClamp(clamp)
			return quota
		}
	}

	quota, clamp := common.QuotaFromDecimalChecked(decimal.NewFromInt(int64(tieredQuota)).Add(summary.ToolCallSurchargeQuota))
	relayInfo.NoteQuotaClamp(clamp)
	return quota
}

// composeTieredStandardBillingQuota 组合阶梯计费的标准计费基准。
//
// TieredResult.ActualQuotaBeforeGroup 已经是表达式部分排除 group_ratio 后的精确额度。
// 工具调用附加费不属于表达式引擎，因此需要以 group_ratio=1 单独加回，保证成本列
// 既不受下游倍率影响，也不会漏掉请求实际触发的工具成本。
func composeTieredStandardBillingQuota(summary textQuotaSummary, tieredQuota int, tieredResult *billingexpr.TieredResult) int {
	if tieredResult == nil {
		if quotaBeforeGroup, ok := StandardBillingQuotaFromFinalQuota(tieredQuota, summary.GroupRatio); ok {
			quota, _ := common.QuotaFromDecimalChecked(decimal.NewFromInt(int64(quotaBeforeGroup)).Add(summary.StandardToolSurcharge))
			return quota
		}
		return summary.StandardBillingQuota
	}
	quota, _ := common.QuotaFromDecimalChecked(decimal.NewFromFloat(tieredResult.ActualQuotaBeforeGroup).Add(summary.StandardToolSurcharge))
	return quota
}

// calculateTextStandardBillingQuota 计算文本请求排除下游分组倍率后的标准计费基准。
//
// 该函数与真实扣费公式保持同构：保留模型倍率/固定价、补全倍率、缓存/图片/音频
// 细分倍率、工具调用附加费和 OtherRatios，只把下游 group_ratio/user_group_ratio 视为 1。
func calculateTextStandardBillingQuota(relayInfo *relaycommon.RelayInfo, summary textQuotaSummary, promptQuota decimal.Decimal, completionQuota decimal.Decimal, audioInputQuota decimal.Decimal, standardToolSurcharge decimal.Decimal) int {
	dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
	var quotaCalculateDecimal decimal.Decimal
	if !relayInfo.PriceData.UsePrice {
		quotaCalculateDecimal = promptQuota.Add(completionQuota).Mul(decimal.NewFromFloat(summary.ModelRatio))
	} else {
		quotaCalculateDecimal = decimal.NewFromFloat(summary.ModelPrice).Mul(dQuotaPerUnit)
	}
	quotaCalculateDecimal = quotaCalculateDecimal.Add(standardToolSurcharge).Add(audioInputQuota)
	if len(relayInfo.PriceData.OtherRatios) > 0 {
		for _, otherRatio := range relayInfo.PriceData.OtherRatios {
			quotaCalculateDecimal = quotaCalculateDecimal.Mul(decimal.NewFromFloat(otherRatio))
		}
	}
	if !relayInfo.PriceData.UsePrice && summary.ModelRatio != 0 && quotaCalculateDecimal.LessThanOrEqual(decimal.Zero) {
		quotaCalculateDecimal = decimal.NewFromInt(1)
	}
	quota, _ := common.QuotaFromDecimalChecked(quotaCalculateDecimal)
	return quota
}

// calculateTextQuotaSummary 计算文本配额汇总
// 这是文本配额计算的核心函数，负责：
// 1. 从 relayInfo 和 usage 中提取各种 Token 数量
// 2. 处理 OpenRouter Claude 特殊计费逻辑
// 3. 计算各种倍率和价格
// 4. 计算工具调用附加费
// 5. 根据倍率模式或价格模式计算最终配额
//
// 参数：
//   - ctx: Gin 上下文
//   - relayInfo: 中继信息
//   - usage: 使用量信息
//
// 返回值：
//   - textQuotaSummary: 文本配额汇总
func calculateTextQuotaSummary(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.Usage) textQuotaSummary {
	summary := textQuotaSummary{
		ModelName:            relayInfo.OriginModelName,
		TokenName:            ctx.GetString("token_name"),
		UseTimeSeconds:       time.Now().Unix() - relayInfo.StartTime.Unix(),
		CompletionRatio:      relayInfo.PriceData.CompletionRatio,
		CacheRatio:           relayInfo.PriceData.CacheRatio,
		ImageRatio:           relayInfo.PriceData.ImageRatio,
		ModelRatio:           relayInfo.PriceData.ModelRatio,
		GroupRatio:           relayInfo.PriceData.GroupRatioInfo.GroupRatio,
		ModelPrice:           relayInfo.PriceData.ModelPrice,
		CacheCreationRatio:   relayInfo.PriceData.CacheCreationRatio,
		CacheCreationRatio5m: relayInfo.PriceData.CacheCreation5mRatio,
		CacheCreationRatio1h: relayInfo.PriceData.CacheCreation1hRatio,
		UsageSemantic:        usageSemanticFromUsage(relayInfo, usage),
	}
	summary.IsClaudeUsageSemantic = summary.UsageSemantic == "anthropic"

	if usage == nil {
		usage = &dto.Usage{
			PromptTokens:     relayInfo.GetEstimatePromptTokens(),
			CompletionTokens: 0,
			TotalTokens:      relayInfo.GetEstimatePromptTokens(),
		}
	}

	summary.PromptTokens = usage.PromptTokens
	summary.CompletionTokens = usage.CompletionTokens
	summary.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	summary.CacheTokens = usage.PromptTokensDetails.CachedTokens
	summary.CacheCreationTokens = usage.PromptTokensDetails.CachedCreationTokens
	summary.CacheCreationTokens5m = usage.ClaudeCacheCreation5mTokens
	summary.CacheCreationTokens1h = usage.ClaudeCacheCreation1hTokens
	summary.ImageTokens = usage.PromptTokensDetails.ImageTokens
	summary.AudioTokens = usage.PromptTokensDetails.AudioTokens
	legacyClaudeDerived := isLegacyClaudeDerivedOpenAIUsage(relayInfo, usage)
	isOpenRouterClaudeBilling := relayInfo.ChannelMeta != nil &&
		relayInfo.ChannelType == constant.ChannelTypeOpenRouter &&
		summary.IsClaudeUsageSemantic

	if isOpenRouterClaudeBilling {
		summary.PromptTokens -= summary.CacheTokens
		isUsingCustomSettings := relayInfo.PriceData.UsePrice || hasCustomModelRatio(summary.ModelName, relayInfo.PriceData.ModelRatio)
		if summary.CacheCreationTokens == 0 && relayInfo.PriceData.CacheCreationRatio != 1 && usage.Cost != 0 && !isUsingCustomSettings {
			maybeCacheCreationTokens := CalcOpenRouterCacheCreateTokens(*usage, relayInfo.PriceData)
			if maybeCacheCreationTokens >= 0 && summary.PromptTokens >= maybeCacheCreationTokens {
				summary.CacheCreationTokens = maybeCacheCreationTokens
			}
		}
		summary.PromptTokens -= summary.CacheCreationTokens
	}

	dPromptTokens := decimal.NewFromInt(int64(summary.PromptTokens))
	dCacheTokens := decimal.NewFromInt(int64(summary.CacheTokens))
	dImageTokens := decimal.NewFromInt(int64(summary.ImageTokens))
	dAudioTokens := decimal.NewFromInt(int64(summary.AudioTokens))
	dCompletionTokens := decimal.NewFromInt(int64(summary.CompletionTokens))
	dCachedCreationTokens := decimal.NewFromInt(int64(summary.CacheCreationTokens))
	dCompletionRatio := decimal.NewFromFloat(summary.CompletionRatio)
	dCacheRatio := decimal.NewFromFloat(summary.CacheRatio)
	dImageRatio := decimal.NewFromFloat(summary.ImageRatio)
	dModelRatio := decimal.NewFromFloat(summary.ModelRatio)
	dGroupRatio := decimal.NewFromFloat(summary.GroupRatio)
	dModelPrice := decimal.NewFromFloat(summary.ModelPrice)
	dCacheCreationRatio := decimal.NewFromFloat(summary.CacheCreationRatio)
	dCacheCreationRatio5m := decimal.NewFromFloat(summary.CacheCreationRatio5m)
	dCacheCreationRatio1h := decimal.NewFromFloat(summary.CacheCreationRatio1h)
	dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)

	ratio := dModelRatio.Mul(dGroupRatio)
	summary.ToolCallSurchargeQuota = calculateTextToolCallSurcharge(ctx, relayInfo, &summary)
	summary.StandardToolSurcharge = calculateTextToolCallSurchargeWithGroupRatio(ctx, relayInfo, &summary, decimal.NewFromInt(1), false)

	var audioInputQuota decimal.Decimal
	var standardAudioInputQuota decimal.Decimal
	if !relayInfo.PriceData.UsePrice {
		baseTokens := dPromptTokens

		var cachedTokensWithRatio decimal.Decimal
		if !dCacheTokens.IsZero() {
			if !summary.IsClaudeUsageSemantic && !legacyClaudeDerived {
				baseTokens = baseTokens.Sub(dCacheTokens)
			}
			cachedTokensWithRatio = dCacheTokens.Mul(dCacheRatio)
		}

		var cachedCreationTokensWithRatio decimal.Decimal
		hasSplitCacheCreationTokens := summary.CacheCreationTokens5m > 0 || summary.CacheCreationTokens1h > 0
		if !dCachedCreationTokens.IsZero() || hasSplitCacheCreationTokens {
			if !summary.IsClaudeUsageSemantic && !legacyClaudeDerived {
				baseTokens = baseTokens.Sub(dCachedCreationTokens)
				cachedCreationTokensWithRatio = dCachedCreationTokens.Mul(dCacheCreationRatio)
			} else {
				remaining := summary.CacheCreationTokens - summary.CacheCreationTokens5m - summary.CacheCreationTokens1h
				if remaining < 0 {
					remaining = 0
				}
				cachedCreationTokensWithRatio = decimal.NewFromInt(int64(remaining)).Mul(dCacheCreationRatio)
				cachedCreationTokensWithRatio = cachedCreationTokensWithRatio.Add(decimal.NewFromInt(int64(summary.CacheCreationTokens5m)).Mul(dCacheCreationRatio5m))
				cachedCreationTokensWithRatio = cachedCreationTokensWithRatio.Add(decimal.NewFromInt(int64(summary.CacheCreationTokens1h)).Mul(dCacheCreationRatio1h))
			}
		}

		var imageTokensWithRatio decimal.Decimal
		if !dImageTokens.IsZero() {
			baseTokens = baseTokens.Sub(dImageTokens)
			imageTokensWithRatio = dImageTokens.Mul(dImageRatio)
		}

		if !dAudioTokens.IsZero() {
			summary.AudioInputPrice = operation_setting.GetGeminiInputAudioPricePerMillionTokens(summary.ModelName)
			if summary.AudioInputPrice > 0 {
				baseTokens = baseTokens.Sub(dAudioTokens)
				audioInputQuota = decimal.NewFromFloat(summary.AudioInputPrice).
					Div(decimal.NewFromInt(1000000)).Mul(dAudioTokens).Mul(dGroupRatio).Mul(dQuotaPerUnit)
				standardAudioInputQuota = decimal.NewFromFloat(summary.AudioInputPrice).
					Div(decimal.NewFromInt(1000000)).Mul(dAudioTokens).Mul(dQuotaPerUnit)
			}
		}

		promptQuota := baseTokens.Add(cachedTokensWithRatio).Add(imageTokensWithRatio).Add(cachedCreationTokensWithRatio)
		completionQuota := dCompletionTokens.Mul(dCompletionRatio)
		summary.StandardBillingQuota = calculateTextStandardBillingQuota(relayInfo, summary, promptQuota, completionQuota, standardAudioInputQuota, summary.StandardToolSurcharge)
		quotaCalculateDecimal := promptQuota.Add(completionQuota).Mul(ratio)
		quotaCalculateDecimal = quotaCalculateDecimal.Add(summary.ToolCallSurchargeQuota)
		quotaCalculateDecimal = quotaCalculateDecimal.Add(audioInputQuota)

		if len(relayInfo.PriceData.OtherRatios) > 0 {
			for _, otherRatio := range relayInfo.PriceData.OtherRatios {
				quotaCalculateDecimal = quotaCalculateDecimal.Mul(decimal.NewFromFloat(otherRatio))
			}
		}

		if !ratio.IsZero() && quotaCalculateDecimal.LessThanOrEqual(decimal.Zero) {
			quotaCalculateDecimal = decimal.NewFromInt(1)
		}
		quota, clamp := common.QuotaFromDecimalChecked(quotaCalculateDecimal)
		relayInfo.NoteQuotaClamp(clamp)
		summary.Quota = quota
	} else {
		summary.StandardBillingQuota = calculateTextStandardBillingQuota(relayInfo, summary, decimal.Zero, decimal.Zero, audioInputQuota, summary.StandardToolSurcharge)
		quotaCalculateDecimal := dModelPrice.Mul(dQuotaPerUnit).Mul(dGroupRatio)
		quotaCalculateDecimal = quotaCalculateDecimal.Add(summary.ToolCallSurchargeQuota)
		quotaCalculateDecimal = quotaCalculateDecimal.Add(audioInputQuota)
		if len(relayInfo.PriceData.OtherRatios) > 0 {
			for _, otherRatio := range relayInfo.PriceData.OtherRatios {
				quotaCalculateDecimal = quotaCalculateDecimal.Mul(decimal.NewFromFloat(otherRatio))
			}
		}
		quota, clamp := common.QuotaFromDecimalChecked(quotaCalculateDecimal)
		relayInfo.NoteQuotaClamp(clamp)
		summary.Quota = quota
	}

	if summary.TotalTokens == 0 {
		summary.Quota = 0
		summary.StandardBillingQuota = 0
	} else {
		if !ratio.IsZero() && summary.Quota == 0 {
			summary.Quota = 1
		}
		if !relayInfo.PriceData.UsePrice && summary.ModelRatio != 0 && summary.StandardBillingQuota == 0 {
			summary.StandardBillingQuota = 1
		}
	}

	return summary
}

// usageSemanticFromUsage 从使用量信息中提取使用量语义类型
// 返回值：openai 或 anthropic
//
// 参数：
//   - relayInfo: 中继信息
//   - usage: 使用量信息
//
// 返回值：
//   - string: 使用量语义类型
func usageSemanticFromUsage(relayInfo *relaycommon.RelayInfo, usage *dto.Usage) string {
	if usage != nil && usage.UsageSemantic != "" {
		return usage.UsageSemantic
	}
	if relayInfo != nil && relayInfo.GetFinalRequestRelayFormat() == types.RelayFormatClaude {
		return "anthropic"
	}
	return "openai"
}

// PostTextConsumeQuota 文本请求结算配额
// 在文本请求完成后结算实际使用的配额
//
// 处理流程：
// 1. 计算文本配额汇总
// 2. 尝试阶梯计费结算
// 3. 计算工具调用附加费
// 4. 更新用户和渠道的使用量
// 5. 结算计费
// 6. 记录消费日志
// 7. 记录性能指标
//
// 参数：
//   - ctx: Gin 上下文
//   - relayInfo: 中继信息
//   - usage: 使用量信息
//   - extraContent: 额外日志内容
func PostTextConsumeQuota(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.Usage, extraContent []string) {
	originUsage := usage
	if usage == nil {
		extraContent = append(extraContent, "上游无计费信息")
	}
	if originUsage != nil {
		ObserveChannelAffinityUsageCacheByRelayFormat(ctx, usage, relayInfo.GetFinalRequestRelayFormat())
	}

	adminRejectReason := common.GetContextKeyString(ctx, constant.ContextKeyAdminRejectReason)
	summary := calculateTextQuotaSummary(ctx, relayInfo, usage)

	var tieredResult *billingexpr.TieredResult
	tieredBillingApplied := false
	if originUsage != nil {
		var tieredUsedVars map[string]bool
		if snap := relayInfo.TieredBillingSnapshot; snap != nil {
			tieredUsedVars = billingexpr.UsedVars(snap.ExprString)
		}
		tieredOk, tieredQuota, tieredRes := TryTieredSettle(relayInfo, BuildTieredTokenParams(usage, summary.IsClaudeUsageSemantic, tieredUsedVars))
		if tieredOk {
			tieredBillingApplied = true
			tieredResult = tieredRes
			summary.Quota = composeTieredTextQuota(relayInfo, summary, tieredQuota, tieredRes)
			summary.StandardBillingQuota = composeTieredStandardBillingQuota(summary, tieredQuota, tieredRes)
		}
	}

	if summary.WebSearchCallCount > 0 {
		extraContent = append(extraContent, fmt.Sprintf("Web Search 调用 %d 次，调用花费 %s", summary.WebSearchCallCount, decimal.NewFromFloat(summary.WebSearchPrice).Mul(decimal.NewFromInt(int64(summary.WebSearchCallCount))).Div(decimal.NewFromInt(1000)).Mul(decimal.NewFromFloat(summary.GroupRatio)).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).String()))
	}
	if summary.ClaudeWebSearchCallCount > 0 {
		extraContent = append(extraContent, fmt.Sprintf("Claude Web Search 调用 %d 次，调用花费 %s", summary.ClaudeWebSearchCallCount, decimal.NewFromFloat(summary.ClaudeWebSearchPrice).Div(decimal.NewFromInt(1000)).Mul(decimal.NewFromFloat(summary.GroupRatio)).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).Mul(decimal.NewFromInt(int64(summary.ClaudeWebSearchCallCount))).String()))
	}
	if summary.FileSearchCallCount > 0 {
		extraContent = append(extraContent, fmt.Sprintf("File Search 调用 %d 次，调用花费 %s", summary.FileSearchCallCount, decimal.NewFromFloat(summary.FileSearchPrice).Mul(decimal.NewFromInt(int64(summary.FileSearchCallCount))).Div(decimal.NewFromInt(1000)).Mul(decimal.NewFromFloat(summary.GroupRatio)).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).String()))
	}
	if summary.AudioInputPrice > 0 && summary.AudioTokens > 0 {
		extraContent = append(extraContent, fmt.Sprintf("Audio Input 花费 %s", decimal.NewFromFloat(summary.AudioInputPrice).Div(decimal.NewFromInt(1000000)).Mul(decimal.NewFromInt(int64(summary.AudioTokens))).Mul(decimal.NewFromFloat(summary.GroupRatio)).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).String()))
	}
	if summary.ImageGenerationCallPrice > 0 {
		extraContent = append(extraContent, fmt.Sprintf("Image Generation Call 花费 %s", decimal.NewFromFloat(summary.ImageGenerationCallPrice).Mul(decimal.NewFromFloat(summary.GroupRatio)).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).String()))
	}

	if summary.TotalTokens == 0 {
		extraContent = append(extraContent, "上游没有返回计费信息，无法扣费（可能是上游超时）")
		logger.LogError(ctx, fmt.Sprintf("total tokens is 0, cannot consume quota, userId %d, channelId %d, tokenId %d, model %s， pre-consumed quota %d", relayInfo.UserId, relayInfo.ChannelId, relayInfo.TokenId, summary.ModelName, relayInfo.FinalPreConsumedQuota))
	} else {
		model.UpdateUserUsedQuotaAndRequestCount(relayInfo.UserId, summary.Quota)
		model.UpdateChannelUsedQuota(relayInfo.ChannelId, summary.Quota)
		AddRelayAccountUsedQuota(relayInfo, int64(summary.Quota))
		RecordRelayPoolAccountUsageSuccess(ctx, relayInfo, summary.PromptTokens, summary.CompletionTokens, summary.Quota, int(summary.UseTimeSeconds))
	}

	if err := SettleBilling(ctx, relayInfo, summary.Quota); err != nil {
		logger.LogError(ctx, "error settling billing: "+err.Error())
	}

	logModel := summary.ModelName
	if strings.HasPrefix(logModel, "gpt-4-gizmo") {
		logModel = "gpt-4-gizmo-*"
		extraContent = append(extraContent, fmt.Sprintf("模型 %s", summary.ModelName))
	}
	if strings.HasPrefix(logModel, "gpt-4o-gizmo") {
		logModel = "gpt-4o-gizmo-*"
		extraContent = append(extraContent, fmt.Sprintf("模型 %s", summary.ModelName))
	}

	logContent := strings.Join(extraContent, ", ")
	var other map[string]interface{}
	if summary.IsClaudeUsageSemantic {
		other = GenerateClaudeOtherInfo(ctx, relayInfo,
			summary.ModelRatio, summary.GroupRatio, summary.CompletionRatio,
			summary.CacheTokens, summary.CacheRatio,
			summary.CacheCreationTokens, summary.CacheCreationRatio,
			summary.CacheCreationTokens5m, summary.CacheCreationRatio5m,
			summary.CacheCreationTokens1h, summary.CacheCreationRatio1h,
			summary.ModelPrice, relayInfo.PriceData.GroupRatioInfo.GroupSpecialRatio)
		other["usage_semantic"] = "anthropic"
	} else {
		other = GenerateTextOtherInfo(ctx, relayInfo, summary.ModelRatio, summary.GroupRatio, summary.CompletionRatio, summary.CacheTokens, summary.CacheRatio, summary.ModelPrice, relayInfo.PriceData.GroupRatioInfo.GroupSpecialRatio)
	}
	if adminRejectReason != "" {
		other["reject_reason"] = adminRejectReason
	}
	if summary.ImageTokens != 0 {
		other["image"] = true
		other["image_ratio"] = summary.ImageRatio
		other["image_output"] = summary.ImageTokens
	}
	if summary.WebSearchCallCount > 0 {
		other["web_search"] = true
		other["web_search_call_count"] = summary.WebSearchCallCount
		other["web_search_price"] = summary.WebSearchPrice
	} else if summary.ClaudeWebSearchCallCount > 0 {
		other["web_search"] = true
		other["web_search_call_count"] = summary.ClaudeWebSearchCallCount
		other["web_search_price"] = summary.ClaudeWebSearchPrice
	}
	if summary.FileSearchCallCount > 0 {
		other["file_search"] = true
		other["file_search_call_count"] = summary.FileSearchCallCount
		other["file_search_price"] = summary.FileSearchPrice
	}
	if summary.AudioInputPrice > 0 && summary.AudioTokens > 0 {
		other["audio_input_seperate_price"] = true
		other["audio_input_token_count"] = summary.AudioTokens
		other["audio_input_price"] = summary.AudioInputPrice
	}
	if summary.ImageGenerationCallPrice > 0 {
		other["image_generation_call"] = true
		other["image_generation_call_price"] = summary.ImageGenerationCallPrice
	}
	if summary.CacheCreationTokens > 0 {
		other["cache_creation_tokens"] = summary.CacheCreationTokens
		other["cache_creation_ratio"] = summary.CacheCreationRatio
	}
	if summary.CacheCreationTokens5m > 0 {
		other["cache_creation_tokens_5m"] = summary.CacheCreationTokens5m
		other["cache_creation_ratio_5m"] = summary.CacheCreationRatio5m
	}
	if summary.CacheCreationTokens1h > 0 {
		other["cache_creation_tokens_1h"] = summary.CacheCreationTokens1h
		other["cache_creation_ratio_1h"] = summary.CacheCreationRatio1h
	}
	cacheWriteTokens := cacheWriteTokensTotal(summary)
	if cacheWriteTokens > 0 {
		// cache_write_tokens: normalized cache creation total for UI display.
		// If split 5m/1h values are present, this is their sum; otherwise it falls back
		// to cache_creation_tokens.
		other["cache_write_tokens"] = cacheWriteTokens
	}
	if relayInfo.GetFinalRequestRelayFormat() != types.RelayFormatClaude && usage != nil && usage.UsageSource != "" && usage.InputTokens > 0 {
		// input_tokens_total: explicit normalized total input used by the usage log UI.
		// Only write this field when upstream/current conversion has already provided a
		// reliable total input value and tagged the usage source. Do not infer it from
		// prompt/cache fields here, otherwise old upstream payloads may be double-counted.
		other["input_tokens_total"] = usage.InputTokens
	}
	if tieredBillingApplied {
		InjectTieredBillingInfo(other, relayInfo, tieredResult)
	}
	AttachStandardBillingQuotaToOther(other, summary.StandardBillingQuota)
	AttachQuotaSaturation(ctx, relayInfo, other)

	model.RecordConsumeLog(ctx, relayInfo.UserId, model.RecordConsumeLogParams{
		ChannelId:        relayInfo.ChannelId,
		PromptTokens:     summary.PromptTokens,
		CompletionTokens: summary.CompletionTokens,
		ModelName:        logModel,
		TokenName:        summary.TokenName,
		Quota:            summary.Quota,
		Content:          logContent,
		TokenId:          relayInfo.TokenId,
		UseTimeSeconds:   int(summary.UseTimeSeconds),
		IsStream:         relayInfo.IsStream,
		Group:            relayInfo.UsingGroup,
		Other:            other,
	})
	gopool.Go(func() {
		perfmetrics.RecordRelaySample(relayInfo, true, int64(summary.CompletionTokens))
		RecordRoutingCandidateTTFT(relayInfo)
	})
}
