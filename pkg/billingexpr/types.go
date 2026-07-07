// Package billingexpr - types.go
// 该文件定义了计费表达式系统的数据类型
//
// 主要类型：
// - RequestInput：请求输入（头部和请求体）
// - TokenParams：Token 维度参数（P、C 及可选的缓存 Token 维度）
// - BillingSnapshot：计费快照（表达式、版本、配额转换系数）
// - TieredResult：分层计费结果
//
// 用途：
// - 在表达式编译、运行、结算各阶段传递数据
package billingexpr

import (
	"crypto/sha256"
	"fmt"

	"github.com/c1cada/NexusTok/common"
)

// RequestInput 保存表达式运行时可读取的请求上下文。
// Headers 和 Body 供 header()/param() 等函数使用。
type RequestInput struct {
	Headers map[string]string
	Body    []byte
}

// TokenParams 保存一次表达式求值需要的全部 token 维度。
// P/C 之外的字段是可选维度，缺省时按 0 处理，因此旧的只关心 P/C 的表达式
// 不需要迁移即可继续工作。
type TokenParams struct {
	P    float64 // 输入文本 token；当子类别被单独计价时会自动排除对应 token
	C    float64 // 输出文本 token；当子类别被单独计价时会自动排除对应 token
	Len  float64 // 总输入上下文长度，用于阶梯条件；非 Claude 为原始 prompt_tokens，Claude 为文本+缓存读+缓存写
	CR   float64 // 缓存读取（命中）token
	CC   float64 // 缓存创建 token；Claude 中表示 5 分钟 TTL，其它模型表示通用缓存创建
	CC1h float64 // 1 小时 TTL 缓存创建 token，仅 Claude 使用
	Img  float64 // 输入图片 token
	ImgO float64 // 输出图片 token
	AI   float64 // 输入音频 token
	AO   float64 // 输出音频 token
}

// TraceResult 保存 tier() 函数在表达式执行期间捕获的旁路信息。
// 表达式本身是计费逻辑的唯一事实来源，TraceResult 只记录匹配层级和费用结果。
type TraceResult struct {
	MatchedTier string  `json:"matched_tier"`
	Cost        float64 `json:"cost"`
}

// BillingSnapshot 保存预消费时冻结的计费规则状态。
// 该结构必须可序列化，不能持有已编译程序指针，保证结算阶段可以复用预扣时的合同。
type BillingSnapshot struct {
	BillingMode               string  `json:"billing_mode"`
	ModelName                 string  `json:"model_name"`
	ExprString                string  `json:"expr_string"`
	ExprHash                  string  `json:"expr_hash"`
	GroupRatio                float64 `json:"group_ratio"`
	EstimatedPromptTokens     int     `json:"estimated_prompt_tokens"`
	EstimatedCompletionTokens int     `json:"estimated_completion_tokens"`
	EstimatedQuotaBeforeGroup float64 `json:"estimated_quota_before_group"`
	EstimatedQuotaAfterGroup  int     `json:"estimated_quota_after_group"`
	EstimatedTier             string  `json:"estimated_tier"`
	QuotaPerUnit              float64 `json:"quota_per_unit"`
	ExprVersion               int     `json:"expr_version"`
}

// TieredResult 保存阶梯计费结算后的结果。
type TieredResult struct {
	ActualQuotaBeforeGroup float64 `json:"actual_quota_before_group"`
	ActualQuotaAfterGroup  int     `json:"actual_quota_after_group"`
	MatchedTier            string  `json:"matched_tier"`
	CrossedTier            bool    `json:"crossed_tier"`
	// Clamp 记录配额换算时触发的 int32 饱和事件。
	// 该字段不参与 JSON 序列化，调用方会通过统一的 quota_saturation 审计路径写入日志。
	Clamp *common.QuotaClamp `json:"-"`
}

// ExprHashString 返回表达式字符串的 SHA-256 十六进制摘要。
func ExprHashString(expr string) string {
	h := sha256.Sum256([]byte(expr))
	return fmt.Sprintf("%x", h)
}
