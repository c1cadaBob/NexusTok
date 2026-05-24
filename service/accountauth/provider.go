// provider.go 定义了账号认证提供者的接口规范和管理器。
// Provider 接口规定了所有认证提供者（如 Codex）必须实现的方法，
// Manager 负责提供者的注册、查找和枚举。
// 通过 RegisterProvider 注册的提供者会被存储在全局默认管理器中。
package accountauth

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/c1cada/NexusTok/model"
)

// Provider 定义了账号认证提供者的接口规范。
// 所有认证提供者（如 Codex/OpenAI）都必须实现此接口。
// 提供者负责完成 OAuth 或 Device Code 认证流程，管理凭据的刷新，
// 以及将凭据转换为渠道可用的密钥格式。
type Provider interface {
	// Name 返回提供者的唯一标识名称（如 "codex"）
	Name() string
	// DisplayName 返回提供者的可读显示名称（如 "Codex"）
	DisplayName() string
	// SupportsOAuth 返回该提供者是否支持 OAuth 授权码流程
	SupportsOAuth() bool
	// SupportsDevice 返回该提供者是否支持 Device Code 流程
	SupportsDevice() bool
	// RefreshLead 返回令牌刷新的提前量。
	// 如果返回 nil，表示该提供者不支持自动刷新；
	// 返回非 nil 值表示在令牌过期前多久开始尝试刷新。
	RefreshLead() *time.Duration
	// StartOAuth 启动 OAuth 授权码流程，生成授权 URL 并保存会话
	StartOAuth(ctx context.Context, group *model.AccountPoolGroup, req LoginStartRequest) (*LoginStartResult, error)
	// CompleteOAuth 完成 OAuth 授权码流程，用授权码交换 token 并构建凭证
	CompleteOAuth(ctx context.Context, group *model.AccountPoolGroup, req LoginCompleteRequest) (*AccountCredential, error)
	// StartDevice 启动 Device Code 流程，获取 user_code 和验证 URL
	StartDevice(ctx context.Context, group *model.AccountPoolGroup, req LoginStartRequest) (*LoginStartResult, error)
	// CompleteDevice 完成 Device Code 流程，轮询服务器获取授权结果
	CompleteDevice(ctx context.Context, group *model.AccountPoolGroup, req LoginCompleteRequest) (*AccountCredential, error)
	// Refresh 使用 refresh_token 刷新 OAuth 令牌，返回更新后的凭证
	Refresh(ctx context.Context, account *model.PoolAccount) (*AccountCredential, error)
	// BuildChannelKey 从账号凭证中提取用于渠道认证的密钥
	BuildChannelKey(account *model.PoolAccount) (string, error)
	// Summarize 将原始凭证 JSON 转换为简短的摘要字符串，用于界面展示
	Summarize(raw string) string
}

// Manager 是认证提供者的管理器，负责提供者的注册和查找。
// 使用读写锁保证并发安全。
type Manager struct {
	mu        sync.RWMutex           // 读写锁，保护 providers map 的并发访问
	providers map[string]Provider     // 已注册的提供者映射表，key 为提供者名称
}

// defaultManager 是全局默认的提供者管理器实例。
// 所有通过 RegisterProvider 注册的提供者都存储在此实例中。
var defaultManager = NewManager()

// NewManager 创建一个新的空提供者管理器实例。
func NewManager() *Manager {
	return &Manager{providers: map[string]Provider{}}
}

// RegisterProvider 将官方账号 provider 注册到默认管理器。
// 这是一个便捷函数，内部委托给 defaultManager.Register。
func RegisterProvider(provider Provider) {
	defaultManager.Register(provider)
}

// DefaultManager 返回全局默认的提供者管理器实例。
func DefaultManager() *Manager {
	return defaultManager
}

// Register 将一个提供者注册到管理器中。
// 提供者名称会被自动标准化为小写并去除首尾空格。
// 如果管理器或提供者为 nil，或名称为空，则静默忽略。
func (m *Manager) Register(provider Provider) {
	if m == nil || provider == nil {
		return
	}
	name := normalizeProvider(provider.Name())
	if name == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.providers == nil {
		m.providers = map[string]Provider{}
	}
	m.providers[name] = provider
}

// Provider 根据名称查找已注册的提供者。
// 返回提供者实例和是否找到的布尔值。
func (m *Manager) Provider(name string) (Provider, bool) {
	if m == nil {
		return nil, false
	}
	name = normalizeProvider(name)
	if name == "" {
		return nil, false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	provider, ok := m.providers[name]
	return provider, ok
}

// MustProvider 根据名称查找已注册的提供者，如果未找到则返回错误。
// 适用于必须使用特定提供者的场景。
func (m *Manager) MustProvider(name string) (Provider, error) {
	provider, ok := m.Provider(name)
	if !ok {
		return nil, fmt.Errorf("账号认证 provider 未注册: %s", name)
	}
	return provider, nil
}

// Providers 返回所有已注册提供者的信息列表。
// 返回的列表按提供者名称字母序排列，每个条目包含名称、显示名称和支持的认证方式。
func (m *Manager) Providers() []ProviderInfo {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	items := make([]ProviderInfo, 0, len(m.providers))
	for _, provider := range m.providers {
		if provider == nil {
			continue
		}
		items = append(items, ProviderInfo{
			Name:            normalizeProvider(provider.Name()),
			DisplayName:     provider.DisplayName(),
			SupportsOAuth:   provider.SupportsOAuth(),
			SupportsDevice:  provider.SupportsDevice(),
			SupportsRefresh: provider.RefreshLead() != nil,
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
	return items
}

// normalizeProvider 将提供者名称标准化为小写并去除首尾空格。
func normalizeProvider(provider string) string {
	return strings.ToLower(strings.TrimSpace(provider))
}

// ResolveProviderName 从账号、分组和回退值中解析提供者名称。
// 优先级：账号凭证中的提供者 > 分组的平台字段 > 回退值。
//
// 参数：
//   - group: 账号池分组信息
//   - account: 账号池账号信息
//   - fallback: 回退的提供者名称
//
// 返回：
//   - string: 解析后的提供者名称
func ResolveProviderName(group *model.AccountPoolGroup, account *model.PoolAccount, fallback string) string {
	if account != nil {
		if provider := account.GetCredentialProvider(); provider != "" {
			return provider
		}
	}
	if group != nil && strings.TrimSpace(group.Platform) != "" {
		return normalizeProvider(group.Platform)
	}
	return normalizeProvider(fallback)
}
