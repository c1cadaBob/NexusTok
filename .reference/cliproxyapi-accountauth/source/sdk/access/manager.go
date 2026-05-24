// 包 access - manager.go
// 该文件定义了请求认证管理器，用于协调多个认证提供者的认证流程。
// 管理器按顺序遍历已注册的提供者，直到有一个成功处理请求。
package access

import (
	"context"
	"net/http"
	"sync"
)

// Manager 协调多个请求认证提供者。
type Manager struct {
	mu        sync.RWMutex // 保护 providers 列表的读写锁
	providers []Provider   // 已注册的认证提供者列表
}

// NewManager 构造一个空的认证管理器实例。
//
// 返回:
//   - *Manager: 认证管理器实例
func NewManager() *Manager {
	return &Manager{}
}

// SetProviders 替换当前活跃的提供者列表。内部会进行克隆以避免外部修改影响。
//
// 参数:
//   - providers: 新的提供者列表
func (m *Manager) SetProviders(providers []Provider) {
	if m == nil {
		return
	}
	cloned := make([]Provider, len(providers))
	copy(cloned, providers)
	m.mu.Lock()
	m.providers = cloned
	m.mu.Unlock()
}

// Providers 返回当前活跃提供者的快照。返回的是副本，修改不会影响内部状态。
//
// 返回:
//   - []Provider: 提供者列表快照
func (m *Manager) Providers() []Provider {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	snapshot := make([]Provider, len(m.providers))
	copy(snapshot, m.providers)
	return snapshot
}

// Authenticate 按顺序遍历提供者直到有一个成功认证。
// 返回第一个成功的认证结果；如果全部失败，根据错误类型返回最合适的错误。
//
// 参数:
//   - ctx: 请求上下文
//   - r: HTTP 请求
//
// 返回:
//   - *Result: 认证成功的结果
//   - *AuthError: 认证失败的错误（成功时为 nil）
func (m *Manager) Authenticate(ctx context.Context, r *http.Request) (*Result, *AuthError) {
	if m == nil {
		return nil, nil
	}
	providers := m.Providers()
	if len(providers) == 0 {
		return nil, nil
	}

	var (
		missing bool
		invalid bool
	)

	for _, provider := range providers {
		if provider == nil {
			continue
		}
		res, authErr := provider.Authenticate(ctx, r)
		if authErr == nil {
			return res, nil
		}
		if IsAuthErrorCode(authErr, AuthErrorCodeNotHandled) {
			continue
		}
		if IsAuthErrorCode(authErr, AuthErrorCodeNoCredentials) {
			missing = true
			continue
		}
		if IsAuthErrorCode(authErr, AuthErrorCodeInvalidCredential) {
			invalid = true
			continue
		}
		return nil, authErr
	}

	if invalid {
		return nil, NewInvalidCredentialError()
	}
	if missing {
		return nil, NewNoCredentialsError()
	}
	return nil, NewNoCredentialsError()
}
