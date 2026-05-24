// chat_completions - openai_openai_request.go
// OpenAI 的 OpenAI Chat Completions 格式请求转换器。
// 作为简单的模型名称替换层，将请求中的 model 字段更新为网关指定的模型名称。
// 其他字段保持不变，实现 OpenAI -> OpenAI 的透传。
package chat_completions

import (
	"github.com/tidwall/sjson"
)

// ConvertOpenAIRequestToOpenAI 将 OpenAI Chat Completions 请求中的模型名称替换为网关指定的模型名称。
// 仅修改 model 字段，其他所有请求参数保持不变。
//
// 参数：
//   - modelName: 网关指定的模型名称
//   - inputRawJSON: 原始的 OpenAI Chat Completions 格式 JSON 请求数据
//   - stream: 是否为流式请求（当前实现中未使用）
//
// 返回值：
//   - []byte: 更新了模型名称的 JSON 请求数据
func ConvertOpenAIRequestToOpenAI(modelName string, inputRawJSON []byte, _ bool) []byte {
	// Update the "model" field in the JSON payload with the provided modelName
	// The sjson.SetBytes function returns a new byte slice with the updated JSON.
	updatedJSON, err := sjson.SetBytes(inputRawJSON, "model", modelName)
	if err != nil {
		// If there's an error, return the original JSON or handle the error appropriately.
		// For now, we'll return the original, but in a real scenario, logging or a more robust error
		// handling mechanism would be needed.
		return inputRawJSON
	}
	return updatedJSON
}
