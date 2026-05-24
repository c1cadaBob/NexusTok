// auth - custom_headers.go
// 该文件实现了从认证凭据元数据中提取和应用自定义 HTTP 头的功能，
// 允许在认证条目级别配置额外的请求头注入。
package auth

import "strings"

// ExtractCustomHeadersFromMetadata 从认证元数据中提取自定义 HTTP 头映射。
// 支持 map[string]string 和 map[string]any 两种格式的 headers 字段，
// 自动去除键值的首尾空白并过滤空值。
func ExtractCustomHeadersFromMetadata(metadata map[string]any) map[string]string {
	if len(metadata) == 0 {
		return nil
	}
	raw, ok := metadata["headers"]
	if !ok || raw == nil {
		return nil
	}

	out := make(map[string]string)
	switch headers := raw.(type) {
	case map[string]string:
		for key, value := range headers {
			name := strings.TrimSpace(key)
			if name == "" {
				continue
			}
			val := strings.TrimSpace(value)
			if val == "" {
				continue
			}
			out[name] = val
		}
	case map[string]any:
		for key, value := range headers {
			name := strings.TrimSpace(key)
			if name == "" {
				continue
			}
			rawVal, ok := value.(string)
			if !ok {
				continue
			}
			val := strings.TrimSpace(rawVal)
			if val == "" {
				continue
			}
			out[name] = val
		}
	default:
		return nil
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

// ApplyCustomHeadersFromMetadata 将认证元数据中的自定义头应用到认证凭据的 Attributes 中，
// 以 "header:头名称" 为键存储，供后续请求构建时使用。
func ApplyCustomHeadersFromMetadata(auth *Auth) {
	if auth == nil || len(auth.Metadata) == 0 {
		return
	}
	headers := ExtractCustomHeadersFromMetadata(auth.Metadata)
	if len(headers) == 0 {
		return
	}
	if auth.Attributes == nil {
		auth.Attributes = make(map[string]string)
	}
	for name, value := range headers {
		auth.Attributes["header:"+name] = value
	}
}
