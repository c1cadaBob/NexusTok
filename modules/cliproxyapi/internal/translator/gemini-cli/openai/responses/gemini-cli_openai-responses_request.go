// responses - gemini-cli_openai-responses_request.go
// Gemini CLI 的 OpenAI Responses 请求转换器。
// 将 OpenAI Responses API 格式的请求转换为 Gemini CLI 格式，
// 采用两阶段转换策略：先将 OpenAI Responses 格式转换为 Gemini 标准格式，
// 再将 Gemini 格式转换为 Gemini CLI 特有的封装格式。
package responses

import (
	. "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/gemini-cli/gemini"
	. "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/gemini/openai/responses"
)

// ConvertOpenAIResponsesRequestToGeminiCLI 将 OpenAI Responses 格式的请求转换为 Gemini CLI 格式。
// 该函数采用委托模式，先调用 Gemini 标准的 Responses 请求转换器完成核心转换，
// 再调用 Gemini CLI 的请求转换器添加 CLI 特有的封装结构。
//
// 参数：
//   - modelName: 模型名称，用于设置请求中的 model 字段
//   - inputRawJSON: 原始的 OpenAI Responses 格式 JSON 请求数据
//   - stream: 是否为流式请求
//
// 返回值：
//   - []byte: 转换后的 Gemini CLI 格式 JSON 请求数据
func ConvertOpenAIResponsesRequestToGeminiCLI(modelName string, inputRawJSON []byte, stream bool) []byte {
	rawJSON := inputRawJSON
	rawJSON = ConvertOpenAIResponsesRequestToGemini(modelName, rawJSON, stream)
	return ConvertGeminiRequestToGeminiCLI(modelName, rawJSON, stream)
}
