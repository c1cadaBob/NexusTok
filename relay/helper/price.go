// Package helper - price.go
// 本文件提供了模型定价和配额预消费计算的辅助函数，是计费系统的核心组成部分。
// 主要功能包括：
//   - ModelPriceHelper: 基于 Token 用量的标准定价计算（Chat Completions、Embedding 等）
//   - ModelPriceHelperPerCall: 按次计费的定价计算（Midjourney、异步任务等）
//   - modelPriceHelperTiered: 分层表达式计费的定价计算（支持动态定价表达式）
//   - HasModelBillingConfig: 检查模型是否已配置计费信息
//   - HandleGroupRatio: 处理用户分组的倍率调整
//   - modelPriceNotConfiguredError: 生成模型未配置价格的错误信息
//
// 定价系统支持两种计费模式：
//   - 倍率模式（Ratio）：基于模型倍率 * Token 数量计算配额
//   - 固定价格模式（Price）：基于每千 Token 固定价格计算配额
//   - 分层表达式模式（Tiered Expr）：基于动态定价表达式计算配额
package helper

import (
	"fmt"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/logger"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/pkg/billingexpr"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/setting/billing_setting"
	"github.com/c1cada/NexusTok/setting/operation_setting"
	"github.com/c1cada/NexusTok/setting/ratio_setting"
	"github.com/c1cada/NexusTok/types"

	"github.com/gin-gonic/gin"
)

// modelPriceNotConfiguredError 生成模型价格未配置的错误信息。
// 根据用户是否为管理员返回不同详细程度的错误提示：
//   - 管理员：提示前往系统设置配置价格或开启自用模式
//   - 普通用户：提示联系站点管理员
//
// 参数：
//   - modelName: 未配置价格的模型名称
//   - userId: 当前用户 ID，用于判断是否为管理员
//
// 返回值：
//   - error: 包含中英双语的错误描述
func modelPriceNotConfiguredError(modelName string, userId int) error {
	if model.IsAdmin(userId) {
		return fmt.Errorf(
			"模型 %s 的价格未配置。请前往「系统设置 → 运营设置」开启自用模式，或在「模型 → 同步源模型」中同步并配置该模型价格；"+
				"Model %s price not configured. Go to System Settings → Operation Settings to enable self-use mode, or sync and configure this model in Models → Sync Source Models.",
			modelName, modelName,
		)
	}
	return fmt.Errorf(
		"模型 %s 的价格尚未由管理员配置，暂时无法使用，请联系站点管理员开启该模型；"+
			"Model %s has not been priced by the administrator yet. Please contact the site administrator to enable this model.",
		modelName, modelName,
	)
}

// claudeCacheCreation1hMultiplier Claude 1 小时缓存写入价格的倍率系数。
// 根据 Claude 官方文档，1 小时缓存的写入价格是 5 分钟缓存的 6/3.75 倍。
// 参考：https://docs.claude.com/en/docs/build-with-claude/prompt-caching#1-hour-cache-duration
const claudeCacheCreation1hMultiplier = 6 / 3.75

// HandleGroupRatio 处理用户分组的倍率逻辑。
// 检查上下文中是否存在 auto_group（自动分组选择结果），如果存在则更新 RelayInfo.UsingGroup。
// 然后查找用户分组与使用分组之间的特殊倍率配置，如果不存在则使用分组的默认倍率。
//
// 参数：
//   - ctx: Gin 上下文
//   - relayInfo: 中继信息，包含用户分组和使用分组信息
//
// 返回值：
//   - types.GroupRatioInfo: 分组倍率信息，包含 GroupRatio（基础倍率）、GroupSpecialRatio（特殊倍率）、HasSpecialRatio（是否使用特殊倍率）
func HandleGroupRatio(ctx *gin.Context, relayInfo *relaycommon.RelayInfo) types.GroupRatioInfo {
	groupRatioInfo := types.GroupRatioInfo{
		GroupRatio:        1.0, // default ratio
		GroupSpecialRatio: -1,
	}

	// check auto group
	autoGroup, exists := ctx.Get("auto_group")
	if exists {
		logger.LogDebug(ctx, fmt.Sprintf("final group: %s", autoGroup))
		relayInfo.UsingGroup = autoGroup.(string)
	}

	// check user group special ratio
	userGroupRatio, ok := ratio_setting.GetGroupGroupRatio(relayInfo.UserGroup, relayInfo.UsingGroup)
	if ok {
		// user group special ratio
		groupRatioInfo.GroupSpecialRatio = userGroupRatio
		groupRatioInfo.GroupRatio = userGroupRatio
		groupRatioInfo.HasSpecialRatio = true
	} else {
		// normal group ratio
		groupRatioInfo.GroupRatio = ratio_setting.GetGroupRatio(relayInfo.UsingGroup)
	}

	return groupRatioInfo
}

