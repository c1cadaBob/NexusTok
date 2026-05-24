// Package ali - adaptor.go
// 该文件实现了阿里云通义千问（DashScope）的 API 适配器
//
// 支持的功能：
// - 文本生成（Chat Completions）
// - 图像生成（Images Generations）
// - 图像编辑（Images Edits）
// - 文本嵌入（Embeddings）
// - 文本重排（Rerank）
// - Claude 格式兼容（通过 Anthropic Messages API）
// - Responses API 兼容
//
// 特殊处理：
// - 支持同步和异步图像生成模型
// - 支持通过环境变量配置 Anthropic Messages 兼容模型
// - 支持流式响应（SSE）
package ali

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/relay/channel"
	"github.com/c1cada/NexusTok/relay/channel/claude"
	"github.com/c1cada/NexusTok/relay/channel/openai"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/relay/constant"
	"github.com/c1cada/NexusTok/service"
	"github.com/c1cada/NexusTok/setting/model_setting"
	"github.com/c1cada/NexusTok/types"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

// Adaptor 阿里云通义千问 API 适配器
type Adaptor struct {
	IsSyncImageModel bool // 是否为同步图像模型
}

// 环境变量和默认值：配置支持 Anthropic Messages 格式的模型列表
const aliAnthropicMessagesModelsEnv = "ALI_ANTHROPIC_MESSAGES_MODELS"
const defaultAliAnthropicMessagesModels = "qwen,deepseek-v4,kimi,glm,minimax-m"

/*
	var syncModels = []string{
		"z-image",
		"qwen-image",
		"wan2.6",
	}
*/
// supportsAliAnthropicMessages 检查模型是否支持 Anthropic Messages 格式
//
// 通过环境变量 ALI_ANTHROPIC_MESSAGES_MODELS 配置支持的模型列表
// 模型名称匹配规则：忽略大小写，包含配置的模式字符串即为匹配
//
// 参数：
//   - modelName: 模型名称
//
// 返回值：
//   - bool: 是否支持 Anthropic Messages 格式
func supportsAliAnthropicMessages(modelName string) bool {
	normalizedModelName := strings.ToLower(strings.TrimSpace(modelName))
	if normalizedModelName == "" {
		return false
	}

	return lo.SomeBy(aliAnthropicMessagesModelPatterns(), func(pattern string) bool {
		return strings.Contains(normalizedModelName, pattern)
	})
}

// aliAnthropicMessagesModelPatterns 获取支持 Anthropic Messages 的模型模式列表
//
// 从环境变量读取配置，如果未配置则使用默认值
//
// 返回值：
//   - []string: 模型模式列表（小写）
func aliAnthropicMessagesModelPatterns() []string {
	configuredModels := common.GetEnvOrDefaultString(aliAnthropicMessagesModelsEnv, defaultAliAnthropicMessagesModels)
	return lo.FilterMap(strings.Split(configuredModels, ","), func(item string, _ int) (string, bool) {
		pattern := strings.ToLower(strings.TrimSpace(item))
		return pattern, pattern != ""
	})
}

// syncModels 同步图像生成模型列表
var syncModels = []string{
	"z-image",
	"qwen-image",
	"wan2.6",
}

// isSyncImageModel 检查模型是否为同步图像模型
//
// 参数：
//   - modelName: 模型名称
//
// 返回值：
//   - bool: 是否为同步图像模型
func isSyncImageModel(modelName string) bool {
	return model_setting.IsSyncImageModel(modelName)
}

// ConvertGeminiRequest 转换 Gemini 格式请求（未实现）
func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertClaudeRequest 转换 Claude 格式请求
//
// 如果模型支持 Anthropic Messages 格式，直接返回原始请求
// 否则将 Claude 格式转换为 OpenAI 格式再处理
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继信息
//   - req: Claude 格式请求
//
// 返回值：
//   - any: 转换后的请求
//   - error: 转换错误
func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, req *dto.ClaudeRequest) (any, error) {
	if supportsAliAnthropicMessages(info.UpstreamModelName) {
		return req, nil
	}

	oaiReq, err := service.ClaudeToOpenAIRequest(*req, info)
	if err != nil {
		return nil, err
	}
	if info.SupportStreamOptions && info.IsStream {
		oaiReq.StreamOptions = &dto.StreamOptions{IncludeUsage: true}
	}
	return a.ConvertOpenAIRequest(c, info, oaiReq)
}

// Init 初始化适配器（当前为空实现）
func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

