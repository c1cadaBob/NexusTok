package common

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

var SessionCookieSecure = false       // 会话 Cookie 是否只允许通过 HTTPS 发送
var SessionCookieTrustedURLs []string // 启用 Secure Cookie 时声明的可信 HTTPS 访问地址

// InitSessionCookieSettings 初始化会话 Cookie 的安全配置。
// 默认保持 Secure=false，兼容 HTTP 本地开发和当前热更新容器；只有显式设置
// SESSION_COOKIE_SECURE=true 时才启用 HTTPS-only Cookie，并要求提供可信 HTTPS URL。
func InitSessionCookieSettings() error {
	secureRaw := strings.TrimSpace(os.Getenv("SESSION_COOKIE_SECURE"))
	trustedURLsRaw := strings.TrimSpace(os.Getenv("SESSION_COOKIE_TRUSTED_URL"))

	SessionCookieSecure = false
	SessionCookieTrustedURLs = nil

	if secureRaw == "" || strings.EqualFold(secureRaw, "false") {
		if trustedURLsRaw != "" {
			return fmt.Errorf("SESSION_COOKIE_TRUSTED_URL requires SESSION_COOKIE_SECURE=true")
		}
		return nil
	}

	if !strings.EqualFold(secureRaw, "true") {
		return fmt.Errorf("SESSION_COOKIE_SECURE must be true or false")
	}

	if trustedURLsRaw == "" {
		return fmt.Errorf("SESSION_COOKIE_SECURE=true requires SESSION_COOKIE_TRUSTED_URL")
	}

	for _, trustedURL := range strings.Split(trustedURLsRaw, ",") {
		trustedURL = strings.TrimSpace(trustedURL)
		if trustedURL == "" {
			return fmt.Errorf("SESSION_COOKIE_TRUSTED_URL contains an empty URL")
		}
		parsedURL, err := url.Parse(trustedURL)
		if err != nil {
			return fmt.Errorf("invalid SESSION_COOKIE_TRUSTED_URL: %w", err)
		}
		if parsedURL.Scheme != "https" || parsedURL.Host == "" {
			return fmt.Errorf("SESSION_COOKIE_TRUSTED_URL must contain only https URLs with hosts")
		}
		SessionCookieTrustedURLs = append(SessionCookieTrustedURLs, trustedURL)
	}

	SessionCookieSecure = true
	return nil
}
