// responses_to_chat.go 实现了 OpenAI Responses API 响应到 Chat Completions 响应的转换。
// 将 /v1/responses 格式的响应转换回 /v1/chat/completions 格式，
// 包括文本提取、工具调用映射、usage 统计转换等。
package openaicompat

import (
	"errors"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/dto" // 数据传输对象
)

// ResponsesResponseToChatCompletionsResponse 将 Responses API 响应转换为 Chat Completions 响应。
//
// 主要转换逻辑：
//   - 提取输出文本作为 assistant 消息内容
//   - 将 function_call 输出项映射为 Chat 格式的 tool_calls
//   - 转换 usage 统计字段
//   - 根据是否有 tool_calls 设置 finish_reason
//
// 参数：
//   - resp: Responses API 响应对象
//   - id: 响应 ID
//
// 返回：
//   - *dto.OpenAITextResponse: Chat Completions 格式的响应
//   - *dto.Usage: 用量统计
//   - error: 转换错误
func ResponsesResponseToChatCompletionsResponse(resp *dto.OpenAIResponsesResponse, id string) (*dto.OpenAITextResponse, *dto.Usage, error) {
	if resp == nil {
		return nil, nil, errors.New("response is nil")
	}

	// 从 Responses 输出中提取文本内容
	text := ExtractOutputTextFromResponses(resp)

	// 转换 usage 统计
	usage := UsageFromResponsesUsage(resp.Usage)

	created := resp.CreatedAt

	// Responses 允许文本、reasoning 和工具调用同时出现在 output 中。
	// Chat Completions 也可以用 assistant content + tool_calls 表达该组合，因此不能因为已有文本就丢弃工具调用。
	var toolCalls []dto.ToolCallResponse
	if len(resp.Output) > 0 {
		for _, out := range resp.Output {
			if !isResponsesToolOutputType(out.Type) {
				continue
			}
			name := strings.TrimSpace(out.Name)
			if name == "" {
				continue
			}
			// 优先使用 call_id，其次使用 output 项 ID
			callId := strings.TrimSpace(out.CallId)
			if callId == "" {
				callId = strings.TrimSpace(out.ID)
			}
			toolCalls = append(toolCalls, dto.ToolCallResponse{
				ID:   callId,
				Type: "function",
				Function: dto.FunctionResponse{
					Name:      name,
					Arguments: out.ArgumentsString(),
				},
			})
		}
	}

	// 根据是否有 tool_calls 设置 finish_reason
	finishReason := "stop"
	if mappedReason, ok := ResponsesFinishReasonFromStatus(resp); ok {
		finishReason = mappedReason
	} else if len(toolCalls) > 0 {
		finishReason = "tool_calls"
	}

	// 构建 assistant 消息
	msg := dto.Message{
		Role:    "assistant",
		Content: text,
	}
	if reasoning := ExtractReasoningTextFromResponses(resp); reasoning != "" {
		msg.ReasoningContent = &reasoning
	}
	if len(toolCalls) > 0 {
		toolCallsRaw, err := common.Marshal(toolCalls)
		if err != nil {
			return nil, nil, err
		}
		msg.ToolCalls = toolCallsRaw
	}

	// 构建 Chat Completions 响应
	out := &dto.OpenAITextResponse{
		Id:      id,
		Object:  "chat.completion",
		Created: created,
		Model:   resp.Model,
		Choices: []dto.OpenAITextResponseChoice{
			{
				Index:        0,
				Message:      msg,
				FinishReason: finishReason,
			},
		},
		Usage: *usage,
	}

	return out, usage, nil
}

// ResponsesFinishReasonFromStatus 将 Responses incomplete 状态映射为 Chat finish_reason。
func ResponsesFinishReasonFromStatus(resp *dto.OpenAIResponsesResponse) (string, bool) {
	if resp == nil {
		return "", false
	}

	status := responseStatusString(resp)
	if status != "incomplete" {
		return "", false
	}

	reason := ""
	if resp.IncompleteDetails != nil {
		reason = strings.TrimSpace(resp.IncompleteDetails.Reason)
		if reason == "" {
			reason = strings.TrimSpace(resp.IncompleteDetails.Reasoning)
		}
	}
	if reason == responsesIncompleteReasonContentFilter {
		return "content_filter", true
	}
	return "length", true
}