// GetRequestURL 获取请求的完整 URL
//
// 根据中继格式和模式选择不同的 API 端点：
// - Claude 格式：使用 Anthropic Messages API 或兼容模式
// - OpenAI 格式：根据 RelayMode 选择不同的端点
//   - Embeddings: /compatible-mode/v1/embeddings
//   - Rerank: /api/v1/services/rerank/text-rerank/text-rerank
//   - Responses: /api/v2/apps/protocols/compatible-mode/v1/responses
//   - ImagesGenerations: 图像生成（同步/异步）
//   - ImagesEdits: 图像编辑
//   - Completions: /compatible-mode/v1/completions
//   - 默认: /compatible-mode/v1/chat/completions
//
// 参数：
//   - info: 中继信息
//
// 返回值：
//   - string: 完整的请求 URL
//   - error: 错误
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	var fullRequestURL string
	switch info.RelayFormat {
	case types.RelayFormatClaude:
		if supportsAliAnthropicMessages(info.UpstreamModelName) {
			fullRequestURL = fmt.Sprintf("%s/apps/anthropic/v1/messages", info.ChannelBaseUrl)
		} else {
			fullRequestURL = fmt.Sprintf("%s/compatible-mode/v1/chat/completions", info.ChannelBaseUrl)
		}
	default:
		switch info.RelayMode {
		case constant.RelayModeEmbeddings:
			fullRequestURL = fmt.Sprintf("%s/compatible-mode/v1/embeddings", info.ChannelBaseUrl)
		case constant.RelayModeRerank:
			fullRequestURL = fmt.Sprintf("%s/api/v1/services/rerank/text-rerank/text-rerank", info.ChannelBaseUrl)
		case constant.RelayModeResponses:
			fullRequestURL = fmt.Sprintf("%s/api/v2/apps/protocols/compatible-mode/v1/responses", info.ChannelBaseUrl)
		case constant.RelayModeImagesGenerations:
			if isSyncImageModel(info.OriginModelName) {
				fullRequestURL = fmt.Sprintf("%s/api/v1/services/aigc/multimodal-generation/generation", info.ChannelBaseUrl)
			} else {
				fullRequestURL = fmt.Sprintf("%s/api/v1/services/aigc/text2image/image-synthesis", info.ChannelBaseUrl)
			}
		case constant.RelayModeImagesEdits:
			if isOldWanModel(info.OriginModelName) {
				fullRequestURL = fmt.Sprintf("%s/api/v1/services/aigc/image2image/image-synthesis", info.ChannelBaseUrl)
			} else if isWanModel(info.OriginModelName) {
				fullRequestURL = fmt.Sprintf("%s/api/v1/services/aigc/image-generation/generation", info.ChannelBaseUrl)
			} else {
				fullRequestURL = fmt.Sprintf("%s/api/v1/services/aigc/multimodal-generation/generation", info.ChannelBaseUrl)
			}
		case constant.RelayModeCompletions:
			fullRequestURL = fmt.Sprintf("%s/compatible-mode/v1/completions", info.ChannelBaseUrl)
		default:
			fullRequestURL = fmt.Sprintf("%s/compatible-mode/v1/chat/completions", info.ChannelBaseUrl)
		}
	}

	return fullRequestURL, nil
}

// SetupRequestHeader 设置请求头
//
// 设置的请求头包括：
// - Authorization: Bearer token
// - X-DashScope-SSE: 流式响应开关
// - X-DashScope-Plugin: 插件标识
// - X-DashScope-Async: 异步图像生成开关
// - Content-Type: 图像编辑时设置为 application/json
//
// 参数：
//   - c: Gin 上下文
//   - req: 请求头
//   - info: 中继信息
//
// 返回值：
//   - error: 错误
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", "Bearer "+info.ApiKey)
	if info.IsStream {
		req.Set("X-DashScope-SSE", "enable")
	}
	if c.GetString("plugin") != "" {
		req.Set("X-DashScope-Plugin", c.GetString("plugin"))
	}
	if info.RelayMode == constant.RelayModeImagesGenerations {
		if isSyncImageModel(info.OriginModelName) {

		} else {
			req.Set("X-DashScope-Async", "enable")
		}
	}
	if info.RelayMode == constant.RelayModeImagesEdits {
		if isWanModel(info.OriginModelName) {
			req.Set("X-DashScope-Async", "enable")
		}
		req.Set("Content-Type", "application/json")
	}
	return nil
}

// ConvertOpenAIRequest 转换 OpenAI 格式请求为阿里云格式
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继信息
//   - request: OpenAI 格式请求
//
// 返回值：
//   - any: 阿里云格式请求
//   - error: 转换错误
func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	// docs: https://bailian.console.aliyun.com/?tab=api#/api/?type=model&url=2712216
	// fix: InternalError.Algo.InvalidParameter: The value of the enable_thinking parameter is restricted to True.
	//if strings.Contains(request.Model, "thinking") {
	//	request.EnableThinking = true
	//	request.Stream = true
	//	info.IsStream = true
	//}
	//// fix: ali parameter.enable_thinking must be set to false for non-streaming calls
	//if !info.IsStream {
	//	request.EnableThinking = false
	//}

	switch info.RelayMode {
	default:
		aliReq := requestOpenAI2Ali(*request)
		return aliReq, nil
	}
}

