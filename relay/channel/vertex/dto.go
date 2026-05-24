// Package vertex 实现 Google Vertex AI 渠道的数据传输对象定义
package vertex

import (
	"encoding/json" // JSON 编解码

	"github.com/c1cada/NexusTok/dto" // 数据传输对象
)

// VertexAIClaudeRequest Vertex AI 格式的 Claude 请求结构体
// 用于通过 Vertex AI 调用 Claude 模型
type VertexAIClaudeRequest struct {
	AnthropicVersion string              `json:"anthropic_version"`            // Anthropic API 版本
	Messages         []dto.ClaudeMessage `json:"messages"`                     // 消息列表
	System           any                 `json:"system,omitempty"`             // 系统提示词
	MaxTokens        *uint               `json:"max_tokens,omitempty"`         // 最大生成 token 数
	StopSequences    []string            `json:"stop_sequences,omitempty"`     // 停止序列
	Stream           *bool               `json:"stream,omitempty"`             // 是否流式输出
	Temperature      *float64            `json:"temperature,omitempty"`        // 温度参数
	TopP             *float64            `json:"top_p,omitempty"`              // Top-P 采样
	TopK             *int                `json:"top_k,omitempty"`              // Top-K 采样
	Tools            any                 `json:"tools,omitempty"`              // 工具定义
	ToolChoice       any                 `json:"tool_choice,omitempty"`        // 工具选择策略
	Thinking         *dto.Thinking       `json:"thinking,omitempty"`           // 思考配置
	OutputConfig     json.RawMessage     `json:"output_config,omitempty"`      // 输出配置
	//Metadata         json.RawMessage     `json:"metadata,omitempty"`
}

// copyRequest 将标准 Claude 请求转换为 Vertex AI 格式
// 设置 Anthropic 版本号并复制所有请求字段
//
// 参数：
//   - req: 标准 Claude 请求
//   - version: Anthropic API 版本号
//
// 返回值：
//   - *VertexAIClaudeRequest: Vertex AI 格式的请求
func copyRequest(req *dto.ClaudeRequest, version string) *VertexAIClaudeRequest {
	return &VertexAIClaudeRequest{
		AnthropicVersion: version,
		System:           req.System,
		Messages:         req.Messages,
		MaxTokens:        req.MaxTokens,
		Stream:           req.Stream,
		Temperature:      req.Temperature,
		TopP:             req.TopP,
		TopK:             req.TopK,
		StopSequences:    req.StopSequences,
		Tools:            req.Tools,
		ToolChoice:       req.ToolChoice,
		Thinking:         req.Thinking,
		OutputConfig:     req.OutputConfig,
	}
}
