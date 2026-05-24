// chat_completions - gemini_openai_response.go
// Gemini 的 OpenAI Chat Completions 响应转换器。
// 负责将 Gemini API 的响应转换为 OpenAI Chat Completions 兼容的 JSON 格式。
// 支持流式和非流式两种模式。
//
// 转换特性：
// - 流式模式：增量输出 SSE 事件，支持多候选（candidate_count > 1）
// - 非流式模式：聚合所有内容到单个响应对象
// - 工具调用：使用进程级唯一计数器生成函数调用 ID，支持多候选的独立索引
// - 图片处理：将内联数据转换为 data: URL 格式
// - 推理内容：区分思维内容（reasoning_content）和正文内容（content）
// - 用量元数据：映射 Gemini 的 usageMetadata 到 OpenAI 的 usage 格式
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
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// convertGeminiResponseToOpenAIChatParams 保存流式响应转换过程中需要保持的状态参数。
type convertGeminiResponseToOpenAIChatParams struct {
	// UnixTimestamp 响应创建时间的 Unix 时间戳
	UnixTimestamp int64
	// FunctionIndex 按候选索引跟踪工具调用索引，支持多候选场景
	FunctionIndex map[int]int
	// SanitizedNameMap 工具名称清理映射表，用于还原被清理过的函数名称
	SanitizedNameMap map[string]string
}

// functionCallIDCounter 提供进程级别的唯一函数调用标识符计数器。
// 使用原子操作确保在并发场景下的线程安全性。
var functionCallIDCounter uint64

