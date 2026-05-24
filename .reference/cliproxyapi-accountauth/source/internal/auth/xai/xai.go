// xai - xai.go
// 包 xai 提供 xAI Grok 的 OAuth2 认证辅助功能。
// 该文件实现了 xAI OAuth 发现、令牌交换和刷新等功能。
package xai

import (
	"context"
	"encoding/base64"
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

// XAIAuth 执行 xAI OAuth 发现、令牌交换和刷新操作。
type XAIAuth struct {
	// httpClient 是用于发送 HTTP 请求的客户端
	httpClient *http.Client
}

// NewXAIAuth 使用配置代理设置创建 xAI OAuth 辅助器。
//
// 参数：
//   - cfg: 应用程序配置
//
// 返回：
//   - *XAIAuth: 新的 xAI OAuth 辅助器实例
func NewXAIAuth(cfg *config.Config) *XAIAuth {
	return NewXAIAuthWithProxyURL(cfg, "")
}

// NewXAIAuthWithProxyURL 使用显式代理 URL 创建 xAI OAuth 辅助器。
//
// 参数：
//   - cfg: 应用程序配置
//   - proxyURL: 代理 URL
//
// 返回：
//   - *XAIAuth: 新的 xAI OAuth 辅助器实例
func NewXAIAuthWithProxyURL(cfg *config.Config, proxyURL string) *XAIAuth {
	effectiveProxyURL := strings.TrimSpace(proxyURL)
	var sdkCfg config.SDKConfig
	if cfg != nil {
		sdkCfg = cfg.SDKConfig
		if effectiveProxyURL == "" {
			effectiveProxyURL = strings.TrimSpace(cfg.ProxyURL)
		}
	}
	sdkCfg.ProxyURL = effectiveProxyURL
	return &XAIAuth{httpClient: util.SetProxy(&sdkCfg, &http.Client{})}
}

// ValidateOAuthEndpoint 验证 xAI 发现返回的端点。
// 确保端点使用 HTTPS 协议且主机名属于 x.ai 域。
//
// 参数：
//   - rawURL: 要验证的原始 URL
//   - field: 字段名称，用于错误消息
//
// 返回：
//   - string: 验证后的 URL
//   - error: 验证失败时返回的错误
func ValidateOAuthEndpoint(rawURL string, field string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", fmt.Errorf("xai discovery %s is empty", field)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("xai discovery %s is invalid: %w", field, err)
	}
	if parsed.Scheme != "https" {
		return "", fmt.Errorf("xai discovery %s must use https: %q", field, rawURL)
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host != "x.ai" && !strings.HasSuffix(host, ".x.ai") {
		return "", fmt.Errorf("xai discovery %s host %q is not on x.ai", field, host)
	}
	return rawURL, nil
}

// BuildAuthorizeURL 构建 xAI OAuth 的浏览器授权 URL。
// 包含所有必需的 OAuth 参数，如客户端 ID、重定向 URI、权限范围和 PKCE 挑战码。
//
// 参数：
//   - params: 授权 URL 参数
//
// 返回：
//   - string: 完整的授权 URL
//   - error: 构建失败时返回的错误
func BuildAuthorizeURL(params AuthorizeURLParams) (string, error) {
	endpoint, err := ValidateOAuthEndpoint(params.AuthorizationEndpoint, "authorization_endpoint")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(params.RedirectURI) == "" {
		return "", fmt.Errorf("xai authorize URL: redirect URI is required")
	}
	if strings.TrimSpace(params.CodeChallenge) == "" {
		return "", fmt.Errorf("xai authorize URL: code challenge is required")
	}
	if strings.TrimSpace(params.State) == "" {
		return "", fmt.Errorf("xai authorize URL: state is required")
	}
	if strings.TrimSpace(params.Nonce) == "" {
		return "", fmt.Errorf("xai authorize URL: nonce is required")
	}
	values := url.Values{
		"response_type":         {"code"},
		"client_id":             {ClientID},
		"redirect_uri":          {strings.TrimSpace(params.RedirectURI)},
		"scope":                 {Scope},
		"code_challenge":        {strings.TrimSpace(params.CodeChallenge)},
		"code_challenge_method": {"S256"},
		"state":                 {strings.TrimSpace(params.State)},
		"nonce":                 {strings.TrimSpace(params.Nonce)},
		"plan":                  {"generic"},
		"referrer":              {"cli-proxy-api"},
	}
	return endpoint + "?" + values.Encode(), nil
}

// Discover 通过 OIDC 发现解析 xAI OAuth 端点。
// 从 xAI 的 OIDC 发现端点获取授权和令牌端点信息。
//
// 参数：
//   - ctx: 请求的上下文
//
// 返回：
//   - *Discovery: 包含 OAuth 端点的发现结果
//   - error: 发现失败时返回的错误
func (a *XAIAuth) Discover(ctx context.Context) (*Discovery, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, DiscoveryURL, nil)
	if err != nil {
		return nil, fmt.Errorf("xai discovery: create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("xai discovery: request failed: %w", err)
	}
	defer func() {
		if errClose := resp.Body.Close(); errClose != nil {
			log.Errorf("xai discovery: close response body error: %v", errClose)
		}
	}()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("xai discovery: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("xai discovery failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload struct {
		AuthorizationEndpoint string `json:"authorization_endpoint"`
		TokenEndpoint         string `json:"token_endpoint"`
	}
	if err = json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("xai discovery: parse response: %w", err)
	}
	authorizationEndpoint, err := ValidateOAuthEndpoint(payload.AuthorizationEndpoint, "authorization_endpoint")
	if err != nil {
		return nil, err
	}
	tokenEndpoint, err := ValidateOAuthEndpoint(payload.TokenEndpoint, "token_endpoint")
	if err != nil {
		return nil, err
	}
	return &Discovery{AuthorizationEndpoint: authorizationEndpoint, TokenEndpoint: tokenEndpoint}, nil
}

// ExchangeCodeForTokens 用授权码交换 xAI OAuth 令牌。
// 向 xAI 令牌端点发送请求，将授权码和 PKCE 验证器交换为访问令牌。
//
// 参数：
//   - ctx: 请求的上下文
//   - code: 从 OAuth 回调获取的授权码
//   - redirectURI: 重定向 URI
//   - pkceCodes: PKCE 代码
//   - tokenEndpoint: 令牌端点 URL
//
// 返回：
//   - *AuthBundle: 认证包
//   - error: 令牌交换失败时返回的错误
func (a *XAIAuth) ExchangeCodeForTokens(ctx context.Context, code, redirectURI string, pkceCodes *PKCECodes, tokenEndpoint string) (*AuthBundle, error) {
	if pkceCodes == nil {
		return nil, fmt.Errorf("xai token exchange: PKCE codes are required")
	}
	if strings.TrimSpace(code) == "" {
		return nil, fmt.Errorf("xai token exchange: authorization code is required")
	}
	if strings.TrimSpace(redirectURI) == "" {
		return nil, fmt.Errorf("xai token exchange: redirect URI is required")
	}
	if strings.TrimSpace(tokenEndpoint) == "" {
		discovery, errDiscover := a.Discover(ctx)
		if errDiscover != nil {
			return nil, errDiscover
		}
		tokenEndpoint = discovery.TokenEndpoint
	}
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {strings.TrimSpace(code)},
		"redirect_uri":  {strings.TrimSpace(redirectURI)},
		"client_id":     {ClientID},
		"code_verifier": {pkceCodes.CodeVerifier},
	}
	tokenData, err := a.postTokenForm(ctx, tokenEndpoint, form)
	if err != nil {
		return nil, err
	}
	return &AuthBundle{
		TokenData:     *tokenData,
		LastRefresh:   time.Now().UTC().Format(time.RFC3339),
		BaseURL:       DefaultAPIBaseURL,
		RedirectURI:   strings.TrimSpace(redirectURI),
		TokenEndpoint: strings.TrimSpace(tokenEndpoint),
	}, nil
}

