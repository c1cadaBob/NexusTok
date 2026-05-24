// Package executor 提供 CLI Proxy API 运行时执行器的测试。
// 本文件测试 Anthropic 提示缓存控制注入功能，验证 system prompt、tools 和 messages
// 上的 cache_control 标记是否按正确顺序和规则注入。
package executor

import (
	"fmt"
	"testing"

	"github.com/tidwall/gjson"
)

// TestEnsureCacheControl 验证 ensureCacheControl 函数在各种场景下正确注入 cache_control 标记。
// 测试覆盖：字符串/数组 system prompt、工具缓存、独立断点、仅工具无 system、大量工具（Claude Code 场景）、
// 空工具数组、多轮消息缓存（倒数第二条用户消息）以及已有 cache_control 时的跳过逻辑。
func TestEnsureCacheControl(t *testing.T) {
	// 测试用例 1：system prompt 为字符串
	t.Run("String System Prompt", func(t *testing.T) {
		input := []byte(`{"model": "claude-3-5-sonnet", "system": "This is a long system prompt", "messages": []}`)
		output := ensureCacheControl(input)

		res := gjson.GetBytes(output, "system.0.cache_control.type")
		if res.String() != "ephemeral" {
			t.Errorf("cache_control not found in system string. Output: %s", string(output))
		}
	})

	// 测试用例 2：system prompt 为数组，cache_control 只应在最后一个元素上
	t.Run("Array System Prompt", func(t *testing.T) {
		input := []byte(`{"model": "claude-3-5-sonnet", "system": [{"type": "text", "text": "Part 1"}, {"type": "text", "text": "Part 2"}], "messages": []}`)
		output := ensureCacheControl(input)

		// cache_control 只应在最后一个元素上
		res0 := gjson.GetBytes(output, "system.0.cache_control")
		res1 := gjson.GetBytes(output, "system.1.cache_control.type")

		if res0.Exists() {
			t.Errorf("cache_control should NOT be on the first element")
		}
		if res1.String() != "ephemeral" {
			t.Errorf("cache_control not found on last system element. Output: %s", string(output))
		}
	})

	// 测试用例 3：工具缓存
	t.Run("Tools Caching", func(t *testing.T) {
		input := []byte(`{
			"model": "claude-3-5-sonnet",
			"tools": [
				{"name": "tool1", "description": "First tool", "input_schema": {"type": "object"}},
				{"name": "tool2", "description": "Second tool", "input_schema": {"type": "object"}}
			],
			"system": "System prompt",
			"messages": []
		}`)
		output := ensureCacheControl(input)

		// cache_control 只应在最后一个工具上
		tool0Cache := gjson.GetBytes(output, "tools.0.cache_control")
		tool1Cache := gjson.GetBytes(output, "tools.1.cache_control.type")

		if tool0Cache.Exists() {
			t.Errorf("cache_control should NOT be on the first tool")
		}
		if tool1Cache.String() != "ephemeral" {
			t.Errorf("cache_control not found on last tool. Output: %s", string(output))
		}

		// system 也应有 cache_control
		systemCache := gjson.GetBytes(output, "system.0.cache_control.type")
		if systemCache.String() != "ephemeral" {
			t.Errorf("cache_control not found in system. Output: %s", string(output))
		}
	})

	// 测试用例 4：工具和 system 是独立的缓存断点
	// 根据 Anthropic 文档：最多允许 4 个断点，工具和 system 是分开缓存的
	t.Run("Independent Cache Breakpoints", func(t *testing.T) {
		input := []byte(`{
			"model": "claude-3-5-sonnet",
			"tools": [
				{"name": "tool1", "description": "First tool", "input_schema": {"type": "object"}, "cache_control": {"type": "ephemeral"}}
			],
			"system": [{"type": "text", "text": "System"}],
			"messages": []
		}`)
		output := ensureCacheControl(input)

		// 工具已有 cache_control - 不应被修改
		tool0Cache := gjson.GetBytes(output, "tools.0.cache_control.type")
		if tool0Cache.String() != "ephemeral" {
			t.Errorf("existing cache_control was incorrectly removed")
		}

		// system 应获得自己的 cache_control，因为它是独立的断点
		// 工具和 system 在缓存层级中是分开的
		systemCache := gjson.GetBytes(output, "system.0.cache_control.type")
		if systemCache.String() != "ephemeral" {
			t.Errorf("system should have its own cache_control breakpoint (independent of tools)")
		}
	})

	// 测试用例 5：仅有工具，无 system
	t.Run("Only Tools No System", func(t *testing.T) {
		input := []byte(`{
			"model": "claude-3-5-sonnet",
			"tools": [
				{"name": "tool1", "description": "Tool", "input_schema": {"type": "object"}}
			],
			"messages": [{"role": "user", "content": "Hi"}]
		}`)
		output := ensureCacheControl(input)

		toolCache := gjson.GetBytes(output, "tools.0.cache_control.type")
		if toolCache.String() != "ephemeral" {
			t.Errorf("cache_control not found on tool. Output: %s", string(output))
		}
	})

	// 测试用例 6：大量工具（Claude Code 场景）
	t.Run("Many Tools (Claude Code Scenario)", func(t *testing.T) {
		// 模拟 Claude Code 使用大量工具的场景
		toolsJSON := `[`
		for i := 0; i < 50; i++ {
			if i > 0 {
				toolsJSON += ","
			}
			toolsJSON += fmt.Sprintf(`{"name": "tool%d", "description": "Tool %d", "input_schema": {"type": "object"}}`, i, i)
		}
		toolsJSON += `]`

		input := []byte(fmt.Sprintf(`{
			"model": "claude-3-5-sonnet",
			"tools": %s,
			"system": [{"type": "text", "text": "You are Claude Code"}],
			"messages": [{"role": "user", "content": "Hello"}]
		}`, toolsJSON))

		output := ensureCacheControl(input)

		// 只有最后一个工具（索引 49）应有 cache_control
		for i := 0; i < 49; i++ {
			path := fmt.Sprintf("tools.%d.cache_control", i)
			if gjson.GetBytes(output, path).Exists() {
				t.Errorf("tool %d should NOT have cache_control", i)
			}
		}

		lastToolCache := gjson.GetBytes(output, "tools.49.cache_control.type")
		if lastToolCache.String() != "ephemeral" {
			t.Errorf("last tool (49) should have cache_control")
		}

		// system 也应有 cache_control
		systemCache := gjson.GetBytes(output, "system.0.cache_control.type")
		if systemCache.String() != "ephemeral" {
			t.Errorf("system should have cache_control")
		}

		t.Log("test passed: 50 tools - cache_control only on last tool")
	})

	// 测试用例 7：空工具数组
	t.Run("Empty Tools Array", func(t *testing.T) {
		input := []byte(`{"model": "claude-3-5-sonnet", "tools": [], "system": "Test", "messages": []}`)
		output := ensureCacheControl(input)

		// 即使工具数组为空，system 仍应获得 cache_control
		systemCache := gjson.GetBytes(output, "system.0.cache_control.type")
		if systemCache.String() != "ephemeral" {
			t.Errorf("system should have cache_control even with empty tools array")
		}
	})

	// 测试用例 8：多轮消息缓存（倒数第二条用户消息）
	t.Run("Messages Caching Second-To-Last User", func(t *testing.T) {
		input := []byte(`{
			"model": "claude-3-5-sonnet",
			"messages": [
				{"role": "user", "content": "First user"},
				{"role": "assistant", "content": "Assistant reply"},
				{"role": "user", "content": "Second user"},
				{"role": "assistant", "content": "Assistant reply 2"},
				{"role": "user", "content": "Third user"}
			]
		}`)
		output := ensureCacheControl(input)

		cacheType := gjson.GetBytes(output, "messages.2.content.0.cache_control.type")
		if cacheType.String() != "ephemeral" {
			t.Errorf("cache_control not found on second-to-last user turn. Output: %s", string(output))
		}

		lastUserCache := gjson.GetBytes(output, "messages.4.content.0.cache_control")
		if lastUserCache.Exists() {
			t.Errorf("last user turn should NOT have cache_control")
		}
	})

	// 测试用例 9：已有消息 cache_control 时应跳过注入
	t.Run("Messages Skip When Cache Control Exists", func(t *testing.T) {
		input := []byte(`{
			"model": "claude-3-5-sonnet",
			"messages": [
				{"role": "user", "content": [{"type": "text", "text": "First user"}]},
				{"role": "assistant", "content": [{"type": "text", "text": "Assistant reply", "cache_control": {"type": "ephemeral"}}]},
				{"role": "user", "content": [{"type": "text", "text": "Second user"}]}
			]
		}`)
		output := ensureCacheControl(input)

		userCache := gjson.GetBytes(output, "messages.0.content.0.cache_control")
		if userCache.Exists() {
			t.Errorf("cache_control should NOT be injected when a message already has cache_control")
		}

		existingCache := gjson.GetBytes(output, "messages.1.content.0.cache_control.type")
		if existingCache.String() != "ephemeral" {
			t.Errorf("existing cache_control should be preserved. Output: %s", string(output))
		}
	})
}

