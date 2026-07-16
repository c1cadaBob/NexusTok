// openai - adaptor.go
// OpenAI 渠道的适配器实现文件。
// Adaptor 结构体实现了 relay/channel 接口，负责：
// - 将各种格式的请求（OpenAI、Claude、Gemini）转换为 OpenAI API 兼容格式
// - 构建请求 URL（支持 Azure、自定义渠道等不同场景）
// - 设置请求头（认证、组织、WebSocket 协议等）
// - 处理音频（TTS/STT）、图片生成/编辑、嵌入、重排序等多模态请求
// - 根据不同的 RelayMode 分发响应处理
// - 管理模型列表和渠道名称
package openai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"path/filepath"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/logger"
	"github.com/c1cada/NexusTok/relay/channel"
	"github.com/c1cada/NexusTok/relay/channel/ai360"
	"github.com/c1cada/NexusTok/relay/channel/lingyiwanwu"

	//"github.com/c1cada/NexusTok/relay/channel/minimax"
	"github.com/c1cada/NexusTok/relay/channel/openrouter"
	"github.com/c1cada/NexusTok/relay/channel/xinference"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/relay/common_handler"
	relayconstant "github.com/c1cada/NexusTok/relay/constant"
	"github.com/c1cada/NexusTok/service"
	"github.com/c1cada/NexusTok/setting/model_setting"
	"github.com/c1cada/NexusTok/setting/reasoning"
	"github.com/c1cada/NexusTok/types"
	"github.com/samber/lo"

	"github.com/gin-gonic/gin"
)

// Adaptor 是 OpenAI 渠道的适配器结构体。
// 实现了中继（relay）系统的适配器接口，负责请求转换和响应处理。
// 支持多种子渠道类型（OpenAI、Azure、OpenRouter、自定义等）。
type Adaptor struct {
	ChannelType    int    // 渠道类型常量（如 constant.ChannelTypeOpenAI、constant.ChannelTypeAzure）
	ResponseFormat string // 音频响应格式（在 STT 场景中用于指定输出格式）
}

// ConvertGeminiRequest 将 Gemini 格式的聊天请求转换为 OpenAI 格式。
// 先通过 service.GeminiToOpenAIRequest 将 Gemini 请求转换为通用 OpenAI 请求，
// 再调用 ConvertOpenAIRequest 进行进一步的格式适配和参数调整。
//
// 参数:
//   - c: Gin 上下文
//   - info: 中继信息，包含渠道配置和请求上下文
//   - request: Gemini 格式的聊天请求
//
// 返回:
//   - any: 转换后的请求对象（可直接序列化发送到上游）
//   - error: 转换失败时的错误
func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	// 使用 service.GeminiToOpenAIRequest 转换请求格式
	openaiRequest, err := service.GeminiToOpenAIRequest(request, info)
	if err != nil {
		return nil, err
	}
	return a.ConvertOpenAIRequest(c, info, openaiRequest)
}

// ConvertClaudeRequest 将 Claude 格式的请求转换为 OpenAI 格式。
// 先通过 service.ClaudeToOpenAIRequest 将 Claude 请求转换为通用 OpenAI 请求，
// 如果上游支持 StreamOptions 且为流式请求，则自动启用 usage 统计。
// 最后调用 ConvertOpenAIRequest 进行进一步的格式适配。
//
// 参数:
//   - c: Gin 上下文
//   - info: 中继信息，包含渠道配置
//   - request: Claude 格式的请求对象
//
// 返回:
//   - any: 转换后的请求对象
//   - error: 转换失败时的错误
func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	//if !strings.Contains(request.Model, "claude") {
	//	return nil, fmt.Errorf("you are using openai channel type with path /v1/messages, only claude model supported convert, but got %s", request.Model)
	//}
	//if common.DebugEnabled {
	//	bodyBytes := []byte(common.GetJsonString(request))
	//	err := os.WriteFile(fmt.Sprintf("claude_request_%s.txt", c.GetString(common.RequestIdKey)), bodyBytes, 0644)
	//	if err != nil {
	//		println(fmt.Sprintf("failed to save request body to file: %v", err))
	//	}
	//}
	aiRequest, err := service.ClaudeToOpenAIRequest(*request, info)
	if err != nil {
		return nil, err
	}
	//if common.DebugEnabled {
	//	println(fmt.Sprintf("convert claude to openai request result: %s", common.GetJsonString(aiRequest)))
	//	// Save request body to file for debugging
	//	bodyBytes := []byte(common.GetJsonString(aiRequest))
	//	err = os.WriteFile(fmt.Sprintf("claude_to_openai_request_%s.txt", c.GetString(common.RequestIdKey)), bodyBytes, 0644)
	//	if err != nil {
	//		println(fmt.Sprintf("failed to save request body to file: %v", err))
	//	}
	//}
	if info.SupportStreamOptions && info.IsStream {
		aiRequest.StreamOptions = &dto.StreamOptions{
			IncludeUsage: true,
		}
	}
	return a.ConvertOpenAIRequest(c, info, aiRequest)
}

