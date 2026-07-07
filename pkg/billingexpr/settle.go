// Package billingexpr - settle.go
// 该文件实现了计费表达式的结算功能
//
// 核心功能：
// - quotaConversion：表达式输出到配额的转换
// - ComputeTieredQuota：运行分层计费表达式并返回结算结果
// - 支持不同表达式版本的转换公式
//
// 结算流程：
// 1. 从 BillingSnapshot 获取表达式和参数
// 2. 运行表达式计算原始费用
// 3. 根据版本转换为配额值
// 4. 返回分层计费结果
package billingexpr

import "github.com/c1cada/NexusTok/common"

// quotaConversion 根据表达式版本将原始表达式输出转换为配额。
// 这是未来表达式版本切换不同换算公式的统一分发点。
func quotaConversion(exprOutput float64, snap *BillingSnapshot) float64 {
	switch snap.ExprVersion {
	default: // v1：表达式系数表示 $/1M tokens 的真实价格
		return exprOutput / 1_000_000 * snap.QuotaPerUnit
	}
}

// ComputeTieredQuota 使用冻结的 BillingSnapshot 和实际 token 数运行表达式，
// 并返回结算结果。
func ComputeTieredQuota(snap *BillingSnapshot, params TokenParams) (TieredResult, error) {
	return ComputeTieredQuotaWithRequest(snap, params, RequestInput{})
}

func ComputeTieredQuotaWithRequest(snap *BillingSnapshot, params TokenParams, request RequestInput) (TieredResult, error) {
	cost, trace, err := RunExprByHashWithRequest(snap.ExprString, snap.ExprHash, params, request)
	if err != nil {
		return TieredResult{}, err
	}

	quotaBeforeGroup := quotaConversion(cost, snap)
	afterGroup, clamp := common.QuotaRoundChecked(quotaBeforeGroup * snap.GroupRatio)
	crossed := trace.MatchedTier != snap.EstimatedTier

	return TieredResult{
		ActualQuotaBeforeGroup: quotaBeforeGroup,
		ActualQuotaAfterGroup:  afterGroup,
		MatchedTier:            trace.MatchedTier,
		CrossedTier:            crossed,
		Clamp:                  clamp,
	}, nil
}
