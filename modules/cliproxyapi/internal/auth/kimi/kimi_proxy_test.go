// kimi - kimi_proxy_test.go
// 包含 Kimi 设备流程客户端代理配置的单元测试。
// 测试代理 URL 覆盖和直接连接模式的行为。
package kimi

import (
	"net/http"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
)

// TestNewDeviceFlowClientWithDeviceIDAndProxyURL_OverrideDirectDisablesProxy 测试 "direct" 代理覆盖会禁用代理。
// 验证当指定 "direct" 时，Transport 的 Proxy 函数为 nil。
func TestNewDeviceFlowClientWithDeviceIDAndProxyURL_OverrideDirectDisablesProxy(t *testing.T) {
	cfg := &config.Config{SDKConfig: config.SDKConfig{ProxyURL: "http://proxy.example.com:8080"}}
	client := NewDeviceFlowClientWithDeviceIDAndProxyURL(cfg, "device-1", "direct")

	transport, ok := client.httpClient.Transport.(*http.Transport)
	if !ok || transport == nil {
		t.Fatalf("expected http.Transport, got %T", client.httpClient.Transport)
	}
	if transport.Proxy != nil {
		t.Fatal("expected direct transport to disable proxy function")
	}
}

// TestNewDeviceFlowClientWithDeviceIDAndProxyURL_OverrideProxyTakesPrecedence 测试代理 URL 覆盖优先于全局配置。
// 验证指定的代理 URL 会覆盖全局配置中的代理设置。
func TestNewDeviceFlowClientWithDeviceIDAndProxyURL_OverrideProxyTakesPrecedence(t *testing.T) {
	cfg := &config.Config{SDKConfig: config.SDKConfig{ProxyURL: "http://global.example.com:8080"}}
	client := NewDeviceFlowClientWithDeviceIDAndProxyURL(cfg, "device-1", "http://override.example.com:8081")

	transport, ok := client.httpClient.Transport.(*http.Transport)
	if !ok || transport == nil {
		t.Fatalf("expected http.Transport, got %T", client.httpClient.Transport)
	}
	req, errReq := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if errReq != nil {
		t.Fatalf("new request: %v", errReq)
	}
	proxyURL, errProxy := transport.Proxy(req)
	if errProxy != nil {
		t.Fatalf("proxy func: %v", errProxy)
	}
	if proxyURL == nil || proxyURL.String() != "http://override.example.com:8081" {
		t.Fatalf("proxy URL = %v, want http://override.example.com:8081", proxyURL)
	}
}
