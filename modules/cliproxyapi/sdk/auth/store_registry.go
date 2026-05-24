// auth - store_registry.go
// 本文件实现了全局令牌存储注册表，提供线程安全的令牌存储后端注册与获取机制。
// 默认使用基于文件系统的 FileTokenStore 作为存储后端。
package auth

import (
	"sync"

	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

var (
	// storeMu 用于保护 registeredStore 的并发读写访问。
	storeMu sync.RWMutex

	// registeredStore 是全局注册的令牌存储实例。
	// 初始为 nil，首次通过 GetTokenStore 获取时自动创建 FileTokenStore。
	registeredStore coreauth.Store
)

// RegisterTokenStore 设置全局令牌存储后端。
// 该存储后端将被认证管理器用于持久化认证记录。
// 调用此方法会替换之前注册的存储后端。
// 参数 store 可为 nil，但通常应提供一个有效的存储实现。
func RegisterTokenStore(store coreauth.Store) {
	storeMu.Lock()
	registeredStore = store
	storeMu.Unlock()
}

// GetTokenStore 返回全局注册的令牌存储后端。
// 如果尚未注册任何存储后端，则自动创建一个默认的 FileTokenStore 实例。
// 该方法使用双重检查锁定模式确保线程安全的懒初始化。
func GetTokenStore() coreauth.Store {
	// 快速路径：使用读锁检查是否已初始化
	storeMu.RLock()
	s := registeredStore
	storeMu.RUnlock()
	if s != nil {
		return s
	}
	// 慢速路径：使用写锁进行懒初始化
	storeMu.Lock()
	defer storeMu.Unlock()
	if registeredStore == nil {
		registeredStore = NewFileTokenStore()
	}
	return registeredStore
}
