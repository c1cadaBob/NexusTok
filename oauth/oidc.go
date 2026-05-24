// Package oauth - oidc.go
// 该文件实现了 OIDC（OpenID Connect）认证提供商
//
// 功能说明：
// - 支持标准 OIDC 授权流程
// - 支持 OIDC Discovery 自动发现端点
// - 获取 OIDC 用户信息（sub、name、email 等）
// - 实现 Provider 接口的所有方法
package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/i18n"
	"github.com/c1cada/NexusTok/logger"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/setting/system_setting"
	"github.com/gin-gonic/gin"
)

// init 在包初始化时自动注册 OIDC 提供商
func init() {
	Register("oidc", &OIDCProvider{})
}

// OIDCProvider 实现标准 OpenID Connect 认证
// OIDC 是基于 OAuth 2.0 的身份认证层，提供标准化的用户信息端点
type OIDCProvider struct{}

// oidcOAuthResponse 表示 OIDC Token 端点的响应
type oidcOAuthResponse struct {
	AccessToken  string `json:"access_token"`  // 访问令牌
	IDToken      string `json:"id_token"`      // OIDC ID 令牌（JWT，包含用户身份声明）
	RefreshToken string `json:"refresh_token"` // 刷新令牌
	TokenType    string `json:"token_type"`    // 令牌类型
	ExpiresIn    int    `json:"expires_in"`    // 过期时间（秒）
	Scope        string `json:"scope"`         // 授权范围
}

// oidcUser 表示 OIDC UserInfo 端点的响应
// 字段名遵循 OpenID Connect Core 1.0 规范
type oidcUser struct {
	OpenID            string `json:"sub"`                // Subject：用户唯一标识（OIDC 标准字段）
	Email             string `json:"email"`              // 邮箱地址
	Name              string `json:"name"`               // 全名
	PreferredUsername string `json:"preferred_username"` // 首选用户名
	Picture           string `json:"picture"`            // 头像 URL
}

// GetName 返回提供商显示名称
func (p *OIDCProvider) GetName() string {
	return "OIDC"
}

// IsEnabled 检查 OIDC 是否已启用（从系统设置读取）
func (p *OIDCProvider) IsEnabled() bool {
	return system_setting.GetOIDCSettings().Enabled
}

// ExchangeToken 使用授权码向 OIDC Token 端点交换访问令牌
// Token 端点地址从系统设置中读取（支持 OIDC Discovery 自动发现）
func (p *OIDCProvider) ExchangeToken(ctx context.Context, code string, c *gin.Context) (*OAuthToken, error) {
	if code == "" {
		return nil, NewOAuthError(i18n.MsgOAuthInvalidCode, nil)
	}

	logger.LogDebug(ctx, "[OAuth-OIDC] ExchangeToken: code=%s...", code[:min(len(code), 10)])

	settings := system_setting.GetOIDCSettings()
	redirectUri := fmt.Sprintf("%s/oauth/oidc", system_setting.ServerAddress)
	values := url.Values{}
	values.Set("client_id", settings.ClientId)
	values.Set("client_secret", settings.ClientSecret)
	values.Set("code", code)
	values.Set("grant_type", "authorization_code")
	values.Set("redirect_uri", redirectUri)

	logger.LogDebug(ctx, "[OAuth-OIDC] ExchangeToken: token_endpoint=%s, redirect_uri=%s", settings.TokenEndpoint, redirectUri)

	req, err := http.NewRequestWithContext(ctx, "POST", settings.TokenEndpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	client := http.Client{
		Timeout: 5 * time.Second,
	}
	res, err := client.Do(req)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("[OAuth-OIDC] ExchangeToken error: %s", err.Error()))
		return nil, NewOAuthErrorWithRaw(i18n.MsgOAuthConnectFailed, map[string]any{"Provider": "OIDC"}, err.Error())
	}
	defer res.Body.Close()

	logger.LogDebug(ctx, "[OAuth-OIDC] ExchangeToken response status: %d", res.StatusCode)

	var oidcResponse oidcOAuthResponse
	err = json.NewDecoder(res.Body).Decode(&oidcResponse)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("[OAuth-OIDC] ExchangeToken decode error: %s", err.Error()))
		return nil, err
	}

	if oidcResponse.AccessToken == "" {
		logger.LogError(ctx, "[OAuth-OIDC] ExchangeToken failed: empty access token")
		return nil, NewOAuthError(i18n.MsgOAuthTokenFailed, map[string]any{"Provider": "OIDC"})
	}

	logger.LogDebug(ctx, "[OAuth-OIDC] ExchangeToken success: scope=%s", oidcResponse.Scope)

	return &OAuthToken{
		AccessToken:  oidcResponse.AccessToken,
		TokenType:    oidcResponse.TokenType,
		RefreshToken: oidcResponse.RefreshToken,
		ExpiresIn:    oidcResponse.ExpiresIn,
		Scope:        oidcResponse.Scope,
		IDToken:      oidcResponse.IDToken,
	}, nil
}

