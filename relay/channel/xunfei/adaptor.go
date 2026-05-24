// Package xunfei 实现讯飞星火大模型的 API 适配器。
// 负责将 OpenAI 格式的请求转换为讯飞星火 WebSocket 协议格式，
// 并将讯飞的响应转换回 OpenAI 兼容格式。
package xunfei

import (
	"errors"
	"io"
	"net/http"
	"strings"

	// 项目内部依赖
	"github.com/c1cada/NexusTok/dto"                // 数据传输对象定义
	"github.com/c1cada/NexusTok/relay/channel"        // 渠道通用工具函数
	relaycommon "github.com/c1cada/NexusTok/relay/common" // 中继层通用结构体和接口
	"github.com/c1cada/NexusTok/types"                // 错误类型定义

	// 第三方依赖
	"github.com/gin-gonic/gin" // HTTP 框架
)

// Adaptor 讯飞星火适配器，实现 relay/channel 中定义的 Adaptor 接口。
// 注意：讯飞星火使用 WebSocket 协议而非 HTTP，因此 DoRequest 返回虚拟 HTTP 响应，
// 实际的请求发送和响应接收在 DoResponse 中通过 WebSocket 完成。
type Adaptor struct {
	request *dto.GeneralOpenAIRequest // 缓存的 OpenAI 格式请求，供 DoResponse 使用
}

// ConvertGeminiRequest 讯飞不支持 Gemini 格式请求，返回未实现错误。
func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertClaudeRequest 讯飞不支持 Claude 格式请求，返回未实现错误。
func (a *Adaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	//TODO implement me
	panic("implement me")
	return nil, nil
}

// ConvertAudioRequest 讯飞不支持音频请求，返回未实现错误。
func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertImageRequest 讯飞不支持图片请求，返回未实现错误。
func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// Init 初始化适配器，讯飞适配器无需特殊初始化。
func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

// GetRequestURL 返回空字符串，因为讯飞使用 WebSocket 协议，不通过 HTTP 请求。
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return "", nil
}

// SetupRequestHeader 设置通用的 API 请求头。
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	return nil
}

// ConvertOpenAIRequest 将 OpenAI 格式的请求转换为讯飞格式。
// 讯飞直接使用 OpenAI 请求结构体，但会缓存请求以供后续 DoResponse 使用。
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继信息
//   - request: OpenAI 格式的请求
//
// 返回：转换后的请求对象（原样返回）和可能的错误。
func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	a.request = request // 缓存请求，供 DoResponse 中通过 WebSocket 发送
	return request, nil
}

// ConvertRerankRequest 讯飞不支持 Rerank 请求，返回 nil。
func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

// ConvertEmbeddingRequest 讯飞不支持 Embedding 请求，返回未实现错误。
func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertOpenAIResponsesRequest 讯飞不支持 OpenAI Responses 格式，返回未实现错误。
func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	// TODO implement me
	return nil, errors.New("not implemented")
}

// DoRequest 讯飞使用 WebSocket 而非 HTTP，因此这里返回一个虚拟的 HTTP 200 响应。
// 实际的请求发送在 DoResponse 中完成。
func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	dummyResp := &http.Response{}
	dummyResp.StatusCode = http.StatusOK
	return dummyResp, nil
}

// DoResponse 处理讯飞的响应。讯飞使用 WebSocket 协议，所以在这里完成实际的请求-响应流程。
//
// 参数：
//   - c: Gin 上下文
//   - resp: 虚拟的 HTTP 响应（未使用）
//   - info: 中继信息，包含 API Key（格式为 "appId|apiKey|apiSecret"）
//
// 返回：token 使用量统计和可能的错误。
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NexusTokError) {
	// 讯飞 API Key 格式为 "appId|apiKey|apiSecret"，用竖线分隔
	splits := strings.Split(info.ApiKey, "|")
	if len(splits) != 3 {
		return nil, types.NewError(errors.New("invalid auth"), types.ErrorCodeChannelInvalidKey)
	}
	if a.request == nil {
		return nil, types.NewError(errors.New("request is nil"), types.ErrorCodeInvalidRequest)
	}
	// 根据是否为流式请求，调用不同的处理函数
	if info.IsStream {
		usage, err = xunfeiStreamHandler(c, *a.request, splits[0], splits[1], splits[2])
	} else {
		usage, err = xunfeiHandler(c, *a.request, splits[0], splits[1], splits[2])
	}
	return
}

// GetModelList 返回讯飞支持的模型列表。
func (a *Adaptor) GetModelList() []string {
	return ModelList
}

// GetChannelName 返回渠道名称 "xunfei"。
func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
