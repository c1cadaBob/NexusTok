// Package common - endpoint_defaults.go
// 该文件定义了各端点类型的默认请求信息
//
// 端点类型（EndpointType）标识不同的 API 接口类型
// 每个端点类型有默认的请求路径和 HTTP 方法
// 这些默认值用于：
// - API 文档生成
// - 渠道测试
// - 健康检查
package common

import "github.com/c1cada/NexusTok/constant"

// EndpointInfo 描述单个端点的默认请求信息
//
// 包含上游 API 的路径和 HTTP 方法
// json 标签用于直接序列化到 API 输出
// 例如：{"path":"/v1/chat/completions","method":"POST"}
type EndpointInfo struct {
	Path   string `json:"path"`   // 上游 API 路径
	Method string `json:"method"` // HTTP 请求方法（POST/GET 等）
}

// defaultEndpointInfoMap 保存内置端点的默认 Path 与 Method
//
// 映射关系：
// - OpenAI 端点: /v1/chat/completions (POST)
// - OpenAI Response 端点: /v1/responses (POST)
// - Anthropic 端点: /v1/messages (POST)
// - Gemini 端点: /v1beta/models/{model}:generateContent (POST)
// - Jina Rerank 端点: /v1/rerank (POST)
// - 图像生成端点: /v1/images/generations (POST)
// - Embeddings 端点: /v1/embeddings (POST)
var defaultEndpointInfoMap = map[constant.EndpointType]EndpointInfo{
	constant.EndpointTypeOpenAI:                {Path: "/v1/chat/completions", Method: "POST"},
	constant.EndpointTypeOpenAIResponse:        {Path: "/v1/responses", Method: "POST"},
	constant.EndpointTypeOpenAIResponseCompact: {Path: "/v1/responses/compact", Method: "POST"},
	constant.EndpointTypeAnthropic:             {Path: "/v1/messages", Method: "POST"},
	constant.EndpointTypeGemini:                {Path: "/v1beta/models/{model}:generateContent", Method: "POST"},
	constant.EndpointTypeJinaRerank:            {Path: "/v1/rerank", Method: "POST"},
	constant.EndpointTypeImageGeneration:       {Path: "/v1/images/generations", Method: "POST"},
	constant.EndpointTypeEmbeddings:            {Path: "/v1/embeddings", Method: "POST"},
}

// GetDefaultEndpointInfo 返回指定端点类型的默认信息
//
// 参数：
//   - et: 端点类型
//
// 返回值：
//   - EndpointInfo: 端点信息
//   - bool: 是否存在该端点类型的默认信息
func GetDefaultEndpointInfo(et constant.EndpointType) (EndpointInfo, bool) {
	info, ok := defaultEndpointInfoMap[et]
	return info, ok
}
