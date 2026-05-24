// Package dto - openai_compaction.go
// 该文件定义了 OpenAI Responses API 压缩响应的数据传输对象
//
// 主要结构体：
// - OpenAIResponsesCompactionResponse：Responses API 压缩响应结构体
//
// 压缩功能说明：
// - 用于将长对话历史压缩为更紧凑的表示
// - 保留完整的输出内容和用量统计
// - 支持错误信息传递
package dto

import (
	"encoding/json"

	"github.com/c1cada/NexusTok/types"
)

// OpenAIResponsesCompactionResponse OpenAI Responses API 压缩响应
// ID：响应唯一标识
// Object：对象类型（如 "response.compaction"）
// CreatedAt：创建时间戳
// Output：压缩后的输出内容（JSON 格式）
// Usage：Token 用量统计
// Error：错误信息（可选）
type OpenAIResponsesCompactionResponse struct {
	ID        string          `json:"id"`
	Object    string          `json:"object"`
	CreatedAt int             `json:"created_at"`
	Output    json.RawMessage `json:"output"`
	Usage     *Usage          `json:"usage"`
	Error     any             `json:"error,omitempty"`
}

// GetOpenAIError 从压缩响应中提取 OpenAI 格式的错误信息
// 如果 Error 字段为空或非 OpenAI 格式，返回 nil
func (o *OpenAIResponsesCompactionResponse) GetOpenAIError() *types.OpenAIError {
	return GetOpenAIError(o.Error)
}
