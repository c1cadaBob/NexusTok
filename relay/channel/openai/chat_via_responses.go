// Package openai 的 Responses API 转 Chat Completions 格式处理文件。
// 负责将 OpenAI 的 Responses API 格式转换为 Chat Completions 格式。
// 支持：
// - 非流式 Responses 响应转换为 Chat Completions 响应
// - 流式 Responses 响应转换为 Chat Completions 流式响应
// - 多格式输出（OpenAI、Claude、Gemini）
// - 工具调用（function call）的流式处理
// - 思考内容（reasoning summary）的流式处理
package openai

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	// 项目内部包
	"github.com/c1cada/NexusTok/common"                              // 通用工具（JSON、UUID 等）
	"github.com/c1cada/NexusTok/dto"                                 // 数据传输对象
	"github.com/c1cada/NexusTok/logger"                              // 日志工具
	relaycommon "github.com/c1cada/NexusTok/relay/common"            // Relay 通用信息
	"github.com/c1cada/NexusTok/relay/helper"                        // Relay 辅助工具
	"github.com/c1cada/NexusTok/service"                             // 服务层（响应转换等）
	"github.com/c1cada/NexusTok/types"                               // 类型定义

	// 第三方依赖
	"github.com/gin-gonic/gin"                                       // Gin Web 框架
)

// responsesStreamIndexKey 生成流式响应中用于索引的唯一键。
// 用于跟踪工具调用的状态，格式为 "itemID:index"。
// 参数:
//   - itemID: 输出项 ID
//   - idx: 索引指针（可选）
// 返回:
//   - string: 唯一键
func responsesStreamIndexKey(itemID string, idx *int) string {
	if itemID == "" {
		return ""
	}
	if idx == nil {
		return itemID
	}
	return fmt.Sprintf("%s:%d", itemID, *idx)
}

// stringDeltaFromPrefix 计算字符串增量。
// 用于从累积文本中提取新增部分。
// 参数:
//   - prev: 之前的文本
//   - next: 新的完整文本
// 返回:
//   - string: 增量文本
func stringDeltaFromPrefix(prev string, next string) string {
	if next == "" {
		return ""
	}
	if prev != "" && strings.HasPrefix(next, prev) {
		return next[len(prev):]
	}
	return next
}

// OaiResponsesToChatHandler 处理非流式 Responses 响应并转换为 Chat Completions 格式。
// 处理流程：
// 1. 读取并解析 Responses API 响应
// 2. 检查错误响应
// 3. 转换为 Chat Completions 响应
// 4. 如果没有使用量统计，从文本估算
// 5. 根据目标格式（OpenAI/Claude/Gemini）序列化输出
// 参数:
//   - c: Gin 上下文
//   - info: Relay 信息（包含目标格式、模型信息等）
//   - resp: 上游 HTTP 响应
// 返回:
//   - *dto.Usage: 使用量统计
//   - *types.NexusTokError: 错误信息（成功时为 nil）
func OaiResponsesToChatHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NexusTokError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	defer service.CloseResponseBodyGracefully(resp)

	// 读取响应体
	var responsesResp dto.OpenAIResponsesResponse
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}

	// 解析 JSON 响应
	if err := common.Unmarshal(body, &responsesResp); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	// 检查上游错误
	if oaiError := responsesResp.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}

	// 转换为 Chat Completions 响应
	chatId := helper.GetResponseID(c)
	chatResp, usage, err := service.ResponsesResponseToChatCompletionsResponse(&responsesResp, chatId)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	// 如果没有使用量统计，从文本估算
	if usage == nil || usage.TotalTokens == 0 {
		text := service.ExtractOutputTextFromResponses(&responsesResp)
		usage = service.ResponseText2Usage(c, text, info.UpstreamModelName, info.GetEstimatePromptTokens())
		chatResp.Usage = *usage
	}

	// 根据目标格式序列化响应
	var responseBody []byte
	switch info.RelayFormat {
	case types.RelayFormatClaude:
		// 转换为 Claude 格式
		claudeResp := service.ResponseOpenAI2Claude(chatResp, info)
		responseBody, err = common.Marshal(claudeResp)
	case types.RelayFormatGemini:
		// 转换为 Gemini 格式
		geminiResp := service.ResponseOpenAI2Gemini(chatResp, info)
		responseBody, err = common.Marshal(geminiResp)
	default:
		// 保持 OpenAI 格式
		responseBody, err = common.Marshal(chatResp)
	}
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
	}

	// 写入响应
	service.IOCopyBytesGracefully(c, resp, responseBody)
	return usage, nil
}

