// gemini-cli/claude - gemini-cli_claude_request.go
// 提供将 Claude Code API 请求转换为 Gemini CLI 兼容格式的功能。
// 负责将 Claude 的消息内容、系统指令、工具声明、tool_choice、thinking 配置
// 以及生成参数（temperature、top_p、top_k）映射为 Gemini CLI API 期望的 JSON 结构。
package claude

import (
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/translator/gemini/common"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/util"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// geminiCLIClaudeThoughtSignature 是用于跳过思维签名验证的常量标记。
// 在 Claude -> Gemini CLI 转换过程中，为 functionCall 和 thoughtSignature 部分
// 添加此标记以绕过 Gemini CLI 的思维签名校验。
const geminiCLIClaudeThoughtSignature = "skip_thought_signature_validator"

// ConvertClaudeRequestToCLI 将 Claude Code API 请求解析并转换为 Gemini CLI API 格式。
// 从原始 JSON 请求中提取模型名称、系统指令、消息内容和工具声明，
// 并按照 Gemini CLI API 期望的格式重新组织。
// 执行以下转换步骤：
// 1. 从请求中提取模型信息
// 2. 重构 JSON 结构以匹配 Gemini CLI API 格式
// 3. 将系统指令转换为 Gemini CLI 期望的格式
// 4. 映射消息内容并进行角色转换（assistant -> model）
// 5. 处理工具声明和工具选择（tool_choice）
// 6. 映射生成配置参数（temperature、top_p、top_k）和 thinking 配置
// 7. 附加默认安全设置
//
// 参数:
//   - modelName: 用于请求的模型名称
//   - inputRawJSON: 来自 Claude Code API 的原始 JSON 请求数据
//   - _: 是否为流式响应（当前实现中未使用）
//
// 返回:
//   - []byte: Gemini CLI API 格式的转换后请求数据
func ConvertClaudeRequestToCLI(modelName string, inputRawJSON []byte, _ bool) []byte {
	rawJSON := inputRawJSON

	// 构建输出的 Gemini CLI 请求 JSON 模板
	out := []byte(`{"model":"","request":{"contents":[]}}`)
	out, _ = sjson.SetBytes(out, "model", modelName)

	// 处理系统指令：将 Claude 的 system 数组转换为 Gemini CLI 的 systemInstruction 格式
	// system instruction
	if systemResult := gjson.GetBytes(rawJSON, "system"); systemResult.IsArray() {
		systemInstruction := []byte(`{"role":"user","parts":[]}`)
		hasSystemParts := false
		systemResult.ForEach(func(_, systemPromptResult gjson.Result) bool {
			if systemPromptResult.Get("type").String() == "text" {
				textResult := systemPromptResult.Get("text")
				if textResult.Type == gjson.String {
					if util.IsClaudeCodeAttributionSystemText(textResult.String()) {
						return true
					}
					part := []byte(`{"text":""}`)
					part, _ = sjson.SetBytes(part, "text", textResult.String())
					systemInstruction, _ = sjson.SetRawBytes(systemInstruction, "parts.-1", part)
					hasSystemParts = true
				}
			}
			return true
		})
		if hasSystemParts {
			out, _ = sjson.SetRawBytes(out, "request.systemInstruction", systemInstruction)
		}
	} else if systemResult.Type == gjson.String && !util.IsClaudeCodeAttributionSystemText(systemResult.String()) {
		out, _ = sjson.SetBytes(out, "request.systemInstruction.parts.-1.text", systemResult.String())
	}

	// 处理消息内容：遍历 Claude 的 messages 数组，将角色和内容转换为 Gemini CLI 格式
	// contents
	if messagesResult := gjson.GetBytes(rawJSON, "messages"); messagesResult.IsArray() {
		messagesResult.ForEach(func(_, messageResult gjson.Result) bool {
			roleResult := messageResult.Get("role")
			if roleResult.Type != gjson.String {
				return true
			}
			role := roleResult.String()
			if role == "assistant" {
				role = "model"
			}

			contentJSON := []byte(`{"role":"","parts":[]}`)
			contentJSON, _ = sjson.SetBytes(contentJSON, "role", role)

			contentsResult := messageResult.Get("content")
			if contentsResult.IsArray() {
				contentsResult.ForEach(func(_, contentResult gjson.Result) bool {
					switch contentResult.Get("type").String() {
					case "text":
						part := []byte(`{"text":""}`)
						part, _ = sjson.SetBytes(part, "text", contentResult.Get("text").String())
						contentJSON, _ = sjson.SetRawBytes(contentJSON, "parts.-1", part)

					case "tool_use":
						functionName := util.SanitizeFunctionName(contentResult.Get("name").String())
						functionArgs := contentResult.Get("input").String()
						argsResult := gjson.Parse(functionArgs)
						if argsResult.IsObject() && gjson.Valid(functionArgs) {
							part := []byte(`{"thoughtSignature":"","functionCall":{"name":"","args":{}}}`)
							part, _ = sjson.SetBytes(part, "thoughtSignature", geminiCLIClaudeThoughtSignature)
							part, _ = sjson.SetBytes(part, "functionCall.name", functionName)
							part, _ = sjson.SetRawBytes(part, "functionCall.args", []byte(functionArgs))
							contentJSON, _ = sjson.SetRawBytes(contentJSON, "parts.-1", part)
						}

					case "tool_result":
						toolCallID := contentResult.Get("tool_use_id").String()
						if toolCallID == "" {
							return true
						}
						funcName := toolCallID
						toolCallIDs := strings.Split(toolCallID, "-")
						if len(toolCallIDs) > 1 {
							funcName = strings.Join(toolCallIDs[0:len(toolCallIDs)-1], "-")
						}
						responseData := contentResult.Get("content").Raw
						part := []byte(`{"functionResponse":{"name":"","response":{"result":""}}}`)
						part, _ = sjson.SetBytes(part, "functionResponse.name", util.SanitizeFunctionName(funcName))
						part, _ = sjson.SetBytes(part, "functionResponse.response.result", responseData)
						contentJSON, _ = sjson.SetRawBytes(contentJSON, "parts.-1", part)

					case "image":
						source := contentResult.Get("source")
						if source.Get("type").String() == "base64" {
							mimeType := source.Get("media_type").String()
							data := source.Get("data").String()
							if mimeType != "" && data != "" {
								part := []byte(`{"inlineData":{"mime_type":"","data":""}}`)
								part, _ = sjson.SetBytes(part, "inlineData.mime_type", mimeType)
								part, _ = sjson.SetBytes(part, "inlineData.data", data)
								contentJSON, _ = sjson.SetRawBytes(contentJSON, "parts.-1", part)
							}
						}
					}
					return true
				})
				out, _ = sjson.SetRawBytes(out, "request.contents.-1", contentJSON)
			} else if contentsResult.Type == gjson.String {
				part := []byte(`{"text":""}`)
				part, _ = sjson.SetBytes(part, "text", contentsResult.String())
				contentJSON, _ = sjson.SetRawBytes(contentJSON, "parts.-1", part)
				out, _ = sjson.SetRawBytes(out, "request.contents.-1", contentJSON)
			}
			return true
		})
	}

	// 处理工具声明：将 Claude 的 tools 数组转换为 Gemini CLI 的 functionDeclarations 格式
	// tools
	if toolsResult := gjson.GetBytes(rawJSON, "tools"); toolsResult.IsArray() {
		hasTools := false
		toolsResult.ForEach(func(_, toolResult gjson.Result) bool {
			inputSchemaResult := toolResult.Get("input_schema")
			if inputSchemaResult.Exists() && inputSchemaResult.IsObject() {
				inputSchema := util.CleanJSONSchemaForGemini(inputSchemaResult.Raw)
				tool, _ := sjson.DeleteBytes([]byte(toolResult.Raw), "input_schema")
				tool, _ = sjson.SetRawBytes(tool, "parametersJsonSchema", []byte(inputSchema))
				tool, _ = sjson.SetBytes(tool, "name", util.SanitizeFunctionName(gjson.GetBytes(tool, "name").String()))
				tool, _ = sjson.DeleteBytes(tool, "strict")
				tool, _ = sjson.DeleteBytes(tool, "input_examples")
				tool, _ = sjson.DeleteBytes(tool, "type")
				tool, _ = sjson.DeleteBytes(tool, "cache_control")
				tool, _ = sjson.DeleteBytes(tool, "defer_loading")
				tool, _ = sjson.DeleteBytes(tool, "eager_input_streaming")
				if gjson.ValidBytes(tool) && gjson.ParseBytes(tool).IsObject() {
					if !hasTools {
						out, _ = sjson.SetRawBytes(out, "request.tools", []byte(`[{"functionDeclarations":[]}]`))
						hasTools = true
					}
					out, _ = sjson.SetRawBytes(out, "request.tools.0.functionDeclarations.-1", tool)
				}
			}
			return true
		})
		if !hasTools {
			out, _ = sjson.DeleteBytes(out, "request.tools")
		}
	}

	// 处理工具选择：将 Claude 的 tool_choice 映射到 Gemini CLI 的 functionCallingConfig.mode
	// tool_choice
	toolChoiceResult := gjson.GetBytes(rawJSON, "tool_choice")
	if toolChoiceResult.Exists() {
		toolChoiceType := ""
		toolChoiceName := ""
		if toolChoiceResult.IsObject() {
			toolChoiceType = toolChoiceResult.Get("type").String()
			toolChoiceName = toolChoiceResult.Get("name").String()
		} else if toolChoiceResult.Type == gjson.String {
			toolChoiceType = toolChoiceResult.String()
		}

		switch toolChoiceType {
		case "auto":
			out, _ = sjson.SetBytes(out, "request.toolConfig.functionCallingConfig.mode", "AUTO")
		case "none":
			out, _ = sjson.SetBytes(out, "request.toolConfig.functionCallingConfig.mode", "NONE")
		case "any":
			out, _ = sjson.SetBytes(out, "request.toolConfig.functionCallingConfig.mode", "ANY")
		case "tool":
			out, _ = sjson.SetBytes(out, "request.toolConfig.functionCallingConfig.mode", "ANY")
			if toolChoiceName != "" {
				out, _ = sjson.SetBytes(out, "request.toolConfig.functionCallingConfig.allowedFunctionNames", []string{util.SanitizeFunctionName(toolChoiceName)})
			}
		}
	}

	// 将 Anthropic thinking 配置映射到 Gemini CLI 的 thinkingConfig
	// Map Anthropic thinking -> Gemini CLI thinkingConfig when enabled
	// Translator only does format conversion, ApplyThinking handles model capability validation.
	if t := gjson.GetBytes(rawJSON, "thinking"); t.Exists() && t.IsObject() {
		switch t.Get("type").String() {
		case "enabled":
			if b := t.Get("budget_tokens"); b.Exists() && b.Type == gjson.Number {
				budget := int(b.Int())
				out, _ = sjson.SetBytes(out, "request.generationConfig.thinkingConfig.thinkingBudget", budget)
				out, _ = sjson.SetBytes(out, "request.generationConfig.thinkingConfig.includeThoughts", true)
			}
		case "adaptive", "auto":
			// For adaptive thinking:
			// - If output_config.effort is explicitly present, pass through as thinkingLevel.
			// - Otherwise, treat it as "enabled with target-model maximum" and emit high.
			// ApplyThinking handles clamping to target model's supported levels.
			effort := ""
			if v := gjson.GetBytes(rawJSON, "output_config.effort"); v.Exists() && v.Type == gjson.String {
				effort = strings.ToLower(strings.TrimSpace(v.String()))
			}
			if effort != "" {
				out, _ = sjson.SetBytes(out, "request.generationConfig.thinkingConfig.thinkingLevel", effort)
			} else {
				out, _ = sjson.SetBytes(out, "request.generationConfig.thinkingConfig.thinkingLevel", "high")
			}
			out, _ = sjson.SetBytes(out, "request.generationConfig.thinkingConfig.includeThoughts", true)
		}
	}
	if v := gjson.GetBytes(rawJSON, "temperature"); v.Exists() && v.Type == gjson.Number {
		out, _ = sjson.SetBytes(out, "request.generationConfig.temperature", v.Num)
	}
	if v := gjson.GetBytes(rawJSON, "top_p"); v.Exists() && v.Type == gjson.Number {
		out, _ = sjson.SetBytes(out, "request.generationConfig.topP", v.Num)
	}
	if v := gjson.GetBytes(rawJSON, "top_k"); v.Exists() && v.Type == gjson.Number {
		out, _ = sjson.SetBytes(out, "request.generationConfig.topK", v.Num)
	}

	out = common.AttachDefaultSafetySettings(out, "request.safetySettings")
	return out
}
