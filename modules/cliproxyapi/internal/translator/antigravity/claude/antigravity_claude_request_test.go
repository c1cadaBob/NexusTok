// Package claude - antigravity_claude_request_test.go
// 测试 Claude 到 Antigravity（Gemini 兼容）请求格式转换功能。
// 覆盖基本结构、角色映射、thinking 块处理（签名缓存/绕过模式）、
// 工具声明与工具选择、工具调用与工具结果转换、
// 图片内容处理、生成参数配置、thinking 配置、
// 以及排序策略（thinking 优先、functionCall 置后）等测试用例。
package claude

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/cache"
	"github.com/tidwall/gjson"
	"google.golang.org/protobuf/encoding/protowire"
)

// testAnthropicNativeSignature 生成一个符合 Anthropic 原生格式的 protobuf 签名用于测试
func testAnthropicNativeSignature(t *testing.T) string {
	t.Helper()

	payload := buildClaudeSignaturePayload(t, 12, uint64Ptr(2), "claude-sonnet-4-6", true)
	signature := base64.StdEncoding.EncodeToString(payload)
	if len(signature) < cache.MinValidSignatureLen {
		t.Fatalf("test signature too short: %d", len(signature))
	}
	return signature
}

// testMinimalAnthropicSignature 生成一个最小化的 Anthropic 签名用于测试
func testMinimalAnthropicSignature(t *testing.T) string {
	t.Helper()

	payload := buildClaudeSignaturePayload(t, 12, nil, "", false)
	return base64.StdEncoding.EncodeToString(payload)
}

// buildClaudeSignaturePayload 使用 protobuf 编码构建 Claude 签名的二进制载荷
func buildClaudeSignaturePayload(t *testing.T, channelID uint64, field2 *uint64, modelText string, includeField7 bool) []byte {
	t.Helper()

	channelBlock := []byte{}
	channelBlock = protowire.AppendTag(channelBlock, 1, protowire.VarintType)
	channelBlock = protowire.AppendVarint(channelBlock, channelID)
	if field2 != nil {
		channelBlock = protowire.AppendTag(channelBlock, 2, protowire.VarintType)
		channelBlock = protowire.AppendVarint(channelBlock, *field2)
	}
	if modelText != "" {
		channelBlock = protowire.AppendTag(channelBlock, 6, protowire.BytesType)
		channelBlock = protowire.AppendString(channelBlock, modelText)
	}
	if includeField7 {
		channelBlock = protowire.AppendTag(channelBlock, 7, protowire.VarintType)
		channelBlock = protowire.AppendVarint(channelBlock, 0)
	}

	container := []byte{}
	container = protowire.AppendTag(container, 1, protowire.BytesType)
	container = protowire.AppendBytes(container, channelBlock)
	container = protowire.AppendTag(container, 2, protowire.BytesType)
	container = protowire.AppendBytes(container, bytes.Repeat([]byte{0x11}, 12))
	container = protowire.AppendTag(container, 3, protowire.BytesType)
	container = protowire.AppendBytes(container, bytes.Repeat([]byte{0x22}, 12))
	container = protowire.AppendTag(container, 4, protowire.BytesType)
	container = protowire.AppendBytes(container, bytes.Repeat([]byte{0x33}, 48))

	payload := []byte{}
	payload = protowire.AppendTag(payload, 2, protowire.BytesType)
	payload = protowire.AppendBytes(payload, container)
	payload = protowire.AppendTag(payload, 3, protowire.VarintType)
	payload = protowire.AppendVarint(payload, 1)
	return payload
}

// uint64Ptr 返回 uint64 值的指针，用于测试辅助
func uint64Ptr(v uint64) *uint64 {
	return &v
}

// TestConvertClaudeRequestToAntigravity_StripsClaudeCodeAttribution 测试
// Claude Code 归属信息头应从系统指令中被剥离
func TestConvertClaudeRequestToAntigravity_StripsClaudeCodeAttribution(t *testing.T) {
	inputJSON := []byte(`{
		"model": "claude-sonnet-4-5",
		"messages": [{"role": "user", "content": [{"type": "text", "text": "Hello"}]}],
		"system": [
			{"type": "text", "text": "x-anthropic-billing-header: cc_version=2.1.63.abc; cc_entrypoint=cli; cch=12345;"},
			{"type": "text", "text": "Antigravity system prompt"}
		]
	}`)

	output := ConvertClaudeRequestToAntigravity("claude-sonnet-4-5", inputJSON, false)
	outputStr := string(output)

	parts := gjson.Get(outputStr, "request.systemInstruction.parts").Array()
	if len(parts) != 1 {
		t.Fatalf("Expected 1 system part after attribution strip, got %d: %s", len(parts), gjson.Get(outputStr, "request.systemInstruction.parts").Raw)
	}
	if got := parts[0].Get("text").String(); got != "Antigravity system prompt" {
		t.Fatalf("Unexpected system part: %q", got)
	}
}

// testNonAnthropicRawSignature 生成一个非 Anthropic 格式的原始签名用于测试
func testNonAnthropicRawSignature(t *testing.T) string {
	t.Helper()

	payload := bytes.Repeat([]byte{0x34}, 48)
	signature := base64.StdEncoding.EncodeToString(payload)
	if len(signature) < cache.MinValidSignatureLen {
		t.Fatalf("test signature too short: %d", len(signature))
	}
	return signature
}

// testGeminiRawSignature 生成一个 Gemini 格式的原始签名用于测试
func testGeminiRawSignature(t *testing.T) string {
	t.Helper()

	payload := append([]byte{0x0A}, bytes.Repeat([]byte{0x56}, 48)...)
	signature := base64.StdEncoding.EncodeToString(payload)
	if len(signature) < cache.MinValidSignatureLen {
		t.Fatalf("test signature too short: %d", len(signature))
	}
	return signature
}

// TestConvertClaudeRequestToAntigravity_BasicStructure 测试基本的请求结构转换，
// 包括模型名称、contents 数组、角色映射和系统指令
func TestConvertClaudeRequestToAntigravity_BasicStructure(t *testing.T) {
	inputJSON := []byte(`{
		"model": "claude-3-5-sonnet-20240620",
		"messages": [
			{
				"role": "user",
				"content": [
					{"type": "text", "text": "Hello"}
				]
			}
		],
		"system": [
			{"type": "text", "text": "You are helpful"}
		]
	}`)

	output := ConvertClaudeRequestToAntigravity("claude-sonnet-4-5", inputJSON, false)
	outputStr := string(output)

	// Check model
	if gjson.Get(outputStr, "model").String() != "claude-sonnet-4-5" {
		t.Errorf("Expected model 'claude-sonnet-4-5', got '%s'", gjson.Get(outputStr, "model").String())
	}

	// Check contents exist
	contents := gjson.Get(outputStr, "request.contents")
	if !contents.Exists() || !contents.IsArray() {
		t.Error("request.contents should exist and be an array")
	}

	// Check role mapping (assistant -> model)
	firstContent := gjson.Get(outputStr, "request.contents.0")
	if firstContent.Get("role").String() != "user" {
		t.Errorf("Expected role 'user', got '%s'", firstContent.Get("role").String())
	}

	// Check systemInstruction
	sysInstruction := gjson.Get(outputStr, "request.systemInstruction")
	if !sysInstruction.Exists() {
		t.Error("systemInstruction should exist")
	}
	if sysInstruction.Get("parts.0.text").String() != "You are helpful" {
		t.Error("systemInstruction text mismatch")
	}
}

// TestConvertClaudeRequestToAntigravity_RoleMapping 测试角色映射：assistant -> model
func TestConvertClaudeRequestToAntigravity_RoleMapping(t *testing.T) {
	inputJSON := []byte(`{
		"model": "claude-3-5-sonnet-20240620",
		"messages": [
			{"role": "user", "content": [{"type": "text", "text": "Hi"}]},
			{"role": "assistant", "content": [{"type": "text", "text": "Hello"}]}
		]
	}`)

	output := ConvertClaudeRequestToAntigravity("claude-sonnet-4-5", inputJSON, false)
	outputStr := string(output)

	// assistant should be mapped to model
	secondContent := gjson.Get(outputStr, "request.contents.1")
	if secondContent.Get("role").String() != "model" {
		t.Errorf("Expected role 'model' (mapped from 'assistant'), got '%s'", secondContent.Get("role").String())
	}
}

// TestConvertClaudeRequestToAntigravity_ThinkingBlocks 测试 thinking 块的转换，
// 包括签名缓存和 thoughtSignature 字段的正确设置
func TestConvertClaudeRequestToAntigravity_ThinkingBlocks(t *testing.T) {
	cache.ClearSignatureCache("")

	// Valid signature must be at least 50 characters
	validSignature := "abc123validSignature1234567890123456789012345678901234567890"
	thinkingText := "Let me think..."

	// Pre-cache the signature (simulating a previous response for the same thinking text)
	inputJSON := []byte(`{
		"model": "claude-sonnet-4-5-thinking",
		"messages": [
			{
				"role": "user",
				"content": [{"type": "text", "text": "Test user message"}]
			},
			{
				"role": "assistant",
				"content": [
					{"type": "thinking", "thinking": "` + thinkingText + `", "signature": "` + validSignature + `"},
					{"type": "text", "text": "Answer"}
				]
			}
		]
	}`)

	cache.CacheSignature("claude-sonnet-4-5-thinking", thinkingText, validSignature)

	output := ConvertClaudeRequestToAntigravity("claude-sonnet-4-5-thinking", inputJSON, false)
	outputStr := string(output)

	// Check thinking block conversion (now in contents.1 due to user message)
	firstPart := gjson.Get(outputStr, "request.contents.1.parts.0")
	if !firstPart.Get("thought").Bool() {
		t.Error("thinking block should have thought: true")
	}
	if firstPart.Get("text").String() != thinkingText {
		t.Error("thinking text mismatch")
	}
	if firstPart.Get("thoughtSignature").String() != validSignature {
		t.Errorf("Expected thoughtSignature '%s', got '%s'", validSignature, firstPart.Get("thoughtSignature").String())
	}
}

// TestValidateBypassMode_AcceptsClaudeSingleAndDoubleLayer 测试绕过模式验证器
// 应接受单层和双层编码的 Claude 原生签名
func TestValidateBypassMode_AcceptsClaudeSingleAndDoubleLayer(t *testing.T) {
	rawSignature := testAnthropicNativeSignature(t)
	doubleEncoded := base64.StdEncoding.EncodeToString([]byte(rawSignature))

	inputJSON := []byte(`{
		"messages": [
			{
				"role": "assistant",
				"content": [
					{"type": "thinking", "thinking": "one", "signature": "` + rawSignature + `"},
					{"type": "thinking", "thinking": "two", "signature": "claude#` + doubleEncoded + `"}
				]
			}
		]
	}`)

	if err := ValidateClaudeBypassSignatures(inputJSON); err != nil {
		t.Fatalf("ValidateBypassModeSignatures returned error: %v", err)
	}
}

// TestValidateBypassMode_RejectsGeminiSignature 测试应拒绝 Gemini 格式的签名
func TestValidateBypassMode_RejectsGeminiSignature(t *testing.T) {
	inputJSON := []byte(`{
		"messages": [
			{
				"role": "assistant",
				"content": [
					{"type": "thinking", "thinking": "one", "signature": "` + testGeminiRawSignature(t) + `"}
				]
			}
		]
	}`)

	err := ValidateClaudeBypassSignatures(inputJSON)
	if err == nil {
		t.Fatal("expected Gemini signature to be rejected")
	}
}

// TestValidateBypassMode_RejectsMissingSignature 测试应拒绝缺少签名的 thinking 块
func TestValidateBypassMode_RejectsMissingSignature(t *testing.T) {
	inputJSON := []byte(`{
		"messages": [
			{
				"role": "assistant",
				"content": [
					{"type": "thinking", "thinking": "one"}
				]
			}
		]
	}`)

	err := ValidateClaudeBypassSignatures(inputJSON)
	if err == nil {
		t.Fatal("expected missing signature to be rejected")
	}
	if !strings.Contains(err.Error(), "missing thinking signature") {
		t.Fatalf("expected missing signature message, got: %v", err)
	}
}

