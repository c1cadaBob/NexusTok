// xai - apply_test.go
// xAI 提供者（Grok 系列模型）思考参数应用器的单元测试文件。
// 验证 Applier 能将通用的 ThinkingConfig 正确转换为 xAI 专有的
// reasoning.effort 字段，并处理不支持 "disable" 级别的模型的降级逻辑。
package xai

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/thinking"
	"github.com/tidwall/gjson"
)

// TestApplySetsReasoningEffort 验证当 ThinkingConfig 为 ModeLevel（级别模式）
// 且 Level 为 LevelHigh 时，Applier.Apply 能在请求体中正确设置
// reasoning.effort 字段为 "high"。
// 测试使用支持 ZeroAllowed 的 grok-4.3 模型配置。
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
	// 验证 reasoning.effort 被正确设置为 "high"
	if got := gjson.GetBytes(out, "reasoning.effort").String(); got != "high" {
		t.Fatalf("reasoning.effort = %q, want high; body=%s", got, string(out))
	}
}

// TestApplyNoneFallsBackToLowestLevelWhenDisableUnsupported 验证当模型不支持
// "none"（禁用）级别时，ModeNone（关闭思考）模式会降级使用最低可用级别 "low"。
// 测试使用 grok-3-mini 模型，其支持的级别为 ["low", "medium", "high"]，
// 不包含 "none" 或 "disabled"，因此关闭思考时应降级为 "low"。
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
	// 验证 reasoning.effort 降级为 "low"（最低可用级别）
	if got := gjson.GetBytes(out, "reasoning.effort").String(); got != "low" {
		t.Fatalf("reasoning.effort = %q, want low; body=%s", got, string(out))
	}
}
