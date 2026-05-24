package openai

import (
	"encoding/json"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/logger"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	relayconstant "github.com/c1cada/NexusTok/relay/constant"
	"github.com/c1cada/NexusTok/relay/helper"
	"github.com/c1cada/NexusTok/service"
	"github.com/c1cada/NexusTok/types"

	"github.com/samber/lo"

	"github.com/gin-gonic/gin"
)

// openai - helper.go
// OpenAI 渠道的辅助函数文件。
// 本文件提供了流式响应处理的核心辅助功能，包括：
// - 多格式流式数据分发（OpenAI、Claude、Gemini 三种格式）
// - 流式响应的 token 统计和文本累积
// - 最终响应的处理和发送（包含 usage 统计）
// - Responses API 的流式数据转发
package openai

import (
	"encoding/json"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/logger"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	relayconstant "github.com/c1cada/NexusTok/relay/constant"
	"github.com/c1cada/NexusTok/relay/helper"
	"github.com/c1cada/NexusTok/service"
	"github.com/c1cada/NexusTok/types"

	"github.com/samber/lo"

	"github.com/gin-gonic/gin"
)

// HandleStreamFormat 根据中继格式将流式数据分发到对应的格式处理器。
// 支持三种输出格式：
//   - OpenAI 格式: 通过 sendStreamData 处理，支持强制格式化和思考内容转换
//   - Claude 格式: 通过 handleClaudeFormat 处理，将 OpenAI 流式响应转换为 Claude 格式
//   - Gemini 格式: 通过 handleGeminiFormat 处理，将 OpenAI 流式响应转换为 Gemini 格式
//
// 每次调用会递增 SendResponseCount 计数器，用于追踪已发送的响应数量。
//
// 参数:
//   - c: Gin 上下文
//   - info: 中继信息，包含输出格式和转换状态
//   - data: 原始流式数据字符串（JSON 格式）
//   - forceFormat: 是否强制重新序列化响应（即使格式未变）
//   - thinkToContent: 是否将思考内容（reasoning）转换为普通内容输出
//
// 返回:
//   - error: 处理失败时的错误
func HandleStreamFormat(c *gin.Context, info *relaycommon.RelayInfo, data string, forceFormat bool, thinkToContent bool) error {
	info.SendResponseCount++

	switch info.RelayFormat {
	case types.RelayFormatOpenAI:
		return sendStreamData(c, info, data, forceFormat, thinkToContent)
	case types.RelayFormatClaude:
		return handleClaudeFormat(c, data, info)
	case types.RelayFormatGemini:
		return handleGeminiFormat(c, data, info)
	}
	return nil
}

// handleClaudeFormat 将 OpenAI 格式的流式响应转换为 Claude 格式并发送。
// 解析 OpenAI 流式响应，提取 usage 信息后，
// 通过 service.StreamResponseOpenAI2Claude 转换为 Claude 格式的响应数组，
// 然后逐个发送给客户端。
//
// 参数:
//   - c: Gin 上下文
//   - data: OpenAI 格式的流式数据字符串
//   - info: 中继信息，包含 Claude 转换状态
//
// 返回:
//   - error: 解析或转换失败时的错误
func handleClaudeFormat(c *gin.Context, data string, info *relaycommon.RelayInfo) error {
	var streamResponse dto.ChatCompletionsStreamResponse
	if err := common.Unmarshal(common.StringToByteSlice(data), &streamResponse); err != nil {
		return err
	}

	if streamResponse.Usage != nil {
		info.ClaudeConvertInfo.Usage = streamResponse.Usage
	}
	claudeResponses := service.StreamResponseOpenAI2Claude(&streamResponse, info)
	for _, resp := range claudeResponses {
		helper.ClaudeData(c, *resp)
	}
	return nil
}

// handleGeminiFormat 将 OpenAI 格式的流式响应转换为 Gemini 格式并发送。
// 解析 OpenAI 流式响应后，通过 service.StreamResponseOpenAI2Gemini 转换为 Gemini 格式。
// 如果转换结果为 nil（表示没有实际内容），则跳过发送。
// 最终以 SSE 格式（data: {...}）发送给客户端。
//
// 参数:
//   - c: Gin 上下文
//   - data: OpenAI 格式的流式数据字符串
//   - info: 中继信息
//
// 返回:
//   - error: 解析、转换或发送失败时的错误
func handleGeminiFormat(c *gin.Context, data string, info *relaycommon.RelayInfo) error {
	var streamResponse dto.ChatCompletionsStreamResponse
	if err := common.Unmarshal(common.StringToByteSlice(data), &streamResponse); err != nil {
		logger.LogError(c, "failed to unmarshal stream response: "+err.Error())
		return err
	}

	geminiResponse := service.StreamResponseOpenAI2Gemini(&streamResponse, info)

	// 如果返回 nil，表示没有实际内容，跳过发送
	if geminiResponse == nil {
		return nil
	}

	geminiResponseStr, err := common.Marshal(geminiResponse)
	if err != nil {
		logger.LogError(c, "failed to marshal gemini response: "+err.Error())
		return err
	}

	// send gemini format response
	c.Render(-1, common.CustomEvent{Data: "data: " + string(geminiResponseStr)})
	_ = helper.FlushWriter(c)
	return nil
}

