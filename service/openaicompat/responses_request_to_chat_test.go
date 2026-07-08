package openaicompat

import (
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/dto"
	"github.com/stretchr/testify/require"
)

func mustRawMessage(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := common.Marshal(value)
	require.NoError(t, err)
	return raw
}

func TestResponsesRequestToChatCompletionsRequestStringInput(t *testing.T) {
	stream := false
	store := false
	temperature := 0.0
	topP := 0.0
	topLogProbs := 0
	maxOutputTokens := uint(0)

	req := &dto.OpenAIResponsesRequest{
		Model:                "gemini-2.5-pro",
		Input:                mustRawMessage(t, "hello"),
		Instructions:         mustRawMessage(t, "You are careful."),
		Stream:               &stream,
		Store:                mustRawMessage(t, store),
		Temperature:          &temperature,
		TopP:                 &topP,
		TopLogProbs:          &topLogProbs,
		MaxOutputTokens:      &maxOutputTokens,
		ParallelToolCalls:    mustRawMessage(t, false),
		ServiceTier:          "default",
		PromptCacheKey:       mustRawMessage(t, "cache-key"),
		PromptCacheRetention: mustRawMessage(t, "24h"),
		SafetyIdentifier:     mustRawMessage(t, "safe-user"),
		EnableThinking:       mustRawMessage(t, false),
		Metadata:             mustRawMessage(t, map[string]any{"trace": "t-1"}),
		User:                 mustRawMessage(t, "user-1"),
	}

	chatReq, err := ResponsesRequestToChatCompletionsRequest(req)
	require.NoError(t, err)

	require.Equal(t, "gemini-2.5-pro", chatReq.Model)
	require.Len(t, chatReq.Messages, 2)
	require.Equal(t, dto.Message{Role: "system", Content: "You are careful."}, chatReq.Messages[0])
	require.Equal(t, dto.Message{Role: "user", Content: "hello"}, chatReq.Messages[1])
	require.NotNil(t, chatReq.Stream)
	require.False(t, *chatReq.Stream)
	require.NotNil(t, chatReq.Temperature)
	require.Equal(t, 0.0, *chatReq.Temperature)
	require.NotNil(t, chatReq.TopP)
	require.Equal(t, 0.0, *chatReq.TopP)
	require.NotNil(t, chatReq.TopLogProbs)
	require.Equal(t, 0, *chatReq.TopLogProbs)
	require.NotNil(t, chatReq.MaxCompletionTokens)
	require.Equal(t, uint(0), *chatReq.MaxCompletionTokens)
	require.NotNil(t, chatReq.ParallelTooCalls)
	require.False(t, *chatReq.ParallelTooCalls)
	require.Equal(t, "cache-key", chatReq.PromptCacheKey)
	require.JSONEq(t, `"default"`, string(chatReq.ServiceTier))
	require.JSONEq(t, `false`, string(chatReq.Store))
	require.JSONEq(t, `"24h"`, string(chatReq.PromptCacheRetention))
	require.JSONEq(t, `"safe-user"`, string(chatReq.SafetyIdentifier))
	require.JSONEq(t, `false`, string(chatReq.EnableThinking))
	require.JSONEq(t, `{"trace":"t-1"}`, string(chatReq.Metadata))
	require.JSONEq(t, `"user-1"`, string(chatReq.User))
}

