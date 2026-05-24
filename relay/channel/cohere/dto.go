// 本文件定义了 Cohere 渠道的数据传输对象（DTO）。
// 包含请求和响应的结构体定义，用于与 Cohere API 进行数据交互。
package cohere

import "github.com/c1cada/NexusTok/dto"

// CohereRequest 表示发送到 Cohere 聊天 API 的请求结构体。
type CohereRequest struct {
	Model       string        `json:"model"`                              // 模型名称
	ChatHistory []ChatHistory `json:"chat_history"`                       // 聊天历史记录列表
	Message     string        `json:"message"`                            // 当前用户消息
	Stream      bool          `json:"stream"`                             // 是否启用流式响应
	MaxTokens   uint          `json:"max_tokens"`                         // 最大生成 token 数
	SafetyMode  string        `json:"safety_mode,omitempty"`              // 安全模式设置（如 "NONE"）
}

// ChatHistory 表示 Cohere 聊天历史中的一条消息。
type ChatHistory struct {
	Role    string `json:"role"`    // 角色标识：USER、CHATBOT、SYSTEM
	Message string `json:"message"` // 消息内容
}

// CohereResponse 表示 Cohere 流式响应中的单个事件。
// 流式模式下，每个事件包含部分文本或结束信号。
type CohereResponse struct {
	IsFinished   bool                  `json:"is_finished"`              // 是否为最后一个事件
	EventType    string                `json:"event_type"`               // 事件类型
	Text         string                `json:"text,omitempty"`           // 文本内容（非结束事件时有值）
	FinishReason string                `json:"finish_reason,omitempty"`  // 结束原因（结束事件时有值）
	Response     *CohereResponseResult `json:"response"`                 // 完整响应结果（结束事件时包含用量信息）
}

// CohereResponseResult 表示 Cohere 非流式聊天响应的完整结果。
type CohereResponseResult struct {
	ResponseId   string     `json:"response_id"`           // 响应唯一标识
	FinishReason string     `json:"finish_reason,omitempty"` // 结束原因
	Text         string     `json:"text"`                  // 生成的文本内容
	Meta         CohereMeta `json:"meta"`                  // 元数据，包含用量信息
}

// CohereRerankRequest 表示发送到 Cohere 重排序 API 的请求结构体。
type CohereRerankRequest struct {
	Documents       []any  `json:"documents"`       // 待排序的文档列表
	Query           string `json:"query"`           // 查询文本
	Model           string `json:"model"`           // 重排序模型名称
	TopN            int    `json:"top_n"`           // 返回前 N 个结果
	ReturnDocuments bool   `json:"return_documents"` // 是否在结果中返回文档内容
}

// CohereRerankResponseResult 表示 Cohere 重排序 API 的响应结构体。
type CohereRerankResponseResult struct {
	Results []dto.RerankResponseResult `json:"results"` // 重排序结果列表
	Meta    CohereMeta                 `json:"meta"`    // 元数据，包含用量信息
}

// CohereMeta 表示 Cohere API 响应的元数据。
type CohereMeta struct {
	//Tokens CohereTokens `json:"tokens"`
	BilledUnits CohereBilledUnits `json:"billed_units"` // 计费 token 用量
}

// CohereBilledUnits 表示 Cohere API 的计费 token 用量统计。
type CohereBilledUnits struct {
	InputTokens  int `json:"input_tokens"`  // 输入 token 数量
	OutputTokens int `json:"output_tokens"` // 输出 token 数量
}

// CohereTokens 表示 Cohere API 的 token 用量统计（当前未使用）。
type CohereTokens struct {
	InputTokens  int `json:"input_tokens"`  // 输入 token 数量
	OutputTokens int `json:"output_tokens"` // 输出 token 数量
}
