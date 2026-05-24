// thinking - reasoning_effort_test.go
// 推理努力级别（reasoning effort）提取功能的单元测试文件。
// 验证 ExtractReasoningEffort 函数能从请求体 JSON 和模型名称后缀中
// 正确提取推理努力级别，并支持多种上游协议格式（OpenAI、Claude、OpenAI Responses）。
package thinking

import "testing"

// TestExtractReasoningEffortUsesSuffixOverBody 验证当请求体 JSON 和模型名称后缀
// 同时指定了不同的 reasoning_effort 时，模型名称后缀的优先级更高。
// 例如：body 中为 "low"，模型名为 "gpt-5.4(high)"，应返回 "high"。
func TestExtractReasoningEffortUsesSuffixOverBody(t *testing.T) {
	got := ExtractReasoningEffort([]byte(`{"reasoning_effort":"low"}`), "openai", "gpt-5.4(high)")
	if got != "high" {
		t.Fatalf("ExtractReasoningEffort() = %q, want %q", got, "high")
	}
}

// TestExtractReasoningEffortConvertsBudgetToLevel 验证对于 Claude 格式的请求体，
// ExtractReasoningEffort 能将 thinking.budget_tokens 数值转换为对应的努力级别。
// 例如：budget_tokens 为 8192 应映射为 "medium" 级别。
func TestExtractReasoningEffortConvertsBudgetToLevel(t *testing.T) {
	got := ExtractReasoningEffort([]byte(`{"thinking":{"type":"enabled","budget_tokens":8192}}`), "claude", "claude-sonnet-4-5")
	if got != "medium" {
		t.Fatalf("ExtractReasoningEffort() = %q, want %q", got, "medium")
	}
}

// TestExtractReasoningEffortSupportsOpenAIResponses 验证 ExtractReasoningEffort
// 支持 OpenAI Responses 格式的请求体，即从 reasoning.effort 字段提取努力级别。
func TestExtractReasoningEffortSupportsOpenAIResponses(t *testing.T) {
	got := ExtractReasoningEffort([]byte(`{"reasoning":{"effort":"medium"}}`), "openai-response", "gpt-5.4")
	if got != "medium" {
		t.Fatalf("ExtractReasoningEffort() = %q, want %q", got, "medium")
	}
}

// TestExtractReasoningEffortMissingConfigIsEmpty 验证当请求体中不包含任何推理
// 相关配置，且模型名称后缀也未指定努力级别时，ExtractReasoningEffort 返回空字符串。
func TestExtractReasoningEffortMissingConfigIsEmpty(t *testing.T) {
	got := ExtractReasoningEffort([]byte(`{"messages":[{"role":"user","content":"hi"}]}`), "openai", "gpt-5.4")
	if got != "" {
		t.Fatalf("ExtractReasoningEffort() = %q, want empty", got)
	}
}
