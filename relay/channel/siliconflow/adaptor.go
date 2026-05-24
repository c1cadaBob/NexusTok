// Package siliconflow 实现 SiliconFlow AI 平台的渠道适配器。
// SiliconFlow 是一个兼容 OpenAI API 格式的 AI 代理平台，
// 因此大部分请求/响应处理逻辑委托给 OpenAI 适配器实现，
// 仅在图片生成和 Rerank 等特殊场景进行自定义处理。
package siliconflow

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/relay/channel"
	"github.com/c1cada/NexusTok/relay/channel/openai"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/relay/constant"
	"github.com/c1cada/NexusTok/types"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

// Adaptor 是 SiliconFlow 渠道的适配器结构体。
// 实现了 channel.Adaptor 接口，支持文本补全、图片生成、Rerank 和 Embedding 等功能。
// 由于 SiliconFlow 兼容 OpenAI API，大部分请求和响应处理直接委托给 OpenAI 适配器。
type Adaptor struct {
}

// ConvertGeminiRequest 未实现，SiliconFlow 渠道不支持 Gemini 格式请求。
// 参数:
//   - c: Gin 上下文
//   - info: 中继信息
//   - request: Gemini 格式的聊天请求
// 返回:
//   - any: 始终返回 nil
//   - error: 始终返回 "not implemented" 错误
func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertClaudeRequest 将 Claude 格式请求转换为 OpenAI 格式。
// SiliconFlow 兼容 OpenAI 格式，因此委托给 OpenAI 适配器完成转换。
// 参数:
//   - c: Gin 上下文
//   - info: 中继信息
//   - req: Claude 格式的请求体
// 返回:
//   - any: 转换后的请求体
//   - error: 转换失败时返回错误
func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, req *dto.ClaudeRequest) (any, error) {
	adaptor := openai.Adaptor{}
	return adaptor.ConvertClaudeRequest(c, info, req)
}

// ConvertAudioRequest 将音频请求转换为 SiliconFlow 格式。
// SiliconFlow 兼容 OpenAI 的音频 API，因此委托给 OpenAI 适配器处理。
// 参数:
//   - c: Gin 上下文
//   - info: 中继信息
//   - request: 音频请求体
// 返回:
//   - io.Reader: 转换后的请求体读取器
//   - error: 转换失败时返回错误
func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	adaptor := openai.Adaptor{}
	return adaptor.ConvertAudioRequest(c, info, request)
}

// ConvertImageRequest 将通用图片生成请求转换为 SiliconFlow 的 SFImageRequest 格式。
// SiliconFlow 支持额外的图片生成参数（如 image_size、batch_size、negative_prompt 等），
// 这些参数通过 request.Extra 传入并解析到 SFImageRequest 中。
// 若未指定 image_size/batch_size，则回退使用 OpenAI 标准的 size/n 字段。
// 参数:
//   - c: Gin 上下文
//   - info: 中继信息
//   - request: 统一的图片生成请求
// 返回:
//   - any: 转换后的 SFImageRequest
//   - error: 始终返回 nil
func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	// 解析extra到SFImageRequest里，以填入SiliconFlow特殊字段。若失败重建一个空的。
	sfRequest := &SFImageRequest{}
	extra, err := common.Marshal(request.Extra)
	if err == nil {
		err = common.Unmarshal(extra, sfRequest)
		if err != nil {
			sfRequest = &SFImageRequest{}
		}
	}

	sfRequest.Model = request.Model
	sfRequest.Prompt = request.Prompt
	// 优先使用image_size/batch_size，否则使用OpenAI标准的size/n
	if sfRequest.ImageSize == "" {
		sfRequest.ImageSize = request.Size
	}
	if sfRequest.BatchSize == 0 {
		if request.N != nil {
			sfRequest.BatchSize = lo.FromPtr(request.N)
		}
	}

	return sfRequest, nil
}

// Init 初始化 SiliconFlow 适配器。当前无需执行任何初始化操作。
// 参数:
//   - info: 中继请求的上下文信息
func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

// GetRequestURL 构建 SiliconFlow API 的完整请求 URL。
// 对于 Rerank 模式，使用专用的 /v1/rerank 端点；
// 其他模式使用通用的 URL 拼接方式。
// 参数:
//   - info: 包含基础 URL 和请求路径的中继信息
// 返回:
//   - string: 完整的请求 URL
//   - error: 始终返回 nil
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info.RelayMode == constant.RelayModeRerank {
		return fmt.Sprintf("%s/v1/rerank", info.ChannelBaseUrl), nil
	}
	return relaycommon.GetFullRequestURL(info.ChannelBaseUrl, info.RequestURLPath, info.ChannelType), nil
}

