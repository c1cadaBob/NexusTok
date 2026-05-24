// gemini-cli/gemini - init.go
// 向翻译器注册中心注册 Gemini CLI 到 Gemini API 的双向转换器。
// 将 Gemini CLI 格式的请求转换为原生 Gemini API 格式，并将 Gemini API 格式的响应
// （流式和非流式）转换回 Gemini CLI 格式。
package gemini

import (
	. "github.com/router-for-me/CLIProxyAPI/v7/internal/constant"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/interfaces"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/translator/translator"
)

// init 在包初始化时自动执行，将 Gemini CLI -> Gemini API 的请求/响应转换器
// 注册到全局翻译器注册表中，包含流式、非流式和 Token 计数三种响应转换器。
func init() {
	translator.Register(
		Gemini,
		GeminiCLI,
		ConvertGeminiRequestToGeminiCLI,
		interfaces.TranslateResponse{
			Stream:     ConvertGeminiCliResponseToGemini,
			NonStream:  ConvertGeminiCliResponseToGeminiNonStream,
			TokenCount: GeminiTokenCount,
		},
	)
}
