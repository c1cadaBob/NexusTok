package upstreamaccount

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/c1cada/NexusTok/common"
)

// NormalizeAuthMode 规范化上游账号同步认证方式。
//
// 空值按 password 处理，用于兼容旧前端和外部脚本。这里也接受少量常见别名，避免
// 管理员或 API 调用方从 UI 文案推导字段值时因为横线/下划线差异导致同步失败。
func NormalizeAuthMode(mode string) string {
	normalized := strings.ToLower(strings.TrimSpace(mode))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	switch normalized {
	case "", "password", "account_password", "account":
		return AuthModePassword
	case "session", "cookie", "session_cookie":
		return AuthModeSessionCookie
	case "token", "access_token", "bearer":
		return AuthModeAccessToken
	case "oauth", "oauth_browser", "browser_auth":
		return AuthModeOAuthBrowser
	default:
		return normalized
	}
}

// HydrateCredentialFromSession 将已加密保存的登录态回填成同步客户端可直接使用的凭据。
//
// StoredCredential 为了安全不会在顶层保存 Access Token / Refresh Token，真正的明文
// 只存在加密 Session 中。刷新渠道时如果不在后端内存里做这一步，access_token 模式会
// 因顶层 AccessToken 为空而被误判为“未填写 token”。该函数只操作临时 Credential，
// 不会把 AT/RT/Cookie 写回 API 响应。
func HydrateCredentialFromSession(credential Credential) Credential {
	if credential.Session == nil {
		return credential
	}
	session := normalizeAuthenticatedSession(
		firstNonEmpty(credential.Platform, credential.Session.Platform),
		firstNonEmpty(credential.BaseURL, credential.Session.BaseURL),
		credential.Session,
	)
	if session == nil {
		return credential
	}
	credential.Session = session
	if strings.TrimSpace(credential.Platform) == "" {
		credential.Platform = session.Platform
	}
	if strings.TrimSpace(credential.BaseURL) == "" {
		credential.BaseURL = session.BaseURL
	}
	if strings.TrimSpace(credential.ManagementBaseURL) == "" {
		credential.ManagementBaseURL = firstNonEmpty(credential.BaseURL, session.BaseURL)
	}
	if strings.TrimSpace(credential.AuthMode) == "" {
		credential.AuthMode = session.AuthMode
	}
	switch NormalizePlatform(session.Platform) {
	case PlatformNewAPI:
		if session.NewAPI != nil {
			if strings.TrimSpace(credential.UserID) == "" {
				credential.UserID = strings.TrimSpace(session.NewAPI.UserID)
			}
			if strings.TrimSpace(credential.AccessToken) == "" {
				credential.AccessToken = strings.TrimSpace(session.NewAPI.AccessToken)
			}
		}
	case PlatformSub2API:
		if session.Sub2API != nil {
			if strings.TrimSpace(credential.AccessToken) == "" {
				credential.AccessToken = strings.TrimSpace(session.Sub2API.AccessToken)
			}
			if strings.TrimSpace(credential.RefreshToken) == "" {
				credential.RefreshToken = strings.TrimSpace(session.Sub2API.RefreshToken)
			}
			if credential.ExpiresAt <= 0 {
				credential.ExpiresAt = normalizeUnixSeconds(session.Sub2API.ExpiresAt)
			}
		}
	}
	return credential
}

// PrepareImportedCredential 将 Cookie/Access Token 导入请求转换成内部可复用登录态。
//
// 第一版只导入目标 new-api/sub2api 站点已经签发的登录态，不模拟 GitHub、LinuxDO/L 站
// 等第三方 OAuth 登录流程。这样可以避开验证码、人机验证、回调域名差异和站点魔改，
// 同时仍能让管理员把第三方注册的上游账号同步进 NexusTok。
func PrepareImportedCredential(credential Credential) (Credential, error) {
	credential = HydrateCredentialFromSession(credential)
	credential.Platform = NormalizePlatform(credential.Platform)
	credential.AuthMode = NormalizeAuthMode(credential.AuthMode)
	if credential.AuthMode == "" {
		credential.AuthMode = AuthModePassword
	}
	if credential.Platform == PlatformSub2API {
		managementBaseURL := firstNonEmpty(credential.ManagementBaseURL, credential.BaseURL)
		credential.ManagementBaseURL = managementBaseURL
		credential.BaseURL = managementBaseURL
	}

	switch credential.AuthMode {
	case AuthModePassword:
		return credential, nil
	case AuthModeSessionCookie:
		return prepareSessionCookieCredential(credential)
	case AuthModeAccessToken:
		return prepareAccessTokenCredential(credential)
	case AuthModeOAuthBrowser:
		return credential, fmt.Errorf("暂不支持自动 OAuth 登录，请先在目标站完成 GitHub/LinuxDO 登录后导入 Session/Cookie 或 Access Token")
	default:
		return credential, fmt.Errorf("不支持的上游账号认证方式：%s", credential.AuthMode)
	}
}

