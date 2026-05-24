// Package constant - finish_reason.go
// 该文件定义了 AI 模型响应的完成原因常量
//
// 完成原因（Finish Reason）表示模型停止生成内容的原因
// 不同的 AI 提供商可能使用不同的完成原因值
//
// 使用场景：
// - 响应格式化：将不同提供商的完成原因统一为标准格式
// - 流式传输：判断流式响应是否结束
// - 计费统计：根据完成原因判断是否需要计费
package constant

var (
	// FinishReasonStop 模型自然停止生成
	// 模型认为已经生成了完整的回答
	FinishReasonStop = "stop"

	// FinishReasonToolCalls 模型请求调用工具
	// 模型生成了工具调用请求，需要执行工具后继续对话
	FinishReasonToolCalls = "tool_calls"

	// FinishReasonLength 达到最大长度限制
	// 生成的 Token 数量达到了 max_tokens 限制
	FinishReasonLength = "length"

	// FinishReasonFunctionCall 模型请求调用函数（旧版格式）
	// 与 tool_calls 类似，但是旧版的函数调用格式
	FinishReasonFunctionCall = "function_call"

	// FinishReasonContentFilter 内容过滤
	// 生成的内容触发了安全过滤器
	FinishReasonContentFilter = "content_filter"
)