// ModelPriceHelper 计算基于 Token 用量的标准模型定价。
// 支持两种计费模式：
//   - 倍率模式（!usePrice）：预扣额度 = max(promptTokens, PreConsumedQuota) * 模型倍率 * 分组倍率
//   - 固定价格模式（usePrice）：预扣额度 = 模型单价 * QuotaPerUnit * 分组倍率
//
// 计算过程中还会确定以下比率参数，用于后续结算：
//   - completionRatio: 输出 Token 与输入 Token 的价格比率
//   - cacheRatio: 缓存 Token 的价格比率
//   - cacheCreationRatio: 缓存写入的价格比率（支持 5 分钟和 1 小时两种）
//   - imageRatio: 图片的价格比率
//   - audioRatio / audioCompletionRatio: 音频输入/输出的价格比率
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继信息，包含模型名称、用户信息等
//   - promptTokens: 输入 Token 数量
//   - meta: Token 计数元数据，包含 MaxTokens、ImagePriceRatio 等
//
// 返回值：
//   - types.PriceData: 定价数据，包含所有比率和预扣额度
//   - error: 模型未配置价格等错误
func ModelPriceHelper(c *gin.Context, info *relaycommon.RelayInfo, promptTokens int, meta *types.TokenCountMeta) (types.PriceData, error) {
	modelPrice, usePrice := ratio_setting.GetModelPrice(info.OriginModelName, false)

	groupRatioInfo := HandleGroupRatio(c, info)

	// Check if this model uses tiered_expr billing
	if billing_setting.GetBillingMode(info.OriginModelName) == billing_setting.BillingModeTieredExpr {
		return modelPriceHelperTiered(c, info, promptTokens, meta, groupRatioInfo)
	}

	var preConsumedQuota int
	var modelRatio float64
	var completionRatio float64
	var cacheRatio float64
	var imageRatio float64
	var cacheCreationRatio float64
	var cacheCreationRatio5m float64
	var cacheCreationRatio1h float64
	var audioRatio float64
	var audioCompletionRatio float64
	var freeModel bool
	if !usePrice {
		preConsumedTokens := common.Max(promptTokens, common.PreConsumedQuota)
		if meta.MaxTokens != 0 {
			preConsumedTokens += meta.MaxTokens
		}
		var success bool
		var matchName string
		modelRatio, success, matchName = ratio_setting.GetModelRatio(info.OriginModelName)
		if !success {
			acceptUnsetRatio := false
			if info.UserSetting.AcceptUnsetRatioModel {
				acceptUnsetRatio = true
			}
			if !acceptUnsetRatio {
				return types.PriceData{}, modelPriceNotConfiguredError(matchName, info.UserId)
			}
		}
		completionRatio = ratio_setting.GetCompletionRatio(info.OriginModelName)
		cacheRatio, _ = ratio_setting.GetCacheRatio(info.OriginModelName)
		cacheCreationRatio, _ = ratio_setting.GetCreateCacheRatio(info.OriginModelName)
		cacheCreationRatio5m = cacheCreationRatio
		// 固定1h和5min缓存写入价格的比例
		cacheCreationRatio1h = cacheCreationRatio * claudeCacheCreation1hMultiplier
		imageRatio, _ = ratio_setting.GetImageRatio(info.OriginModelName)
		audioRatio = ratio_setting.GetAudioRatio(info.OriginModelName)
		audioCompletionRatio = ratio_setting.GetAudioCompletionRatio(info.OriginModelName)
		ratio := modelRatio * groupRatioInfo.GroupRatio
		var clamp *common.QuotaClamp
		preConsumedQuota, clamp = common.QuotaFromFloatChecked(float64(preConsumedTokens) * ratio)
		info.NoteQuotaClamp(clamp)
	} else {
		if meta.ImagePriceRatio != 0 {
			modelPrice = modelPrice * meta.ImagePriceRatio
		}
		var clamp *common.QuotaClamp
		preConsumedQuota, clamp = common.QuotaFromFloatChecked(modelPrice * common.QuotaPerUnit * groupRatioInfo.GroupRatio)
		info.NoteQuotaClamp(clamp)
	}

	// check if free model pre-consume is disabled
	if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume {
		// if model price or ratio is 0, do not pre-consume quota
		if groupRatioInfo.GroupRatio == 0 {
			preConsumedQuota = 0
			freeModel = true
		} else if usePrice {
			if modelPrice == 0 {
				preConsumedQuota = 0
				freeModel = true
			}
		} else {
			if modelRatio == 0 {
				preConsumedQuota = 0
				freeModel = true
			}
		}
	}

	priceData := types.PriceData{
		FreeModel:            freeModel,
		ModelPrice:           modelPrice,
		ModelRatio:           modelRatio,
		CompletionRatio:      completionRatio,
		GroupRatioInfo:       groupRatioInfo,
		UsePrice:             usePrice,
		CacheRatio:           cacheRatio,
		ImageRatio:           imageRatio,
		AudioRatio:           audioRatio,
		AudioCompletionRatio: audioCompletionRatio,
		CacheCreationRatio:   cacheCreationRatio,
		CacheCreation5mRatio: cacheCreationRatio5m,
		CacheCreation1hRatio: cacheCreationRatio1h,
		QuotaToPreConsume:    preConsumedQuota,
	}

	if common.DebugEnabled {
		println(fmt.Sprintf("model_price_helper result: %s", priceData.ToSetting()))
	}
	info.PriceData = priceData
	return priceData, nil
}

