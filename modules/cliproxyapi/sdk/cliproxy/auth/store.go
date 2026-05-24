// auth - store.go
// 定义认证状态持久化接口，抽象 Auth 记录的增删查操作，支持跨重启的状态恢复。
package auth

import "context"

// Store 抽象认证状态的持久化存储，支持跨重启恢复 Auth 记录。
type Store interface {
	// List 返回存储后端中所有的认证记录。
	List(ctx context.Context) ([]*Auth, error)
	// Save 持久化指定的认证记录，替换已存在的具有相同 ID 的记录。
	Save(ctx context.Context, auth *Auth) (string, error)
	// Delete 删除由 id 标识的认证记录。
	Delete(ctx context.Context, id string) error
}
