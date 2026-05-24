// 包 logging - requestmeta.go
// 该文件提供了请求元数据的上下文存储功能。
// 用于在请求处理链中传递端点名称、响应状态码和响应头等元数据。
package logging

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
)

// endpointKey 是用于存储/检索端点名称的上下文键。
type endpointKey struct{}

// responseStatusKey 是用于存储/检索响应状态码持有者的上下文键。
type responseStatusKey struct{}

// responseHeadersKey 是用于存储/检索响应头持有者的上下文键。
type responseHeadersKey struct{}

// responseStatusHolder 使用原子操作存储响应状态码，支持并发安全访问。
type responseStatusHolder struct {
	// status 是原子存储的 HTTP 状态码
	status atomic.Int32
}

// responseHeadersHolder 使用读写互斥锁保护响应头的并发访问。
type responseHeadersHolder struct {
	// mu 保护 headers 的并发读写
	mu sync.RWMutex
	// headers 存储 HTTP 响应头
	headers http.Header
}

// WithEndpoint 返回附加了端点名称的新上下文。
//
// 参数：
//   - ctx: 父上下文，为 nil 时使用 Background
//   - endpoint: 端点名称
//
// 返回：
//   - context.Context: 附加了端点名称的上下文
func WithEndpoint(ctx context.Context, endpoint string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, endpointKey{}, endpoint)
}

// GetEndpoint 从上下文中检索端点名称。
// 未找到时返回空字符串。
func GetEndpoint(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if endpoint, ok := ctx.Value(endpointKey{}).(string); ok {
		return endpoint
	}
	return ""
}

// WithResponseStatusHolder 返回附加了响应状态码持有者的新上下文。
// 如果上下文中已存在持有者则直接返回原上下文。
func WithResponseStatusHolder(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if holder, ok := ctx.Value(responseStatusKey{}).(*responseStatusHolder); ok && holder != nil {
		return ctx
	}
	return context.WithValue(ctx, responseStatusKey{}, &responseStatusHolder{})
}

// WithResponseHeadersHolder 返回附加了响应头持有者的新上下文。
// 如果上下文中已存在持有者则直接返回原上下文。
func WithResponseHeadersHolder(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if holder, ok := ctx.Value(responseHeadersKey{}).(*responseHeadersHolder); ok && holder != nil {
		return ctx
	}
	return context.WithValue(ctx, responseHeadersKey{}, &responseHeadersHolder{})
}

// SetResponseStatus 在上下文中存储响应状态码（线程安全）。
// 状态码 <= 0 时忽略。
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

// SetResponseHeaders 在上下文中存储响应头（线程安全）。
// 存储的是响应头的深拷贝，修改原始头不影响已存储的副本。
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

// GetResponseStatus 从上下文中检索响应状态码。
// 未找到时返回 0。
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

// GetResponseHeaders 从上下文中检索响应头（返回深拷贝）。
// 未找到时返回 nil。
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

// cloneHTTPHeader 深拷贝 HTTP 响应头。
// 输入为空时返回 nil。
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
