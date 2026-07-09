package service

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/setting/system_setting"
)

// ssrfResolver 抽象 DNS 解析器，方便单元测试模拟 DNS rebinding。
type ssrfResolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

// protectedFetchDialer 在真正 Dial 前用 FetchSetting 对目标进行二次 SSRF 校验。
type protectedFetchDialer struct {
	resolver      ssrfResolver
	dialContext   func(ctx context.Context, network, address string) (net.Conn, error)
	getProtection func() (*common.SSRFProtection, bool, error)
}

// ssrfProtectedRoundTripper 为用户可控 URL 提供带 Dial 阶段 SSRF 防护的 Transport。
//
// 普通 Relay 上游可能合法指向内网 Ollama、自建模型或私有网关，因此该 client 不替换
// 全局 GetHttpClient；只有下载、Webhook、Bark/Gotify 等用户可控 URL 才应显式使用。
type ssrfProtectedRoundTripper struct {
	resolver      ssrfResolver
	dialContext   func(ctx context.Context, network, address string) (net.Conn, error)
	getProtection func() (*common.SSRFProtection, bool, error)
	proxy         func(*http.Request) (*url.URL, error)

	mutex      sync.Mutex
	transports map[string]*http.Transport
}

var ssrfProtectedHTTPClient *http.Client

// currentFetchProtection 读取当前 FetchSetting 并构建 SSRF 防护配置。
func currentFetchProtection() (*common.SSRFProtection, bool, error) {
	fetchSetting := system_setting.GetFetchSetting()
	if !fetchSetting.EnableSSRFProtection {
		return nil, false, nil
	}

	protection, err := common.NewSSRFProtectionFromFetchSetting(
		fetchSetting.AllowPrivateIp,
		fetchSetting.DomainFilterMode,
		fetchSetting.IpFilterMode,
		fetchSetting.DomainList,
		fetchSetting.IpList,
		fetchSetting.AllowedPorts,
		fetchSetting.ApplyIPFilterForDomain,
	)
	if err != nil {
		return nil, true, err
	}
	return protection, true, nil
}

// InitSSRFProtectedHTTPClient 初始化用户可控 URL 专用的受保护 HTTP client。
func InitSSRFProtectedHTTPClient() {
	ssrfProtectedHTTPClient = newProtectedFetchHTTPClient()
}

// GetSSRFProtectedHTTPClient 返回带 Dial 阶段 SSRF 校验的 client。
//
// 当 FetchSetting 关闭 SSRF 防护时返回普通全局 client，保持历史行为；开启时返回
// protected client，并在首次调用时懒初始化，便于单元测试和非标准启动路径使用。
func GetSSRFProtectedHTTPClient() *http.Client {
	fetchSetting := system_setting.GetFetchSetting()
	if !fetchSetting.EnableSSRFProtection {
		if client := GetHttpClient(); client != nil {
			return client
		}
		return http.DefaultClient
	}
	if ssrfProtectedHTTPClient == nil {
		InitSSRFProtectedHTTPClient()
	}
	return ssrfProtectedHTTPClient
}

func newProtectedFetchHTTPClient() *http.Client {
	return newProtectedFetchHTTPClientWithDialer(nil, nil, nil)
}

func newProtectedFetchHTTPClientWithDialer(resolver ssrfResolver, dialContext func(ctx context.Context, network, address string) (net.Conn, error), getProtection func() (*common.SSRFProtection, bool, error)) *http.Client {
	return newProtectedFetchHTTPClientWithProxy(resolver, dialContext, getProtection, http.ProxyFromEnvironment)
}

func newProtectedFetchHTTPClientWithProxy(resolver ssrfResolver, dialContext func(ctx context.Context, network, address string) (net.Conn, error), getProtection func() (*common.SSRFProtection, bool, error), proxy func(*http.Request) (*url.URL, error)) *http.Client {
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	if dialContext == nil {
		netDialer := &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}
		dialContext = netDialer.DialContext
	}
	if getProtection == nil {
		getProtection = currentFetchProtection
	}
	if proxy == nil {
		proxy = http.ProxyFromEnvironment
	}

	client := &http.Client{
		Transport: &ssrfProtectedRoundTripper{
			resolver:      resolver,
			dialContext:   dialContext,
			getProtection: getProtection,
			proxy:         proxy,
			transports:    make(map[string]*http.Transport),
		},
		CheckRedirect: checkRedirect,
	}
	if common.RelayTimeout != 0 {
		client.Timeout = time.Duration(common.RelayTimeout) * time.Second
	}
	return client
}

