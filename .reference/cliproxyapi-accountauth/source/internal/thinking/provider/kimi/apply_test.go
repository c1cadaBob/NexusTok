// kimi - apply_test.go
// Kimi 提供者（月之暗面 K 系列模型）思考参数应用器的单元测试文件。
// 验证 Applier 能将通用的 ThinkingConfig 正确转换为 Kimi 专有的参数格式：
// - ModeNone：设置 thinking.type = "disabled" 并移除 reasoning_effort
// - ModeLevel：设置 reasoning_effort 并移除 thinking 对象
// 同时覆盖用户自定义模型的处理逻辑。
package kimi

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/thinking"
	"github.com/tidwall/gjson"
)

// TestApply_ModeNone_UsesDisabledThinking 验证当 ThinkingConfig 为 ModeNone
// （关闭思考）时，Applier.Apply 的行为：
// 1. 将 thinking.type 设置为 "disabled"
// 2. 移除 thinking.budget_tokens（禁用时无需预算）
// 3. 移除 reasoning_effort（ModeNone 不使用努力级别）
// 即使请求体中原本包含 thinking 和 reasoning_effort 配置，也会被覆盖/移除。
func TestApply_ModeNone_UsesDisabledThinking(t *testing.T) {
	applier := NewApplier()
	modelInfo := &registry.ModelInfo{
		ID:       "kimi-k2.5",
		Thinking: &registry.ThinkingSupport{Min: 1024, Max: 32000, ZeroAllowed: true, DynamicAllowed: true},
	}
	body := []byte(`{"model":"kimi-k2.5","reasoning_effort":"none","thinking":{"type":"enabled","budget_tokens":2048}}`)

	out, errApply := applier.Apply(body, thinking.ThinkingConfig{Mode: thinking.ModeNone}, modelInfo)
	if errApply != nil {
		t.Fatalf("Apply() error = %v", errApply)
	}
	// 验证 thinking.type 被设置为 "disabled"
	if got := gjson.GetBytes(out, "thinking.type").String(); got != "disabled" {
		t.Fatalf("thinking.type = %q, want %q, body=%s", got, "disabled", string(out))
	}
	// 验证 budget_tokens 已被移除
	if gjson.GetBytes(out, "thinking.budget_tokens").Exists() {
		t.Fatalf("thinking.budget_tokens should be removed, body=%s", string(out))
	}
	// 验证 reasoning_effort 已被移除
	if gjson.GetBytes(out, "reasoning_effort").Exists() {
		t.Fatalf("reasoning_effort should be removed in ModeNone, body=%s", string(out))
	}
}

// TestApply_ModeLevel_UsesReasoningEffort 验证当 ThinkingConfig 为 ModeLevel
// （级别模式）且 Level 为 LevelHigh 时，Applier.Apply 的行为：
// 1. 设置 reasoning_effort 为 "high"
// 2. 移除 thinking 对象（Kimi 在使用 reasoning_effort 时不同时使用 thinking 对象）
// 即使请求体中原本包含 thinking 配置，也会被移除。
func TestApply_ModeLevel_UsesReasoningEffort(t *testing.T) {
	applier := NewApplier()
	modelInfo := &registry.ModelInfo{
		ID:       "kimi-k2.5",
		Thinking: &registry.ThinkingSupport{Min: 1024, Max: 32000, ZeroAllowed: true, DynamicAllowed: true},
	}
	body := []byte(`{"model":"kimi-k2.5","thinking":{"type":"disabled"}}`)

	out, errApply := applier.Apply(body, thinking.ThinkingConfig{Mode: thinking.ModeLevel, Level: thinking.LevelHigh}, modelInfo)
	if errApply != nil {
		t.Fatalf("Apply() error = %v", errApply)
	}
	// 验证 reasoning_effort 被设置为 "high"
	if got := gjson.GetBytes(out, "reasoning_effort").String(); got != "high" {
		t.Fatalf("reasoning_effort = %q, want %q, body=%s", got, "high", string(out))
	}
	// 验证 thinking 对象已被移除
	if gjson.GetBytes(out, "thinking").Exists() {
		t.Fatalf("thinking should be removed when reasoning_effort is used, body=%s", string(out))
	}
}

// TestApply_UserDefinedModeNone_UsesDisabledThinking 验证当模型为用户自定义模型
// （UserDefined = true）且 ThinkingConfig 为 ModeNone 时，Applier.Apply 仍能
// 正确处理：设置 thinking.type = "disabled" 并移除 reasoning_effort。
// 用户自定义模型没有预定义的 ThinkingSupport 配置，需要走单独的处理路径。
func TestApply_UserDefinedModeNone_UsesDisabledThinking(t *testing.T) {
	applier := NewApplier()
	modelInfo := &registry.ModelInfo{
		ID:          "custom-kimi-model",
		UserDefined: true,
	}
	body := []byte(`{"model":"custom-kimi-model","reasoning_effort":"none"}`)

	out, errApply := applier.Apply(body, thinking.ThinkingConfig{Mode: thinking.ModeNone}, modelInfo)
	if errApply != nil {
		t.Fatalf("Apply() error = %v", errApply)
	}
	// 验证 thinking.type 被设置为 "disabled"
	if got := gjson.GetBytes(out, "thinking.type").String(); got != "disabled" {
		t.Fatalf("thinking.type = %q, want %q, body=%s", got, "disabled", string(out))
	}
	// 验证 reasoning_effort 已被移除
	if gjson.GetBytes(out, "reasoning_effort").Exists() {
		t.Fatalf("reasoning_effort should be removed in ModeNone, body=%s", string(out))
	}
}
