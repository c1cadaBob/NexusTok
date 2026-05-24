// access - registry.go
// 本文件实现了全局认证提供者注册表，提供线程安全的提供者注册、注销和查询机制。
// 提供者按注册顺序维护，确保认证时按预期顺序尝试。
package access

import (
	"context"
	"net/http"
	"strings"
	"sync"
)

// Provider 定义了认证提供者的核心接口。
// 每个认证提供者负责验证传入 HTTP 请求中的凭证（如 API Key、Bearer Token 等）。
type Provider interface {
	// Identifier 返回该提供者的唯一标识字符串。
	Identifier() string

	// Authenticate 验证 HTTP 请求中的凭证。
	// 参数说明：
	//   - ctx: 上下文，用于控制请求超时和取消
	//   - r: HTTP 请求，提供者从中提取凭证
	//
	// 返回值：
	//   - *Result: 认证成功时的结果，包含提供者标识和主体信息
	//   - *AuthError: 认证失败时的错误，包含错误分类码和 HTTP 状态码
	Authenticate(ctx context.Context, r *http.Request) (*Result, *AuthError)
}

// Result 封装认证成功的结果信息。
type Result struct {
	// Provider 是执行认证的提供者标识。
	Provider string

	// Principal 是认证通过的主体标识（如用户名、API Key 名称等）。
	Principal string

	// Metadata 是认证过程中提取的附加信息（如权限范围、配额等）。
	Metadata map[string]string
}

var (
	// registryMu 保护注册表的并发读写访问。
	registryMu sync.RWMutex

	// registry 是以类型标识为键的提供者实例映射。
	registry = make(map[string]Provider)

	// order 维护提供者的注册顺序，确保认证时按注册顺序尝试。
	order []string
)

// RegisterProvider 注册一个预构建的提供者实例到全局注册表。
// 如果相同类型的提供者已存在，会被替换但保持原有的注册顺序。
// 参数说明：
//   - typ: 提供者类型标识（去除首尾空白后不能为空）
//   - provider: 提供者实例（不能为 nil）
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
// 如果该类型不存在，则忽略。
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

// RegisteredProviders 返回全局注册表中所有提供者实例的快照。
// 返回的切片按注册顺序排列，可安全并发使用。
// 如果注册表为空，返回 nil。
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
