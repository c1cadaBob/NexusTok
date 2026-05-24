// legal.go — 法律条款配置管理
// 职责：管理用户协议和隐私政策的内容配置。
// 通过 config.GlobalConfig 注册实现持久化存储。

package system_setting

import "github.com/c1cada/NexusTok/setting/config"

// LegalSettings 法律条款配置结构体
type LegalSettings struct {
	// UserAgreement 用户协议的内容（支持 HTML 或 Markdown）
	UserAgreement string `json:"user_agreement"`
	// PrivacyPolicy 隐私政策的内容（支持 HTML 或 Markdown）
	PrivacyPolicy string `json:"privacy_policy"`
}

// defaultLegalSettings 法律条款的默认配置（均为空字符串）
var defaultLegalSettings = LegalSettings{
	UserAgreement: "",
	PrivacyPolicy: "",
}

// init 注册法律条款配置到全局配置管理系统
func init() {
	config.GlobalConfig.Register("legal", &defaultLegalSettings)
}

// GetLegalSettings 获取当前法律条款配置的指针
// 返回值：指向当前配置的指针
func GetLegalSettings() *LegalSettings {
	return &defaultLegalSettings
}
