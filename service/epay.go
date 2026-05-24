// 本文件 (epay.go) 提供易支付 (ePay) 相关的工具函数。
// 负责获取支付回调地址，支持自定义回调地址覆盖默认服务器地址。
package service

import (
	"github.com/c1cada/NexusTok/setting/operation_setting" // 运营设置（自定义回调地址）
	"github.com/c1cada/NexusTok/setting/system_setting"     // 系统设置（服务器地址）
)

// GetCallbackAddress 获取支付回调地址。
// 如果配置了自定义回调地址（CustomCallbackAddress），则使用自定义地址；
// 否则使用系统配置的服务器地址（ServerAddress）。
// 返回值:
//   - string: 支付回调的基础地址
func GetCallbackAddress() string {
	if operation_setting.CustomCallbackAddress == "" {
		return system_setting.ServerAddress
	}
	return operation_setting.CustomCallbackAddress
}
