// auth - filestore_test.go
// 令牌访问令牌提取功能测试
// 验证 extractAccessToken 函数能够从不同结构的元数据中正确提取 access_token，
// 支持顶层字段、嵌套 token 对象、空值、类型错误等多种边界情况。
package auth

import "testing"

// TestExtractAccessToken 测试 extractAccessToken 函数的各种场景：
// - Antigravity 提供商：顶层 access_token 字段
// - Gemini 提供商：嵌套在 token.access_token 中的令牌
// - 优先级：顶层字段优先于嵌套字段
// - 空元数据：返回空字符串
// - 纯空白字符串：返回空字符串
// - 类型错误（非字符串）：返回空字符串
// - token 字段非 map 类型：返回空字符串
// - 顶层为空时回退到嵌套字段
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
