// codex.go 实现了 Codex (OpenAI) 账号认证提供者。
// 负责通过 OAuth 2.0 授权码流程和 Device Code 流程完成用户登录，
// 获取并管理 access_token / refresh_token，构建账号凭证，以及定时刷新令牌。
package accountauth

import (
	"bytes"   // 用于构建 HTTP 请求体
	"context" // 用于传递请求上下文和超时控制
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common" // 公共工具：JSON 序列化等
	"github.com/c1cada/NexusTok/model"  // 数据模型：账号池、账号等
)

// Codex OAuth 认证相关的常量定义
const (
	codexProvider                = "codex"                                                    // 提供者标识
	codexOAuthClientID           = "app_EMoamEEZ73f0CkXaXp7hrann"                             // OAuth 客户端 ID
	codexOAuthAuthorizeURL       = "https://auth.openai.com/oauth/authorize"                  // OAuth 授权页面地址
	codexOAuthTokenURL           = "https://auth.openai.com/oauth/token"                      // OAuth Token 交换地址
	codexOAuthRedirectURI        = "http://localhost:1455/auth/callback"                      // OAuth 回调地址（本地）
	codexDeviceRedirectURI       = "https://auth.openai.com/deviceauth/callback"              // Device 流程回调地址
	codexDeviceUserCodeURL       = "https://auth.openai.com/api/accounts/deviceauth/usercode" // Device 流程获取 user_code 的地址
	codexDeviceTokenURL          = "https://auth.openai.com/api/accounts/deviceauth/token"    // Device 流程轮询 token 的地址
	codexDeviceVerificationURL   = "https://auth.openai.com/codex/device"                     // Device 流程用户验证页面
	codexOAuthScope              = "openid profile email offline_access"                      // OAuth 请求的权限范围
	codexJWTClaimPath            = "https://api.openai.com/auth"                              // JWT 中 OpenAI 自定义 claims 路径
	codexDeviceDefaultInterval   = 5 * time.Second                                            // Device 流程默认轮询间隔
	codexDeviceSessionExpiration = 15 * time.Minute                                           // Device 流程会话过期时间
)

// CodexProvider 实现了 Provider 接口，提供 Codex (OpenAI) 的认证能力
type CodexProvider struct{}

// codexOAuthKey 表示 Codex OAuth 凭证的存储结构，用于序列化/反序列化到数据库
type codexOAuthKey struct {
	IDToken      string `json:"id_token,omitempty"`      // OpenID Connect ID Token
	AccessToken  string `json:"access_token,omitempty"`  // 访问令牌
	RefreshToken string `json:"refresh_token,omitempty"` // 刷新令牌
	AccountID    string `json:"account_id,omitempty"`    // ChatGPT 账号 ID
	LastRefresh  string `json:"last_refresh,omitempty"`  // 上次刷新时间
	Email        string `json:"email,omitempty"`         // 用户邮箱
	Type         string `json:"type,omitempty"`          // 凭证类型标识
	Expired      string `json:"expired,omitempty"`       // 令牌过期时间
}

// codexTokenResult 表示 OAuth Token 交换的返回结果
type codexTokenResult struct {
	IDToken      string    // ID Token
	AccessToken  string    // 访问令牌
	RefreshToken string    // 刷新令牌
	ExpiresAt    time.Time // 令牌过期时间点
}

// init 向全局注册表注册 Codex 提供者实例
func init() {
	RegisterProvider(&CodexProvider{})
}

// Name 返回提供者标识名称
func (p *CodexProvider) Name() string {
	return codexProvider
}

// DisplayName 返回提供者的显示名称
func (p *CodexProvider) DisplayName() string {
	return "Codex"
}

// SupportsOAuth 表示该提供者支持 OAuth 授权码流程
func (p *CodexProvider) SupportsOAuth() bool {
	return true
}

// SupportsDevice 表示该提供者支持 Device Code 流程
func (p *CodexProvider) SupportsDevice() bool {
	return true
}

// RefreshLead 返回令牌刷新的提前量（5天），
// 表示在令牌过期前 5 天就开始尝试刷新
func (p *CodexProvider) RefreshLead() *time.Duration {
	lead := 5 * 24 * time.Hour
	return &lead
}

