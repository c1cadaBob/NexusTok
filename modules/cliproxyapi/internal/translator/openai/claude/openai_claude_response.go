// openai/claude - openai_claude_response.go
// Package claude provides response translation functionality for OpenAI to Anthropic API.
// 本文件提供 OpenAI Chat Completions API 响应到 Anthropic API 格式的转换功能。
// 支持流式和非流式模式，处理文本内容、工具调用和使用量元数据。
// 流式模式下实现了复杂的状态机，管理不同内容类型的 SSE 事件排序，
// 包括 thinking（reasoning_content）、text、tool_use 等内容块的生命周期管理。
package claude

import (
	"bytes"
	"context"
	"sort"
	"strings"

	translatorcommon "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/common"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/util"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// dataTag 是 SSE 数据行的前缀标识，用于从原始响应中提取 JSON 数据。
var (
	dataTag = []byte("data:")
)

// ConvertOpenAIResponseToAnthropicParams 持有响应转换的状态参数。
// 跟踪流式响应中各种内容块的状态，确保正确的 SSE 事件排序。
type ConvertOpenAIResponseToAnthropicParams struct {
	MessageID   string            // 消息 ID
	Model       string            // 模型名称
	CreatedAt   int64             // 创建时间戳
	ToolNameMap map[string]string // 工具名称映射表
	// SawToolCall 为 true 表示至少已发出一个 tool_use content_block_start。
	// 使用原始上游 tool_calls 存在性可能导致 stop_reason=tool_use 但没有已宣布的工具块。
	SawToolCall bool
	// ContentAccumulator 内容累积器，用于流式模式
	ContentAccumulator strings.Builder
	// ToolCallsAccumulator 工具调用累积器，用于流式模式
	ToolCallsAccumulator map[int]*ToolCallAccumulator
	// TextContentBlockStarted 跟踪文本内容块是否已开始
	TextContentBlockStarted bool
	// ThinkingContentBlockStarted 跟踪思考内容块是否已开始
	ThinkingContentBlockStarted bool
	// FinishReason 跟踪完成原因以供后续使用
	FinishReason string
	// ContentBlocksStopped 跟踪内容块是否已停止
	ContentBlocksStopped bool
	// MessageDeltaSent 跟踪 message_delta 是否已发送
	MessageDeltaSent bool
	// MessageStarted 跟踪 message_start 是否已发送
	MessageStarted bool
	// MessageStopSent 跟踪 message_stop 是否已发送
	MessageStopSent bool
	// ToolCallBlockIndexes 工具调用内容块索引映射
	ToolCallBlockIndexes map[int]int
	// TextContentBlockIndex 文本内容块的索引
	TextContentBlockIndex int
	// ThinkingContentBlockIndex 思考内容块的索引
	ThinkingContentBlockIndex int
	// NextContentBlockIndex 下一个可用的内容块索引
	NextContentBlockIndex int
}

// ToolCallAccumulator 持有工具调用数据的累积状态。
// 在流式模式下累积工具调用的 ID、名称和参数。
type ToolCallAccumulator struct {
	ID        string          // 工具调用 ID
	Name      string          // 函数名称
	Arguments strings.Builder // 累积的参数 JSON 字符串
	// StartEmitted 跟踪是否已为此工具索引发送了 content_block_start
	StartEmitted bool
}

// ConvertOpenAIResponseToClaude converts OpenAI streaming response format to Anthropic API format.
// This function processes OpenAI streaming chunks and transforms them into Anthropic-compatible JSON responses.
// It handles text content, tool calls, and usage metadata, outputting responses that match the Anthropic API format.
//
// Parameters:
//   - ctx: The context for the request.
//   - modelName: The name of the model.
//   - rawJSON: The raw JSON response from the OpenAI API.
//   - param: A pointer to a parameter object for the conversion.
//
// Returns:
//   - [][]byte: A slice of byte chunks, each containing an Anthropic-compatible JSON response.
func ConvertOpenAIResponseToClaude(_ context.Context, _ string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) [][]byte {
	if *param == nil {
		*param = &ConvertOpenAIResponseToAnthropicParams{
			MessageID:                   "",
			Model:                       "",
			CreatedAt:                   0,
			ToolNameMap:                 nil,
			SawToolCall:                 false,
			ContentAccumulator:          strings.Builder{},
			ToolCallsAccumulator:        nil,
			TextContentBlockStarted:     false,
			ThinkingContentBlockStarted: false,
			FinishReason:                "",
			ContentBlocksStopped:        false,
			MessageDeltaSent:            false,
			ToolCallBlockIndexes:        make(map[int]int),
			TextContentBlockIndex:       -1,
			ThinkingContentBlockIndex:   -1,
			NextContentBlockIndex:       0,
		}
	}

	if !bytes.HasPrefix(rawJSON, dataTag) {
		return [][]byte{}
	}
	rawJSON = bytes.TrimSpace(rawJSON[5:])

	if (*param).(*ConvertOpenAIResponseToAnthropicParams).ToolNameMap == nil {
		(*param).(*ConvertOpenAIResponseToAnthropicParams).ToolNameMap = util.ToolNameMapFromClaudeRequest(originalRequestRawJSON)
	}

	// Check if this is the [DONE] marker
	if bytes.Equal(bytes.TrimSpace(rawJSON), []byte("[DONE]")) {
		return convertOpenAIDoneToAnthropic((*param).(*ConvertOpenAIResponseToAnthropicParams))
	}

	streamResult := gjson.GetBytes(originalRequestRawJSON, "stream")
	if !streamResult.Exists() || (streamResult.Exists() && streamResult.Type == gjson.False) {
		return convertOpenAINonStreamingToAnthropic(rawJSON)
	} else {
		return convertOpenAIStreamingChunkToAnthropic(rawJSON, (*param).(*ConvertOpenAIResponseToAnthropicParams))
	}
}

func effectiveOpenAIFinishReason(param *ConvertOpenAIResponseToAnthropicParams) string {
	if param == nil {
		return ""
	}
	if param.SawToolCall {
		return "tool_calls"
	}
	return param.FinishReason
}

// convertOpenAIStreamingChunkToAnthropic converts OpenAI streaming chunk to Anthropic streaming events
func convertOpenAIStreamingChunkToAnthropic(rawJSON []byte, param *ConvertOpenAIResponseToAnthropicParams) [][]byte {
	root := gjson.ParseBytes(rawJSON)
	var results [][]byte

	// Initialize parameters if needed
	if param.MessageID == "" {
		param.MessageID = root.Get("id").String()
	}
	if param.Model == "" {
		param.Model = root.Get("model").String()
	}
	if param.CreatedAt == 0 {
		param.CreatedAt = root.Get("created").Int()
	}

	// Emit message_start on the very first chunk, regardless of whether it has a role field.
	// Some providers (like Copilot) may send tool_calls in the first chunk without a role field.
	if delta := root.Get("choices.0.delta"); delta.Exists() {
		if !param.MessageStarted {
			// Send message_start event
			messageStartJSON := []byte(`{"type":"message_start","message":{"id":"","type":"message","role":"assistant","model":"","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":0,"output_tokens":0}}}`)
			messageStartJSON, _ = sjson.SetBytes(messageStartJSON, "message.id", param.MessageID)
			messageStartJSON, _ = sjson.SetBytes(messageStartJSON, "message.model", param.Model)
			results = append(results, translatorcommon.AppendSSEEventBytes(nil, "message_start", messageStartJSON, 2))
			param.MessageStarted = true

			// Don't send content_block_start for text here - wait for actual content
		}

		// Handle reasoning content delta
		if reasoning := delta.Get("reasoning_content"); reasoning.Exists() {
			for _, reasoningText := range collectOpenAIReasoningTexts(reasoning) {
				if reasoningText == "" {
					continue
				}
				stopTextContentBlock(param, &results)
				if !param.ThinkingContentBlockStarted {
					if param.ThinkingContentBlockIndex == -1 {
						param.ThinkingContentBlockIndex = param.NextContentBlockIndex
						param.NextContentBlockIndex++
					}
					contentBlockStartJSON := `{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`
					contentBlockStartJSONBytes := []byte(contentBlockStartJSON)
					contentBlockStartJSONBytes, _ = sjson.SetBytes(contentBlockStartJSONBytes, "index", param.ThinkingContentBlockIndex)
					results = append(results, translatorcommon.AppendSSEEventBytes(nil, "content_block_start", contentBlockStartJSONBytes, 2))
					param.ThinkingContentBlockStarted = true
				}

				thinkingDeltaJSON := `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":""}}`
				thinkingDeltaJSONBytes := []byte(thinkingDeltaJSON)
				thinkingDeltaJSONBytes, _ = sjson.SetBytes(thinkingDeltaJSONBytes, "index", param.ThinkingContentBlockIndex)
				thinkingDeltaJSONBytes, _ = sjson.SetBytes(thinkingDeltaJSONBytes, "delta.thinking", reasoningText)
				results = append(results, translatorcommon.AppendSSEEventBytes(nil, "content_block_delta", thinkingDeltaJSONBytes, 2))
			}
		}

		// Handle content delta
		if content := delta.Get("content"); content.Exists() && content.String() != "" {
			// Send content_block_start for text if not already sent
			if !param.TextContentBlockStarted {
				stopThinkingContentBlock(param, &results)
				if param.TextContentBlockIndex == -1 {
					param.TextContentBlockIndex = param.NextContentBlockIndex
					param.NextContentBlockIndex++
				}
				contentBlockStartJSON := `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`
				contentBlockStartJSONBytes := []byte(contentBlockStartJSON)
				contentBlockStartJSONBytes, _ = sjson.SetBytes(contentBlockStartJSONBytes, "index", param.TextContentBlockIndex)
				results = append(results, translatorcommon.AppendSSEEventBytes(nil, "content_block_start", contentBlockStartJSONBytes, 2))
				param.TextContentBlockStarted = true
			}

			contentDeltaJSON := `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":""}}`
			contentDeltaJSONBytes := []byte(contentDeltaJSON)
			contentDeltaJSONBytes, _ = sjson.SetBytes(contentDeltaJSONBytes, "index", param.TextContentBlockIndex)
			contentDeltaJSONBytes, _ = sjson.SetBytes(contentDeltaJSONBytes, "delta.text", content.String())
			results = append(results, translatorcommon.AppendSSEEventBytes(nil, "content_block_delta", contentDeltaJSONBytes, 2))

			// Accumulate content
			param.ContentAccumulator.WriteString(content.String())
		}

		// Handle tool calls
		if toolCalls := delta.Get("tool_calls"); toolCalls.Exists() && toolCalls.IsArray() {
			if param.ToolCallsAccumulator == nil {
				param.ToolCallsAccumulator = make(map[int]*ToolCallAccumulator)
			}

			toolCalls.ForEach(func(_, toolCall gjson.Result) bool {
				index := int(toolCall.Get("index").Int())

				// Initialize accumulator if needed
				if _, exists := param.ToolCallsAccumulator[index]; !exists {
					param.ToolCallsAccumulator[index] = &ToolCallAccumulator{}
				}

				accumulator := param.ToolCallsAccumulator[index]

				// Handle tool call ID. Only accept JSON-string, non-empty
				// values so malformed upstream fields do not overwrite a
				// valid ID or coerce into a content_block.id.
				if id := toolCall.Get("id"); id.Exists() && id.Type == gjson.String {
					if idStr := id.String(); idStr != "" {
						accumulator.ID = idStr
					}
				}

				// Handle function name and arguments
				if function := toolCall.Get("function"); function.Exists() {
					// Only record the name until content_block_start has been
					// emitted. Some upstreams send "name": "" or repeat the
					// field across chunks; reassigning after start could drift
					// from what was already announced.
					if !accumulator.StartEmitted {
						if name := function.Get("name"); name.Exists() && name.Type == gjson.String && name.String() != "" {
							accumulator.Name = util.MapToolName(param.ToolNameMap, name.String())
						}
					}

					// Handle function arguments
					if args := function.Get("arguments"); args.Exists() {
						argsText := args.String()
						if argsText != "" {
							accumulator.Arguments.WriteString(argsText)
						}
					}
				}

				// Re-check on every chunk, not only chunks with a function
				// object. Some upstreams split function.name and id across
				// separate deltas.
				if !accumulator.StartEmitted && accumulator.Name != "" && accumulator.ID != "" && !param.ContentBlocksStopped {
					emitToolUseStart(param, index, accumulator, &results)
				}

				return true
			})
		}
	}

	// Handle finish_reason (but don't send message_delta/message_stop yet)
	if finishReason := root.Get("choices.0.finish_reason"); finishReason.Exists() && finishReason.String() != "" {
		reason := finishReason.String()
		switch {
		case param.SawToolCall:
			param.FinishReason = "tool_calls"
		case reason == "tool_calls":
			param.FinishReason = "stop"
		default:
			param.FinishReason = reason
		}

		// Send content_block_stop for thinking content if needed
		if param.ThinkingContentBlockStarted {
			contentBlockStopJSON := []byte(`{"type":"content_block_stop","index":0}`)
			contentBlockStopJSON, _ = sjson.SetBytes(contentBlockStopJSON, "index", param.ThinkingContentBlockIndex)
			results = append(results, translatorcommon.AppendSSEEventBytes(nil, "content_block_stop", contentBlockStopJSON, 2))
			param.ThinkingContentBlockStarted = false
			param.ThinkingContentBlockIndex = -1
		}

		// Send content_block_stop for text if text content block was started
		stopTextContentBlock(param, &results)

		// Send content_block_stop for any tool calls
		if !param.ContentBlocksStopped {
			for _, index := range toolCallAccumulatorIndexes(param.ToolCallsAccumulator) {
				accumulator := param.ToolCallsAccumulator[index]
				if !accumulator.StartEmitted {
					// Belated emit for streams that supplied a valid name but
					// never sent an id. SanitizeClaudeToolID("") produces the
					// expected stable synthetic toolu_<nanos>_<n> ID shape.
					if accumulator.Name == "" {
						continue
					}
					emitToolUseStart(param, index, accumulator, &results)
				}
				blockIndex := param.toolContentBlockIndex(index)

				// Send complete input_json_delta with all accumulated arguments
				if accumulator.Arguments.Len() > 0 {
					inputDeltaJSON := []byte(`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":""}}`)
					inputDeltaJSON, _ = sjson.SetBytes(inputDeltaJSON, "index", blockIndex)
					inputDeltaJSON, _ = sjson.SetBytes(inputDeltaJSON, "delta.partial_json", util.FixJSON(accumulator.Arguments.String()))
					results = append(results, translatorcommon.AppendSSEEventBytes(nil, "content_block_delta", inputDeltaJSON, 2))
				}

				contentBlockStopJSON := []byte(`{"type":"content_block_stop","index":0}`)
				contentBlockStopJSON, _ = sjson.SetBytes(contentBlockStopJSON, "index", blockIndex)
				results = append(results, translatorcommon.AppendSSEEventBytes(nil, "content_block_stop", contentBlockStopJSON, 2))
				delete(param.ToolCallBlockIndexes, index)
			}
			param.ContentBlocksStopped = true
		}

		// Don't send message_delta here - wait for usage info or [DONE]
	}

	// Handle usage information separately (this comes in a later chunk)
	// Only process if usage has actual values (not null)
	if param.FinishReason != "" {
		usage := root.Get("usage")
		var inputTokens, outputTokens, cachedTokens int64
		if usage.Exists() && usage.Type != gjson.Null {
			inputTokens, outputTokens, cachedTokens = extractOpenAIUsage(usage)
			// Send message_delta with usage
			messageDeltaJSON := []byte(`{"type":"message_delta","delta":{"stop_reason":"","stop_sequence":null},"usage":{"input_tokens":0,"output_tokens":0}}`)
			messageDeltaJSON, _ = sjson.SetBytes(messageDeltaJSON, "delta.stop_reason", mapOpenAIFinishReasonToAnthropic(effectiveOpenAIFinishReason(param)))
			messageDeltaJSON, _ = sjson.SetBytes(messageDeltaJSON, "usage.input_tokens", inputTokens)
			messageDeltaJSON, _ = sjson.SetBytes(messageDeltaJSON, "usage.output_tokens", outputTokens)
			if cachedTokens > 0 {
				messageDeltaJSON, _ = sjson.SetBytes(messageDeltaJSON, "usage.cache_read_input_tokens", cachedTokens)
			}
			results = append(results, translatorcommon.AppendSSEEventBytes(nil, "message_delta", messageDeltaJSON, 2))
			param.MessageDeltaSent = true

			emitMessageStopIfNeeded(param, &results)
		}
	}

	return results
}

// convertOpenAIDoneToAnthropic handles the [DONE] marker and sends final events
func convertOpenAIDoneToAnthropic(param *ConvertOpenAIResponseToAnthropicParams) [][]byte {
	var results [][]byte

	// Ensure all content blocks are stopped before final events
	if param.ThinkingContentBlockStarted {
		contentBlockStopJSON := []byte(`{"type":"content_block_stop","index":0}`)
		contentBlockStopJSON, _ = sjson.SetBytes(contentBlockStopJSON, "index", param.ThinkingContentBlockIndex)
		results = append(results, translatorcommon.AppendSSEEventBytes(nil, "content_block_stop", contentBlockStopJSON, 2))
		param.ThinkingContentBlockStarted = false
		param.ThinkingContentBlockIndex = -1
	}

	stopTextContentBlock(param, &results)

	if !param.ContentBlocksStopped {
		for _, index := range toolCallAccumulatorIndexes(param.ToolCallsAccumulator) {
			accumulator := param.ToolCallsAccumulator[index]
			if !accumulator.StartEmitted {
				// Belated emit at [DONE]; same behavior as the finish_reason
				// path for name-but-no-id streams.
				if accumulator.Name == "" {
					continue
				}
				emitToolUseStart(param, index, accumulator, &results)
			}
			blockIndex := param.toolContentBlockIndex(index)

			if accumulator.Arguments.Len() > 0 {
				inputDeltaJSON := []byte(`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":""}}`)
				inputDeltaJSON, _ = sjson.SetBytes(inputDeltaJSON, "index", blockIndex)
				inputDeltaJSON, _ = sjson.SetBytes(inputDeltaJSON, "delta.partial_json", util.FixJSON(accumulator.Arguments.String()))
				results = append(results, translatorcommon.AppendSSEEventBytes(nil, "content_block_delta", inputDeltaJSON, 2))
			}

			contentBlockStopJSON := []byte(`{"type":"content_block_stop","index":0}`)
			contentBlockStopJSON, _ = sjson.SetBytes(contentBlockStopJSON, "index", blockIndex)
			results = append(results, translatorcommon.AppendSSEEventBytes(nil, "content_block_stop", contentBlockStopJSON, 2))
			delete(param.ToolCallBlockIndexes, index)
		}
		param.ContentBlocksStopped = true
	}

	// If we haven't sent message_delta yet (no usage info was received), send it now
	if param.FinishReason != "" && !param.MessageDeltaSent {
		messageDeltaJSON := []byte(`{"type":"message_delta","delta":{"stop_reason":"","stop_sequence":null},"usage":{"input_tokens":0,"output_tokens":0}}`)
		messageDeltaJSON, _ = sjson.SetBytes(messageDeltaJSON, "delta.stop_reason", mapOpenAIFinishReasonToAnthropic(effectiveOpenAIFinishReason(param)))
		results = append(results, translatorcommon.AppendSSEEventBytes(nil, "message_delta", messageDeltaJSON, 2))
		param.MessageDeltaSent = true
	}

	emitMessageStopIfNeeded(param, &results)

	return results
}

// convertOpenAINonStreamingToAnthropic converts OpenAI non-streaming response to Anthropic format
func convertOpenAINonStreamingToAnthropic(rawJSON []byte) [][]byte {
	root := gjson.ParseBytes(rawJSON)

	out := []byte(`{"id":"","type":"message","role":"assistant","model":"","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":0,"output_tokens":0}}`)
	out, _ = sjson.SetBytes(out, "id", root.Get("id").String())
	out, _ = sjson.SetBytes(out, "model", root.Get("model").String())

	// Process message content and tool calls
	if choices := root.Get("choices"); choices.Exists() && choices.IsArray() && len(choices.Array()) > 0 {
		choice := choices.Array()[0] // Take first choice

		reasoningNode := choice.Get("message.reasoning_content")
		for _, reasoningText := range collectOpenAIReasoningTexts(reasoningNode) {
			if reasoningText == "" {
				continue
			}
			block := []byte(`{"type":"thinking","thinking":""}`)
			block, _ = sjson.SetBytes(block, "thinking", reasoningText)
			out, _ = sjson.SetRawBytes(out, "content.-1", block)
		}

		// Handle text content
		if content := choice.Get("message.content"); content.Exists() && content.String() != "" {
			block := []byte(`{"type":"text","text":""}`)
			block, _ = sjson.SetBytes(block, "text", content.String())
			out, _ = sjson.SetRawBytes(out, "content.-1", block)
		}

		// Handle tool calls
		if toolCalls := choice.Get("message.tool_calls"); toolCalls.Exists() && toolCalls.IsArray() {
			toolCalls.ForEach(func(_, toolCall gjson.Result) bool {
				toolUseBlock := []byte(`{"type":"tool_use","id":"","name":"","input":{}}`)
				toolUseBlock, _ = sjson.SetBytes(toolUseBlock, "id", util.SanitizeClaudeToolID(toolCall.Get("id").String()))
				toolUseBlock, _ = sjson.SetBytes(toolUseBlock, "name", toolCall.Get("function.name").String())

				argsStr := util.FixJSON(toolCall.Get("function.arguments").String())
				if argsStr != "" && gjson.Valid(argsStr) {
					argsJSON := gjson.Parse(argsStr)
					if argsJSON.IsObject() {
						toolUseBlock, _ = sjson.SetRawBytes(toolUseBlock, "input", []byte(argsJSON.Raw))
					} else {
						toolUseBlock, _ = sjson.SetRawBytes(toolUseBlock, "input", []byte(`{}`))
					}
				} else {
					toolUseBlock, _ = sjson.SetRawBytes(toolUseBlock, "input", []byte(`{}`))
				}

				out, _ = sjson.SetRawBytes(out, "content.-1", toolUseBlock)
				return true
			})
		}

		// Set stop reason
		if finishReason := choice.Get("finish_reason"); finishReason.Exists() {
			out, _ = sjson.SetBytes(out, "stop_reason", mapOpenAIFinishReasonToAnthropic(finishReason.String()))
		}
	}

	// Set usage information
	if usage := root.Get("usage"); usage.Exists() {
		inputTokens, outputTokens, cachedTokens := extractOpenAIUsage(usage)
		out, _ = sjson.SetBytes(out, "usage.input_tokens", inputTokens)
		out, _ = sjson.SetBytes(out, "usage.output_tokens", outputTokens)
		if cachedTokens > 0 {
			out, _ = sjson.SetBytes(out, "usage.cache_read_input_tokens", cachedTokens)
		}
	}

	return [][]byte{out}
}

// mapOpenAIFinishReasonToAnthropic 将 OpenAI 完成原因映射为 Anthropic 等效值。
func mapOpenAIFinishReasonToAnthropic(openAIReason string) string {
	switch openAIReason {
	case "stop":
		return "end_turn"
	case "length":
		return "max_tokens"
	case "tool_calls":
		return "tool_use"
	case "content_filter":
		return "end_turn" // Anthropic doesn't have direct equivalent
	case "function_call": // Legacy OpenAI
		return "tool_use"
	default:
		return "end_turn"
	}
}

// toolContentBlockIndex 获取或分配工具调用的内容块索引。
// 使用懒初始化策略，首次访问时分配新索引。
func (p *ConvertOpenAIResponseToAnthropicParams) toolContentBlockIndex(openAIToolIndex int) int {
	if idx, ok := p.ToolCallBlockIndexes[openAIToolIndex]; ok {
		return idx
	}
	idx := p.NextContentBlockIndex
	p.NextContentBlockIndex++
	p.ToolCallBlockIndexes[openAIToolIndex] = idx
	return idx
}

// collectOpenAIReasoningTexts 从 OpenAI 响应中收集推理/思考文本。
// 支持字符串、数组和对象格式的推理内容。
func collectOpenAIReasoningTexts(node gjson.Result) []string {
	var texts []string
	if !node.Exists() {
		return texts
	}

	if node.IsArray() {
		node.ForEach(func(_, value gjson.Result) bool {
			texts = append(texts, collectOpenAIReasoningTexts(value)...)
			return true
		})
		return texts
	}

	switch node.Type {
	case gjson.String:
		if text := node.String(); text != "" {
			texts = append(texts, text)
		}
	case gjson.JSON:
		if text := node.Get("text"); text.Exists() {
			if textStr := text.String(); textStr != "" {
				texts = append(texts, textStr)
			}
		} else if raw := node.Raw; raw != "" && !strings.HasPrefix(raw, "{") && !strings.HasPrefix(raw, "[") {
			texts = append(texts, raw)
		}
	}

	return texts
}

// stopThinkingContentBlock 停止思考内容块（如果已打开）。
func stopThinkingContentBlock(param *ConvertOpenAIResponseToAnthropicParams, results *[][]byte) {
	if !param.ThinkingContentBlockStarted {
		return
	}
	contentBlockStopJSON := []byte(`{"type":"content_block_stop","index":0}`)
	contentBlockStopJSON, _ = sjson.SetBytes(contentBlockStopJSON, "index", param.ThinkingContentBlockIndex)
	*results = append(*results, translatorcommon.AppendSSEEventBytes(nil, "content_block_stop", contentBlockStopJSON, 2))
	param.ThinkingContentBlockStarted = false
	param.ThinkingContentBlockIndex = -1
}

// emitMessageStopIfNeeded 在需要时发送 message_stop 事件（仅发送一次）。
func emitMessageStopIfNeeded(param *ConvertOpenAIResponseToAnthropicParams, results *[][]byte) {
	if param.MessageStopSent {
		return
	}
	*results = append(*results, translatorcommon.AppendSSEEventBytes(nil, "message_stop", []byte(`{"type":"message_stop"}`), 2))
	param.MessageStopSent = true
}

// stopTextContentBlock 停止文本内容块（如果已打开）。
func stopTextContentBlock(param *ConvertOpenAIResponseToAnthropicParams, results *[][]byte) {
	if !param.TextContentBlockStarted {
		return
	}
	contentBlockStopJSON := []byte(`{"type":"content_block_stop","index":0}`)
	contentBlockStopJSON, _ = sjson.SetBytes(contentBlockStopJSON, "index", param.TextContentBlockIndex)
	*results = append(*results, translatorcommon.AppendSSEEventBytes(nil, "content_block_stop", contentBlockStopJSON, 2))
	param.TextContentBlockStarted = false
	param.TextContentBlockIndex = -1
}

// emitToolUseStart 发送工具使用的 content_block_start 事件。
// 先停止当前的思考和文本内容块，然后开始新的工具使用块。
func emitToolUseStart(param *ConvertOpenAIResponseToAnthropicParams, openAIToolIndex int, accumulator *ToolCallAccumulator, results *[][]byte) {
	stopThinkingContentBlock(param, results)
	stopTextContentBlock(param, results)

	blockIndex := param.toolContentBlockIndex(openAIToolIndex)
	contentBlockStartJSON := []byte(`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"","name":"","input":{}}}`)
	contentBlockStartJSON, _ = sjson.SetBytes(contentBlockStartJSON, "index", blockIndex)
	contentBlockStartJSON, _ = sjson.SetBytes(contentBlockStartJSON, "content_block.id", util.SanitizeClaudeToolID(accumulator.ID))
	contentBlockStartJSON, _ = sjson.SetBytes(contentBlockStartJSON, "content_block.name", accumulator.Name)
	*results = append(*results, translatorcommon.AppendSSEEventBytes(nil, "content_block_start", contentBlockStartJSON, 2))
	accumulator.StartEmitted = true
	param.SawToolCall = true
}

// toolCallAccumulatorIndexes 返回工具调用累积器的有序索引列表。
func toolCallAccumulatorIndexes(accumulators map[int]*ToolCallAccumulator) []int {
	indexes := make([]int, 0, len(accumulators))
	for index := range accumulators {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	return indexes
}

// ConvertOpenAIResponseToClaudeNonStream converts a non-streaming OpenAI response to a non-streaming Anthropic response.
//
// Parameters:
//   - ctx: The context for the request.
//   - modelName: The name of the model.
//   - rawJSON: The raw JSON response from the OpenAI API.
//   - param: A pointer to a parameter object for the conversion.
//
// Returns:
//   - []byte: An Anthropic-compatible JSON response.
func ConvertOpenAIResponseToClaudeNonStream(_ context.Context, _ string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, _ *any) []byte {
	_ = requestRawJSON

	root := gjson.ParseBytes(rawJSON)
	toolNameMap := util.ToolNameMapFromClaudeRequest(originalRequestRawJSON)
	out := []byte(`{"id":"","type":"message","role":"assistant","model":"","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":0,"output_tokens":0}}`)
	out, _ = sjson.SetBytes(out, "id", root.Get("id").String())
	out, _ = sjson.SetBytes(out, "model", root.Get("model").String())

	hasToolCall := false
	stopReasonSet := false

	if choices := root.Get("choices"); choices.Exists() && choices.IsArray() && len(choices.Array()) > 0 {
		choice := choices.Array()[0]

		if finishReason := choice.Get("finish_reason"); finishReason.Exists() {
			out, _ = sjson.SetBytes(out, "stop_reason", mapOpenAIFinishReasonToAnthropic(finishReason.String()))
			stopReasonSet = true
		}

		if message := choice.Get("message"); message.Exists() {
			if contentResult := message.Get("content"); contentResult.Exists() {
				if contentResult.IsArray() {
					var textBuilder strings.Builder
					var thinkingBuilder strings.Builder

					flushText := func() {
						if textBuilder.Len() == 0 {
							return
						}
						block := []byte(`{"type":"text","text":""}`)
						block, _ = sjson.SetBytes(block, "text", textBuilder.String())
						out, _ = sjson.SetRawBytes(out, "content.-1", block)
						textBuilder.Reset()
					}

					flushThinking := func() {
						if thinkingBuilder.Len() == 0 {
							return
						}
						block := []byte(`{"type":"thinking","thinking":""}`)
						block, _ = sjson.SetBytes(block, "thinking", thinkingBuilder.String())
						out, _ = sjson.SetRawBytes(out, "content.-1", block)
						thinkingBuilder.Reset()
					}

					for _, item := range contentResult.Array() {
						switch item.Get("type").String() {
						case "text":
							flushThinking()
							textBuilder.WriteString(item.Get("text").String())
						case "tool_calls":
							flushThinking()
							flushText()
							toolCalls := item.Get("tool_calls")
							if toolCalls.IsArray() {
								toolCalls.ForEach(func(_, tc gjson.Result) bool {
									hasToolCall = true
									toolUse := []byte(`{"type":"tool_use","id":"","name":"","input":{}}`)
									toolUse, _ = sjson.SetBytes(toolUse, "id", util.SanitizeClaudeToolID(tc.Get("id").String()))
									toolUse, _ = sjson.SetBytes(toolUse, "name", util.MapToolName(toolNameMap, tc.Get("function.name").String()))

									argsStr := util.FixJSON(tc.Get("function.arguments").String())
									if argsStr != "" && gjson.Valid(argsStr) {
										argsJSON := gjson.Parse(argsStr)
										if argsJSON.IsObject() {
											toolUse, _ = sjson.SetRawBytes(toolUse, "input", []byte(argsJSON.Raw))
										} else {
											toolUse, _ = sjson.SetRawBytes(toolUse, "input", []byte(`{}`))
										}
									} else {
										toolUse, _ = sjson.SetRawBytes(toolUse, "input", []byte(`{}`))
									}

									out, _ = sjson.SetRawBytes(out, "content.-1", toolUse)
									return true
								})
							}
						case "reasoning":
							flushText()
							if thinking := item.Get("text"); thinking.Exists() {
								thinkingBuilder.WriteString(thinking.String())
							}
						default:
							flushThinking()
							flushText()
						}
					}

					flushThinking()
					flushText()
				} else if contentResult.Type == gjson.String {
					textContent := contentResult.String()
					if textContent != "" {
						block := []byte(`{"type":"text","text":""}`)
						block, _ = sjson.SetBytes(block, "text", textContent)
						out, _ = sjson.SetRawBytes(out, "content.-1", block)
					}
				}
			}

			if reasoning := message.Get("reasoning_content"); reasoning.Exists() {
				for _, reasoningText := range collectOpenAIReasoningTexts(reasoning) {
					if reasoningText == "" {
						continue
					}
					block := []byte(`{"type":"thinking","thinking":""}`)
					block, _ = sjson.SetBytes(block, "thinking", reasoningText)
					out, _ = sjson.SetRawBytes(out, "content.-1", block)
				}
			}

			if toolCalls := message.Get("tool_calls"); toolCalls.Exists() && toolCalls.IsArray() {
				toolCalls.ForEach(func(_, toolCall gjson.Result) bool {
					hasToolCall = true
					toolUseBlock := []byte(`{"type":"tool_use","id":"","name":"","input":{}}`)
					toolUseBlock, _ = sjson.SetBytes(toolUseBlock, "id", util.SanitizeClaudeToolID(toolCall.Get("id").String()))
					toolUseBlock, _ = sjson.SetBytes(toolUseBlock, "name", util.MapToolName(toolNameMap, toolCall.Get("function.name").String()))

					argsStr := util.FixJSON(toolCall.Get("function.arguments").String())
					if argsStr != "" && gjson.Valid(argsStr) {
						argsJSON := gjson.Parse(argsStr)
						if argsJSON.IsObject() {
							toolUseBlock, _ = sjson.SetRawBytes(toolUseBlock, "input", []byte(argsJSON.Raw))
						} else {
							toolUseBlock, _ = sjson.SetRawBytes(toolUseBlock, "input", []byte(`{}`))
						}
					} else {
						toolUseBlock, _ = sjson.SetRawBytes(toolUseBlock, "input", []byte(`{}`))
					}

					out, _ = sjson.SetRawBytes(out, "content.-1", toolUseBlock)
					return true
				})
			}
		}
	}

	if respUsage := root.Get("usage"); respUsage.Exists() {
		inputTokens, outputTokens, cachedTokens := extractOpenAIUsage(respUsage)
		out, _ = sjson.SetBytes(out, "usage.input_tokens", inputTokens)
		out, _ = sjson.SetBytes(out, "usage.output_tokens", outputTokens)
		if cachedTokens > 0 {
			out, _ = sjson.SetBytes(out, "usage.cache_read_input_tokens", cachedTokens)
		}
	}

	if !stopReasonSet {
		if hasToolCall {
			out, _ = sjson.SetBytes(out, "stop_reason", "tool_use")
		} else {
			out, _ = sjson.SetBytes(out, "stop_reason", "end_turn")
		}
	}

	return out
}

// ClaudeTokenCount 生成 Claude 格式的 Token 计数 JSON 响应。
func ClaudeTokenCount(ctx context.Context, count int64) []byte {
	return translatorcommon.ClaudeInputTokensJSON(count)
}

// extractOpenAIUsage 从 OpenAI 响应的 usage 字段中提取 Token 使用信息。
// 返回输入 Token 数、输出 Token 数和缓存 Token 数。
// 如果存在缓存 Token，会从输入 Token 中扣除已缓存部分。
func extractOpenAIUsage(usage gjson.Result) (int64, int64, int64) {
	if !usage.Exists() || usage.Type == gjson.Null {
		return 0, 0, 0
	}

	inputTokens := usage.Get("prompt_tokens").Int()
	outputTokens := usage.Get("completion_tokens").Int()
	cachedTokens := usage.Get("prompt_tokens_details.cached_tokens").Int()

	if cachedTokens > 0 {
		if inputTokens >= cachedTokens {
			inputTokens -= cachedTokens
		} else {
			inputTokens = 0
		}
	}

	return inputTokens, outputTokens, cachedTokens
}