// Init 初始化适配器，在请求处理开始前调用。
// 设置渠道类型，并在启用了 thinking_to_content 功能时初始化 ThinkingContentInfo。
// ThinkingContentInfo 用于追踪思考内容的转换状态（首次发送、已发送标记等）。
//
// 参数:
//   - info: 中继信息，包含渠道配置和请求上下文
func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType

	// initialize ThinkingContentInfo when thinking_to_content is enabled
	if info.ChannelSetting.ThinkingToContent {
		info.ThinkingContentInfo = relaycommon.ThinkingContentInfo{
			IsFirstThinkingContent:  true,
			SendLastThinkingContent: false,
			HasSentThinkingContent:  false,
		}
	}
}

// GetRequestURL 根据渠道类型和请求模式构建完整的上游请求 URL。
// 不同渠道类型的 URL 构建逻辑不同：
//   - Azure: 使用部署路径格式 /openai/deployments/{model}/{task}?api-version={version}
//   - 支持 Responses API（含 compact 模式），使用不同的 API 版本
//   - 2025年5月10日后创建的渠道不再移除模型名中的点号
//   - 支持 Realtime 模式的 WebSocket URL（wss://）
//   - 支持 Claude 格式的请求路径转换
//   - Custom: 支持 URL 中的 {model} 占位符替换
//   - 其他渠道（OpenAI、OpenRouter 等）: 使用标准的完整请求路径
//   - Claude/Gemini 格式的非 Responses 请求统一转发到 /v1/chat/completions
//
// 参数:
//   - info: 中继信息，包含渠道类型、基础 URL、请求路径、模型名称等
//
// 返回:
//   - string: 构建完成的完整请求 URL
//   - error: URL 构建失败时的错误
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info.RelayMode == relayconstant.RelayModeRealtime {
		if strings.HasPrefix(info.ChannelBaseUrl, "https://") {
			baseUrl := strings.TrimPrefix(info.ChannelBaseUrl, "https://")
			baseUrl = "wss://" + baseUrl
			info.ChannelBaseUrl = baseUrl
		} else if strings.HasPrefix(info.ChannelBaseUrl, "http://") {
			baseUrl := strings.TrimPrefix(info.ChannelBaseUrl, "http://")
			baseUrl = "ws://" + baseUrl
			info.ChannelBaseUrl = baseUrl
		}
	}
	switch info.ChannelType {
	case constant.ChannelTypeAzure:
		apiVersion := info.ApiVersion
		if apiVersion == "" {
			apiVersion = constant.AzureDefaultAPIVersion
		}
		// https://learn.microsoft.com/en-us/azure/cognitive-services/openai/chatgpt-quickstart?pivots=rest-api&tabs=command-line#rest-api
		requestURL := strings.Split(info.RequestURLPath, "?")[0]
		requestURL = fmt.Sprintf("%s?api-version=%s", requestURL, apiVersion)
		task := strings.TrimPrefix(requestURL, "/v1/")

		if info.RelayFormat == types.RelayFormatClaude {
			task = strings.TrimPrefix(task, "messages")
			task = "chat/completions" + task
		}

		// 特殊处理 responses API（包含 compact）
		if info.RelayMode == relayconstant.RelayModeResponses || info.RelayMode == relayconstant.RelayModeResponsesCompact {
			responsesApiVersion := "preview"

			subUrl := "/openai/v1/responses"
			if strings.Contains(info.ChannelBaseUrl, "cognitiveservices.azure.com") {
				subUrl = "/openai/responses"
				responsesApiVersion = apiVersion
			}

			if info.ChannelOtherSettings.AzureResponsesVersion != "" {
				responsesApiVersion = info.ChannelOtherSettings.AzureResponsesVersion
			}

			// compact 模式追加 /compact
			if info.RelayMode == relayconstant.RelayModeResponsesCompact {
				subUrl = subUrl + "/compact"
			}

			requestURL = fmt.Sprintf("%s?api-version=%s", subUrl, responsesApiVersion)
			return relaycommon.GetFullRequestURL(info.ChannelBaseUrl, requestURL, info.ChannelType), nil
		}

		model_ := info.UpstreamModelName
		// 2025年5月10日后创建的渠道不移除.
		if info.ChannelCreateTime < constant.AzureNoRemoveDotTime {
			model_ = strings.Replace(model_, ".", "", -1)
		}
		// https://github.com/songquanpeng/one-api/issues/67
		requestURL = fmt.Sprintf("/openai/deployments/%s/%s", model_, task)
		if info.RelayMode == relayconstant.RelayModeRealtime {
			requestURL = fmt.Sprintf("/openai/realtime?deployment=%s&api-version=%s", model_, apiVersion)
		}
		return relaycommon.GetFullRequestURL(info.ChannelBaseUrl, requestURL, info.ChannelType), nil
	//case constant.ChannelTypeMiniMax:
	//	return minimax.GetRequestURL(info)
	case constant.ChannelTypeCustom:
		url := info.ChannelBaseUrl
		url = strings.Replace(url, "{model}", info.UpstreamModelName, -1)
		return url, nil
	default:
		if (info.RelayFormat == types.RelayFormatClaude || info.RelayFormat == types.RelayFormatGemini) &&
			info.RelayMode != relayconstant.RelayModeResponses &&
			info.RelayMode != relayconstant.RelayModeResponsesCompact {
			return fmt.Sprintf("%s/v1/chat/completions", info.ChannelBaseUrl), nil
		}
		return relaycommon.GetFullRequestURL(info.ChannelBaseUrl, info.RequestURLPath, info.ChannelType), nil
	}
}

