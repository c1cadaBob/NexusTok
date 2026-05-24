// Package vertex 实现 Google Vertex AI 渠道的常量定义
// Vertex AI 是 Google Cloud 的机器学习平台，支持多种 AI 模型
package vertex

// ModelList 支持的 Vertex AI 模型列表
// 包含 Meta Llama 等开源模型（Claude 和 Gemini 模型已注释，由专用渠道处理）
var ModelList = []string{
	//"claude-3-sonnet-20240229",
	//"claude-3-opus-20240229",
	//"claude-3-haiku-20240307",
	//"claude-3-5-sonnet-20240620",

	//"gemini-1.5-pro-latest", "gemini-1.5-flash-latest",
	//"gemini-1.5-pro-001", "gemini-1.5-flash-001", "gemini-pro", "gemini-pro-vision",

	"meta/llama3-405b-instruct-maas", // Meta Llama 3 405B 指令模型
}

// ChannelName 渠道名称标识
var ChannelName = "vertex-ai"
