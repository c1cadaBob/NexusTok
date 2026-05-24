// interfaces - error_message.go
// 本文件定义了错误消息封装结构，用于提供包含 HTTP 状态码的详细错误信息。
package interfaces

import "net/http"

// ErrorMessage 封装了带有关联 HTTP 状态码的错误。
// 该结构用于提供详细的错误信息，包括 HTTP 状态和底层错误。
type ErrorMessage struct {
	// StatusCode 是 API 返回的 HTTP 状态码。
	StatusCode int

	// Error 是发生的底层错误。
	Error error

	// Addon 包含要添加到响应中的额外头部。
	Addon http.Header
}
