// Package xinference 的常量定义文件。
// 定义了 Xinference 渠道支持的模型列表和渠道名称。
// Xinference 是一个开源的模型推理框架，支持多种模型的部署和推理。

package xinference

// ModelList 是 Xinference 渠道当前支持的模型列表。
// 主要是重排（Rerank）模型，用于搜索结果的语义重排序。
var ModelList = []string{
	"bge-reranker-v2-m3", // BAAI 的多语言重排模型
	"jina-reranker-v2",   // Jina AI 的重排模型 v2
}

// ChannelName 渠道名称标识，用于路由和日志中识别 Xinference 渠道。
var ChannelName = "xinference"
