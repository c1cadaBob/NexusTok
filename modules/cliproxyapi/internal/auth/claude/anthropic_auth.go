// claude - anthropic_auth.go
// 实现 Claude/Anthropic OAuth2 认证流程，包括 PKCE 授权 URL 构建、授权码交换、
// Token 刷新（带 singleflight 去重和指数退避重试）、429 限流阻断等功能。
package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/singleflight"
)

// Claude/Anthropic OAuth 配置常量。
const (
	AuthURL     = "https://claude.ai/oauth/authorize"
	TokenURL    = "https://api.anthropic.com/v1/oauth/token"
	ClientID    = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	RedirectURI = "http://localhost:54545/callback"

	claudeRefreshMinBackoff = 5 * time.Second
	claudeRefreshMaxBackoff = 5 * time.Minute
)

var (
	// claudeRefreshGroup 用于对并发刷新请求进行 singleflight 去重
	claudeRefreshGroup singleflight.Group
	// claudeRefreshMu 保护 claudeRefreshBlock 的互斥锁
	claudeRefreshMu sync.Mutex
	// claudeRefreshBlock 记录每个 refresh token 被 429 限流阻断后的解封时间
	claudeRefreshBlock = make(map[string]time.Time)
)

// refreshHTTPError 表示 Token 刷新过程中的 HTTP 错误。
type refreshHTTPError struct {
	// status 是 HTTP 状态码
	status int
	// message 是错误消息内容
	message string
	// retryable 标识此错误是否可重试
	retryable bool
}

// Error 返回 HTTP 刷新错误的字符串表示。
func (e *refreshHTTPError) Error() string {
	return fmt.Sprintf("token refresh failed with status %d: %s", e.status, e.message)
}

// Retryable 返回此错误是否可重试。
func (e *refreshHTTPError) Retryable() bool {
	return e != nil && e.retryable
}

// resetClaudeRefreshState 重置 Claude Token 刷新的全局状态（用于测试）。
func resetClaudeRefreshState() {
	claudeRefreshMu.Lock()
	defer claudeRefreshMu.Unlock()
	claudeRefreshBlock = make(map[string]time.Time)
	claudeRefreshGroup = singleflight.Group{}
}

// claudeRefreshBlockedUntil 返回指定 refresh token 的阻断截止时间。
func claudeRefreshBlockedUntil(refreshToken string) time.Time {
	claudeRefreshMu.Lock()
	defer claudeRefreshMu.Unlock()
	return claudeRefreshBlock[refreshToken]
}

// setClaudeRefreshBlockedUntil 设置指定 refresh token 的阻断截止时间。
func setClaudeRefreshBlockedUntil(refreshToken string, until time.Time) {
	claudeRefreshMu.Lock()
	defer claudeRefreshMu.Unlock()
	claudeRefreshBlock[refreshToken] = until
}

// clearClaudeRefreshBlockedUntil 清除指定 refresh token 的阻断状态。
func clearClaudeRefreshBlockedUntil(refreshToken string) {
	claudeRefreshMu.Lock()
	defer claudeRefreshMu.Unlock()
	delete(claudeRefreshBlock, refreshToken)
}

// clampClaudeRefreshBackoff 将退避时长限制在最小和最大值之间。
func clampClaudeRefreshBackoff(d time.Duration) time.Duration {
	if d < claudeRefreshMinBackoff {
		return claudeRefreshMinBackoff
	}
	if d > claudeRefreshMaxBackoff {
		return claudeRefreshMaxBackoff
	}
	return d
}

// parseClaudeRetryAfter 从 HTTP 响应头中解析 Retry-After 或 Retry-After-Ms 值，
// 返回经过钳制的退避时长。
func parseClaudeRetryAfter(resp *http.Response) time.Duration {
	if resp == nil {
		return claudeRefreshMinBackoff
	}
	if raw := strings.TrimSpace(resp.Header.Get("Retry-After")); raw != "" {
		if seconds, err := time.ParseDuration(raw + "s"); err == nil {
			return clampClaudeRefreshBackoff(seconds)
		}
		if when, err := http.ParseTime(raw); err == nil {
			return clampClaudeRefreshBackoff(time.Until(when))
		}
	}
	if raw := strings.TrimSpace(resp.Header.Get("Retry-After-Ms")); raw != "" {
		if ms, err := time.ParseDuration(raw + "ms"); err == nil {
			return clampClaudeRefreshBackoff(ms)
		}
	}
	return claudeRefreshMinBackoff
}

