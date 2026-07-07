// Package common - quota_math.go
// 该文件集中处理配额数值从 float64 / decimal 转为 int 的边界保护。
package common

import (
	"fmt"
	"math"

	"github.com/shopspring/decimal"
)

// 配额相关数据库字段和日志字段长期以 int32 范围内的数值为稳定边界。
// 当模型价格、分组倍率、任务数量、表达式返回值等乘积异常放大时，直接
// int(...) 转换可能在不同平台或 Go 版本下产生不可审计的截断结果。本文件
// 使用饱和策略将异常值固定到边界，并返回 clamp 信息供关键计费链路记录。
const (
	MaxQuota = math.MaxInt32
	MinQuota = math.MinInt32
)

// QuotaClamp.Kind 的取值。调用方可以把这些值写入日志，便于管理员区分
// 溢出、下溢和 NaN 这三类异常来源。
const (
	QuotaClampOverflow  = "overflow"
	QuotaClampUnderflow = "underflow"
	QuotaClampNaN       = "nan"
)

// QuotaClamp 描述一次配额转换饱和事件。
//
// 字段说明：
//   - Op: 触发转换的入口名称，帮助定位是截断、四舍五入还是 decimal 转换。
//   - Kind: 饱和类型，可能是 overflow、underflow 或 nan。
//   - Original: 饱和前的近似值；decimal 转换时这是 Float64 近似值。
//   - Clamped: 实际用于计费的边界值或 NaN 兜底值。
type QuotaClamp struct {
	Op       string  `json:"op"`
	Kind     string  `json:"kind"`
	Original float64 `json:"original"`
	Clamped  int     `json:"clamped"`
}

// AuditMap 将饱和事件渲染为日志友好的 map。
// 返回 nil 表示没有发生饱和，调用方可以直接跳过写入。
func (c *QuotaClamp) AuditMap() map[string]any {
	if c == nil {
		return nil
	}
	return map[string]any{
		"op":       c.Op,
		"kind":     c.Kind,
		"original": c.Original,
		"clamped":  c.Clamped,
	}
}

// saturateQuota 将已经完成舍入策略的 float64 转为 int。
//
// 不变量：
//   - 普通有限值按 Go 的 int 转换语义处理。
//   - NaN 无法表示真实费用，统一兜底为 0，并返回可审计的 clamp。
//   - 超过 int32 边界的值按饱和策略固定到 MaxQuota / MinQuota，避免 wraparound。
func saturateQuota(value float64, op string) (int, *QuotaClamp) {
	switch {
	case math.IsNaN(value):
		SysError(fmt.Sprintf("quota conversion (%s) received NaN, falling back to 0", op))
		return 0, &QuotaClamp{Op: op, Kind: QuotaClampNaN, Original: value, Clamped: 0}
	case value >= MaxQuota:
		SysError(fmt.Sprintf("quota conversion (%s) overflow: %g exceeds max quota, clamped to %d", op, value, MaxQuota))
		return MaxQuota, &QuotaClamp{Op: op, Kind: QuotaClampOverflow, Original: value, Clamped: MaxQuota}
	case value <= MinQuota:
		SysError(fmt.Sprintf("quota conversion (%s) underflow: %g below min quota, clamped to %d", op, value, MinQuota))
		return MinQuota, &QuotaClamp{Op: op, Kind: QuotaClampUnderflow, Original: value, Clamped: MinQuota}
	default:
		return int(value), nil
	}
}

// QuotaFromFloat 将 float64 配额截断为 int，并对异常大值执行饱和保护。
// 适用于保持历史“向零截断”语义的按量计费路径。
func QuotaFromFloat(value float64) int {
	quota, _ := QuotaFromFloatChecked(value)
	return quota
}

// QuotaFromFloatChecked 与 QuotaFromFloat 相同，但在发生饱和时返回 clamp 信息。
func QuotaFromFloatChecked(value float64) (int, *QuotaClamp) {
	return saturateQuota(value, "QuotaFromFloat")
}

// QuotaRound 将 float64 配额按 half-away-from-zero 规则四舍五入为 int。
// 所有表达式计费路径必须使用此入口，以避免预扣、结算、日志展示出现 +-1 差异。
func QuotaRound(value float64) int {
	quota, _ := QuotaRoundChecked(value)
	return quota
}

// QuotaRoundChecked 与 QuotaRound 相同，但在发生饱和时返回 clamp 信息。
func QuotaRoundChecked(value float64) (int, *QuotaClamp) {
	return saturateQuota(math.Round(value), "QuotaRound")
}

// QuotaFromDecimal 将 decimal 配额四舍五入为 int，并对异常大值执行饱和保护。
// 适用于文本结算和工具调用附加费等历史语义为“四舍五入”的路径。
func QuotaFromDecimal(value decimal.Decimal) int {
	quota, _ := QuotaFromDecimalChecked(value)
	return quota
}

// QuotaFromDecimalChecked 与 QuotaFromDecimal 相同，但在发生饱和时返回 clamp 信息。
func QuotaFromDecimalChecked(value decimal.Decimal) (int, *QuotaClamp) {
	rounded, _ := value.Round(0).Float64()
	return saturateQuota(rounded, "QuotaFromDecimal")
}

// QuotaFromDecimalTruncated 将 decimal 配额按向零截断语义转为 int，并执行饱和保护。
// 适用于充值入账等历史上使用 decimal.IntPart() 的路径，避免把 1.9 这类边界值
// 四舍五入为 2，从而改变用户实际到账额度。
func QuotaFromDecimalTruncated(value decimal.Decimal) int {
	quota, _ := QuotaFromDecimalTruncatedChecked(value)
	return quota
}

// QuotaFromDecimalTruncatedChecked 与 QuotaFromDecimalTruncated 相同，但在发生饱和时返回 clamp 信息。
func QuotaFromDecimalTruncatedChecked(value decimal.Decimal) (int, *QuotaClamp) {
	truncated, _ := value.Truncate(0).Float64()
	return saturateQuota(truncated, "QuotaFromDecimalTruncated")
}
