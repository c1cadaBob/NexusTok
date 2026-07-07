// Package billingexpr - round.go
// 该文件提供了配额四舍五入函数
//
// 核心功能：
// - QuotaRound：将 float64 配额值四舍五入为 int
// - 使用 half-away-from-zero 策略（标准四舍五入）
// - 复用 common 层的饱和保护，避免异常表达式结果溢出后产生反向计费
//
// 重要说明：
// - 所有分层计费路径（预消费、结算、日志）必须使用此函数
// - 避免 +-1 的舍入差异
package billingexpr

import "github.com/c1cada/NexusTok/common"

// QuotaRound 将 float64 配额值按 half-away-from-zero 规则四舍五入为 int。
// 所有阶梯计费路径（预消费、结算、明细校验、日志字段）都必须使用此函数：
// 一方面避免不同路径出现 +-1 的舍入差异，另一方面通过 common.QuotaRound
// 保证表达式异常返回超大值、无穷大或 NaN 时不会发生 int 转换溢出。
func QuotaRound(f float64) int {
	return common.QuotaRound(f)
}
