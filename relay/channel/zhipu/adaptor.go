// Package zhipu 实现智谱 ChatGLM 大模型的 API 适配器。
// 负责将 OpenAI 格式的请求转换为智谱 API 格式，
// 处理 JWT 鉴权，并将智谱的流式/非流式响应转换回 OpenAI 兼容格式。
package zhipu

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	// 项目内部依赖
	"github.com/c1cada/NexusTok/dto"                // 数据传输对象
	"github.com/c1cada/NexusTok/relay/channel"        // 渠道通用工具函数
	relaycommon "github.com/c1cada/NexusTok/relay/common" // 中继层通用结构体
	"github.com/c1cada/NexusTok/types"                // 错误类型
	"github.com/samber/lo"                           // 泛型工具库

	// 第三方依赖
	"github.com/gin-gonic/gin" // HTTP 框架
)

// Adaptor 智谱 ChatGLM 适配器，实现 relay/channel 中定义的 Adaptor 接口。
// 智谱使用标准 HTTP 协议，通过 SSE 实现流式响应。
type Adaptor struct {
}

// ConvertGeminiRequest 智谱不支持 Gemini 格式请求，返回未实现错误。
func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertClaudeRequest 智谱不支持 Claude 格式请求，返回未实现错误。
func (a *Adaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	//TODO implement me
	panic("implement me")
	return nil, nil
}

// ConvertAudioRequest 智谱不支持音频请求，返回未实现错误。
func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertImageRequest 智谱不支持图片请求，返回未实现错误。
func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// Init 初始化适配器，智谱适配器无需特殊初始化。
func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

// GetRequestURL 构建智谱 API 的请求 URL。
//
// URL 格式：{baseURL}/api/paas/v3/model-api/{model}/{method}
// 其中 method 根据是否流式请求为 "invoke" 或 "sse-invoke"。
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	method := "invoke"
	if info.IsStream {
		method = "sse-invoke"
	}
	return fmt.Sprintf("%s/api/paas/v3/model-api/%s/%s", info.ChannelBaseUrl, info.UpstreamModelName, method), nil
}

// SetupRequestHeader 设置请求头，包括通用 API 头和 JWT 鉴权令牌。
// 智谱使用自定义的 JWT 令牌进行鉴权，令牌由 API Key 的 id.secret 格式生成。
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	token := getZhipuToken(info.ApiKey)
	req.Set("Authorization", token)
	return nil
}

// ConvertOpenAIRequest 将 OpenAI 格式的请求转换为智谱格式。
// 特殊处理：当 top_p >= 1 时，限制为 0.99（智谱不接受 1.0）。
func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	// 智谱要求 top_p < 1，限制最大值为 0.99
	if lo.FromPtrOr(request.TopP, 0) >= 1 {
		request.TopP = lo.ToPtr(0.99)
	}
	return requestOpenAI2Zhipu(*request), nil
}

// ConvertRerankRequest 智谱不支持 Rerank 请求，返回 nil。
func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

// ConvertEmbeddingRequest 智谱不支持 Embedding 请求，返回未实现错误。
func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
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

// DoResponse 处理智谱的响应，根据是否流式分发到不同的处理函数。
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NexusTokError) {
	if info.IsStream {
		usage, err = zhipuStreamHandler(c, info, resp)
	} else {
		usage, err = zhipuHandler(c, info, resp)
	}
	return
}

// GetModelList 返回智谱支持的模型列表。
func (a *Adaptor) GetModelList() []string {
	return ModelList
}

// GetChannelName 返回渠道名称 "zhipu"。
func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