// TestValidateBypassMode_RejectsNonREPrefix 测试应拒绝非 R/E 前缀的签名
func TestValidateBypassMode_RejectsNonREPrefix(t *testing.T) {
	inputJSON := []byte(`{
		"messages": [
			{
				"role": "assistant",
				"content": [
					{"type": "thinking", "thinking": "one", "signature": "` + testNonAnthropicRawSignature(t) + `"}
				]
			}
		]
	}`)

	err := ValidateClaudeBypassSignatures(inputJSON)
	if err == nil {
		t.Fatal("expected non-R/E signature to be rejected")
	}
}

// TestValidateBypassMode_RejectsEPrefixWrongFirstByte 测试
// E 前缀但首字节不是 0x80 的签名应被拒绝
func TestValidateBypassMode_RejectsEPrefixWrongFirstByte(t *testing.T) {
	t.Parallel()
	payload := append([]byte{0x10}, bytes.Repeat([]byte{0x34}, 48)...)
	sig := base64.StdEncoding.EncodeToString(payload)
	if sig[0] != 'E' {
		t.Fatalf("test setup: expected E prefix, got %c", sig[0])
	}

	inputJSON := []byte(`{
		"messages": [{"role": "assistant", "content": [
			{"type": "thinking", "thinking": "t", "signature": "` + sig + `"}
		]}]
	}`)

	err := ValidateClaudeBypassSignatures(inputJSON)
	if err == nil {
		t.Fatal("expected E-prefix with wrong first byte (0x10) to be rejected")
	}
	if !strings.Contains(err.Error(), "0x10") {
		t.Fatalf("expected error to mention 0x10, got: %v", err)
	}
}

// TestValidateBypassMode_RejectsTopLevel12WithoutClaudeTree 测试
// 严格模式下应拒绝没有 Claude protobuf 树的 0x12 顶层签名
func TestValidateBypassMode_RejectsTopLevel12WithoutClaudeTree(t *testing.T) {
	previous := cache.SignatureBypassStrictMode()
	cache.SetSignatureBypassStrictMode(true)
	t.Cleanup(func() {
		cache.SetSignatureBypassStrictMode(previous)
	})

	payload := append([]byte{0x12}, bytes.Repeat([]byte{0x34}, 48)...)
	sig := base64.StdEncoding.EncodeToString(payload)

	inputJSON := []byte(`{
		"messages": [{"role": "assistant", "content": [
			{"type": "thinking", "thinking": "t", "signature": "` + sig + `"}
		]}]
	}`)

	err := ValidateClaudeBypassSignatures(inputJSON)
	if err == nil {
		t.Fatal("expected non-Claude protobuf tree to be rejected in strict mode")
	}
	if !strings.Contains(err.Error(), "malformed protobuf") && !strings.Contains(err.Error(), "Field 2") {
		t.Fatalf("expected protobuf tree error, got: %v", err)
	}
}

// TestValidateBypassMode_NonStrictAccepts12WithoutClaudeTree 测试
// 非严格模式下应接受没有 Claude protobuf 树的 0x12 顶层签名
func TestValidateBypassMode_NonStrictAccepts12WithoutClaudeTree(t *testing.T) {
	previous := cache.SignatureBypassStrictMode()
	cache.SetSignatureBypassStrictMode(false)
	t.Cleanup(func() {
		cache.SetSignatureBypassStrictMode(previous)
	})

	payload := append([]byte{0x12}, bytes.Repeat([]byte{0x34}, 48)...)
	sig := base64.StdEncoding.EncodeToString(payload)

	inputJSON := []byte(`{
		"messages": [{"role": "assistant", "content": [
			{"type": "thinking", "thinking": "t", "signature": "` + sig + `"}
		]}]
	}`)

	err := ValidateClaudeBypassSignatures(inputJSON)
	if err != nil {
		t.Fatalf("non-strict mode should accept 0x12 without protobuf tree, got: %v", err)
	}
}

// TestValidateBypassMode_RejectsRPrefixInnerNotE 测试
// R 前缀但内部签名不是 E 前缀的应被拒绝
func TestValidateBypassMode_RejectsRPrefixInnerNotE(t *testing.T) {
	t.Parallel()
	inner := "F" + strings.Repeat("a", 60)
	outer := base64.StdEncoding.EncodeToString([]byte(inner))
	if outer[0] != 'R' {
		t.Fatalf("test setup: expected R prefix, got %c", outer[0])
	}

	inputJSON := []byte(`{
		"messages": [{"role": "assistant", "content": [
			{"type": "thinking", "thinking": "t", "signature": "` + outer + `"}
		]}]
	}`)

	err := ValidateClaudeBypassSignatures(inputJSON)
	if err == nil {
		t.Fatal("expected R-prefix with non-E inner to be rejected")
	}
}

// TestValidateBypassMode_RejectsInvalidBase64 测试应拒绝无效 base64 编码的签名
func TestValidateBypassMode_RejectsInvalidBase64(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		sig  string
	}{
		{"E invalid", "E!!!invalid!!!"},
		{"R invalid", "R$$$invalid$$$"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			inputJSON := []byte(`{
				"messages": [{"role": "assistant", "content": [
					{"type": "thinking", "thinking": "t", "signature": "` + tt.sig + `"}
				]}]
			}`)
			err := ValidateClaudeBypassSignatures(inputJSON)
			if err == nil {
				t.Fatal("expected invalid base64 to be rejected")
			}
			if !strings.Contains(err.Error(), "base64") {
				t.Fatalf("expected base64 error, got: %v", err)
			}
		})
	}
}

// TestValidateBypassMode_RejectsPrefixStrippedToEmpty 测试
// 仅有前缀（如 "claude#"）的签名应被拒绝
func TestValidateBypassMode_RejectsPrefixStrippedToEmpty(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		sig  string
	}{
		{"prefix only", "claude#"},
		{"prefix with spaces", "claude#   "},
		{"hash only", "#"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			inputJSON := []byte(`{
				"messages": [{"role": "assistant", "content": [
					{"type": "thinking", "thinking": "t", "signature": "` + tt.sig + `"}
				]}]
			}`)
			err := ValidateClaudeBypassSignatures(inputJSON)
			if err == nil {
				t.Fatal("expected prefix-only signature to be rejected")
			}
		})
	}
}

// TestValidateBypassMode_HandlesMultipleHashMarks 测试
// 包含多个 # 字符的签名应被拒绝（无效 base64）
func TestValidateBypassMode_HandlesMultipleHashMarks(t *testing.T) {
	t.Parallel()
	rawSignature := testAnthropicNativeSignature(t)
	sig := "claude#" + rawSignature + "#extra"

	inputJSON := []byte(`{
		"messages": [{"role": "assistant", "content": [
			{"type": "thinking", "thinking": "t", "signature": "` + sig + `"}
		]}]
	}`)

	err := ValidateClaudeBypassSignatures(inputJSON)
	if err == nil {
		t.Fatal("expected signature with trailing # to be rejected (invalid base64)")
	}
}

// TestValidateBypassMode_HandlesWhitespace 测试
// 包含前导/尾随空白的签名应被正确处理（空白被修剪后验证通过）
func TestValidateBypassMode_HandlesWhitespace(t *testing.T) {
	t.Parallel()
	rawSignature := testAnthropicNativeSignature(t)
	tests := []struct {
		name string
		sig  string
	}{
		{"leading space", " " + rawSignature},
		{"trailing space", rawSignature + " "},
		{"both spaces", " " + rawSignature + " "},
		{"leading tab", "\t" + rawSignature},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			inputJSON := []byte(`{
				"messages": [{"role": "assistant", "content": [
					{"type": "thinking", "thinking": "t", "signature": "` + tt.sig + `"}
				]}]
			}`)
			if err := ValidateClaudeBypassSignatures(inputJSON); err != nil {
				t.Fatalf("expected whitespace-padded signature to be accepted, got: %v", err)
			}
		})
	}
}

// TestValidateBypassMode_RejectsOversizedSignature 测试
// 超过最大长度限制的签名应被拒绝
func TestValidateBypassMode_RejectsOversizedSignature(t *testing.T) {
	t.Parallel()
	sig := strings.Repeat("A", maxBypassSignatureLen+1)

	inputJSON := []byte(`{
		"messages": [{"role": "assistant", "content": [
			{"type": "thinking", "thinking": "t", "signature": "` + sig + `"}
		]}]
	}`)

	err := ValidateClaudeBypassSignatures(inputJSON)
	if err == nil {
		t.Fatal("expected oversized signature to be rejected")
	}
	if !strings.Contains(err.Error(), "maximum length") {
		t.Fatalf("expected length error, got: %v", err)
	}
}

// TestValidateBypassMode_StrictAcceptsSignatureBetween16KiBAnd32MiB 测试
// 严格模式下应接受 16KiB 到 32MiB 之间的签名
func TestValidateBypassMode_StrictAcceptsSignatureBetween16KiBAnd32MiB(t *testing.T) {
	previous := cache.SignatureBypassStrictMode()
	cache.SetSignatureBypassStrictMode(true)
	t.Cleanup(func() {
		cache.SetSignatureBypassStrictMode(previous)
	})

	payload := buildClaudeSignaturePayload(t, 12, uint64Ptr(2), strings.Repeat("m", 20000), true)
	sig := base64.StdEncoding.EncodeToString(payload)
	if len(sig) <= 1<<14 {
		t.Fatalf("test setup: signature should exceed previous 16KiB guardrail, got %d", len(sig))
	}
	if len(sig) > maxBypassSignatureLen {
		t.Fatalf("test setup: signature should remain within new max length, got %d", len(sig))
	}

	inputJSON := []byte(`{
		"messages": [{"role": "assistant", "content": [
			{"type": "thinking", "thinking": "t", "signature": "` + sig + `"}
		]}]
	}`)

	if err := ValidateClaudeBypassSignatures(inputJSON); err != nil {
		t.Fatalf("expected strict mode to accept signature below 32MiB max, got: %v", err)
	}
}

// TestResolveBypassModeSignature_TrimsWhitespace 测试
// 签名解析函数应正确修剪尾随空白
func TestResolveBypassModeSignature_TrimsWhitespace(t *testing.T) {
	previous := cache.SignatureCacheEnabled()
	cache.SetSignatureCacheEnabled(false)
	t.Cleanup(func() {
		cache.SetSignatureCacheEnabled(previous)
	})

	rawSignature := testAnthropicNativeSignature(t)
	expected := resolveBypassModeSignature(rawSignature)
	if expected == "" {
		t.Fatal("test setup: expected non-empty normalized signature")
	}

	got := resolveBypassModeSignature(rawSignature + "  ")
	if got != expected {
		t.Fatalf("expected trailing whitespace to be trimmed:\n  got:  %q\n  want: %q", got, expected)
	}
}

// TestConvertClaudeRequestToAntigravity_BypassModeNormalizesESignature 测试
// 绕过模式下 E 前缀签名应被标准化为双层编码格式
func TestConvertClaudeRequestToAntigravity_BypassModeNormalizesESignature(t *testing.T) {
	cache.ClearSignatureCache("")
	previous := cache.SignatureCacheEnabled()
	cache.SetSignatureCacheEnabled(false)
	t.Cleanup(func() {
		cache.SetSignatureCacheEnabled(previous)
		cache.ClearSignatureCache("")
	})

	thinkingText := "Let me think..."
	cachedSignature := "cachedSignature1234567890123456789012345678901234567890123"
	rawSignature := testAnthropicNativeSignature(t)
	expectedSignature := base64.StdEncoding.EncodeToString([]byte(rawSignature))

	cache.CacheSignature("claude-sonnet-4-5-thinking", thinkingText, cachedSignature)

	inputJSON := []byte(`{
		"model": "claude-sonnet-4-5-thinking",
		"messages": [
			{
				"role": "assistant",
				"content": [
					{"type": "thinking", "thinking": "` + thinkingText + `", "signature": "` + rawSignature + `"},
					{"type": "text", "text": "Answer"}
				]
			}
		]
	}`)

	output := ConvertClaudeRequestToAntigravity("claude-sonnet-4-5-thinking", inputJSON, false)
	outputStr := string(output)

	part := gjson.Get(outputStr, "request.contents.0.parts.0")
	if part.Get("thoughtSignature").String() != expectedSignature {
		t.Fatalf("Expected bypass-mode signature '%s', got '%s'", expectedSignature, part.Get("thoughtSignature").String())
	}
	if part.Get("thoughtSignature").String() == cachedSignature {
		t.Fatal("Bypass mode should not reuse cached signature")
	}
}

