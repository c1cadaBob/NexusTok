// Package oauth - github.go
// 该文件实现了 GitHub OAuth 认证提供商
//
// 功能说明：
// - 支持 GitHub OAuth 2.0 授权流程
// - 获取 GitHub 用户信息（ID、用户名、邮箱等）
// - 实现 Provider 接口的所有方法
package oauth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/i18n"
	"github.com/c1cada/NexusTok/logger"
	"github.com/c1cada/NexusTok/model"
	"github.com/gin-gonic/gin"
)

// init 在包初始化时自动注册 GitHub 提供商到全局注册表
// 通过 Go 的 init 机制实现自动注册，无需手动调用
func init() {
	Register("github", &GitHubProvider{})
}

// GitHubProvider 实现 GitHub OAuth 2.0 认证
// 使用 GitHub 的数字 ID（永久不变）作为用户标识
type GitHubProvider struct{}

// gitHubOAuthResponse 表示 GitHub OAuth Token 端点的响应
type gitHubOAuthResponse struct {
	AccessToken string `json:"access_token"` // 访问令牌
	Scope       string `json:"scope"`        // 授权范围
	TokenType   string `json:"token_type"`   // 令牌类型（通常为 "bearer"）
}

// gitHubUser 表示 GitHub 用户 API（/user）的响应
type gitHubUser struct {
	Id    int64  `json:"id"`    // GitHub 数字 ID（永久不变，即使用户名修改也不受影响）
	Login string `json:"login"` // GitHub 用户名（可由用户修改）
	Name  string `json:"name"`  // 显示名称（用户全名）
	Email string `json:"email"` // 邮箱地址（可能未公开）
}

// GetName 返回提供商显示名称
func (p *GitHubProvider) GetName() string {
	return "GitHub"
}

// IsEnabled 检查 GitHub OAuth 是否已启用（通过环境变量配置）
func (p *GitHubProvider) IsEnabled() bool {
	return common.GitHubOAuthEnabled
}

// ExchangeToken 使用授权码向 GitHub Token 端点交换访问令牌
// GitHub 要求使用 JSON 格式发送请求（而非标准的 form-urlencoded）
// 端点：https://github.com/login/oauth/access_token
func (p *GitHubProvider) ExchangeToken(ctx context.Context, code string, c *gin.Context) (*OAuthToken, error) {
	if code == "" {
		return nil, NewOAuthError(i18n.MsgOAuthInvalidCode, nil)
	}

	logger.LogDebug(ctx, "[OAuth-GitHub] ExchangeToken: code=%s...", code[:min(len(code), 10)])

	values := map[string]string{
		"client_id":     common.GitHubClientId,
		"client_secret": common.GitHubClientSecret,
		"code":          code,
	}
	jsonData, err := json.Marshal(values)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://github.com/login/oauth/access_token", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := http.Client{
		Timeout: 20 * time.Second,
	}
	res, err := client.Do(req)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("[OAuth-GitHub] ExchangeToken error: %s", err.Error()))
		return nil, NewOAuthErrorWithRaw(i18n.MsgOAuthConnectFailed, map[string]any{"Provider": "GitHub"}, err.Error())
	}
	defer res.Body.Close()

	logger.LogDebug(ctx, "[OAuth-GitHub] ExchangeToken response status: %d", res.StatusCode)

	var oAuthResponse gitHubOAuthResponse
	err = json.NewDecoder(res.Body).Decode(&oAuthResponse)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("[OAuth-GitHub] ExchangeToken decode error: %s", err.Error()))
		return nil, err
	}

	if oAuthResponse.AccessToken == "" {
		logger.LogError(ctx, "[OAuth-GitHub] ExchangeToken failed: empty access token")
		return nil, NewOAuthError(i18n.MsgOAuthTokenFailed, map[string]any{"Provider": "GitHub"})
	}

	logger.LogDebug(ctx, "[OAuth-GitHub] ExchangeToken success: scope=%s", oAuthResponse.Scope)

	return &OAuthToken{
		AccessToken: oAuthResponse.AccessToken,
		TokenType:   oAuthResponse.TokenType,
		Scope:       oAuthResponse.Scope,
	}, nil
}

