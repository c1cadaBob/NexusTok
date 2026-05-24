// access - errors.go
// 本文件定义了请求认证系统的错误类型和错误辅助函数。
// 包含 AuthErrorCode 错误分类码、AuthError 错误结构体，
// 以及用于创建和判断各类认证错误的工厂函数。
//
// Package access 提供请求认证和访问控制功能。
// 该包定义了认证提供者（Provider）接口、认证结果、错误类型，
// 以及用于管理认证提供者的注册表和管理器。
package access

import (
	"fmt"
	"net/http"
	"strings"
)

// AuthErrorCode 认证错误分类码，用于区分不同类型的认证失败原因。
type AuthErrorCode string

const (
	// AuthErrorCodeNoCredentials 表示请求中缺少凭证（如未提供 API Key）。
	AuthErrorCodeNoCredentials AuthErrorCode = "no_credentials"
	// AuthErrorCodeInvalidCredential 表示提供的凭证无效。
	AuthErrorCodeInvalidCredential AuthErrorCode = "invalid_credential"
	// AuthErrorCodeNotHandled 表示当前认证提供者不处理该请求（应交由下一个提供者处理）。
	AuthErrorCodeNotHandled AuthErrorCode = "not_handled"
	// AuthErrorCodeInternal 表示认证服务内部错误。
	AuthErrorCodeInternal AuthErrorCode = "internal_error"
)

// AuthError 携带认证失败的详细信息和对应的 HTTP 状态码。
// 实现了 error 接口和 Unwrap 方法，支持错误链。
type AuthError struct {
	Code       AuthErrorCode // 错误分类码
	Message    string        // 人类可读的错误描述
	StatusCode int           // 对应的 HTTP 状态码
	Cause      error         // 底层错误（用于错误链包装）
}

// Error 返回认证错误的字符串表示。
// 如果存在底层错误，则附加到消息后面。
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

// Unwrap 返回底层错误，支持 Go 1.13+ 的错误链（errors.Is / errors.As）。
func (e *AuthError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// HTTPStatusCode returns a safe fallback for missing status codes.
func (e *AuthError) HTTPStatusCode() int {
	if e == nil || e.StatusCode <= 0 {
		return http.StatusInternalServerError
	}
	return e.StatusCode
}

// newAuthError 是内部工厂函数，创建一个带有完整信息的 AuthError 实例。
func newAuthError(code AuthErrorCode, message string, statusCode int, cause error) *AuthError {
	return &AuthError{
		Code:       code,
		Message:    message,
		StatusCode: statusCode,
		Cause:      cause,
	}
}

// NewNoCredentialsError 创建一个表示缺少凭证的认证错误（HTTP 401）。
func NewNoCredentialsError() *AuthError {
	return newAuthError(AuthErrorCodeNoCredentials, "Missing API key", http.StatusUnauthorized, nil)
}

// NewInvalidCredentialError 创建一个表示凭证无效的认证错误（HTTP 401）。
func NewInvalidCredentialError() *AuthError {
	return newAuthError(AuthErrorCodeInvalidCredential, "Invalid API key", http.StatusUnauthorized, nil)
}

// NewNotHandledError 创建一个表示当前提供者不处理该请求的错误。
// 该错误用于在 Manager 中跳过当前提供者，继续尝试下一个。
func NewNotHandledError() *AuthError {
	return newAuthError(AuthErrorCodeNotHandled, "authentication provider did not handle request", 0, nil)
}

// NewInternalAuthError 创建一个表示认证服务内部错误的认证错误（HTTP 500）。
// 参数说明：
//   - message: 错误描述，为空时使用默认描述
//   - cause: 底层错误，用于错误链包装
func NewInternalAuthError(message string, cause error) *AuthError {
	normalizedMessage := strings.TrimSpace(message)
	if normalizedMessage == "" {
		normalizedMessage = "Authentication service error"
	}
	return newAuthError(AuthErrorCodeInternal, normalizedMessage, http.StatusInternalServerError, cause)
}

// IsAuthErrorCode 判断认证错误是否为指定的错误分类码。
// 如果 authErr 为 nil，返回 false。
func IsAuthErrorCode(authErr *AuthError, code AuthErrorCode) bool {
	if authErr == nil {
		return false
	}
	return authErr.Code == code
}
