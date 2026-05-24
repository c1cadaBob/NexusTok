// Package cmd - openai_login.go
// 提供 OpenAI Codex 的 OAuth 认证流程和登录选项定义。
package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/auth/codex"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	sdkAuth "github.com/router-for-me/CLIProxyAPI/v7/sdk/auth"
	log "github.com/sirupsen/logrus"
)

// LoginOptions 包含登录过程的选项。
// 为认证流程提供配置，包括浏览器行为和交互式提示功能。
//
// LoginOptions contains options for the login processes.
type LoginOptions struct {
	// NoBrowser 表示是否跳过自动打开浏览器。
	NoBrowser bool

	// CallbackPort 覆盖本地 OAuth 回调端口（当设置且 >0 时生效）。
	CallbackPort int

	// Prompt 允许调用者在需要时提供交互式输入。
	Prompt func(prompt string) (string, error)
}

// DoCodexLogin 通过共享认证管理器触发 Codex OAuth 流程。
// 启动 OpenAI Codex 服务的 OAuth 认证过程，并将认证令牌保存到配置的认证目录。
//
// 参数:
//   - cfg: 应用程序配置
//   - options: 登录选项，包括浏览器行为和提示
//
// DoCodexLogin triggers the Codex OAuth flow through the shared authentication manager.
func DoCodexLogin(cfg *config.Config, options *LoginOptions) {
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

	_, savedPath, err := manager.Login(context.Background(), "codex", cfg, authOpts)
	if err != nil {
		if authErr, ok := errors.AsType[*codex.AuthenticationError](err); ok {
			log.Error(codex.GetUserFriendlyMessage(authErr))
			if authErr.Type == codex.ErrPortInUse.Type {
				os.Exit(codex.ErrPortInUse.Code)
			}
			return
		}
		fmt.Printf("Codex authentication failed: %v\n", err)
		return
	}

	if savedPath != "" {
		fmt.Printf("Authentication saved to %s\n", savedPath)
	}
	fmt.Println("Codex authentication successful!")
}