// ConvertGeminiResponseToOpenAI 将 Gemini API 的流式响应转换为 OpenAI Chat Completions 流式格式。
// 支持多候选（candidate_count > 1），每个候选独立生成一个 SSE 事件。
//
// 处理流程：
// 1. 初始化或恢复流式转换状态
// 2. 提取模型版本、创建时间和响应 ID
// 3. 映射用量元数据（token 计数）
// 4. 遍历所有候选，为每个候选独立处理内容部分
// 5. 区分文本、函数调用和内联数据，生成对应的 SSE 事件
// 6. 设置完成原因（tool_calls 或 stop/max_tokens）
//
// 参数：
//   - ctx: 请求上下文（当前实现中未使用）
//   - modelName: 模型名称（当前实现中未使用）
//   - originalRequestRawJSON: 原始请求的 JSON 数据，用于获取工具名称映射
//   - requestRawJSON: 经过转换的请求 JSON 数据
//   - rawJSON: Gemini 格式的原始响应 JSON 数据
//   - param: 用于在多次调用之间保持状态的参数指针
//
// 返回值：
//   - [][]byte: OpenAI Chat Completions 格式的 SSE 事件数据切片
func ConvertGeminiResponseToOpenAI(_ context.Context, _ string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) [][]byte {
	// Initialize parameters if nil.
	if *param == nil {
		*param = &convertGeminiResponseToOpenAIChatParams{
			UnixTimestamp:    0,
			FunctionIndex:    make(map[int]int),
			SanitizedNameMap: util.SanitizedToolNameMap(originalRequestRawJSON),
		}
	}

	// Ensure the Map is initialized (handling cases where param might be reused from older context).
	p := (*param).(*convertGeminiResponseToOpenAIChatParams)
	if p.FunctionIndex == nil {
		p.FunctionIndex = make(map[int]int)
	}
	if p.SanitizedNameMap == nil {
		p.SanitizedNameMap = util.SanitizedToolNameMap(originalRequestRawJSON)
	}

	if bytes.HasPrefix(rawJSON, []byte("data:")) {
		rawJSON = bytes.TrimSpace(rawJSON[5:])
	}

	if bytes.Equal(rawJSON, []byte("[DONE]")) {
		return [][]byte{}
	}

	// Initialize the OpenAI SSE base template.
	// We use a base template and clone it for each candidate to support multiple candidates.
	baseTemplate := []byte(`{"id":"","object":"chat.completion.chunk","created":12345,"model":"model","choices":[{"index":0,"delta":{"role":null,"content":null,"reasoning_content":null,"tool_calls":null},"finish_reason":null,"native_finish_reason":null}]}`)

	// Extract and set the model version.
	if modelVersionResult := gjson.GetBytes(rawJSON, "modelVersion"); modelVersionResult.Exists() {
		baseTemplate, _ = sjson.SetBytes(baseTemplate, "model", modelVersionResult.String())
	}

	// Extract and set the creation timestamp.
	if createTimeResult := gjson.GetBytes(rawJSON, "createTime"); createTimeResult.Exists() {
		t, err := time.Parse(time.RFC3339Nano, createTimeResult.String())
		if err == nil {
			p.UnixTimestamp = t.Unix()
		}
		baseTemplate, _ = sjson.SetBytes(baseTemplate, "created", p.UnixTimestamp)
	} else {
		baseTemplate, _ = sjson.SetBytes(baseTemplate, "created", p.UnixTimestamp)
	}

	// Extract and set the response ID.
	if responseIDResult := gjson.GetBytes(rawJSON, "responseId"); responseIDResult.Exists() {
		baseTemplate, _ = sjson.SetBytes(baseTemplate, "id", responseIDResult.String())
	}

	// Extract and set usage metadata (token counts).
	// Usage is applied to the base template so it appears in the chunks.
	if usageResult := gjson.GetBytes(rawJSON, "usageMetadata"); usageResult.Exists() {
		cachedTokenCount := usageResult.Get("cachedContentTokenCount").Int()
		if candidatesTokenCountResult := usageResult.Get("candidatesTokenCount"); candidatesTokenCountResult.Exists() {
			baseTemplate, _ = sjson.SetBytes(baseTemplate, "usage.completion_tokens", candidatesTokenCountResult.Int())
		}
		if totalTokenCountResult := usageResult.Get("totalTokenCount"); totalTokenCountResult.Exists() {
			baseTemplate, _ = sjson.SetBytes(baseTemplate, "usage.total_tokens", totalTokenCountResult.Int())
		}
		promptTokenCount := usageResult.Get("promptTokenCount").Int()
		thoughtsTokenCount := usageResult.Get("thoughtsTokenCount").Int()
		baseTemplate, _ = sjson.SetBytes(baseTemplate, "usage.prompt_tokens", promptTokenCount)
		if thoughtsTokenCount > 0 {
			baseTemplate, _ = sjson.SetBytes(baseTemplate, "usage.completion_tokens_details.reasoning_tokens", thoughtsTokenCount)
		}
		// Include cached token count if present (indicates prompt caching is working)
		if cachedTokenCount > 0 {
			var err error
			baseTemplate, err = sjson.SetBytes(baseTemplate, "usage.prompt_tokens_details.cached_tokens", cachedTokenCount)
			if err != nil {
				log.Warnf("gemini openai response: failed to set cached_tokens in streaming: %v", err)
			}
		}
	}

	var responseStrings [][]byte
	candidates := gjson.GetBytes(rawJSON, "candidates")

	// Iterate over all candidates to support candidate_count > 1.
	if candidates.IsArray() {
		candidates.ForEach(func(_, candidate gjson.Result) bool {
			// Clone the template for the current candidate.
			template := append([]byte(nil), baseTemplate...)

			// Set the specific index for this candidate.
			candidateIndex := int(candidate.Get("index").Int())
			template, _ = sjson.SetBytes(template, "choices.0.index", candidateIndex)

			finishReason := ""
			if stopReasonResult := gjson.GetBytes(rawJSON, "stop_reason"); stopReasonResult.Exists() {
				finishReason = stopReasonResult.String()
			}
			if finishReason == "" {
				if finishReasonResult := gjson.GetBytes(rawJSON, "candidates.0.finishReason"); finishReasonResult.Exists() {
					finishReason = finishReasonResult.String()
				}
			}
			finishReason = strings.ToLower(finishReason)

			partsResult := candidate.Get("content.parts")
			hasFunctionCall := false

			if partsResult.IsArray() {
				partResults := partsResult.Array()
				for i := 0; i < len(partResults); i++ {
					partResult := partResults[i]
					partTextResult := partResult.Get("text")
					functionCallResult := partResult.Get("functionCall")
					inlineDataResult := partResult.Get("inlineData")
					if !inlineDataResult.Exists() {
						inlineDataResult = partResult.Get("inline_data")
					}
					thoughtSignatureResult := partResult.Get("thoughtSignature")
					if !thoughtSignatureResult.Exists() {
						thoughtSignatureResult = partResult.Get("thought_signature")
					}

					hasThoughtSignature := thoughtSignatureResult.Exists() && thoughtSignatureResult.String() != ""
					hasContentPayload := partTextResult.Exists() || functionCallResult.Exists() || inlineDataResult.Exists()

					// Skip pure thoughtSignature parts but keep any actual payload in the same part.
					if hasThoughtSignature && !hasContentPayload {
						continue
					}

					if partTextResult.Exists() {
						text := partTextResult.String()
						// Handle text content, distinguishing between regular content and reasoning/thoughts.
						if partResult.Get("thought").Bool() {
							template, _ = sjson.SetBytes(template, "choices.0.delta.reasoning_content", text)
						} else {
							template, _ = sjson.SetBytes(template, "choices.0.delta.content", text)
						}
						template, _ = sjson.SetBytes(template, "choices.0.delta.role", "assistant")
					} else if functionCallResult.Exists() {
						// Handle function call content.
						hasFunctionCall = true
						toolCallsResult := gjson.GetBytes(template, "choices.0.delta.tool_calls")

						// Retrieve the function index for this specific candidate.
						functionCallIndex := p.FunctionIndex[candidateIndex]
						p.FunctionIndex[candidateIndex]++

						if toolCallsResult.Exists() && toolCallsResult.IsArray() {
							functionCallIndex = len(toolCallsResult.Array())
						} else {
							template, _ = sjson.SetRawBytes(template, "choices.0.delta.tool_calls", []byte(`[]`))
						}

						functionCallTemplate := []byte(`{"id":"","index":0,"type":"function","function":{"name":"","arguments":""}}`)
						fcName := util.RestoreSanitizedToolName(p.SanitizedNameMap, functionCallResult.Get("name").String())
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
			} else if finishReason != "" {
				// Only pass through specific finish reasons
				if finishReason == "max_tokens" || finishReason == "stop" {
					template, _ = sjson.SetBytes(template, "choices.0.finish_reason", finishReason)
					template, _ = sjson.SetBytes(template, "choices.0.native_finish_reason", finishReason)
				}
			}

			responseStrings = append(responseStrings, template)
			return true // continue loop
		})
	} else {
		// If there are no candidates (e.g., a pure usageMetadata chunk), return the usage chunk if present.
		if gjson.GetBytes(rawJSON, "usageMetadata").Exists() && len(responseStrings) == 0 {
			responseStrings = append(responseStrings, append([]byte(nil), baseTemplate...))
		}
	}

	return responseStrings
}

// ConvertGeminiResponseToOpenAINonStream 将 Gemini 的非流式响应转换为 OpenAI Chat Completions 格式。
// 处理完整的 Gemini 响应，支持多候选，将所有内容聚合到单个 OpenAI 兼容的 JSON 响应中。
//
// 参数：
//   - ctx: 请求上下文（当前实现中未使用）
//   - modelName: 模型名称（当前实现中未使用）
//   - originalRequestRawJSON: 原始请求的 JSON 数据，用于获取工具名称映射
//   - requestRawJSON: 经过转换的请求 JSON 数据
//   - rawJSON: Gemini 格式的原始响应 JSON 数据
//   - _: 未使用的参数指针
//
// 返回值：
//   - []byte: OpenAI Chat Completions 格式的完整 JSON 响应数据
func ConvertGeminiResponseToOpenAINonStream(_ context.Context, _ string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, _ *any) []byte {
	sanitizedNameMap := util.SanitizedToolNameMap(originalRequestRawJSON)
	var unixTimestamp int64
	// Initialize template with an empty choices array to support multiple candidates.
	template := []byte(`{"id":"","object":"chat.completion","created":123456,"model":"model","choices":[]}`)

	if modelVersionResult := gjson.GetBytes(rawJSON, "modelVersion"); modelVersionResult.Exists() {
		template, _ = sjson.SetBytes(template, "model", modelVersionResult.String())
	}

	if createTimeResult := gjson.GetBytes(rawJSON, "createTime"); createTimeResult.Exists() {
		t, err := time.Parse(time.RFC3339Nano, createTimeResult.String())
		if err == nil {
			unixTimestamp = t.Unix()
		}
		template, _ = sjson.SetBytes(template, "created", unixTimestamp)
	} else {
		template, _ = sjson.SetBytes(template, "created", unixTimestamp)
	}

	if responseIDResult := gjson.GetBytes(rawJSON, "responseId"); responseIDResult.Exists() {
		template, _ = sjson.SetBytes(template, "id", responseIDResult.String())
	}

	if usageResult := gjson.GetBytes(rawJSON, "usageMetadata"); usageResult.Exists() {
		if candidatesTokenCountResult := usageResult.Get("candidatesTokenCount"); candidatesTokenCountResult.Exists() {
			template, _ = sjson.SetBytes(template, "usage.completion_tokens", candidatesTokenCountResult.Int())
		}
		if totalTokenCountResult := usageResult.Get("totalTokenCount"); totalTokenCountResult.Exists() {
			template, _ = sjson.SetBytes(template, "usage.total_tokens", totalTokenCountResult.Int())
		}
		promptTokenCount := usageResult.Get("promptTokenCount").Int()
		thoughtsTokenCount := usageResult.Get("thoughtsTokenCount").Int()
		cachedTokenCount := usageResult.Get("cachedContentTokenCount").Int()
		template, _ = sjson.SetBytes(template, "usage.prompt_tokens", promptTokenCount)
		if thoughtsTokenCount > 0 {
			template, _ = sjson.SetBytes(template, "usage.completion_tokens_details.reasoning_tokens", thoughtsTokenCount)
		}
		// Include cached token count if present (indicates prompt caching is working)
		if cachedTokenCount > 0 {
			var err error
			template, err = sjson.SetBytes(template, "usage.prompt_tokens_details.cached_tokens", cachedTokenCount)
			if err != nil {
				log.Warnf("gemini openai response: failed to set cached_tokens in non-streaming: %v", err)
			}
		}
	}

	// Process the main content part of the response for all candidates.
	candidates := gjson.GetBytes(rawJSON, "candidates")
	if candidates.IsArray() {
		candidates.ForEach(func(_, candidate gjson.Result) bool {
			// Construct a single Choice object.
			choiceTemplate := []byte(`{"index":0,"message":{"role":"assistant","content":null,"reasoning_content":null,"tool_calls":null},"finish_reason":null,"native_finish_reason":null}`)

			// Set the index for this choice.
			choiceTemplate, _ = sjson.SetBytes(choiceTemplate, "index", candidate.Get("index").Int())

			// Set finish reason.
			if finishReasonResult := candidate.Get("finishReason"); finishReasonResult.Exists() {
				choiceTemplate, _ = sjson.SetBytes(choiceTemplate, "finish_reason", strings.ToLower(finishReasonResult.String()))
				choiceTemplate, _ = sjson.SetBytes(choiceTemplate, "native_finish_reason", strings.ToLower(finishReasonResult.String()))
			}

			partsResult := candidate.Get("content.parts")
			hasFunctionCall := false
			if partsResult.IsArray() {
				partsResults := partsResult.Array()
				for i := 0; i < len(partsResults); i++ {
					partResult := partsResults[i]
					partTextResult := partResult.Get("text")
					functionCallResult := partResult.Get("functionCall")
					inlineDataResult := partResult.Get("inlineData")
					if !inlineDataResult.Exists() {
						inlineDataResult = partResult.Get("inline_data")
					}

					if partTextResult.Exists() {
						// Append text content, distinguishing between regular content and reasoning.
						if partResult.Get("thought").Bool() {
							oldVal := gjson.GetBytes(choiceTemplate, "message.reasoning_content").String()
							choiceTemplate, _ = sjson.SetBytes(choiceTemplate, "message.reasoning_content", oldVal+partTextResult.String())
						} else {
							oldVal := gjson.GetBytes(choiceTemplate, "message.content").String()
							choiceTemplate, _ = sjson.SetBytes(choiceTemplate, "message.content", oldVal+partTextResult.String())
						}
						choiceTemplate, _ = sjson.SetBytes(choiceTemplate, "message.role", "assistant")
					} else if functionCallResult.Exists() {
						// Append function call content to the tool_calls array.
						hasFunctionCall = true
						toolCallsResult := gjson.GetBytes(choiceTemplate, "message.tool_calls")
						if !toolCallsResult.Exists() || !toolCallsResult.IsArray() {
							choiceTemplate, _ = sjson.SetRawBytes(choiceTemplate, "message.tool_calls", []byte(`[]`))
						}
						functionCallItemTemplate := []byte(`{"id":"","type":"function","function":{"name":"","arguments":""}}`)
						fcName := util.RestoreSanitizedToolName(sanitizedNameMap, functionCallResult.Get("name").String())
						functionCallItemTemplate, _ = sjson.SetBytes(functionCallItemTemplate, "id", fmt.Sprintf("%s-%d-%d", fcName, time.Now().UnixNano(), atomic.AddUint64(&functionCallIDCounter, 1)))
						functionCallItemTemplate, _ = sjson.SetBytes(functionCallItemTemplate, "function.name", fcName)
						if fcArgsResult := functionCallResult.Get("args"); fcArgsResult.Exists() {
							functionCallItemTemplate, _ = sjson.SetBytes(functionCallItemTemplate, "function.arguments", fcArgsResult.Raw)
						}
						choiceTemplate, _ = sjson.SetBytes(choiceTemplate, "message.role", "assistant")
						choiceTemplate, _ = sjson.SetRawBytes(choiceTemplate, "message.tool_calls.-1", functionCallItemTemplate)
					} else if inlineDataResult.Exists() {
						data := inlineDataResult.Get("data").String()
						if data != "" {
							mimeType := inlineDataResult.Get("mimeType").String()
							if mimeType == "" {
								mimeType = inlineDataResult.Get("mime_type").String()
							}
							if mimeType == "" {
								mimeType = "image/png"
							}
							imageURL := fmt.Sprintf("data:%s;base64,%s", mimeType, data)
							imagesResult := gjson.GetBytes(choiceTemplate, "message.images")
							if !imagesResult.Exists() || !imagesResult.IsArray() {
								choiceTemplate, _ = sjson.SetRawBytes(choiceTemplate, "message.images", []byte(`[]`))
							}
							imageIndex := len(gjson.GetBytes(choiceTemplate, "message.images").Array())
							imagePayload := []byte(`{"type":"image_url","image_url":{"url":""}}`)
							imagePayload, _ = sjson.SetBytes(imagePayload, "index", imageIndex)
							imagePayload, _ = sjson.SetBytes(imagePayload, "image_url.url", imageURL)
							choiceTemplate, _ = sjson.SetBytes(choiceTemplate, "message.role", "assistant")
							choiceTemplate, _ = sjson.SetRawBytes(choiceTemplate, "message.images.-1", imagePayload)
						}
					}
				}
			}

			if hasFunctionCall {
				choiceTemplate, _ = sjson.SetBytes(choiceTemplate, "finish_reason", "tool_calls")
				choiceTemplate, _ = sjson.SetBytes(choiceTemplate, "native_finish_reason", "tool_calls")
			}

			// Append the constructed choice to the main choices array.
			template, _ = sjson.SetRawBytes(template, "choices.-1", choiceTemplate)
			return true
		})
	}

	return template
}