// TestConvertClaudeRequestToAntigravity_BypassModePreservesShortValidSignature 测试
// 绕过模式下短的有效签名应被保留
func TestConvertClaudeRequestToAntigravity_BypassModePreservesShortValidSignature(t *testing.T) {
	cache.ClearSignatureCache("")
	previous := cache.SignatureCacheEnabled()
	cache.SetSignatureCacheEnabled(false)
	t.Cleanup(func() {
		cache.SetSignatureCacheEnabled(previous)
		cache.ClearSignatureCache("")
	})

	rawSignature := testMinimalAnthropicSignature(t)
	expectedSignature := base64.StdEncoding.EncodeToString([]byte(rawSignature))
	inputJSON := []byte(`{
		"model": "claude-sonnet-4-5-thinking",
		"messages": [
			{
				"role": "assistant",
				"content": [
					{"type": "thinking", "thinking": "tiny", "signature": "` + rawSignature + `"},
					{"type": "text", "text": "Answer"}
				]
			}
		]
	}`)

	output := ConvertClaudeRequestToAntigravity("claude-sonnet-4-5-thinking", inputJSON, false)
	parts := gjson.GetBytes(output, "request.contents.0.parts").Array()
	if len(parts) != 2 {
		t.Fatalf("expected thinking part to be preserved in bypass mode, got %d parts", len(parts))
	}
	if parts[0].Get("thoughtSignature").String() != expectedSignature {
		t.Fatalf("expected normalized short signature %q, got %q", expectedSignature, parts[0].Get("thoughtSignature").String())
	}
	if !parts[0].Get("thought").Bool() {
		t.Fatalf("expected first part to remain a thought block, got %s", parts[0].Raw)
	}
	if parts[1].Get("text").String() != "Answer" {
		t.Fatalf("expected trailing text part, got %s", parts[1].Raw)
	}
	if thoughtSig := gjson.GetBytes(output, "request.contents.0.parts.1.thoughtSignature").String(); thoughtSig != "" {
		t.Fatalf("expected plain text part to have no thought signature, got %q", thoughtSig)
	}
	if functionSig := gjson.GetBytes(output, "request.contents.0.parts.0.functionCall.thoughtSignature").String(); functionSig != "" {
		t.Fatalf("unexpected functionCall payload in thinking part: %q", functionSig)
	}
}

// TestInspectClaudeSignaturePayload_ExtractsSpecTree 测试
// 签名载荷检查函数应能正确提取结构化的 Claude 路由信息
func TestInspectClaudeSignaturePayload_ExtractsSpecTree(t *testing.T) {
	t.Parallel()
	payload := buildClaudeSignaturePayload(t, 12, uint64Ptr(2), "claude-sonnet-4-6", true)

	tree, err := inspectClaudeSignaturePayload(payload, 1)
	if err != nil {
		t.Fatalf("expected structured Claude payload to parse, got: %v", err)
	}
	if tree.RoutingClass != "routing_class_12" {
		t.Fatalf("routing_class = %q, want routing_class_12", tree.RoutingClass)
	}
	if tree.InfrastructureClass != "infra_google" {
		t.Fatalf("infrastructure_class = %q, want infra_google", tree.InfrastructureClass)
	}
	if tree.SchemaFeatures != "extended_model_tagged_schema" {
		t.Fatalf("schema_features = %q, want extended_model_tagged_schema", tree.SchemaFeatures)
	}
	if tree.ModelText != "claude-sonnet-4-6" {
		t.Fatalf("model_text = %q, want claude-sonnet-4-6", tree.ModelText)
	}
}

// TestInspectDoubleLayerSignature_TracksEncodingLayers 测试
// 双层签名检查函数应正确追踪编码层数
func TestInspectDoubleLayerSignature_TracksEncodingLayers(t *testing.T) {
	t.Parallel()
	inner := base64.StdEncoding.EncodeToString(buildClaudeSignaturePayload(t, 11, uint64Ptr(2), "", false))
	outer := base64.StdEncoding.EncodeToString([]byte(inner))

	tree, err := inspectDoubleLayerSignature(outer)
	if err != nil {
		t.Fatalf("expected double-layer Claude signature to parse, got: %v", err)
	}
	if tree.EncodingLayers != 2 {
		t.Fatalf("encoding_layers = %d, want 2", tree.EncodingLayers)
	}
	if tree.LegacyRouteHint != "legacy_vertex_direct" {
		t.Fatalf("legacy_route_hint = %q, want legacy_vertex_direct", tree.LegacyRouteHint)
	}
}

// TestConvertClaudeRequestToAntigravity_CacheModeDropsRawSignature 测试
// 缓存模式下原始签名的 thinking 块应被丢弃（仅保留文本部分）
func TestConvertClaudeRequestToAntigravity_CacheModeDropsRawSignature(t *testing.T) {
	cache.ClearSignatureCache("")
	previous := cache.SignatureCacheEnabled()
	cache.SetSignatureCacheEnabled(true)
	t.Cleanup(func() {
		cache.SetSignatureCacheEnabled(previous)
		cache.ClearSignatureCache("")
	})

	rawSignature := testAnthropicNativeSignature(t)
	inputJSON := []byte(`{
		"model": "claude-sonnet-4-5-thinking",
		"messages": [
			{
				"role": "assistant",
				"content": [
					{"type": "thinking", "thinking": "Let me think...", "signature": "` + rawSignature + `"},
					{"type": "text", "text": "Answer"}
				]
			}
		]
	}`)

	output := ConvertClaudeRequestToAntigravity("claude-sonnet-4-5-thinking", inputJSON, false)
	parts := gjson.GetBytes(output, "request.contents.0.parts").Array()
	if len(parts) != 1 {
		t.Fatalf("Expected raw signature thinking block to be dropped in cache mode, got %d parts", len(parts))
	}
	if parts[0].Get("text").String() != "Answer" {
		t.Fatalf("Expected remaining text part, got %s", parts[0].Raw)
	}
}

// TestConvertClaudeRequestToAntigravity_BypassModeDropsInvalidSignature 测试
// 绕过模式下无效签名的 thinking 块应被丢弃
func TestConvertClaudeRequestToAntigravity_BypassModeDropsInvalidSignature(t *testing.T) {
	cache.ClearSignatureCache("")
	previous := cache.SignatureCacheEnabled()
	cache.SetSignatureCacheEnabled(false)
	t.Cleanup(func() {
		cache.SetSignatureCacheEnabled(previous)
		cache.ClearSignatureCache("")
	})

	invalidRawSignature := testNonAnthropicRawSignature(t)
	inputJSON := []byte(`{
		"model": "claude-sonnet-4-5-thinking",
		"messages": [
			{
				"role": "assistant",
				"content": [
					{"type": "thinking", "thinking": "Let me think...", "signature": "` + invalidRawSignature + `"},
					{"type": "text", "text": "Answer"}
				]
			}
		]
	}`)

	output := ConvertClaudeRequestToAntigravity("claude-sonnet-4-5-thinking", inputJSON, false)
	outputStr := string(output)

	parts := gjson.Get(outputStr, "request.contents.0.parts").Array()
	if len(parts) != 1 {
		t.Fatalf("Expected invalid thinking block to be removed, got %d parts", len(parts))
	}
	if parts[0].Get("text").String() != "Answer" {
		t.Fatalf("Expected remaining text part, got %s", parts[0].Raw)
	}
	if parts[0].Get("thought").Bool() {
		t.Fatal("Invalid raw signature should not preserve thinking block")
	}
}

// TestConvertClaudeRequestToAntigravity_BypassModeDropsGeminiSignature 测试
// 绕过模式下 Gemini 格式签名的 thinking 块应被丢弃
func TestConvertClaudeRequestToAntigravity_BypassModeDropsGeminiSignature(t *testing.T) {
	cache.ClearSignatureCache("")
	previous := cache.SignatureCacheEnabled()
	cache.SetSignatureCacheEnabled(false)
	t.Cleanup(func() {
		cache.SetSignatureCacheEnabled(previous)
		cache.ClearSignatureCache("")
	})

	geminiPayload := append([]byte{0x0A}, bytes.Repeat([]byte{0x56}, 48)...)
	geminiSig := base64.StdEncoding.EncodeToString(geminiPayload)
	inputJSON := []byte(`{
		"model": "claude-sonnet-4-5-thinking",
		"messages": [
			{
				"role": "assistant",
				"content": [
					{"type": "thinking", "thinking": "hmm", "signature": "` + geminiSig + `"},
					{"type": "text", "text": "Answer"}
				]
			}
		]
	}`)

	output := ConvertClaudeRequestToAntigravity("claude-sonnet-4-5-thinking", inputJSON, false)
	parts := gjson.GetBytes(output, "request.contents.0.parts").Array()
	if len(parts) != 1 {
		t.Fatalf("expected Gemini-signed thinking block to be dropped, got %d parts", len(parts))
	}
	if parts[0].Get("text").String() != "Answer" {
		t.Fatalf("expected remaining text part, got %s", parts[0].Raw)
	}
}

// TestConvertClaudeRequestToAntigravity_ThinkingBlockWithoutSignature 测试
// 无签名的 thinking 块应被完全移除（不转换为文本）
func TestConvertClaudeRequestToAntigravity_ThinkingBlockWithoutSignature(t *testing.T) {
	cache.ClearSignatureCache("")

	// Unsigned thinking blocks should be removed entirely (not converted to text)
	inputJSON := []byte(`{
		"model": "claude-sonnet-4-5-thinking",
		"messages": [
			{
				"role": "assistant",
				"content": [
					{"type": "thinking", "thinking": "Let me think..."},
					{"type": "text", "text": "Answer"}
				]
			}
		]
	}`)

	output := ConvertClaudeRequestToAntigravity("claude-sonnet-4-5-thinking", inputJSON, false)
	outputStr := string(output)

	// Without signature, thinking block should be removed (not converted to text)
	parts := gjson.Get(outputStr, "request.contents.0.parts").Array()
	if len(parts) != 1 {
		t.Fatalf("Expected 1 part (thinking removed), got %d", len(parts))
	}

	// Only text part should remain
	if parts[0].Get("thought").Bool() {
		t.Error("Thinking block should be removed, not preserved")
	}
	if parts[0].Get("text").String() != "Answer" {
		t.Errorf("Expected text 'Answer', got '%s'", parts[0].Get("text").String())
	}
}

// TestConvertClaudeRequestToAntigravity_ToolDeclarations 测试工具声明的转换，
// 包括 input_schema 到 parametersJsonSchema 的重命名
func TestConvertClaudeRequestToAntigravity_ToolDeclarations(t *testing.T) {
	inputJSON := []byte(`{
		"model": "claude-3-5-sonnet-20240620",
		"messages": [],
		"tools": [
			{
				"name": "test_tool",
				"description": "A test tool",
				"input_schema": {
					"type": "object",
					"properties": {
						"name": {"type": "string"}
					},
					"required": ["name"]
				}
			}
		]
	}`)

	output := ConvertClaudeRequestToAntigravity("gemini-1.5-pro", inputJSON, false)
	outputStr := string(output)

	// Check tools structure
	tools := gjson.Get(outputStr, "request.tools")
	if !tools.Exists() {
		t.Error("Tools should exist in output")
	}

	funcDecl := gjson.Get(outputStr, "request.tools.0.functionDeclarations.0")
	if funcDecl.Get("name").String() != "test_tool" {
		t.Errorf("Expected tool name 'test_tool', got '%s'", funcDecl.Get("name").String())
	}

	// Check input_schema renamed to parametersJsonSchema
	if funcDecl.Get("parametersJsonSchema").Exists() {
		t.Log("parametersJsonSchema exists (expected)")
	}
	if funcDecl.Get("input_schema").Exists() {
		t.Error("input_schema should be removed")
	}
}

