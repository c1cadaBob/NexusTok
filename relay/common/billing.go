// Package common - billing.go
// 本文件定义了计费会话的抽象接口 BillingSettler。
// BillingSettler 封装了预扣费/结算/退款的完整生命周期，
// 由 service.BillingSession 实现，存储在 RelayInfo 上以避免循环引用。
package common

import "github.com/gin-gonic/gin"

// BillingSettler 抽象计费会话的生命周期操作。
// 由 service.BillingSession 实现，存储在 RelayInfo 上以避免循环引用。
// 计费会话管理请求的预扣费（PreConsume）、结算（Settle）和退款（Refund）三个阶段，
// 确保用户在请求过程中的配额使用安全可靠。
type BillingSettler interface {
	// Settle 根据实际消耗额度进行结算，计算 delta = actualQuota - preConsumedQuota，
	// 同时调整资金来源（钱包/订阅）和令牌额度。
	Settle(actualQuota int) error

	// Refund 退还所有预扣费额度（资金来源 + 令牌），幂等安全。
	// 通过 gopool 异步执行。如果已经结算或退款则不做任何操作。
	Refund(c *gin.Context)

	// NeedsRefund 返回会话是否存在需要退还的预扣状态（未结算且未退款）。
	NeedsRefund() bool

	// GetPreConsumedQuota 返回实际预扣的额度值（信任用户可能为 0）。
	GetPreConsumedQuota() int

	// Reserve 将预扣额度补到目标值；若目标值不高于当前预扣额度则不做任何事。
	// 用于流式响应过程中，当预估消耗逐渐明确时，逐步提升预扣额度。
	Reserve(targetQuota int) error
}
