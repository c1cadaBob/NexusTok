// gemini/gemini-cli - gemini_gemini-cli_request.go
// Package geminiCLI provides request translation functionality for Claude API.
// 本文件提供 Gemini CLI API 请求到 Gemini API 格式的转换功能。
// 负责从 Gemini CLI 请求中提取内部 request 对象，恢复模型信息，
// 将 systemInstruction 重命名为 system_instruction，并附加默认安全设置。
// 同时处理工具参数的 parameters 到 parametersJsonSchema 重命名，
// 以及为模型轮次中的 functionCall 部分注入 thoughtSignature 跳过标记。
package geminiCLI

import (
	"fmt"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/translator/gemini/common"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/util"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ConvertGeminiCLIRequestToGemini parses and transforms a Gemini CLI API request into Gemini API format.
// 将 Gemini CLI API 请求转换为 Gemini API 格式。
// 从 CLI 请求中提取内部 request 对象，恢复模型信息，
// 将 systemInstruction 重命名为 system_instruction，
// 将 parameters 重命名为 parametersJsonSchema，
// 为模型轮次中的 functionCall/thoughtSignature 注入跳过标记，
// 并附加默认安全设置。
func ConvertGeminiCLIRequestToGemini(_ string, inputRawJSON []byte, _ bool) []byte {
	rawJSON := inputRawJSON
	modelResult := gjson.GetBytes(rawJSON, "model")
	rawJSON = []byte(gjson.GetBytes(rawJSON, "request").Raw)
	rawJSON, _ = sjson.SetBytes(rawJSON, "model", modelResult.String())
	if gjson.GetBytes(rawJSON, "systemInstruction").Exists() {
		rawJSON, _ = sjson.SetRawBytes(rawJSON, "system_instruction", []byte(gjson.GetBytes(rawJSON, "systemInstruction").Raw))
		rawJSON, _ = sjson.DeleteBytes(rawJSON, "systemInstruction")
	}

	toolsResult := gjson.GetBytes(rawJSON, "tools")
	if toolsResult.Exists() && toolsResult.IsArray() {
		toolResults := toolsResult.Array()
		for i := 0; i < len(toolResults); i++ {
			functionDeclarationsResult := gjson.GetBytes(rawJSON, fmt.Sprintf("tools.%d.function_declarations", i))
			if functionDeclarationsResult.Exists() && functionDeclarationsResult.IsArray() {
				functionDeclarationsResults := functionDeclarationsResult.Array()
				for j := 0; j < len(functionDeclarationsResults); j++ {
					parametersResult := gjson.GetBytes(rawJSON, fmt.Sprintf("tools.%d.function_declarations.%d.parameters", i, j))
					if parametersResult.Exists() {
						strJson, _ := util.RenameKey(string(rawJSON), fmt.Sprintf("tools.%d.function_declarations.%d.parameters", i, j), fmt.Sprintf("tools.%d.function_declarations.%d.parametersJsonSchema", i, j))
						rawJSON = []byte(strJson)
					}
				}
			}
		}
	}

	gjson.GetBytes(rawJSON, "contents").ForEach(func(key, content gjson.Result) bool {
		if content.Get("role").String() == "model" {
			content.Get("parts").ForEach(func(partKey, part gjson.Result) bool {
				if part.Get("functionCall").Exists() {
					rawJSON, _ = sjson.SetBytes(rawJSON, fmt.Sprintf("contents.%d.parts.%d.thoughtSignature", key.Int(), partKey.Int()), "skip_thought_signature_validator")
				} else if part.Get("thoughtSignature").Exists() {
					rawJSON, _ = sjson.SetBytes(rawJSON, fmt.Sprintf("contents.%d.parts.%d.thoughtSignature", key.Int(), partKey.Int()), "skip_thought_signature_validator")
				}
				return true
			})
		}
		return true
	})

	return common.AttachDefaultSafetySettings(rawJSON, "safetySettings")
}
