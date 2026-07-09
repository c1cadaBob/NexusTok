// openai_chat_responses_compat.go
// 本文件是 OpenAI Chat Completions 与 Responses API 之间的兼容层。
// 提供 Chat Completions 请求/响应格式与 Responses API 请求/响应格式之间的相互转换。
// 这些函数是对 openaicompat 包中实现的薄封装，便于 service 层直接调用。

package service

import (
	// 项目内部包
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/service/openaicompat"
)

// ChatCompletionsRequestToResponsesRequest 将 Chat Completions 请求转换为 Responses API 请求格式
// 用于在 Responses API 模式下处理来自 Chat Completions 兼容客户端的请求
// 参数:
//   - req: Chat Completions 格式的请求
//
// 返回值:
//   - *dto.OpenAIResponsesRequest: Responses API 格式的请求
//   - error: 转换失败时返回错误
func ChatCompletionsRequestToResponsesRequest(req *dto.GeneralOpenAIRequest) (*dto.OpenAIResponsesRequest, error) {
	return openaicompat.ChatCompletionsRequestToResponsesRequest(req)
}

// ResponsesRequestToChatCompletionsRequest 将 Responses API 请求转换为 Chat Completions 请求格式
// 用于让不支持 Responses 原生端点的上游渠道复用 Chat Completions 适配链路。
// 参数:
//   - req: Responses API 格式的请求
//
// 返回值:
//   - *dto.GeneralOpenAIRequest: Chat Completions 格式的请求
//   - error: 转换失败时返回错误
func ResponsesRequestToChatCompletionsRequest(req *dto.OpenAIResponsesRequest) (*dto.GeneralOpenAIRequest, error) {
	return openaicompat.ResponsesRequestToChatCompletionsRequest(req)
}

// ChatCompletionsResponseToResponsesResponse 将 Chat Completions 响应转换为 Responses API 响应格式
// 用于不支持 Responses 原生端点的上游渠道复用 Chat 适配链路后，将响应恢复为客户端请求的 Responses 形态。
// 参数:
//   - resp: Chat Completions 格式的响应
//   - id: Responses 响应 ID
//
// 返回值:
//   - *dto.OpenAIResponsesResponse: Responses API 格式的响应
//   - *dto.Usage: 使用量信息
//   - error: 转换失败时返回错误
func ChatCompletionsResponseToResponsesResponse(resp *dto.OpenAITextResponse, id string) (*dto.OpenAIResponsesResponse, *dto.Usage, error) {
	return openaicompat.ChatCompletionsResponseToResponsesResponse(resp, id)
}

// UsageFromChatUsage 将 Chat usage 字段补齐为 Responses usage 语义。
func UsageFromChatUsage(src *dto.Usage) *dto.Usage {
	return openaicompat.UsageFromChatUsage(src)
}

// ChatToResponsesStreamEvent 表示从 Chat 流式 chunk 生成的一条 Responses SSE 事件。
type ChatToResponsesStreamEvent = openaicompat.ChatToResponsesStreamEvent

// ChatToResponsesStreamState 保存 Chat 流式响应转换成 Responses 事件所需的跨 chunk 状态。
type ChatToResponsesStreamState = openaicompat.ChatToResponsesStreamState

// NewChatToResponsesStreamState 创建 Chat 流式响应到 Responses 事件的转换状态。
func NewChatToResponsesStreamState(id string, model string) *ChatToResponsesStreamState {
	return openaicompat.NewChatToResponsesStreamState(id, model)
}

// ChatCompletionsStreamChunkToResponsesEvents 将一个 Chat 流式 chunk 转换为 Responses SSE 事件。
func ChatCompletionsStreamChunkToResponsesEvents(chunk *dto.ChatCompletionsStreamResponse, state *ChatToResponsesStreamState) ([]ChatToResponsesStreamEvent, error) {
	return openaicompat.ChatCompletionsStreamChunkToResponsesEvents(chunk, state)
}

// FinalizeChatCompletionsStreamToResponses 结束转换并生成终态 Responses 事件。
func FinalizeChatCompletionsStreamToResponses(state *ChatToResponsesStreamState) []ChatToResponsesStreamEvent {
	return openaicompat.FinalizeChatCompletionsStreamToResponses(state)
}

// ResponsesToChatStreamState 保存 Responses SSE 转 Chat chunks 的跨事件状态。
type ResponsesToChatStreamState = openaicompat.ResponsesToChatStreamState

// NewResponsesToChatStreamState 创建 Responses SSE 到 Chat chunks 的转换状态。
func NewResponsesToChatStreamState(model string, includeUsage bool) *ResponsesToChatStreamState {
	return openaicompat.NewResponsesToChatStreamState(model, includeUsage)
}

// ResponsesStreamEventToChatChunks 将一条 Responses SSE 事件转换为 Chat chunks。
func ResponsesStreamEventToChatChunks(event *dto.ResponsesStreamResponse, state *ResponsesToChatStreamState) ([]dto.ChatCompletionsStreamResponse, error) {
	return openaicompat.ResponsesStreamEventToChatChunks(event, state)
}

// FinalizeResponsesToChatStream 结束 Responses SSE 到 Chat chunks 的转换。
func FinalizeResponsesToChatStream(state *ResponsesToChatStreamState) []dto.ChatCompletionsStreamResponse {
	return openaicompat.FinalizeResponsesToChatStream(state)
}

// ResponsesBufferedAccumulator 累积 Responses SSE 片段，用于还原非流式 JSON。
type ResponsesBufferedAccumulator = openaicompat.ResponsesBufferedAccumulator

// NewResponsesBufferedAccumulator 创建 Responses SSE buffered 累积器。
func NewResponsesBufferedAccumulator() *ResponsesBufferedAccumulator {
	return openaicompat.NewResponsesBufferedAccumulator()
}

// ResponsesResponseToChatCompletionsResponse 将 Responses API 响应转换为 Chat Completions 响应格式
// 用于在 Responses API 模式下将上游响应转换回 Chat Completions 格式返回给客户端
// 参数:
//   - resp: Responses API 格式的响应
//   - id: 请求 ID，用于填充 Chat Completions 响应的 id 字段
//
// 返回值:
//   - *dto.OpenAITextResponse: Chat Completions 格式的响应
//   - *dto.Usage: 使用量信息
//   - error: 转换失败时返回错误
func ResponsesResponseToChatCompletionsResponse(resp *dto.OpenAIResponsesResponse, id string) (*dto.OpenAITextResponse, *dto.Usage, error) {
	return openaicompat.ResponsesResponseToChatCompletionsResponse(resp, id)
}

// ExtractOutputTextFromResponses 从 Responses API 响应中提取输出文本
// 遍历响应的 output 数组，拼接所有 output_text 类型的内容
// 参数:
//   - resp: Responses API 格式的响应
//
// 返回值:
//   - string: 提取的输出文本
func ExtractOutputTextFromResponses(resp *dto.OpenAIResponsesResponse) string {
	return openaicompat.ExtractOutputTextFromResponses(resp)
}
