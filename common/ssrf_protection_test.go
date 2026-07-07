package common

import (
	"net"
	"testing"
)

func TestNewSSRFProtectionFromFetchSettingRejectsInvalidPort(t *testing.T) {
	_, err := NewSSRFProtectionFromFetchSetting(false, false, false, nil, nil, []string{"99999"}, true)
	if err == nil {
		t.Fatalf("期望非法端口返回错误")
	}
}

func TestValidateNetworkTargetRejectsPrivateIP(t *testing.T) {
	protection := &SSRFProtection{
		AllowPrivateIp:   false,
		DomainFilterMode: false,
		IpFilterMode:     false,
	}

	err := protection.ValidateNetworkTarget("127.0.0.1", 80)
	if err == nil {
		t.Fatalf("期望私有 IP 被拒绝")
	}
}

func TestValidateNetworkTargetAllowsWhitelistedPrivateIP(t *testing.T) {
	protection := &SSRFProtection{
		AllowPrivateIp:   true,
		DomainFilterMode: false,
		IpFilterMode:     true,
		IpList:           []string{"10.0.0.0/8"},
	}

	if err := protection.ValidateNetworkTarget("10.1.2.3", 443); err != nil {
		t.Fatalf("白名单私有 IP 应允许访问，got %v", err)
	}
}

func TestValidateResolvedIPRejectsPrivateRebinding(t *testing.T) {
	protection := &SSRFProtection{
		AllowPrivateIp:   false,
		DomainFilterMode: false,
		IpFilterMode:     false,
	}

	err := protection.ValidateResolvedIP("safe.example", net.ParseIP("10.0.0.1"))
	if err == nil {
		t.Fatalf("期望解析到私有 IP 的域名被拒绝")
	}
}
