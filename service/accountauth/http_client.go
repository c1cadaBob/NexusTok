// http_client.go 提供带代理支持的 HTTP 客户端工厂函数。
// 支持 HTTP/HTTPS 和 SOCKS5 代理协议，用于账号认证过程中的外部 API 调用。
package accountauth

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common" // 公共配置：TLS 设置等

	"golang.org/x/net/proxy" // SOCKS5 代理支持
)

// providerHTTPTimeout 是与上游认证服务通信的默认超时时间（20秒）
const providerHTTPTimeout = 20 * time.Second

// httpClientWithProxy 根据代理 URL 创建 HTTP 客户端。
// 支持三种情况：
//   - 无代理：返回默认客户端
//   - HTTP/HTTPS 代理：通过 http.ProxyURL 配置传输层
//   - SOCKS5/SOCKS5H 代理：通过 golang.org/x/net/proxy 创建 SOCKS5 拨号器
//
// 如果全局 TLS 配置启用了跳过证书验证，会应用到传输层。
//
// 参数：
//   - proxyURL: 代理地址（可以为空）
//
// 返回：
//   - *http.Client: 配置好的 HTTP 客户端
//   - error: 代理解析或创建错误
func httpClientWithProxy(proxyURL string) (*http.Client, error) {
	proxyURL = strings.TrimSpace(proxyURL)
	// 无代理时返回带超时的默认客户端
	if proxyURL == "" {
		return &http.Client{Timeout: providerHTTPTimeout}, nil
	}
	parsedURL, err := url.Parse(proxyURL)
	if err != nil {
		return nil, err
	}
	switch parsedURL.Scheme {
	case "http", "https":
		// HTTP/HTTPS 代理：使用标准 ProxyURL 配置
		transport := &http.Transport{
			ForceAttemptHTTP2: true,
			Proxy:             http.ProxyURL(parsedURL),
		}
		if common.TLSInsecureSkipVerify {
			transport.TLSClientConfig = common.InsecureTLSConfig
		}
		return &http.Client{Transport: transport, Timeout: providerHTTPTimeout}, nil
	case "socks5", "socks5h":
		// SOCKS5 代理：解析认证信息并创建拨号器
		var auth *proxy.Auth
		if parsedURL.User != nil {
			auth = &proxy.Auth{User: parsedURL.User.Username()}
			if password, ok := parsedURL.User.Password(); ok {
				auth.Password = password
			}
		}
		dialer, err := proxy.SOCKS5("tcp", parsedURL.Host, auth, proxy.Direct)
		if err != nil {
			return nil, err
		}
		transport := &http.Transport{
			ForceAttemptHTTP2: true,
			// 使用 SOCKS5 拨号器替换默认的 TCP 拨号器
			DialContext: func(ctx context.Context, network string, addr string) (net.Conn, error) {
				return dialer.Dial(network, addr)
			},
		}
		if common.TLSInsecureSkipVerify {
			transport.TLSClientConfig = common.InsecureTLSConfig
		}
		return &http.Client{Transport: transport, Timeout: providerHTTPTimeout}, nil
	default:
		return nil, fmt.Errorf("unsupported proxy scheme: %s", parsedURL.Scheme)
	}
}
