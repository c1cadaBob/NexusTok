// openai/gemini-cli - openai_gemini_response.go
// Package geminiCLI provides response translation functionality for OpenAI to Gemini API.
// 本文件提供 OpenAI API 响应到 Gemini CLI API 格式的转换功能。
// 将 OpenAI 流式/非流式响应委托给 openai/gemini 包进行格式转换，
// 然后在外层包裹 {"response": {...}} 结构以匹配 Gemini CLI API 响应格式。
package geminiCLI

import (
	"context"

	translatorcommon "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/common"
	. "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/openai/gemini"
)

// ConvertOpenAIResponseToGeminiCLI converts OpenAI Chat Completions streaming response format to Gemini API format.
// This function processes OpenAI streaming chunks and transforms them into Gemini-compatible JSON responses.
// It handles text content, tool calls, and usage metadata, outputting responses that match the Gemini API format.
//
// Parameters:
//   - ctx: The context for the request.
//   - modelName: The name of the model.
//   - rawJSON: The raw JSON response from the OpenAI API.
//   - param: A pointer to a parameter object for the conversion.
//
// Returns:
//   - [][]byte: A slice of Gemini-compatible JSON responses.
func ConvertOpenAIResponseToGeminiCLI(ctx context.Context, modelName string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) [][]byte {
	outputs := ConvertOpenAIResponseToGemini(ctx, modelName, originalRequestRawJSON, requestRawJSON, rawJSON, param)
	newOutputs := make([][]byte, 0, len(outputs))
	for i := 0; i < len(outputs); i++ {
		newOutputs = append(newOutputs, translatorcommon.WrapGeminiCLIResponse(outputs[i]))
	}
	return newOutputs
}

// ConvertOpenAIResponseToGeminiCLINonStream converts a non-streaming OpenAI response to a non-streaming Gemini CLI response.
//
// Parameters:
//   - ctx: The context for the request.
//   - modelName: The name of the model.
//   - rawJSON: The raw JSON response from the OpenAI API.
//   - param: A pointer to a parameter object for the conversion.
//
// Returns:
//   - []byte: A Gemini-compatible JSON response.
func ConvertOpenAIResponseToGeminiCLINonStream(ctx context.Context, modelName string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) []byte {
	out := ConvertOpenAIResponseToGeminiNonStream(ctx, modelName, originalRequestRawJSON, requestRawJSON, rawJSON, param)
	return translatorcommon.WrapGeminiCLIResponse(out)
}

// GeminiCLITokenCount 生成 Gemini 格式的 Token 计数 JSON 响应。
func GeminiCLITokenCount(ctx context.Context, count int64) []byte {
	return translatorcommon.GeminiTokenCountJSON(count)
}
