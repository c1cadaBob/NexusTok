// 包 access - errors.go
// 该文件定义了请求认证流程中的错误类型和错误码。
// 包括认证错误码分类、AuthError 结构体及各类认证失败的构造函数。
package access

import (
	"fmt"
	"net/http"
	"strings"
)

// AuthErrorCode 对认证失败进行分类。
type AuthErrorCode string

const (
	// AuthErrorCodeNoCredentials 表示请求中缺少认证凭据。
	AuthErrorCodeNoCredentials AuthErrorCode = "no_credentials"
	// AuthErrorCodeInvalidCredential 表示提供的认证凭据无效。
	AuthErrorCodeInvalidCredential AuthErrorCode = "invalid_credential"
	// AuthErrorCodeNotHandled 表示当前认证提供者不处理此请求。
	AuthErrorCodeNotHandled AuthErrorCode = "not_handled"
	// AuthErrorCodeInternal 表示认证服务内部错误。
	AuthErrorCodeInternal AuthErrorCode = "internal_error"
)

// AuthError 携带认证失败的详细信息和 HTTP 状态码。
type AuthError struct {
	Code       AuthErrorCode // 错误码分类
	Message    string        // 人类可读的错误消息
	StatusCode int           // HTTP 状态码
	Cause      error         // 底层错误原因
}

// Error 返回错误的字符串表示。
func (e *AuthError) Error() string {
	if e == nil {
		return ""
	}
	message := strings.TrimSpace(e.Message)
	if message == "" {
		message = "authentication error"
	}
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", message, e.Cause)
	}
	return message
}

// Unwrap 返回底层错误原因，支持 Go 1.13+ 的 errors.Unwrap 链式错误检查。
func (e *AuthError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// HTTPStatusCode 返回安全的 HTTP 状态码。缺失时回退到 500 内部服务器错误。
//
// 返回:
//   - int: HTTP 状态码
func (e *AuthError) HTTPStatusCode() int {
	if e == nil || e.StatusCode <= 0 {
		return http.StatusInternalServerError
	}
	return e.StatusCode
}

// newAuthError 构造一个新的认证错误实例。
func newAuthError(code AuthErrorCode, message string, statusCode int, cause error) *AuthError {
	return &AuthError{
		Code:       code,
		Message:    message,
		StatusCode: statusCode,
		Cause:      cause,
	}
}

// NewNoCredentialsError 创建缺少 API 密钥的认证错误（HTTP 401）。
//
// 返回:
//   - *AuthError: 认证错误实例
func NewNoCredentialsError() *AuthError {
	return newAuthError(AuthErrorCodeNoCredentials, "Missing API key", http.StatusUnauthorized, nil)
}

// NewInvalidCredentialError 创建 API 密钥无效的认证错误（HTTP 401）。
//
// 返回:
//   - *AuthError: 认证错误实例
func NewInvalidCredentialError() *AuthError {
	return newAuthError(AuthErrorCodeInvalidCredential, "Invalid API key", http.StatusUnauthorized, nil)
}

// NewNotHandledError 创建认证提供者未处理请求的错误。
//
// 返回:
//   - *AuthError: 认证错误实例
func NewNotHandledError() *AuthError {
	return newAuthError(AuthErrorCodeNotHandled, "authentication provider did not handle request", 0, nil)
}

// NewInternalAuthError 创建认证服务内部错误（HTTP 500）。
//
// 参数:
//   - message: 错误消息（为空时使用默认消息）
//   - cause: 底层错误原因
//
// 返回:
//   - *AuthError: 认证错误实例
func NewInternalAuthError(message string, cause error) *AuthError {
	normalizedMessage := strings.TrimSpace(message)
	if normalizedMessage == "" {
		normalizedMessage = "Authentication service error"
	}
	return newAuthError(AuthErrorCodeInternal, normalizedMessage, http.StatusInternalServerError, cause)
}

// IsAuthErrorCode 检查认证错误是否匹配指定的错误码。
//
// 参数:
//   - authErr: 待检查的认证错误（可为 nil）
//   - code: 期望的错误码
//
// 返回:
//   - bool: 如果错误码匹配返回 true
func IsAuthErrorCode(authErr *AuthError, code AuthErrorCode) bool {
	if authErr == nil {
		return false
	}
	return authErr.Code == code
}
