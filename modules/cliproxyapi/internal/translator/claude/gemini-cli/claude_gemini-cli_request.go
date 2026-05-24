// claude/gemini-cli - claude_gemini-cli_request.go
// 提供将 Gemini CLI API 请求转换为 Claude Code API 格式的功能。
// 从 Gemini CLI 的嵌套请求结构中提取 model 和 request 字段，
// 将 systemInstruction 重命名为 system_instruction，
// 然后委托给 Gemini -> Claude 的转换函数完成剩余的格式转换。
package geminiCLI

import (
	. "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/claude/gemini"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ConvertGeminiCLIRequestToClaude 将 Gemini CLI API 请求解析并转换为 Claude Code API 格式。
// 从 Gemini CLI 的嵌套 JSON 结构中提取模型名称和内部请求对象，
// 将 systemInstruction 重命名为 system_instruction 以匹配 Claude Code 的格式要求，
// 然后委托给 ConvertGeminiRequestToClaude 进行进一步的格式转换。
//
// 参数:
//   - modelName: 用于请求的模型名称
//   - inputRawJSON: 来自 Gemini CLI API 的原始 JSON 请求数据
//   - stream: 是否为流式响应
//
// 返回:
//   - []byte: Claude Code API 格式的转换后请求数据
func ConvertGeminiCLIRequestToClaude(modelName string, inputRawJSON []byte, stream bool) []byte {
	rawJSON := inputRawJSON

	modelResult := gjson.GetBytes(rawJSON, "model")
	// Extract the inner request object and promote it to the top level
	// 提取嵌套的 request 对象并将其提升到顶层
	rawJSON = []byte(gjson.GetBytes(rawJSON, "request").Raw)
	// 将模型信息恢复到顶层
	rawJSON, _ = sjson.SetBytes(rawJSON, "model", modelResult.String())
	// 将 systemInstruction 字段重命名为 system_instruction 以兼容 Claude Code 格式
	if gjson.GetBytes(rawJSON, "systemInstruction").Exists() {
		rawJSON, _ = sjson.SetRawBytes(rawJSON, "system_instruction", []byte(gjson.GetBytes(rawJSON, "systemInstruction").Raw))
		rawJSON, _ = sjson.DeleteBytes(rawJSON, "systemInstruction")
	}
	// Delegate to the Gemini-to-Claude conversion function for further processing
	// 委托给 Gemini -> Claude 转换函数进行进一步处理
	return ConvertGeminiRequestToClaude(modelName, rawJSON, stream)
}
