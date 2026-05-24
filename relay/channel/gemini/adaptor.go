// gemini - adaptor.go
// Google Gemini 渠道的适配器实现。
// 本文件实现了 relay/channel 包中定义的 Adaptor 接口，负责：
//   - 将 OpenAI / Claude / Gemini 原生请求转换为 Gemini API 格式
//   - 构建 Gemini API 的请求 URL（支持聊天、嵌入、图像生成等端点）
//   - 设置 HTTP 请求头（包含 API Key 认证）
//   - 将 Gemini API 的响应转换为 OpenAI 兼容格式
//   - 处理文本生成、图像生成、嵌入等不同类型的请求和响应
package gemini

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/relay/channel"
	"github.com/c1cada/NexusTok/relay/channel/openai"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/relay/constant"
	"github.com/c1cada/NexusTok/setting/model_setting"
	"github.com/c1cada/NexusTok/setting/reasoning"
	"github.com/c1cada/NexusTok/types"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

// Adaptor 是 Google Gemini 渠道的适配器结构体。
// 实现了 relay/channel 包中定义的适配器接口，提供请求格式转换、URL 构建、
// HTTP 头设置以及响应处理等能力。该结构体本身无状态，所有状态通过方法参数传递。
type Adaptor struct {
}

// ConvertGeminiRequest 将 Gemini 原生格式的请求进行预处理。
// 主要处理：
//   - 如果第一条消息的 Role 为空，默认设置为 "user"
//   - 对包含 YouTube 链接的 FileData，自动补充 "video/webm" 的 MimeType
//
// 参数:
//   - c: Gin 上下文
//   - info: Relay 中继信息（包含渠道配置、模型名称等）
//   - request: Gemini 原生聊天请求
//
// 返回:
//   - any: 处理后的请求对象（原样返回）
//   - error: 错误信息，处理成功时为 nil
func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	if len(request.Contents) > 0 {
		for i, content := range request.Contents {
			if i == 0 {
				if request.Contents[0].Role == "" {
					request.Contents[0].Role = "user"
				}
			}
			for _, part := range content.Parts {
				if part.FileData != nil {
					if part.FileData.MimeType == "" && strings.Contains(part.FileData.FileUri, "www.youtube.com") {
						part.FileData.MimeType = "video/webm"
					}
				}
			}
		}
	}
	return request, nil
}

// ConvertClaudeRequest 将 Claude 格式的请求转换为 Gemini 格式。
// 转换流程：Claude 格式 -> OpenAI 格式 -> Gemini 格式（通过 OpenAI 适配器中转）。
// 参数:
//   - c: Gin 上下文
//   - info: Relay 中继信息
//   - req: Claude 格式的请求对象
//
// 返回:
//   - any: 转换后的 Gemini 格式请求
//   - error: 转换过程中的错误
func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, req *dto.ClaudeRequest) (any, error) {
	adaptor := openai.Adaptor{}
	oaiReq, err := adaptor.ConvertClaudeRequest(c, info, req)
	if err != nil {
		return nil, err
	}
	return a.ConvertOpenAIRequest(c, info, oaiReq.(*dto.GeneralOpenAIRequest))
}

