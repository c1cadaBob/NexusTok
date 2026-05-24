// Package util 提供 CLIProxyAPI 应用中使用的工具函数。
// 包括根据模型名确定 AI 服务提供商和管理 HTTP 代理等辅助函数。
package util

import (
	"net/url"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	log "github.com/sirupsen/logrus"
)

// GetProviderName 确定能够服务指定模型的所有 AI 服务提供商。
// 首先查询全局模型注册表获取支持该模型的提供商，如果模型未注册则回退到传统字符串启发式推断。
//
// 支持的提供商包括（但不限于）：
//   - "gemini" 用于 Google Gemini 系列
//   - "codex" 用于 OpenAI GPT 兼容提供商
//   - "claude" 用于 Anthropic 模型
//   - "openai-compatibility" 用于外部 OpenAI 兼容提供商
//
// 参数：
//   - modelName: 要识别提供商的模型名称
//
// 返回：
//   - []string: 能够服务该模型的所有提供商标识符，按优先级排序
func GetProviderName(modelName string) []string {
	if modelName == "" {
		return nil
	}

	providers := make([]string, 0, 4)
	seen := make(map[string]struct{})

	appendProvider := func(name string) {
		if name == "" {
			return
		}
		if _, exists := seen[name]; exists {
			return
		}
		seen[name] = struct{}{}
		providers = append(providers, name)
	}

	for _, provider := range registry.GetGlobalRegistry().GetModelProviders(modelName) {
		appendProvider(provider)
	}

	if len(providers) > 0 {
		return providers
	}

	return providers
}

// ResolveAutoModel 将 "auto" 模型名解析为实际可用的模型。
// 使用空处理器类型从注册表中获取任意可用模型。
//
// 参数：
//   - modelName: 要检查的模型名（应为 "auto"）
//
// 返回：
//   - string: 解析后的模型名，如果不是 "auto" 或解析失败则返回原始名称
func ResolveAutoModel(modelName string) string {
	if modelName != "auto" {
		return modelName
	}

	// Use empty string as handler type to get any available model
	firstModel, err := registry.GetGlobalRegistry().GetFirstAvailableModel("")
	if err != nil {
		log.Warnf("Failed to resolve 'auto' model: %v, falling back to original model name", err)
		return modelName
	}

	log.Infof("Resolved 'auto' model to: %s", firstModel)
	return firstModel
}

// IsOpenAICompatibilityAlias 检查给定模型名是否是 OpenAI 兼容性路由配置的别名。
//
// 参数：
//   - modelName: 要检查的模型名
//   - cfg: 包含 OpenAI 兼容性设置的应用配置
//
// 返回：
//   - bool: 如果模型名是 OpenAI 兼容别名则返回 true
func IsOpenAICompatibilityAlias(modelName string, cfg *config.Config) bool {
	if cfg == nil {
		return false
	}

	for _, compat := range cfg.OpenAICompatibility {
		if compat.Disabled {
			continue
		}
		for _, model := range compat.Models {
			if model.Alias == modelName {
				return true
			}
		}
	}
	return false
}

// GetOpenAICompatibilityConfig 返回给定别名对应的 OpenAI 兼容性配置和模型详情。
//
// 参数：
//   - alias: 要查找配置的模型别名
//   - cfg: 包含 OpenAI 兼容性设置的应用配置
//
// 返回：
//   - *config.OpenAICompatibility: 匹配的兼容性配置，未找到则为 nil
//   - *config.OpenAICompatibilityModel: 匹配的模型配置，未找到则为 nil
func GetOpenAICompatibilityConfig(alias string, cfg *config.Config) (*config.OpenAICompatibility, *config.OpenAICompatibilityModel) {
	if cfg == nil {
		return nil, nil
	}

	for _, compat := range cfg.OpenAICompatibility {
		if compat.Disabled {
			continue
		}
		for _, model := range compat.Models {
			if model.Alias == alias {
				return &compat, &model
			}
		}
	}
	return nil, nil
}

// InArray 检查字符串是否存在于字符串切片中。
// 遍历切片，找到目标字符串则返回 true，否则返回 false。
//
// 参数：
//   - hystack: 要搜索的字符串切片
//   - needle: 要查找的字符串
//
// 返回：
//   - bool: 找到则返回 true，否则返回 false
func InArray(hystack []string, needle string) bool {
	for _, item := range hystack {
		if needle == item {
			return true
		}
	}
	return false
}

