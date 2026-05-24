// Package ollama 实现 Ollama 本地模型服务的适配器。
// Ollama 是一个本地运行大语言模型的工具，本适配器负责：
// - 请求格式转换（OpenAI/Claude 格式到 Ollama 原生格式）
// - 响应格式转换（Ollama 原生格式到 OpenAI 格式）
// - 支持聊天（/api/chat）、生成（/api/generate）和嵌入（/api/embed）三种端点
// - 流式和非流式响应处理
package ollama

import (
	"errors"
	"io"
	"net/http"
	"strings"

	// 项目内部包
	"github.com/c1cada/NexusTok/dto"                                 // 数据传输对象
	"github.com/c1cada/NexusTok/relay/channel"                       // 渠道通用工具
	"github.com/c1cada/NexusTok/relay/channel/openai"                // OpenAI 适配器（用于格式转换中转）
	relaycommon "github.com/c1cada/NexusTok/relay/common"            // Relay 通用信息
	relayconstant "github.com/c1cada/NexusTok/relay/constant"        // Relay 常量（RelayMode 等）
	"github.com/c1cada/NexusTok/types"                               // 类型定义（错误类型等）

	// 第三方依赖
	"github.com/gin-gonic/gin"                                       // Gin Web 框架
)

// Adaptor 是 Ollama 渠道的适配器实现。
// Ollama 使用自有的 API 格式，与 OpenAI 格式不同，需要进行格式转换。
type Adaptor struct {
}

// ConvertGeminiRequest 将 Gemini 格式请求转换为 Ollama 上游格式。
// Ollama 不支持 Gemini 格式，返回未实现错误。
// 参数:
//   - c: Gin 上下文
//   - info: Relay 信息
//   - request: Gemini 格式的聊天请求
// 返回:
//   - any: 转换后的请求体（此处不会返回）
//   - error: 始终返回 "not implemented" 错误
func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	return nil, errors.New("not implemented")
}

// ConvertClaudeRequest 将 Claude 格式请求转换为 Ollama 上游格式。
// 转换路径：Claude 格式 -> OpenAI 格式 -> Ollama 聊天格式。
// 会自动启用 StreamOptions 以获取使用量统计。
// 参数:
//   - c: Gin 上下文
//   - info: Relay 信息
//   - request: Claude 格式请求
// 返回:
//   - any: Ollama 聊天格式请求
//   - error: 转换过程中的错误
func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	openaiAdaptor := openai.Adaptor{}
	openaiRequest, err := openaiAdaptor.ConvertClaudeRequest(c, info, request)
	if err != nil {
		return nil, err
	}
	// 启用 StreamOptions 以获取使用量统计
	openaiRequest.(*dto.GeneralOpenAIRequest).StreamOptions = &dto.StreamOptions{
		IncludeUsage: true,
	}
	// 转换路径：Claude -> OpenAI -> Ollama 聊天格式
	return openAIChatToOllamaChat(c, openaiRequest.(*dto.GeneralOpenAIRequest))
}

// ConvertAudioRequest 将音频请求转换为 Ollama 上游格式。
// Ollama 不支持音频请求，返回未实现错误。
// 参数:
//   - c: Gin 上下文
//   - info: Relay 信息
//   - request: 音频请求
// 返回:
//   - io.Reader: 请求体（此处不会返回）
//   - error: 始终返回 "not implemented" 错误
func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, errors.New("not implemented")
}

// ConvertImageRequest 将图片请求转换为 Ollama 上游格式。
// Ollama 不支持图片生成请求，返回未实现错误。
// 参数:
//   - c: Gin 上下文
//   - info: Relay 信息
//   - request: 图片请求
// 返回:
//   - any: 转换后的请求体（此处不会返回）
//   - error: 始终返回 "not implemented" 错误
func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	return nil, errors.New("not implemented")
}

// Init 初始化适配器。
// Ollama 适配器无需特殊初始化逻辑。
// 参数:
//   - info: Relay 信息
func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

