// Package helper 提供了中继层的各种辅助函数。
// 本文件负责从 HTTP 请求中解析并验证各种格式的 API 请求体。
// 支持的请求格式包括：OpenAI 通用格式、Claude 格式、Gemini 格式、
// OpenAI Responses 格式、图像生成格式、嵌入格式、Rerank 格式和音频格式。
package helper

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/logger"
	relayconstant "github.com/c1cada/NexusTok/relay/constant"
	"github.com/c1cada/NexusTok/types"
	"github.com/samber/lo"

	"github.com/gin-gonic/gin"
)

// GetAndValidateRequest 根据中继格式（RelayFormat）分发到对应的请求解析和验证函数。
// 这是请求解析的入口函数，根据 format 参数路由到具体的处理函数。
//
// 参数：
//   - c: Gin 请求上下文
//   - format: 中继格式标识（如 RelayFormatOpenAI、RelayFormatClaude 等）
//
// 返回值：
//   - request: 解析后的请求对象（实现 dto.Request 接口）
//   - err: 解析或验证过程中的错误
func GetAndValidateRequest(c *gin.Context, format types.RelayFormat) (request dto.Request, err error) {
	// 从 URL 路径推断中继模式
	relayMode := relayconstant.Path2RelayMode(c.Request.URL.Path)

	switch format {
	case types.RelayFormatOpenAI:
		request, err = GetAndValidateTextRequest(c, relayMode)
	case types.RelayFormatGemini:
		// Gemini 有多种嵌入接口，需要根据路径区分
		if strings.Contains(c.Request.URL.Path, ":embedContent") {
			request, err = GetAndValidateGeminiEmbeddingRequest(c)
		} else if strings.Contains(c.Request.URL.Path, ":batchEmbedContents") {
			request, err = GetAndValidateGeminiBatchEmbeddingRequest(c)
		} else {
			request, err = GetAndValidateGeminiRequest(c)
		}
	case types.RelayFormatClaude:
		request, err = GetAndValidateClaudeRequest(c)
	case types.RelayFormatOpenAIResponses:
		request, err = GetAndValidateResponsesRequest(c)
	case types.RelayFormatOpenAIResponsesCompaction:
		request, err = GetAndValidateResponsesCompactionRequest(c)

	case types.RelayFormatOpenAIImage:
		request, err = GetAndValidOpenAIImageRequest(c, relayMode)
	case types.RelayFormatEmbedding:
		request, err = GetAndValidateEmbeddingRequest(c, relayMode)
	case types.RelayFormatRerank:
		request, err = GetAndValidateRerankRequest(c)
	case types.RelayFormatOpenAIAudio:
		request, err = GetAndValidAudioRequest(c, relayMode)
	case types.RelayFormatOpenAIRealtime:
		// 实时通信 API 不需要解析请求体
		request = &dto.BaseRequest{}
	default:
		return nil, fmt.Errorf("unsupported relay format: %s", format)
	}
	return request, err
}

// GetAndValidAudioRequest 解析并验证音频相关请求（TTS、Whisper）。
// 对于语音合成（AudioSpeech）和转录/翻译模式，都要求 model 字段非空。
// 转录/翻译模式下如果未指定 responseFormat，默认设为 "json"。
//
// 参数：
//   - c: Gin 请求上下文
//   - relayMode: 中继模式常量
//
// 返回值：
//   - *dto.AudioRequest: 解析后的音频请求对象
//   - error: 验证错误
func GetAndValidAudioRequest(c *gin.Context, relayMode int) (*dto.AudioRequest, error) {
	audioRequest := &dto.AudioRequest{}
	err := common.UnmarshalBodyReusable(c, audioRequest)
	if err != nil {
		return nil, err
	}
	switch relayMode {
	case relayconstant.RelayModeAudioSpeech:
		if audioRequest.Model == "" {
			return nil, errors.New("model is required")
		}
	default:
		if audioRequest.Model == "" {
			return nil, errors.New("model is required")
		}
		if audioRequest.ResponseFormat == "" {
			audioRequest.ResponseFormat = "json"
		}
	}
	return audioRequest, nil
}

