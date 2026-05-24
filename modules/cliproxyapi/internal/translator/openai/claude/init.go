// openai/claude - init.go
// 本文件负责注册 Claude 到 OpenAI 的翻译器。
// 在 init 函数中调用 translator.Register 将请求转换函数和响应转换函数注册到翻译器注册表中，
// 使得系统能够在 Claude API 格式和 OpenAI API 格式之间进行双向转换。
package claude

import (
	. "github.com/router-for-me/CLIProxyAPI/v7/internal/constant"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/interfaces"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/translator/translator"
)

// init 注册 Claude 到 OpenAI 的翻译器，包括请求转换和响应转换（流式/非流式）以及 Token 计数函数。
func init() {
	translator.Register(
		Claude,                              // 源 API 类型：Claude
		OpenAI,                              // 目标 API 类型：OpenAI
		ConvertClaudeRequestToOpenAI,        // 请求转换函数
		interfaces.TranslateResponse{
			Stream:     ConvertOpenAIResponseToClaude,         // 流式响应转换函数
			NonStream:  ConvertOpenAIResponseToClaudeNonStream, // 非流式响应转换函数
			TokenCount: ClaudeTokenCount,                       // Token 计数函数
		},
	)
}
