// responses - init.go
// Antigravity 的 OpenAI Responses 格式翻译器注册入口。
// 在包初始化时将 Antigravity 的请求转换函数和响应转换函数注册到全局翻译器注册表中，
// 使网关能够自动识别并路由 Antigravity 的 OpenAI Responses API 请求。
package responses

import (
	. "github.com/router-for-me/CLIProxyAPI/v7/internal/constant"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/interfaces"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/translator/translator"
)

// init 包初始化函数，注册 Antigravity 的 OpenAI Responses 格式翻译器。
// 注册内容包括：API 类型（OpenaiResponse）、提供商（Antigravity）、
// 请求转换函数（ConvertOpenAIResponsesRequestToAntigravity）以及
// 流式/非流式响应转换函数。
func init() {
	translator.Register(
		OpenaiResponse,
		Antigravity,
		ConvertOpenAIResponsesRequestToAntigravity,
		interfaces.TranslateResponse{
			Stream:    ConvertAntigravityResponseToOpenAIResponses,
			NonStream: ConvertAntigravityResponseToOpenAIResponsesNonStream,
		},
	)
}
