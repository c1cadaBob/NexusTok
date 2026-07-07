// Package common - ssrf_protection.go
// 该文件实现了 SSRF（Server-Side Request Forgery）攻击防护
//
// 说明：SSRF 是一种攻击方式，攻击者通过构造恶意 URL，使服务器发起请求到内部网络或其他受限资源
//
// 防护措施：
// - 协议限制：只允许 HTTP/HTTPS 协议
// - 端口限制：可配置允许的端口范围
// - IP 过滤：支持白名单/黑名单模式，阻止私有/保留 IP
// - 域名过滤：支持白名单/黑名单模式，支持通配符匹配
// - DNS 解析验证：可选对域名进行 DNS 解析后验证 IP
//
// 使用场景：
// - Webhook URL 验证
// - 上游 API 地址验证
// - 用户自定义 URL 验证
package common

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// SSRFProtection SSRF 防护配置
type SSRFProtection struct {
	AllowPrivateIp         bool     // 是否允许私有 IP 地址
	DomainFilterMode       bool     // 域名过滤模式（true: 白名单, false: 黑名单）
	DomainList             []string // 域名列表（支持通配符，如 *.example.com）
	IpFilterMode           bool     // IP 过滤模式（true: 白名单, false: 黑名单）
	IpList                 []string // IP/CIDR 列表
	AllowedPorts           []int    // 允许的端口列表
	ApplyIPFilterForDomain bool     // 是否对域名启用 IP 过滤（DNS 解析后验证）
}

// DefaultSSRFProtection 默认 SSRF 防护配置
var DefaultSSRFProtection = &SSRFProtection{
	AllowPrivateIp:   false,      // 默认不允许私有 IP
	DomainFilterMode: true,       // 默认白名单模式
	DomainList:       []string{}, // 默认空列表
	IpFilterMode:     true,       // 默认白名单模式
	IpList:           []string{}, // 默认空列表
	AllowedPorts:     []int{},    // 默认允许所有端口
}

// NewSSRFProtectionFromFetchSetting 根据持久化的 FetchSetting 字段构建 SSRF 防护配置。
//
// 该 helper 将端口范围解析集中到一处，供 URL 预校验和受保护 HTTP client 的
// Dial 阶段校验共同使用，避免两个入口对同一批配置产生不同解释。
func NewSSRFProtectionFromFetchSetting(allowPrivateIp bool, domainFilterMode bool, ipFilterMode bool, domainList, ipList, allowedPorts []string, applyIPFilterForDomain bool) (*SSRFProtection, error) {
	allowedPortInts, err := parsePortRanges(allowedPorts)
	if err != nil {
		return nil, fmt.Errorf("request reject - invalid port configuration: %v", err)
	}

	return &SSRFProtection{
		AllowPrivateIp:         allowPrivateIp,
		DomainFilterMode:       domainFilterMode,
		DomainList:             domainList,
		IpFilterMode:           ipFilterMode,
		IpList:                 ipList,
		AllowedPorts:           allowedPortInts,
		ApplyIPFilterForDomain: applyIPFilterForDomain,
	}, nil
}

// privateIPv4Nets IPv4 私有/保留/特殊用途网段
// 参考 IANA IPv4 Special-Purpose Address Registry
// https://www.iana.org/assignments/iana-ipv4-special-registry/
var privateIPv4Nets = []net.IPNet{
	{IP: net.IPv4(0, 0, 0, 0), Mask: net.CIDRMask(8, 32)},          // 0.0.0.0/8 ("This network" / 未指定)
	{IP: net.IPv4(10, 0, 0, 0), Mask: net.CIDRMask(8, 32)},         // 10.0.0.0/8 (私有)
	{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)},      // 100.64.0.0/10 (运营商级 NAT / CGNAT)
	{IP: net.IPv4(127, 0, 0, 0), Mask: net.CIDRMask(8, 32)},        // 127.0.0.0/8 (回环)
	{IP: net.IPv4(169, 254, 0, 0), Mask: net.CIDRMask(16, 32)},     // 169.254.0.0/16 (链路本地)
	{IP: net.IPv4(172, 16, 0, 0), Mask: net.CIDRMask(12, 32)},      // 172.16.0.0/12 (私有)
	{IP: net.IPv4(192, 0, 0, 0), Mask: net.CIDRMask(24, 32)},       // 192.0.0.0/24 (IETF 协议分配)
	{IP: net.IPv4(192, 0, 2, 0), Mask: net.CIDRMask(24, 32)},       // 192.0.2.0/24 (TEST-NET-1)
	{IP: net.IPv4(192, 168, 0, 0), Mask: net.CIDRMask(16, 32)},     // 192.168.0.0/16 (私有)
	{IP: net.IPv4(198, 18, 0, 0), Mask: net.CIDRMask(15, 32)},      // 198.18.0.0/15 (基准测试)
	{IP: net.IPv4(198, 51, 100, 0), Mask: net.CIDRMask(24, 32)},    // 198.51.100.0/24 (TEST-NET-2)
	{IP: net.IPv4(203, 0, 113, 0), Mask: net.CIDRMask(24, 32)},     // 203.0.113.0/24 (TEST-NET-3)
	{IP: net.IPv4(224, 0, 0, 0), Mask: net.CIDRMask(4, 32)},        // 224.0.0.0/4 (组播)
	{IP: net.IPv4(240, 0, 0, 0), Mask: net.CIDRMask(4, 32)},        // 240.0.0.0/4 (保留)
	{IP: net.IPv4(255, 255, 255, 255), Mask: net.CIDRMask(32, 32)}, // 255.255.255.255/32 (受限广播)
}

