// openai - audio.go
// OpenAI 渠道的音频处理文件。
// 本文件负责处理 OpenAI 兼容的音频 API 请求和响应，包括：
// - TTS（Text-to-Speech）语音合成：处理流式和非流式响应，计算音频时长和 token 使用量
// - STT（Speech-to-Text）语音识别：处理转录/翻译响应，提取 token 使用量
package openai

import (
	"bytes"
	"fmt"
	"io"
	"math"
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

// OpenaiTTSHandler 处理 OpenAI TTS（Text-to-Speech）语音合成 API 的响应。
// 该函数支持流式和非流式两种模式：
//
// 流式模式：
//   - 使用 StreamScannerHandler 逐行扫描 SSE 数据
//   - 从包含 usage 字段的数据中提取 token 使用量
//   - 将每行数据实时转发给客户端
//
// 非流式模式：
//   - 读取完整的音频响应体
//   - 将音频数据写回客户端
//   - 根据音频格式计算时长和 token 使用量：
//   - PCM 格式：根据采样率（24kHz）、位深（16-bit）、声道数（1）直接计算
//   - 其他格式（MP3 等）：通过 common.GetAudioDuration 解析文件头获取时长
//   - token 计算公式：ceil(duration) / 60.0 * 1000（每分钟 1000 tokens）
//   - 如果无法获取时长，按文件大小（KB）粗略估算
//
// 注意：一旦上游已写入响应头，后续的响应体读取失败被视为不可恢复错误，
// 不会返回错误以触发外部重试（类似 nginx 负载均衡的重试策略）。
//
// 参数:
//   - c: Gin 上下文
//   - resp: 上游 API 返回的 HTTP 响应
//   - info: 中继信息，包含流式标记和请求信息
//
// 返回:
//   - *dto.Usage: token 使用量信息
func OpenaiTTSHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) *dto.Usage {
	// the status code has been judged before, if there is a body reading failure,
	// it should be regarded as a non-recoverable error, so it should not return err for external retry.
	// Analogous to nginx's load balancing, it will only retry if it can't be requested or
	// if the upstream returns a specific status code, once the upstream has already written the header,
	// the subsequent failure of the response body should be regarded as a non-recoverable error,
	// and can be terminated directly.
	defer service.CloseResponseBodyGracefully(resp)
	usage := &dto.Usage{}
	usage.PromptTokens = info.GetEstimatePromptTokens()
	usage.TotalTokens = info.GetEstimatePromptTokens()
	for k, v := range resp.Header {
		if !service.ShouldCopyUpstreamHeader(c, k, v) {
			continue
		}
		c.Writer.Header().Set(k, v[0])
	}
	c.Writer.WriteHeader(resp.StatusCode)

	if info.IsStream {
		helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
			if service.SundaySearch(data, "usage") {
				var simpleResponse dto.SimpleResponse
				if err := common.Unmarshal([]byte(data), &simpleResponse); err != nil {
					logger.LogError(c, err.Error())
					sr.Error(err)
				} else if simpleResponse.Usage.TotalTokens != 0 {
					usage.PromptTokens = simpleResponse.Usage.InputTokens
					usage.CompletionTokens = simpleResponse.OutputTokens
					usage.TotalTokens = simpleResponse.TotalTokens
				}
			}
			if err := helper.StringData(c, data); err != nil {
				sr.Error(err)
			}
		})
	} else {
		common.SetContextKey(c, constant.ContextKeyLocalCountTokens, true)
		// 读取响应体到缓冲区
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			logger.LogError(c, fmt.Sprintf("failed to read TTS response body: %v", err))
			c.Writer.WriteHeaderNow()
			return usage
		}

		// 写入响应到客户端
		c.Writer.WriteHeaderNow()
		_, err = c.Writer.Write(bodyBytes)
		if err != nil {
			logger.LogError(c, fmt.Sprintf("failed to write TTS response: %v", err))
		}

		// 计算音频时长并更新 usage
		audioFormat := "mp3" // 默认格式
		if audioReq, ok := info.Request.(*dto.AudioRequest); ok && audioReq.ResponseFormat != "" {
			audioFormat = audioReq.ResponseFormat
		}

		var duration float64
		var durationErr error

		if audioFormat == "pcm" {
			// PCM 格式没有文件头，根据 OpenAI TTS 的 PCM 参数计算时长
			// 采样率: 24000 Hz, 位深度: 16-bit (2 bytes), 声道数: 1
			const sampleRate = 24000
			const bytesPerSample = 2
			const channels = 1
			duration = float64(len(bodyBytes)) / float64(sampleRate*bytesPerSample*channels)
		} else {
			ext := "." + audioFormat
			reader := bytes.NewReader(bodyBytes)
			duration, durationErr = common.GetAudioDuration(c.Request.Context(), reader, ext)
		}

		usage.PromptTokensDetails.TextTokens = usage.PromptTokens

		if durationErr != nil {
			logger.LogWarn(c, fmt.Sprintf("failed to get audio duration: %v", durationErr))
			// 如果无法获取时长，则设置保底的 CompletionTokens，根据body大小计算
			sizeInKB := float64(len(bodyBytes)) / 1000.0
			estimatedTokens := int(math.Ceil(sizeInKB)) // 粗略估算每KB约等于1 token
			usage.CompletionTokens = estimatedTokens
			usage.CompletionTokenDetails.AudioTokens = estimatedTokens
		} else if duration > 0 {
			// 计算 token: ceil(duration) / 60.0 * 1000，即每分钟 1000 tokens
			completionTokens := int(math.Round(math.Ceil(duration) / 60.0 * 1000))
			usage.CompletionTokens = completionTokens
			usage.CompletionTokenDetails.AudioTokens = completionTokens
		}
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}

	return usage
}

