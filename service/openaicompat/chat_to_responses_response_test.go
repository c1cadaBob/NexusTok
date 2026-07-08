package openaicompat

import (
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatCompletionsResponseToResponsesResponsePreservesTextReasoningAndTools(t *testing.T) {
	reasoning := "先检查参数"
	toolCallsRaw, err := common.Marshal([]dto.ToolCallRequest{
		{
			ID:   "call_1",
			Type: "function",
			Function: dto.FunctionRequest{
				Name:      "lookup",
				Arguments: `{"q":"nexus"}`,
			},
		},
	})
	require.NoError(t, err)

	resp := &dto.OpenAITextResponse{
		Id:      "chatcmpl_1",
		Object:  "chat.completion",
		Created: int64(123),
		Model:   "gemini-test",
		Choices: []dto.OpenAITextResponseChoice{
			{
				Index: 0,
				Message: dto.Message{
					Role:             "assistant",
					Content:          "我会查一下。",
					ReasoningContent: &reasoning,
					ToolCalls:        toolCallsRaw,
				},
				FinishReason: "tool_calls",
			},
		},
		Usage: dto.Usage{PromptTokens: 3, CompletionTokens: 4, TotalTokens: 7},
	}

	got, usage, err := ChatCompletionsResponseToResponsesResponse(resp, "resp_1")
	require.NoError(t, err)
	require.NotNil(t, usage)

	assert.Equal(t, "resp_1", got.ID)
	assert.Equal(t, `"completed"`, string(got.Status))
	require.Len(t, got.Output, 3)
	assert.Equal(t, responsesOutputTypeMessage, got.Output[0].Type)
	assert.Equal(t, "我会查一下。", got.Output[0].Content[0].Text)
	assert.Equal(t, responsesOutputTypeReasoning, got.Output[1].Type)
	assert.Equal(t, "先检查参数", got.Output[1].Content[0].Text)
	assert.Equal(t, responsesOutputTypeFunctionCall, got.Output[2].Type)
	assert.Equal(t, "call_1", got.Output[2].CallId)
	assert.Equal(t, "lookup", got.Output[2].Name)
	assert.Equal(t, `{"q":"nexus"}`, got.Output[2].ArgumentsString())
	assert.Equal(t, 3, usage.InputTokens)
	assert.Equal(t, 4, usage.OutputTokens)
	assert.Equal(t, 7, usage.TotalTokens)
}

func TestChatCompletionsResponseToResponsesResponseMapsIncomplete(t *testing.T) {
	resp := &dto.OpenAITextResponse{
		Created: 123,
		Model:   "gemini-test",
		Choices: []dto.OpenAITextResponseChoice{
			{
				Message:      dto.Message{Role: "assistant", Content: "partial"},
				FinishReason: "length",
			},
		},
	}

	got, _, err := ChatCompletionsResponseToResponsesResponse(resp, "resp_1")
	require.NoError(t, err)

	assert.Equal(t, `"incomplete"`, string(got.Status))
	require.NotNil(t, got.IncompleteDetails)
	assert.Equal(t, responsesIncompleteReasonMaxTokens, got.IncompleteDetails.Reason)
	assert.Equal(t, "incomplete", got.Output[0].Status)
}

func TestResponsesResponseToChatCompletionsPreservesTextReasoningAndToolCalls(t *testing.T) {
	resp := &dto.OpenAIResponsesResponse{
		ID:        "resp_1",
		CreatedAt: 123,
		Model:     "gpt-test",
		Status:    []byte(`"completed"`),
		Output: []dto.ResponsesOutput{
			{
				Type: responsesOutputTypeMessage,
				Role: "assistant",
				Content: []dto.ResponsesOutputContent{
					{Type: "output_text", Text: "I will call a tool."},
				},
			},
			{
				Type: responsesOutputTypeReasoning,
				Content: []dto.ResponsesOutputContent{
					{Type: "summary_text", Text: "reasoning"},
				},
			},
			{
				Type:      responsesOutputTypeFunctionCall,
				ID:        "fc_1",
				CallId:    "call_1",
				Name:      "lookup",
				Arguments: []byte(`{"q":"x"}`),
			},
		},
		Usage: &dto.Usage{InputTokens: 3, OutputTokens: 4, TotalTokens: 7},
	}

	chat, usage, err := ResponsesResponseToChatCompletionsResponse(resp, "chatcmpl_1")
	require.NoError(t, err)
	require.NotNil(t, usage)

	require.Len(t, chat.Choices, 1)
	assert.Equal(t, "tool_calls", chat.Choices[0].FinishReason)
	assert.Equal(t, "I will call a tool.", chat.Choices[0].Message.StringContent())
	assert.Equal(t, "reasoning", chat.Choices[0].Message.GetReasoningContent())
	toolCalls := chat.Choices[0].Message.ParseToolCalls()
	require.Len(t, toolCalls, 1)
	assert.Equal(t, "call_1", toolCalls[0].ID)
	assert.Equal(t, "lookup", toolCalls[0].Function.Name)
	assert.Equal(t, `{"q":"x"}`, toolCalls[0].Function.Arguments)
	assert.Equal(t, 7, usage.TotalTokens)
}

func TestResponsesFinishReasonFromIncompleteStatus(t *testing.T) {
	tests := []struct {
		name   string
		reason string
		want   string
	}{
		{name: "max output", reason: responsesIncompleteReasonMaxTokens, want: "length"},
		{name: "content filter", reason: responsesIncompleteReasonContentFilter, want: "content_filter"},
		{name: "unknown", reason: "other", want: "length"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ResponsesFinishReasonFromStatus(&dto.OpenAIResponsesResponse{
				Status:            []byte(`"incomplete"`),
				IncompleteDetails: &dto.IncompleteDetails{Reason: tt.reason},
			})
			require.True(t, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestChatCompletionsStreamChunkToResponsesEvents(t *testing.T) {
	state := NewChatToResponsesStreamState("resp_1", "gemini-test")
	toolIndex := 0

	events, err := ChatCompletionsStreamChunkToResponsesEvents(&dto.ChatCompletionsStreamResponse{
		Id:      "resp_1",
		Created: 123,
		Model:   "gemini-test",
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{
				Index: 0,
				Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
					ToolCalls: []dto.ToolCallResponse{
						{
							Index: &toolIndex,
							ID:    "call_1",
							Type:  "function",
							Function: dto.FunctionResponse{
								Name:      "lookup",
								Arguments: `{"q":"x"}`,
							},
						},
					},
				},
			},
		},
	}, state)
	require.NoError(t, err)
	require.Len(t, events, 3)
	assert.Equal(t, responsesEventCreated, events[0].Type)
	assert.Equal(t, responsesEventOutputItemAdded, events[1].Type)
	assert.Equal(t, responsesEventFunctionArgsDelta, events[2].Type)

	textEvents, err := ChatCompletionsStreamChunkToResponsesEvents(&dto.ChatCompletionsStreamResponse{
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{
				Index: 0,
				Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
					Content: common.GetPointer("done"),
				},
			},
		},
	}, state)
	require.NoError(t, err)
	require.Len(t, textEvents, 2)
	assert.Equal(t, responsesEventOutputItemAdded, textEvents[0].Type)
	assert.Equal(t, responsesEventOutputTextDelta, textEvents[1].Type)

	final := FinalizeChatCompletionsStreamToResponses(state)
	require.NotEmpty(t, final)
	assert.Equal(t, responsesEventCompleted, final[len(final)-1].Type)
	require.NotNil(t, final[len(final)-1].Payload.Response)
	require.Len(t, final[len(final)-1].Payload.Response.Output, 2)
}