// privateIPv6Nets IPv6 私有/保留/特殊用途网段
// 参考 IANA IPv6 Special-Purpose Address Registry
// https://www.iana.org/assignments/iana-ipv6-special-registry/
var privateIPv6Nets = func() []net.IPNet {
	cidrs := []string{
		"::/128",        // 未指定地址
		"::1/128",       // 回环
		"::ffff:0:0/96", // IPv4-mapped
		"64:ff9b::/96",  // IPv4/IPv6 translation
		"100::/64",      // Discard-Only
		"2001::/23",     // IETF Protocol Assignments
		"2001:db8::/32", // 文档
		"fc00::/7",      // Unique Local Address (ULA)
		"fe80::/10",     // 链路本地
		"ff00::/8",      // 组播
	}
	nets := make([]net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		if _, n, err := net.ParseCIDR(c); err == nil && n != nil {
			nets = append(nets, *n)
		}
	}
	return nets
}()

// isPrivateIP 检查 IP 是否为私有/保留/特殊用途地址
//
// 检查范围：
// - 未指定地址（0.0.0.0, ::）
// - 回环地址（127.0.0.1, ::1）
// - 链路本地地址（169.254.x.x, fe80::）
// - IPv4 私有地址（10.x.x.x, 172.16.x.x, 192.168.x.x）
// - IPv6 私有地址（fc00::/7）
// - 组播地址
// - 其他 IANA 保留地址
//
// 参数：
//   - ip: 要检查的 IP 地址
//
// 返回值：
//   - bool: 是否为私有/保留地址
func isPrivateIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	// 未指定地址 (0.0.0.0, ::)
	if ip.IsUnspecified() {
		return true
	}
	// 回环、链路本地 (unicast/multicast)
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	// 接口本地组播 (IPv6 ff01::/16 等)
	if ip.IsInterfaceLocalMulticast() {
		return true
	}

	// IPv4 检查
	if v4 := ip.To4(); v4 != nil {
		for _, privateNet := range privateIPv4Nets {
			if privateNet.Contains(v4) {
				return true
			}
		}
		return false
	}

	// IPv6 检查
	for _, privateNet := range privateIPv6Nets {
		if privateNet.Contains(ip) {
			return true
		}
	}
	// 兜底: Go 标准库识别的其他私有地址
	if ip.IsPrivate() {
		return true
	}
	return false
}

// parsePortRanges 解析端口范围配置
//
// 支持的格式：
// - 单个端口："80"
// - 端口范围："8000-9000"
//
// 参数：
//   - portConfigs: 端口配置字符串列表
//
// 返回值：
//   - []int: 解析后的端口列表
//   - error: 解析错误
func parsePortRanges(portConfigs []string) ([]int, error) {
	var ports []int

	for _, config := range portConfigs {
		config = strings.TrimSpace(config)
		if config == "" {
			continue
		}

		if strings.Contains(config, "-") {
			// 处理端口范围 "8000-9000"
			parts := strings.Split(config, "-")
			if len(parts) != 2 {
				return nil, fmt.Errorf("invalid port range format: %s", config)
			}

			startPort, err := strconv.Atoi(strings.TrimSpace(parts[0]))
			if err != nil {
				return nil, fmt.Errorf("invalid start port in range %s: %v", config, err)
			}

			endPort, err := strconv.Atoi(strings.TrimSpace(parts[1]))
			if err != nil {
				return nil, fmt.Errorf("invalid end port in range %s: %v", config, err)
			}

			if startPort > endPort {
				return nil, fmt.Errorf("invalid port range %s: start port cannot be greater than end port", config)
			}

			if startPort < 1 || startPort > 65535 || endPort < 1 || endPort > 65535 {
				return nil, fmt.Errorf("port range %s contains invalid port numbers (must be 1-65535)", config)
			}

			// 添加范围内的所有端口
			for port := startPort; port <= endPort; port++ {
				ports = append(ports, port)
			}
		} else {
			// 处理单个端口 "80"
			port, err := strconv.Atoi(config)
			if err != nil {
				return nil, fmt.Errorf("invalid port number: %s", config)
			}

			if port < 1 || port > 65535 {
				return nil, fmt.Errorf("invalid port number %d (must be 1-65535)", port)
			}

			ports = append(ports, port)
		}
	}

	return ports, nil
}

