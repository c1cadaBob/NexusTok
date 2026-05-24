// chat_completions - gemini-cli_openai_response.go
// Gemini CLI 的 OpenAI Chat Completions 响应转换器。
// 负责将 Gemini CLI API 的响应转换为 OpenAI Chat Completions 兼容的 JSON 格式。
// 支持流式和非流式两种模式，处理文本内容、工具调用、推理内容和用量元数据。
//
// 转换特性：
// - 流式模式：增量输出 SSE 事件，每个事件包含部分响应数据
// - 非流式模式：委托给 Gemini 标准的非流式转换器
// - 工具调用：使用进程级唯一计数器生成函数调用 ID
// - 图片处理：将内联数据转换为 data: URL 格式
// - 推理内容：区分思维内容（reasoning_content）和正文内容（content）
package chat_completions

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	. "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/gemini/openai/chat-completions"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/util"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// convertCliResponseToOpenAIChatParams 保存流式响应转换过程中需要保持的状态参数。
type convertCliResponseToOpenAIChatParams struct {
	// UnixTimestamp 响应创建时间的 Unix 时间戳，用于填充 OpenAI 响应的 created 字段
	UnixTimestamp int64
	// FunctionIndex 当前函数调用的索引计数，用于为多个工具调用分配递增的索引
	FunctionIndex int
	// SanitizedNameMap 工具名称清理映射表，用于还原被清理过的函数名称
	SanitizedNameMap map[string]string
}

// functionCallIDCounter 提供进程级别的唯一函数调用标识符计数器。
// 使用原子操作确保在并发场景下的线程安全性。
var functionCallIDCounter uint64

// ConvertCliResponseToOpenAI 将 Gemini CLI API 的流式响应转换为 OpenAI Chat Completions 流式格式。
// 该函数处理各种 Gemini CLI 事件类型，将它们转换为 OpenAI 兼容的 JSON 响应。
// 支持文本内容、工具调用、推理内容和用量元数据的增量更新。
//
// 处理流程：
// 1. 初始化或恢复流式转换状态（参数对象）
// 2. 提取模型版本、创建时间和响应 ID
// 3. 提取并映射用量元数据（token 计数）
// 4. 遍历内容部分，区分文本、函数调用和内联数据
// 5. 根据是否有函数调用设置完成原因
//
// 参数：
//   - ctx: 请求上下文（当前实现中未使用）
//   - modelName: 模型名称（当前实现中未使用）
//   - originalRequestRawJSON: 原始请求的 JSON 数据，用于获取工具名称映射
//   - requestRawJSON: 经过转换的请求 JSON 数据
//   - rawJSON: Gemini CLI 格式的原始响应 JSON 数据
//   - param: 用于在多次调用之间保持状态的参数指针
//
// 返回值：
//   - [][]byte: OpenAI Chat Completions 格式的 SSE 事件数据切片
func ConvertCliResponseToOpenAI(_ context.Context, _ string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) [][]byte {
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

	finishReason := ""
	if stopReasonResult := gjson.GetBytes(rawJSON, "response.stop_reason"); stopReasonResult.Exists() {
		finishReason = stopReasonResult.String()
	}
	if finishReason == "" {
		if finishReasonResult := gjson.GetBytes(rawJSON, "response.candidates.0.finishReason"); finishReasonResult.Exists() {
			finishReason = finishReasonResult.String()
		}
	}
	finishReason = strings.ToLower(finishReason)

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
				log.Warnf("gemini-cli openai response: failed to set cached_tokens: %v", err)
			}
		}
	}

	// Process the main content part of the response.
	partsResult := gjson.GetBytes(rawJSON, "response.candidates.0.content.parts")
	hasFunctionCall := false
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
				hasFunctionCall = true
				toolCallsResult := gjson.GetBytes(template, "choices.0.delta.tool_calls")
				functionCallIndex := (*param).(*convertCliResponseToOpenAIChatParams).FunctionIndex
				(*param).(*convertCliResponseToOpenAIChatParams).FunctionIndex++
				if toolCallsResult.Exists() && toolCallsResult.IsArray() {
					functionCallIndex = len(toolCallsResult.Array())
				} else {
					template, _ = sjson.SetRawBytes(template, "choices.0.delta.tool_calls", []byte(`[]`))
				}

				functionCallTemplate := []byte(`{"id":"","index":0,"type":"function","function":{"name":"","arguments":""}}`)
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

	if hasFunctionCall {
		template, _ = sjson.SetBytes(template, "choices.0.finish_reason", "tool_calls")
		template, _ = sjson.SetBytes(template, "choices.0.native_finish_reason", "tool_calls")
	} else if finishReason != "" && (*param).(*convertCliResponseToOpenAIChatParams).FunctionIndex == 0 {
		// Only pass through specific finish reasons
		if finishReason == "max_tokens" || finishReason == "stop" {
			template, _ = sjson.SetBytes(template, "choices.0.finish_reason", finishReason)
			template, _ = sjson.SetBytes(template, "choices.0.native_finish_reason", finishReason)
		}
	}

	return [][]byte{template}
}

// ConvertCliResponseToOpenAINonStream 将 Gemini CLI 的非流式响应转换为 OpenAI Chat Completions 格式。
// 该函数首先检查响应中是否包含 "response" 封装层（Gemini CLI 特有结构），
// 如果存在则提取内部响应数据，然后委托给 Gemini 标准的非流式转换器完成转换。
//
// 参数：
//   - ctx: 请求上下文，用于取消和超时处理
//   - modelName: 模型名称
//   - originalRequestRawJSON: 原始请求的 JSON 数据
//   - requestRawJSON: 经过转换的请求 JSON 数据
//   - rawJSON: Gemini CLI 格式的原始响应 JSON 数据
//   - param: 用于转换过程中的参数指针
//
// 返回值：
//   - []byte: OpenAI Chat Completions 格式的完整 JSON 响应数据
func ConvertCliResponseToOpenAINonStream(ctx context.Context, modelName string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) []byte {
	responseResult := gjson.GetBytes(rawJSON, "response")
	if responseResult.Exists() {
		return ConvertGeminiResponseToOpenAINonStream(ctx, modelName, originalRequestRawJSON, requestRawJSON, []byte(responseResult.Raw), param)
	}
	return []byte{}
}
