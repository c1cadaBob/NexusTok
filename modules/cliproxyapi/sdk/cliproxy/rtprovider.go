// cliproxy - rtprovider.go
// 该文件实现了默认的 HTTP RoundTripper 提供者。
// 根据 Auth 的 ProxyURL 配置为每个认证条目创建独立的 HTTP transport，
// 并按代理 URL 字符串缓存 transport 实例以避免重复创建。

package cliproxy

import (
	"net/http"
	"strings"
	"sync"

	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/proxyutil"
	log "github.com/sirupsen/logrus"
)

// defaultRoundTripperProvider 根据 Auth.ProxyURL 值为每个认证条目提供独立的 HTTP RoundTripper。
// 按代理 URL 字符串缓存 transport 实例。
type defaultRoundTripperProvider struct {
	mu    sync.RWMutex
	cache map[string]http.RoundTripper
}

// newDefaultRoundTripperProvider 创建默认的 RoundTripper 提供者实例。
func newDefaultRoundTripperProvider() *defaultRoundTripperProvider {
	return &defaultRoundTripperProvider{cache: make(map[string]http.RoundTripper)}
}

// RoundTripperFor 实现 coreauth.RoundTripperProvider 接口。
// 根据 auth 的 ProxyURL 返回配置好代理的 HTTP transport。
// "direct" 模式会绕过所有代理，空值返回 nil（使用默认 transport）。
func (p *defaultRoundTripperProvider) RoundTripperFor(auth *coreauth.Auth) http.RoundTripper {
	if auth == nil {
		return nil
	}
	proxyStr := strings.TrimSpace(auth.ProxyURL)
	if proxyStr == "" {
		return nil
	}
	p.mu.RLock()
	rt := p.cache[proxyStr]
	p.mu.RUnlock()
	if rt != nil {
		return rt
	}
	transport, _, errBuild := proxyutil.BuildHTTPTransport(proxyStr)
	if errBuild != nil {
		log.Errorf("%v", errBuild)
		return nil
	}
	if transport == nil {
		return nil
	}
	p.mu.Lock()
	p.cache[proxyStr] = transport
	p.mu.Unlock()
	return transport
}