// OpenaiSTTHandler 处理 OpenAI STT（Speech-to-Text）语音识别 API 的响应。
// 该函数处理音频转录（Transcription）和翻译（Translation）两种模式的响应。
//
// 处理流程：
//  1. 读取上游 API 返回的完整响应体
//  2. 将响应体写回客户端
//  3. 尝试从响应中解析 usage 字段：
//     - 如果存在有效的 TotalTokens，直接使用上游返回的 token 统计
//     - 将 InputTokens 映射到 PromptTokens，OutputTokens 映射到 CompletionTokens
//  4. 如果上游未返回 usage 信息，则使用估算的 prompt token 数作为 usage
//
// 参数:
//   - c: Gin 上下文
//   - resp: 上游 API 返回的 HTTP 响应
//   - info: 中继信息，包含估算的 prompt token 数
//   - responseFormat: 响应格式（用于日志记录等辅助用途）
//
// 返回:
//   - *types.NexusTokError: 处理过程中的错误
//   - *dto.Usage: token 使用量信息
func OpenaiSTTHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo, responseFormat string) (*types.NexusTokError, *dto.Usage) {
	defer service.CloseResponseBodyGracefully(resp)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError), nil
	}
	// 写入新的 response body
	service.IOCopyBytesGracefully(c, resp, responseBody)

	var responseData struct {
		Usage *dto.Usage `json:"usage"`
	}
	if err := common.Unmarshal(responseBody, &responseData); err == nil && responseData.Usage != nil {
		if responseData.Usage.TotalTokens > 0 {
			usage := responseData.Usage
			if usage.PromptTokens == 0 {
				usage.PromptTokens = usage.InputTokens
			}
			if usage.CompletionTokens == 0 {
				usage.CompletionTokens = usage.OutputTokens
			}
			return nil, usage
		}
	}

	usage := &dto.Usage{}
	usage.PromptTokens = info.GetEstimatePromptTokens()
	usage.CompletionTokens = 0
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	return nil, usage
}
