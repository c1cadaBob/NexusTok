// config_test.go — 配置更新函数 UpdateConfigFromMap 的单元测试
// 职责：验证配置更新逻辑的正确性，包括 Map 类型字段的替换、
// 空 Map 清空所有条目、以及标量字段的正常更新等功能。

package config

import (
	"testing"
)

// testConfigWithMap 用于测试的配置结构体，包含 Map 类型字段和标量字段
type testConfigWithMap struct {
	// Modes 存储模型名到计费模式的映射
	Modes map[string]string `json:"modes"`
	// Exprs 存储模型名到计费表达式的映射
	Exprs map[string]string `json:"exprs"`
	// Name 是一个标量字段，用于测试标量更新逻辑
	Name string `json:"name"`
}

// TestUpdateConfigFromMap_MapReplacement 验证 Map 字段的替换语义：
// 更新时只包含指定键，则其他键应被移除（整体替换而非合并）
func TestUpdateConfigFromMap_MapReplacement(t *testing.T) {
	cfg := &testConfigWithMap{
		Modes: map[string]string{
			"model-a": "tiered_expr",
			"model-b": "tiered_expr",
		},
		Exprs: map[string]string{
			"model-a": "p * 5 + c * 25",
			"model-b": "p * 10 + c * 50",
		},
		Name: "billing",
	}

	// 模拟移除 model-a：新的值只包含 model-b
	err := UpdateConfigFromMap(cfg, map[string]string{
		"modes": `{"model-b": "tiered_expr"}`,
		"exprs": `{"model-b": "p * 10 + c * 50"}`,
	})
	if err != nil {
		t.Fatalf("UpdateConfigFromMap failed: %v", err)
	}

	// 验证 model-a 已被移除
	if _, ok := cfg.Modes["model-a"]; ok {
		t.Errorf("Modes still contains model-a after it was removed from the update; got %v", cfg.Modes)
	}
	if _, ok := cfg.Exprs["model-a"]; ok {
		t.Errorf("Exprs still contains model-a after it was removed from the update; got %v", cfg.Exprs)
	}

	// 验证 model-b 的值正确保留
	if cfg.Modes["model-b"] != "tiered_expr" {
		t.Errorf("Modes[model-b] = %q, want %q", cfg.Modes["model-b"], "tiered_expr")
	}
	if cfg.Exprs["model-b"] != "p * 10 + c * 50" {
		t.Errorf("Exprs[model-b] = %q, want %q", cfg.Exprs["model-b"], "p * 10 + c * 50")
	}
}

// TestUpdateConfigFromMap_EmptyMapClearsAll 验证使用空 Map 更新时，
// 原有 Map 字段的所有条目应被清空
func TestUpdateConfigFromMap_EmptyMapClearsAll(t *testing.T) {
	cfg := &testConfigWithMap{
		Modes: map[string]string{
			"model-a": "tiered_expr",
		},
		Exprs: map[string]string{
			"model-a": "p * 5 + c * 25",
		},
	}

	err := UpdateConfigFromMap(cfg, map[string]string{
		"modes": `{}`,
		"exprs": `{}`,
	})
	if err != nil {
		t.Fatalf("UpdateConfigFromMap failed: %v", err)
	}

	// 验证 Map 已被清空
	if len(cfg.Modes) != 0 {
		t.Errorf("Modes should be empty after updating with {}, got %v", cfg.Modes)
	}
	if len(cfg.Exprs) != 0 {
		t.Errorf("Exprs should be empty after updating with {}, got %v", cfg.Exprs)
	}
}

// TestUpdateConfigFromMap_ScalarFieldsUnchanged 验证标量字段的正常更新，
// 以及未包含在更新 Map 中的字段应保持不变
func TestUpdateConfigFromMap_ScalarFieldsUnchanged(t *testing.T) {
	cfg := &testConfigWithMap{
		Modes: map[string]string{"m": "v"},
		Name:  "old",
	}

	err := UpdateConfigFromMap(cfg, map[string]string{
		"name": "new",
	})
	if err != nil {
		t.Fatalf("UpdateConfigFromMap failed: %v", err)
	}

	// 验证标量字段被正确更新
	if cfg.Name != "new" {
		t.Errorf("Name = %q, want %q", cfg.Name, "new")
	}
	// modes 未在 configMap 中，应保持不变
	if cfg.Modes["m"] != "v" {
		t.Errorf("Modes should be unchanged, got %v", cfg.Modes)
	}
}
