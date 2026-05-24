// cloudflare - adaptor.go
// 本文件实现了 Cloudflare Workers AI 渠道的请求适配器。
// 负责将 OpenAI 兼容格式的请求转换为 Cloudflare AI API 格式，
// 并将 Cloudflare AI 的响应转换回 OpenAI 兼容格式。
// 支持的中继模式包括：聊天补全（ChatCompletions）、嵌入（Embeddings）、
// Responses 接口、音频转录（AudioTranscription/Translation）以及通用 AI 模型调用。
package cloudflare

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/relay/channel"
	"github.com/c1cada/NexusTok/relay/channel/openai"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/relay/constant"
	"github.com/c1cada/NexusTok/types"

	"github.com/gin-gonic/gin"
)

// Adaptor 是 Cloudflare Workers AI 渠道的适配器结构体。
// 实现了 channel.Adaptor 接口，提供请求转换、响应处理等能力。
type Adaptor struct {
}

// ConvertGeminiRequest 将 Gemini 格式的请求转换为 Cloudflare 格式。
// 当前尚未实现，调用将返回错误。
// 参数：ctx - Gin 上下文；info - 中继请求信息；request - Gemini 聊天请求体。
// 返回值：始终返回 nil 和 "not implemented" 错误。
func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertClaudeRequest 将 Claude 格式的请求转换为 Cloudflare 格式。
// 当前尚未实现，调用将触发 panic。
// 参数：ctx - Gin 上下文；info - 中继请求信息；request - Claude 请求体。
func (a *Adaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	//TODO implement me
	panic("implement me")
	return nil, nil
}

// Init 初始化适配器，在中继请求开始前调用。
// 当前为空实现，Cloudflare 渠道无需额外初始化。
// 参数：info - 中继请求信息。
func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

// GetRequestURL 根据中继模式构建 Cloudflare AI API 的请求 URL。
// Cloudflare AI API 的 URL 格式遵循 https://api.cloudflare.com/client/v4/accounts/{account_id}/ai/... 模式。
// 中继模式与 URL 的对应关系：
//   - RelayModeChatCompletions: .../ai/v1/chat/completions
//   - RelayModeEmbeddings: .../ai/v1/embeddings
//   - RelayModeResponses: .../ai/v1/responses
//   - 其他模式（通用模型调用）: .../ai/run/{model_name}
//
// 参数：info - 中继请求信息，包含 ChannelBaseUrl 和 ApiVersion（用作 AccountID）。
// 返回值：构建完成的请求 URL 和可能的错误。
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	switch info.RelayMode {
	case constant.RelayModeChatCompletions:
		return fmt.Sprintf("%s/client/v4/accounts/%s/ai/v1/chat/completions", info.ChannelBaseUrl, info.ApiVersion), nil
	case constant.RelayModeEmbeddings:
		return fmt.Sprintf("%s/client/v4/accounts/%s/ai/v1/embeddings", info.ChannelBaseUrl, info.ApiVersion), nil
	case constant.RelayModeResponses:
		return fmt.Sprintf("%s/client/v4/accounts/%s/ai/v1/responses", info.ChannelBaseUrl, info.ApiVersion), nil
	default:
		return fmt.Sprintf("%s/client/v4/accounts/%s/ai/run/%s", info.ChannelBaseUrl, info.ApiVersion, info.UpstreamModelName), nil
	}
}

// SetupRequestHeader 设置发送到 Cloudflare AI API 的请求头。
// 设置标准 API 请求头，并通过 Bearer Token 方式设置授权头。
// 参数：c - Gin 上下文；req - 请求头指针；info - 中继请求信息。
// 返回值：始终返回 nil。
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", fmt.Sprintf("Bearer %s", info.ApiKey))
	return nil
}

// ConvertOpenAIRequest 将 OpenAI 格式的请求转换为 Cloudflare 兼容格式。
// 对于 completions 模式，调用 convertCf2CompletionsRequest 进行特殊转换；
// 其他模式直接透传原始请求。
// 参数：c - Gin 上下文；info - 中继请求信息；request - OpenAI 通用请求体。
// 返回值：转换后的请求体和可能的错误。
func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	switch info.RelayMode {
	case constant.RelayModeCompletions:
		return convertCf2CompletionsRequest(*request), nil
	default:
		return request, nil
	}
}

