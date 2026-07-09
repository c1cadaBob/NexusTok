// Package controller - payment_webhook_availability.go
// 该文件实现了支付 Webhook 可用性检查的工具函数
//
// 提供各支付渠道（Stripe、Creem、Waffo、Epay）的配置状态检查
// 用于前端显示支付方式的可用状态
//
// 支付渠道：
// - Stripe：国际信用卡支付
// - Creem：加密货币支付
// - Waffo：东南亚支付
// - Waffo Pancake：Waffo Pancake 支付
// - Epay：易支付（支付宝、微信等）
package controller

import (
	"strings"

	"github.com/c1cada/NexusTok/setting"
	"github.com/c1cada/NexusTok/setting/operation_setting"
)

// isPaymentComplianceConfirmed 检查支付合规是否已确认
func isPaymentComplianceConfirmed() bool {
	return operation_setting.IsPaymentComplianceConfirmed()
}

// isStripeTopUpEnabled 检查 Stripe 充值是否启用
//
// 需要支付合规已确认，且 API 密钥、Webhook 密钥和价格 ID 都已配置
func isStripeTopUpEnabled() bool {
	if !isPaymentComplianceConfirmed() {
		return false
	}
	return strings.TrimSpace(setting.StripeApiSecret) != "" &&
		strings.TrimSpace(setting.StripeWebhookSecret) != "" &&
		strings.TrimSpace(setting.StripePriceId) != ""
}

// isStripeWebhookConfigured 检查 Stripe Webhook 是否已配置
func isStripeWebhookConfigured() bool {
	return strings.TrimSpace(setting.StripeWebhookSecret) != ""
}

// isStripeWebhookEnabled 检查 Stripe Webhook 是否启用
func isStripeWebhookEnabled() bool {
	return isStripeTopUpEnabled()
}

// isCreemTopUpEnabled 检查 Creem 充值是否启用
//
// 需要支付合规已确认，且 API 密钥和产品配置都已设置
func isCreemTopUpEnabled() bool {
	if !isPaymentComplianceConfirmed() {
		return false
	}
	products := strings.TrimSpace(setting.CreemProducts)
	return strings.TrimSpace(setting.CreemApiKey) != "" &&
		products != "" &&
		products != "[]"
}

// isCreemWebhookConfigured 检查 Creem Webhook 是否已配置
func isCreemWebhookConfigured() bool {
	return strings.TrimSpace(setting.CreemWebhookSecret) != ""
}

// isCreemWebhookEnabled 检查 Creem Webhook 是否启用
func isCreemWebhookEnabled() bool {
	return isCreemTopUpEnabled() && isCreemWebhookConfigured()
}

// isWaffoTopUpEnabled 检查 Waffo 充值是否启用
//
// 需要支付合规已确认、功能已启用且 Webhook 已配置
func isWaffoTopUpEnabled() bool {
	if !isPaymentComplianceConfirmed() {
		return false
	}
	if !setting.WaffoEnabled {
		return false
	}

	return isWaffoWebhookConfigured()
}

// isWaffoWebhookConfigured 检查 Waffo Webhook 是否已配置
//
// 根据沙盒模式检查对应的密钥配置
func isWaffoWebhookConfigured() bool {
	if setting.WaffoSandbox {
		return strings.TrimSpace(setting.WaffoSandboxApiKey) != "" &&
			strings.TrimSpace(setting.WaffoSandboxPrivateKey) != "" &&
			strings.TrimSpace(setting.WaffoSandboxPublicCert) != ""
	}

	return strings.TrimSpace(setting.WaffoApiKey) != "" &&
		strings.TrimSpace(setting.WaffoPrivateKey) != "" &&
		strings.TrimSpace(setting.WaffoPublicCert) != ""
}

// isWaffoWebhookEnabled 检查 Waffo Webhook 是否启用
func isWaffoWebhookEnabled() bool {
	return isWaffoTopUpEnabled()
}

// isWaffoPancakeTopUpEnabled 检查 Waffo Pancake 充值是否启用
//
// 需要支付合规已确认、功能已启用且钱包充值所需的 Store/Product 配置都已设置。
func isWaffoPancakeTopUpEnabled() bool {
	if !isPaymentComplianceConfirmed() {
		return false
	}
	if !setting.WaffoPancakeEnabled {
		return false
	}

	return isWaffoPancakeWebhookConfigured() &&
		strings.TrimSpace(setting.WaffoPancakeMerchantID) != "" &&
		strings.TrimSpace(setting.WaffoPancakePrivateKey) != "" &&
		strings.TrimSpace(setting.WaffoPancakeStoreID) != "" &&
		strings.TrimSpace(setting.WaffoPancakeProductID) != ""
}

// isWaffoPancakeRuntimeConfigured 检查 Waffo Pancake 运行时凭证是否可用于 checkout/webhook。
//
// 这里不要求钱包充值 ProductID。订阅套餐可以绑定自己的 Pancake product，只要全局
// Merchant/PrivateKey/Webhook key 可用，就应允许 webhook 接收订阅支付回调。
func isWaffoPancakeRuntimeConfigured() bool {
	if !isPaymentComplianceConfirmed() {
		return false
	}
	if !setting.WaffoPancakeEnabled {
		return false
	}
	return isWaffoPancakeWebhookConfigured() &&
		strings.TrimSpace(setting.WaffoPancakeMerchantID) != "" &&
		strings.TrimSpace(setting.WaffoPancakePrivateKey) != ""
}

// isWaffoPancakeWebhookConfigured 检查 Waffo Pancake Webhook 是否已配置
func isWaffoPancakeWebhookConfigured() bool {
	currentWebhookKey := strings.TrimSpace(setting.WaffoPancakeWebhookPublicKey)
	if setting.WaffoPancakeSandbox {
		currentWebhookKey = strings.TrimSpace(setting.WaffoPancakeWebhookTestKey)
	}

	return currentWebhookKey != ""
}

// isWaffoPancakeWebhookEnabled 检查 Waffo Pancake Webhook 是否启用
func isWaffoPancakeWebhookEnabled() bool {
	return isWaffoPancakeRuntimeConfigured()
}

// isEpayTopUpEnabled 检查易支付充值是否启用
//
// 需要支付合规已确认、Webhook 已配置且有可用的支付方式
func isEpayTopUpEnabled() bool {
	if !isPaymentComplianceConfirmed() {
		return false
	}
	return isEpayWebhookConfigured() && len(operation_setting.PayMethods) > 0
}

// isEpayWebhookConfigured 检查易支付 Webhook 是否已配置
func isEpayWebhookConfigured() bool {
	return strings.TrimSpace(operation_setting.PayAddress) != "" &&
		strings.TrimSpace(operation_setting.EpayId) != "" &&
		strings.TrimSpace(operation_setting.EpayKey) != ""
}

// isEpayWebhookEnabled 检查易支付 Webhook 是否启用
func isEpayWebhookEnabled() bool {
	return isEpayTopUpEnabled()
}