// isAllowedPort 检查端口是否被允许
//
// 参数：
//   - port: 端口号
//
// 返回值：
//   - bool: 是否允许
func (p *SSRFProtection) isAllowedPort(port int) bool {
	if len(p.AllowedPorts) == 0 {
		return true // 如果没有配置端口限制，则允许所有端口
	}

	for _, allowedPort := range p.AllowedPorts {
		if port == allowedPort {
			return true
		}
	}
	return false
}

// isDomainListed 检查域名是否在列表中
//
// 支持精确匹配和通配符匹配（*.example.com）
//
// 参数：
//   - domain: 域名
//   - list: 域名列表
//
// 返回值：
//   - bool: 是否在列表中
func isDomainListed(domain string, list []string) bool {
	if len(list) == 0 {
		return false
	}

	domain = strings.ToLower(domain)
	for _, item := range list {
		item = strings.ToLower(strings.TrimSpace(item))
		if item == "" {
			continue
		}
		// 精确匹配
		if domain == item {
			return true
		}
		// 通配符匹配 (*.example.com)
		if strings.HasPrefix(item, "*.") {
			suffix := strings.TrimPrefix(item, "*.")
			if strings.HasSuffix(domain, "."+suffix) || domain == suffix {
				return true
			}
		}
	}
	return false
}

// isDomainAllowed 检查域名是否允许访问
//
// 根据 DomainFilterMode 决定是白名单还是黑名单模式
//
// 参数：
//   - domain: 域名
//
// 返回值：
//   - bool: 是否允许
func (p *SSRFProtection) isDomainAllowed(domain string) bool {
	listed := isDomainListed(domain, p.DomainList)
	if p.DomainFilterMode { // 白名单
		return listed
	}
	// 黑名单
	return !listed
}

// isIPListed 检查 IP 是否在列表中
//
// 参数：
//   - ip: IP 地址
//   - list: IP/CIDR 列表
//
// 返回值：
//   - bool: 是否在列表中
func isIPListed(ip net.IP, list []string) bool {
	if len(list) == 0 {
		return false
	}

	return IsIpInCIDRList(ip, list)
}

// IsIPAccessAllowed 检查 IP 是否允许访问
//
// 检查流程：
// 1. 如果是私有 IP 且不允许私有 IP，拒绝
// 2. 根据 IpFilterMode 决定是白名单还是黑名单模式
//
// 参数：
//   - ip: IP 地址
//
// 返回值：
//   - bool: 是否允许
func (p *SSRFProtection) IsIPAccessAllowed(ip net.IP) bool {
	// 私有 IP 限制
	if isPrivateIP(ip) && !p.AllowPrivateIp {
		return false
	}

	listed := isIPListed(ip, p.IpList)
	if p.IpFilterMode { // 白名单
		return listed
	}
	// 黑名单
	return !listed
}

// ipAccessError 根据 IP 过滤失败原因生成稳定错误信息。
//
// host 为空表示用户直接访问 IP；host 非空表示域名解析到了该 IP。将两种场景
// 区分开，方便日志和测试判断是 URL 输入本身不安全，还是 DNS 解析/重绑定导致。
func (p *SSRFProtection) ipAccessError(host string, ip net.IP) error {
	if host != "" {
		if isPrivateIP(ip) && !p.AllowPrivateIp {
			return fmt.Errorf("private IP address not allowed: %s resolves to %s", host, ip.String())
		}
		if p.IpFilterMode {
			return fmt.Errorf("ip not in whitelist: %s resolves to %s", host, ip.String())
		}
		return fmt.Errorf("ip in blacklist: %s resolves to %s", host, ip.String())
	}

	if isPrivateIP(ip) && !p.AllowPrivateIp {
		return fmt.Errorf("private IP address not allowed: %s", ip.String())
	}
	if p.IpFilterMode {
		return fmt.Errorf("ip not in whitelist: %s", ip.String())
	}
	return fmt.Errorf("ip in blacklist: %s", ip.String())
}

