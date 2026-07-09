package service

import (
	"net/http"
	"testing"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/stretchr/testify/require"
)

// resetHTTPClientConfigForTest 保存并恢复通用 HTTP client 相关全局配置。
// 这些变量会在 InitHttpClient 与代理 client 缓存中被读取，测试必须隔离，避免影响其它用例。
func resetHTTPClientConfigForTest(t *testing.T) {
	t.Helper()

	originalHTTPClient := httpClient
	originalProxyClients := proxyClients
	originalProtectedClient := ssrfProtectedHTTPClient
	originalIdleTimeout := common.RelayIdleConnTimeout
	originalMaxIdleConns := common.RelayMaxIdleConns
	originalMaxIdleConnsPerHost := common.RelayMaxIdleConnsPerHost
	originalRelayTimeout := common.RelayTimeout
	originalTLSInsecureSkipVerify := common.TLSInsecureSkipVerify
	originalInsecureTLSConfig := common.InsecureTLSConfig

	t.Cleanup(func() {
		httpClient = originalHTTPClient
		proxyClients = originalProxyClients
		ssrfProtectedHTTPClient = originalProtectedClient
		common.RelayIdleConnTimeout = originalIdleTimeout
		common.RelayMaxIdleConns = originalMaxIdleConns
		common.RelayMaxIdleConnsPerHost = originalMaxIdleConnsPerHost
		common.RelayTimeout = originalRelayTimeout
		common.TLSInsecureSkipVerify = originalTLSInsecureSkipVerify
		common.InsecureTLSConfig = originalInsecureTLSConfig
	})

	httpClient = nil
	proxyClients = make(map[string]*http.Client)
	ssrfProtectedHTTPClient = nil
	common.RelayTimeout = 0
	common.TLSInsecureSkipVerify = false
	common.RelayIdleConnTimeout = 12
	common.RelayMaxIdleConns = 34
	common.RelayMaxIdleConnsPerHost = 5
}

func requireTransport(t *testing.T, client *http.Client) *http.Transport {
	t.Helper()

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	require.NotNil(t, transport)
	return transport
}

func assertRelayTransportSettings(t *testing.T, transport *http.Transport) {
	t.Helper()

	require.Equal(t, common.RelayMaxIdleConns, transport.MaxIdleConns)
	require.Equal(t, common.RelayMaxIdleConnsPerHost, transport.MaxIdleConnsPerHost)
	require.Equal(t, time.Duration(common.RelayIdleConnTimeout)*time.Second, transport.IdleConnTimeout)
	require.True(t, transport.ForceAttemptHTTP2)
}

func TestInitHttpClientUsesRelayIdleConnTimeout(t *testing.T) {
	resetHTTPClientConfigForTest(t)
	common.RelayIdleConnTimeout = 17

	InitHttpClient()

	assertRelayTransportSettings(t, requireTransport(t, GetHttpClient()))
}

func TestHTTPProxyClientUsesRelayIdleConnTimeout(t *testing.T) {
	resetHTTPClientConfigForTest(t)
	common.RelayIdleConnTimeout = 23

	client, err := NewProxyHttpClient("http://127.0.0.1:3128")
	require.NoError(t, err)

	assertRelayTransportSettings(t, requireTransport(t, client))
}

func TestSOCKS5ProxyClientUsesRelayIdleConnTimeout(t *testing.T) {
	resetHTTPClientConfigForTest(t)
	common.RelayIdleConnTimeout = 31

	client, err := NewProxyHttpClient("socks5://127.0.0.1:1080")
	require.NoError(t, err)

	assertRelayTransportSettings(t, requireTransport(t, client))
}
