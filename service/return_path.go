// 本文件 (return_path.go) 提供支付回调 URL 的构建功能。
// 根据系统配置的服务器地址和主题感知路径，生成完整的支付回调地址。
package service

import (
	"strings" // 字符串操作（去除尾部斜杠）

	"github.com/c1cada/NexusTok/common"                    // 项目公共工具包（主题感知路径）
	"github.com/c1cada/NexusTok/setting/system_setting"     // 系统设置（服务器地址）
)

// PaymentReturnURL 构建支付回调的完整 URL。
// 将系统配置的服务器地址与指定的回调路径后缀拼接，
// 路径会经过主题感知处理以适配不同的前端主题。
// 参数:
//   - suffix: 回调路径后缀（如 "/pay/callback"）
// 返回值:
//   - string: 完整的支付回调 URL
func PaymentReturnURL(suffix string) string {
	// 去除服务器地址尾部的斜杠，避免拼接时出现双斜杠
	base := strings.TrimRight(system_setting.ServerAddress, "/")
	return base + common.ThemeAwarePath(suffix)
}