func prepareSessionCookieCredential(credential Credential) (Credential, error) {
	if credential.Platform != PlatformNewAPI {
		return credential, fmt.Errorf("Session/Cookie 导入目前用于 new-api 平台；sub2api 请使用 Access Token 导入")
	}
	if strings.TrimSpace(credential.BaseURL) == "" {
		return credential, fmt.Errorf("上游平台地址不能为空")
	}
	cookies, err := ParseImportedCookies(credential.SessionCookie)
	if err != nil {
		return credential, err
	}
	if len(cookies) == 0 {
		return credential, fmt.Errorf("目标站 Session/Cookie 不能为空")
	}
	userID := strings.TrimSpace(credential.UserID)
	if userID == "" && isNumericNewAPIUserID(credential.Username) {
		userID = strings.TrimSpace(credential.Username)
	}
	if userID != "" && !isNumericNewAPIUserID(userID) {
		return credential, fmt.Errorf("new-api New-Api-User 必须是目标站数字用户 ID，不能使用用户名或邮箱")
	}
	now := common.GetTimestamp()
	credential.Password = ""
	credential.Session = &AuthenticatedSession{
		Platform:   PlatformNewAPI,
		BaseURL:    credential.BaseURL,
		AuthMode:   AuthModeSessionCookie,
		ImportedAt: now,
		UpdatedAt:  now,
		NewAPI: &NewAPISessionData{
			UserID:  userID,
			Cookies: cookies,
		},
	}
	return credential, nil
}

func prepareAccessTokenCredential(credential Credential) (Credential, error) {
	if strings.TrimSpace(credential.BaseURL) == "" {
		return credential, fmt.Errorf("上游平台地址不能为空")
	}
	accessToken := normalizeImportedBearerToken(credential.AccessToken)
	if accessToken == "" {
		return credential, fmt.Errorf("目标站 Access Token 不能为空")
	}
	now := common.GetTimestamp()
	credential.Password = ""
	switch credential.Platform {
	case PlatformNewAPI:
		userID := strings.TrimSpace(firstNonEmpty(credential.UserID, credential.Username))
		if userID == "" {
			return credential, fmt.Errorf("new-api Access Token 导入必须同时提供 New-Api-User / User ID")
		}
		if !isNumericNewAPIUserID(userID) {
			return credential, fmt.Errorf("new-api New-Api-User 必须是目标站数字用户 ID，不能使用用户名或邮箱")
		}
		credential.Session = &AuthenticatedSession{
			Platform:   PlatformNewAPI,
			BaseURL:    credential.BaseURL,
			AuthMode:   AuthModeAccessToken,
			ImportedAt: now,
			UpdatedAt:  now,
			NewAPI: &NewAPISessionData{
				UserID:      userID,
				AccessToken: accessToken,
			},
		}
	case PlatformSub2API:
		managementBaseURL := firstNonEmpty(credential.ManagementBaseURL, credential.BaseURL)
		credential.Session = &AuthenticatedSession{
			Platform:   PlatformSub2API,
			BaseURL:    managementBaseURL,
			AuthMode:   AuthModeAccessToken,
			ImportedAt: now,
			UpdatedAt:  now,
			Sub2API: &Sub2APISessionData{
				AccessToken:  accessToken,
				RefreshToken: strings.TrimSpace(credential.RefreshToken),
				ExpiresAt:    normalizeUnixSeconds(credential.ExpiresAt),
			},
		}
	default:
		return credential, fmt.Errorf("Access Token 导入目前支持 new-api 和 sub2api 平台")
	}
	return credential, nil
}

