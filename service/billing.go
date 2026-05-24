// Package service - billing.go
// 该文件实现了计费相关的核心服务
// 包括预扣费、结算、退款等计费流程
//
// 计费流程：
// 1. 预扣费（PreConsumeBilling）- 在请求发送前预估并扣除配额
// 2. 结算（SettleBilling）- 请求完成后根据实际消耗结算
// 3. 退款（Refund）- 请求失败时退还预扣的配额
package service

import (
	"fmt"

	"github.com/c1cada/NexusTok/logger"       // 日志
	relaycommon "github.com/c1cada/NexusTok/relay/common" // 中继公共包
	"github.com/c1cada/NexusTok/types"         // 类型定义

	"github.com/gin-gonic/gin" // Gin 框架
)

// 计费来源常量
const (
	BillingSourceWallet       = "wallet"       // 钱包余额计费
	BillingSourceSubscription = "subscription" // 订阅计费
)

// PreConsumeBilling 预扣费函数
// 根据用户计费偏好创建 BillingSession 并执行预扣费
// 会话存储在 relayInfo.Billing 上，供后续 Settle / Refund 使用
//
// 计费流程：
// 1. 创建计费会话（BillingSession）
// 2. 执行预扣费（从用户钱包或订阅中预扣配额）
// 3. 将会话绑定到 relayInfo，供后续使用
//
// 参数：
//   - c: Gin 上下文
//   - preConsumedQuota: 预估消耗的配额
//   - relayInfo: 中继信息
//
// 返回值：
//   - *types.NexusTokError: 错误信息，成功返回 nil
func PreConsumeBilling(c *gin.Context, preConsumedQuota int, relayInfo *relaycommon.RelayInfo) *types.NexusTokError {
	// 创建计费会话
	session, apiErr := NewBillingSession(c, relayInfo, preConsumedQuota)
	if apiErr != nil {
		return apiErr
	}

	// 将计费会话绑定到中继信息
	relayInfo.Billing = session

	return nil
}

// SettleBilling 结算计费函数
// 执行计费结算，根据实际消耗调整预扣费
//
// 结算逻辑：
// 1. 如果有 BillingSession，通过 session 结算
//    - 实际消耗 > 预扣费：补扣差额
//    - 实际消耗 < 预扣费：返还差额
//    - 实际消耗 = 预扣费：无需调整
// 2. 如果没有 BillingSession，使用旧的 PostConsumeQuota 路径（兼容）
//
// 参数：
//   - ctx: Gin 上下文
//   - relayInfo: 中继信息
//   - actualQuota: 实际消耗的配额
//
// 返回值：
//   - error: 错误信息，成功返回 nil
func SettleBilling(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, actualQuota int) error {
	// 检查是否有计费会话
	if relayInfo.Billing != nil {
		preConsumed := relayInfo.Billing.GetPreConsumedQuota()
		delta := actualQuota - preConsumed

		// 记录结算日志
		if delta > 0 {
			// 需要补扣
			logger.LogInfo(ctx, fmt.Sprintf("预扣费后补扣费：%s（实际消耗：%s，预扣费：%s）",
				logger.FormatQuota(delta),
				logger.FormatQuota(actualQuota),
				logger.FormatQuota(preConsumed),
			))
		} else if delta < 0 {
			// 需要返还
			logger.LogInfo(ctx, fmt.Sprintf("预扣费后返还扣费：%s（实际消耗：%s，预扣费：%s）",
				logger.FormatQuota(-delta),
				logger.FormatQuota(actualQuota),
				logger.FormatQuota(preConsumed),
			))
		} else {
			// 无需调整
			logger.LogInfo(ctx, fmt.Sprintf("预扣费与实际消耗一致，无需调整：%s（按次计费）",
				logger.FormatQuota(actualQuota),
			))
		}

		// 执行结算
		if err := relayInfo.Billing.Settle(actualQuota); err != nil {
			return err
		}

		// 发送额度通知
		if actualQuota != 0 {
			if relayInfo.BillingSource == BillingSourceSubscription {
				// 订阅计费：检查订阅剩余额度
				checkAndSendSubscriptionQuotaNotify(relayInfo)
			} else {
				// 钱包计费：检查钱包余额
				checkAndSendQuotaNotify(relayInfo, actualQuota-preConsumed, preConsumed)
			}
		}

		return nil
	}

	// 回退：无 BillingSession 时使用旧路径
	quotaDelta := actualQuota - relayInfo.FinalPreConsumedQuota
	if quotaDelta != 0 {
		return PostConsumeQuota(relayInfo, quotaDelta, relayInfo.FinalPreConsumedQuota, true)
	}

	return nil
}
