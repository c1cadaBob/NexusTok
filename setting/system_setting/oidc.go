// oidc.go — OIDC（OpenID Connect）登录配置管理
// 职责：管理通用 OIDC 第三方登录的配置，支持自定义
// Well-Known 端点或手动指定授权/令牌/用户信息端点。
// 通过 config.GlobalConfig 注册实现持久化存储。

package system_setting

import "github.com/c1cada/NexusTok/setting/config"

// OIDCSettings OIDC 登录配置结构体
type OIDCSettings struct {
	// Enabled 是否启用 OIDC 登录
	Enabled bool `json:"enabled"`
	// ClientId OIDC 应用的客户端 ID
	ClientId string `json:"client_id"`
	// ClientSecret OIDC 应用的客户端密钥
	ClientSecret string `json:"client_secret"`
	// WellKnown OIDC 的 Well-Known 端点 URL，用于自动发现其他端点
	WellKnown string `json:"well_known"`
	// AuthorizationEndpoint 授权端点 URL（Well-Known 为空时使用）
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	// TokenEndpoint 令牌端点 URL（Well-Known 为空时使用）
	TokenEndpoint string `json:"token_endpoint"`
	// UserInfoEndpoint 用户信息端点 URL（Well-Known 为空时使用）
	UserInfoEndpoint string `json:"user_info_endpoint"`
}

// 默认配置
var defaultOIDCSettings = OIDCSettings{}

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("oidc", &defaultOIDCSettings)
}

// GetOIDCSettings 获取当前 OIDC 登录配置的指针
// 返回值：指向当前配置的指针
func GetOIDCSettings() *OIDCSettings {
	return &defaultOIDCSettings
}
