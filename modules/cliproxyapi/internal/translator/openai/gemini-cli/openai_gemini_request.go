// openai/gemini-cli - openai_gemini_request.go
// Package geminiCLI provides request translation functionality for Gemini CLI to OpenAI API.
// 本文件提供 Gemini CLI API 请求到 OpenAI Chat Completions API 格式的转换功能。
// 从 Gemini CLI 请求中提取内部 request 对象，进行字段规范化（如 systemInstruction -> system_instruction），
// 然后委托给 openai/gemini 包完成实际的格式转换。
package geminiCLI

import (
	. "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/openai/gemini"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ConvertGeminiCLIRequestToOpenAI parses and transforms a Gemini CLI API request into OpenAI Chat Completions API format.
// It extracts the model name, generation config, message contents, and tool declarations
// from the raw JSON request and returns them in the format expected by the OpenAI API.
// 从 Gemini CLI 请求中提取内部 request 对象，规范化 systemInstruction 字段名后，
// 委托给 ConvertGeminiRequestToOpenAI 完成实际的格式转换。
func ConvertGeminiCLIRequestToOpenAI(modelName string, inputRawJSON []byte, stream bool) []byte {
	rawJSON := inputRawJSON
	rawJSON = []byte(gjson.GetBytes(rawJSON, "request").Raw)
	rawJSON, _ = sjson.SetBytes(rawJSON, "model", modelName)
	if gjson.GetBytes(rawJSON, "systemInstruction").Exists() {
		rawJSON, _ = sjson.SetRawBytes(rawJSON, "system_instruction", []byte(gjson.GetBytes(rawJSON, "systemInstruction").Raw))
		rawJSON, _ = sjson.DeleteBytes(rawJSON, "systemInstruction")
	}

	return ConvertGeminiRequestToOpenAI(modelName, rawJSON, stream)
}
