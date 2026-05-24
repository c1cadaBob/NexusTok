// payment_waffo_pancake.go — Waffo Pancake 支付渠道配置
// 职责：存储 Waffo Pancake 支付平台的商户信息、密钥、Webhook 配置、
// 商店/商品 ID、回调 URL、货币类型、单价和最低充值金额等配置。

package setting

var (
	// WaffoPancakeEnabled 控制是否启用 Waffo Pancake 支付渠道
	WaffoPancakeEnabled bool
	// WaffoPancakeSandbox 控制是否启用沙箱/测试环境
	WaffoPancakeSandbox bool
	// WaffoPancakeMerchantID 是 Waffo Pancake 的商户 ID
	WaffoPancakeMerchantID string
	// WaffoPancakePrivateKey 是商户的私钥，用于签名请求
	WaffoPancakePrivateKey string
	// WaffoPancakeWebhookPublicKey 是验证 Webhook 回调的公钥（生产环境）
	WaffoPancakeWebhookPublicKey string
	// WaffoPancakeWebhookTestKey 是验证 Webhook 回调的密钥（测试环境）
	WaffoPancakeWebhookTestKey string
	// WaffoPancakeStoreID 是 Waffo Pancake 中的商店 ID
	WaffoPancakeStoreID string
	// WaffoPancakeProductID 是 Waffo Pancake 中的商品 ID
	WaffoPancakeProductID string
	// WaffoPancakeReturnURL 是支付完成后的回调地址
	WaffoPancakeReturnURL string
	// WaffoPancakeCurrency 是交易使用的货币代码，默认 "USD"
	WaffoPancakeCurrency string = "USD"
	// WaffoPancakeUnitPrice 是每单位额度对应的货币价格
	WaffoPancakeUnitPrice float64 = 1.0
	// WaffoPancakeMinTopUp 是通过 Waffo Pancake 充值的最低额度
	WaffoPancakeMinTopUp int = 1
)