// StartOAuth 启动 OAuth 授权码流程。
// 生成 state、PKCE verifier/challenge，构建授权 URL，并保存登录会话。
//
// 参数：
//   - ctx: 请求上下文
//   - group: 账号池分组信息（本方法未使用）
//   - req: 登录请求，包含分组 ID、名称、选项等
//
// 返回：
//   - *LoginStartResult: 包含会话 ID、授权 URL、过期时间等
//   - error: 错误信息
func (p *CodexProvider) StartOAuth(ctx context.Context, group *model.AccountPoolGroup, req LoginStartRequest) (*LoginStartResult, error) {
	_ = ctx
	_ = group
	// 生成随机 state 参数防止 CSRF 攻击
	state, err := randomHex(16)
	if err != nil {
		return nil, err
	}
	// 生成 PKCE code_verifier 和 code_challenge 用于安全验证
	verifier, challenge, err := generateCodexPKCEPair()
	if err != nil {
		return nil, err
	}
	// 构建完整的 OAuth 授权 URL
	authorizeURL, err := buildCodexAuthorizeURL(state, challenge)
	if err != nil {
		return nil, err
	}
	// 保存登录会话到持久化存储
	session, err := SaveLoginSession(&LoginSession{
		Provider:     p.Name(),
		Mode:         "oauth",
		PoolGroupID:  req.PoolGroupID,
		Name:         strings.TrimSpace(req.Name),
		Options:      req.Options,
		State:        state,
		Verifier:     verifier,
		Challenge:    challenge,
		AuthorizeURL: authorizeURL,
		ExpiresAt:    time.Now().Add(defaultLoginSessionTTL),
	})
	if err != nil {
		return nil, err
	}
	return &LoginStartResult{
		SessionID:    session.SessionID,
		Provider:     p.Name(),
		Mode:         "oauth",
		AuthorizeURL: authorizeURL,
		ExpiresAt:    session.ExpiresAt.Unix(),
	}, nil
}

// CompleteOAuth 完成 OAuth 授权码流程。
// 从回调中解析 code 和 state，验证会话状态后交换 token，构建凭证。
//
// 参数：
//   - ctx: 请求上下文
//   - group: 账号池分组信息（本方法未使用）
//   - req: 登录完成请求，包含会话 ID、回调输入等
//
// 返回：
//   - *AccountCredential: 构建好的账号凭证
//   - error: 错误信息
func (p *CodexProvider) CompleteOAuth(ctx context.Context, group *model.AccountPoolGroup, req LoginCompleteRequest) (*AccountCredential, error) {
	_ = group
	// 从回调输入中解析 authorization code 和 state
	code, state, err := parseOAuthCallbackInput(req.Input)
	if err != nil {
		return nil, err
	}
	// 尝试通过会话 ID 获取登录会话
	session, ok := GetLoginSession(req.SessionID)
	if !ok {
		// 会话 ID 无效时，尝试通过 state 反查 OAuth 会话
		session, ok = FindOAuthLoginSession(p.Name(), req.PoolGroupID, state)
	}
	if !ok || session == nil {
		return nil, fmt.Errorf("oauth flow not started or session expired")
	}
	if session.Status == LoginSessionCancelled {
		return nil, fmt.Errorf("oauth flow cancelled")
	}
	// 验证 state 参数是否匹配（防止 CSRF 攻击）
	if state != session.State {
		return nil, fmt.Errorf("state mismatch")
	}
	// 获取代理配置，优先使用请求中的，其次使用会话中的
	proxy := strings.TrimSpace(req.Options.Proxy)
	if proxy == "" {
		proxy = strings.TrimSpace(session.Options.Proxy)
	}
	// 使用授权码和 PKCE verifier 交换 token
	token, err := exchangeCodexAuthorizationCode(ctx, code, session.Verifier, codexOAuthRedirectURI, proxy)
	if err != nil {
		session.Status = LoginSessionFailed
		session.StatusMessage = err.Error()
		UpdateLoginSession(session)
		return nil, err
	}
	// 构建账号凭证对象
	credential, err := p.buildCredential(req.Name, proxy, token)
	if err != nil {
		return nil, err
	}
	// 更新会话状态为已完成
	session.Status = LoginSessionCompleted
	session.Account = credential
	UpdateLoginSession(session)
	return credential, nil
}

