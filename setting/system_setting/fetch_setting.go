// fetch_setting.go — 外部请求安全策略配置管理
// 职责：管理外部 HTTP 请求的安全策略，包括 SSRF 防护、
// 私有 IP 访问控制、域名/IP 黑白名单过滤和端口限制。
// 通过 config.GlobalConfig 注册实现持久化存储。

package system_setting

import "github.com/c1cada/NexusTok/setting/config"

// FetchSetting 外部请求安全策略配置结构体
type FetchSetting struct {
	// EnableSSRFProtection 是否启用 SSRF（服务端请求伪造）防护
	EnableSSRFProtection bool `json:"enable_ssrf_protection"`
	// AllowPrivateIp 是否允许访问私有 IP 地址
	AllowPrivateIp bool `json:"allow_private_ip"`
	// DomainFilterMode 域名过滤模式，true 为白名单模式，false 为黑名单模式
	DomainFilterMode bool `json:"domain_filter_mode"`
	// IpFilterMode IP 过滤模式，true 为白名单模式，false 为黑名单模式
	IpFilterMode bool `json:"ip_filter_mode"`
	// DomainList 域名过滤列表，支持通配符格式如 example.com、*.example.com
	DomainList []string `json:"domain_list"`
	// IpList IP 过滤列表，CIDR 格式如 192.168.0.0/16
	IpList []string `json:"ip_list"`
	// AllowedPorts 允许的端口列表，支持范围格式如 80、443、8000-9000
	AllowedPorts []string `json:"allowed_ports"`
	// ApplyIPFilterForDomain 是否对域名解析后的 IP 也应用 IP 过滤规则（实验性功能）
	ApplyIPFilterForDomain bool `json:"apply_ip_filter_for_domain"`
}

// defaultFetchSetting 默认的外部请求安全策略配置
var defaultFetchSetting = FetchSetting{
	EnableSSRFProtection:   true, // 默认开启SSRF防护
	AllowPrivateIp:         false,
	DomainFilterMode:       false,
	IpFilterMode:           false,
	DomainList:             []string{},
	IpList:                 []string{},
	AllowedPorts:           []string{"80", "443", "8080", "8443"},
	ApplyIPFilterForDomain: true,
}

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("fetch_setting", &defaultFetchSetting)
}

// GetFetchSetting 获取当前外部请求安全策略配置的指针
// 返回值：指向当前配置的指针
func GetFetchSetting() *FetchSetting {
	return &defaultFetchSetting
}
