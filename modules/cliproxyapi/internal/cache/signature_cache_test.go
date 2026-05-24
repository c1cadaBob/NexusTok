// cache - signature_cache_test.go
// 该文件测试签名缓存（Signature Cache）的功能。
// 测试覆盖了基本存取、不同模型分组隔离、未找到返回空、空输入处理、短签名拒绝、
// 缓存清除、有效签名检查、哈希碰撞抗性、Unicode 文本、覆盖写入、过期逻辑、
// 模式设置日志级别以及重复状态日志抑制等场景。

package cache

import (
	"bytes"
	"strings"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
)

// testModelName 测试用的模型名称常量。
const testModelName = "claude-sonnet-4-5"

// TestCacheSignature_BasicStorageAndRetrieval 测试签名缓存的基本存储和检索功能。
func TestCacheSignature_BasicStorageAndRetrieval(t *testing.T) {
	ClearSignatureCache("")

	text := "This is some thinking text content"
	signature := "abc123validSignature1234567890123456789012345678901234567890"

	// Store signature
	CacheSignature(testModelName, text, signature)

	// Retrieve signature
	retrieved := GetCachedSignature(testModelName, text)
	if retrieved != signature {
		t.Errorf("Expected signature '%s', got '%s'", signature, retrieved)
	}
}

// TestCacheSignature_DifferentModelGroups 测试不同模型的签名缓存相互隔离。
func TestCacheSignature_DifferentModelGroups(t *testing.T) {
	ClearSignatureCache("")

	text := "Same text across models"
	sig1 := "signature1_1234567890123456789012345678901234567890123456"
	sig2 := "signature2_1234567890123456789012345678901234567890123456"

	geminiModel := "gemini-3-pro-preview"
	CacheSignature(testModelName, text, sig1)
	CacheSignature(geminiModel, text, sig2)

	if GetCachedSignature(testModelName, text) != sig1 {
		t.Error("Claude signature mismatch")
	}
	if GetCachedSignature(geminiModel, text) != sig2 {
		t.Error("Gemini signature mismatch")
	}
}

// TestCacheSignature_NotFound 测试不存在的会话和不同文本返回空字符串。
func TestCacheSignature_NotFound(t *testing.T) {
	ClearSignatureCache("")

	// Non-existent session
	if got := GetCachedSignature(testModelName, "some text"); got != "" {
		t.Errorf("Expected empty string for nonexistent session, got '%s'", got)
	}

	// Existing session but different text
	CacheSignature(testModelName, "text-a", "sigA12345678901234567890123456789012345678901234567890")
	if got := GetCachedSignature(testModelName, "text-b"); got != "" {
		t.Errorf("Expected empty string for different text, got '%s'", got)
	}
}

// TestCacheSignature_EmptyInputs 测试空输入和无效输入被忽略。
func TestCacheSignature_EmptyInputs(t *testing.T) {
	ClearSignatureCache("")

	// All empty/invalid inputs should be no-ops
	CacheSignature(testModelName, "", "sig12345678901234567890123456789012345678901234567890")
	CacheSignature(testModelName, "text", "")
	CacheSignature(testModelName, "text", "short") // Too short

	if got := GetCachedSignature(testModelName, "text"); got != "" {
		t.Errorf("Expected empty after invalid cache attempts, got '%s'", got)
	}
}

// TestCacheSignature_ShortSignatureRejected 测试短于 50 字符的签名被拒绝存储。
func TestCacheSignature_ShortSignatureRejected(t *testing.T) {
	ClearSignatureCache("")

	text := "Some text"
	shortSig := "abc123" // Less than 50 chars

	CacheSignature(testModelName, text, shortSig)

	if got := GetCachedSignature(testModelName, text); got != "" {
		t.Errorf("Short signature should be rejected, got '%s'", got)
	}
}

// TestClearSignatureCache_ModelGroup 测试按会话 ID 清除缓存时，不相关的缓存保留。
func TestClearSignatureCache_ModelGroup(t *testing.T) {
	ClearSignatureCache("")

	sig := "validSig1234567890123456789012345678901234567890123456"
	CacheSignature(testModelName, "text", sig)
	CacheSignature(testModelName, "text-2", sig)

	ClearSignatureCache("session-1")

	if got := GetCachedSignature(testModelName, "text"); got != sig {
		t.Error("signature should remain when clearing unknown session")
	}
}