// TestConvertClaudeRequestToAntigravity_ToolChoice_SpecificTool 测试指定工具选择到
// Antigravity toolConfig 的映射
func TestConvertClaudeRequestToAntigravity_ToolChoice_SpecificTool(t *testing.T) {
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

	output := ConvertClaudeRequestToAntigravity("gemini-3-flash-preview", inputJSON, false)
	outputStr := string(output)

	if got := gjson.Get(outputStr, "request.toolConfig.functionCallingConfig.mode").String(); got != "ANY" {
		t.Fatalf("Expected toolConfig.functionCallingConfig.mode 'ANY', got '%s'", got)
	}
	allowed := gjson.Get(outputStr, "request.toolConfig.functionCallingConfig.allowedFunctionNames").Array()
	if len(allowed) != 1 || allowed[0].String() != "json" {
		t.Fatalf("Expected allowedFunctionNames ['json'], got %s", gjson.Get(outputStr, "request.toolConfig.functionCallingConfig.allowedFunctionNames").Raw)
	}
}

// TestConvertClaudeRequestToAntigravity_ToolUse 测试工具调用的转换，
// 包括 functionCall 结构和 skip_thought_signature_validator 的注入
func TestConvertClaudeRequestToAntigravity_ToolUse(t *testing.T) {
	inputJSON := []byte(`{
		"model": "claude-3-5-sonnet-20240620",
		"messages": [
			{
				"role": "assistant",
				"content": [
					{
						"type": "tool_use",
						"id": "call_123",
						"name": "get_weather",
						"input": "{\"location\": \"Paris\"}"
					}
				]
			}
		]
	}`)

	output := ConvertClaudeRequestToAntigravity("claude-sonnet-4-5", inputJSON, false)
	outputStr := string(output)

	// Now we expect only 1 part (tool_use), no dummy thinking block injected
	parts := gjson.Get(outputStr, "request.contents.0.parts").Array()
	if len(parts) != 1 {
		t.Fatalf("Expected 1 part (tool only, no dummy injection), got %d", len(parts))
	}

	// Check function call conversion at parts[0]
	funcCall := parts[0].Get("functionCall")
	if !funcCall.Exists() {
		t.Error("functionCall should exist at parts[0]")
	}
	if funcCall.Get("name").String() != "get_weather" {
		t.Errorf("Expected function name 'get_weather', got '%s'", funcCall.Get("name").String())
	}
	if funcCall.Get("id").String() != "call_123" {
		t.Errorf("Expected function id 'call_123', got '%s'", funcCall.Get("id").String())
	}
	// Verify skip_thought_signature_validator is added (bypass for tools without valid thinking)
	expectedSig := "skip_thought_signature_validator"
	actualSig := parts[0].Get("thoughtSignature").String()
	if actualSig != expectedSig {
		t.Errorf("Expected thoughtSignature '%s', got '%s'", expectedSig, actualSig)
	}
}

// TestConvertClaudeRequestToAntigravity_ToolUse_WithSignature 测试带签名的工具调用，
// 签名应从前置 thinking 块传递到工具调用的 thoughtSignature 字段
func TestConvertClaudeRequestToAntigravity_ToolUse_WithSignature(t *testing.T) {
	cache.ClearSignatureCache("")

	validSignature := "abc123validSignature1234567890123456789012345678901234567890"
	thinkingText := "Let me think..."

	inputJSON := []byte(`{
		"model": "claude-sonnet-4-5-thinking",
		"messages": [
			{
				"role": "user",
				"content": [{"type": "text", "text": "Test user message"}]
			},
			{
				"role": "assistant",
				"content": [
					{"type": "thinking", "thinking": "` + thinkingText + `", "signature": "` + validSignature + `"},
					{
						"type": "tool_use",
						"id": "call_123",
						"name": "get_weather",
						"input": "{\"location\": \"Paris\"}"
					}
				]
			}
		]
	}`)

	cache.CacheSignature("claude-sonnet-4-5-thinking", thinkingText, validSignature)

	output := ConvertClaudeRequestToAntigravity("claude-sonnet-4-5-thinking", inputJSON, false)
	outputStr := string(output)

	// Check function call has the signature from the preceding thinking block (now in contents.1)
	part := gjson.Get(outputStr, "request.contents.1.parts.1")
	if part.Get("functionCall.name").String() != "get_weather" {
		t.Errorf("Expected functionCall, got %s", part.Raw)
	}
	if part.Get("thoughtSignature").String() != validSignature {
		t.Errorf("Expected thoughtSignature '%s' on tool_use, got '%s'", validSignature, part.Get("thoughtSignature").String())
	}
}

// TestConvertClaudeRequestToAntigravity_ReorderThinking 测试排序策略：
// 当 text 块在 thinking 块之前时，应重新排序为 thinking 在前
func TestConvertClaudeRequestToAntigravity_ReorderThinking(t *testing.T) {
	cache.ClearSignatureCache("")

	// Case: text block followed by thinking block -> should be reordered to thinking first
	validSignature := "abc123validSignature1234567890123456789012345678901234567890"
	thinkingText := "Planning..."

	inputJSON := []byte(`{
		"model": "claude-sonnet-4-5-thinking",
		"messages": [
			{
				"role": "user",
				"content": [{"type": "text", "text": "Test user message"}]
			},
			{
				"role": "assistant",
				"content": [
					{"type": "text", "text": "Here is the plan."},
					{"type": "thinking", "thinking": "` + thinkingText + `", "signature": "` + validSignature + `"}
				]
			}
		]
	}`)

	cache.CacheSignature("claude-sonnet-4-5-thinking", thinkingText, validSignature)

	output := ConvertClaudeRequestToAntigravity("claude-sonnet-4-5-thinking", inputJSON, false)
	outputStr := string(output)

	// Verify order: Thinking block MUST be first (now in contents.1 due to user message)
	parts := gjson.Get(outputStr, "request.contents.1.parts").Array()
	if len(parts) != 2 {
		t.Fatalf("Expected 2 parts, got %d", len(parts))
	}

	if !parts[0].Get("thought").Bool() {
		t.Error("First part should be thinking block after reordering")
	}
	if parts[1].Get("text").String() != "Here is the plan." {
		t.Error("Second part should be text block")
	}
}

// TestConvertClaudeRequestToAntigravity_ReorderTextAfterFunctionCall 测试排序策略：
// functionCall 后的 text 部分应被移到 functionCall 之前，
// 以避免 Antigravity 在 functionCall 边界处拆分消息
func TestConvertClaudeRequestToAntigravity_ReorderTextAfterFunctionCall(t *testing.T) {
	// Bug: text part after tool_use in an assistant message causes Antigravity
	// to split at functionCall boundary, creating an extra assistant turn that
	// breaks tool_use↔tool_result adjacency (upstream issue #989).
	// Fix: reorder parts so functionCall comes last.
	inputJSON := []byte(`{
		"model": "claude-sonnet-4-5",
		"messages": [
			{
				"role": "assistant",
				"content": [
					{"type": "text", "text": "Let me check..."},
					{
						"type": "tool_use",
						"id": "call_abc",
						"name": "Read",
						"input": {"file": "test.go"}
					},
					{"type": "text", "text": "Reading the file now"}
				]
			},
			{
				"role": "user",
				"content": [
					{
						"type": "tool_result",
						"tool_use_id": "call_abc",
						"content": "file content"
					}
				]
			}
		]
	}`)

	output := ConvertClaudeRequestToAntigravity("claude-sonnet-4-5", inputJSON, false)
	outputStr := string(output)

	parts := gjson.Get(outputStr, "request.contents.0.parts").Array()
	if len(parts) != 3 {
		t.Fatalf("Expected 3 parts, got %d", len(parts))
	}

	// Text parts should come before functionCall
	if parts[0].Get("text").String() != "Let me check..." {
		t.Errorf("Expected first text part first, got %s", parts[0].Raw)
	}
	if parts[1].Get("text").String() != "Reading the file now" {
		t.Errorf("Expected second text part second, got %s", parts[1].Raw)
	}
	if !parts[2].Get("functionCall").Exists() {
		t.Errorf("Expected functionCall last, got %s", parts[2].Raw)
	}
	if parts[2].Get("functionCall.name").String() != "Read" {
		t.Errorf("Expected functionCall name 'Read', got '%s'", parts[2].Get("functionCall.name").String())
	}
}

// TestConvertClaudeRequestToAntigravity_ReorderParallelFunctionCalls 测试
// 多个并行 functionCall 的排序策略
func TestConvertClaudeRequestToAntigravity_ReorderParallelFunctionCalls(t *testing.T) {
	inputJSON := []byte(`{
		"model": "claude-sonnet-4-5",
		"messages": [
			{
				"role": "assistant",
				"content": [
					{"type": "text", "text": "Reading both files."},
					{
						"type": "tool_use",
						"id": "call_1",
						"name": "Read",
						"input": {"file": "a.go"}
					},
					{"type": "text", "text": "And this one too."},
					{
						"type": "tool_use",
						"id": "call_2",
						"name": "Read",
						"input": {"file": "b.go"}
					}
				]
			}
		]
	}`)

	output := ConvertClaudeRequestToAntigravity("claude-sonnet-4-5", inputJSON, false)
	outputStr := string(output)

	parts := gjson.Get(outputStr, "request.contents.0.parts").Array()
	if len(parts) != 4 {
		t.Fatalf("Expected 4 parts, got %d", len(parts))
	}

	if parts[0].Get("text").String() != "Reading both files." {
		t.Errorf("Expected first text, got %s", parts[0].Raw)
	}
	if parts[1].Get("text").String() != "And this one too." {
		t.Errorf("Expected second text, got %s", parts[1].Raw)
	}
	if parts[2].Get("functionCall.name").String() != "Read" || parts[2].Get("functionCall.id").String() != "call_1" {
		t.Errorf("Expected fc1 third, got %s", parts[2].Raw)
	}
	if parts[3].Get("functionCall.name").String() != "Read" || parts[3].Get("functionCall.id").String() != "call_2" {
		t.Errorf("Expected fc2 fourth, got %s", parts[3].Raw)
	}
}

// TestConvertClaudeRequestToAntigravity_ReorderThinkingAndTextBeforeFunctionCall 测试
// thinking 和 text 部分应在 functionCall 之前排序
func TestConvertClaudeRequestToAntigravity_ReorderThinkingAndTextBeforeFunctionCall(t *testing.T) {
	cache.ClearSignatureCache("")

	validSignature := "abc123validSignature1234567890123456789012345678901234567890"
	thinkingText := "Let me think about this..."

	inputJSON := []byte(`{
		"model": "claude-sonnet-4-5-thinking",
		"messages": [
			{
				"role": "user",
				"content": [{"type": "text", "text": "Hello"}]
			},
			{
				"role": "assistant",
				"content": [
					{"type": "text", "text": "Before thinking"},
					{"type": "thinking", "thinking": "` + thinkingText + `", "signature": "` + validSignature + `"},
					{
						"type": "tool_use",
						"id": "call_xyz",
						"name": "Bash",
						"input": {"command": "ls"}
					},
					{"type": "text", "text": "After tool call"}
				]
			}
		]
	}`)

	cache.CacheSignature("claude-sonnet-4-5-thinking", thinkingText, validSignature)

	output := ConvertClaudeRequestToAntigravity("claude-sonnet-4-5-thinking", inputJSON, false)
	outputStr := string(output)

	// contents.1 = assistant message (contents.0 = user)
	parts := gjson.Get(outputStr, "request.contents.1.parts").Array()
	if len(parts) != 4 {
		t.Fatalf("Expected 4 parts, got %d", len(parts))
	}

	// Order: thinking → text → text → functionCall
	if !parts[0].Get("thought").Bool() {
		t.Error("First part should be thinking")
	}
	if parts[1].Get("functionCall").Exists() || parts[1].Get("thought").Bool() {
		t.Errorf("Second part should be text, got %s", parts[1].Raw)
	}
	if parts[2].Get("functionCall").Exists() || parts[2].Get("thought").Bool() {
		t.Errorf("Third part should be text, got %s", parts[2].Raw)
	}
	if !parts[3].Get("functionCall").Exists() {
		t.Errorf("Last part should be functionCall, got %s", parts[3].Raw)
	}
}

