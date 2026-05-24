// Package vertex 实现 Google Vertex AI 平台的渠道适配器。
// Vertex AI 是 Google Cloud 的机器学习平台，支持多种 AI 模型，包括：
// - Claude 模型（通过 Anthropic on Vertex AI）
// - Gemini 模型（Google 原生）
// - 开源模型（如 Meta Llama 系列，通过 Model-as-a-Service）
// 适配器根据上游模型名称自动选择对应的请求模式，并处理 API Key 和 Service Account
// 两种认证方式，以及不同模型的 URL 构建和请求格式转换。
package vertex

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/relay/channel"
	"github.com/c1cada/NexusTok/relay/channel/claude"
	"github.com/c1cada/NexusTok/relay/channel/gemini"
	"github.com/c1cada/NexusTok/relay/channel/openai"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/relay/constant"
	"github.com/c1cada/NexusTok/setting/model_setting"
	"github.com/c1cada/NexusTok/setting/reasoning"
	"github.com/c1cada/NexusTok/types"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

// 请求模式常量，标识 Vertex AI 中使用的模型类型。
const (
	RequestModeClaude     = 1 // Claude 模型请求模式（Anthropic on Vertex AI）
	RequestModeGemini     = 2 // Gemini 模型请求模式（Google 原生）
	RequestModeOpenSource = 3 // 开源模型请求模式（如 Meta Llama，通过 MaaS）
)

// claudeModelMap 是标准 Claude 模型名称到 Vertex AI 格式的映射表。
// Vertex AI 使用 "@date" 后缀的格式（如 "claude-3-sonnet@20240229"）
// 替代标准的 "-date" 后缀格式。
var claudeModelMap = map[string]string{
	"claude-3-sonnet-20240229":   "claude-3-sonnet@20240229",
	"claude-3-opus-20240229":     "claude-3-opus@20240229",
	"claude-3-haiku-20240307":    "claude-3-haiku@20240307",
	"claude-3-5-sonnet-20240620": "claude-3-5-sonnet@20240620",
	"claude-3-5-sonnet-20241022": "claude-3-5-sonnet-v2@20241022",
	"claude-3-7-sonnet-20250219": "claude-3-7-sonnet@20250219",
	"claude-sonnet-4-20250514":   "claude-sonnet-4@20250514",
	"claude-opus-4-20250514":     "claude-opus-4@20250514",
	"claude-opus-4-1-20250805":   "claude-opus-4-1@20250805",
	"claude-sonnet-4-5-20250929": "claude-sonnet-4-5@20250929",
	"claude-haiku-4-5-20251001":  "claude-haiku-4-5@20251001",
	"claude-opus-4-5-20251101":   "claude-opus-4-5@20251101",
	"claude-opus-4-6":            "claude-opus-4-6",
	"claude-opus-4-7":            "claude-opus-4-7",
}

// anthropicVersion 是 Vertex AI 上 Claude API 的版本标识。
// 用于设置请求头中的 anthropic-version 字段。
const anthropicVersion = "vertex-2023-10-16"

// Adaptor 是 Vertex AI 渠道的适配器结构体。
// 实现了 channel.Adaptor 接口，支持 Claude、Gemini 和开源模型三种请求模式。
// 根据上游模型名称在 Init 阶段自动确定请求模式。
type Adaptor struct {
	RequestMode        int          // 请求模式（RequestModeClaude/RequestModeGemini/RequestModeOpenSource）
	AccountCredentials Credentials  // Google Cloud 服务账号凭据（JSON 格式）
}

// ConvertGeminiRequest 将 Gemini 格式请求转换为 Vertex AI 兼容格式。
// 如果启用了移除 functionResponse.id 的设置，会清除请求中的 FunctionResponse ID 字段
// （Vertex AI 不支持该字段）。
// 参数:
//   - c: Gin 上下文
//   - info: 中继信息
//   - request: Gemini 格式的聊天请求
// 返回:
//   - any: 转换后的请求体
//   - error: 转换失败时返回错误
func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	// Vertex AI does not support functionResponse.id; keep it stripped here for consistency.
	if model_setting.GetGeminiSettings().RemoveFunctionResponseIdEnabled {
		removeFunctionResponseID(request)
	}
	geminiAdaptor := gemini.Adaptor{}
	return geminiAdaptor.ConvertGeminiRequest(c, info, request)
}

