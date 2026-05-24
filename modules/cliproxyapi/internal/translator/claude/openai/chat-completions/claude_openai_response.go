// chat_completions - claude_openai_response.go
// Claude 的 OpenAI Chat Completions 响应转换器。
// 负责将 Claude Code API 的响应转换为 OpenAI Chat Completions 兼容的 JSON 格式。
// 支持流式和非流式两种模式。
//
// 转换特性：
// - 流式模式：处理 Claude 的 SSE 事件类型（message_start、content_block_start/delta/stop、
//   message_delta、message_stop、ping、error）
// - 非流式模式：聚合所有 Claude SSE 事件到单个 OpenAI 响应
// - 工具调用：使用 ToolCallAccumulator 累积工具调用参数，在 content_block_stop 时完成
// - 推理内容：将 Claude 的 thinking_delta 转换为 reasoning_content
// - 用量映射：将 Claude 的 cache_read/cache_creation token 合并到 OpenAI 的 prompt_tokens 中
// - 停止原因映射：end_turn -> stop，tool_use -> tool_calls，max_tokens -> length
package chat_completions

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// dataTag 是 Claude SSE 事件的数据前缀标识。
var (
	dataTag = []byte("data:")
)

// ConvertAnthropicResponseToOpenAIParams 保存 Claude 响应到 OpenAI 格式转换过程中的状态参数。
type ConvertAnthropicResponseToOpenAIParams struct {
	// CreatedAt 响应创建时间的 Unix 时间戳
	CreatedAt int64
	// ResponseID 响应唯一标识符
	ResponseID string
	// FinishReason 停止原因（已映射为 OpenAI 格式）
	FinishReason string
	// Usage 用量统计数据
	Usage claudeUsageTokens
	// ToolCallsAccumulator 流式模式下的工具调用累积器，按内容块索引索引
	ToolCallsAccumulator map[int]*ToolCallAccumulator
}

// claudeUsageTokens 保存 Claude API 的 token 用量数据。
type claudeUsageTokens struct {
	// InputTokens 输入 token 数量
	InputTokens int64
	// OutputTokens 输出 token 数量
	OutputTokens int64
	// CacheCreationInputTokens 缓存创建输入 token 数量
	CacheCreationInputTokens int64
	// CacheReadInputTokens 缓存读取输入 token 数量
	CacheReadInputTokens int64
	// HasUsage 标记是否已接收到用量数据
	HasUsage bool
}

// ToolCallAccumulator 保存流式工具调用的累积状态。
type ToolCallAccumulator struct {
	// ID 工具调用的唯一标识符
	ID string
	// Name 工具名称
	Name string
	// Arguments 累积的工具调用参数 JSON 字符串
	Arguments strings.Builder
}

// Merge 将 Claude 的 usage 数据合并到当前用量统计中。
func (u *claudeUsageTokens) Merge(usage gjson.Result) {
	if !usage.Exists() {
		return
	}
	u.HasUsage = true
	if inputTokens := usage.Get("input_tokens"); inputTokens.Exists() {
		u.InputTokens = inputTokens.Int()
	}
	if outputTokens := usage.Get("output_tokens"); outputTokens.Exists() {
		u.OutputTokens = outputTokens.Int()
	}
	if cacheCreationInputTokens := usage.Get("cache_creation_input_tokens"); cacheCreationInputTokens.Exists() {
		u.CacheCreationInputTokens = cacheCreationInputTokens.Int()
	}
	if cacheReadInputTokens := usage.Get("cache_read_input_tokens"); cacheReadInputTokens.Exists() {
		u.CacheReadInputTokens = cacheReadInputTokens.Int()
	}
}

// OpenAIUsage 将 Claude 的 token 用量转换为 OpenAI 格式。
// 计算方式：prompt_tokens = input_tokens + cache_creation_input_tokens + cache_read_input_tokens
// cached_tokens = cache_read_input_tokens
func (u claudeUsageTokens) OpenAIUsage() (promptTokens, completionTokens, totalTokens, cachedTokens int64) {
	cachedTokens = u.CacheReadInputTokens
	promptTokens = u.InputTokens + u.CacheCreationInputTokens + cachedTokens
	completionTokens = u.OutputTokens
	totalTokens = promptTokens + completionTokens
	return promptTokens, completionTokens, totalTokens, cachedTokens
}

