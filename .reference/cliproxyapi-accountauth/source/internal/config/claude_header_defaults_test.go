// config - claude_header_defaults_test.go
// Claude 请求头默认值配置测试
// 验证 LoadConfigOptional 函数能够正确解析 claude-header-defaults 配置节，
// 包括 User-Agent、版本号、操作系统、架构、超时时间等字段，
// 并对所有字符串值进行首尾空格清理。
package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadConfigOptional_ClaudeHeaderDefaults 测试 Claude 请求头默认值的加载：
// 1. 从 YAML 配置文件中读取 claude-header-defaults 节
// 2. 验证各字符串字段的首尾空格被正确去除
// 3. 验证 StabilizeDeviceProfile 指针类型字段的解析（false 值）
func TestLoadConfigOptional_ClaudeHeaderDefaults(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	configYAML := []byte(`
claude-header-defaults:
  user-agent: "  claude-cli/2.1.70 (external, cli)  "
  package-version: "  0.80.0  "
  runtime-version: "  v24.5.0  "
  os: "  MacOS  "
  arch: "  arm64  "
  timeout: "  900  "
  stabilize-device-profile: false
`)
	if err := os.WriteFile(configPath, configYAML, 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := LoadConfigOptional(configPath, false)
	if err != nil {
		t.Fatalf("LoadConfigOptional() error = %v", err)
	}

	if got := cfg.ClaudeHeaderDefaults.UserAgent; got != "claude-cli/2.1.70 (external, cli)" {
		t.Fatalf("UserAgent = %q, want %q", got, "claude-cli/2.1.70 (external, cli)")
	}
	if got := cfg.ClaudeHeaderDefaults.PackageVersion; got != "0.80.0" {
		t.Fatalf("PackageVersion = %q, want %q", got, "0.80.0")
	}
	if got := cfg.ClaudeHeaderDefaults.RuntimeVersion; got != "v24.5.0" {
		t.Fatalf("RuntimeVersion = %q, want %q", got, "v24.5.0")
	}
	if got := cfg.ClaudeHeaderDefaults.OS; got != "MacOS" {
		t.Fatalf("OS = %q, want %q", got, "MacOS")
	}
	if got := cfg.ClaudeHeaderDefaults.Arch; got != "arm64" {
		t.Fatalf("Arch = %q, want %q", got, "arm64")
	}
	if got := cfg.ClaudeHeaderDefaults.Timeout; got != "900" {
		t.Fatalf("Timeout = %q, want %q", got, "900")
	}
	if cfg.ClaudeHeaderDefaults.StabilizeDeviceProfile == nil {
		t.Fatal("StabilizeDeviceProfile = nil, want non-nil")
	}
	if got := *cfg.ClaudeHeaderDefaults.StabilizeDeviceProfile; got {
		t.Fatalf("StabilizeDeviceProfile = %v, want false", got)
	}
}
