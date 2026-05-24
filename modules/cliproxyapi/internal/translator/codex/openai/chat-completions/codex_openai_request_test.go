// chat_completions - codex_openai_request_test.go
// Codex 的 OpenAI Chat Completions 格式请求转换器测试文件。
// 包含以下测试用例：
// 1. 基本工具调用转换（system + user + assistant(tool_calls) + tool result）
// 2. 带内容的工具调用（assistant 同时有文本和 tool_calls）
// 3. 多模态工具输出（text、image_url、file 类型）
// 4. 无效结构化部分的回退处理
// 5. 非字符串 JSON 内容处理（null、object）
// 6. 并行多工具调用
// 7. 无空 assistant 消息回归测试
// 8. 多轮工具调用
// 9. 工具名称截断（64 字符限制）
// 10. 空字符串 content 处理
// 11. call_id 匹配验证
// 12. 工具定义转换验证
package chat_completions

import (
	"testing"

	"github.com/tidwall/gjson"
)

// TestToolCallSimple 测试基本的工具调用转换。
// 验证 system -> developer 角色转换、function_call 和 function_call_output 的正确生成。
// 确保不产生空的 assistant 消息。
func TestToolCallSimple(t *testing.T) {
	input := []byte(`{
		"model": "gpt-4o",
		"messages": [
			{"role": "system", "content": "You are a helpful assistant."},
			{"role": "user", "content": "What is the weather in Paris?"},
			{
				"role": "assistant",
				"content": null,
				"tool_calls": [
					{
						"id": "call_1",
						"type": "function",
						"function": {
							"name": "get_weather",
							"arguments": "{\"city\":\"Paris\"}"
						}
					}
				]
			},
			{
				"role": "tool",
				"tool_call_id": "call_1",
				"content": "sunny, 22C"
			}
		],
		"tools": [
			{
				"type": "function",
				"function": {
					"name": "get_weather",
					"description": "Get weather for a city",
					"parameters": {"type": "object", "properties": {"city": {"type": "string"}}}
				}
			}
		]
	}`)

	out := ConvertOpenAIRequestToCodex("gpt-4o", input, true)
	result := string(out)

	items := gjson.Get(result, "input").Array()
	if len(items) != 4 {
		t.Fatalf("expected 4 input items, got %d: %s", len(items), gjson.Get(result, "input").Raw)
	}

	// system -> developer
	if items[0].Get("type").String() != "message" {
		t.Errorf("item 0: expected type 'message', got '%s'", items[0].Get("type").String())
	}
	if items[0].Get("role").String() != "developer" {
		t.Errorf("item 0: expected role 'developer', got '%s'", items[0].Get("role").String())
	}

	// user
	if items[1].Get("type").String() != "message" {
		t.Errorf("item 1: expected type 'message', got '%s'", items[1].Get("type").String())
	}
	if items[1].Get("role").String() != "user" {
		t.Errorf("item 1: expected role 'user', got '%s'", items[1].Get("role").String())
	}

	// function_call, not an empty assistant msg
	if items[2].Get("type").String() != "function_call" {
		t.Errorf("item 2: expected type 'function_call', got '%s'", items[2].Get("type").String())
	}
	if items[2].Get("call_id").String() != "call_1" {
		t.Errorf("item 2: expected call_id 'call_1', got '%s'", items[2].Get("call_id").String())
	}
	if items[2].Get("name").String() != "get_weather" {
		t.Errorf("item 2: expected name 'get_weather', got '%s'", items[2].Get("name").String())
	}
	if items[2].Get("arguments").String() != `{"city":"Paris"}` {
		t.Errorf("item 2: unexpected arguments: %s", items[2].Get("arguments").String())
	}

	// function_call_output
	if items[3].Get("type").String() != "function_call_output" {
		t.Errorf("item 3: expected type 'function_call_output', got '%s'", items[3].Get("type").String())
	}
	if items[3].Get("call_id").String() != "call_1" {
		t.Errorf("item 3: expected call_id 'call_1', got '%s'", items[3].Get("call_id").String())
	}
	if items[3].Get("output").String() != "sunny, 22C" {
		t.Errorf("item 3: expected output 'sunny, 22C', got '%s'", items[3].Get("output").String())
	}
}

