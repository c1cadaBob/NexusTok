// Package cohere 实现了 Cohere 渠道适配器。
// Cohere 提供大语言模型服务，支持聊天补全和重排序（Rerank）功能。
// 该适配器将 OpenAI 格式的请求转换为 Cohere 原生格式，并将响应
// 转换回 OpenAI 兼容格式。
package cohere

// 标准库导入
import (
	"errors"
	"fmt"
	"io"
	"net/http"

	// 项目内部包
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/relay/channel"         // 通用渠道工具函数
	relaycommon "github.com/c1cada/NexusTok/relay/common" // relay 层公共工具
	"github.com/c1cada/NexusTok/relay/constant"          // relay 层常量
	"github.com/c1cada/NexusTok/types"

	// 第三方依赖
	"github.com/gin-gonic/gin"
)

// Adaptor 是 Cohere 渠道的适配器，实现了 channel.Adaptor 接口。
// 支持聊天补全（/v1/chat）和重排序（/v1/rerank）端点。
type Adaptor struct {
}

// ConvertGeminiRequest Cohere 渠道不支持 Gemini 请求格式。
func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertClaudeRequest Cohere 渠道不支持 Claude 请求格式。
func (a *Adaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	//TODO implement me
	panic("implement me")
	return nil, nil
}

// ConvertAudioRequest Cohere 渠道不支持音频请求。
func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertImageRequest Cohere 渠道不支持图像请求。
func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// Init 初始化适配器，Cohere 渠道无需额外初始化。
func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

// GetRequestURL 构建发送到 Cohere 上游服务的请求 URL。
// 根据 relay 模式返回不同的端点：
//   - RelayModeRerank: /v1/rerank（重排序端点）
//   - 其他模式: /v1/chat（聊天补全端点）
//
// 参数：info - 中继请求信息。
// 返回值：完整的请求 URL 和可能的错误。
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info.RelayMode == constant.RelayModeRerank {
		return fmt.Sprintf("%s/v1/rerank", info.ChannelBaseUrl), nil
	} else {
		return fmt.Sprintf("%s/v1/chat", info.ChannelBaseUrl), nil
	}
}

// SetupRequestHeader 设置发送到 Cohere 上游服务的 HTTP 请求头。
// 设置通用请求头和 Bearer Token 认证。
// 参数：c - Gin 上下文，req - 请求头，info - 中继请求信息。
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", fmt.Sprintf("Bearer %s", info.ApiKey))
	return nil
}

// ConvertOpenAIRequest 将 OpenAI 格式的聊天请求转换为 Cohere 格式。
// 参数：c - Gin 上下文，info - 中继请求信息，request - OpenAI 请求体。
// 返回值：转换后的 Cohere 请求体和可能的错误。
func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	return requestOpenAI2Cohere(*request), nil
}

// ConvertOpenAIResponsesRequest Cohere 渠道不支持 OpenAI Responses API。
func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	// TODO implement me
	return nil, errors.New("not implemented")
}

// DoRequest 发送 API 请求到上游 Cohere 服务。
// 参数：c - Gin 上下文，info - 中继请求信息，requestBody - 请求体 reader。
func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

// ConvertRerankRequest 将通用重排序请求转换为 Cohere 重排序格式。
// 参数：c - Gin 上下文，relayMode - 中继模式，request - 重排序请求体。
func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return requestConvertRerank2Cohere(request), nil
}

// ConvertEmbeddingRequest Cohere 渠道不支持 Embedding 请求。
func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// DoResponse 处理上游 Cohere 服务的响应。
// 根据请求模式分发到不同的处理器：
//   - RelayModeRerank: 使用 cohereRerankHandler 处理重排序响应
//   - 流式模式: 使用 cohereStreamHandler 处理流式聊天响应
//   - 非流式模式: 使用 cohereHandler 处理普通聊天响应
//
// 返回值：usage 用量信息和可能的错误。
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NexusTokError) {
	if info.RelayMode == constant.RelayModeRerank {
		usage, err = cohereRerankHandler(c, resp, info)
	} else {
		if info.IsStream {
			usage, err = cohereStreamHandler(c, info, resp) // TODO: fix this
		} else {
			usage, err = cohereHandler(c, info, resp)
		}
	}
	return
}

// GetModelList 返回 Cohere 渠道支持的模型列表。
func (a *Adaptor) GetModelList() []string {
	return ModelList
}

// GetChannelName 返回渠道名称 "cohere"。
func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