// ProcessStreamResponse 处理单个 OpenAI 流式响应，累积文本内容和工具调用信息。
// 遍历响应中的所有 choice，将以下内容追加到 responseTextBuilder 中：
//   - 普通文本内容（delta.content）
//   - 推理/思考内容（delta.reasoning_content）
//   - 工具调用的函数名和参数
//
// 同时追踪工具调用的最大数量（用于后续 token 估算，每个工具调用约 7 个 token）。
//
// 参数:
//   - streamResponse: OpenAI 流式响应对象
//   - responseTextBuilder: 用于累积响应文本的字符串构建器
//   - toolCount: 工具调用最大数量的指针（会被更新）
//
// 返回:
//   - error: 始终返回 nil（当前实现无错误路径）
func ProcessStreamResponse(streamResponse dto.ChatCompletionsStreamResponse, responseTextBuilder *strings.Builder, toolCount *int) error {
	for _, choice := range streamResponse.Choices {
		responseTextBuilder.WriteString(choice.Delta.GetContentString())
		responseTextBuilder.WriteString(choice.Delta.GetReasoningContent())
		if choice.Delta.ToolCalls != nil {
			if len(choice.Delta.ToolCalls) > *toolCount {
				*toolCount = len(choice.Delta.ToolCalls)
			}
			for _, tool := range choice.Delta.ToolCalls {
				responseTextBuilder.WriteString(tool.Function.Name)
				responseTextBuilder.WriteString(tool.Function.Arguments)
			}
		}
	}
	return nil
}

// processTokens 根据中继模式处理流式响应中的 token 信息。
// 将所有流式数据项合并为 JSON 数组格式，然后根据中继模式分发到对应的处理器：
//   - RelayModeChatCompletions: 聊天补全模式，提取文本、推理内容和工具调用
//   - RelayModeCompletions: 文本补全模式，提取补全文本
//
// 参数:
//   - relayMode: 中继模式常量
//   - streamItems: 流式数据项数组
//   - responseTextBuilder: 用于累积响应文本的字符串构建器
//   - toolCount: 工具调用最大数量的指针
//
// 返回:
//   - error: 处理失败时的错误
func processTokens(relayMode int, streamItems []string, responseTextBuilder *strings.Builder, toolCount *int) error {
	streamResp := "[" + strings.Join(streamItems, ",") + "]"

	switch relayMode {
	case relayconstant.RelayModeChatCompletions:
		return processChatCompletions(streamResp, streamItems, responseTextBuilder, toolCount)
	case relayconstant.RelayModeCompletions:
		return processCompletions(streamResp, streamItems, responseTextBuilder)
	}
	return nil
}

// processChatCompletions 处理聊天补全模式的流式响应 token 统计。
// 首先尝试将所有流式数据项批量解析为 ChatCompletionsStreamResponse 数组。
// 如果批量解析失败（例如数据格式不一致），则逐个解析每个数据项。
// 从每个响应的 choice 中提取文本内容、推理内容和工具调用信息。
//
// 参数:
//   - streamResp: 合并后的 JSON 数组字符串
//   - streamItems: 原始流式数据项数组（用于逐个解析的回退方案）
//   - responseTextBuilder: 用于累积响应文本的字符串构建器
//   - toolCount: 工具调用最大数量的指针
//
// 返回:
//   - error: 解析失败时的错误
func processChatCompletions(streamResp string, streamItems []string, responseTextBuilder *strings.Builder, toolCount *int) error {
	var streamResponses []dto.ChatCompletionsStreamResponse
	if err := json.Unmarshal(common.StringToByteSlice(streamResp), &streamResponses); err != nil {
		// 一次性解析失败，逐个解析
		common.SysLog("error unmarshalling stream response: " + err.Error())
		for _, item := range streamItems {
			var streamResponse dto.ChatCompletionsStreamResponse
			if err := json.Unmarshal(common.StringToByteSlice(item), &streamResponse); err != nil {
				return err
			}
			if err := ProcessStreamResponse(streamResponse, responseTextBuilder, toolCount); err != nil {
				common.SysLog("error processing stream response: " + err.Error())
			}
		}
		return nil
	}

	// 批量处理所有响应
	for _, streamResponse := range streamResponses {
		for _, choice := range streamResponse.Choices {
			responseTextBuilder.WriteString(choice.Delta.GetContentString())
			responseTextBuilder.WriteString(choice.Delta.GetReasoningContent())
			if choice.Delta.ToolCalls != nil {
				if len(choice.Delta.ToolCalls) > *toolCount {
					*toolCount = len(choice.Delta.ToolCalls)
				}
				for _, tool := range choice.Delta.ToolCalls {
					responseTextBuilder.WriteString(tool.Function.Name)
					responseTextBuilder.WriteString(tool.Function.Arguments)
				}
			}
		}
	}
	return nil
}

