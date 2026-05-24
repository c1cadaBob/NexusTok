// Package submodel 实现了子模型渠道的适配器。
// 子模型渠道是一种轻量级适配器，主要用于代理 OpenAI 兼容格式的请求，
// 将请求直接透传到上游服务，响应处理委托给 OpenAI 适配器的通用处理器。
// 不支持音频、图片、Embedding、Rerank 等非文本功能。
package submodel

import (
	"errors"
	"io"
	"net/http"

	// 项目内部依赖
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/relay/channel"
	"github.com/c1cada/NexusTok/relay/channel/openai"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/types"

	// 第三方依赖
	"github.com/gin-gonic/gin"
)

// Adaptor 是子模型渠道的适配器结构体。
// 实现了 channel.Adaptor 接口，支持 OpenAI 格式的文本补全请求。
type Adaptor struct {
}

// ConvertGeminiRequest 未实现，子模型渠道不支持 Gemini 格式请求。
func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	return nil, errors.New("submodel channel: endpoint not supported")
}

// ConvertClaudeRequest 未实现，子模型渠道不支持 Claude 格式请求。
func (a *Adaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	return nil, errors.New("submodel channel: endpoint not supported")
}

// ConvertAudioRequest 未实现，子模型渠道不支持音频请求。
func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, errors.New("submodel channel: endpoint not supported")
}

// ConvertImageRequest 未实现，子模型渠道不支持图片生成请求。
func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	return nil, errors.New("submodel channel: endpoint not supported")
}

// Init 初始化适配器。子模型渠道无需额外初始化操作。
// 参数:
//   - info: 中继请求的上下文信息
func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

// GetRequestURL 构建子模型渠道的完整请求 URL。
// 直接拼接基础 URL 和请求路径。
// 参数:
//   - info: 包含基础 URL 和请求路径的中继信息
//
// 返回:
//   - string: 完整的请求 URL
//   - error: 始终返回 nil
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return relaycommon.GetFullRequestURL(info.ChannelBaseUrl, info.RequestURLPath, info.ChannelType), nil
}

// SetupRequestHeader 设置子模型渠道的请求头。
// 设置通用 API 请求头和 Bearer Token 认证。
// 参数:
//   - c: Gin 上下文
//   - req: HTTP 请求头指针
//   - info: 包含 API Key 等信息的中继信息
//
// 返回:
//   - error: 始终返回 nil
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", "Bearer "+info.ApiKey)
	return nil
}

// ConvertOpenAIRequest 将 OpenAI 格式请求直接透传。
// 子模型渠道兼容 OpenAI 格式，无需转换。
// 参数:
//   - c: Gin 上下文
//   - info: 中继信息
//   - request: OpenAI 格式的通用请求
//
// 返回:
//   - any: 原始请求（直接透传）
//   - error: 请求为 nil 时返回错误
func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	return request, nil
}

// ConvertRerankRequest 未实现，子模型渠道不支持 Rerank 请求。
func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, errors.New("submodel channel: endpoint not supported")
}

// ConvertEmbeddingRequest 未实现，子模型渠道不支持 Embedding 请求。
func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return nil, errors.New("submodel channel: endpoint not supported")
}

// ConvertOpenAIResponsesRequest 未实现，子模型渠道不支持 OpenAI Responses 请求。
func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return nil, errors.New("submodel channel: endpoint not supported")
}

// DoRequest 执行实际的 HTTP API 请求，委托给通用的 DoApiRequest 方法。
// 参数:
//   - c: Gin 上下文
//   - info: 中继信息
//   - requestBody: 请求体 io.Reader
//
// 返回:
//   - any: 原始响应
//   - error: 请求失败时返回错误
func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

// DoResponse 处理上游 API 的响应。
// 根据是否为流式请求，分别调用 OpenAI 适配器的流式或非流式处理器。
// 参数:
//   - c: Gin 上下文
//   - resp: 上游 HTTP 响应
//   - info: 中继信息
//
// 返回:
//   - usage: token 使用量
//   - err: 处理过程中的错误信息
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NexusTokError) {
	if info.IsStream {
		usage, err = openai.OaiStreamHandler(c, info, resp)
	} else {
		usage, err = openai.OpenaiHandler(c, info, resp)
	}
	return
}

// GetModelList 返回子模型渠道支持的模型列表。
// 返回:
//   - []string: 模型名称切片
func (a *Adaptor) GetModelList() []string {
	return ModelList
}

// GetChannelName 返回渠道名称标识 "submodel"。
// 返回:
//   - string: 渠道名称
func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