// ValidateNetworkTarget 在真正建立网络连接前校验 host 与 port。
//
// 该方法不解析域名；它只处理端口、直接 IP、域名黑白名单。对域名解析后的 IP
// 校验由 ValidateResolvedIP 完成，这样 protected fetch client 可以在 Dial 阶段
// 使用同一套规则阻断 DNS rebinding。
func (p *SSRFProtection) ValidateNetworkTarget(host string, port int) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return fmt.Errorf("invalid host")
	}
	if port < 1 || port > 65535 {
		return fmt.Errorf("invalid port: %d", port)
	}
	if !p.isAllowedPort(port) {
		return fmt.Errorf("port %d is not allowed", port)
	}

	if ip := net.ParseIP(host); ip != nil {
		if !p.IsIPAccessAllowed(ip) {
			return p.ipAccessError("", ip)
		}
		return nil
	}

	if !p.isDomainAllowed(host) {
		if p.DomainFilterMode {
			return fmt.Errorf("domain not in whitelist: %s", host)
		}
		return fmt.Errorf("domain in blacklist: %s", host)
	}
	return nil
}

// ValidateResolvedIP 校验域名解析后的候选 IP。
//
// 该方法应在发起 Dial 前尽可能靠近网络连接处调用，用于弥补 URL 预校验和真正
// 连接之间的 DNS 解析变化窗口。
func (p *SSRFProtection) ValidateResolvedIP(host string, ip net.IP) error {
	if !p.IsIPAccessAllowed(ip) {
		return p.ipAccessError(host, ip)
	}
	return nil
}

// ValidateURL 验证 URL 是否安全
//
// 验证流程：
// 1. 解析 URL
// 2. 检查协议（只允许 HTTP/HTTPS）
// 3. 解析主机和端口
// 4. 检查端口是否允许
// 5. 如果是 IP，检查 IP 是否允许
// 6. 如果是域名，检查域名是否允许
// 7. 如果启用了 ApplyIPFilterForDomain，DNS 解析后检查 IP
//
// 参数：
//   - urlStr: URL 字符串
//
// 返回值：
//   - error: 验证错误（nil 表示安全）
func (p *SSRFProtection) ValidateURL(urlStr string) error {
	// 解析 URL
	u, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL format: %v", err)
	}

	// 只允许 HTTP/HTTPS 协议
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported protocol: %s (only http/https allowed)", u.Scheme)
	}

	// 解析主机和端口
	host, portStr, err := net.SplitHostPort(u.Host)
	if err != nil {
		// 没有端口，使用默认端口
		host = u.Hostname()
		if u.Scheme == "https" {
			portStr = "443"
		} else {
			portStr = "80"
		}
	}

	// 验证端口
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("invalid port: %s", portStr)
	}

	if err := p.ValidateNetworkTarget(host, port); err != nil {
		return err
	}

	// 如果 host 是 IP，或未启用域名解析后的 IP 过滤，则到此通过。
	if net.ParseIP(host) != nil || !p.ApplyIPFilterForDomain {
		return nil
	}

	// 解析域名对应 IP 并检查
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("DNS resolution failed for %s: %v", host, err)
	}
	for _, ip := range ips {
		if err := p.ValidateResolvedIP(host, ip); err != nil {
			return err
		}
	}
	return nil
}

// ValidateURLWithFetchSetting 使用 FetchSetting 配置验证 URL
//
// 这是一个便捷函数，从配置参数创建 SSRFProtection 并验证 URL
//
// 参数：
//   - urlStr: URL 字符串
//   - enableSSRFProtection: 是否启用 SSRF 防护
//   - allowPrivateIp: 是否允许私有 IP
//   - domainFilterMode: 域名过滤模式（true: 白名单, false: 黑名单）
//   - ipFilterMode: IP 过滤模式（true: 白名单, false: 黑名单）
//   - domainList: 域名列表
//   - ipList: IP/CIDR 列表
//   - allowedPorts: 端口配置列表
//   - applyIPFilterForDomain: 是否对域名启用 IP 过滤
//
// 返回值：
//   - error: 验证错误
func ValidateURLWithFetchSetting(urlStr string, enableSSRFProtection, allowPrivateIp bool, domainFilterMode bool, ipFilterMode bool, domainList, ipList, allowedPorts []string, applyIPFilterForDomain bool) error {
	// 如果 SSRF 防护被禁用，直接返回成功
	if !enableSSRFProtection {
		return nil
	}

	protection, err := NewSSRFProtectionFromFetchSetting(allowPrivateIp, domainFilterMode, ipFilterMode, domainList, ipList, allowedPorts, applyIPFilterForDomain)
	if err != nil {
		return err
	}
	return protection.ValidateURL(urlStr)
}
