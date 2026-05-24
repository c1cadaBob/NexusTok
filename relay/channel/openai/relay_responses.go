// openai - relay_responses.go
// OpenAI Responses API 的中继处理文件。
// 本文件负责处理 OpenAI Responses API（/v1/responses 端点）的请求和响应，包括：
// - 非流式 Responses API 响应处理：解析 usage、内置工具计数、图片生成检测
// - 流式 Responses API 响应处理：逐事件处理 SSE 数据、统计 token、追踪工具调用
package openai

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/logger"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/relay/helper"
	"github.com/c1cada/NexusTok/service"
	"github.com/c1cada/NexusTok/types"

	"github.com/gin-gonic/gin"
)

// OaiResponsesHandler 处理 OpenAI Responses API 的非流式响应。
// 该函数执行以下处理流程：
//
//  1. 读取并解析响应体为 OpenAIResponsesResponse 格式
//  2. 检查上游返回的错误信息
//  3. 图片生成检测：如果响应包含 image_generation_call，
//     将图片质量（quality）和尺寸（size）信息存储到 Gin 上下文中
//  4. 将响应体写回客户端
//  5. 提取 token 使用量：
//     - InputTokens -> PromptTokens
//     - OutputTokens -> CompletionTokens
//     - TotalTokens -> TotalTokens
//     - InputTokensDetails.CachedTokens -> PromptTokensDetails.CachedTokens
//  6. 解析内置工具（Built-in Tools）的调用次数：
//     - 遍历响应中的 tools 数组
//     - 根据 tool type 匹配到对应的内置工具信息
//     - 递增 CallCount 计数器
//
// 参数:
//   - c: Gin 上下文
//   - info: 中继信息，包含内置工具配置
//   - resp: 上游 API 返回的 HTTP 响应
//
// 返回:
//   - *dto.Usage: token 使用量信息
//   - *types.NexusTokError: 处理过程中的错误
func OaiResponsesHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NexusTokError) {
	defer service.CloseResponseBodyGracefully(resp)

	// read response body
	var responsesResponse dto.OpenAIResponsesResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	err = common.Unmarshal(responseBody, &responsesResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if oaiError := responsesResponse.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}

	if responsesResponse.HasImageGenerationCall() {
		c.Set("image_generation_call", true)
		c.Set("image_generation_call_quality", responsesResponse.GetQuality())
		c.Set("image_generation_call_size", responsesResponse.GetSize())
	}

	// 写入新的 response body
	service.IOCopyBytesGracefully(c, resp, responseBody)

	// compute usage
	usage := dto.Usage{}
	if responsesResponse.Usage != nil {
		usage.PromptTokens = responsesResponse.Usage.InputTokens
		usage.CompletionTokens = responsesResponse.Usage.OutputTokens
		usage.TotalTokens = responsesResponse.Usage.TotalTokens
		if responsesResponse.Usage.InputTokensDetails != nil {
			usage.PromptTokensDetails.CachedTokens = responsesResponse.Usage.InputTokensDetails.CachedTokens
		}
	}
	if info == nil || info.ResponsesUsageInfo == nil || info.ResponsesUsageInfo.BuiltInTools == nil {
		return &usage, nil
	}
	// 解析 Tools 用量
	for _, tool := range responsesResponse.Tools {
		buildToolinfo, ok := info.ResponsesUsageInfo.BuiltInTools[common.Interface2String(tool["type"])]
		if !ok || buildToolinfo == nil {
			logger.LogError(c, fmt.Sprintf("BuiltInTools not found for tool type: %v", tool["type"]))
			continue
		}
		buildToolinfo.CallCount++
	}
	return &usage, nil
}

