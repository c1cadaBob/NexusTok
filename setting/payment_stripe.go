// payment_stripe.go — Stripe 支付渠道配置
// 职责：存储 Stripe 支付平台的 API 密钥、Webhook 密钥、商品价格 ID、
// 单位价格、最低充值金额和促销码开关等配置信息。

package setting

// StripeApiSecret 是 Stripe 平台的 API 私钥
var StripeApiSecret = ""

// StripeWebhookSecret 是用于验证 Stripe Webhook 回调请求的密钥
var StripeWebhookSecret = ""

// StripePriceId 是 Stripe 中配置的商品价格 ID
var StripePriceId = ""

// StripeUnitPrice 是每单位额度对应的美元价格
var StripeUnitPrice = 8.0

// StripeMinTopUp 是通过 Stripe 充值的最低额度
var StripeMinTopUp = 1

// StripePromotionCodesEnabled 控制是否允许用户在 Stripe 结账时使用促销码
var StripePromotionCodesEnabled = false
