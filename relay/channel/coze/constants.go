// coze - constants.go
// 本文件定义了 Coze 渠道支持的模型列表和渠道名称常量。
// Coze 是字节跳动旗下的 AI Bot 构建平台，通过 API 提供多种模型的访问。
// 模型列表包含多个第三方模型系列，覆盖月之暗面、百川、智谱、通义千问、深度求索、阶跃星辰、豆包等。
package coze

// ModelList 定义了 Coze 渠道支持的模型列表。
// 包含多个第三方模型系列：
//   - Moonshot（月之暗面）：moonshot-v1 系列，支持 8k/32k/128k 上下文长度
//   - Baichuan（百川）：Baichuan4 大模型
//   - MiniMax：abab6.5s-chat-pro 模型
//   - GLM（智谱）：glm-4-0520 模型
//   - Qwen（通义千问）：qwen-max 模型
//   - DeepSeek（深度求索）：deepseek-r1、deepseek-v3 及其蒸馏版本
//   - Step（阶跃星辰）：step-1v-8k、step-1.5v-mini 模型
//   - Doubao（豆包/火山引擎）：包含 pro、lite、vision 等多个系列，支持多种上下文长度
var ModelList = []string{
	"moonshot-v1-8k",
	"moonshot-v1-32k",
	"moonshot-v1-128k",
	"Baichuan4",
	"abab6.5s-chat-pro",
	"glm-4-0520",
	"qwen-max",
	"deepseek-r1",
	"deepseek-v3",
	"deepseek-r1-distill-qwen-32b",
	"deepseek-r1-distill-qwen-7b",
	"step-1v-8k",
	"step-1.5v-mini",
	"Doubao-pro-32k",
	"Doubao-pro-256k",
	"Doubao-lite-128k",
	"Doubao-lite-32k",
	"Doubao-vision-lite-32k",
	"Doubao-vision-pro-32k",
	"Doubao-1.5-pro-vision-32k",
	"Doubao-1.5-lite-32k",
	"Doubao-1.5-pro-32k",
	"Doubao-1.5-thinking-pro",
	"Doubao-1.5-pro-256k",
}

// ChannelName 定义了渠道名称标识符。
// 用于在系统中唯一标识 Coze 渠道，值为 "coze"。
var ChannelName = "coze"