// isClaudeRefreshRetryable 检查 Token 刷新错误是否可重试。
func isClaudeRefreshRetryable(err error) bool {
	var httpErr *refreshHTTPError
	if errors.As(err, &httpErr) {
		return httpErr.Retryable()
	}
	return true
}

// tokenResponse 表示 Anthropic OAuth Token 端点的响应结构。
// 包含访问令牌、刷新令牌及关联的用户/组织信息。
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Organization struct {
		UUID string `json:"uuid"`
		Name string `json:"name"`
	} `json:"organization"`
	Account struct {
		UUID         string `json:"uuid"`
		EmailAddress string `json:"email_address"`
	} `json:"account"`
}

// ClaudeAuth 处理 Anthropic OAuth2 认证流程。
// 提供生成授权 URL、用授权码交换 Token、以及使用 PKCE 安全刷新过期 Token 的方法。
type ClaudeAuth struct {
	httpClient *http.Client
}

// NewClaudeAuth 创建新的 Anthropic 认证服务实例。
// 使用自定义 TLS 传输层（Chrome 指纹）绕过 Anthropic 域名上的 Cloudflare TLS 指纹检测。
//
// 参数:
//   - cfg: 包含代理设置的应用配置
//
// 返回值:
//   - *ClaudeAuth: 新的 Claude 认证服务实例
func NewClaudeAuth(cfg *config.Config) *ClaudeAuth {
	return NewClaudeAuthWithProxyURL(cfg, "")
}

// NewClaudeAuthWithProxyURL 创建带有显式代理 URL 覆盖的 Anthropic 认证服务实例。
// 当 proxyURL 非空时，其优先级高于 cfg.ProxyURL。
func NewClaudeAuthWithProxyURL(cfg *config.Config, proxyURL string) *ClaudeAuth {
	effectiveProxyURL := strings.TrimSpace(proxyURL)
	var sdkCfg *config.SDKConfig
	if cfg != nil {
		sdkCfgCopy := cfg.SDKConfig
		if effectiveProxyURL == "" {
			effectiveProxyURL = strings.TrimSpace(cfg.ProxyURL)
		}
		sdkCfgCopy.ProxyURL = effectiveProxyURL
		sdkCfg = &sdkCfgCopy
	} else if effectiveProxyURL != "" {
		sdkCfgCopy := config.SDKConfig{ProxyURL: effectiveProxyURL}
		sdkCfg = &sdkCfgCopy
	}

	// 使用自定义 HTTP 客户端（Chrome TLS 指纹）绕过
	// Anthropic 域名上的 Cloudflare 机器人检测
	return &ClaudeAuth{
		httpClient: NewAnthropicHttpClient(sdkCfg),
	}
}

// GenerateAuthURL 创建带有 PKCE 的 OAuth 授权 URL。
// 该方法生成包含 PKCE 挑战码的安全授权 URL，用于 Anthropic API 的 OAuth2 流程。
//
// 参数:
//   - state: 用于 CSRF 防护的随机 state 参数
//   - pkceCodes: 用于安全代码交换的 PKCE 码对
//
// 返回值:
//   - string: 完整的授权 URL
//   - string: 用于验证的 state 参数
//   - error: PKCE 码缺失或 URL 生成失败时返回错误
func (o *ClaudeAuth) GenerateAuthURL(state string, pkceCodes *PKCECodes) (string, string, error) {
	if pkceCodes == nil {
		return "", "", fmt.Errorf("PKCE codes are required")
	}

	params := url.Values{
		"code":                  {"true"},
		"client_id":             {ClientID},
		"response_type":         {"code"},
		"redirect_uri":          {RedirectURI},
		"scope":                 {"user:profile user:inference user:sessions:claude_code user:mcp_servers user:file_upload"},
		"code_challenge":        {pkceCodes.CodeChallenge},
		"code_challenge_method": {"S256"},
		"state":                 {state},
	}

	authURL := fmt.Sprintf("%s?%s", AuthURL, params.Encode())
	return authURL, state, nil
}

