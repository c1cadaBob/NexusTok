// antigravity/claude - antigravity_claude_response.go
// Package claude provides response translation functionality for Claude Code API compatibility.
// 本文件提供 Antigravity（Gemini CLI）API 响应到 Claude Code API 格式的转换功能。
// 实现了复杂的状态机，将后端客户端响应转换为 Claude Code 兼容的 Server-Sent Events (SSE) 格式。
// 支持文本内容、思考过程（thinking + thoughtSignature）和函数调用（functionCall）等不同响应类型的处理，
// 维护跨多个响应块的状态信息以确保正确的 SSE 事件排序。
// 响应类型状态：0=无, 1=内容, 2=思考, 3=函数调用。
// 包含签名缓存支持，用于在流式响应中累积思考文本并缓存对应的签名。
package claude

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/cache"
	translatorcommon "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/common"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/util"
	log "github.com/sirupsen/logrus"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// decodeSignature 将双层 Base64 编码的签名（R 前缀）解码为单层 Base64（E 前缀，Anthropic 格式）。
// 解码失败时返回空字符串（跳过无效签名）。
func decodeSignature(signature string) string {
	if signature == "" {
		return signature
	}
	if strings.HasPrefix(signature, "R") {
		decoded, err := base64.StdEncoding.DecodeString(signature)
		if err != nil {
			log.Warnf("antigravity claude response: failed to decode signature, skipping")
			return ""
		}
		return string(decoded)
	}
	return signature
}

// formatClaudeSignatureValue 格式化 Claude 签名值。
// 缓存模式下添加模型组前缀；Claude 模型组下解码双层签名。
func formatClaudeSignatureValue(modelName, signature string) string {
	if cache.SignatureCacheEnabled() {
		return fmt.Sprintf("%s#%s", cache.GetModelGroup(modelName), signature)
	}
	if cache.GetModelGroup(modelName) == "claude" {
		return decodeSignature(signature)
	}
	return signature
}

// Params 持有响应转换的状态参数和签名缓存支持。
// 跟踪当前响应翻译过程的状态，确保 SSE 事件的正确排序和不同内容类型之间的状态转换。
type Params struct {
	HasFirstResponse     bool              // 是否已发送初始 message_start 事件
	ResponseType         int               // 当前响应类型：0=无, 1=内容, 2=思考, 3=函数调用
	ResponseIndex        int               // 流式响应中内容块的索引计数器
	HasFinishReason      bool              // 是否已观察到完成原因
	FinishReason         string            // 提供者返回的完成原因字符串
	HasUsageMetadata     bool              // 是否已观察到使用量元数据
	PromptTokenCount     int64             // 缓存的提示 Token 计数
	CandidatesTokenCount int64             // 缓存的候选 Token 计数
	ThoughtsTokenCount   int64             // 缓存的思考 Token 计数
	TotalTokenCount      int64             // 缓存的总 Token 计数
	CachedTokenCount     int64             // 缓存的内容 Token 计数（表示提示缓存）
	HasSentFinalEvents   bool              // 是否已发送最终内容/消息事件
	HasToolUse           bool              // 是否在流中观察到工具使用
	HasContent           bool              // 跟踪是否已输出任何内容

	// 签名缓存支持
	CurrentThinkingText strings.Builder   // 累积思考文本用于签名缓存

	// 反向映射：清理后的 Gemini 函数名 → 原始 Claude 工具名。
	// 在第一个响应块中从原始请求 JSON 延迟初始化。
	ToolNameMap map[string]string
}

// toolUseIDCounter 提供进程范围内的唯一工具使用标识符计数器。
var toolUseIDCounter uint64

