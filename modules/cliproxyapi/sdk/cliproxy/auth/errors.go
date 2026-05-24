// auth - errors.go
// 定义认证相关的通用错误类型，提供与具体提供商无关的认证失败描述。
package auth

// Error 描述与认证相关的失败，采用与提供商无关的通用格式。
type Error struct {
	// Code 是短机器可读的错误标识符。
	Code string `json:"code,omitempty"`
	// Message 是人类可读的错误描述信息。
	Message string `json:"message"`
	// Retryable 指示重试是否可能自动修复该问题。
	Retryable bool `json:"retryable"`
	// HTTPStatus 可选地记录与错误关联的 HTTP 状态码。
	HTTPStatus int `json:"http_status,omitempty"`
}

// Error 实现 error 接口，返回格式化的错误字符串。
func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

// StatusCode 实现可选的状态码访问器，供 Manager 进行决策判断。
func (e *Error) StatusCode() int {
	if e == nil {
		return 0
	}
	return e.HTTPStatus
}
