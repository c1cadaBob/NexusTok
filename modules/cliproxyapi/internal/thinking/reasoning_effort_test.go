// thinking - reasoning_effort_test.go
// 该文件测试 ExtractReasoningEffort 函数的行为。
// 测试覆盖了后缀优先于请求体、预算转级别、OpenAI Responses 格式支持和缺失配置返回空字符串等场景。

package thinking

import "testing"

// TestExtractReasoningEffortUsesSuffixOverBody 测试后缀中的级别优先于请求体中的 reasoning_effort。
func TestExtractReasoningEffortUsesSuffixOverBody(t *testing.T) {
	got := ExtractReasoningEffort([]byte(`{"reasoning_effort":"low"}`), "openai", "gpt-5.4(high)")
	if got != "high" {
		t.Fatalf("ExtractReasoningEffort() = %q, want %q", got, "high")
	}
}

// TestExtractReasoningEffortConvertsBudgetToLevel 测试 Claude 格式的预算值能正确转换为级别名称。
func TestExtractReasoningEffortConvertsBudgetToLevel(t *testing.T) {
	got := ExtractReasoningEffort([]byte(`{"thinking":{"type":"enabled","budget_tokens":8192}}`), "claude", "claude-sonnet-4-5")
	if got != "medium" {
		t.Fatalf("ExtractReasoningEffort() = %q, want %q", got, "medium")
	}
}

// TestExtractReasoningEffortSupportsOpenAIResponses 测试 OpenAI Responses 格式的 reasoning.effort 字段解析。
func TestExtractReasoningEffortSupportsOpenAIResponses(t *testing.T) {
	got := ExtractReasoningEffort([]byte(`{"reasoning":{"effort":"medium"}}`), "openai-response", "gpt-5.4")
	if got != "medium" {
		t.Fatalf("ExtractReasoningEffort() = %q, want %q", got, "medium")
	}
}

// TestExtractReasoningEffortMissingConfigIsEmpty 测试未提供思考配置时返回空字符串。
func TestExtractReasoningEffortMissingConfigIsEmpty(t *testing.T) {
	got := ExtractReasoningEffort([]byte(`{"messages":[{"role":"user","content":"hi"}]}`), "openai", "gpt-5.4")
	if got != "" {
		t.Fatalf("ExtractReasoningEffort() = %q, want empty", got)
	}
}
