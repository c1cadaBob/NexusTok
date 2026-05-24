// proxyutil - proxy_test.go
// 代理工具包的单元测试文件，用于验证代理 URL 解析、HTTP 传输构建、
// CONNECT 隧道拨号、凭据脱敏等功能的正确性和安全性。
package proxyutil

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// mustDefaultTransport 获取 http.DefaultTransport 并断言其类型为 *http.Transport。
// 如果类型断言失败或值为 nil，则终止测试。用作测试辅助函数。
func mustDefaultTransport(t *testing.T) *http.Transport {
	t.Helper()

	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok || transport == nil {
		t.Fatal("http.DefaultTransport is not an *http.Transport")
	}
	return transport
}

// TestParse 验证 Parse 函数对各种代理模式字符串的解析行为，
// 包括：空字符串（继承模式）、"direct"/"none"（直连模式）、
// http/https/socks5/socks5h URL（代理模式）以及非法输入（应报错）。
// 所有子测试并行执行。
func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string // 测试用例名称
		input   string // 输入的代理模式字符串
		want    Mode   // 期望解析得到的模式枚举值
		wantErr bool   // 是否期望产生解析错误
	}{
		{name: "inherit", input: "", want: ModeInherit},
		{name: "direct", input: "direct", want: ModeDirect},
		{name: "none", input: "none", want: ModeDirect},
		{name: "http", input: "http://proxy.example.com:8080", want: ModeProxy},
		{name: "https", input: "https://proxy.example.com:8443", want: ModeProxy},
		{name: "socks5", input: "socks5://proxy.example.com:1080", want: ModeProxy},
		{name: "socks5h", input: "socks5h://proxy.example.com:1080", want: ModeProxy},
		{name: "invalid", input: "bad-value", want: ModeInvalid, wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			setting, errParse := Parse(tt.input)
			if tt.wantErr && errParse == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && errParse != nil {
				t.Fatalf("unexpected error: %v", errParse)
			}
			if setting.Mode != tt.want {
				t.Fatalf("mode = %d, want %d", setting.Mode, tt.want)
			}
		})
	}
}

// TestBuildHTTPTransportDirectBypassesProxy 验证当代理模式为 "direct" 时，
// BuildHTTPTransport 返回的 Transport 的 Proxy 函数为 nil（即绕过代理，直连目标服务器）。
func TestBuildHTTPTransportDirectBypassesProxy(t *testing.T) {
	t.Parallel()

	transport, mode, errBuild := BuildHTTPTransport("direct")
	if errBuild != nil {
		t.Fatalf("BuildHTTPTransport returned error: %v", errBuild)
	}
	if mode != ModeDirect {
		t.Fatalf("mode = %d, want %d", mode, ModeDirect)
	}
	if transport == nil {
		t.Fatal("expected transport, got nil")
	}
	if transport.Proxy != nil {
		t.Fatal("expected direct transport to disable proxy function")
	}
}

// TestBuildHTTPTransportHTTPProxy 验证当传入 HTTP 代理 URL 时，
// BuildHTTPTransport 返回的 Transport 能正确设置代理地址，
// 并且继承了 http.DefaultTransport 的关键超时参数（ForceAttemptHTTP2、
// IdleConnTimeout、TLSHandshakeTimeout）。
func TestBuildHTTPTransportHTTPProxy(t *testing.T) {
	t.Parallel()

	transport, mode, errBuild := BuildHTTPTransport("http://proxy.example.com:8080")
	if errBuild != nil {
		t.Fatalf("BuildHTTPTransport returned error: %v", errBuild)
	}
	if mode != ModeProxy {
		t.Fatalf("mode = %d, want %d", mode, ModeProxy)
	}
	if transport == nil {
		t.Fatal("expected transport, got nil")
	}

	req, errRequest := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if errRequest != nil {
		t.Fatalf("http.NewRequest returned error: %v", errRequest)
	}

	proxyURL, errProxy := transport.Proxy(req)
	if errProxy != nil {
		t.Fatalf("transport.Proxy returned error: %v", errProxy)
	}
	if proxyURL == nil || proxyURL.String() != "http://proxy.example.com:8080" {
		t.Fatalf("proxy URL = %v, want http://proxy.example.com:8080", proxyURL)
	}

	defaultTransport := mustDefaultTransport(t)
	if transport.ForceAttemptHTTP2 != defaultTransport.ForceAttemptHTTP2 {
		t.Fatalf("ForceAttemptHTTP2 = %v, want %v", transport.ForceAttemptHTTP2, defaultTransport.ForceAttemptHTTP2)
	}
	if transport.IdleConnTimeout != defaultTransport.IdleConnTimeout {
		t.Fatalf("IdleConnTimeout = %v, want %v", transport.IdleConnTimeout, defaultTransport.IdleConnTimeout)
	}
	if transport.TLSHandshakeTimeout != defaultTransport.TLSHandshakeTimeout {
		t.Fatalf("TLSHandshakeTimeout = %v, want %v", transport.TLSHandshakeTimeout, defaultTransport.TLSHandshakeTimeout)
	}
}

