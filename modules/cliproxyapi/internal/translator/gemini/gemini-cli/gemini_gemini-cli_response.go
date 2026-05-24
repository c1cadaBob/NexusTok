// gemini/gemini-cli - gemini_gemini-cli_response.go
// Package gemini_cli provides response translation functionality for Gemini API to Gemini CLI API.
// 本文件提供 Gemini API 响应到 Gemini CLI API 格式的转换功能。
// 将 Gemini 流式/非流式响应包装为 Gemini CLI 期望的单行 JSON 格式，
// 即在原始响应外层包裹 {"response": {...}} 结构。
package geminiCLI

import (
	"bytes"
	"context"

	translatorcommon "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/common"
	"github.com/tidwall/sjson"
)

// dataTag 是 SSE 数据行的前缀标识，用于从原始响应中提取 JSON 数据。
var dataTag = []byte("data:")

// ConvertGeminiResponseToGeminiCLI converts Gemini streaming response format to Gemini CLI single-line JSON format.
// This function processes various Gemini event types and transforms them into Gemini CLI-compatible JSON responses.
// It handles thinking content, regular text content, and function calls, outputting single-line JSON
// that matches the Gemini CLI API response format.
//
// Parameters:
//   - ctx: The context for the request.
//   - modelName: The name of the model.
//   - rawJSON: The raw JSON response from the Gemini API.
//   - param: A pointer to a parameter object for the conversion (unused).
//
// Returns:
//   - [][]byte: A slice of Gemini CLI-compatible JSON responses.
func ConvertGeminiResponseToGeminiCLI(_ context.Context, _ string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, _ *any) [][]byte {
	if !bytes.HasPrefix(rawJSON, dataTag) {
		return [][]byte{}
	}
	rawJSON = bytes.TrimSpace(rawJSON[5:])

	if bytes.Equal(rawJSON, []byte("[DONE]")) {
		return [][]byte{}
	}
	rawJSON, _ = sjson.SetRawBytes([]byte(`{"response":{}}`), "response", rawJSON)
	return [][]byte{rawJSON}
}

// ConvertGeminiResponseToGeminiCLINonStream converts a non-streaming Gemini response to a non-streaming Gemini CLI response.
//
// Parameters:
//   - ctx: The context for the request.
//   - modelName: The name of the model.
//   - rawJSON: The raw JSON response from the Gemini API.
//   - param: A pointer to a parameter object for the conversion (unused).
//
// Returns:
//   - []byte: A Gemini CLI-compatible JSON response.
func ConvertGeminiResponseToGeminiCLINonStream(_ context.Context, _ string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, _ *any) []byte {
	rawJSON, _ = sjson.SetRawBytes([]byte(`{"response":{}}`), "response", rawJSON)
	return rawJSON
}

// GeminiCLITokenCount 生成 Gemini 格式的 Token 计数 JSON 响应。
func GeminiCLITokenCount(ctx context.Context, count int64) []byte {
	return translatorcommon.GeminiTokenCountJSON(count)
}
