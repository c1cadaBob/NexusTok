// xai - xai_auth_test.go
// XAI OAuth 认证流程测试
// 验证 xAI 提供商的 OAuth 认证相关功能：
// - 授权 URL 构建（包含所有必需参数）
// - OAuth 端点验证（仅接受 x.ai 域名的 HTTPS 端点）
// - 令牌刷新请求格式（POST 表单提交 client_id 和 refresh_token）
package xai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// TestBuildAuthorizeURLIncludesXAIRequiredParameters 验证构建的授权 URL
// 包含 xAI OAuth 所需的所有参数：response_type、client_id、redirect_uri、
// scope、code_challenge、code_challenge_method、state、nonce、plan、referrer。
func TestBuildAuthorizeURLIncludesXAIRequiredParameters(t *testing.T) {
	authURL, err := BuildAuthorizeURL(AuthorizeURLParams{
		AuthorizationEndpoint: "https://auth.x.ai/oauth/authorize",
		RedirectURI:           "http://127.0.0.1:56121/callback",
		CodeChallenge:         "challenge",
		State:                 "state-123",
		Nonce:                 "nonce-123",
	})
	if err != nil {
		t.Fatalf("BuildAuthorizeURL() error = %v", err)
	}

	parsed, errParse := url.Parse(authURL)
	if errParse != nil {
		t.Fatalf("parse authorize URL: %v", errParse)
	}
	if parsed.Scheme != "https" || parsed.Host != "auth.x.ai" || parsed.Path != "/oauth/authorize" {
		t.Fatalf("authorize URL endpoint = %s://%s%s", parsed.Scheme, parsed.Host, parsed.Path)
	}

	query := parsed.Query()
	want := map[string]string{
		"response_type":         "code",
		"client_id":             ClientID,
		"redirect_uri":          "http://127.0.0.1:56121/callback",
		"scope":                 Scope,
		"code_challenge":        "challenge",
		"code_challenge_method": "S256",
		"state":                 "state-123",
		"nonce":                 "nonce-123",
		"plan":                  "generic",
		"referrer":              "cli-proxy-api",
	}
	for key, value := range want {
		if got := query.Get(key); got != value {
			t.Fatalf("%s = %q, want %q", key, got, value)
		}
	}
}

// TestValidateOAuthEndpointRejectsNonXAIOrigin 验证 OAuth 端点验证：
// - 仅接受 https://auth.x.ai 域名的端点
// - 拒绝非 HTTPS 端点
// - 拒绝非 x.ai 域名的端点
func TestValidateOAuthEndpointRejectsNonXAIOrigin(t *testing.T) {
	if _, err := ValidateOAuthEndpoint("https://auth.x.ai/oauth/token", "token_endpoint"); err != nil {
		t.Fatalf("ValidateOAuthEndpoint(xai) error = %v", err)
	}
	if _, err := ValidateOAuthEndpoint("http://auth.x.ai/oauth/token", "token_endpoint"); err == nil {
		t.Fatal("expected non-HTTPS endpoint to be rejected")
	}
	if _, err := ValidateOAuthEndpoint("https://evil.example/oauth/token", "token_endpoint"); err == nil {
		t.Fatal("expected non-xAI endpoint to be rejected")
	}
}

// TestRefreshTokensPostsClientIDAndRefreshToken 验证令牌刷新请求：
// - 使用 POST 方法
// - Content-Type 为 application/x-www-form-urlencoded
// - 包含 grant_type=refresh_token、client_id、refresh_token 参数
// - 正确解析返回的 access_token 和 refresh_token
func TestRefreshTokensPostsClientIDAndRefreshToken(t *testing.T) {
	var gotForm url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/x-www-form-urlencoded") {
			t.Fatalf("Content-Type = %q, want form", got)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}
		gotForm = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "new-access",
			"refresh_token": "new-refresh",
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	}))
	defer server.Close()

	auth := NewXAIAuth(nil)
	tokenData, err := auth.RefreshTokens(context.Background(), "old-refresh", server.URL)
	if err != nil {
		t.Fatalf("RefreshTokens() error = %v", err)
	}
	if tokenData.AccessToken != "new-access" {
		t.Fatalf("access token = %q, want new-access", tokenData.AccessToken)
	}
	if gotForm.Get("grant_type") != "refresh_token" {
		t.Fatalf("grant_type = %q, want refresh_token", gotForm.Get("grant_type"))
	}
	if gotForm.Get("client_id") != ClientID {
		t.Fatalf("client_id = %q, want %q", gotForm.Get("client_id"), ClientID)
	}
	if gotForm.Get("refresh_token") != "old-refresh" {
		t.Fatalf("refresh_token = %q, want old-refresh", gotForm.Get("refresh_token"))
	}
}