// SetupRequestHeader 设置发送到上游 API 的 HTTP 请求头。
// 根据不同的渠道类型和请求模式设置不同的认证头：
//   - Azure: 使用 api-key 头进行认证
//   - OpenAI: 支持 OpenAI-Organization 头设置组织信息
//   - Realtime 模式: 设置 Sec-WebSocket-Protocol 或 openai-beta 头
//   - OpenRouter: 设置 HTTP-Referer 和 X-OpenRouter-Title 头
//   - 其他渠道: 使用标准的 Bearer token 认证
//
// 如果请求中通过 HeadersOverride 指定了 Authorization 头，
// 则跳过默认的 Authorization 设置，避免冲突。
//
// 参数:
//   - c: Gin 上下文
//   - header: 要设置的 HTTP 请求头指针
//   - info: 中继信息，包含 API 密钥、组织信息等
//
// 返回:
//   - error: 设置失败时的错误（当前实现始终返回 nil）
func (a *Adaptor) SetupRequestHeader(c *gin.Context, header *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, header)
	if info.ChannelType == constant.ChannelTypeAzure {
		header.Set("api-key", info.ApiKey)
		return nil
	}
	if info.ChannelType == constant.ChannelTypeOpenAI && "" != info.Organization {
		header.Set("OpenAI-Organization", info.Organization)
	}
	// 检查 Header Override 是否已设置 Authorization，如果已设置则跳过默认设置
	// 这样可以避免在 Header Override 应用时被覆盖（虽然 Header Override 会在之后应用，但这里作为额外保护）
	hasAuthOverride := false
	if len(info.HeadersOverride) > 0 {
		for k := range info.HeadersOverride {
			if strings.EqualFold(k, "Authorization") {
				hasAuthOverride = true
				break
			}
		}
	}
	if info.RelayMode == relayconstant.RelayModeRealtime {
		swp := c.Request.Header.Get("Sec-WebSocket-Protocol")
		if swp != "" {
			items := []string{
				"realtime",
				"openai-insecure-api-key." + info.ApiKey,
				"openai-beta.realtime-v1",
			}
			header.Set("Sec-WebSocket-Protocol", strings.Join(items, ","))
			//req.Header.Set("Sec-WebSocket-Key", c.Request.Header.Get("Sec-WebSocket-Key"))
			//req.Header.Set("Sec-Websocket-Extensions", c.Request.Header.Get("Sec-Websocket-Extensions"))
			//req.Header.Set("Sec-Websocket-Version", c.Request.Header.Get("Sec-Websocket-Version"))
		} else {
			header.Set("openai-beta", "realtime=v1")
			if !hasAuthOverride {
				header.Set("Authorization", "Bearer "+info.ApiKey)
			}
		}
	} else {
		if !hasAuthOverride {
			header.Set("Authorization", "Bearer "+info.ApiKey)
		}
	}
	if info.ChannelType == constant.ChannelTypeOpenRouter {
		if header.Get("HTTP-Referer") == "" {
			header.Set("HTTP-Referer", "https://github.com/c1cada/NexusTok")
		}
		if header.Get("X-OpenRouter-Title") == "" {
			header.Set("X-OpenRouter-Title", "NexusTok")
		}
	}
	return nil
}