// StartDevice 启动 Device Code 流程。
// 向 Codex 服务器请求 device_code 和 user_code，并保存登录会话。
//
// 参数：
//   - ctx: 请求上下文
//   - group: 账号池分组信息（本方法未使用）
//   - req: 登录请求，包含代理配置等
//
// 返回：
//   - *LoginStartResult: 包含会话 ID、验证 URL、user_code、轮询间隔等
//   - error: 错误信息
func (p *CodexProvider) StartDevice(ctx context.Context, group *model.AccountPoolGroup, req LoginStartRequest) (*LoginStartResult, error) {
	_ = group
	// 根据代理配置创建 HTTP 客户端
	proxyURL := strings.TrimSpace(req.Options.Proxy)
	client, err := httpClientWithProxy(proxyURL)
	if err != nil {
		return nil, err
	}
	// 向服务器请求 device_code 和 user_code
	userCode, err := requestCodexDeviceUserCode(ctx, client)
	if err != nil {
		return nil, err
	}
	// 优先使用 user_code，若为空则使用备选字段 usercode
	userCodeText := strings.TrimSpace(userCode.UserCode)
	if userCodeText == "" {
		userCodeText = strings.TrimSpace(userCode.UserCodeAlt)
	}
	if strings.TrimSpace(userCode.DeviceAuthID) == "" || userCodeText == "" {
		return nil, fmt.Errorf("codex device flow did not return required fields")
	}
	// 解析服务器建议的轮询间隔
	interval := parseCodexDevicePollInterval(userCode.Interval)
	// 保存 Device 流程登录会话
	session, err := SaveLoginSession(&LoginSession{
		Provider:        p.Name(),
		Mode:            "device",
		PoolGroupID:     req.PoolGroupID,
		Name:            strings.TrimSpace(req.Name),
		Options:         req.Options,
		DeviceAuthID:    strings.TrimSpace(userCode.DeviceAuthID),
		UserCode:        userCodeText,
		VerificationURL: codexDeviceVerificationURL,
		ExpiresAt:       time.Now().Add(codexDeviceSessionExpiration),
		PollInterval:    interval,
	})
	if err != nil {
		return nil, err
	}
	return &LoginStartResult{
		SessionID:       session.SessionID,
		Provider:        p.Name(),
		Mode:            "device",
		VerificationURL: codexDeviceVerificationURL,
		UserCode:        userCodeText,
		ExpiresAt:       session.ExpiresAt.Unix(),
		PollInterval:    int64(interval.Seconds()),
	}, nil
}

// CompleteDevice 完成 Device Code 流程。
// 轮询服务器等待用户授权完成，获取授权码后交换 token。
//
// 参数：
//   - ctx: 请求上下文
//   - group: 账号池分组信息（本方法未使用）
//   - req: 登录完成请求
//
// 返回：
//   - *AccountCredential: 构建好的账号凭证
//   - error: 错误信息
func (p *CodexProvider) CompleteDevice(ctx context.Context, group *model.AccountPoolGroup, req LoginCompleteRequest) (*AccountCredential, error) {
	_ = group
	// 获取并验证登录会话
	session, ok := GetLoginSession(req.SessionID)
	if !ok || session == nil {
		return nil, fmt.Errorf("device flow not started or session expired")
	}
	if session.Provider != p.Name() || session.Mode != "device" {
		return nil, fmt.Errorf("login session is not a codex device flow")
	}
	if session.Status == LoginSessionCancelled {
		return nil, fmt.Errorf("device flow cancelled")
	}
	// 获取代理配置
	proxyURL := strings.TrimSpace(req.Options.Proxy)
	if proxyURL == "" {
		proxyURL = strings.TrimSpace(session.Options.Proxy)
	}
	client, err := httpClientWithProxy(proxyURL)
	if err != nil {
		return nil, err
	}
	// 轮询设备 token 端点，等待用户完成授权
	deviceToken, err := pollCodexDeviceToken(ctx, client, session.DeviceAuthID, session.UserCode, session.PollInterval, session.ExpiresAt)
	if err != nil {
		session.Status = LoginSessionFailed
		session.StatusMessage = err.Error()
		UpdateLoginSession(session)
		return nil, err
	}
	// 使用获取到的授权码和 code_verifier 交换正式 token
	token, err := exchangeCodexAuthorizationCodeWithPKCE(ctx, deviceToken.AuthorizationCode, deviceToken.CodeVerifier, codexDeviceRedirectURI, proxyURL)
	if err != nil {
		session.Status = LoginSessionFailed
		session.StatusMessage = err.Error()
		UpdateLoginSession(session)
		return nil, err
	}
	// 构建账号凭证
	credential, err := p.buildCredential(req.Name, proxyURL, token)
	if err != nil {
		return nil, err
	}
	session.Status = LoginSessionCompleted
	session.Account = credential
	UpdateLoginSession(session)
	return credential, nil
}

