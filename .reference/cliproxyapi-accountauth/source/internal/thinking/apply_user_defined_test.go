// thinking_test - apply_user_defined_test.go
// 用户自定义模型的思考（thinking）配置应用功能的单元测试文件。
// 验证当模型被注册为 UserDefined（用户自定义）时，ApplyThinking 函数能正确
// 处理思考参数，包括保留自适应（adaptive）级别和后缀指定的努力级别，
// 并移除不兼容的 budget_tokens 字段。
package thinking_test

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/thinking"
	// 导入 Claude 提供者以注册其思考参数处理器（通过 init 函数自动注册）
	_ "github.com/router-for-me/CLIProxyAPI/v7/internal/thinking/provider/claude"
	"github.com/tidwall/gjson"
)

// TestApplyThinking_UserDefinedClaudePreservesAdaptiveLevel 验证 ApplyThinking 函数
// 对用户自定义的 Claude 模型能正确保留 adaptive（自适应）思考类型和对应的努力级别。
//
// 测试场景包括：
// 1. 请求体中直接指定 {"thinking":{"type":"adaptive"}, "output_config":{"effort":"high"}}
//    -- 应保留 adaptive 类型和 high 努力级别，并移除 budget_tokens
// 2. 通过模型名称后缀指定努力级别：modelID + "(high)"
//    -- 应保留 adaptive 类型和 high 努力级别
//
// 两种场景都验证 budget_tokens 字段被移除，因为 adaptive 模式不使用固定预算。
func TestApplyThinking_UserDefinedClaudePreservesAdaptiveLevel(t *testing.T) {
	// 获取全局模型注册表
	reg := registry.GetGlobalRegistry()
	clientID := "test-user-defined-claude-" + t.Name()
	modelID := "custom-claude-4-6"
	// 注册一个用户自定义的 Claude 模型
	reg.RegisterClient(clientID, "claude", []*registry.ModelInfo{{ID: modelID, UserDefined: true}})
	t.Cleanup(func() {
		reg.UnregisterClient(clientID)
	})

	tests := []struct {
		name  string // 测试用例名称
		model string // 模型标识（可能包含后缀）
		body  []byte // 请求体 JSON
	}{
		{
			name:  "claude adaptive effort body",
			model: modelID,
			body:  []byte(`{"thinking":{"type":"adaptive"},"output_config":{"effort":"high"}}`),
		},
		{
			name:  "suffix level",
			model: modelID + "(high)",
			body:  []byte(`{}`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 调用 ApplyThinking，传入请求体、模型名、输入/输出 API 类型和提供者类型
			out, err := thinking.ApplyThinking(tt.body, tt.model, "openai", "claude", "claude")
			if err != nil {
				t.Fatalf("ApplyThinking() error = %v", err)
			}
			// 验证思考类型为 adaptive（自适应）
			if got := gjson.GetBytes(out, "thinking.type").String(); got != "adaptive" {
				t.Fatalf("thinking.type = %q, want %q, body=%s", got, "adaptive", string(out))
			}
			// 验证努力级别为 high
			if got := gjson.GetBytes(out, "output_config.effort").String(); got != "high" {
				t.Fatalf("output_config.effort = %q, want %q, body=%s", got, "high", string(out))
			}
			// 验证 budget_tokens 已被移除（adaptive 模式不使用固定预算）
			if gjson.GetBytes(out, "thinking.budget_tokens").Exists() {
				t.Fatalf("thinking.budget_tokens should be removed, body=%s", string(out))
			}
		})
	}
}
