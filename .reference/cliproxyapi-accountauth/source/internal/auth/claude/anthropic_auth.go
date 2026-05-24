// claude - anthropic_auth.go
// 包 claude 提供 Anthropic Claude API 的 OAuth2 认证功能。
// 该文件实现了完整的 OAuth2 PKCE 认证流程，包括生成授权 URL、
// 用授权码换取令牌、刷新过期令牌以及令牌存储管理等功能。
// 使用 PKCE（Proof Key for Code Exchange）机制增强安全性。
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
// 定义了 OAuth2 认证流程所需的端点 URL、客户端 ID 和重定向 URI。
const (
	// AuthURL 是 Claude OAuth 授权端点
	AuthURL = "https://claude.ai/oauth/authorize"
	// TokenURL 是 Anthropic OAuth 令牌端点
	TokenURL = "https://api.anthropic.com/v1/oauth/token"
	// ClientID 是 OAuth 应用的客户端标识符
	ClientID = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	// RedirectURI 是 OAuth 回调地址
	RedirectURI = "http://localhost:54545/callback"

	// claudeRefreshMinBackoff 是令牌刷新重试的最小退避时间
	claudeRefreshMinBackoff = 5 * time.Second
	// claudeRefreshMaxBackoff 是令牌刷新重试的最大退避时间
	claudeRefreshMaxBackoff = 5 * time.Minute
)

var (
	// claudeRefreshGroup 用于合并同一刷新令牌的并发刷新请求（singleflight 模式）
	claudeRefreshGroup singleflight.Group
	// claudeRefreshMu 保护刷新阻塞映射的互斥锁
	claudeRefreshMu sync.Mutex
	// claudeRefreshBlock 记录每个刷新令牌被临时阻塞的截止时间
	claudeRefreshBlock = make(map[string]time.Time)
)

// refreshHTTPError 表示令牌刷新过程中发生的 HTTP 错误。
// 包含状态码、错误消息和是否可重试的标志。
type refreshHTTPError struct {
	// status 是 HTTP 响应状态码
	status int
	// message 是错误消息内容
	message string
	// retryable 指示该错误是否可以重试
	retryable bool
}

// Error 返回 HTTP 刷新错误的字符串表示。
func (e *refreshHTTPError) Error() string {
	return fmt.Sprintf("token refresh failed with status %d: %s", e.status, e.message)
}

// Retryable 返回该错误是否可重试。
func (e *refreshHTTPError) Retryable() bool {
	return e != nil && e.retryable
}

// resetClaudeRefreshState 重置 Claude 令牌刷新的全局状态。
// 清除所有刷新阻塞记录和 singleflight 组，通常在测试或重置时调用。
func resetClaudeRefreshState() {
	claudeRefreshMu.Lock()
	defer claudeRefreshMu.Unlock()
	claudeRefreshBlock = make(map[string]time.Time)
	claudeRefreshGroup = singleflight.Group{}
}

// claudeRefreshBlockedUntil 查询指定刷新令牌被临时阻塞的截止时间。
//
// 参数：
//   - refreshToken: 要查询的刷新令牌
//
// 返回：
//   - time.Time: 阻塞截止时间，如果未被阻塞则返回零值
func claudeRefreshBlockedUntil(refreshToken string) time.Time {
	claudeRefreshMu.Lock()
	defer claudeRefreshMu.Unlock()
	return claudeRefreshBlock[refreshToken]
}

// setClaudeRefreshBlockedUntil 设置指定刷新令牌的临时阻塞截止时间。
// 当遇到 429 Too Many Requests 响应时，会设置阻塞以避免短时间内重复刷新。
//
// 参数：
//   - refreshToken: 要设置阻塞的刷新令牌
//   - until: 阻塞截止时间
func setClaudeRefreshBlockedUntil(refreshToken string, until time.Time) {
	claudeRefreshMu.Lock()
	defer claudeRefreshMu.Unlock()
	claudeRefreshBlock[refreshToken] = until
}

// clearClaudeRefreshBlockedUntil 清除指定刷新令牌的临时阻塞状态。
// 当令牌刷新成功后调用此方法，允许后续的刷新请求正常执行。
//
// 参数：
//   - refreshToken: 要清除阻塞的刷新令牌
func clearClaudeRefreshBlockedUntil(refreshToken string) {
	claudeRefreshMu.Lock()
	defer claudeRefreshMu.Unlock()
	delete(claudeRefreshBlock, refreshToken)
}

// clampClaudeRefreshBackoff 将退避时间限制在最小和最大值之间。
// 确保退避时间不会过短（导致频繁重试）或过长（导致长时间等待）。
//
// 参数：
//   - d: 原始退避时间
//
// 返回：
//   - time.Duration: 限制后的退避时间
func clampClaudeRefreshBackoff(d time.Duration) time.Duration {
	if d < claudeRefreshMinBackoff {
		return claudeRefreshMinBackoff
	}
	if d > claudeRefreshMaxBackoff {
		return claudeRefreshMaxBackoff
	}
	return d
}

