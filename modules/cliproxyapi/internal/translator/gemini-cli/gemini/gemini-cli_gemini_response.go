// gemini-cli/gemini - gemini-cli_gemini_response.go
// 提供将 Gemini CLI API 响应转换为原生 Gemini API 格式的功能。
// 处理流式和非流式两种响应模式，从 Gemini CLI 的嵌套响应结构中提取
// 实际的 Gemini API 响应数据。
package gemini

import (
	"bytes"
	"context"

	translatorcommon "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/common"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ConvertGeminiCliResponseToGemini 将 Gemini CLI 流式响应转换为原生 Gemini API 格式。
// 从 Gemini CLI 响应包装结构中提取实际的 response 数据。
// 支持两种响应格式：
// 1. 当 context 中的 alt 为空时，从 "response" 字段提取单个响应对象
// 2. 当 alt 非空时，处理数组格式响应，提取每个元素的 "response" 字段
//
// 参数:
//   - ctx: 请求上下文，包含 alt 参数用于选择响应格式
//   - _: 模型名称（当前未使用）
//   - originalRequestRawJSON: 原始请求的 JSON 数据
//   - requestRawJSON: 请求的 JSON 数据
//   - rawJSON: Gemini CLI API 的原始 JSON 响应
//   - _: 转换参数（当前未使用）
//
// 返回:
//   - [][]byte: 转换后的 Gemini API 格式响应数据
func ConvertGeminiCliResponseToGemini(ctx context.Context, _ string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, _ *any) [][]byte {
	if bytes.HasPrefix(rawJSON, []byte("data:")) {
		rawJSON = bytes.TrimSpace(rawJSON[5:])
	}

	if alt, ok := ctx.Value("alt").(string); ok {
		var chunk []byte
		if alt == "" {
			responseResult := gjson.GetBytes(rawJSON, "response")
			if responseResult.Exists() {
				chunk = []byte(responseResult.Raw)
			}
		} else {
			chunkTemplate := []byte(`[]`)
			responseResult := gjson.ParseBytes(chunk)
			if responseResult.IsArray() {
				responseResultItems := responseResult.Array()
				for i := 0; i < len(responseResultItems); i++ {
					responseResultItem := responseResultItems[i]
					if responseResultItem.Get("response").Exists() {
						chunkTemplate, _ = sjson.SetRawBytes(chunkTemplate, "-1", []byte(responseResultItem.Get("response").Raw))
					}
				}
			}
			chunk = chunkTemplate
		}
		return [][]byte{chunk}
	}
	return [][]byte{}
}

// ConvertGeminiCliResponseToGeminiNonStream 将非流式的 Gemini CLI 响应转换为原生 Gemini API 格式。
// 从完整的 Gemini CLI 响应中提取 "response" 字段，如果存在则返回其内容，
// 否则返回原始响应数据。
//
// 参数:
//   - _: 请求上下文（当前未使用）
//   - _: 模型名称（当前未使用）
//   - originalRequestRawJSON: 原始请求的 JSON 数据
//   - requestRawJSON: 请求的 JSON 数据
//   - rawJSON: Gemini CLI API 的原始 JSON 响应
//   - _: 转换参数（当前未使用）
//
// 返回:
//   - []byte: 转换后的 Gemini API 格式 JSON 响应
func ConvertGeminiCliResponseToGeminiNonStream(_ context.Context, _ string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, _ *any) []byte {
	responseResult := gjson.GetBytes(rawJSON, "response")
	if responseResult.Exists() {
		return []byte(responseResult.Raw)
	}
	return rawJSON
}

// GeminiTokenCount 生成 Gemini 格式的 token 计数 JSON 响应。
// 用于在流式传输开始前报告输入 token 数量。
func GeminiTokenCount(ctx context.Context, count int64) []byte {
	return translatorcommon.GeminiTokenCountJSON(count)
}