// OaiResponsesToChatStreamHandler 处理流式 Responses 响应并转换为 Chat Completions 流式格式。
// 这是一个复杂的函数，处理 OpenAI Responses API 的流式事件并转换为 Chat Completions 流式格式。
// 支持的事件类型：
//   - response.created: 响应创建
//   - response.reasoning_summary_text.delta: 思考摘要增量
//   - response.output_text.delta: 输出文本增量
//   - response.output_item.added/done: 输出项添加/完成（用于工具调用）
//   - response.function_call_arguments.delta: 函数调用参数增量
//   - response.completed: 响应完成
//   - response.error/failed: 错误处理
//
// 参数:
//   - c: Gin 上下文
//   - info: Relay 信息（包含目标格式、模型信息等）
//   - resp: 上游 HTTP 响应
// 返回:
//   - *dto.Usage: 使用量统计
//   - *types.NexusTokError: 错误信息（成功时为 nil）
func OaiResponsesToChatStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NexusTokError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	defer service.CloseResponseBodyGracefully(resp)

	// 初始化流式处理状态
	responseId := helper.GetResponseID(c)
	createAt := time.Now().Unix()
	model := info.UpstreamModelName

	// 状态变量
	var (
		usage       = &dto.Usage{}
		outputText  strings.Builder  // 输出文本累积器
		usageText   strings.Builder  // 使用量文本累积器（用于估算 token）
		sentStart   bool             // 是否已发送开始响应
		sentStop    bool             // 是否已发送停止响应
		sawToolCall bool             // 是否看到了工具调用
		streamErr   *types.NexusTokError // 流式处理错误
	)

	// 工具调用跟踪映射
	toolCallIndexByID := make(map[string]int)      // callID -> 索引
	toolCallNameByID := make(map[string]string)    // callID -> 函数名
	toolCallArgsByID := make(map[string]string)    // callID -> 累积参数
	toolCallNameSent := make(map[string]bool)      // callID -> 是否已发送函数名
	toolCallCanonicalIDByItemID := make(map[string]string) // itemID -> callID
	hasSentReasoningSummary := false               // 是否已发送思考摘要
	needsReasoningSummarySeparator := false        // 是否需要思考摘要分隔符

	// 初始化 Claude 转换信息（如果目标格式是 Claude）
	if info.RelayFormat == types.RelayFormatClaude && info.ClaudeConvertInfo == nil {
		info.ClaudeConvertInfo = &relaycommon.ClaudeConvertInfo{LastMessagesType: relaycommon.LastMessageTypeNone}
	}

	// 发送 Chat Completions 流式块的辅助函数
	sendChatChunk := func(chunk *dto.ChatCompletionsStreamResponse) bool {
		if chunk == nil {
			return true
		}
		// OpenAI 格式直接发送对象
		if info.RelayFormat == types.RelayFormatOpenAI {
			if err := helper.ObjectData(c, chunk); err != nil {
				streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
				return false
			}
			return true
		}

		// 其他格式需要序列化后通过 HandleStreamFormat 处理
		chunkData, err := common.Marshal(chunk)
		if err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
			return false
		}
		if err := HandleStreamFormat(c, info, string(chunkData), false, false); err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
			return false
		}
		return true
	}

	// 按需发送开始响应
	sendStartIfNeeded := func() bool {
		if sentStart {
			return true
		}
		if !sendChatChunk(helper.GenerateStartEmptyResponse(responseId, createAt, model, nil)) {
			return false
		}
		sentStart = true
		return true
	}

	// 发送思考摘要增量的辅助函数
	sendReasoningSummaryDelta := func(delta string) bool {
		if delta == "" {
			return true
		}
		// 处理分隔符逻辑
		if needsReasoningSummarySeparator {
			if strings.HasPrefix(delta, "\n\n") {
				needsReasoningSummarySeparator = false
			} else if strings.HasPrefix(delta, "\n") {
				delta = "\n" + delta
				needsReasoningSummarySeparator = false
			} else {
				delta = "\n\n" + delta
				needsReasoningSummarySeparator = false
			}
		}
		if !sendStartIfNeeded() {
			return false
		}

		usageText.WriteString(delta)
		chunk := &dto.ChatCompletionsStreamResponse{
			Id:      responseId,
			Object:  "chat.completion.chunk",
			Created: createAt,
			Model:   model,
			Choices: []dto.ChatCompletionsStreamResponseChoice{
				{
					Index: 0,
					Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
						ReasoningContent: &delta,
					},
				},
			},
		}
		if !sendChatChunk(chunk) {
			return false
		}
		hasSentReasoningSummary = true
		return true
	}

	// 发送工具调用增量的辅助函数
	sendToolCallDelta := func(callID string, name string, argsDelta string) bool {
		if callID == "" {
			return true
		}
		// 如果已有输出文本，优先流式传输文本而非工具调用
		if outputText.Len() > 0 {
			return true
		}
		if !sendStartIfNeeded() {
			return false
		}

		// 获取或分配工具调用索引
		idx, ok := toolCallIndexByID[callID]
		if !ok {
			idx = len(toolCallIndexByID)
			toolCallIndexByID[callID] = idx
		}
		if name != "" {
			toolCallNameByID[callID] = name
		}
		if toolCallNameByID[callID] != "" {
			name = toolCallNameByID[callID]
		}

		// 构建工具调用响应
		tool := dto.ToolCallResponse{
			ID:   callID,
			Type: "function",
			Function: dto.FunctionResponse{
				Arguments: argsDelta,
			},
		}
		tool.SetIndex(idx)
		// 只在第一次发送时包含函数名
		if name != "" && !toolCallNameSent[callID] {
			tool.Function.Name = name
			toolCallNameSent[callID] = true
		}

		chunk := &dto.ChatCompletionsStreamResponse{
			Id:      responseId,
			Object:  "chat.completion.chunk",
			Created: createAt,
			Model:   model,
			Choices: []dto.ChatCompletionsStreamResponseChoice{
				{
					Index: 0,
					Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
						ToolCalls: []dto.ToolCallResponse{tool},
					},
				},
			},
		}
		if !sendChatChunk(chunk) {
			return false
		}
		sawToolCall = true

		// 将工具调用数据添加到本地构建器用于 token 估算
		if tool.Function.Name != "" {
			usageText.WriteString(tool.Function.Name)
		}
		if argsDelta != "" {
			usageText.WriteString(argsDelta)
		}
		return true
	}

	// 处理流式事件
	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		if streamErr != nil {
			sr.Stop(streamErr)
			return
		}

		// 解析流式事件
		var streamResp dto.ResponsesStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResp); err != nil {
			logger.LogError(c, "failed to unmarshal responses stream event: "+err.Error())
			sr.Error(err)
			return
		}

		// 根据事件类型处理
		switch streamResp.Type {
		case "response.created":
			// 响应创建事件，提取模型和创建时间
			if streamResp.Response != nil {
				if streamResp.Response.Model != "" {
					model = streamResp.Response.Model
				}
				if streamResp.Response.CreatedAt != 0 {
					createAt = int64(streamResp.Response.CreatedAt)
				}
			}

		case "response.reasoning_summary_text.delta":
			// 思考摘要文本增量
			if !sendReasoningSummaryDelta(streamResp.Delta) {
				sr.Stop(streamErr)
				return
			}

		case "response.reasoning_summary_text.done":
			// 思考摘要完成，设置分隔符标志
			if hasSentReasoningSummary {
				needsReasoningSummarySeparator = true
			}

		case "response.output_text.delta":
			// 输出文本增量
			if !sendStartIfNeeded() {
				sr.Stop(streamErr)
				return
			}

			if streamResp.Delta != "" {
				outputText.WriteString(streamResp.Delta)
				usageText.WriteString(streamResp.Delta)
				delta := streamResp.Delta
				chunk := &dto.ChatCompletionsStreamResponse{
					Id:      responseId,
					Object:  "chat.completion.chunk",
					Created: createAt,
					Model:   model,
					Choices: []dto.ChatCompletionsStreamResponseChoice{
						{
							Index: 0,
							Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
								Content: &delta,
							},
						},
					},
				}
				if !sendChatChunk(chunk) {
					sr.Stop(streamErr)
					return
				}
			}

		case "response.output_item.added", "response.output_item.done":
			// 输出项添加/完成事件（用于工具调用跟踪）
			if streamResp.Item == nil {
				break
			}
			if streamResp.Item.Type != "function_call" {
				break
			}

			// 提取工具调用信息
			itemID := strings.TrimSpace(streamResp.Item.ID)
			callID := strings.TrimSpace(streamResp.Item.CallId)
			if callID == "" {
				callID = itemID
			}
			if itemID != "" && callID != "" {
				toolCallCanonicalIDByItemID[itemID] = callID
			}
			name := strings.TrimSpace(streamResp.Item.Name)
			if name != "" {
				toolCallNameByID[callID] = name
			}

			// 计算参数增量
			newArgs := streamResp.Item.ArgumentsString()
			prevArgs := toolCallArgsByID[callID]
			argsDelta := ""
			if newArgs != "" {
				if strings.HasPrefix(newArgs, prevArgs) {
					argsDelta = newArgs[len(prevArgs):]
				} else {
					argsDelta = newArgs
				}
				toolCallArgsByID[callID] = newArgs
			}

			if !sendToolCallDelta(callID, name, argsDelta) {
				sr.Stop(streamErr)
				return
			}

		case "response.function_call_arguments.delta":
			// 函数调用参数增量
			itemID := strings.TrimSpace(streamResp.ItemID)
			callID := toolCallCanonicalIDByItemID[itemID]
			if callID == "" {
				callID = itemID
			}
			if callID == "" {
				break
			}
			toolCallArgsByID[callID] += streamResp.Delta
			if !sendToolCallDelta(callID, "", streamResp.Delta) {
				sr.Stop(streamErr)
				return
			}

		case "response.function_call_arguments.done":
			// 函数调用参数完成（无需特殊处理）

		case "response.completed":
			// 响应完成事件，提取使用量统计
			if streamResp.Response != nil {
				if streamResp.Response.Model != "" {
					model = streamResp.Response.Model
				}
				if streamResp.Response.CreatedAt != 0 {
					createAt = int64(streamResp.Response.CreatedAt)
				}
				// 提取使用量统计
				if streamResp.Response.Usage != nil {
					if streamResp.Response.Usage.InputTokens != 0 {
						usage.PromptTokens = streamResp.Response.Usage.InputTokens
						usage.InputTokens = streamResp.Response.Usage.InputTokens
					}
					if streamResp.Response.Usage.OutputTokens != 0 {
						usage.CompletionTokens = streamResp.Response.Usage.OutputTokens
						usage.OutputTokens = streamResp.Response.Usage.OutputTokens
					}
					if streamResp.Response.Usage.TotalTokens != 0 {
						usage.TotalTokens = streamResp.Response.Usage.TotalTokens
					} else {
						usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
					}
					// 提取详细使用量信息
					if streamResp.Response.Usage.InputTokensDetails != nil {
						usage.PromptTokensDetails.CachedTokens = streamResp.Response.Usage.InputTokensDetails.CachedTokens
						usage.PromptTokensDetails.ImageTokens = streamResp.Response.Usage.InputTokensDetails.ImageTokens
						usage.PromptTokensDetails.AudioTokens = streamResp.Response.Usage.InputTokensDetails.AudioTokens
					}
					if streamResp.Response.Usage.CompletionTokenDetails.ReasoningTokens != 0 {
						usage.CompletionTokenDetails.ReasoningTokens = streamResp.Response.Usage.CompletionTokenDetails.ReasoningTokens
					}
				}
			}

			// 发送开始响应（如果尚未发送）
			if !sendStartIfNeeded() {
				sr.Stop(streamErr)
				return
			}
			// 发送停止响应（如果尚未发送）
			if !sentStop {
				if info.RelayFormat == types.RelayFormatClaude && info.ClaudeConvertInfo != nil {
					info.ClaudeConvertInfo.Usage = usage
				}
				finishReason := "stop"
				if sawToolCall && outputText.Len() == 0 {
					finishReason = "tool_calls"
				}
				stop := helper.GenerateStopResponse(responseId, createAt, model, finishReason)
				if !sendChatChunk(stop) {
					sr.Stop(streamErr)
					return
				}
				sentStop = true
			}

		case "response.error", "response.failed":
			// 错误处理
			if streamResp.Response != nil {
				if oaiErr := streamResp.Response.GetOpenAIError(); oaiErr != nil && oaiErr.Type != "" {
					streamErr = types.WithOpenAIError(*oaiErr, http.StatusInternalServerError)
					sr.Stop(streamErr)
					return
				}
			}
			streamErr = types.NewOpenAIError(fmt.Errorf("responses stream error: %s", streamResp.Type), types.ErrorCodeBadResponse, http.StatusInternalServerError)
			sr.Stop(streamErr)
			return

		default:
			// 忽略未知事件类型
		}
	})

	// 处理流式处理错误
	if streamErr != nil {
		return nil, streamErr
	}

	// 如果没有使用量统计，从文本估算
	if usage.TotalTokens == 0 {
		usage = service.ResponseText2Usage(c, usageText.String(), info.UpstreamModelName, info.GetEstimatePromptTokens())
	}

	// 确保发送了开始和停止响应
	if !sentStart {
		if !sendChatChunk(helper.GenerateStartEmptyResponse(responseId, createAt, model, nil)) {
			return nil, streamErr
		}
	}
	if !sentStop {
		if info.RelayFormat == types.RelayFormatClaude && info.ClaudeConvertInfo != nil {
			info.ClaudeConvertInfo.Usage = usage
		}
		finishReason := "stop"
		if sawToolCall && outputText.Len() == 0 {
			finishReason = "tool_calls"
		}
		stop := helper.GenerateStopResponse(responseId, createAt, model, finishReason)
		if !sendChatChunk(stop) {
			return nil, streamErr
		}
	}
	// OpenAI 格式需要发送使用量统计帧
	if info.RelayFormat == types.RelayFormatOpenAI && info.ShouldIncludeUsage && usage != nil {
		if err := helper.ObjectData(c, helper.GenerateFinalUsageResponse(responseId, createAt, model, *usage)); err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
		}
	}

	// OpenAI 格式需要发送 [DONE] 标记
	if info.RelayFormat == types.RelayFormatOpenAI {
		helper.Done(c)
	}
	return usage, nil
}