// removeFunctionResponseID 递归移除 Gemini 请求中 FunctionResponse 的 ID 字段。
// Vertex AI 的 Gemini API 不支持 functionResponse.id 字段，需要在发送前清除。
// 支持递归处理嵌套的 Requests 字段。
// 参数:
//   - request: Gemini 格式的聊天请求（会被原地修改）
func removeFunctionResponseID(request *dto.GeminiChatRequest) {
	if request == nil {
		return
	}

	if len(request.Contents) > 0 {
		for i := range request.Contents {
			if len(request.Contents[i].Parts) == 0 {
				continue
			}
			for j := range request.Contents[i].Parts {
				part := &request.Contents[i].Parts[j]
				if part.FunctionResponse == nil {
					continue
				}
				if len(part.FunctionResponse.ID) > 0 {
					part.FunctionResponse.ID = nil
				}
			}
		}
	}

	if len(request.Requests) > 0 {
		for i := range request.Requests {
			removeFunctionResponseID(&request.Requests[i])
		}
	}
}

// ConvertClaudeRequest 将 Claude 格式请求转换为 Vertex AI 格式。
// 先将模型名称映射为 Vertex AI 格式（如 "claude-3-sonnet@20240229"），
// 然后复制请求并设置 anthropic-version 头信息。
// 参数:
//   - c: Gin 上下文
//   - info: 中继信息
//   - request: Claude 格式的请求体
// 返回:
//   - any: 转换后的 Vertex AI Claude 请求体
//   - error: 始终返回 nil
func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	if v, ok := claudeModelMap[info.UpstreamModelName]; ok {
		c.Set("request_model", v)
	} else {
		c.Set("request_model", request.Model)
	}
	vertexClaudeReq := copyRequest(request, anthropicVersion)
	return vertexClaudeReq, nil
}

// ConvertAudioRequest 未实现，Vertex AI 渠道暂不支持音频请求。
// 参数:
//   - c: Gin 上下文
//   - info: 中继信息
//   - request: 音频请求体
// 返回:
//   - io.Reader: 始终返回 nil
//   - error: 始终返回 "not implemented" 错误
func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertImageRequest 将图片生成请求委托给 Gemini 适配器处理。
// Vertex AI 的图片生成通过 Gemini API 实现。
// 参数:
//   - c: Gin 上下文
//   - info: 中继信息
//   - request: 统一的图片生成请求
// 返回:
//   - any: 转换后的请求体
//   - error: 转换失败时返回错误
func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	geminiAdaptor := gemini.Adaptor{}
	return geminiAdaptor.ConvertImageRequest(c, info, request)
}

// Init 初始化 Vertex AI 适配器。
// 根据上游模型名称前缀确定请求模式：
//   - 以 "claude" 开头 -> RequestModeClaude
//   - 包含 "llama" 或 "-maas" -> RequestModeOpenSource
//   - 其他 -> RequestModeGemini
// 参数:
//   - info: 中继请求的上下文信息
func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
	if strings.HasPrefix(info.UpstreamModelName, "claude") {
		a.RequestMode = RequestModeClaude
	} else if strings.Contains(info.UpstreamModelName, "llama") ||
		// open source models
		strings.Contains(info.UpstreamModelName, "-maas") {
		a.RequestMode = RequestModeOpenSource
	} else {
		a.RequestMode = RequestModeGemini
	}
}

