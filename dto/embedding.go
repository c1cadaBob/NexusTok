// Package dto - embedding.go
// 该文件定义了文本嵌入（Embedding）API 的数据传输对象
//
// 主要结构体：
// - EmbeddingOptions：嵌入模型的可选参数（温度、top_k、频率惩罚等）
// - EmbeddingRequest：嵌入请求（支持单文本和批量文本输入）
// - EmbeddingResponseItem：单个嵌入结果（包含索引和向量）
// - EmbeddingResponse：嵌入响应（包含所有嵌入结果和用量统计）
//
// 输入格式说明：
// - Input 字段支持 string 和 []any 两种类型
// - ParseInput 方法统一将两种格式转换为字符串切片
package dto

import (
	"strings"

	"github.com/c1cada/NexusTok/types"

	"github.com/gin-gonic/gin"
)

// EmbeddingOptions 嵌入模型的可选配置参数
// Seed：随机种子（用于结果可复现）
// Temperature：温度参数
// TopK：Top-K 采样参数
// TopP：Top-P（核）采样参数
// FrequencyPenalty/PresencePenalty：频率/存在惩罚
// NumPredict/NumCtx：Ollama 兼容参数（预测 token 数/上下文窗口大小）
type EmbeddingOptions struct {
	Seed             int      `json:"seed,omitempty"`
	Temperature      *float64 `json:"temperature,omitempty"`
	TopK             int      `json:"top_k,omitempty"`
	TopP             *float64 `json:"top_p,omitempty"`
	FrequencyPenalty *float64 `json:"frequency_penalty,omitempty"`
	PresencePenalty  *float64 `json:"presence_penalty,omitempty"`
	NumPredict       int      `json:"num_predict,omitempty"`
	NumCtx           int      `json:"num_ctx,omitempty"`
}

// EmbeddingRequest 文本嵌入请求结构体
// Model：目标嵌入模型名称（如 text-embedding-3-small 等）
// Input：输入文本（支持单个字符串或字符串数组）
// EncodingFormat：向量格式（"float" 或 "base64"）
// Dimensions：输出向量维度（部分模型支持降维）
// User：最终用户标识（用于滥用监控）
// Seed/Temperature/TopP 等：生成参数（部分嵌入模型支持）
type EmbeddingRequest struct {
	Model            string   `json:"model"`
	Input            any      `json:"input"`
	EncodingFormat   string   `json:"encoding_format,omitempty"`
	Dimensions       *int     `json:"dimensions,omitempty"`
	User             string   `json:"user,omitempty"`
	Seed             *float64 `json:"seed,omitempty"`
	Temperature      *float64 `json:"temperature,omitempty"`
	TopP             *float64 `json:"top_p,omitempty"`
	FrequencyPenalty *float64 `json:"frequency_penalty,omitempty"`
	PresencePenalty  *float64 `json:"presence_penalty,omitempty"`
}

// GetTokenCountMeta 获取嵌入请求的 Token 计数元数据
// 将所有输入文本用换行符拼接后返回，用于计费和配额检查
func (r *EmbeddingRequest) GetTokenCountMeta() *types.TokenCountMeta {
	var texts = make([]string, 0)

	inputs := r.ParseInput()
	for _, input := range inputs {
		texts = append(texts, input)
	}

	return &types.TokenCountMeta{
		CombineText: strings.Join(texts, "\n"),
	}
}

// IsStream 嵌入请求不支持流式输出，始终返回 false
func (r *EmbeddingRequest) IsStream(c *gin.Context) bool {
	return false
}

// SetModelName 设置嵌入请求的模型名称
// 仅在 modelName 非空时更新，用于上游模型映射或路由替换
func (r *EmbeddingRequest) SetModelName(modelName string) {
	if modelName != "" {
		r.Model = modelName
	}
}

// ParseInput 解析嵌入请求的输入字段
// 支持两种格式：
// - string：单个文本字符串，返回包含该字符串的切片
// - []any：文本数组（JSON 数组），提取其中的字符串元素
// 其他类型返回空切片
func (r *EmbeddingRequest) ParseInput() []string {
	if r.Input == nil {
		return make([]string, 0)
	}
	var input []string
	switch r.Input.(type) {
	case string:
		input = []string{r.Input.(string)}
	case []any:
		input = make([]string, 0, len(r.Input.([]any)))
		for _, item := range r.Input.([]any) {
			if str, ok := item.(string); ok {
				input = append(input, str)
			}
		}
	}
	return input
}

// EmbeddingResponseItem 单个嵌入结果
// Object：对象类型标识（如 "embedding"）
// Index：在输入数组中的索引位置
// Embedding：嵌入向量（浮点数数组）
type EmbeddingResponseItem struct {
	Object    string    `json:"object"`
	Index     int       `json:"index"`
	Embedding []float64 `json:"embedding"`
}

// EmbeddingResponse 嵌入 API 的完整响应
// Object：对象类型标识（"list"）
// Data：嵌入结果列表
// Model：实际使用的模型名称
// Usage：Token 用量统计
type EmbeddingResponse struct {
	Object string                  `json:"object"`
	Data   []EmbeddingResponseItem `json:"data"`
	Model  string                  `json:"model"`
	Usage  `json:"usage"`
}