// isNumericNewAPIUserID 判断 new-api 管理接口要求的 New-Api-User 是否为数字 ID。
//
// new-api 的 UserAuth 中间件会把 `New-Api-User` 解析为整数，并要求它与当前登录
// session / access token 对应的用户 ID 一致。LinuxDO、GitHub 等第三方登录只影响
// 目标站如何完成登录，不会改变该 header 的语义；因此这里明确拒绝用户名、邮箱和
// linuxdo-connect.invalid 这类第三方登录标识，避免管理员把可读账号名误填成用户 ID。
func isNumericNewAPIUserID(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	for _, r := range trimmed {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func normalizeImportedBearerToken(raw string) string {
	token := strings.TrimSpace(raw)
	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		token = strings.TrimSpace(token[7:])
	}
	return token
}

func normalizeUnixSeconds(value int64) int64 {
	if value > 1_000_000_000_000 {
		return value / 1000
	}
	return value
}

// ParseImportedCookies 支持 Cookie header、JSON cookie 数组和 name/value 映射三种导入格式。
//
// 浏览器开发者工具、Cookie 编辑器和不同目标站的导出格式并不统一。这里接受最常见的
// 三类格式并归一化成 StoredHTTPCookie；无论输入格式如何，后续持久化仍会整体加密。
func ParseImportedCookies(raw string) ([]StoredHTTPCookie, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	if strings.HasPrefix(trimmed, "[") {
		var cookies []StoredHTTPCookie
		if err := common.UnmarshalJsonStr(trimmed, &cookies); err != nil {
			return nil, fmt.Errorf("解析 Cookie JSON 失败：%w", err)
		}
		return normalizeImportedCookies(cookies), nil
	}
	if strings.HasPrefix(trimmed, "{") {
		cookies, err := parseImportedCookieObject(trimmed)
		if err != nil {
			return nil, err
		}
		return normalizeImportedCookies(cookies), nil
	}
	return normalizeImportedCookies(parseCookieHeader(trimmed)), nil
}

func parseImportedCookieObject(raw string) ([]StoredHTTPCookie, error) {
	var wrapped struct {
		Cookies []StoredHTTPCookie `json:"cookies"`
	}
	if err := common.UnmarshalJsonStr(raw, &wrapped); err == nil && len(wrapped.Cookies) > 0 {
		return wrapped.Cookies, nil
	}

	var pairs map[string]string
	if err := common.UnmarshalJsonStr(raw, &pairs); err != nil {
		return nil, fmt.Errorf("解析 Cookie JSON 失败：%w", err)
	}
	cookies := make([]StoredHTTPCookie, 0, len(pairs))
	for name, value := range pairs {
		cookies = append(cookies, StoredHTTPCookie{Name: name, Value: value, Path: "/"})
	}
	return cookies, nil
}

func parseCookieHeader(header string) []StoredHTTPCookie {
	parts := strings.Split(header, ";")
	cookies := make([]StoredHTTPCookie, 0, len(parts))
	for _, part := range parts {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			continue
		}
		cookies = append(cookies, StoredHTTPCookie{
			Name:  name,
			Value: strings.TrimSpace(value),
			Path:  "/",
		})
	}
	return cookies
}

func normalizeImportedCookies(cookies []StoredHTTPCookie) []StoredHTTPCookie {
	result := make([]StoredHTTPCookie, 0, len(cookies))
	seen := map[string]struct{}{}
	for _, cookie := range cookies {
		name := strings.TrimSpace(cookie.Name)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		if cookie.Path == "" {
			cookie.Path = "/"
		}
		cookie.Name = name
		result = append(result, cookie)
	}
	return result
}

func credentialRequiresImportedSession(credential Credential, mode string) bool {
	return NormalizeAuthMode(credential.AuthMode) == mode
}

func credentialNeedsPassword(credential Credential) bool {
	return NormalizeAuthMode(credential.AuthMode) == AuthModePassword &&
		strings.TrimSpace(credential.Password) == "" &&
		!hasReusableAuthSession(credential.Session)
}

// BrowserAuthRequest 是站内 OAuth 自动化预留接口的请求体。
type BrowserAuthRequest struct {
	Platform string `json:"platform"`
	BaseURL  string `json:"base_url"`
}

// BrowserAuthResult 描述当前平台的 OAuth 自动化支持状态。
type BrowserAuthResult struct {
	Supported bool     `json:"supported"`
	Message   string   `json:"message"`
	AuthModes []string `json:"auth_modes"`
}

// StartBrowserAuth 预留目标站浏览器 OAuth 自动化入口。
func StartBrowserAuth(_ context.Context, req BrowserAuthRequest) (*BrowserAuthResult, error) {
	platform := NormalizePlatform(req.Platform)
	if platform == "" {
		return nil, fmt.Errorf("上游平台不能为空")
	}
	return &BrowserAuthResult{
		Supported: false,
		Message:   "请使用 NexusTok 登录态采集助手生成油猴脚本，在目标站登录后由脚本采集并回填登录态",
		AuthModes: []string{AuthModePassword, AuthModeSessionCookie, AuthModeAccessToken, AuthModeOAuthBrowser},
	}, nil
}

// CompleteBrowserAuth 预留目标站浏览器 OAuth 自动化回调入口。
func CompleteBrowserAuth(_ context.Context, req BrowserAuthRequest) (*BrowserAuthResult, error) {
	platform := NormalizePlatform(req.Platform)
	if platform == "" {
		return nil, fmt.Errorf("上游平台不能为空")
	}
	return &BrowserAuthResult{
		Supported: false,
		Message:   "目标站 OAuth 自动回调需要目标站配合；当前请使用油猴脚本采集或手动导入登录态",
		AuthModes: []string{AuthModePassword, AuthModeSessionCookie, AuthModeAccessToken, AuthModeOAuthBrowser},
	}, nil
}

func restoreImportedCookiesToJar(api *httpClient, cookies []StoredHTTPCookie) error {
	if len(cookies) == 0 {
		return fmt.Errorf("目标站 Session/Cookie 不能为空")
	}
	return restoreCookiesToJar(api, cookies)
}

func buildCookieHeader(cookies []StoredHTTPCookie) string {
	values := make([]string, 0, len(cookies))
	for _, cookie := range cookies {
		if strings.TrimSpace(cookie.Name) == "" {
			continue
		}
		values = append(values, (&http.Cookie{Name: cookie.Name, Value: cookie.Value}).String())
	}
	return strings.Join(values, "; ")
}

func cookieOriginURL(baseURL string) *url.URL {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil
	}
	return parsed
}