// ConvertAudioRequest 将音频请求转换为 Gemini 格式。
// 当前尚未实现，调用时会返回 "not implemented" 错误。
// 参数:
//   - c: Gin 上下文
//   - info: Relay 中继信息
//   - request: 音频请求对象
//
// 返回:
//   - io.Reader: 请求体读取器
//   - error: 始终返回未实现错误
func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertImageRequest 将图像生成请求转换为 Gemini Imagen 格式。
// 仅支持 imagen 系列模型。转换逻辑包括：
//   - 将 OpenAI 的 size 参数转换为 Gemini 的 aspectRatio（宽高比）
//   - 将 OpenAI 的 quality 参数映射为 Gemini 的 imageSize（1K 或 2K）
//   - 支持直接传入宽高比格式（如 "16:9"）
//
// 参数:
//   - c: Gin 上下文
//   - info: Relay 中继信息
//   - request: OpenAI 格式的图像生成请求
//
// 返回:
//   - any: Gemini Imagen 格式的请求对象
//   - error: 模型不支持时返回错误
func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	if !strings.HasPrefix(info.UpstreamModelName, "imagen") {
		return nil, errors.New("not supported model for image generation, only imagen models are supported")
	}

	// convert size to aspect ratio but allow user to specify aspect ratio
	aspectRatio := "1:1" // default aspect ratio
	size := strings.TrimSpace(request.Size)
	if size != "" {
		if strings.Contains(size, ":") {
			aspectRatio = size
		} else {
			switch size {
			case "256x256", "512x512", "1024x1024":
				aspectRatio = "1:1"
			case "1536x1024":
				aspectRatio = "3:2"
			case "1024x1536":
				aspectRatio = "2:3"
			case "1024x1792":
				aspectRatio = "9:16"
			case "1792x1024":
				aspectRatio = "16:9"
			}
		}
	}

	// build gemini imagen request
	geminiRequest := dto.GeminiImageRequest{
		Instances: []dto.GeminiImageInstance{
			{
				Prompt: request.Prompt,
			},
		},
		Parameters: dto.GeminiImageParameters{
			SampleCount:      int(lo.FromPtrOr(request.N, uint(1))),
			AspectRatio:      aspectRatio,
			PersonGeneration: "allow_adult", // default allow adult
		},
	}

	// Set imageSize when quality parameter is specified
	// Map quality parameter to imageSize (only supported by Standard and Ultra models)
	// quality values: auto, high, medium, low (for gpt-image-1), hd, standard (for dall-e-3)
	// imageSize values: 1K (default), 2K
	// https://ai.google.dev/gemini-api/docs/imagen
	// https://platform.openai.com/docs/api-reference/images/create
	if request.Quality != "" {
		imageSize := "1K" // default
		switch request.Quality {
		case "hd", "high":
			imageSize = "2K"
		case "2K":
			imageSize = "2K"
		case "standard", "medium", "low", "auto", "1K":
			imageSize = "1K"
		default:
			// unknown quality value, default to 1K
			imageSize = "1K"
		}
		geminiRequest.Parameters.ImageSize = imageSize
	}

	return geminiRequest, nil
}

// Init 初始化适配器。
// 当前 Gemini 适配器无需额外初始化操作。
// 参数:
//   - info: Relay 中继信息
func (a *Adaptor) Init(info *relaycommon.RelayInfo) {

}

// GetRequestURL 构建 Gemini API 的请求 URL。
// 根据模型类型和请求模式，生成不同的 API 端点地址：
//   - imagen 模型：使用 predict 端点
//   - embedding 模型：使用 embedContent 或 batchEmbedContents 端点
//   - 文本生成模型：使用 generateContent（非流式）或 streamGenerateContent（流式）端点
//
// 同时处理 Thinking 模型后缀的剥离（如 -thinking、-nothinking、-thinking-<budget>）。
// 参数:
//   - info: Relay 中继信息（包含模型名称、基础 URL、是否流式等）
//
// 返回:
//   - string: 完整的 API 请求 URL
//   - error: URL 构建过程中的错误
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {

	if model_setting.GetGeminiSettings().ThinkingAdapterEnabled &&
		!model_setting.ShouldPreserveThinkingSuffix(info.OriginModelName) {
		// 新增逻辑：处理 -thinking-<budget> 格式
		if strings.Contains(info.UpstreamModelName, "-thinking-") {
			parts := strings.Split(info.UpstreamModelName, "-thinking-")
			info.UpstreamModelName = parts[0]
		} else if strings.HasSuffix(info.UpstreamModelName, "-thinking") { // 旧的适配
			info.UpstreamModelName = strings.TrimSuffix(info.UpstreamModelName, "-thinking")
		} else if strings.HasSuffix(info.UpstreamModelName, "-nothinking") {
			info.UpstreamModelName = strings.TrimSuffix(info.UpstreamModelName, "-nothinking")
		} else if baseModel, level, ok := reasoning.TrimEffortSuffix(info.UpstreamModelName); ok && level != "" {
			info.UpstreamModelName = baseModel
		}
	}

	version := model_setting.GetGeminiVersionSetting(info.UpstreamModelName)

	if strings.HasPrefix(info.UpstreamModelName, "imagen") {
		return fmt.Sprintf("%s/%s/models/%s:predict", info.ChannelBaseUrl, version, info.UpstreamModelName), nil
	}

	if strings.HasPrefix(info.UpstreamModelName, "text-embedding") ||
		strings.HasPrefix(info.UpstreamModelName, "embedding") ||
		strings.HasPrefix(info.UpstreamModelName, "gemini-embedding") {
		action := "embedContent"
		if info.IsGeminiBatchEmbedding {
			action = "batchEmbedContents"
		}
		return fmt.Sprintf("%s/%s/models/%s:%s", info.ChannelBaseUrl, version, info.UpstreamModelName, action), nil
	}

	action := "generateContent"
	if info.IsStream {
		action = "streamGenerateContent?alt=sse"
		if info.RelayMode == constant.RelayModeGemini {
			info.DisablePing = true
		}
	}
	return fmt.Sprintf("%s/%s/models/%s:%s", info.ChannelBaseUrl, version, info.UpstreamModelName, action), nil
}