func TestResponsesRequestToChatCompletionsRequestArrayInputAndTools(t *testing.T) {
	req := &dto.OpenAIResponsesRequest{
		Model: "gpt-5.1",
		Input: mustRawMessage(t, []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{"type": "input_text", "text": "describe"},
					{"type": "input_image", "image_url": "https://example.com/a.png", "detail": "low"},
				},
			},
			{
				"type":      "function_call",
				"call_id":   "call_1",
				"name":      "lookup",
				"arguments": map[string]any{"q": "nexus"},
			},
			{
				"type":    "function_call_output",
				"call_id": "call_1",
				"output":  map[string]any{"ok": true},
			},
		}),
		Tools: mustRawMessage(t, []map[string]any{
			{
				"type":        "function",
				"name":        "lookup",
				"description": "Lookup records",
				"parameters":  map[string]any{"type": "object"},
			},
		}),
		ToolChoice: mustRawMessage(t, map[string]any{
			"type": "function",
			"name": "lookup",
		}),
		Text: mustRawMessage(t, map[string]any{
			"format": map[string]any{
				"type": "json_schema",
				"name": "Result",
			},
		}),
	}

	chatReq, err := ResponsesRequestToChatCompletionsRequest(req)
	require.NoError(t, err)

	require.Len(t, chatReq.Messages, 3)
	require.Equal(t, "user", chatReq.Messages[0].Role)
	content, ok := chatReq.Messages[0].Content.([]any)
	require.True(t, ok)
	require.Len(t, content, 2)
	require.Equal(t, "assistant", chatReq.Messages[1].Role)
	toolCalls := chatReq.Messages[1].ParseToolCalls()
	require.Len(t, toolCalls, 1)
	require.Equal(t, "call_1", toolCalls[0].ID)
	require.Equal(t, "function", toolCalls[0].Type)
	require.Equal(t, "lookup", toolCalls[0].Function.Name)
	require.JSONEq(t, `{"q":"nexus"}`, toolCalls[0].Function.Arguments)
	require.Equal(t, "tool", chatReq.Messages[2].Role)
	require.Equal(t, "call_1", chatReq.Messages[2].ToolCallId)
	require.JSONEq(t, `{"ok":true}`, chatReq.Messages[2].Content.(string))

	require.Len(t, chatReq.Tools, 1)
	require.Equal(t, "lookup", chatReq.Tools[0].Function.Name)
	require.Equal(t, "Lookup records", chatReq.Tools[0].Function.Description)
	require.Equal(t, map[string]any{"type": "function", "function": map[string]any{"name": "lookup"}}, chatReq.ToolChoice)
	require.NotNil(t, chatReq.ResponseFormat)
	require.Equal(t, "json_schema", chatReq.ResponseFormat.Type)
	require.JSONEq(t, `{"name":"Result","type":"json_schema"}`, string(chatReq.ResponseFormat.JsonSchema))
}

func TestResponsesRequestToChatCompletionsRequestRejectsStatefulFields(t *testing.T) {
	req := &dto.OpenAIResponsesRequest{
		Model:              "gpt-5.1",
		Input:              mustRawMessage(t, "hello"),
		PreviousResponseID: "resp_1",
		Conversation:       mustRawMessage(t, "conv_1"),
		ContextManagement:  mustRawMessage(t, map[string]any{"type": "auto"}),
	}

	_, err := ResponsesRequestToChatCompletionsRequest(req)
	require.Error(t, err)
	require.Contains(t, err.Error(), "previous_response_id")
	require.Contains(t, err.Error(), "conversation")
	require.Contains(t, err.Error(), "context_management")
}

func TestResponsesRequestToChatCompletionsRequestCustomToolCall(t *testing.T) {
	req := &dto.OpenAIResponsesRequest{
		Model: "gpt-5.1",
		Input: mustRawMessage(t, []map[string]any{
			{
				"type":    "custom_tool_call",
				"call_id": "custom_1",
				"name":    "freeform",
				"input":   "raw command",
			},
		}),
	}

	chatReq, err := ResponsesRequestToChatCompletionsRequest(req)
	require.NoError(t, err)
	require.Len(t, chatReq.Messages, 1)
	require.Equal(t, "assistant", chatReq.Messages[0].Role)
	toolCalls := chatReq.Messages[0].ParseToolCalls()
	require.Len(t, toolCalls, 1)
	require.Equal(t, dto.CustomType, toolCalls[0].Type)
	require.Equal(t, "custom_1", toolCalls[0].ID)
	require.JSONEq(t, `{"call_id":"custom_1","input":"raw command","name":"freeform","type":"custom_tool_call"}`, string(toolCalls[0].Custom))
}
