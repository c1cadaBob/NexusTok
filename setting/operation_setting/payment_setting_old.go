// 此文件为旧版支付设置文件，如需增加新的参数、变量等，请在 payment_setting.go 中添加。
// This file is the old version of the payment settings file. If you need to add new parameters, variables, etc., please add them in payment_setting.go

package operation_setting

import (
	"github.com/c1cada/NexusTok/common"
)

// PayAddress 支付网关地址（旧版配置，由新版 payment_setting 替代）
var PayAddress = ""

// CustomCallbackAddress 自定义支付回调地址
var CustomCallbackAddress = ""

// EpayId 易支付商户 ID
var EpayId = ""

// EpayKey 易支付商户密钥
var EpayKey = ""

// Price 默认单价（旧版）
var Price = 7.3

// MinTopUp 最低充值金额
var MinTopUp = 1

// USDExchangeRate 美元兑人民币汇率
var USDExchangeRate = 7.3

// PayMethods 支付方式列表，每项包含名称、颜色、类型等信息
var PayMethods = []map[string]string{
	{
		"name":  "支付宝",
		"color": "rgba(var(--semi-blue-5), 1)",
		"type":  "alipay",
	},
	{
		"name":  "微信",
		"color": "rgba(var(--semi-green-5), 1)",
		"type":  "wxpay",
	},
	{
		"name":      "自定义1",
		"color":     "black",
		"type":      "custom1",
		"min_topup": "50",
	},
}

// UpdatePayMethodsByJsonString 从 JSON 字符串更新支付方式列表
func UpdatePayMethodsByJsonString(jsonString string) error {
	PayMethods = make([]map[string]string, 0)
	return common.Unmarshal([]byte(jsonString), &PayMethods)
}

// PayMethods2JsonString 将支付方式列表序列化为 JSON 字符串
func PayMethods2JsonString() string {
	jsonBytes, err := common.Marshal(PayMethods)
	if err != nil {
		return "[]"
	}
	return string(jsonBytes)
}

// ContainsPayMethod 判断支付方式列表中是否包含指定类型的支付方式
func ContainsPayMethod(method string) bool {
	for _, payMethod := range PayMethods {
		if payMethod["type"] == method {
			return true
		}
	}
	return false
}
