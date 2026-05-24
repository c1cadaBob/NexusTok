// Package billingexpr - round.go
// 该文件提供了配额四舍五入函数
//
// 核心功能：
// - QuotaRound：将 float64 配额值四舍五入为 int
// - 使用 half-away-from-zero 策略（标准四舍五入）
//
// 重要说明：
// - 所有分层计费路径（预消费、结算、日志）必须使用此函数
// - 避免 +-1 的舍入差异
package billingexpr

import "math"

// QuotaRound converts a float64 quota value to int using half-away-from-zero
// rounding. Every tiered billing path (pre-consume, settlement, breakdown
// validation, log fields) MUST use this function to avoid +-1 discrepancies.
func QuotaRound(f float64) int {
	return int(math.Round(f))
}
