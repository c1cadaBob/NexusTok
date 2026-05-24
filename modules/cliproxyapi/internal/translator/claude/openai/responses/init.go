// responses - init.go
// Claude 的 OpenAI Responses 格式翻译器注册入口。
// 在包初始化时将 Claude 的请求转换函数和响应转换函数注册到全局翻译器注册表中，
// 使网关能够自动识别并路由 Claude 的 OpenAI Responses API 请求。
package responses

import (
	. "github.com/router-for-me/CLIProxyAPI/v7/internal/constant"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/interfaces"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/translator/translator"
)

// init 包初始化函数，注册 Claude 的 OpenAI Responses 格式翻译器。
// 注册内容包括：API 类型（OpenaiResponse）、提供商（Claude）、
// 请求转换函数（ConvertOpenAIResponsesRequestToClaude）以及
// 流式/非流式响应转换函数。
func init() {
	translator.Register(
		OpenaiResponse,
		Claude,
		ConvertOpenAIResponsesRequestToClaude,
		interfaces.TranslateResponse{
			Stream:    ConvertClaudeResponseToOpenAIResponses,
			NonStream: ConvertClaudeResponseToOpenAIResponsesNonStream,
		},
	)
}
