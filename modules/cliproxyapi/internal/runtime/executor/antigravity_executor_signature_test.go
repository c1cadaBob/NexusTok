// Package executor 提供 CLI Proxy API 运行时执行器的测试。
// 本文件测试 Antigravity 执行器的签名验证功能，包括严格模式和缓存模式。
package executor

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/cache"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
)

// testGeminiSignaturePayload 生成测试用的 Gemini 签名负载。
func testGeminiSignaturePayload() string {
	payload := append([]byte{0x0A}, bytes.Repeat([]byte{0x56}, 48)...)
	return base64.StdEncoding.EncodeToString(payload)
}

// testFakeClaudeSignature 返回一个以 'E' 开头的 base64 字符串，
// 能通过轻量级的 hasValidClaudeSignature 检查但 protobuf 内容无效，
// 在严格模式下会失败深度验证。
func testFakeClaudeSignature() string {
	return base64.StdEncoding.EncodeToString([]byte{0x12, 0xFF, 0xFE, 0xFD})
}

// testAntigravityAuth 创建测试用的 Antigravity 认证对象。
func testAntigravityAuth(baseURL string) *cliproxyauth.Auth {
	return &cliproxyauth.Auth{
		Attributes: map[string]string{
			"base_url": baseURL,
		},
		Metadata: map[string]any{
			"access_token": "token-123",
			"expired":      time.Now().Add(24 * time.Hour).Format(time.RFC3339),
		},
	}
}

// invalidClaudeThinkingPayload 返回包含无效签名的 Claude thinking 负载。
func invalidClaudeThinkingPayload() []byte {
	return []byte(`{
		"model": "claude-sonnet-4-5-thinking",
		"messages": [
			{
				"role": "assistant",
				"content": [
					{"type": "thinking", "thinking": "bad", "signature": "` + testFakeClaudeSignature() + `"},
					{"type": "text", "text": "hello"}
				]
			}
		]
	}`)
}

// TestAntigravityExecutor_StrictBypassRejectsInvalidSignature 验证严格模式下
// 无效签名在请求发送到上游之前被拒绝。
func TestAntigravityExecutor_StrictBypassRejectsInvalidSignature(t *testing.T) {
	previousCache := cache.SignatureCacheEnabled()
	previousStrict := cache.SignatureBypassStrictMode()
	cache.SetSignatureCacheEnabled(false)
	cache.SetSignatureBypassStrictMode(true)
	t.Cleanup(func() {
		cache.SetSignatureCacheEnabled(previousCache)
		cache.SetSignatureBypassStrictMode(previousStrict)
	})

	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"response":{"candidates":[{"content":{"parts":[{"text":"ok"}]}}]}}`))
	}))
	defer server.Close()

	executor := NewAntigravityExecutor(nil)
	auth := testAntigravityAuth(server.URL)
	payload := invalidClaudeThinkingPayload()
	opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("claude"), OriginalRequest: payload}
	req := cliproxyexecutor.Request{Model: "claude-sonnet-4-5-thinking", Payload: payload}

	tests := []struct {
		name   string
		invoke func() error
	}{
		{
			name: "execute",
			invoke: func() error {
				_, err := executor.Execute(context.Background(), auth, req, opts)
				return err
			},
		},
		{
			name: "stream",
			invoke: func() error {
				_, err := executor.ExecuteStream(context.Background(), auth, req, cliproxyexecutor.Options{SourceFormat: opts.SourceFormat, OriginalRequest: payload, Stream: true})
				return err
			},
		},
		{
			name: "count tokens",
			invoke: func() error {
				_, err := executor.CountTokens(context.Background(), auth, req, opts)
				return err
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := tt.invoke()
			if err == nil {
				t.Fatal("expected invalid signature to return an error")
			}
			statusProvider, ok := err.(interface{ StatusCode() int })
			if !ok {
				t.Fatalf("expected status error, got %T: %v", err, err)
			}
			if statusProvider.StatusCode() != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", statusProvider.StatusCode(), http.StatusBadRequest)
			}
		})
	}

	if got := hits.Load(); got != 0 {
		t.Fatalf("expected invalid signature to be rejected before upstream request, got %d upstream hits", got)
	}
}

// TestAntigravityExecutor_NonStrictBypassSkipsPrecheck 验证非严格模式下跳过签名预检查。
func TestAntigravityExecutor_NonStrictBypassSkipsPrecheck(t *testing.T) {
	previousCache := cache.SignatureCacheEnabled()
	previousStrict := cache.SignatureBypassStrictMode()
	cache.SetSignatureCacheEnabled(false)
	cache.SetSignatureBypassStrictMode(false)
	t.Cleanup(func() {
		cache.SetSignatureCacheEnabled(previousCache)
		cache.SetSignatureBypassStrictMode(previousStrict)
	})

	payload := invalidClaudeThinkingPayload()
	from := sdktranslator.FromString("claude")

	_, err := validateAntigravityRequestSignatures(from, payload)
	if err != nil {
		t.Fatalf("non-strict bypass should skip precheck, got: %v", err)
	}
}

// TestAntigravityExecutor_CacheModeSkipsPrecheck 验证缓存模式下跳过签名预检查。
func TestAntigravityExecutor_CacheModeSkipsPrecheck(t *testing.T) {
	previous := cache.SignatureCacheEnabled()
	cache.SetSignatureCacheEnabled(true)
	t.Cleanup(func() {
		cache.SetSignatureCacheEnabled(previous)
	})

	payload := invalidClaudeThinkingPayload()
	from := sdktranslator.FromString("claude")

	_, err := validateAntigravityRequestSignatures(from, payload)
	if err != nil {
		t.Fatalf("cache mode should skip precheck, got: %v", err)
	}
}
