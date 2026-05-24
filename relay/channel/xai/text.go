// Package xai 的文本补全响应处理文件。
// 负责将 xAI（Grok）API 的流式和非流式响应转换为 OpenAI 兼容格式。
// xAI 的响应格式基本兼容 OpenAI，但在 usage 计算上有特殊处理：
// 需要手动计算 CompletionTokens（TotalTokens - PromptTokens）。
package xai

import (
	"io"
	"net/http"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/relay/channel/openai"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/relay/helper"
	"github.com/c1cada/NexusTok/service"
	"github.com/c1cada/NexusTok/types"

	"github.com/gin-gonic/gin"
)

// streamResponseXAI2OpenAI 将 xAI 的流式响应转换为 OpenAI 格式。
// 将 xAI 响应中的 Usage.CompletionTokens 替换为从上下文累积的值，
// 以确保跨多个 chunk 的 token 计数准确。
// 参数:
//   - xAIResp: xAI 格式的流式响应
//   - usage: 当前累积的 token 使用量
// 返回:
//   - *dto.ChatCompletionsStreamResponse: OpenAI 格式的流式响应
func streamResponseXAI2OpenAI(xAIResp *dto.ChatCompletionsStreamResponse, usage *dto.Usage) *dto.ChatCompletionsStreamResponse {
	if xAIResp == nil {
		return nil
	}
	if xAIResp.Usage != nil {
		xAIResp.Usage.CompletionTokens = usage.CompletionTokens
	}
	openAIResp := &dto.ChatCompletionsStreamResponse{
		Id:      xAIResp.Id,
		Object:  xAIResp.Object,
		Created: xAIResp.Created,
		Model:   xAIResp.Model,
		Choices: xAIResp.Choices,
		Usage:   xAIResp.Usage,
	}

	return openAIResp
}

// xAIStreamHandler 处理 xAI API 的流式聊天响应。
// 逐块解析 SSE 数据，将 xAI 格式的 usage 转换为 OpenAI 格式，
// 并通过 EventSource 流式推送给客户端。
// 特殊处理：
//   - 如果上游返回了流式 usage，直接使用其值计算 CompletionTokens
//   - 如果上游未返回 usage，使用 ResponseText2Usage 函数估算
//   - 工具调用数量额外加 7 个 token/次
// 参数:
//   - c: Gin 上下文
//   - info: 中继信息
//   - resp: 上游 HTTP 响应
// 返回:
//   - *dto.Usage: token 使用量统计
//   - *types.NexusTokError: 处理过程中的错误信息
func xAIStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NexusTokError) {
	usage := &dto.Usage{}
	var responseTextBuilder strings.Builder
	var toolCount int
	var containStreamUsage bool

	helper.SetEventStreamHeaders(c)

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		var xAIResp *dto.ChatCompletionsStreamResponse
		if err := common.UnmarshalJsonStr(data, &xAIResp); err != nil {
			common.SysLog("error unmarshalling stream response: " + err.Error())
			sr.Error(err)
			return
		}

		// 把 xAI 的usage转换为 OpenAI 的usage
		if xAIResp.Usage != nil {
			containStreamUsage = true
			usage.PromptTokens = xAIResp.Usage.PromptTokens
			usage.TotalTokens = xAIResp.Usage.TotalTokens
			usage.CompletionTokens = usage.TotalTokens - usage.PromptTokens
		}

		openaiResponse := streamResponseXAI2OpenAI(xAIResp, usage)
		_ = openai.ProcessStreamResponse(*openaiResponse, &responseTextBuilder, &toolCount)
		if err := helper.ObjectData(c, openaiResponse); err != nil {
			common.SysLog(err.Error())
			sr.Error(err)
		}
	})

	if !containStreamUsage {
		usage = service.ResponseText2Usage(c, responseTextBuilder.String(), info.UpstreamModelName, info.GetEstimatePromptTokens())
		usage.CompletionTokens += toolCount * 7
	}

	helper.Done(c)
	service.CloseResponseBodyGracefully(resp)
	return usage, nil
}

// xAIHandler 处理 xAI API 的非流式聊天响应。
// 读取完整的响应体，修正 usage 中的 CompletionTokens 和 TextTokens 字段
// （xAI 返回 TotalTokens 但不单独返回 CompletionTokens），
// 然后将修正后的 JSON 重新写入客户端。
// 参数:
//   - c: Gin 上下文
//   - info: 中继信息
//   - resp: 上游 HTTP 响应
// 返回:
//   - *dto.Usage: token 使用量统计
//   - *types.NexusTokError: 处理过程中的错误信息
func xAIHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NexusTokError) {
	defer service.CloseResponseBodyGracefully(resp)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	var xaiResponse ChatCompletionResponse
	err = common.Unmarshal(responseBody, &xaiResponse)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	if xaiResponse.Usage != nil {
		xaiResponse.Usage.CompletionTokens = xaiResponse.Usage.TotalTokens - xaiResponse.Usage.PromptTokens
		xaiResponse.Usage.CompletionTokenDetails.TextTokens = xaiResponse.Usage.CompletionTokens - xaiResponse.Usage.CompletionTokenDetails.ReasoningTokens
	}

	// new body
	encodeJson, err := common.Marshal(xaiResponse)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}

	service.IOCopyBytesGracefully(c, resp, encodeJson)

	return xaiResponse.Usage, nil
}