// RefreshTokens 刷新 xAI 访问令牌。
// 使用刷新令牌向 xAI 令牌端点请求新的访问令牌。
//
// 参数：
//   - ctx: 请求的上下文
//   - refreshToken: 刷新令牌
//   - tokenEndpoint: 令牌端点 URL
//
// 返回：
//   - *TokenData: 新的令牌数据
//   - error: 刷新失败时返回的错误
func (a *XAIAuth) RefreshTokens(ctx context.Context, refreshToken, tokenEndpoint string) (*TokenData, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return nil, fmt.Errorf("xai token refresh: refresh token is required")
	}
	if strings.TrimSpace(tokenEndpoint) == "" {
		discovery, errDiscover := a.Discover(ctx)
		if errDiscover != nil {
			return nil, errDiscover
		}
		tokenEndpoint = discovery.TokenEndpoint
	}
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {ClientID},
		"refresh_token": {strings.TrimSpace(refreshToken)},
	}
	return a.postTokenForm(ctx, tokenEndpoint, form)
}

// postTokenForm 向令牌端点发送表单 POST 请求。
// 处理令牌请求的通用逻辑，包括请求创建、响应解析和错误处理。
//
// 参数：
//   - ctx: 请求的上下文
//   - tokenEndpoint: 令牌端点 URL
//   - form: 表单数据
//
// 返回：
//   - *TokenData: 令牌数据
//   - error: 请求失败时返回的错误
func (a *XAIAuth) postTokenForm(ctx context.Context, tokenEndpoint string, form url.Values) (*TokenData, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSpace(tokenEndpoint), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("xai token request: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("xai token request failed: %w", err)
	}
	defer func() {
		if errClose := resp.Body.Close(); errClose != nil {
			log.Errorf("xai token request: close response body error: %v", errClose)
		}
	}()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("xai token response: read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("xai token request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		IDToken      string `json:"id_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err = json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("xai token response: parse body: %w", err)
	}
	if strings.TrimSpace(payload.AccessToken) == "" {
		return nil, fmt.Errorf("xai token response missing access_token")
	}
	email, subject := parseJWTIdentity(payload.IDToken)
	return &TokenData{
		AccessToken:  strings.TrimSpace(payload.AccessToken),
		RefreshToken: strings.TrimSpace(payload.RefreshToken),
		IDToken:      strings.TrimSpace(payload.IDToken),
		TokenType:    strings.TrimSpace(payload.TokenType),
		ExpiresIn:    payload.ExpiresIn,
		Expire:       time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second).UTC().Format(time.RFC3339),
		Email:        email,
		Subject:      subject,
	}, nil
}

// CreateTokenStorage 将认证包转换为可持久化的存储。
//
// 参数：
//   - bundle: 认证包
//
// 返回：
//   - *TokenStorage: 令牌存储实例
func (a *XAIAuth) CreateTokenStorage(bundle *AuthBundle) *TokenStorage {
	if bundle == nil {
		return nil
	}
	return &TokenStorage{
		Type:          "xai",
		AccessToken:   bundle.TokenData.AccessToken,
		RefreshToken:  bundle.TokenData.RefreshToken,
		IDToken:       bundle.TokenData.IDToken,
		TokenType:     bundle.TokenData.TokenType,
		ExpiresIn:     bundle.TokenData.ExpiresIn,
		Expire:        bundle.TokenData.Expire,
		LastRefresh:   bundle.LastRefresh,
		Email:         strings.TrimSpace(bundle.TokenData.Email),
		Subject:       bundle.TokenData.Subject,
		BaseURL:       firstNonEmpty(bundle.BaseURL, DefaultAPIBaseURL),
		RedirectURI:   bundle.RedirectURI,
		TokenEndpoint: bundle.TokenEndpoint,
		AuthKind:      "oauth",
	}
}

// parseJWTIdentity 从 JWT 令牌中解析用户身份信息。
// 提取电子邮件地址和主题标识符。
//
// 参数：
//   - token: JWT 令牌字符串
//
// 返回：
//   - email: 用户的电子邮件地址
//   - subject: 用户的主题标识符
func parseJWTIdentity(token string) (email string, subject string) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return "", ""
	}
	payload := parts[1]
	payload += strings.Repeat("=", (4-len(payload)%4)%4)
	raw, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		return "", ""
	}
	var claims map[string]any
	if err = json.Unmarshal(raw, &claims); err != nil {
		return "", ""
	}
	if v, ok := claims["email"].(string); ok {
		email = strings.TrimSpace(v)
	}
	if v, ok := claims["sub"].(string); ok {
		subject = strings.TrimSpace(v)
	}
	return email, subject
}

// firstNonEmpty 返回第一个非空的字符串。
// 用于在多个可能的值中选择第一个有效的。
//
// 参数：
//   - values: 要检查的字符串列表
//
// 返回：
//   - string: 第一个非空的字符串，如果都为空则返回空字符串
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