// TestConvertClaudeRequestToAntigravity_ToolResult 测试工具结果的转换，
// 包括 functionResponse 结构的正确生成
func TestConvertClaudeRequestToAntigravity_ToolResult(t *testing.T) {
	inputJSON := []byte(`{
		"model": "claude-3-5-sonnet-20240620",
		"messages": [
			{
				"role": "assistant",
				"content": [
					{
						"type": "tool_use",
						"id": "get_weather-call-123",
						"name": "get_weather",
						"input": {"location": "Paris"}
					}
				]
			},
			{
				"role": "user",
				"content": [
					{
						"type": "tool_result",
						"tool_use_id": "get_weather-call-123",
						"content": "22C sunny"
					}
				]
			}
		]
	}`)

	output := ConvertClaudeRequestToAntigravity("claude-sonnet-4-5", inputJSON, false)
	outputStr := string(output)

	// Check function response conversion
	funcResp := gjson.Get(outputStr, "request.contents.1.parts.0.functionResponse")
	if !funcResp.Exists() {
		t.Error("functionResponse should exist")
	}
	if funcResp.Get("id").String() != "get_weather-call-123" {
		t.Errorf("Expected function id, got '%s'", funcResp.Get("id").String())
	}
	if funcResp.Get("name").String() != "get_weather" {
		t.Errorf("Expected function name 'get_weather', got '%s'", funcResp.Get("name").String())
	}
}

// TestConvertClaudeRequestToAntigravity_ToolResultName_TouluFormat 测试
// toolu_ 格式的工具 ID 应能正确解析出工具名称
func TestConvertClaudeRequestToAntigravity_ToolResultName_TouluFormat(t *testing.T) {
	inputJSON := []byte(`{
		"model": "claude-haiku-4-5-20251001",
		"messages": [
			{
				"role": "assistant",
				"content": [
					{
						"type": "tool_use",
						"id": "toolu_tool-48fca351f12844eabf49dad8b63886d2",
						"name": "Glob",
						"input": {"pattern": "**/*.py"}
					},
					{
						"type": "tool_use",
						"id": "toolu_tool-cf2d061f75f845c49aacc18ee75ee708",
						"name": "Bash",
						"input": {"command": "ls"}
					}
				]
			},
			{
				"role": "user",
				"content": [
					{
						"type": "tool_result",
						"tool_use_id": "toolu_tool-48fca351f12844eabf49dad8b63886d2",
						"content": "file1.py\nfile2.py"
					},
					{
						"type": "tool_result",
						"tool_use_id": "toolu_tool-cf2d061f75f845c49aacc18ee75ee708",
						"content": "total 10"
					}
				]
			}
		]
	}`)

	output := ConvertClaudeRequestToAntigravity("claude-haiku-4-5-20251001", inputJSON, false)
	outputStr := string(output)

	funcResp0 := gjson.Get(outputStr, "request.contents.1.parts.0.functionResponse")
	if !funcResp0.Exists() {
		t.Fatal("first functionResponse should exist")
	}
	if got := funcResp0.Get("name").String(); got != "Glob" {
		t.Errorf("Expected name 'Glob' for toolu_ format, got '%s'", got)
	}

	funcResp1 := gjson.Get(outputStr, "request.contents.1.parts.1.functionResponse")
	if !funcResp1.Exists() {
		t.Fatal("second functionResponse should exist")
	}
	if got := funcResp1.Get("name").String(); got != "Bash" {
		t.Errorf("Expected name 'Bash' for toolu_ format, got '%s'", got)
	}
}

// TestConvertClaudeRequestToAntigravity_ToolResultName_CustomFormat 测试
// 自定义格式的工具 ID 应能正确解析出工具名称
func TestConvertClaudeRequestToAntigravity_ToolResultName_CustomFormat(t *testing.T) {
	inputJSON := []byte(`{
		"model": "claude-haiku-4-5-20251001",
		"messages": [
			{
				"role": "assistant",
				"content": [
					{
						"type": "tool_use",
						"id": "Read-1773420180464065165-1327",
						"name": "Read",
						"input": {"file_path": "/tmp/test.py"}
					}
				]
			},
			{
				"role": "user",
				"content": [
					{
						"type": "tool_result",
						"tool_use_id": "Read-1773420180464065165-1327",
						"content": "file content here"
					}
				]
			}
		]
	}`)

	output := ConvertClaudeRequestToAntigravity("claude-haiku-4-5-20251001", inputJSON, false)
	outputStr := string(output)

	funcResp := gjson.Get(outputStr, "request.contents.1.parts.0.functionResponse")
	if !funcResp.Exists() {
		t.Fatal("functionResponse should exist")
	}
	if got := funcResp.Get("name").String(); got != "Read" {
		t.Errorf("Expected name 'Read', got '%s'", got)
	}
}

// TestConvertClaudeRequestToAntigravity_ToolResultName_NoMatchingToolUse_Heuristic 测试
// 无匹配 tool_use 时应通过启发式方法从 ID 推导工具名称
func TestConvertClaudeRequestToAntigravity_ToolResultName_NoMatchingToolUse_Heuristic(t *testing.T) {
	inputJSON := []byte(`{
		"model": "claude-sonnet-4-5",
		"messages": [
			{
				"role": "user",
				"content": [
					{
						"type": "tool_result",
						"tool_use_id": "get_weather-call-123",
						"content": "22C sunny"
					}
				]
			}
		]
	}`)

	output := ConvertClaudeRequestToAntigravity("claude-sonnet-4-5", inputJSON, false)
	outputStr := string(output)

	funcResp := gjson.Get(outputStr, "request.contents.0.parts.0.functionResponse")
	if !funcResp.Exists() {
		t.Fatal("functionResponse should exist")
	}
	if got := funcResp.Get("name").String(); got != "get_weather" {
		t.Errorf("Expected heuristic-derived name 'get_weather', got '%s'", got)
	}
}

// TestConvertClaudeRequestToAntigravity_ToolResultName_NoMatchingToolUse_RawID 测试
// 无匹配 tool_use 且无法推导时应使用原始 ID 作为工具名称
func TestConvertClaudeRequestToAntigravity_ToolResultName_NoMatchingToolUse_RawID(t *testing.T) {
	inputJSON := []byte(`{
		"model": "claude-sonnet-4-5",
		"messages": [
			{
				"role": "user",
				"content": [
					{
						"type": "tool_result",
						"tool_use_id": "toolu_tool-48fca351f12844eabf49dad8b63886d2",
						"content": "result data"
					}
				]
			}
		]
	}`)

	output := ConvertClaudeRequestToAntigravity("claude-sonnet-4-5", inputJSON, false)
	outputStr := string(output)

	funcResp := gjson.Get(outputStr, "request.contents.0.parts.0.functionResponse")
	if !funcResp.Exists() {
		t.Fatal("functionResponse should exist")
	}
	got := funcResp.Get("name").String()
	if got == "" {
		t.Error("functionResponse.name must not be empty")
	}
	if got != "toolu_tool-48fca351f12844eabf49dad8b63886d2" {
		t.Errorf("Expected raw ID as last-resort name, got '%s'", got)
	}
}

// TestConvertClaudeRequestToAntigravity_ThinkingConfig 测试 thinking 配置的转换，
// 包括 thinkingBudget 和 includeThoughts 参数
func TestConvertClaudeRequestToAntigravity_ThinkingConfig(t *testing.T) {
	// Note: This test requires the model to be registered in the registry
	// with Thinking metadata. If the registry is not populated in test environment,
	// thinkingConfig won't be added. We'll test the basic structure only.
	inputJSON := []byte(`{
		"model": "claude-sonnet-4-5-thinking",
		"messages": [],
		"thinking": {
			"type": "enabled",
			"budget_tokens": 8000
		}
	}`)

	output := ConvertClaudeRequestToAntigravity("claude-sonnet-4-5-thinking", inputJSON, false)
	outputStr := string(output)

	// Check thinking config conversion (only if model supports thinking in registry)
	thinkingConfig := gjson.Get(outputStr, "request.generationConfig.thinkingConfig")
	if thinkingConfig.Exists() {
		if thinkingConfig.Get("thinkingBudget").Int() != 8000 {
			t.Errorf("Expected thinkingBudget 8000, got %d", thinkingConfig.Get("thinkingBudget").Int())
		}
		if !thinkingConfig.Get("includeThoughts").Bool() {
			t.Error("includeThoughts should be true")
		}
	} else {
		t.Log("thinkingConfig not present - model may not be registered in test registry")
	}
}

// TestConvertClaudeRequestToAntigravity_ImageContent 测试图片内容的转换，
// 包括 base64 图片到 inlineData 的映射
func TestConvertClaudeRequestToAntigravity_ImageContent(t *testing.T) {
	inputJSON := []byte(`{
		"model": "claude-3-5-sonnet-20240620",
		"messages": [
			{
				"role": "user",
				"content": [
					{
						"type": "image",
						"source": {
							"type": "base64",
							"media_type": "image/png",
							"data": "iVBORw0KGgoAAAANSUhEUg=="
						}
					}
				]
			}
		]
	}`)

	output := ConvertClaudeRequestToAntigravity("claude-sonnet-4-5", inputJSON, false)
	outputStr := string(output)

	// Check inline data conversion
	inlineData := gjson.Get(outputStr, "request.contents.0.parts.0.inlineData")
	if !inlineData.Exists() {
		t.Error("inlineData should exist")
	}
	if inlineData.Get("mimeType").String() != "image/png" {
		t.Error("mimeType mismatch")
	}
	if !strings.Contains(inlineData.Get("data").String(), "iVBORw0KGgo") {
		t.Error("data mismatch")
	}
}

// TestConvertClaudeRequestToAntigravity_GenerationConfig 测试生成参数的转换，
// 包括 temperature、top_p、top_k、max_tokens 到 Antigravity 格式的映射
func TestConvertClaudeRequestToAntigravity_GenerationConfig(t *testing.T) {
	inputJSON := []byte(`{
		"model": "claude-3-5-sonnet-20240620",
		"messages": [],
		"temperature": 0.7,
		"top_p": 0.9,
		"top_k": 40,
		"max_tokens": 2000
	}`)

	output := ConvertClaudeRequestToAntigravity("claude-sonnet-4-5", inputJSON, false)
	outputStr := string(output)

	genConfig := gjson.Get(outputStr, "request.generationConfig")
	if genConfig.Get("temperature").Float() != 0.7 {
		t.Errorf("Expected temperature 0.7, got %f", genConfig.Get("temperature").Float())
	}
	if genConfig.Get("topP").Float() != 0.9 {
		t.Errorf("Expected topP 0.9, got %f", genConfig.Get("topP").Float())
	}
	if genConfig.Get("topK").Float() != 40 {
		t.Errorf("Expected topK 40, got %f", genConfig.Get("topK").Float())
	}
	if genConfig.Get("maxOutputTokens").Float() != 2000 {
		t.Errorf("Expected maxOutputTokens 2000, got %f", genConfig.Get("maxOutputTokens").Float())
	}
}

// ============================================================================
// 尾部无签名 thinking 块移除测试
// ============================================================================

