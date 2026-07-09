package ollama

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/dto"
	relaycommon "github.com/c1cada/NexusTok/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOllamaChatHandlerNonStreamToolCalls 验证 Ollama 非流式 tool_calls 能转换为 OpenAI 兼容响应。
func TestOllamaChatHandlerNonStreamToolCalls(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "compact json per-line parse path",
			raw:  `{"model":"llama3.1","created_at":"2026-05-27T12:00:00Z","message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"get_weather","arguments":{"city":"Paris","days":0}}}]},"done":true,"done_reason":"stop","prompt_eval_count":5,"eval_count":7}`,
		},
		{
			name: "pretty json fallback parse path",
			raw: `{
  "model": "llama3.1",
  "created_at": "2026-05-27T12:00:00Z",
  "message": {
    "role": "assistant",
    "content": "",
    "tool_calls": [
      {
        "function": {
          "name": "get_weather",
          "arguments": {
            "city": "Paris",
            "days": 0
          }
        }
      }
    ]
  },
  "done": true,
  "done_reason": "stop",
  "prompt_eval_count": 5,
  "eval_count": 7
}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(tt.raw)),
			}

			usage, apiErr := ollamaChatHandler(c, ollamaRelayInfoForTest("fallback-model"), resp)

			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Equal(t, 12, usage.TotalTokens)

			var out dto.OpenAITextResponse
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &out))
			require.Len(t, out.Choices, 1)
			assert.Equal(t, constant.FinishReasonToolCalls, out.Choices[0].FinishReason)

			var toolCalls []dto.ToolCallResponse
			require.NoError(t, common.Unmarshal(out.Choices[0].Message.ToolCalls, &toolCalls))
			require.Len(t, toolCalls, 1)
			assert.NotEmpty(t, toolCalls[0].ID)
			assert.Equal(t, "function", toolCalls[0].Type)
			assert.Equal(t, "get_weather", toolCalls[0].Function.Name)
			assert.Nil(t, toolCalls[0].Index)

			var args map[string]any
			require.NoError(t, common.Unmarshal([]byte(toolCalls[0].Function.Arguments), &args))
			assert.Equal(t, "Paris", args["city"])
			assert.Equal(t, float64(0), args["days"])
		})
	}
}

// TestOllamaChatHandlerNonStreamToolCallsAggregatesNDJSON 验证多行 NDJSON 响应会聚合内容和多个工具调用。
func TestOllamaChatHandlerNonStreamToolCallsAggregatesNDJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)

	raw := strings.Join([]string{
		`{"model":"llama3.1","created_at":"2026-05-27T12:00:00Z","message":{"role":"assistant","content":"I will check. "},"done":false}`,
		`{"model":"llama3.1","created_at":"2026-05-27T12:00:01Z","message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"get_weather","arguments":{"city":"Paris"}}},{"function":{"name":"get_time","arguments":null}}]},"done":false}`,
		`{"model":"llama3.1","created_at":"2026-05-27T12:00:02Z","done":true,"done_reason":"stop","prompt_eval_count":5,"eval_count":7}`,
	}, "\n")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(raw)),
	}

	usage, apiErr := ollamaChatHandler(c, ollamaRelayInfoForTest("fallback-model"), resp)

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 12, usage.TotalTokens)

	var out dto.OpenAITextResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &out))
	require.Len(t, out.Choices, 1)
	assert.Equal(t, "I will check. ", out.Choices[0].Message.StringContent())
	assert.Equal(t, constant.FinishReasonToolCalls, out.Choices[0].FinishReason)

	var toolCalls []dto.ToolCallResponse
	require.NoError(t, common.Unmarshal(out.Choices[0].Message.ToolCalls, &toolCalls))
	require.Len(t, toolCalls, 2)
	assert.Equal(t, "call_0", toolCalls[0].ID)
	assert.Equal(t, "call_1", toolCalls[1].ID)
	assert.Equal(t, "get_weather", toolCalls[0].Function.Name)
	assert.Equal(t, "get_time", toolCalls[1].Function.Name)
	assert.Nil(t, toolCalls[0].Index)
	assert.Nil(t, toolCalls[1].Index)
	assert.Equal(t, `{}`, toolCalls[1].Function.Arguments)
}

// TestOllamaStreamHandlerToolCallsFinishReason 验证流式工具调用结束帧使用 tool_calls。
func TestOllamaStreamHandlerToolCallsFinishReason(t *testing.T) {
	gin.SetMode(gin.TestMode)

	raw := strings.Join([]string{
		`{"model":"llama3.1","created_at":"2026-05-27T12:00:00Z","message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"get_weather","arguments":{"city":"Paris"}}}]},"done":false}`,
		`{"model":"llama3.1","created_at":"2026-05-27T12:00:01Z","done":true,"done_reason":"stop","prompt_eval_count":5,"eval_count":7}`,
	}, "\n")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(raw)),
	}

	usage, apiErr := ollamaStreamHandler(c, ollamaRelayInfoForTest("fallback-model"), resp)

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 12, usage.TotalTokens)

	chunks := decodeOllamaStreamChunksForTest(t, recorder.Body.String())
	var sawToolCall bool
	var finishReason string
	for _, chunk := range chunks {
		if len(chunk.Choices) == 0 {
			continue
		}
		choice := chunk.Choices[0]
		if len(choice.Delta.ToolCalls) > 0 {
			sawToolCall = true
			assert.NotNil(t, choice.Delta.ToolCalls[0].Index)
			assert.Equal(t, "get_weather", choice.Delta.ToolCalls[0].Function.Name)
		}
		if choice.FinishReason != nil {
			finishReason = *choice.FinishReason
		}
	}

	assert.True(t, sawToolCall)
	assert.Equal(t, constant.FinishReasonToolCalls, finishReason)
}

// ollamaRelayInfoForTest 构造带上游模型名的 RelayInfo。
func ollamaRelayInfoForTest(upstreamModel string) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: upstreamModel},
	}
}

// decodeOllamaStreamChunksForTest 解析测试响应中的 SSE data 帧。
func decodeOllamaStreamChunksForTest(t *testing.T, body string) []dto.ChatCompletionsStreamResponse {
	t.Helper()

	var chunks []dto.ChatCompletionsStreamResponse
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			continue
		}
		var chunk dto.ChatCompletionsStreamResponse
		require.NoError(t, common.UnmarshalJsonStr(payload, &chunk))
		chunks = append(chunks, chunk)
	}
	return chunks
}
