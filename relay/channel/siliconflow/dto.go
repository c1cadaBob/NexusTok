// Package siliconflow 的数据传输对象（DTO）定义文件。
// 定义了 SiliconFlow API 特有的请求和响应结构体，
// 包括 Rerank 响应和图片生成请求等。
package siliconflow

import "github.com/c1cada/NexusTok/dto"

// SFTokens 表示 SiliconFlow API 返回的 token 使用量信息。
type SFTokens struct {
	InputTokens  int `json:"input_tokens"`  // 输入 token 数量
	OutputTokens int `json:"output_tokens"` // 输出 token 数量
}

// SFMeta 表示 SiliconFlow API 响应中的元数据。
type SFMeta struct {
	Tokens SFTokens `json:"tokens"` // token 使用量信息
}

// SFRerankResponse 是 SiliconFlow Rerank API 的响应结构体。
// 包含重排结果列表和元数据（token 使用量等）。
type SFRerankResponse struct {
	Results []dto.RerankResponseResult `json:"results"` // 重排结果列表
	Meta    SFMeta                     `json:"meta"`    // 响应元数据
}

// SFImageRequest 是 SiliconFlow 图片生成 API 的请求结构体。
// 支持文本生图和图生图等多种模式。
type SFImageRequest struct {
	Model             string  `json:"model"`                        // 模型名称
	Prompt            string  `json:"prompt"`                       // 生成提示词
	NegativePrompt    string  `json:"negative_prompt,omitempty"`    // 负面提示词（不想出现的内容）
	ImageSize         string  `json:"image_size,omitempty"`         // 图片尺寸，如 "1024x1024"
	BatchSize         uint    `json:"batch_size,omitempty"`         // 批量生成数量
	Seed              uint64  `json:"seed,omitempty"`               // 随机种子，用于复现结果
	NumInferenceSteps uint    `json:"num_inference_steps,omitempty"` // 推理步数，影响图片质量和速度
	GuidanceScale     float64 `json:"guidance_scale,omitempty"`     // 引导比例，控制提示词的影响力
	Cfg               float64 `json:"cfg,omitempty"`                // CFG 值（Classifier-Free Guidance）
	Image             string  `json:"image,omitempty"`              // 参考图片 URL（图生图模式）
	Image2            string  `json:"image2,omitempty"`             // 第二参考图片 URL
	Image3            string  `json:"image3,omitempty"`             // 第三参考图片 URL
}
