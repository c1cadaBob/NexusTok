// 包 auth - custom_headers.go
// 该文件提供了从认证元数据中提取和应用自定义请求头的功能。
// 允许在认证文件中配置额外的请求头，这些请求头会在执行请求时自动附加。
package auth

import "strings"

// ExtractCustomHeadersFromMetadata 从认证元数据中提取自定义请求头。
// 支持 map[string]string 和 map[string]any 两种格式的 headers 字段。
//
// 参数:
//   - metadata: 认证元数据
//
// 返回:
//   - map[string]string: 提取的自定义请求头映射；无有效请求头时返回 nil
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

// ApplyCustomHeadersFromMetadata 从认证元数据中提取自定义请求头并应用到 Auth 的 Attributes 中。
// 请求头以 "header:<name>" 格式存储在 Attributes 中。
//
// 参数:
//   - auth: 认证记录
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
