// Package xai 的数据传输对象（DTO）定义文件。
// 定义了 xAI API 特有的请求和响应结构体。
package xai

import "github.com/c1cada/NexusTok/dto"

// ChatCompletionResponse 是 xAI 聊天补全 API 的响应结构体。
// 格式与 OpenAI 兼容，包含 ID、对象类型、创建时间、模型名、
// 选择列表、使用量和系统指纹等字段。
type ChatCompletionResponse struct {
	Id                string                         `json:"id"`                 // 响应唯一标识
	Object            string                         `json:"object"`             // 对象类型，如 "chat.completion"
	Created           int64                          `json:"created"`            // 创建时间戳
	Model             string                         `json:"model"`              // 使用的模型名称
	Choices           []dto.OpenAITextResponseChoice `json:"choices"`            // 模型生成的选择列表
	Usage             *dto.Usage                     `json:"usage"`              // token 使用量
	SystemFingerprint string                         `json:"system_fingerprint"` // 系统指纹
}

// ImageRequest 是 xAI 图片生成 API 的请求结构体。
// 注意：xAI API 当前不支持 quality、size 和 style 参数。
type ImageRequest struct {
	Model  string `json:"model"`            // 模型名称
	Prompt string `json:"prompt" binding:"required"` // 生成提示词（必填）
	N      int    `json:"n,omitempty"`      // 生成图片数量
	// Size           string          `json:"size,omitempty"`      // 不支持
	// Quality        string          `json:"quality,omitempty"`   // 不支持
	ResponseFormat string `json:"response_format,omitempty"` // 响应格式（url 或 b64_json）
	// Style          string          `json:"style,omitempty"`     // 不支持
	// User           string          `json:"user,omitempty"`
	// ExtraFields    json.RawMessage `json:"extra_fields,omitempty"`
}
