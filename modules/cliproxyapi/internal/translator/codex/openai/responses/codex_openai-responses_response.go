package responses

import (
	"bytes"
	"context"

	"github.com/tidwall/gjson"
)

// responses - codex_openai-responses_response.go
// Codex 的 OpenAI Responses 格式响应转换器。
// 负责将 Codex 上游的响应透传转换为 OpenAI Responses API 兼容的格式。
//
// 转换逻辑较为简单，因为 Codex 上游本身返回的就是 OpenAI Responses 格式：
// - 流式模式：剥离 data: 前缀后重新添加标准 SSE 格式前缀
// - 非流式模式：从 response.completed 事件中提取 response 对象

// ConvertCodexResponseToOpenAIResponses 将 Codex 的流式响应转换为 OpenAI Responses SSE 格式。
// 由于 Codex 上游返回的已经是 Responses 格式，主要处理 SSE data: 前缀的标准化。
//
// 参数：
//   - ctx: 请求上下文（未使用）
//   - modelName: 模型名称（未使用）
//   - rawJSON: Codex 上游的原始响应数据
//
// 返回值：
//   - [][]byte: 标准化的 OpenAI Responses SSE 事件数据切片

func ConvertCodexResponseToOpenAIResponses(_ context.Context, _ string, _, _, rawJSON []byte, _ *any) [][]byte {
	if bytes.HasPrefix(rawJSON, []byte("data:")) {
		rawJSON = bytes.TrimSpace(rawJSON[5:])
		out := make([]byte, 0, len(rawJSON)+len("data: "))
		out = append(out, []byte("data: ")...)
		out = append(out, rawJSON...)
		return [][]byte{out}
	}
	return [][]byte{rawJSON}
}

// ConvertCodexResponseToOpenAIResponsesNonStream 将 Codex 的非流式响应转换为 OpenAI Responses JSON 格式。
// 从 response.completed 事件中提取 response 对象作为完整的 Responses 响应。
//
// 参数：
//   - ctx: 请求上下文（未使用）
//   - modelName: 模型名称（未使用）
//   - rawJSON: Codex 上游的原始响应数据
//
// 返回值：
//   - []byte: OpenAI Responses 格式的完整 JSON 响应数据
func ConvertCodexResponseToOpenAIResponsesNonStream(_ context.Context, _ string, _, _, rawJSON []byte, _ *any) []byte {
	rootResult := gjson.ParseBytes(rawJSON)
	// Verify this is a response.completed event
	if rootResult.Get("type").String() != "response.completed" {
		return []byte{}
	}
	responseResult := rootResult.Get("response")
	return []byte(responseResult.Raw)
}
