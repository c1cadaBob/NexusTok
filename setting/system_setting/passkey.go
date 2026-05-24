// passkey.go — WebAuthn/Passkey 无密码登录配置管理
// 职责：管理 WebAuthn/Passkey 的相关配置，包括依赖方（RP）
// 信息、来源验证、用户验证策略和认证器偏好。
// 支持从 ServerAddress 自动推导 RP ID 和 Origins。
// 通过 config.GlobalConfig 注册实现持久化存储。

package system_setting

import (
	"net/url"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/setting/config"
)

// PasskeySettings WebAuthn/Passkey 配置结构体
type PasskeySettings struct {
	// Enabled 是否启用 Passkey 登录
	Enabled bool `json:"enabled"`
	// RPDisplayName 依赖方（Relying Party）显示名称
	RPDisplayName string `json:"rp_display_name"`
	// RPID 依赖方标识，通常为域名
	RPID string `json:"rp_id"`
	// Origins 允许的来源 URL 列表（逗号分隔）
	Origins string `json:"origins"`
	// AllowInsecureOrigin 是否允许非 HTTPS 来源
	AllowInsecureOrigin bool `json:"allow_insecure_origin"`
	// UserVerification 用户验证策略，如 "preferred"、"required"、"discouraged"
	UserVerification string `json:"user_verification"`
	// AttachmentPreference 认证器附件偏好，如 "platform"、"cross-platform"
	AttachmentPreference string `json:"attachment_preference"`
}

// defaultPasskeySettings 默认的 Passkey 配置
var defaultPasskeySettings = PasskeySettings{
	Enabled:              false,
	RPDisplayName:        common.SystemName,
	RPID:                 "",
	Origins:              "",
	AllowInsecureOrigin:  false,
	UserVerification:     "preferred",
	AttachmentPreference: "",
}

func init() {
	config.GlobalConfig.Register("passkey", &defaultPasskeySettings)
}

// GetPasskeySettings 获取当前 Passkey 配置的指针
// 如果 RPID 未配置，会尝试从 ServerAddress 自动推导域名作为 RPID
// 如果 Origins 未配置，会使用 ServerAddress 作为默认来源
// 返回值：指向当前配置的指针
func GetPasskeySettings() *PasskeySettings {
	if defaultPasskeySettings.RPID == "" && ServerAddress != "" {
		// 从ServerAddress提取域名作为RPID
		// ServerAddress 可能是 "https://nexustok.example" 这种格式
		serverAddr := strings.TrimSpace(ServerAddress)
		if parsed, err := url.Parse(serverAddr); err == nil && parsed.Host != "" {
			defaultPasskeySettings.RPID = parsed.Host
		} else {
			defaultPasskeySettings.RPID = serverAddr
		}
	}
	if defaultPasskeySettings.Origins == "" || defaultPasskeySettings.Origins == "[]" {
		defaultPasskeySettings.Origins = ServerAddress
	}
	return &defaultPasskeySettings
}