// GetUserInfo 使用访问令牌从 GitHub API 获取用户信息
// 端点：https://api.github.com/user
// 使用数字 ID 作为主标识（不使用 login，因为 login 可以修改）
// 同时在 Extra 中保存 login 用于旧账号迁移
func (p *GitHubProvider) GetUserInfo(ctx context.Context, token *OAuthToken) (*OAuthUser, error) {
	logger.LogDebug(ctx, "[OAuth-GitHub] GetUserInfo: fetching user info")

	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/user", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token.AccessToken))

	client := http.Client{
		Timeout: 20 * time.Second,
	}
	res, err := client.Do(req)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("[OAuth-GitHub] GetUserInfo error: %s", err.Error()))
		return nil, NewOAuthErrorWithRaw(i18n.MsgOAuthConnectFailed, map[string]any{"Provider": "GitHub"}, err.Error())
	}
	defer res.Body.Close()

	logger.LogDebug(ctx, "[OAuth-GitHub] GetUserInfo response status: %d", res.StatusCode)

	// Check for non-200 status codes before attempting to decode
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		bodyStr := string(body)
		if len(bodyStr) > 500 {
			bodyStr = bodyStr[:500] + "..."
		}
		logger.LogError(ctx, fmt.Sprintf("[OAuth-GitHub] GetUserInfo failed: status=%d, body=%s", res.StatusCode, bodyStr))
		return nil, NewOAuthErrorWithRaw(i18n.MsgOAuthGetUserErr, map[string]any{"Provider": "GitHub"}, fmt.Sprintf("status %d", res.StatusCode))
	}

	var githubUser gitHubUser
	err = json.NewDecoder(res.Body).Decode(&githubUser)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("[OAuth-GitHub] GetUserInfo decode error: %s", err.Error()))
		return nil, err
	}

	if githubUser.Id == 0 || githubUser.Login == "" {
		logger.LogError(ctx, "[OAuth-GitHub] GetUserInfo failed: empty id or login field")
		return nil, NewOAuthError(i18n.MsgOAuthUserInfoEmpty, map[string]any{"Provider": "GitHub"})
	}

	logger.LogDebug(ctx, "[OAuth-GitHub] GetUserInfo success: id=%d, login=%s, name=%s, email=%s",
		githubUser.Id, githubUser.Login, githubUser.Name, githubUser.Email)

	return &OAuthUser{
		ProviderUserID: strconv.FormatInt(githubUser.Id, 10), // Use numeric ID as primary identifier
		Username:       githubUser.Login,
		DisplayName:    githubUser.Name,
		Email:          githubUser.Email,
		Extra: map[string]any{
			"legacy_id": githubUser.Login, // Store login for migration from old accounts
		},
	}, nil
}

// IsUserIDTaken 检查 GitHub 数字 ID 是否已被其他账号关联
func (p *GitHubProvider) IsUserIDTaken(providerUserID string) bool {
	return model.IsGitHubIdAlreadyTaken(providerUserID)
}

// FillUserByProviderID 通过 GitHub ID 查找并填充用户信息
// 设置 User.GitHubId 后调用数据库查询
func (p *GitHubProvider) FillUserByProviderID(user *model.User, providerUserID string) error {
	user.GitHubId = providerUserID
	return user.FillUserByGitHubId()
}

// SetProviderUserID 将 GitHub ID 设置到用户模型
// 用于新用户注册时关联 GitHub 账号
func (p *GitHubProvider) SetProviderUserID(user *model.User, providerUserID string) {
	user.GitHubId = providerUserID
}

// GetProviderPrefix 返回 GitHub 用户名前缀 "github_"
// 用于 OAuth 登录自动生成用户名（如 "github_johndoe"）
func (p *GitHubProvider) GetProviderPrefix() string {
	return "github_"
}