// ModelPriceHelperPerCall 按次/按量计费的定价计算函数。
// 主要用于 Midjourney 图片生成、异步任务（视频/音乐生成）等非 Token 计费场景。
// 计算逻辑：
//  1. 优先使用模型固定价格（GetModelPrice）
//  2. 其次使用默认价格表（GetDefaultModelPriceMap）
//  3. 最后回退到模型倍率，以倍率的一半作为预扣额度
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继信息，包含模型名称
//
// 返回值：
//   - types.PriceData: 定价数据，包含 Quota（预扣额度）和 ModelPrice
//   - error: 模型未配置价格等错误
func ModelPriceHelperPerCall(c *gin.Context, info *relaycommon.RelayInfo) (types.PriceData, error) {
	groupRatioInfo := HandleGroupRatio(c, info)

	modelPrice, success := ratio_setting.GetModelPrice(info.OriginModelName, true)
	usePrice := success
	var modelRatio float64

	if !success {
		defaultPrice, ok := ratio_setting.GetDefaultModelPriceMap()[info.OriginModelName]
		if ok {
			modelPrice = defaultPrice
			usePrice = true
		} else {
			var ratioSuccess bool
			var matchName string
			modelRatio, ratioSuccess, matchName = ratio_setting.GetModelRatio(info.OriginModelName)
			acceptUnsetRatio := false
			if info.UserSetting.AcceptUnsetRatioModel {
				acceptUnsetRatio = true
			}
			if !ratioSuccess && !acceptUnsetRatio {
				return types.PriceData{}, modelPriceNotConfiguredError(matchName, info.UserId)
			}
		}
	}

	var quota int
	freeModel := false

	if usePrice {
		var clamp *common.QuotaClamp
		quota, clamp = common.QuotaFromFloatChecked(modelPrice * common.QuotaPerUnit * groupRatioInfo.GroupRatio)
		info.NoteQuotaClamp(clamp)
		if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume {
			if groupRatioInfo.GroupRatio == 0 || modelPrice == 0 {
				quota = 0
				freeModel = true
			}
		}
	} else {
		// 按量计费：以模型倍率的一半作为预扣额度
		var clamp *common.QuotaClamp
		quota, clamp = common.QuotaFromFloatChecked(modelRatio / 2 * common.QuotaPerUnit * groupRatioInfo.GroupRatio)
		info.NoteQuotaClamp(clamp)
		modelPrice = -1
		if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume {
			if groupRatioInfo.GroupRatio == 0 || modelRatio == 0 {
				quota = 0
				freeModel = true
			}
		}
	}

	priceData := types.PriceData{
		FreeModel:      freeModel,
		ModelPrice:     modelPrice,
		ModelRatio:     modelRatio,
		UsePrice:       usePrice,
		Quota:          quota,
		GroupRatioInfo: groupRatioInfo,
	}
	return priceData, nil
}

// HasModelBillingConfig 检查指定模型是否已配置计费信息。
// 按以下优先级检查：
//  1. 是否有固定价格配置（GetModelPrice）
//  2. 是否有模型倍率配置（GetModelRatio）
//  3. 是否有分层表达式计费配置（tiered_expr 模式且表达式非空）
//
// 参数：
//   - modelName: 模型名称
//
// 返回值：
//   - bool: 是否已配置计费信息
func HasModelBillingConfig(modelName string) bool {
	if _, ok := ratio_setting.GetModelPrice(modelName, false); ok {
		return true
	}
	if _, ok, _ := ratio_setting.GetModelRatio(modelName); ok {
		return true
	}
	if billing_setting.GetBillingMode(modelName) != billing_setting.BillingModeTieredExpr {
		return false
	}
	expr, ok := billing_setting.GetBillingExpr(modelName)
	return ok && strings.TrimSpace(expr) != ""
}

