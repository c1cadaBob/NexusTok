// 包 auth - store.go
// 该文件定义了认证状态持久化的 Store 接口。
// Store 接口抽象了跨重启的 Auth 状态存储操作，支持列举、保存和删除认证记录。
package auth

import "context"

// Store 抽象了跨重启的 Auth 状态持久化操作。
type Store interface {
	// List 返回后端存储中所有认证记录。
	List(ctx context.Context) ([]*Auth, error)
	// Save 持久化提供的认证记录，替换具有相同 ID 的现有记录。
	Save(ctx context.Context, auth *Auth) (string, error)
	// Delete 删除由 id 标识的认证记录。
	Delete(ctx context.Context, id string) error
}
