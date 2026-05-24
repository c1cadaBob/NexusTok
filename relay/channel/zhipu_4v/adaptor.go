// Package zhipu_4v 实现智谱 GLM-4V 多模态大模型的 API 适配器。
// GLM-4V 是智谱的视觉语言模型，支持文本、图片输入。
// 该适配器支持 OpenAI、Claude 和 Embedding 三种格式的请求，
// 并通过不同的 URL 路径路由到对应的 API 端点。
package zhipu_4v

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	// 项目内部依赖
	channelconstant "github.com/c1cada/NexusTok/constant"           // 渠道常量（URL 映射等）
	"github.com/c1cada/NexusTok/dto"                                  // 数据传输对象
	"github.com/c1cada/NexusTok/relay/channel"                        // 渠道通用工具函数
	"github.com/c1cada/NexusTok/relay/channel/claude"                 // Claude 适配器（用于 Claude 格式响应）
	"github.com/c1cada/NexusTok/relay/channel/openai"                 // OpenAI 适配器（用于 OpenAI 格式响应）
	relaycommon "github.com/c1cada/NexusTok/relay/common"             // 中继层通用结构体
	relayconstant "github.com/c1cada/NexusTok/relay/constant"         // 中继层常量
	"github.com/c1cada/NexusTok/types"                                // 错误类型
	"github.com/samber/lo"                                           // 泛型工具库

	// 第三方依赖
	"github.com/gin-gonic/gin" // HTTP 框架
)

// Adaptor 智谱 GLM-4V 适配器，实现 relay/channel 中定义的 Adaptor 接口。
// 支持多种请求格式（OpenAI、Claude、Embedding、Image），
// 根据 relayFormat 和 relayMode 动态选择下游处理逻辑。
type Adaptor struct {
}

// ConvertGeminiRequest 智谱不支持 Gemini 格式请求，返回未实现错误。
func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertClaudeRequest 将 Claude 格式请求直接透传（智谱 GLM-4V 兼容 Claude 格式）。
func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, req *dto.ClaudeRequest) (any, error) {
	return req, nil
}

// ConvertAudioRequest 智谱不支持音频请求，返回未实现错误。
func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertImageRequest 将图片请求直接透传。
func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	return request, nil
}

// Init 初始化适配器，智谱 GLM-4V 适配器无需特殊初始化。
func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

// GetRequestURL 根据请求格式和模式构建智谱 API 的请求 URL。
//
// 支持的路由：
//   - Claude 格式：/api/anthropic/v1/messages 或 /v1/messages（特殊域名）
//   - Embedding 模式：/api/paas/v4/embeddings
//   - 图片生成模式：/api/paas/v4/images/generations
//   - 默认（Chat）：/api/paas/v4/chat/completions
//
// 特殊域名（ChannelSpecialBases）可以覆盖默认的 URL 前缀。
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	baseURL := info.ChannelBaseUrl
	if baseURL == "" {
		baseURL = channelconstant.ChannelBaseURLs[channelconstant.ChannelTypeZhipu_v4]
	}
	specialPlan, hasSpecialPlan := channelconstant.ChannelSpecialBases[baseURL]

	switch info.RelayFormat {
	case types.RelayFormatClaude:
		// Claude 格式的消息端点
		if hasSpecialPlan && specialPlan.ClaudeBaseURL != "" {
			return fmt.Sprintf("%s/v1/messages", specialPlan.ClaudeBaseURL), nil
		}
		return fmt.Sprintf("%s/api/anthropic/v1/messages", baseURL), nil
	default:
		switch info.RelayMode {
		case relayconstant.RelayModeEmbeddings:
			// Embedding 端点
			if hasSpecialPlan && specialPlan.OpenAIBaseURL != "" {
				return fmt.Sprintf("%s/embeddings", specialPlan.OpenAIBaseURL), nil
			}
			return fmt.Sprintf("%s/api/paas/v4/embeddings", baseURL), nil
		case relayconstant.RelayModeImagesGenerations:
			// 图片生成端点
			if hasSpecialPlan && specialPlan.OpenAIBaseURL != "" {
				return fmt.Sprintf("%s/images/generations", specialPlan.OpenAIBaseURL), nil
			}
			return fmt.Sprintf("%s/api/paas/v4/images/generations", baseURL), nil
		default:
			// 默认聊天补全端点
			if hasSpecialPlan && specialPlan.OpenAIBaseURL != "" {
				return fmt.Sprintf("%s/chat/completions", specialPlan.OpenAIBaseURL), nil
			}
			return fmt.Sprintf("%s/api/paas/v4/chat/completions", baseURL), nil
		}
	}
}

// SetupRequestHeader 设置请求头，包括通用 API 头和 Bearer 令牌鉴权。
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", "Bearer "+info.ApiKey)
	return nil
}

// ConvertOpenAIRequest 将 OpenAI 格式的请求转换为智谱格式。
// 特殊处理：当 top_p >= 1 时，限制为 0.99。
func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	if lo.FromPtrOr(request.TopP, 0) >= 1 {
		request.TopP = lo.ToPtr(0.99)
	}
	return requestOpenAI2Zhipu(*request), nil
}

// ConvertRerankRequest 智谱不支持 Rerank 请求，返回 nil。
func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

// ConvertEmbeddingRequest 将 Embedding 请求直接透传。
func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return request, nil
}

// ConvertOpenAIResponsesRequest 智谱不支持 OpenAI Responses 格式，返回未实现错误。
func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	// TODO implement me
	return nil, errors.New("not implemented")
}

// DoRequest 发送 HTTP 请求到智谱 API。
func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

// DoResponse 处理智谱的响应，根据请求格式选择不同的处理方式：
//   - Claude 格式：委托给 Claude 适配器处理
//   - 图片生成模式：使用智谱自定义的图片处理函数
//   - 默认：委托给 OpenAI 适配器处理
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NexusTokError) {
	switch info.RelayFormat {
	case types.RelayFormatClaude:
		adaptor := claude.Adaptor{}
		return adaptor.DoResponse(c, resp, info)
	default:
		if info.RelayMode == relayconstant.RelayModeImagesGenerations {
			return zhipu4vImageHandler(c, resp, info)
		}
		adaptor := openai.Adaptor{}
		return adaptor.DoResponse(c, resp, info)
	}
}

// GetModelList 返回智谱 GLM-4V 支持的模型列表。
func (a *Adaptor) GetModelList() []string {
	return ModelList
}

// GetChannelName 返回渠道名称 "zhipu_4v"。
func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