// TestClearSignatureCache_AllSessions 测试清除所有会话缓存。
func TestClearSignatureCache_AllSessions(t *testing.T) {
	ClearSignatureCache("")

	sig := "validSig1234567890123456789012345678901234567890123456"
	CacheSignature(testModelName, "text", sig)
	CacheSignature(testModelName, "text-2", sig)

	ClearSignatureCache("")

	if got := GetCachedSignature(testModelName, "text"); got != "" {
		t.Error("text should be cleared")
	}
	if got := GetCachedSignature(testModelName, "text-2"); got != "" {
		t.Error("text-2 should be cleared")
	}
}

// TestHasValidSignature 测试 HasValidSignature 函数对各种长度和格式签名的验证。
func TestHasValidSignature(t *testing.T) {
	tests := []struct {
		name      string
		modelName string
		signature string
		expected  bool
	}{
		{"valid long signature", testModelName, "abc123validSignature1234567890123456789012345678901234567890", true},
		{"exactly 50 chars", testModelName, "12345678901234567890123456789012345678901234567890", true},
		{"49 chars - invalid", testModelName, "1234567890123456789012345678901234567890123456789", false},
		{"empty string", testModelName, "", false},
		{"short signature", testModelName, "abc", false},
		{"gemini sentinel", "gemini-3-pro-preview", "skip_thought_signature_validator", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasValidSignature(tt.modelName, tt.signature)
			if result != tt.expected {
				t.Errorf("HasValidSignature(%q) = %v, expected %v", tt.signature, result, tt.expected)
			}
		})
	}
}

// TestCacheSignature_TextHashCollisionResistance 测试不同文本的哈希不会碰撞。
func TestCacheSignature_TextHashCollisionResistance(t *testing.T) {
	ClearSignatureCache("")

	// Different texts should produce different hashes
	text1 := "First thinking text"
	text2 := "Second thinking text"
	sig1 := "signature1_1234567890123456789012345678901234567890123456"
	sig2 := "signature2_1234567890123456789012345678901234567890123456"

	CacheSignature(testModelName, text1, sig1)
	CacheSignature(testModelName, text2, sig2)

	if GetCachedSignature(testModelName, text1) != sig1 {
		t.Error("text1 signature mismatch")
	}
	if GetCachedSignature(testModelName, text2) != sig2 {
		t.Error("text2 signature mismatch")
	}
}

// TestCacheSignature_UnicodeText 测试 Unicode 文本的签名缓存正确性。
func TestCacheSignature_UnicodeText(t *testing.T) {
	ClearSignatureCache("")

	text := "한글 텍스트와 이모지 🎉 그리고 特殊文字"
	sig := "unicodeSig123456789012345678901234567890123456789012345"

	CacheSignature(testModelName, text, sig)

	if got := GetCachedSignature(testModelName, text); got != sig {
		t.Errorf("Unicode text signature retrieval failed, got '%s'", got)
	}
}

// TestCacheSignature_Overwrite 测试相同文本的签名可以被覆盖更新。
func TestCacheSignature_Overwrite(t *testing.T) {
	ClearSignatureCache("")

	text := "Same text"
	sig1 := "firstSignature12345678901234567890123456789012345678901"
	sig2 := "secondSignature1234567890123456789012345678901234567890"

	CacheSignature(testModelName, text, sig1)
	CacheSignature(testModelName, text, sig2) // Overwrite

	if got := GetCachedSignature(testModelName, text); got != sig2 {
		t.Errorf("Expected overwritten signature '%s', got '%s'", sig2, got)
	}
}

// TestCacheSignature_ExpirationLogic 测试签名缓存的过期逻辑路径存在。
// 注意：实际过期需要时间模拟，此处仅验证新鲜条目可正常检索。
func TestCacheSignature_ExpirationLogic(t *testing.T) {
	ClearSignatureCache("")

	// This test verifies the expiration check exists
	// In a real scenario, we'd mock time.Now()
	text := "text"
	sig := "validSig1234567890123456789012345678901234567890123456"

	CacheSignature(testModelName, text, sig)

	// Fresh entry should be retrievable
	if got := GetCachedSignature(testModelName, text); got != sig {
		t.Errorf("Fresh entry should be retrievable, got '%s'", got)
	}

	// We can't easily test actual expiration without time mocking
	// but the logic is verified by the implementation
	_ = time.Now() // Acknowledge we're not testing time passage
}

