// Package xai 实现了 xAI（Grok）平台的渠道适配器。
// 负责将统一请求转换为 xAI API 格式，处理搜索增强、推理模型特殊参数、
// 图片生成等功能。支持 Grok 系列语言模型和图片生成模型。
package xai

import (
	"errors"
	"io"
	"net/http"
	"strings"

	// 项目内部依赖
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/relay/channel"
	"github.com/c1cada/NexusTok/relay/channel/openai"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/types"

	"github.com/c1cada/NexusTok/relay/constant"

	// 第三方依赖
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

// Adaptor 是 xAI 渠道的适配器结构体。
// 实现了 channel.Adaptor 接口，支持文本补全、图片生成和 Responses 等功能。
type Adaptor struct {
}

// ConvertGeminiRequest 未实现，xAI 渠道不支持 Gemini 格式请求。
func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertClaudeRequest 未实现，xAI 渠道不支持 Claude 格式请求。
func (a *Adaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	//TODO implement me
	//panic("implement me")
	return nil, errors.New("not available")
}

// ConvertAudioRequest 未实现，xAI 渠道不支持音频请求。
func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//not available
	return nil, errors.New("not available")
}

// ConvertImageRequest 将统一的图片生成请求转换为 xAI API 格式。
// xAI 的图片 API 参数较简单，仅支持 model、prompt、n 和 response_format。
// 参数:
//   - c: Gin 上下文
//   - info: 中继信息
//   - request: 统一的图片请求结构体
//
// 返回:
//   - any: 转换后的 xAI 图片请求体
//   - error: 始终返回 nil
func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	xaiRequest := ImageRequest{
		Model:          request.Model,
		Prompt:         request.Prompt,
		N:              int(lo.FromPtrOr(request.N, uint(1))),
		ResponseFormat: request.ResponseFormat,
	}
	return xaiRequest, nil
}

// Init 初始化适配器。xAI 渠道无需额外初始化操作。
// 参数:
//   - info: 中继请求的上下文信息
func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

// GetRequestURL 构建 xAI API 的完整请求 URL。
// 参数:
//   - info: 包含基础 URL 和请求路径的中继信息
//
// 返回:
//   - string: 完整的请求 URL
//   - error: 始终返回 nil
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return relaycommon.GetFullRequestURL(info.ChannelBaseUrl, info.RequestURLPath, info.ChannelType), nil
}

// SetupRequestHeader 设置 xAI API 请求头。
// 设置通用 API 请求头和 Bearer Token 认证。
// 参数:
//   - c: Gin 上下文
//   - req: HTTP 请求头指针
//   - info: 包含 API Key 的中继信息
//
// 返回:
//   - error: 始终返回 nil
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", "Bearer "+info.ApiKey)
	return nil
}

// ConvertOpenAIRequest 将 OpenAI 格式请求转换为 xAI API 格式。
// 特殊处理逻辑：
// 1. 搜索增强模式：模型名以 "-search" 结尾时，去除后缀并添加 search_parameters
// 2. grok-3-mini 推理模型：
//   - 将 MaxTokens 转换为 MaxCompletionTokens（xAI 推理模型要求）
//   - 处理 "-high"/"-low" 后缀设置 reasoning_effort 参数
//
// 参数:
//   - c: Gin 上下文
//   - info: 中继信息
//   - request: OpenAI 格式的通用请求
//
// 返回:
//   - any: 转换后的请求体（可能是结构体或 map）
//   - error: 请求为 nil 时返回错误
func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	if strings.HasSuffix(info.UpstreamModelName, "-search") {
		info.UpstreamModelName = strings.TrimSuffix(info.UpstreamModelName, "-search")
		request.Model = info.UpstreamModelName
		toMap := request.ToMap()
		toMap["search_parameters"] = map[string]any{
			"mode": "on",
		}
		return toMap, nil
	}
	if strings.HasPrefix(request.Model, "grok-3-mini") {
		if lo.FromPtrOr(request.MaxCompletionTokens, uint(0)) == 0 && lo.FromPtrOr(request.MaxTokens, uint(0)) != 0 {
			request.MaxCompletionTokens = request.MaxTokens
			request.MaxTokens = nil
		}
		if strings.HasSuffix(request.Model, "-high") {
			request.ReasoningEffort = "high"
			request.Model = strings.TrimSuffix(request.Model, "-high")
		} else if strings.HasSuffix(request.Model, "-low") {
			request.ReasoningEffort = "low"
			request.Model = strings.TrimSuffix(request.Model, "-low")
		}
		info.ReasoningEffort = request.ReasoningEffort
		info.UpstreamModelName = request.Model
	}
	return request, nil
}

// ConvertRerankRequest 转换 Rerank 请求（当前返回 nil）。
func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

// ConvertEmbeddingRequest 未实现，xAI 渠道不支持 Embedding 请求。
func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	//not available
	return nil, errors.New("not available")
}

// ConvertOpenAIResponsesRequest 转换 OpenAI Responses 格式请求。
// 如果请求中未设置模型名，则使用中继信息中的上游模型名。
// 参数:
//   - c: Gin 上下文
//   - info: 中继信息
//   - request: OpenAI Responses 格式请求
//
// 返回:
//   - any: 转换后的请求体
//   - error: 始终返回 nil
func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	if request.Model == "" && info != nil {
		request.Model = info.UpstreamModelName
	}
	return request, nil
}

// DoRequest 执行实际的 HTTP API 请求，委托给通用的 DoApiRequest 方法。
func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

// DoResponse 处理 xAI API 的响应。
// 根据中继模式分发到不同的处理器：
// - 图片生成/编辑：使用 OpenAI 适配器的通用处理器
// - Responses 模式：使用 OpenAI 的 Responses 处理器（流式/非流式）
// - 文本补全：使用 xAI 自有的流式/非流式处理器
// 参数:
//   - c: Gin 上下文
//   - resp: 上游 HTTP 响应
//   - info: 中继信息
//
// 返回:
//   - usage: token 使用量
//   - err: 处理过程中的错误信息
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NexusTokError) {
	switch info.RelayMode {
	case constant.RelayModeImagesGenerations, constant.RelayModeImagesEdits:
		usage, err = openai.OpenaiHandlerWithUsage(c, info, resp)
	case constant.RelayModeResponses:
		if info.IsStream {
			usage, err = openai.OaiResponsesStreamHandler(c, info, resp)
		} else {
			usage, err = openai.OaiResponsesHandler(c, info, resp)
		}
	default:
		if info.IsStream {
			usage, err = xAIStreamHandler(c, info, resp)
		} else {
			usage, err = xAIHandler(c, info, resp)
		}
	}
	return
}

// GetModelList 返回 xAI 渠道支持的模型列表。
// 返回:
//   - []string: 模型名称切片（包含语言模型、搜索变体、推理变体、图片和视频模型）
func (a *Adaptor) GetModelList() []string {
	return ModelList
}

// GetChannelName 返回渠道名称标识 "xai"。
// 返回:
//   - string: 渠道名称
func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
