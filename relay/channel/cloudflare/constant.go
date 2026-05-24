// Package cloudflare 实现 Cloudflare Workers AI 平台的渠道适配器常量定义。
// 包含该渠道支持的模型列表和渠道名称标识。
package cloudflare

// ModelList 定义 Cloudflare Workers AI 渠道支持的模型列表。
// 包含 Llama、Mistral、DeepSeek、Gemma、Qwen 等多种开源模型的 Cloudflare 部署版本。
// 模型名称使用 @cf/ 或 @hf/ 前缀标识来源（Cloudflare 托管或 HuggingFace 模型）。
var ModelList = []string{
	"@cf/meta/llama-3.1-8b-instruct",                    // Meta Llama 3.1 8B 指令微调版
	"@cf/meta/llama-2-7b-chat-fp16",                     // Meta Llama 2 7B 对话模型 FP16 版
	"@cf/meta/llama-2-7b-chat-int8",                     // Meta Llama 2 7B 对话模型 INT8 量化版
	"@cf/mistral/mistral-7b-instruct-v0.1",              // Mistral 7B 指令微调 v0.1
	"@hf/thebloke/deepseek-coder-6.7b-base-awq",         // DeepSeek Coder 6.7B 基础版 AWQ 量化
	"@hf/thebloke/deepseek-coder-6.7b-instruct-awq",     // DeepSeek Coder 6.7B 指令版 AWQ 量化
	"@cf/deepseek-ai/deepseek-math-7b-base",              // DeepSeek Math 7B 数学基础模型
	"@cf/deepseek-ai/deepseek-math-7b-instruct",          // DeepSeek Math 7B 数学指令模型
	"@cf/thebloke/discolm-german-7b-v1-awq",              // DiscoLM German 7B 德语模型 AWQ 量化
	"@cf/tiiuae/falcon-7b-instruct",                      // Falcon 7B 指令微调版
	"@cf/google/gemma-2b-it-lora",                        // Google Gemma 2B 指令版 LoRA
	"@hf/google/gemma-7b-it",                             // Google Gemma 7B 指令版
	"@cf/google/gemma-7b-it-lora",                        // Google Gemma 7B 指令版 LoRA
	"@hf/nousresearch/hermes-2-pro-mistral-7b",           // Hermes 2 Pro Mistral 7B
	"@hf/thebloke/llama-2-13b-chat-awq",                  // Llama 2 13B 对话模型 AWQ 量化
	"@cf/meta-llama/llama-2-7b-chat-hf-lora",             // Llama 2 7B 对话模型 HuggingFace LoRA
	"@cf/meta/llama-3-8b-instruct",                       // Meta Llama 3 8B 指令微调版
	"@hf/thebloke/llamaguard-7b-awq",                     // LlamaGuard 7B 安全模型 AWQ 量化
	"@hf/thebloke/mistral-7b-instruct-v0.1-awq",          // Mistral 7B 指令版 v0.1 AWQ 量化
	"@hf/mistralai/mistral-7b-instruct-v0.2",             // Mistral 7B 指令版 v0.2
	"@cf/mistral/mistral-7b-instruct-v0.2-lora",          // Mistral 7B 指令版 v0.2 LoRA
	"@hf/thebloke/neural-chat-7b-v3-1-awq",               // Neural Chat 7B v3.1 AWQ 量化
	"@cf/openchat/openchat-3.5-0106",                     // OpenChat 3.5 对话模型
	"@hf/thebloke/openhermes-2.5-mistral-7b-awq",        // OpenHermes 2.5 Mistral 7B AWQ 量化
	"@cf/microsoft/phi-2",                                // Microsoft Phi-2 小型语言模型
	"@cf/qwen/qwen1.5-0.5b-chat",                        // Qwen 1.5 0.5B 对话模型
	"@cf/qwen/qwen1.5-1.8b-chat",                        // Qwen 1.5 1.8B 对话模型
	"@cf/qwen/qwen1.5-14b-chat-awq",                     // Qwen 1.5 14B 对话模型 AWQ 量化
	"@cf/qwen/qwen1.5-7b-chat-awq",                      // Qwen 1.5 7B 对话模型 AWQ 量化
	"@cf/defog/sqlcoder-7b-2",                            // SQLCoder 7B SQL 代码生成模型
	"@hf/nexusflow/starling-lm-7b-beta",                  // Starling LM 7B Beta
	"@cf/tinyllama/tinyllama-1.1b-chat-v1.0",             // TinyLlama 1.1B 对话模型
	"@hf/thebloke/zephyr-7b-beta-awq",                    // Zephyr 7B Beta AWQ 量化
}

// ChannelName 定义 Cloudflare Workers AI 渠道的名称标识，用于渠道注册和路由匹配。
var ChannelName = "cloudflare"
