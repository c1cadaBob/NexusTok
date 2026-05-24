// util - claude_tool_id.go
// 本文件提供了 Claude 工具 ID 相关的工具函数。
// 处理 Claude 工具调用 ID 的生成和验证。
package util

import (
	"fmt"
	"regexp"
	"sync/atomic"
	"time"
)

var (
	claudeToolUseIDSanitizer = regexp.MustCompile(`[^a-zA-Z0-9_-]`)
	claudeToolUseIDCounter   uint64
)

// SanitizeClaudeToolID ensures the given id conforms to Claude's
// tool_use.id regex ^[a-zA-Z0-9_-]+$.  Non-conforming characters are
// replaced with '_'; an empty result gets a generated fallback.
func SanitizeClaudeToolID(id string) string {
	s := claudeToolUseIDSanitizer.ReplaceAllString(id, "_")
	if s == "" {
		s = fmt.Sprintf("toolu_%d_%d", time.Now().UnixNano(), atomic.AddUint64(&claudeToolUseIDCounter, 1))
	}
	return s
}
