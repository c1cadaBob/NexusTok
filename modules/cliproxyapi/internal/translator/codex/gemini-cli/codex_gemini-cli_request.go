// codex/gemini-cli - codex_gemini-cli_request.go
// Package geminiCLI provides request translation functionality for Gemini CLI to Codex API compatibility.
// 本文件提供 Gemini CLI API 请求到 Codex API 格式的转换功能。
// 从 Gemini CLI 请求中提取内部 request 对象，恢复模型信息，
// 将 systemInstruction 重命名为 system_instruction，然后委托给
// codex/gemini 包的 ConvertGeminiRequestToCodex 函数进行进一步转换。
package geminiCLI

import (
	. "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/codex/gemini"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ConvertGeminiCLIRequestToCodex parses and transforms a Gemini CLI API request into Codex API format.
// It extracts the model name, system instruction, message contents, and tool declarations
// from the raw JSON request and returns them in the format expected by the Codex API.
// The function performs the following transformations:
// 1. Extracts the inner request object and promotes it to the top level
// 2. Restores the model information at the top level
// 3. Converts systemInstruction field to system_instruction for Codex compatibility
// 4. Delegates to the Gemini-to-Codex conversion function for further processing
//
// Parameters:
//   - modelName: The name of the model to use for the request
//   - rawJSON: The raw JSON request data from the Gemini CLI API
//   - stream: A boolean indicating if the request is for a streaming response
//
// Returns:
//   - []byte: The transformed request data in Codex API format
func ConvertGeminiCLIRequestToCodex(modelName string, inputRawJSON []byte, stream bool) []byte {
	rawJSON := inputRawJSON

	rawJSON = []byte(gjson.GetBytes(rawJSON, "request").Raw)
	rawJSON, _ = sjson.SetBytes(rawJSON, "model", modelName)
	if gjson.GetBytes(rawJSON, "systemInstruction").Exists() {
		rawJSON, _ = sjson.SetRawBytes(rawJSON, "system_instruction", []byte(gjson.GetBytes(rawJSON, "systemInstruction").Raw))
		rawJSON, _ = sjson.DeleteBytes(rawJSON, "systemInstruction")
	}

	return ConvertGeminiRequestToCodex(modelName, rawJSON, stream)
}
