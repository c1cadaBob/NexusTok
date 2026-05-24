// Package jina 实现 Jina AI 通道的适配器。
// Jina 提供 Rerank（重排序）和 Embedding（向量嵌入）两种能力。
// 不支持文本对话、图片生成和音频处理。
package jina

// 标准库导入
import (
	"errors" // 错误创建
	"fmt"    // 格式化字符串
	"io"     // IO 读写接口
	"net/http" // HTTP 客户端和响应处理

	// 项目内部依赖
	"github.com/c1cada/NexusTok/dto"                     // 数据传输对象定义
	"github.com/c1cada/NexusTok/relay/channel"            // 通道通用工具函数
	"github.com/c1cada/NexusTok/relay/channel/openai"     // OpenAI 通道处理器（用于 Embedding 响应处理）
	relaycommon "github.com/c1cada/NexusTok/relay/common"   // Relay 通用模块
	"github.com/c1cada/NexusTok/relay/common_handler"     // 公共处理器（Rerank 响应处理）
	"github.com/c1cada/NexusTok/relay/constant"           // Relay 常量定义
	"github.com/c1cada/NexusTok/types"                    // 公共类型定义

	// 第三方依赖
	"github.com/gin-gonic/gin" // Gin Web 框架
)

// Adaptor Jina AI 通道适配器。
// 支持 Rerank（重排序）和 Embedding（向量嵌入）两种请求模式。
type Adaptor struct {
}

// ConvertGeminiRequest 将 Gemini 格式请求转换为 Jina 格式（未实现）。
func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertClaudeRequest 将 Claude 格式请求转换为 Jina 格式（未实现）。
func (a *Adaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	//TODO implement me
	panic("implement me")
	return nil, nil
}

// ConvertAudioRequest 音频请求转换（Jina 不支持）。
func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertImageRequest 图片请求转换（Jina 不支持）。
func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// Init 初始化 Jina 适配器（无特殊初始化操作）。
func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

// GetRequestURL 根据 RelayMode 构建 Jina API 的请求 URL。
// - Rerank 模式: {baseUrl}/v1/rerank
// - Embedding 模式: {baseUrl}/v1/embeddings
// 参数:
//   - info: Relay 信息，包含基础 URL 和请求模式
// 返回:
//   - string: 完整的 API 请求 URL
//   - error: 不支持的模式时返回错误
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info.RelayMode == constant.RelayModeRerank {
		return fmt.Sprintf("%s/v1/rerank", info.ChannelBaseUrl), nil
	} else if info.RelayMode == constant.RelayModeEmbeddings {
		return fmt.Sprintf("%s/v1/embeddings", info.ChannelBaseUrl), nil
	}
	return "", errors.New("invalid relay mode")
}

// SetupRequestHeader 设置 Jina API 请求头。
// 使用标准 API 请求头设置，并添加 Bearer Token 认证。
// 参数:
//   - c: Gin 上下文
//   - req: HTTP 请求头指针
//   - info: Relay 信息，包含 API Key
// 返回: error 错误信息
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", fmt.Sprintf("Bearer %s", info.ApiKey))
	return nil
}

// ConvertOpenAIRequest 将 OpenAI 格式请求转换为 Jina 格式。
// 当前直接透传原始请求。
func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	return request, nil
}

// ConvertOpenAIResponsesRequest OpenAI Responses API 请求转换（未实现）。
func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	// TODO implement me
	return nil, errors.New("not implemented")
}

// DoRequest 执行向 Jina API 的 HTTP 请求。
// 委托给通道通用的 DoApiRequest 函数处理。
func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

// ConvertRerankRequest 将 Rerank 请求透传给 Jina API。
func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return request, nil
}

// ConvertEmbeddingRequest 将 Embedding 请求转换为 Jina 格式。
// 清除 encoding_format 字段，因为 Jina 不支持该参数。
func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	request.EncodingFormat = ""
	return request, nil
}

// DoResponse 处理 Jina API 的响应。
// 根据 RelayMode 分发到不同的处理器：
//   - Rerank 模式: 使用 RerankHandler 处理重排序响应
//   - Embedding 模式: 使用 OpenAI 格式处理器处理嵌入响应
//
// 参数:
//   - c: Gin 上下文
//   - resp: Jina API 返回的 HTTP 响应
//   - info: Relay 信息
// 返回:
//   - usage: token 使用量信息
//   - err: 错误信息
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NexusTokError) {
	if info.RelayMode == constant.RelayModeRerank {
		usage, err = common_handler.RerankHandler(c, info, resp)
	} else if info.RelayMode == constant.RelayModeEmbeddings {
		usage, err = openai.OpenaiHandler(c, info, resp)
	}
	return
}

// GetModelList 返回 Jina 支持的模型列表。
func (a *Adaptor) GetModelList() []string {
	return ModelList
}

// GetChannelName 返回通道名称。
func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
