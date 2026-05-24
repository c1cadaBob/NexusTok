// Package dto - openai_responses_compaction_request.go
// 该文件定义了 OpenAI Responses API 压缩请求的数据传输对象
//
// 主要结构体：
// - OpenAIResponsesCompactionRequest：Responses API 压缩请求
//
// 压缩功能说明：
// - 用于将长对话历史压缩为更紧凑的表示
// - 支持指定前置响应 ID 以维护对话上下文
// - 输入和指令字段使用 json.RawMessage 以保持灵活的格式兼容
package dto

import (
	"encoding/json"
	"strings"

	"github.com/c1cada/NexusTok/types"

	"github.com/gin-gonic/gin"
)

// OpenAIResponsesCompactionRequest OpenAI Responses API 压缩请求
// Model：目标模型名称
// Input：输入内容（待压缩的对话历史）
// Instructions：系统指令（用于指导压缩行为）
// PreviousResponseID：前置响应 ID（用于维护多轮对话上下文）
type OpenAIResponsesCompactionRequest struct {
	Model              string          `json:"model"`
	Input              json.RawMessage `json:"input,omitempty"`
	Instructions       json.RawMessage `json:"instructions,omitempty"`
	PreviousResponseID string          `json:"previous_response_id,omitempty"`
}

// GetTokenCountMeta 获取压缩请求的 Token 计数元数据
// 将指令和输入内容拼接返回，用于计费和配额检查
func (r *OpenAIResponsesCompactionRequest) GetTokenCountMeta() *types.TokenCountMeta {
	var parts []string
	if len(r.Instructions) > 0 {
		parts = append(parts, string(r.Instructions))
	}
	if len(r.Input) > 0 {
		parts = append(parts, string(r.Input))
	}
	return &types.TokenCountMeta{
		CombineText: strings.Join(parts, "\n"),
	}
}

// IsStream 压缩请求不支持流式输出，始终返回 false
func (r *OpenAIResponsesCompactionRequest) IsStream(c *gin.Context) bool {
	return false
}

// SetModelName 设置压缩请求的模型名称
// 仅在 modelName 非空时更新
func (r *OpenAIResponsesCompactionRequest) SetModelName(modelName string) {
	if modelName != "" {
		r.Model = modelName
	}
}