// Refresh 使用 refresh_token 刷新 OAuth 令牌。
// 解密现有凭证，提取 refresh_token，请求新的 token 对。
//
// 参数：
//   - ctx: 请求上下文
//   - account: 账号池账号对象
//
// 返回：
//   - *AccountCredential: 刷新后的账号凭证
//   - error: 错误信息
func (p *CodexProvider) Refresh(ctx context.Context, account *model.PoolAccount) (*AccountCredential, error) {
	if account == nil {
		return nil, fmt.Errorf("account is required")
	}
	// 解密账号凭证
	raw, err := account.GetDecryptedCredentials()
	if err != nil {
		return nil, err
	}
	oauthKey, err := parseCodexOAuthKey(raw)
	if err != nil {
		return nil, fmt.Errorf("codex oauth credential is invalid")
	}
	if strings.TrimSpace(oauthKey.RefreshToken) == "" {
		return nil, fmt.Errorf("refresh_token is required")
	}
	// 使用 refresh_token 请求新的 token 对
	token, err := refreshCodexOAuthToken(ctx, oauthKey.RefreshToken, account.Proxy)
	if err != nil {
		return nil, err
	}
	return p.buildCredential(account.GetCredentialLabel(), account.Proxy, token)
}

// BuildChannelKey 从账号凭证中提取用于渠道认证的密钥。
// 直接返回解密后的原始凭证 JSON 字符串。
//
// 参数：
//   - account: 账号池账号对象
//
// 返回：
//   - string: 渠道密钥（凭证 JSON）
//   - error: 错误信息
func (p *CodexProvider) BuildChannelKey(account *model.PoolAccount) (string, error) {
	if account == nil {
		return "", fmt.Errorf("account is required")
	}
	raw, err := account.GetDecryptedCredentials()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(raw) == "" {
		return "", fmt.Errorf("codex account credential is empty")
	}
	oauthKey, err := parseCodexOAuthKey(raw)
	if err != nil {
		return "", fmt.Errorf("codex channel credential is invalid")
	}
	if strings.TrimSpace(oauthKey.AccessToken) == "" {
		return "", fmt.Errorf("codex channel: access_token is required")
	}
	if strings.TrimSpace(oauthKey.AccountID) == "" {
		return "", fmt.Errorf("codex channel: account_id is required")
	}
	// Sub2api 导出的 access-token-only 凭据会带有大量额外字段和数字型过期时间。
	// 这里统一压缩为 Codex relay adaptor 可解析的最小 OAuth JSON，避免热路径被无关字段类型绊倒。
	data, err := common.Marshal(oauthKey)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Summarize 将原始凭证转换为简短的摘要字符串，用于界面展示
func (p *CodexProvider) Summarize(raw string) string {
	return model.NormalizeAccountPoolCredentialSummary(raw)
}

// parseCodexOAuthKey 以宽松方式解析 Codex OAuth 凭据。
// 原生 OAuth 登录写入的是字符串型 expired，而 Sub2api 导出的 access-token-only
// 凭据常把 expired/expires_at 写成 Unix 数字，并且可能只提供 access_token 而没有
// refresh_token。直接反序列化到 codexOAuthKey 会因为字段类型不一致失败，因此这里先
// 解析为 map，再提取热路径真正需要的字段。
func parseCodexOAuthKey(raw string) (*codexOAuthKey, error) {
	var payload map[string]any
	if err := common.UnmarshalJsonStr(raw, &payload); err != nil {
		return nil, err
	}
	key := &codexOAuthKey{
		IDToken:      readCodexOAuthString(payload, "id_token", "idToken"),
		AccessToken:  readCodexOAuthString(payload, "access_token", "accessToken"),
		RefreshToken: readCodexOAuthString(payload, "refresh_token", "refreshToken"),
		AccountID:    readCodexOAuthString(payload, "account_id", "accountId", "chatgpt_account_id"),
		LastRefresh:  readCodexOAuthString(payload, "last_refresh", "lastRefresh", "last_refreshed_at", "lastRefreshedAt"),
		Email:        readCodexOAuthString(payload, "email"),
		Type:         firstNonEmptyCodexOAuthString(readCodexOAuthString(payload, "type"), readCodexOAuthString(payload, "provider"), readCodexOAuthString(payload, "platform")),
		Expired:      readCodexOAuthString(payload, "expired", "expires_at", "expiresAt", "expiry", "expires"),
	}
	if strings.TrimSpace(key.AccountID) == "" {
		if accountID, ok := extractCodexAccountIDFromJWT(key.AccessToken); ok {
			key.AccountID = accountID
		}
	}
	if strings.TrimSpace(key.Email) == "" {
		if email, ok := extractEmailFromJWT(key.AccessToken); ok {
			key.Email = email
		}
	}
	if strings.TrimSpace(key.Type) == "" {
		key.Type = codexProvider
	}
	return key, nil
}

// readCodexOAuthString 从 Codex OAuth 凭据中读取字符串字段。
// 外部导入来源可能使用不同字段名，也可能把过期时间写成数字；这里统一转成字符串，
// 让 refresh 和 relay key 构造共享同一套兼容规则。
func readCodexOAuthString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case string:
			if trimmed := strings.TrimSpace(typed); trimmed != "" {
				return trimmed
			}
		case fmt.Stringer:
			if trimmed := strings.TrimSpace(typed.String()); trimmed != "" {
				return trimmed
			}
		case float64:
			if typed > 0 {
				return strconv.FormatInt(int64(typed), 10)
			}
		case int:
			if typed > 0 {
				return strconv.Itoa(typed)
			}
		case int64:
			if typed > 0 {
				return strconv.FormatInt(typed, 10)
			}
		case bool:
			return strconv.FormatBool(typed)
		default:
			text := strings.TrimSpace(fmt.Sprintf("%v", typed))
			if text != "" && text != "<nil>" {
				return text
			}
		}
	}
	return ""
}

