// Package gemini - antigravity_gemini_request_test.go
// 测试 Gemini 到 Antigravity 请求格式转换功能。
// 覆盖 functionCall 和 text 部分的 thoughtSignature 替换（skip_thought_signature_validator）、
// string 类型 thought 部分的处理、Claude 模型的跳过逻辑、
// 并行 functionCall 的签名处理、以及 fixCLIToolResponse 的名称回填等功能。
package gemini

import (
	"fmt"
	"testing"

	"github.com/tidwall/gjson"
)

// TestConvertGeminiRequestToAntigravity_ReplacesClientSignatureOnFunctionCall 测试
// functionCall 上的客户端签名应被替换为 skip_thought_signature_validator
func TestConvertGeminiRequestToAntigravity_ReplacesClientSignatureOnFunctionCall(t *testing.T) {
	// Client signatures on Gemini function calls are not portable to Antigravity.
	validSignature := "abc123validSignature1234567890123456789012345678901234567890"
	inputJSON := []byte(fmt.Sprintf(`{
		"model": "gemini-3-pro-preview",
		"contents": [
			{
				"role": "model",
				"parts": [
					{"functionCall": {"name": "test_tool", "args": {}}, "thoughtSignature": "%s"}
				]
			}
		]
	}`, validSignature))

	output := ConvertGeminiRequestToAntigravity("gemini-3-pro-preview", inputJSON, false)
	outputStr := string(output)

	parts := gjson.Get(outputStr, "request.contents.0.parts").Array()
	if len(parts) != 1 {
		t.Fatalf("Expected 1 part, got %d", len(parts))
	}

	sig := parts[0].Get("thoughtSignature").String()
	expectedSig := "skip_thought_signature_validator"
	if sig != expectedSig {
		t.Errorf("Expected thoughtSignature '%s', got '%s'", expectedSig, sig)
	}
}

// TestConvertGeminiRequestToAntigravity_ReplacesClientSignatureOnTextPart 测试
// text 部分上的客户端签名应被替换为 skip_thought_signature_validator
func TestConvertGeminiRequestToAntigravity_ReplacesClientSignatureOnTextPart(t *testing.T) {
	validSignature := "abc123validSignature1234567890123456789012345678901234567890"
	inputJSON := []byte(fmt.Sprintf(`{
		"model": "gemini-3-pro-preview",
		"contents": [
			{
				"role": "model",
				"parts": [
					{"text": "previous answer", "thoughtSignature": "%s"}
				]
			}
		]
	}`, validSignature))

	output := ConvertGeminiRequestToAntigravity("gemini-3-pro-preview", inputJSON, false)
	outputStr := string(output)

	sig := gjson.Get(outputStr, "request.contents.0.parts.0.thoughtSignature").String()
	expectedSig := "skip_thought_signature_validator"
	if sig != expectedSig {
		t.Errorf("Expected thoughtSignature '%s', got '%s'", expectedSig, sig)
	}
}

// TestConvertGeminiRequestToAntigravity_AddsSkipSentinelToStringThoughtPart 测试
// string 类型的 thought 部分应添加 skip_thought_signature_validator 标记
func TestConvertGeminiRequestToAntigravity_AddsSkipSentinelToStringThoughtPart(t *testing.T) {
	inputJSON := []byte(`{
		"model": "gemini-3-pro-preview",
		"contents": [
			{
				"role": "model",
				"parts": [
					{"thought": "internal reasoning"}
				]
			}
		]
	}`)

	output := ConvertGeminiRequestToAntigravity("gemini-3-pro-preview", inputJSON, false)
	outputStr := string(output)

	sig := gjson.Get(outputStr, "request.contents.0.parts.0.thoughtSignature").String()
	expectedSig := "skip_thought_signature_validator"
	if sig != expectedSig {
		t.Errorf("Expected thoughtSignature '%s', got '%s'", expectedSig, sig)
	}
}

// TestConvertGeminiRequestToAntigravity_SkipsUppercaseClaudeModel 测试
// 大写开头的 Claude 模型名应跳过签名处理
func TestConvertGeminiRequestToAntigravity_SkipsUppercaseClaudeModel(t *testing.T) {
	inputJSON := []byte(`{
		"model": "Claude-Test",
		"contents": [
			{
				"role": "model",
				"parts": [
					{"functionCall": {"name": "test_tool", "args": {}}}
				]
			}
		]
	}`)

	output := ConvertGeminiRequestToAntigravity("Claude-Test", inputJSON, false)
	outputStr := string(output)

	if sig := gjson.Get(outputStr, "request.contents.0.parts.0.thoughtSignature"); sig.Exists() {
		t.Fatalf("Expected no thoughtSignature for Claude model, got %s", sig.Raw)
	}
}

