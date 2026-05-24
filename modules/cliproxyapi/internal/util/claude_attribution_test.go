// util - claude_attribution_test.go
// 该文件包含 IsClaudeCodeAttributionSystemText 函数的单元测试，
// 验证是否能正确识别 Claude Code 的计费归因系统文本（x-anthropic-billing-header）。
package util

import "testing"

// TestIsClaudeCodeAttributionSystemText 测试各种输入场景下
// IsClaudeCodeAttributionSystemText 函数的判断准确性，包括标准归因块、
// 前导空白、普通系统提示和空文本等用例。
func TestIsClaudeCodeAttributionSystemText(t *testing.T) {
	tests := []struct {
		name string
		text string
		want bool
	}{
		{
			name: "Claude Code attribution block",
			text: "x-anthropic-billing-header: cc_version=2.1.63.abc; cc_entrypoint=cli; cch=12345;",
			want: true,
		},
		{
			name: "leading whitespace",
			text: "\n\t x-anthropic-billing-header: cc_version=2.1.63.abc; cch=12345;",
			want: true,
		},
		{
			name: "regular system prompt",
			text: "You are helpful.",
			want: false,
		},
		{
			name: "empty text",
			text: "",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsClaudeCodeAttributionSystemText(tt.text); got != tt.want {
				t.Fatalf("IsClaudeCodeAttributionSystemText(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}
