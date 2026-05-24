// codex_oauth.go
// 本文件实现了 Codex（OpenAI）的 OAuth 2.0 授权流程，
// 包括 PKCE 授权码交换、Token 刷新、JWT 解析等功能。
// 用于管理 Codex 渠道的 OAuth 凭据获取与刷新。

package service

import (
	// 标准库
	"context"
	"crypto/rand"    // 用于生成安全随机数（state、PKCE verifier）
	"crypto/sha256"  // 用于 PKCE code_challenge 的 SHA256 哈希
	"encoding/base64" // 用于 PKCE 和 JWT 的 Base64 编解码
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	// 项目内部包
	"github.com/c1cada/NexusTok/common"
)

// Codex OAuth 2.0 相关常量定义
const (
	codexOAuthClientID     = "app_EMoamEEZ73f0CkXaXp7hrann" // OAuth 客户端 ID
	codexOAuthAuthorizeURL = "https://auth.openai.com/oauth/authorize" // OAuth 授权页面 URL
	codexOAuthTokenURL     = "https://auth.openai.com/oauth/token"     // OAuth Token 端点 URL
	codexOAuthRedirectURI  = "http://localhost:1455/auth/callback"      // OAuth 回调地址
	codexOAuthScope        = "openid profile email offline_access"       // OAuth 请求的权限范围
	codexJWTClaimPath      = "https://api.openai.com/auth"              // JWT claims 中 Codex 认证信息的路径
	defaultHTTPTimeout     = 20 * time.Second                           // 默认 HTTP 请求超时时间
)

// CodexOAuthTokenResult 表示 OAuth Token 交换/刷新的结果
type CodexOAuthTokenResult struct {
	AccessToken  string    // 访问令牌
	RefreshToken string    // 刷新令牌，用于后续获取新的 AccessToken
	ExpiresAt    time.Time // 访问令牌的过期时间
}

// CodexOAuthAuthorizationFlow 表示 OAuth 授权流程的上下文信息
// 包含 PKCE 验证所需的 state、verifier、challenge 以及授权页面 URL
type CodexOAuthAuthorizationFlow struct {
	State        string // 随机 state 参数，用于防止 CSRF 攻击
	Verifier     string // PKCE code_verifier，原始随机字符串
	Challenge    string // PKCE code_challenge，由 verifier 经 SHA256 哈希后 Base64 编码得到
	AuthorizeURL string // 完整的 OAuth 授权页面 URL，用户需访问此 URL 进行授权
}

// RefreshCodexOAuthToken 使用 refresh_token 刷新 Codex OAuth 访问令牌（不使用代理）
// 参数:
//   - ctx: 上下文，用于控制请求超时和取消
//   - refreshToken: 用于刷新访问令牌的 refresh_token
// 返回值:
//   - *CodexOAuthTokenResult: 包含新的 access_token、refresh_token 和过期时间
//   - error: 刷新失败时返回错误
func RefreshCodexOAuthToken(ctx context.Context, refreshToken string) (*CodexOAuthTokenResult, error) {
	return RefreshCodexOAuthTokenWithProxy(ctx, refreshToken, "")
}

// RefreshCodexOAuthTokenWithProxy 使用 refresh_token 刷新 Codex OAuth 访问令牌（支持代理）
// 参数:
//   - ctx: 上下文，用于控制请求超时和取消
//   - refreshToken: 用于刷新访问令牌的 refresh_token
//   - proxyURL: 代理服务器地址，为空则不使用代理
// 返回值:
//   - *CodexOAuthTokenResult: 包含新的 access_token、refresh_token 和过期时间
//   - error: 刷新失败时返回错误
func RefreshCodexOAuthTokenWithProxy(ctx context.Context, refreshToken string, proxyURL string) (*CodexOAuthTokenResult, error) {
	client, err := getCodexOAuthHTTPClient(proxyURL)
	if err != nil {
		return nil, err
	}
	return refreshCodexOAuthToken(ctx, client, codexOAuthTokenURL, codexOAuthClientID, refreshToken)
}

