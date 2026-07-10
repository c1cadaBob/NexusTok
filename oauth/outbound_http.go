package oauth

import (
	"net/http"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/service"
	"github.com/c1cada/NexusTok/setting/system_setting"
)

// validateConfiguredOAuthEndpointURL 使用系统 FetchSetting 校验管理员配置的 OAuth 端点。
//
// 系统 OIDC 和自定义 OAuth 的 token/userinfo endpoint 都来自管理员配置，登录回调会
// 根据这些地址触发服务端出站请求。这里复用统一 FetchSetting，避免错误配置或恶意输入
// 访问回环地址、内网地址、受限端口或被黑名单命中的域名/IP。
func validateConfiguredOAuthEndpointURL(urlStr string) error {
	fetchSetting := system_setting.GetFetchSetting()
	return common.ValidateURLWithFetchSetting(
		urlStr,
		fetchSetting.EnableSSRFProtection,
		fetchSetting.AllowPrivateIp,
		fetchSetting.DomainFilterMode,
		fetchSetting.IpFilterMode,
		fetchSetting.DomainList,
		fetchSetting.IpList,
		fetchSetting.AllowedPorts,
		fetchSetting.ApplyIPFilterForDomain,
	)
}

// newConfiguredOAuthHTTPClient 返回管理员配置 OAuth 端点专用 HTTP client。
//
// 复用 protected fetch client 的 Transport 和 redirect 校验以获得 Dial 阶段 DNS rebinding
// 防护；复制 client 后覆盖 Timeout，保持各 provider 既有超时语义。
func newConfiguredOAuthHTTPClient(timeout time.Duration) *http.Client {
	baseClient := service.GetSSRFProtectedHTTPClient()
	if baseClient == nil {
		return &http.Client{Timeout: timeout}
	}
	client := *baseClient
	client.Timeout = timeout
	return &client
}
