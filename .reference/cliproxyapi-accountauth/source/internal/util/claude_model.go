// 包 util - claude_model.go
// 该文件提供了 Claude 模型相关的辅助函数。
package util

import "strings"

// IsClaudeThinkingModel 检查模型是否为需要 interleaved-thinking beta 头的 Claude 思考模型。
func IsClaudeThinkingModel(model string) bool {
	lower := strings.ToLower(model)
	return strings.Contains(lower, "claude") && strings.Contains(lower, "thinking")
}
