// 包 auth - persist_policy.go
// 该文件定义了认证持久化策略的上下文控制机制。
// 通过 WithSkipPersist 函数可以禁用特定代码路径的持久化，
// 用于文件监视器事件处理等场景以避免写回循环。
package auth

import "context"

type skipPersistContextKey struct{} // 上下文键类型，用于标记跳过持久化

// WithSkipPersist 返回一个禁用 Manager Update/Register 调用持久化的派生上下文。
// 用于响应文件监视器事件的代码路径，此时磁盘上的文件已是真实来源，
// 再次持久化将创建写回循环。
func WithSkipPersist(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, skipPersistContextKey{}, true)
}

// shouldSkipPersist 检查上下文是否标记为跳过持久化。
func shouldSkipPersist(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v := ctx.Value(skipPersistContextKey{})
	enabled, ok := v.(bool)
	return ok && enabled
}
