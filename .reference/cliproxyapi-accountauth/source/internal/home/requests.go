// 包 home - requests.go
// 该文件定义了 Home 客户端使用的请求结构体。
package home

// authDispatchRequest 是认证分发请求的结构体。
type authDispatchRequest struct {
	// Type 是请求类型
	Type string `json:"type"`
	// Model 是请求的模型名称
	Model string `json:"model"`
	// Count 是请求的认证数量
	Count int `json:"count"`
	// SessionID 是会话标识符
	SessionID string `json:"session_id,omitempty"`
	// Headers 是请求头
	Headers map[string]string `json:"headers,omitempty"`
}

// refreshRequest 是认证刷新请求的结构体。
type refreshRequest struct {
	// Type 是请求类型
	Type string `json:"type"`
	// AuthIndex 是认证索引
	AuthIndex string `json:"auth_index"`
}