// GetAndValidateRerankRequest 解析并验证 Rerank（重排序）请求。
// 验证规则：query 字段和 documents 字段均不能为空。
//
// 参数：
//   - c: Gin 请求上下文
//
// 返回值：
//   - *dto.RerankRequest: 解析后的 Rerank 请求对象
//   - error: 验证错误
func GetAndValidateRerankRequest(c *gin.Context) (*dto.RerankRequest, error) {
	var rerankRequest *dto.RerankRequest
	err := common.UnmarshalBodyReusable(c, &rerankRequest)
	if err != nil {
		logger.LogError(c, fmt.Sprintf("getAndValidateTextRequest failed: %s", err.Error()))
		return nil, types.NewError(err, types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}

	if rerankRequest.Query == "" {
		return nil, types.NewError(fmt.Errorf("query is empty"), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	if len(rerankRequest.Documents) == 0 {
		return nil, types.NewError(fmt.Errorf("documents is empty"), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	return rerankRequest, nil
}

// GetAndValidateEmbeddingRequest 解析并验证嵌入（Embedding）请求。
// 对于审核模式（Moderations），model 默认为 "omni-moderation-latest"；
// 对于嵌入模式（Embeddings），model 默认从 URL 路径参数获取。
//
// 参数：
//   - c: Gin 请求上下文
//   - relayMode: 中继模式常量
//
// 返回值：
//   - *dto.EmbeddingRequest: 解析后的嵌入请求对象
//   - error: 验证错误
func GetAndValidateEmbeddingRequest(c *gin.Context, relayMode int) (*dto.EmbeddingRequest, error) {
	var embeddingRequest *dto.EmbeddingRequest
	err := common.UnmarshalBodyReusable(c, &embeddingRequest)
	if err != nil {
		logger.LogError(c, fmt.Sprintf("getAndValidateTextRequest failed: %s", err.Error()))
		return nil, types.NewError(err, types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}

	if embeddingRequest.Input == nil {
		return nil, fmt.Errorf("input is empty")
	}
	if relayMode == relayconstant.RelayModeModerations && embeddingRequest.Model == "" {
		embeddingRequest.Model = "omni-moderation-latest"
	}
	if relayMode == relayconstant.RelayModeEmbeddings && embeddingRequest.Model == "" {
		embeddingRequest.Model = c.Param("model")
	}
	return embeddingRequest, nil
}

// maxTokensLimit 约束客户端可传入的最大输出 token 字段。
// 这些字段会进入预消费额度估算、动态计费和部分 provider 的 int 转换；
// 使用 math.MaxInt32/2 作为保守上限，可以避免超大 uint 在后续链路中溢出或反向计费。
const maxTokensLimit = math.MaxInt32 / 2

func exceedsMaxTokensLimit(values ...*uint) bool {
	for _, value := range values {
		if lo.FromPtrOr(value, uint(0)) > maxTokensLimit {
			return true
		}
	}
	return false
}

// GetAndValidateResponsesRequest 解析并验证 OpenAI Responses API 请求。
// 验证规则：model 和 input 字段均不能为空。
//
// 参数：
//   - c: Gin 请求上下文
//
// 返回值：
//   - *dto.OpenAIResponsesRequest: 解析后的 Responses 请求对象
//   - error: 验证错误
func GetAndValidateResponsesRequest(c *gin.Context) (*dto.OpenAIResponsesRequest, error) {
	request := &dto.OpenAIResponsesRequest{}
	err := common.UnmarshalBodyReusable(c, request)
	if err != nil {
		return nil, err
	}
	if request.Model == "" {
		return nil, errors.New("model is required")
	}
	if request.Input == nil {
		return nil, errors.New("input is required")
	}
	if exceedsMaxTokensLimit(request.MaxOutputTokens) {
		return nil, errors.New("max_output_tokens is invalid")
	}
	return request, nil
}

// GetAndValidateResponsesCompactionRequest 解析并验证 Responses 精简模式请求。
// 验证规则：model 字段不能为空。
func GetAndValidateResponsesCompactionRequest(c *gin.Context) (*dto.OpenAIResponsesCompactionRequest, error) {
	request := &dto.OpenAIResponsesCompactionRequest{}
	if err := common.UnmarshalBodyReusable(c, request); err != nil {
		return nil, err
	}
	if request.Model == "" {
		return nil, errors.New("model is required")
	}
	return request, nil
}

// GetAndValidOpenAIImageRequest 解析并验证 OpenAI 图像生成/编辑请求。
// 支持两种输入方式：multipart/form-data（图像编辑）和 JSON body。
// 针对不同模型（dall-e、dall-e-2、dall-e-3、gpt-image-1）有不同的参数验证规则。
//
// 参数：
//   - c: Gin 请求上下文
//   - relayMode: 中继模式常量
//
// 返回值：
//   - *dto.ImageRequest: 解析后的图像请求对象
//   - error: 验证错误
func GetAndValidOpenAIImageRequest(c *gin.Context, relayMode int) (*dto.ImageRequest, error) {
	imageRequest := &dto.ImageRequest{}

	switch relayMode {
	case relayconstant.RelayModeImagesEdits:
		if strings.Contains(c.Request.Header.Get("Content-Type"), "multipart/form-data") {
			form, err := common.ParseMultipartFormReusable(c)
			if err != nil {
				return nil, fmt.Errorf("failed to parse image edit form request: %w", err)
			}
			formData := url.Values(form.Value)
			c.Request.MultipartForm = form
			c.Request.PostForm = formData
			imageRequest.Prompt = formData.Get("prompt")
			imageRequest.Model = formData.Get("model")
			if nValue := strings.TrimSpace(formData.Get("n")); nValue != "" {
				n, err := strconv.Atoi(nValue)
				if err != nil || n < 0 || n > dto.MaxImageN {
					return nil, fmt.Errorf("n must be an integer between 1 and %d", dto.MaxImageN)
				}
				imageRequest.N = common.GetPointer(uint(n))
			}
			imageRequest.Quality = formData.Get("quality")
			imageRequest.Size = formData.Get("size")
			if streamValue := strings.TrimSpace(formData.Get("stream")); streamValue != "" {
				stream, err := strconv.ParseBool(streamValue)
				if err != nil {
					return nil, fmt.Errorf("invalid stream value: %w", err)
				}
				imageRequest.Stream = common.GetPointer(stream)
			}
			if imageValue := formData.Get("image"); imageValue != "" {
				imageRequest.Image, _ = common.Marshal(imageValue)
			}

			if imageRequest.Model == "gpt-image-1" {
				if imageRequest.Quality == "" {
					imageRequest.Quality = "standard"
				}
			}
			if imageRequest.N == nil || *imageRequest.N == 0 {
				imageRequest.N = common.GetPointer(uint(1))
			}

			hasWatermark := formData.Has("watermark")
			if hasWatermark {
				watermark := formData.Get("watermark") == "true"
				imageRequest.Watermark = &watermark
			}
			break
		}
		fallthrough
	default:
		if strings.HasPrefix(c.Request.Header.Get("Content-Type"), "application/json") {
			if err := validateOpenAIImageRawN(c); err != nil {
				return nil, err
			}
		}
		err := common.UnmarshalBodyReusable(c, imageRequest)
		if err != nil {
			return nil, err
		}

		if imageRequest.Model == "" {
			//imageRequest.Model = "dall-e-3"
			return nil, errors.New("model is required")
		}

		if strings.Contains(imageRequest.Size, "×") {
			return nil, errors.New("size an unexpected error occurred in the parameter, please use 'x' instead of the multiplication sign '×'")
		}

		if imageRequest.N != nil && *imageRequest.N > dto.MaxImageN {
			return nil, fmt.Errorf("n must be an integer between 1 and %d", dto.MaxImageN)
		}

		// Not "256x256", "512x512", or "1024x1024"
		if imageRequest.Model == "dall-e-2" || imageRequest.Model == "dall-e" {
			if imageRequest.Size != "" && imageRequest.Size != "256x256" && imageRequest.Size != "512x512" && imageRequest.Size != "1024x1024" {
				return nil, errors.New("size must be one of 256x256, 512x512, or 1024x1024 for dall-e-2 or dall-e")
			}
			if imageRequest.Size == "" {
				imageRequest.Size = "1024x1024"
			}
		} else if imageRequest.Model == "dall-e-3" {
			if imageRequest.Size != "" && imageRequest.Size != "1024x1024" && imageRequest.Size != "1024x1792" && imageRequest.Size != "1792x1024" {
				return nil, errors.New("size must be one of 1024x1024, 1024x1792 or 1792x1024 for dall-e-3")
			}
			if imageRequest.Quality == "" {
				imageRequest.Quality = "standard"
			}
			if imageRequest.Size == "" {
				imageRequest.Size = "1024x1024"
			}
		} else if imageRequest.Model == "gpt-image-1" {
			if imageRequest.Quality == "" {
				imageRequest.Quality = "auto"
			}
		}

		//if imageRequest.Prompt == "" {
		//	return nil, errors.New("prompt is required")
		//}

		if imageRequest.N == nil || *imageRequest.N == 0 {
			imageRequest.N = common.GetPointer(uint(1))
		}
	}

	return imageRequest, nil
}

// validateOpenAIImageRawN 在正式解析 ImageRequest 前预检 n 的数值边界。
// 这里先读取原始 JSON，是为了在客户端传入超大整数时避免 json 到 uint 的溢出错误
// 泄漏到底层类型细节，并统一返回图片接口的业务边界错误。
func validateOpenAIImageRawN(c *gin.Context) error {
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return err
	}
	requestBody, err := storage.Bytes()
	if err != nil {
		return err
	}
	var raw struct {
		N *float64 `json:"n"`
	}
	if err := common.Unmarshal(requestBody, &raw); err != nil {
		return err
	}
	if raw.N == nil {
		return nil
	}
	n := *raw.N
	if n < 0 || n > float64(dto.MaxImageN) || math.Trunc(n) != n {
		return fmt.Errorf("n must be an integer between 1 and %d", dto.MaxImageN)
	}
	return nil
}

// GetAndValidateClaudeRequest 解析并验证 Claude 格式的请求。
// 验证规则：messages 和 model 字段均不能为空。
//
// 参数：
//   - c: Gin 请求上下文
//
// 返回值：
//   - *dto.ClaudeRequest: 解析后的 Claude 请求对象
//   - error: 验证错误
func GetAndValidateClaudeRequest(c *gin.Context) (textRequest *dto.ClaudeRequest, err error) {
	textRequest = &dto.ClaudeRequest{}
	err = common.UnmarshalBodyReusable(c, textRequest)
	if err != nil {
		return nil, err
	}
	if textRequest.Messages == nil || len(textRequest.Messages) == 0 {
		return nil, errors.New("field messages is required")
	}
	if textRequest.Model == "" {
		return nil, errors.New("field model is required")
	}
	if exceedsMaxTokensLimit(textRequest.MaxTokens, textRequest.MaxTokensToSample) {
		return nil, errors.New("max_tokens is invalid")
	}

	//if textRequest.Stream {
	//	relayInfo.IsStream = true
	//}

	return textRequest, nil
}

// GetAndValidateTextRequest 解析并验证 OpenAI 通用格式的文本请求。
// 支持聊天补全、文本补全、嵌入、审核和编辑等多种中继模式。
// 验证规则因模式而异，包括 model、messages、prompt、input、instruction 等字段。
// 还支持 FIM（Fill-in-the-middle）请求，此时 messages 可选。
//
// 参数：
//   - c: Gin 请求上下文
//   - relayMode: 中继模式常量
//
// 返回值：
//   - *dto.GeneralOpenAIRequest: 解析后的通用 OpenAI 请求对象
//   - error: 验证错误
func GetAndValidateTextRequest(c *gin.Context, relayMode int) (*dto.GeneralOpenAIRequest, error) {
	textRequest := &dto.GeneralOpenAIRequest{}
	err := common.UnmarshalBodyReusable(c, textRequest)
	if err != nil {
		return nil, err
	}

	if relayMode == relayconstant.RelayModeModerations && textRequest.Model == "" {
		textRequest.Model = "text-moderation-latest"
	}
	if relayMode == relayconstant.RelayModeEmbeddings && textRequest.Model == "" {
		textRequest.Model = c.Param("model")
	}

	if exceedsMaxTokensLimit(textRequest.MaxTokens, textRequest.MaxCompletionTokens) {
		return nil, errors.New("max_tokens is invalid")
	}
	if textRequest.Model == "" {
		return nil, errors.New("model is required")
	}
	if textRequest.WebSearchOptions != nil {
		if textRequest.WebSearchOptions.SearchContextSize != "" {
			validSizes := map[string]bool{
				"high":   true,
				"medium": true,
				"low":    true,
			}
			if !validSizes[textRequest.WebSearchOptions.SearchContextSize] {
				return nil, errors.New("invalid search_context_size, must be one of: high, medium, low")
			}
		} else {
			textRequest.WebSearchOptions.SearchContextSize = "medium"
		}
	}
	switch relayMode {
	case relayconstant.RelayModeCompletions:
		if textRequest.Prompt == "" {
			return nil, errors.New("field prompt is required")
		}
	case relayconstant.RelayModeChatCompletions:
		// FIM（Fill-in-the-middle）请求带 prefix/suffix 时允许 messages 为空。
		// 具体 provider 适配器会在需要时补齐，部分模型提供商（如 DeepSeek）也允许该形态。
		if len(textRequest.Messages) == 0 && textRequest.Prefix == nil && textRequest.Suffix == nil {
			return nil, errors.New("field messages is required")
		}
	case relayconstant.RelayModeEmbeddings:
	case relayconstant.RelayModeModerations:
		if textRequest.Input == nil || textRequest.Input == "" {
			return nil, errors.New("field input is required")
		}
	case relayconstant.RelayModeEdits:
		if textRequest.Instruction == "" {
			return nil, errors.New("field instruction is required")
		}
	}
	return textRequest, nil
}

// GetAndValidateGeminiRequest 解析并验证 Gemini 格式的聊天请求。
// 验证规则：contents 和 requests 字段不能同时为空。
func GetAndValidateGeminiRequest(c *gin.Context) (*dto.GeminiChatRequest, error) {
	request := &dto.GeminiChatRequest{}
	err := common.UnmarshalBodyReusable(c, request)
	if err != nil {
		return nil, err
	}
	if len(request.Contents) == 0 && len(request.Requests) == 0 {
		return nil, errors.New("contents is required")
	}
	if exceedsMaxTokensLimit(request.GenerationConfig.MaxOutputTokens) {
		return nil, errors.New("maxOutputTokens is invalid")
	}

	//if c.Query("alt") == "sse" {
	//	relayInfo.IsStream = true
	//}

	return request, nil
}

// GetAndValidateGeminiEmbeddingRequest 解析并验证 Gemini 单条嵌入请求。
func GetAndValidateGeminiEmbeddingRequest(c *gin.Context) (*dto.GeminiEmbeddingRequest, error) {
	request := &dto.GeminiEmbeddingRequest{}
	err := common.UnmarshalBodyReusable(c, request)
	if err != nil {
		return nil, err
	}
	return request, nil
}

// GetAndValidateGeminiBatchEmbeddingRequest 解析并验证 Gemini 批量嵌入请求。
func GetAndValidateGeminiBatchEmbeddingRequest(c *gin.Context) (*dto.GeminiBatchEmbeddingRequest, error) {
	request := &dto.GeminiBatchEmbeddingRequest{}
	err := common.UnmarshalBodyReusable(c, request)
	if err != nil {
		return nil, err
	}
	return request, nil
}
