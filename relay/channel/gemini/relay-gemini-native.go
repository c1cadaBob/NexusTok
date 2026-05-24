// gemini - relay-gemini-native.go
// Gemini 原生格式的中继处理逻辑。
// 本文件处理以 Gemini 原生格式（而非 OpenAI 兼容格式）发出的请求和响应，
// 主要包括：
//   - 非流式文本生成响应处理（GeminiTextGenerationHandler）
//   - 原生嵌入响应处理（NativeGeminiEmbeddingHandler）
//   - 流式文本生成响应处理（GeminiTextGenerationStreamHandler）
// 这些处理器将 Gemini 原生响应透传给客户端，不做 OpenAI 格式转换。
package gemini

import (
	"fmt"
	"io"
	"net/http"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/logger"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/relay/helper"
	"github.com/c1cada/NexusTok/service"
	"github.com/c1cada/NexusTok/types"

	"github.com/gin-gonic/gin"
)

// GeminiTextGenerationHandler 处理 Gemini 原生格式的非流式文本生成响应。
// 直接解析 Gemini API 返回的 JSON 响应，计算使用量统计，
// 并将原始响应体透传给客户端（不做 OpenAI 格式转换）。
// 当响应中无候选结果但存在 PromptFeedback.BlockReason 时，记录拒绝原因。
// 参数:
//   - c: Gin 上下文
//   - info: Relay 中继信息
//   - resp: Gemini API 的 HTTP 响应
//
// 返回:
//   - *dto.Usage: 使用量统计（基于 UsageMetadata 计算）
//   - *types.NexusTokError: 错误信息
func GeminiTextGenerationHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NexusTokError) {
	defer service.CloseResponseBodyGracefully(resp)

	// 读取响应体
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	if common.DebugEnabled {
		println(string(responseBody))
	}

	// 解析为 Gemini 原生响应格式
	var geminiResponse dto.GeminiChatResponse
	err = common.Unmarshal(responseBody, &geminiResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	if len(geminiResponse.Candidates) == 0 && geminiResponse.PromptFeedback != nil && geminiResponse.PromptFeedback.BlockReason != nil {
		common.SetContextKey(c, constant.ContextKeyAdminRejectReason, fmt.Sprintf("gemini_block_reason=%s", *geminiResponse.PromptFeedback.BlockReason))
	}

	// 计算使用量（基于 UsageMetadata）
	usage := buildUsageFromGeminiMetadata(geminiResponse.UsageMetadata, info.GetEstimatePromptTokens())

	service.IOCopyBytesGracefully(c, resp, responseBody)

	return &usage, nil
}

// NativeGeminiEmbeddingHandler 处理 Gemini 原生格式的嵌入响应。
// 支持两种嵌入模式：
//   - 批量嵌入（batchEmbedContents）：解析为 GeminiBatchEmbeddingResponse
//   - 单条嵌入（embedContent）：解析为 GeminiEmbeddingResponse
//
// 嵌入响应以 Gemini 原生格式透传给客户端，不做 OpenAI 格式转换。
// Token 使用量通过 service.ResponseText2Usage 计算。
// 参数:
//   - c: Gin 上下文
//   - resp: Gemini API 的 HTTP 响应
//   - info: Relay 中继信息（包含 IsGeminiBatchEmbedding 标记）
//
// 返回:
//   - *dto.Usage: 使用量统计
//   - *types.NexusTokError: 错误信息
func NativeGeminiEmbeddingHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NexusTokError) {
	defer service.CloseResponseBodyGracefully(resp)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	if common.DebugEnabled {
		println(string(responseBody))
	}

	usage := service.ResponseText2Usage(c, "", info.UpstreamModelName, info.GetEstimatePromptTokens())

	if info.IsGeminiBatchEmbedding {
		var geminiResponse dto.GeminiBatchEmbeddingResponse
		err = common.Unmarshal(responseBody, &geminiResponse)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
	} else {
		var geminiResponse dto.GeminiEmbeddingResponse
		err = common.Unmarshal(responseBody, &geminiResponse)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
	}

	service.IOCopyBytesGracefully(c, resp, responseBody)

	return usage, nil
}

// GeminiTextGenerationStreamHandler 处理 Gemini 原生格式的流式文本生成响应。
// 设置 SSE（Server-Sent Events）流式响应头，然后通过 geminiStreamHandler
// 逐块处理流式数据。每块数据直接以原始格式发送给客户端（不做 OpenAI 格式转换）。
// 参数:
//   - c: Gin 上下文
//   - info: Relay 中继信息
//   - resp: Gemini API 的 HTTP 流式响应
//
// 返回:
//   - *dto.Usage: 使用量统计
//   - *types.NexusTokError: 错误信息
func GeminiTextGenerationStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NexusTokError) {
	helper.SetEventStreamHeaders(c)

	return geminiStreamHandler(c, info, resp, func(data string, geminiResponse *dto.GeminiChatResponse) bool {
		err := helper.StringData(c, data)
		if err != nil {
			logger.LogError(c, "failed to write stream data: "+err.Error())
			return false
		}
		info.SendResponseCount++
		return true
	})
}