// SetupRequestHeader 设置 Gemini API 请求的 HTTP 头部。
// 设置通用 API 请求头，并通过 x-goog-api-key 头传递 Google API 密钥。
// 参数:
//   - c: Gin 上下文
//   - req: HTTP 请求头指针
//   - info: Relay 中继信息（包含 API Key 等）
//
// 返回:
//   - error: 设置过程中的错误
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("x-goog-api-key", info.ApiKey)
	return nil
}

// ConvertOpenAIRequest 将 OpenAI 格式的请求转换为 Gemini 格式。
// 调用 CovertOpenAI2Gemini 函数完成实际的格式转换，包括消息、工具、
// 安全设置、思考配置等方面的适配。
// 参数:
//   - c: Gin 上下文
//   - info: Relay 中继信息
//   - request: OpenAI 格式的请求对象
//
// 返回:
//   - any: Gemini 格式的请求对象
//   - error: 转换过程中的错误
func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}

	geminiRequest, err := CovertOpenAI2Gemini(c, *request, info)
	if err != nil {
		return nil, err
	}

	return geminiRequest, nil
}

// ConvertRerankRequest 将重排序请求转换为 Gemini 格式。
// 当前 Gemini 渠道不支持重排序功能，返回 nil。
// 参数:
//   - c: Gin 上下文
//   - relayMode: 中继模式
//   - request: 重排序请求对象
//
// 返回:
//   - any: 始终返回 nil
//   - error: 始终返回 nil
func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

// ConvertEmbeddingRequest 将嵌入请求转换为 Gemini 批量嵌入格式。
// 始终构建批量格式的请求（batchEmbedContents），支持以下模型的特殊参数：
//   - text-embedding-004, gemini-embedding-exp-03-07, gemini-embedding-001：支持 outputDimensionality 参数
//
// 参数:
//   - c: Gin 上下文
//   - info: Relay 中继信息（会设置 IsGeminiBatchEmbedding = true）
//   - request: OpenAI 格式的嵌入请求
//
// 返回:
//   - any: Gemini 批量嵌入请求对象（包含 requests 数组）
//   - error: 输入为空时返回错误
func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	if request.Input == nil {
		return nil, errors.New("input is required")
	}

	inputs := request.ParseInput()
	if len(inputs) == 0 {
		return nil, errors.New("input is empty")
	}
	// We always build a batch-style payload with `requests`, so ensure we call the
	// batch endpoint upstream to avoid payload/endpoint mismatches.
	info.IsGeminiBatchEmbedding = true
	// process all inputs
	geminiRequests := make([]map[string]interface{}, 0, len(inputs))
	for _, input := range inputs {
		geminiRequest := map[string]interface{}{
			"model": fmt.Sprintf("models/%s", info.UpstreamModelName),
			"content": dto.GeminiChatContent{
				Parts: []dto.GeminiPart{
					{
						Text: input,
					},
				},
			},
		}

		// set specific parameters for different models
		// https://ai.google.dev/api/embeddings?hl=zh-cn#method:-models.embedcontent
		switch info.UpstreamModelName {
		case "text-embedding-004", "gemini-embedding-exp-03-07", "gemini-embedding-001":
			// Only newer models introduced after 2024 support OutputDimensionality
			dimensions := lo.FromPtrOr(request.Dimensions, 0)
			if dimensions > 0 {
				geminiRequest["outputDimensionality"] = dimensions
			}
		}
		geminiRequests = append(geminiRequests, geminiRequest)
	}

	return map[string]interface{}{
		"requests": geminiRequests,
	}, nil
}

