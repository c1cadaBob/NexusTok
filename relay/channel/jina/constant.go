// Jina AI 通道的常量定义文件。
// 定义通道名称和支持的模型列表。
package jina

// ModelList Jina AI 支持的模型列表。
// 包含 CLIP 视觉模型和多种 Rerank 重排序模型。
var ModelList = []string{
	"jina-clip-v1",                        // Jina CLIP 视觉-文本多模态模型
	"jina-reranker-v2-base-multilingual",  // Jina 多语言重排序模型 v2
	"jina-reranker-m0",                    // Jina 重排序模型 m0
}

// ChannelName 通道名称，用于标识 Jina AI 通道
var ChannelName = "jina"