// ConvertAntigravityResponseToClaude performs sophisticated streaming response format conversion.
// This function implements a complex state machine that translates backend client responses
// into Claude Code-compatible Server-Sent Events (SSE) format. It manages different response types
// and handles state transitions between content blocks, thinking processes, and function calls.
//
// Response type states: 0=none, 1=content, 2=thinking, 3=function
// The function maintains state across multiple calls to ensure proper SSE event sequencing.
//
// Parameters:
//   - ctx: The context for the request, used for cancellation and timeout handling
//   - modelName: The name of the model being used for the response (unused in current implementation)
//   - rawJSON: The raw JSON response from the Gemini CLI API
//   - param: A pointer to a parameter object for maintaining state between calls
//
// Returns:
//   - [][]byte: A slice of bytes, each containing a Claude Code-compatible SSE payload.
func ConvertAntigravityResponseToClaude(_ context.Context, _ string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) [][]byte {
	if *param == nil {
		*param = &Params{
			HasFirstResponse: false,
			ResponseType:     0,
			ResponseIndex:    0,
			ToolNameMap:      util.SanitizedToolNameMap(originalRequestRawJSON),
		}
	}
	modelName := gjson.GetBytes(requestRawJSON, "model").String()

	params := (*param).(*Params)

	if bytes.Equal(rawJSON, []byte("[DONE]")) {
		output := make([]byte, 0, 256)
		// Only send final events if we have actually output content
		if params.HasContent {
			appendFinalEvents(params, &output, true)
			output = translatorcommon.AppendSSEEventString(output, "message_stop", `{"type":"message_stop"}`, 3)
			return [][]byte{output}
		}
		return [][]byte{}
	}

	output := make([]byte, 0, 1024)
	appendEvent := func(event, payload string) {
		output = translatorcommon.AppendSSEEventString(output, event, payload, 3)
	}

	// Initialize the streaming session with a message_start event
	// This is only sent for the very first response chunk to establish the streaming session
	if !params.HasFirstResponse {
		// Create the initial message structure with default values according to Claude Code API specification
		// This follows the Claude Code API specification for streaming message initialization
		messageStartTemplate := []byte(`{"type": "message_start", "message": {"id": "msg_1nZdL29xx5MUA1yADyHTEsnR8uuvGzszyY", "type": "message", "role": "assistant", "content": [], "model": "claude-3-5-sonnet-20241022", "stop_reason": null, "stop_sequence": null, "usage": {"input_tokens": 0, "output_tokens": 0}}}`)

		// Use cpaUsageMetadata within the message_start event for Claude.
		if promptTokenCount := gjson.GetBytes(rawJSON, "response.cpaUsageMetadata.promptTokenCount"); promptTokenCount.Exists() {
			messageStartTemplate, _ = sjson.SetBytes(messageStartTemplate, "message.usage.input_tokens", promptTokenCount.Int())
		}
		if candidatesTokenCount := gjson.GetBytes(rawJSON, "response.cpaUsageMetadata.candidatesTokenCount"); candidatesTokenCount.Exists() {
			messageStartTemplate, _ = sjson.SetBytes(messageStartTemplate, "message.usage.output_tokens", candidatesTokenCount.Int())
		}

		// Override default values with actual response metadata if available from the Gemini CLI response
		if modelVersionResult := gjson.GetBytes(rawJSON, "response.modelVersion"); modelVersionResult.Exists() {
			messageStartTemplate, _ = sjson.SetBytes(messageStartTemplate, "message.model", modelVersionResult.String())
		}
		if responseIDResult := gjson.GetBytes(rawJSON, "response.responseId"); responseIDResult.Exists() {
			messageStartTemplate, _ = sjson.SetBytes(messageStartTemplate, "message.id", responseIDResult.String())
		}
		appendEvent("message_start", string(messageStartTemplate))

		params.HasFirstResponse = true
	}

	// Process the response parts array from the backend client
	// Each part can contain text content, thinking content, or function calls
	partsResult := gjson.GetBytes(rawJSON, "response.candidates.0.content.parts")
	if partsResult.IsArray() {
		partResults := partsResult.Array()
		for i := 0; i < len(partResults); i++ {
			partResult := partResults[i]

			// Extract the different types of content from each part
			partTextResult := partResult.Get("text")
			functionCallResult := partResult.Get("functionCall")

			// Handle text content (both regular content and thinking)
			if partTextResult.Exists() {
				// Process thinking content (internal reasoning)
				if partResult.Get("thought").Bool() {
					if thoughtSignature := partResult.Get("thoughtSignature"); thoughtSignature.Exists() && thoughtSignature.String() != "" {
						// log.Debug("Branch: signature_delta")

						// Flush co-located text before emitting the signature
						if partText := partTextResult.String(); partText != "" {
							if params.ResponseType != 2 {
								if params.ResponseType != 0 {
									appendEvent("content_block_stop", fmt.Sprintf(`{"type":"content_block_stop","index":%d}`, params.ResponseIndex))
									params.ResponseIndex++
								}
								appendEvent("content_block_start", fmt.Sprintf(`{"type":"content_block_start","index":%d,"content_block":{"type":"thinking","thinking":""}}`, params.ResponseIndex))
								params.ResponseType = 2
								params.CurrentThinkingText.Reset()
							}
							params.CurrentThinkingText.WriteString(partText)
							data, _ := sjson.SetBytes([]byte(fmt.Sprintf(`{"type":"content_block_delta","index":%d,"delta":{"type":"thinking_delta","thinking":""}}`, params.ResponseIndex)), "delta.thinking", partText)
							appendEvent("content_block_delta", string(data))
						}

						if params.CurrentThinkingText.Len() > 0 {
							cache.CacheSignature(modelName, params.CurrentThinkingText.String(), thoughtSignature.String())
							// log.Debugf("Cached signature for thinking block (textLen=%d)", params.CurrentThinkingText.Len())
							params.CurrentThinkingText.Reset()
						}

						sigValue := formatClaudeSignatureValue(modelName, thoughtSignature.String())
						data, _ := sjson.SetBytes([]byte(fmt.Sprintf(`{"type":"content_block_delta","index":%d,"delta":{"type":"signature_delta","signature":""}}`, params.ResponseIndex)), "delta.signature", sigValue)
						appendEvent("content_block_delta", string(data))
						params.HasContent = true
					} else if params.ResponseType == 2 { // Continue existing thinking block if already in thinking state
						params.CurrentThinkingText.WriteString(partTextResult.String())
						data, _ := sjson.SetBytes([]byte(fmt.Sprintf(`{"type":"content_block_delta","index":%d,"delta":{"type":"thinking_delta","thinking":""}}`, params.ResponseIndex)), "delta.thinking", partTextResult.String())
						appendEvent("content_block_delta", string(data))
						params.HasContent = true
					} else {
						// Transition from another state to thinking
						// First, close any existing content block
						if params.ResponseType != 0 {
							if params.ResponseType == 2 {
								// output = output + "event: content_block_delta\n"
								// output = output + fmt.Sprintf(`data: {"type":"content_block_delta","index":%d,"delta":{"type":"signature_delta","signature":null}}`, params.ResponseIndex)
								// output = output + "\n\n\n"
							}
							appendEvent("content_block_stop", fmt.Sprintf(`{"type":"content_block_stop","index":%d}`, params.ResponseIndex))
							params.ResponseIndex++
						}

						// Start a new thinking content block
						appendEvent("content_block_start", fmt.Sprintf(`{"type":"content_block_start","index":%d,"content_block":{"type":"thinking","thinking":""}}`, params.ResponseIndex))
						data, _ := sjson.SetBytes([]byte(fmt.Sprintf(`{"type":"content_block_delta","index":%d,"delta":{"type":"thinking_delta","thinking":""}}`, params.ResponseIndex)), "delta.thinking", partTextResult.String())
						appendEvent("content_block_delta", string(data))
						params.ResponseType = 2 // Set state to thinking
						params.HasContent = true
						// Start accumulating thinking text for signature caching
						params.CurrentThinkingText.Reset()
						params.CurrentThinkingText.WriteString(partTextResult.String())
					}
				} else {
					finishReasonResult := gjson.GetBytes(rawJSON, "response.candidates.0.finishReason")
					if partTextResult.String() != "" || !finishReasonResult.Exists() {
						// Process regular text content (user-visible output)
						// Continue existing text block if already in content state
						if params.ResponseType == 1 {
							data, _ := sjson.SetBytes([]byte(fmt.Sprintf(`{"type":"content_block_delta","index":%d,"delta":{"type":"text_delta","text":""}}`, params.ResponseIndex)), "delta.text", partTextResult.String())
							appendEvent("content_block_delta", string(data))
							params.HasContent = true
						} else {
							// Transition from another state to text content
							// First, close any existing content block
							if params.ResponseType != 0 {
								if params.ResponseType == 2 {
									// output = output + "event: content_block_delta\n"
									// output = output + fmt.Sprintf(`data: {"type":"content_block_delta","index":%d,"delta":{"type":"signature_delta","signature":null}}`, params.ResponseIndex)
									// output = output + "\n\n\n"
								}
								appendEvent("content_block_stop", fmt.Sprintf(`{"type":"content_block_stop","index":%d}`, params.ResponseIndex))
								params.ResponseIndex++
							}
							if partTextResult.String() != "" {
								// Start a new text content block
								appendEvent("content_block_start", fmt.Sprintf(`{"type":"content_block_start","index":%d,"content_block":{"type":"text","text":""}}`, params.ResponseIndex))
								data, _ := sjson.SetBytes([]byte(fmt.Sprintf(`{"type":"content_block_delta","index":%d,"delta":{"type":"text_delta","text":""}}`, params.ResponseIndex)), "delta.text", partTextResult.String())
								appendEvent("content_block_delta", string(data))
								params.ResponseType = 1 // Set state to content
								params.HasContent = true
							}
						}
					}
				}
			} else if functionCallResult.Exists() {
				// Handle function/tool calls from the AI model
				// This processes tool usage requests and formats them for Claude Code API compatibility
				params.HasToolUse = true
				fcName := util.RestoreSanitizedToolName(params.ToolNameMap, functionCallResult.Get("name").String())

				// Handle state transitions when switching to function calls
				// Close any existing function call block first
				if params.ResponseType == 3 {
					appendEvent("content_block_stop", fmt.Sprintf(`{"type":"content_block_stop","index":%d}`, params.ResponseIndex))
					params.ResponseIndex++
					params.ResponseType = 0
				}

				// Special handling for thinking state transition
				if params.ResponseType == 2 {
					// output = output + "event: content_block_delta\n"
					// output = output + fmt.Sprintf(`data: {"type":"content_block_delta","index":%d,"delta":{"type":"signature_delta","signature":null}}`, params.ResponseIndex)
					// output = output + "\n\n\n"
				}

				// Close any other existing content block
				if params.ResponseType != 0 {
					appendEvent("content_block_stop", fmt.Sprintf(`{"type":"content_block_stop","index":%d}`, params.ResponseIndex))
					params.ResponseIndex++
				}

				// Start a new tool use content block
				// This creates the structure for a function call in Claude Code format
				// Create the tool use block with unique ID and function details
				data := []byte(fmt.Sprintf(`{"type":"content_block_start","index":%d,"content_block":{"type":"tool_use","id":"","name":"","input":{}}}`, params.ResponseIndex))
				data, _ = sjson.SetBytes(data, "content_block.id", util.SanitizeClaudeToolID(fmt.Sprintf("%s-%d-%d", fcName, time.Now().UnixNano(), atomic.AddUint64(&toolUseIDCounter, 1))))
				data, _ = sjson.SetBytes(data, "content_block.name", fcName)
				appendEvent("content_block_start", string(data))

				if fcArgsResult := functionCallResult.Get("args"); fcArgsResult.Exists() {
					data, _ = sjson.SetBytes([]byte(fmt.Sprintf(`{"type":"content_block_delta","index":%d,"delta":{"type":"input_json_delta","partial_json":""}}`, params.ResponseIndex)), "delta.partial_json", fcArgsResult.Raw)
					appendEvent("content_block_delta", string(data))
				}
				params.ResponseType = 3
				params.HasContent = true
			}
		}
	}

	if finishReasonResult := gjson.GetBytes(rawJSON, "response.candidates.0.finishReason"); finishReasonResult.Exists() {
		params.HasFinishReason = true
		params.FinishReason = finishReasonResult.String()
	}

	if usageResult := gjson.GetBytes(rawJSON, "response.usageMetadata"); usageResult.Exists() {
		params.HasUsageMetadata = true
		params.CachedTokenCount = usageResult.Get("cachedContentTokenCount").Int()
		params.PromptTokenCount = usageResult.Get("promptTokenCount").Int() - params.CachedTokenCount
		params.CandidatesTokenCount = usageResult.Get("candidatesTokenCount").Int()
		params.ThoughtsTokenCount = usageResult.Get("thoughtsTokenCount").Int()
		params.TotalTokenCount = usageResult.Get("totalTokenCount").Int()
		if params.CandidatesTokenCount == 0 && params.TotalTokenCount > 0 {
			params.CandidatesTokenCount = params.TotalTokenCount - params.PromptTokenCount - params.ThoughtsTokenCount
			if params.CandidatesTokenCount < 0 {
				params.CandidatesTokenCount = 0
			}
		}
	}

	if params.HasUsageMetadata && params.HasFinishReason {
		appendFinalEvents(params, &output, false)
	}

	return [][]byte{output}
}

