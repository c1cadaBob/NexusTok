// claude/gemini-cli - claude_gemini-cli_response.go
// 提供将 Claude Code API 响应转换为 Gemini CLI 兼容格式的功能。
// 将 Claude Code 的流式和非流式响应转换为 Gemini CLI 格式，
// 并将每个转换后的响应包装在 "response" 对象中以匹配 Gemini CLI API 结构。
package geminiCLI

import (
	"context"

	. "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/claude/gemini"
	translatorcommon "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/common"
)

// ConvertClaudeResponseToGeminiCLI 将 Claude Code 流式响应格式转换为 Gemini CLI 格式。
// 处理各种 Claude Code 事件类型（文本内容、工具调用、使用量元数据），
// 并将它们转换为 Gemini CLI API 格式的 JSON 响应。
// 每个转换后的响应都被包装在 "response" 对象中以匹配 Gemini CLI API 结构。
//
// 参数:
//   - ctx: 请求上下文，用于取消和超时处理
//   - modelName: 响应使用的模型名称
//   - originalRequestRawJSON: 原始请求的 JSON 数据
//   - requestRawJSON: 请求的 JSON 数据
//   - rawJSON: Claude Code API 的原始 JSON 响应
//   - param: 指向参数对象的指针，用于在调用之间维护状态
//
// 返回:
//   - [][]byte: 包装在 response 对象中的 Gemini CLI 兼容 JSON 响应切片
func ConvertClaudeResponseToGeminiCLI(ctx context.Context, modelName string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) [][]byte {
	outputs := ConvertClaudeResponseToGemini(ctx, modelName, originalRequestRawJSON, requestRawJSON, rawJSON, param)
	// 将每个转换后的响应包装在 "response" 对象中以匹配 Gemini CLI API 结构
	newOutputs := make([][]byte, 0, len(outputs))
	for i := 0; i < len(outputs); i++ {
		newOutputs = append(newOutputs, translatorcommon.WrapGeminiCLIResponse(outputs[i]))
	}
	return newOutputs
}

// ConvertClaudeResponseToGeminiCLINonStream 将非流式的 Claude Code 响应转换为非流式的 Gemini CLI 响应。
// 处理完整的 Claude Code 响应并将其转换为单个 Gemini CLI 兼容的 JSON 响应。
// 转换后的响应被包装在 "response" 对象中以匹配 Gemini CLI API 结构。
//
// 参数:
//   - ctx: 请求上下文
//   - modelName: 模型名称
//   - originalRequestRawJSON: 原始请求的 JSON 数据
//   - requestRawJSON: 请求的 JSON 数据
//   - rawJSON: Claude Code API 的原始 JSON 响应
//   - param: 指向参数对象的指针
//
// 返回:
//   - []byte: 包装在 response 对象中的 Gemini CLI 兼容 JSON 响应
func ConvertClaudeResponseToGeminiCLINonStream(ctx context.Context, modelName string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) []byte {
	out := ConvertClaudeResponseToGeminiNonStream(ctx, modelName, originalRequestRawJSON, requestRawJSON, rawJSON, param)
	// 将转换后的响应包装在 "response" 对象中以匹配 Gemini CLI API 结构
	return translatorcommon.WrapGeminiCLIResponse(out)
}

// GeminiCLITokenCount 生成 Gemini CLI 格式的 token 计数 JSON 响应。
// 委托给 GeminiTokenCount 函数进行实际的 token 计数格式化。
func GeminiCLITokenCount(ctx context.Context, count int64) []byte {
	return GeminiTokenCount(ctx, count)
}
