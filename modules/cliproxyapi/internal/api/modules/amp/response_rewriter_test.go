// amp - response_rewriter_test.go
// 响应重写器（ResponseRewriter）的单元测试。
// 测试以下功能：
// - 非流式响应中的模型名重写（顶层、response.model、response.created）
// - 无模型字段时的无操作处理
// - 空原始模型名时的跳过处理
// - 流式 SSE 响应中的模型名重写
// - 多事件块和 message.model 的重写
// - 思维内容块的保留和签名注入
// - 请求体清理：移除无效签名的 thinking 块、剥离 tool_use 块的签名
// - 工具名称规范化：bash->Bash、read->Read 等大小写修正
// - 流式和非流式工具名称规范化
// - 已正确大小写的工具名和未知工具名的无操作处理
package amp

import (
	"strings"
	"testing"
)

// TestRewriteModelInResponse_TopLevel 测试顶层 model 字段的重写
func TestRewriteModelInResponse_TopLevel(t *testing.T) {
	rw := &ResponseRewriter{originalModel: "gpt-5.2-codex"}

	input := []byte(`{"id":"resp_1","model":"gpt-5.3-codex","output":[]}`)
	result := rw.rewriteModelInResponse(input)

	expected := `{"id":"resp_1","model":"gpt-5.2-codex","output":[]}`
	if string(result) != expected {
		t.Errorf("expected %s, got %s", expected, string(result))
	}
}

// TestRewriteModelInResponse_ResponseModel 测试 response.completed 事件中的 model 重写
func TestRewriteModelInResponse_ResponseModel(t *testing.T) {
	rw := &ResponseRewriter{originalModel: "gpt-5.2-codex"}

	input := []byte(`{"type":"response.completed","response":{"id":"resp_1","model":"gpt-5.3-codex","status":"completed"}}`)
	result := rw.rewriteModelInResponse(input)

	expected := `{"type":"response.completed","response":{"id":"resp_1","model":"gpt-5.2-codex","status":"completed"}}`
	if string(result) != expected {
		t.Errorf("expected %s, got %s", expected, string(result))
	}
}

// TestRewriteModelInResponse_ResponseCreated 测试 response.created 事件中的 model 重写
func TestRewriteModelInResponse_ResponseCreated(t *testing.T) {
	rw := &ResponseRewriter{originalModel: "gpt-5.2-codex"}

	input := []byte(`{"type":"response.created","response":{"id":"resp_1","model":"gpt-5.3-codex","status":"in_progress"}}`)
	result := rw.rewriteModelInResponse(input)

	expected := `{"type":"response.created","response":{"id":"resp_1","model":"gpt-5.2-codex","status":"in_progress"}}`
	if string(result) != expected {
		t.Errorf("expected %s, got %s", expected, string(result))
	}
}

// TestRewriteModelInResponse_NoModelField 测试无 model 字段时的无操作处理
func TestRewriteModelInResponse_NoModelField(t *testing.T) {
	rw := &ResponseRewriter{originalModel: "gpt-5.2-codex"}

	input := []byte(`{"type":"response.output_item.added","item":{"id":"item_1","type":"message"}}`)
	result := rw.rewriteModelInResponse(input)

	if string(result) != string(input) {
		t.Errorf("expected no modification, got %s", string(result))
	}
}

// TestRewriteModelInResponse_EmptyOriginalModel 测试空原始模型名时的跳过处理
func TestRewriteModelInResponse_EmptyOriginalModel(t *testing.T) {
	rw := &ResponseRewriter{originalModel: ""}

	input := []byte(`{"model":"gpt-5.3-codex"}`)
	result := rw.rewriteModelInResponse(input)

	if string(result) != string(input) {
		t.Errorf("expected no modification when originalModel is empty, got %s", string(result))
	}
}

// TestRewriteStreamChunk_SSEWithResponseModel 测试 SSE 流式响应中的 model 重写
func TestRewriteStreamChunk_SSEWithResponseModel(t *testing.T) {
	rw := &ResponseRewriter{originalModel: "gpt-5.2-codex"}

	chunk := []byte("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"model\":\"gpt-5.3-codex\",\"status\":\"completed\"}}\n\n")
	result := rw.rewriteStreamChunk(chunk)

	expected := "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"model\":\"gpt-5.2-codex\",\"status\":\"completed\"}}\n\n"
	if string(result) != expected {
		t.Errorf("expected %s, got %s", expected, string(result))
	}
}

