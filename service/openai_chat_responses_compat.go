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
// 返回值:
//   - *dto.OpenAIResponsesRequest: Responses API 格式的请求
//   - error: 转换失败时返回错误
func ChatCompletionsRequestToResponsesRequest(req *dto.GeneralOpenAIRequest) (*dto.OpenAIResponsesRequest, error) {
	return openaicompat.ChatCompletionsRequestToResponsesRequest(req)
}

// ResponsesResponseToChatCompletionsResponse 将 Responses API 响应转换为 Chat Completions 响应格式
// 用于在 Responses API 模式下将上游响应转换回 Chat Completions 格式返回给客户端
// 参数:
//   - resp: Responses API 格式的响应
//   - id: 请求 ID，用于填充 Chat Completions 响应的 id 字段
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
// 返回值:
//   - string: 提取的输出文本
func ExtractOutputTextFromResponses(resp *dto.OpenAIResponsesResponse) string {
	return openaicompat.ExtractOutputTextFromResponses(resp)
}
