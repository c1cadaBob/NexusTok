// Package test - claude_code_compatibility_sentinel_test.go
// Claude Code 兼容性哨兵测试。
// 测试 Claude Code 特有的事件类型的数据结构是否符合预期格式，
// 包括工具进度、会话状态变化、工具使用摘要和控制请求等事件。
// 这些测试确保代理能够正确处理 Claude Code 的专有事件格式。
package test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// jsonObject 是 JSON 对象的类型别名，用于简化测试代码。
type jsonObject = map[string]any

// loadClaudeCodeSentinelFixture 从 testdata 目录加载 Claude Code 哨兵测试数据文件。
// 文件路径为 testdata/claude_code_sentinels/{name}。
//
// 参数:
//   - t: 测试实例
//   - name: 测试数据文件名
//
// 返回值:
//   - jsonObject: 解析后的 JSON 对象
func loadClaudeCodeSentinelFixture(t *testing.T, name string) jsonObject {
	t.Helper()
	path := filepath.Join("testdata", "claude_code_sentinels", name)
	data := mustReadFile(t, path)
	var payload jsonObject
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal %s: %v", name, err)
	}
	return payload
}

// mustReadFile 读取指定路径的文件内容，失败时终止测试。
//
// 参数:
//   - t: 测试实例
//   - path: 文件路径
//
// 返回值:
//   - []byte: 文件内容
func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

// requireStringField 从 JSON 对象中提取指定键的字符串值。
// 如果键不存在或值为空，终止测试。
//
// 参数:
//   - t: 测试实例
//   - obj: JSON 对象
//   - key: 要提取的键名
//
// 返回值:
//   - string: 键对应的字符串值
func requireStringField(t *testing.T, obj jsonObject, key string) string {
	t.Helper()
	value, ok := obj[key].(string)
	if !ok || value == "" {
		t.Fatalf("field %q missing or empty: %#v", key, obj[key])
	}
	return value
}

// TestClaudeCodeSentinel_ToolProgressShape 验证工具进度事件的数据结构。
// 检查 type、tool_use_id、tool_name、session_id 和 elapsed_time_seconds 字段。
func TestClaudeCodeSentinel_ToolProgressShape(t *testing.T) {
	payload := loadClaudeCodeSentinelFixture(t, "tool_progress.json")
	if got := requireStringField(t, payload, "type"); got != "tool_progress" {
		t.Fatalf("type = %q, want tool_progress", got)
	}
	requireStringField(t, payload, "tool_use_id")
	requireStringField(t, payload, "tool_name")
	requireStringField(t, payload, "session_id")
	if _, ok := payload["elapsed_time_seconds"].(float64); !ok {
		t.Fatalf("elapsed_time_seconds missing or non-number: %#v", payload["elapsed_time_seconds"])
	}
}

// TestClaudeCodeSentinel_SessionStateShape 验证会话状态变化事件的数据结构。
// 检查 type、subtype、state（必须为 idle/running/requires_action 之一）和 session_id 字段。
func TestClaudeCodeSentinel_SessionStateShape(t *testing.T) {
	payload := loadClaudeCodeSentinelFixture(t, "session_state_changed.json")
	if got := requireStringField(t, payload, "type"); got != "system" {
		t.Fatalf("type = %q, want system", got)
	}
	if got := requireStringField(t, payload, "subtype"); got != "session_state_changed" {
		t.Fatalf("subtype = %q, want session_state_changed", got)
	}
	state := requireStringField(t, payload, "state")
	switch state {
	case "idle", "running", "requires_action":
	default:
		t.Fatalf("unexpected session state %q", state)
	}
	requireStringField(t, payload, "session_id")
}

// TestClaudeCodeSentinel_ToolUseSummaryShape 验证工具使用摘要事件的数据结构。
// 检查 type、summary 和 preceding_tool_use_ids 字段。
func TestClaudeCodeSentinel_ToolUseSummaryShape(t *testing.T) {
	payload := loadClaudeCodeSentinelFixture(t, "tool_use_summary.json")
	if got := requireStringField(t, payload, "type"); got != "tool_use_summary" {
		t.Fatalf("type = %q, want tool_use_summary", got)
	}
	requireStringField(t, payload, "summary")
	rawIDs, ok := payload["preceding_tool_use_ids"].([]any)
	if !ok || len(rawIDs) == 0 {
		t.Fatalf("preceding_tool_use_ids missing or empty: %#v", payload["preceding_tool_use_ids"])
	}
	for i, raw := range rawIDs {
		if id, ok := raw.(string); !ok || id == "" {
			t.Fatalf("preceding_tool_use_ids[%d] invalid: %#v", i, raw)
		}
	}
}

// TestClaudeCodeSentinel_ControlRequestCanUseToolShape 验证控制请求（can_use_tool）事件的数据结构。
// 检查 type、request_id 和 request 对象中的 subtype、tool_name、tool_use_id、input 字段。
func TestClaudeCodeSentinel_ControlRequestCanUseToolShape(t *testing.T) {
	payload := loadClaudeCodeSentinelFixture(t, "control_request_can_use_tool.json")
	if got := requireStringField(t, payload, "type"); got != "control_request" {
		t.Fatalf("type = %q, want control_request", got)
	}
	requireStringField(t, payload, "request_id")
	request, ok := payload["request"].(map[string]any)
	if !ok {
		t.Fatalf("request missing or invalid: %#v", payload["request"])
	}
	if got := requireStringField(t, request, "subtype"); got != "can_use_tool" {
		t.Fatalf("request.subtype = %q, want can_use_tool", got)
	}
	requireStringField(t, request, "tool_name")
	requireStringField(t, request, "tool_use_id")
	if input, ok := request["input"].(map[string]any); !ok || len(input) == 0 {
		t.Fatalf("request.input missing or empty: %#v", request["input"])
	}
}