// processCompletions 处理文本补全模式的流式响应 token 统计。
// 首先尝试将所有流式数据项批量解析为 CompletionsStreamResponse 数组。
// 如果批量解析失败，则逐个解析每个数据项。
// 从每个响应的 choice 中提取补全文本（choice.Text）。
//
// 参数:
//   - streamResp: 合并后的 JSON 数组字符串
//   - streamItems: 原始流式数据项数组
//   - responseTextBuilder: 用于累积响应文本的字符串构建器
//
// 返回:
//   - error: 解析失败时的错误（逐个解析时跳过失败项）
func processCompletions(streamResp string, streamItems []string, responseTextBuilder *strings.Builder) error {
	var streamResponses []dto.CompletionsStreamResponse
	if err := json.Unmarshal(common.StringToByteSlice(streamResp), &streamResponses); err != nil {
		// 一次性解析失败，逐个解析
		common.SysLog("error unmarshalling stream response: " + err.Error())
		for _, item := range streamItems {
			var streamResponse dto.CompletionsStreamResponse
			if err := json.Unmarshal(common.StringToByteSlice(item), &streamResponse); err != nil {
				continue
			}
			for _, choice := range streamResponse.Choices {
				responseTextBuilder.WriteString(choice.Text)
			}
		}
		return nil
	}

	// 批量处理所有响应
	for _, streamResponse := range streamResponses {
		for _, choice := range streamResponse.Choices {
			responseTextBuilder.WriteString(choice.Text)
		}
	}
	return nil
}

// handleLastResponse 处理流式响应中的最后一个数据项，提取元数据和 usage 信息。
// 从最后一个流式数据项中解析以下信息：
//   - responseId: 响应 ID
//   - createAt: 响应创建时间戳
//   - systemFingerprint: 系统指纹（用于缓存一致性）
//   - model: 模型名称
//   - usage: token 使用量信息（如果上游返回了有效的 usage）
//
// 当最后一个数据项包含有效的 usage 信息时：
//   - 标记 containStreamUsage 为 true
//   - 判断是否需要发送最后一个响应（当 choice 中包含实际内容时才发送）
//
// 参数:
//   - lastStreamData: 最后一个流式数据项的 JSON 字符串
//   - responseId: 响应 ID 的指针（会被更新）
//   - createAt: 创建时间的指针（会被更新）
//   - systemFingerprint: 系统指纹的指针（会被更新）
//   - model: 模型名称的指针（会被更新）
//   - usage: usage 对象的指针（会被更新）
//   - containStreamUsage: 是否包含流式 usage 的指针（会被更新）
//   - info: 中继信息
//   - shouldSendLastResp: 是否应发送最后一个响应的指针（会被更新）
//
// 返回:
//   - error: 解析失败时的错误
func handleLastResponse(lastStreamData string, responseId *string, createAt *int64,
	systemFingerprint *string, model *string, usage **dto.Usage,
	containStreamUsage *bool, info *relaycommon.RelayInfo,
	shouldSendLastResp *bool) error {

	var lastStreamResponse dto.ChatCompletionsStreamResponse
	if err := common.Unmarshal(common.StringToByteSlice(lastStreamData), &lastStreamResponse); err != nil {
		return err
	}

	*responseId = lastStreamResponse.Id
	*createAt = lastStreamResponse.Created
	*systemFingerprint = lastStreamResponse.GetSystemFingerprint()
	*model = lastStreamResponse.Model

	if service.ValidUsage(lastStreamResponse.Usage) {
		*containStreamUsage = true
		*usage = lastStreamResponse.Usage
		if !info.ShouldIncludeUsage {
			*shouldSendLastResp = lo.SomeBy(lastStreamResponse.Choices, func(choice dto.ChatCompletionsStreamResponseChoice) bool {
				return choice.Delta.GetContentString() != "" || choice.Delta.GetReasoningContent() != ""
			})
		}
	}

	return nil
}

