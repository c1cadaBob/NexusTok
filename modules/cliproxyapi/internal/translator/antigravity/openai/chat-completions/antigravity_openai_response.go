// chat_completions - antigravity_openai_response.go
// Antigravity 的 OpenAI Chat Completions 格式响应转换器。
// 负责将 Antigravity (Gemini CLI) API 的响应转换为 OpenAI Chat Completions 兼容的 JSON 格式。
// 支持流式和非流式两种模式。
//
// 转换特性：
// - 流式模式：处理 Gemini CLI 的响应结构（candidates、content.parts）
// - 工具调用：使用 SawToolCall 标记追踪工具调用，优先级高于 MAX_TOKENS
// - 推理内容：将 thought=true 的文本部分转换为 reasoning_content
// - 图片内联：将 inlineData 转换为 data: URL 格式的图片
// - 停止原因优先级：tool_calls > max_tokens > stop
// - 非流式模式：委托给 Gemini 的非流式转换器处理
package chat_completions

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/util"
	log "github.com/sirupsen/logrus"

	. "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/gemini/openai/chat-completions"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// convertCliResponseToOpenAIChatParams 保存 Antigravity 响应到 OpenAI 格式转换过程中的状态参数。
type convertCliResponseToOpenAIChatParams struct {
	// UnixTimestamp 响应创建时间的 Unix 时间戳
	UnixTimestamp int64
	// FunctionIndex 当前函数调用的索引
	FunctionIndex int
	// SawToolCall 标记在整个流中是否看到过工具调用（跨 chunk 持久化）
	SawToolCall bool
	// UpstreamFinishReason 缓存上游的停止原因，用于最终 chunk
	UpstreamFinishReason string
	// SanitizedNameMap 工具名称清理映射表，用于恢复原始工具名称
	SanitizedNameMap map[string]string
}

// functionCallIDCounter 提供进程范围内唯一的函数调用标识符计数器。
var functionCallIDCounter uint64

