// 包 proxyutil - proxy.go
// 该文件提供了代理配置解析和连接构建的工具函数。
// 支持 HTTP、HTTPS 和 SOCKS5 代理协议，包括代理设置解析、
// HTTP 传输层构建、连接层拨号器构建和代理 URL 脱敏等功能。
package proxyutil

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/proxy"
)

// Mode 描述代理设置的解释方式。
type Mode int

const (
	// ModeInherit 表示未配置显式代理行为，继承环境默认设置。
	ModeInherit Mode = iota
	// ModeDirect 表示出站请求必须显式绕过代理，直接连接。
	ModeDirect
	// ModeProxy 表示配置了具体的代理 URL。
	ModeProxy
	// ModeInvalid 表示代理设置存在但格式错误或不支持。
	ModeInvalid
)

// Setting 是代理配置值的规范化解释结果。
type Setting struct {
	Raw  string   // 原始代理配置字符串
	Mode Mode     // 解析后的代理模式
	URL  *url.URL // 解析后的代理 URL（仅 ModeProxy 模式下有效）
}

// Parse 将代理配置值规范化为继承、直连或代理模式。
// 支持 "direct"/"none" 关键字、以及 socks5://、http://、https:// 等协议前缀。
//
// 参数:
//   - raw: 原始代理配置字符串
//
// 返回:
//   - Setting: 解析后的代理设置
//   - error: 解析失败时返回错误信息
func Parse(raw string) (Setting, error) {
	trimmed := strings.TrimSpace(raw)
	setting := Setting{Raw: trimmed}

	if trimmed == "" {
		setting.Mode = ModeInherit
		return setting, nil
	}

	if strings.EqualFold(trimmed, "direct") || strings.EqualFold(trimmed, "none") {
		setting.Mode = ModeDirect
		return setting, nil
	}

	parsedURL, errParse := url.Parse(trimmed)
	if errParse != nil {
		setting.Mode = ModeInvalid
		return setting, fmt.Errorf("parse proxy URL failed")
	}
	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		setting.Mode = ModeInvalid
		return setting, fmt.Errorf("proxy URL missing scheme/host")
	}

	switch parsedURL.Scheme {
	case "socks5", "socks5h", "http", "https":
		setting.Mode = ModeProxy
		setting.URL = parsedURL
		return setting, nil
	default:
		setting.Mode = ModeInvalid
		return setting, fmt.Errorf("unsupported proxy scheme: %s", parsedURL.Scheme)
	}
}

// cloneDefaultTransport 克隆默认 HTTP 传输层配置，避免修改全局默认值。
func cloneDefaultTransport() *http.Transport {
	if transport, ok := http.DefaultTransport.(*http.Transport); ok && transport != nil {
		return transport.Clone()
	}
	return &http.Transport{}
}

// NewDirectTransport 返回一个绕过环境代理的 HTTP 传输层。
// 其 Proxy 字段设为 nil，确保所有请求直接连接目标服务器。
//
// 返回:
//   - *http.Transport: 直连模式的 HTTP 传输层
func NewDirectTransport() *http.Transport {
	clone := cloneDefaultTransport()
	clone.Proxy = nil
	return clone
}

