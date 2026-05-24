// util - claude_attribution_test.go
// Claude Code 归属标识检测测试
// 验证 IsClaudeCodeAttributionSystemText 函数能够正确识别
// Claude Code 客户端发送的归属标识（billing header），
// 区分真正的系统提示词和客户端元数据。
package util

import "testing"

// TestIsClaudeCodeAttributionSystemText 测试各种场景：
// - 包含 x-anthropic-billing-header 的文本应返回 true（Claude Code 归属标识）
// - 带前导空白的归属标识应返回 true
// - 普通系统提示词应返回 false
// - 空文本应返回 false
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