// TestBuildHTTPTransportSOCKS5ProxyInheritsDefaultTransportSettings 验证当传入
// SOCKS5 代理 URL 时，BuildHTTPTransport 返回的 Transport：
// 1. 代理模式为 ModeProxy
// 2. Proxy 函数为 nil（SOCKS5 通过自定义 Dialer 实现代理，而非 HTTP Proxy 函数）
// 3. 继承了 http.DefaultTransport 的关键超时参数
func TestBuildHTTPTransportSOCKS5ProxyInheritsDefaultTransportSettings(t *testing.T) {
	t.Parallel()

	transport, mode, errBuild := BuildHTTPTransport("socks5://proxy.example.com:1080")
	if errBuild != nil {
		t.Fatalf("BuildHTTPTransport returned error: %v", errBuild)
	}
	if mode != ModeProxy {
		t.Fatalf("mode = %d, want %d", mode, ModeProxy)
	}
	if transport == nil {
		t.Fatal("expected transport, got nil")
	}
	if transport.Proxy != nil {
		t.Fatal("expected SOCKS5 transport to bypass http proxy function")
	}

	defaultTransport := mustDefaultTransport(t)
	if transport.ForceAttemptHTTP2 != defaultTransport.ForceAttemptHTTP2 {
		t.Fatalf("ForceAttemptHTTP2 = %v, want %v", transport.ForceAttemptHTTP2, defaultTransport.ForceAttemptHTTP2)
	}
	if transport.IdleConnTimeout != defaultTransport.IdleConnTimeout {
		t.Fatalf("IdleConnTimeout = %v, want %v", transport.IdleConnTimeout, defaultTransport.IdleConnTimeout)
	}
	if transport.TLSHandshakeTimeout != defaultTransport.TLSHandshakeTimeout {
		t.Fatalf("TLSHandshakeTimeout = %v, want %v", transport.TLSHandshakeTimeout, defaultTransport.TLSHandshakeTimeout)
	}
}

// TestBuildHTTPTransportSOCKS5HProxy 验证当传入 SOCKS5H 代理 URL 时，
// BuildHTTPTransport 返回的 Transport：
// 1. 代理模式为 ModeProxy
// 2. Proxy 函数为 nil（不使用 HTTP 代理机制）
// 3. 设置了自定义 DialContext（用于通过 SOCKS5H 代理在代理端进行 DNS 解析）
func TestBuildHTTPTransportSOCKS5HProxy(t *testing.T) {
	t.Parallel()

	transport, mode, errBuild := BuildHTTPTransport("socks5h://proxy.example.com:1080")
	if errBuild != nil {
		t.Fatalf("BuildHTTPTransport returned error: %v", errBuild)
	}
	if mode != ModeProxy {
		t.Fatalf("mode = %d, want %d", mode, ModeProxy)
	}
	if transport == nil {
		t.Fatal("expected transport, got nil")
	}
	if transport.Proxy != nil {
		t.Fatal("expected SOCKS5H transport to bypass http proxy function")
	}
	if transport.DialContext == nil {
		t.Fatal("expected SOCKS5H transport to have custom DialContext")
	}
}

