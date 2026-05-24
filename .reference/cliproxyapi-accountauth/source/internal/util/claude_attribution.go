// 包 util - claude_attribution.go
// 该文件提供了 Claude Code 归属文本检测功能。
// 用于识别携带按请求计费和提示指纹数据的 Claude Code 归属块。
package util

import (
	"strings"
	"unicode"
)

// claudeCodeAttributionSystemPrefix 是 Claude Code 归属系统文本的前缀。
const claudeCodeAttributionSystemPrefix = "x-anthropic-billing-header:"

// IsClaudeCodeAttributionSystemText 检查文本是否为 Claude Code 归属块。
// 归属块携带按请求计费和提示指纹数据。
func IsClaudeCodeAttributionSystemText(text string) bool {
	text = strings.TrimLeftFunc(text, unicode.IsSpace)
	return strings.HasPrefix(text, claudeCodeAttributionSystemPrefix)
}
