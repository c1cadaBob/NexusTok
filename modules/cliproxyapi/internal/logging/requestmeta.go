// logging - requestmeta.go
// 本文件提供请求元数据在 context 中的存储与检索功能。
// 包括 API 端点标识、响应状态码和响应头的上下文传递机制，
// 用于在请求处理链的各个阶段共享请求级元数据。
package logging

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
)

// endpointKey 是用于在 context 中存储端点标识的键类型。
type endpointKey struct{}

// responseStatusKey 是用于在 context 中存储响应状态码持有者的键类型。
type responseStatusKey struct{}

// responseHeadersKey 是用于在 context 中存储响应头持有者的键类型。
type responseHeadersKey struct{}

// responseStatusHolder 线程安全地持有 HTTP 响应状态码。
type responseStatusHolder struct {
	// status 使用原子操作存储状态码，避免竞态条件。
	status atomic.Int32
}

// responseHeadersHolder 线程安全地持有 HTTP 响应头。
type responseHeadersHolder struct {
	// mu 保护 headers 的并发读写。
	mu      sync.RWMutex
	// headers 存储 HTTP 响应头的副本。
	headers http.Header
}

// WithEndpoint 将端点标识存入 context 中。
//
// 参数：
//   - ctx: 父 context
//   - endpoint: 端点标识字符串（如 "claude"、"gemini" 等）
//
// 返回值：
//   - context.Context: 包含端点标识的新 context
func WithEndpoint(ctx context.Context, endpoint string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, endpointKey{}, endpoint)
}

// GetEndpoint 从 context 中获取端点标识。
//
// 参数：
//   - ctx: 包含端点标识的 context
//
// 返回值：
//   - string: 端点标识字符串，未找到时返回空字符串
func GetEndpoint(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if endpoint, ok := ctx.Value(endpointKey{}).(string); ok {
		return endpoint
	}
	return ""
}

// WithResponseStatusHolder 在 context 中注入一个响应状态码持有者。
// 如果 context 中已存在持有者，则直接返回原 context 避免重复注入。
//
// 参数：
//   - ctx: 父 context
//
// 返回值：
//   - context.Context: 包含响应状态码持有者的新 context
func WithResponseStatusHolder(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if holder, ok := ctx.Value(responseStatusKey{}).(*responseStatusHolder); ok && holder != nil {
		return ctx
	}
	return context.WithValue(ctx, responseStatusKey{}, &responseStatusHolder{})
}

// WithResponseHeadersHolder 在 context 中注入一个响应头持有者。
// 如果 context 中已存在持有者，则直接返回原 context 避免重复注入。
//
// 参数：
//   - ctx: 父 context
//
// 返回值：
//   - context.Context: 包含响应头持有者的新 context
func WithResponseHeadersHolder(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if holder, ok := ctx.Value(responseHeadersKey{}).(*responseHeadersHolder); ok && holder != nil {
		return ctx
	}
	return context.WithValue(ctx, responseHeadersKey{}, &responseHeadersHolder{})
}

// SetResponseStatus 将 HTTP 响应状态码存入 context 中的持有者。
// 状态码必须大于 0 才会被存储。
//
// 参数：
//   - ctx: 包含响应状态码持有者的 context
//   - status: HTTP 响应状态码
func SetResponseStatus(ctx context.Context, status int) {
	if ctx == nil || status <= 0 {
		return
	}
	holder, ok := ctx.Value(responseStatusKey{}).(*responseStatusHolder)
	if !ok || holder == nil {
		return
	}
	holder.status.Store(int32(status))
}

// SetResponseHeaders 将 HTTP 响应头存入 context 中的持有者。
// 响应头会被深拷贝以避免并发修改问题。
//
// 参数：
//   - ctx: 包含响应头持有者的 context
//   - headers: HTTP 响应头
func SetResponseHeaders(ctx context.Context, headers http.Header) {
	if ctx == nil {
		return
	}
	holder, ok := ctx.Value(responseHeadersKey{}).(*responseHeadersHolder)
	if !ok || holder == nil {
		return
	}
	holder.mu.Lock()
	defer holder.mu.Unlock()
	holder.headers = cloneHTTPHeader(headers)
}

// GetResponseStatus 从 context 中获取 HTTP 响应状态码。
//
// 参数：
//   - ctx: 包含响应状态码持有者的 context
//
// 返回值：
//   - int: HTTP 响应状态码，未找到时返回 0
func GetResponseStatus(ctx context.Context) int {
	if ctx == nil {
		return 0
	}
	holder, ok := ctx.Value(responseStatusKey{}).(*responseStatusHolder)
	if !ok || holder == nil {
		return 0
	}
	return int(holder.status.Load())
}

// GetResponseHeaders 从 context 中获取 HTTP 响应头的副本。
//
// 参数：
//   - ctx: 包含响应头持有者的 context
//
// 返回值：
//   - http.Header: HTTP 响应头的深拷贝副本，未找到时返回 nil
func GetResponseHeaders(ctx context.Context) http.Header {
	if ctx == nil {
		return nil
	}
	holder, ok := ctx.Value(responseHeadersKey{}).(*responseHeadersHolder)
	if !ok || holder == nil {
		return nil
	}
	holder.mu.RLock()
	defer holder.mu.RUnlock()
	return cloneHTTPHeader(holder.headers)
}

// cloneHTTPHeader 深拷贝一个 HTTP 响应头映射。
//
// 参数：
//   - src: 源 HTTP 响应头
//
// 返回值：
//   - http.Header: 深拷贝后的 HTTP 响应头，源为空时返回 nil
func cloneHTTPHeader(src http.Header) http.Header {
	if len(src) == 0 {
		return nil
	}
	dst := make(http.Header, len(src))
	for key, values := range src {
		dst[key] = append([]string(nil), values...)
	}
	return dst
}
