// 包 access - registry.go
// 该文件定义了请求认证提供者的接口、结果类型和全局注册表。
// Provider 接口定义了认证提供者的契约，全局注册表管理提供者的注册和检索。
package access

import (
	"context"
	"net/http"
	"strings"
	"sync"
)

// Provider 定义了请求认证提供者的接口。
// 每个提供者负责验证特定类型的认证凭据。
type Provider interface {
	// Identifier 返回提供者的唯一标识名称。
	Identifier() string
	// Authenticate 验证请求中的认证凭据。
	Authenticate(ctx context.Context, r *http.Request) (*Result, *AuthError)
}

// Result 传达认证结果。
type Result struct {
	Provider  string            // 认证成功的提供者标识
	Principal string            // 认证主体（如用户名、API 密钥标识）
	Metadata  map[string]string // 附加的认证元数据
}

var (
	registryMu sync.RWMutex        // 保护全局注册表的读写锁
	registry   = make(map[string]Provider) // 提供者类型到实例的映射
	order      []string             // 提供者注册顺序
)

// RegisterProvider 注册一个预构建的提供者实例到全局注册表。
// 如果同类型提供者已存在，则替换之。
//
// 参数:
//   - typ: 提供者类型标识
//   - provider: 提供者实例
func RegisterProvider(typ string, provider Provider) {
	normalizedType := strings.TrimSpace(typ)
	if normalizedType == "" || provider == nil {
		return
	}

	registryMu.Lock()
	if _, exists := registry[normalizedType]; !exists {
		order = append(order, normalizedType)
	}
	registry[normalizedType] = provider
	registryMu.Unlock()
}

// UnregisterProvider 从全局注册表中移除指定类型的提供者。
//
// 参数:
//   - typ: 提供者类型标识
func UnregisterProvider(typ string) {
	normalizedType := strings.TrimSpace(typ)
	if normalizedType == "" {
		return
	}
	registryMu.Lock()
	if _, exists := registry[normalizedType]; !exists {
		registryMu.Unlock()
		return
	}
	delete(registry, normalizedType)
	for index := range order {
		if order[index] != normalizedType {
			continue
		}
		order = append(order[:index], order[index+1:]...)
		break
	}
	registryMu.Unlock()
}

// RegisteredProviders 返回全局注册表中按注册顺序排列的提供者实例列表。
//
// 返回:
//   - []Provider: 提供者实例列表；无注册时返回 nil
func RegisteredProviders() []Provider {
	registryMu.RLock()
	if len(order) == 0 {
		registryMu.RUnlock()
		return nil
	}
	providers := make([]Provider, 0, len(order))
	for _, providerType := range order {
		provider, exists := registry[providerType]
		if !exists || provider == nil {
			continue
		}
		providers = append(providers, provider)
	}
	registryMu.RUnlock()
	return providers
}
