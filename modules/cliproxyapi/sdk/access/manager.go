// access - manager.go
// 本文件实现了认证管理器 Manager，负责协调多个认证提供者的认证流程。
// Manager 维护一个提供者列表，按注册顺序依次尝试认证，直到某个提供者成功或全部失败。
package access

import (
	"context"
	"net/http"
	"sync"
)

// Manager 协调多个认证提供者的认证流程。
// 它维护一个线程安全的提供者列表，按注册顺序依次尝试认证。
// 当某个提供者返回成功结果时，立即返回；当所有提供者都未处理或失败时，
// 根据错误类型返回相应的认证错误。
type Manager struct {
	// mu 保护 providers 列表的并发读写访问。
	mu sync.RWMutex

	// providers 是已注册的认证提供者列表，按注册顺序排列。
	providers []Provider
}

// NewManager 创建一个空的认证管理器实例。
func NewManager() *Manager {
	return &Manager{}
}

// SetProviders 替换当前活跃的提供者列表。
// 该方法会克隆传入的切片，避免外部修改影响内部状态。
// 如果 Manager 为 nil，则忽略此次调用。
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

// Providers 返回当前活跃提供者列表的快照。
// 返回的切片是内部列表的副本，可安全并发使用。
// 如果 Manager 为 nil，返回 nil。
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

// Authenticate 按顺序遍历所有提供者执行认证，直到某个提供者成功或全部遍历完毕。
// 认证逻辑：
//  1. 遍历所有提供者，调用其 Authenticate 方法
//  2. 如果某个提供者返回成功结果，立即返回
//  3. 如果返回 NotHandled 错误，跳过该提供者继续下一个
//  4. 如果返回 NoCredentials 或 InvalidCredential 错误，记录但继续尝试下一个
//  5. 其他错误直接返回
//  6. 全部遍历完毕后，根据记录的错误类型返回（InvalidCredential 优先于 NoCredentials）
//
// 参数说明：
//   - ctx: 上下文，用于控制请求超时和取消
//   - r: HTTP 请求，传递给各提供者进行凭证提取和验证
//
// 返回值为认证结果和认证错误（成功时错误为 nil）。
func (m *Manager) Authenticate(ctx context.Context, r *http.Request) (*Result, *AuthError) {
	if m == nil {
		return nil, nil
	}
	providers := m.Providers()
	if len(providers) == 0 {
		return nil, nil
	}

	var (
		missing bool // 是否有提供者报告缺少凭证
		invalid bool // 是否有提供者报告凭证无效
	)

	for _, provider := range providers {
		if provider == nil {
			continue
		}
		res, authErr := provider.Authenticate(ctx, r)
		if authErr == nil {
			// 认证成功，立即返回结果
			return res, nil
		}
		if IsAuthErrorCode(authErr, AuthErrorCodeNotHandled) {
			// 当前提供者不处理该请求，跳过
			continue
		}
		if IsAuthErrorCode(authErr, AuthErrorCodeNoCredentials) {
			// 请求中缺少凭证，记录并继续
			missing = true
			continue
		}
		if IsAuthErrorCode(authErr, AuthErrorCodeInvalidCredential) {
			// 凭证无效，记录并继续
			invalid = true
			continue
		}
		// 其他错误（如内部错误），直接返回
		return nil, authErr
	}

	// 全部提供者遍历完毕，根据错误类型返回
	if invalid {
		return nil, NewInvalidCredentialError()
	}
	if missing {
		return nil, NewNoCredentialsError()
	}
	return nil, NewNoCredentialsError()
}