// UsageFromResponsesUsage 将 Responses usage 字段补齐为 Chat usage 语义。
func UsageFromResponsesUsage(src *dto.Usage) *dto.Usage {
	usage := &dto.Usage{}
	if src == nil {
		return usage
	}
	if src.InputTokens != 0 {
		usage.PromptTokens = src.InputTokens
		usage.InputTokens = src.InputTokens
	}
	if src.PromptTokens != 0 {
		usage.PromptTokens = src.PromptTokens
		usage.InputTokens = src.PromptTokens
	}
	if src.OutputTokens != 0 {
		usage.CompletionTokens = src.OutputTokens
		usage.OutputTokens = src.OutputTokens
	}
	if src.CompletionTokens != 0 {
		usage.CompletionTokens = src.CompletionTokens
		usage.OutputTokens = src.CompletionTokens
	}
	if src.TotalTokens != 0 {
		usage.TotalTokens = src.TotalTokens
	} else {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	if src.InputTokensDetails != nil {
		usage.PromptTokensDetails.CachedTokens = src.InputTokensDetails.CachedTokens
		usage.PromptTokensDetails.ImageTokens = src.InputTokensDetails.ImageTokens
		usage.PromptTokensDetails.AudioTokens = src.InputTokensDetails.AudioTokens
		usage.PromptTokensDetails.TextTokens = src.InputTokensDetails.TextTokens
		usage.PromptTokensDetails.CachedCreationTokens = src.InputTokensDetails.CachedCreationTokens
	}
	if src.PromptTokensDetails.CachedTokens != 0 ||
		src.PromptTokensDetails.ImageTokens != 0 ||
		src.PromptTokensDetails.AudioTokens != 0 ||
		src.PromptTokensDetails.TextTokens != 0 ||
		src.PromptTokensDetails.CachedCreationTokens != 0 {
		usage.PromptTokensDetails = src.PromptTokensDetails
	}
	if src.CompletionTokenDetails.ReasoningTokens != 0 ||
		src.CompletionTokenDetails.TextTokens != 0 ||
		src.CompletionTokenDetails.AudioTokens != 0 ||
		src.CompletionTokenDetails.ImageTokens != 0 {
		usage.CompletionTokenDetails = src.CompletionTokenDetails
	}
	return usage
}

// ExtractOutputTextFromResponses 从 Responses API 响应中提取输出文本。
// 优先提取 assistant 角色的 message 类型输出中的 output_text 内容。
// 若无 assistant message，回退到提取所有输出中的文本。
//
// 参数：
//   - resp: Responses API 响应对象
//
// 返回：
//   - string: 拼接后的输出文本
func ExtractOutputTextFromResponses(resp *dto.OpenAIResponsesResponse) string {
	if resp == nil || len(resp.Output) == 0 {
		return ""
	}

	var sb strings.Builder

	// 优先提取 assistant message 的 output_text
	for _, out := range resp.Output {
		if out.Type != "message" {
			continue
		}
		if out.Role != "" && out.Role != "assistant" {
			continue
		}
		for _, c := range out.Content {
			if c.Type == "output_text" && c.Text != "" {
				sb.WriteString(c.Text)
			}
		}
	}
	if sb.Len() > 0 {
		return sb.String()
	}
	// 回退：提取所有输出中的文本内容
	for _, out := range resp.Output {
		for _, c := range out.Content {
			if c.Text != "" {
				sb.WriteString(c.Text)
			}
		}
	}
	return sb.String()
}

// ExtractReasoningTextFromResponses 从 Responses 输出中提取 reasoning summary 文本。
func ExtractReasoningTextFromResponses(resp *dto.OpenAIResponsesResponse) string {
	if resp == nil || len(resp.Output) == 0 {
		return ""
	}

	var sb strings.Builder
	for _, out := range resp.Output {
		if out.Type != responsesOutputTypeReasoning {
			continue
		}
		for _, c := range out.Content {
			if c.Text != "" {
				sb.WriteString(c.Text)
			}
		}
	}
	return sb.String()
}

func isResponsesToolOutputType(outputType string) bool {
	return outputType == responsesOutputTypeFunctionCall || outputType == responsesOutputTypeCustomToolCall
}