// appendFinalEvents 追加最终的 SSE 事件（content_block_stop 和 message_delta）。
// 仅在使用量元数据可用或强制模式下发送，且必须有实际内容输出。
func appendFinalEvents(params *Params, output *[]byte, force bool) {
	if params.HasSentFinalEvents {
		return
	}

	if !params.HasUsageMetadata && !force {
		return
	}

	// Only send final events if we have actually output content
	if !params.HasContent {
		return
	}

	if params.ResponseType != 0 {
		*output = translatorcommon.AppendSSEEventString(*output, "content_block_stop", fmt.Sprintf(`{"type":"content_block_stop","index":%d}`, params.ResponseIndex), 3)
		params.ResponseType = 0
	}

	stopReason := resolveStopReason(params)
	usageOutputTokens := params.CandidatesTokenCount + params.ThoughtsTokenCount
	if usageOutputTokens == 0 && params.TotalTokenCount > 0 {
		usageOutputTokens = params.TotalTokenCount - params.PromptTokenCount
		if usageOutputTokens < 0 {
			usageOutputTokens = 0
		}
	}

	delta := []byte(fmt.Sprintf(`{"type":"message_delta","delta":{"stop_reason":"%s","stop_sequence":null},"usage":{"input_tokens":%d,"output_tokens":%d}}`, stopReason, params.PromptTokenCount, usageOutputTokens))
	// Add cache_read_input_tokens if cached tokens are present (indicates prompt caching is working)
	if params.CachedTokenCount > 0 {
		var err error
		delta, err = sjson.SetBytes(delta, "usage.cache_read_input_tokens", params.CachedTokenCount)
		if err != nil {
			log.Warnf("antigravity claude response: failed to set cache_read_input_tokens: %v", err)
		}
	}
	*output = translatorcommon.AppendSSEEventString(*output, "message_delta", string(delta), 3)

	params.HasSentFinalEvents = true
}

