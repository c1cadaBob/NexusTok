// xai - apply_test.go
// 测试 xAI 提供商的思维推理配置应用逻辑，验证推理努力级别设置和禁用模式回退行为
package xai

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/thinking"
	"github.com/tidwall/gjson"
)

// TestApplySetsReasoningEffort 测试 Apply 方法正确设置推理努力级别
func TestApplySetsReasoningEffort(t *testing.T) {
	applier := NewApplier()
	modelInfo := &registry.ModelInfo{
		ID: "grok-4.3",
		Thinking: &registry.ThinkingSupport{
			ZeroAllowed: true,
			Levels:      []string{"none", "low", "medium", "high"},
		},
	}

	out, err := applier.Apply([]byte(`{"input":"hello"}`), thinking.ThinkingConfig{
		Mode:  thinking.ModeLevel,
		Level: thinking.LevelHigh,
	}, modelInfo)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if got := gjson.GetBytes(out, "reasoning.effort").String(); got != "high" {
		t.Fatalf("reasoning.effort = %q, want high; body=%s", got, string(out))
	}
}

// TestApplyNoneFallsBackToLowestLevelWhenDisableUnsupported 测试当禁用模式不支持时回退到最低级别
func TestApplyNoneFallsBackToLowestLevelWhenDisableUnsupported(t *testing.T) {
	applier := NewApplier()
	modelInfo := &registry.ModelInfo{
		ID: "grok-3-mini",
		Thinking: &registry.ThinkingSupport{
			Levels: []string{"low", "medium", "high"},
		},
	}

	out, err := applier.Apply([]byte(`{"input":"hello"}`), thinking.ThinkingConfig{
		Mode: thinking.ModeNone,
	}, modelInfo)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if got := gjson.GetBytes(out, "reasoning.effort").String(); got != "low" {
		t.Fatalf("reasoning.effort = %q, want low; body=%s", got, string(out))
	}
}
