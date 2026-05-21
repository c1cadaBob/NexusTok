package accountauth

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common"

	"golang.org/x/net/proxy"
)

const providerHTTPTimeout = 20 * time.Second

func httpClientWithProxy(proxyURL string) (*http.Client, error) {
	proxyURL = strings.TrimSpace(proxyURL)
	if proxyURL == "" {
		return &http.Client{Timeout: providerHTTPTimeout}, nil
	}
	parsedURL, err := url.Parse(proxyURL)
	if err != nil {
		return nil, err
	}
	switch parsedURL.Scheme {
	case "http", "https":
		transport := &http.Transport{
			ForceAttemptHTTP2: true,
			Proxy:             http.ProxyURL(parsedURL),
		}
		if common.TLSInsecureSkipVerify {
			transport.TLSClientConfig = common.InsecureTLSConfig
		}
		return &http.Client{Transport: transport, Timeout: providerHTTPTimeout}, nil
	case "socks5", "socks5h":
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
