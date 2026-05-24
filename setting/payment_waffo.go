// payment_waffo.go — Waffo 支付渠道配置
// 职责：存储 Waffo 支付平台的 API 密钥、商户信息、证书、Webhook 配置、
// 回调 URL、货币类型、单价、最低充值金额等配置信息。
// 提供 Waffo 支付方式（WaffoPayMethods）的读取、设置和序列化功能。

package setting

import (
	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
)

var (
	// WaffoEnabled 控制是否启用 Waffo 支付渠道
	WaffoEnabled bool
	// WaffoApiKey 是 Waffo 平台的 API 密钥（生产环境）
	WaffoApiKey string
	// WaffoPrivateKey 是商户的私钥，用于签名请求（生产环境）
	WaffoPrivateKey string
	// WaffoPublicCert 是验证 Webhook 回调的公钥（生产环境）
	WaffoPublicCert string
	// WaffoSandboxPublicCert 是验证 Webhook 回调的公钥（沙箱/测试环境）
	WaffoSandboxPublicCert string
	// WaffoSandboxApiKey 是 Waffo 平台的 API 密钥（沙箱/测试环境）
	WaffoSandboxApiKey string
	// WaffoSandboxPrivateKey 是商户的私钥（沙箱/测试环境）
	WaffoSandboxPrivateKey string
	// WaffoSandbox 控制是否启用 Waffo 沙箱/测试环境
	WaffoSandbox bool
	// WaffoMerchantId 是 Waffo 平台的商户 ID
	WaffoMerchantId string
	// WaffoNotifyUrl 是支付结果异步通知的回调地址
	WaffoNotifyUrl string
	// WaffoReturnUrl 是支付完成后用户的前端跳转地址
	WaffoReturnUrl string
	// WaffoSubscriptionReturnUrl 是订阅支付完成后的前端跳转地址
	WaffoSubscriptionReturnUrl string
	// WaffoCurrency 是交易使用的货币代码
	WaffoCurrency string
	// WaffoUnitPrice 是每单位额度对应的货币价格
	WaffoUnitPrice float64 = 1.0
	// WaffoMinTopUp 是通过 Waffo 充值的最低额度
	WaffoMinTopUp int = 1
)

// GetWaffoPayMethods 从 options 读取 Waffo 支付方式配置
func GetWaffoPayMethods() []constant.WaffoPayMethod {
	common.OptionMapRWMutex.RLock()
	jsonStr := common.OptionMap["WaffoPayMethods"]
	common.OptionMapRWMutex.RUnlock()

	if jsonStr == "" {
		return copyDefaultWaffoPayMethods()
	}
	var methods []constant.WaffoPayMethod
	if err := common.UnmarshalJsonStr(jsonStr, &methods); err != nil {
		return copyDefaultWaffoPayMethods()
	}
	return methods
}

// SetWaffoPayMethods 序列化 Waffo 支付方式配置并更新 OptionMap
func SetWaffoPayMethods(methods []constant.WaffoPayMethod) error {
	jsonBytes, err := common.Marshal(methods)
	if err != nil {
		return err
	}
	common.OptionMapRWMutex.Lock()
	common.OptionMap["WaffoPayMethods"] = string(jsonBytes)
	common.OptionMapRWMutex.Unlock()
	return nil
}

func copyDefaultWaffoPayMethods() []constant.WaffoPayMethod {
	cp := make([]constant.WaffoPayMethod, len(constant.DefaultWaffoPayMethods))
	copy(cp, constant.DefaultWaffoPayMethods)
	return cp
}

// WaffoPayMethods2JsonString 将默认 WaffoPayMethods 序列化为 JSON 字符串（供 InitOptionMap 使用）
func WaffoPayMethods2JsonString() string {
	jsonBytes, err := common.Marshal(constant.DefaultWaffoPayMethods)
	if err != nil {
		return "[]"
	}
	return string(jsonBytes)
}
