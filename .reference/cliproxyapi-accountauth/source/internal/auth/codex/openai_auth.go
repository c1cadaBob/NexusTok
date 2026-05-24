// codex - openai_auth.go
// 包 codex 提供 OpenAI Codex API 的认证和令牌管理功能。
// 该文件实现了完整的 OAuth2 PKCE 认证流程，包括生成授权 URL、
// 用授权码换取令牌、刷新过期令牌以及令牌存储管理等功能。
package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/util"
	log "github.com/sirupsen/logrus"
)

// OpenAI Codex OAuth 配置常量。
// 定义了 OAuth2 认证流程所需的端点 URL、客户端 ID 和重定向 URI。
const (
	// AuthURL 是 OpenAI OAuth 授权端点
	AuthURL = "https://auth.openai.com/oauth/authorize"
	// TokenURL 是 OpenAI OAuth 令牌端点
	TokenURL = "https://auth.openai.com/oauth/token"
	// ClientID 是 OAuth 应用的客户端标识符
	ClientID = "app_EMoamEEZ73f0CkXaXp7hrann"
	// RedirectURI 是 OAuth 回调地址
	RedirectURI = "http://localhost:1455/auth/callback"
)

// CodexAuth 处理 OpenAI OAuth2 认证流程。
// 管理 HTTP 客户端，并提供生成授权 URL、用授权码换取令牌和刷新访问令牌等方法。
type CodexAuth struct {
	// httpClient 是用于发送 HTTP 请求的客户端
	httpClient *http.Client
}

// NewCodexAuth 创建一个新的 CodexAuth 服务实例。
// 使用提供的配置初始化 HTTP 客户端，并设置代理。
//
// 参数：
//   - cfg: 应用程序配置，包含代理设置
//
// 返回：
//   - *CodexAuth: 新的 CodexAuth 服务实例
func NewCodexAuth(cfg *config.Config) *CodexAuth {
	return NewCodexAuthWithProxyURL(cfg, "")
}

// NewCodexAuthWithProxyURL 创建一个新的 CodexAuth 服务实例。
// 当 proxyURL 非空时，优先使用它而非 cfg.ProxyURL。
//
// 参数：
//   - cfg: 应用程序配置
//   - proxyURL: 可选的代理 URL，优先级高于配置文件中的代理设置
//
// 返回：
//   - *CodexAuth: 新的 CodexAuth 服务实例
func NewCodexAuthWithProxyURL(cfg *config.Config, proxyURL string) *CodexAuth {
	effectiveProxyURL := strings.TrimSpace(proxyURL)
	var sdkCfg config.SDKConfig
	if cfg != nil {
		sdkCfg = cfg.SDKConfig
		if effectiveProxyURL == "" {
			effectiveProxyURL = strings.TrimSpace(cfg.ProxyURL)
		}
	}
	sdkCfg.ProxyURL = effectiveProxyURL
	return &CodexAuth{
		httpClient: util.SetProxy(&sdkCfg, &http.Client{}),
	}
}

// GenerateAuthURL 创建包含 PKCE 的 OAuth 授权 URL。
// 构建包含客户端 ID、响应类型、重定向 URI、权限范围和 PKCE 挑战码的授权 URL。
//
// 参数：
//   - state: 用于 CSRF 防护的随机状态参数
//   - pkceCodes: PKCE 代码，用于安全的代码交换
//
// 返回：
//   - string: 完整的授权 URL
//   - error: PKCE 代码缺失时返回的错误
func (o *CodexAuth) GenerateAuthURL(state string, pkceCodes *PKCECodes) (string, error) {
	if pkceCodes == nil {
		return "", fmt.Errorf("PKCE codes are required")
	}

	params := url.Values{
		"client_id":                  {ClientID},
		"response_type":              {"code"},
		"redirect_uri":               {RedirectURI},
		"scope":                      {"openid email profile offline_access"},
		"state":                      {state},
		"code_challenge":             {pkceCodes.CodeChallenge},
		"code_challenge_method":      {"S256"},
		"prompt":                     {"login"},
		"id_token_add_organizations": {"true"},
		"codex_cli_simplified_flow":  {"true"},
	}

	authURL := fmt.Sprintf("%s?%s", AuthURL, params.Encode())
	return authURL, nil
}