// TestConvertGeminiRequestToAntigravity_AddSkipSentinelToFunctionCall 测试
// 无签名的 functionCall 应添加 skip_thought_signature_validator
func TestConvertGeminiRequestToAntigravity_AddSkipSentinelToFunctionCall(t *testing.T) {
	// functionCall without signature should get skip_thought_signature_validator
	inputJSON := []byte(`{
		"model": "gemini-3-pro-preview",
		"contents": [
			{
				"role": "model",
				"parts": [
					{"functionCall": {"name": "test_tool", "args": {}}}
				]
			}
		]
	}`)

	output := ConvertGeminiRequestToAntigravity("gemini-3-pro-preview", inputJSON, false)
	outputStr := string(output)

	// Check that skip_thought_signature_validator is added to functionCall
	sig := gjson.Get(outputStr, "request.contents.0.parts.0.thoughtSignature").String()
	expectedSig := "skip_thought_signature_validator"
	if sig != expectedSig {
		t.Errorf("Expected skip sentinel '%s', got '%s'", expectedSig, sig)
	}
}

// TestConvertGeminiRequestToAntigravity_ParallelFunctionCalls 测试
// 多个并行 functionCall 都应添加 skip_thought_signature_validator
func TestConvertGeminiRequestToAntigravity_ParallelFunctionCalls(t *testing.T) {
	// Multiple functionCalls should all get skip_thought_signature_validator
	inputJSON := []byte(`{
		"model": "gemini-3-pro-preview",
		"contents": [
			{
				"role": "model",
				"parts": [
					{"functionCall": {"name": "tool_one", "args": {"a": "1"}}},
					{"functionCall": {"name": "tool_two", "args": {"b": "2"}}}
				]
			}
		]
	}`)

	output := ConvertGeminiRequestToAntigravity("gemini-3-pro-preview", inputJSON, false)
	outputStr := string(output)

	parts := gjson.Get(outputStr, "request.contents.0.parts").Array()
	if len(parts) != 2 {
		t.Fatalf("Expected 2 parts, got %d", len(parts))
	}

	expectedSig := "skip_thought_signature_validator"
	for i, part := range parts {
		sig := part.Get("thoughtSignature").String()
		if sig != expectedSig {
			t.Errorf("Part %d: Expected '%s', got '%s'", i, expectedSig, sig)
		}
	}
}

// TestFixCLIToolResponse_PreservesFunctionResponseParts 测试
// functionResponse 中的 parts 字段（含 inlineData）应被保留
func TestFixCLIToolResponse_PreservesFunctionResponseParts(t *testing.T) {
	// When functionResponse contains a "parts" field with inlineData (from Claude
	// translator's image embedding), fixCLIToolResponse should preserve it as-is.
	// parseFunctionResponseRaw returns response.Raw for valid JSON objects,
	// so extra fields like "parts" survive the pipeline.
	input := `{
		"model": "claude-opus-4-6-thinking",
		"request": {
			"contents": [
				{
					"role": "model",
					"parts": [
						{
							"functionCall": {"name": "screenshot", "args": {}}
						}
					]
				},
				{
					"role": "function",
					"parts": [
						{
							"functionResponse": {
								"id": "tool-001",
								"name": "screenshot",
								"response": {"result": "Screenshot taken"},
								"parts": [
									{"inlineData": {"mimeType": "image/png", "data": "iVBOR"}}
								]
							}
						}
					]
				}
			]
		}
	}`

	result, err := fixCLIToolResponse(input)
	if err != nil {
		t.Fatalf("fixCLIToolResponse failed: %v", err)
	}

	// Find the function response content (role=function)
	contents := gjson.Get(result, "request.contents").Array()
	var funcContent gjson.Result
	for _, c := range contents {
		if c.Get("role").String() == "function" {
			funcContent = c
			break
		}
	}
	if !funcContent.Exists() {
		t.Fatal("function role content should exist in output")
	}

	// The functionResponse should be preserved with its parts field
	funcResp := funcContent.Get("parts.0.functionResponse")
	if !funcResp.Exists() {
		t.Fatal("functionResponse should exist in output")
	}

	// Verify the parts field with inlineData is preserved
	inlineParts := funcResp.Get("parts").Array()
	if len(inlineParts) != 1 {
		t.Fatalf("Expected 1 inlineData part in functionResponse.parts, got %d", len(inlineParts))
	}
	if inlineParts[0].Get("inlineData.mimeType").String() != "image/png" {
		t.Errorf("Expected mimeType 'image/png', got '%s'", inlineParts[0].Get("inlineData.mimeType").String())
	}
	if inlineParts[0].Get("inlineData.data").String() != "iVBOR" {
		t.Errorf("Expected data 'iVBOR', got '%s'", inlineParts[0].Get("inlineData.data").String())
	}

	// Verify response.result is also preserved
	if funcResp.Get("response.result").String() != "Screenshot taken" {
		t.Errorf("Expected response.result 'Screenshot taken', got '%s'", funcResp.Get("response.result").String())
	}
}