// HandleFinalResponse 根据输出格式处理并发送最终的流式响应。
// 不同格式的处理逻辑：
//
// OpenAI 格式：
//   - 如果需要包含 usage 但上游未返回，则生成一个包含 usage 的最终响应
//   - 发送 [DONE] 标记结束流
//
// Claude 格式：
//   - 将最后一个 OpenAI 流式响应转换为 Claude 格式
//   - 设置 Claude 转换完成标记（Done = true）
//
// Gemini 格式：
//   - 将最后一个 OpenAI 流式响应转换为 Gemini 格式
//   - 以 SSE 格式发送最终响应
//   - 注意：OpenAI 转换的最后一个响应可能比 Google 官方多一个空 parts 响应
//
// 参数:
//   - c: Gin 上下文
//   - info: 中继信息，包含输出格式和转换状态
//   - lastStreamData: 最后一个流式数据项的 JSON 字符串
//   - responseId: 响应 ID
//   - createAt: 创建时间戳
//   - model: 模型名称
//   - systemFingerprint: 系统指纹
//   - usage: token 使用量信息
//   - containStreamUsage: 上游是否已返回 usage 信息
func HandleFinalResponse(c *gin.Context, info *relaycommon.RelayInfo, lastStreamData string,
	responseId string, createAt int64, model string, systemFingerprint string,
	usage *dto.Usage, containStreamUsage bool) {

	switch info.RelayFormat {
	case types.RelayFormatOpenAI:
		if info.ShouldIncludeUsage && !containStreamUsage {
			response := helper.GenerateFinalUsageResponse(responseId, createAt, model, *usage)
			response.SetSystemFingerprint(systemFingerprint)
			helper.ObjectData(c, response)
		}
		helper.Done(c)

	case types.RelayFormatClaude:
		var streamResponse dto.ChatCompletionsStreamResponse
		if err := common.Unmarshal(common.StringToByteSlice(lastStreamData), &streamResponse); err != nil {
			common.SysLog("error unmarshalling stream response: " + err.Error())
			return
		}

		info.ClaudeConvertInfo.Usage = usage

		claudeResponses := service.StreamResponseOpenAI2Claude(&streamResponse, info)
		for _, resp := range claudeResponses {
			_ = helper.ClaudeData(c, *resp)
		}
		info.ClaudeConvertInfo.Done = true

	case types.RelayFormatGemini:
		var streamResponse dto.ChatCompletionsStreamResponse
		if err := common.Unmarshal(common.StringToByteSlice(lastStreamData), &streamResponse); err != nil {
			common.SysLog("error unmarshalling stream response: " + err.Error())
			return
		}

		// 这里处理的是 openai 最后一个流响应，其 delta 为空，有 finish_reason 字段
		// 因此相比较于 google 官方的流响应，由 openai 转换而来会多一个 parts 为空，finishReason 为 STOP 的响应
		// 而包含最后一段文本输出的响应（倒数第二个）的 finishReason 为 null
		// 暂不知是否有程序会不兼容。

		geminiResponse := service.StreamResponseOpenAI2Gemini(&streamResponse, info)

		// openai 流响应开头的空数据
		if geminiResponse == nil {
			return
		}

		geminiResponseStr, err := common.Marshal(geminiResponse)
		if err != nil {
			common.SysLog("error marshalling gemini response: " + err.Error())
			return
		}

		// 发送最终的 Gemini 响应
		c.Render(-1, common.CustomEvent{Data: "data: " + string(geminiResponseStr)})
		_ = helper.FlushWriter(c)
	}
}

// sendResponsesStreamData 转发 Responses API 的流式数据到客户端。
// 如果数据为空则跳过发送。通过 helper.ResponseChunkData 将数据
// 以 Responses API 的流式格式发送给客户端。
//
// 参数:
//   - c: Gin 上下文
//   - streamResponse: Responses API 的流式响应对象（包含类型和事件元数据）
//   - data: 要发送的原始数据字符串
func sendResponsesStreamData(c *gin.Context, streamResponse dto.ResponsesStreamResponse, data string) {
	if data == "" {
		return
	}
	helper.ResponseChunkData(c, streamResponse, data)
}
