// Package dto - rerank.go
// 该文件定义了文本重排序（Rerank）API 的数据传输对象
//
// 主要结构体：
// - RerankRequest：重排序请求（文档列表 + 查询）
// - RerankResponse：重排序响应（按相关性排序的结果）
// - RerankResponseResult：单个重排序结果
// - RerankDocument：重排序文档
//
// 用途：用于检索增强生成（RAG）场景，根据查询对文档进行相关性排序
package dto

import (
	"fmt"
	"strings"

	"github.com/c1cada/NexusTok/types"
	"github.com/gin-gonic/gin"
)

// RerankRequest 重排序请求
// Documents：待排序的文档列表（支持任意类型）
// Query：查询文本
// Model：重排序模型名称（如 bge-reranker-v2-m3 等）
// TopN：返回前 N 个结果
// ReturnDocuments：是否在结果中返回原始文档
// MaxChunkPerDoc：每个文档的最大分块数
// OverLapTokens：分块重叠 token 数
type RerankRequest struct {
	Documents       []any  `json:"documents"`
	Query           string `json:"query"`
	Model           string `json:"model"`
	TopN            *int   `json:"top_n,omitempty"`
	ReturnDocuments *bool  `json:"return_documents,omitempty"`
	MaxChunkPerDoc  *int   `json:"max_chunk_per_doc,omitempty"`
	OverLapTokens   *int   `json:"overlap_tokens,omitempty"`
}

// IsStream 重排序请求不支持流式输出，始终返回 false
func (r *RerankRequest) IsStream(c *gin.Context) bool {
	return false
}

// GetTokenCountMeta 获取重排序请求的 Token 计数元数据
// 将所有文档和查询文本拼接返回
func (r *RerankRequest) GetTokenCountMeta() *types.TokenCountMeta {
	var texts = make([]string, 0)

	for _, document := range r.Documents {
		texts = append(texts, fmt.Sprintf("%v", document))
	}

	if r.Query != "" {
		texts = append(texts, r.Query)
	}

	return &types.TokenCountMeta{
		CombineText: strings.Join(texts, "\n"),
	}
}

// SetModelName 设置重排序请求的模型名称
// 仅在 modelName 非空时更新
func (r *RerankRequest) SetModelName(modelName string) {
	if modelName != "" {
		r.Model = modelName
	}
}

// GetReturnDocuments 获取是否返回文档的配置
// 如果 ReturnDocuments 为 nil，返回 false
func (r *RerankRequest) GetReturnDocuments() bool {
	if r.ReturnDocuments == nil {
		return false
	}
	return *r.ReturnDocuments
}

// RerankResponseResult 重排序结果项
// Document：原始文档（可选，取决于 ReturnDocuments 配置）
// Index：文档在原始列表中的索引
// RelevanceScore：相关性分数（0-1，越高越相关）
type RerankResponseResult struct {
	Document       any     `json:"document,omitempty"`
	Index          int     `json:"index"`
	RelevanceScore float64 `json:"relevance_score"`
}

// RerankDocument 重排序文档
// Text：文档文本内容
type RerankDocument struct {
	Text any `json:"text"`
}

// RerankResponse 重排序响应
// Results：按相关性排序的结果列表
// Usage：Token 用量统计
type RerankResponse struct {
	Results []RerankResponseResult `json:"results"`
	Usage   Usage                  `json:"usage"`
}
