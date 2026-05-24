// gemini-cli/gemini - gemini-cli_gemini_request.go
// 提供将 Gemini API 请求转换为 Gemini CLI 格式的功能。
// 负责将 Gemini API 的消息内容、系统指令、工具声明重新组织为 Gemini CLI 期望的结构，
// 包括：system_instruction -> systemInstruction 重命名、parameters -> parametersJsonSchema 重命名、
// 角色规范化、函数调用响应分组合并、思维签名处理，以及空 parts 内容的过滤。
package gemini

import (
	"fmt"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/translator/gemini/common"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/util"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ConvertGeminiRequestToGeminiCLI 将 Gemini API 请求解析并转换为 Gemini CLI 格式。
// 从原始 JSON 请求中提取模型名称、系统指令、消息内容和工具声明，
// 并按照 Gemini CLI API 期望的格式重新组织。
// 执行以下转换步骤：
// 1. 从请求中提取模型信息并移至顶层
// 2. 重构 JSON 结构以匹配 Gemini CLI API 格式（添加 project、request 包装）
// 3. 修复 CLI 工具响应格式和分组（合并函数调用与对应的函数响应）
// 4. 将 system_instruction 重命名为 systemInstruction
// 5. 规范化 contents 中的角色（确保交替的 user/model 模式）
// 6. 将 parameters 重命名为 parametersJsonSchema
// 7. 处理思维签名（thoughtSignature）
// 8. 过滤掉空 parts 的 contents 以避免 Gemini API 错误
// 9. 附加默认安全设置
//
// 参数:
//   - _: 模型名称（当前未使用，从请求中提取）
//   - inputRawJSON: 来自 Gemini API 的原始 JSON 请求数据
//   - _: 是否为流式响应（当前未使用）
//
// 返回:
//   - []byte: Gemini CLI API 格式的转换后请求数据
func ConvertGeminiRequestToGeminiCLI(_ string, inputRawJSON []byte, _ bool) []byte {
	rawJSON := inputRawJSON
	// 构建 Gemini CLI 请求模板，将原始请求嵌套到 request 字段中
	template := []byte(`{"project":"","request":{},"model":""}`)
	template, _ = sjson.SetRawBytes(template, "request", rawJSON)
	template, _ = sjson.SetBytes(template, "model", gjson.GetBytes(template, "request.model").String())
	template, _ = sjson.DeleteBytes(template, "request.model")

	// 修复 CLI 工具响应格式，将线性格式转换为分组格式
	templateStr, errFixCLIToolResponse := fixCLIToolResponse(string(template))
	if errFixCLIToolResponse != nil {
		return []byte{}
	}
	template = []byte(templateStr)

	// 将 system_instruction 重命名为 systemInstruction
	systemInstructionResult := gjson.GetBytes(template, "request.system_instruction")
	if systemInstructionResult.Exists() {
		template, _ = sjson.SetRawBytes(template, "request.systemInstruction", []byte(systemInstructionResult.Raw))
		template, _ = sjson.DeleteBytes(template, "request.system_instruction")
	}
	rawJSON = template

	// 规范化 request.contents 中的角色：确保缺少或无效的角色被替换为有效的 user/model 交替模式
	// Normalize roles in request.contents: default to valid values if missing/invalid
	contents := gjson.GetBytes(rawJSON, "request.contents")
	if contents.Exists() {
		prevRole := ""
		idx := 0
		contents.ForEach(func(_ gjson.Result, value gjson.Result) bool {
			role := value.Get("role").String()
			valid := role == "user" || role == "model"
			if role == "" || !valid {
				var newRole string
				if prevRole == "" {
					newRole = "user"
				} else if prevRole == "user" {
					newRole = "model"
				} else {
					newRole = "user"
				}
				path := fmt.Sprintf("request.contents.%d.role", idx)
				rawJSON, _ = sjson.SetBytes(rawJSON, path, newRole)
				role = newRole
			}
			prevRole = role
			idx++
			return true
		})
	}

	// 将工具声明中的 parameters 重命名为 parametersJsonSchema
	toolsResult := gjson.GetBytes(rawJSON, "request.tools")
	if toolsResult.Exists() && toolsResult.IsArray() {
		toolResults := toolsResult.Array()
		for i := 0; i < len(toolResults); i++ {
			functionDeclarationsResult := gjson.GetBytes(rawJSON, fmt.Sprintf("request.tools.%d.function_declarations", i))
			if functionDeclarationsResult.Exists() && functionDeclarationsResult.IsArray() {
				functionDeclarationsResults := functionDeclarationsResult.Array()
				for j := 0; j < len(functionDeclarationsResults); j++ {
					parametersResult := gjson.GetBytes(rawJSON, fmt.Sprintf("request.tools.%d.function_declarations.%d.parameters", i, j))
					if parametersResult.Exists() {
						strJson, _ := util.RenameKey(string(rawJSON), fmt.Sprintf("request.tools.%d.function_declarations.%d.parameters", i, j), fmt.Sprintf("request.tools.%d.function_declarations.%d.parametersJsonSchema", i, j))
						rawJSON = []byte(strJson)
					}
				}
			}
		}
	}

	// 为模型的 functionCall 和 thoughtSignature 部分添加思维签名标记
	gjson.GetBytes(rawJSON, "request.contents").ForEach(func(key, content gjson.Result) bool {
		if content.Get("role").String() == "model" {
			content.Get("parts").ForEach(func(partKey, part gjson.Result) bool {
				if part.Get("functionCall").Exists() {
					rawJSON, _ = sjson.SetBytes(rawJSON, fmt.Sprintf("request.contents.%d.parts.%d.thoughtSignature", key.Int(), partKey.Int()), "skip_thought_signature_validator")
				} else if part.Get("thoughtSignature").Exists() {
					rawJSON, _ = sjson.SetBytes(rawJSON, fmt.Sprintf("request.contents.%d.parts.%d.thoughtSignature", key.Int(), partKey.Int()), "skip_thought_signature_validator")
				}
				return true
			})
		}
		return true
	})

	// 过滤掉包含空 parts 的 contents，避免 Gemini API 错误：
	// "required oneof field 'data' must have one initialized field"
	// Filter out contents with empty parts to avoid Gemini API error:
	// "required oneof field 'data' must have one initialized field"
	filteredContents := []byte(`[]`)
	hasFiltered := false
	gjson.GetBytes(rawJSON, "request.contents").ForEach(func(_, content gjson.Result) bool {
		parts := content.Get("parts")
		if !parts.IsArray() || len(parts.Array()) == 0 {
			hasFiltered = true
			return true
		}
		filteredContents, _ = sjson.SetRawBytes(filteredContents, "-1", []byte(content.Raw))
		return true
	})
	if hasFiltered {
		rawJSON, _ = sjson.SetRawBytes(rawJSON, "request.contents", filteredContents)
	}

	return common.AttachDefaultSafetySettings(rawJSON, "request.safetySettings")
}

// FunctionCallGroup 表示一组函数调用及其对应的响应。
// 用于将分散的函数调用和函数响应正确配对并合并，
// 确保对话流的正确性和 API 兼容性。
type FunctionCallGroup struct {
	ResponsesNeeded int      // 此组需要的函数响应数量
	CallNames       []string // 有序的函数调用名称列表，用于回填空的响应名称
}

// backfillFunctionResponseName 确保 functionResponse JSON 对象具有非空的 name 字段。
// 如果原始名称为空，则使用 fallbackName 作为后备值。
// 这在函数响应名称丢失或为空时提供兜底机制。
func backfillFunctionResponseName(raw string, fallbackName string) string {
	name := gjson.Get(raw, "functionResponse.name").String()
	if strings.TrimSpace(name) == "" && fallbackName != "" {
		rawBytes, _ := sjson.SetBytes([]byte(raw), "functionResponse.name", fallbackName)
		raw = string(rawBytes)
	}
	return raw
}

// fixCLIToolResponse 执行复杂的工具响应格式转换和分组操作。
// 将 CLI 工具响应从线性格式（每个函数调用和响应独立存在）转换为
// 分组格式（函数调用与对应的响应被正确关联和结构化）。
// 使用 FIFO 队列管理待处理的函数调用组，并按顺序匹配收集到的函数响应。
//
// 参数:
//   - input: 待处理的输入 JSON 字符串
//
// 返回:
//   - string: 处理后的 JSON 字符串，包含已分组的函数调用和响应
//   - error: 处理失败时返回的错误
func fixCLIToolResponse(input string) (string, error) {
	// 解析输入 JSON 以提取对话结构
	parsed := gjson.Parse(input)

	// 提取包含对话消息的 contents 数组
	contents := parsed.Get("request.contents")
	if !contents.Exists() {
		// log.Debugf(input)
		return input, fmt.Errorf("contents not found in input")
	}

	// 初始化数据结构用于处理和分组
	// pendingGroups: 等待响应完成的函数调用组队列（FIFO）
	// collectedResponses: 已收集但尚未匹配的独立响应列表
	contentsWrapper := []byte(`{"contents":[]}`)
	var pendingGroups []*FunctionCallGroup // Groups awaiting completion with responses
	var collectedResponses []gjson.Result  // Standalone responses to be matched

	// 遍历对话中的每个内容对象
	// 迭代消息并将函数调用与对应的响应进行分组
	// Process each content object in the conversation
	// This iterates through messages and groups function calls with their responses
	contents.ForEach(func(key, value gjson.Result) bool {
		role := value.Get("role").String()
		parts := value.Get("parts")

		// 检查此内容是否包含函数响应
		var responsePartsInThisContent []gjson.Result
		parts.ForEach(func(_, part gjson.Result) bool {
			if part.Get("functionResponse").Exists() {
				responsePartsInThisContent = append(responsePartsInThisContent, part)
			}
			return true
		})

		// 如果此内容包含函数响应，则收集它们
		if len(responsePartsInThisContent) > 0 {
			collectedResponses = append(collectedResponses, responsePartsInThisContent...)

			// 检查待处理组是否可以被满足（FIFO：最旧的组优先）
			for len(pendingGroups) > 0 && len(collectedResponses) >= pendingGroups[0].ResponsesNeeded {
				group := pendingGroups[0]
				pendingGroups = pendingGroups[1:]

				// 取出此组所需的响应数量
				groupResponses := collectedResponses[:group.ResponsesNeeded]
				collectedResponses = collectedResponses[group.ResponsesNeeded:]

				// 创建合并后的函数响应内容
				functionResponseContent := []byte(`{"parts":[],"role":"function"}`)
				for ri, response := range groupResponses {
					if !response.IsObject() {
						log.Warnf("failed to parse function response")
						continue
					}
					raw := backfillFunctionResponseName(response.Raw, group.CallNames[ri])
					functionResponseContent, _ = sjson.SetRawBytes(functionResponseContent, "parts.-1", []byte(raw))
				}

				if gjson.GetBytes(functionResponseContent, "parts.#").Int() > 0 {
					contentsWrapper, _ = sjson.SetRawBytes(contentsWrapper, "contents.-1", functionResponseContent)
				}
			}

			return true // 跳过添加此内容，响应已被合并
		}

		// 如果是包含函数调用的模型内容，创建新的分组
		if role == "model" {
			var callNames []string
			parts.ForEach(func(_, part gjson.Result) bool {
				if part.Get("functionCall").Exists() {
					callNames = append(callNames, part.Get("functionCall.name").String())
				}
				return true
			})

			if len(callNames) > 0 {
				// 将模型内容添加到输出
				if !value.IsObject() {
					log.Warnf("failed to parse model content")
					return true
				}
				contentsWrapper, _ = sjson.SetRawBytes(contentsWrapper, "contents.-1", []byte(value.Raw))

				// 创建新的函数调用分组用于跟踪响应
				group := &FunctionCallGroup{
					ResponsesNeeded: len(callNames),
					CallNames:       callNames,
				}
				pendingGroups = append(pendingGroups, group)
			} else {
				// 不含函数调用的普通模型内容
				// Regular model content without function calls
				if !value.IsObject() {
					log.Warnf("failed to parse content")
					return true
				}
				contentsWrapper, _ = sjson.SetRawBytes(contentsWrapper, "contents.-1", []byte(value.Raw))
			}
		} else {
			// 非模型内容（用户消息等）
			// Non-model content (user, etc.)
			if !value.IsObject() {
				log.Warnf("failed to parse content")
				return true
			}
			contentsWrapper, _ = sjson.SetRawBytes(contentsWrapper, "contents.-1", []byte(value.Raw))
		}

		return true
	})

	// 处理剩余的待处理组和剩余的响应
	// Handle any remaining pending groups with remaining responses
	for _, group := range pendingGroups {
		if len(collectedResponses) >= group.ResponsesNeeded {
			groupResponses := collectedResponses[:group.ResponsesNeeded]
			collectedResponses = collectedResponses[group.ResponsesNeeded:]

			functionResponseContent := []byte(`{"parts":[],"role":"function"}`)
			for ri, response := range groupResponses {
				if !response.IsObject() {
					log.Warnf("failed to parse function response")
					continue
				}
				raw := backfillFunctionResponseName(response.Raw, group.CallNames[ri])
				functionResponseContent, _ = sjson.SetRawBytes(functionResponseContent, "parts.-1", []byte(raw))
			}

			if gjson.GetBytes(functionResponseContent, "parts.#").Int() > 0 {
				contentsWrapper, _ = sjson.SetRawBytes(contentsWrapper, "contents.-1", functionResponseContent)
			}
		}
	}

	// 用新的 contents 更新原始 JSON
	// Update the original JSON with the new contents
	result := []byte(input)
	result, _ = sjson.SetRawBytes(result, "request.contents", []byte(gjson.GetBytes(contentsWrapper, "contents").Raw))

	return string(result), nil
}
