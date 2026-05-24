// Package common - quota.go
// 该文件定义了配额相关的工具函数
//
// 配额是系统中的虚拟货币单位，用于计量 API 使用量
// 1 单位配额 = $0.002 / 1K tokens（由 QuotaPerUnit 定义）
package common

// GetTrustQuota 获取信任配额
//
// 信任配额是系统默认分配给新用户的初始配额
// 当前值为 10 单位配额（即 $0.02 或约 500 万 tokens）
//
// 返回值：
//   - int: 信任配额值
func GetTrustQuota() int {
	return int(10 * QuotaPerUnit)
}