// TestToolCallWithContent 测试 assistant 同时包含文本内容和 tool_calls 的转换。
// 验证 assistant 消息被保留（非空内容），后面跟着 function_call 对象。
func TestToolCallWithContent(t *testing.T) {
	input := []byte(`{
		"model": "gpt-4o",
		"messages": [
			{"role": "user", "content": "What is the weather?"},
			{
				"role": "assistant",
				"content": "Let me check the weather for you.",
				"tool_calls": [
					{
						"id": "call_abc",
						"type": "function",
						"function": {
							"name": "get_weather",
							"arguments": "{}"
						}
					}
				]
			},
			{
				"role": "tool",
				"tool_call_id": "call_abc",
				"content": "rainy, 15C"
			}
		],
		"tools": [
			{
				"type": "function",
				"function": {
					"name": "get_weather",
					"description": "Get weather",
					"parameters": {"type": "object", "properties": {}}
				}
			}
		]
	}`)

	out := ConvertOpenAIRequestToCodex("gpt-4o", input, true)
	result := string(out)

	items := gjson.Get(result, "input").Array()
	// user + assistant(with content) + function_call + function_call_output
	if len(items) != 4 {
		t.Fatalf("expected 4 input items, got %d: %s", len(items), gjson.Get(result, "input").Raw)
	}

	if items[0].Get("role").String() != "user" {
		t.Errorf("item 0: expected role 'user', got '%s'", items[0].Get("role").String())
	}

	// assistant with content — should be kept
	if items[1].Get("type").String() != "message" {
		t.Errorf("item 1: expected type 'message', got '%s'", items[1].Get("type").String())
	}
	if items[1].Get("role").String() != "assistant" {
		t.Errorf("item 1: expected role 'assistant', got '%s'", items[1].Get("role").String())
	}
	contentParts := items[1].Get("content").Array()
	if len(contentParts) == 0 {
		t.Errorf("item 1: assistant message should have content parts")
	}

	if items[2].Get("type").String() != "function_call" {
		t.Errorf("item 2: expected type 'function_call', got '%s'", items[2].Get("type").String())
	}
	if items[2].Get("call_id").String() != "call_abc" {
		t.Errorf("item 2: expected call_id 'call_abc', got '%s'", items[2].Get("call_id").String())
	}

	if items[3].Get("type").String() != "function_call_output" {
		t.Errorf("item 3: expected type 'function_call_output', got '%s'", items[3].Get("type").String())
	}
	if items[3].Get("call_id").String() != "call_abc" {
		t.Errorf("item 3: expected call_id 'call_abc', got '%s'", items[3].Get("call_id").String())
	}
}

