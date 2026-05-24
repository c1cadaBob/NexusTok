// amp - model_mapping_test.go
// 模型映射器（ModelMapper）的单元测试。
// 测试以下功能：
// - 映射器的创建和初始化
// - 模型映射的正确性（有/无注册提供商）
// - 大小写不敏感匹配
// - 未知模型和空输入的处理
// - 映射更新和替换行为
// - 无效映射的跳过
// - GetMappings 返回副本的不可变性
// - 正则表达式映射：基础匹配、优先级、无效模式、大小写不敏感
// - 思维后缀保留：数字后缀、级别后缀、配置后缀优先级、正则后缀保留
package amp

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
)

// TestNewModelMapper 测试创建映射器并验证映射数量正确
func TestNewModelMapper(t *testing.T) {
	mappings := []config.AmpModelMapping{
		{From: "claude-opus-4.5", To: "claude-sonnet-4"},
		{From: "gpt-5", To: "gemini-2.5-pro"},
	}

	mapper := NewModelMapper(mappings)
	if mapper == nil {
		t.Fatal("Expected non-nil mapper")
	}

	result := mapper.GetMappings()
	if len(result) != 2 {
		t.Errorf("Expected 2 mappings, got %d", len(result))
	}
}

// TestNewModelMapper_Empty 测试空映射创建
func TestNewModelMapper_Empty(t *testing.T) {
	mapper := NewModelMapper(nil)
	if mapper == nil {
		t.Fatal("Expected non-nil mapper")
	}

	result := mapper.GetMappings()
	if len(result) != 0 {
		t.Errorf("Expected 0 mappings, got %d", len(result))
	}
}

// TestModelMapper_MapModel_NoProvider 测试当目标模型没有注册的提供商时，映射返回空字符串
func TestModelMapper_MapModel_NoProvider(t *testing.T) {
	mappings := []config.AmpModelMapping{
		{From: "claude-opus-4.5", To: "claude-sonnet-4"},
	}

	mapper := NewModelMapper(mappings)

	// Without a registered provider for the target, mapping should return empty
	result := mapper.MapModel("claude-opus-4.5")
	if result != "" {
		t.Errorf("Expected empty result when target has no provider, got %s", result)
	}
}

// TestModelMapper_MapModel_WithProvider 测试当目标模型有注册的提供商时，映射正确工作
func TestModelMapper_MapModel_WithProvider(t *testing.T) {
	// Register a mock provider for the target model
	reg := registry.GetGlobalRegistry()
	reg.RegisterClient("test-client", "claude", []*registry.ModelInfo{
		{ID: "claude-sonnet-4", OwnedBy: "anthropic", Type: "claude"},
	})
	defer reg.UnregisterClient("test-client")

	mappings := []config.AmpModelMapping{
		{From: "claude-opus-4.5", To: "claude-sonnet-4"},
	}

	mapper := NewModelMapper(mappings)

	// With a registered provider, mapping should work
	result := mapper.MapModel("claude-opus-4.5")
	if result != "claude-sonnet-4" {
		t.Errorf("Expected claude-sonnet-4, got %s", result)
	}
}

// TestModelMapper_MapModel_TargetWithThinkingSuffix 测试目标模型包含思维后缀时的映射
func TestModelMapper_MapModel_TargetWithThinkingSuffix(t *testing.T) {
	reg := registry.GetGlobalRegistry()
	reg.RegisterClient("test-client-thinking", "codex", []*registry.ModelInfo{
		{ID: "gpt-5.2", OwnedBy: "openai", Type: "codex"},
	})
	defer reg.UnregisterClient("test-client-thinking")

	mappings := []config.AmpModelMapping{
		{From: "gpt-5.2-alias", To: "gpt-5.2(xhigh)"},
	}

	mapper := NewModelMapper(mappings)

	result := mapper.MapModel("gpt-5.2-alias")
	if result != "gpt-5.2(xhigh)" {
		t.Errorf("Expected gpt-5.2(xhigh), got %s", result)
	}
}

