// helps - home_refresh_test.go
// Home 刷新错误码状态映射的单元测试。
// 测试 authentication_error 和 unauthorized 错误码被正确映射为 HTTP 401 状态码。
package helps

import (
	"net/http"
	"testing"
)

// TestStatusFromHomeErrorCodeMapsAuthenticationErrorToUnauthorized 测试认证相关错误码
// （authentication_error、unauthorized）被映射为 HTTP 401 状态码
func TestStatusFromHomeErrorCodeMapsAuthenticationErrorToUnauthorized(t *testing.T) {
	if got := statusFromHomeErrorCode("authentication_error"); got != http.StatusUnauthorized {
		t.Fatalf("statusFromHomeErrorCode(authentication_error) = %d, want %d", got, http.StatusUnauthorized)
	}
	if got := statusFromHomeErrorCode("unauthorized"); got != http.StatusUnauthorized {
		t.Fatalf("statusFromHomeErrorCode(unauthorized) = %d, want %d", got, http.StatusUnauthorized)
	}
}
