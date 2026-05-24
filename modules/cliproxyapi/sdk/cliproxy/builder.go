// cliproxy - builder.go
// 该文件定义了 Service 的构建器（Builder）模式实现。
// 通过流式接口（Fluent Interface）配置 Service 的所有组件，
// 包括配置、客户端加载器、文件监视器、认证管理器和生命周期钩子。

// Package cliproxy provides the core service implementation for the CLI Proxy API.
// It includes service lifecycle management, authentication handling, file watching,
// and integration with various AI service providers through a unified interface.
package cliproxy

import (
	"fmt"
	"strings"
	"time"

	configaccess "github.com/router-for-me/CLIProxyAPI/v7/internal/access/config_access"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/api"
	sdkaccess "github.com/router-for-me/CLIProxyAPI/v7/sdk/access"
	sdkAuth "github.com/router-for-me/CLIProxyAPI/v7/sdk/auth"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
)

// Builder 使用可自定义的组件构建 Service 实例。
// 提供流式接口用于配置服务的各个方面，
// 包括认证、文件监视、HTTP 服务器选项和生命周期钩子。
type Builder struct {
	// cfg holds the application configuration.
	cfg *config.Config

	// configPath is the path to the configuration file.
	configPath string

	// tokenProvider handles loading token-based clients.
	tokenProvider TokenClientProvider

	// apiKeyProvider handles loading API key-based clients.
	apiKeyProvider APIKeyClientProvider

	// watcherFactory creates file watcher instances.
	watcherFactory WatcherFactory

	// hooks provides lifecycle callbacks.
	hooks Hooks

	// authManager handles legacy authentication operations.
	authManager *sdkAuth.Manager

	// accessManager handles request authentication providers.
	accessManager *sdkaccess.Manager

	// coreManager handles core authentication and execution.
	coreManager *coreauth.Manager

	// serverOptions contains additional server configuration options.
	serverOptions []api.ServerOption
}

// Hooks 允许调用方接入服务生命周期阶段。
// 这些回调提供了在服务启动和关闭期间执行自定义初始化和清理操作的机会。
type Hooks struct {
	// OnBeforeStart 在服务启动前调用，允许修改配置或执行额外的初始化设置。
	OnBeforeStart func(*config.Config)

	// OnAfterStart 在服务成功启动后调用，提供对 Service 实例的访问以执行额外操作。
	OnAfterStart func(*Service)
}

// NewBuilder 创建一个依赖项未设置的 Builder。
// 使用流式接口方法配置服务，然后调用 Build()。
func NewBuilder() *Builder {
	return &Builder{}
}

// WithConfig 设置服务使用的配置实例。
func (b *Builder) WithConfig(cfg *config.Config) *Builder {
	b.cfg = cfg
	return b
}

// WithConfigPath 设置用于重载监视的绝对配置文件路径。
func (b *Builder) WithConfigPath(path string) *Builder {
	b.configPath = path
	return b
}

// WithTokenClientProvider 覆盖负责基于令牌的客户端加载的提供器。
func (b *Builder) WithTokenClientProvider(provider TokenClientProvider) *Builder {
	b.tokenProvider = provider
	return b
}

// WithAPIKeyClientProvider 覆盖负责基于 API Key 的客户端加载的提供器。
func (b *Builder) WithAPIKeyClientProvider(provider APIKeyClientProvider) *Builder {
	b.apiKeyProvider = provider
	return b
}

// WithWatcherFactory 允许自定义处理重载的监视器工厂。
func (b *Builder) WithWatcherFactory(factory WatcherFactory) *Builder {
	b.watcherFactory = factory
	return b
}

// WithHooks 注册服务启动时执行的生命周期钩子。
func (b *Builder) WithHooks(h Hooks) *Builder {
	b.hooks = h
	return b
}

// WithAuthManager 覆盖用于令牌生命周期操作的认证管理器。
func (b *Builder) WithAuthManager(mgr *sdkAuth.Manager) *Builder {
	b.authManager = mgr
	return b
}

// WithRequestAccessManager 覆盖请求认证管理器。
func (b *Builder) WithRequestAccessManager(mgr *sdkaccess.Manager) *Builder {
	b.accessManager = mgr
	return b
}

// WithCoreAuthManager 覆盖负责请求执行的运行时认证管理器。
func (b *Builder) WithCoreAuthManager(mgr *coreauth.Manager) *Builder {
	b.coreManager = mgr
	return b
}

