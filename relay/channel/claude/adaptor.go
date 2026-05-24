// Package claude 实现 Anthropic Claude AI 平台的渠道适配器。
// 该文件定义了 Adaptor 结构体及其方法，负责请求 URL 构建、请求头设置、
// OpenAI 格式到 Claude Messages API 格式的请求转换和响应处理。
package claude

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/relay/channel"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/setting/model_setting"
	"github.com/c1cada/NexusTok/types"

	"github.com/gin-gonic/gin"
)

// Adaptor 是 Claude 渠道的适配器结构体。
// 实现了 channel.Adaptor 接口，提供从 OpenAI 格式到 Claude Messages API 格式的请求转换能力。
type Adaptor struct {
}

// ConvertGeminiRequest 将 Gemini 格式的请求转换为 Claude 格式。
// 当前未实现，直接返回错误。
func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertClaudeRequest 将 Claude 格式的请求直接透传。
// 由于请求已经是 Claude 原生格式，无需转换。
func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	return request, nil
}

// ConvertAudioRequest 将音频请求转换为 Claude 格式。
// 当前未实现，Claude 暂不支持音频请求。
func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertImageRequest 将图像请求转换为 Claude 格式。
// 当前未实现。
func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// Init 初始化 Claude 渠道适配器。
// 当前为空实现。
func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

// GetRequestURL 构建 Claude Messages API 的完整请求 URL。
// 基础 URL 格式: {baseUrl}/v1/messages
// 如果需要启用 Claude Beta 功能，会在 URL 中添加 ?beta=true 查询参数。
//
// 参数:
//   - info: 中继信息，包含渠道基础 URL 和 Beta 配置
//
// 返回值:
//   - string: 完整的 API 请求 URL
//   - error: URL 解析失败时返回错误
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	requestURL := fmt.Sprintf("%s/v1/messages", info.ChannelBaseUrl)
	if !shouldAppendClaudeBetaQuery(info) {
		return requestURL, nil
	}

	parsedURL, err := url.Parse(requestURL)
	if err != nil {
		return "", err
	}
	query := parsedURL.Query()
	query.Set("beta", "true")
	parsedURL.RawQuery = query.Encode()
	return parsedURL.String(), nil
}

// shouldAppendClaudeBetaQuery 判断是否需要在请求 URL 中添加 Claude Beta 查询参数。
// 当中继信息中设置了 IsClaudeBetaQuery 或渠道配置中启用了 ClaudeBetaQuery 时返回 true。
//
// 参数:
//   - info: 中继信息
//
// 返回值:
//   - bool: 是否需要添加 Beta 查询参数
func shouldAppendClaudeBetaQuery(info *relaycommon.RelayInfo) bool {
	if info == nil {
		return false
	}
	if info.IsClaudeBetaQuery {
		return true
	}
	if info.ChannelOtherSettings.ClaudeBetaQuery {
		return true
	}
	return false
}

// CommonClaudeHeadersOperation 设置 Claude API 的通用请求头。
// 包括：
//   - 透传客户端的 anthropic-beta 头（如果有）
//   - 根据模型配置写入特定的请求头（通过 model_setting）
//
// 参数:
//   - c: gin 请求上下文
//   - req: 待设置的 HTTP 请求头指针
//   - info: 中继信息
func CommonClaudeHeadersOperation(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) {
	// common headers operation
	anthropicBeta := c.Request.Header.Get("anthropic-beta")
	if anthropicBeta != "" {
		req.Set("anthropic-beta", anthropicBeta)
	}
	model_setting.GetClaudeSettings().WriteHeaders(info.OriginModelName, req)
}

// SetupRequestHeader 设置 Claude API 请求的 HTTP 头部。
// 设置的头部包括：
//   - 通用 API 请求头
//   - x-api-key: Claude API 密钥
//   - anthropic-version: API 版本号（默认 "2023-06-01"，客户端可覆盖）
//   - Claude 特定的通用头部（通过 CommonClaudeHeadersOperation）
//
// 参数:
//   - c: gin 请求上下文
//   - req: 待设置的 HTTP 请求头指针
//   - info: 中继信息，包含 API Key
//
// 返回值:
//   - error: 始终返回 nil
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("x-api-key", info.ApiKey)
	anthropicVersion := c.Request.Header.Get("anthropic-version")
	if anthropicVersion == "" {
		anthropicVersion = "2023-06-01"
	}
	req.Set("anthropic-version", anthropicVersion)
	CommonClaudeHeadersOperation(c, req, info)
	return nil
}

// ConvertOpenAIRequest 将 OpenAI 格式的对话请求转换为 Claude Messages API 格式。
// 调用 RequestOpenAI2ClaudeMessage 进行格式转换，处理消息格式、系统提示词、
// 工具调用等 OpenAI 与 Claude 之间的差异。
//
// 参数:
//   - c: gin 请求上下文
//   - info: 中继信息
//   - request: OpenAI 格式的通用请求
//
// 返回值:
//   - any: 转换后的 Claude 格式请求
//   - error: 请求为 nil 时返回错误
func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	return RequestOpenAI2ClaudeMessage(c, *request)
}

// ConvertRerankRequest 将重排序请求转换为 Claude 格式。
// Claude 不支持重排序功能，始终返回 nil。
func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

// ConvertEmbeddingRequest 将向量化请求转换为 Claude 格式。
// 当前未实现，Claude 暂不支持向量化功能。
func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertOpenAIResponsesRequest 将 OpenAI Responses 格式的请求转换为 Claude 格式。
// 当前未实现。
func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	// TODO implement me
	return nil, errors.New("not implemented")
}

// DoRequest 执行向 Claude API 发送 HTTP 请求。
// 委托给 channel.DoApiRequest 通用请求方法处理。
func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

// DoResponse 处理 Claude API 的响应。
// 设置最终请求的中继格式为 Claude，然后根据是否为流式请求分别调用：
//   - 流式请求: ClaudeStreamHandler
//   - 非流式请求: ClaudeHandler
//
// 参数:
//   - c: gin 请求上下文
//   - resp: Claude 上游 API 的 HTTP 响应
//   - info: 中继信息
//
// 返回值:
//   - usage: token 使用量统计
//   - err: 处理过程中的错误
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NexusTokError) {
	info.FinalRequestRelayFormat = types.RelayFormatClaude
	if info.IsStream {
		return ClaudeStreamHandler(c, resp, info)
	} else {
		return ClaudeHandler(c, resp, info)
	}
}

// GetModelList 返回 Claude 渠道支持的模型列表。
func (a *Adaptor) GetModelList() []string {
	return ModelList
}

// GetChannelName 返回 Claude 渠道的名称标识。
func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
