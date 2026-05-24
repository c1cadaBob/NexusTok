// util - claude_model.go
// 本文件提供了 Claude 模型相关的工具函数。
// 处理 Claude 模型名称的解析和判断。
package util

import "strings"

// IsClaudeThinkingModel checks if the model is a Claude thinking model
// that requires the interleaved-thinking beta header.
func IsClaudeThinkingModel(model string) bool {
	lower := strings.ToLower(model)
	return strings.Contains(lower, "claude") && strings.Contains(lower, "thinking")
}
