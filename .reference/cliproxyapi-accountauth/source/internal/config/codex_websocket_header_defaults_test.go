// config - codex_websocket_header_defaults_test.go
// Codex WebSocket 请求头默认值配置测试
// 验证 LoadConfigOptional 函数能够正确解析 codex-header-defaults 配置节，
// 并对配置值进行首尾空格清理（trim）。
package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadConfigOptional_CodexHeaderDefaults 测试 Codex 请求头默认值的加载：
// 1. 从 YAML 配置文件中读取 codex-header-defaults 节
// 2. 验证 user-agent 字段的首尾空格被正确去除
// 3. 验证 beta-features 字段的首尾空格被正确去除
func TestLoadConfigOptional_CodexHeaderDefaults(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	configYAML := []byte(`
codex-header-defaults:
  user-agent: "  my-codex-client/1.0  "
  beta-features: "  feature-a,feature-b  "
`)
	if err := os.WriteFile(configPath, configYAML, 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := LoadConfigOptional(configPath, false)
	if err != nil {
		t.Fatalf("LoadConfigOptional() error = %v", err)
	}

	if got := cfg.CodexHeaderDefaults.UserAgent; got != "my-codex-client/1.0" {
		t.Fatalf("UserAgent = %q, want %q", got, "my-codex-client/1.0")
	}
	if got := cfg.CodexHeaderDefaults.BetaFeatures; got != "feature-a,feature-b" {
		t.Fatalf("BetaFeatures = %q, want %q", got, "feature-a,feature-b")
	}
}
