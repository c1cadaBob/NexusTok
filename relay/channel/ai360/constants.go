// Package ai360 实现 360 AI 平台的渠道适配器常量定义。
// 包含该渠道支持的模型列表和渠道名称标识。
package ai360

// ModelList 定义 360 AI 渠道支持的模型列表。
// 包含对话模型（如 360gpt-turbo、360gpt-pro）和向量化模型（如 embedding-bert-512-v1）。
var ModelList = []string{
	"360gpt-turbo",                    // 360 GPT Turbo 对话模型
	"360gpt-turbo-responsibility-8k",  // 360 GPT Turbo 责任版 8K 上下文模型
	"360gpt-pro",                      // 360 GPT Pro 对话模型
	"360gpt2-pro",                     // 360 GPT2 Pro 对话模型（第二代）
	"360GPT_S2_V9",                    // 360 GPT S2 V9 对话模型
	"embedding-bert-512-v1",           // BERT 向量化模型，512 维
	"embedding_s1_v1",                 // S1 向量化模型 v1
	"semantic_similarity_s1_v1",       // S1 语义相似度模型 v1
}

// ChannelName 定义 360 AI 渠道的名称标识，用于渠道注册和路由匹配。
var ChannelName = "ai360"
