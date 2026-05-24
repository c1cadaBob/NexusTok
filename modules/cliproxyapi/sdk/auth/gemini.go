// auth - gemini.go
// 本文件实现了 Google Gemini CLI 账号的 OAuth 登录认证流程。
// Gemini 认证器通过 Web 登录流程获取 Google OAuth 令牌，
// 支持指定 GCP 项目 ID，并自动构建包含邮箱和项目 ID 的认证记录。
package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/auth/gemini"
	// legacy client removed
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

// GeminiAuthenticator 实现了 Google Gemini CLI 账号的登录流程。
// 该认证器不支持令牌刷新（RefreshLead 返回 nil），
// 因为 Gemini 令牌通过 Google OAuth 标准流程管理。
type GeminiAuthenticator struct{}

// NewGeminiAuthenticator 创建一个新的 Gemini 认证器实例。
func NewGeminiAuthenticator() *GeminiAuthenticator {
	return &GeminiAuthenticator{}
}

// Provider 返回该认证器对应的提供商标识 "gemini"。
func (a *GeminiAuthenticator) Provider() string {
	return "gemini"
}

// RefreshLead 返回令牌刷新前置时间。
// Gemini 认证器不支持主动刷新，返回 nil。
func (a *GeminiAuthenticator) RefreshLead() *time.Duration {
	return nil
}

// Login 执行 Gemini 账号的 Web 登录流程。
// 流程概述：
//  1. 初始化 Gemini 认证服务
//  2. 通过 Web 登录流程获取 Google OAuth 令牌
//  3. 构建包含邮箱和项目 ID 的认证记录
//
// 参数说明：
//   - ctx: 上下文，用于控制请求超时和取消
//   - cfg: 全局配置，不能为 nil
//   - opts: 登录选项，可为 nil 使用默认值；opts.ProjectID 可指定 GCP 项目 ID
func (a *GeminiAuthenticator) Login(ctx context.Context, cfg *config.Config, opts *LoginOptions) (*coreauth.Auth, error) {
	if cfg == nil {
		return nil, fmt.Errorf("cliproxy auth: configuration is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if opts == nil {
		opts = &LoginOptions{}
	}

	// 初始化令牌存储，设置可选的项目 ID
	var ts gemini.GeminiTokenStorage
	if opts.ProjectID != "" {
		ts.ProjectID = opts.ProjectID
	}

	// 创建 Gemini 认证服务并执行 Web 登录
	geminiAuth := gemini.NewGeminiAuth()
	_, err := geminiAuth.GetAuthenticatedClient(ctx, &ts, cfg, &gemini.WebLoginOptions{
		NoBrowser:    opts.NoBrowser,
		CallbackPort: opts.CallbackPort,
		Prompt:       opts.Prompt,
	})
	if err != nil {
		return nil, fmt.Errorf("gemini authentication failed: %w", err)
	}

	// Skip onboarding here; rely on upstream configuration

	// 构建认证文件名（格式：{email}-{project_id}.json）
	fileName := fmt.Sprintf("%s-%s.json", ts.Email, ts.ProjectID)
	metadata := map[string]any{
		"email":      ts.Email,
		"project_id": ts.ProjectID,
	}

	fmt.Println("Gemini authentication successful")

	// 返回认证记录
	return &coreauth.Auth{
		ID:       fileName,
		Provider: a.Provider(),
		FileName: fileName,
		Storage:  &ts,
		Metadata: metadata,
	}, nil
}
