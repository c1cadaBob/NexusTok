// 包 auth - gemini.go
// 该文件实现了 Google Gemini CLI 账户的登录流程。
// 通过 Gemini 认证服务获取认证客户端，并将令牌和元数据持久化到文件。
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

// GeminiAuthenticator 实现了 Google Gemini CLI 账户的登录认证器。
type GeminiAuthenticator struct{}

// NewGeminiAuthenticator 构造一个新的 Gemini 认证器实例。
//
// 返回:
//   - *GeminiAuthenticator: Gemini 认证器实例
func NewGeminiAuthenticator() *GeminiAuthenticator {
	return &GeminiAuthenticator{}
}

// Provider 返回 Gemini 提供商的标识名称。
func (a *GeminiAuthenticator) Provider() string {
	return "gemini"
}

// RefreshLead 返回 nil，表示 Gemini 认证不支持自动令牌刷新。
//
// 返回:
//   - *time.Duration: nil，不支持自动刷新
func (a *GeminiAuthenticator) RefreshLead() *time.Duration {
	return nil
}

// Login 执行 Gemini CLI 账户的登录流程。
// 通过 Gemini 认证服务获取认证客户端，并将令牌和元数据持久化到文件。
//
// 参数:
//   - ctx: 请求上下文
//   - cfg: 应用配置
//   - opts: 登录选项
//
// 返回:
//   - *coreauth.Auth: 认证结果，包含令牌存储和元数据
//   - error: 登录失败时返回错误信息
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

	var ts gemini.GeminiTokenStorage
	if opts.ProjectID != "" {
		ts.ProjectID = opts.ProjectID
	}

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

	fileName := fmt.Sprintf("%s-%s.json", ts.Email, ts.ProjectID)
	metadata := map[string]any{
		"email":      ts.Email,
		"project_id": ts.ProjectID,
	}

	fmt.Println("Gemini authentication successful")

	return &coreauth.Auth{
		ID:       fileName,
		Provider: a.Provider(),
		FileName: fileName,
		Storage:  &ts,
		Metadata: metadata,
	}, nil
}
