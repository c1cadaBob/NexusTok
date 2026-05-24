// 本文件定义了 Cohere 渠道支持的模型列表和渠道名称常量。
// 包含 Command 系列聊天模型、Aya 多语言模型和 Rerank 重排序模型。
package cohere

// ModelList 定义了 Cohere 渠道支持的模型列表。
// 包含：
//   - Command 系列：命令模型及其变体（command-r, command-r-plus 等）
//   - Aya 系列：多语言模型（c4ai-aya-23-35b, c4ai-aya-23-8b）
//   - Rerank 系列：重排序模型（支持英语和多语言）
var ModelList = []string{
	"command-a-03-2025",
	"command-r", "command-r-plus",
	"command-r-08-2024", "command-r-plus-08-2024",
	"c4ai-aya-23-35b", "c4ai-aya-23-8b",
	"command-light", "command-light-nightly", "command", "command-nightly",
	"rerank-english-v3.0", "rerank-multilingual-v3.0", "rerank-english-v2.0", "rerank-multilingual-v2.0",
}

// ChannelName 定义了渠道名称标识符。
var ChannelName = "cohere"
