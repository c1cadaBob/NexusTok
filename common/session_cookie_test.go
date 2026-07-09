package common

import "testing"

func resetSessionCookieSettingsAfterTest(t *testing.T) {
	t.Helper()
	oldSecure := SessionCookieSecure
	oldTrustedURLs := append([]string(nil), SessionCookieTrustedURLs...)
	t.Cleanup(func() {
		SessionCookieSecure = oldSecure
		SessionCookieTrustedURLs = oldTrustedURLs
	})
}

// TestInitSessionCookieSettingsDefaultsToInsecure 验证默认配置不改变现有 HTTP 开发体验。
func TestInitSessionCookieSettingsDefaultsToInsecure(t *testing.T) {
	resetSessionCookieSettingsAfterTest(t)
	t.Setenv("SESSION_COOKIE_SECURE", "")
	t.Setenv("SESSION_COOKIE_TRUSTED_URL", "")

	if err := InitSessionCookieSettings(); err != nil {
		t.Fatalf("InitSessionCookieSettings() error = %v", err)
	}
	if SessionCookieSecure {
		t.Fatal("SessionCookieSecure = true, want false")
	}
	if len(SessionCookieTrustedURLs) != 0 {
		t.Fatalf("SessionCookieTrustedURLs = %v, want empty", SessionCookieTrustedURLs)
	}
}

// TestInitSessionCookieSettingsRequiresBothEnvVars 验证 Secure Cookie 与可信 URL 必须成对配置。
func TestInitSessionCookieSettingsRequiresBothEnvVars(t *testing.T) {
	tests := []struct {
		name       string
		secure     string
		trustedURL string
	}{
		{name: "secure without trusted url", secure: "true", trustedURL: ""},
		{name: "trusted url without secure", secure: "", trustedURL: "https://example.com"},
		{name: "invalid secure value", secure: "yes", trustedURL: "https://example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetSessionCookieSettingsAfterTest(t)
			t.Setenv("SESSION_COOKIE_SECURE", tt.secure)
			t.Setenv("SESSION_COOKIE_TRUSTED_URL", tt.trustedURL)

			if err := InitSessionCookieSettings(); err == nil {
				t.Fatal("InitSessionCookieSettings() error = nil, want error")
			}
		})
	}
}

// TestInitSessionCookieSettingsRequiresHTTPSURL 验证 trusted URL 只能使用带 host 的 HTTPS 地址。
func TestInitSessionCookieSettingsRequiresHTTPSURL(t *testing.T) {
	tests := []struct {
		name       string
		trustedURL string
	}{
		{name: "http scheme", trustedURL: "http://example.com"},
		{name: "missing host", trustedURL: "https://"},
		{name: "empty item", trustedURL: "https://example.com,"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetSessionCookieSettingsAfterTest(t)
			t.Setenv("SESSION_COOKIE_SECURE", "true")
			t.Setenv("SESSION_COOKIE_TRUSTED_URL", tt.trustedURL)

			if err := InitSessionCookieSettings(); err == nil {
				t.Fatal("InitSessionCookieSettings() error = nil, want error")
			}
		})
	}
}

// TestInitSessionCookieSettingsEnablesSecureCookie 验证 HTTPS trusted URL 可启用 Secure Cookie。
func TestInitSessionCookieSettingsEnablesSecureCookie(t *testing.T) {
	resetSessionCookieSettingsAfterTest(t)
	t.Setenv("SESSION_COOKIE_SECURE", "true")
	t.Setenv("SESSION_COOKIE_TRUSTED_URL", "https://example.com")

	if err := InitSessionCookieSettings(); err != nil {
		t.Fatalf("InitSessionCookieSettings() error = %v", err)
	}
	if !SessionCookieSecure {
		t.Fatal("SessionCookieSecure = false, want true")
	}
	if got, want := len(SessionCookieTrustedURLs), 1; got != want {
		t.Fatalf("trusted URL count = %d, want %d", got, want)
	}
	if SessionCookieTrustedURLs[0] != "https://example.com" {
		t.Fatalf("SessionCookieTrustedURLs = %v, want [https://example.com]", SessionCookieTrustedURLs)
	}
}

// TestInitSessionCookieSettingsAllowsMultipleTrustedURLs 验证逗号分隔的多个可信入口会被 trim 后保存。
func TestInitSessionCookieSettingsAllowsMultipleTrustedURLs(t *testing.T) {
	resetSessionCookieSettingsAfterTest(t)
	t.Setenv("SESSION_COOKIE_SECURE", "true")
	t.Setenv("SESSION_COOKIE_TRUSTED_URL", "https://example.com, https://admin.example.com")

	if err := InitSessionCookieSettings(); err != nil {
		t.Fatalf("InitSessionCookieSettings() error = %v", err)
	}
	want := []string{"https://example.com", "https://admin.example.com"}
	if len(SessionCookieTrustedURLs) != len(want) {
		t.Fatalf("SessionCookieTrustedURLs = %v, want %v", SessionCookieTrustedURLs, want)
	}
	for i := range want {
		if SessionCookieTrustedURLs[i] != want[i] {
			t.Fatalf("SessionCookieTrustedURLs = %v, want %v", SessionCookieTrustedURLs, want)
		}
	}
}