// BuildHTTPTransport 根据代理设置构建 HTTP 传输层。
// 对于 SOCKS5 代理，创建自定义拨号器；对于 HTTP/HTTPS 代理，使用标准 ProxyURL。
//
// 参数:
//   - raw: 原始代理配置字符串
//
// 返回:
//   - *http.Transport: 构建的传输层（ModeInherit 时为 nil）
//   - Mode: 解析后的代理模式
//   - error: 构建失败时返回错误信息
func BuildHTTPTransport(raw string) (*http.Transport, Mode, error) {
	setting, errParse := Parse(raw)
	if errParse != nil {
		return nil, setting.Mode, errParse
	}

	switch setting.Mode {
	case ModeInherit:
		return nil, setting.Mode, nil
	case ModeDirect:
		return NewDirectTransport(), setting.Mode, nil
	case ModeProxy:
		if setting.URL.Scheme == "socks5" || setting.URL.Scheme == "socks5h" {
			var proxyAuth *proxy.Auth
			if setting.URL.User != nil {
				username := setting.URL.User.Username()
				password, _ := setting.URL.User.Password()
				proxyAuth = &proxy.Auth{User: username, Password: password}
			}
			dialer, errSOCKS5 := proxy.SOCKS5("tcp", setting.URL.Host, proxyAuth, proxy.Direct)
			if errSOCKS5 != nil {
				return nil, setting.Mode, fmt.Errorf("create SOCKS5 dialer failed: %w", errSOCKS5)
			}
			transport := cloneDefaultTransport()
			transport.Proxy = nil
			transport.DialContext = func(_ context.Context, network, addr string) (net.Conn, error) {
				return dialer.Dial(network, addr)
			}
			return transport, setting.Mode, nil
		}
		transport := cloneDefaultTransport()
		transport.Proxy = http.ProxyURL(setting.URL)
		return transport, setting.Mode, nil
	default:
		return nil, setting.Mode, nil
	}
}

// BuildDialer 根据代理设置构建连接层拨号器。
// 对于 HTTP/HTTPS 代理返回 httpConnectDialer，对于 SOCKS5 代理返回标准代理拨号器。
//
// 参数:
//   - raw: 原始代理配置字符串
//
// 返回:
//   - proxy.Dialer: 构建的拨号器（ModeInherit 时为 nil）
//   - Mode: 解析后的代理模式
//   - error: 构建失败时返回错误信息
func BuildDialer(raw string) (proxy.Dialer, Mode, error) {
	setting, errParse := Parse(raw)
	if errParse != nil {
		return nil, setting.Mode, errParse
	}

	switch setting.Mode {
	case ModeInherit:
		return nil, setting.Mode, nil
	case ModeDirect:
		return proxy.Direct, setting.Mode, nil
	case ModeProxy:
		if setting.URL.Scheme == "http" || setting.URL.Scheme == "https" {
			return &httpConnectDialer{proxyURL: setting.URL, dialer: proxy.Direct}, setting.Mode, nil
		}
		dialer, errDialer := proxy.FromURL(setting.URL, proxy.Direct)
		if errDialer != nil {
			return nil, setting.Mode, fmt.Errorf("create proxy dialer failed: %w", errDialer)
		}
		return dialer, setting.Mode, nil
	default:
		return nil, setting.Mode, nil
	}
}

// httpConnectDialer 实现了通过 HTTP CONNECT 方法建立隧道的代理拨号器。
// 支持 HTTP 和 HTTPS 代理服务器的连接建立。
type httpConnectDialer struct {
	proxyURL *url.URL     // 代理服务器 URL
	dialer   proxy.Dialer // 底层拨号器
}

