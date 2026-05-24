// antigravity/gemini - antigravity_gemini_request.go
// Package gemini provides request translation functionality for Gemini CLI to Gemini API compatibility.
// 本文件提供 Gemini CLI API 请求到 Gemini API 格式的转换功能。
// 负责解析和转换 Gemini CLI API 请求，提取模型信息、系统指令、消息内容和工具声明。
// 主要功能包括：
// 1. 从请求中提取内部 request 对象并重组为 Gemini API 格式
// 2. 系统指令格式转换（system_instruction -> systemInstruction）
// 3. 角色规范化（缺失或无效角色的默认值填充）
// 4. 工具参数重命名（parameters -> parametersJsonSchema）
// 5. 非 Claude 模型的 thoughtSignature 跳过标记注入
// 6. CLI 工具响应格式修复和分组（fixCLIToolResponse）
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

// ConvertGeminiRequestToAntigravity parses and transforms a Gemini CLI API request into Gemini API format.
// It extracts the model name, system instruction, message contents, and tool declarations
// from the raw JSON request and returns them in the format expected by the Gemini API.
// The function performs the following transformations:
// 1. Extracts the model information from the request
// 2. Restructures the JSON to match Gemini API format
// 3. Converts system instructions to the expected format
// 4. Fixes CLI tool response format and grouping
//
// 将 Gemini API 请求转换为 Antigravity（Gemini CLI）格式。
// 主要转换步骤：
// 1. 从请求中提取模型信息并重组为 {"project":"","request":{},"model":""} 结构
// 2. 调用 fixCLIToolResponse 修复 CLI 工具响应格式和分组
// 3. 将 system_instruction 重命名为 systemInstruction
// 4. 规范化 contents 中的角色（缺失或无效时自动填充）
// 5. 将 function_declarations 中的 parameters 重命名为 parametersJsonSchema
// 6. 为非 Claude 模型的 functionCall/thought/thoughtSignature 注入跳过标记
// 7. 附加默认安全设置
//
// Parameters:
//   - modelName: The name of the model to use for the request (unused in current implementation)
//   - rawJSON: The raw JSON request data from the Gemini CLI API
//   - stream: A boolean indicating if the request is for a streaming response (unused in current implementation)
//
// Returns:
//   - []byte: The transformed request data in Gemini API format
func ConvertGeminiRequestToAntigravity(modelName string, inputRawJSON []byte, _ bool) []byte {
	rawJSON := inputRawJSON
	template := `{"project":"","request":{},"model":""}`
	templateBytes, _ := sjson.SetRawBytes([]byte(template), "request", rawJSON)
	templateBytes, _ = sjson.SetBytes(templateBytes, "model", modelName)
	template = string(templateBytes)
	template, _ = sjson.Delete(template, "request.model")

	template, errFixCLIToolResponse := fixCLIToolResponse(template)
	if errFixCLIToolResponse != nil {
		return []byte{}
	}

	systemInstructionResult := gjson.Get(template, "request.system_instruction")
	if systemInstructionResult.Exists() {
		templateBytes, _ = sjson.SetRawBytes([]byte(template), "request.systemInstruction", []byte(systemInstructionResult.Raw))
		template = string(templateBytes)
		template, _ = sjson.Delete(template, "request.system_instruction")
	}
	rawJSON = []byte(template)

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

	// Gemini-specific handling for non-Claude models:
	// - Replace client-provided thoughtSignature values with the skip sentinel.
	// - Add the same sentinel to functionCall and thinking parts so upstream can bypass signature validation.
	if !strings.Contains(strings.ToLower(modelName), "claude") {
		const skipSentinel = "skip_thought_signature_validator"

		gjson.GetBytes(rawJSON, "request.contents").ForEach(func(contentIdx, content gjson.Result) bool {
			if content.Get("role").String() == "model" {
				content.Get("parts").ForEach(func(partIdx, part gjson.Result) bool {
					if part.Get("functionCall").Exists() || part.Get("thought").Exists() || part.Get("thoughtSignature").Exists() {
						rawJSON, _ = sjson.SetBytes(rawJSON, fmt.Sprintf("request.contents.%d.parts.%d.thoughtSignature", contentIdx.Int(), partIdx.Int()), skipSentinel)
					}
					return true
				})
			}
			return true
		})
	}

	return common.AttachDefaultSafetySettings(rawJSON, "request.safetySettings")
}