// getRequestUrl 根据请求模式和认证方式构建 Vertex AI API 的完整请求 URL。
// 支持两种认证方式：
//   - Service Account（JSON 凭据）：URL 中不包含 key 参数
//   - API Key：URL 中追加 key=xxx 查询参数
// 不同请求模式使用不同的 URL 构建函数：
//   - Gemini -> BuildGoogleModelURL
//   - Claude -> BuildAnthropicModelURL
//   - OpenSource -> BuildOpenSourceChatCompletionsURL
// 参数:
//   - info: 中继信息（包含 API Key、API Version 等）
//   - modelName: 模型名称
//   - suffix: URL 后缀路径（如 "generateContent"、"streamRawPredict?alt=sse" 等）
// 返回:
//   - string: 完整的请求 URL
//   - error: 凭据解析失败或不支持的模式时返回错误
func (a *Adaptor) getRequestUrl(info *relaycommon.RelayInfo, modelName, suffix string) (string, error) {
	region := GetModelRegion(info.ApiVersion, info.OriginModelName)
	if info.ChannelOtherSettings.VertexKeyType != dto.VertexKeyTypeAPIKey {
		adc := &Credentials{}
		if err := common.Unmarshal([]byte(info.ApiKey), adc); err != nil {
			return "", fmt.Errorf("failed to decode credentials file: %w", err)
		}
		a.AccountCredentials = *adc

		if a.RequestMode == RequestModeGemini {
			return BuildGoogleModelURL(info.ChannelBaseUrl, DefaultAPIVersion, adc.ProjectID, region, modelName, suffix), nil
		} else if a.RequestMode == RequestModeClaude {
			return BuildAnthropicModelURL(info.ChannelBaseUrl, DefaultAPIVersion, adc.ProjectID, region, modelName, suffix), nil
		} else if a.RequestMode == RequestModeOpenSource {
			return BuildOpenSourceChatCompletionsURL(info.ChannelBaseUrl, adc.ProjectID, region), nil
		}
	} else {
		var keyPrefix string
		if strings.HasSuffix(suffix, "?alt=sse") {
			keyPrefix = "&"
		} else {
			keyPrefix = "?"
		}
		if a.RequestMode == RequestModeGemini {
			return fmt.Sprintf(
				"%s%skey=%s",
				BuildGoogleModelURL(info.ChannelBaseUrl, DefaultAPIVersion, "", region, modelName, suffix),
				keyPrefix,
				info.ApiKey,
			), nil
		} else if a.RequestMode == RequestModeClaude {
			return fmt.Sprintf(
				"%s%skey=%s",
				BuildAnthropicModelURL(info.ChannelBaseUrl, DefaultAPIVersion, "", region, modelName, suffix),
				keyPrefix,
				info.ApiKey,
			), nil
		}
	}
	return "", errors.New("unsupported request mode")
}

// GetRequestURL 构建 Vertex AI API 的完整请求 URL。
// 根据请求模式选择不同的 URL 后缀：
//   - Gemini: streamGenerateContent?alt=sse（流式）或 generateContent（非流式）
//   - Claude: streamRawPredict?alt=sse（流式）或 rawPredict（非流式）
//   - OpenSource: 使用通用的 chat completions 端点
//
// 特殊处理：
//   - Thinking 后缀模型（如 "-thinking"、"-nothinking"、"-thinking-<budget>"）会被清理
//   - Imagen 模型使用 "predict" 后缀
// 参数:
//   - info: 中继信息
// 返回:
//   - string: 完整的请求 URL
//   - error: 不支持的请求模式时返回错误
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	suffix := ""
	if a.RequestMode == RequestModeGemini {
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

		if info.IsStream {
			suffix = "streamGenerateContent?alt=sse"
		} else {
			suffix = "generateContent"
		}

		if strings.HasPrefix(info.UpstreamModelName, "imagen") {
			suffix = "predict"
		}
		return a.getRequestUrl(info, info.UpstreamModelName, suffix)
	} else if a.RequestMode == RequestModeClaude {
		if info.IsStream {
			suffix = "streamRawPredict?alt=sse"
		} else {
			suffix = "rawPredict"
		}
		model := info.UpstreamModelName
		if v, ok := claudeModelMap[info.UpstreamModelName]; ok {
			model = v
		}
		return a.getRequestUrl(info, model, suffix)
	} else if a.RequestMode == RequestModeOpenSource {
		return a.getRequestUrl(info, "", "")
	}
	return "", errors.New("unsupported request mode")
}

