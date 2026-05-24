// Package reasonmap 提供了不同 AI 提供商之间的停止原因（finish reason / stop reason）映射。
// 主要用于 Claude 和 OpenAI 之间的停止原因格式转换。
package reasonmap

import (
	"strings"

	"github.com/c1cada/NexusTok/constant"
)

// ClaudeStopReasonToOpenAIFinishReason 将 Claude 的停止原因转换为 OpenAI 格式的 finish reason。
// 映射关系：
//   - stop_sequence / end_turn -> "stop"（正常停止）
//   - max_tokens -> "length"（达到最大 token 数）
//   - tool_use -> "tool_calls"（请求调用工具）
//   - refusal -> ContentFilter（内容过滤）
//   - 其他 -> 原样返回
//
// 参数：
//   - stopReason: Claude 返回的停止原因字符串
//
// 返回值：
//   - string: 对应的 OpenAI finish reason 字符串
func ClaudeStopReasonToOpenAIFinishReason(stopReason string) string {
	switch strings.ToLower(stopReason) {
	case "stop_sequence":
		return "stop"
	case "end_turn":
		return "stop"
	case "max_tokens":
		return "length"
	case "tool_use":
		return "tool_calls"
	case "refusal":
		return constant.FinishReasonContentFilter
	default:
		return stopReason
	}
}

// OpenAIFinishReasonToClaudeStopReason 将 OpenAI 的 finish reason 转换为 Claude 格式的 stop reason。
// 映射关系：
//   - stop -> "end_turn"（正常对话结束）
//   - stop_sequence -> "stop_sequence"（停止序列命中）
//   - length / max_tokens -> "max_tokens"（达到最大 token 数）
//   - ContentFilter -> "refusal"（内容过滤拒绝）
//   - tool_calls -> "tool_use"（请求调用工具）
//   - 其他 -> 原样返回
//
// 参数：
//   - finishReason: OpenAI 返回的 finish reason 字符串
//
// 返回值：
//   - string: 对应的 Claude stop reason 字符串
func OpenAIFinishReasonToClaudeStopReason(finishReason string) string {
	switch strings.ToLower(finishReason) {
	case "stop":
		return "end_turn"
	case "stop_sequence":
		return "stop_sequence"
	case "length", "max_tokens":
		return "max_tokens"
	case constant.FinishReasonContentFilter:
		return "refusal"
	case "tool_calls":
		return "tool_use"
	default:
		return finishReason
	}
}
