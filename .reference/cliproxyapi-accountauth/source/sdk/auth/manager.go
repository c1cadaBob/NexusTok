// 包 auth - manager.go
// 该文件定义了认证管理器，用于聚合多个认证器并协调认证记录的持久化。
// 管理器通过提供商标识查找对应的认证器，执行登录流程后将结果保存到令牌存储。
package auth

import (
	"context"
	"fmt"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

// Manager 聚合多个认证器并通过令牌存储协调认证记录的持久化。
type Manager struct {
	authenticators map[string]Authenticator // 提供商标识到认证器的映射
	store          coreauth.Store           // 令牌持久化存储
}

// NewManager 使用提供的令牌存储和认证器列表构造管理器。
// 如果 store 为 nil，调用方后续必须通过 SetStore 设置。
//
// 参数:
//   - store: 令牌持久化存储（可为 nil）
//   - authenticators: 初始认证器列表
//
// 返回:
//   - *Manager: 认证管理器实例
func NewManager(store coreauth.Store, authenticators ...Authenticator) *Manager {
	mgr := &Manager{
		authenticators: make(map[string]Authenticator),
		store:          store,
	}
	for i := range authenticators {
		mgr.Register(authenticators[i])
	}
	return mgr
}

// Register 添加或替换按提供商标识键控的认证器。
//
// 参数:
//   - a: 要注册的认证器实例
func (m *Manager) Register(a Authenticator) {
	if a == nil {
		return
	}
	if m.authenticators == nil {
		m.authenticators = make(map[string]Authenticator)
	}
	m.authenticators[a.Provider()] = a
}

// SetStore 更新用于持久化的令牌存储。
//
// 参数:
//   - store: 新的令牌存储实例
func (m *Manager) SetStore(store coreauth.Store) {
	m.store = store
}

// Login 执行指定提供商的登录流程并持久化认证记录。
// 如果令牌存储实现了 SetBaseDir 方法，会自动设置认证目录。
//
// 参数:
//   - ctx: 请求上下文
//   - provider: 提供商标识名称
//   - cfg: 应用配置
//   - opts: 登录选项
//
// 返回:
//   - *coreauth.Auth: 认证结果
//   - string: 保存的文件路径（无存储时为空）
//   - error: 登录或持久化失败时返回错误信息
func (m *Manager) Login(ctx context.Context, provider string, cfg *config.Config, opts *LoginOptions) (*coreauth.Auth, string, error) {
	auth, ok := m.authenticators[provider]
	if !ok {
		return nil, "", fmt.Errorf("cliproxy auth: authenticator %s not registered", provider)
	}

	record, err := auth.Login(ctx, cfg, opts)
	if err != nil {
		return nil, "", err
	}
	if record == nil {
		return nil, "", fmt.Errorf("cliproxy auth: authenticator %s returned nil record", provider)
	}

	if m.store == nil {
		return record, "", nil
	}

	if cfg != nil {
		if dirSetter, ok := m.store.(interface{ SetBaseDir(string) }); ok {
			dirSetter.SetBaseDir(cfg.AuthDir)
		}
	}

	savedPath, err := m.store.Save(ctx, record)
	if err != nil {
		return record, "", err
	}
	return record, savedPath, nil
}