func (t *ssrfProtectedRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil || req.URL == nil {
		return nil, fmt.Errorf("invalid request")
	}

	protection, enabled, err := t.getProtection()
	if err != nil {
		return nil, err
	}
	if enabled {
		if err := protection.ValidateURL(req.URL.String()); err != nil {
			return nil, err
		}
	}

	proxyURL, err := t.proxy(req)
	if err != nil {
		return nil, err
	}
	return t.transportFor(proxyURL).RoundTrip(req)
}

func (t *ssrfProtectedRoundTripper) CloseIdleConnections() {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	for _, transport := range t.transports {
		transport.CloseIdleConnections()
	}
}

func (t *ssrfProtectedRoundTripper) transportFor(proxyURL *url.URL) *http.Transport {
	// 只按代理地址分组：代理来自环境变量，取值有限，map 有界；
	// 目标 origin 是用户可控输入，不能作为缓存 key。
	key := "direct"
	if proxyURL != nil {
		key = proxyURL.String()
	}
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if transport, ok := t.transports[key]; ok {
		return transport
	}

	transport := t.newTransport(proxyURL)
	t.transports[key] = transport
	return transport
}

func (t *ssrfProtectedRoundTripper) newTransport(proxyURL *url.URL) *http.Transport {
	dialContext := t.dialContext
	proxyFunc := http.ProxyURL(proxyURL)
	if proxyURL == nil {
		protectedDialer := &protectedFetchDialer{
			resolver:      t.resolver,
			dialContext:   t.dialContext,
			getProtection: t.getProtection,
		}
		dialContext = protectedDialer.DialContext
		proxyFunc = nil
	}

	transport := &http.Transport{
		MaxIdleConns:        common.RelayMaxIdleConns,
		MaxIdleConnsPerHost: common.RelayMaxIdleConnsPerHost,
		IdleConnTimeout:     time.Duration(common.RelayIdleConnTimeout) * time.Second,
		ForceAttemptHTTP2:   true,
		Proxy:               proxyFunc,
		DialContext:         dialContext,
	}
	if common.TLSInsecureSkipVerify {
		transport.TLSClientConfig = common.InsecureTLSConfig
	}
	return transport
}

func (d *protectedFetchDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	protection, enabled, err := d.getProtection()
	if err != nil {
		return nil, err
	}
	if !enabled {
		return d.dialContext(ctx, network, addr)
	}

	host, portText, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("invalid dial address %s: %w", addr, err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return nil, fmt.Errorf("invalid port: %s", portText)
	}
	if err := protection.ValidateNetworkTarget(host, port); err != nil {
		return nil, err
	}

	if ip := net.ParseIP(host); ip != nil {
		return d.dialContext(ctx, network, net.JoinHostPort(ip.String(), portText))
	}
	if !protection.ApplyIPFilterForDomain {
		return d.dialContext(ctx, network, addr)
	}

	resolved, err := d.resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("DNS resolution failed for %s: %v", host, err)
	}

	var candidateIPs []net.IP
	for _, ipAddr := range resolved {
		ip := ipAddr.IP
		if ip == nil || !networkAllowsIP(network, ip) {
			continue
		}
		if err := protection.ValidateResolvedIP(host, ip); err != nil {
			return nil, err
		}
		candidateIPs = append(candidateIPs, ip)
	}

	var lastDialErr error
	for _, ip := range candidateIPs {
		conn, err := d.dialContext(ctx, network, net.JoinHostPort(ip.String(), portText))
		if err == nil {
			return conn, nil
		}
		lastDialErr = err
	}

	if lastDialErr != nil {
		return nil, lastDialErr
	}
	return nil, fmt.Errorf("DNS resolution for %s returned no usable IP addresses", host)
}

func networkAllowsIP(network string, ip net.IP) bool {
	switch network {
	case "tcp4":
		return ip.To4() != nil
	case "tcp6":
		return ip.To4() == nil
	default:
		return true
	}
}