// ConvertOpenAIRequest 对 OpenAI 格式的请求进行最终的格式适配和参数调整。
// 根据不同的渠道类型和模型特性进行特殊处理：
//
//  1. 非 OpenAI/Azure 渠道：移除 StreamOptions 字段（不支持）
//
//  2. OpenRouter 渠道特殊处理：
//     - 自动添加 usage 统计字段
//     - 处理 -thinking 模型后缀（转换为 reasoning 参数）
//     - 适配 Anthropic Claude 模型的 thinking 格式
//     - 转换 ReasoningEffort 为 OpenRouter 的 reasoning 格式
//
//  3. o 系列和 GPT-5 系列模型特殊处理：
//     - 将 MaxTokens 转换为 MaxCompletionTokens
//     - o 系列模型：移除 Temperature 参数
//     - GPT-5 系列：移除 Temperature、TopP、LogProbs 参数
//     - 解析模型名称中的推理力度后缀（如 o3-mini-high）
//     - 将 system 消息角色转换为 developer（o1-mini 和 o1-preview 除外）
//
// 参数:
//   - c: Gin 上下文
//   - info: 中继信息
//   - request: OpenAI 格式的通用请求对象
//
// 返回:
//   - any: 适配后的请求对象
//   - error: 适配失败时的错误
func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	if info.ChannelType != constant.ChannelTypeOpenAI && info.ChannelType != constant.ChannelTypeAzure {
		request.StreamOptions = nil
	}
	if info.ChannelType == constant.ChannelTypeOpenRouter {
		if len(request.Usage) == 0 {
			request.Usage = json.RawMessage(`{"include":true}`)
		}
		// 适配 OpenRouter 的 thinking 后缀
		if !model_setting.ShouldPreserveThinkingSuffix(info.OriginModelName) &&
			strings.HasSuffix(info.UpstreamModelName, "-thinking") {
			info.UpstreamModelName = strings.TrimSuffix(info.UpstreamModelName, "-thinking")
			request.Model = info.UpstreamModelName
			if len(request.Reasoning) == 0 {
				reasoning := map[string]any{
					"enabled": true,
				}
				if request.ReasoningEffort != "" && request.ReasoningEffort != "none" {
					reasoning["effort"] = request.ReasoningEffort
				}
				marshal, err := common.Marshal(reasoning)
				if err != nil {
					return nil, fmt.Errorf("error marshalling reasoning: %w", err)
				}
				request.Reasoning = marshal
			}
			// 清空多余的ReasoningEffort
			request.ReasoningEffort = ""
		} else {
			if len(request.Reasoning) == 0 {
				// 适配 OpenAI 的 ReasoningEffort 格式
				if request.ReasoningEffort != "" {
					reasoning := map[string]any{
						"enabled": true,
					}
					if request.ReasoningEffort != "none" {
						reasoning["effort"] = request.ReasoningEffort
						marshal, err := common.Marshal(reasoning)
						if err != nil {
							return nil, fmt.Errorf("error marshalling reasoning: %w", err)
						}
						request.Reasoning = marshal
					}
				}
			}
			request.ReasoningEffort = ""
		}

		// https://docs.anthropic.com/en/api/openai-sdk#extended-thinking-support
		// 没有做排除3.5Haiku等，要出问题再加吧，最佳兼容性（不是
		if request.THINKING != nil && strings.HasPrefix(info.UpstreamModelName, "anthropic") {
			var thinking dto.Thinking // Claude标准Thinking格式
			if err := json.Unmarshal(request.THINKING, &thinking); err != nil {
				return nil, fmt.Errorf("error Unmarshal thinking: %w", err)
			}

			// 只有当 thinking.Type 是 "enabled" 时才处理
			if thinking.Type == "enabled" {
				// 检查 BudgetTokens 是否为 nil
				if thinking.BudgetTokens == nil {
					return nil, fmt.Errorf("BudgetTokens is nil when thinking is enabled")
				}

				reasoning := openrouter.RequestReasoning{
					Enabled:   true,
					MaxTokens: *thinking.BudgetTokens,
				}

				marshal, err := common.Marshal(reasoning)
				if err != nil {
					return nil, fmt.Errorf("error marshalling reasoning: %w", err)
				}

				request.Reasoning = marshal
			}

			// 清空 THINKING
			request.THINKING = nil
		}

	}
	isOModel := dto.IsOpenAIReasoningOModel(info.UpstreamModelName)
	isGPT5Model := dto.IsOpenAIGPT5Model(info.UpstreamModelName)
	if isOModel || isGPT5Model {
		if lo.FromPtrOr(request.MaxCompletionTokens, uint(0)) == 0 && lo.FromPtrOr(request.MaxTokens, uint(0)) != 0 {
			request.MaxCompletionTokens = request.MaxTokens
			request.MaxTokens = nil
		}

		if isOModel {
			request.Temperature = nil
		}

		// gpt-5系列模型适配 归零不再支持的参数
		if isGPT5Model {
			request.Temperature = nil
			request.TopP = nil
			request.LogProbs = nil
		}

		// 转换模型推理力度后缀
		effort, originModel := reasoning.ParseOpenAIReasoningEffortFromModelSuffix(info.UpstreamModelName)
		if effort != "" {
			request.ReasoningEffort = effort
			info.UpstreamModelName = originModel
			request.Model = originModel
		}

		info.ReasoningEffort = request.ReasoningEffort

		// o系列模型developer适配（o1-mini除外）
		if !strings.HasPrefix(info.UpstreamModelName, "o1-mini") && !strings.HasPrefix(info.UpstreamModelName, "o1-preview") {
			//修改第一个Message的内容，将system改为developer
			if len(request.Messages) > 0 && request.Messages[0].Role == "system" {
				request.Messages[0].Role = "developer"
			}
		}
	}

	return request, nil
}

