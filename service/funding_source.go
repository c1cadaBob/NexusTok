// Package service - funding_source.go
// 该文件定义了计费系统的资金来源抽象层
//
// 功能：
// - 定义 FundingSource 接口，抽象预扣费的资金来源
// - 实现 WalletFunding（钱包资金来源）
// - 实现 SubscriptionFunding（订阅资金来源）
// - 提供带重试机制的退款函数
package service

import (
	"time"

	"github.com/c1cada/NexusTok/model"
)

// ---------------------------------------------------------------------------
// FundingSource — 资金来源接口（钱包 or 订阅）
// ---------------------------------------------------------------------------

// FundingSource 抽象了预扣费的资金来源。
type FundingSource interface {
	// Source 返回资金来源标识："wallet" 或 "subscription"
	Source() string
	// PreConsume 从该资金来源预扣 amount 额度
	PreConsume(amount int) error
	// Settle 根据差额调整资金来源（正数补扣，负数退还）
	Settle(delta int) error
	// Refund 退还所有预扣费
	Refund() error
}

// ---------------------------------------------------------------------------
// WalletFunding — 钱包资金来源实现
// ---------------------------------------------------------------------------

// WalletFunding 钱包资金来源实现
// 通过用户钱包余额进行预扣费和结算
type WalletFunding struct {
	userId   int
	consumed int // 实际预扣的用户额度
}

// Source 返回资金来源标识
func (w *WalletFunding) Source() string { return BillingSourceWallet }

// PreConsume 从钱包预扣指定额度
//
// 参数：
//   - amount: 预扣额度
//
// 返回值：
//   - error: 错误
func (w *WalletFunding) PreConsume(amount int) error {
	if amount <= 0 {
		return nil
	}
	if err := model.DecreaseUserQuota(w.userId, amount, false); err != nil {
		return err
	}
	w.consumed = amount
	return nil
}

// Settle 根据差额调整钱包余额
// 正数表示补扣，负数表示退还
//
// 参数：
//   - delta: 差额
//
// 返回值：
//   - error: 错误
func (w *WalletFunding) Settle(delta int) error {
	if delta == 0 {
		return nil
	}
	if delta > 0 {
		return model.DecreaseUserQuota(w.userId, delta, false)
	}
	return model.IncreaseUserQuota(w.userId, -delta, false)
}

// Refund 退还钱包中所有预扣费
// 注意：IncreaseUserQuota 是非幂等操作，不能重试
//
// 返回值：
//   - error: 错误
func (w *WalletFunding) Refund() error {
	if w.consumed <= 0 {
		return nil
	}
	// IncreaseUserQuota 是 quota += N 的非幂等操作，不能重试，否则会多退额度。
	// 订阅的 RefundSubscriptionPreConsume 有 requestId 幂等保护所以可以重试。
	return model.IncreaseUserQuota(w.userId, w.consumed, false)
}

// ---------------------------------------------------------------------------
// SubscriptionFunding — 订阅资金来源实现
// ---------------------------------------------------------------------------

// SubscriptionFunding 订阅资金来源实现
// 通过用户订阅额度进行预扣费和结算
type SubscriptionFunding struct {
	requestId      string
	userId         int
	modelName      string
	amount         int64 // 预扣的订阅额度（subConsume）
	subscriptionId int
	preConsumed    int64
	// 以下字段在 PreConsume 成功后填充，供 RelayInfo 同步使用
	AmountTotal     int64
	AmountUsedAfter int64
	PlanId          int
	PlanTitle       string
}

// Source 返回资金来源标识
func (s *SubscriptionFunding) Source() string { return BillingSourceSubscription }

// PreConsume 从订阅额度预扣指定金额
// amount 参数被忽略，使用内部 s.amount（已在构造时根据 preConsumedQuota 计算）
//
// 参数：
//   - _: 预扣金额（忽略，使用内部 amount）
//
// 返回值：
//   - error: 错误
func (s *SubscriptionFunding) PreConsume(_ int) error {
	// amount 参数被忽略，使用内部 s.amount（已在构造时根据 preConsumedQuota 计算）
	res, err := model.PreConsumeUserSubscription(s.requestId, s.userId, s.modelName, 0, s.amount)
	if err != nil {
		return err
	}
	s.subscriptionId = res.UserSubscriptionId
	s.preConsumed = res.PreConsumed
	s.AmountTotal = res.AmountTotal
	s.AmountUsedAfter = res.AmountUsedAfter
	// 获取订阅计划信息
	if planInfo, err := model.GetSubscriptionPlanInfoByUserSubscriptionId(res.UserSubscriptionId); err == nil && planInfo != nil {
		s.PlanId = planInfo.PlanId
		s.PlanTitle = planInfo.PlanTitle
	}
	return nil
}

// Settle 根据差额调整订阅额度
// 正数表示补扣，负数表示退还
//
// 参数：
//   - delta: 差额
//
// 返回值：
//   - error: 错误
func (s *SubscriptionFunding) Settle(delta int) error {
	if delta == 0 {
		return nil
	}
	return model.PostConsumeUserSubscriptionDelta(s.subscriptionId, int64(delta))
}

// Refund 退还订阅中所有预扣费
// 使用 requestId 幂等保护，支持重试
//
// 返回值：
//   - error: 错误
func (s *SubscriptionFunding) Refund() error {
	if s.preConsumed <= 0 {
		return nil
	}
	return refundWithRetry(func() error {
		return model.RefundSubscriptionPreConsume(s.requestId)
	})
}

// refundWithRetry 尝试多次执行退款操作以提高成功率，只能用于基于事务的退款函数！！！！！！
// try to refund with retries, only for refund functions based on transactions!!!
func refundWithRetry(fn func() error) error {
	if fn == nil {
		return nil
	}
	const maxAttempts = 3
	var lastErr error
	for i := 0; i < maxAttempts; i++ {
		if err := fn(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if i < maxAttempts-1 {
			time.Sleep(time.Duration(200*(i+1)) * time.Millisecond)
		}
	}
	return lastErr
}
