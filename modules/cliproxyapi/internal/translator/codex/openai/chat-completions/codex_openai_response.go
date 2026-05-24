// chat_completions - codex_openai_response.go
// Codex 的 OpenAI Chat Completions 格式响应转换器。
// 负责将 Codex API 的响应转换为 OpenAI Chat Completions 兼容的 JSON 格式。
// 支持流式和非流式两种模式。
//
// 转换特性：
// - 流式模式：处理 Codex 的 SSE 事件类型（response.created、response.output_text.delta、
//   response.reasoning_summary_text.delta/done、response.function_call_arguments.delta/done、
//   response.output_item.added/done、response.image_generation_call.partial_image、response.completed）
// - 非流式模式：从 response.completed 事件中聚合所有内容到单个 OpenAI 响应
// - 工具调用：累积函数调用参数，支持 output_item.added 和 output_item.done 两种路径
// - 推理内容：将 Codex 的 reasoning_summary_text 转换为 reasoning_content
// - 图片生成：将 Codex 的 image_generation_call 转换为 delta.images 格式，支持 SHA256 去重
// - 工具名称恢复：将截断的工具名称通过反向映射表恢复为原始名称
package chat_completions

import (
	"bytes"
	"context"
	"crypto/sha256"
	"strings"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// dataTag 是 Codex SSE 事件的数据前缀标识。
var (
	dataTag = []byte("data:")
)

// ConvertCliToOpenAIParams 保存 Codex 响应到 OpenAI 格式转换过程中的状态参数。
type ConvertCliToOpenAIParams struct {
	// ResponseID 响应唯一标识符
	ResponseID string
	// CreatedAt 响应创建时间的 Unix 时间戳
	CreatedAt int64
	// Model 模型名称
	Model string
	// FunctionCallIndex 当前函数调用的索引（-1 表示尚未开始）
	FunctionCallIndex int
	// HasReceivedArgumentsDelta 标记是否已接收到函数调用参数的增量更新
	HasReceivedArgumentsDelta bool
	// HasToolCallAnnounced 标记工具调用是否已通过 output_item.added 事件宣布
	HasToolCallAnnounced bool
	// LastImageHashByItemID 按项目 ID 索引的最后图片 SHA256 哈希，用于去重
	LastImageHashByItemID map[string][32]byte
}

// ConvertCodexResponseToOpenAI 将 Codex 的流式响应转换为 OpenAI Chat Completions 流式格式。
//
// 处理的 Codex 事件类型：
// - response.created: 初始化响应元数据（ID、模型、创建时间）
// - response.output_text.delta: 增量文本内容
// - response.reasoning_summary_text.delta/done: 推理/思考内容
// - response.output_item.added: 新的输出项（function_call 或 image_generation_call）
// - response.function_call_arguments.delta/done: 函数调用参数增量
// - response.output_item.done: 输出项完成
// - response.image_generation_call.partial_image: 图片生成增量
// - response.completed: 响应完成（设置停止原因和用量）
//
// 参数：
//   - ctx: 请求上下文（当前实现中未使用）
//   - modelName: 模型名称
//   - originalRequestRawJSON: 原始请求的 JSON 数据（用于工具名称恢复）
//   - rawJSON: Codex 格式的原始响应 JSON 数据（data: 前缀格式）
//   - param: 用于在多次调用之间保持状态的参数指针
//
// 返回值：
//   - [][]byte: OpenAI Chat Completions 格式的 SSE 事件数据切片
func ConvertCodexResponseToOpenAI(_ context.Context, modelName string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) [][]byte {
	if *param == nil {
		*param = &ConvertCliToOpenAIParams{
			Model:                     modelName,
			CreatedAt:                 0,
			ResponseID:                "",
			FunctionCallIndex:         -1,
			HasReceivedArgumentsDelta: false,
			HasToolCallAnnounced:      false,
			LastImageHashByItemID:     make(map[string][32]byte),
		}
	}

	if !bytes.HasPrefix(rawJSON, dataTag) {
		return [][]byte{}
	}
	rawJSON = bytes.TrimSpace(rawJSON[5:])

	// Initialize the OpenAI SSE template.
	template := []byte(`{"id":"","object":"chat.completion.chunk","created":12345,"model":"model","choices":[{"index":0,"delta":{},"finish_reason":null,"native_finish_reason":null}]}`)

	rootResult := gjson.ParseBytes(rawJSON)

	typeResult := rootResult.Get("type")
	dataType := typeResult.String()
	if dataType == "response.created" {
		(*param).(*ConvertCliToOpenAIParams).ResponseID = rootResult.Get("response.id").String()
		(*param).(*ConvertCliToOpenAIParams).CreatedAt = rootResult.Get("response.created_at").Int()
		(*param).(*ConvertCliToOpenAIParams).Model = rootResult.Get("response.model").String()
		if (*param).(*ConvertCliToOpenAIParams).LastImageHashByItemID == nil {
			(*param).(*ConvertCliToOpenAIParams).LastImageHashByItemID = make(map[string][32]byte)
		}
		return [][]byte{}
	}

	// Extract and set the model version.
	cachedModel := (*param).(*ConvertCliToOpenAIParams).Model
	if modelResult := gjson.GetBytes(rawJSON, "model"); modelResult.Exists() {
		template, _ = sjson.SetBytes(template, "model", modelResult.String())
	} else if cachedModel != "" {
		template, _ = sjson.SetBytes(template, "model", cachedModel)
	} else if modelName != "" {
		template, _ = sjson.SetBytes(template, "model", modelName)
	}

	template, _ = sjson.SetBytes(template, "created", (*param).(*ConvertCliToOpenAIParams).CreatedAt)

	// Extract and set the response ID.
	template, _ = sjson.SetBytes(template, "id", (*param).(*ConvertCliToOpenAIParams).ResponseID)

	// Extract and set usage metadata (token counts).
	if usageResult := gjson.GetBytes(rawJSON, "response.usage"); usageResult.Exists() {
		if outputTokensResult := usageResult.Get("output_tokens"); outputTokensResult.Exists() {
			template, _ = sjson.SetBytes(template, "usage.completion_tokens", outputTokensResult.Int())
		}
		if totalTokensResult := usageResult.Get("total_tokens"); totalTokensResult.Exists() {
			template, _ = sjson.SetBytes(template, "usage.total_tokens", totalTokensResult.Int())
		}
		if inputTokensResult := usageResult.Get("input_tokens"); inputTokensResult.Exists() {
			template, _ = sjson.SetBytes(template, "usage.prompt_tokens", inputTokensResult.Int())
		}
		if cachedTokensResult := usageResult.Get("input_tokens_details.cached_tokens"); cachedTokensResult.Exists() {
			template, _ = sjson.SetBytes(template, "usage.prompt_tokens_details.cached_tokens", cachedTokensResult.Int())
		}
		if reasoningTokensResult := usageResult.Get("output_tokens_details.reasoning_tokens"); reasoningTokensResult.Exists() {
			template, _ = sjson.SetBytes(template, "usage.completion_tokens_details.reasoning_tokens", reasoningTokensResult.Int())
		}
	}

	if dataType == "response.reasoning_summary_text.delta" {
		if deltaResult := rootResult.Get("delta"); deltaResult.Exists() {
			template, _ = sjson.SetBytes(template, "choices.0.delta.role", "assistant")
			template, _ = sjson.SetBytes(template, "choices.0.delta.reasoning_content", deltaResult.String())
		}
	} else if dataType == "response.reasoning_summary_text.done" {
		template, _ = sjson.SetBytes(template, "choices.0.delta.role", "assistant")
		template, _ = sjson.SetBytes(template, "choices.0.delta.reasoning_content", "\n\n")
	} else if dataType == "response.output_text.delta" {
		if deltaResult := rootResult.Get("delta"); deltaResult.Exists() {
			template, _ = sjson.SetBytes(template, "choices.0.delta.role", "assistant")
			template, _ = sjson.SetBytes(template, "choices.0.delta.content", deltaResult.String())
		}
	} else if dataType == "response.image_generation_call.partial_image" {
		itemID := rootResult.Get("item_id").String()
		b64 := rootResult.Get("partial_image_b64").String()
		if b64 == "" {
			return [][]byte{}
		}
		if itemID != "" {
			p := (*param).(*ConvertCliToOpenAIParams)
			if p.LastImageHashByItemID == nil {
				p.LastImageHashByItemID = make(map[string][32]byte)
			}
			hash := sha256.Sum256([]byte(b64))
			if last, ok := p.LastImageHashByItemID[itemID]; ok && last == hash {
				return [][]byte{}
			}
			p.LastImageHashByItemID[itemID] = hash
		}

		outputFormat := rootResult.Get("output_format").String()
		mimeType := mimeTypeFromCodexOutputFormat(outputFormat)
		imageURL := "data:" + mimeType + ";base64," + b64

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
	} else if dataType == "response.completed" {
		finishReason := "stop"
		if (*param).(*ConvertCliToOpenAIParams).FunctionCallIndex != -1 {
			finishReason = "tool_calls"
		}
		template, _ = sjson.SetBytes(template, "choices.0.finish_reason", finishReason)
		template, _ = sjson.SetBytes(template, "choices.0.native_finish_reason", finishReason)
	} else if dataType == "response.output_item.added" {
		itemResult := rootResult.Get("item")
		if !itemResult.Exists() || itemResult.Get("type").String() != "function_call" {
			return [][]byte{}
		}

		// Increment index for this new function call item.
		(*param).(*ConvertCliToOpenAIParams).FunctionCallIndex++
		(*param).(*ConvertCliToOpenAIParams).HasReceivedArgumentsDelta = false
		(*param).(*ConvertCliToOpenAIParams).HasToolCallAnnounced = true

		functionCallItemTemplate := []byte(`{"index":0,"id":"","type":"function","function":{"name":"","arguments":""}}`)
		functionCallItemTemplate, _ = sjson.SetBytes(functionCallItemTemplate, "index", (*param).(*ConvertCliToOpenAIParams).FunctionCallIndex)
		functionCallItemTemplate, _ = sjson.SetBytes(functionCallItemTemplate, "id", itemResult.Get("call_id").String())

		// Restore original tool name if it was shortened.
		name := itemResult.Get("name").String()
		rev := buildReverseMapFromOriginalOpenAI(originalRequestRawJSON)
		if orig, ok := rev[name]; ok {
			name = orig
		}
		functionCallItemTemplate, _ = sjson.SetBytes(functionCallItemTemplate, "function.name", name)
		functionCallItemTemplate, _ = sjson.SetBytes(functionCallItemTemplate, "function.arguments", "")

		template, _ = sjson.SetBytes(template, "choices.0.delta.role", "assistant")
		template, _ = sjson.SetRawBytes(template, "choices.0.delta.tool_calls", []byte(`[]`))
		template, _ = sjson.SetRawBytes(template, "choices.0.delta.tool_calls.-1", functionCallItemTemplate)

	} else if dataType == "response.function_call_arguments.delta" {
		(*param).(*ConvertCliToOpenAIParams).HasReceivedArgumentsDelta = true

		deltaValue := rootResult.Get("delta").String()
		functionCallItemTemplate := []byte(`{"index":0,"function":{"arguments":""}}`)
		functionCallItemTemplate, _ = sjson.SetBytes(functionCallItemTemplate, "index", (*param).(*ConvertCliToOpenAIParams).FunctionCallIndex)
		functionCallItemTemplate, _ = sjson.SetBytes(functionCallItemTemplate, "function.arguments", deltaValue)

		template, _ = sjson.SetRawBytes(template, "choices.0.delta.tool_calls", []byte(`[]`))
		template, _ = sjson.SetRawBytes(template, "choices.0.delta.tool_calls.-1", functionCallItemTemplate)

	} else if dataType == "response.function_call_arguments.done" {
		if (*param).(*ConvertCliToOpenAIParams).HasReceivedArgumentsDelta {
			// Arguments were already streamed via delta events; nothing to emit.
			return [][]byte{}
		}

		// Fallback: no delta events were received, emit the full arguments as a single chunk.
		fullArgs := rootResult.Get("arguments").String()
		functionCallItemTemplate := []byte(`{"index":0,"function":{"arguments":""}}`)
		functionCallItemTemplate, _ = sjson.SetBytes(functionCallItemTemplate, "index", (*param).(*ConvertCliToOpenAIParams).FunctionCallIndex)
		functionCallItemTemplate, _ = sjson.SetBytes(functionCallItemTemplate, "function.arguments", fullArgs)

		template, _ = sjson.SetRawBytes(template, "choices.0.delta.tool_calls", []byte(`[]`))
		template, _ = sjson.SetRawBytes(template, "choices.0.delta.tool_calls.-1", functionCallItemTemplate)

	} else if dataType == "response.output_item.done" {
		itemResult := rootResult.Get("item")
		if !itemResult.Exists() {
			return [][]byte{}
		}
		itemType := itemResult.Get("type").String()
		if itemType == "image_generation_call" {
			itemID := itemResult.Get("id").String()
			b64 := itemResult.Get("result").String()
			if b64 == "" {
				return [][]byte{}
			}
			if itemID != "" {
				p := (*param).(*ConvertCliToOpenAIParams)
				if p.LastImageHashByItemID == nil {
					p.LastImageHashByItemID = make(map[string][32]byte)
				}
				hash := sha256.Sum256([]byte(b64))
				if last, ok := p.LastImageHashByItemID[itemID]; ok && last == hash {
					return [][]byte{}
				}
				p.LastImageHashByItemID[itemID] = hash
			}

			outputFormat := itemResult.Get("output_format").String()
			mimeType := mimeTypeFromCodexOutputFormat(outputFormat)
			imageURL := "data:" + mimeType + ";base64," + b64

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
			return [][]byte{template}
		}
		if itemType != "function_call" {
			return [][]byte{}
		}

		if (*param).(*ConvertCliToOpenAIParams).HasToolCallAnnounced {
			// Tool call was already announced via output_item.added; skip emission.
			(*param).(*ConvertCliToOpenAIParams).HasToolCallAnnounced = false
			return [][]byte{}
		}

		// Fallback path: model skipped output_item.added, so emit complete tool call now.
		(*param).(*ConvertCliToOpenAIParams).FunctionCallIndex++

		functionCallItemTemplate := []byte(`{"index":0,"id":"","type":"function","function":{"name":"","arguments":""}}`)
		functionCallItemTemplate, _ = sjson.SetBytes(functionCallItemTemplate, "index", (*param).(*ConvertCliToOpenAIParams).FunctionCallIndex)

		template, _ = sjson.SetRawBytes(template, "choices.0.delta.tool_calls", []byte(`[]`))
		functionCallItemTemplate, _ = sjson.SetBytes(functionCallItemTemplate, "id", itemResult.Get("call_id").String())

		// Restore original tool name if it was shortened.
		name := itemResult.Get("name").String()
		rev := buildReverseMapFromOriginalOpenAI(originalRequestRawJSON)
		if orig, ok := rev[name]; ok {
			name = orig
		}
		functionCallItemTemplate, _ = sjson.SetBytes(functionCallItemTemplate, "function.name", name)

		functionCallItemTemplate, _ = sjson.SetBytes(functionCallItemTemplate, "function.arguments", itemResult.Get("arguments").String())
		template, _ = sjson.SetBytes(template, "choices.0.delta.role", "assistant")
		template, _ = sjson.SetRawBytes(template, "choices.0.delta.tool_calls.-1", functionCallItemTemplate)

	} else {
		return [][]byte{}
	}

	return [][]byte{template}
}

// ConvertCodexResponseToOpenAINonStream 将 Codex 的非流式响应转换为 OpenAI Chat Completions 格式。
// 从 response.completed 事件中提取所有内容，包括文本、推理、工具调用和图片，
// 构建完整的 OpenAI 响应。
//
// 参数：
//   - ctx: 请求上下文（当前实现中未使用）
//   - modelName: 模型名称（当前实现中未使用）
//   - originalRequestRawJSON: 原始请求的 JSON 数据（用于工具名称恢复）
//   - rawJSON: Codex 格式的原始响应数据
//
// 返回值：
//   - []byte: OpenAI Chat Completions 格式的完整 JSON 响应数据
func ConvertCodexResponseToOpenAINonStream(_ context.Context, _ string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, _ *any) []byte {
	rootResult := gjson.ParseBytes(rawJSON)
	// Verify this is a response.completed event
	if rootResult.Get("type").String() != "response.completed" {
		return []byte{}
	}

	unixTimestamp := time.Now().Unix()

	responseResult := rootResult.Get("response")

	template := []byte(`{"id":"","object":"chat.completion","created":123456,"model":"model","choices":[{"index":0,"message":{"role":"assistant","content":null,"reasoning_content":null,"tool_calls":null},"finish_reason":null,"native_finish_reason":null}]}`)

	// Extract and set the model version.
	if modelResult := responseResult.Get("model"); modelResult.Exists() {
		template, _ = sjson.SetBytes(template, "model", modelResult.String())
	}

	// Extract and set the creation timestamp.
	if createdAtResult := responseResult.Get("created_at"); createdAtResult.Exists() {
		template, _ = sjson.SetBytes(template, "created", createdAtResult.Int())
	} else {
		template, _ = sjson.SetBytes(template, "created", unixTimestamp)
	}

	// Extract and set the response ID.
	if idResult := responseResult.Get("id"); idResult.Exists() {
		template, _ = sjson.SetBytes(template, "id", idResult.String())
	}

	// Extract and set usage metadata (token counts).
	if usageResult := responseResult.Get("usage"); usageResult.Exists() {
		if outputTokensResult := usageResult.Get("output_tokens"); outputTokensResult.Exists() {
			template, _ = sjson.SetBytes(template, "usage.completion_tokens", outputTokensResult.Int())
		}
		if totalTokensResult := usageResult.Get("total_tokens"); totalTokensResult.Exists() {
			template, _ = sjson.SetBytes(template, "usage.total_tokens", totalTokensResult.Int())
		}
		if inputTokensResult := usageResult.Get("input_tokens"); inputTokensResult.Exists() {
			template, _ = sjson.SetBytes(template, "usage.prompt_tokens", inputTokensResult.Int())
		}
		if cachedTokensResult := usageResult.Get("input_tokens_details.cached_tokens"); cachedTokensResult.Exists() {
			template, _ = sjson.SetBytes(template, "usage.prompt_tokens_details.cached_tokens", cachedTokensResult.Int())
		}
		if reasoningTokensResult := usageResult.Get("output_tokens_details.reasoning_tokens"); reasoningTokensResult.Exists() {
			template, _ = sjson.SetBytes(template, "usage.completion_tokens_details.reasoning_tokens", reasoningTokensResult.Int())
		}
	}

	// Process the output array for content and function calls
	var toolCalls [][]byte
	var images [][]byte
	outputResult := responseResult.Get("output")
	if outputResult.IsArray() {
		outputArray := outputResult.Array()
		var contentText string
		var reasoningText string

		for _, outputItem := range outputArray {
			outputType := outputItem.Get("type").String()

			switch outputType {
			case "reasoning":
				// Extract reasoning content from summary
				if summaryResult := outputItem.Get("summary"); summaryResult.IsArray() {
					summaryArray := summaryResult.Array()
					for _, summaryItem := range summaryArray {
						if summaryItem.Get("type").String() == "summary_text" {
							reasoningText = summaryItem.Get("text").String()
							break
						}
					}
				}
			case "message":
				// Extract message content
				if contentResult := outputItem.Get("content"); contentResult.IsArray() {
					contentArray := contentResult.Array()
					for _, contentItem := range contentArray {
						if contentItem.Get("type").String() == "output_text" {
							contentText = contentItem.Get("text").String()
							break
						}
					}
				}
			case "function_call":
				// Handle function call content
				functionCallTemplate := []byte(`{"id":"","type":"function","function":{"name":"","arguments":""}}`)

				if callIdResult := outputItem.Get("call_id"); callIdResult.Exists() {
					functionCallTemplate, _ = sjson.SetBytes(functionCallTemplate, "id", callIdResult.String())
				}

				if nameResult := outputItem.Get("name"); nameResult.Exists() {
					n := nameResult.String()
					rev := buildReverseMapFromOriginalOpenAI(originalRequestRawJSON)
					if orig, ok := rev[n]; ok {
						n = orig
					}
					functionCallTemplate, _ = sjson.SetBytes(functionCallTemplate, "function.name", n)
				}

				if argsResult := outputItem.Get("arguments"); argsResult.Exists() {
					functionCallTemplate, _ = sjson.SetBytes(functionCallTemplate, "function.arguments", argsResult.String())
				}

				toolCalls = append(toolCalls, functionCallTemplate)
			case "image_generation_call":
				b64 := outputItem.Get("result").String()
				if b64 == "" {
					break
				}
				outputFormat := outputItem.Get("output_format").String()
				mimeType := mimeTypeFromCodexOutputFormat(outputFormat)
				imageURL := "data:" + mimeType + ";base64," + b64

				imagePayload := []byte(`{"type":"image_url","image_url":{"url":""}}`)
				imagePayload, _ = sjson.SetBytes(imagePayload, "index", len(images))
				imagePayload, _ = sjson.SetBytes(imagePayload, "image_url.url", imageURL)
				images = append(images, imagePayload)
			}
		}

		// Set content and reasoning content if found
		if contentText != "" {
			template, _ = sjson.SetBytes(template, "choices.0.message.content", contentText)
			template, _ = sjson.SetBytes(template, "choices.0.message.role", "assistant")
		}

		if reasoningText != "" {
			template, _ = sjson.SetBytes(template, "choices.0.message.reasoning_content", reasoningText)
			template, _ = sjson.SetBytes(template, "choices.0.message.role", "assistant")
		}

		// Add tool calls if any
		if len(toolCalls) > 0 {
			template, _ = sjson.SetRawBytes(template, "choices.0.message.tool_calls", []byte(`[]`))
			for _, toolCall := range toolCalls {
				template, _ = sjson.SetRawBytes(template, "choices.0.message.tool_calls.-1", toolCall)
			}
			template, _ = sjson.SetBytes(template, "choices.0.message.role", "assistant")
		}

		// Add images if any
		if len(images) > 0 {
			template, _ = sjson.SetRawBytes(template, "choices.0.message.images", []byte(`[]`))
			for _, image := range images {
				template, _ = sjson.SetRawBytes(template, "choices.0.message.images.-1", image)
			}
			template, _ = sjson.SetBytes(template, "choices.0.message.role", "assistant")
		}
	}

	// Extract and set the finish reason based on status
	if statusResult := responseResult.Get("status"); statusResult.Exists() {
		status := statusResult.String()
		if status == "completed" {
			finishReason := "stop"
			if len(toolCalls) > 0 {
				finishReason = "tool_calls"
			}
			template, _ = sjson.SetBytes(template, "choices.0.finish_reason", finishReason)
			template, _ = sjson.SetBytes(template, "choices.0.native_finish_reason", finishReason)
		}
	}

	return template
}

// buildReverseMapFromOriginalOpenAI 从原始 OpenAI 风格的请求 JSON 构建截断工具名称到原始名称的反向映射。
// 使用与请求转换相同的截断逻辑，确保名称恢复的准确性。
func buildReverseMapFromOriginalOpenAI(original []byte) map[string]string {
	tools := gjson.GetBytes(original, "tools")
	rev := map[string]string{}
	if tools.IsArray() && len(tools.Array()) > 0 {
		var names []string
		arr := tools.Array()
		for i := 0; i < len(arr); i++ {
			t := arr[i]
			if t.Get("type").String() != "function" {
				continue
			}
			fn := t.Get("function")
			if !fn.Exists() {
				continue
			}
			if v := fn.Get("name"); v.Exists() {
				names = append(names, v.String())
			}
		}
		if len(names) > 0 {
			m := buildShortNameMap(names)
			for orig, short := range m {
				rev[short] = orig
			}
		}
	}
	return rev
}

// mimeTypeFromCodexOutputFormat 将 Codex 的图片输出格式转换为 MIME 类型字符串。
// 支持的格式：png、jpg/jpeg、webp、gif。
// 如果输入已包含 "/" 则视为完整 MIME 类型直接返回。
// 默认返回 "image/png"。
func mimeTypeFromCodexOutputFormat(outputFormat string) string {
	if outputFormat == "" {
		return "image/png"
	}
	if strings.Contains(outputFormat, "/") {
		return outputFormat
	}
	switch strings.ToLower(outputFormat) {
	case "png":
		return "image/png"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	case "gif":
		return "image/gif"
	default:
		return "image/png"
	}
}
