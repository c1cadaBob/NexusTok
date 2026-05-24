// Package constant - waffo_pay_method.go
// 该文件定义了 Waffo 支付方式相关的常量和结构体
//
// Waffo 是一个支付网关服务，支持多种支付方式：
// - 信用卡/借记卡
// - Apple Pay
// - Google Pay
//
// 每种支付方式包含：
// - 前端显示名称和图标
// - Waffo API 的 PayMethodType 和 PayMethodName
package constant

// WaffoPayMethod Waffo 支付方式配置
//
// 定义支付方式的前端显示和 API 参数映射
type WaffoPayMethod struct {
	// Name 前端显示名称
	Name string `json:"name"`
	// Icon 前端图标标识或路径
	// 支持：credit-card, apple, google 或自定义图标路径
	Icon string `json:"icon"`
	// PayMethodType Waffo API 的支付方式类型
	// 可以是逗号分隔的多个值（如 "CREDITCARD,DEBITCARD"）
	PayMethodType string `json:"payMethodType"`
	// PayMethodName Waffo API 的支付方式名称
	// 为空时由 Waffo Checkout 自动选择
	PayMethodName string `json:"payMethodName"`
}

// DefaultWaffoPayMethods 默认支持的支付方式列表
//
// 包含三种常用支付方式：
// 1. 银行卡（信用卡/借记卡）
// 2. Apple Pay
// 3. Google Pay
var DefaultWaffoPayMethods = []WaffoPayMethod{
	{Name: "Card", Icon: "/pay-card.png", PayMethodType: "CREDITCARD,DEBITCARD", PayMethodName: ""},
	{Name: "Apple Pay", Icon: "/pay-apple.png", PayMethodType: "APPLEPAY", PayMethodName: "APPLEPAY"},
	{Name: "Google Pay", Icon: "/pay-google.png", PayMethodType: "GOOGLEPAY", PayMethodName: "GOOGLEPAY"},
}