// modelPriceHelperTiered 分层表达式计费的定价计算函数。
// 使用 billingexpr 包提供的表达式引擎，根据请求参数（如 stream 模式）动态选择
// 计费层（tier）并计算配额消耗。表达式支持条件判断、参数引用和层选择等高级功能。
//
// 计算流程：
//  1. 获取模型的计费表达式字符串
//  2. 解析请求输入（headers + body），用于表达式中的参数引用
//  3. 运行表达式，获取原始成本（$/1M tokens）
//  4. 将原始成本转换为配额：rawCost / 1M * QuotaPerUnit * GroupRatio
//  5. 生成 BillingSnapshot 用于结算时的快照参考
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继信息
//   - promptTokens: 输入 Token 数量
//   - meta: Token 计数元数据
//   - groupRatioInfo: 分组倍率信息
//
// 返回值：
//   - types.PriceData: 定价数据
//   - error: 表达式不存在或运行失败等错误
func modelPriceHelperTiered(c *gin.Context, info *relaycommon.RelayInfo, promptTokens int, meta *types.TokenCountMeta, groupRatioInfo types.GroupRatioInfo) (types.PriceData, error) {
	exprStr, ok := billing_setting.GetBillingExpr(info.OriginModelName)
	if !ok {
		return types.PriceData{}, fmt.Errorf("model %s is configured as tiered_expr but has no billing expression", info.OriginModelName)
	}

	estimatedCompletionTokens := 0
	if meta.MaxTokens != 0 {
		estimatedCompletionTokens = meta.MaxTokens
	}

	requestInput, err := ResolveIncomingBillingExprRequestInput(c, info)
	if err != nil {
		return types.PriceData{}, err
	}

	rawCost, trace, err := billingexpr.RunExprWithRequest(exprStr, billingexpr.TokenParams{
		P:   float64(promptTokens),
		C:   float64(estimatedCompletionTokens),
		Len: float64(promptTokens),
	}, requestInput)
	if err != nil {
		return types.PriceData{}, fmt.Errorf("model %s tiered expr run failed: %w", info.OriginModelName, err)
	}

	// Expression coefficients are $/1M tokens prices; convert to quota the same way per-call billing does.
	quotaBeforeGroup := rawCost / 1_000_000 * common.QuotaPerUnit
	preConsumedQuota, clamp := common.QuotaRoundChecked(quotaBeforeGroup * groupRatioInfo.GroupRatio)
	info.NoteQuotaClamp(clamp)

	freeModel := false
	if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume {
		if groupRatioInfo.GroupRatio == 0 {
			preConsumedQuota = 0
			freeModel = true
		}
	}

	exprHash := billingexpr.ExprHashString(exprStr)
	snapshot := &billingexpr.BillingSnapshot{
		BillingMode:               billing_setting.BillingModeTieredExpr,
		ModelName:                 info.OriginModelName,
		ExprString:                exprStr,
		ExprHash:                  exprHash,
		GroupRatio:                groupRatioInfo.GroupRatio,
		EstimatedPromptTokens:     promptTokens,
		EstimatedCompletionTokens: estimatedCompletionTokens,
		EstimatedQuotaBeforeGroup: quotaBeforeGroup,
		EstimatedQuotaAfterGroup:  preConsumedQuota,
		EstimatedTier:             trace.MatchedTier,
		QuotaPerUnit:              common.QuotaPerUnit,
		ExprVersion:               billingexpr.ExprVersion(exprStr),
	}
	info.TieredBillingSnapshot = snapshot
	info.BillingRequestInput = &requestInput

	priceData := types.PriceData{
		FreeModel:         freeModel,
		GroupRatioInfo:    groupRatioInfo,
		QuotaToPreConsume: preConsumedQuota,
	}

	if common.DebugEnabled {
		println(fmt.Sprintf("model_price_helper_tiered result: model=%s preConsume=%d quotaBeforeGroup=%.2f groupRatio=%.2f tier=%s", info.OriginModelName, preConsumedQuota, quotaBeforeGroup, groupRatioInfo.GroupRatio, trace.MatchedTier))
	}

	info.PriceData = priceData
	return priceData, nil
}