// parseClaudeRetryAfter 从 HTTP 响应头中解析 Retry-After 值。
// 支持秒数格式和 HTTP 日期格式的 Retry-After 头，
// 以及毫秒精度的 Retry-After-Ms 头。
//
// 参数：
//   - resp: HTTP 响应对象
//
// 返回：
//   - time.Duration: 解析后的退避时间，解析失败时返回最小退避时间
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

// isClaudeRefreshRetryable 判断令牌刷新错误是否可以重试。
// 如果错误是 HTTP 错误且标记为不可重试，则返回 false；
// 其他错误默认认为可以重试。
//
// 参数：
//   - err: 要检查的错误
//
// 返回：
//   - bool: 如果错误可以重试返回 true
func isClaudeRefreshRetryable(err error) bool {
	var httpErr *refreshHTTPError
	if errors.As(err, &httpErr) {
		return httpErr.Retryable()
	}
	return true
}

// tokenResponse 表示从 Anthropic OAuth 令牌端点返回的响应结构。
// 包含访问令牌、刷新令牌以及关联的用户和组织信息。
type tokenResponse struct {
	// AccessToken 是 OAuth2 访问令牌
	AccessToken string `json:"access_token"`
	// RefreshToken 是用于获取新访问令牌的刷新令牌
	RefreshToken string `json:"refresh_token"`
	// TokenType 是令牌类型，通常为 "Bearer"
	TokenType string `json:"token_type"`
	// ExpiresIn 是访问令牌的过期时间（秒）
	ExpiresIn int `json:"expires_in"`
	// Organization 包含组织相关信息
	Organization struct {
		// UUID 是组织的唯一标识符
		UUID string `json:"uuid"`
		// Name 是组织名称
		Name string `json:"name"`
	} `json:"organization"`
	// Account 包含账户相关信息
	Account struct {
		// UUID 是账户的唯一标识符
		UUID string `json:"uuid"`
		// EmailAddress 是账户的电子邮件地址
		EmailAddress string `json:"email_address"`
	} `json:"account"`
}

// ClaudeAuth 是 Anthropic OAuth2 认证流程的处理器。
// 提供生成授权 URL、用授权码换取令牌以及刷新过期令牌等方法。
// 使用 PKCE 机制增强 OAuth2 流程的安全性。
type ClaudeAuth struct {
	// httpClient 是用于发送 HTTP 请求的客户端，使用自定义 TLS 传输层
	httpClient *http.Client
}

// NewClaudeAuth 创建一个新的 Anthropic 认证服务实例。
// 使用默认配置初始化，内部调用 NewClaudeAuthWithProxyURL 创建。
//
// 参数：
//   - cfg: 应用程序配置，包含代理设置
//
// 返回：
//   - *ClaudeAuth: 新的 Claude 认证服务实例
func NewClaudeAuth(cfg *config.Config) *ClaudeAuth {
	return NewClaudeAuthWithProxyURL(cfg, "")
}

// NewClaudeAuthWithProxyURL 创建一个带代理覆盖的 Anthropic 认证服务实例。
// 当 proxyURL 非空时，优先使用它而非 cfg.ProxyURL。
// 使用自定义 HTTP 客户端和 Firefox TLS 指纹来绕过 Anthropic 域名的 Cloudflare 检测。
//
// 参数：
//   - cfg: 应用程序配置
//   - proxyURL: 可选的代理 URL，优先级高于配置文件中的代理设置
//
// 返回：
//   - *ClaudeAuth: 新的 Claude 认证服务实例
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

	// Use custom HTTP client with Firefox TLS fingerprint to bypass
	// Cloudflare's bot detection on Anthropic domains
	return &ClaudeAuth{
		httpClient: NewAnthropicHttpClient(sdkCfg),
	}
}

// GenerateAuthURL 创建包含 PKCE 的 OAuth 授权 URL。
// 生成包含 PKCE 挑战码的安全授权 URL，用于 Anthropic API 的 OAuth2 流程。
//
// 参数：
//   - state: 用于 CSRF 防护的随机状态参数
//   - pkceCodes: PKCE 代码，用于安全的代码交换
//
// 返回：
//   - string: 完整的授权 URL
//   - string: 状态参数，用于后续验证
//   - error: PKCE 代码缺失或 URL 生成失败时返回的错误
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

// parseCodeAndState 从 OAuth 回调响应中提取授权码和状态参数。
// 处理可能包含额外片段的 code 参数，以 "#" 分隔。
//
// 参数：
//   - code: 来自 OAuth 回调的原始 code 参数
//
// 返回：
//   - parsedCode: 提取的授权码
//   - parsedState: 提取的状态参数（如果存在）
func (c *ClaudeAuth) parseCodeAndState(code string) (parsedCode, parsedState string) {
	splits := strings.Split(code, "#")
	parsedCode = splits[0]
	if len(splits) > 1 {
		parsedState = splits[1]
	}
	return
}