// TestConvertClaudeRequestToAntigravity_TrailingUnsignedThinking_Removed 测试
// 最后一条 assistant 消息末尾的无签名 thinking 块应被移除
func TestConvertClaudeRequestToAntigravity_TrailingUnsignedThinking_Removed(t *testing.T) {
	// Last assistant message ends with unsigned thinking block - should be removed
	inputJSON := []byte(`{
		"model": "claude-sonnet-4-5-thinking",
		"messages": [
			{
				"role": "user",
				"content": [{"type": "text", "text": "Hello"}]
			},
			{
				"role": "assistant",
				"content": [
					{"type": "text", "text": "Here is my answer"},
					{"type": "thinking", "thinking": "I should think more..."}
				]
			}
		]
	}`)

	output := ConvertClaudeRequestToAntigravity("claude-sonnet-4-5-thinking", inputJSON, false)
	outputStr := string(output)

	// The last part of the last assistant message should NOT be a thinking block
	lastMessageParts := gjson.Get(outputStr, "request.contents.1.parts")
	if !lastMessageParts.IsArray() {
		t.Fatal("Last message should have parts array")
	}
	parts := lastMessageParts.Array()
	if len(parts) == 0 {
		t.Fatal("Last message should have at least one part")
	}

	// The unsigned thinking should be removed, leaving only the text
	lastPart := parts[len(parts)-1]
	if lastPart.Get("thought").Bool() {
		t.Error("Trailing unsigned thinking block should be removed")
	}
}

// TestConvertClaudeRequestToAntigravity_TrailingSignedThinking_Kept 测试
// 最后一条 assistant 消息末尾的有签名 thinking 块应被保留
func TestConvertClaudeRequestToAntigravity_TrailingSignedThinking_Kept(t *testing.T) {
	cache.ClearSignatureCache("")

	// Last assistant message ends with signed thinking block - should be kept
	validSignature := "abc123validSignature1234567890123456789012345678901234567890"
	thinkingText := "Valid thinking..."

	inputJSON := []byte(`{
		"model": "claude-sonnet-4-5-thinking",
		"messages": [
			{
				"role": "user",
				"content": [{"type": "text", "text": "Hello"}]
			},
			{
				"role": "assistant",
				"content": [
					{"type": "text", "text": "Here is my answer"},
					{"type": "thinking", "thinking": "` + thinkingText + `", "signature": "` + validSignature + `"}
				]
			}
		]
	}`)

	cache.CacheSignature("claude-sonnet-4-5-thinking", thinkingText, validSignature)

	output := ConvertClaudeRequestToAntigravity("claude-sonnet-4-5-thinking", inputJSON, false)
	outputStr := string(output)

	// The signed thinking block should be preserved
	lastMessageParts := gjson.Get(outputStr, "request.contents.1.parts")
	parts := lastMessageParts.Array()
	if len(parts) < 2 {
		t.Error("Signed thinking block should be preserved")
	}
}

// TestConvertClaudeRequestToAntigravity_MiddleUnsignedThinking_Removed 测试
// 中间消息的无签名 thinking 块应被完全移除
func TestConvertClaudeRequestToAntigravity_MiddleUnsignedThinking_Removed(t *testing.T) {
	// Middle message has unsigned thinking - should be removed entirely
	inputJSON := []byte(`{
		"model": "claude-sonnet-4-5-thinking",
		"messages": [
			{
				"role": "assistant",
				"content": [
					{"type": "thinking", "thinking": "Middle thinking..."},
					{"type": "text", "text": "Answer"}
				]
			},
			{
				"role": "user",
				"content": [{"type": "text", "text": "Follow up"}]
			}
		]
	}`)

	output := ConvertClaudeRequestToAntigravity("claude-sonnet-4-5-thinking", inputJSON, false)
	outputStr := string(output)

	// Unsigned thinking should be removed entirely
	parts := gjson.Get(outputStr, "request.contents.0.parts").Array()
	if len(parts) != 1 {
		t.Fatalf("Expected 1 part (thinking removed), got %d", len(parts))
	}

	// Only text part should remain
	if parts[0].Get("thought").Bool() {
		t.Error("Thinking block should be removed, not preserved")
	}
	if parts[0].Get("text").String() != "Answer" {
		t.Errorf("Expected text 'Answer', got '%s'", parts[0].Get("text").String())
	}
}

// ============================================================================
// 工具 + Thinking 系统提示注入测试
// ============================================================================

// TestConvertClaudeRequestToAntigravity_ToolAndThinking_HintInjected 测试
// 当同时启用工具和 thinking 时，系统指令中应注入交错 thinking 提示
func TestConvertClaudeRequestToAntigravity_ToolAndThinking_HintInjected(t *testing.T) {
	// When both tools and thinking are enabled, hint should be injected into system instruction
	inputJSON := []byte(`{
		"model": "claude-sonnet-4-5-thinking",
		"messages": [{"role": "user", "content": [{"type": "text", "text": "Hello"}]}],
		"system": [{"type": "text", "text": "You are helpful."}],
		"tools": [
			{
				"name": "get_weather",
				"description": "Get weather",
				"input_schema": {"type": "object", "properties": {"location": {"type": "string"}}}
			}
		],
		"thinking": {"type": "enabled", "budget_tokens": 8000}
	}`)

	output := ConvertClaudeRequestToAntigravity("claude-sonnet-4-5-thinking", inputJSON, false)
	outputStr := string(output)

	// System instruction should contain the interleaved thinking hint
	sysInstruction := gjson.Get(outputStr, "request.systemInstruction")
	if !sysInstruction.Exists() {
		t.Fatal("systemInstruction should exist")
	}

	// Check if hint is appended
	sysText := sysInstruction.Get("parts").Array()
	found := false
	for _, part := range sysText {
		if strings.Contains(part.Get("text").String(), "Interleaved thinking is enabled") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Interleaved thinking hint should be injected when tools and thinking are both active, got: %v", sysInstruction.Raw)
	}
}

// TestConvertClaudeRequestToAntigravity_ToolsOnly_NoHint 测试
// 仅有工具（无 thinking）时不应注入提示
func TestConvertClaudeRequestToAntigravity_ToolsOnly_NoHint(t *testing.T) {
	// When only tools are present (no thinking), hint should NOT be injected
	inputJSON := []byte(`{
		"model": "claude-sonnet-4-5",
		"messages": [{"role": "user", "content": [{"type": "text", "text": "Hello"}]}],
		"system": [{"type": "text", "text": "You are helpful."}],
		"tools": [
			{
				"name": "get_weather",
				"description": "Get weather",
				"input_schema": {"type": "object", "properties": {"location": {"type": "string"}}}
			}
		]
	}`)

	output := ConvertClaudeRequestToAntigravity("claude-sonnet-4-5", inputJSON, false)
	outputStr := string(output)

	// System instruction should NOT contain the hint
	sysInstruction := gjson.Get(outputStr, "request.systemInstruction")
	if sysInstruction.Exists() {
		for _, part := range sysInstruction.Get("parts").Array() {
			if strings.Contains(part.Get("text").String(), "Interleaved thinking is enabled") {
				t.Error("Hint should NOT be injected when only tools are present (no thinking)")
			}
		}
	}
}

// TestConvertClaudeRequestToAntigravity_ThinkingOnly_NoHint 测试
// 仅有 thinking（无工具）时不应注入提示
func TestConvertClaudeRequestToAntigravity_ThinkingOnly_NoHint(t *testing.T) {
	// When only thinking is enabled (no tools), hint should NOT be injected
	inputJSON := []byte(`{
		"model": "claude-sonnet-4-5-thinking",
		"messages": [{"role": "user", "content": [{"type": "text", "text": "Hello"}]}],
		"system": [{"type": "text", "text": "You are helpful."}],
		"thinking": {"type": "enabled", "budget_tokens": 8000}
	}`)

	output := ConvertClaudeRequestToAntigravity("claude-sonnet-4-5-thinking", inputJSON, false)
	outputStr := string(output)

	// System instruction should NOT contain the hint (no tools)
	sysInstruction := gjson.Get(outputStr, "request.systemInstruction")
	if sysInstruction.Exists() {
		for _, part := range sysInstruction.Get("parts").Array() {
			if strings.Contains(part.Get("text").String(), "Interleaved thinking is enabled") {
				t.Error("Hint should NOT be injected when only thinking is present (no tools)")
			}
		}
	}
}

// TestConvertClaudeRequestToAntigravity_ToolResultNoContent 测试
// tool_result 无 content 字段时应生成有效的 JSON
func TestConvertClaudeRequestToAntigravity_ToolResultNoContent(t *testing.T) {
	// Bug repro: tool_result with no content field produces invalid JSON
	inputJSON := []byte(`{
		"model": "claude-opus-4-6-thinking",
		"messages": [
			{
				"role": "assistant",
				"content": [
					{
						"type": "tool_use",
						"id": "MyTool-123-456",
						"name": "MyTool",
						"input": {"key": "value"}
					}
				]
			},
			{
				"role": "user",
				"content": [
					{
						"type": "tool_result",
						"tool_use_id": "MyTool-123-456"
					}
				]
			}
		]
	}`)

	output := ConvertClaudeRequestToAntigravity("claude-opus-4-6-thinking", inputJSON, true)
	outputStr := string(output)

	if !gjson.Valid(outputStr) {
		t.Errorf("Result is not valid JSON:\n%s", outputStr)
	}

	// Verify the functionResponse has a valid result value
	fr := gjson.Get(outputStr, "request.contents.1.parts.0.functionResponse.response.result")
	if !fr.Exists() {
		t.Error("functionResponse.response.result should exist")
	}
}

// TestConvertClaudeRequestToAntigravity_ToolResultNullContent 测试
// tool_result 的 content 为 null 时应生成有效的 JSON
func TestConvertClaudeRequestToAntigravity_ToolResultNullContent(t *testing.T) {
	// Bug repro: tool_result with null content produces invalid JSON
	inputJSON := []byte(`{
		"model": "claude-opus-4-6-thinking",
		"messages": [
			{
				"role": "assistant",
				"content": [
					{
						"type": "tool_use",
						"id": "MyTool-123-456",
						"name": "MyTool",
						"input": {"key": "value"}
					}
				]
			},
			{
				"role": "user",
				"content": [
					{
						"type": "tool_result",
						"tool_use_id": "MyTool-123-456",
						"content": null
					}
				]
			}
		]
	}`)

	output := ConvertClaudeRequestToAntigravity("claude-opus-4-6-thinking", inputJSON, true)
	outputStr := string(output)

	if !gjson.Valid(outputStr) {
		t.Errorf("Result is not valid JSON:\n%s", outputStr)
	}
}

// TestConvertClaudeRequestToAntigravity_ToolResultWithImage 测试
// tool_result 中包含文本和图片数组时，图片应放入 functionResponse.parts 中
func TestConvertClaudeRequestToAntigravity_ToolResultWithImage(t *testing.T) {
	// tool_result with array content containing text + image should place
	// image data inside functionResponse.parts as inlineData, not as a
	// sibling part in the outer content (to avoid base64 context bloat).
	inputJSON := []byte(`{
		"model": "claude-3-5-sonnet-20240620",
		"messages": [
			{
				"role": "user",
				"content": [
					{
						"type": "tool_result",
						"tool_use_id": "Read-123-456",
						"content": [
							{
								"type": "text",
								"text": "File content here"
							},
							{
								"type": "image",
								"source": {
									"type": "base64",
									"media_type": "image/png",
									"data": "iVBORw0KGgoAAAANSUhEUg=="
								}
							}
						]
					}
				]
			}
		]
	}`)

	output := ConvertClaudeRequestToAntigravity("claude-sonnet-4-5", inputJSON, false)
	outputStr := string(output)

	if !gjson.Valid(outputStr) {
		t.Fatalf("Result is not valid JSON:\n%s", outputStr)
	}

	// Image should be inside functionResponse.parts, not as outer sibling part
	funcResp := gjson.Get(outputStr, "request.contents.0.parts.0.functionResponse")
	if !funcResp.Exists() {
		t.Fatal("functionResponse should exist")
	}

	// Text content should be in response.result
	resultText := funcResp.Get("response.result.text").String()
	if resultText != "File content here" {
		t.Errorf("Expected response.result.text = 'File content here', got '%s'", resultText)
	}

	// Image should be in functionResponse.parts[0].inlineData
	inlineData := funcResp.Get("parts.0.inlineData")
	if !inlineData.Exists() {
		t.Fatal("functionResponse.parts[0].inlineData should exist")
	}
	if inlineData.Get("mimeType").String() != "image/png" {
		t.Errorf("Expected mimeType 'image/png', got '%s'", inlineData.Get("mimeType").String())
	}
	if !strings.Contains(inlineData.Get("data").String(), "iVBORw0KGgo") {
		t.Error("data mismatch")
	}

	// Image should NOT be in outer parts (only functionResponse part should exist)
	outerParts := gjson.Get(outputStr, "request.contents.0.parts")
	if outerParts.IsArray() && len(outerParts.Array()) > 1 {
		t.Errorf("Expected only 1 outer part (functionResponse), got %d", len(outerParts.Array()))
	}
}