// TestModelMapper_MapModel_CaseInsensitive 测试大小写不敏感的模型匹配
func TestModelMapper_MapModel_CaseInsensitive(t *testing.T) {
	reg := registry.GetGlobalRegistry()
	reg.RegisterClient("test-client2", "claude", []*registry.ModelInfo{
		{ID: "claude-sonnet-4", OwnedBy: "anthropic", Type: "claude"},
	})
	defer reg.UnregisterClient("test-client2")

	mappings := []config.AmpModelMapping{
		{From: "Claude-Opus-4.5", To: "claude-sonnet-4"},
	}

	mapper := NewModelMapper(mappings)

	// Should match case-insensitively
	result := mapper.MapModel("claude-opus-4.5")
	if result != "claude-sonnet-4" {
		t.Errorf("Expected claude-sonnet-4, got %s", result)
	}
}

// TestModelMapper_MapModel_NotFound 测试未知模型返回空字符串
func TestModelMapper_MapModel_NotFound(t *testing.T) {
	mappings := []config.AmpModelMapping{
		{From: "claude-opus-4.5", To: "claude-sonnet-4"},
	}

	mapper := NewModelMapper(mappings)

	// Unknown model should return empty
	result := mapper.MapModel("unknown-model")
	if result != "" {
		t.Errorf("Expected empty for unknown model, got %s", result)
	}
}

// TestModelMapper_MapModel_EmptyInput 测试空输入返回空字符串
func TestModelMapper_MapModel_EmptyInput(t *testing.T) {
	mappings := []config.AmpModelMapping{
		{From: "claude-opus-4.5", To: "claude-sonnet-4"},
	}

	mapper := NewModelMapper(mappings)

	result := mapper.MapModel("")
	if result != "" {
		t.Errorf("Expected empty for empty input, got %s", result)
	}
}

// TestModelMapper_UpdateMappings 测试映射更新：替换而非追加
func TestModelMapper_UpdateMappings(t *testing.T) {
	mapper := NewModelMapper(nil)

	// Initially empty
	if len(mapper.GetMappings()) != 0 {
		t.Error("Expected 0 initial mappings")
	}

	// Update with new mappings
	mapper.UpdateMappings([]config.AmpModelMapping{
		{From: "model-a", To: "model-b"},
		{From: "model-c", To: "model-d"},
	})

	result := mapper.GetMappings()
	if len(result) != 2 {
		t.Errorf("Expected 2 mappings after update, got %d", len(result))
	}

	// Update again should replace, not append
	mapper.UpdateMappings([]config.AmpModelMapping{
		{From: "model-x", To: "model-y"},
	})

	result = mapper.GetMappings()
	if len(result) != 1 {
		t.Errorf("Expected 1 mapping after second update, got %d", len(result))
	}
}

// TestModelMapper_UpdateMappings_SkipsInvalid 测试无效映射（空 from/to、空白 from）被跳过
func TestModelMapper_UpdateMappings_SkipsInvalid(t *testing.T) {
	mapper := NewModelMapper(nil)

	mapper.UpdateMappings([]config.AmpModelMapping{
		{From: "", To: "model-b"},        // Invalid: empty from
		{From: "model-a", To: ""},        // Invalid: empty to
		{From: "  ", To: "model-b"},      // Invalid: whitespace from
		{From: "model-c", To: "model-d"}, // Valid
	})

	result := mapper.GetMappings()
	if len(result) != 1 {
		t.Errorf("Expected 1 valid mapping, got %d", len(result))
	}
}

