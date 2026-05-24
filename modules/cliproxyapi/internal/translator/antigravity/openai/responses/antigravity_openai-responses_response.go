// responses - antigravity_openai-responses_response.go
// Antigravity 的 OpenAI Responses 格式响应转换器。
// 负责从 Antigravity 的 "response" 包装中提取 Gemini 格式的响应数据，
// 然后委托给 Gemini 的响应转换器处理。
//
// Antigravity 上游返回的响应包含 "response" 包装字段，需要先剥离该包装
// 才能得到标准的 Gemini 响应格式。
package responses

import (
	"context"

	. "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/gemini/openai/responses"
	"github.com/tidwall/gjson"
)

// ConvertAntigravityResponseToOpenAIResponses 将 Antigravity 的流式响应转换为 OpenAI Responses SSE 格式。
// 从 Antigravity 的 "response" 包装中提取 Gemini 响应，然后委托给 Gemini 转换器处理。
//
// 参数：
//   - ctx: 请求上下文
//   - modelName: 模型名称
//   - originalRequestRawJSON: 原始请求的 JSON 数据
//   - requestRawJSON: 经过转换的请求 JSON 数据
//   - rawJSON: Antigravity 格式的原始响应 JSON 数据
//   - param: 用于在多次调用之间保持状态的参数指针
//
// 返回值：
//   - [][]byte: OpenAI Responses 格式的 SSE 事件数据切片
func ConvertAntigravityResponseToOpenAIResponses(ctx context.Context, modelName string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) [][]byte {
	responseResult := gjson.GetBytes(rawJSON, "response")
	if responseResult.Exists() {
		rawJSON = []byte(responseResult.Raw)
	}
	return ConvertGeminiResponseToOpenAIResponses(ctx, modelName, originalRequestRawJSON, requestRawJSON, rawJSON, param)
}

// ConvertAntigravityResponseToOpenAIResponsesNonStream 将 Antigravity 的非流式响应转换为 OpenAI Responses JSON 格式。
// 从 Antigravity 的 "response" 包装中提取 Gemini 响应和请求数据，然后委托给 Gemini 转换器处理。
//
// 参数：
//   - ctx: 请求上下文
//   - modelName: 模型名称
//   - originalRequestRawJSON: 原始请求的 JSON 数据（包含 "request" 包装）
//   - requestRawJSON: 经过转换的请求 JSON 数据（包含 "request" 包装）
//   - rawJSON: Antigravity 格式的原始响应数据（包含 "response" 包装）
//   - param: 用于在多次调用之间保持状态的参数指针
//
// 返回值：
//   - []byte: OpenAI Responses 格式的完整 JSON 响应数据
func ConvertAntigravityResponseToOpenAIResponsesNonStream(ctx context.Context, modelName string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) []byte {
	responseResult := gjson.GetBytes(rawJSON, "response")
	if responseResult.Exists() {
		rawJSON = []byte(responseResult.Raw)
	}

	requestResult := gjson.GetBytes(originalRequestRawJSON, "request")
	if responseResult.Exists() {
		originalRequestRawJSON = []byte(requestResult.Raw)
	}

	requestResult = gjson.GetBytes(requestRawJSON, "request")
	if responseResult.Exists() {
		requestRawJSON = []byte(requestResult.Raw)
	}

	return ConvertGeminiResponseToOpenAIResponsesNonStream(ctx, modelName, originalRequestRawJSON, requestRawJSON, rawJSON, param)
}
