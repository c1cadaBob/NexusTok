// 包 auth - refresh_registry.go
// 该文件在 init 阶段注册所有提供商的令牌刷新前导时间（RefreshLead）。
// 通过工厂函数延迟创建认证器，避免在注册阶段产生不必要的开销。
package auth

import (
	"time"

	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

// init 注册所有支持的提供商的令牌刷新前导时间。
func init() {
	registerRefreshLead("codex", func() Authenticator { return NewCodexAuthenticator() })
	registerRefreshLead("claude", func() Authenticator { return NewClaudeAuthenticator() })
	registerRefreshLead("gemini", func() Authenticator { return NewGeminiAuthenticator() })
	registerRefreshLead("gemini-cli", func() Authenticator { return NewGeminiAuthenticator() })
	registerRefreshLead("antigravity", func() Authenticator { return NewAntigravityAuthenticator() })
	registerRefreshLead("kimi", func() Authenticator { return NewKimiAuthenticator() })
	registerRefreshLead("xai", func() Authenticator { return NewXAIAuthenticator() })
}

// registerRefreshLead 将提供商的刷新前导时间注册到全局注册表。
// 使用工厂函数延迟创建认证器，仅在需要时才实例化。
//
// 参数:
//   - provider: 提供商标识名称
//   - factory: 认证器工厂函数
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