// TestModelMapper_GetMappings_ReturnsCopy 测试 GetMappings 返回的映射是副本，
// 修改返回值不影响原始映射
func TestModelMapper_GetMappings_ReturnsCopy(t *testing.T) {
	mappings := []config.AmpModelMapping{
		{From: "model-a", To: "model-b"},
	}

	mapper := NewModelMapper(mappings)

	// Get mappings and modify the returned map
	result := mapper.GetMappings()
	result["new-key"] = "new-value"

	// Original should be unchanged
	original := mapper.GetMappings()
	if len(original) != 1 {
		t.Errorf("Expected original to have 1 mapping, got %d", len(original))
	}
	if _, exists := original["new-key"]; exists {
		t.Error("Original map was modified")
	}
}

// TestModelMapper_Regex_MatchBaseWithoutParens 测试正则映射时保留思维后缀：
// 正则匹配基础模型名，后缀 (high) 被保留到目标模型
func TestModelMapper_Regex_MatchBaseWithoutParens(t *testing.T) {
	reg := registry.GetGlobalRegistry()
	reg.RegisterClient("test-client-regex-1", "gemini", []*registry.ModelInfo{
		{ID: "gemini-2.5-pro", OwnedBy: "google", Type: "gemini"},
	})
	defer reg.UnregisterClient("test-client-regex-1")

	mappings := []config.AmpModelMapping{
		{From: "^gpt-5$", To: "gemini-2.5-pro", Regex: true},
	}

	mapper := NewModelMapper(mappings)

	// Incoming model has reasoning suffix, regex matches base, suffix is preserved
	result := mapper.MapModel("gpt-5(high)")
	if result != "gemini-2.5-pro(high)" {
		t.Errorf("Expected gemini-2.5-pro(high), got %s", result)
	}
}

// TestModelMapper_Regex_ExactPrecedence 测试精确匹配优先于正则匹配
func TestModelMapper_Regex_ExactPrecedence(t *testing.T) {
	reg := registry.GetGlobalRegistry()
	reg.RegisterClient("test-client-regex-2", "claude", []*registry.ModelInfo{
		{ID: "claude-sonnet-4", OwnedBy: "anthropic", Type: "claude"},
	})
	reg.RegisterClient("test-client-regex-3", "gemini", []*registry.ModelInfo{
		{ID: "gemini-2.5-pro", OwnedBy: "google", Type: "gemini"},
	})
	defer reg.UnregisterClient("test-client-regex-2")
	defer reg.UnregisterClient("test-client-regex-3")

	mappings := []config.AmpModelMapping{
		{From: "gpt-5", To: "claude-sonnet-4"},                 // exact
		{From: "^gpt-5.*$", To: "gemini-2.5-pro", Regex: true}, // regex
	}

	mapper := NewModelMapper(mappings)

	// Exact match should win over regex
	result := mapper.MapModel("gpt-5")
	if result != "claude-sonnet-4" {
		t.Errorf("Expected claude-sonnet-4, got %s", result)
	}
}

// TestModelMapper_Regex_InvalidPattern_Skipped 测试无效正则模式被跳过，不会 panic
func TestModelMapper_Regex_InvalidPattern_Skipped(t *testing.T) {
	// Invalid regex should be skipped and not cause panic
	mappings := []config.AmpModelMapping{
		{From: "(", To: "target", Regex: true},
	}

	mapper := NewModelMapper(mappings)

	result := mapper.MapModel("anything")
	if result != "" {
		t.Errorf("Expected empty result due to invalid regex, got %s", result)
	}
}

// TestModelMapper_Regex_CaseInsensitive 测试正则映射的大小写不敏感匹配
func TestModelMapper_Regex_CaseInsensitive(t *testing.T) {
	reg := registry.GetGlobalRegistry()
	reg.RegisterClient("test-client-regex-4", "claude", []*registry.ModelInfo{
		{ID: "claude-sonnet-4", OwnedBy: "anthropic", Type: "claude"},
	})
	defer reg.UnregisterClient("test-client-regex-4")

	mappings := []config.AmpModelMapping{
		{From: "^CLAUDE-OPUS-.*$", To: "claude-sonnet-4", Regex: true},
	}

	mapper := NewModelMapper(mappings)

	result := mapper.MapModel("claude-opus-4.5")
	if result != "claude-sonnet-4" {
		t.Errorf("Expected claude-sonnet-4, got %s", result)
	}
}

