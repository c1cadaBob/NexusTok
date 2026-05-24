// Package openrouter 的数据传输对象 (DTO) 定义文件。
// 定义了 OpenRouter 特有的请求和响应结构体。
package openrouter

import "encoding/json" // 用于 json.RawMessage 类型定义

// RequestReasoning 定义 OpenRouter 的推理（reasoning）配置。
// 用于控制模型的推理行为，支持两种风格：
//   - OpenAI 风格：使用 effort 字段（high/medium/low）
//   - Anthropic 风格：使用 max_tokens 字段（具体 token 限制）
//
// 注意：effort 和 max_tokens 不能同时使用。
type RequestReasoning struct {
	Enabled bool `json:"enabled"` // 是否启用推理
	// 以下两种方式二选一（不能同时使用）：
	Effort    string `json:"effort,omitempty"`     // 推理努力程度（OpenAI 风格）：high/medium/low
	MaxTokens int    `json:"max_tokens,omitempty"` // 推理 token 限制（Anthropic 风格）
	// 可选：默认为 false，所有模型支持
	Exclude bool `json:"exclude,omitempty"` // 是否从响应中排除推理 token
}

// OpenRouterEnterpriseResponse 是 OpenRouter 企业版 API 的响应格式。
// 用于企业级功能的响应。
type OpenRouterEnterpriseResponse struct {
	Data    json.RawMessage `json:"data"`    // 响应数据（JSON 原始格式）
	Success bool            `json:"success"` // 是否成功
}
