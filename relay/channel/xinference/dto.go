// Package xinference 的数据传输对象（DTO）定义文件。
// 定义了 Xinference Rerank API 响应的结构体。
package xinference

// XinRerankResponseDocument 是 Xinference Rerank 响应中的单个文档结果。
// 包含原始文档内容、在结果列表中的索引和相关性分数。
type XinRerankResponseDocument struct {
	Document       any     `json:"document,omitempty"`  // 原始文档内容（类型不确定）
	Index          int     `json:"index"`               // 文档在原始列表中的索引
	RelevanceScore float64 `json:"relevance_score"`     // 与查询的相关性分数（0~1）
}

// XinRerankResponse 是 Xinference Rerank API 的响应结构体。
// 包含按相关性排序的文档结果列表。
type XinRerankResponse struct {
	Results []XinRerankResponseDocument `json:"results"` // 重排后的文档结果列表
}
