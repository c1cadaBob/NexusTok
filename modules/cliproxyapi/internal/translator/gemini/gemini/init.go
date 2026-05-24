// gemini/gemini - init.go
// 本文件负责注册 Gemini 到 Gemini 的直通翻译器。
// 在 init 函数中调用 translator.Register 注册请求标准化函数和直通响应函数，
// 用于在 Gemini API 本身格式之间进行请求规范化处理（如角色修正、参数重命名）。
package gemini

import (
	. "github.com/router-for-me/CLIProxyAPI/v7/internal/constant"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/interfaces"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/translator/translator"
)

// init 注册 Gemini 到 Gemini 的直通翻译器，用于请求标准化和响应透传。
// 请求转换器确保缺失或无效的角色被规范化为有效值。
func init() {
	translator.Register(
		Gemini,                              // 源 API 类型：Gemini
		Gemini,                              // 目标 API 类型：Gemini（同类型直通）
		ConvertGeminiRequestToGemini,        // 请求标准化函数
		interfaces.TranslateResponse{
			Stream:     PassthroughGeminiResponseStream,     // 流式响应直通函数
			NonStream:  PassthroughGeminiResponseNonStream,   // 非流式响应直通函数
			TokenCount: GeminiTokenCount,                     // Token 计数函数
		},
	)
}