// firstNonEmptyCodexOAuthString 返回第一个非空字符串。
// 用于在 type、provider、platform 等等价字段之间选择稳定的凭据类型。
func firstNonEmptyCodexOAuthString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// buildCredential 根据 token 交换结果构建完整的账号凭证对象。
// 从 JWT 中提取 account_id 和 email，组装凭证数据并设置刷新时间。
//
// 参数：
//   - name: 用户自定义的凭证名称
//   - proxy: 代理地址
//   - token: OAuth token 交换结果
//
// 返回：
//   - *AccountCredential: 构建好的账号凭证
//   - error: 错误信息
func (p *CodexProvider) buildCredential(name string, proxy string, token *codexTokenResult) (*AccountCredential, error) {
	if token == nil {
		return nil, fmt.Errorf("token result is empty")
	}
	// 从 access_token 的 JWT payload 中提取 ChatGPT account_id
	accountID, ok := extractCodexAccountIDFromJWT(token.AccessToken)
	if !ok {
		return nil, fmt.Errorf("failed to extract account_id from access_token")
	}
	// 从 JWT 中提取用户邮箱（可能为空）
	email, _ := extractEmailFromJWT(token.AccessToken)
	now := time.Now()
	// 构建 OAuth 凭证存储结构
	oauthKey := codexOAuthKey{
		IDToken:      token.IDToken,
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		AccountID:    accountID,
		LastRefresh:  now.Format(time.RFC3339),
		Expired:      token.ExpiresAt.Format(time.RFC3339),
		Email:        email,
		Type:         codexProvider,
	}
	data, err := common.Marshal(oauthKey)
	if err != nil {
		return nil, err
	}
	// 确定凭证标签：优先使用用户指定名称，其次邮箱，最后 account_id
	label := strings.TrimSpace(name)
	if label == "" {
		label = email
	}
	if label == "" {
		label = accountID
	}
	// 构建元数据和属性映射
	metadata := map[string]any{
		"email":      email,
		"account_id": accountID,
		"expired":    token.ExpiresAt.Format(time.RFC3339),
	}
	attrs := map[string]string{
		"account_id": accountID,
	}
	return &AccountCredential{
		Provider:        codexProvider,
		AuthType:        model.AccountPoolAuthTypeOfficialOAuth,
		Label:           label,
		Credentials:     string(data),
		Summary:         model.NormalizeAccountPoolCredentialSummary(string(data)),
		Metadata:        metadata,
		Attributes:      attrs,
		ExpiresAt:       token.ExpiresAt,
		LastRefreshedAt: now,
		NextRefreshAt:   nextCodexRefreshAt(now, token.ExpiresAt),
	}, nil
}