// ConvertOpenAIResponsesRequest 将 OpenAI Responses 格式的请求转换为 Gemini 格式。
// 当前尚未实现，调用时会返回 "not implemented" 错误。
// 参数:
//   - c: Gin 上下文
//   - info: Relay 中继信息
//   - request: OpenAI Responses 格式的请求对象
//
// 返回:
//   - any: 始终返回 nil
//   - error: 始终返回未实现错误
func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	// TODO implement me
	return nil, errors.New("not implemented")
}

// DoRequest 执行向 Gemini API 的 HTTP 请求。
// 通过 channel.DoApiRequest 通用方法发送请求，自动处理代理、超时等。
// 参数:
//   - c: Gin 上下文
//   - info: Relay 中继信息（包含 URL、Header 等）
//   - requestBody: 请求体 io.Reader
//
// 返回:
//   - any: HTTP 响应对象
//   - error: 请求过程中的错误
func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

// DoResponse 处理 Gemini API 的 HTTP 响应。
// 根据请求类型和模型类型分发到不同的处理器：
//   - Gemini 原生模式：嵌入 -> NativeGeminiEmbeddingHandler；流式 -> GeminiTextGenerationStreamHandler；非流式 -> GeminiTextGenerationHandler
//   - imagen 模型：-> GeminiImageHandler
//   - embedding 模型：-> GeminiEmbeddingHandler
//   - 普通文本生成模型：流式 -> GeminiChatStreamHandler；非流式 -> GeminiChatHandler
//
// 参数:
//   - c: Gin 上下文
//   - resp: Gemini API 返回的 HTTP 响应
//   - info: Relay 中继信息
//
// 返回:
//   - usage: token 使用量统计
//   - err: 错误信息
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NexusTokError) {
	if info.RelayMode == constant.RelayModeGemini {
		if strings.Contains(info.RequestURLPath, ":embedContent") ||
			strings.Contains(info.RequestURLPath, ":batchEmbedContents") {
			return NativeGeminiEmbeddingHandler(c, resp, info)
		}
		if info.IsStream {
			return GeminiTextGenerationStreamHandler(c, info, resp)
		} else {
			return GeminiTextGenerationHandler(c, info, resp)
		}
	}

	if strings.HasPrefix(info.UpstreamModelName, "imagen") {
		return GeminiImageHandler(c, info, resp)
	}

	// check if the model is an embedding model
	if strings.HasPrefix(info.UpstreamModelName, "text-embedding") ||
		strings.HasPrefix(info.UpstreamModelName, "embedding") ||
		strings.HasPrefix(info.UpstreamModelName, "gemini-embedding") {
		return GeminiEmbeddingHandler(c, info, resp)
	}

	if info.IsStream {
		return GeminiChatStreamHandler(c, info, resp)
	} else {
		return GeminiChatHandler(c, info, resp)
	}

}

// GetModelList 返回 Gemini 渠道支持的模型列表。
// 返回值来自 constant.go 中定义的 ModelList 变量。
// 返回:
//   - []string: 支持的模型名称列表
func (a *Adaptor) GetModelList() []string {
	return ModelList
}

// GetChannelName 返回渠道名称标识符。
// 返回值为 "google gemini"，用于标识此适配器对应的上游渠道。
// 返回:
//   - string: 渠道名称
func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