// Dial 通过 HTTP CONNECT 方法建立到目标地址的隧道连接。
// 对于 HTTPS 代理，先建立 TLS 连接再发送 CONNECT 请求。
//
// 参数:
//   - network: 网络类型（如 "tcp"）
//   - addr: 目标地址（host:port 格式）
//
// 返回:
//   - net.Conn: 建立的连接
//   - error: 连接失败时返回错误信息
func (d *httpConnectDialer) Dial(network, addr string) (net.Conn, error) {
	proxyConn, errDial := d.dialer.Dial(network, proxyDialAddr(d.proxyURL))
	if errDial != nil {
		return nil, fmt.Errorf("dial HTTP proxy failed: %w", errDial)
	}

	conn := proxyConn
	if d.proxyURL.Scheme == "https" {
		tlsConn := tls.Client(conn, &tls.Config{ServerName: d.proxyURL.Hostname()})
		if errHandshake := tlsConn.Handshake(); errHandshake != nil {
			if errClose := conn.Close(); errClose != nil {
				return nil, fmt.Errorf("HTTPS proxy TLS handshake failed: %w; close failed: %v", errHandshake, errClose)
			}
			return nil, fmt.Errorf("HTTPS proxy TLS handshake failed: %w", errHandshake)
		}
		conn = tlsConn
	}

	req := &http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Host: addr},
		Host:   addr,
		Header: make(http.Header),
	}
	if d.proxyURL.User != nil {
		req.Header.Set("Proxy-Authorization", proxyAuthorization(d.proxyURL.User))
	}
	if errWrite := req.Write(conn); errWrite != nil {
		if errClose := conn.Close(); errClose != nil {
			return nil, fmt.Errorf("write CONNECT request failed: %w; close failed: %v", errWrite, errClose)
		}
		return nil, fmt.Errorf("write CONNECT request failed: %w", errWrite)
	}

	reader := bufio.NewReader(conn)
	resp, errRead := http.ReadResponse(reader, req)
	if errRead != nil {
		if errClose := conn.Close(); errClose != nil {
			return nil, fmt.Errorf("read CONNECT response failed: %w; close failed: %v", errRead, errClose)
		}
		return nil, fmt.Errorf("read CONNECT response failed: %w", errRead)
	}
	if resp.StatusCode != http.StatusOK {
		if resp.Body != nil {
			_ = resp.Body.Close()
		}
		if errClose := conn.Close(); errClose != nil {
			return nil, fmt.Errorf("proxy CONNECT returned status %s; close failed: %v", resp.Status, errClose)
		}
		return nil, fmt.Errorf("proxy CONNECT returned status %s", resp.Status)
	}

	if reader.Buffered() > 0 {
		return &bufferedConn{Conn: conn, reader: reader}, nil
	}
	return conn, nil
}

// proxyDialAddr 根据代理 URL 构建拨号地址（host:port 格式）。
// 未指定端口时，HTTP 默认使用 80，HTTPS 默认使用 443。
//
// 参数:
//   - proxyURL: 代理服务器 URL
//
// 返回:
//   - string: 格式化的拨号地址
func proxyDialAddr(proxyURL *url.URL) string {
	port := proxyURL.Port()
	if port == "" {
		port = "80"
		if proxyURL.Scheme == "https" {
			port = "443"
		}
	}
	return net.JoinHostPort(proxyURL.Hostname(), port)
}

// proxyAuthorization 根据用户信息生成 Proxy-Authorization 请求头值。
// 使用 Basic 认证方式，将用户名和密码进行 Base64 编码。
//
// 参数:
//   - user: 包含用户名和密码的 Userinfo 对象
//
// 返回:
//   - string: "Basic <base64>" 格式的认证字符串
func proxyAuthorization(user *url.Userinfo) string {
	username := user.Username()
	password, _ := user.Password()
	encoded := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	return "Basic " + encoded
}

// Redact 返回脱敏后的代理 URL，移除凭据和路径信息，适用于日志记录。
//
// 参数:
//   - raw: 原始代理 URL 字符串
//
// 返回:
//   - string: 脱敏后的 URL，凭据替换为 "redacted"；无效 URL 返回 "<invalid proxy URL>"
func Redact(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	parsedURL, errParse := url.Parse(trimmed)
	if errParse != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return "<invalid proxy URL>"
	}

	redacted := &url.URL{
		Scheme: parsedURL.Scheme,
		Host:   parsedURL.Host,
	}
	if parsedURL.User != nil {
		redacted.User = url.User("redacted")
	}
	return redacted.String()
}

// bufferedConn 包装了 net.Conn，在读取时优先从缓冲区读取。
// 用于 HTTP CONNECT 隧道建立后，缓冲区中可能残留的响应数据不被丢失。
type bufferedConn struct {
	net.Conn
	reader *bufio.Reader // 带缓冲的读取器
}

// Read 从缓冲区或底层连接读取数据。
// 优先读取缓冲区中的残留数据，缓冲区为空后回退到底层连接。
//
// 参数:
//   - p: 读取数据的目标缓冲区
//
// 返回:
//   - int: 读取的字节数
//   - error: 读取失败时返回错误信息
func (c *bufferedConn) Read(p []byte) (int, error) {
	if c.reader.Buffered() > 0 {
		return c.reader.Read(p)
	}
	return c.Conn.Read(p)
}
