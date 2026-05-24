// cliproxy - rtdriprovider_test.go
// 该文件测试默认 HTTP RoundTripper 提供者的行为。
// 验证 "direct" 代理模式是否正确禁用代理函数。

package cliproxy

import (
	"net/http"
	"testing"

	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

// TestRoundTripperForDirectBypassesProxy 测试 "direct" 代理模式是否正确禁用代理函数。
// 当 auth 的 ProxyURL 设置为 "direct" 时，返回的 transport 不应包含代理设置。
func TestRoundTripperForDirectBypassesProxy(t *testing.T) {
	t.Parallel()

	provider := newDefaultRoundTripperProvider()
	rt := provider.RoundTripperFor(&coreauth.Auth{ProxyURL: "direct"})
	transport, ok := rt.(*http.Transport)
	if !ok {
		t.Fatalf("transport type = %T, want *http.Transport", rt)
	}
	if transport.Proxy != nil {
		t.Fatal("expected direct transport to disable proxy function")
	}
}
