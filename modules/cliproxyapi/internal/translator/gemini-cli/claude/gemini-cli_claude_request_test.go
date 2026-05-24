// gemini-cli/claude - gemini-cli_claude_request_test.go
// Claude Code API 请求到 Gemini CLI 格式转换的单元测试。
// 验证 tool_choice 中特定工具选择（type="tool"）能否正确映射为
// Gemini CLI 的 functionCallingConfig.mode="ANY" 以及 allowedFunctionNames。
// 同时测试 Claude Code 归属头信息（x-anthropic-billing-header）的自动剥离。
package claude

import (
	"testing"

	"github.com/tidwall/gjson"
)

// TestConvertClaudeRequestToCLI_ToolChoice_SpecificTool 测试当 Claude 请求的 tool_choice
// 指定特定工具（type="tool", name="json"）时，转换后的 Gemini CLI 请求应包含：
// - functionCallingConfig.mode 为 "ANY"
// - allowedFunctionNames 仅包含指定的工具名称 "json"
func TestConvertClaudeRequestToCLI_ToolChoice_SpecificTool(t *testing.T) {
	inputJSON := []byte(`{
		"model": "gemini-3-flash-preview",
		"messages": [
			{
				"role": "user",
				"content": [
					{"type": "text", "text": "hi"}
				]
			}
		],
		"tools": [
			{
				"name": "json",
				"description": "A JSON tool",
				"input_schema": {
					"type": "object",
					"properties": {}
				}
			}
		],
		"tool_choice": {"type": "tool", "name": "json"}
	}`)

	output := ConvertClaudeRequestToCLI("gemini-3-flash-preview", inputJSON, false)

	if got := gjson.GetBytes(output, "request.toolConfig.functionCallingConfig.mode").String(); got != "ANY" {
		t.Fatalf("Expected request.toolConfig.functionCallingConfig.mode 'ANY', got '%s'", got)
	}
	allowed := gjson.GetBytes(output, "request.toolConfig.functionCallingConfig.allowedFunctionNames").Array()
	if len(allowed) != 1 || allowed[0].String() != "json" {
		t.Fatalf("Expected allowedFunctionNames ['json'], got %s", gjson.GetBytes(output, "request.toolConfig.functionCallingConfig.allowedFunctionNames").Raw)
	}
}

// TestConvertClaudeRequestToCLI_StripsClaudeCodeAttribution 测试 Claude Code 归属头信息
// （以 x-anthropic-billing-header: 开头的系统文本）会被自动剥离，
// 仅保留用户实际编写的系统提示词。
func TestConvertClaudeRequestToCLI_StripsClaudeCodeAttribution(t *testing.T) {
	inputJSON := []byte(`{
		"model": "claude-sonnet-4-5",
		"system": [
			{"type": "text", "text": "x-anthropic-billing-header: cc_version=2.1.63.abc; cc_entrypoint=cli; cch=12345;"},
			{"type": "text", "text": "User system prompt"}
		],
		"messages": [{"role": "user", "content": [{"type": "text", "text": "hi"}]}]
	}`)

	output := ConvertClaudeRequestToCLI("gemini-3-flash-preview", inputJSON, false)

	parts := gjson.GetBytes(output, "request.systemInstruction.parts").Array()
	if len(parts) != 1 {
		t.Fatalf("Expected 1 system part after attribution strip, got %d: %s", len(parts), gjson.GetBytes(output, "request.systemInstruction.parts").Raw)
	}
	if got := parts[0].Get("text").String(); got != "User system prompt" {
		t.Fatalf("Unexpected system part: %q", got)
	}
}