// resolveStopReason 根据状态参数解析 Claude 停止原因。
// 如果存在工具使用则返回 "tool_use"，否则根据完成原因映射。
func resolveStopReason(params *Params) string {
	if params.HasToolUse {
		return "tool_use"
	}

	switch params.FinishReason {
	case "MAX_TOKENS":
		return "max_tokens"
	case "STOP", "FINISH_REASON_UNSPECIFIED", "UNKNOWN":
		return "end_turn"
	}

	return "end_turn"
}

// ConvertAntigravityResponseToClaudeNonStream converts a non-streaming Gemini CLI response to a non-streaming Claude response.
//
// Parameters:
//   - ctx: The context for the request.
//   - modelName: The name of the model.
//   - rawJSON: The raw JSON response from the Gemini CLI API.
//   - param: A pointer to a parameter object for the conversion.
//
// Returns:
//   - []byte: A Claude-compatible JSON response.
func ConvertAntigravityResponseToClaudeNonStream(_ context.Context, _ string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, _ *any) []byte {
	toolNameMap := util.SanitizedToolNameMap(originalRequestRawJSON)
	modelName := gjson.GetBytes(requestRawJSON, "model").String()

	root := gjson.ParseBytes(rawJSON)
	promptTokens := root.Get("response.usageMetadata.promptTokenCount").Int()
	candidateTokens := root.Get("response.usageMetadata.candidatesTokenCount").Int()
	thoughtTokens := root.Get("response.usageMetadata.thoughtsTokenCount").Int()
	totalTokens := root.Get("response.usageMetadata.totalTokenCount").Int()
	cachedTokens := root.Get("response.usageMetadata.cachedContentTokenCount").Int()
	outputTokens := candidateTokens + thoughtTokens
	if outputTokens == 0 && totalTokens > 0 {
		outputTokens = totalTokens - promptTokens
		if outputTokens < 0 {
			outputTokens = 0
		}
	}

	responseJSON := []byte(`{"id":"","type":"message","role":"assistant","model":"","content":null,"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":0,"output_tokens":0}}`)
	responseJSON, _ = sjson.SetBytes(responseJSON, "id", root.Get("response.responseId").String())
	responseJSON, _ = sjson.SetBytes(responseJSON, "model", root.Get("response.modelVersion").String())
	responseJSON, _ = sjson.SetBytes(responseJSON, "usage.input_tokens", promptTokens)
	responseJSON, _ = sjson.SetBytes(responseJSON, "usage.output_tokens", outputTokens)
	// Add cache_read_input_tokens if cached tokens are present (indicates prompt caching is working)
	if cachedTokens > 0 {
		var err error
		responseJSON, err = sjson.SetBytes(responseJSON, "usage.cache_read_input_tokens", cachedTokens)
		if err != nil {
			log.Warnf("antigravity claude response: failed to set cache_read_input_tokens: %v", err)
		}
	}

	contentArrayInitialized := false
	ensureContentArray := func() {
		if contentArrayInitialized {
			return
		}
		responseJSON, _ = sjson.SetRawBytes(responseJSON, "content", []byte("[]"))
		contentArrayInitialized = true
	}

	parts := root.Get("response.candidates.0.content.parts")
	textBuilder := strings.Builder{}
	thinkingBuilder := strings.Builder{}
	thinkingSignature := ""
	toolIDCounter := 0
	hasToolCall := false

	flushText := func() {
		if textBuilder.Len() == 0 {
			return
		}
		ensureContentArray()
		block := []byte(`{"type":"text","text":""}`)
		block, _ = sjson.SetBytes(block, "text", textBuilder.String())
		responseJSON, _ = sjson.SetRawBytes(responseJSON, "content.-1", block)
		textBuilder.Reset()
	}

	flushThinking := func() {
		if thinkingBuilder.Len() == 0 && thinkingSignature == "" {
			return
		}
		ensureContentArray()
		block := []byte(`{"type":"thinking","thinking":""}`)
		block, _ = sjson.SetBytes(block, "thinking", thinkingBuilder.String())
		if thinkingSignature != "" {
			sigValue := formatClaudeSignatureValue(modelName, thinkingSignature)
			block, _ = sjson.SetBytes(block, "signature", sigValue)
		}
		responseJSON, _ = sjson.SetRawBytes(responseJSON, "content.-1", block)
		thinkingBuilder.Reset()
		thinkingSignature = ""
	}

	if parts.IsArray() {
		for _, part := range parts.Array() {
			isThought := part.Get("thought").Bool()
			if isThought {
				sig := part.Get("thoughtSignature")
				if !sig.Exists() {
					sig = part.Get("thought_signature")
				}
				if sig.Exists() && sig.String() != "" {
					thinkingSignature = sig.String()
				}
			}

			if text := part.Get("text"); text.Exists() && text.String() != "" {
				if isThought {
					flushText()
					thinkingBuilder.WriteString(text.String())
					continue
				}
				flushThinking()
				textBuilder.WriteString(text.String())
				continue
			}

			if functionCall := part.Get("functionCall"); functionCall.Exists() {
				flushThinking()
				flushText()
				hasToolCall = true

				name := util.RestoreSanitizedToolName(toolNameMap, functionCall.Get("name").String())
				toolIDCounter++
				toolBlock := []byte(`{"type":"tool_use","id":"","name":"","input":{}}`)
				toolBlock, _ = sjson.SetBytes(toolBlock, "id", fmt.Sprintf("tool_%d", toolIDCounter))
				toolBlock, _ = sjson.SetBytes(toolBlock, "name", name)

				if args := functionCall.Get("args"); args.Exists() && args.Raw != "" && gjson.Valid(args.Raw) && args.IsObject() {
					toolBlock, _ = sjson.SetRawBytes(toolBlock, "input", []byte(args.Raw))
				}

				ensureContentArray()
				responseJSON, _ = sjson.SetRawBytes(responseJSON, "content.-1", toolBlock)
				continue
			}
		}
	}

	flushThinking()
	flushText()

	stopReason := "end_turn"
	if hasToolCall {
		stopReason = "tool_use"
	} else {
		if finish := root.Get("response.candidates.0.finishReason"); finish.Exists() {
			switch finish.String() {
			case "MAX_TOKENS":
				stopReason = "max_tokens"
			case "STOP", "FINISH_REASON_UNSPECIFIED", "UNKNOWN":
				stopReason = "end_turn"
			default:
				stopReason = "end_turn"
			}
		}
	}
	responseJSON, _ = sjson.SetBytes(responseJSON, "stop_reason", stopReason)

	if promptTokens == 0 && outputTokens == 0 {
		if usageMeta := root.Get("response.usageMetadata"); !usageMeta.Exists() {
			responseJSON, _ = sjson.DeleteBytes(responseJSON, "usage")
		}
	}

	return responseJSON
}

// ClaudeTokenCount 生成 Claude 格式的 Token 计数 JSON 响应。
func ClaudeTokenCount(ctx context.Context, count int64) []byte {
	return translatorcommon.ClaudeInputTokensJSON(count)
}