// TestFixCLIToolResponse_BackfillsEmptyFunctionResponseName 测试
// 空名称的 functionResponse 应从对应的 functionCall 回填名称
func TestFixCLIToolResponse_BackfillsEmptyFunctionResponseName(t *testing.T) {
	// When the Amp client sends functionResponse with an empty name,
	// fixCLIToolResponse should backfill it from the corresponding functionCall.
	input := `{
		"model": "gemini-3-pro-preview",
		"request": {
			"contents": [
				{
					"role": "model",
					"parts": [
						{"functionCall": {"name": "Bash", "args": {"cmd": "ls"}}}
					]
				},
				{
					"role": "function",
					"parts": [
						{"functionResponse": {"name": "", "response": {"output": "file1.txt"}}}
					]
				}
			]
		}
	}`

	result, err := fixCLIToolResponse(input)
	if err != nil {
		t.Fatalf("fixCLIToolResponse failed: %v", err)
	}

	contents := gjson.Get(result, "request.contents").Array()
	var funcContent gjson.Result
	for _, c := range contents {
		if c.Get("role").String() == "function" {
			funcContent = c
			break
		}
	}
	if !funcContent.Exists() {
		t.Fatal("function role content should exist in output")
	}

	name := funcContent.Get("parts.0.functionResponse.name").String()
	if name != "Bash" {
		t.Errorf("Expected backfilled name 'Bash', got '%s'", name)
	}
}

// TestFixCLIToolResponse_BackfillsMultipleEmptyNames 测试
// 多个并行 functionResponse 的空名称都应被正确回填
func TestFixCLIToolResponse_BackfillsMultipleEmptyNames(t *testing.T) {
	// Parallel function calls: both responses have empty names.
	input := `{
		"model": "gemini-3-pro-preview",
		"request": {
			"contents": [
				{
					"role": "model",
					"parts": [
						{"functionCall": {"name": "Read", "args": {"path": "/a"}}},
						{"functionCall": {"name": "Grep", "args": {"pattern": "x"}}}
					]
				},
				{
					"role": "function",
					"parts": [
						{"functionResponse": {"name": "", "response": {"result": "content a"}}},
						{"functionResponse": {"name": "", "response": {"result": "match x"}}}
					]
				}
			]
		}
	}`

	result, err := fixCLIToolResponse(input)
	if err != nil {
		t.Fatalf("fixCLIToolResponse failed: %v", err)
	}

	contents := gjson.Get(result, "request.contents").Array()
	var funcContent gjson.Result
	for _, c := range contents {
		if c.Get("role").String() == "function" {
			funcContent = c
			break
		}
	}
	if !funcContent.Exists() {
		t.Fatal("function role content should exist in output")
	}

	parts := funcContent.Get("parts").Array()
	if len(parts) != 2 {
		t.Fatalf("Expected 2 function response parts, got %d", len(parts))
	}

	name0 := parts[0].Get("functionResponse.name").String()
	name1 := parts[1].Get("functionResponse.name").String()
	if name0 != "Read" {
		t.Errorf("Expected first response name 'Read', got '%s'", name0)
	}
	if name1 != "Grep" {
		t.Errorf("Expected second response name 'Grep', got '%s'", name1)
	}
}

