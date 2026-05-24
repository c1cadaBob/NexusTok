// deepseek - constants.go
// 本文件定义了 DeepSeek 渠道支持的模型列表和渠道名称常量。
// DeepSeek（深度求索）是一家专注于 AI 大模型研发的公司，提供通用聊天和推理专用模型。
// 模型列表包含基础模型和带有思维链后缀的变体，用于控制模型的推理深度。
package deepseek

// ModelList 定义了 DeepSeek 渠道支持的模型列表。
// 包含以下模型系列：
//   - deepseek-chat：通用聊天模型，适用于日常对话和文本生成
//   - deepseek-reasoner：推理专用模型，适用于需要复杂逻辑推理的场景
//   - deepseek-v4-flash 系列：快速模型，响应速度更快
//   - deepseek-v4-pro 系列：专业模型，推理能力更强
//
// 思维链后缀说明（用于 v4 系列模型）：
//   - 无后缀：使用默认的思维链行为
//   - -none：禁用思维链推理，直接输出结果
//   - -max：启用最大推理深度，适用于复杂推理任务
var ModelList = []string{
	"deepseek-chat", "deepseek-reasoner",
	"deepseek-v4-flash", "deepseek-v4-flash-none", "deepseek-v4-flash-max",
	"deepseek-v4-pro", "deepseek-v4-pro-none", "deepseek-v4-pro-max",
}

// ChannelName 定义了渠道名称标识符。
// 用于在系统中唯一标识 DeepSeek 渠道，值为 "deepseek"。
var ChannelName = "deepseek"
