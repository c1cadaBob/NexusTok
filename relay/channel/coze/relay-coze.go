// coze - relay-coze.go
// 本文件实现了 Coze 渠道的核心中继逻辑。
// 包含将 OpenAI 格式请求转换为 Coze 格式、处理 Coze 流式和非流式响应的函数，
// 以及与 Coze API 交互的辅助函数（轮询状态、获取详情、发送请求）。
// Coze API 的非流式模式需要三步流程：创建聊天 -> 轮询状态 -> 获取消息详情。
package coze

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/dto"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/relay/helper"
	"github.com/c1cada/NexusTok/service"
	"github.com/c1cada/NexusTok/types"
	"github.com/samber/lo"

	"github.com/gin-gonic/gin"
)

// convertCozeChatRequest 将 OpenAI 格式的通用聊天请求转换为 Coze 格式的聊天请求。
// 转换规则：
//   - 仅提取 role 为 "user" 的消息，转换为 CozeEnterMessage
//   - 用户标识优先使用请求中的 user 字段，否则使用响应 ID
//   - Bot ID 从 Gin 上下文的 "bot_id" 键获取（由中间件设置）
//   - 流式模式由请求的 Stream 字段控制
//
// 参数：
//   - c: Gin 上下文（包含 bot_id 等渠道特定信息）
//   - request: OpenAI 格式的通用聊天请求
//
// 返回值：转换后的 CozeChatRequest 指针。
func convertCozeChatRequest(c *gin.Context, request dto.GeneralOpenAIRequest) *CozeChatRequest {
	var messages []CozeEnterMessage
	// 将 request 的 messages 中 role 为 user 的 content 转换为 CozeMessage
	for _, message := range request.Messages {
		if message.Role == "user" {
			messages = append(messages, CozeEnterMessage{
				Role:    "user",
				Content: message.Content,
				// TODO: support more content type
				ContentType: "text",
			})
		}
	}
	user := request.User
	if len(user) == 0 {
		user = json.RawMessage(helper.GetResponseID(c))
	}
	cozeRequest := &CozeChatRequest{
		BotId:              c.GetString("bot_id"),
		UserId:             user,
		AdditionalMessages: messages,
		Stream:             lo.FromPtrOr(request.Stream, false),
	}
	return cozeRequest
}

// cozeChatHandler 处理 Coze 非流式聊天响应。
// 将 Coze 的聊天详情响应转换为 OpenAI 兼容的 TextResponse 格式。
// 处理流程：
//  1. 读取并解析 CozeChatDetailResponse 响应体
//  2. 从上下文中获取 token 用量信息（由轮询阶段设置）
//  3. 遍历消息详情列表，找到 type 为 "answer" 的消息作为回答内容
//  4. 构建 OpenAI 格式的 TextResponse 并写入客户端
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继请求信息
//   - resp: 上游 Coze API 的 HTTP 响应
//
// 返回值：token 用量信息指针和可能的 NexusTok 错误。
func cozeChatHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NexusTokError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	service.CloseResponseBodyGracefully(resp)
	// 将 Coze 响应转换为 OpenAI 响应格式
	var response dto.TextResponse
	var cozeResponse CozeChatDetailResponse
	response.Model = info.UpstreamModelName
	err = json.Unmarshal(responseBody, &cozeResponse)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	if cozeResponse.Code != 0 {
		return nil, types.NewError(errors.New(cozeResponse.Msg), types.ErrorCodeBadResponseBody)
	}
	// 从上下文获取 usage（由轮询阶段的 checkIfChatComplete 设置）
	var usage dto.Usage
	usage.PromptTokens = c.GetInt("coze_input_count")
	usage.CompletionTokens = c.GetInt("coze_output_count")
	usage.TotalTokens = c.GetInt("coze_token_count")
	response.Usage = usage
	response.Id = helper.GetResponseID(c)

	// 遍历消息详情，找到类型为 "answer" 的消息作为最终回答
	var responseContent json.RawMessage
	for _, data := range cozeResponse.Data {
		if data.Type == "answer" {
			responseContent = data.Content
			response.Created = data.CreatedAt
		}
	}
	// 构建 OpenAI 格式的选择列表
	response.Choices = []dto.OpenAITextResponseChoice{
		{
			Index:        0,
			Message:      dto.Message{Role: "assistant", Content: responseContent},
			FinishReason: "stop",
		},
	}
	jsonResponse, err := json.Marshal(response)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, _ = c.Writer.Write(jsonResponse)

	return &usage, nil
}

