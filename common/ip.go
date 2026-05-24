// Package common - ip.go
// 该文件提供了 IP 地址相关的工具函数
//
// 包含的功能：
// - IP 地址验证和解析
// - 私有 IP 地址判断
// - CIDR 列表匹配
//
// 使用场景：
// - 请求来源验证
// - IP 白名单/黑名单
// - 内网/外网判断
package common

import "net"

// IsIP 判断字符串是否为有效的 IP 地址
//
// 支持 IPv4 和 IPv6 格式
//
// 参数：
//   - s: 要检查的字符串
//
// 返回值：
//   - bool: 是否为有效的 IP 地址
func IsIP(s string) bool {
	ip := net.ParseIP(s)
	return ip != nil
}

// ParseIP 解析 IP 地址字符串
//
// 参数：
//   - s: IP 地址字符串
//
// 返回值：
//   - net.IP: 解析后的 IP 地址（无效则返回 nil）
func ParseIP(s string) net.IP {
	return net.ParseIP(s)
}

// IsPrivateIP 判断 IP 是否为私有地址
//
// 私有地址包括：
// - 回环地址（127.0.0.0/8）
// - 链路本地地址（169.254.0.0/16）
// - RFC 1918 私有地址：
//   - 10.0.0.0/8
//   - 172.16.0.0/12
//   - 192.168.0.0/16
//
// 参数：
//   - ip: 要检查的 IP 地址
//
// 返回值：
//   - bool: 是否为私有地址
func IsPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}

	private := []net.IPNet{
		{IP: net.IPv4(10, 0, 0, 0), Mask: net.CIDRMask(8, 32)},     // 10.0.0.0/8
		{IP: net.IPv4(172, 16, 0, 0), Mask: net.CIDRMask(12, 32)},   // 172.16.0.0/12
		{IP: net.IPv4(192, 168, 0, 0), Mask: net.CIDRMask(16, 32)},  // 192.168.0.0/16
	}

	for _, privateNet := range private {
		if privateNet.Contains(ip) {
			return true
		}
	}
	return false
}

// IsIpInCIDRList 判断 IP 是否在 CIDR 列表中
//
// 支持两种格式：
// - CIDR 格式（如 "192.168.1.0/24"）
// - 单个 IP 地址（如 "192.168.1.100"）
//
// 参数：
//   - ip: 要检查的 IP 地址
//   - cidrList: CIDR 或 IP 列表
//
// 返回值：
//   - bool: IP 是否在列表中
func IsIpInCIDRList(ip net.IP, cidrList []string) bool {
	for _, cidr := range cidrList {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			// 尝试作为单个 IP 处理
			if whitelistIP := net.ParseIP(cidr); whitelistIP != nil {
				if ip.Equal(whitelistIP) {
					return true
				}
			}
			continue
		}

		if network.Contains(ip) {
			return true
		}
	}
	return false
}