// OaiResponsesStreamHandler 处理 OpenAI Responses API 的流式响应。
// 该函数使用 StreamScannerHandler 逐行扫描 SSE 数据，处理以下类型的事件：
//
//   - response.completed: 响应完成事件
//     - 提取 token 使用量（InputTokens、OutputTokens、TotalTokens）
//     - 提取缓存 token 信息（CachedTokens）
//     - 检测图片生成调用并存储到上下文
//
//   - response.output_text.delta: 输出文本增量事件
//     - 将文本片段追加到 responseTextBuilder 用于后续 token 估算
//
//   - response.output_item.done: 输出项完成事件
//     - 处理函数调用（如 web_search_call 类型的内置工具调用）
//     - 递增对应内置工具的 CallCount 计数器
//
// 流式处理完成后：
//   - 如果上游未返回 CompletionTokens，使用累积的文本内容进行 token 估算
//   - 如果未返回 PromptTokens 但有 CompletionTokens，使用估算的 prompt token 数
//   - 计算 TotalTokens = PromptTokens + CompletionTokens
//
// 参数:
//   - c: Gin 上下文
//   - info: 中继信息，包含模型名称和内置工具配置
//   - resp: 上游 API 返回的 HTTP 响应
//
// 返回:
//   - *dto.Usage: token 使用量信息
//   - *types.NexusTokError: 处理过程中的错误
func OaiResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NexusTokError) {
	if resp == nil || resp.Body == nil {
		logger.LogError(c, "invalid response or response body")
		return nil, types.NewError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse)
	}

	defer service.CloseResponseBodyGracefully(resp)

	var usage = &dto.Usage{}
	var responseTextBuilder strings.Builder

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {

		// 检查当前数据是否包含 completed 状态和 usage 信息
		var streamResponse dto.ResponsesStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResponse); err != nil {
			logger.LogError(c, "failed to unmarshal stream response: "+err.Error())
			sr.Error(err)
			return
		}
		sendResponsesStreamData(c, streamResponse, data)
		switch streamResponse.Type {
		case "response.completed":
			if streamResponse.Response != nil {
				if streamResponse.Response.Usage != nil {
					if streamResponse.Response.Usage.InputTokens != 0 {
						usage.PromptTokens = streamResponse.Response.Usage.InputTokens
					}
					if streamResponse.Response.Usage.OutputTokens != 0 {
						usage.CompletionTokens = streamResponse.Response.Usage.OutputTokens
					}
					if streamResponse.Response.Usage.TotalTokens != 0 {
						usage.TotalTokens = streamResponse.Response.Usage.TotalTokens
					}
					if streamResponse.Response.Usage.InputTokensDetails != nil {
						usage.PromptTokensDetails.CachedTokens = streamResponse.Response.Usage.InputTokensDetails.CachedTokens
					}
				}
				if streamResponse.Response.HasImageGenerationCall() {
					c.Set("image_generation_call", true)
					c.Set("image_generation_call_quality", streamResponse.Response.GetQuality())
					c.Set("image_generation_call_size", streamResponse.Response.GetSize())
				}
			}
		case "response.output_text.delta":
			// 处理输出文本
			responseTextBuilder.WriteString(streamResponse.Delta)
		case dto.ResponsesOutputTypeItemDone:
			// 函数调用处理
			if streamResponse.Item != nil {
				switch streamResponse.Item.Type {
				case dto.BuildInCallWebSearchCall:
					if info != nil && info.ResponsesUsageInfo != nil && info.ResponsesUsageInfo.BuiltInTools != nil {
						if webSearchTool, exists := info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearchPreview]; exists && webSearchTool != nil {
							webSearchTool.CallCount++
						}
					}
				}
			}
		}
	})

	if usage.CompletionTokens == 0 {
		// 计算输出文本的 token 数量
		tempStr := responseTextBuilder.String()
		if len(tempStr) > 0 {
			// 非正常结束，使用输出文本的 token 数量
			completionTokens := service.CountTextToken(tempStr, info.UpstreamModelName)
			usage.CompletionTokens = completionTokens
		}
	}

	if usage.PromptTokens == 0 && usage.CompletionTokens != 0 {
		usage.PromptTokens = info.GetEstimatePromptTokens()
	}

	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens

	return usage, nil
}