// ExchangeCodexAuthorizationCode 将授权码交换为 OAuth 令牌（不使用代理）
// 参数:
//   - ctx: 上下文，用于控制请求超时和取消
//   - code: OAuth 授权码
//   - verifier: PKCE code_verifier，用于验证授权请求
// 返回值:
//   - *CodexOAuthTokenResult: 包含 access_token、refresh_token 和过期时间
//   - error: 交换失败时返回错误
func ExchangeCodexAuthorizationCode(ctx context.Context, code string, verifier string) (*CodexOAuthTokenResult, error) {
	return ExchangeCodexAuthorizationCodeWithProxy(ctx, code, verifier, "")
}

// ExchangeCodexAuthorizationCodeWithProxy 将授权码交换为 OAuth 令牌（支持代理）
// 参数:
//   - ctx: 上下文，用于控制请求超时和取消
//   - code: OAuth 授权码
//   - verifier: PKCE code_verifier，用于验证授权请求
//   - proxyURL: 代理服务器地址，为空则不使用代理
// 返回值:
//   - *CodexOAuthTokenResult: 包含 access_token、refresh_token 和过期时间
//   - error: 交换失败时返回错误
func ExchangeCodexAuthorizationCodeWithProxy(ctx context.Context, code string, verifier string, proxyURL string) (*CodexOAuthTokenResult, error) {
	client, err := getCodexOAuthHTTPClient(proxyURL)
	if err != nil {
		return nil, err
	}
	return exchangeCodexAuthorizationCode(ctx, client, codexOAuthTokenURL, codexOAuthClientID, code, verifier, codexOAuthRedirectURI)
}

// CreateCodexOAuthAuthorizationFlow 创建一个新的 OAuth 授权流程
// 生成 PKCE 参数对（verifier + challenge）和随机 state，构建授权 URL
// 返回值:
//   - *CodexOAuthAuthorizationFlow: 包含 state、verifier、challenge 和授权 URL
//   - error: 生成随机数或构建 URL 失败时返回错误
func CreateCodexOAuthAuthorizationFlow() (*CodexOAuthAuthorizationFlow, error) {
	state, err := createStateHex(16)
	if err != nil {
		return nil, err
	}
	verifier, challenge, err := generatePKCEPair()
	if err != nil {
		return nil, err
	}
	u, err := buildCodexAuthorizeURL(state, challenge)
	if err != nil {
		return nil, err
	}
	return &CodexOAuthAuthorizationFlow{
		State:        state,
		Verifier:     verifier,
		Challenge:    challenge,
		AuthorizeURL: u,
	}, nil
}

// refreshCodexOAuthToken 内部函数：使用 refresh_token 刷新 OAuth 访问令牌
// 向 Token 端点发送 POST 请求，携带 grant_type=refresh_token
// 参数:
//   - ctx: 上下文，用于控制请求超时和取消
//   - client: HTTP 客户端（可配置代理）
//   - tokenURL: OAuth Token 端点地址
//   - clientID: OAuth 客户端 ID
//   - refreshToken: 刷新令牌
// 返回值:
//   - *CodexOAuthTokenResult: 包含新的 access_token、refresh_token 和过期时间
//   - error: 刷新失败时返回错误
func refreshCodexOAuthToken(
	ctx context.Context,
	client *http.Client,
	tokenURL string,
	clientID string,
	refreshToken string,
) (*CodexOAuthTokenResult, error) {
	rt := strings.TrimSpace(refreshToken)
	if rt == "" {
		return nil, errors.New("empty refresh_token")
	}

	// 构建 form 表单，使用 refresh_token 授权类型
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", rt)
	form.Set("client_id", clientID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var payload struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}

	if err := common.DecodeJson(resp.Body, &payload); err != nil {
		return nil, err
	}
	// 检查 HTTP 响应状态码是否在 2xx 成功范围内
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("codex oauth refresh failed: status=%d", resp.StatusCode)
	}

	// 验证响应中包含必需的字段
	if strings.TrimSpace(payload.AccessToken) == "" || strings.TrimSpace(payload.RefreshToken) == "" || payload.ExpiresIn <= 0 {
		return nil, errors.New("codex oauth refresh response missing fields")
	}

	// 计算令牌过期时间：当前时间 + expires_in 秒数
	return &CodexOAuthTokenResult{
		AccessToken:  strings.TrimSpace(payload.AccessToken),
		RefreshToken: strings.TrimSpace(payload.RefreshToken),
		ExpiresAt:    time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second),
	}, nil
}

