// helps - claude_builtin_tools_test.go
// Claude 内置工具注册表的单元测试。
// 测试以下功能：
// - 默认种子回退：当未提供工具列表时，使用默认的内置工具名称
// - 从请求体增强：将带 type 字段的工具添加到内置注册表，无 type 的工具不加入
package helps

import "testing"

// TestClaudeBuiltinToolRegistry_DefaultSeedFallback 测试当传入 nil 时，
// 回退注册表包含所有默认的 Claude 内置工具名称
func TestClaudeBuiltinToolRegistry_DefaultSeedFallback(t *testing.T) {
	registry := AugmentClaudeBuiltinToolRegistry(nil, nil)
	for _, name := range defaultClaudeBuiltinToolNames {
		if !registry[name] {
			t.Fatalf("default builtin %q missing from fallback registry", name)
		}
	}
}

// TestClaudeBuiltinToolRegistry_AugmentsTypedBuiltinsFromBody 测试从请求体增强注册表：
// - 带 type 字段的工具（如 web_search_20250305）被添加到注册表
// - 自定义带 type 的工具也被添加
// - 无 type 字段的工具（如 Read）不被添加
func TestClaudeBuiltinToolRegistry_AugmentsTypedBuiltinsFromBody(t *testing.T) {
	registry := AugmentClaudeBuiltinToolRegistry([]byte(`{
		"tools": [
			{"type": "web_search_20250305", "name": "web_search"},
			{"type": "custom_builtin_20250401", "name": "special_builtin"},
			{"name": "Read"}
		]
	}`), nil)

	if !registry["web_search"] {
		t.Fatal("expected default typed builtin web_search in registry")
	}
	if !registry["special_builtin"] {
		t.Fatal("expected typed builtin from body to be added to registry")
	}
	if registry["Read"] {
		t.Fatal("expected untyped custom tool to stay out of builtin registry")
	}
}