// TestConvertClaudeRequestToAntigravity_ToolResultWithSingleImage 测试
// tool_result 中仅包含单张图片时的处理
func TestConvertClaudeRequestToAntigravity_ToolResultWithSingleImage(t *testing.T) {
	// tool_result with single image object as content should place
	// image data inside functionResponse.parts, not as outer sibling part.
	inputJSON := []byte(`{
		"model": "claude-3-5-sonnet-20240620",
		"messages": [
			{
				"role": "user",
				"content": [
					{
						"type": "tool_result",
						"tool_use_id": "Read-789-012",
						"content": {
							"type": "image",
							"source": {
								"type": "base64",
								"media_type": "image/jpeg",
								"data": "/9j/4AAQSkZJRgABAQ=="
							}
						}
					}
				]
			}
		]
	}`)

	output := ConvertClaudeRequestToAntigravity("claude-sonnet-4-5", inputJSON, false)
	outputStr := string(output)

	if !gjson.Valid(outputStr) {
		t.Fatalf("Result is not valid JSON:\n%s", outputStr)
	}

	funcResp := gjson.Get(outputStr, "request.contents.0.parts.0.functionResponse")
	if !funcResp.Exists() {
		t.Fatal("functionResponse should exist")
	}

	// response.result should be empty (image only)
	if funcResp.Get("response.result").String() != "" {
		t.Errorf("Expected empty response.result for image-only content, got '%s'", funcResp.Get("response.result").String())
	}

	// Image should be in functionResponse.parts[0].inlineData
	inlineData := funcResp.Get("parts.0.inlineData")
	if !inlineData.Exists() {
		t.Fatal("functionResponse.parts[0].inlineData should exist")
	}
	if inlineData.Get("mimeType").String() != "image/jpeg" {
		t.Errorf("Expected mimeType 'image/jpeg', got '%s'", inlineData.Get("mimeType").String())
	}

	// Image should NOT be in outer parts
	outerParts := gjson.Get(outputStr, "request.contents.0.parts")
	if outerParts.IsArray() && len(outerParts.Array()) > 1 {
		t.Errorf("Expected only 1 outer part, got %d", len(outerParts.Array()))
	}
}

// TestConvertClaudeRequestToAntigravity_ToolResultWithMultipleImagesAndTexts 测试
// tool_result 中包含多个文本和多张图片的混合内容处理
func TestConvertClaudeRequestToAntigravity_ToolResultWithMultipleImagesAndTexts(t *testing.T) {
	// tool_result with array content: 2 text items + 2 images
	// All images go into functionResponse.parts, texts into response.result array
	inputJSON := []byte(`{
		"model": "claude-3-5-sonnet-20240620",
		"messages": [
			{
				"role": "user",
				"content": [
					{
						"type": "tool_result",
						"tool_use_id": "Multi-001",
						"content": [
							{"type": "text", "text": "First text"},
							{
								"type": "image",
								"source": {"type": "base64", "media_type": "image/png", "data": "AAAA"}
							},
							{"type": "text", "text": "Second text"},
							{
								"type": "image",
								"source": {"type": "base64", "media_type": "image/jpeg", "data": "BBBB"}
							}
						]
					}
				]
			}
		]
	}`)

	output := ConvertClaudeRequestToAntigravity("claude-sonnet-4-5", inputJSON, false)
	outputStr := string(output)

	if !gjson.Valid(outputStr) {
		t.Fatalf("Result is not valid JSON:\n%s", outputStr)
	}

	funcResp := gjson.Get(outputStr, "request.contents.0.parts.0.functionResponse")
	if !funcResp.Exists() {
		t.Fatal("functionResponse should exist")
	}

	// Multiple text items => response.result is an array
	resultArr := funcResp.Get("response.result")
	if !resultArr.IsArray() {
		t.Fatalf("Expected response.result to be an array, got: %s", resultArr.Raw)
	}
	results := resultArr.Array()
	if len(results) != 2 {
		t.Fatalf("Expected 2 result items, got %d", len(results))
	}

	// Both images should be in functionResponse.parts
	imgParts := funcResp.Get("parts").Array()
	if len(imgParts) != 2 {
		t.Fatalf("Expected 2 image parts in functionResponse.parts, got %d", len(imgParts))
	}
	if imgParts[0].Get("inlineData.mimeType").String() != "image/png" {
		t.Errorf("Expected first image mimeType 'image/png', got '%s'", imgParts[0].Get("inlineData.mimeType").String())
	}
	if imgParts[0].Get("inlineData.data").String() != "AAAA" {
		t.Errorf("Expected first image data 'AAAA', got '%s'", imgParts[0].Get("inlineData.data").String())
	}
	if imgParts[1].Get("inlineData.mimeType").String() != "image/jpeg" {
		t.Errorf("Expected second image mimeType 'image/jpeg', got '%s'", imgParts[1].Get("inlineData.mimeType").String())
	}
	if imgParts[1].Get("inlineData.data").String() != "BBBB" {
		t.Errorf("Expected second image data 'BBBB', got '%s'", imgParts[1].Get("inlineData.data").String())
	}

	// Only 1 outer part (the functionResponse itself)
	outerParts := gjson.Get(outputStr, "request.contents.0.parts").Array()
	if len(outerParts) != 1 {
		t.Errorf("Expected 1 outer part, got %d", len(outerParts))
	}
}

// TestConvertClaudeRequestToAntigravity_ToolResultWithOnlyMultipleImages 测试
// tool_result 中仅包含多张图片（无文本）时的处理
func TestConvertClaudeRequestToAntigravity_ToolResultWithOnlyMultipleImages(t *testing.T) {
	// tool_result with only images (no text) — response.result should be empty string
	inputJSON := []byte(`{
		"model": "claude-3-5-sonnet-20240620",
		"messages": [
			{
				"role": "user",
				"content": [
					{
						"type": "tool_result",
						"tool_use_id": "ImgOnly-001",
						"content": [
							{
								"type": "image",
								"source": {"type": "base64", "media_type": "image/png", "data": "PNG1"}
							},
							{
								"type": "image",
								"source": {"type": "base64", "media_type": "image/gif", "data": "GIF1"}
							}
						]
					}
				]
			}
		]
	}`)

	output := ConvertClaudeRequestToAntigravity("claude-sonnet-4-5", inputJSON, false)
	outputStr := string(output)

	if !gjson.Valid(outputStr) {
		t.Fatalf("Result is not valid JSON:\n%s", outputStr)
	}

	funcResp := gjson.Get(outputStr, "request.contents.0.parts.0.functionResponse")
	if !funcResp.Exists() {
		t.Fatal("functionResponse should exist")
	}

	// No text => response.result should be empty string
	if funcResp.Get("response.result").String() != "" {
		t.Errorf("Expected empty response.result, got '%s'", funcResp.Get("response.result").String())
	}

	// Both images in functionResponse.parts
	imgParts := funcResp.Get("parts").Array()
	if len(imgParts) != 2 {
		t.Fatalf("Expected 2 image parts, got %d", len(imgParts))
	}
	if imgParts[0].Get("inlineData.mimeType").String() != "image/png" {
		t.Error("first image mimeType mismatch")
	}
	if imgParts[1].Get("inlineData.mimeType").String() != "image/gif" {
		t.Error("second image mimeType mismatch")
	}

	// Only 1 outer part
	outerParts := gjson.Get(outputStr, "request.contents.0.parts").Array()
	if len(outerParts) != 1 {
		t.Errorf("Expected 1 outer part, got %d", len(outerParts))
	}
}

// TestConvertClaudeRequestToAntigravity_ToolResultImageNotBase64 测试
// 非 base64 类型的图片应被视为非图片内容处理
func TestConvertClaudeRequestToAntigravity_ToolResultImageNotBase64(t *testing.T) {
	// image with source.type != "base64" should be treated as non-image (falls through)
	inputJSON := []byte(`{
		"model": "claude-3-5-sonnet-20240620",
		"messages": [
			{
				"role": "user",
				"content": [
					{
						"type": "tool_result",
						"tool_use_id": "NotB64-001",
						"content": [
							{"type": "text", "text": "some output"},
							{
								"type": "image",
								"source": {"type": "url", "url": "https://example.com/img.png"}
							}
						]
					}
				]
			}
		]
	}`)

	output := ConvertClaudeRequestToAntigravity("claude-sonnet-4-5", inputJSON, false)
	outputStr := string(output)

	if !gjson.Valid(outputStr) {
		t.Fatalf("Result is not valid JSON:\n%s", outputStr)
	}

	funcResp := gjson.Get(outputStr, "request.contents.0.parts.0.functionResponse")
	if !funcResp.Exists() {
		t.Fatal("functionResponse should exist")
	}

	// Non-base64 image is treated as non-image, so it goes into the filtered results
	// along with the text item. Since there are 2 non-image items, result is array.
	resultArr := funcResp.Get("response.result")
	if !resultArr.IsArray() {
		t.Fatalf("Expected response.result to be an array (2 non-image items), got: %s", resultArr.Raw)
	}
	results := resultArr.Array()
	if len(results) != 2 {
		t.Fatalf("Expected 2 result items, got %d", len(results))
	}

	// No functionResponse.parts (no base64 images collected)
	if funcResp.Get("parts").Exists() {
		t.Error("functionResponse.parts should NOT exist when no base64 images")
	}
}

// TestConvertClaudeRequestToAntigravity_ToolResultImageMissingData 测试
// base64 图片缺少 data 字段时的处理
func TestConvertClaudeRequestToAntigravity_ToolResultImageMissingData(t *testing.T) {
	// image with source.type=base64 but missing data field
	inputJSON := []byte(`{
		"model": "claude-3-5-sonnet-20240620",
		"messages": [
			{
				"role": "user",
				"content": [
					{
						"type": "tool_result",
						"tool_use_id": "NoData-001",
						"content": [
							{"type": "text", "text": "output"},
							{
								"type": "image",
								"source": {"type": "base64", "media_type": "image/png"}
							}
						]
					}
				]
			}
		]
	}`)

	output := ConvertClaudeRequestToAntigravity("claude-sonnet-4-5", inputJSON, false)
	outputStr := string(output)

	if !gjson.Valid(outputStr) {
		t.Fatalf("Result is not valid JSON:\n%s", outputStr)
	}

	funcResp := gjson.Get(outputStr, "request.contents.0.parts.0.functionResponse")
	if !funcResp.Exists() {
		t.Fatal("functionResponse should exist")
	}

	// The image is still classified as base64 image (type check passes),
	// but data field is missing => inlineData has mimeType but no data
	imgParts := funcResp.Get("parts").Array()
	if len(imgParts) != 1 {
		t.Fatalf("Expected 1 image part, got %d", len(imgParts))
	}
	if imgParts[0].Get("inlineData.mimeType").String() != "image/png" {
		t.Error("mimeType should still be set")
	}
	if imgParts[0].Get("inlineData.data").Exists() {
		t.Error("data should not exist when source.data is missing")
	}
}