// TestBuildDialerHTTPProxyCONNECT 验证 BuildDialer 在 HTTP 代理场景下的完整
// CONNECT 隧道流程。测试流程如下：
// 1. 启动一个本地模拟 HTTP 代理服务器，监听随机端口
// 2. 代理服务器接收 CONNECT 请求，验证目标地址和 Proxy-Authorization 头
// 3. 代理服务器返回 "200 Connection Established" 建立隧道
// 4. 客户端通过隧道发送 "ping" 数据，代理服务器验证接收
// 5. 代理服务器通过隧道回复 "ok"，客户端验证接收
// 该测试覆盖了带认证凭据的 HTTP 代理 CONNECT 方法的完整握手和数据传输。
func TestBuildDialerHTTPProxyCONNECT(t *testing.T) {
	t.Parallel()

	// 启动本地 TCP 监听器，模拟 HTTP 代理服务器
	listener, errListen := net.Listen("tcp", "127.0.0.1:0")
	if errListen != nil {
		t.Fatalf("net.Listen returned error: %v", errListen)
	}
	defer func() {
		if errClose := listener.Close(); errClose != nil {
			t.Errorf("listener.Close returned error: %v", errClose)
		}
	}()

	// done 通道用于接收模拟代理服务器的处理结果
	done := make(chan error, 1)
	go func() {
		// 接受客户端连接
		conn, errAccept := listener.Accept()
		if errAccept != nil {
			done <- errAccept
			return
		}
		defer func() { _ = conn.Close() }()
		if errDeadline := conn.SetDeadline(time.Now().Add(5 * time.Second)); errDeadline != nil {
			done <- errDeadline
			return
		}

		// 读取客户端发送的 CONNECT 请求
		req, errRead := http.ReadRequest(bufio.NewReader(conn))
		if errRead != nil {
			done <- fmt.Errorf("read CONNECT request failed: %w", errRead)
			return
		}
		// 验证请求方法为 CONNECT
		if req.Method != http.MethodConnect {
			done <- fmt.Errorf("method = %s, want CONNECT", req.Method)
			return
		}
		// 验证目标主机地址
		if req.Host != "target.example.com:443" {
			done <- fmt.Errorf("host = %s, want target.example.com:443", req.Host)
			return
		}
		// 验证 Proxy-Authorization 头中的 Basic 认证凭据
		wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("user:pass"))
		if gotAuth := req.Header.Get("Proxy-Authorization"); gotAuth != wantAuth {
			done <- fmt.Errorf("Proxy-Authorization = %q, want %q", gotAuth, wantAuth)
			return
		}

		// 发送 200 响应，建立 CONNECT 隧道
		if _, errWrite := io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\nok"); errWrite != nil {
			done <- fmt.Errorf("write CONNECT response failed: %w", errWrite)
			return
		}

		// 读取客户端通过隧道发送的数据（期望为 "ping"）
		buf := make([]byte, 4)
		n, errReadTunnel := io.ReadFull(conn, buf)
		if errReadTunnel != nil {
			done <- fmt.Errorf("read tunneled payload failed after %d bytes: %w", n, errReadTunnel)
			return
		}
		if string(buf) != "ping" {
			done <- fmt.Errorf("tunneled payload = %q, want ping", string(buf))
			return
		}
		done <- nil
	}()

	// 使用 BuildDialer 构建带认证的 HTTP 代理拨号器
	dialer, mode, errBuild := BuildDialer("http://user:pass@" + listener.Addr().String())
	if errBuild != nil {
		t.Fatalf("BuildDialer returned error: %v", errBuild)
	}
	if mode != ModeProxy {
		t.Fatalf("mode = %d, want %d", mode, ModeProxy)
	}
	if dialer == nil {
		t.Fatal("expected dialer, got nil")
	}

	// 通过代理拨号器建立连接（会发送 CONNECT 请求）
	conn, errDial := dialer.Dial("tcp", "target.example.com:443")
	if errDial != nil {
		t.Fatalf("dialer.Dial returned error: %v", errDial)
	}
	defer func() {
		if errClose := conn.Close(); errClose != nil {
			t.Errorf("conn.Close returned error: %v", errClose)
		}
	}()

	// 从隧道连接读取代理服务器的响应（期望为 "ok"）
	buf := make([]byte, 2)
	n, errRead := io.ReadFull(conn, buf)
	if errRead != nil {
		t.Fatalf("conn.Read returned error after %d bytes: %v", n, errRead)
	}
	if string(buf) != "ok" {
		t.Fatalf("buffered tunnel payload = %q, want ok", string(buf))
	}

	// 通过隧道发送 "ping" 数据给代理服务器
	if _, errWrite := conn.Write([]byte("ping")); errWrite != nil {
		t.Fatalf("conn.Write returned error: %v", errWrite)
	}

	// 等待模拟代理服务器完成验证
	if errServer := <-done; errServer != nil {
		t.Fatalf("proxy server returned error: %v", errServer)
	}
}

// TestRedactProxyURL 验证 Redact 函数对代理 URL 的凭据脱敏行为：
// 1. 包含凭据的 URL：凭据部分替换为 "redacted"，去除路径和查询参数
// 2. 不含凭据的 URL：原样返回
// 3. 非法 URL：返回 "<invalid proxy URL>" 占位符
func TestRedactProxyURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string // 测试用例名称
		input string // 输入的代理 URL
		want  string // 期望的脱敏结果
	}{
		{
			name:  "with credentials",
			input: "http://user:pass@proxy.example.com:8080/path?token=secret",
			want:  "http://redacted@proxy.example.com:8080",
		},
		{
			name:  "without credentials",
			input: "socks5://proxy.example.com:1080",
			want:  "socks5://proxy.example.com:1080",
		},
		{
			name:  "invalid",
			input: "bad-value",
			want:  "<invalid proxy URL>",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := Redact(tt.input); got != tt.want {
				t.Fatalf("Redact() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestParseErrorDoesNotExposeProxyCredentials 安全测试：验证当 Parse 函数解析
// 非法代理 URL 返回错误时，错误信息中不会泄露用户的代理凭据（用户名、密码等）。
// 这是一个防止敏感信息通过日志或错误消息泄露的安全保障测试。
func TestParseErrorDoesNotExposeProxyCredentials(t *testing.T) {
	t.Parallel()

	input := "http://user:secret%@proxy.example.com:8080"
	_, errParse := Parse(input)
	if errParse == nil {
		t.Fatal("expected Parse to return an error")
	}
	// 确保错误信息不包含原始输入、用户名或密码
	if strings.Contains(errParse.Error(), input) ||
		strings.Contains(errParse.Error(), "user") ||
		strings.Contains(errParse.Error(), "secret") {
		t.Fatalf("parse error exposes proxy credentials: %q", errParse.Error())
	}
}
