// Package dify 实现了 Dify 渠道适配器。
// Dify 是一个开源的 LLM 应用开发平台，支持多种 Bot 类型：
// ChatFlow（聊天流）、Agent（智能体）、WorkFlow（工作流）和 Completion（补全）。
// 该适配器将 OpenAI 格式的请求转换为 Dify 原生格式。
package dify

// 标准库导入
import (
	"errors"
	"fmt"
	"io"
	"net/http"

	// 项目内部包
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/relay/channel"         // 通用渠道工具
	relaycommon "github.com/c1cada/NexusTok/relay/common" // relay 层公共工具
	"github.com/c1cada/NexusTok/types"

	// 第三方依赖
	"github.com/gin-gonic/gin"
)

// BotType 常量定义了 Dify 平台支持的 Bot 类型。
// 不同类型对应不同的 API 端点。
const (
	BotTypeChatFlow   = 1 // ChatFlow 类型（默认），使用 /v1/chat-messages 端点
	BotTypeAgent      = 2 // Agent 类型，使用 /v1/chat-messages 端点
	BotTypeWorkFlow   = 3 // WorkFlow 类型，使用 /v1/workflows/run 端点
	BotTypeCompletion = 4 // Completion 类型，使用 /v1/completion-messages 端点
)

// Adaptor 是 Dify 渠道的适配器，实现了 channel.Adaptor 接口。
// BotType 字段标识当前 Bot 的类型，影响请求 URL 的选择。
type Adaptor struct {
	BotType int // Bot 类型，决定使用哪个 API 端点
}

// ConvertGeminiRequest Dify 渠道不支持 Gemini 请求格式。
func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertClaudeRequest Dify 渠道不支持 Claude 请求格式。
func (a *Adaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	//TODO implement me
	panic("implement me")
	return nil, nil
}

// ConvertAudioRequest Dify 渠道不支持音频请求。
func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertImageRequest Dify 渠道不支持图像请求。
func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// Init 初始化适配器，设置 Bot 类型。
// 当前默认使用 ChatFlow 类型。
// 参数：info - 中继请求信息。
func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
	// 可根据模型名前缀区分 Bot 类型（当前注释掉了，统一使用 ChatFlow）
	//if strings.HasPrefix(info.UpstreamModelName, "agent") {
	//	a.BotType = BotTypeAgent
	//} else if strings.HasPrefix(info.UpstreamModelName, "workflow") {
	//	a.BotType = BotTypeWorkFlow
	//} else if strings.HasPrefix(info.UpstreamModelName, "chat") {
	//	a.BotType = BotTypeCompletion
	//} else {
	//}
	a.BotType = BotTypeChatFlow

}

// GetRequestURL 构建发送到 Dify 上游服务的请求 URL。
// 根据 Bot 类型返回不同的端点：
//   - BotTypeWorkFlow: /v1/workflows/run
//   - BotTypeCompletion: /v1/completion-messages
//   - BotTypeAgent / 默认: /v1/chat-messages
//
// 参数：info - 中继请求信息。
// 返回值：完整的请求 URL 和可能的错误。
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	switch a.BotType {
	case BotTypeWorkFlow:
		return fmt.Sprintf("%s/v1/workflows/run", info.ChannelBaseUrl), nil
	case BotTypeCompletion:
		return fmt.Sprintf("%s/v1/completion-messages", info.ChannelBaseUrl), nil
	case BotTypeAgent:
		fallthrough
	default:
		return fmt.Sprintf("%s/v1/chat-messages", info.ChannelBaseUrl), nil
	}
}

// SetupRequestHeader 设置发送到 Dify 上游服务的 HTTP 请求头。
// 设置通用请求头和 Bearer Token 认证。
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", "Bearer "+info.ApiKey)
	return nil
}

// ConvertOpenAIRequest 将 OpenAI 格式的聊天请求转换为 Dify 格式。
// 参数：c - Gin 上下文，info - 中继请求信息，request - OpenAI 请求体。
func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	return requestOpenAI2Dify(c, info, *request), nil
}

// ConvertRerankRequest Dify 渠道不支持重排序请求。
func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

// ConvertEmbeddingRequest Dify 渠道不支持 Embedding 请求。
func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertOpenAIResponsesRequest Dify 渠道不支持 OpenAI Responses API。
func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	// TODO implement me
	return nil, errors.New("not implemented")
}

// DoRequest 发送 API 请求到上游 Dify 服务。
func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

// DoResponse 处理上游 Dify 服务的响应。
// 根据是否为流式请求分发到不同的处理器：
//   - 流式模式：使用 difyStreamHandler
//   - 非流式模式：使用 difyHandler
//
// 返回值：usage 用量信息和可能的错误。
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NexusTokError) {
	if info.IsStream {
		return difyStreamHandler(c, info, resp)
	} else {
		return difyHandler(c, info, resp)
	}
	return
}

// GetModelList 返回 Dify 渠道支持的模型列表。
func (a *Adaptor) GetModelList() []string {
	return ModelList
}

// GetChannelName 返回渠道名称 "dify"。
func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