// FunctionCallGroup 表示一组函数调用及其响应。
// 用于在 fixCLIToolResponse 中跟踪待匹配的函数调用组。
type FunctionCallGroup struct {
	ResponsesNeeded int      // 该组需要的响应数量
	CallNames       []string // 有序的函数调用名称列表，用于回填空的响应名称
}

// parseFunctionResponseRaw 尝试将函数响应部分规范化为 JSON 对象字符串。
// 解析失败时回退到最小化的 "functionResponse" 对象。
// fallbackName 在响应自身的 name 为空时使用。
func parseFunctionResponseRaw(response gjson.Result, fallbackName string) string {
	if response.IsObject() && gjson.Valid(response.Raw) {
		raw := response.Raw
		name := response.Get("functionResponse.name").String()
		if strings.TrimSpace(name) == "" && fallbackName != "" {
			updated, _ := sjson.SetBytes([]byte(raw), "functionResponse.name", fallbackName)
			raw = string(updated)
		}
		return raw
	}

	log.Debugf("parse function response failed, using fallback")
	funcResp := response.Get("functionResponse")
	if funcResp.Exists() {
		fr := []byte(`{"functionResponse":{"name":"","response":{"result":""}}}`)
		name := funcResp.Get("name").String()
		if strings.TrimSpace(name) == "" {
			name = fallbackName
		}
		fr, _ = sjson.SetBytes(fr, "functionResponse.name", name)
		fr, _ = sjson.SetBytes(fr, "functionResponse.response.result", funcResp.Get("response").String())
		if id := funcResp.Get("id").String(); id != "" {
			fr, _ = sjson.SetBytes(fr, "functionResponse.id", id)
		}
		return string(fr)
	}

	useName := fallbackName
	if useName == "" {
		useName = "unknown"
	}
	fr := []byte(`{"functionResponse":{"name":"","response":{"result":""}}}`)
	fr, _ = sjson.SetBytes(fr, "functionResponse.name", useName)
	fr, _ = sjson.SetBytes(fr, "functionResponse.response.result", response.String())
	return string(fr)
}