// TestToolCallOutputWithMultimodalContent 测试工具输出中包含多模态内容的转换。
// 验证 text -> input_text、image_url -> input_image、file -> input_file 的正确转换。
func TestToolCallOutputWithMultimodalContent(t *testing.T) {
	input := []byte(`{
		"model": "gpt-4o",
		"messages": [
			{"role": "user", "content": "Show me the generated result."},
			{
				"role": "assistant",
				"content": null,
				"tool_calls": [
					{
						"id": "call_output_1",
						"type": "function",
						"function": {"name": "render_output", "arguments": "{}"}
					}
				]
			},
			{
				"role": "tool",
				"tool_call_id": "call_output_1",
				"content": [
					{"type":"text","text":"Rendered result attached."},
					{"type":"image_url","image_url":{"url":"https://example.com/generated.png","detail":"high"}},
					{"type":"image_url","image_url":{"file_id":"file-img-123"}},
					{"type":"file","file":{"file_id":"file-doc-123","filename":"doc.pdf"}},
					{"type":"file","file":{"file_data":"SGVsbG8=","filename":"inline.txt"}},
					{"type":"file","file":{"file_url":"https://example.com/report.pdf","filename":"report.pdf"}}
				]
			}
		],
		"tools": [
			{
				"type": "function",
				"function": {"name": "render_output", "description": "Render output", "parameters": {"type": "object", "properties": {}}}
			}
		]
	}`)

	out := ConvertOpenAIRequestToCodex("gpt-4o", input, true)
	result := string(out)

	output := gjson.Get(result, "input.2.output")
	if !output.IsArray() {
		t.Fatalf("expected tool output to be an array, got: %s", output.Raw)
	}

	parts := output.Array()
	if len(parts) != 6 {
		t.Fatalf("expected 6 output parts, got %d: %s", len(parts), output.Raw)
	}
	if parts[0].Get("type").String() != "input_text" || parts[0].Get("text").String() != "Rendered result attached." {
		t.Fatalf("part 0: expected input_text with rendered text, got %s", parts[0].Raw)
	}
	if parts[1].Get("type").String() != "input_image" {
		t.Fatalf("part 1: expected input_image, got %s", parts[1].Raw)
	}
	if parts[1].Get("image_url").String() != "https://example.com/generated.png" {
		t.Errorf("part 1: unexpected image_url %s", parts[1].Get("image_url").String())
	}
	if parts[1].Get("detail").String() != "high" {
		t.Errorf("part 1: unexpected detail %s", parts[1].Get("detail").String())
	}
	if parts[2].Get("type").String() != "input_image" || parts[2].Get("file_id").String() != "file-img-123" {
		t.Fatalf("part 2: expected file_id-backed input_image, got %s", parts[2].Raw)
	}
	if parts[3].Get("type").String() != "input_file" || parts[3].Get("file_id").String() != "file-doc-123" {
		t.Fatalf("part 3: expected file_id-backed input_file, got %s", parts[3].Raw)
	}
	if parts[3].Get("filename").String() != "doc.pdf" {
		t.Errorf("part 3: unexpected filename %s", parts[3].Get("filename").String())
	}
	if parts[4].Get("type").String() != "input_file" || parts[4].Get("file_data").String() != "SGVsbG8=" {
		t.Fatalf("part 4: expected file_data-backed input_file, got %s", parts[4].Raw)
	}
	if parts[5].Get("type").String() != "input_file" || parts[5].Get("file_url").String() != "https://example.com/report.pdf" {
		t.Fatalf("part 5: expected file_url-backed input_file, got %s", parts[5].Raw)
	}
}

// TestToolCallOutputFallsBackForInvalidStructuredParts 测试无效结构化部分的回退处理。
// 验证缺少必要字段的 image_url、file 和未知类型都被回退为 input_text。
func TestToolCallOutputFallsBackForInvalidStructuredParts(t *testing.T) {
	input := []byte(`{
		"model": "gpt-4o",
		"messages": [
			{"role": "user", "content": "Check tool output."},
			{
				"role": "assistant",
				"content": null,
				"tool_calls": [
					{"id": "call_invalid_parts", "type": "function", "function": {"name": "inspect", "arguments": "{}"}}
				]
			},
			{
				"role": "tool",
				"tool_call_id": "call_invalid_parts",
				"content": [
					{"type":"image_url","image_url":{"detail":"low"}},
					{"type":"file","file":{"filename":"orphan.txt"}},
					{"type":"unknown_type","foo":"bar","nested":{"a":1}}
				]
			}
		],
		"tools": [
			{"type": "function", "function": {"name": "inspect", "description": "Inspect", "parameters": {"type": "object", "properties": {}}}}
		]
	}`)

	out := ConvertOpenAIRequestToCodex("gpt-4o", input, true)
	result := string(out)

	parts := gjson.Get(result, "input.2.output").Array()
	if len(parts) != 3 {
		t.Fatalf("expected 3 output parts, got %d: %s", len(parts), gjson.Get(result, "input.2.output").Raw)
	}

	expectedFallbacks := []string{
		`{"type":"image_url","image_url":{"detail":"low"}}`,
		`{"type":"file","file":{"filename":"orphan.txt"}}`,
		`{"type":"unknown_type","foo":"bar","nested":{"a":1}}`,
	}
	for i, expectedFallback := range expectedFallbacks {
		if parts[i].Get("type").String() != "input_text" {
			t.Fatalf("part %d: expected input_text fallback, got %s", i, parts[i].Raw)
		}
		if parts[i].Get("text").String() != expectedFallback {
			t.Fatalf("part %d: expected fallback %s, got %s", i, expectedFallback, parts[i].Get("text").String())
		}
	}
}

