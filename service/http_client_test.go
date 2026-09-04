package service

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/stretchr/testify/require"

	"golang.org/x/net/proxy"
)

type blockingProxyDialer struct {
	started chan struct{}
	release chan struct{}
}

func (d *blockingProxyDialer) Dial(network, address string) (net.Conn, error) {
	close(d.started)
	<-d.release
	return nil, errors.New("dial released")
}

type contextProxyDialer struct {
	called bool
}

func (d *contextProxyDialer) Dial(network, address string) (net.Conn, error) {
	return nil, errors.New("Dial should not be called")
}

func (d *contextProxyDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	d.called = true
	return nil, ctx.Err()
}

var _ proxy.Dialer = (*blockingProxyDialer)(nil)
var _ proxy.ContextDialer = (*contextProxyDialer)(nil)

func TestDialProxyContextHonorsCancellationForLegacyDialer(t *testing.T) {
	dialer := &blockingProxyDialer{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	start := time.Now()
	conn, err := dialProxyContext(ctx, dialer, "tcp", "example.com:443")
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Nil(t, conn)
	require.Less(t, time.Since(start), time.Second)
	<-dialer.started
	close(dialer.release)
}

func TestDialProxyContextUsesContextDialer(t *testing.T) {
	dialer := &contextProxyDialer{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	conn, err := dialProxyContext(ctx, dialer, "tcp", "example.com:443")
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, conn)
	require.True(t, dialer.called)
}

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
	originalMaxConnsPerHost := common.RelayMaxConnsPerHost
	originalResponseHeaderTimeout := common.RelayResponseHeaderTimeout
	originalProxyClientCacheTTL := common.RelayProxyClientCacheTTL
	originalProxyClientCacheMaxSize := common.RelayProxyClientCacheMaxSize
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
		common.RelayMaxConnsPerHost = originalMaxConnsPerHost
		common.RelayResponseHeaderTimeout = originalResponseHeaderTimeout
		common.RelayProxyClientCacheTTL = originalProxyClientCacheTTL
		common.RelayProxyClientCacheMaxSize = originalProxyClientCacheMaxSize
		common.RelayTimeout = originalRelayTimeout
		common.TLSInsecureSkipVerify = originalTLSInsecureSkipVerify
		common.InsecureTLSConfig = originalInsecureTLSConfig
	})

	httpClient = nil
	proxyClients = make(map[string]*proxyClientEntry)
	ssrfProtectedHTTPClient = nil
	common.RelayTimeout = 0
	common.TLSInsecureSkipVerify = false
	common.RelayIdleConnTimeout = 12
	common.RelayMaxIdleConns = 34
	common.RelayMaxIdleConnsPerHost = 5
	common.RelayMaxConnsPerHost = 7
	common.RelayResponseHeaderTimeout = 9
	common.RelayProxyClientCacheTTL = 900
	common.RelayProxyClientCacheMaxSize = 4096
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
	require.Equal(t, common.RelayMaxConnsPerHost, transport.MaxConnsPerHost)
	require.Equal(t, time.Duration(common.RelayIdleConnTimeout)*time.Second, transport.IdleConnTimeout)
	require.Equal(t, time.Duration(common.RelayResponseHeaderTimeout)*time.Second, transport.ResponseHeaderTimeout)
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

func TestProxyClientCacheEvictsOldestWhenMaxSizeExceeded(t *testing.T) {
	resetHTTPClientConfigForTest(t)
	common.RelayProxyClientCacheTTL = 0
	common.RelayProxyClientCacheMaxSize = 1

	first, err := NewProxyHttpClient("http://127.0.0.1:3128")
	require.NoError(t, err)
	time.Sleep(time.Millisecond)
	second, err := NewProxyHttpClient("http://127.0.0.1:3129")
	require.NoError(t, err)

	require.NotSame(t, first, second)
	require.Len(t, proxyClients, 1)
	require.Contains(t, proxyClients, "http://127.0.0.1:3129")
}

func TestProxyClientCacheRemovesExpiredEntries(t *testing.T) {
	resetHTTPClientConfigForTest(t)
	common.RelayProxyClientCacheTTL = 1
	common.RelayProxyClientCacheMaxSize = 0

	proxyClients["http://127.0.0.1:3128"] = &proxyClientEntry{
		client:   &http.Client{Transport: &http.Transport{}},
		lastUsed: time.Now().Add(-2 * time.Second),
	}

	client, err := NewProxyHttpClient("http://127.0.0.1:3129")
	require.NoError(t, err)

	require.NotNil(t, client)
	require.Len(t, proxyClients, 1)
	require.Contains(t, proxyClients, "http://127.0.0.1:3129")
}

func TestSOCKS5ProxyClientUsesRelayIdleConnTimeout(t *testing.T) {
	resetHTTPClientConfigForTest(t)
	common.RelayIdleConnTimeout = 31

	client, err := NewProxyHttpClient("socks5://127.0.0.1:1080")
	require.NoError(t, err)

	assertRelayTransportSettings(t, requireTransport(t, client))
}
