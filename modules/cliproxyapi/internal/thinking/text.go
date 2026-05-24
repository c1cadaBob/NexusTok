// thinking - text.go
// 本文件提供了从响应内容中提取 thinking 文本的功能。
// 支持多种格式：简单字符串、包装对象、Gemini 风格等。
package thinking

import (
	"github.com/tidwall/gjson"
)

// GetThinkingText 从内容片段中提取 thinking 文本。
// 支持以下格式：
//   - 简单字符串：{ "thinking": "text" } 或 { "text": "text" }
//   - 包装对象：{ "thinking": { "text": "text", "cache_control": {...} } }
//   - Gemini 风格：{ "thought": true, "text": "text" }
//
// 返回提取的文本字符串。
func GetThinkingText(part gjson.Result) string {
	// Try direct text field first (Gemini-style)
	if text := part.Get("text"); text.Exists() && text.Type == gjson.String {
		return text.String()
	}

	// Try thinking field
	thinkingField := part.Get("thinking")
	if !thinkingField.Exists() {
		return ""
	}

	// thinking is a string
	if thinkingField.Type == gjson.String {
		return thinkingField.String()
	}

	// thinking is an object with inner text/thinking
	if thinkingField.IsObject() {
		if inner := thinkingField.Get("text"); inner.Exists() && inner.Type == gjson.String {
			return inner.String()
		}
		if inner := thinkingField.Get("thinking"); inner.Exists() && inner.Type == gjson.String {
			return inner.String()
		}
	}

	return ""
}