// exchangeCodexAuthorizationCode 内部函数：将授权码交换为 OAuth 令牌
// 向 Token 端点发送 POST 请求，携带 grant_type=authorization_code 和 PKCE verifier
// 参数:
//   - ctx: 上下文，用于控制请求超时和取消
//   - client: HTTP 客户端（可配置代理）
//   - tokenURL: OAuth Token 端点地址
//   - clientID: OAuth 客户端 ID
//   - code: OAuth 授权码
//   - verifier: PKCE code_verifier
//   - redirectURI: 回调地址，需与授权请求中的一致
// 返回值:
//   - *CodexOAuthTokenResult: 包含 access_token、refresh_token 和过期时间
//   - error: 交换失败时返回错误
func exchangeCodexAuthorizationCode(
	ctx context.Context,
	client *http.Client,
	tokenURL string,
	clientID string,
	code string,
	verifier string,
	redirectURI string,
) (*CodexOAuthTokenResult, error) {
	c := strings.TrimSpace(code)
	v := strings.TrimSpace(verifier)
	if c == "" {
		return nil, errors.New("empty authorization code")
	}
	if v == "" {
		return nil, errors.New("empty code_verifier")
	}

	// 构建 form 表单，使用 authorization_code 授权类型，并携带 PKCE verifier
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", clientID)
	form.Set("code", c)
	form.Set("code_verifier", v) // PKCE 验证：服务端用此 verifier 验证之前的 challenge
	form.Set("redirect_uri", redirectURI)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var payload struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := common.DecodeJson(resp.Body, &payload); err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("codex oauth code exchange failed: status=%d", resp.StatusCode)
	}
	if strings.TrimSpace(payload.AccessToken) == "" || strings.TrimSpace(payload.RefreshToken) == "" || payload.ExpiresIn <= 0 {
		return nil, errors.New("codex oauth token response missing fields")
	}
	return &CodexOAuthTokenResult{
		AccessToken:  strings.TrimSpace(payload.AccessToken),
		RefreshToken: strings.TrimSpace(payload.RefreshToken),
		ExpiresAt:    time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second),
	}, nil
}

// getCodexOAuthHTTPClient 获取用于 OAuth 请求的 HTTP 客户端
// 如果指定了代理地址，则创建带代理的客户端；否则返回默认客户端
// 参数:
//   - proxyURL: 代理服务器地址，为空则不使用代理
// 返回值:
//   - *http.Client: 配置好超时和代理的 HTTP 客户端
//   - error: 创建代理客户端失败时返回错误
func getCodexOAuthHTTPClient(proxyURL string) (*http.Client, error) {
	baseClient, err := GetHttpClientWithProxy(strings.TrimSpace(proxyURL))
	if err != nil {
		return nil, err
	}
	if baseClient == nil {
		return &http.Client{Timeout: defaultHTTPTimeout}, nil
	}
	clientCopy := *baseClient
	clientCopy.Timeout = defaultHTTPTimeout
	return &clientCopy, nil
}