// parseCodeAndState 从回调响应中提取授权码和 state 参数。
// 处理可能包含额外片段的 code 参数。
//
// 参数:
//   - code: OAuth 回调中的原始 code 参数
//
// 返回值:
//   - parsedCode: 提取的授权码
//   - parsedState: 提取的 state 参数（如果存在）
func (c *ClaudeAuth) parseCodeAndState(code string) (parsedCode, parsedState string) {
	splits := strings.Split(code, "#")
	parsedCode = splits[0]
	if len(splits) > 1 {
		parsedState = splits[1]
	}
	return
}

// ExchangeCodeForTokens 将授权码交换为访问令牌。
// 该方法实现使用 PKCE 安全验证的 OAuth2 Token 交换流程，
// 将授权码与 PKCE Verifier 一起发送以获取访问令牌和刷新令牌。
//
// 参数:
//   - ctx: 请求的上下文
//   - code: 从 OAuth 回调收到的授权码
//   - state: 用于验证的 state 参数
//   - pkceCodes: 用于安全验证的 PKCE 码对
//
// 返回值:
//   - *ClaudeAuthBundle: 包含 Token 的完整认证包
//   - error: Token 交换失败时返回错误
func (o *ClaudeAuth) ExchangeCodeForTokens(ctx context.Context, code, state string, pkceCodes *PKCECodes) (*ClaudeAuthBundle, error) {
	if pkceCodes == nil {
		return nil, fmt.Errorf("PKCE codes are required for token exchange")
	}
	newCode, newState := o.parseCodeAndState(code)

	// 准备 Token 交换请求
	reqBody := map[string]interface{}{
		"code":          newCode,
		"state":         state,
		"grant_type":    "authorization_code",
		"client_id":     ClientID,
		"redirect_uri":  RedirectURI,
		"code_verifier": pkceCodes.CodeVerifier,
	}

	// 如果存在 state 则包含
	if newState != "" {
		reqBody["state"] = newState
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	// log.Debugf("Token exchange request: %s", string(jsonBody))

	req, err := http.NewRequestWithContext(ctx, "POST", TokenURL, strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token exchange request failed: %w", err)
	}
	defer func() {
		if errClose := resp.Body.Close(); errClose != nil {
			log.Errorf("failed to close response body: %v", errClose)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read token response: %w", err)
	}
	// log.Debugf("Token response: %s", string(body))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed with status %d: %s", resp.StatusCode, string(body))
	}
	// log.Debugf("Token response: %s", string(body))

	var tokenResp tokenResponse
	if err = json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	// 创建 Token 数据
	tokenData := ClaudeTokenData{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		Email:        tokenResp.Account.EmailAddress,
		Expire:       time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second).Format(time.RFC3339),
	}

	// 创建认证包
	bundle := &ClaudeAuthBundle{
		TokenData:   tokenData,
		LastRefresh: time.Now().Format(time.RFC3339),
	}

	return bundle, nil
}

// RefreshTokens 使用刷新令牌刷新访问令牌。
// 该方法用有效的刷新令牌交换新的访问令牌，延长用户的认证会话。
// 内部使用 singleflight 机制对并发刷新请求进行去重。
//
// 参数:
//   - ctx: 请求的上下文
//   - refreshToken: 用于获取新访问令牌的刷新令牌
//
// 返回值:
//   - *ClaudeTokenData: 包含更新后访问令牌的新 Token 数据
//   - error: Token 刷新失败时返回错误
func (o *ClaudeAuth) RefreshTokens(ctx context.Context, refreshToken string) (*ClaudeTokenData, error) {
	if refreshToken == "" {
		return nil, fmt.Errorf("refresh token is required")
	}
	if blockedUntil := claudeRefreshBlockedUntil(refreshToken); blockedUntil.After(time.Now()) {
		return nil, &refreshHTTPError{
			status:    http.StatusTooManyRequests,
			message:   fmt.Sprintf("refresh temporarily blocked until %s", blockedUntil.Format(time.RFC3339)),
			retryable: false,
		}
	}

	result, err, _ := claudeRefreshGroup.Do(refreshToken, func() (interface{}, error) {
		return o.refreshTokensSingleFlight(context.WithoutCancel(ctx), refreshToken)
	})
	if err != nil {
		return nil, err
	}
	tokenData, ok := result.(*ClaudeTokenData)
	if !ok || tokenData == nil {
		return nil, fmt.Errorf("token refresh failed: invalid single-flight result")
	}
	return tokenData, nil
}

