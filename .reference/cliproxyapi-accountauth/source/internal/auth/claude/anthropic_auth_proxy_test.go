// claude - anthropic_auth_proxy_test.go
// Claude 认证代理配置测试
// 验证 NewClaudeAuthWithProxyURL 函数的代理覆盖功能：
// - "direct" 参数禁用代理（即使全局配置了代理）
// - 代理 URL 参数覆盖全局代理配置
package claude

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"golang.org/x/net/proxy"
)

// TestNewClaudeAuthWithProxyURL_OverrideDirectTakesPrecedence 验证：
// 当代理覆盖参数为 "direct" 时，即使全局配置了 SOCKS5 代理，
// 认证客户端也使用直连（proxy.Direct）。
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

// TestNewClaudeAuthWithProxyURL_OverrideProxyAppliedWithoutConfig 验证：
// 即使没有全局代理配置，代理覆盖参数也能正确应用。
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
