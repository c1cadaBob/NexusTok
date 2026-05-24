// auth - xai_test.go
// XAI 认证器单元测试
// 测试 XAI (xAI) 提供商的认证器基本功能，包括：
// - 提供商标识符验证
// - 刷新提前量（RefreshLead）配置验证
// - 手动回调令牌解析（支持原始授权码和回调 URL 格式）
package auth

import "testing"

// TestXAIAuthenticatorProviderAndRefreshLead 验证 XAI 认证器：
// 1. Provider() 方法返回 "xai"
// 2. RefreshLead() 方法返回正数时间间隔（用于提前刷新令牌）
func TestXAIAuthenticatorProviderAndRefreshLead(t *testing.T) {
	authenticator := NewXAIAuthenticator()
	if authenticator.Provider() != "xai" {
		t.Fatalf("Provider() = %q, want xai", authenticator.Provider())
	}
	lead := authenticator.RefreshLead()
	if lead == nil || *lead <= 0 {
		t.Fatalf("RefreshLead() = %v, want positive duration", lead)
	}
}

// TestParseXAIManualCallbackTokenAcceptsRawCode 验证解析函数能够接受
// 原始授权码字符串（带前后空格），正确去除空格并保留原始 code 和 state 值。
func TestParseXAIManualCallbackTokenAcceptsRawCode(t *testing.T) {
	result, ok, err := parseXAIManualCallbackToken(" V0auoESADonzF4bY_Ag2whBFnVeqzHJm6nW2uW012rqCCW5cstFV58qvDFBvnPBXXe0rZSKOcs3PwwfACKp1qg ", "state-1")
	if err != nil {
		t.Fatalf("parseXAIManualCallbackToken() error = %v", err)
	}
	if !ok {
		t.Fatal("parseXAIManualCallbackToken() ok = false, want true")
	}
	if result.Code != "V0auoESADonzF4bY_Ag2whBFnVeqzHJm6nW2uW012rqCCW5cstFV58qvDFBvnPBXXe0rZSKOcs3PwwfACKp1qg" {
		t.Fatalf("Code = %q", result.Code)
	}
	if result.State != "state-1" {
		t.Fatalf("State = %q, want state-1", result.State)
	}
}

// TestParseXAIManualCallbackTokenRejectsCallbackURL 验证解析函数能够
// 正确拒绝完整的回调 URL 格式（包含 http:// 前缀），应返回错误。
func TestParseXAIManualCallbackTokenRejectsCallbackURL(t *testing.T) {
	_, _, err := parseXAIManualCallbackToken("http://127.0.0.1:56121/callback?state=state-1&code=token-1", "state-1")
	if err == nil {
		t.Fatal("parseXAIManualCallbackToken() error = nil, want error")
	}
}