// nextCodexRefreshAt 计算下次刷新时间。
// 策略：在令牌过期前 5 分钟刷新，但至少间隔 1 分钟。
func nextCodexRefreshAt(now time.Time, expiresAt time.Time) time.Time {
	if expiresAt.IsZero() {
		return now.Add(10 * time.Minute)
	}
	next := expiresAt.Add(-5 * time.Minute)
	minNext := now.Add(time.Minute)
	if next.Before(minNext) {
		return minNext
	}
	return next
}

// codexDeviceUserCodeResponse 表示 Device Code 流程中服务器返回的 user_code 响应
type codexDeviceUserCodeResponse struct {
	DeviceAuthID string `json:"device_auth_id"` // 设备授权会话 ID
	UserCode     string `json:"user_code"`      // 用户需要输入的验证码
	UserCodeAlt  string `json:"usercode"`       // 备选字段名（兼容不同 API 版本）
	Interval     any    `json:"interval"`       // 建议的轮询间隔（秒）
}

// codexDeviceTokenResponse 表示 Device Code 流程中服务器返回的 token 响应
type codexDeviceTokenResponse struct {
	AuthorizationCode string `json:"authorization_code"` // 授权码
	CodeVerifier      string `json:"code_verifier"`      // PKCE code_verifier
	CodeChallenge     string `json:"code_challenge"`     // PKCE code_challenge
}

// requestCodexDeviceUserCode 向 Codex 服务器请求 Device Code 流程的 user_code。
//
// 参数：
//   - ctx: 请求上下文
//   - client: 带代理配置的 HTTP 客户端
//
// 返回：
//   - *codexDeviceUserCodeResponse: 包含 device_auth_id 和 user_code
//   - error: 错误信息
func requestCodexDeviceUserCode(ctx context.Context, client *http.Client) (*codexDeviceUserCodeResponse, error) {
	// 构建请求体，仅需 client_id
	body, err := common.Marshal(map[string]string{"client_id": codexOAuthClientID})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, codexDeviceUserCodeURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to request codex device code: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	// 非 2xx 状态码视为请求失败
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("codex device code request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	var parsed codexDeviceUserCodeResponse
	if err := common.Unmarshal(respBody, &parsed); err != nil {
		return nil, err
	}
	return &parsed, nil
}

// pollCodexDeviceToken 轮询 Codex 服务器等待用户完成 Device Code 授权。
// 按固定间隔反复请求 token 端点，直到成功、超时或遇到不可重试的错误。
//
// 参数：
//   - ctx: 请求上下文（用于取消轮询）
//   - client: 带代理配置的 HTTP 客户端
//   - deviceAuthID: 设备授权会话 ID
//   - userCode: 用户验证码
//   - interval: 轮询间隔
//   - expiresAt: 轮询截止时间
//
// 返回：
//   - *codexDeviceTokenResponse: 包含授权码和 PKCE verifier
//   - error: 错误信息
func pollCodexDeviceToken(ctx context.Context, client *http.Client, deviceAuthID string, userCode string, interval time.Duration, expiresAt time.Time) (*codexDeviceTokenResponse, error) {
	if interval <= 0 {
		interval = codexDeviceDefaultInterval
	}
	if expiresAt.IsZero() {
		expiresAt = time.Now().Add(codexDeviceSessionExpiration)
	}
	// 持续轮询直到成功或超时
	for {
		if time.Now().After(expiresAt) {
			return nil, fmt.Errorf("codex device authentication timed out")
		}
		// 构建轮询请求体
		body, err := common.Marshal(map[string]string{
			"device_auth_id": deviceAuthID,
			"user_code":      userCode,
		})
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, codexDeviceTokenURL, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to poll codex device token: %w", err)
		}
		respBody, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		// 2xx 表示用户已完成授权
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			var parsed codexDeviceTokenResponse
			if err := common.Unmarshal(respBody, &parsed); err != nil {
				return nil, err
			}
			if strings.TrimSpace(parsed.AuthorizationCode) == "" || strings.TrimSpace(parsed.CodeVerifier) == "" {
				return nil, fmt.Errorf("codex device token response missing required fields")
			}
			return &parsed, nil
		}
		// 403/404 表示用户尚未完成授权，继续轮询；其他状态码视为错误
		if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusNotFound {
			return nil, fmt.Errorf("codex device token polling failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
		}
		// 等待一个轮询间隔后重试
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
	}
}

// exchangeCodexAuthorizationCode 使用授权码交换 token（委托给 PKCE 版本）
func exchangeCodexAuthorizationCode(ctx context.Context, code string, verifier string, redirectURI string, proxyURL string) (*codexTokenResult, error) {
	return exchangeCodexAuthorizationCodeWithPKCE(ctx, code, verifier, redirectURI, proxyURL)
}

