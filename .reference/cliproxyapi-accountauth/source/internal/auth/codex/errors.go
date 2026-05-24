// codex - errors.go
// 包 codex 提供 OpenAI Codex API 的认证和令牌管理功能。
// 该文件定义了认证过程中使用的错误类型和错误处理函数，
// 包括 OAuth 错误、认证错误以及用户友好的错误消息生成。
package codex

import (
	"errors"
	"fmt"
	"net/http"
)

// OAuthError 表示 OAuth 特定的错误。
// 包含 OAuth 错误代码、描述信息和关联的 HTTP 状态码。
type OAuthError struct {
	// Code 是 OAuth 错误代码
	Code string `json:"error"`
	// Description 是错误的人类可读描述
	Description string `json:"error_description,omitempty"`
	// URI 是指向错误信息网页的 URI
	URI string `json:"error_uri,omitempty"`
	// StatusCode 是与错误关联的 HTTP 状态码
	StatusCode int `json:"-"`
}

// Error 返回 OAuth 错误的字符串表示。
func (e *OAuthError) Error() string {
	if e.Description != "" {
		return fmt.Sprintf("OAuth error %s: %s", e.Code, e.Description)
	}
	return fmt.Sprintf("OAuth error: %s", e.Code)
}

// NewOAuthError 创建一个新的 OAuth 错误。
//
// 参数：
//   - code: OAuth 错误代码
//   - description: 错误描述
//   - statusCode: HTTP 状态码
//
// 返回：
//   - *OAuthError: 新的 OAuth 错误实例
func NewOAuthError(code, description string, statusCode int) *OAuthError {
	return &OAuthError{
		Code:        code,
		Description: description,
		StatusCode:  statusCode,
	}
}

// AuthenticationError 表示认证相关的错误。
// 包含错误类型、消息、HTTP 状态码和底层原因错误。
type AuthenticationError struct {
	// Type 是认证错误的类型
	Type string `json:"type"`
	// Message 是错误的人类可读消息
	Message string `json:"message"`
	// Code 是与错误关联的 HTTP 状态码
	Code int `json:"code"`
	// Cause 是导致此认证错误的底层错误
	Cause error `json:"-"`
}

// Error 返回认证错误的字符串表示。
func (e *AuthenticationError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s (caused by: %v)", e.Type, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Type, e.Message)
}

// 预定义的认证错误类型。
var (
	// ErrInvalidState 表示 OAuth 状态参数无效的错误
	ErrInvalidState = &AuthenticationError{
		Type:    "invalid_state",
		Message: "OAuth state parameter is invalid",
		Code:    http.StatusBadRequest,
	}

	// ErrCodeExchangeFailed 表示用授权码换取令牌失败的错误
	ErrCodeExchangeFailed = &AuthenticationError{
		Type:    "code_exchange_failed",
		Message: "Failed to exchange authorization code for tokens",
		Code:    http.StatusBadRequest,
	}

	// ErrServerStartFailed 表示启动 OAuth 回调服务器失败的错误
	ErrServerStartFailed = &AuthenticationError{
		Type:    "server_start_failed",
		Message: "Failed to start OAuth callback server",
		Code:    http.StatusInternalServerError,
	}

	// ErrPortInUse 表示 OAuth 回调端口已被占用的错误
	ErrPortInUse = &AuthenticationError{
		Type:    "port_in_use",
		Message: "OAuth callback port is already in use",
		Code:    13, // 端口占用的特殊退出代码
	}

	// ErrCallbackTimeout 表示等待 OAuth 回调超时的错误
	ErrCallbackTimeout = &AuthenticationError{
		Type:    "callback_timeout",
		Message: "Timeout waiting for OAuth callback",
		Code:    http.StatusRequestTimeout,
	}

	// ErrBrowserOpenFailed 表示打开浏览器进行认证失败的错误
	ErrBrowserOpenFailed = &AuthenticationError{
		Type:    "browser_open_failed",
		Message: "Failed to open browser for authentication",
		Code:    http.StatusInternalServerError,
	}
)

// NewAuthenticationError 基于基础错误创建带有原因的新认证错误。
//
// 参数：
//   - baseErr: 基础认证错误，提供类型、消息和代码
//   - cause: 导致此错误的底层错误
//
// 返回：
//   - *AuthenticationError: 新的认证错误实例
func NewAuthenticationError(baseErr *AuthenticationError, cause error) *AuthenticationError {
	return &AuthenticationError{
		Type:    baseErr.Type,
		Message: baseErr.Message,
		Code:    baseErr.Code,
		Cause:   cause,
	}
}

// IsAuthenticationError 检查错误是否为认证错误。
//
// 参数：
//   - err: 要检查的错误
//
// 返回：
//   - bool: 如果错误是认证错误返回 true
func IsAuthenticationError(err error) bool {
	var authenticationError *AuthenticationError
	ok := errors.As(err, &authenticationError)
	return ok
}

// IsOAuthError 检查错误是否为 OAuth 错误。
//
// 参数：
//   - err: 要检查的错误
//
// 返回：
//   - bool: 如果错误是 OAuth 错误返回 true
func IsOAuthError(err error) bool {
	var oAuthError *OAuthError
	ok := errors.As(err, &oAuthError)
	return ok
}

// GetUserFriendlyMessage 根据错误类型返回用户友好的错误消息。
// 将技术性的错误转换为普通用户可以理解的提示信息。
//
// 参数：
//   - err: 要转换的错误
//
// 返回：
//   - string: 用户友好的错误消息
func GetUserFriendlyMessage(err error) string {
	switch {
	case IsAuthenticationError(err):
		var authErr *AuthenticationError
		errors.As(err, &authErr)
		switch authErr.Type {
		case "token_expired":
			return "Your authentication has expired. Please log in again."
		case "token_invalid":
			return "Your authentication is invalid. Please log in again."
		case "authentication_required":
			return "Please log in to continue."
		case "port_in_use":
			return "The required port is already in use. Please close any applications using port 3000 and try again."
		case "callback_timeout":
			return "Authentication timed out. Please try again."
		case "browser_open_failed":
			return "Could not open your browser automatically. Please copy and paste the URL manually."
		default:
			return "Authentication failed. Please try again."
		}
	case IsOAuthError(err):
		var oauthErr *OAuthError
		errors.As(err, &oauthErr)
		switch oauthErr.Code {
		case "access_denied":
			return "Authentication was cancelled or denied."
		case "invalid_request":
			return "Invalid authentication request. Please try again."
		case "server_error":
			return "Authentication server error. Please try again later."
		default:
			return fmt.Sprintf("Authentication failed: %s", oauthErr.Description)
		}
	default:
		return "An unexpected error occurred. Please try again."
	}
}
