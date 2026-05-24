// util - claude_model_test.go
// Claude 思考模型识别测试
// 验证 IsClaudeThinkingModel 函数能够正确识别 Claude 的思考模型变体，
// 同时排除非 Claude 模型和不带思考功能的 Claude 模型。
package util

import "testing"

// TestIsClaudeThinkingModel 测试模型识别的各种场景：
// - Claude 思考模型（如 claude-sonnet-4-5-thinking）应返回 true
// - 大小写不敏感（Claude-Sonnet-4-5-Thinking 也应返回 true）
// - 非思考 Claude 模型（如 claude-sonnet-4-5）应返回 false
// - 非 Claude 模型（如 gemini-3-pro-thinking）应返回 false
// - 空字符串应返回 false
func TestIsClaudeThinkingModel(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		expected bool
	}{
		// Claude thinking models - should return true
		{"claude-sonnet-4-5-thinking", "claude-sonnet-4-5-thinking", true},
		{"claude-opus-4-5-thinking", "claude-opus-4-5-thinking", true},
		{"claude-opus-4-6-thinking", "claude-opus-4-6-thinking", true},
		{"Claude-Sonnet-Thinking uppercase", "Claude-Sonnet-4-5-Thinking", true},
		{"claude thinking mixed case", "Claude-THINKING-Model", true},

		// Non-thinking Claude models - should return false
		{"claude-sonnet-4-5 (no thinking)", "claude-sonnet-4-5", false},
		{"claude-opus-4-5 (no thinking)", "claude-opus-4-5", false},
		{"claude-3-5-sonnet", "claude-3-5-sonnet-20240620", false},

		// Non-Claude models - should return false
		{"gemini-3-pro-preview", "gemini-3-pro-preview", false},
		{"gemini-thinking model", "gemini-3-pro-thinking", false}, // not Claude
		{"gpt-4o", "gpt-4o", false},
		{"empty string", "", false},

		// Edge cases
		{"thinking without claude", "thinking-model", false},
		{"claude without thinking", "claude-model", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsClaudeThinkingModel(tt.model)
			if result != tt.expected {
				t.Errorf("IsClaudeThinkingModel(%q) = %v, expected %v", tt.model, result, tt.expected)
			}
		})
	}
}
