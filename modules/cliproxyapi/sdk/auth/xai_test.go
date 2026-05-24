// auth - xai_test.go
// 本文件包含 XAIAuthenticator 和 parseXAIManualCallbackToken 函数的单元测试。
package auth

import "testing"

// TestXAIAuthenticatorProviderAndRefreshLead 验证 xAI 认证器的 Provider 和 RefreshLead 方法。
// 确保 Provider 返回 "xai"，且 RefreshLead 返回正数时间间隔。
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

// TestParseXAIManualCallbackTokenAcceptsRawCode 验证 parseXAIManualCallbackToken
// 能正确接受纯 Token 输入（带前后空格），并正确设置 Code 和 State。
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

// TestParseXAIManualCallbackTokenRejectsCallbackURL 验证 parseXAIManualCallbackToken
// 能正确拒绝 URL 格式的输入（应仅粘贴纯 Token）。
func TestParseXAIManualCallbackTokenRejectsCallbackURL(t *testing.T) {
	_, _, err := parseXAIManualCallbackToken("http://127.0.0.1:56121/callback?state=state-1&code=token-1", "state-1")
	if err == nil {
		t.Fatal("parseXAIManualCallbackToken() error = nil, want error")
	}
}