// SetupRequestHeader 设置 SiliconFlow API 的请求头。
// 包括通用 API 请求头和 Bearer Token 认证。
// 参数:
//   - c: Gin 上下文
//   - req: HTTP 请求头指针
//   - info: 包含 API Key 的中继信息
// 返回:
//   - error: 始终返回 nil
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", fmt.Sprintf("Bearer %s", info.ApiKey))
	return nil
}

// ConvertOpenAIRequest 将 OpenAI 格式的请求转换为 SiliconFlow 兼容格式。
// 特殊处理：对于 FIM（Fill-In-the-Middle）补全请求（包含 Prefix 或 Suffix），
// SiliconFlow 要求 messages 数组不为空，因此会添加一条空的 user 消息以满足要求。
// 参数:
//   - c: Gin 上下文
//   - info: 中继信息
//   - request: OpenAI 格式的通用请求
// 返回:
//   - any: 转换后的请求体
//   - error: 始终返回 nil
func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	// SiliconFlow requires messages array for FIM requests, even if client doesn't send it
	if (request.Prefix != nil || request.Suffix != nil) && len(request.Messages) == 0 {
		// Add an empty user message to satisfy SiliconFlow's requirement
		request.Messages = []dto.Message{
			{
				Role:    "user",
				Content: "",
			},
		}
	}
	return request, nil
}

// ConvertOpenAIResponsesRequest 未实现，SiliconFlow 渠道不支持 OpenAI Responses 格式请求。
// 参数:
//   - c: Gin 上下文
//   - info: 中继信息
//   - request: OpenAI Responses 格式的请求
// 返回:
//   - any: 始终返回 nil
//   - error: 始终返回 "not implemented" 错误
func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	// TODO implement me
	return nil, errors.New("not implemented")
}

// DoRequest 执行实际的 HTTP API 请求。
// SiliconFlow 兼容 OpenAI 格式，委托给 OpenAI 适配器的 DoRequest 方法。
// 参数:
//   - c: Gin 上下文
//   - info: 中继信息
//   - requestBody: 请求体读取器
// 返回:
//   - any: 响应结果
//   - error: 请求失败时返回错误
func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	adaptor := openai.Adaptor{}
	return adaptor.DoRequest(c, info, requestBody)
}

// ConvertRerankRequest 将 Rerank 请求直接透传。
// SiliconFlow 的 Rerank API 与标准格式兼容，无需转换。
// 参数:
//   - c: Gin 上下文
//   - relayMode: 中继模式
//   - request: Rerank 请求体
// 返回:
//   - any: 原始请求体
//   - error: 始终返回 nil
func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return request, nil
}

// ConvertEmbeddingRequest 将 Embedding 请求直接透传。
// SiliconFlow 的 Embedding API 与 OpenAI 标准格式兼容，无需转换。
// 参数:
//   - c: Gin 上下文
//   - info: 中继信息
//   - request: Embedding 请求体
// 返回:
//   - any: 原始请求体
//   - error: 始终返回 nil
func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return request, nil
}

// DoResponse 处理 SiliconFlow API 的响应。
// 对于 Rerank 模式使用自定义的 siliconflowRerankHandler；
// 其他模式委托给 OpenAI 适配器处理。
// 参数:
//   - c: Gin 上下文
//   - resp: 上游 HTTP 响应
//   - info: 中继信息
// 返回:
//   - usage: token 使用量统计
//   - err: 处理过程中的错误信息
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NexusTokError) {
	switch info.RelayMode {
	case constant.RelayModeRerank:
		usage, err = siliconflowRerankHandler(c, info, resp)
	default:
		adaptor := openai.Adaptor{}
		usage, err = adaptor.DoResponse(c, resp, info)
	}
	return
}

// GetModelList 返回 SiliconFlow 渠道支持的模型列表。
// 返回:
//   - []string: 模型名称切片
func (a *Adaptor) GetModelList() []string {
	return ModelList
}

// GetChannelName 返回渠道名称标识 "siliconflow"。
// 返回:
//   - string: 渠道名称
func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