// TestRewriteStreamChunk_MultipleEvents 测试多事件块中的 model 重写
func TestRewriteStreamChunk_MultipleEvents(t *testing.T) {
	rw := &ResponseRewriter{originalModel: "gpt-5.2-codex"}

	chunk := []byte("data: {\"type\":\"response.created\",\"response\":{\"model\":\"gpt-5.3-codex\"}}\n\ndata: {\"type\":\"response.output_item.added\",\"item\":{\"id\":\"item_1\"}}\n\n")
	result := rw.rewriteStreamChunk(chunk)

	if string(result) == string(chunk) {
		t.Error("expected response.model to be rewritten in SSE stream")
	}
	if !contains(result, []byte(`"model":"gpt-5.2-codex"`)) {
		t.Errorf("expected rewritten model in output, got %s", string(result))
	}
}

// TestRewriteStreamChunk_MessageModel 测试 message.model 字段的重写
func TestRewriteStreamChunk_MessageModel(t *testing.T) {
	rw := &ResponseRewriter{originalModel: "claude-opus-4.5"}

	chunk := []byte("data: {\"message\":{\"model\":\"claude-sonnet-4\",\"role\":\"assistant\"}}\n\n")
	result := rw.rewriteStreamChunk(chunk)

	expected := "data: {\"message\":{\"model\":\"claude-opus-4.5\",\"role\":\"assistant\"}}\n\n"
	if string(result) != expected {
		t.Errorf("expected %s, got %s", expected, string(result))
	}
}

// TestRewriteStreamChunk_PreservesThinkingWithSignatureInjection 测试流式模式下：
// - 思维内容块被保留（不被抑制），避免破坏 SSE 索引对齐和 TUI 渲染
// - 思维和 tool_use 块中注入签名字段
func TestRewriteStreamChunk_PreservesThinkingWithSignatureInjection(t *testing.T) {
	rw := &ResponseRewriter{}

	chunk := []byte("event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"thinking\",\"thinking\":\"\"}}\n\nevent: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"abc\"}}\n\nevent: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\nevent: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":1,\"content_block\":{\"type\":\"tool_use\",\"name\":\"bash\",\"input\":{}}}\n\n")
	result := rw.rewriteStreamChunk(chunk)

	// Streaming mode preserves thinking blocks (does NOT suppress them)
	// to avoid breaking SSE index alignment and TUI rendering
	if !contains(result, []byte(`"content_block":{"type":"thinking"`)) {
		t.Fatalf("expected thinking content_block_start to be preserved, got %s", string(result))
	}
	if !contains(result, []byte(`"delta":{"type":"thinking_delta"`)) {
		t.Fatalf("expected thinking_delta to be preserved, got %s", string(result))
	}
	if !contains(result, []byte(`"type":"content_block_stop","index":0`)) {
		t.Fatalf("expected content_block_stop for thinking block to be preserved, got %s", string(result))
	}
	if !contains(result, []byte(`"content_block":{"type":"tool_use"`)) {
		t.Fatalf("expected tool_use content_block frame to remain, got %s", string(result))
	}
	// Signature should be injected into both thinking and tool_use blocks
	if count := strings.Count(string(result), `"signature":""`); count != 2 {
		t.Fatalf("expected 2 signature injections, but got %d in %s", count, string(result))
	}
}

// TestSanitizeAmpRequestBody_RemovesWhitespaceAndNonStringSignatures 测试请求体清理：
// - 仅含空白的签名的 thinking 块被移除
// - 非字符串签名（数字）的 thinking 块被移除
// - 有效签名的 thinking 块被保留
// - 非 thinking 内容被保留
func TestSanitizeAmpRequestBody_RemovesWhitespaceAndNonStringSignatures(t *testing.T) {
	input := []byte(`{"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"drop-whitespace","signature":"   "},{"type":"thinking","thinking":"drop-number","signature":123},{"type":"thinking","thinking":"keep-valid","signature":"valid-signature"},{"type":"text","text":"keep-text"}]}]}`)
	result := SanitizeAmpRequestBody(input)

	if contains(result, []byte("drop-whitespace")) {
		t.Fatalf("expected whitespace-only signature block to be removed, got %s", string(result))
	}
	if contains(result, []byte("drop-number")) {
		t.Fatalf("expected non-string signature block to be removed, got %s", string(result))
	}
	if !contains(result, []byte("keep-valid")) {
		t.Fatalf("expected valid thinking block to remain, got %s", string(result))
	}
	if !contains(result, []byte("keep-text")) {
		t.Fatalf("expected non-thinking content to remain, got %s", string(result))
	}
}

