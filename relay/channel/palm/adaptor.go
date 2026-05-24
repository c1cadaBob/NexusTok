// Package palm 实现 Google PaLM (Pathways Language Model) 的适配器。
// PaLM 是 Google 的大语言模型，本适配器负责：
// - 请求格式转换（OpenAI 格式到 PaLM 格式）
// - 响应格式转换（PaLM 格式到 OpenAI 格式）
// - 流式和非流式响应处理
// - 使用 Google 的 generateMessage API
package palm

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	// 项目内部包
	"github.com/c1cada/NexusTok/dto"                                 // 数据传输对象
	"github.com/c1cada/NexusTok/relay/channel"                       // 渠道通用工具
	relaycommon "github.com/c1cada/NexusTok/relay/common"            // Relay 通用信息
	"github.com/c1cada/NexusTok/service"                             // 服务层（使用量估算等）
	"github.com/c1cada/NexusTok/types"                               // 类型定义（错误类型等）

	// 第三方依赖
	"github.com/gin-gonic/gin"                                       // Gin Web 框架
)

// Adaptor 是 PaLM 渠道的适配器实现。
// PaLM 使用 Google 自有的 API 格式，需要进行格式转换。
type Adaptor struct {
}

// ConvertGeminiRequest 将 Gemini 格式请求转换为 PaLM 上游格式。
// PaLM 不支持 Gemini 格式，返回未实现错误。
// 参数:
//   - c: Gin 上下文
//   - info: Relay 信息
//   - request: Gemini 格式的聊天请求
// 返回:
//   - any: 转换后的请求体（此处不会返回）
//   - error: 始终返回 "not implemented" 错误
func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertClaudeRequest 将 Claude 格式请求转换为 PaLM 上游格式。
// PaLM 不支持 Claude 格式，此方法未实现。
// 参数:
//   - c: Gin 上下文
//   - info: Relay 信息
//   - request: Claude 格式请求
// 返回:
//   - any: 转换后的请求体（此处不会返回）
//   - error: 始终 panic
func (a *Adaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	//TODO implement me
	panic("implement me")
	return nil, nil
}

// ConvertAudioRequest 将音频请求转换为 PaLM 上游格式。
// PaLM 不支持音频请求，返回未实现错误。
// 参数:
//   - c: Gin 上下文
//   - info: Relay 信息
//   - request: 音频请求
// 返回:
//   - io.Reader: 请求体（此处不会返回）
//   - error: 始终返回 "not implemented" 错误
func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertImageRequest 将图片请求转换为 PaLM 上游格式。
// PaLM 不支持图片生成请求，返回未实现错误。
// 参数:
//   - c: Gin 上下文
//   - info: Relay 信息
//   - request: 图片请求
// 返回:
//   - any: 转换后的请求体（此处不会返回）
//   - error: 始终返回 "not implemented" 错误
func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// Init 初始化适配器。
// PaLM 适配器无需特殊初始化逻辑。
// 参数:
//   - info: Relay 信息
func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

// GetRequestURL 构建上游请求的完整 URL。
// PaLM 使用 Google 的 generateMessage API 端点。
// 参数:
//   - info: Relay 信息（包含 ChannelBaseUrl 等）
// 返回:
//   - string: 完整的上游请求 URL
//   - error: URL 构建过程中的错误（此处始终返回 nil）
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return fmt.Sprintf("%s/v1beta2/models/chat-bison-001:generateMessage", info.ChannelBaseUrl), nil
}

// SetupRequestHeader 设置上游请求的 HTTP 头部。
// PaLM 使用 x-goog-api-key 头部进行认证（Google API 风格）。
// 参数:
//   - c: Gin 上下文
//   - req: HTTP 请求头指针
//   - info: Relay 信息（包含 ApiKey 等）
// 返回:
//   - error: 始终返回 nil
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("x-goog-api-key", info.ApiKey)
	return nil
}

// ConvertOpenAIRequest 将 OpenAI 格式请求转换为 PaLM 上游格式。
// PaLM 兼容 OpenAI 请求格式，直接返回原请求。
// 参数:
//   - c: Gin 上下文
//   - info: Relay 信息
//   - request: OpenAI 格式请求
// 返回:
//   - any: 原请求体
//   - error: 请求为 nil 时返回错误
func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	return request, nil
}

// ConvertRerankRequest 将 Rerank 请求转换为 PaLM 上游格式。
// PaLM 不支持 Rerank，返回 nil。
// 参数:
//   - c: Gin 上下文
//   - relayMode: Relay 模式
//   - request: Rerank 请求
// 返回:
//   - any: nil
//   - error: nil
func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

// ConvertEmbeddingRequest 将嵌入请求转换为 PaLM 上游格式。
// PaLM 不支持嵌入请求，返回未实现错误。
// 参数:
//   - c: Gin 上下文
//   - info: Relay 信息
//   - request: 嵌入请求
// 返回:
//   - any: 转换后的请求体（此处不会返回）
//   - error: 始终返回 "not implemented" 错误
func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertOpenAIResponsesRequest 将 OpenAI Responses 格式请求转换为上游格式。
// PaLM 不支持 Responses API，返回未实现错误。
// 参数:
//   - c: Gin 上下文
//   - info: Relay 信息
//   - request: OpenAI Responses 格式请求
// 返回:
//   - any: 转换后的请求体（此处不会返回）
//   - error: 始终返回 "not implemented" 错误
func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	// TODO implement me
	return nil, errors.New("not implemented")
}

// DoRequest 执行上游 HTTP 请求。
// 委托给通用的 channel.DoApiRequest 方法处理。
// 参数:
//   - c: Gin 上下文
//   - info: Relay 信息
//   - requestBody: 请求体的 io.Reader
// 返回:
//   - any: 原始 HTTP 响应
//   - error: 请求过程中的错误
func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

// DoResponse 处理上游响应并转换为客户端格式。
// 根据 IsStream 标志选择不同的处理器：
//   - 流式：使用 palmStreamHandler
//   - 非流式：使用 palmHandler
//
// 参数:
//   - c: Gin 上下文
//   - resp: 上游 HTTP 响应
//   - info: Relay 信息
// 返回:
//   - any: 使用量统计
//   - *types.NexusTokError: 错误信息（成功时为 nil）
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NexusTokError) {
	if info.IsStream {
		var responseText string
		err, responseText = palmStreamHandler(c, resp)
		usage = service.ResponseText2Usage(c, responseText, info.UpstreamModelName, info.GetEstimatePromptTokens())
	} else {
		usage, err = palmHandler(c, info, resp)
	}
	return
}

// GetModelList 返回 PaLM 支持的模型列表。
// 返回:
//   - []string: 模型名称列表
func (a *Adaptor) GetModelList() []string {
	return ModelList
}

// GetChannelName 返回渠道名称。
// 返回:
//   - string: 渠道名称 "google palm"
func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
