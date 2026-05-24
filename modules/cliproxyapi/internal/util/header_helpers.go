// util - header_helpers.go
// 本文件提供从认证属性映射中提取和应用自定义 HTTP 头的功能。
// 支持通过 "header:" 前缀的键名来定义自定义请求头。
package util

import (
	"net/http"
	"strings"
)

// ApplyCustomHeadersFromAttrs 从属性映射中提取自定义 HTTP 头并应用到请求中。
// 属性中以 "header:" 为前缀的键会被识别为自定义头定义。
// 自定义头会覆盖内置默认头。
//
// 参数：
//   - r: 要应用自定义头的 HTTP 请求
//   - attrs: 包含自定义头定义的属性映射
func ApplyCustomHeadersFromAttrs(r *http.Request, attrs map[string]string) {
	if r == nil {
		return
	}
	applyCustomHeaders(r, extractCustomHeaders(attrs))
}

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