// buildCodexAuthorizeURL 构建 OAuth 授权页面的完整 URL
// 包含 response_type、client_id、redirect_uri、scope、PKCE challenge 等参数
// 参数:
//   - state: 随机 state 参数，用于防止 CSRF 攻击
//   - challenge: PKCE code_challenge，由 verifier 经 SHA256 哈希得到
// 返回值:
//   - string: 完整的授权 URL
//   - error: URL 解析失败时返回错误
func buildCodexAuthorizeURL(state string, challenge string) (string, error) {
	u, err := url.Parse(codexOAuthAuthorizeURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("response_type", "code")           // 使用授权码模式
	q.Set("client_id", codexOAuthClientID)
	q.Set("redirect_uri", codexOAuthRedirectURI)
	q.Set("scope", codexOAuthScope)
	q.Set("code_challenge", challenge)        // PKCE code_challenge
	q.Set("code_challenge_method", "S256")    // 使用 SHA256 方法生成 challenge
	q.Set("state", state)                     // 防 CSRF 的随机 state
	q.Set("id_token_add_organizations", "true") // 请求 ID Token 中包含组织信息
	q.Set("codex_cli_simplified_flow", "true")  // 使用 Codex CLI 简化流程
	q.Set("originator", "codex_cli_rs")         // 标识请求来源
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// createStateHex 生成指定字节数的随机十六进制字符串，用作 OAuth state 参数
// 参数:
//   - nBytes: 随机字节数
// 返回值:
//   - string: 十六进制编码的随机字符串
//   - error: 随机数生成失败时返回错误
func createStateHex(nBytes int) (string, error) {
	if nBytes <= 0 {
		return "", errors.New("invalid state bytes length")
	}
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", b), nil
}

// generatePKCEPair 生成 PKCE（Proof Key for Code Exchange）参数对
// verifier: 32 字节随机数经 Base64URL 编码
// challenge: verifier 经 SHA256 哈希后再经 Base64URL 编码
// 返回值:
//   - verifier: PKCE code_verifier
//   - challenge: PKCE code_challenge
//   - err: 随机数生成失败时返回错误
func generatePKCEPair() (verifier string, challenge string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)       // Base64URL 编码作为 code_verifier
	sum := sha256.Sum256([]byte(verifier))                   // 对 verifier 做 SHA256 哈希
	challenge = base64.RawURLEncoding.EncodeToString(sum[:]) // 哈希结果 Base64URL 编码作为 code_challenge
	return verifier, challenge, nil
}

// ExtractCodexAccountIDFromJWT 从 JWT 令牌中提取 Codex 账户 ID
// 解析 JWT 的 payload 部分，从 "https://api.openai.com/auth" 路径下提取 "chatgpt_account_id"
// 参数:
//   - token: JWT 令牌字符串
// 返回值:
//   - string: Codex 账户 ID
//   - bool: 是否成功提取
func ExtractCodexAccountIDFromJWT(token string) (string, bool) {
	claims, ok := decodeJWTClaims(token)
	if !ok {
		return "", false
	}
	raw, ok := claims[codexJWTClaimPath]
	if !ok {
		return "", false
	}
	obj, ok := raw.(map[string]any)
	if !ok {
		return "", false
	}
	v, ok := obj["chatgpt_account_id"]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	return s, true
}

// ExtractEmailFromJWT 从 JWT 令牌中提取邮箱地址
// 解析 JWT 的 payload 部分，提取顶层 "email" 字段
// 参数:
//   - token: JWT 令牌字符串
// 返回值:
//   - string: 邮箱地址
//   - bool: 是否成功提取
func ExtractEmailFromJWT(token string) (string, bool) {
	claims, ok := decodeJWTClaims(token)
	if !ok {
		return "", false
	}
	v, ok := claims["email"]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	return s, true
}

// decodeJWTClaims 解析 JWT 令牌的 payload 部分为 claims 映射
// JWT 由三部分组成（header.payload.signature），本函数解码中间的 payload 部分
// 注意：此函数不验证签名，仅做解码提取
// 参数:
//   - token: JWT 令牌字符串
// 返回值:
//   - map[string]any: 解析后的 claims 键值对
//   - bool: 是否成功解析
func decodeJWTClaims(token string) (map[string]any, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 { // JWT 必须由三部分组成：header.payload.signature
		return nil, false
	}
	payloadRaw, err := base64.RawURLEncoding.DecodeString(parts[1]) // 解码第二部分（payload）
	if err != nil {
		return nil, false
	}
	var claims map[string]any
	if err := common.Unmarshal(payloadRaw, &claims); err != nil {
		return nil, false
	}
	return claims, true
}
