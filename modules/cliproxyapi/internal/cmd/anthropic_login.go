// Package cmd - anthropic_login.go
// 提供 Anthropic Claude 的 OAuth 认证流程。
package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/auth/claude"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	sdkAuth "github.com/router-for-me/CLIProxyAPI/v7/sdk/auth"
	log "github.com/sirupsen/logrus"
)

// DoClaudeLogin 通过共享认证管理器触发 Claude OAuth 流程。
// 启动 Anthropic Claude 服务的 OAuth 认证过程，并将认证令牌保存到配置的认证目录。
//
// 参数:
//   - cfg: 应用程序配置
//   - options: 登录选项，包括浏览器行为和提示
//
// DoClaudeLogin triggers the Claude OAuth flow through the shared authentication manager.
func DoClaudeLogin(cfg *config.Config, options *LoginOptions) {
	if options == nil {
		options = &LoginOptions{}
	}

	promptFn := options.Prompt
	if promptFn == nil {
		promptFn = defaultProjectPrompt()
	}

	manager := newAuthManager()

	authOpts := &sdkAuth.LoginOptions{
		NoBrowser:    options.NoBrowser,
		CallbackPort: options.CallbackPort,
		Metadata:     map[string]string{},
		Prompt:       promptFn,
	}

	_, savedPath, err := manager.Login(context.Background(), "claude", cfg, authOpts)
	if err != nil {
		if authErr, ok := errors.AsType[*claude.AuthenticationError](err); ok {
			log.Error(claude.GetUserFriendlyMessage(authErr))
			if authErr.Type == claude.ErrPortInUse.Type {
				os.Exit(claude.ErrPortInUse.Code)
			}
			return
		}
		fmt.Printf("Claude authentication failed: %v\n", err)
		return
	}

	if savedPath != "" {
		fmt.Printf("Authentication saved to %s\n", savedPath)
	}

	fmt.Println("Claude authentication successful!")
}