// SetupRequestHeader 设置 Vertex AI API 的请求头。
// 包括通用 API 请求头和认证信息：
//   - Service Account 模式：使用 OAuth2 Access Token 进行 Bearer 认证
//   - API Key 模式：不设置 Authorization 头（Key 在 URL 中传递）
// 如果使用 Service Account，还会设置 x-goog-user-project 头标识项目。
// 对于 Claude 模型，额外设置 Claude 特定的请求头。
// 参数:
//   - c: Gin 上下文
//   - req: HTTP 请求头指针
//   - info: 中继信息
// 返回:
//   - error: Access Token 获取失败时返回错误
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	if info.ChannelOtherSettings.VertexKeyType != dto.VertexKeyTypeAPIKey {
		accessToken, err := getAccessToken(a, info)
		if err != nil {
			return err
		}
		req.Set("Authorization", "Bearer "+accessToken)
	}
	if a.AccountCredentials.ProjectID != "" {
		req.Set("x-goog-user-project", a.AccountCredentials.ProjectID)
	}
	if strings.Contains(info.UpstreamModelName, "claude") {
		claude.CommonClaudeHeadersOperation(c, req, info)
	}
	return nil
}

// ConvertOpenAIRequest 将 OpenAI 格式的请求转换为 Vertex AI 对应模型格式。
// 根据请求模式分发到不同的转换器：
//   - Gemini + Imagen: 从消息中提取 prompt，构建 ImageRequest，委托给 ConvertImageRequest
//   - Claude: 转换为 Claude 格式，设置 anthropic-version
//   - Gemini: 转换为 Gemini 格式
//   - OpenSource: 直接透传 OpenAI 格式请求
// 参数:
//   - c: Gin 上下文
//   - info: 中继信息
//   - request: OpenAI 格式的通用请求
// 返回:
//   - any: 转换后的请求体
//   - error: 请求为 nil 或转换失败时返回错误
func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	if a.RequestMode == RequestModeGemini && strings.HasPrefix(info.UpstreamModelName, "imagen") {
		prompt := ""
		for _, m := range request.Messages {
			if m.Role == "user" {
				prompt = m.StringContent()
				if prompt != "" {
					break
				}
			}
		}
		if prompt == "" {
			if p, ok := request.Prompt.(string); ok {
				prompt = p
			}
		}
		if prompt == "" {
			return nil, errors.New("prompt is required for image generation")
		}

		imgReq := dto.ImageRequest{
			Model:  request.Model,
			Prompt: prompt,
			N:      lo.ToPtr(uint(1)),
			Size:   "1024x1024",
		}
		if request.N != nil && *request.N > 0 {
			imgReq.N = lo.ToPtr(uint(*request.N))
		}
		if request.Size != "" {
			imgReq.Size = request.Size
		}
		if len(request.ExtraBody) > 0 {
			var extra map[string]any
			if err := json.Unmarshal(request.ExtraBody, &extra); err == nil {
				if n, ok := extra["n"].(float64); ok && n > 0 {
					imgReq.N = lo.ToPtr(uint(n))
				}
				if size, ok := extra["size"].(string); ok {
					imgReq.Size = size
				}
				// accept aspectRatio in extra body (top-level or under parameters)
				if ar, ok := extra["aspectRatio"].(string); ok && ar != "" {
					imgReq.Size = ar
				}
				if params, ok := extra["parameters"].(map[string]any); ok {
					if ar, ok := params["aspectRatio"].(string); ok && ar != "" {
						imgReq.Size = ar
					}
				}
			}
		}
		c.Set("request_model", request.Model)
		return a.ConvertImageRequest(c, info, imgReq)
	}
	if a.RequestMode == RequestModeClaude {
		claudeReq, err := claude.RequestOpenAI2ClaudeMessage(c, *request)
		if err != nil {
			return nil, err
		}
		vertexClaudeReq := copyRequest(claudeReq, anthropicVersion)
		c.Set("request_model", claudeReq.Model)
		info.UpstreamModelName = claudeReq.Model
		return vertexClaudeReq, nil
	} else if a.RequestMode == RequestModeGemini {
		geminiRequest, err := gemini.CovertOpenAI2Gemini(c, *request, info)
		if err != nil {
			return nil, err
		}
		c.Set("request_model", request.Model)
		return geminiRequest, nil
	} else if a.RequestMode == RequestModeOpenSource {
		return request, nil
	}
	return nil, errors.New("unsupported request mode")
}