// WithServerOptions 追加构建时使用的服务器配置选项。
func (b *Builder) WithServerOptions(opts ...api.ServerOption) *Builder {
	b.serverOptions = append(b.serverOptions, opts...)
	return b
}

// WithLocalManagementPassword 配置仅从本地主机管理请求接受的密码。
func (b *Builder) WithLocalManagementPassword(password string) *Builder {
	if password == "" {
		return b
	}
	b.serverOptions = append(b.serverOptions, api.WithLocalManagementPassword(password))
	return b
}

// WithPostAuthHook 注册在 Auth 记录创建后、持久化前调用的钩子。
func (b *Builder) WithPostAuthHook(hook coreauth.PostAuthHook) *Builder {
	if hook == nil {
		return b
	}
	b.serverOptions = append(b.serverOptions, api.WithPostAuthHook(hook))
	return b
}

// Build 验证输入、应用默认值并返回可运行的服务。
func (b *Builder) Build() (*Service, error) {
	if b.cfg == nil {
		return nil, fmt.Errorf("cliproxy: configuration is required")
	}
	if b.configPath == "" {
		return nil, fmt.Errorf("cliproxy: configuration path is required")
	}

	tokenProvider := b.tokenProvider
	if tokenProvider == nil {
		tokenProvider = NewFileTokenClientProvider()
	}

	apiKeyProvider := b.apiKeyProvider
	if apiKeyProvider == nil {
		apiKeyProvider = NewAPIKeyClientProvider()
	}

	watcherFactory := b.watcherFactory
	if watcherFactory == nil {
		watcherFactory = defaultWatcherFactory
	}

	authManager := b.authManager
	if authManager == nil {
		authManager = newDefaultAuthManager()
	}

	accessManager := b.accessManager
	if accessManager == nil {
		accessManager = sdkaccess.NewManager()
	}

	configaccess.Register(&b.cfg.SDKConfig)
	accessManager.SetProviders(sdkaccess.RegisteredProviders())

	coreManager := b.coreManager
	if coreManager == nil {
		tokenStore := sdkAuth.GetTokenStore()
		if dirSetter, ok := tokenStore.(interface{ SetBaseDir(string) }); ok && b.cfg != nil {
			dirSetter.SetBaseDir(b.cfg.AuthDir)
		}

		strategy := ""
		sessionAffinity := false
		sessionAffinityTTL := time.Hour
		if b.cfg != nil {
			strategy = strings.ToLower(strings.TrimSpace(b.cfg.Routing.Strategy))
			// Support both legacy ClaudeCodeSessionAffinity and new universal SessionAffinity
			sessionAffinity = b.cfg.Routing.SessionAffinity
			if ttlStr := strings.TrimSpace(b.cfg.Routing.SessionAffinityTTL); ttlStr != "" {
				if parsed, err := time.ParseDuration(ttlStr); err == nil && parsed > 0 {
					sessionAffinityTTL = parsed
				}
			}
		}
		var selector coreauth.Selector
		switch strategy {
		case "fill-first", "fillfirst", "ff":
			selector = &coreauth.FillFirstSelector{}
		default:
			selector = &coreauth.RoundRobinSelector{}
		}

		// Wrap with session affinity if enabled (failover is always on)
		if sessionAffinity {
			selector = coreauth.NewSessionAffinitySelectorWithConfig(coreauth.SessionAffinityConfig{
				Fallback: selector,
				TTL:      sessionAffinityTTL,
			})
		}

		coreManager = coreauth.NewManager(tokenStore, selector, nil)
	}
	// Attach a default RoundTripper provider so providers can opt-in per-auth transports.
	coreManager.SetRoundTripperProvider(newDefaultRoundTripperProvider())
	coreManager.SetConfig(b.cfg)
	coreManager.SetOAuthModelAlias(b.cfg.OAuthModelAlias)

	service := &Service{
		cfg:            b.cfg,
		configPath:     b.configPath,
		tokenProvider:  tokenProvider,
		apiKeyProvider: apiKeyProvider,
		watcherFactory: watcherFactory,
		hooks:          b.hooks,
		authManager:    authManager,
		accessManager:  accessManager,
		coreManager:    coreManager,
		serverOptions:  append([]api.ServerOption(nil), b.serverOptions...),
	}
	return service, nil
}
