// discord.go — Discord OAuth 登录配置管理
// 职责：管理 Discord 第三方登录的 OAuth 配置，包括客户端 ID 和密钥。
// 通过 config.GlobalConfig 注册实现持久化存储。

package system_setting

import "github.com/c1cada/NexusTok/setting/config"

// DiscordSettings Discord OAuth 配置结构体
type DiscordSettings struct {
	// Enabled 是否启用 Discord 登录
	Enabled bool `json:"enabled"`
	// ClientId Discord 应用的客户端 ID
	ClientId string `json:"client_id"`
	// ClientSecret Discord 应用的客户端密钥
	ClientSecret string `json:"client_secret"`
}

// 默认配置
var defaultDiscordSettings = DiscordSettings{}

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("discord", &defaultDiscordSettings)
}

// GetDiscordSettings 获取当前 Discord OAuth 配置的指针
// 返回值：指向当前配置的指针
func GetDiscordSettings() *DiscordSettings {
	return &defaultDiscordSettings
}
