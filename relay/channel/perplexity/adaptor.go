// Package perplexity 实现 Perplexity AI 渠道的适配器
package perplexity

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/c1cada/NexusTok/dto"                   // 数据传输对象
	"github.com/c1cada/NexusTok/relay/channel"          // 渠道通用工具
	"github.com/c1cada/NexusTok/relay/channel/openai"   // OpenAI 适配器（用于响应处理）
	relaycommon "github.com/c1cada/NexusTok/relay/common" // 中继通用类型
	relayconstant "github.com/c1cada/NexusTok/relay/constant" // 中继常量
	"github.com/c1cada/NexusTok/types"                  // 错误类型
	"github.com/samber/lo"                              // 泛型工具库

	"github.com/gin-gonic/gin" // Gin Web 框架
)

// Adaptor Perplexity 渠道适配器
// 实现了 relay.Adaptor 接口，支持聊天完成和 Responses API
type Adaptor struct {
}

// ConvertGeminiRequest Gemini 格式请求转换（未实现）
func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertClaudeRequest Claude 格式请求转换
// 委托给 OpenAI 适配器处理，因为 Perplexity 兼容 OpenAI 格式
func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, req *dto.ClaudeRequest) (any, error) {
	adaptor := openai.Adaptor{}
	return adaptor.ConvertClaudeRequest(c, info, req)
}

// ConvertAudioRequest 音频请求转换（未实现）
func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertImageRequest 图像请求转换（未实现）
func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// Init 初始化适配器
func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

// GetRequestURL 构建请求 URL
// 根据中继模式返回对应的 API 端点
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info.RelayMode == relayconstant.RelayModeResponses {
		return fmt.Sprintf("%s/v1/responses", info.ChannelBaseUrl), nil
	}
	return fmt.Sprintf("%s/chat/completions", info.ChannelBaseUrl), nil
}

// SetupRequestHeader 设置请求头
// 配置 API 密钥和通用请求头
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", "Bearer "+info.ApiKey)
	return nil
}

// ConvertOpenAIRequest OpenAI 格式请求转换
// 将 OpenAI 格式转换为 Perplexity 格式，处理 TopP 参数限制
func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	// Perplexity 要求 TopP < 1
	if lo.FromPtrOr(request.TopP, 0) >= 1 {
		request.TopP = lo.ToPtr(0.99)
	}
	return requestOpenAI2Perplexity(*request), nil
}

// ConvertRerankRequest 重排序请求转换（未实现）
func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

// ConvertEmbeddingRequest 嵌入请求转换（未实现）
func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertOpenAIResponsesRequest Responses API 请求转换
// Perplexity 直接透传 Responses 格式请求
func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return request, nil
}

// DoRequest 执行 API 请求
func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

// DoResponse 处理 API 响应
// 委托给 OpenAI 适配器处理响应
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NexusTokError) {
	adaptor := openai.Adaptor{}
	usage, err = adaptor.DoResponse(c, resp, info)
	return
}

// GetModelList 获取支持的模型列表
func (a *Adaptor) GetModelList() []string {
	return ModelList
}

// GetChannelName 获取渠道名称
func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
