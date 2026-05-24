// handlers - header_filter.go
// 提供上游响应头过滤功能，移除逐跳头部、安全敏感头部和已知 AI 网关注入的头部，
// 避免 Claude Code 客户端遥测检测到代理类型。
package handlers

import (
	"net/http"
	"strings"
)

// gatewayHeaderPrefixes 列出已知 AI 网关代理注入的头部名称前缀。
// Claude Code 的客户端遥测会检测这些头部并报告网关类型，
// 因此需要从上游响应中剥离这些头部以避免被检测。
var gatewayHeaderPrefixes = []string{
	"x-litellm-",
	"helicone-",
	"x-portkey-",
	"cf-aig-",
	"x-kong-",
	"x-bt-",
}

// hopByHopHeaders 列出 RFC 7230 第 6.1 节定义的逐跳头部（代理不得转发），
// 以及不应泄露的安全敏感头部。
var hopByHopHeaders = map[string]struct{}{
	// RFC 7230 hop-by-hop
	"Connection":          {},
	"Keep-Alive":          {},
	"Proxy-Authenticate":  {},
	"Proxy-Authorization": {},
	"Te":                  {},
	"Trailer":             {},
	"Transfer-Encoding":   {},
	"Upgrade":             {},
	// Security-sensitive
	"Set-Cookie": {},
	// CPA-managed (set by handlers, not upstream)
	"Content-Length":   {},
	"Content-Encoding": {},
}

// FilterUpstreamHeaders 返回 src 的副本，其中移除了逐跳头部和安全敏感头部。
// 如果 src 为 nil 或过滤后为空则返回 nil。
func FilterUpstreamHeaders(src http.Header) http.Header {
	if src == nil {
		return nil
	}
	connectionScoped := connectionScopedHeaders(src)
	dst := make(http.Header)
	for key, values := range src {
		canonicalKey := http.CanonicalHeaderKey(key)
		if _, blocked := hopByHopHeaders[canonicalKey]; blocked {
			continue
		}
		if _, scoped := connectionScoped[canonicalKey]; scoped {
			continue
		}
		// Strip headers injected by known AI gateway proxies to avoid
		// Claude Code client-side gateway detection.
		lowerKey := strings.ToLower(key)
		gatewayMatch := false
		for _, prefix := range gatewayHeaderPrefixes {
			if strings.HasPrefix(lowerKey, prefix) {
				gatewayMatch = true
				break
			}
		}
		if gatewayMatch {
			continue
		}
		dst[key] = values
	}
	if len(dst) == 0 {
		return nil
	}
	return dst
}

// connectionScopedHeaders 解析 Connection 头部中列出的逐跳头部名称。
func connectionScopedHeaders(src http.Header) map[string]struct{} {
	scoped := make(map[string]struct{})
	for _, rawValue := range src.Values("Connection") {
		for _, token := range strings.Split(rawValue, ",") {
			headerName := strings.TrimSpace(token)
			if headerName == "" {
				continue
			}
			scoped[http.CanonicalHeaderKey(headerName)] = struct{}{}
		}
	}
	return scoped
}

// WriteUpstreamHeaders 将过滤后的上游头部写入 gin 响应写入器。
// 已被 CPA 处理器设置的头部（如 Content-Type）不会被覆盖。
func WriteUpstreamHeaders(dst http.Header, src http.Header) {
	if src == nil {
		return
	}
	for key, values := range src {
		// Don't overwrite headers already set by CPA handlers
		if dst.Get(key) != "" {
			continue
		}
		for _, v := range values {
			dst.Add(key, v)
		}
	}
}
