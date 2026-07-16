// Package dto - openai_request_zero_value_test.go
// 该文件包含 OpenAI 请求显式零值保留的单元测试
//
// 测试内容包括：
// - GeneralOpenAIRequest 的显式零值（0、false）在 JSON 序列化后不被省略
// - OpenAIResponsesRequest 的显式零值保留
// 遵循 Rule 6：上游中继请求 DTO 保留显式零值
// 使用指针类型 + omitempty 确保：字段缺失 => nil => 省略；字段显式设为零 => 非 nil => 保留
package dto

import (
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGeneralOpenAIRequestPreserveExplicitZeroValues(t *testing.T) {
	raw := []byte(`{
		"model":"gpt-4.1",
		"stream":false,
		"max_tokens":0,
		"max_completion_tokens":0,
		"top_p":0,
		"top_k":0,
		"n":0,
		"frequency_penalty":0,
		"presence_penalty":0,
		"seed":0,
		"logprobs":false,
		"top_logprobs":0,
		"dimensions":0,
		"return_images":false,
		"return_related_questions":false
	}`)

	var req GeneralOpenAIRequest
	err := common.Unmarshal(raw, &req)
	require.NoError(t, err)

	encoded, err := common.Marshal(req)
	require.NoError(t, err)

	require.True(t, gjson.GetBytes(encoded, "stream").Exists())
	require.True(t, gjson.GetBytes(encoded, "max_tokens").Exists())
	require.True(t, gjson.GetBytes(encoded, "max_completion_tokens").Exists())
	require.True(t, gjson.GetBytes(encoded, "top_p").Exists())
	require.True(t, gjson.GetBytes(encoded, "top_k").Exists())
	require.True(t, gjson.GetBytes(encoded, "n").Exists())
	require.True(t, gjson.GetBytes(encoded, "frequency_penalty").Exists())
	require.True(t, gjson.GetBytes(encoded, "presence_penalty").Exists())
	require.True(t, gjson.GetBytes(encoded, "seed").Exists())
	require.True(t, gjson.GetBytes(encoded, "logprobs").Exists())
	require.True(t, gjson.GetBytes(encoded, "top_logprobs").Exists())
	require.True(t, gjson.GetBytes(encoded, "dimensions").Exists())
	require.True(t, gjson.GetBytes(encoded, "return_images").Exists())
	require.True(t, gjson.GetBytes(encoded, "return_related_questions").Exists())
}

func TestOpenAIResponsesRequestPreserveExplicitZeroValues(t *testing.T) {
	raw := []byte(`{
		"model":"gpt-4.1",
		"max_output_tokens":0,
		"max_tool_calls":0,
		"stream":false,
		"top_p":0
	}`)

	var req OpenAIResponsesRequest
	err := common.Unmarshal(raw, &req)
	require.NoError(t, err)

	encoded, err := common.Marshal(req)
	require.NoError(t, err)

	require.True(t, gjson.GetBytes(encoded, "max_output_tokens").Exists())
	require.True(t, gjson.GetBytes(encoded, "max_tool_calls").Exists())
	require.True(t, gjson.GetBytes(encoded, "stream").Exists())
	require.True(t, gjson.GetBytes(encoded, "top_p").Exists())
}

func TestImageRequestPreserveExplicitStreamFalse(t *testing.T) {
	raw := []byte(`{
		"model":"gpt-image-1",
		"prompt":"draw a cat",
		"stream":false
	}`)

	var req ImageRequest
	err := common.Unmarshal(raw, &req)
	require.NoError(t, err)
	require.NotNil(t, req.Stream)
	require.False(t, *req.Stream)

	encoded, err := common.Marshal(req)
	require.NoError(t, err)

	require.True(t, gjson.GetBytes(encoded, "stream").Exists())
	require.False(t, gjson.GetBytes(encoded, "stream").Bool())
}

func TestGeneralOpenAIRequestGetSystemRoleName(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		expected string
	}{
		{name: "o1 mini keeps system", model: "o1-mini", expected: "system"},
		{name: "o1 preview keeps system", model: "o1-preview", expected: "system"},
		{name: "o3 uses developer", model: "o3-mini", expected: "developer"},
		{name: "o4 uses developer", model: "o4-mini", expected: "developer"},
		{name: "gpt5 uses developer", model: "gpt-5.4", expected: "developer"},
		{name: "omni moderation stays system", model: "omni-moderation-latest", expected: "system"},
		{name: "plain gpt4 stays system", model: "gpt-4.1", expected: "system"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := GeneralOpenAIRequest{Model: test.model}
			require.Equal(t, test.expected, request.GetSystemRoleName())
		})
	}
}
