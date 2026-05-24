// payment_setting.go — 支付配置（新版）
// 职责：管理用户充值支付的相关配置，包括充值金额选项、
// 金额折扣规则以及合规条款确认状态。

package operation_setting

import "github.com/c1cada/NexusTok/setting/config"

// PaymentSetting 支付配置结构体
type PaymentSetting struct {
	// AmountOptions 充值金额选项列表（单位：分或货币最小单位）
	AmountOptions []int `json:"amount_options"`
	// AmountDiscount 充值金额对应的折扣映射，例如 100 元 0.9 表示 100 元充值享受 9 折优惠
	AmountDiscount map[int]float64 `json:"amount_discount"`

	// ComplianceConfirmed 是否已确认合规条款
	ComplianceConfirmed bool `json:"compliance_confirmed"`
	// ComplianceTermsVersion 已确认的合规条款版本号
	ComplianceTermsVersion string `json:"compliance_terms_version"`
	// ComplianceConfirmedAt 合规条款确认时间（Unix 时间戳）
	ComplianceConfirmedAt int64 `json:"compliance_confirmed_at"`
	// ComplianceConfirmedBy 确认合规条款的管理员用户 ID
	ComplianceConfirmedBy int `json:"compliance_confirmed_by"`
	// ComplianceConfirmedIP 确认合规条款时的客户端 IP 地址
	ComplianceConfirmedIP string `json:"compliance_confirmed_ip"`
}

// CurrentComplianceTermsVersion 当前合规条款版本号
const CurrentComplianceTermsVersion = "v1"

// 默认配置
var paymentSetting = PaymentSetting{
	AmountOptions:  []int{10, 20, 50, 100, 200, 500},
	AmountDiscount: map[int]float64{},
}

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("payment_setting", &paymentSetting)
}

// GetPaymentSetting 获取支付配置
func GetPaymentSetting() *PaymentSetting {
	return &paymentSetting
}

// IsPaymentComplianceConfirmed 判断合规条款是否已确认且版本匹配
func IsPaymentComplianceConfirmed() bool {
	return paymentSetting.ComplianceConfirmed &&
		paymentSetting.ComplianceTermsVersion == CurrentComplianceTermsVersion
}
