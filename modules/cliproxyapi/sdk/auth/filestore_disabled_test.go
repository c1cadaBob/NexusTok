// auth - filestore_disabled_test.go
// 本文件包含 extractAccessToken 辅助函数的单元测试，
// 验证从不同格式的元数据中正确提取访问令牌的功能。
package auth

import "testing"

// TestExtractAccessToken 测试 extractAccessToken 函数的各种场景。
// 覆盖的场景包括：
//   - Antigravity 格式（顶层 access_token）
//   - Gemini 格式（嵌套在 token 对象中的 access_token）
//   - 顶层优先级高于嵌套
//   - 空元数据
//   - 空白字符串
//   - 错误类型
//   - 回退到嵌套字段
func TestExtractAccessToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		metadata map[string]any
		expected string
	}{
		{
			"antigravity top-level access_token",
			map[string]any{"access_token": "tok-abc"},
			"tok-abc",
		},
		{
			"gemini nested token.access_token",
			map[string]any{
				"token": map[string]any{"access_token": "tok-nested"},
			},
			"tok-nested",
		},
		{
			"codex legacy token_data.access_token",
			map[string]any{
				"token_data": map[string]any{"access_token": "tok-token-data"},
			},
			"tok-token-data",
		},
		{
			"top-level takes precedence over nested",
			map[string]any{
				"access_token": "tok-top",
				"token":        map[string]any{"access_token": "tok-nested"},
			},
			"tok-top",
		},
		{
			"empty metadata",
			map[string]any{},
			"",
		},
		{
			"whitespace-only access_token",
			map[string]any{"access_token": "   "},
			"",
		},
		{
			"wrong type access_token",
			map[string]any{"access_token": 12345},
			"",
		},
		{
			"token is not a map",
			map[string]any{"token": "not-a-map"},
			"",
		},
		{
			"nested whitespace-only",
			map[string]any{
				"token": map[string]any{"access_token": "  "},
			},
			"",
		},
		{
			"fallback to nested when top-level empty",
			map[string]any{
				"access_token": "",
				"token":        map[string]any{"access_token": "tok-fallback"},
			},
			"tok-fallback",
		},
		{
			"fallback to token_data when top-level empty",
			map[string]any{
				"access_token": "",
				"token_data":   map[string]any{"access_token": "tok-token-data-fallback"},
			},
			"tok-token-data-fallback",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := extractAccessToken(tt.metadata)
			if got != tt.expected {
				t.Errorf("extractAccessToken() = %q, want %q", got, tt.expected)
			}
		})
	}
}
