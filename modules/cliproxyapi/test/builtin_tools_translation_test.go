// Package test - builtin_tools_translation_test.go
// 内置工具翻译测试。
// 测试在不同 API 格式之间转换请求时，内置工具（如 web_search）的处理行为。
// 验证 OpenAI 格式到 Codex 格式的转换保留内置工具，
// 以及 OpenAI Responses 格式到 OpenAI Chat Completions 格式的转换忽略内置工具。
package test

import (
	"testing"

	_ "github.com/router-for-me/CLIProxyAPI/v7/internal/translator"

	sdktranslator "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
	"github.com/tidwall/gjson"
)

// TestOpenAIToCodex_PreservesBuiltinTools 验证从 OpenAI 格式转换到 Codex 格式时，
// 内置工具（如 web_search）被正确保留。
// 包括工具类型、搜索上下文大小和工具选择等字段。
func TestOpenAIToCodex_PreservesBuiltinTools(t *testing.T) {
	in := []byte(`{
		"model":"gpt-5",
		"messages":[{"role":"user","content":"hi"}],
		"tools":[{"type":"web_search","search_context_size":"high"}],
		"tool_choice":{"type":"web_search"}
	}`)

	out := sdktranslator.TranslateRequest(sdktranslator.FormatOpenAI, sdktranslator.FormatCodex, "gpt-5", in, false)

	if got := gjson.GetBytes(out, "tools.#").Int(); got != 1 {
		t.Fatalf("expected 1 tool, got %d: %s", got, string(out))
	}
	if got := gjson.GetBytes(out, "tools.0.type").String(); got != "web_search" {
		t.Fatalf("expected tools[0].type=web_search, got %q: %s", got, string(out))
	}
	if got := gjson.GetBytes(out, "tools.0.search_context_size").String(); got != "high" {
		t.Fatalf("expected tools[0].search_context_size=high, got %q: %s", got, string(out))
	}
	if got := gjson.GetBytes(out, "tool_choice.type").String(); got != "web_search" {
		t.Fatalf("expected tool_choice.type=web_search, got %q: %s", got, string(out))
	}
}

// TestOpenAIResponsesToOpenAI_IgnoresBuiltinTools 验证从 OpenAI Responses 格式
// 转换到 OpenAI Chat Completions 格式时，内置工具被正确忽略。
// 因为 Chat Completions API 不支持内置工具类型。
func TestOpenAIResponsesToOpenAI_IgnoresBuiltinTools(t *testing.T) {
	in := []byte(`{
		"model":"gpt-5",
		"input":[{"role":"user","content":[{"type":"input_text","text":"hi"}]}],
		"tools":[{"type":"web_search","search_context_size":"low"}]
	}`)

	out := sdktranslator.TranslateRequest(sdktranslator.FormatOpenAIResponse, sdktranslator.FormatOpenAI, "gpt-5", in, false)

	if got := gjson.GetBytes(out, "tools.#").Int(); got != 0 {
		t.Fatalf("expected 0 tools (builtin tools not supported in Chat Completions), got %d: %s", got, string(out))
	}
}
