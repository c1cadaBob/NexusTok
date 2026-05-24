// chat_completions - init.go
// Gemini 的 OpenAI Chat Completions 格式翻译器注册入口。
// 在包初始化时将 Gemini 的请求转换函数和响应转换函数注册到全局翻译器注册表中，
// 使网关能够自动识别并路由 Gemini 的 Chat Completions API 请求。
package chat_completions

import (
	. "github.com/router-for-me/CLIProxyAPI/v7/internal/constant"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/interfaces"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/translator/translator"
)

// init 包初始化函数，注册 Gemini 的 OpenAI Chat Completions 格式翻译器。
// 注册内容包括：API 类型（OpenAI）、提供商（Gemini）、
// 请求转换函数（ConvertOpenAIRequestToGemini）以及
// 流式/非流式响应转换函数。
func init() {
	translator.Register(
		OpenAI,
		Gemini,
		ConvertOpenAIRequestToGemini,
		interfaces.TranslateResponse{
			Stream:    ConvertGeminiResponseToOpenAI,
			NonStream: ConvertGeminiResponseToOpenAINonStream,
		},
	)
}
