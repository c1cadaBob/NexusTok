// Package mistral 实现 Mistral AI 通道的适配器。
// Mistral AI 是一家法国 AI 公司，提供多种开源和商业大语言模型。
// 该适配器将 OpenAI 格式请求转换为 Mistral 格式，处理 tool_call_id 格式兼容等问题。
// 支持流式和非流式文本对话，其他功能（嵌入、音频、图片等）暂未实现。
package mistral

// 标准库导入
import (
	"errors" // 错误创建
	"io"     // IO 读写接口
	"net/http" // HTTP 客户端和响应处理

	// 项目内部依赖
	"github.com/c1cada/NexusTok/dto"                     // 数据传输对象定义
	"github.com/c1cada/NexusTok/relay/channel"            // 通道通用工具函数
	"github.com/c1cada/NexusTok/relay/channel/openai"     // OpenAI 通道处理器（用于流式/非流式响应处理）
	relaycommon "github.com/c1cada/NexusTok/relay/common"   // Relay 通用模块
	"github.com/c1cada/NexusTok/types"                    // 公共类型定义

	// 第三方依赖
	"github.com/gin-gonic/gin" // Gin Web 框架
)

// Adaptor Mistral AI 通道适配器。
// 实现了 ChannelAdaptor 接口，负责请求格式转换和响应处理。
type Adaptor struct {
}

// ConvertGeminiRequest 将 Gemini 格式请求转换为 Mistral 格式（未实现）。
func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertClaudeRequest 将 Claude 格式请求转换为 Mistral 格式（未实现）。
func (a *Adaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	//TODO implement me
	panic("implement me")
	return nil, nil
}

// ConvertAudioRequest 音频请求转换（Mistral 不支持）。
func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertImageRequest 图片请求转换（Mistral 不支持）。
func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// Init 初始化 Mistral 适配器（无特殊初始化操作）。
func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

// GetRequestURL 构建 Mistral API 的请求 URL。
// 使用通用的 URL 拼接方式，基于渠道基础 URL 和请求路径。
// 参数:
//   - info: Relay 信息，包含渠道基础 URL 和请求路径
// 返回:
//   - string: 完整的 API 请求 URL
//   - error: 错误信息（当前始终返回 nil）
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return relaycommon.GetFullRequestURL(info.ChannelBaseUrl, info.RequestURLPath, info.ChannelType), nil
}

// SetupRequestHeader 设置 Mistral API 请求头。
// 使用标准 API 请求头设置，并添加 Bearer Token 认证。
// 参数:
//   - c: Gin 上下文
//   - req: HTTP 请求头指针
//   - info: Relay 信息，包含 API Key
// 返回: error 错误信息
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", "Bearer "+info.ApiKey)
	return nil
}

// ConvertOpenAIRequest 将 OpenAI 格式请求转换为 Mistral 格式。
// 委托给 requestOpenAI2Mistral 函数进行格式转换，主要处理：
// - tool_call_id 格式兼容（Mistral 要求 9 位字母数字 ID）
// - 图片 URL 格式标准化
// - 消息结构精简
// 参数:
//   - c: Gin 上下文
//   - info: Relay 信息
//   - request: OpenAI 格式的通用请求对象
// 返回:
//   - any: Mistral 格式的请求体
//   - error: 请求为 nil 时返回错误
func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	return requestOpenAI2Mistral(request), nil
}

// ConvertRerankRequest Rerank 请求透传。
func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

// ConvertEmbeddingRequest Embedding 请求转换（未实现）。
func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertOpenAIResponsesRequest OpenAI Responses API 请求转换（未实现）。
func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	// TODO implement me
	return nil, errors.New("not implemented")
}

// DoRequest 执行向 Mistral API 的 HTTP 请求。
// 委托给通道通用的 DoApiRequest 函数处理。
func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

// DoResponse 处理 Mistral API 的响应。
// Mistral 响应格式与 OpenAI 兼容，直接委托给 OpenAI 处理器。
// 流式模式使用 OaiStreamHandler，非流式模式使用 OpenaiHandler。
// 参数:
//   - c: Gin 上下文
//   - resp: Mistral API 返回的 HTTP 响应
//   - info: Relay 信息
// 返回:
//   - usage: token 使用量信息
//   - err: 错误信息
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NexusTokError) {
	if info.IsStream {
		usage, err = openai.OaiStreamHandler(c, info, resp)
	} else {
		usage, err = openai.OpenaiHandler(c, info, resp)
	}
	return
}

// GetModelList 返回 Mistral 支持的模型列表。
func (a *Adaptor) GetModelList() []string {
	return ModelList
}

// GetChannelName 返回通道名称。
func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