// ExchangeCodeForTokens 用授权码换取访问令牌和刷新令牌。
// 向 OpenAI 令牌端点发送 HTTP POST 请求，将提供的授权码和 PKCE 验证器交换为令牌。
//
// 参数：
//   - ctx: 请求的上下文
//   - code: 从 OAuth 回调获取的授权码
//   - pkceCodes: 用于安全验证的 PKCE 代码
//
// 返回：
//   - *CodexAuthBundle: 包含令牌的完整认证包
//   - error: 令牌交换失败时返回的错误
func (o *CodexAuth) ExchangeCodeForTokens(ctx context.Context, code string, pkceCodes *PKCECodes) (*CodexAuthBundle, error) {
	return o.ExchangeCodeForTokensWithRedirect(ctx, code, RedirectURI, pkceCodes)
}

// ExchangeCodeForTokensWithRedirect 使用调用方提供的重定向 URI 用授权码换取令牌。
// 支持替代的认证流程（如设备登录），同时保留现有的令牌解析和存储行为。
//
// 参数：
//   - ctx: 请求的上下文
//   - code: 从 OAuth 回调获取的授权码
//   - redirectURI: 调用方提供的重定向 URI
//   - pkceCodes: 用于安全验证的 PKCE 代码
//
// 返回：
//   - *CodexAuthBundle: 包含令牌的完整认证包
//   - error: 令牌交换失败时返回的错误
func (o *CodexAuth) ExchangeCodeForTokensWithRedirect(ctx context.Context, code, redirectURI string, pkceCodes *PKCECodes) (*CodexAuthBundle, error) {
	if pkceCodes == nil {
		return nil, fmt.Errorf("PKCE codes are required for token exchange")
	}
	if strings.TrimSpace(redirectURI) == "" {
		return nil, fmt.Errorf("redirect URI is required for token exchange")
	}

	// Prepare token exchange request
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {ClientID},
		"code":          {code},
		"redirect_uri":  {strings.TrimSpace(redirectURI)},
		"code_verifier": {pkceCodes.CodeVerifier},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token exchange request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read token response: %w", err)
	}
	// log.Debugf("Token response: %s", string(body))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse token response
	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		IDToken      string `json:"id_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
	}

	if err = json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	// Extract account ID from ID token
	claims, err := ParseJWTToken(tokenResp.IDToken)
	if err != nil {
		log.Warnf("Failed to parse ID token: %v", err)
	}

	accountID := ""
	email := ""
	if claims != nil {
		accountID = claims.GetAccountID()
		email = claims.GetUserEmail()
	}

	// Create token data
	tokenData := CodexTokenData{
		IDToken:      tokenResp.IDToken,
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		AccountID:    accountID,
		Email:        email,
		Expire:       time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second).Format(time.RFC3339),
	}

	// Create auth bundle
	bundle := &CodexAuthBundle{
		TokenData:   tokenData,
		LastRefresh: time.Now().Format(time.RFC3339),
	}

	return bundle, nil
}

// RefreshTokens 使用刷新令牌刷新访问令牌。
// 当访问令牌过期时调用此方法，向令牌端点发送请求以获取新的令牌集。
//
// 参数：
//   - ctx: 请求的上下文
//   - refreshToken: 用于获取新访问令牌的刷新令牌
//
// 返回：
//   - *CodexTokenData: 包含新访问令牌的令牌数据
//   - error: 令牌刷新失败时返回的错误
func (o *CodexAuth) RefreshTokens(ctx context.Context, refreshToken string) (*CodexTokenData, error) {
	if refreshToken == "" {
		return nil, fmt.Errorf("refresh token is required")
	}

	data := url.Values{
		"client_id":     {ClientID},
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"scope":         {"openid profile email"},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create refresh request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token refresh request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read refresh response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token refresh failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		IDToken      string `json:"id_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
	}

	if err = json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse refresh response: %w", err)
	}

	// Extract account ID from ID token
	claims, err := ParseJWTToken(tokenResp.IDToken)
	if err != nil {
		log.Warnf("Failed to parse refreshed ID token: %v", err)
	}

	accountID := ""
	email := ""
	if claims != nil {
		accountID = claims.GetAccountID()
		email = claims.Email
	}

	return &CodexTokenData{
		IDToken:      tokenResp.IDToken,
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		AccountID:    accountID,
		Email:        email,
		Expire:       time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second).Format(time.RFC3339),
	}, nil
}

