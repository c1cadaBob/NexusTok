// configaccess - provider.go
// 基于配置文件 API Key 的访问认证提供者实现。
// 该提供者通过比对请求中携带的 API Key 与配置文件中定义的 Key 列表来完成身份认证，
// 支持多种 Key 来源：Authorization 头、X-Goog-Api-Key 头、X-Api-Key 头、
// URL 查询参数 key 和 auth_token。
package configaccess

import (
	"context"
	"net/http"
	"strings"

	sdkaccess "github.com/router-for-me/CLIProxyAPI/v7/sdk/access"
	sdkconfig "github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
)

// Register 确保基于配置的访问认证提供者在访问管理器中可用。
// 当 SDKConfig 为 nil 或其中的 APIKeys 为空时，会注销该提供者；
// 否则将标准化后的 Key 列表注册为新的提供者实例。
func Register(cfg *sdkconfig.SDKConfig) {
	if cfg == nil {
		sdkaccess.UnregisterProvider(sdkaccess.AccessProviderTypeConfigAPIKey)
		return
	}

	keys := normalizeKeys(cfg.APIKeys)
	if len(keys) == 0 {
		sdkaccess.UnregisterProvider(sdkaccess.AccessProviderTypeConfigAPIKey)
		return
	}

	sdkaccess.RegisterProvider(
		sdkaccess.AccessProviderTypeConfigAPIKey,
		newProvider(sdkaccess.DefaultAccessProviderName, keys),
	)
}

// provider 是基于配置文件 API Key 的访问认证提供者。
// 它存储一个标准化的 Key 集合（map[string]struct{}），用于快速查找匹配的凭据。
type provider struct {
	name string              // 提供者名称，用于标识和日志记录
	keys map[string]struct{} // 标准化后的 API Key 集合，用于 O(1) 时间复杂度的查找
}

// newProvider 创建一个新的基于配置的访问认证提供者实例。
// 如果传入的名称为空，则使用默认提供者名称。
// 函数会将传入的 Key 切片转换为 map 集合以加速查找。
func newProvider(name string, keys []string) *provider {
	providerName := strings.TrimSpace(name)
	if providerName == "" {
		providerName = sdkaccess.DefaultAccessProviderName
	}
	keySet := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		keySet[key] = struct{}{}
	}
	return &provider{name: providerName, keys: keySet}
}

// Identifier 返回该提供者的唯一标识名称。
// 当 provider 为 nil 或名称为空时，返回默认提供者名称。
func (p *provider) Identifier() string {
	if p == nil || p.name == "" {
		return sdkaccess.DefaultAccessProviderName
	}
	return p.name
}

// Authenticate 从 HTTP 请求中提取凭据并与配置的 API Key 列表进行比对。
// 支持以下凭据来源（按优先级顺序）：
//   - Authorization 头（Bearer token 格式）
//   - X-Goog-Api-Key 头（Google API Key）
//   - X-Api-Key 头（Anthropic API Key）
//   - URL 查询参数 key
//   - URL 查询参数 auth_token
//
// 返回认证成功的结果或对应的认证错误。
func (p *provider) Authenticate(_ context.Context, r *http.Request) (*sdkaccess.Result, *sdkaccess.AuthError) {
	if p == nil {
		return nil, sdkaccess.NewNotHandledError()
	}
	if len(p.keys) == 0 {
		return nil, sdkaccess.NewNotHandledError()
	}
	authHeader := r.Header.Get("Authorization")
	authHeaderGoogle := r.Header.Get("X-Goog-Api-Key")
	authHeaderAnthropic := r.Header.Get("X-Api-Key")
	queryKey := ""
	queryAuthToken := ""
	if r.URL != nil {
		queryKey = r.URL.Query().Get("key")
		queryAuthToken = r.URL.Query().Get("auth_token")
	}
	if authHeader == "" && authHeaderGoogle == "" && authHeaderAnthropic == "" && queryKey == "" && queryAuthToken == "" {
		return nil, sdkaccess.NewNoCredentialsError()
	}

	apiKey := extractBearerToken(authHeader)

	candidates := []struct {
		value  string
		source string
	}{
		{apiKey, "authorization"},
		{authHeaderGoogle, "x-goog-api-key"},
		{authHeaderAnthropic, "x-api-key"},
		{queryKey, "query-key"},
		{queryAuthToken, "query-auth-token"},
	}

	for _, candidate := range candidates {
		if candidate.value == "" {
			continue
		}
		if _, ok := p.keys[candidate.value]; ok {
			return &sdkaccess.Result{
				Provider:  p.Identifier(),
				Principal: candidate.value,
				Metadata: map[string]string{
					"source": candidate.source,
				},
			}, nil
		}
	}

	return nil, sdkaccess.NewInvalidCredentialError()
}

// extractBearerToken 从 Authorization 头中提取 Bearer token。
// 如果头部为空则返回空字符串；如果格式不符合 "Bearer <token>" 则原样返回头部值。
func extractBearerToken(header string) string {
	if header == "" {
		return ""
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 {
		return header
	}
	if strings.ToLower(parts[0]) != "bearer" {
		return header
	}
	return strings.TrimSpace(parts[1])
}

// normalizeKeys 对 API Key 列表进行标准化处理。
// 去除每个 Key 的首尾空白字符，过滤空字符串，并去除重复项。
// 返回标准化后的去重 Key 切片，如果结果为空则返回 nil。
func normalizeKeys(keys []string) []string {
	if len(keys) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		if _, exists := seen[trimmedKey]; exists {
			continue
		}
		seen[trimmedKey] = struct{}{}
		normalized = append(normalized, trimmedKey)
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}
