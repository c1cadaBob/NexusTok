// Package perplexity 实现 Perplexity AI 渠道的适配器
// Perplexity 是一个 AI 搜索引擎，提供实时搜索增强的对话能力
package perplexity

// ModelList 支持的 Perplexity 模型列表
// 包含 Llama 3 Sonar 系列（搜索增强）和基础指令模型
var ModelList = []string{
	"llama-3-sonar-small-32k-chat", "llama-3-sonar-small-32k-online", "llama-3-sonar-large-32k-chat", "llama-3-sonar-large-32k-online", "llama-3-8b-instruct", "llama-3-70b-instruct", "mixtral-8x7b-instruct",
	"sonar", "sonar-pro", "sonar-reasoning",
}

// ChannelName 渠道名称标识
var ChannelName = "perplexity"
