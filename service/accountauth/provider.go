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

type Provider interface {
	Name() string
	DisplayName() string
	SupportsOAuth() bool
	SupportsDevice() bool
	RefreshLead() *time.Duration
	StartOAuth(ctx context.Context, group *model.AccountPoolGroup, req LoginStartRequest) (*LoginStartResult, error)
	CompleteOAuth(ctx context.Context, group *model.AccountPoolGroup, req LoginCompleteRequest) (*AccountCredential, error)
	StartDevice(ctx context.Context, group *model.AccountPoolGroup, req LoginStartRequest) (*LoginStartResult, error)
	CompleteDevice(ctx context.Context, group *model.AccountPoolGroup, req LoginCompleteRequest) (*AccountCredential, error)
	Refresh(ctx context.Context, account *model.PoolAccount) (*AccountCredential, error)
	BuildChannelKey(account *model.PoolAccount) (string, error)
	Summarize(raw string) string
}

type Manager struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

var defaultManager = NewManager()

func NewManager() *Manager {
	return &Manager{providers: map[string]Provider{}}
}

// RegisterProvider 将官方账号 provider 注册到默认管理器。
func RegisterProvider(provider Provider) {
	defaultManager.Register(provider)
}

func DefaultManager() *Manager {
	return defaultManager
}

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

func (m *Manager) MustProvider(name string) (Provider, error) {
	provider, ok := m.Provider(name)
	if !ok {
		return nil, fmt.Errorf("账号认证 provider 未注册: %s", name)
	}
	return provider, nil
}

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

func normalizeProvider(provider string) string {
	return strings.ToLower(strings.TrimSpace(provider))
}

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