// GetRequestURL 构建上游请求的完整 URL。
// 根据 RelayMode 和请求路径选择 Ollama 的端点：
//   - 嵌入请求: /api/embed
//   - 补全请求（/v1/completions 或 RelayModeCompletions）: /api/generate
//   - 聊天请求: /api/chat
//
// 参数:
//   - info: Relay 信息（包含 ChannelBaseUrl、RelayMode、RequestURLPath 等）
// 返回:
//   - string: 完整的上游请求 URL
//   - error: URL 构建过程中的错误（此处始终返回 nil）
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	// 嵌入请求使用 /api/embed 端点
	if info.RelayMode == relayconstant.RelayModeEmbeddings {
		return info.ChannelBaseUrl + "/api/embed", nil
	}
	// 补全请求使用 /api/generate 端点
	if strings.Contains(info.RequestURLPath, "/v1/completions") || info.RelayMode == relayconstant.RelayModeCompletions {
		return info.ChannelBaseUrl + "/api/generate", nil
	}
	// 默认聊天请求使用 /api/chat 端点
	return info.ChannelBaseUrl + "/api/chat", nil
}

// SetupRequestHeader 设置上游请求的 HTTP 头部。
// 包含通用 API 请求头和 Bearer Token 认证头。
// 参数:
//   - c: Gin 上下文
//   - req: HTTP 请求头指针
//   - info: Relay 信息（包含 ApiKey 等）
// 返回:
//   - error: 始终返回 nil
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", "Bearer "+info.ApiKey)
	return nil
}

// ConvertOpenAIRequest 将 OpenAI 格式请求转换为 Ollama 上游格式。
// 根据请求路径和 RelayMode 决定转换目标：
//   - 补全请求：转换为 Ollama 生成格式
//   - 聊天请求：转换为 Ollama 聊天格式
//
// 参数:
//   - c: Gin 上下文
//   - info: Relay 信息
//   - request: OpenAI 格式请求
// 返回:
//   - any: Ollama 格式请求
//   - error: 转换过程中的错误
func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	// 根据请求路径判断是生成还是聊天
	if strings.Contains(info.RequestURLPath, "/v1/completions") || info.RelayMode == relayconstant.RelayModeCompletions {
		return openAIToGenerate(c, request)
	}
	return openAIChatToOllamaChat(c, request)
}

// ConvertRerankRequest 将 Rerank 请求转换为 Ollama 上游格式。
// Ollama 不支持 Rerank，返回 nil。
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

// ConvertEmbeddingRequest 将嵌入请求转换为 Ollama 上游格式。
// 使用 requestOpenAI2Embeddings 函数进行格式转换。
// 参数:
//   - c: Gin 上下文
//   - info: Relay 信息
//   - request: 嵌入请求
// 返回:
//   - any: Ollama 嵌入格式请求
//   - error: 始终返回 nil
func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return requestOpenAI2Embeddings(request), nil
}

// ConvertOpenAIResponsesRequest 将 OpenAI Responses 格式请求转换为上游格式。
// Ollama 不支持 Responses API，返回未实现错误。
// 参数:
//   - c: Gin 上下文
//   - info: Relay 信息
//   - request: OpenAI Responses 格式请求
// 返回:
//   - any: 转换后的请求体（此处不会返回）
//   - error: 始终返回 "not implemented" 错误
func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
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
// 根据 RelayMode 和 IsStream 选择不同的处理器：
//   - 嵌入请求：使用 ollamaEmbeddingHandler
//   - 流式聊天/生成：使用 ollamaStreamHandler
//   - 非流式聊天/生成：使用 ollamaChatHandler
//
// 参数:
//   - c: Gin 上下文
//   - resp: 上游 HTTP 响应
//   - info: Relay 信息
// 返回:
//   - any: 使用量统计
//   - *types.NexusTokError: 错误信息（成功时为 nil）
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NexusTokError) {
	switch info.RelayMode {
	case relayconstant.RelayModeEmbeddings:
		return ollamaEmbeddingHandler(c, info, resp)
	default:
		if info.IsStream {
			return ollamaStreamHandler(c, info, resp)
		}
		return ollamaChatHandler(c, info, resp)
	}
}

// GetModelList 返回 Ollama 支持的模型列表。
// 返回:
//   - []string: 模型名称列表
func (a *Adaptor) GetModelList() []string {
	return ModelList
}

// GetChannelName 返回渠道名称。
// 返回:
//   - string: 渠道名称 "ollama"
func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
