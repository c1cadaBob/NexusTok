// 包 auth - store_registry.go
// 该文件提供了全局令牌存储的注册和获取机制。
// 通过 RegisterTokenStore 和 GetTokenStore 函数管理全局单例的令牌存储实例。
package auth

import (
	"sync"

	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

var (
	storeMu         sync.RWMutex   // 保护全局令牌存储的读写锁
	registeredStore coreauth.Store // 全局注册的令牌存储实例
)

// RegisterTokenStore 设置全局令牌存储，供认证辅助函数使用。
//
// 参数:
//   - store: 令牌存储实例
func RegisterTokenStore(store coreauth.Store) {
	storeMu.Lock()
	registeredStore = store
	storeMu.Unlock()
}

// GetTokenStore 返回全局注册的令牌存储。
// 如果尚未注册任何存储，会自动创建一个默认的 FileTokenStore。
//
// 返回:
//   - coreauth.Store: 全局令牌存储实例
func GetTokenStore() coreauth.Store {
	storeMu.RLock()
	s := registeredStore
	storeMu.RUnlock()
	if s != nil {
		return s
	}
	storeMu.Lock()
	defer storeMu.Unlock()
	if registeredStore == nil {
		registeredStore = NewFileTokenStore()
	}
	return registeredStore
}
