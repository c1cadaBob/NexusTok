// Package helper - common.go
// 本文件提供了中继层的通用辅助函数，包括：
//   - 流式响应工具：FlushWriter、SetEventStreamHeaders、StringData、ObjectData、Done 等
//   - Claude 格式输出：ClaudeData、ClaudeChunkData
//   - Responses API 格式输出：ResponseChunkData
//   - WebSocket 通信：WssString、WssObject、WssError
//   - 心跳 Ping：PingData
//   - ID 生成：GetResponseID、GetLocalRealtimeID
//   - 流式响应模板生成：GenerateStartEmptyResponse、GenerateStopResponse、GenerateFinalUsageResponse
package helper

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/logger"
	"github.com/c1cada/NexusTok/types"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// FlushWriter 刷新 Gin 的 ResponseWriter 缓冲区。
// 在流式响应中，需要手动刷新以确保数据立即发送到客户端。
// 包含 panic 恢复和请求上下文取消检测。
//
// 参数：
//   - c: Gin 上下文
//
// 返回值：
//   - error: 刷新失败时的错误
func FlushWriter(c *gin.Context) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("flush panic recovered: %v", r)
		}
	}()

	if c == nil || c.Writer == nil {
		return nil
	}

	if c.Request != nil && c.Request.Context().Err() != nil {
		return fmt.Errorf("request context done: %w", c.Request.Context().Err())
	}

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return errors.New("streaming error: flusher not found")
	}

	flusher.Flush()
	return nil
}

// SetEventStreamHeaders 设置 SSE（Server-Sent Events）流式响应的 HTTP 头。
// 设置的头包括：Content-Type: text/event-stream、Cache-Control: no-cache、
// Connection: keep-alive、Transfer-Encoding: chunked、X-Accel-Buffering: no。
// 使用标志位防止重复设置。
//
// 参数：
//   - c: Gin 上下文
func SetEventStreamHeaders(c *gin.Context) {
	// 检查是否已经设置过头部
	if _, exists := c.Get("event_stream_headers_set"); exists {
		return
	}

	// 设置标志，表示头部已经设置过
	c.Set("event_stream_headers_set", true)

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Transfer-Encoding", "chunked")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
}

// ClaudeData 向客户端发送 Claude 格式的 SSE 事件。
// 先发送事件类型行（event: xxx），再发送数据行（data: xxx）。
//
// 参数：
//   - c: Gin 上下文
//   - resp: Claude 响应对象
//
// 返回值：
//   - error: 发送失败时的错误
func ClaudeData(c *gin.Context, resp dto.ClaudeResponse) error {
	jsonData, err := common.Marshal(resp)
	if err != nil {
		common.SysError("error marshalling stream response: " + err.Error())
	} else {
		c.Render(-1, common.CustomEvent{Data: fmt.Sprintf("event: %s\n", resp.Type)})
		c.Render(-1, common.CustomEvent{Data: "data: " + string(jsonData)})
	}
	_ = FlushWriter(c)
	return nil
}

// ClaudeChunkData 向客户端发送 Claude 格式的 SSE 数据块。
// 与 ClaudeData 类似，但接受预格式化的数据字符串。
func ClaudeChunkData(c *gin.Context, resp dto.ClaudeResponse, data string) {
	c.Render(-1, common.CustomEvent{Data: fmt.Sprintf("event: %s\n", resp.Type)})
	c.Render(-1, common.CustomEvent{Data: fmt.Sprintf("data: %s\n", data)})
	_ = FlushWriter(c)
}

// ResponseChunkData 向客户端发送 Responses API 格式的 SSE 数据块。
func ResponseChunkData(c *gin.Context, resp dto.ResponsesStreamResponse, data string) {
	c.Render(-1, common.CustomEvent{Data: fmt.Sprintf("event: %s\n", resp.Type)})
	c.Render(-1, common.CustomEvent{Data: fmt.Sprintf("data: %s", data)})
	_ = FlushWriter(c)
}

// StringData 向客户端发送原始字符串格式的 SSE 数据。
// 格式为 "data: {str}"。
//
// 参数：
//   - c: Gin 上下文
//   - str: 要发送的字符串数据
//
// 返回值：
//   - error: 发送失败时的错误
func StringData(c *gin.Context, str string) error {
	if c == nil || c.Writer == nil {
		return errors.New("context or writer is nil")
	}

	if c.Request != nil && c.Request.Context().Err() != nil {
		return fmt.Errorf("request context done: %w", c.Request.Context().Err())
	}

	c.Render(-1, common.CustomEvent{Data: "data: " + str})
	return FlushWriter(c)
}

// PingData 向客户端发送 SSE 心跳注释（": PING"）。
// 用于保持 SSE 连接活跃，防止代理或负载均衡器超时断开。
//
// 参数：
//   - c: Gin 上下文
//
// 返回值：
//   - error: 发送失败时的错误
func PingData(c *gin.Context) error {
	if c == nil || c.Writer == nil {
		return errors.New("context or writer is nil")
	}

	if c.Request != nil && c.Request.Context().Err() != nil {
		return fmt.Errorf("request context done: %w", c.Request.Context().Err())
	}

	if _, err := c.Writer.Write([]byte(": PING\n\n")); err != nil {
		return fmt.Errorf("write ping data failed: %w", err)
	}
	return FlushWriter(c)
}