// ConvertRerankRequest 将重排序请求转换为 OpenAI 格式。
// OpenAI 格式的重排序请求无需额外转换，直接透传。
//
// 参数:
//   - c: Gin 上下文
//   - relayMode: 中继模式
//   - request: 重排序请求对象
//
// 返回:
//   - any: 转换后的请求（原样返回）
//   - error: 始终返回 nil
func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return request, nil
}

// ConvertEmbeddingRequest 将嵌入请求转换为 OpenAI 格式。
// OpenAI 格式的嵌入请求无需额外转换，直接透传。
//
// 参数:
//   - c: Gin 上下文
//   - info: 中继信息
//   - request: 嵌入请求对象
//
// 返回:
//   - any: 转换后的请求（原样返回）
//   - error: 始终返回 nil
func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return request, nil
}

// ConvertAudioRequest 将音频请求转换为 OpenAI 格式。
// 根据不同的中继模式使用不同的处理方式：
//   - TTS（AudioSpeech）: 将请求序列化为 JSON 字节流
//   - STT（AudioTranscription/AudioTranslation）: 构建 multipart/form-data 请求体
//   - 包含 model 字段
//   - 复制原始表单中的所有非文件字段
//   - 处理音频文件上传（file 字段）
//   - 自动设置 Content-Type 头（含 boundary）
//
// 参数:
//   - c: Gin 上下文
//   - info: 中继信息
//   - request: 音频请求对象
//
// 返回:
//   - io.Reader: 请求体读取器
//   - error: 转换失败时的错误
func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	a.ResponseFormat = request.ResponseFormat
	if info.RelayMode == relayconstant.RelayModeAudioSpeech {
		jsonData, err := common.Marshal(request)
		if err != nil {
			return nil, fmt.Errorf("error marshalling object: %w", err)
		}
		return bytes.NewReader(jsonData), nil
	} else {
		var requestBody bytes.Buffer
		writer := multipart.NewWriter(&requestBody)

		writer.WriteField("model", request.Model)

		formData, err2 := common.ParseMultipartFormReusable(c)
		if err2 != nil {
			return nil, fmt.Errorf("error parsing multipart form: %w", err2)
		}

		// 打印类似 curl 命令格式的信息
		logger.LogDebug(c.Request.Context(), fmt.Sprintf("--form 'model=\"%s\"'", request.Model))

		// 遍历表单字段并打印输出
		for key, values := range formData.Value {
			if key == "model" {
				continue
			}
			for _, value := range values {
				writer.WriteField(key, value)
				logger.LogDebug(c.Request.Context(), fmt.Sprintf("--form '%s=\"%s\"'", key, value))
			}
		}

		// 从 formData 中获取文件
		fileHeaders := formData.File["file"]
		if len(fileHeaders) == 0 {
			return nil, errors.New("file is required")
		}

		// 使用 formData 中的第一个文件
		fileHeader := fileHeaders[0]
		logger.LogDebug(c.Request.Context(), fmt.Sprintf("--form 'file=@\"%s\"' (size: %d bytes, content-type: %s)",
			fileHeader.Filename, fileHeader.Size, fileHeader.Header.Get("Content-Type")))

		file, err := fileHeader.Open()
		if err != nil {
			return nil, fmt.Errorf("error opening audio file: %v", err)
		}
		defer file.Close()

		part, err := writer.CreateFormFile("file", fileHeader.Filename)
		if err != nil {
			return nil, errors.New("create form file failed")
		}
		if _, err := io.Copy(part, file); err != nil {
			return nil, errors.New("copy file failed")
		}

		// 关闭 multipart 编写器以设置分界线
		writer.Close()
		c.Request.Header.Set("Content-Type", writer.FormDataContentType())
		logger.LogDebug(c.Request.Context(), fmt.Sprintf("--header 'Content-Type: %s'", writer.FormDataContentType()))
		return &requestBody, nil
	}
}

