// common - bytes.go
// 提供 SSE（Server-Sent Events）帧构建和 API 响应封装的字节操作工具函数。
// 包含 Gemini CLI 响应包装、Token 计数 JSON 构建、Claude 输入 Token 统计、
// 以及 SSE 事件帧拼接等高效字节拼接操作。
package common

import (
	"strconv"

	"github.com/tidwall/sjson"
)

// WrapGeminiCLIResponse 将 Gemini CLI 原始响应封装为标准的 {"response": {...}} 格式。
// 如果 sjson 设置失败，则返回原始响应不做修改。
func WrapGeminiCLIResponse(response []byte) []byte {
	out, err := sjson.SetRawBytes([]byte(`{"response":{}}`), "response", response)
	if err != nil {
		return response
	}
	return out
}

// GeminiTokenCountJSON 构建 Gemini Token 计数的 JSON 响应。
// 返回格式为 {"totalTokens":count,"promptTokensDetails":[{"modality":"TEXT","tokenCount":count}]}。
// 使用预分配缓冲区和 strconv.AppendInt 实现高效拼接，避免额外的内存分配。
func GeminiTokenCountJSON(count int64) []byte {
	out := make([]byte, 0, 96)
	out = append(out, `{"totalTokens":`...)
	out = strconv.AppendInt(out, count, 10)
	out = append(out, `,"promptTokensDetails":[{"modality":"TEXT","tokenCount":`...)
	out = strconv.AppendInt(out, count, 10)
	out = append(out, `}]}`...)
	return out
}

// ClaudeInputTokensJSON 构建 Claude 输入 Token 计数的 JSON 响应。
// 返回格式为 {"input_tokens":count}。
// 使用预分配缓冲区实现高效字节拼接。
func ClaudeInputTokensJSON(count int64) []byte {
	out := make([]byte, 0, 32)
	out = append(out, `{"input_tokens":`...)
	out = strconv.AppendInt(out, count, 10)
	out = append(out, '}')
	return out
}

// SSEEventData 构建一个标准的 SSE 事件帧，格式为 "event: {event}\ndata: {payload}"。
// 该函数不添加尾部换行符，调用方需根据协议要求自行添加。
func SSEEventData(event string, payload []byte) []byte {
	out := make([]byte, 0, len(event)+len(payload)+14)
	out = append(out, "event: "...)
	out = append(out, event...)
	out = append(out, '\n')
	out = append(out, "data: "...)
	out = append(out, payload...)
	return out
}

// AppendSSEEventString 将 SSE 事件帧追加到输出缓冲区。
// event 为事件名称，payload 为事件数据（字符串形式），trailingNewlines 为尾部换行符数量。
// 标准 SSE 协议要求事件之间以两个换行符（\n\n）分隔。
func AppendSSEEventString(out []byte, event, payload string, trailingNewlines int) []byte {
	out = append(out, "event: "...)
	out = append(out, event...)
	out = append(out, '\n')
	out = append(out, "data: "...)
	out = append(out, payload...)
	for i := 0; i < trailingNewlines; i++ {
		out = append(out, '\n')
	}
	return out
}

// AppendSSEEventBytes 将 SSE 事件帧追加到输出缓冲区（字节切片形式）。
// 与 AppendSSEEventString 功能相同，但接受 []byte 类型的 payload，
// 避免字符串到字节的转换开销，适用于已有的字节数据。
func AppendSSEEventBytes(out []byte, event string, payload []byte, trailingNewlines int) []byte {
	out = append(out, "event: "...)
	out = append(out, event...)
	out = append(out, '\n')
	out = append(out, "data: "...)
	out = append(out, payload...)
	for i := 0; i < trailingNewlines; i++ {
		out = append(out, '\n')
	}
	return out
}
