// 讯飞星火 API 的数据传输对象（DTO）定义。
// 定义了讯飞星火 WebSocket API 的请求和响应结构体。
package xunfei

import "github.com/c1cada/NexusTok/dto"

// XunfeiMessage 讯飞消息格式，包含角色和内容。
type XunfeiMessage struct {
	Role    string `json:"role"`    // 消息角色："user"（用户）、"assistant"（助手）、"system"（系统）
	Content string `json:"content"` // 消息文本内容
}

// XunfeiChatRequest 讯飞星火聊天请求结构体。
// 遵循讯飞 WebSocket API 的三段式结构：header、parameter、payload。
type XunfeiChatRequest struct {
	Header struct {
		AppId string `json:"app_id"` // 讯飞应用 ID
	} `json:"header"`
	Parameter struct {
		Chat struct {
			Domain      string   `json:"domain,omitempty"`      // 模型领域标识，如 "generalv3"、"4.0Ultra" 等
			Temperature *float64 `json:"temperature,omitempty"` // 生成温度，控制随机性
			TopK        int      `json:"top_k,omitempty"`       // Top-K 采样参数
			MaxTokens   uint     `json:"max_tokens,omitempty"`  // 最大生成 token 数
			Auditing    bool     `json:"auditing,omitempty"`    // 是否启用内容审核
		} `json:"chat"`
	} `json:"parameter"`
	Payload struct {
		Message struct {
			Text []XunfeiMessage `json:"text"` // 消息列表
		} `json:"message"`
	} `json:"payload"`
}

// XunfeiChatResponseTextItem 讯飞响应中的单条文本选项。
type XunfeiChatResponseTextItem struct {
	Content string `json:"content"` // 文本内容
	Role    string `json:"role"`    // 角色
	Index   int    `json:"index"`   // 选项索引
}

// XunfeiChatResponse 讯飞星火聊天响应结构体。
// 包含头部状态信息和负载（选项列表及 token 使用量）。
type XunfeiChatResponse struct {
	Header struct {
		Code    int    `json:"code"`    // 状态码，0 表示成功
		Message string `json:"message"` // 状态消息
		Sid     string `json:"sid"`     // 会话 ID
		Status  int    `json:"status"`  // 消息状态：0-中间结果，1-第一个结果，2-最后一个结果
	} `json:"header"`
	Payload struct {
		Choices struct {
			Status int                          `json:"status"` // 选择状态：2 表示完成
			Seq    int                          `json:"seq"`    // 序列号
			Text   []XunfeiChatResponseTextItem `json:"text"`   // 响应文本选项列表
		} `json:"choices"`
		Usage struct {
			Text dto.Usage `json:"text"` // token 使用量统计
		} `json:"usage"`
	} `json:"payload"`
}
