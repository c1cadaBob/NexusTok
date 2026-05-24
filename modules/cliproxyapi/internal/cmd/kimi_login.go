// Package cmd - kimi_login.go
// 提供 Kimi（月之暗面 AI）的设备流程 OAuth 认证。
package cmd

import (
	"context"
	"fmt"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	sdkAuth "github.com/router-for-me/CLIProxyAPI/v7/sdk/auth"
	log "github.com/sirupsen/logrus"
)

// DoKimiLogin 触发 Kimi（月之暗面 AI）的设备流程 OAuth 认证并保存令牌。
// 启动设备流程认证，向用户显示验证 URL，并在保存令牌之前等待授权。
//
// 参数:
//   - cfg: 包含代理和认证目录设置的应用程序配置
//   - options: 登录选项，包括浏览器行为设置
//
// DoKimiLogin triggers the OAuth device flow for Kimi (Moonshot AI) and saves tokens.
func DoKimiLogin(cfg *config.Config, options *LoginOptions) {
	if options == nil {
		options = &LoginOptions{}
	}

	manager := newAuthManager()
	authOpts := &sdkAuth.LoginOptions{
		NoBrowser: options.NoBrowser,
		Metadata:  map[string]string{},
		Prompt:    options.Prompt,
	}

	record, savedPath, err := manager.Login(context.Background(), "kimi", cfg, authOpts)
	if err != nil {
		log.Errorf("Kimi authentication failed: %v", err)
		return
	}

	if savedPath != "" {
		fmt.Printf("Authentication saved to %s\n", savedPath)
	}
	if record != nil && record.Label != "" {
		fmt.Printf("Authenticated as %s\n", record.Label)
	}
	fmt.Println("Kimi authentication successful!")
}
