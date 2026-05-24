// chat_completions - openai_openai_response.go
// OpenAI 的 OpenAI Chat Completions 格式响应转换器。
// 作为简单的透传层，标准化 OpenAI 兼容的 SSE 行格式。
//
// 转换逻辑：
// - 流式模式：剥离 "data:" 前缀，丢弃 "[DONE]" 标记
// - 非流式模式：直接透传原始响应
package chat_completions

import (
	"bytes"
	"context"
)

// ConvertOpenAIResponseToOpenAI 标准化单个 OpenAI 兼容流式响应块。
// 如果 chunk 是 SSE "data:" 行，剥离前缀并返回剩余的 JSON 载荷。
// "[DONE]" 标记不产生输出。
//
// 参数：
//   - ctx: 请求上下文（当前实现中未使用）
//   - modelName: 模型名称（当前实现中未使用）
//   - rawJSON: 原始的 SSE 格式响应数据
//   - param: 用于在多次调用之间保持状态的参数指针（当前实现中未使用）
//
// 返回值：
//   - [][]byte: 标准化的 JSON 载荷切片
func ConvertOpenAIResponseToOpenAI(_ context.Context, _ string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) [][]byte {
	if bytes.HasPrefix(rawJSON, []byte("data:")) {
		rawJSON = bytes.TrimSpace(rawJSON[5:])
	}
	if bytes.Equal(rawJSON, []byte("[DONE]")) {
		return [][]byte{}
	}
	return [][]byte{rawJSON}
}

// ConvertOpenAIResponseToOpenAINonStream 透传非流式 OpenAI 响应。
// 直接返回原始响应数据，不做任何转换。
//
// 参数：
//   - ctx: 请求上下文（当前实现中未使用）
//   - modelName: 模型名称（当前实现中未使用）
//   - rawJSON: 原始的 JSON 响应数据
//   - param: 用于在多次调用之间保持状态的参数指针（当前实现中未使用）
//
// 返回值：
//   - []byte: 原始的 JSON 响应数据
func ConvertOpenAIResponseToOpenAINonStream(ctx context.Context, modelName string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) []byte {
	return rawJSON
}