// TestCacheControlOrder 验证缓存控制的正确顺序：tools -> system -> messages。
// 确保最后一个工具和最后一个 system 元素获得 cache_control，而第一个元素不会获得。
func TestCacheControlOrder(t *testing.T) {
	input := []byte(`{
		"model": "claude-sonnet-4",
		"tools": [
			{"name": "Read", "description": "Read file", "input_schema": {"type": "object", "properties": {"path": {"type": "string"}}}},
			{"name": "Write", "description": "Write file", "input_schema": {"type": "object", "properties": {"path": {"type": "string"}, "content": {"type": "string"}}}}
		],
		"system": [
			{"type": "text", "text": "You are Claude Code, Anthropic's official CLI for Claude."},
			{"type": "text", "text": "Additional instructions here..."}
		],
		"messages": [
			{"role": "user", "content": "Hello"}
		]
	}`)

	output := ensureCacheControl(input)

	// 1. 最后一个工具应有 cache_control
	if gjson.GetBytes(output, "tools.1.cache_control.type").String() != "ephemeral" {
		t.Error("last tool should have cache_control")
	}

	// 2. 第一个工具不应有 cache_control
	if gjson.GetBytes(output, "tools.0.cache_control").Exists() {
		t.Error("first tool should NOT have cache_control")
	}

	// 3. 最后一个 system 元素应有 cache_control
	if gjson.GetBytes(output, "system.1.cache_control.type").String() != "ephemeral" {
		t.Error("last system element should have cache_control")
	}

	// 4. 第一个 system 元素不应有 cache_control
	if gjson.GetBytes(output, "system.0.cache_control").Exists() {
		t.Error("first system element should NOT have cache_control")
	}

	t.Log("cache order correct: tools -> system")
}