// ExchangeCodeForTokens 用授权码换取访问令牌。
// 实现 OAuth2 令牌交换流程，使用 PKCE 验证器进行安全验证。
// 将授权码与 PKCE 验证器一起发送以获取访问令牌和刷新令牌。
//
// 参数：
//   - ctx: 请求的上下文
//   - code: 从 OAuth 回调获取的授权码
//   - state: 用于验证的状态参数
//   - pkceCodes: 用于安全验证的 PKCE 代码
//
// 返回：
//   - *ClaudeAuthBundle: 包含令牌的完整认证包
//   - error: 令牌交换失败时返回的错误
func (o *ClaudeAuth) ExchangeCodeForTokens(ctx context.Context, code, state string, pkceCodes *PKCECodes) (*ClaudeAuthBundle, error) {
	if pkceCodes == nil {
		return nil, fmt.Errorf("PKCE codes are required for token exchange")
	}
	newCode, newState := o.parseCodeAndState(code)

	// Prepare token exchange request
	reqBody := map[string]interface{}{
		"code":          newCode,
		"state":         state,
		"grant_type":    "authorization_code",
		"client_id":     ClientID,
		"redirect_uri":  RedirectURI,
		"code_verifier": pkceCodes.CodeVerifier,
	}

	// Include state if present
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

	// Create token data
	tokenData := ClaudeTokenData{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		Email:        tokenResp.Account.EmailAddress,
		Expire:       time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second).Format(time.RFC3339),
	}

	// Create auth bundle
	bundle := &ClaudeAuthBundle{
		TokenData:   tokenData,
		LastRefresh: time.Now().Format(time.RFC3339),
	}

	return bundle, nil
}

// RefreshTokens 使用刷新令牌刷新访问令牌。
// 使用有效的刷新令牌交换新的访问令牌，延长用户的认证会话。
// 使用 singleflight 模式合并并发的刷新请求，避免重复刷新。
//
// 参数：
//   - ctx: 请求的上下文
//   - refreshToken: 用于获取新访问令牌的刷新令牌
//
// 返回：
//   - *ClaudeTokenData: 包含新访问令牌的令牌数据
//   - error: 令牌刷新失败时返回的错误
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

// refreshTokensSingleFlight 是 RefreshTokens 的内部实现，通过 singleflight 机制执行。
// 确保同一刷新令牌的并发刷新请求只会执行一次实际的网络请求。
//
// 参数：
//   - ctx: 请求的上下文
//   - refreshToken: 用于获取新访问令牌的刷新令牌
//
// 返回：
//   - *ClaudeTokenData: 包含新访问令牌的令牌数据
//   - error: 令牌刷新失败时返回的错误
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

// CreateTokenStorage 从认证包创建 ClaudeTokenStorage 实例。
// 将认证包中的令牌数据转换为适合持久化存储的结构。
//
// 参数：
//   - bundle: 包含令牌数据的认证包
//
// 返回：
//   - *ClaudeTokenStorage: 新的令牌存储实例
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

// RefreshTokensWithRetry 带自动重试逻辑的令牌刷新。
// 实现指数退避重试策略，为令牌刷新操作提供弹性，
// 以应对临时的网络或服务问题。
//
// 参数：
//   - ctx: 请求的上下文
//   - refreshToken: 用于刷新的刷新令牌
//   - maxRetries: 最大重试次数
//
// 返回：
//   - *ClaudeTokenData: 刷新后的令牌数据
//   - error: 所有重试尝试都失败时返回的错误
func (o *ClaudeAuth) RefreshTokensWithRetry(ctx context.Context, refreshToken string, maxRetries int) (*ClaudeTokenData, error) {
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

		lastErr = err
		log.Warnf("Token refresh attempt %d failed: %v", attempt+1, err)
		if !isClaudeRefreshRetryable(err) {
			break
		}
	}

	return nil, fmt.Errorf("token refresh failed after %d attempts: %w", maxRetries, lastErr)
}

// UpdateTokenStorage 使用新的令牌数据更新现有的令牌存储。
// 在成功刷新令牌后调用此方法，用新获取的访问令牌和刷新令牌更新存储，
// 同时更新时间戳和过期信息。
//
// 参数：
//   - storage: 要更新的现有令牌存储
//   - tokenData: 要应用的新令牌数据
func (o *ClaudeAuth) UpdateTokenStorage(storage *ClaudeTokenStorage, tokenData *ClaudeTokenData) {
	storage.AccessToken = tokenData.AccessToken
	storage.RefreshToken = tokenData.RefreshToken
	storage.LastRefresh = time.Now().Format(time.RFC3339)
	storage.Email = tokenData.Email
	storage.Expire = tokenData.Expire
}
