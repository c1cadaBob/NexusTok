// 包 util - claude_tool_id.go
// 该文件提供了 Claude tool_use ID 的清理功能。
package util

import (
	"fmt"
	"regexp"
	"sync/atomic"
	"time"
)

var (
	// claudeToolUseIDSanitizer 用于清理不符合 Claude tool_use.id 正则要求的字符。
	claudeToolUseIDSanitizer = regexp.MustCompile(`[^a-zA-Z0-9_-]`)
	// claudeToolUseIDCounter 是用于生成唯一 tool_use ID 的原子计数器。
	claudeToolUseIDCounter uint64
)

// SanitizeClaudeToolID 确保给定的 ID 符合 Claude 的 tool_use.id 正则 ^[a-zA-Z0-9_-]+$。
// 不符合的字符替换为 '_'；空结果生成回退 ID。
func SanitizeClaudeToolID(id string) string {
	s := claudeToolUseIDSanitizer.ReplaceAllString(id, "_")
	if s == "" {
		s = fmt.Sprintf("toolu_%d_%d", time.Now().UnixNano(), atomic.AddUint64(&claudeToolUseIDCounter, 1))
	}
	return s
}
