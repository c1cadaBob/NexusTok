// gemini/gemini-cli - init.go
// 本文件负责注册 GeminiCLI 到 Gemini 的翻译器。
// 在 init 函数中调用 translator.Register 将请求转换函数和响应转换函数注册到翻译器注册表中，
// 使得系统能够在 Gemini CLI API 格式和 Gemini API 格式之间进行双向转换。
package geminiCLI

import (
	. "github.com/router-for-me/CLIProxyAPI/v7/internal/constant"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/interfaces"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/translator/translator"
)

// init 注册 GeminiCLI 到 Gemini 的翻译器，包括请求转换和响应转换（流式/非流式）以及 Token 计数函数。
func init() {
	translator.Register(
		GeminiCLI,                                  // 源 API 类型：GeminiCLI
		Gemini,                                     // 目标 API 类型：Gemini
		ConvertGeminiCLIRequestToGemini,            // 请求转换函数
		interfaces.TranslateResponse{
			Stream:     ConvertGeminiResponseToGeminiCLI,         // 流式响应转换函数
			NonStream:  ConvertGeminiResponseToGeminiCLINonStream, // 非流式响应转换函数
			TokenCount: GeminiCLITokenCount,                        // Token 计数函数
		},
	)
}
