// Package common - url_validator.go
// 该文件实现了重定向 URL 的安全验证功能
//
// 防止开放重定向（Open Redirect）攻击：
// 攻击者可能构造恶意的重定向 URL，将用户引导到钓鱼网站
// 通过验证重定向 URL 的域名是否在可信域名列表中，确保安全性
//
// 验证规则：
// 1. URL 格式必须合法
// 2. 协议必须是 http 或 https
// 3. 域名必须在可信域名列表中（精确匹配或子域名匹配）
package common

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/c1cada/NexusTok/constant"
)

// ValidateRedirectURL 验证重定向 URL 是否安全
//
// 安全检查项：
// - URL 格式是否合法
// - 协议是否为 http 或 https（阻止 javascript: 等危险协议）
// - 域名是否在可信域名列表中（防止开放重定向）
//
// 域名匹配规则：
// - 精确匹配：domain == trustedDomain
// - 子域名匹配：domain 以 .trustedDomain 结尾
//   - 例如：app.example.com 匹配 example.com
//
// 参数：
//   - rawURL: 待验证的重定向 URL
//
// 返回值：
//   - error: 验证错误（nil 表示 URL 安全）
func ValidateRedirectURL(rawURL string) error {
	// 解析 URL
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL format: %s", err.Error())
	}

	// 只允许 http 和 https 协议
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("invalid URL scheme: only http and https are allowed")
	}

	// 获取域名（转小写以进行不区分大小写的比较）
	domain := strings.ToLower(parsedURL.Hostname())

	// 检查域名是否在可信域名列表中
	for _, trustedDomain := range constant.TrustedRedirectDomains {
		// 精确匹配或子域名匹配
		if domain == trustedDomain || strings.HasSuffix(domain, "."+trustedDomain) {
			return nil
		}
	}

	return fmt.Errorf("domain %s is not in the trusted domains list", domain)
}
