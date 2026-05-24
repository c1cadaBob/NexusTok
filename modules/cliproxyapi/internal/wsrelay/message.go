// wsrelay - message.go
// 本文件定义了 WebSocket 中继协议的消息结构和消息类型常量。
// WebSocket 中继允许客户端通过 WebSocket 连接代理 HTTP 请求到 AI 服务提供商。
package wsrelay

// Message 表示与 WebSocket 客户端交换的 JSON 消息载荷。
// 每条消息包含唯一标识、消息类型和可选的数据负载。
type Message struct {
	// ID 是消息的唯一标识符，用于请求-响应关联。
	ID      string         `json:"id"`
	// Type 标识消息的类型，决定负载的处理方式。
	Type    string         `json:"type"`
	// Payload 是消息的数据负载，结构取决于消息类型。
	Payload map[string]any `json:"payload,omitempty"`
}

const (
	// MessageTypeHTTPReq 标识 HTTP 风格的请求信封。
	MessageTypeHTTPReq = "http_request"
	// MessageTypeHTTPResp 标识非流式 HTTP 响应信封。
	MessageTypeHTTPResp = "http_response"
	// MessageTypeStreamStart 标记流式响应的开始。
	MessageTypeStreamStart = "stream_start"
	// MessageTypeStreamChunk 携带流式响应的数据块。
	MessageTypeStreamChunk = "stream_chunk"
	// MessageTypeStreamEnd 标记流式响应的完成。
	MessageTypeStreamEnd = "stream_end"
	// MessageTypeError 携带错误响应。
	MessageTypeError = "error"
	// MessageTypePing 表示来自客户端的 ping 消息。
	MessageTypePing = "ping"
	// MessageTypePong 表示返回给客户端的 pong 响应。
	MessageTypePong = "pong"
)
