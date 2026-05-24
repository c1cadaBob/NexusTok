// pipeline - context.go
// 该文件定义了 CLI Proxy API 管道执行的上下文结构，封装了请求、选项、认证凭据、
// 转换器管道和 HTTP 客户端等执行状态，并提供 Hook 接口用于在执行前后注入自定义逻辑。
package pipeline

import (
	"context"
	"net/http"

	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
)

// Context 封装了在中间件、转换器和执行器之间共享的执行状态。
type Context struct {
	// Request 封装了面向上游提供商的请求载荷。
	Request cliproxyexecutor.Request
	// Options 携带执行标志（流式传输、请求头等）。
	Options cliproxyexecutor.Options
	// Auth 引用本次执行选定的认证凭据。
	Auth *cliproxyauth.Auth
	// Translator 表示负责 Schema 适配的转换器管道。
	Translator *sdktranslator.Pipeline
	// HTTPClient 允许中间件为每个请求自定义出站传输层。
	HTTPClient *http.Client
}

// Hook 定义了执行生命周期的中间件回调接口。
type Hook interface {
	// BeforeExecute 在执行器调用前触发，可用于修改请求上下文。
	BeforeExecute(ctx context.Context, execCtx *Context)
	// AfterExecute 在执行器调用后触发，可用于处理响应或错误。
	AfterExecute(ctx context.Context, execCtx *Context, resp cliproxyexecutor.Response, err error)
	// OnStreamChunk 在流式传输过程中每收到一个数据块时触发。
	OnStreamChunk(ctx context.Context, execCtx *Context, chunk cliproxyexecutor.StreamChunk)
}

// HookFunc 聚合了可选的 Hook 回调函数，允许按需实现部分 Hook 方法。
type HookFunc struct {
	Before func(context.Context, *Context)                                                    // 执行前回调
	After  func(context.Context, *Context, cliproxyexecutor.Response, error)                  // 执行后回调
	Stream func(context.Context, *Context, cliproxyexecutor.StreamChunk)                      // 流式数据块回调
}

// BeforeExecute 实现 Hook 接口，调用 Before 回调（若已设置）。
func (h HookFunc) BeforeExecute(ctx context.Context, execCtx *Context) {
	if h.Before != nil {
		h.Before(ctx, execCtx)
	}
}

// AfterExecute 实现 Hook 接口，调用 After 回调（若已设置）。
func (h HookFunc) AfterExecute(ctx context.Context, execCtx *Context, resp cliproxyexecutor.Response, err error) {
	if h.After != nil {
		h.After(ctx, execCtx, resp, err)
	}
}

// OnStreamChunk 实现 Hook 接口，调用 Stream 回调（若已设置）。
func (h HookFunc) OnStreamChunk(ctx context.Context, execCtx *Context, chunk cliproxyexecutor.StreamChunk) {
	if h.Stream != nil {
		h.Stream(ctx, execCtx, chunk)
	}
}

// RoundTripperProvider 允许为每个认证条目注入自定义的 HTTP 传输层。
type RoundTripperProvider interface {
	// RoundTripperFor 返回指定认证条目对应的 HTTP RoundTripper。
	RoundTripperFor(auth *cliproxyauth.Auth) http.RoundTripper
}
