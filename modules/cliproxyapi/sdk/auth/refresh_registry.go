// auth - refresh_registry.go
// 本文件在包初始化时将所有内置认证器的令牌刷新前置时间注册到核心认证模块。
// 通过 init 函数自动完成，无需手动调用。
// 注册的提供商包括：codex、claude、gemini、gemini-cli、antigravity、kimi、xai。
package auth

import (
	"time"

	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

// init 在包加载时自动执行，将所有内置认证器的刷新前置时间注册到核心认证模块。
// 每个提供商通过工厂函数延迟创建认证器实例，以获取其 RefreshLead 值。
func init() {
	registerRefreshLead("codex", func() Authenticator { return NewCodexAuthenticator() })
	registerRefreshLead("claude", func() Authenticator { return NewClaudeAuthenticator() })
	registerRefreshLead("gemini", func() Authenticator { return NewGeminiAuthenticator() })
	registerRefreshLead("gemini-cli", func() Authenticator { return NewGeminiAuthenticator() })
	registerRefreshLead("antigravity", func() Authenticator { return NewAntigravityAuthenticator() })
	registerRefreshLead("kimi", func() Authenticator { return NewKimiAuthenticator() })
	registerRefreshLead("xai", func() Authenticator { return NewXAIAuthenticator() })
}

// registerRefreshLead 将指定提供商的刷新前置时间注册到核心认证模块。
// 通过工厂函数延迟创建认证器，避免在 init 阶段执行过重的初始化操作。
// 参数说明：
//   - provider: 提供商标识字符串
//   - factory: 认证器工厂函数，返回 Authenticator 实例
func registerRefreshLead(provider string, factory func() Authenticator) {
	cliproxyauth.RegisterRefreshLeadProvider(provider, func() *time.Duration {
		if factory == nil {
			return nil
		}
		auth := factory()
		if auth == nil {
			return nil
		}
		return auth.RefreshLead()
	})
}