// ConvertClaudeResponseToOpenAI 将 Claude Code 的流式响应转换为 OpenAI Chat Completions 流式格式。
//
// 处理的 Claude 事件类型：
// - message_start: 初始化响应元数据（ID、模型、创建时间）
// - content_block_start: 开始新的内容块（text、tool_use、thinking）
// - content_block_delta: 增量内容更新（text_delta、thinking_delta、input_json_delta）
// - content_block_stop: 完成内容块，输出完整的工具调用
// - message_delta: 更新停止原因和用量数据
// - message_stop: 最终消息事件
// - ping: 心跳事件（忽略）
// - error: 错误事件
//
// 参数：
//   - ctx: 请求上下文（当前实现中未使用）
//   - modelName: 模型名称
//   - originalRequestRawJSON: 原始请求的 JSON 数据
//   - requestRawJSON: 经过转换的请求 JSON 数据
//   - rawJSON: Claude 格式的原始响应 JSON 数据（data: 前缀格式）
//   - param: 用于在多次调用之间保持状态的参数指针
//
// 返回值：
//   - [][]byte: OpenAI Chat Completions 格式的 SSE 事件数据切片
func ConvertClaudeResponseToOpenAI(_ context.Context, modelName string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) [][]byte {
	if *param == nil {
		*param = &ConvertAnthropicResponseToOpenAIParams{
			CreatedAt:    0,
			ResponseID:   "",
			FinishReason: "",
		}
	}

	if !bytes.HasPrefix(rawJSON, dataTag) {
		return [][]byte{}
	}
	rawJSON = bytes.TrimSpace(rawJSON[5:])

	root := gjson.ParseBytes(rawJSON)
	eventType := root.Get("type").String()

	// Base OpenAI streaming response template
	template := []byte(`{"id":"","object":"chat.completion.chunk","created":0,"model":"","choices":[{"index":0,"delta":{},"finish_reason":null}]}`)

	// Set model
	if modelName != "" {
		template, _ = sjson.SetBytes(template, "model", modelName)
	}

	// Set response ID and creation time
	if (*param).(*ConvertAnthropicResponseToOpenAIParams).ResponseID != "" {
		template, _ = sjson.SetBytes(template, "id", (*param).(*ConvertAnthropicResponseToOpenAIParams).ResponseID)
	}
	if (*param).(*ConvertAnthropicResponseToOpenAIParams).CreatedAt > 0 {
		template, _ = sjson.SetBytes(template, "created", (*param).(*ConvertAnthropicResponseToOpenAIParams).CreatedAt)
	}

	switch eventType {
	case "message_start":
		// Initialize response with message metadata when a new message begins
		if message := root.Get("message"); message.Exists() {
			(*param).(*ConvertAnthropicResponseToOpenAIParams).ResponseID = message.Get("id").String()
			(*param).(*ConvertAnthropicResponseToOpenAIParams).CreatedAt = time.Now().Unix()

			template, _ = sjson.SetBytes(template, "id", (*param).(*ConvertAnthropicResponseToOpenAIParams).ResponseID)
			template, _ = sjson.SetBytes(template, "model", modelName)
			template, _ = sjson.SetBytes(template, "created", (*param).(*ConvertAnthropicResponseToOpenAIParams).CreatedAt)

			// Set initial role to assistant for the response
			template, _ = sjson.SetBytes(template, "choices.0.delta.role", "assistant")

			// Initialize tool calls accumulator for tracking tool call progress
			if (*param).(*ConvertAnthropicResponseToOpenAIParams).ToolCallsAccumulator == nil {
				(*param).(*ConvertAnthropicResponseToOpenAIParams).ToolCallsAccumulator = make(map[int]*ToolCallAccumulator)
			}
			(*param).(*ConvertAnthropicResponseToOpenAIParams).Usage.Merge(message.Get("usage"))
		}
		return [][]byte{template}

	case "content_block_start":
		// Start of a content block (text, tool use, or reasoning)
		if contentBlock := root.Get("content_block"); contentBlock.Exists() {
			blockType := contentBlock.Get("type").String()

			if blockType == "tool_use" {
				// Start of tool call - initialize accumulator to track arguments
				toolCallID := contentBlock.Get("id").String()
				toolName := contentBlock.Get("name").String()
				index := int(root.Get("index").Int())

				if (*param).(*ConvertAnthropicResponseToOpenAIParams).ToolCallsAccumulator == nil {
					(*param).(*ConvertAnthropicResponseToOpenAIParams).ToolCallsAccumulator = make(map[int]*ToolCallAccumulator)
				}

				(*param).(*ConvertAnthropicResponseToOpenAIParams).ToolCallsAccumulator[index] = &ToolCallAccumulator{
					ID:   toolCallID,
					Name: toolName,
				}

				// Don't output anything yet - wait for complete tool call
				return [][]byte{}
			}
		}
		return [][]byte{}

	case "content_block_delta":
		// Handle content delta (text, tool use arguments, or reasoning content)
		hasContent := false
		if delta := root.Get("delta"); delta.Exists() {
			deltaType := delta.Get("type").String()

			switch deltaType {
			case "text_delta":
				// Text content delta - send incremental text updates
				if text := delta.Get("text"); text.Exists() {
					template, _ = sjson.SetBytes(template, "choices.0.delta.content", text.String())
					hasContent = true
				}
			case "thinking_delta":
				// Accumulate reasoning/thinking content
				if thinking := delta.Get("thinking"); thinking.Exists() {
					template, _ = sjson.SetBytes(template, "choices.0.delta.reasoning_content", thinking.String())
					hasContent = true
				}
			case "input_json_delta":
				// Tool use input delta - accumulate arguments for tool calls
				if partialJSON := delta.Get("partial_json"); partialJSON.Exists() {
					index := int(root.Get("index").Int())
					if (*param).(*ConvertAnthropicResponseToOpenAIParams).ToolCallsAccumulator != nil {
						if accumulator, exists := (*param).(*ConvertAnthropicResponseToOpenAIParams).ToolCallsAccumulator[index]; exists {
							accumulator.Arguments.WriteString(partialJSON.String())
						}
					}
				}
				// Don't output anything yet - wait for complete tool call
				return [][]byte{}
			}
		}
		if hasContent {
			return [][]byte{template}
		} else {
			return [][]byte{}
		}

	case "content_block_stop":
		// End of content block - output complete tool call if it's a tool_use block
		index := int(root.Get("index").Int())
		if (*param).(*ConvertAnthropicResponseToOpenAIParams).ToolCallsAccumulator != nil {
			if accumulator, exists := (*param).(*ConvertAnthropicResponseToOpenAIParams).ToolCallsAccumulator[index]; exists {
				// Build complete tool call with accumulated arguments
				arguments := accumulator.Arguments.String()
				if arguments == "" {
					arguments = "{}"
				}
				template, _ = sjson.SetBytes(template, "choices.0.delta.tool_calls.0.index", index)
				template, _ = sjson.SetBytes(template, "choices.0.delta.tool_calls.0.id", accumulator.ID)
				template, _ = sjson.SetBytes(template, "choices.0.delta.tool_calls.0.type", "function")
				template, _ = sjson.SetBytes(template, "choices.0.delta.tool_calls.0.function.name", accumulator.Name)
				template, _ = sjson.SetBytes(template, "choices.0.delta.tool_calls.0.function.arguments", arguments)

				// Clean up the accumulator for this index
				delete((*param).(*ConvertAnthropicResponseToOpenAIParams).ToolCallsAccumulator, index)

				return [][]byte{template}
			}
		}
		return [][]byte{}

	case "message_delta":
		// Handle message-level changes including stop reason and usage
		if delta := root.Get("delta"); delta.Exists() {
			if stopReason := delta.Get("stop_reason"); stopReason.Exists() {
				(*param).(*ConvertAnthropicResponseToOpenAIParams).FinishReason = mapAnthropicStopReasonToOpenAI(stopReason.String())
				template, _ = sjson.SetBytes(template, "choices.0.finish_reason", (*param).(*ConvertAnthropicResponseToOpenAIParams).FinishReason)
			}
		}

		// Handle usage information for token counts
		if usage := root.Get("usage"); usage.Exists() {
			(*param).(*ConvertAnthropicResponseToOpenAIParams).Usage.Merge(usage)
			promptTokens, completionTokens, totalTokens, cachedTokens := (*param).(*ConvertAnthropicResponseToOpenAIParams).Usage.OpenAIUsage()
			template, _ = sjson.SetBytes(template, "usage.prompt_tokens", promptTokens)
			template, _ = sjson.SetBytes(template, "usage.completion_tokens", completionTokens)
			template, _ = sjson.SetBytes(template, "usage.total_tokens", totalTokens)
			template, _ = sjson.SetBytes(template, "usage.prompt_tokens_details.cached_tokens", cachedTokens)
		}
		return [][]byte{template}

	case "message_stop":
		// Final message event - no additional output needed
		return [][]byte{}

	case "ping":
		// Ping events for keeping connection alive - no output needed
		return [][]byte{}

	case "error":
		// Error event - format and return error response
		if errorData := root.Get("error"); errorData.Exists() {
			errorJSON := []byte(`{"error":{"message":"","type":""}}`)
			errorJSON, _ = sjson.SetBytes(errorJSON, "error.message", errorData.Get("message").String())
			errorJSON, _ = sjson.SetBytes(errorJSON, "error.type", errorData.Get("type").String())
			return [][]byte{errorJSON}
		}
		return [][]byte{}

	default:
		// Unknown event type - ignore
		return [][]byte{}
	}
}

