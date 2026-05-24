// payment_creem.go — Creem 支付渠道配置
// 职责：存储 Creem 支付平台的 API 密钥、商品列表、测试模式开关
// 和 Webhook 密钥等配置信息。

package setting

// CreemApiKey 是 Creem 支付平台的 API 密钥
var CreemApiKey = ""

// CreemProducts 是 Creem 平台的商品列表（JSON 格式字符串）
var CreemProducts = "[]"

// CreemTestMode 控制是否启用 Creem 的测试/沙箱模式
var CreemTestMode = false

// CreemWebhookSecret 是用于验证 Creem Webhook 回调请求的密钥
var CreemWebhookSecret = ""