// cozeChatStreamHandler 处理 Coze 流式聊天响应。
// 将 Coze 的 SSE（Server-Sent Events）事件流转换为 OpenAI 兼容的流式格式。
// 处理流程：
//  1. 设置 SSE 响应头
//  2. 使用 bufio.Scanner 逐行读取 SSE 事件
//  3. 解析 event: 和 data: 行，构建事件对象
//  4. 遇到空行时处理上一个完整的事件
//  5. 流结束后发送 [DONE] 标记
//  6. 如果上游未返回 token 用量，使用估算值
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继请求信息
//   - resp: 上游 Coze API 的 HTTP 响应
//
// 返回值：token 用量信息指针和可能的 NexusTok 错误。
func cozeChatStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NexusTokError) {
	scanner := bufio.NewScanner(resp.Body)
	scanner.Split(bufio.ScanLines)
	helper.SetEventStreamHeaders(c)
	id := helper.GetResponseID(c)
	var responseText string

	var currentEvent string
	var currentData string
	var usage = &dto.Usage{}

	for scanner.Scan() {
		line := scanner.Text()

		// 空行表示一个完整的事件结束，处理该事件
		if line == "" {
			if currentEvent != "" && currentData != "" {
				// 处理上一个完整事件
				handleCozeEvent(c, currentEvent, currentData, &responseText, usage, id, info)
				currentEvent = ""
				currentData = ""
			}
			continue
		}

		// 解析 SSE 事件类型
		if strings.HasPrefix(line, "event:") {
			currentEvent = strings.TrimSpace(line[6:])
			continue
		}

		// 解析 SSE 事件数据
		if strings.HasPrefix(line, "data:") {
			currentData = strings.TrimSpace(line[5:])
			continue
		}
	}

	// 处理最后一个事件（如果存在）
	if currentEvent != "" && currentData != "" {
		handleCozeEvent(c, currentEvent, currentData, &responseText, usage, id, info)
	}

	if err := scanner.Err(); err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	helper.Done(c)

	// 如果上游未返回 token 用量，使用估算值
	if usage.TotalTokens == 0 {
		usage = service.ResponseText2Usage(c, responseText, info.UpstreamModelName, c.GetInt("coze_input_count"))
	}

	return usage, nil
}

// handleCozeEvent 处理单个 Coze SSE 事件。
// 根据事件类型执行不同的处理逻辑：
//   - conversation.chat.completed: 聊天完成事件，提取 token 用量并发送停止响应
//   - conversation.message.delta: 消息增量事件，将文本内容转换为 OpenAI 流式格式
//   - error: 错误事件，记录错误日志
//
// 参数：
//   - c: Gin 上下文
//   - event: SSE 事件类型
//   - data: SSE 事件数据（JSON 字符串）
//   - responseText: 累积的响应文本指针（用于 token 估算）
//   - usage: token 用量信息指针
//   - id: 响应 ID
//   - info: 中继请求信息
func handleCozeEvent(c *gin.Context, event string, data string, responseText *string, usage *dto.Usage, id string, info *relaycommon.RelayInfo) {
	switch event {
	case "conversation.chat.completed":
		// 聊天完成事件，解析 token 用量
		var chatData CozeChatResponseData
		err := json.Unmarshal([]byte(data), &chatData)
		if err != nil {
			common.SysLog("error_unmarshalling_stream_response: " + err.Error())
			return
		}

		usage.PromptTokens = chatData.Usage.InputCount
		usage.CompletionTokens = chatData.Usage.OutputCount
		usage.TotalTokens = chatData.Usage.TokenCount

		// 发送停止响应，标记流结束
		finishReason := "stop"
		stopResponse := helper.GenerateStopResponse(id, common.GetTimestamp(), info.UpstreamModelName, finishReason)
		helper.ObjectData(c, stopResponse)

	case "conversation.message.delta":
		// 消息增量事件，提取文本内容并转换为 OpenAI 流式格式
		var messageData CozeChatV3MessageDetail
		err := json.Unmarshal([]byte(data), &messageData)
		if err != nil {
			common.SysLog("error_unmarshalling_stream_response: " + err.Error())
			return
		}

		// 解析消息内容（JSON 字符串）
		var content string
		err = json.Unmarshal(messageData.Content, &content)
		if err != nil {
			common.SysLog("error_unmarshalling_stream_response: " + err.Error())
			return
		}

		// 累积响应文本（用于 token 估算）
		*responseText += content

		// 构建 OpenAI 格式的流式响应
		openaiResponse := dto.ChatCompletionsStreamResponse{
			Id:      id,
			Object:  "chat.completion.chunk",
			Created: common.GetTimestamp(),
			Model:   info.UpstreamModelName,
		}

		choice := dto.ChatCompletionsStreamResponseChoice{
			Index: 0,
		}
		choice.Delta.SetContentString(content)
		openaiResponse.Choices = append(openaiResponse.Choices, choice)

		helper.ObjectData(c, openaiResponse)

	case "error":
		// 错误事件，记录错误日志
		var errorData CozeError
		err := json.Unmarshal([]byte(data), &errorData)
		if err != nil {
			common.SysLog("error_unmarshalling_stream_response: " + err.Error())
			return
		}

		common.SysLog(fmt.Sprintf("stream event error: %v %v", errorData.Code, errorData.Message))
	}
}

