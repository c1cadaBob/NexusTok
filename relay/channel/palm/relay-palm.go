// Package palm 的响应处理文件。
// 负责将 PaLM 的原生响应格式转换为 OpenAI 兼容的响应格式。
// 支持：
// - 非流式响应处理（palmHandler）
// - 流式响应处理（palmStreamHandler）
// - PaLM 响应到 OpenAI 格式的转换
package palm

import (
	"encoding/json"
	"io"
	"net/http"

	// 项目内部包
	"github.com/c1cada/NexusTok/common"                              // 通用工具（JSON、UUID、时间戳等）
	"github.com/c1cada/NexusTok/constant"                            // 常量定义（FinishReason 等）
	"github.com/c1cada/NexusTok/dto"                                 // 数据传输对象
	relaycommon "github.com/c1cada/NexusTok/relay/common"            // Relay 通用信息
	"github.com/c1cada/NexusTok/relay/helper"                        // Relay 辅助工具（流式响应生成）
	"github.com/c1cada/NexusTok/service"                             // 服务层（响应转换等）
	"github.com/c1cada/NexusTok/types"                               // 类型定义

	// 第三方依赖
	"github.com/gin-gonic/gin"                                       // Gin Web 框架
)

// responsePaLM2OpenAI 将 PaLM 响应转换为 OpenAI 格式。
// 将 PaLM 的候选响应列表转换为 OpenAI 的 choices 格式。
// 参数:
//   - response: PaLM 格式的聊天响应
// 返回:
//   - *dto.OpenAITextResponse: OpenAI 格式的文本响应
func responsePaLM2OpenAI(response *PaLMChatResponse) *dto.OpenAITextResponse {
	fullTextResponse := dto.OpenAITextResponse{
		Choices: make([]dto.OpenAITextResponseChoice, 0, len(response.Candidates)),
	}
	// 遍历候选响应，转换为 OpenAI 格式
	for i, candidate := range response.Candidates {
		choice := dto.OpenAITextResponseChoice{
			Index: i,
			Message: dto.Message{
				Role:    "assistant",
				Content: candidate.Content,
			},
			FinishReason: "stop",
		}
		fullTextResponse.Choices = append(fullTextResponse.Choices, choice)
	}
	return &fullTextResponse
}

// streamResponsePaLM2OpenAI 将 PaLM 流式响应转换为 OpenAI 流式格式。
// PaLM 不支持真正的流式响应，这里将完整响应包装为单个流式块。
// 参数:
//   - palmResponse: PaLM 格式的聊天响应
// 返回:
//   - *dto.ChatCompletionsStreamResponse: OpenAI 格式的流式响应
func streamResponsePaLM2OpenAI(palmResponse *PaLMChatResponse) *dto.ChatCompletionsStreamResponse {
	var choice dto.ChatCompletionsStreamResponseChoice
	if len(palmResponse.Candidates) > 0 {
		choice.Delta.SetContentString(palmResponse.Candidates[0].Content)
	}
	choice.FinishReason = &constant.FinishReasonStop
	var response dto.ChatCompletionsStreamResponse
	response.Object = "chat.completion.chunk"
	response.Model = "palm2"
	response.Choices = []dto.ChatCompletionsStreamResponseChoice{choice}
	return &response
}

// palmStreamHandler 处理 PaLM 的流式响应。
// PaLM 不支持真正的流式响应，这里读取完整响应后模拟流式输出。
// 使用 channel 和 goroutine 异步处理响应。
// 参数:
//   - c: Gin 上下文
//   - resp: PaLM 的 HTTP 响应
// 返回:
//   - *types.NexusTokError: 错误信息（成功时为 nil）
//   - string: 响应文本（用于使用量估算）
func palmStreamHandler(c *gin.Context, resp *http.Response) (*types.NexusTokError, string) {
	responseText := ""
	responseId := helper.GetResponseID(c)
	createdTime := common.GetTimestamp()
	dataChan := make(chan string)
	stopChan := make(chan bool)

	// 异步读取和处理响应
	go func() {
		responseBody, err := io.ReadAll(resp.Body)
		if err != nil {
			common.SysLog("error reading stream response: " + err.Error())
			stopChan <- true
			return
		}
		service.CloseResponseBodyGracefully(resp)

		// 解析 PaLM 响应
		var palmResponse PaLMChatResponse
		err = json.Unmarshal(responseBody, &palmResponse)
		if err != nil {
			common.SysLog("error unmarshalling stream response: " + err.Error())
			stopChan <- true
			return
		}

		// 转换为 OpenAI 格式
		fullTextResponse := streamResponsePaLM2OpenAI(&palmResponse)
		fullTextResponse.Id = responseId
		fullTextResponse.Created = createdTime
		if len(palmResponse.Candidates) > 0 {
			responseText = palmResponse.Candidates[0].Content
		}

		// 序列化响应
		jsonResponse, err := json.Marshal(fullTextResponse)
		if err != nil {
			common.SysLog("error marshalling stream response: " + err.Error())
			stopChan <- true
			return
		}
		dataChan <- string(jsonResponse)
		stopChan <- true
	}()

	// 设置 SSE 头部
	helper.SetEventStreamHeaders(c)
	// 使用 Gin 的 Stream 方法发送 SSE 事件
	c.Stream(func(w io.Writer) bool {
		select {
		case data := <-dataChan:
			c.Render(-1, common.CustomEvent{Data: "data: " + data})
			return true
		case <-stopChan:
			c.Render(-1, common.CustomEvent{Data: "data: [DONE]"})
			return false
		}
	})
	service.CloseResponseBodyGracefully(resp)
	return nil, responseText
}

// palmHandler 处理 PaLM 的非流式响应。
// 将 PaLM 的非流式响应转换为 OpenAI 兼容的 JSON 格式。
// 参数:
//   - c: Gin 上下文
//   - info: Relay 信息
//   - resp: PaLM 的 HTTP 响应
// 返回:
//   - *dto.Usage: 使用量统计
//   - *types.NexusTokError: 错误信息（成功时为 nil）
func palmHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NexusTokError) {
	// 读取响应体
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	service.CloseResponseBodyGracefully(resp)

	// 解析 PaLM 响应
	var palmResponse PaLMChatResponse
	err = json.Unmarshal(responseBody, &palmResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	// 检查错误响应
	if palmResponse.Error.Code != 0 || len(palmResponse.Candidates) == 0 {
		return nil, types.WithOpenAIError(types.OpenAIError{
			Message: palmResponse.Error.Message,
			Type:    palmResponse.Error.Status,
			Param:   "",
			Code:    palmResponse.Error.Code,
		}, resp.StatusCode)
	}

	// 转换为 OpenAI 格式
	fullTextResponse := responsePaLM2OpenAI(&palmResponse)
	// 估算使用量
	usage := service.ResponseText2Usage(c, palmResponse.Candidates[0].Content, info.UpstreamModelName, info.GetEstimatePromptTokens())
	fullTextResponse.Usage = *usage

	// 序列化响应
	jsonResponse, err := common.Marshal(fullTextResponse)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}

	// 写入响应
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	service.IOCopyBytesGracefully(c, resp, jsonResponse)
	return usage, nil
}
