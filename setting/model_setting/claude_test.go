// claude_test.go — Claude 模型配置中 WriteHeaders 方法的单元测试
// 职责：验证 ClaudeSettings 的 WriteHeaders 方法能正确地将配置的
// HTTP Header 合并到现有 Header 中，并处理去重逻辑。

package model_setting

import (
	"net/http"
	"testing"
)

// TestClaudeSettingsWriteHeadersMergesConfiguredValuesIntoSingleHeader
// 验证配置的 Header 值能正确追加到已有 Header 中，合并为单个逗号分隔的值
func TestClaudeSettingsWriteHeadersMergesConfiguredValuesIntoSingleHeader(t *testing.T) {
	// 创建包含 anthropic-beta Header 配置的 ClaudeSettings
	settings := &ClaudeSettings{
		HeadersSettings: map[string]map[string][]string{
			"claude-3-7-sonnet-20250219-thinking": {
				"anthropic-beta": {
					"token-efficient-tools-2025-02-19",
				},
			},
		},
	}

	// 预设一个已存在的 Header 值
	headers := http.Header{}
	headers.Set("anthropic-beta", "output-128k-2025-02-19")

	// 执行 Header 写入
	settings.WriteHeaders("claude-3-7-sonnet-20250219-thinking", &headers)

	// 验证 Header 值被正确合并为单个逗号分隔的字符串
	got := headers.Values("anthropic-beta")
	if len(got) != 1 {
		t.Fatalf("expected a single merged header value, got %v", got)
	}
	expected := "output-128k-2025-02-19,token-efficient-tools-2025-02-19"
	if got[0] != expected {
		t.Fatalf("expected merged header %q, got %q", expected, got[0])
	}
}

// TestClaudeSettingsWriteHeadersDeduplicatesAcrossCommaSeparatedAndRepeatedValues
// 验证在合并 Header 时能正确去除重复值，包括跨逗号分隔值和重复 Header 条目的去重
func TestClaudeSettingsWriteHeadersDeduplicatesAcrossCommaSeparatedAndRepeatedValues(t *testing.T) {
	// 创建包含两个 anthropic-beta 值的配置
	settings := &ClaudeSettings{
		HeadersSettings: map[string]map[string][]string{
			"claude-3-7-sonnet-20250219-thinking": {
				"anthropic-beta": {
					"token-efficient-tools-2025-02-19",
					"computer-use-2025-01-24",
				},
			},
		},
	}

	// 预设包含重复值的 Header
	headers := http.Header{}
	headers.Add("anthropic-beta", "output-128k-2025-02-19, token-efficient-tools-2025-02-19")
	headers.Add("anthropic-beta", "token-efficient-tools-2025-02-19")

	settings.WriteHeaders("claude-3-7-sonnet-20250219-thinking", &headers)

	// 验证重复值被去重，结果为单个合并后的 Header
	got := headers.Values("anthropic-beta")
	if len(got) != 1 {
		t.Fatalf("expected duplicate values to collapse into one header, got %v", got)
	}
	expected := "output-128k-2025-02-19,token-efficient-tools-2025-02-19,computer-use-2025-01-24"
	if got[0] != expected {
		t.Fatalf("expected deduplicated merged header %q, got %q", expected, got[0])
	}
}