// HideAPIKey 模糊处理 API 密钥用于日志记录，仅显示前几个和后几个字符。
//
// 参数：
//   - apiKey: 要模糊处理的 API 密钥
//
// 返回：
//   - string: 模糊处理后的 API 密钥
func HideAPIKey(apiKey string) string {
	if len(apiKey) > 8 {
		return apiKey[:4] + "..." + apiKey[len(apiKey)-4:]
	} else if len(apiKey) > 4 {
		return apiKey[:2] + "..." + apiKey[len(apiKey)-2:]
	} else if len(apiKey) > 2 {
		return apiKey[:1] + "..." + apiKey[len(apiKey)-1:]
	}
	return apiKey
}

// MaskAuthorizationHeader 模糊处理 Authorization 头的值，同时保留认证类型前缀。
// 常见格式如 "Bearer <token>"、"Basic <credentials>"、"ApiKey <key>" 等。
// 保留前缀（如 "Bearer "），仅模糊处理令牌/凭证部分。
//
// 参数：
//   - value: Authorization 头的值
//
// 返回：
//   - string: 保留前缀的模糊处理 Authorization 值
func MaskAuthorizationHeader(value string) string {
	parts := strings.SplitN(strings.TrimSpace(value), " ", 2)
	if len(parts) < 2 {
		return HideAPIKey(value)
	}
	return parts[0] + " " + HideAPIKey(parts[1])
}

// MaskSensitiveHeaderValue 根据头名称模糊处理敏感头值，同时保留预期格式。
//
// 按头名称（不区分大小写）的行为：
//   - "Authorization": 保留认证类型前缀（如 "Bearer "），仅模糊处理凭证部分
//   - 包含 "api-key" 的头: 使用 HideAPIKey 模糊处理整个值
//   - 其他: 原样返回值
//
// 参数：
//   - key: 要检查的 HTTP 头名称（不区分大小写）
//   - value: 敏感时要模糊处理的头值
//
// 返回：
//   - string: 根据头类型模糊处理后的值；非敏感则不变
func MaskSensitiveHeaderValue(key, value string) string {
	lowerKey := strings.ToLower(strings.TrimSpace(key))
	switch {
	case strings.Contains(lowerKey, "authorization"):
		return MaskAuthorizationHeader(value)
	case strings.Contains(lowerKey, "api-key"),
		strings.Contains(lowerKey, "apikey"),
		strings.Contains(lowerKey, "token"),
		strings.Contains(lowerKey, "secret"):
		return HideAPIKey(value)
	default:
		return value
	}
}

// MaskSensitiveQuery 模糊处理原始查询字符串中的敏感查询参数（如 auth_token）。
func MaskSensitiveQuery(raw string) string {
	if raw == "" {
		return ""
	}
	parts := strings.Split(raw, "&")
	changed := false
	for i, part := range parts {
		if part == "" {
			continue
		}
		keyPart := part
		valuePart := ""
		if idx := strings.Index(part, "="); idx >= 0 {
			keyPart = part[:idx]
			valuePart = part[idx+1:]
		}
		decodedKey, err := url.QueryUnescape(keyPart)
		if err != nil {
			decodedKey = keyPart
		}
		if !shouldMaskQueryParam(decodedKey) {
			continue
		}
		decodedValue, err := url.QueryUnescape(valuePart)
		if err != nil {
			decodedValue = valuePart
		}
		masked := HideAPIKey(strings.TrimSpace(decodedValue))
		parts[i] = keyPart + "=" + url.QueryEscape(masked)
		changed = true
	}
	if !changed {
		return raw
	}
	return strings.Join(parts, "&")
}

// shouldMaskQueryParam 判断查询参数是否需要模糊处理。
// 包含 key、api-key、apikey、api_key、token、secret 的参数需要模糊处理。
func shouldMaskQueryParam(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return false
	}
	key = strings.TrimSuffix(key, "[]")
	if key == "key" || strings.Contains(key, "api-key") || strings.Contains(key, "apikey") || strings.Contains(key, "api_key") {
		return true
	}
	if strings.Contains(key, "token") || strings.Contains(key, "secret") {
		return true
	}
	return false
}
