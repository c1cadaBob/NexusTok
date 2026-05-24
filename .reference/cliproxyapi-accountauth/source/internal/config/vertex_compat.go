// 包 config - vertex_compat.go
// 该文件定义了 Vertex AI 兼容 API 密钥的配置结构体和清理函数。
package config

import "strings"

// VertexCompatKey 表示 Vertex AI 兼容 API 密钥的配置。
// 支持使用 Vertex AI 风格端点路径但使用简单 API 密钥认证的第三方服务
// （/publishers/google/models/{model}:streamGenerateContent）。
//
// 示例服务：zenmux.ai 和类似的 Vertex 兼容提供商。
type VertexCompatKey struct {
	// APIKey 是访问 Vertex 兼容 API 的认证密钥。
	// 映射到 x-goog-api-key 头。
	APIKey string `yaml:"api-key" json:"api-key"`

	// Priority 控制多个凭证匹配时的选择偏好。
	// 值越高越优先；默认为 0。
	Priority int `yaml:"priority,omitempty" json:"priority,omitempty"`

	// Prefix 可选地为此凭证的模型别名命名空间化（如 "teamA/vertex-pro"）。
	Prefix string `yaml:"prefix,omitempty" json:"prefix,omitempty"`

	// BaseURL 可选地覆盖 Vertex 兼容 API 端点。
	// 执行器将在此基础上追加 "/v1/publishers/google/models/{model}:action"。
	// 为空时，请求回退到默认的 Vertex API 基础 URL。
	BaseURL string `yaml:"base-url,omitempty" json:"base-url,omitempty"`

	// ProxyURL 可选地覆盖此 API 密钥的全局代理。
	ProxyURL string `yaml:"proxy-url,omitempty" json:"proxy-url,omitempty"`

	// Headers 可选地为此密钥发送的请求添加额外的 HTTP 头。
	// 通常用于 cookies、user-agent 和其他认证头。
	Headers map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`

	// Models 定义包括路由别名的模型配置。
	Models []VertexCompatModel `yaml:"models,omitempty" json:"models,omitempty"`

	// ExcludedModels 列出此提供商应排除的模型 ID。
	ExcludedModels []string `yaml:"excluded-models,omitempty" json:"excluded-models,omitempty"`
}

// GetAPIKey 返回 Vertex 兼容 API 密钥。
func (k VertexCompatKey) GetAPIKey() string { return k.APIKey }

// GetBaseURL 返回 Vertex 兼容 API 的基础 URL。
func (k VertexCompatKey) GetBaseURL() string { return k.BaseURL }

// VertexCompatModel 表示 Vertex 兼容的模型配置，
// 包括实际模型名称及其用于 API 路由的别名。
type VertexCompatModel struct {
	// Name 是外部提供商使用的实际模型名称
	Name string `yaml:"name" json:"name"`

	// Alias 是客户端用于引用此模型的模型名称别名
	Alias string `yaml:"alias" json:"alias"`
}

// GetName 返回实际模型名称。
func (m VertexCompatModel) GetName() string { return m.Name }

// GetAlias 返回模型别名。
func (m VertexCompatModel) GetAlias() string { return m.Alias }

// SanitizeVertexCompatKeys 去重和规范化 Vertex 兼容 API 密钥凭证。
func (cfg *Config) SanitizeVertexCompatKeys() {
	if cfg == nil {
		return
	}

	seen := make(map[string]struct{}, len(cfg.VertexCompatAPIKey))
	out := cfg.VertexCompatAPIKey[:0]
	for i := range cfg.VertexCompatAPIKey {
		entry := cfg.VertexCompatAPIKey[i]
		entry.APIKey = strings.TrimSpace(entry.APIKey)
		if entry.APIKey == "" {
			continue
		}
		entry.Prefix = normalizeModelPrefix(entry.Prefix)
		entry.BaseURL = strings.TrimSpace(entry.BaseURL)
		entry.ProxyURL = strings.TrimSpace(entry.ProxyURL)
		entry.Headers = NormalizeHeaders(entry.Headers)
		entry.ExcludedModels = NormalizeExcludedModels(entry.ExcludedModels)

		// Sanitize models: remove entries without valid alias
		sanitizedModels := make([]VertexCompatModel, 0, len(entry.Models))
		for _, model := range entry.Models {
			model.Alias = strings.TrimSpace(model.Alias)
			model.Name = strings.TrimSpace(model.Name)
			if model.Alias != "" && model.Name != "" {
				sanitizedModels = append(sanitizedModels, model)
			}
		}
		entry.Models = sanitizedModels

		// Use API key + base URL as uniqueness key
		uniqueKey := entry.APIKey + "|" + entry.BaseURL
		if _, exists := seen[uniqueKey]; exists {
			continue
		}
		seen[uniqueKey] = struct{}{}
		out = append(out, entry)
	}
	cfg.VertexCompatAPIKey = out
}