// refreshTokensSingleFlight 是 singleflight 保护下的实际刷新逻辑。
func (o *ClaudeAuth) refreshTokensSingleFlight(ctx context.Context, refreshToken string) (*ClaudeTokenData, error) {
	if blockedUntil := claudeRefreshBlockedUntil(refreshToken); blockedUntil.After(time.Now()) {
		return nil, &refreshHTTPError{
			status:    http.StatusTooManyRequests,
			message:   fmt.Sprintf("refresh temporarily blocked until %s", blockedUntil.Format(time.RFC3339)),
			retryable: false,
		}
	}

	reqBody := map[string]interface{}{
		"client_id":     ClientID,
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", TokenURL, strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, fmt.Errorf("failed to create refresh request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
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
		message := string(body)
		if resp.StatusCode == http.StatusTooManyRequests {
			retryAfter := parseClaudeRetryAfter(resp)
			setClaudeRefreshBlockedUntil(refreshToken, time.Now().Add(retryAfter))
			return nil, &refreshHTTPError{status: resp.StatusCode, message: message, retryable: false}
		}
		return nil, &refreshHTTPError{
			status:    resp.StatusCode,
			message:   message,
			retryable: resp.StatusCode >= http.StatusInternalServerError,
		}
	}

	// log.Debugf("Token response: %s", string(body))

	var tokenResp tokenResponse
	if err = json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	// Create token data
	clearClaudeRefreshBlockedUntil(refreshToken)

	return &ClaudeTokenData{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		Email:        tokenResp.Account.EmailAddress,
		Expire:       time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second).Format(time.RFC3339),
	}, nil
}

// CreateTokenStorage 从认证包和用户信息创建新的 ClaudeTokenStorage。
// 该方法将认证包转换为适合持久化的 Token 存储结构。
//
// 参数:
//   - bundle: 包含 Token 数据的认证包
//
// 返回值:
//   - *ClaudeTokenStorage: 新的 Token 存储实例
func (o *ClaudeAuth) CreateTokenStorage(bundle *ClaudeAuthBundle) *ClaudeTokenStorage {
	storage := &ClaudeTokenStorage{
		AccessToken:  bundle.TokenData.AccessToken,
		RefreshToken: bundle.TokenData.RefreshToken,
		LastRefresh:  bundle.LastRefresh,
		Email:        bundle.TokenData.Email,
		Expire:       bundle.TokenData.Expire,
	}

	return storage
}

// RefreshTokensWithRetry 带自动重试逻辑的 Token 刷新。
// 该方法实现了指数退避重试逻辑，为 Token 刷新操作提供对临时网络或服务问题的弹性。
//
// 参数:
//   - ctx: 请求的上下文
//   - refreshToken: 用于刷新的刷新令牌
//   - maxRetries: 最大重试次数
//
// 返回值:
//   - *ClaudeTokenData: 刷新后的 Token 数据
//   - error: 所有重试尝试均失败时返回错误
func (o *ClaudeAuth) RefreshTokensWithRetry(ctx context.Context, refreshToken string, maxRetries int) (*ClaudeTokenData, error) {
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// 重试前等待
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

		lastErr = err
		log.Warnf("Token refresh attempt %d failed: %v", attempt+1, err)
		if !isClaudeRefreshRetryable(err) {
			break
		}
	}

	return nil, fmt.Errorf("token refresh failed after %d attempts: %w", maxRetries, lastErr)
}

// UpdateTokenStorage 使用新的 Token 数据更新已有的 Token 存储。
// 该方法用新获取的访问令牌和刷新令牌刷新 Token 存储，更新时间戳和过期信息。
//
// 参数:
//   - storage: 需要更新的已有 Token 存储
//   - tokenData: 要应用的新 Token 数据
func (o *ClaudeAuth) UpdateTokenStorage(storage *ClaudeTokenStorage, tokenData *ClaudeTokenData) {
	storage.AccessToken = tokenData.AccessToken
	storage.RefreshToken = tokenData.RefreshToken
	storage.LastRefresh = time.Now().Format(time.RFC3339)
	storage.Email = tokenData.Email
	storage.Expire = tokenData.Expire
}
