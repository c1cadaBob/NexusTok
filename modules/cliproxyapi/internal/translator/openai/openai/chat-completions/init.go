// chat_completions - init.go
// OpenAI 的 OpenAI Chat Completions 格式翻译器注册入口。
// 在包初始化时将 OpenAI 的请求转换函数和响应转换函数注册到全局翻译器注册表中，
// 使网关能够自动识别并路由 OpenAI 的 Chat Completions API 请求。
package chat_completions

import (
	. "github.com/router-for-me/CLIProxyAPI/v7/internal/constant"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/interfaces"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/translator/translator"
)

// init 包初始化函数，注册 OpenAI 的 Chat Completions 格式翻译器。
// 注册内容包括：API 类型（OpenAI）、提供商（OpenAI）、
// 请求转换函数（ConvertOpenAIRequestToOpenAI）以及
// 流式/非流式响应转换函数。
func init() {
	translator.Register(
		OpenAI,
		OpenAI,
		ConvertOpenAIRequestToOpenAI,
		interfaces.TranslateResponse{
			Stream:    ConvertOpenAIResponseToOpenAI,
			NonStream: ConvertOpenAIResponseToOpenAINonStream,
		},
	)
}
