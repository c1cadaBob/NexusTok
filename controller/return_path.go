// Package controller - return_path.go
// 该文件实现了支付回调路径构建的工具函数
//
// 用于构建支付完成后的回调 URL
package controller

import (
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/setting/system_setting"
)

// paymentReturnPath 构建支付回调路径
//
// 将服务器地址与主题感知路径拼接
//
// 参数：
//   - suffix: 路径后缀
//
// 返回值：
//   - string: 完整的回调 URL
func paymentReturnPath(suffix string) string {
	base := strings.TrimRight(system_setting.ServerAddress, "/")
	return base + common.ThemeAwarePath(suffix)
}
