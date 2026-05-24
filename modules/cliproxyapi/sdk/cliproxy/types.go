// cliproxy - types.go
// 该文件定义了 CLI Proxy API SDK 的核心接口和类型。
// 包括 Token 客户端提供者、API Key 客户端提供者、文件监视器工厂等接口，
// 以及 WatcherWrapper 代理结构体，为外部程序嵌入 CLI 代理提供标准化接口。

// Package cliproxy provides the core service implementation for the CLI Proxy API.
// It includes service lifecycle management, authentication handling, file watching,
// and integration with various AI service providers through a unified interface.
package cliproxy

import (
	"context"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/watcher"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
)

// TokenClientProvider 定义了加载基于存储 token 的客户端的接口。
// 提供从各种来源加载认证 token 并创建 AI 服务提供商客户端的能力。
type TokenClientProvider interface {
	// Load 从配置的来源加载基于 token 的客户端。
	//
	// 参数：
	//   - ctx: 加载操作的上下文
	//   - cfg: 应用配置
	//
	// 返回值：
	//   - *TokenClientResult: 包含加载结果的信息
	//   - error: 加载失败时返回错误
	Load(ctx context.Context, cfg *config.Config) (*TokenClientResult, error)
}

// TokenClientResult 表示从持久化 token 生成的客户端结果。
// 包含加载操作的元数据和成功认证的数量。
type TokenClientResult struct {
	// SuccessfulAuthed 成功认证的客户端数量
	SuccessfulAuthed int
}

// APIKeyClientProvider 定义了加载基于 API Key 的客户端的接口。
// 提供从配置中加载 API Key 并创建各种 AI 服务提供商客户端的能力。
type APIKeyClientProvider interface {
	// Load 从配置中加载基于 API Key 的客户端。
	//
	// 参数：
	//   - ctx: 加载操作的上下文
	//   - cfg: 应用配置
	//
	// 返回值：
	//   - *APIKeyClientResult: 包含各提供商加载计数的结果
	//   - error: 加载失败时返回错误
	Load(ctx context.Context, cfg *config.Config) (*APIKeyClientResult, error)
}

// APIKeyClientResult 由 APIKeyClientProvider.Load() 返回，包含各提供商的 API Key 加载计数。
type APIKeyClientResult struct {
	// GeminiKeyCount 已加载的 Gemini API Key 数量
	GeminiKeyCount int

	// VertexCompatKeyCount 已加载的 Vertex 兼容 API Key 数量
	VertexCompatKeyCount int

	// ClaudeKeyCount 已加载的 Claude API Key 数量
	ClaudeKeyCount int

	// CodexKeyCount 已加载的 Codex API Key 数量
	CodexKeyCount int

	// OpenAICompatCount 已加载的 OpenAI 兼容 API Key 数量
	OpenAICompatCount int
}

// WatcherFactory 创建用于监视配置和 token 变化的文件监视器。
// 当检测到变化时，reload 回调会接收到更新后的配置。
//
// 参数：
//   - configPath: 要监视的配置文件路径
//   - authDir: 包含认证 token 的目录路径
//   - reload: 检测到变化时调用的回调函数
//
// 返回值：
//   - *WatcherWrapper: 监视器包装器实例
//   - error: 监视器创建失败时返回错误
type WatcherFactory func(configPath, authDir string, reload func(*config.Config)) (*WatcherWrapper, error)

// WatcherWrapper 暴露了 SDK 所需的监视器方法子集。
// 通过函数字段代理底层监视器的调用，提供灵活的可替换性。
type WatcherWrapper struct {
	start func(ctx context.Context) error
	stop  func() error

	setConfig             func(cfg *config.Config)
	snapshotAuths         func() []*coreauth.Auth
	setUpdateQueue        func(queue chan<- watcher.AuthUpdate)
	dispatchRuntimeUpdate func(update watcher.AuthUpdate) bool
}

// Start 代理底层监视器的 Start 方法，启动文件监视。
func (w *WatcherWrapper) Start(ctx context.Context) error {
	if w == nil || w.start == nil {
		return nil
	}
	return w.start(ctx)
}

// Stop 代理底层监视器的 Stop 方法，停止文件监视。
func (w *WatcherWrapper) Stop() error {
	if w == nil || w.stop == nil {
		return nil
	}
	return w.stop()
}

// SetConfig 更新监视器的配置缓存。
func (w *WatcherWrapper) SetConfig(cfg *config.Config) {
	if w == nil || w.setConfig == nil {
		return
	}
	w.setConfig(cfg)
}

// DispatchRuntimeAuthUpdate 将运行时认证更新（如 websocket 提供商）转发到监视器管理的认证更新队列中。
// 如果队列可用，返回 true 表示更新已成功入队。
func (w *WatcherWrapper) DispatchRuntimeAuthUpdate(update watcher.AuthUpdate) bool {
	if w == nil || w.dispatchRuntimeUpdate == nil {
		return false
	}
	return w.dispatchRuntimeUpdate(update)
}

// SetClients 已移除；watcher 管理自己的缓存
// SetClients and SetAPIKeyClients removed; watcher manages its own caches

// SnapshotClients 已移除；请使用 SnapshotAuths
// SnapshotClients removed; use SnapshotAuths

// SnapshotAuths 返回从旧版客户端派生的当前认证条目列表。
func (w *WatcherWrapper) SnapshotAuths() []*coreauth.Auth {
	if w == nil || w.snapshotAuths == nil {
		return nil
	}
	return w.snapshotAuths()
}

// SetAuthUpdateQueue 注册用于传播认证更新的通道。
func (w *WatcherWrapper) SetAuthUpdateQueue(queue chan<- watcher.AuthUpdate) {
	if w == nil || w.setUpdateQueue == nil {
		return
	}
	w.setUpdateQueue(queue)
}