// exchangeCodexAuthorizationCodeWithPKCE 使用授权码 + PKCE code_verifier 交换 token。
//
// 参数：
//   - ctx: 请求上下文
//   - code: OAuth 授权码
//   - verifier: PKCE code_verifier
//   - redirectURI: 回调地址（需与授权时一致）
//   - proxyURL: 代理地址
//
// 返回：
//   - *codexTokenResult: 包含 access_token、refresh_token 等
//   - error: 错误信息
func exchangeCodexAuthorizationCodeWithPKCE(ctx context.Context, code string, verifier string, redirectURI string, proxyURL string) (*codexTokenResult, error) {
	if strings.TrimSpace(code) == "" {
		return nil, errors.New("empty authorization code")
	}
	if strings.TrimSpace(verifier) == "" {
		return nil, errors.New("empty code_verifier")
	}
	client, err := httpClientWithProxy(proxyURL)
	if err != nil {
		return nil, err
	}
	// 构建 token 交换的表单参数
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", codexOAuthClientID)
	form.Set("code", strings.TrimSpace(code))
	form.Set("code_verifier", strings.TrimSpace(verifier))
	form.Set("redirect_uri", strings.TrimSpace(redirectURI))
	return requestCodexToken(ctx, client, form, "codex oauth code exchange failed")
}

// refreshCodexOAuthToken 使用 refresh_token 刷新 OAuth token。
//
// 参数：
//   - ctx: 请求上下文
//   - refreshToken: 当前的 refresh_token
//   - proxyURL: 代理地址
//
// 返回：
//   - *codexTokenResult: 刷新后的 token 对
//   - error: 错误信息
func refreshCodexOAuthToken(ctx context.Context, refreshToken string, proxyURL string) (*codexTokenResult, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return nil, errors.New("empty refresh_token")
	}
	client, err := httpClientWithProxy(proxyURL)
	if err != nil {
		return nil, err
	}
	// 构建 token 刷新的表单参数
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", strings.TrimSpace(refreshToken))
	form.Set("client_id", codexOAuthClientID)
	form.Set("scope", "openid profile email")
	return requestCodexToken(ctx, client, form, "codex oauth refresh failed")
}

