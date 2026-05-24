// Package palm 的数据传输对象 (DTO) 定义文件。
// 定义了 Google PaLM API 请求和响应的各种结构体。
// 参考文档：
// - 请求体：https://developers.generativeai.google/api/rest/generativelanguage/models/generateMessage#request-body
// - 响应体：https://developers.generativeai.google/api/rest/generativelanguage/models/generateMessage#response-body
package palm

import "github.com/c1cada/NexusTok/dto" // 数据传输对象

// PaLMChatMessage 表示 PaLM 聊天消息格式。
type PaLMChatMessage struct {
	Author  string `json:"author"`  // 消息作者（角色标识）
	Content string `json:"content"` // 消息内容
}

// PaLMFilter 表示 PaLM 内容过滤结果。
type PaLMFilter struct {
	Reason  string `json:"reason"`  // 过滤原因
	Message string `json:"message"` // 过滤消息
}

// PaLMPrompt 是 PaLM 聊天请求的提示格式。
type PaLMPrompt struct {
	Messages []PaLMChatMessage `json:"messages"` // 消息列表
}

// PaLMChatRequest 是 PaLM 聊天 API 的请求格式。
// 对应 Google 的 generateMessage 端点。
type PaLMChatRequest struct {
	Prompt         PaLMPrompt `json:"prompt"`                   // 提示内容
	Temperature    *float64   `json:"temperature,omitempty"`    // 温度参数（0-1）
	CandidateCount int        `json:"candidateCount,omitempty"` // 候选响应数量
	TopP           float64    `json:"topP,omitempty"`           // Top-P 采样参数
	TopK           uint       `json:"topK,omitempty"`           // Top-K 采样参数
}

// PaLMError 表示 PaLM API 的错误响应。
type PaLMError struct {
	Code    int    `json:"code"`    // 错误代码
	Message string `json:"message"` // 错误消息
	Status  string `json:"status"`  // 错误状态
}

// PaLMChatResponse 是 PaLM 聊天 API 的响应格式。
type PaLMChatResponse struct {
	Candidates []PaLMChatMessage `json:"candidates"` // 候选响应列表
	Messages   []dto.Message     `json:"messages"`   // 消息历史
	Filters    []PaLMFilter      `json:"filters"`    // 内容过滤结果
	Error      PaLMError         `json:"error"`      // 错误信息
}