// ConvertOpenAIResponsesRequest 将 OpenAI Responses 格式的请求转换为 Cloudflare 兼容格式。
// Cloudflare 原生支持 Responses API，因此直接透传原始请求。
// 参数：c - Gin 上下文；info - 中继请求信息；request - OpenAI Responses 请求体。
// 返回值：原始请求体和 nil 错误。
func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return request, nil
}

// DoRequest 执行向 Cloudflare AI API 的实际 HTTP 请求。
// 内部调用 channel.DoApiRequest 统一处理请求发送逻辑。
// 参数：c - Gin 上下文；info - 中继请求信息；requestBody - 请求体 Reader。
// 返回值：HTTP 响应和可能的错误。
func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

// ConvertRerankRequest 将通用重排序请求转换为 Cloudflare 兼容格式。
// Cloudflare 直接透传重排序请求，不做额外转换。
// 参数：c - Gin 上下文；relayMode - 中继模式；request - 重排序请求体。
// 返回值：原始请求体和 nil 错误。
func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return request, nil
}

// ConvertEmbeddingRequest 将通用嵌入请求转换为 Cloudflare 兼容格式。
// Cloudflare 直接透传嵌入请求，不做额外转换。
// 参数：c - Gin 上下文；info - 中继请求信息；request - 嵌入请求体。
// 返回值：原始请求体和 nil 错误。
func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return request, nil
}

// ConvertAudioRequest 将音频请求转换为 Cloudflare 兼容格式。
// 从 multipart 表单中提取上传的音频文件，将其内容读取到内存缓冲区中返回。
// 参数：c - Gin 上下文；info - 中继请求信息；request - 音频请求体。
// 返回值：包含音频文件内容的 io.Reader 和可能的错误。
func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	// 添加文件字段
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		return nil, errors.New("file is required")
	}
	defer file.Close()
	// 打开临时文件用于保存上传的文件内容
	requestBody := &bytes.Buffer{}

	// 将上传的文件内容复制到临时文件
	if _, err := io.Copy(requestBody, file); err != nil {
		return nil, err
	}
	return requestBody, nil
}

// ConvertImageRequest 将图像生成请求转换为 Cloudflare 兼容格式。
// 当前尚未实现，调用将返回错误。
// 参数：c - Gin 上下文；info - 中继请求信息；request - 图像请求体。
// 返回值：始终返回 nil 和 "not implemented" 错误。
func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// DoResponse 处理 Cloudflare AI API 的响应并转换为 OpenAI 兼容格式。
// 根据中继模式分发到不同的处理函数：
//   - 聊天补全/嵌入模式：流式使用 cfStreamHandler，非流式使用 cfHandler
//   - Responses 模式：复用 OpenAI 的 OaiResponsesStreamHandler/OaiResponsesHandler
//   - 音频转录/翻译模式：使用 cfSTTHandler
//
// 参数：c - Gin 上下文；resp - 上游 HTTP 响应；info - 中继请求信息。
// 返回值：usage 用量信息和可能的 NexusTokError 错误。
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NexusTokError) {
	switch info.RelayMode {
	case constant.RelayModeEmbeddings:
		fallthrough
	case constant.RelayModeChatCompletions:
		if info.IsStream {
			err, usage = cfStreamHandler(c, info, resp)
		} else {
			err, usage = cfHandler(c, info, resp)
		}
	case constant.RelayModeResponses:
		if info.IsStream {
			usage, err = openai.OaiResponsesStreamHandler(c, info, resp)
		} else {
			usage, err = openai.OaiResponsesHandler(c, info, resp)
		}
	case constant.RelayModeAudioTranslation:
		fallthrough
	case constant.RelayModeAudioTranscription:
		err, usage = cfSTTHandler(c, info, resp)
	}
	return
}

// GetModelList 返回 Cloudflare 渠道支持的模型列表。
// 返回值：模型名称字符串切片，数据来源于 constant.go 中定义的 ModelList。
func (a *Adaptor) GetModelList() []string {
	return ModelList
}

// GetChannelName 返回 Cloudflare 渠道的名称标识符。
// 返回值：渠道名称字符串，数据来源于 constant.go 中定义的 ChannelName。
func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
