// Package service 提供业务逻辑层服务
// 本文件实现阶梯计费结算功能，将用量数据转换为阶梯计费参数并计算配额
package service

import (
	"github.com/c1cada/NexusTok/dto"                // 数据传输对象
	"github.com/c1cada/NexusTok/pkg/billingexpr"    // 计费表达式引擎
	relaycommon "github.com/c1cada/NexusTok/relay/common" // 中继通用类型
)

// TieredResultWrapper 阶梯计费结果包装器
// 将 billingexpr.TieredResult 类型别名导出到服务层使用
type TieredResultWrapper = billingexpr.TieredResult

// BuildTieredTokenParams 从 dto.Usage 构建阶梯计费的 token 参数
// 标准化 P（输入 token）和 C（输出 token），使其表示"未被表达式单独定价的 token 数量"
// 子类别（缓存、图片、音频）仅在表达式引用了对应变量时才会被减去
//
// GPT 格式 API：prompt_tokens / completion_tokens 是包含所有子类别的总数
// Claude 格式 API：prompt_tokens / completion_tokens 是纯文本数量
// 当子类别被单独定价时，此函数会将 GPT 格式标准化为纯文本数量
//
// 参数：
//   - usage: API 返回的用量数据
//   - isClaudeUsageSemantic: 是否为 Claude 语义的用量格式
//   - usedVars: 表达式中使用的变量集合（如 "cr", "cc", "img" 等）
//
// 返回值：
//   - billingexpr.TokenParams: 标准化后的 token 参数
func BuildTieredTokenParams(usage *dto.Usage, isClaudeUsageSemantic bool, usedVars map[string]bool) billingexpr.TokenParams {
	// 提取各类 token 数量
	p := float64(usage.PromptTokens)       // 输入 token 总数
	c := float64(usage.CompletionTokens)    // 输出 token 总数
	cr := float64(usage.PromptTokensDetails.CachedTokens)      // 缓存读取 token 数
	cc5m := float64(usage.PromptTokensDetails.CachedCreationTokens) // 5 分钟缓存创建 token 数
	cc1h := float64(0)                                             // 1 小时缓存创建 token 数

	// Anthropic 语义的用量需要特殊处理缓存创建 token
	if usage.UsageSemantic == "anthropic" {
		cc1h = float64(usage.ClaudeCacheCreation1hTokens)
		cc5m = float64(usage.ClaudeCacheCreation5mTokens)
	}

	img := float64(usage.PromptTokensDetails.ImageTokens)    // 输入图片 token 数
	ai := float64(usage.PromptTokensDetails.AudioTokens)     // 输入音频 token 数
	imgO := float64(usage.CompletionTokenDetails.ImageTokens) // 输出图片 token 数
	ao := float64(usage.CompletionTokenDetails.AudioTokens)   // 输出音频 token 数

	// inputLen = 总输入上下文长度，用于阶梯条件评估
	// 非 Claude 格式：prompt_tokens 已包含所有子类别
	// Claude 格式：input_tokens 是纯文本，需要加上缓存读取和缓存创建
	inputLen := p
	if isClaudeUsageSemantic {
		inputLen = p + cr + cc5m + cc1h
	}

	// 非 Claude 格式：当表达式引用了子类别变量时，从总数中减去对应的子类别数量
	if !isClaudeUsageSemantic {
		if usedVars["cr"] {
			p -= cr // 减去缓存读取 token
		}
		if usedVars["cc"] {
			p -= cc5m // 减去 5 分钟缓存创建 token
		}
		if usedVars["cc1h"] {
			p -= cc1h // 减去 1 小时缓存创建 token
		}
		if usedVars["img"] {
			p -= img // 减去输入图片 token
		}
		if usedVars["ai"] {
			p -= ai // 减去输入音频 token
		}
		if usedVars["img_o"] {
			c -= imgO // 减去输出图片 token
		}
		if usedVars["ao"] {
			c -= ao // 减去输出音频 token
		}
	}

	// 确保 token 数量不为负数
	if p < 0 {
		p = 0
	}
	if c < 0 {
		c = 0
	}

	return billingexpr.TokenParams{
		P:    p,        // 标准化后的输入 token 数
		C:    c,        // 标准化后的输出 token 数
		Len:  inputLen, // 总输入上下文长度
		CR:   cr,       // 缓存读取 token 数
		CC:   cc5m,     // 5 分钟缓存创建 token 数
		CC1h: cc1h,    // 1 小时缓存创建 token 数
		Img:  img,      // 输入图片 token 数
		ImgO: imgO,    // 输出图片 token 数
		AI:   ai,       // 输入音频 token 数
		AO:   ao,       // 输出音频 token 数
	}
}

// TryTieredSettle 尝试进行阶梯计费结算
// 检查请求是否使用 tiered_expr 计费模式，如果是则使用冻结的 BillingSnapshot 计算实际配额
//
// 返回值：
//   - ok: 是否应用了阶梯计费（true 表示已处理，false 表示需要回退到现有逻辑）
//   - quota: 计算后的配额（仅在 ok=true 时有意义）
//   - result: 阶梯计费结果详情（可能为 nil，例如计算出错时）
func TryTieredSettle(relayInfo *relaycommon.RelayInfo, params billingexpr.TokenParams) (ok bool, quota int, result *billingexpr.TieredResult) {
	snap := relayInfo.TieredBillingSnapshot
	// 如果没有阶梯计费快照或计费模式不是 tiered_expr，直接返回
	if snap == nil || snap.BillingMode != "tiered_expr" {
		return false, 0, nil
	}

	// 获取请求输入参数（可能包含用户等级、标签等信息）
	requestInput := billingexpr.RequestInput{}
	if relayInfo.BillingRequestInput != nil {
		requestInput = *relayInfo.BillingRequestInput
	}

	// 使用阶梯计费引擎计算实际配额
	tr, err := billingexpr.ComputeTieredQuotaWithRequest(snap, params, requestInput)
	if err != nil {
		// 计算出错时，使用预消费配额或估算配额作为兜底
		quota = relayInfo.FinalPreConsumedQuota
		if quota <= 0 {
			quota = snap.EstimatedQuotaAfterGroup
		}
		return true, quota, nil
	}

	// 返回计算后的实际配额
	return true, tr.ActualQuotaAfterGroup, &tr
}