// mapAnthropicStopReasonToOpenAI 将 Anthropic 的停止原因映射为 OpenAI 的停止原因。
// 映射规则：end_turn -> stop，tool_use -> tool_calls，max_tokens -> length，stop_sequence -> stop
func mapAnthropicStopReasonToOpenAI(anthropicReason string) string {
	switch anthropicReason {
	case "end_turn":
		return "stop"
	case "tool_use":
		return "tool_calls"
	case "max_tokens":
		return "length"
	case "stop_sequence":
		return "stop"
	default:
		return "stop"
	}
}

// ConvertClaudeResponseToOpenAINonStream 将 Claude 的非流式响应转换为 OpenAI Chat Completions 格式。
// 遍历所有 Claude SSE 数据行，聚合文本、推理和工具调用内容，构建完整的 OpenAI 响应。
//
// 参数：
//   - ctx: 请求上下文（当前实现中未使用）
//   - modelName: 模型名称（当前实现中未使用）
//   - originalRequestRawJSON: 原始请求的 JSON 数据
//   - requestRawJSON: 经过转换的请求 JSON 数据
//   - rawJSON: Claude 格式的原始响应数据（多行 data: 格式）
//   - _: 未使用的参数指针
//
// 返回值：
//   - []byte: OpenAI Chat Completions 格式的完整 JSON 响应数据
func ConvertClaudeResponseToOpenAINonStream(_ context.Context, _ string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, _ *any) []byte {
	chunks := make([][]byte, 0)

	lines := bytes.Split(rawJSON, []byte("\n"))
	for _, line := range lines {
		if !bytes.HasPrefix(line, dataTag) {
			continue
		}
		chunks = append(chunks, bytes.TrimSpace(line[5:]))
	}

	// Base OpenAI non-streaming response template
	out := []byte(`{"id":"","object":"chat.completion","created":0,"model":"","choices":[{"index":0,"message":{"role":"assistant","content":""},"finish_reason":"stop"}],"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}}`)

	var messageID string
	var model string
	var createdAt int64
	var stopReason string
	var contentParts []string
	var reasoningParts []string
	usageTokens := claudeUsageTokens{}
	toolCallsAccumulator := make(map[int]*ToolCallAccumulator)

	for _, chunk := range chunks {
		root := gjson.ParseBytes(chunk)
		eventType := root.Get("type").String()

		switch eventType {
		case "message_start":
			// Extract initial message metadata including ID, model, and input token count
			if message := root.Get("message"); message.Exists() {
				messageID = message.Get("id").String()
				model = message.Get("model").String()
				createdAt = time.Now().Unix()
				usageTokens.Merge(message.Get("usage"))
			}

		case "content_block_start":
			// Handle different content block types at the beginning
			if contentBlock := root.Get("content_block"); contentBlock.Exists() {
				blockType := contentBlock.Get("type").String()
				if blockType == "thinking" {
					// Start of thinking/reasoning content - skip for now as it's handled in delta
					continue
				} else if blockType == "tool_use" {
					// Initialize tool call accumulator for this index
					index := int(root.Get("index").Int())
					toolCallsAccumulator[index] = &ToolCallAccumulator{
						ID:   contentBlock.Get("id").String(),
						Name: contentBlock.Get("name").String(),
					}
				}
			}

		case "content_block_delta":
			// Process incremental content updates
			if delta := root.Get("delta"); delta.Exists() {
				deltaType := delta.Get("type").String()
				switch deltaType {
				case "text_delta":
					// Accumulate text content
					if text := delta.Get("text"); text.Exists() {
						contentParts = append(contentParts, text.String())
					}
				case "thinking_delta":
					// Accumulate reasoning/thinking content
					if thinking := delta.Get("thinking"); thinking.Exists() {
						reasoningParts = append(reasoningParts, thinking.String())
					}
				case "input_json_delta":
					// Accumulate tool call arguments
					if partialJSON := delta.Get("partial_json"); partialJSON.Exists() {
						index := int(root.Get("index").Int())
						if accumulator, exists := toolCallsAccumulator[index]; exists {
							accumulator.Arguments.WriteString(partialJSON.String())
						}
					}
				}
			}

		case "content_block_stop":
			// Finalize tool call arguments for this index when content block ends
			index := int(root.Get("index").Int())
			if accumulator, exists := toolCallsAccumulator[index]; exists {
				if accumulator.Arguments.Len() == 0 {
					accumulator.Arguments.WriteString("{}")
				}
			}

		case "message_delta":
			// Extract stop reason and output token count when message ends
			if delta := root.Get("delta"); delta.Exists() {
				if sr := delta.Get("stop_reason"); sr.Exists() {
					stopReason = sr.String()
				}
			}
			if usage := root.Get("usage"); usage.Exists() {
				usageTokens.Merge(usage)
			}
		}
	}

	if usageTokens.HasUsage {
		promptTokens, completionTokens, totalTokens, cachedTokens := usageTokens.OpenAIUsage()
		out, _ = sjson.SetBytes(out, "usage.prompt_tokens", promptTokens)
		out, _ = sjson.SetBytes(out, "usage.completion_tokens", completionTokens)
		out, _ = sjson.SetBytes(out, "usage.total_tokens", totalTokens)
		out, _ = sjson.SetBytes(out, "usage.prompt_tokens_details.cached_tokens", cachedTokens)
	}

	// Set basic response fields including message ID, creation time, and model
	out, _ = sjson.SetBytes(out, "id", messageID)
	out, _ = sjson.SetBytes(out, "created", createdAt)
	out, _ = sjson.SetBytes(out, "model", model)

	// Set message content by combining all text parts
	messageContent := strings.Join(contentParts, "")
	out, _ = sjson.SetBytes(out, "choices.0.message.content", messageContent)

	// Add reasoning content if available (following OpenAI reasoning format)
	if len(reasoningParts) > 0 {
		reasoningContent := strings.Join(reasoningParts, "")
		// Add reasoning as a separate field in the message
		out, _ = sjson.SetBytes(out, "choices.0.message.reasoning", reasoningContent)
	}

	// Set tool calls if any were accumulated during processing
	if len(toolCallsAccumulator) > 0 {
		toolCallsCount := 0
		maxIndex := -1
		for index := range toolCallsAccumulator {
			if index > maxIndex {
				maxIndex = index
			}
		}

		for i := 0; i <= maxIndex; i++ {
			accumulator, exists := toolCallsAccumulator[i]
			if !exists {
				continue
			}

			arguments := accumulator.Arguments.String()

			idPath := fmt.Sprintf("choices.0.message.tool_calls.%d.id", toolCallsCount)
			typePath := fmt.Sprintf("choices.0.message.tool_calls.%d.type", toolCallsCount)
			namePath := fmt.Sprintf("choices.0.message.tool_calls.%d.function.name", toolCallsCount)
			argumentsPath := fmt.Sprintf("choices.0.message.tool_calls.%d.function.arguments", toolCallsCount)

			out, _ = sjson.SetBytes(out, idPath, accumulator.ID)
			out, _ = sjson.SetBytes(out, typePath, "function")
			out, _ = sjson.SetBytes(out, namePath, accumulator.Name)
			out, _ = sjson.SetBytes(out, argumentsPath, arguments)
			toolCallsCount++
		}
		if toolCallsCount > 0 {
			out, _ = sjson.SetBytes(out, "choices.0.finish_reason", "tool_calls")
		} else {
			out, _ = sjson.SetBytes(out, "choices.0.finish_reason", mapAnthropicStopReasonToOpenAI(stopReason))
		}
	} else {
		out, _ = sjson.SetBytes(out, "choices.0.finish_reason", mapAnthropicStopReasonToOpenAI(stopReason))
	}

	return out
}