// TestSanitizeAmpRequestBody_StripsSignatureFromToolUseBlocks 测试从 tool_use 块中剥离签名字段
func TestSanitizeAmpRequestBody_StripsSignatureFromToolUseBlocks(t *testing.T) {
	input := []byte(`{"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"thought","signature":"valid-sig"},{"type":"tool_use","id":"toolu_01","name":"Bash","input":{"cmd":"ls"},"signature":""}]}]}`)
	result := SanitizeAmpRequestBody(input)

	if contains(result, []byte(`"signature":""`)) {
		t.Fatalf("expected signature to be stripped from tool_use block, got %s", string(result))
	}
	if !contains(result, []byte(`"valid-sig"`)) {
		t.Fatalf("expected thinking signature to remain, got %s", string(result))
	}
	if !contains(result, []byte(`"tool_use"`)) {
		t.Fatalf("expected tool_use block to remain, got %s", string(result))
	}
}

// TestSanitizeAmpRequestBody_MixedInvalidThinkingAndToolUseSignature 测试混合场景：
// 无效 thinking 块被移除，tool_use 块的签名被剥离
func TestSanitizeAmpRequestBody_MixedInvalidThinkingAndToolUseSignature(t *testing.T) {
	input := []byte(`{"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"drop-me","signature":""},{"type":"tool_use","id":"toolu_01","name":"Bash","input":{"cmd":"ls"},"signature":""}]}]}`)
	result := SanitizeAmpRequestBody(input)

	if contains(result, []byte("drop-me")) {
		t.Fatalf("expected invalid thinking block to be removed, got %s", string(result))
	}
	if contains(result, []byte(`"signature"`)) {
		t.Fatalf("expected signature to be stripped from tool_use block, got %s", string(result))
	}
	if !contains(result, []byte(`"tool_use"`)) {
		t.Fatalf("expected tool_use block to remain, got %s", string(result))
	}
}

// TestNormalizeAmpToolNames_NonStream 测试非流式模式下的工具名称规范化：
// bash->Bash、read->Read
func TestNormalizeAmpToolNames_NonStream(t *testing.T) {
	input := []byte(`{"content":[{"type":"tool_use","id":"toolu_01","name":"bash","input":{"cmd":"ls"}},{"type":"tool_use","id":"toolu_02","name":"read","input":{"path":"/tmp"}},{"type":"text","text":"hello"}]}`)
	result := normalizeAmpToolNames(input)

	if !contains(result, []byte(`"name":"Bash"`)) {
		t.Errorf("expected bash->Bash, got %s", string(result))
	}
	if !contains(result, []byte(`"name":"Read"`)) {
		t.Errorf("expected read->Read, got %s", string(result))
	}
	if contains(result, []byte(`"name":"bash"`)) {
		t.Errorf("expected lowercase bash to be replaced, got %s", string(result))
	}
}

// TestNormalizeAmpToolNames_Streaming 测试流式模式下的工具名称规范化
func TestNormalizeAmpToolNames_Streaming(t *testing.T) {
	input := []byte(`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","name":"grep","id":"toolu_01","input":{}}}`)
	result := normalizeAmpToolNames(input)

	if !contains(result, []byte(`"name":"Grep"`)) {
		t.Errorf("expected grep->Grep in streaming, got %s", string(result))
	}
}

// TestNormalizeAmpToolNames_AlreadyCorrect 测试已正确大小写的工具名不做修改
func TestNormalizeAmpToolNames_AlreadyCorrect(t *testing.T) {
	input := []byte(`{"content":[{"type":"tool_use","id":"toolu_01","name":"Bash","input":{"cmd":"ls"}}]}`)
	result := normalizeAmpToolNames(input)

	if string(result) != string(input) {
		t.Errorf("expected no modification for correctly-cased tool, got %s", string(result))
	}
}

// TestNormalizeAmpToolNames_GlobPreserved 测试 glob 工具名保持小写不修改
func TestNormalizeAmpToolNames_GlobPreserved(t *testing.T) {
	input := []byte(`{"content":[{"type":"tool_use","id":"toolu_01","name":"glob","input":{"pattern":"*.go"}}]}`)
	result := normalizeAmpToolNames(input)

	if string(result) != string(input) {
		t.Errorf("expected glob to remain lowercase, got %s", string(result))
	}
}

// TestNormalizeAmpToolNames_UnknownToolUntouched 测试未知工具名不做修改
func TestNormalizeAmpToolNames_UnknownToolUntouched(t *testing.T) {
	input := []byte(`{"content":[{"type":"tool_use","id":"toolu_01","name":"edit_file","input":{"path":"/tmp/x"}}]}`)
	result := normalizeAmpToolNames(input)

	if string(result) != string(input) {
		t.Errorf("expected no modification for unknown tool, got %s", string(result))
	}
}

// contains 是字节切片包含检查的辅助函数
func contains(data, substr []byte) bool {
	for i := 0; i <= len(data)-len(substr); i++ {
		if string(data[i:i+len(substr)]) == string(substr) {
			return true
		}
	}
	return false
}
