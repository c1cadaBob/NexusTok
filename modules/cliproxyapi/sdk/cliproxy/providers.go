// cliproxy - providers.go
// 该文件提供 AI 服务提供商客户端的加载实现。
// 包括基于文件的 Token 客户端加载器和基于 API Key 的客户端加载器。

package cliproxy

import (
	"context"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/watcher"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
)

// NewFileTokenClientProvider 返回默认的基于文件的 Token 客户端加载器。
// 该实现是无状态的，由无状态执行器处理 token。
func NewFileTokenClientProvider() TokenClientProvider {
	return &fileTokenClientProvider{}
}

// fileTokenClientProvider 是 TokenClientProvider 的无状态实现。
type fileTokenClientProvider struct{}

// Load 加载基于 token 的客户端。当前实现是无状态的，直接返回空结果。
func (p *fileTokenClientProvider) Load(ctx context.Context, cfg *config.Config) (*TokenClientResult, error) {
	// 无状态执行器处理 token
	_ = ctx
	_ = cfg
	return &TokenClientResult{SuccessfulAuthed: 0}, nil
}

// NewAPIKeyClientProvider 返回默认的 API Key 客户端加载器，复用现有的加载逻辑。
func NewAPIKeyClientProvider() APIKeyClientProvider {
	return &apiKeyClientProvider{}
}

// apiKeyClientProvider 是 APIKeyClientProvider 的默认实现。
type apiKeyClientProvider struct{}

// Load 从配置中加载 API Key 客户端，返回各提供商的加载计数。
func (p *apiKeyClientProvider) Load(ctx context.Context, cfg *config.Config) (*APIKeyClientResult, error) {
	geminiCount, vertexCompatCount, claudeCount, codexCount, openAICompat := watcher.BuildAPIKeyClients(cfg)
	if ctx != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}
	return &APIKeyClientResult{
		GeminiKeyCount:       geminiCount,
		VertexCompatKeyCount: vertexCompatCount,
		ClaudeKeyCount:       claudeCount,
		CodexKeyCount:        codexCount,
		OpenAICompatCount:    openAICompat,
	}, nil
}