// checkIfChatComplete 检查 Coze 聊天是否已完成。
// 通过调用 /v3/chat/retrieve 端点轮询聊天状态。
// 状态说明：
//   - "completed": 聊天完成，将 token 用量信息保存到上下文
//   - "failed"/"canceled"/"requires_action": 聊天失败，返回错误
//   - 其他状态：聊天仍在进行中
//
// 参数：
//   - a: Coze 适配器指针
//   - c: Gin 上下文（包含 conversation_id 和 chat_id）
//   - info: 中继请求信息
//
// 返回值：错误信息（如果有）和是否完成的布尔值。
func checkIfChatComplete(a *Adaptor, c *gin.Context, info *relaycommon.RelayInfo) (error, bool) {
	requestURL := fmt.Sprintf("%s/v3/chat/retrieve", info.ChannelBaseUrl)

	// 构建查询参数：conversation_id 和 chat_id
	requestURL = requestURL + "?conversation_id=" + c.GetString("coze_conversation_id") + "&chat_id=" + c.GetString("coze_chat_id")
	req, err := http.NewRequest("GET", requestURL, nil)
	if err != nil {
		return err, false
	}
	// 设置请求头（包含认证信息）
	err = a.SetupRequestHeader(c, &req.Header, info)
	if err != nil {
		return err, false
	}

	resp, err := doRequest(req, info)
	if err != nil {
		return err, false
	}
	if resp == nil {
		return fmt.Errorf("resp is nil"), false
	}
	defer resp.Body.Close()

	// 解析响应
	var cozeResponse CozeChatResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body failed: %w", err), false
	}
	err = json.Unmarshal(responseBody, &cozeResponse)
	if err != nil {
		return fmt.Errorf("unmarshal response body failed: %w", err), false
	}
	if cozeResponse.Data.Status == "completed" {
		// 聊天完成，将 token 用量信息保存到上下文
		c.Set("coze_token_count", cozeResponse.Data.Usage.TokenCount)
		c.Set("coze_output_count", cozeResponse.Data.Usage.OutputCount)
		c.Set("coze_input_count", cozeResponse.Data.Usage.InputCount)
		return nil, true
	} else if cozeResponse.Data.Status == "failed" || cozeResponse.Data.Status == "canceled" || cozeResponse.Data.Status == "requires_action" {
		return fmt.Errorf("chat status: %s", cozeResponse.Data.Status), false
	} else {
		return nil, false
	}
}

// getChatDetail 获取 Coze 聊天的消息详情列表。
// 通过调用 /v3/chat/message/list 端点获取聊天过程中产生的所有消息。
// 参数：
//   - a: Coze 适配器指针
//   - c: Gin 上下文（包含 conversation_id 和 chat_id）
//   - info: 中继请求信息
//
// 返回值：HTTP 响应指针和可能的错误。
func getChatDetail(a *Adaptor, c *gin.Context, info *relaycommon.RelayInfo) (*http.Response, error) {
	requestURL := fmt.Sprintf("%s/v3/chat/message/list", info.ChannelBaseUrl)

	// 构建查询参数：conversation_id 和 chat_id
	requestURL = requestURL + "?conversation_id=" + c.GetString("coze_conversation_id") + "&chat_id=" + c.GetString("coze_chat_id")
	req, err := http.NewRequest("GET", requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("new request failed: %w", err)
	}
	// 设置请求头（包含认证信息）
	err = a.SetupRequestHeader(c, &req.Header, info)
	if err != nil {
		return nil, fmt.Errorf("setup request header failed: %w", err)
	}
	resp, err := doRequest(req, info)
	if err != nil {
		return nil, fmt.Errorf("do request failed: %w", err)
	}
	return resp, nil
}

// doRequest 执行 HTTP 请求。
// 根据渠道配置决定是否使用代理：
//   - 如果配置了代理（ChannelSetting.Proxy），则创建代理 HTTP 客户端
//   - 否则使用默认的 HTTP 客户端
//
// 参数：
//   - req: 要执行的 HTTP 请求
//   - info: 中继请求信息（包含代理配置）
//
// 返回值：HTTP 响应指针和可能的错误。
func doRequest(req *http.Request, info *relaycommon.RelayInfo) (*http.Response, error) {
	var client *http.Client
	var err error
	if info.ChannelSetting.Proxy != "" {
		client, err = service.NewProxyHttpClient(info.ChannelSetting.Proxy)
		if err != nil {
			return nil, fmt.Errorf("new proxy http client failed: %w", err)
		}
	} else {
		client = service.GetHttpClient()
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("client.Do failed: %w", err)
	}
	return resp, nil
}
