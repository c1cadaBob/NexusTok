// gemini/gemini - gemini_gemini_response.go
// 本文件提供 Gemini API 响应的直通处理功能。
// 包括流式和非流式响应的透传函数，以及 Token 计数函数。
// 这些函数在 Gemini 到 Gemini 的翻译路径中直接返回原始响应数据，不做格式转换。
package gemini

import (
	"bytes"
	"context"

	translatorcommon "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/common"
)

// PassthroughGeminiResponseStream 直通转发 Gemini 流式响应，不做任何格式转换。
// 去除 SSE data: 前缀和 [DONE] 标记后直接返回原始响应数据。
//
// 参数：
//   - ctx: 请求上下文
//   - rawJSON: 原始响应数据
//
// 返回：
//   - [][]byte: 直通后的响应数据切片
func PassthroughGeminiResponseStream(_ context.Context, _ string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, _ *any) [][]byte {
	if bytes.HasPrefix(rawJSON, []byte("data:")) {
		rawJSON = bytes.TrimSpace(rawJSON[5:])
	}

	if bytes.Equal(rawJSON, []byte("[DONE]")) {
		return [][]byte{}
	}

	return [][]byte{rawJSON}
}

// PassthroughGeminiResponseNonStream 直通转发 Gemini 非流式响应，不做任何格式转换。
//
// 参数：
//   - ctx: 请求上下文
//   - rawJSON: 原始响应数据
//
// 返回：
//   - []byte: 直通后的响应数据
func PassthroughGeminiResponseNonStream(_ context.Context, _ string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, _ *any) []byte {
	return rawJSON
}

// GeminiTokenCount 生成 Gemini 格式的 Token 计数 JSON 响应。
//
// 参数：
//   - ctx: 请求上下文
//   - count: Token 数量
//
// 返回：
//   - []byte: Gemini 格式的 Token 计数 JSON
func GeminiTokenCount(ctx context.Context, count int64) []byte {
	return translatorcommon.GeminiTokenCountJSON(count)
}
