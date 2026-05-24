// tiered_billing.go — 分层/表达式计费模式配置
// 职责：管理模型的计费模式选择（按比率计费 或 按表达式计费）。
// 支持为每个模型独立配置计费方式和计费表达式，通过 config.GlobalConfig
// 注册实现持久化存储。提供读取器、副本获取、定价数据同步和表达式冒烟测试功能。

package billing_setting

import (
	"fmt"

	"github.com/c1cada/NexusTok/pkg/billingexpr"
	"github.com/c1cada/NexusTok/setting/config"
	"github.com/samber/lo"
)

// --- 计费模式常量 ---
const (
	// BillingModeRatio 表示按固定比率计费（默认模式）
	BillingModeRatio = "ratio"
	// BillingModeTieredExpr 表示按自定义表达式计费（支持分层、动态定价）
	BillingModeTieredExpr = "tiered_expr"
	// BillingModeField 是配置中计费模式字段的键名
	BillingModeField = "billing_mode"
	// BillingExprField 是配置中计费表达式字段的键名
	BillingExprField = "billing_expr"
)

// BillingSetting 计费配置结构体，通过 config.GlobalConfig.Register 注册管理。
// 数据库键名：billing_setting.billing_mode, billing_setting.billing_expr
type BillingSetting struct {
	// BillingMode 存储每个模型的计费模式，键为模型名，值为模式标识
	BillingMode map[string]string `json:"billing_mode"`
	// BillingExpr 存储每个模型的计费表达式，键为模型名，值为表达式字符串
	BillingExpr map[string]string `json:"billing_expr"`
}

// billingSetting 是全局计费配置实例
var billingSetting = BillingSetting{
	BillingMode: make(map[string]string),
	BillingExpr: make(map[string]string),
}

// init 注册计费配置到全局配置管理系统
func init() {
	config.GlobalConfig.Register("billing_setting", &billingSetting)
}

// ---------------------------------------------------------------------------
// 读取器（热路径，必须快速）
// ---------------------------------------------------------------------------

// GetBillingMode 获取指定模型的计费模式
// 参数：
//   - model: 模型名称
//
// 返回值：计费模式标识，未配置时默认返回 BillingModeRatio
func GetBillingMode(model string) string {
	if mode, ok := billingSetting.BillingMode[model]; ok {
		return mode
	}
	return BillingModeRatio
}

// GetBillingExpr 获取指定模型的计费表达式
// 参数：
//   - model: 模型名称
//
// 返回值：
//   - string: 计费表达式字符串
//   - bool: 该模型是否已配置自定义表达式
func GetBillingExpr(model string) (string, bool) {
	expr, ok := billingSetting.BillingExpr[model]
	return expr, ok
}

// GetBillingModeCopy 获取所有模型计费模式配置的浅拷贝
// 返回值：模型名到计费模式的映射副本
func GetBillingModeCopy() map[string]string {
	return lo.Assign(billingSetting.BillingMode)
}

// GetBillingExprCopy 获取所有模型计费表达式配置的浅拷贝
// 返回值：模型名到计费表达式的映射副本
func GetBillingExprCopy() map[string]string {
	return lo.Assign(billingSetting.BillingExpr)
}

// GetPricingSyncData 将计费配置数据合并到基础定价数据中，用于前端同步
// 参数：
//   - base: 基础定价数据
//
// 返回值：合并了计费模式和表达式后的完整定价数据
func GetPricingSyncData(base map[string]any) map[string]any {
	extra := make(map[string]any, 2)
	if modes := GetBillingModeCopy(); len(modes) > 0 {
		extra[BillingModeField] = modes
	}
	if exprs := GetBillingExprCopy(); len(exprs) > 0 {
		extra[BillingExprField] = exprs
	}
	return lo.Assign(base, extra)
}

// ---------------------------------------------------------------------------
// 冒烟测试（保存前由外部调用进行验证）
// ---------------------------------------------------------------------------

// SmokeTestExpr 对计费表达式进行冒烟测试，验证表达式的合法性和正确性
// 参数：
//   - exprStr: 待测试的计费表达式字符串
//
// 返回值：表达式不合法时返回错误
func SmokeTestExpr(exprStr string) error {
	return smokeTestExpr(exprStr)
}

// smokeTestExpr 使用预定义的测试向量执行表达式，验证其行为是否正确
// 测试策略：使用不同量级的 token 参数（0、1K、100K、1M）和
// 不同的请求上下文（空请求、带 header 的请求）组合测试
// 参数：
//   - exprStr: 待测试的计费表达式字符串
//
// 返回值：测试失败时返回描述错误
func smokeTestExpr(exprStr string) error {
	// 定义不同量级的 token 测试向量
	vectors := []billingexpr.TokenParams{
		{P: 0, C: 0, Len: 0},
		{P: 1000, C: 1000, Len: 1000},
		{P: 100000, C: 100000, Len: 100000},
		{P: 1000000, C: 1000000, Len: 1000000},
	}
	// 定义不同的请求上下文测试用例
	requests := []billingexpr.RequestInput{
		{},
		{
			Headers: map[string]string{
				"anthropic-beta": "fast-mode-2026-02-01",
			},
			Body: []byte(`{"service_tier":"fast","stream_options":{"include_usage":true},"messages":[1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21]}`),
		},
	}

	// 遍历所有测试向量和请求上下文的组合
	for _, v := range vectors {
		for _, request := range requests {
			result, _, err := billingexpr.RunExprWithRequest(exprStr, v, request)
			if err != nil {
				return fmt.Errorf("vector {p=%g, c=%g}: run failed: %w", v.P, v.C, err)
			}
			// 验证结果不为负数
			if result < 0 {
				return fmt.Errorf("vector {p=%g, c=%g}: result %f < 0", v.P, v.C, result)
			}
		}
	}
	return nil
}