// ConvertImageRequest 转换图像请求
//
// 支持两种模式：
// - ImagesGenerations: 图像生成（同步/异步）
// - ImagesEdits: 图像编辑（支持表单和 JSON 两种格式）
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继信息
//   - request: OpenAI 格式图像请求
//
// 返回值：
//   - any: 阿里云格式图像请求
//   - error: 转换错误
func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	if info.RelayMode == constant.RelayModeImagesGenerations {
		if isSyncImageModel(info.OriginModelName) {
			a.IsSyncImageModel = true
		}
		aliRequest, err := oaiImage2AliImageRequest(info, request, a.IsSyncImageModel)
		if err != nil {
			return nil, fmt.Errorf("convert image request to async ali image request failed: %w", err)
		}
		return aliRequest, nil
	} else if info.RelayMode == constant.RelayModeImagesEdits {
		if isOldWanModel(info.OriginModelName) {
			return oaiFormEdit2WanxImageEdit(c, info, request)
		}
		if isSyncImageModel(info.OriginModelName) {
			if isWanModel(info.OriginModelName) {
				a.IsSyncImageModel = false
			} else {
				a.IsSyncImageModel = true
			}
		}
		// ali image edit https://bailian.console.aliyun.com/?tab=api#/api/?type=model&url=2976416
		// 如果用户使用表单，则需要解析表单数据
		if strings.Contains(c.Request.Header.Get("Content-Type"), "multipart/form-data") {
			aliRequest, err := oaiFormEdit2AliImageEdit(c, info, request)
			if err != nil {
				return nil, fmt.Errorf("convert image edit form request failed: %w", err)
			}
			return aliRequest, nil
		} else {
			aliRequest, err := oaiImage2AliImageRequest(info, request, a.IsSyncImageModel)
			if err != nil {
				return nil, fmt.Errorf("convert image request to async ali image request failed: %w", err)
			}
			return aliRequest, nil
		}
	}
	return nil, fmt.Errorf("unsupported image relay mode: %d", info.RelayMode)
}

// ConvertRerankRequest 转换文本重排请求
//
// 参数：
//   - c: Gin 上下文
//   - relayMode: 中继模式
//   - request: OpenAI 格式重排请求
//
// 返回值：
//   - any: 阿里云格式重排请求
//   - error: 转换错误
func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return ConvertRerankRequest(request), nil
}

// ConvertEmbeddingRequest 转换文本嵌入请求
//
// 阿里云的嵌入 API 与 OpenAI 格式兼容，直接返回原始请求
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继信息
//   - request: OpenAI 格式嵌入请求
//
// 返回值：
//   - any: 嵌入请求
//   - error: 转换错误
func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return request, nil
}

// ConvertAudioRequest 转换音频请求（未实现）
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继信息
//   - request: 音频请求
//
// 返回值：
//   - io.Reader: 请求体
//   - error: 错误
func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertOpenAIResponsesRequest 转换 OpenAI Responses 格式请求
//
// 阿里云的 Responses API 与 OpenAI 格式兼容，直接返回原始请求
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继信息
//   - request: OpenAI Responses 格式请求
//
// 返回值：
//   - any: 请求
//   - error: 转换错误
func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return request, nil
}

// DoRequest 发送请求到上游
//
// 使用通用的 API 请求发送函数
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继信息
//   - requestBody: 请求体
//
// 返回值：
//   - any: 响应
//   - error: 错误
func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

// DoResponse 处理上游响应
//
// 根据中继格式和模式选择不同的响应处理器：
// - Claude 格式：如果支持 Anthropic Messages 则使用 Claude 适配器，否则使用 OpenAI 适配器
// - 图像生成/编辑：使用阿里云图像处理器
// - 重排：使用重排处理器
// - 默认：使用 OpenAI 适配器
//
// 参数：
//   - c: Gin 上下文
//   - resp: HTTP 响应
//   - info: 中继信息
//
// 返回值：
//   - usage: 用量信息
//   - err: 错误信息
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NexusTokError) {
	switch info.RelayFormat {
	case types.RelayFormatClaude:
		if supportsAliAnthropicMessages(info.UpstreamModelName) {
			adaptor := claude.Adaptor{}
			return adaptor.DoResponse(c, resp, info)
		}

		adaptor := openai.Adaptor{}
		return adaptor.DoResponse(c, resp, info)
	default:
		switch info.RelayMode {
		case constant.RelayModeImagesGenerations:
			err, usage = aliImageHandler(a, c, resp, info)
		case constant.RelayModeImagesEdits:
			err, usage = aliImageHandler(a, c, resp, info)
		case constant.RelayModeRerank:
			err, usage = RerankHandler(c, resp, info)
		default:
			adaptor := openai.Adaptor{}
			usage, err = adaptor.DoResponse(c, resp, info)
		}
		return usage, err
	}
}

// GetModelList 获取支持的模型列表
//
// 返回值：
//   - []string: 模型列表
func (a *Adaptor) GetModelList() []string {
	return ModelList
}

// GetChannelName 获取渠道名称
//
// 返回值：
//   - string: 渠道名称
func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
