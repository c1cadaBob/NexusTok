// claude - utls_transport.go
// 包 claude 提供 Anthropic Claude API 的认证功能。
// 该文件实现了使用 utls 的自定义 HTTP 传输层，用于绕过 TLS 指纹检测。
// 使用 Chrome 浏览器的 TLS 指纹来规避 Anthropic 域名上的 Cloudflare 检测。
package claude

import (
	"net/http"
	"strings"
	"sync"

	tls "github.com/refraction-networking/utls"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/proxyutil"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/http2"
	"golang.org/x/net/proxy"
)

// utlsRoundTripper 实现了使用 utls 和 Chrome 指纹的 http.RoundTripper。
// 用于绕过 Anthropic 域名上的 Cloudflare TLS 指纹检测。
type utlsRoundTripper struct {
	// mu 保护 connections 映射和 pending 映射
	mu sync.Mutex
	// connections 缓存每个主机的 HTTP/2 客户端连接
	connections map[string]*http2.ClientConn
	// pending 跟踪当前正在建立连接的主机（防止竞态条件）
	pending map[string]*sync.Cond
	// dialer 用于创建网络连接，支持代理
	dialer proxy.Dialer
}

// newUtlsRoundTripper 创建一个新的基于 utls 的 round tripper，支持可选代理。
//
// 参数：
//   - cfg: SDK 配置，包含代理设置
//
// 返回：
//   - *utlsRoundTripper: 新的 utls round tripper 实例
func newUtlsRoundTripper(cfg *config.SDKConfig) *utlsRoundTripper {
	var dialer proxy.Dialer = proxy.Direct
	if cfg != nil {
		proxyDialer, mode, errBuild := proxyutil.BuildDialer(cfg.ProxyURL)
		if errBuild != nil {
			log.Errorf("failed to configure proxy dialer for %q: %v", proxyutil.Redact(cfg.ProxyURL), errBuild)
		} else if mode != proxyutil.ModeInherit && proxyDialer != nil {
			dialer = proxyDialer
		}
	}

	return &utlsRoundTripper{
		connections: make(map[string]*http2.ClientConn),
		pending:     make(map[string]*sync.Cond),
		dialer:      dialer,
	}
}

// getOrCreateConnection 获取现有连接或创建新连接。
// 使用每个主机的锁定机制来防止多个 goroutine 同时创建到同一主机的连接。
//
// 参数：
//   - host: 目标主机名
//   - addr: 目标地址（包含端口）
//
// 返回：
//   - *http2.ClientConn: HTTP/2 客户端连接
//   - error: 连接创建失败时返回的错误
func (t *utlsRoundTripper) getOrCreateConnection(host, addr string) (*http2.ClientConn, error) {
	t.mu.Lock()

	// Check if connection exists and is usable
	if h2Conn, ok := t.connections[host]; ok && h2Conn.CanTakeNewRequest() {
		t.mu.Unlock()
		return h2Conn, nil
	}

	// Check if another goroutine is already creating a connection
	if cond, ok := t.pending[host]; ok {
		// Wait for the other goroutine to finish
		cond.Wait()
		// Check if connection is now available
		if h2Conn, ok := t.connections[host]; ok && h2Conn.CanTakeNewRequest() {
			t.mu.Unlock()
			return h2Conn, nil
		}
		// Connection still not available, we'll create one
	}

	// Mark this host as pending
	cond := sync.NewCond(&t.mu)
	t.pending[host] = cond
	t.mu.Unlock()

	// Create connection outside the lock
	h2Conn, err := t.createConnection(host, addr)

	t.mu.Lock()
	defer t.mu.Unlock()

	// Remove pending marker and wake up waiting goroutines
	delete(t.pending, host)
	cond.Broadcast()

	if err != nil {
		return nil, err
	}

	// Store the new connection
	t.connections[host] = h2Conn
	return h2Conn, nil
}

// createConnection 创建带有 Chrome TLS 指纹的新 HTTP/2 连接。
// Chrome 的 TLS 指纹更接近 Node.js/OpenSSL（真实 Claude Code 使用的），
// 减少了 TLS 层和 HTTP 头之间的不匹配。
//
// 参数：
//   - host: 目标主机名
//   - addr: 目标地址（包含端口）
//
// 返回：
//   - *http2.ClientConn: 新创建的 HTTP/2 客户端连接
//   - error: 连接创建失败时返回的错误
func (t *utlsRoundTripper) createConnection(host, addr string) (*http2.ClientConn, error) {
	conn, err := t.dialer.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}

	tlsConfig := &tls.Config{ServerName: host}
	tlsConn := tls.UClient(conn, tlsConfig, tls.HelloChrome_Auto)

	if err := tlsConn.Handshake(); err != nil {
		conn.Close()
		return nil, err
	}

	tr := &http2.Transport{}
	h2Conn, err := tr.NewClientConn(tlsConn)
	if err != nil {
		tlsConn.Close()
		return nil, err
	}

	return h2Conn, nil
}

// RoundTrip 实现 http.RoundTripper 接口。
// 处理 HTTP 请求，获取或创建到目标主机的连接，并执行请求。
//
// 参数：
//   - req: 要执行的 HTTP 请求
//
// 返回：
//   - *http.Response: HTTP 响应
//   - error: 请求执行失败时返回的错误
func (t *utlsRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	host := req.URL.Host
	addr := host
	if !strings.Contains(addr, ":") {
		addr += ":443"
	}

	// Get hostname without port for TLS ServerName
	hostname := req.URL.Hostname()

	h2Conn, err := t.getOrCreateConnection(hostname, addr)
	if err != nil {
		return nil, err
	}

	resp, err := h2Conn.RoundTrip(req)
	if err != nil {
		// Connection failed, remove it from cache
		t.mu.Lock()
		if cached, ok := t.connections[hostname]; ok && cached == h2Conn {
			delete(t.connections, hostname)
		}
		t.mu.Unlock()
		return nil, err
	}

	return resp, nil
}

// NewAnthropicHttpClient 创建一个绕过 TLS 指纹检测的 HTTP 客户端。
// 通过使用 utls 和 Chrome 指纹来规避 Anthropic 域名上的 Cloudflare 检测。
// 接受可选的 SDK 配置用于代理设置。
//
// 参数：
//   - cfg: SDK 配置，包含代理设置
//
// 返回：
//   - *http.Client: 配置了自定义传输层的 HTTP 客户端
func NewAnthropicHttpClient(cfg *config.SDKConfig) *http.Client {
	return &http.Client{
		Transport: newUtlsRoundTripper(cfg),
	}
}
