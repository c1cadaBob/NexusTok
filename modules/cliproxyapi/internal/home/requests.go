// home - requests.go
// 本文件定义了 Home 客户端与控制平面之间通信的请求数据结构。
// 包括认证分发请求和令牌刷新请求的结构体定义。
package home

// authDispatchRequest 表示认证分发请求，用于从控制平面获取可用的认证凭据。
// 包含请求类型、目标模型、所需凭据数量等信息。
type authDispatchRequest struct {
	Type      string            `json:"type"`
	Model     string            `json:"model"`
	Count     int               `json:"count"`
	SessionID string            `json:"session_id,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
}

// refreshRequest 表示令牌刷新请求，用于请求控制平面刷新指定索引的认证令牌。
type refreshRequest struct {
	Type      string `json:"type"`
	AuthIndex string `json:"auth_index"`
}
