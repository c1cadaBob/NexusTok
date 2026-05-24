// helps - proxy_helpers_test.go
// 代理感知 HTTP 客户端辅助函数的单元测试。
// 测试在全局代理与认证级别的 direct 代理配置共存时，
// NewProxyAwareHTTPClient 能否正确绕过全局代理，直连目标地址。
package helps

import (
	"context"
	"net/http"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	sdkconfig "github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
)

// TestNewProxyAwareHTTPClientDirectBypassesGlobalProxy 测试当认证记录的代理设置为 "direct" 时，
// 即使全局配置了代理地址，NewProxyAwareHTTPClient 也应禁用代理函数（Transport.Proxy 为 nil），
// 使请求直接连接目标服务器，不经过任何代理。
func TestNewProxyAwareHTTPClientDirectBypassesGlobalProxy(t *testing.T) {
	t.Parallel()

	client := NewProxyAwareHTTPClient(
		context.Background(),
		&config.Config{SDKConfig: sdkconfig.SDKConfig{ProxyURL: "http://global-proxy.example.com:8080"}},
		&cliproxyauth.Auth{ProxyURL: "direct"},
		0,
	)

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport type = %T, want *http.Transport", client.Transport)
	}
	if transport.Proxy != nil {
		t.Fatal("expected direct transport to disable proxy function")
	}
}