// GetUserInfo 使用访问令牌从 OIDC UserInfo 端点获取用户信息
// UserInfo 端点地址从系统设置中读取
// 使用 OIDC 标准字段（sub 作为用户唯一标识）
func (p *OIDCProvider) GetUserInfo(ctx context.Context, token *OAuthToken) (*OAuthUser, error) {
	settings := system_setting.GetOIDCSettings()

	logger.LogDebug(ctx, "[OAuth-OIDC] GetUserInfo: userinfo_endpoint=%s", settings.UserInfoEndpoint)

	req, err := http.NewRequestWithContext(ctx, "GET", settings.UserInfoEndpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)

	client := http.Client{
		Timeout: 5 * time.Second,
	}
	res, err := client.Do(req)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("[OAuth-OIDC] GetUserInfo error: %s", err.Error()))
		return nil, NewOAuthErrorWithRaw(i18n.MsgOAuthConnectFailed, map[string]any{"Provider": "OIDC"}, err.Error())
	}
	defer res.Body.Close()

	logger.LogDebug(ctx, "[OAuth-OIDC] GetUserInfo response status: %d", res.StatusCode)

	if res.StatusCode != http.StatusOK {
		logger.LogError(ctx, fmt.Sprintf("[OAuth-OIDC] GetUserInfo failed: status=%d", res.StatusCode))
		return nil, NewOAuthError(i18n.MsgOAuthGetUserErr, nil)
	}

	var oidcUser oidcUser
	err = json.NewDecoder(res.Body).Decode(&oidcUser)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("[OAuth-OIDC] GetUserInfo decode error: %s", err.Error()))
		return nil, err
	}

	if oidcUser.OpenID == "" || oidcUser.Email == "" {
		logger.LogError(ctx, fmt.Sprintf("[OAuth-OIDC] GetUserInfo failed: empty fields (sub=%s, email=%s)", oidcUser.OpenID, oidcUser.Email))
		return nil, NewOAuthError(i18n.MsgOAuthUserInfoEmpty, map[string]any{"Provider": "OIDC"})
	}

	logger.LogDebug(ctx, "[OAuth-OIDC] GetUserInfo success: sub=%s, username=%s, name=%s, email=%s", oidcUser.OpenID, oidcUser.PreferredUsername, oidcUser.Name, oidcUser.Email)

	return &OAuthUser{
		ProviderUserID: oidcUser.OpenID,
		Username:       oidcUser.PreferredUsername,
		DisplayName:    oidcUser.Name,
		Email:          oidcUser.Email,
	}, nil
}

// IsUserIDTaken 检查 OIDC sub（Subject ID）是否已被其他账号关联
func (p *OIDCProvider) IsUserIDTaken(providerUserID string) bool {
	return model.IsOidcIdAlreadyTaken(providerUserID)
}

// FillUserByProviderID 通过 OIDC sub 查找并填充用户信息
func (p *OIDCProvider) FillUserByProviderID(user *model.User, providerUserID string) error {
	user.OidcId = providerUserID
	return user.FillUserByOidcId()
}

// SetProviderUserID 将 OIDC sub 设置到用户模型
func (p *OIDCProvider) SetProviderUserID(user *model.User, providerUserID string) {
	user.OidcId = providerUserID
}

// GetProviderPrefix 返回 OIDC 用户名前缀 "oidc_"
func (p *OIDCProvider) GetProviderPrefix() string {
	return "oidc_"
}
