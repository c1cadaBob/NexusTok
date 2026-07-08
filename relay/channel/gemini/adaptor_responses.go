// adaptor_responses.go 提供 Gemini 渠道的 OpenAI Responses 请求预处理。
// Gemini 当前没有 Responses API 的一比一语义，本文件只保留能安全映射到 Gemini Chat 的子集，
// 对 custom/freeform 工具调用做显式过滤，避免把无法保真的工具语义误交给上游。
package gemini

import (
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/dto"
)

const (
	geminiResponsesInputTypeCustomToolCall       = "custom_tool_call"
	geminiResponsesInputTypeCustomToolCallOutput = "custom_tool_call_output"
	geminiResponsesInputTypeFunctionCallOutput   = "function_call_output"
)

func preprocessGeminiOpenAIResponsesRequest(request dto.OpenAIResponsesRequest) (dto.OpenAIResponsesRequest, error) {
	tools, err := filterGeminiResponsesTools(request.Tools)
	if err != nil {
		return request, err
	}
	request.Tools = tools

	input, err := filterGeminiResponsesInput(request.Input)
	if err != nil {
		return request, err
	}
	request.Input = input

	return request, nil
}

func filterGeminiResponsesTools(raw []byte) ([]byte, error) {
	if !geminiRawJSONPresent(raw) || common.GetJsonType(raw) != "array" {
		return raw, nil
	}

	var tools []map[string]any
	if err := common.Unmarshal(raw, &tools); err != nil {
		return nil, err
	}

	filtered := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		if strings.TrimSpace(common.Interface2String(tool["type"])) != "function" {
			// Gemini 没有 Responses custom/freeform tools 的安全等价表达，当前只能过滤。
			continue
		}
		filtered = append(filtered, tool)
	}
	if len(filtered) == 0 {
		return nil, nil
	}
	return common.Marshal(filtered)
}

func filterGeminiResponsesInput(raw []byte) ([]byte, error) {
	if !geminiRawJSONPresent(raw) || common.GetJsonType(raw) != "array" {
		return raw, nil
	}

	var items []map[string]any
	if err := common.Unmarshal(raw, &items); err != nil {
		return nil, err
	}

	skippedCustomCallIDs := make(map[string]struct{})
	for _, item := range items {
		if strings.TrimSpace(common.Interface2String(item["type"])) != geminiResponsesInputTypeCustomToolCall {
			continue
		}
		if callID := strings.TrimSpace(common.Interface2String(item["call_id"])); callID != "" {
			skippedCustomCallIDs[callID] = struct{}{}
		}
	}

	filtered := make([]map[string]any, 0, len(items))
	for _, item := range items {
		itemType := strings.TrimSpace(common.Interface2String(item["type"]))
		switch itemType {
		case geminiResponsesInputTypeCustomToolCall, geminiResponsesInputTypeCustomToolCallOutput:
			// 过滤 Responses custom/freeform 工具调用；它们不能映射为 Gemini function calling。
			continue
		case geminiResponsesInputTypeFunctionCallOutput:
			if _, ok := skippedCustomCallIDs[strings.TrimSpace(common.Interface2String(item["call_id"]))]; ok {
				continue
			}
		}
		filtered = append(filtered, item)
	}

	return common.Marshal(filtered)
}

func geminiRawJSONPresent(raw []byte) bool {
	if len(raw) == 0 {
		return false
	}
	return common.GetJsonType(raw) != "null"
}
