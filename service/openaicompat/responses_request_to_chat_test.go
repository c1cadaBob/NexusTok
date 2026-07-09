package openaicompat

import (
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
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

func TestResponsesRequestToChatCompletionsRequestInstructionsAndScalarInput(t *testing.T) {
	stream := true
	temperature := 0.0
	topP := 0.9
	maxOutputTokens := uint(128)

	got, err := ResponsesRequestToChatCompletionsRequest(&dto.OpenAIResponsesRequest{
		Model:                "gpt-test",
		Instructions:         mustRawMessage(t, "system rules"),
		Input:                mustRawMessage(t, "hello"),
		Stream:               &stream,
		StreamOptions:        &dto.StreamOptions{IncludeUsage: true},
		MaxOutputTokens:      &maxOutputTokens,
		Temperature:          &temperature,
		TopP:                 &topP,
		User:                 mustRawMessage(t, "user-1"),
		Store:                mustRawMessage(t, false),
		Metadata:             mustRawMessage(t, map[string]any{"trace": "abc"}),
		ParallelToolCalls:    mustRawMessage(t, true),
		PromptCacheKey:       mustRawMessage(t, "cache-key"),
		PromptCacheRetention: mustRawMessage(t, "24h"),
		Reasoning:            &dto.Reasoning{Effort: "medium"},
	})
	require.NoError(t, err)

	assert.Equal(t, "gpt-test", got.Model)
	require.Len(t, got.Messages, 2)
	assert.Equal(t, dto.Message{Role: "system", Content: "system rules"}, got.Messages[0])
	assert.Equal(t, dto.Message{Role: "user", Content: "hello"}, got.Messages[1])
	assert.Same(t, &stream, got.Stream)
	require.NotNil(t, got.StreamOptions)
	assert.True(t, got.StreamOptions.IncludeUsage)
	require.NotNil(t, got.MaxCompletionTokens)
	assert.Equal(t, maxOutputTokens, *got.MaxCompletionTokens)
	require.NotNil(t, got.Temperature)
	assert.Equal(t, 0.0, *got.Temperature)
	require.NotNil(t, got.TopP)
	assert.Equal(t, 0.9, *got.TopP)
	require.NotNil(t, got.ParallelTooCalls)
	assert.True(t, *got.ParallelTooCalls)
	assert.Equal(t, "cache-key", got.PromptCacheKey)
	assert.Equal(t, "medium", got.ReasoningEffort)
	assert.JSONEq(t, `"user-1"`, string(got.User))
	assert.JSONEq(t, `false`, string(got.Store))
	assert.Equal(t, "abc", gjson.GetBytes(got.Metadata, "trace").String())
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

func TestResponsesRequestToChatCompletionsRequestMultimodalInput(t *testing.T) {
	got, err := ResponsesRequestToChatCompletionsRequest(&dto.OpenAIResponsesRequest{
		Model: "gpt-test",
		Input: mustRawMessage(t, []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{"type": "input_text", "text": "look"},
					{"type": "input_image", "image_url": "https://example.test/a.png", "detail": "low"},
					{"type": "input_file", "file_id": "file_1", "filename": "a.txt"},
					{"type": "input_audio", "input_audio": map[string]any{"data": "abc", "format": "wav"}},
					{"type": "input_video", "video_url": map[string]any{"url": "https://example.test/v.mp4"}},
				},
			},
		}),
	})
	require.NoError(t, err)

	require.Len(t, got.Messages, 1)
	assert.Equal(t, "user", got.Messages[0].Role)
	parts := got.Messages[0].ParseContent()
	require.Len(t, parts, 5)
	assert.Equal(t, dto.ContentTypeText, parts[0].Type)
	assert.Equal(t, "look", parts[0].Text)
	assert.Equal(t, dto.ContentTypeImageURL, parts[1].Type)
	assert.Equal(t, "https://example.test/a.png", parts[1].GetImageMedia().Url)
	assert.Equal(t, dto.ContentTypeFile, parts[2].Type)
	assert.Equal(t, "file_1", parts[2].GetFile().FileId)
	assert.Equal(t, dto.ContentTypeInputAudio, parts[3].Type)
	assert.Equal(t, "wav", parts[3].GetInputAudio().Format)
	assert.Equal(t, dto.ContentTypeVideoUrl, parts[4].Type)
	assert.Equal(t, "https://example.test/v.mp4", parts[4].GetVideoUrl().Url)
}

