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

// PrepareImportedCredential 将 Cookie/Access Token 导入请求转换成内部可复用登录态。
//
// 第一版只导入目标 new-api/sub2api 站点已经签发的登录态，不模拟 GitHub、LinuxDO/L 站
// 等第三方 OAuth 登录流程。这样可以避开验证码、人机验证、回调域名差异和站点魔改，
// 同时仍能让管理员把第三方注册的上游账号同步进 NexusTok。
func PrepareImportedCredential(credential Credential) (Credential, error) {
	credential.Platform = NormalizePlatform(credential.Platform)
	credential.AuthMode = NormalizeAuthMode(credential.AuthMode)
	if credential.AuthMode == "" {
		credential.AuthMode = AuthModePassword
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
	now := common.GetTimestamp()
	credential.Password = ""
	credential.Session = &AuthenticatedSession{
		Platform:   PlatformNewAPI,
		BaseURL:    credential.BaseURL,
		AuthMode:   AuthModeSessionCookie,
		ImportedAt: now,
		UpdatedAt:  now,
		NewAPI: &NewAPISessionData{
			UserID:  strings.TrimSpace(firstNonEmpty(credential.UserID, credential.Username)),
			Cookies: cookies,
		},
	}
	return credential, nil
}

func prepareAccessTokenCredential(credential Credential) (Credential, error) {
	if credential.Platform != PlatformSub2API {
		return credential, fmt.Errorf("Access Token 导入目前用于 sub2api 平台；new-api 请使用 Session/Cookie 导入")
	}
	if strings.TrimSpace(credential.BaseURL) == "" {
		return credential, fmt.Errorf("上游平台地址不能为空")
	}
	accessToken := strings.TrimSpace(credential.AccessToken)
	if accessToken == "" {
		return credential, fmt.Errorf("目标站 Access Token 不能为空")
	}
	now := common.GetTimestamp()
	credential.Password = ""
	credential.Session = &AuthenticatedSession{
		Platform:   PlatformSub2API,
		BaseURL:    credential.BaseURL,
		AuthMode:   AuthModeAccessToken,
		ImportedAt: now,
		UpdatedAt:  now,
		Sub2API: &Sub2APISessionData{
			AccessToken:  accessToken,
			RefreshToken: strings.TrimSpace(credential.RefreshToken),
			ExpiresAt:    credential.ExpiresAt,
		},
	}
	return credential, nil
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
	return nil, fmt.Errorf("暂不支持自动 OAuth 登录，请先打开目标站完成第三方登录，再使用 Session/Cookie 或 Access Token 导入")
}

// CompleteBrowserAuth 预留目标站浏览器 OAuth 自动化回调入口。
func CompleteBrowserAuth(_ context.Context, req BrowserAuthRequest) (*BrowserAuthResult, error) {
	platform := NormalizePlatform(req.Platform)
	if platform == "" {
		return nil, fmt.Errorf("上游平台不能为空")
	}
	return nil, fmt.Errorf("暂不支持自动 OAuth 登录，请先打开目标站完成第三方登录，再使用 Session/Cookie 或 Access Token 导入")
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