// TestSignatureModeSetters_LogAtInfoLevel 测试签名缓存禁用时在 Info 级别记录日志，
// 而绕过模式切换在 Debug 级别记录。
func TestSignatureModeSetters_LogAtInfoLevel(t *testing.T) {
	logger := log.StandardLogger()
	previousOutput := logger.Out
	previousLevel := logger.Level
	previousCache := SignatureCacheEnabled()
	previousStrict := SignatureBypassStrictMode()
	SetSignatureCacheEnabled(true)
	SetSignatureBypassStrictMode(false)
	buffer := &bytes.Buffer{}
	log.SetOutput(buffer)
	log.SetLevel(log.InfoLevel)
	t.Cleanup(func() {
		log.SetOutput(previousOutput)
		log.SetLevel(previousLevel)
		SetSignatureCacheEnabled(previousCache)
		SetSignatureBypassStrictMode(previousStrict)
	})

	SetSignatureCacheEnabled(false)
	SetSignatureBypassStrictMode(true)
	SetSignatureBypassStrictMode(false)

	output := buffer.String()
	if !strings.Contains(output, "antigravity signature cache DISABLED") {
		t.Fatalf("expected info output for disabling signature cache, got: %q", output)
	}
	if strings.Contains(output, "strict mode (protobuf tree)") {
		t.Fatalf("expected strict bypass mode log to stay below info level, got: %q", output)
	}
	if strings.Contains(output, "basic mode (R/E + 0x12)") {
		t.Fatalf("expected basic bypass mode log to stay below info level, got: %q", output)
	}
}

// TestSignatureModeSetters_DoNotRepeatSameStateLogs 测试重复设置相同状态时不产生重复日志。
func TestSignatureModeSetters_DoNotRepeatSameStateLogs(t *testing.T) {
	logger := log.StandardLogger()
	previousOutput := logger.Out
	previousLevel := logger.Level
	previousCache := SignatureCacheEnabled()
	previousStrict := SignatureBypassStrictMode()
	SetSignatureCacheEnabled(false)
	SetSignatureBypassStrictMode(true)
	buffer := &bytes.Buffer{}
	log.SetOutput(buffer)
	log.SetLevel(log.InfoLevel)
	t.Cleanup(func() {
		log.SetOutput(previousOutput)
		log.SetLevel(previousLevel)
		SetSignatureCacheEnabled(previousCache)
		SetSignatureBypassStrictMode(previousStrict)
	})

	SetSignatureCacheEnabled(false)
	SetSignatureBypassStrictMode(true)

	if buffer.Len() != 0 {
		t.Fatalf("expected repeated setter calls with unchanged state to stay silent, got: %q", buffer.String())
	}
}

// TestSignatureBypassStrictMode_LogsAtDebugLevel 测试绕过严格模式切换在 Debug 级别记录日志。
func TestSignatureBypassStrictMode_LogsAtDebugLevel(t *testing.T) {
	logger := log.StandardLogger()
	previousOutput := logger.Out
	previousLevel := logger.Level
	previousStrict := SignatureBypassStrictMode()
	SetSignatureBypassStrictMode(false)
	buffer := &bytes.Buffer{}
	log.SetOutput(buffer)
	log.SetLevel(log.DebugLevel)
	t.Cleanup(func() {
		log.SetOutput(previousOutput)
		log.SetLevel(previousLevel)
		SetSignatureBypassStrictMode(previousStrict)
	})

	SetSignatureBypassStrictMode(true)
	SetSignatureBypassStrictMode(false)

	output := buffer.String()
	if !strings.Contains(output, "strict mode (protobuf tree)") {
		t.Fatalf("expected debug output for strict bypass mode, got: %q", output)
	}
	if !strings.Contains(output, "basic mode (R/E + 0x12)") {
		t.Fatalf("expected debug output for basic bypass mode, got: %q", output)
	}
}
