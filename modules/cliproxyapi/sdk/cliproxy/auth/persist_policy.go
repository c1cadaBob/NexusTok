// auth - persist_policy.go
// 提供持久化策略控制机制，允许调用方通过 context 标记跳过认证信息的持久化操作。
// 主要用于文件监视器响应配置文件变更时，避免产生回写循环。
package auth

import "context"

// skipPersistContextKey 是用于在 context 中存储跳过持久化标记的键类型。
type skipPersistContextKey struct{}

// WithSkipPersist returns a derived context that disables persistence for Manager Update/Register calls.
// It is intended for code paths that are reacting to file watcher events, where the file on disk is
// already the source of truth and persisting again would create a write-back loop.
func WithSkipPersist(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, skipPersistContextKey{}, true)
}

// shouldSkipPersist 检查 context 中是否设置了跳过持久化的标记。
// 返回 true 表示应跳过持久化操作。
func shouldSkipPersist(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v := ctx.Value(skipPersistContextKey{})
	enabled, ok := v.(bool)
	return ok && enabled
}
