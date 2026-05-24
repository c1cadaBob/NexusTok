// responses - gemini-cli_openai-responses_response.go
// Gemini CLI 的 OpenAI Responses 响应转换器。
// 负责将 Gemini CLI 的响应转换为 OpenAI Responses 格式。
// 处理 Gemini CLI 特有的 "response" 对象封装结构，提取内部数据后
// 委托给 Gemini 标准的 Responses 响应转换器完成最终转换。
// 支持流式和非流式两种模式。
package responses

import (
	"context"

	. "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/gemini/openai/responses"
	"github.com/tidwall/gjson"
)

// ConvertGeminiCLIResponseToOpenAIResponses 将 Gemini CLI 的流式响应转换为 OpenAI Responses 格式。
// 该函数首先从 Gemini CLI 响应中提取 "response" 对象（CLI 特有的封装层），
// 然后将提取后的标准 Gemini 响应委托给 ConvertGeminiResponseToOpenAIResponses 进行转换。
//
// 参数：
//   - ctx: 请求上下文，用于取消和超时处理
//   - modelName: 模型名称
//   - originalRequestRawJSON: 原始请求的 JSON 数据
//   - requestRawJSON: 经过转换的请求 JSON 数据
//   - rawJSON: Gemini CLI 格式的原始响应 JSON 数据
//   - param: 用于在多次调用之间保持状态的参数指针
//
// 返回值：
//   - [][]byte: OpenAI Responses 格式的 SSE 事件数据切片
func ConvertGeminiCLIResponseToOpenAIResponses(ctx context.Context, modelName string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) [][]byte {
	responseResult := gjson.GetBytes(rawJSON, "response")
	if responseResult.Exists() {
		rawJSON = []byte(responseResult.Raw)
	}
	return ConvertGeminiResponseToOpenAIResponses(ctx, modelName, originalRequestRawJSON, requestRawJSON, rawJSON, param)
}

// ConvertGeminiCLIResponseToOpenAIResponsesNonStream 将 Gemini CLI 的非流式响应转换为 OpenAI Responses 格式。
// 该函数从 Gemini CLI 响应中提取 "response" 对象，同时处理请求中的 "request" 封装层，
// 确保请求和响应数据都去除 CLI 特有的封装后，委托给标准转换器完成转换。
//
// 参数：
//   - ctx: 请求上下文，用于取消和超时处理
//   - modelName: 模型名称
//   - originalRequestRawJSON: 原始请求的 JSON 数据（可能包含 "request" 封装）
//   - requestRawJSON: 经过转换的请求 JSON 数据（可能包含 "request" 封装）
//   - rawJSON: Gemini CLI 格式的原始响应 JSON 数据
//   - param: 用于转换过程中的参数指针
//
// 返回值：
//   - []byte: OpenAI Responses 格式的完整 JSON 响应数据
func ConvertGeminiCLIResponseToOpenAIResponsesNonStream(ctx context.Context, modelName string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) []byte {
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