// requestCodexToken 向 Codex OAuth Token 端点发送 token 请求（统一处理 code exchange 和 refresh）。
//
// 参数：
//   - ctx: 请求上下文
//   - client: HTTP 客户端
//   - form: 表单参数
//   - errorPrefix: 错误信息前缀
//
// 返回：
//   - *codexTokenResult: 解析后的 token 结果
//   - error: 错误信息
func requestCodexToken(ctx context.Context, client *http.Client, form url.Values, errorPrefix string) (*codexTokenResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, codexOAuthTokenURL, strings.NewReader(form.Encode()))
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
		IDToken      string `json:"id_token"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := common.DecodeJson(resp.Body, &payload); err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s: status=%d", errorPrefix, resp.StatusCode)
	}
	// 验证响应中包含必要的字段
	if strings.TrimSpace(payload.AccessToken) == "" || strings.TrimSpace(payload.RefreshToken) == "" || payload.ExpiresIn <= 0 {
		return nil, errors.New("codex oauth token response missing fields")
	}
	return &codexTokenResult{
		IDToken:      strings.TrimSpace(payload.IDToken),
		AccessToken:  strings.TrimSpace(payload.AccessToken),
		RefreshToken: strings.TrimSpace(payload.RefreshToken),
		// 根据 expires_in 计算绝对过期时间
		ExpiresAt: time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second),
	}, nil
}

// buildCodexAuthorizeURL 构建 OAuth 授权页面的完整 URL。
// 包含 PKCE challenge、state、scope 等参数。
//
// 参数：
//   - state: 防 CSRF 的随机状态值
//   - challenge: PKCE code_challenge（S256 编码后）
//
// 返回：
//   - string: 完整的授权 URL
//   - error: URL 解析错误
func buildCodexAuthorizeURL(state string, challenge string) (string, error) {
	u, err := url.Parse(codexOAuthAuthorizeURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("response_type", "code")               // 授权码模式
	q.Set("client_id", codexOAuthClientID)       // 客户端 ID
	q.Set("redirect_uri", codexOAuthRedirectURI) // 回调地址
	q.Set("scope", codexOAuthScope)              // 权限范围
	q.Set("code_challenge", challenge)           // PKCE 挑战码
	q.Set("code_challenge_method", "S256")       // PKCE 编码方式
	q.Set("state", state)                        // 防 CSRF 状态值
	q.Set("id_token_add_organizations", "true")  // 请求获取组织信息
	q.Set("codex_cli_simplified_flow", "true")   // Codex CLI 简化流程标志
	q.Set("originator", "codex_cli_rs")          // 来源标识
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// generateCodexPKCEPair 生成 PKCE (Proof Key for Code Exchange) 的 verifier/challenge 对。
// verifier 是 32 字节随机数的 Base64URL 编码，challenge 是其 SHA256 哈希的 Base64URL 编码。
//
// 返回：
//   - string: code_verifier
//   - string: code_challenge
//   - error: 随机数生成错误
func generateCodexPKCEPair() (string, string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	verifier := base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

// parseOAuthCallbackInput 从 OAuth 回调输入中解析 code 和 state。
// 支持完整 URL 和纯查询字符串两种格式。
//
// 参数：
//   - input: 回调输入（URL 或查询字符串）
//
// 返回：
//   - string: authorization code
//   - string: state 参数
//   - error: 解析错误
func parseOAuthCallbackInput(input string) (string, string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", fmt.Errorf("callback input is required")
	}
	// 输入为完整 URL 时，从 query 参数中提取
	if strings.Contains(input, "://") {
		u, err := url.Parse(input)
		if err != nil {
			return "", "", err
		}
		code := strings.TrimSpace(u.Query().Get("code"))
		state := strings.TrimSpace(u.Query().Get("state"))
		if code == "" || state == "" {
			return "", "", fmt.Errorf("callback url missing code or state")
		}
		return code, state, nil
	}
	// 尝试作为纯查询字符串解析
	values, err := url.ParseQuery(input)
	if err == nil {
		code := strings.TrimSpace(values.Get("code"))
		state := strings.TrimSpace(values.Get("state"))
		if code != "" && state != "" {
			return code, state, nil
		}
	}
	return "", "", fmt.Errorf("callback input missing code or state")
}

// parseCodexDevicePollInterval 将服务器返回的轮询间隔值解析为 time.Duration。
// 支持 string、float64、int 三种类型，无效值返回默认间隔。
func parseCodexDevicePollInterval(raw any) time.Duration {
	switch value := raw.(type) {
	case string:
		seconds, err := strconv.Atoi(strings.TrimSpace(value))
		if err == nil && seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
	case float64:
		if value > 0 {
			return time.Duration(value) * time.Second
		}
	case int:
		if value > 0 {
			return time.Duration(value) * time.Second
		}
	}
	return codexDeviceDefaultInterval
}

// extractCodexAccountIDFromJWT 从 access_token 的 JWT payload 中提取 ChatGPT 账号 ID。
// 路径为 claims["https://api.openai.com/auth"]["chatgpt_account_id"]。
//
// 参数：
//   - token: JWT 格式的 access_token
//
// 返回：
//   - string: ChatGPT account_id
//   - bool: 是否成功提取
func extractCodexAccountIDFromJWT(token string) (string, bool) {
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
	value, ok := obj["chatgpt_account_id"].(string)
	if !ok {
		return "", false
	}
	value = strings.TrimSpace(value)
	return value, value != ""
}

// extractEmailFromJWT 从 JWT payload 中提取用户邮箱（claims["email"]）。
//
// 参数：
//   - token: JWT 格式的令牌
//
// 返回：
//   - string: 用户邮箱
//   - bool: 是否成功提取
func extractEmailFromJWT(token string) (string, bool) {
	claims, ok := decodeJWTClaims(token)
	if !ok {
		return "", false
	}
	value, ok := claims["email"].(string)
	if !ok {
		return "", false
	}
	value = strings.TrimSpace(value)
	return value, value != ""
}

// decodeJWTClaims 解码 JWT 的 payload 部分为 JSON map。
// JWT 格式为 header.payload.signature，此处提取中间的 payload 部分。
//
// 参数：
//   - token: JWT 字符串
//
// 返回：
//   - map[string]any: 解码后的 claims
//   - bool: 是否成功解码
func decodeJWTClaims(token string) (map[string]any, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, false
	}
	// Base64URL 解码 payload 部分
	payloadRaw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, false
	}
	var claims map[string]any
	if err := common.Unmarshal(payloadRaw, &claims); err != nil {
		return nil, false
	}
	return claims, true
}
