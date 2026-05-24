// auth - manager.go
// 本文件实现了认证管理器 Manager，负责协调多个认证器的注册和登录流程。
// Manager 聚合了各提供商的 Authenticator 实现，并通过 TokenStore 持久化认证记录。
package auth

import (
	"context"
	"fmt"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

// Manager 是认证管理器，聚合了多个认证器并协调持久化存储。
// 它维护一个以提供商标识为键的认证器映射，以及一个全局的令牌存储后端。
// 通过 Login 方法可执行指定提供商的登录流程，并自动保存认证结果。
type Manager struct {
	// authenticators 以提供商标识字符串（如 "claude"、"codex"）为键，
	// 存储已注册的认证器实例。
	authenticators map[string]Authenticator

	// store 是用于持久化认证记录的令牌存储后端。
	// 可为 nil，此时 Login 方法仅返回认证记录但不持久化。
	store coreauth.Store
}

// NewManager 创建一个新的认证管理器实例。
// 参数说明：
//   - store: 令牌存储后端，可为 nil（后续通过 SetStore 设置）
//   - authenticators: 可变参数，初始注册的认证器列表
//
// 返回的管理器已包含所有传入的认证器，按其 Provider() 值索引。
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

// Register 向管理器注册或替换一个认证器。
// 认证器以其 Provider() 返回值为键进行存储。
// 如果传入的认证器为 nil，则忽略此次调用。
func (m *Manager) Register(a Authenticator) {
	if a == nil {
		return
	}
	if m.authenticators == nil {
		m.authenticators = make(map[string]Authenticator)
	}
	m.authenticators[a.Provider()] = a
}

// SetStore 更新管理器使用的令牌存储后端。
// 可在运行时替换存储实现，例如从文件存储切换到数据库存储。
func (m *Manager) SetStore(store coreauth.Store) {
	m.store = store
}

// Login 执行指定提供商的登录流程，并将认证记录持久化到存储后端。
// 执行步骤：
//  1. 根据 provider 查找已注册的认证器
//  2. 调用认证器的 Login 方法执行 OAuth 流程
//  3. 如果存储后端已配置且配置信息可用，设置认证目录
//  4. 通过存储后端保存认证记录
//
// 参数说明：
//   - ctx: 上下文，用于控制请求超时和取消
//   - provider: 提供商标识字符串（如 "claude"、"codex"）
//   - cfg: 全局配置，包含认证目录等设置
//   - opts: 登录选项，可为 nil 使用默认值
//
// 返回值：
//   - *coreauth.Auth: 认证记录（包含令牌存储和元数据）
//   - string: 认证记录的保存路径（如果已持久化）
//   - error: 登录或持久化过程中的错误
func (m *Manager) Login(ctx context.Context, provider string, cfg *config.Config, opts *LoginOptions) (*coreauth.Auth, string, error) {
	// 查找已注册的认证器
	auth, ok := m.authenticators[provider]
	if !ok {
		return nil, "", fmt.Errorf("cliproxy auth: authenticator %s not registered", provider)
	}

	// 执行提供商的登录流程
	record, err := auth.Login(ctx, cfg, opts)
	if err != nil {
		return nil, "", err
	}
	if record == nil {
		return nil, "", fmt.Errorf("cliproxy auth: authenticator %s returned nil record", provider)
	}

	// 如果未配置存储后端，直接返回认证记录（不持久化）
	if m.store == nil {
		return record, "", nil
	}

	// 如果配置信息可用，设置存储后端的认证目录
	if cfg != nil {
		if dirSetter, ok := m.store.(interface{ SetBaseDir(string) }); ok {
			dirSetter.SetBaseDir(cfg.AuthDir)
		}
	}

	// 持久化认证记录到存储后端
	savedPath, err := m.store.Save(ctx, record)
	if err != nil {
		return record, "", err
	}
	return record, savedPath, nil
}
