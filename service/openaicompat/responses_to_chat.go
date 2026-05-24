// responses_to_chat.go 实现了 OpenAI Responses API 响应到 Chat Completions 响应的转换。
// 将 /v1/responses 格式的响应转换回 /v1/chat/completions 格式，
// 包括文本提取、工具调用映射、usage 统计转换等。
package openaicompat

import (
	"errors"
	"strings"

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
	usage := &dto.Usage{}
	if resp.Usage != nil {
		if resp.Usage.InputTokens != 0 {
			usage.PromptTokens = resp.Usage.InputTokens
			usage.InputTokens = resp.Usage.InputTokens
		}
		if resp.Usage.OutputTokens != 0 {
			usage.CompletionTokens = resp.Usage.OutputTokens
			usage.OutputTokens = resp.Usage.OutputTokens
		}
		if resp.Usage.TotalTokens != 0 {
			usage.TotalTokens = resp.Usage.TotalTokens
		} else {
			// totalTokens 未返回时，用 input + output 计算
			usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
		}
		// 转换 input tokens 详情（缓存、图片、音频 token 数）
		if resp.Usage.InputTokensDetails != nil {
			usage.PromptTokensDetails.CachedTokens = resp.Usage.InputTokensDetails.CachedTokens
			usage.PromptTokensDetails.ImageTokens = resp.Usage.InputTokensDetails.ImageTokens
			usage.PromptTokensDetails.AudioTokens = resp.Usage.InputTokensDetails.AudioTokens
		}
		// 转换 completion token 详情（推理 token 数）
		if resp.Usage.CompletionTokenDetails.ReasoningTokens != 0 {
			usage.CompletionTokenDetails.ReasoningTokens = resp.Usage.CompletionTokenDetails.ReasoningTokens
		}
	}

	created := resp.CreatedAt

	// 如果没有文本输出，检查是否有 function_call 输出 -> 转换为 tool_calls
	var toolCalls []dto.ToolCallResponse
	if text == "" && len(resp.Output) > 0 {
		for _, out := range resp.Output {
			if out.Type != "function_call" {
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
	if len(toolCalls) > 0 {
		finishReason = "tool_calls"
	}

	// 构建 assistant 消息
	msg := dto.Message{
		Role:    "assistant",
		Content: text,
	}
	if len(toolCalls) > 0 {
		// 有 tool_calls 时，清空 content，设置 tool_calls
		msg.SetToolCalls(toolCalls)
		msg.Content = ""
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