// fixCLIToolResponse performs sophisticated tool response format conversion and grouping.
// This function transforms the CLI tool response format by intelligently grouping function calls
// with their corresponding responses, ensuring proper conversation flow and API compatibility.
// It converts from a linear format (1.json) to a grouped format (2.json) where function calls
// and their responses are properly associated and structured.
//
// 修复 CLI 工具响应格式并进行智能分组。
// 将线性格式的函数调用和响应转换为分组格式，确保对话流的正确性和 API 兼容性。
// 使用 FIFO 队列机制：当模型发出 functionCall 时创建待匹配组，
// 当收到 functionResponse 时按顺序匹配最早的待匹配组，
// 将匹配的响应合并为单个 "function" 角色内容。
//
// Parameters:
//   - input: The input JSON string to be processed
//
// Returns:
//   - string: The processed JSON string with grouped function calls and responses
//   - error: An error if the processing fails
func fixCLIToolResponse(input string) (string, error) {
	// Parse the input JSON to extract the conversation structure
	parsed := gjson.Parse(input)

	// Extract the contents array which contains the conversation messages
	contents := parsed.Get("request.contents")
	if !contents.Exists() {
		// log.Debugf(input)
		return input, fmt.Errorf("contents not found in input")
	}

	// Initialize data structures for processing and grouping
	contentsWrapper := []byte(`{"contents":[]}`)
	var pendingGroups []*FunctionCallGroup // Groups awaiting completion with responses
	var collectedResponses []gjson.Result  // Standalone responses to be matched

	// Process each content object in the conversation
	// This iterates through messages and groups function calls with their responses
	contents.ForEach(func(key, value gjson.Result) bool {
		role := value.Get("role").String()
		parts := value.Get("parts")

		// Check if this content has function responses
		var responsePartsInThisContent []gjson.Result
		parts.ForEach(func(_, part gjson.Result) bool {
			if part.Get("functionResponse").Exists() {
				responsePartsInThisContent = append(responsePartsInThisContent, part)
			}
			return true
		})

		// If this content has function responses, collect them
		if len(responsePartsInThisContent) > 0 {
			collectedResponses = append(collectedResponses, responsePartsInThisContent...)

			// Check if pending groups can be satisfied (FIFO: oldest group first)
			for len(pendingGroups) > 0 && len(collectedResponses) >= pendingGroups[0].ResponsesNeeded {
				group := pendingGroups[0]
				pendingGroups = pendingGroups[1:]

				// Take the needed responses for this group
				groupResponses := collectedResponses[:group.ResponsesNeeded]
				collectedResponses = collectedResponses[group.ResponsesNeeded:]

				// Create merged function response content
				functionResponseContent := []byte(`{"parts":[],"role":"function"}`)
				for ri, response := range groupResponses {
					partRaw := parseFunctionResponseRaw(response, group.CallNames[ri])
					if partRaw != "" {
						functionResponseContent, _ = sjson.SetRawBytes(functionResponseContent, "parts.-1", []byte(partRaw))
					}
				}

				if gjson.GetBytes(functionResponseContent, "parts.#").Int() > 0 {
					contentsWrapper, _ = sjson.SetRawBytes(contentsWrapper, "contents.-1", functionResponseContent)
				}
			}

			return true // Skip adding this content, responses are merged
		}

		// If this is a model with function calls, create a new group
		if role == "model" {
			var callNames []string
			parts.ForEach(func(_, part gjson.Result) bool {
				if part.Get("functionCall").Exists() {
					callNames = append(callNames, part.Get("functionCall.name").String())
				}
				return true
			})

			if len(callNames) > 0 {
				// Add the model content
				if !value.IsObject() {
					log.Warnf("failed to parse model content")
					return true
				}
				contentsWrapper, _ = sjson.SetRawBytes(contentsWrapper, "contents.-1", []byte(value.Raw))

				// Create a new group for tracking responses
				group := &FunctionCallGroup{
					ResponsesNeeded: len(callNames),
					CallNames:       callNames,
				}
				pendingGroups = append(pendingGroups, group)
			} else {
				// Regular model content without function calls
				if !value.IsObject() {
					log.Warnf("failed to parse content")
					return true
				}
				contentsWrapper, _ = sjson.SetRawBytes(contentsWrapper, "contents.-1", []byte(value.Raw))
			}
		} else {
			// Non-model content (user, etc.)
			if !value.IsObject() {
				log.Warnf("failed to parse content")
				return true
			}
			contentsWrapper, _ = sjson.SetRawBytes(contentsWrapper, "contents.-1", []byte(value.Raw))
		}

		return true
	})

	// Handle any remaining pending groups with remaining responses
	for _, group := range pendingGroups {
		if len(collectedResponses) >= group.ResponsesNeeded {
			groupResponses := collectedResponses[:group.ResponsesNeeded]
			collectedResponses = collectedResponses[group.ResponsesNeeded:]

			functionResponseContent := []byte(`{"parts":[],"role":"function"}`)
			for ri, response := range groupResponses {
				partRaw := parseFunctionResponseRaw(response, group.CallNames[ri])
				if partRaw != "" {
					functionResponseContent, _ = sjson.SetRawBytes(functionResponseContent, "parts.-1", []byte(partRaw))
				}
			}

			if gjson.GetBytes(functionResponseContent, "parts.#").Int() > 0 {
				contentsWrapper, _ = sjson.SetRawBytes(contentsWrapper, "contents.-1", functionResponseContent)
			}
		}
	}

	// Update the original JSON with the new contents
	result, _ := sjson.SetRawBytes([]byte(input), "request.contents", []byte(gjson.GetBytes(contentsWrapper, "contents").Raw))

	return string(result), nil
}
