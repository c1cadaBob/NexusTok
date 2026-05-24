// Package geminiCLI - codex_gemini-cli_request_test.go
// 测试 Gemini CLI 到 Codex 请求格式转换功能。
// 覆盖 JSON Schema 中名为 "type" 的属性字段的保留测试，
// 确保在 schema 转换过程中不会将属性名 "type" 误识别为类型声明。
package geminiCLI

import (
	"testing"

	"github.com/tidwall/gjson"
)

// TestConvertGeminiCLIRequestToCodex_PreservesSchemaPropertyNamedType 测试
// JSON Schema 中名为 "type" 的属性字段在转换后应被保留为完整的对象结构，
// 而不是被误识别为类型声明字符串
func TestConvertGeminiCLIRequestToCodex_PreservesSchemaPropertyNamedType(t *testing.T) {
	input := []byte(`{
		"request": {
			"tools": [
				{
					"functionDeclarations": [
						{
							"name": "ask_user",
							"description": "Ask the user one or more questions.",
							"parametersJsonSchema": {
								"type": "object",
								"properties": {
									"questions": {
										"type": "array",
										"items": {
											"type": "object",
											"properties": {
												"header": {
													"type": "string"
												},
												"type": {
													"default": "choice",
													"description": "Question type.",
													"enum": [
														"choice",
														"text",
														"yesno"
													],
													"type": "string"
												}
											},
											"required": [
												"question",
												"header",
												"type"
											]
										}
									}
								},
								"required": [
									"questions"
								]
							}
						}
					]
				}
			]
		}
	}`)

	out := ConvertGeminiCLIRequestToCodex("gpt-5.2", input, true)
	tool := gjson.GetBytes(out, "tools.0")
	if got := tool.Get("type").String(); got != "function" {
		t.Fatalf("expected tool type %q, got %q; output=%s", "function", got, string(out))
	}

	typeProperty := tool.Get("parameters.properties.questions.items.properties.type")
	if !typeProperty.IsObject() {
		t.Fatalf("expected schema property named type to stay an object; output=%s", string(out))
	}
	if got := typeProperty.Get("type").String(); got != "string" {
		t.Fatalf("expected schema property type %q, got %q; output=%s", "string", got, string(out))
	}
	if got := typeProperty.Get("default").String(); got != "choice" {
		t.Fatalf("expected default %q, got %q; output=%s", "choice", got, string(out))
	}
	if got := typeProperty.Get("enum.2").String(); got != "yesno" {
		t.Fatalf("expected enum value %q, got %q; output=%s", "yesno", got, string(out))
	}
}