func TestResponsesRequestToChatCompletionsRequestAssistantTextAndFunctionCallCoexist(t *testing.T) {
	got, err := ResponsesRequestToChatCompletionsRequest(&dto.OpenAIResponsesRequest{
		Model: "gpt-test",
		Input: mustRawMessage(t, []map[string]any{
			{
				"role": "assistant",
				"content": []map[string]any{
					{"type": "output_text", "text": "I will call."},
				},
			},
			{
				"type":      "function_call",
				"call_id":   "call_1",
				"name":      "lookup",
				"arguments": map[string]any{"q": "x"},
			},
			{
				"type":    "function_call_output",
				"call_id": "call_1",
				"output":  map[string]any{"ok": true},
			},
		}),
	})
	require.NoError(t, err)

	require.Len(t, got.Messages, 2)
	assert.Equal(t, "assistant", got.Messages[0].Role)
	assert.Equal(t, "I will call.", got.Messages[0].StringContent())
	toolCalls := got.Messages[0].ParseToolCalls()
	require.Len(t, toolCalls, 1)
	assert.Equal(t, "call_1", toolCalls[0].ID)
	assert.Equal(t, "function", toolCalls[0].Type)
	assert.Equal(t, "lookup", toolCalls[0].Function.Name)
	assert.JSONEq(t, `{"q":"x"}`, toolCalls[0].Function.Arguments)
	assert.Equal(t, "tool", got.Messages[1].Role)
	assert.Equal(t, "call_1", got.Messages[1].ToolCallId)
	assert.JSONEq(t, `{"ok":true}`, got.Messages[1].StringContent())
}

func TestResponsesRequestToChatCompletionsRequestOnlyFunctionCallCreatesAssistant(t *testing.T) {
	got, err := ResponsesRequestToChatCompletionsRequest(&dto.OpenAIResponsesRequest{
		Model: "gpt-test",
		Input: mustRawMessage(t, []map[string]any{
			{
				"type":      "function_call",
				"call_id":   "call_1",
				"name":      "lookup",
				"arguments": `{"q":"x"}`,
			},
		}),
	})
	require.NoError(t, err)

	require.Len(t, got.Messages, 1)
	assert.Equal(t, "assistant", got.Messages[0].Role)
	assert.Nil(t, got.Messages[0].Content)
	toolCalls := got.Messages[0].ParseToolCalls()
	require.Len(t, toolCalls, 1)
	assert.Equal(t, `{"q":"x"}`, toolCalls[0].Function.Arguments)
}

func TestResponsesRequestToChatCompletionsRequestToolsToolChoiceAndTextFormat(t *testing.T) {
	got, err := ResponsesRequestToChatCompletionsRequest(&dto.OpenAIResponsesRequest{
		Model: "gpt-test",
		Input: mustRawMessage(t, "hello"),
		Tools: mustRawMessage(t, []map[string]any{
			{
				"type":        "function",
				"name":        "lookup",
				"description": "Lookup data",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"q": map[string]any{"type": "string"},
					},
				},
			},
		}),
		ToolChoice: mustRawMessage(t, map[string]any{
			"type": "function",
			"name": "lookup",
		}),
		Text: mustRawMessage(t, map[string]any{
			"format": map[string]any{
				"type":   "json_schema",
				"name":   "answer",
				"schema": map[string]any{"type": "object"},
				"strict": true,
			},
		}),
	})
	require.NoError(t, err)

	require.Len(t, got.Tools, 1)
	assert.Equal(t, "function", got.Tools[0].Type)
	assert.Equal(t, "lookup", got.Tools[0].Function.Name)
	assert.Equal(t, "Lookup data", got.Tools[0].Function.Description)
	assert.Equal(t, "object", got.Tools[0].Function.Parameters.(map[string]any)["type"])
	assert.Equal(t, map[string]any{
		"type": "function",
		"function": map[string]any{
			"name": "lookup",
		},
	}, got.ToolChoice)
	require.NotNil(t, got.ResponseFormat)
	assert.Equal(t, "json_schema", got.ResponseFormat.Type)
	assert.Equal(t, "answer", gjson.GetBytes(got.ResponseFormat.JsonSchema, "name").String())
	assert.True(t, gjson.GetBytes(got.ResponseFormat.JsonSchema, "strict").Bool())
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

func TestResponsesRequestToChatCompletionsRequestCustomToolCallPreservesRawShape(t *testing.T) {
	got, err := ResponsesRequestToChatCompletionsRequest(&dto.OpenAIResponsesRequest{
		Model: "gpt-test",
		Input: mustRawMessage(t, []map[string]any{
			{
				"type":    "custom_tool_call",
				"call_id": "call_custom",
				"name":    "apply_patch",
				"input":   "patch body",
			},
		}),
	})
	require.NoError(t, err)

	require.Len(t, got.Messages, 1)
	toolCalls := got.Messages[0].ParseToolCalls()
	require.Len(t, toolCalls, 1)
	assert.Equal(t, dto.CustomType, toolCalls[0].Type)
	assert.Equal(t, "call_custom", toolCalls[0].ID)
	assert.Equal(t, "apply_patch", toolCalls[0].Function.Name)
	assert.Equal(t, "patch body", toolCalls[0].Function.Arguments)
	assert.Equal(t, "custom_tool_call", gjson.GetBytes(toolCalls[0].Custom, "type").String())
	assert.Equal(t, "patch body", gjson.GetBytes(toolCalls[0].Custom, "input").String())
}
