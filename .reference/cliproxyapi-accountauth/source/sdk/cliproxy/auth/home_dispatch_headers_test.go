// auth - home_dispatch_headers_test.go
// Home 调度请求头测试
// 验证 homeDispatchHeaders 函数在 Home 集群调度时
// 正确处理凭证请求头的添加和保留逻辑。
package auth

import (
	"context"
	"net/http"
	"testing"
)

// homeDispatchTestGinContext 是用于测试的 Gin 上下文模拟实现。
type homeDispatchTestGinContext struct {
	values map[string]any
	query  map[string]string
}

func (c homeDispatchTestGinContext) Get(key string) (any, bool) {
	v, ok := c.values[key]
	return v, ok
}

func (c homeDispatchTestGinContext) Query(key string) string {
	if c.query == nil {
		return ""
	}
	return c.query[key]
}

// TestHomeDispatchHeadersAddsQueryKeyCredential 验证：
// 当查询参数中包含 key 时，homeDispatchHeaders 会添加 X-Goog-Api-Key 请求头。
func TestHomeDispatchHeadersAddsQueryKeyCredential(t *testing.T) {
	ginCtx := homeDispatchTestGinContext{query: map[string]string{"key": "12345"}}
	ctx := context.WithValue(context.Background(), "gin", ginCtx)
	headers := http.Header{"User-Agent": {"client"}}

	got := homeDispatchHeaders(ctx, headers)

	if got.Get("X-Goog-Api-Key") != "12345" {
		t.Fatalf("X-Goog-Api-Key = %q, want %q", got.Get("X-Goog-Api-Key"), "12345")
	}
	if headers.Get("X-Goog-Api-Key") != "" {
		t.Fatalf("original headers were mutated: %v", headers)
	}
}

// TestHomeDispatchHeadersAddsQueryCredentialFromAccessMetadata 验证：
// 当访问元数据的来源为 query-key 时，从 userApiKey 添加 X-Goog-Api-Key 请求头。
func TestHomeDispatchHeadersAddsQueryCredentialFromAccessMetadata(t *testing.T) {
	ginCtx := homeDispatchTestGinContext{values: map[string]any{
		"accessMetadata": map[string]string{"source": "query-key"},
		"userApiKey":     "12345",
	}}
	ctx := context.WithValue(context.Background(), "gin", ginCtx)
	headers := http.Header{"User-Agent": {"client"}}

	got := homeDispatchHeaders(ctx, headers)

	if got.Get("X-Goog-Api-Key") != "12345" {
		t.Fatalf("X-Goog-Api-Key = %q, want %q", got.Get("X-Goog-Api-Key"), "12345")
	}
	if headers.Get("X-Goog-Api-Key") != "" {
		t.Fatalf("original headers were mutated: %v", headers)
	}
}

// TestHomeDispatchHeadersKeepsExistingCredentialHeader 验证：
// 当请求头中已存在 X-Goog-Api-Key 时，不会被覆盖。
func TestHomeDispatchHeadersKeepsExistingCredentialHeader(t *testing.T) {
	ginCtx := homeDispatchTestGinContext{query: map[string]string{"key": "query-key"}}
	ctx := context.WithValue(context.Background(), "gin", ginCtx)
	headers := http.Header{"X-Goog-Api-Key": {"header-key"}}

	got := homeDispatchHeaders(ctx, headers)

	if got.Get("X-Goog-Api-Key") != "header-key" {
		t.Fatalf("X-Goog-Api-Key = %q, want %q", got.Get("X-Goog-Api-Key"), "header-key")
	}
}

// TestHomeDispatchHeadersIgnoresHeaderCredentialSource 验证：
// 当访问元数据的来源为 authorization（而非 query-key）时，
// 不会添加 X-Goog-Api-Key 请求头。
func TestHomeDispatchHeadersIgnoresHeaderCredentialSource(t *testing.T) {
	ginCtx := homeDispatchTestGinContext{values: map[string]any{
		"accessMetadata": map[string]string{"source": "authorization"},
		"userApiKey":     "12345",
	}}
	ctx := context.WithValue(context.Background(), "gin", ginCtx)
	headers := http.Header{"Authorization": {"Bearer 12345"}}

	got := homeDispatchHeaders(ctx, headers)

	if got.Get("X-Goog-Api-Key") != "" {
		t.Fatalf("X-Goog-Api-Key = %q, want empty", got.Get("X-Goog-Api-Key"))
	}
	if got.Get("Authorization") != "Bearer 12345" {
		t.Fatalf("Authorization = %q, want %q", got.Get("Authorization"), "Bearer 12345")
	}
}