// TestModelMapper_SuffixPreservation 测试各种思维后缀保留场景：
// - 数字后缀 (8192) 保留
// - 级别后缀 (high) 保留
// - 无后缀不变
// - 配置后缀优先于请求后缀
// - 正则映射时后缀保留
// - auto/none 后缀保留
// - 大小写不敏感基础查找时后缀保留
// - 空后缀 () 被过滤
// - 不完整后缀视为无后缀
func TestModelMapper_SuffixPreservation(t *testing.T) {
	reg := registry.GetGlobalRegistry()

	// Register test models
	reg.RegisterClient("test-client-suffix", "gemini", []*registry.ModelInfo{
		{ID: "gemini-2.5-pro", OwnedBy: "google", Type: "gemini"},
	})
	reg.RegisterClient("test-client-suffix-2", "claude", []*registry.ModelInfo{
		{ID: "claude-sonnet-4", OwnedBy: "anthropic", Type: "claude"},
	})
	defer reg.UnregisterClient("test-client-suffix")
	defer reg.UnregisterClient("test-client-suffix-2")

	tests := []struct {
		name     string
		mappings []config.AmpModelMapping
		input    string
		want     string
	}{
		{
			name:     "numeric suffix preserved",
			mappings: []config.AmpModelMapping{{From: "g25p", To: "gemini-2.5-pro"}},
			input:    "g25p(8192)",
			want:     "gemini-2.5-pro(8192)",
		},
		{
			name:     "level suffix preserved",
			mappings: []config.AmpModelMapping{{From: "g25p", To: "gemini-2.5-pro"}},
			input:    "g25p(high)",
			want:     "gemini-2.5-pro(high)",
		},
		{
			name:     "no suffix unchanged",
			mappings: []config.AmpModelMapping{{From: "g25p", To: "gemini-2.5-pro"}},
			input:    "g25p",
			want:     "gemini-2.5-pro",
		},
		{
			name:     "config suffix takes priority",
			mappings: []config.AmpModelMapping{{From: "alias", To: "gemini-2.5-pro(medium)"}},
			input:    "alias(high)",
			want:     "gemini-2.5-pro(medium)",
		},
		{
			name:     "regex with suffix preserved",
			mappings: []config.AmpModelMapping{{From: "^g25.*", To: "gemini-2.5-pro", Regex: true}},
			input:    "g25p(8192)",
			want:     "gemini-2.5-pro(8192)",
		},
		{
			name:     "auto suffix preserved",
			mappings: []config.AmpModelMapping{{From: "g25p", To: "gemini-2.5-pro"}},
			input:    "g25p(auto)",
			want:     "gemini-2.5-pro(auto)",
		},
		{
			name:     "none suffix preserved",
			mappings: []config.AmpModelMapping{{From: "g25p", To: "gemini-2.5-pro"}},
			input:    "g25p(none)",
			want:     "gemini-2.5-pro(none)",
		},
		{
			name:     "case insensitive base lookup with suffix",
			mappings: []config.AmpModelMapping{{From: "G25P", To: "gemini-2.5-pro"}},
			input:    "g25p(high)",
			want:     "gemini-2.5-pro(high)",
		},
		{
			name:     "empty suffix filtered out",
			mappings: []config.AmpModelMapping{{From: "g25p", To: "gemini-2.5-pro"}},
			input:    "g25p()",
			want:     "gemini-2.5-pro",
		},
		{
			name:     "incomplete suffix treated as no suffix",
			mappings: []config.AmpModelMapping{{From: "g25p(high", To: "gemini-2.5-pro"}},
			input:    "g25p(high",
			want:     "gemini-2.5-pro",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapper := NewModelMapper(tt.mappings)
			got := mapper.MapModel(tt.input)
			if got != tt.want {
				t.Errorf("MapModel(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
