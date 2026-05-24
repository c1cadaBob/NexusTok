// responses - antigravity_openai-responses_request.go
// Antigravity 的 OpenAI Responses 格式请求转换器。
// 作为委托层，将 OpenAI Responses 格式的请求先转换为 Gemini 格式，
// 再转换为 Antigravity 格式。利用已有的 Gemini 转换器实现，避免重复代码。
package responses

import (
	. "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/antigravity/gemini"
	. "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/gemini/openai/responses"
)

// ConvertOpenAIResponsesRequestToAntigravity 将 OpenAI Responses API 格式的请求转换为 Antigravity 格式。
// 转换流程：OpenAI Responses -> Gemini -> Antigravity（两步委托）。
//
// 参数：
//   - modelName: 模型名称
//   - inputRawJSON: 原始的 OpenAI Responses 格式 JSON 请求数据
//   - stream: 是否为流式请求
//
// 返回值：
//   - []byte: 转换后的 Antigravity 格式 JSON 请求数据
func ConvertOpenAIResponsesRequestToAntigravity(modelName string, inputRawJSON []byte, stream bool) []byte {
	rawJSON := inputRawJSON
	rawJSON = ConvertOpenAIResponsesRequestToGemini(modelName, rawJSON, stream)
	return ConvertGeminiRequestToAntigravity(modelName, rawJSON, stream)
}
