// executor - context.go
// 提供基于 context 的下游 WebSocket 连接标记功能，
// 允许请求处理链中标记和检测当前请求是否来自下游 WebSocket 连接。
package executor

import "context"

// downstreamWebsocketContextKey 是用于在 context 中存储下游 WebSocket 标记的键类型。
type downstreamWebsocketContextKey struct{}

// WithDownstreamWebsocket marks the current request as coming from a downstream websocket connection.
func WithDownstreamWebsocket(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, downstreamWebsocketContextKey{}, true)
}

// DownstreamWebsocket reports whether the current request originates from a downstream websocket connection.
func DownstreamWebsocket(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	raw := ctx.Value(downstreamWebsocketContextKey{})
	enabled, ok := raw.(bool)
	return ok && enabled
}