// CreateTokenStorage 从 CodexAuthBundle 创建 CodexTokenStorage。
// 使用令牌数据、用户信息和时间戳填充存储结构。
//
// 参数：
//   - bundle: 包含令牌数据的认证包
//
// 返回：
//   - *CodexTokenStorage: 新的令牌存储实例
func (o *CodexAuth) CreateTokenStorage(bundle *CodexAuthBundle) *CodexTokenStorage {
	storage := &CodexTokenStorage{
		IDToken:      bundle.TokenData.IDToken,
		AccessToken:  bundle.TokenData.AccessToken,
		RefreshToken: bundle.TokenData.RefreshToken,
		AccountID:    bundle.TokenData.AccountID,
		LastRefresh:  bundle.LastRefresh,
		Email:        bundle.TokenData.Email,
		Expire:       bundle.TokenData.Expire,
	}

	return storage
}

// RefreshTokensWithRetry 带内置重试机制的令牌刷新。
// 尝试刷新令牌，最多重试指定次数，使用指数退避策略处理瞬态网络错误。
//
// 参数：
//   - ctx: 请求的上下文
//   - refreshToken: 用于刷新的刷新令牌
//   - maxRetries: 最大重试次数
//
// 返回：
//   - *CodexTokenData: 刷新后的令牌数据
//   - error: 所有重试尝试都失败时返回的错误
func (o *CodexAuth) RefreshTokensWithRetry(ctx context.Context, refreshToken string, maxRetries int) (*CodexTokenData, error) {
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Wait before retry
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}

		tokenData, err := o.RefreshTokens(ctx, refreshToken)
		if err == nil {
			return tokenData, nil
		}
		if isNonRetryableRefreshErr(err) {
			log.Warnf("Token refresh attempt %d failed with non-retryable error: %v", attempt+1, err)
			return nil, err
		}

		lastErr = err
		log.Warnf("Token refresh attempt %d failed: %v", attempt+1, err)
	}

	return nil, fmt.Errorf("token refresh failed after %d attempts: %w", maxRetries, lastErr)
}

// isNonRetryableRefreshErr 判断令牌刷新错误是否不可重试。
// 检查错误消息中是否包含 "refresh_token_reused"，如果是则表示不可重试。
//
// 参数：
//   - err: 要检查的错误
//
// 返回：
//   - bool: 如果错误不可重试返回 true
func isNonRetryableRefreshErr(err error) bool {
	if err == nil {
		return false
	}
	raw := strings.ToLower(err.Error())
	return strings.Contains(raw, "refresh_token_reused")
}

// UpdateTokenStorage 使用新的令牌数据更新现有的 CodexTokenStorage。
// 通常在成功刷新令牌后调用此方法，用于持久化新的凭证。
//
// 参数：
//   - storage: 要更新的现有令牌存储
//   - tokenData: 要应用的新令牌数据
func (o *CodexAuth) UpdateTokenStorage(storage *CodexTokenStorage, tokenData *CodexTokenData) {
	storage.IDToken = tokenData.IDToken
	storage.AccessToken = tokenData.AccessToken
	storage.RefreshToken = tokenData.RefreshToken
	storage.AccountID = tokenData.AccountID
	storage.LastRefresh = time.Now().Format(time.RFC3339)
	storage.Email = tokenData.Email
	storage.Expire = tokenData.Expire
}