// ConvertAntigravityResponseToOpenAI 将 Antigravity 的流式响应转换为 OpenAI Chat Completions 流式格式。
//
// 处理的响应内容：
// - 文本内容（text -> content）
// - 推理内容（thought=true 的 text -> reasoning_content）
// - 工具调用（functionCall -> tool_calls）
// - 内联图片（inlineData -> data: URL 格式的 images）
// - 用量元数据（usageMetadata -> usage）
// - 停止原因（优先级：tool_calls > max_tokens > stop）
//
// 参数：
//   - ctx: 请求上下文（当前实现中未使用）
//   - modelName: 模型名称（当前实现中未使用）
//   - originalRequestRawJSON: 原始请求的 JSON 数据（用于工具名称恢复）
//   - rawJSON: Antigravity 格式的原始响应 JSON 数据
//   - param: 用于在多次调用之间保持状态的参数指针
//
// 返回值：
//   - [][]byte: OpenAI Chat Completions 格式的 SSE 事件数据切片
func ConvertAntigravityResponseToOpenAI(_ context.Context, _ string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) [][]byte {
	if *param == nil {
		*param = &convertCliResponseToOpenAIChatParams{
			UnixTimestamp:    0,
			FunctionIndex:    0,
			SanitizedNameMap: util.SanitizedToolNameMap(originalRequestRawJSON),
		}
	}
	if (*param).(*convertCliResponseToOpenAIChatParams).SanitizedNameMap == nil {
		(*param).(*convertCliResponseToOpenAIChatParams).SanitizedNameMap = util.SanitizedToolNameMap(originalRequestRawJSON)
	}

	if bytes.Equal(rawJSON, []byte("[DONE]")) {
		return [][]byte{}
	}

	// Initialize the OpenAI SSE template.
	template := []byte(`{"id":"","object":"chat.completion.chunk","created":12345,"model":"model","choices":[{"index":0,"delta":{"role":null,"content":null,"reasoning_content":null,"tool_calls":null},"finish_reason":null,"native_finish_reason":null}]}`)

	// Extract and set the model version.
	if modelVersionResult := gjson.GetBytes(rawJSON, "response.modelVersion"); modelVersionResult.Exists() {
		template, _ = sjson.SetBytes(template, "model", modelVersionResult.String())
	}

	// Extract and set the creation timestamp.
	if createTimeResult := gjson.GetBytes(rawJSON, "response.createTime"); createTimeResult.Exists() {
		t, err := time.Parse(time.RFC3339Nano, createTimeResult.String())
		if err == nil {
			(*param).(*convertCliResponseToOpenAIChatParams).UnixTimestamp = t.Unix()
		}
		template, _ = sjson.SetBytes(template, "created", (*param).(*convertCliResponseToOpenAIChatParams).UnixTimestamp)
	} else {
		template, _ = sjson.SetBytes(template, "created", (*param).(*convertCliResponseToOpenAIChatParams).UnixTimestamp)
	}

	// Extract and set the response ID.
	if responseIDResult := gjson.GetBytes(rawJSON, "response.responseId"); responseIDResult.Exists() {
		template, _ = sjson.SetBytes(template, "id", responseIDResult.String())
	}

	// Cache the finish reason - do NOT set it in output yet (will be set on final chunk)
	if finishReasonResult := gjson.GetBytes(rawJSON, "response.candidates.0.finishReason"); finishReasonResult.Exists() {
		(*param).(*convertCliResponseToOpenAIChatParams).UpstreamFinishReason = strings.ToUpper(finishReasonResult.String())
	}

	// Extract and set usage metadata (token counts).
	if usageResult := gjson.GetBytes(rawJSON, "response.usageMetadata"); usageResult.Exists() {
		cachedTokenCount := usageResult.Get("cachedContentTokenCount").Int()
		if candidatesTokenCountResult := usageResult.Get("candidatesTokenCount"); candidatesTokenCountResult.Exists() {
			template, _ = sjson.SetBytes(template, "usage.completion_tokens", candidatesTokenCountResult.Int())
		}
		if totalTokenCountResult := usageResult.Get("totalTokenCount"); totalTokenCountResult.Exists() {
			template, _ = sjson.SetBytes(template, "usage.total_tokens", totalTokenCountResult.Int())
		}
		promptTokenCount := usageResult.Get("promptTokenCount").Int()
		thoughtsTokenCount := usageResult.Get("thoughtsTokenCount").Int()
		template, _ = sjson.SetBytes(template, "usage.prompt_tokens", promptTokenCount)
		if thoughtsTokenCount > 0 {
			template, _ = sjson.SetBytes(template, "usage.completion_tokens_details.reasoning_tokens", thoughtsTokenCount)
		}
		// Include cached token count if present (indicates prompt caching is working)
		if cachedTokenCount > 0 {
			var err error
			template, err = sjson.SetBytes(template, "usage.prompt_tokens_details.cached_tokens", cachedTokenCount)
			if err != nil {
				log.Warnf("antigravity openai response: failed to set cached_tokens: %v", err)
			}
		}
	}

	// Process the main content part of the response.
	partsResult := gjson.GetBytes(rawJSON, "response.candidates.0.content.parts")
	if partsResult.IsArray() {
		partResults := partsResult.Array()
		for i := 0; i < len(partResults); i++ {
			partResult := partResults[i]
			partTextResult := partResult.Get("text")
			functionCallResult := partResult.Get("functionCall")
			thoughtSignatureResult := partResult.Get("thoughtSignature")
			if !thoughtSignatureResult.Exists() {
				thoughtSignatureResult = partResult.Get("thought_signature")
			}
			inlineDataResult := partResult.Get("inlineData")
			if !inlineDataResult.Exists() {
				inlineDataResult = partResult.Get("inline_data")
			}

			hasThoughtSignature := thoughtSignatureResult.Exists() && thoughtSignatureResult.String() != ""
			hasContentPayload := partTextResult.Exists() || functionCallResult.Exists() || inlineDataResult.Exists()

			// Ignore encrypted thoughtSignature but keep any actual content in the same part.
			if hasThoughtSignature && !hasContentPayload {
				continue
			}

			if partTextResult.Exists() {
				textContent := partTextResult.String()

				// Handle text content, distinguishing between regular content and reasoning/thoughts.
				if partResult.Get("thought").Bool() {
					template, _ = sjson.SetBytes(template, "choices.0.delta.reasoning_content", textContent)
				} else {
					template, _ = sjson.SetBytes(template, "choices.0.delta.content", textContent)
				}
				template, _ = sjson.SetBytes(template, "choices.0.delta.role", "assistant")
			} else if functionCallResult.Exists() {
				// Handle function call content.
				(*param).(*convertCliResponseToOpenAIChatParams).SawToolCall = true // Persist across chunks
				toolCallsResult := gjson.GetBytes(template, "choices.0.delta.tool_calls")
				functionCallIndex := (*param).(*convertCliResponseToOpenAIChatParams).FunctionIndex
				(*param).(*convertCliResponseToOpenAIChatParams).FunctionIndex++
				if toolCallsResult.Exists() && toolCallsResult.IsArray() {
					functionCallIndex = len(toolCallsResult.Array())
				} else {
					template, _ = sjson.SetRawBytes(template, "choices.0.delta.tool_calls", []byte(`[]`))
				}

				functionCallTemplate := []byte(`{"id": "","index": 0,"type": "function","function": {"name": "","arguments": ""}}`)
				fcName := util.RestoreSanitizedToolName((*param).(*convertCliResponseToOpenAIChatParams).SanitizedNameMap, functionCallResult.Get("name").String())
				functionCallTemplate, _ = sjson.SetBytes(functionCallTemplate, "id", fmt.Sprintf("%s-%d-%d", fcName, time.Now().UnixNano(), atomic.AddUint64(&functionCallIDCounter, 1)))
				functionCallTemplate, _ = sjson.SetBytes(functionCallTemplate, "index", functionCallIndex)
				functionCallTemplate, _ = sjson.SetBytes(functionCallTemplate, "function.name", fcName)
				if fcArgsResult := functionCallResult.Get("args"); fcArgsResult.Exists() {
					functionCallTemplate, _ = sjson.SetBytes(functionCallTemplate, "function.arguments", fcArgsResult.Raw)
				}
				template, _ = sjson.SetBytes(template, "choices.0.delta.role", "assistant")
				template, _ = sjson.SetRawBytes(template, "choices.0.delta.tool_calls.-1", functionCallTemplate)
			} else if inlineDataResult.Exists() {
				data := inlineDataResult.Get("data").String()
				if data == "" {
					continue
				}
				mimeType := inlineDataResult.Get("mimeType").String()
				if mimeType == "" {
					mimeType = inlineDataResult.Get("mime_type").String()
				}
				if mimeType == "" {
					mimeType = "image/png"
				}
				imageURL := fmt.Sprintf("data:%s;base64,%s", mimeType, data)
				imagesResult := gjson.GetBytes(template, "choices.0.delta.images")
				if !imagesResult.Exists() || !imagesResult.IsArray() {
					template, _ = sjson.SetRawBytes(template, "choices.0.delta.images", []byte(`[]`))
				}
				imageIndex := len(gjson.GetBytes(template, "choices.0.delta.images").Array())
				imagePayload := []byte(`{"type":"image_url","image_url":{"url":""}}`)
				imagePayload, _ = sjson.SetBytes(imagePayload, "index", imageIndex)
				imagePayload, _ = sjson.SetBytes(imagePayload, "image_url.url", imageURL)
				template, _ = sjson.SetBytes(template, "choices.0.delta.role", "assistant")
				template, _ = sjson.SetRawBytes(template, "choices.0.delta.images.-1", imagePayload)
			}
		}
	}

	// Determine finish_reason only on the final chunk (has both finishReason and usage metadata)
	params := (*param).(*convertCliResponseToOpenAIChatParams)
	upstreamFinishReason := params.UpstreamFinishReason
	sawToolCall := params.SawToolCall

	usageExists := gjson.GetBytes(rawJSON, "response.usageMetadata").Exists()
	isFinalChunk := upstreamFinishReason != "" && usageExists

	if isFinalChunk {
		var finishReason string
		if sawToolCall {
			finishReason = "tool_calls"
		} else if upstreamFinishReason == "MAX_TOKENS" {
			finishReason = "max_tokens"
		} else {
			finishReason = "stop"
		}
		template, _ = sjson.SetBytes(template, "choices.0.finish_reason", finishReason)
		template, _ = sjson.SetBytes(template, "choices.0.native_finish_reason", strings.ToLower(upstreamFinishReason))
	}

	return [][]byte{template}
}

// ConvertAntigravityResponseToOpenAINonStream 将 Antigravity 的非流式响应转换为 OpenAI Chat Completions 格式。
// 从 Antigravity 的 "response" 包装中提取 Gemini 响应，然后委托给 Gemini 的非流式转换器处理。
//
// 参数：
//   - ctx: 请求上下文
//   - modelName: 模型名称
//   - originalRequestRawJSON: 原始请求的 JSON 数据
//   - requestRawJSON: 经过转换的请求 JSON 数据
//   - rawJSON: Antigravity 格式的原始响应数据（包含 "response" 包装）
//   - param: 用于在多次调用之间保持状态的参数指针
//
// 返回值：
//   - []byte: OpenAI Chat Completions 格式的完整 JSON 响应数据
func ConvertAntigravityResponseToOpenAINonStream(ctx context.Context, modelName string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) []byte {
	responseResult := gjson.GetBytes(rawJSON, "response")
	if responseResult.Exists() {
		return ConvertGeminiResponseToOpenAINonStream(ctx, modelName, originalRequestRawJSON, requestRawJSON, []byte(responseResult.Raw), param)
	}
	return []byte{}
}
