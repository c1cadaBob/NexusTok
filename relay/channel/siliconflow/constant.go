// Package siliconflow 的常量定义文件。
// 定义了 SiliconFlow 渠道支持的模型列表和渠道名称。
// SiliconFlow 是一个 AI 模型聚合平台，提供多种开源模型的推理服务。
package siliconflow

// ModelList 是 SiliconFlow 渠道当前支持的模型列表。
// 包含多种开源大语言模型、图像生成模型、嵌入模型和重排模型等。
var ModelList = []string{
	"THUDM/glm-4-9b-chat",
	//"stabilityai/stable-diffusion-xl-base-1.0",
	//"TencentARC/PhotoMaker",
	"InstantX/InstantID",
	//"stabilityai/stable-diffusion-2-1",
	//"stabilityai/sd-turbo",
	//"stabilityai/sdxl-turbo",
	"ByteDance/SDXL-Lightning",
	"deepseek-ai/deepseek-llm-67b-chat",
	"Qwen/Qwen1.5-14B-Chat",
	"Qwen/Qwen1.5-7B-Chat",
	"Qwen/Qwen1.5-110B-Chat",
	"Qwen/Qwen1.5-32B-Chat",
	"01-ai/Yi-1.5-6B-Chat",
	"01-ai/Yi-1.5-9B-Chat-16K",
	"01-ai/Yi-1.5-34B-Chat-16K",
	"THUDM/chatglm3-6b",
	"deepseek-ai/DeepSeek-V2-Chat",
	"Qwen/Qwen2-72B-Instruct",
	"Qwen/Qwen2-7B-Instruct",
	"Qwen/Qwen2-57B-A14B-Instruct",
	//"stabilityai/stable-diffusion-3-medium",
	"deepseek-ai/DeepSeek-Coder-V2-Instruct",
	"Qwen/Qwen2-1.5B-Instruct",
	"internlm/internlm2_5-7b-chat",
	"BAAI/bge-large-en-v1.5",
	"BAAI/bge-large-zh-v1.5",
	"Pro/Qwen/Qwen2-7B-Instruct",
	"Pro/Qwen/Qwen2-1.5B-Instruct",
	"Pro/Qwen/Qwen1.5-7B-Chat",
	"Pro/THUDM/glm-4-9b-chat",
	"Pro/THUDM/chatglm3-6b",
	"Pro/01-ai/Yi-1.5-9B-Chat-16K",
	"Pro/01-ai/Yi-1.5-6B-Chat",
	"Pro/google/gemma-2-9b-it",
	"Pro/internlm/internlm2_5-7b-chat",
	"Pro/meta-llama/Meta-Llama-3-8B-Instruct",
	"Pro/mistralai/Mistral-7B-Instruct-v0.2",
	"black-forest-labs/FLUX.1-schnell",
	"FunAudioLLM/SenseVoiceSmall",
	"netease-youdao/bce-embedding-base_v1",
	"BAAI/bge-m3",
	"internlm/internlm2_5-20b-chat",
	"Qwen/Qwen2-Math-72B-Instruct",
	"netease-youdao/bce-reranker-base_v1",
	"BAAI/bge-reranker-v2-m3",
}

// ChannelName 渠道名称标识，用于路由和日志中识别 SiliconFlow 渠道。
var ChannelName = "siliconflow"
