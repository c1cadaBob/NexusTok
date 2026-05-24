// responses - init.go
// Codex 的 OpenAI Responses 格式翻译器注册入口。
// 在包初始化时将 Codex 的请求转换函数和响应转换函数注册到全局翻译器注册表中，
// 使网关能够自动识别并路由 Codex 的 OpenAI Responses API 请求。
package responses

import (
	. "github.com/router-for-me/CLIProxyAPI/v7/internal/constant"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/interfaces"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/translator/translator"
)

// init 包初始化函数，注册 Codex 的 OpenAI Responses 格式翻译器。
// 注册内容包括：API 类型（OpenaiResponse）、提供商（Codex）、
// 请求转换函数（ConvertOpenAIResponsesRequestToCodex）以及
// 流式/非流式响应转换函数。
func init() {
	translator.Register(
		OpenaiResponse,
		Codex,
		ConvertOpenAIResponsesRequestToCodex,
		interfaces.TranslateResponse{
			Stream:    ConvertCodexResponseToOpenAIResponses,
			NonStream: ConvertCodexResponseToOpenAIResponsesNonStream,
		},
	)
}
