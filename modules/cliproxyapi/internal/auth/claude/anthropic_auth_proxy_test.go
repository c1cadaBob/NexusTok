// claude - anthropic_auth_proxy_test.go
// 测试 Claude 认证服务的代理 URL 覆盖行为，包括 "direct" 模式优先和
// 无配置时的代理应用。
package claude

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"golang.org/x/net/proxy"
)

// TestNewClaudeAuthWithProxyURL_OverrideDirectTakesPrecedence 验证当传入 "direct" 代理 URL 时，
// 即使配置中已设置 SOCKS5 代理，也会优先使用直连模式。
func TestNewClaudeAuthWithProxyURL_OverrideDirectTakesPrecedence(t *testing.T) {
	cfg := &config.Config{SDKConfig: config.SDKConfig{ProxyURL: "socks5://proxy.example.com:1080"}}
	auth := NewClaudeAuthWithProxyURL(cfg, "direct")

	transport, ok := auth.httpClient.Transport.(*utlsRoundTripper)
	if !ok || transport == nil {
		t.Fatalf("expected utlsRoundTripper, got %T", auth.httpClient.Transport)
	}
	if transport.dialer != proxy.Direct {
		t.Fatalf("expected proxy.Direct, got %T", transport.dialer)
	}
}

// TestNewClaudeAuthWithProxyURL_OverrideProxyAppliedWithoutConfig 验证在无配置的情况下，
// 传入的代理 URL 仍然会被正确应用。
func TestNewClaudeAuthWithProxyURL_OverrideProxyAppliedWithoutConfig(t *testing.T) {
	auth := NewClaudeAuthWithProxyURL(nil, "socks5://proxy.example.com:1080")

	transport, ok := auth.httpClient.Transport.(*utlsRoundTripper)
	if !ok || transport == nil {
		t.Fatalf("expected utlsRoundTripper, got %T", auth.httpClient.Transport)
	}
	if transport.dialer == proxy.Direct {
		t.Fatalf("expected proxy dialer, got %T", transport.dialer)
	}
}