// TestConvertClaudeRequestToAntigravity_ToolResultImageMissingMediaType 测试
// base64 图片缺少 media_type 字段时的处理
func TestConvertClaudeRequestToAntigravity_ToolResultImageMissingMediaType(t *testing.T) {
	// image with source.type=base64 but missing media_type field
	inputJSON := []byte(`{
		"model": "claude-3-5-sonnet-20240620",
		"messages": [
			{
				"role": "user",
				"content": [
					{
						"type": "tool_result",
						"tool_use_id": "NoMime-001",
						"content": [
							{"type": "text", "text": "output"},
							{
								"type": "image",
								"source": {"type": "base64", "data": "AAAA"}
							}
						]
					}
				]
			}
		]
	}`)

	output := ConvertClaudeRequestToAntigravity("claude-sonnet-4-5", inputJSON, false)
	outputStr := string(output)

	if !gjson.Valid(outputStr) {
		t.Fatalf("Result is not valid JSON:\n%s", outputStr)
	}

	funcResp := gjson.Get(outputStr, "request.contents.0.parts.0.functionResponse")
	if !funcResp.Exists() {
		t.Fatal("functionResponse should exist")
	}

	// The image is still classified as base64 image,
	// but media_type is missing => inlineData has data but no mimeType
	imgParts := funcResp.Get("parts").Array()
	if len(imgParts) != 1 {
		t.Fatalf("Expected 1 image part, got %d", len(imgParts))
	}
	if imgParts[0].Get("inlineData.mimeType").Exists() {
		t.Error("mimeType should not exist when media_type is missing")
	}
	if imgParts[0].Get("inlineData.data").String() != "AAAA" {
		t.Error("data should still be set")
	}
}

// TestConvertClaudeRequestToAntigravity_BypassMode_DropsRedactedThinkingBlocks 测试
// 绕过模式下空文本的 redacted thinking 块应被丢弃
func TestConvertClaudeRequestToAntigravity_BypassMode_DropsRedactedThinkingBlocks(t *testing.T) {
	cache.ClearSignatureCache("")
	previous := cache.SignatureCacheEnabled()
	cache.SetSignatureCacheEnabled(false)
	t.Cleanup(func() {
		cache.SetSignatureCacheEnabled(previous)
		cache.ClearSignatureCache("")
	})

	validSignature := testAnthropicNativeSignature(t)

	inputJSON := []byte(`{
		"model": "claude-opus-4-6",
		"messages": [
			{
				"role": "user",
				"content": [{"type": "text", "text": "Hello"}]
			},
			{
				"role": "assistant",
				"content": [
					{"type": "thinking", "thinking": "", "signature": "` + validSignature + `"},
					{"type": "text", "text": "I can help with that."}
				]
			},
			{
				"role": "user",
				"content": [{"type": "text", "text": "Follow up question"}]
			}
		],
		"thinking": {"type": "enabled", "budget_tokens": 10000}
	}`)

	output := ConvertClaudeRequestToAntigravity("claude-opus-4-6", inputJSON, false)

	assistantParts := gjson.GetBytes(output, "request.contents.1.parts").Array()
	if len(assistantParts) != 1 {
		t.Fatalf("Expected 1 part (redacted thinking dropped), got %d: %s",
			len(assistantParts), gjson.GetBytes(output, "request.contents.1.parts").Raw)
	}
	if assistantParts[0].Get("thought").Bool() {
		t.Fatal("Redacted thinking block with empty text should be dropped")
	}
	if assistantParts[0].Get("text").String() != "I can help with that." {
		t.Fatalf("Expected text part preserved, got: %s", assistantParts[0].Raw)
	}
}

// TestConvertClaudeRequestToAntigravity_BypassMode_DropsWrappedRedactedThinking 测试
// 绕过模式下包装格式的 redacted thinking 块应被丢弃
func TestConvertClaudeRequestToAntigravity_BypassMode_DropsWrappedRedactedThinking(t *testing.T) {
	cache.ClearSignatureCache("")
	previous := cache.SignatureCacheEnabled()
	cache.SetSignatureCacheEnabled(false)
	t.Cleanup(func() {
		cache.SetSignatureCacheEnabled(previous)
		cache.ClearSignatureCache("")
	})

	validSignature := testAnthropicNativeSignature(t)

	inputJSON := []byte(`{
		"model": "claude-sonnet-4-6",
		"messages": [
			{
				"role": "user",
				"content": [{"type": "text", "text": "Test user message"}]
			},
			{
				"role": "assistant",
				"content": [
					{"type": "thinking", "thinking": {"cache_control": {"type": "ephemeral"}}, "signature": "` + validSignature + `"},
					{"type": "text", "text": "Answer"}
				]
			},
			{
				"role": "user",
				"content": [{"type": "text", "text": "Follow up"}]
			}
		],
		"thinking": {"type": "enabled", "budget_tokens": 8000}
	}`)

	output := ConvertClaudeRequestToAntigravity("claude-sonnet-4-6", inputJSON, false)

	assistantParts := gjson.GetBytes(output, "request.contents.1.parts").Array()
	if len(assistantParts) != 1 {
		t.Fatalf("Expected 1 part (wrapped redacted thinking dropped), got %d: %s",
			len(assistantParts), gjson.GetBytes(output, "request.contents.1.parts").Raw)
	}
	if assistantParts[0].Get("text").String() != "Answer" {
		t.Fatalf("Expected text part preserved, got: %s", assistantParts[0].Raw)
	}
}

// TestConvertClaudeRequestToAntigravity_BypassMode_KeepsNonEmptyThinking 测试
// 绕过模式下非空的 thinking 块应被保留
func TestConvertClaudeRequestToAntigravity_BypassMode_KeepsNonEmptyThinking(t *testing.T) {
	cache.ClearSignatureCache("")
	previous := cache.SignatureCacheEnabled()
	cache.SetSignatureCacheEnabled(false)
	t.Cleanup(func() {
		cache.SetSignatureCacheEnabled(previous)
		cache.ClearSignatureCache("")
	})

	validSignature := testAnthropicNativeSignature(t)

	inputJSON := []byte(`{
		"model": "claude-opus-4-6",
		"messages": [
			{
				"role": "user",
				"content": [{"type": "text", "text": "Hello"}]
			},
			{
				"role": "assistant",
				"content": [
					{"type": "thinking", "thinking": "Let me reason about this carefully...", "signature": "` + validSignature + `"},
					{"type": "text", "text": "Here is my answer."}
				]
			}
		],
		"thinking": {"type": "enabled", "budget_tokens": 10000}
	}`)

	output := ConvertClaudeRequestToAntigravity("claude-opus-4-6", inputJSON, false)

	assistantParts := gjson.GetBytes(output, "request.contents.1.parts").Array()
	if len(assistantParts) != 2 {
		t.Fatalf("Expected 2 parts (thinking + text), got %d", len(assistantParts))
	}
	if !assistantParts[0].Get("thought").Bool() {
		t.Fatal("First part should be a thought block")
	}
	if assistantParts[0].Get("text").String() != "Let me reason about this carefully..." {
		t.Fatalf("Thinking text mismatch, got: %s", assistantParts[0].Get("text").String())
	}
	if assistantParts[1].Get("text").String() != "Here is my answer." {
		t.Fatalf("Text part mismatch, got: %s", assistantParts[1].Raw)
	}
}

// TestConvertClaudeRequestToAntigravity_BypassMode_MultiTurnRedactedThinking 测试
// 绕过模式下多轮对话中的 redacted thinking 块处理
func TestConvertClaudeRequestToAntigravity_BypassMode_MultiTurnRedactedThinking(t *testing.T) {
	cache.ClearSignatureCache("")
	previous := cache.SignatureCacheEnabled()
	cache.SetSignatureCacheEnabled(false)
	t.Cleanup(func() {
		cache.SetSignatureCacheEnabled(previous)
		cache.ClearSignatureCache("")
	})

	sig := testAnthropicNativeSignature(t)

	inputJSON := []byte(`{
		"model": "claude-opus-4-6",
		"messages": [
			{"role": "user", "content": [{"type": "text", "text": "First question"}]},
			{
				"role": "assistant",
				"content": [
					{"type": "thinking", "thinking": "", "signature": "` + sig + `"},
					{"type": "text", "text": "First answer"},
					{"type": "tool_use", "id": "Bash-123-456", "name": "Bash", "input": {"command": "ls"}}
				]
			},
			{
				"role": "user",
				"content": [
					{"type": "tool_result", "tool_use_id": "Bash-123-456", "content": "file1.txt\nfile2.txt"}
				]
			},
			{
				"role": "assistant",
				"content": [
					{"type": "thinking", "thinking": "", "signature": "` + sig + `"},
					{"type": "text", "text": "Here are the files."}
				]
			},
			{"role": "user", "content": [{"type": "text", "text": "Thanks"}]}
		],
		"thinking": {"type": "enabled", "budget_tokens": 10000}
	}`)

	output := ConvertClaudeRequestToAntigravity("claude-opus-4-6", inputJSON, false)

	if !gjson.ValidBytes(output) {
		t.Fatalf("Output is not valid JSON: %s", string(output))
	}

	firstAssistantParts := gjson.GetBytes(output, "request.contents.1.parts").Array()
	for _, p := range firstAssistantParts {
		if p.Get("thought").Bool() {
			t.Fatal("Redacted thinking should be dropped from first assistant message")
		}
	}
	hasText := false
	hasFC := false
	for _, p := range firstAssistantParts {
		if p.Get("text").String() == "First answer" {
			hasText = true
		}
		if p.Get("functionCall").Exists() {
			hasFC = true
		}
	}
	if !hasText || !hasFC {
		t.Fatalf("First assistant should have text + functionCall, got: %s",
			gjson.GetBytes(output, "request.contents.1.parts").Raw)
	}

	secondAssistantParts := gjson.GetBytes(output, "request.contents.3.parts").Array()
	for _, p := range secondAssistantParts {
		if p.Get("thought").Bool() {
			t.Fatal("Redacted thinking should be dropped from second assistant message")
		}
	}
	if len(secondAssistantParts) != 1 || secondAssistantParts[0].Get("text").String() != "Here are the files." {
		t.Fatalf("Second assistant should have only text part, got: %s",
			gjson.GetBytes(output, "request.contents.3.parts").Raw)
	}
}

// TestConvertClaudeRequestToAntigravity_ToolAndThinking_NoExistingSystem 测试
// 当同时启用工具和 thinking 但无系统指令时，应创建包含提示的系统指令
func TestConvertClaudeRequestToAntigravity_ToolAndThinking_NoExistingSystem(t *testing.T) {
	// When tools + thinking but no system instruction, should create one with hint
	inputJSON := []byte(`{
		"model": "claude-sonnet-4-5-thinking",
		"messages": [{"role": "user", "content": [{"type": "text", "text": "Hello"}]}],
		"tools": [
			{
				"name": "get_weather",
				"description": "Get weather",
				"input_schema": {"type": "object", "properties": {"location": {"type": "string"}}}
			}
		],
		"thinking": {"type": "enabled", "budget_tokens": 8000}
	}`)

	output := ConvertClaudeRequestToAntigravity("claude-sonnet-4-5-thinking", inputJSON, false)
	outputStr := string(output)

	// System instruction should be created with hint
	sysInstruction := gjson.Get(outputStr, "request.systemInstruction")
	if !sysInstruction.Exists() {
		t.Fatal("systemInstruction should be created when tools + thinking are active")
	}

	sysText := sysInstruction.Get("parts").Array()
	found := false
	for _, part := range sysText {
		if strings.Contains(part.Get("text").String(), "Interleaved thinking is enabled") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Interleaved thinking hint should be in created systemInstruction, got: %v", sysInstruction.Raw)
	}
}