// ObjectData 将对象序列化为 JSON 后以 SSE 格式发送给客户端。
func ObjectData(c *gin.Context, object interface{}) error {
	if object == nil {
		return errors.New("object is nil")
	}
	jsonData, err := common.Marshal(object)
	if err != nil {
		return fmt.Errorf("error marshalling object: %w", err)
	}
	return StringData(c, string(jsonData))
}

// Done 向客户端发送 SSE 流结束标记 "[DONE]"。
func Done(c *gin.Context) {
	_ = StringData(c, "[DONE]")
}

// WssString 通过 WebSocket 连接发送文本消息。
func WssString(c *gin.Context, ws *websocket.Conn, str string) error {
	if ws == nil {
		logger.LogError(c, "websocket connection is nil")
		return errors.New("websocket connection is nil")
	}
	//common.LogInfo(c, fmt.Sprintf("sending message: %s", str))
	return ws.WriteMessage(1, []byte(str))
}

// WssObject 将对象序列化为 JSON 后通过 WebSocket 连接发送。
func WssObject(c *gin.Context, ws *websocket.Conn, object interface{}) error {
	jsonData, err := common.Marshal(object)
	if err != nil {
		return fmt.Errorf("error marshalling object: %w", err)
	}
	if ws == nil {
		logger.LogError(c, "websocket connection is nil")
		return errors.New("websocket connection is nil")
	}
	//common.LogInfo(c, fmt.Sprintf("sending message: %s", jsonData))
	return ws.WriteMessage(1, jsonData)
}

// WssError 通过 WebSocket 连接发送错误事件。
func WssError(c *gin.Context, ws *websocket.Conn, openaiError types.OpenAIError) {
	if ws == nil {
		return
	}
	errorObj := &dto.RealtimeEvent{
		Type:    "error",
		EventId: GetLocalRealtimeID(c),
		Error:   &openaiError,
	}
	_ = WssObject(c, ws, errorObj)
}

// GetResponseID 生成 Chat Completions 响应的唯一 ID。
// 格式为 "chatcmpl-{requestId}"。
func GetResponseID(c *gin.Context) string {
	logID := c.GetString(common.RequestIdKey)
	return fmt.Sprintf("chatcmpl-%s", logID)
}

// GetLocalRealtimeID 生成实时通信事件的唯一 ID。
// 格式为 "evt_{requestId}"。
func GetLocalRealtimeID(c *gin.Context) string {
	logID := c.GetString(common.RequestIdKey)
	return fmt.Sprintf("evt_%s", logID)
}

// GenerateStartEmptyResponse 生成流式响应的起始数据块。
// 包含空内容的 assistant 角色 delta，用于初始化流式对话。
//
// 参数：
//   - id: 响应 ID
//   - createAt: 创建时间戳
//   - model: 模型名称
//   - systemFingerprint: 系统指纹（可选）
//
// 返回值：
//   - *dto.ChatCompletionsStreamResponse: 流式响应数据块
func GenerateStartEmptyResponse(id string, createAt int64, model string, systemFingerprint *string) *dto.ChatCompletionsStreamResponse {
	return &dto.ChatCompletionsStreamResponse{
		Id:                id,
		Object:            "chat.completion.chunk",
		Created:           createAt,
		Model:             model,
		SystemFingerprint: systemFingerprint,
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{
				Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
					Role:    "assistant",
					Content: common.GetPointer(""),
				},
			},
		},
	}
}

// GenerateStopResponse 生成流式响应的结束数据块。
// 包含 finish_reason 字段，标记流式响应的结束原因（如 stop、length）。
func GenerateStopResponse(id string, createAt int64, model string, finishReason string) *dto.ChatCompletionsStreamResponse {
	return &dto.ChatCompletionsStreamResponse{
		Id:                id,
		Object:            "chat.completion.chunk",
		Created:           createAt,
		Model:             model,
		SystemFingerprint: nil,
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{
				FinishReason: &finishReason,
			},
		},
	}
}

// GenerateFinalUsageResponse 生成包含使用量统计的最终流式响应数据块。
// 当 StreamOptions.IncludeUsage 为 true 时，在流式响应末尾发送此数据块。
func GenerateFinalUsageResponse(id string, createAt int64, model string, usage dto.Usage) *dto.ChatCompletionsStreamResponse {
	return &dto.ChatCompletionsStreamResponse{
		Id:                id,
		Object:            "chat.completion.chunk",
		Created:           createAt,
		Model:             model,
		SystemFingerprint: nil,
		Choices:           make([]dto.ChatCompletionsStreamResponseChoice, 0),
		Usage:             &usage,
	}
}
