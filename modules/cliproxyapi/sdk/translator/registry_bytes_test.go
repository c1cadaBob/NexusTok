// translator - registry_bytes_test.go
// 该文件包含翻译注册表的字节级操作测试。
// 测试流式响应翻译、非流式响应翻译和 token 计数翻译的正确性。

package translator

import (
	"bytes"
	"context"
	"testing"
)

// TestRegistryTranslateStreamReturnsByteChunks 测试流式响应翻译函数返回正确的字节数据块。
func TestRegistryTranslateStreamReturnsByteChunks(t *testing.T) {
	registry := NewRegistry()
	registry.Register(FormatOpenAI, FormatGemini, nil, ResponseTransform{
		Stream: func(ctx context.Context, model string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) [][]byte {
			return [][]byte{append([]byte(nil), rawJSON...)}
		},
	})

	got := registry.TranslateStream(context.Background(), FormatGemini, FormatOpenAI, "model", nil, nil, []byte(`{"chunk":true}`), nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(got))
	}
	if !bytes.Equal(got[0], []byte(`{"chunk":true}`)) {
		t.Fatalf("unexpected chunk: %s", got[0])
	}
}

// TestRegistryTranslateNonStreamReturnsBytes 测试非流式响应翻译函数返回正确的字节数据。
func TestRegistryTranslateNonStreamReturnsBytes(t *testing.T) {
	registry := NewRegistry()
	registry.Register(FormatOpenAI, FormatGemini, nil, ResponseTransform{
		NonStream: func(ctx context.Context, model string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) []byte {
			return append([]byte(nil), rawJSON...)
		},
	})

	got := registry.TranslateNonStream(context.Background(), FormatGemini, FormatOpenAI, "model", nil, nil, []byte(`{"done":true}`), nil)
	if !bytes.Equal(got, []byte(`{"done":true}`)) {
		t.Fatalf("unexpected payload: %s", got)
	}
}

// TestRegistryTranslateTokenCountReturnsBytes 测试 token 计数翻译函数返回正确的字节数据。
func TestRegistryTranslateTokenCountReturnsBytes(t *testing.T) {
	registry := NewRegistry()
	registry.Register(FormatOpenAI, FormatGemini, nil, ResponseTransform{
		TokenCount: func(ctx context.Context, count int64) []byte {
			return []byte(`{"totalTokens":7}`)
		},
	})

	got := registry.TranslateTokenCount(context.Background(), FormatGemini, FormatOpenAI, 7, []byte(`{"fallback":true}`))
	if !bytes.Equal(got, []byte(`{"totalTokens":7}`)) {
		t.Fatalf("unexpected payload: %s", got)
	}
}