// ConvertImageRequest 将图片请求转换为 OpenAI 格式。
// 根据不同的中继模式使用不同的处理方式：
//   - ImagesEdits（图片编辑）: 构建 multipart/form-data 请求体
//   - 包含 model 字段和所有非文件表单字段
//   - 支持单张或多张图片上传（image 或 image[] 字段名）
//   - 支持 mask 文件上传（用于指定编辑区域）
//   - 自动检测图片 MIME 类型（JPEG、PNG、WebP）
//   - 如果请求体已经是 JSON 格式，则直接返回
//   - ImagesGenerations（图片生成）: 直接返回请求对象
//
// 参数:
//   - c: Gin 上下文
//   - info: 中继信息
//   - request: 图片请求对象
//
// 返回:
//   - any: 转换后的请求（JSON 对象或 multipart 请求体读取器）
//   - error: 转换失败时的错误
func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	switch info.RelayMode {
	case relayconstant.RelayModeImagesEdits:
		if isJSONRequest(c) {
			return request, nil
		}

		var requestBody bytes.Buffer
		writer := multipart.NewWriter(&requestBody)

		writer.WriteField("model", request.Model)
		// 使用已解析的 multipart 表单，避免重复解析
		mf := c.Request.MultipartForm
		if mf == nil {
			form, err := common.ParseMultipartFormReusable(c)
			if err != nil {
				return nil, errors.New("failed to parse multipart form")
			}
			c.Request.MultipartForm = form
			c.Request.PostForm = form.Value
			mf = c.Request.MultipartForm
		}

		// 写入所有非文件字段
		if mf != nil {
			for key, values := range mf.Value {
				if key == "model" {
					continue
				}
				for _, value := range values {
					writer.WriteField(key, value)
				}
			}
		}

		if mf != nil && mf.File != nil {
			// Check if "image" field exists in any form, including array notation
			var imageFiles []*multipart.FileHeader
			var exists bool

			// First check for standard "image" field
			if imageFiles, exists = mf.File["image"]; !exists || len(imageFiles) == 0 {
				// If not found, check for "image[]" field
				if imageFiles, exists = mf.File["image[]"]; !exists || len(imageFiles) == 0 {
					// If still not found, iterate through all fields to find any that start with "image["
					foundArrayImages := false
					for fieldName, files := range mf.File {
						if strings.HasPrefix(fieldName, "image[") && len(files) > 0 {
							foundArrayImages = true
							imageFiles = append(imageFiles, files...)
						}
					}

					// If no image fields found at all
					if !foundArrayImages && (len(imageFiles) == 0) {
						return nil, errors.New("image is required")
					}
				}
			}

			// Process all image files
			for i, fileHeader := range imageFiles {
				file, err := fileHeader.Open()
				if err != nil {
					return nil, fmt.Errorf("failed to open image file %d: %w", i, err)
				}

				// If multiple images, use image[] as the field name
				fieldName := "image"
				if len(imageFiles) > 1 {
					fieldName = "image[]"
				}

				// Determine MIME type based on file extension
				mimeType := detectImageMimeType(fileHeader.Filename)

				// Create a form file with the appropriate content type
				h := make(textproto.MIMEHeader)
				h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fieldName, fileHeader.Filename))
				h.Set("Content-Type", mimeType)

				part, err := writer.CreatePart(h)
				if err != nil {
					return nil, fmt.Errorf("create form part failed for image %d: %w", i, err)
				}

				if _, err := io.Copy(part, file); err != nil {
					return nil, fmt.Errorf("copy file failed for image %d: %w", i, err)
				}

				// 复制完立即关闭，避免在循环内使用 defer 占用资源
				_ = file.Close()
			}

			// Handle mask file if present
			if maskFiles, exists := mf.File["mask"]; exists && len(maskFiles) > 0 {
				maskFile, err := maskFiles[0].Open()
				if err != nil {
					return nil, errors.New("failed to open mask file")
				}
				// 复制完立即关闭，避免在循环内使用 defer 占用资源

				// Determine MIME type for mask file
				mimeType := detectImageMimeType(maskFiles[0].Filename)

				// Create a form file with the appropriate content type
				h := make(textproto.MIMEHeader)
				h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="mask"; filename="%s"`, maskFiles[0].Filename))
				h.Set("Content-Type", mimeType)

				maskPart, err := writer.CreatePart(h)
				if err != nil {
					return nil, errors.New("create form file failed for mask")
				}

				if _, err := io.Copy(maskPart, maskFile); err != nil {
					return nil, errors.New("copy mask file failed")
				}
				_ = maskFile.Close()
			}
		} else {
			return nil, errors.New("no multipart form data found")
		}

		// 关闭 multipart 编写器以设置分界线
		writer.Close()
		c.Request.Header.Set("Content-Type", writer.FormDataContentType())
		return &requestBody, nil

	default:
		return request, nil
	}
}

// isJSONRequest 判断请求的 Content-Type 是否为 application/json。
// 用于在图片编辑等场景中区分 JSON 请求和 multipart 表单请求。
//
// 参数:
//   - c: Gin 上下文
//
// 返回:
//   - bool: 如果 Content-Type 以 "application/json" 开头则返回 true
func isJSONRequest(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	return strings.HasPrefix(c.Request.Header.Get("Content-Type"), "application/json")
}

// detectImageMimeType 根据文件扩展名检测图片的 MIME 类型。
// 支持的格式：JPEG（.jpg/.jpeg）、PNG（.png）、WebP（.webp）。
// 对于无法识别的扩展名，默认返回 "image/png"。
//
// 参数:
//   - filename: 文件名（含扩展名）
//
// 返回:
//   - string: 对应的 MIME 类型字符串
func detectImageMimeType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	default:
		// Try to detect from extension if possible
		if strings.HasPrefix(ext, ".jp") {
			return "image/jpeg"
		}
		// Default to png as a fallback
		return "image/png"
	}
}

// ConvertOpenAIResponsesRequest 对 OpenAI Responses API 的请求进行格式适配。
// 主要处理模型名称中的推理力度后缀（如 o3-mini-high），
// 将其解析为 Reasoning.Effort 参数并恢复原始模型名称。
//
// 参数:
//   - c: Gin 上下文
//   - info: 中继信息（可为 nil）
//   - request: OpenAI Responses API 请求对象
//
// 返回:
//   - any: 适配后的请求对象
//   - error: 适配失败时的错误
func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	//  转换模型推理力度后缀
	effort, originModel := reasoning.ParseOpenAIReasoningEffortFromModelSuffix(request.Model)
	if effort != "" {
		if request.Reasoning == nil {
			request.Reasoning = &dto.Reasoning{
				Effort: effort,
			}
		} else {
			request.Reasoning.Effort = effort
		}
		request.Model = originModel
	}
	if info != nil && request.Reasoning != nil && request.Reasoning.Effort != "" {
		info.ReasoningEffort = request.Reasoning.Effort
	}
	return request, nil
}

// DoRequest 根据请求模式选择合适的 HTTP 请求方式并发送到上游 API。
// 不同的请求模式使用不同的请求方式：
//   - 音频转录/翻译、图片编辑（非 JSON）: 使用 multipart/form-data 表单请求
//   - Realtime 模式: 使用 WebSocket 连接
//   - 其他模式: 使用标准的 API 请求（JSON body）
//
// 参数:
//   - c: Gin 上下文
//   - info: 中继信息
//   - requestBody: 请求体读取器
//
// 返回:
//   - any: 响应对象（通常是 *http.Response）
//   - error: 请求失败时的错误
func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	if info.RelayMode == relayconstant.RelayModeAudioTranscription ||
		info.RelayMode == relayconstant.RelayModeAudioTranslation ||
		(info.RelayMode == relayconstant.RelayModeImagesEdits && !isJSONRequest(c)) {
		return channel.DoFormRequest(a, c, info, requestBody)
	} else if info.RelayMode == relayconstant.RelayModeRealtime {
		return channel.DoWssRequest(a, c, info, requestBody)
	} else {
		return channel.DoApiRequest(a, c, info, requestBody)
	}
}

// DoResponse 根据请求模式分发到对应的响应处理器。
// 不同的请求模式使用不同的处理器：
//   - Realtime: WebSocket 实时对话处理器
//   - AudioSpeech: TTS 语音合成处理器
//   - AudioTranslation/AudioTranscription: STT 语音识别处理器
//   - ImagesGenerations/ImagesEdits: 图片生成/编辑处理器
//   - Rerank: 重排序处理器
//   - Responses: OpenAI Responses API 处理器（支持流式和非流式）
//   - ResponsesCompact: Responses API 压缩模式处理器
//   - 默认（聊天/补全）: 流式使用 OaiStreamHandler，非流式使用 OpenaiHandler
//
// 参数:
//   - c: Gin 上下文
//   - resp: 上游 API 返回的 HTTP 响应
//   - info: 中继信息
//
// 返回:
//   - usage: token 使用量信息（类型为 any，实际为 *dto.Usage 或 *dto.RealtimeUsage）
//   - err: 处理过程中的错误
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NexusTokError) {
	switch info.RelayMode {
	case relayconstant.RelayModeRealtime:
		err, usage = OpenaiRealtimeHandler(c, info)
	case relayconstant.RelayModeAudioSpeech:
		usage = OpenaiTTSHandler(c, resp, info)
	case relayconstant.RelayModeAudioTranslation:
		fallthrough
	case relayconstant.RelayModeAudioTranscription:
		err, usage = OpenaiSTTHandler(c, resp, info, a.ResponseFormat)
	case relayconstant.RelayModeImagesGenerations, relayconstant.RelayModeImagesEdits:
		if info.IsStream {
			usage, err = OpenaiImageStreamHandler(c, info, resp)
		} else {
			usage, err = OpenaiImageHandler(c, info, resp)
		}
	case relayconstant.RelayModeRerank:
		usage, err = common_handler.RerankHandler(c, info, resp)
	case relayconstant.RelayModeResponses:
		if info.IsStream {
			usage, err = OaiResponsesStreamHandler(c, info, resp)
		} else {
			usage, err = OaiResponsesHandler(c, info, resp)
		}
	case relayconstant.RelayModeResponsesCompact:
		usage, err = OaiResponsesCompactionHandler(c, resp)
	default:
		if info.IsStream {
			usage, err = OaiStreamHandler(c, info, resp)
		} else {
			usage, err = OpenaiHandler(c, info, resp)
		}
	}
	return
}

// GetModelList 获取当前渠道类型支持的模型列表。
// 根据不同的子渠道类型返回对应的模型列表：
//   - 360: ai360 渠道的模型列表
//   - LingYiWanWu: 零一万物渠道的模型列表
//   - Xinference: Xinference 渠道的模型列表
//   - OpenRouter: OpenRouter 渠道的模型列表
//   - 默认: 标准 OpenAI 模型列表
//
// 返回:
//   - []string: 支持的模型名称列表
func (a *Adaptor) GetModelList() []string {
	switch a.ChannelType {
	case constant.ChannelType360:
		return ai360.ModelList
	case constant.ChannelTypeLingYiWanWu:
		return lingyiwanwu.ModelList
	//case constant.ChannelTypeMiniMax:
	//	return minimax.ModelList
	case constant.ChannelTypeXinference:
		return xinference.ModelList
	case constant.ChannelTypeOpenRouter:
		return openrouter.ModelList
	default:
		return ModelList
	}
}

// GetChannelName 获取当前渠道类型的标识名称。
// 根据不同的子渠道类型返回对应的渠道名称：
//   - 360: "360"
//   - LingYiWanWu: "lingyiwanwu"
//   - Xinference: "xinference"
//   - OpenRouter: "openrouter"
//   - 默认: "openai"
//
// 返回:
//   - string: 渠道标识名称
func (a *Adaptor) GetChannelName() string {
	switch a.ChannelType {
	case constant.ChannelType360:
		return ai360.ChannelName
	case constant.ChannelTypeLingYiWanWu:
		return lingyiwanwu.ChannelName
	//case constant.ChannelTypeMiniMax:
	//	return minimax.ChannelName
	case constant.ChannelTypeXinference:
		return xinference.ChannelName
	case constant.ChannelTypeOpenRouter:
		return openrouter.ChannelName
	default:
		return ChannelName
	}
}
