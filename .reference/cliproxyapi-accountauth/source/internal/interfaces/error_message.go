// 包 interfaces - error_message.go
// 该文件定义了错误消息结构体，用于封装带有 HTTP 状态码的错误。
package interfaces

import "net/http"

// ErrorMessage 封装了带有关联 HTTP 状态码的错误。
// 此结构用于提供详细的错误信息，包括 HTTP 状态和底层错误。
type ErrorMessage struct {
	// StatusCode 是 API 返回的 HTTP 状态码
	StatusCode int

	// Error 是发生的底层错误
	Error error

	// Addon 包含要添加到响应中的额外头
	Addon http.Header
}
