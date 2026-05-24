// Package cloudflare 定义 Cloudflare Workers AI API 的数据传输对象（DTO）。
// 包含请求和响应的结构体定义，用于与 Cloudflare AI API 进行 JSON 序列化/反序列化。
package cloudflare

// 项目内部导入

// CfRequest 表示 Cloudflare Workers AI 对话 API 的请求结构。
// 支持两种模式：对话模式（使用 messages）和补全模式（使用 prompt）。
type CfRequest struct {
	Messages    []dto.Message `json:"messages,omitempty"`    // 对话消息列表（对话模式）
	Lora        string        `json:"lora,omitempty"`        // LoRA 适配器标识
	MaxTokens   uint          `json:"max_tokens,omitempty"`  // 最大输出 token 数
	Prompt      string        `json:"prompt,omitempty"`      // 提示文本（补全模式）
	Raw         bool          `json:"raw,omitempty"`         // 是否返回原始输出
	Stream      bool          `json:"stream,omitempty"`      // 是否启用流式输出
	Temperature *float64      `json:"temperature,omitempty"` // 采样温度
}

// CfAudioResponse 表示 Cloudflare Workers AI 语音转文字（STT）API 的响应结构。
type CfAudioResponse struct {
	Result CfSTTResult `json:"result"` // 语音识别结果
}

// CfSTTResult 表示 Cloudflare 语音转文字的结果数据。
type CfSTTResult struct {
	Text string `json:"text"` // 识别出的文本内容
}