// TestFixCLIToolResponse_PreservesExistingName 测试
// 已有有效名称的 functionResponse 应被保留不覆盖
func TestFixCLIToolResponse_PreservesExistingName(t *testing.T) {
	// When functionResponse already has a valid name, it should be preserved.
	input := `{
		"model": "gemini-3-pro-preview",
		"request": {
			"contents": [
				{
					"role": "model",
					"parts": [
						{"functionCall": {"name": "Bash", "args": {}}}
					]
				},
				{
					"role": "function",
					"parts": [
						{"functionResponse": {"name": "Bash", "response": {"result": "ok"}}}
					]
				}
			]
		}
	}`

	result, err := fixCLIToolResponse(input)
	if err != nil {
		t.Fatalf("fixCLIToolResponse failed: %v", err)
	}

	contents := gjson.Get(result, "request.contents").Array()
	var funcContent gjson.Result
	for _, c := range contents {
		if c.Get("role").String() == "function" {
			funcContent = c
			break
		}
	}
	if !funcContent.Exists() {
		t.Fatal("function role content should exist in output")
	}

	name := funcContent.Get("parts.0.functionResponse.name").String()
	if name != "Bash" {
		t.Errorf("Expected preserved name 'Bash', got '%s'", name)
	}
}

// TestFixCLIToolResponse_MoreResponsesThanCalls 测试
// 当 functionResponse 数量超过 functionCall 数量时，多余的应被丢弃
func TestFixCLIToolResponse_MoreResponsesThanCalls(t *testing.T) {
	// If there are more function responses than calls, unmatched extras are discarded by grouping.
	input := `{
		"model": "gemini-3-pro-preview",
		"request": {
			"contents": [
				{
					"role": "model",
					"parts": [
						{"functionCall": {"name": "Bash", "args": {}}}
					]
				},
				{
					"role": "function",
					"parts": [
						{"functionResponse": {"name": "", "response": {"result": "ok"}}},
						{"functionResponse": {"name": "", "response": {"result": "extra"}}}
					]
				}
			]
		}
	}`

	result, err := fixCLIToolResponse(input)
	if err != nil {
		t.Fatalf("fixCLIToolResponse failed: %v", err)
	}

	contents := gjson.Get(result, "request.contents").Array()
	var funcContent gjson.Result
	for _, c := range contents {
		if c.Get("role").String() == "function" {
			funcContent = c
			break
		}
	}
	if !funcContent.Exists() {
		t.Fatal("function role content should exist in output")
	}

	// First response should be backfilled from the call
	name0 := funcContent.Get("parts.0.functionResponse.name").String()
	if name0 != "Bash" {
		t.Errorf("Expected first response name 'Bash', got '%s'", name0)
	}
}

// TestFixCLIToolResponse_MultipleGroupsFIFO 测试
// 多个顺序 function call/response 组应按 FIFO 策略匹配
func TestFixCLIToolResponse_MultipleGroupsFIFO(t *testing.T) {
	// Two sequential function call groups should be matched FIFO.
	input := `{
		"model": "gemini-3-pro-preview",
		"request": {
			"contents": [
				{
					"role": "model",
					"parts": [
						{"functionCall": {"name": "Read", "args": {}}}
					]
				},
				{
					"role": "function",
					"parts": [
						{"functionResponse": {"name": "", "response": {"result": "file content"}}}
					]
				},
				{
					"role": "model",
					"parts": [
						{"functionCall": {"name": "Grep", "args": {}}}
					]
				},
				{
					"role": "function",
					"parts": [
						{"functionResponse": {"name": "", "response": {"result": "match"}}}
					]
				}
			]
		}
	}`

	result, err := fixCLIToolResponse(input)
	if err != nil {
		t.Fatalf("fixCLIToolResponse failed: %v", err)
	}

	contents := gjson.Get(result, "request.contents").Array()
	var funcContents []gjson.Result
	for _, c := range contents {
		if c.Get("role").String() == "function" {
			funcContents = append(funcContents, c)
		}
	}
	if len(funcContents) != 2 {
		t.Fatalf("Expected 2 function contents, got %d", len(funcContents))
	}

	name0 := funcContents[0].Get("parts.0.functionResponse.name").String()
	name1 := funcContents[1].Get("parts.0.functionResponse.name").String()
	if name0 != "Read" {
		t.Errorf("Expected first group name 'Read', got '%s'", name0)
	}
	if name1 != "Grep" {
		t.Errorf("Expected second group name 'Grep', got '%s'", name1)
	}
}
