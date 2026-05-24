// Package cmd - auth_manager.go
// 提供认证管理器的创建功能，初始化所有支持的 AI 服务商认证器。
package cmd

import (
	sdkAuth "github.com/router-for-me/CLIProxyAPI/v7/sdk/auth"
)

// newAuthManager 创建一个新的认证管理器实例，包含所有支持的认证器和基于文件的令牌存储。
// 初始化 Gemini、Codex、Claude、Antigravity、Kimi 和 xAI 提供商的认证器。
//
// 返回值:
//   - *sdkAuth.Manager: 配置好的认证管理器实例
//
// newAuthManager creates a new authentication manager instance with all supported authenticators.
func newAuthManager() *sdkAuth.Manager {
	store := sdkAuth.GetTokenStore()
	manager := sdkAuth.NewManager(store,
		sdkAuth.NewGeminiAuthenticator(),
		sdkAuth.NewCodexAuthenticator(),
		sdkAuth.NewClaudeAuthenticator(),
		sdkAuth.NewAntigravityAuthenticator(),
		sdkAuth.NewKimiAuthenticator(),
		sdkAuth.NewXAIAuthenticator(),
	)
	return manager
}
