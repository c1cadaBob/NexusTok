// 包 util - header_helpers.go
// 该文件提供了自定义 HTTP 头的应用功能。
// 用于从属性映射中提取并应用用户自定义的请求头。
package util

import (
	"net/http"
	"strings"
)

// ApplyCustomHeadersFromAttrs 从属性映射中提取并应用用户自定义头。
// 自定义头在冲突时覆盖内置默认值。
//
// 参数：
//   - r: 要修改的 HTTP 请求
//   - attrs: 包含 "header:" 前缀键的属性映射
func ApplyCustomHeadersFromAttrs(r *http.Request, attrs map[string]string) {
	if r == nil {
		return
	}
	applyCustomHeaders(r, extractCustomHeaders(attrs))
}

// extractCustomHeaders 从属性映射中提取 "header:" 前缀的自定义头。
func extractCustomHeaders(attrs map[string]string) map[string]string {
	if len(attrs) == 0 {
		return nil
	}
	headers := make(map[string]string)
	for k, v := range attrs {
		if !strings.HasPrefix(k, "header:") {
			continue
		}
		name := strings.TrimSpace(strings.TrimPrefix(k, "header:"))
		if name == "" {
			continue
		}
		val := strings.TrimSpace(v)
		if val == "" {
			continue
		}
		headers[name] = val
	}
	if len(headers) == 0 {
		return nil
	}
	return headers
}

// applyCustomHeaders 将自定义头应用到 HTTP 请求。
// 特殊处理 Host 头（设置 req.Host）。
func applyCustomHeaders(r *http.Request, headers map[string]string) {
	if r == nil || len(headers) == 0 {
		return
	}
	for k, v := range headers {
		if k == "" || v == "" {
			continue
		}
		// net/http reads Host from req.Host (not req.Header) when writing
		// a real request, so we must mirror it there. Some callers pass
		// synthetic requests (e.g. &http.Request{Header: ...}) and only
		// consume r.Header afterwards, so keep the value in the header
		// map too.
		if http.CanonicalHeaderKey(k) == "Host" {
			r.Host = v
		}
		r.Header.Set(k, v)
	}
}
