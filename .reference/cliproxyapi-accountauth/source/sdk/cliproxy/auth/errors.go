// 包 auth - errors.go
// 该文件定义了认证相关的通用错误类型。
// Error 结构体以提供商无关的格式描述认证失败，包含错误码、消息和重试指示。
package auth

// Error 以提供商无关的格式描述认证相关失败。
type Error struct {
	Code       string `json:"code,omitempty"`       // Code 是简短的机器可读标识符
	Message    string `json:"message"`              // Message 是人类可读的失败描述
	Retryable  bool   `json:"retryable"`            // Retryable 指示重试是否可能自动修复问题
	HTTPStatus int    `json:"http_status,omitempty"` // HTTPStatus 可选地记录类似 HTTP 的状态码
}

// Error 实现 error 接口。
func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

// StatusCode 实现可选的状态访问器，供管理器决策使用。
func (e *Error) StatusCode() int {
	if e == nil {
		return 0
	}
	return e.HTTPStatus
}