// ConvertRerankRequest Vertex AI 不支持 Rerank 请求，返回 nil。
func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

// ConvertEmbeddingRequest 未实现，Vertex AI 渠道暂不支持 Embedding 请求。
func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertOpenAIResponsesRequest 未实现，Vertex AI 渠道暂不支持 OpenAI Responses 格式请求。
func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	// TODO implement me
	return nil, errors.New("not implemented")
}

// DoRequest 执行实际的 HTTP API 请求，委托给通用的 DoApiRequest 方法。
// 参数:
//   - c: Gin 上下文
//   - info: 中继信息
//   - requestBody: 请求体读取器
// 返回:
//   - any: 响应结果
//   - error: 请求失败时返回错误
func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

// DoResponse 处理 Vertex AI API 的响应。
// 根据请求模式和是否为流式请求，分发到不同的处理器：
//   - Claude: 委托给 Claude 适配器处理
//   - Gemini 流式: GeminiChatStreamHandler 或 GeminiTextGenerationStreamHandler
//   - Gemini 非流式: GeminiChatHandler、GeminiImageHandler 或 GeminiTextGenerationHandler
//   - OpenSource: 委托给 OpenAI 适配器处理
// 参数:
//   - c: Gin 上下文
//   - resp: 上游 HTTP 响应
//   - info: 中继信息
// 返回:
//   - usage: token 使用量
//   - err: 处理过程中的错误信息
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NexusTokError) {
	claudeAdaptor := claude.Adaptor{}
	if info.IsStream {
		switch a.RequestMode {
		case RequestModeClaude:
			return claudeAdaptor.DoResponse(c, resp, info)
		case RequestModeGemini:
			if info.RelayMode == constant.RelayModeGemini {
				return gemini.GeminiTextGenerationStreamHandler(c, info, resp)
			} else {
				return gemini.GeminiChatStreamHandler(c, info, resp)
			}
		case RequestModeOpenSource:
			return openai.OaiStreamHandler(c, info, resp)
		}
	} else {
		switch a.RequestMode {
		case RequestModeClaude:
			return claudeAdaptor.DoResponse(c, resp, info)
		case RequestModeGemini:
			if info.RelayMode == constant.RelayModeGemini {
				return gemini.GeminiTextGenerationHandler(c, info, resp)
			} else {
				if strings.HasPrefix(info.UpstreamModelName, "imagen") {
					return gemini.GeminiImageHandler(c, info, resp)
				}
				return gemini.GeminiChatHandler(c, info, resp)
			}
		case RequestModeOpenSource:
			return openai.OpenaiHandler(c, info, resp)
		}
	}
	return
}

// GetModelList 返回 Vertex AI 渠道支持的所有模型列表。
// 合并了 Vertex 专属模型、Claude 模型和 Gemini 模型三个列表。
// 返回:
//   - []string: 模型名称切片
func (a *Adaptor) GetModelList() []string {
	var modelList []string
	for i, s := range ModelList {
		modelList = append(modelList, s)
		ModelList[i] = s
	}
	for i, s := range claude.ModelList {
		modelList = append(modelList, s)
		claude.ModelList[i] = s
	}
	for i, s := range gemini.ModelList {
		modelList = append(modelList, s)
		gemini.ModelList[i] = s
	}
	return modelList
}

// GetChannelName 返回渠道名称标识 "vertex-ai"。
// 返回:
//   - string: 渠道名称
func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
