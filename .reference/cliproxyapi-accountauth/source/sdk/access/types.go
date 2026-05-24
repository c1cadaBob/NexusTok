// 包 access - types.go
// 该文件定义了请求认证配置的数据类型和内置提供者构造函数。
// 包括 AccessConfig、AccessProvider 结构体和内联 API 密钥提供者的构造逻辑。
package access

// AccessConfig 对请求认证提供者进行分组配置。
type AccessConfig struct {
	// Providers 列出已配置的认证提供者。
	Providers []AccessProvider `yaml:"providers,omitempty" json:"providers,omitempty"`
}

// AccessProvider 描述一个请求认证提供者配置条目。
type AccessProvider struct {
	// Name 是提供者的实例标识符。
	Name string `yaml:"name" json:"name"`
	// Type 选择通过 SDK 注册的提供者实现。
	Type string `yaml:"type" json:"type"`
	// SDK 可选地指定提供此提供者的第三方 SDK 模块名称。
	SDK string `yaml:"sdk,omitempty" json:"sdk,omitempty"`
	// APIKeys 列出需要内联密钥的提供者所使用的密钥列表。
	APIKeys []string `yaml:"api-keys,omitempty" json:"api-keys,omitempty"`
	// Config 将提供者特定的选项传递给实现。
	Config map[string]any `yaml:"config,omitempty" json:"config,omitempty"`
}

const (
	// AccessProviderTypeConfigAPIKey 是内置的内联 API 密钥验证提供者类型。
	AccessProviderTypeConfigAPIKey = "config-api-key"
	// DefaultAccessProviderName 是未提供提供者名称时使用的默认名称。
	DefaultAccessProviderName = "config-inline"
)

// MakeInlineAPIKeyProvider 构造一个内联 API 密钥提供者配置。
// 当未提供密钥时返回 nil。
//
// 参数:
//   - keys: API 密钥列表
//
// 返回:
//   - *AccessProvider: 提供者配置；无密钥时返回 nil
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
