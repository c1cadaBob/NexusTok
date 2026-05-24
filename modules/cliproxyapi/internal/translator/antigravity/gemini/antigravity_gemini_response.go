// antigravity/gemini - antigravity_gemini_response.go
// Package gemini provides request translation functionality for Gemini to Gemini CLI API compatibility.
// 本文件提供 Antigravity（Gemini CLI）API 响应到 Gemini API 格式的转换功能。
// 处理流式和非流式响应，从 Gemini CLI 响应中提取 response 对象，
// 并将 cpaUsageMetadata 恢复为 usageMetadata 以匹配标准 Gemini API 格式。
package gemini

import (
	"bytes"
	"context"

	translatorcommon "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/common"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ConvertAntigravityResponseToGemini parses and transforms a Gemini CLI API request into Gemini API format.
// It extracts the model name, system instruction, message contents, and tool declarations
// from the raw JSON request and returns them in the format expected by the Gemini API.
// The function performs the following transformations:
// 1. Extracts the response data from the request
// 2. Handles alternative response formats
// 3. Processes array responses by extracting individual response objects
//
// Parameters:
//   - ctx: The context for the request, used for cancellation and timeout handling
//   - modelName: The name of the model to use for the request (unused in current implementation)
//   - rawJSON: The raw JSON request data from the Gemini CLI API
//   - param: A pointer to a parameter object for the conversion (unused in current implementation)
//
// Returns:
//   - [][]byte: The transformed response data in Gemini API format.
func ConvertAntigravityResponseToGemini(ctx context.Context, _ string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, _ *any) [][]byte {
	if bytes.HasPrefix(rawJSON, []byte("data:")) {
		rawJSON = bytes.TrimSpace(rawJSON[5:])
	}

	if alt, ok := ctx.Value("alt").(string); ok {
		var chunk []byte
		if alt == "" {
			responseResult := gjson.GetBytes(rawJSON, "response")
			if responseResult.Exists() {
				chunk = []byte(responseResult.Raw)
				chunk = restoreUsageMetadata(chunk)
			}
		} else {
			chunkTemplate := []byte("[]")
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

// ConvertAntigravityResponseToGeminiNonStream converts a non-streaming Gemini CLI request to a non-streaming Gemini response.
// This function processes the complete Gemini CLI request and transforms it into a single Gemini-compatible
// JSON response. It extracts the response data from the request and returns it in the expected format.
//
// Parameters:
//   - ctx: The context for the request, used for cancellation and timeout handling
//   - modelName: The name of the model being used for the response (unused in current implementation)
//   - rawJSON: The raw JSON request data from the Gemini CLI API
//   - param: A pointer to a parameter object for the conversion (unused in current implementation)
//
// Returns:
//   - []byte: A Gemini-compatible JSON response containing the response data.
func ConvertAntigravityResponseToGeminiNonStream(_ context.Context, _ string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, _ *any) []byte {
	responseResult := gjson.GetBytes(rawJSON, "response")
	if responseResult.Exists() {
		chunk := restoreUsageMetadata([]byte(responseResult.Raw))
		return chunk
	}
	return rawJSON
}

// GeminiTokenCount 生成 Gemini 格式的 Token 计数 JSON 响应。
func GeminiTokenCount(ctx context.Context, count int64) []byte {
	return translatorcommon.GeminiTokenCountJSON(count)
}

// restoreUsageMetadata 将 cpaUsageMetadata 重命名回 usageMetadata。
// 执行器在非终端块中将 usageMetadata 重命名为 cpaUsageMetadata，
// 以在保留使用数据的同时对不期望它的客户端隐藏。
// 返回标准 Gemini API 格式时，必须恢复原始名称。
func restoreUsageMetadata(chunk []byte) []byte {
	if cpaUsage := gjson.GetBytes(chunk, "cpaUsageMetadata"); cpaUsage.Exists() {
		chunk, _ = sjson.SetRawBytes(chunk, "usageMetadata", []byte(cpaUsage.Raw))
		chunk, _ = sjson.DeleteBytes(chunk, "cpaUsageMetadata")
	}
	return chunk
}
