// Package ali 实现阿里云通义千问（DashScope）AI 平台的渠道适配器。
// 该文件定义渠道支持的模型列表和渠道名称标识。
package ali

// ModelList 定义阿里云通义千问渠道支持的模型列表。
// 包含 Qwen 系列对话模型、向量化模型和重排序模型。
var ModelList = []string{
	"qwen-turbo",           // 通义千问 Turbo 快速对话模型
	"qwen-plus",            // 通义千问 Plus 增强对话模型
	"qwen-max",             // 通义千问 Max 最强对话模型
	"qwen-max-longcontext", // 通义千问 Max 长上下文版本
	"qwq-32b",              // QwQ 32B 推理模型
	"qwen3-235b-a22b",      // Qwen3 235B-A22B MoE 对话模型
	"text-embedding-v1",    // 通义千问文本向量化模型 v1
	"gte-rerank-v2",        // GTE 重排序模型 v2
}

// ChannelName 定义阿里云通义千问渠道的名称标识，用于渠道注册和路由匹配。
var ChannelName = "ali"
