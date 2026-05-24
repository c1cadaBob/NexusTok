// translator - translator.go
// 提供不同 AI API 格式之间的请求和响应翻译功能。
// 作为 SDK 翻译器注册表的包装层，提供便捷函数用于在 OpenAI、Claude、Gemini 等 API 格式之间进行转换。
package translator

import (
	"context"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/interfaces"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
)

// registry 是默认的翻译器注册表实例
var registry = sdktranslator.Default()

// Register 注册一个新的 API 格式翻译器。
// from 为源 API 格式标识符，to 为目标 API 格式标识符。
// request 为请求翻译函数，response 为响应翻译函数。
func Register(from, to string, request interfaces.TranslateRequestFunc, response interfaces.TranslateResponse) {
	registry.Register(sdktranslator.FromString(from), sdktranslator.FromString(to), request, response)
}

// Request 将请求从一种 API 格式翻译为另一种。
// from 为源格式，to 为目标格式，modelName 为模型名称，rawJSON 为原始请求数据，stream 是否为流式请求。
func Request(from, to, modelName string, rawJSON []byte, stream bool) []byte {
	return registry.TranslateRequest(sdktranslator.FromString(from), sdktranslator.FromString(to), modelName, rawJSON, stream)
}

// NeedConvert 检查两种 API 格式之间是否需要响应翻译。
// 返回 true 表示需要翻译，false 表示不需要。
func NeedConvert(from, to string) bool {
	return registry.HasResponseTransformer(sdktranslator.FromString(from), sdktranslator.FromString(to))
}

// Response 将流式响应从一种 API 格式翻译为另一种。
// 处理 SSE 格式的流式响应数据，返回翻译后的响应行列表。
func Response(from, to string, ctx context.Context, modelName string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) [][]byte {
	return registry.TranslateStream(ctx, sdktranslator.FromString(from), sdktranslator.FromString(to), modelName, originalRequestRawJSON, requestRawJSON, rawJSON, param)
}

// ResponseNonStream 将非流式响应从一种 API 格式翻译为另一种。
// 处理完整的 JSON 响应，返回翻译后的响应数据。
func ResponseNonStream(from, to string, ctx context.Context, modelName string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) []byte {
	return registry.TranslateNonStream(ctx, sdktranslator.FromString(from), sdktranslator.FromString(to), modelName, originalRequestRawJSON, requestRawJSON, rawJSON, param)
}