// TestToolCallOutputWithNonStringJSONContent 测试非字符串 JSON 内容的工具输出处理。
// 验证 null 和 object 类型的 content 能够正确保留。
func TestToolCallOutputWithNonStringJSONContent(t *testing.T) {
	tests := []struct {
		name           string
		content        string
		expectedOutput string
	}{
		{name: "null", content: `null`, expectedOutput: `null`},
		{name: "object", content: `{"status":"ok","count":2}`, expectedOutput: `{"status":"ok","count":2}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := []byte(`{
				"model": "gpt-4o",
				"messages": [
					{"role": "user", "content": "Check tool output."},
					{
						"role": "assistant",
						"content": null,
						"tool_calls": [
							{"id": "call_json", "type": "function", "function": {"name": "inspect", "arguments": "{}"}}
						]
					},
					{
						"role": "tool",
						"tool_call_id": "call_json",
						"content": ` + tt.content + `
					}
				],
				"tools": [
					{"type": "function", "function": {"name": "inspect", "description": "Inspect", "parameters": {"type": "object", "properties": {}}}}
				]
			}`)

			out := ConvertOpenAIRequestToCodex("gpt-4o", input, true)
			result := string(out)

			output := gjson.Get(result, "input.2.output")
			if !output.Exists() {
				t.Fatalf("expected output field to exist: %s", gjson.Get(result, "input.2").Raw)
			}
			if output.String() != tt.expectedOutput {
				t.Fatalf("expected output %s, got %s", tt.expectedOutput, output.String())
			}
		})
	}
}

// TestMultipleToolCalls 测试并行多工具调用的转换。
// 验证 assistant 同时调用 3 个工具时，所有 call_id 和输出都能正确配对。
func TestMultipleToolCalls(t *testing.T) {
	input := []byte(`{
		"model": "gpt-4o",
		"messages": [
			{"role": "user", "content": "Compare weather in Paris, London and Tokyo"},
			{
				"role": "assistant",
				"content": null,
				"tool_calls": [
					{
						"id": "call_paris",
						"type": "function",
						"function": {
							"name": "get_weather",
							"arguments": "{\"city\":\"Paris\"}"
						}
					},
					{
						"id": "call_london",
						"type": "function",
						"function": {
							"name": "get_weather",
							"arguments": "{\"city\":\"London\"}"
						}
					},
					{
						"id": "call_tokyo",
						"type": "function",
						"function": {
							"name": "get_weather",
							"arguments": "{\"city\":\"Tokyo\"}"
						}
					}
				]
			},
			{"role": "tool", "tool_call_id": "call_paris", "content": "sunny, 22C"},
			{"role": "tool", "tool_call_id": "call_london", "content": "cloudy, 14C"},
			{"role": "tool", "tool_call_id": "call_tokyo", "content": "humid, 28C"}
		],
		"tools": [
			{
				"type": "function",
				"function": {
					"name": "get_weather",
					"description": "Get weather",
					"parameters": {"type": "object", "properties": {"city": {"type": "string"}}}
				}
			}
		]
	}`)

	out := ConvertOpenAIRequestToCodex("gpt-4o", input, true)
	result := string(out)

	items := gjson.Get(result, "input").Array()
	// user + 3 function_call + 3 function_call_output = 7
	if len(items) != 7 {
		t.Fatalf("expected 7 input items, got %d: %s", len(items), gjson.Get(result, "input").Raw)
	}

	if items[0].Get("role").String() != "user" {
		t.Errorf("item 0: expected role 'user', got '%s'", items[0].Get("role").String())
	}

	expectedCallIDs := []string{"call_paris", "call_london", "call_tokyo"}
	for i, expectedID := range expectedCallIDs {
		idx := i + 1
		if items[idx].Get("type").String() != "function_call" {
			t.Errorf("item %d: expected type 'function_call', got '%s'", idx, items[idx].Get("type").String())
		}
		if items[idx].Get("call_id").String() != expectedID {
			t.Errorf("item %d: expected call_id '%s', got '%s'", idx, expectedID, items[idx].Get("call_id").String())
		}
	}

	expectedOutputs := []string{"sunny, 22C", "cloudy, 14C", "humid, 28C"}
	for i, expectedOutput := range expectedOutputs {
		idx := i + 4
		if items[idx].Get("type").String() != "function_call_output" {
			t.Errorf("item %d: expected type 'function_call_output', got '%s'", idx, items[idx].Get("type").String())
		}
		if items[idx].Get("call_id").String() != expectedCallIDs[i] {
			t.Errorf("item %d: expected call_id '%s', got '%s'", idx, expectedCallIDs[i], items[idx].Get("call_id").String())
		}
		if items[idx].Get("output").String() != expectedOutput {
			t.Errorf("item %d: expected output '%s', got '%s'", idx, expectedOutput, items[idx].Get("output").String())
		}
	}
}

// TestNoSpuriousEmptyAssistantMessage 回归测试 #2132：仅包含工具调用的 assistant 消息（content:null）
// 不应在转换后的输出中产生空的 message 对象。
func TestNoSpuriousEmptyAssistantMessage(t *testing.T) {
	input := []byte(`{
		"model": "gpt-4o",
		"messages": [
			{"role": "user", "content": "Call a tool"},
			{
				"role": "assistant",
				"content": null,
				"tool_calls": [
					{
						"id": "call_x",
						"type": "function",
						"function": {"name": "do_thing", "arguments": "{}"}
					}
				]
			},
			{"role": "tool", "tool_call_id": "call_x", "content": "done"}
		],
		"tools": [
			{
				"type": "function",
				"function": {
					"name": "do_thing",
					"description": "Do a thing",
					"parameters": {"type": "object", "properties": {}}
				}
			}
		]
	}`)

	out := ConvertOpenAIRequestToCodex("gpt-4o", input, true)
	result := string(out)

	items := gjson.Get(result, "input").Array()

	for i, item := range items {
		typ := item.Get("type").String()
		role := item.Get("role").String()
		if typ == "message" && role == "assistant" {
			contentArr := item.Get("content").Array()
			if len(contentArr) == 0 {
				t.Errorf("item %d: empty assistant message breaks call_id matching. item: %s", i, item.Raw)
			}
		}
	}

	// should be exactly: user + function_call + function_call_output
	if len(items) != 3 {
		t.Fatalf("expected 3 input items (user + function_call + function_call_output), got %d: %s", len(items), gjson.Get(result, "input").Raw)
	}
	if items[0].Get("type").String() != "message" || items[0].Get("role").String() != "user" {
		t.Errorf("item 0: expected user message")
	}
	if items[1].Get("type").String() != "function_call" {
		t.Errorf("item 1: expected function_call, got %s", items[1].Get("type").String())
	}
	if items[2].Get("type").String() != "function_call_output" {
		t.Errorf("item 2: expected function_call_output, got %s", items[2].Get("type").String())
	}
}

// TestMultiTurnToolCalling 测试多轮工具调用场景。
// 验证两轮工具调用之间有文本回复时，所有项目都能正确转换和配对。
func TestMultiTurnToolCalling(t *testing.T) {
	input := []byte(`{
		"model": "gpt-4o",
		"messages": [
			{"role": "user", "content": "Weather in Paris?"},
			{
				"role": "assistant",
				"content": null,
				"tool_calls": [{"id": "call_r1", "type": "function", "function": {"name": "get_weather", "arguments": "{\"city\":\"Paris\"}"}}]
			},
			{"role": "tool", "tool_call_id": "call_r1", "content": "sunny"},
			{"role": "assistant", "content": "It is sunny in Paris."},
			{"role": "user", "content": "And London?"},
			{
				"role": "assistant",
				"content": null,
				"tool_calls": [{"id": "call_r2", "type": "function", "function": {"name": "get_weather", "arguments": "{\"city\":\"London\"}"}}]
			},
			{"role": "tool", "tool_call_id": "call_r2", "content": "rainy"}
		],
		"tools": [
			{
				"type": "function",
				"function": {
					"name": "get_weather",
					"description": "Get weather",
					"parameters": {"type": "object", "properties": {"city": {"type": "string"}}}
				}
			}
		]
	}`)

	out := ConvertOpenAIRequestToCodex("gpt-4o", input, true)
	result := string(out)

	items := gjson.Get(result, "input").Array()
	// user, func_call(r1), func_output(r1), assistant text, user, func_call(r2), func_output(r2)
	if len(items) != 7 {
		t.Fatalf("expected 7 input items, got %d: %s", len(items), gjson.Get(result, "input").Raw)
	}

	for i, item := range items {
		if item.Get("type").String() == "message" && item.Get("role").String() == "assistant" {
			if len(item.Get("content").Array()) == 0 {
				t.Errorf("item %d: unexpected empty assistant message", i)
			}
		}
	}

	// round 1
	if items[1].Get("type").String() != "function_call" {
		t.Errorf("item 1: expected function_call, got %s", items[1].Get("type").String())
	}
	if items[1].Get("call_id").String() != "call_r1" {
		t.Errorf("item 1: expected call_id 'call_r1', got '%s'", items[1].Get("call_id").String())
	}
	if items[2].Get("type").String() != "function_call_output" {
		t.Errorf("item 2: expected function_call_output, got %s", items[2].Get("type").String())
	}

	// text reply between rounds
	if items[3].Get("type").String() != "message" || items[3].Get("role").String() != "assistant" {
		t.Errorf("item 3: expected assistant message, got type=%s role=%s", items[3].Get("type").String(), items[3].Get("role").String())
	}

	// round 2
	if items[5].Get("type").String() != "function_call" {
		t.Errorf("item 5: expected function_call, got %s", items[5].Get("type").String())
	}
	if items[5].Get("call_id").String() != "call_r2" {
		t.Errorf("item 5: expected call_id 'call_r2', got '%s'", items[5].Get("call_id").String())
	}
	if items[6].Get("type").String() != "function_call_output" {
		t.Errorf("item 6: expected function_call_output, got %s", items[6].Get("type").String())
	}
}

// TestToolNameShortening 测试超过 64 字符的工具名称截断。
// 验证工具名称被正确截断，而 call_id 保持不变。
func TestToolNameShortening(t *testing.T) {
	longName := "a_very_long_tool_name_that_exceeds_sixty_four_characters_limit_here_test"
	if len(longName) <= 64 {
		t.Fatalf("test setup error: name must be > 64 chars, got %d", len(longName))
	}

	input := []byte(`{
		"model": "gpt-4o",
		"messages": [
			{"role": "user", "content": "Do it"},
			{
				"role": "assistant",
				"content": null,
				"tool_calls": [
					{
						"id": "call_long",
						"type": "function",
						"function": {
							"name": "` + longName + `",
							"arguments": "{}"
						}
					}
				]
			},
			{"role": "tool", "tool_call_id": "call_long", "content": "ok"}
		],
		"tools": [
			{
				"type": "function",
				"function": {
					"name": "` + longName + `",
					"description": "A tool with a very long name",
					"parameters": {"type": "object", "properties": {}}
				}
			}
		]
	}`)

	out := ConvertOpenAIRequestToCodex("gpt-4o", input, true)
	result := string(out)

	items := gjson.Get(result, "input").Array()

	// find function_call
	var funcCallItem gjson.Result
	for _, item := range items {
		if item.Get("type").String() == "function_call" {
			funcCallItem = item
			break
		}
	}

	if !funcCallItem.Exists() {
		t.Fatal("no function_call item found in output")
	}

	// call_id unchanged
	if funcCallItem.Get("call_id").String() != "call_long" {
		t.Errorf("call_id changed: expected 'call_long', got '%s'", funcCallItem.Get("call_id").String())
	}

	// name must be truncated
	translatedName := funcCallItem.Get("name").String()
	if translatedName == longName {
		t.Errorf("tool name was NOT shortened: still '%s'", translatedName)
	}
	if len(translatedName) > 64 {
		t.Errorf("shortened name still > 64 chars: len=%d name='%s'", len(translatedName), translatedName)
	}
}

// TestEmptyStringContent 测试 content 为空字符串（非 null）时的处理。
// 验证空字符串 content 与 null 处理方式一致，不产生空的 assistant 消息。
func TestEmptyStringContent(t *testing.T) {
	input := []byte(`{
		"model": "gpt-4o",
		"messages": [
			{"role": "user", "content": "Do something"},
			{
				"role": "assistant",
				"content": "",
				"tool_calls": [
					{
						"id": "call_empty",
						"type": "function",
						"function": {"name": "action", "arguments": "{}"}
					}
				]
			},
			{"role": "tool", "tool_call_id": "call_empty", "content": "result"}
		],
		"tools": [
			{
				"type": "function",
				"function": {
					"name": "action",
					"description": "An action",
					"parameters": {"type": "object", "properties": {}}
				}
			}
		]
	}`)

	out := ConvertOpenAIRequestToCodex("gpt-4o", input, true)
	result := string(out)

	items := gjson.Get(result, "input").Array()

	for i, item := range items {
		if item.Get("type").String() == "message" && item.Get("role").String() == "assistant" {
			if len(item.Get("content").Array()) == 0 {
				t.Errorf("item %d: empty assistant message from content:\"\"", i)
			}
		}
	}

	// user + function_call + function_call_output
	if len(items) != 3 {
		t.Errorf("expected 3 input items, got %d", len(items))
	}
}

// TestCallIDsMatchBetweenCallAndOutput 验证每个 function_call_output 都有对应的 function_call 通过 call_id 匹配。
func TestCallIDsMatchBetweenCallAndOutput(t *testing.T) {
	input := []byte(`{
		"model": "gpt-4o",
		"messages": [
			{"role": "user", "content": "Multi-tool"},
			{
				"role": "assistant",
				"content": null,
				"tool_calls": [
					{"id": "id_a", "type": "function", "function": {"name": "tool_a", "arguments": "{}"}},
					{"id": "id_b", "type": "function", "function": {"name": "tool_b", "arguments": "{}"}}
				]
			},
			{"role": "tool", "tool_call_id": "id_a", "content": "res_a"},
			{"role": "tool", "tool_call_id": "id_b", "content": "res_b"}
		],
		"tools": [
			{"type": "function", "function": {"name": "tool_a", "description": "A", "parameters": {"type": "object", "properties": {}}}},
			{"type": "function", "function": {"name": "tool_b", "description": "B", "parameters": {"type": "object", "properties": {}}}}
		]
	}`)

	out := ConvertOpenAIRequestToCodex("gpt-4o", input, true)
	result := string(out)

	items := gjson.Get(result, "input").Array()

	// collect call_ids from function_call items
	callIDs := make(map[string]bool)
	for _, item := range items {
		if item.Get("type").String() == "function_call" {
			callIDs[item.Get("call_id").String()] = true
		}
	}

	for i, item := range items {
		if item.Get("type").String() == "function_call_output" {
			outID := item.Get("call_id").String()
			if !callIDs[outID] {
				t.Errorf("item %d: function_call_output has call_id '%s' with no matching function_call", i, outID)
			}
		}
	}

	// 2 calls, 2 outputs
	funcCallCount := 0
	funcOutputCount := 0
	for _, item := range items {
		switch item.Get("type").String() {
		case "function_call":
			funcCallCount++
		case "function_call_output":
			funcOutputCount++
		}
	}
	if funcCallCount != 2 {
		t.Errorf("expected 2 function_calls, got %d", funcCallCount)
	}
	if funcOutputCount != 2 {
		t.Errorf("expected 2 function_call_outputs, got %d", funcOutputCount)
	}
}

// TestToolsDefinitionTranslated 验证工具定义数组能够正确转换到 Responses 格式输出中。
func TestToolsDefinitionTranslated(t *testing.T) {
	input := []byte(`{
		"model": "gpt-4o",
		"messages": [
			{"role": "user", "content": "Hi"}
		],
		"tools": [
			{
				"type": "function",
				"function": {
					"name": "search",
					"description": "Search the web",
					"parameters": {"type": "object", "properties": {"query": {"type": "string"}}, "required": ["query"]}
				}
			}
		]
	}`)

	out := ConvertOpenAIRequestToCodex("gpt-4o", input, true)
	result := string(out)

	tools := gjson.Get(result, "tools").Array()
	if len(tools) == 0 {
		t.Fatal("no tools found in output")
	}

	found := false
	for _, tool := range tools {
		if tool.Get("name").String() == "search" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("tool 'search' not found in output tools: %s", gjson.Get(result, "tools").Raw)
	}
}
