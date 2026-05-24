// access - types.go
// 本文件定义了请求认证系统的配置类型和内置提供者类型常量。
// 包含 AccessConfig（认证配置）、AccessProvider（提供者描述）等配置结构，
// 以及用于构建内置 API Key 提供者的辅助函数。
package access

// AccessConfig 封装请求认证的配置信息。
// 用于从 YAML/JSON 配置文件中加载认证提供者列表。
type AccessConfig struct {
	// Providers 是配置的认证提供者列表。
	Providers []AccessProvider `yaml:"providers,omitempty" json:"providers,omitempty"`
}

// AccessProvider 描述一个请求认证提供者的配置条目。
type AccessProvider struct {
	// Name 是该提供者实例的标识名称，用于区分同一类型的不同实例。
	Name string `yaml:"name" json:"name"`

	// Type 选择通过 SDK 注册的提供者实现类型。
	// 例如 "config-api-key" 表示使用内置的 API Key 验证提供者。
	Type string `yaml:"type" json:"type"`

	// SDK 可选地指定提供该提供者的第三方 SDK 模块名称。
	SDK string `yaml:"sdk,omitempty" json:"sdk,omitempty"`

	// APIKeys 列出内联的 API Key，用于需要预置密钥的提供者。
	APIKeys []string `yaml:"api-keys,omitempty" json:"api-keys,omitempty"`

	// Config 传递提供者特定的选项给实现。
	// 不同的提供者类型可能需要不同的配置项。
	Config map[string]any `yaml:"config,omitempty" json:"config,omitempty"`
}

const (
	// AccessProviderTypeConfigAPIKey 是内置的 API Key 验证提供者类型标识。
	// 该提供者通过比对请求中的 API Key 与预置的 Key 列表来验证身份。
	AccessProviderTypeConfigAPIKey = "config-api-key"

	// DefaultAccessProviderName 是未指定提供者名称时使用的默认名称。
	DefaultAccessProviderName = "config-inline"
)

// MakeInlineAPIKeyProvider 构建一个内联 API Key 提供者配置。
// 当提供的 Key 列表为空时返回 nil。
// 参数说明：
//   - keys: 预置的 API Key 列表
//
// 返回值为配置好的 AccessProvider 指针，或 nil（当 keys 为空时）。
func MakeInlineAPIKeyProvider(keys []string) *AccessProvider {
	if len(keys) == 0 {
		return nil
	}
	provider := &AccessProvider{
		Name:    DefaultAccessProviderName,
		Type:    AccessProviderTypeConfigAPIKey,
		APIKeys: append([]string(nil), keys...),
	}
	return provider
}
