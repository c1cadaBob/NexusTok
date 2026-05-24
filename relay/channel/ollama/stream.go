// Package ollama 的流式和非流式响应处理文件。
// 负责将 Ollama 的原生响应格式转换为 OpenAI 兼容的响应格式。
// 支持：
// - 流式聊天/生成响应（ollamaStreamHandler）
// - 非流式聊天/生成响应（ollamaChatHandler）
// - 思考内容（reasoning content）的提取和转换
// - 工具调用（tool calls）的提取和转换
// - 使用量统计（token 计数）
package ollama

import (
	"bufio"      // 用于逐行读取流式响应
	"encoding/json"
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
	"github.com/c1cada/NexusTok/relay/helper"                        // Relay 辅助工具（流式响应生成）
	"github.com/c1cada/NexusTok/service"                             // 服务层（响应转换等）
	"github.com/c1cada/NexusTok/types"                               // 类型定义

	// 第三方依赖
	"github.com/gin-gonic/gin"                                       // Gin Web 框架
)

// ollamaChatStreamChunk 表示 Ollama 流式响应的单个数据块。
// Ollama 的流式响应是 NDJSON 格式（每行一个 JSON 对象）。
// 支持两种模式：聊天模式（message 字段）和生成模式（response 字段）。
type ollamaChatStreamChunk struct {
	Model     string `json:"model"`      // 模型名称
	CreatedAt string `json:"created_at"` // 创建时间（RFC3339 格式）
	// 聊天模式的响应内容
	Message *struct {
		Role      string          `json:"role"`       // 消息角色
		Content   string          `json:"content"`    // 消息内容
		Thinking  json.RawMessage `json:"thinking"`   // 思考内容
		ToolCalls []struct {
			Function struct {
				Name      string      `json:"name"`      // 函数名称
				Arguments interface{} `json:"arguments"` // 函数参数
			} `json:"function"` // 函数调用详情
		} `json:"tool_calls"` // 工具调用列表
	} `json:"message"` // 聊天消息（聊天模式）
	// 生成模式的响应内容
	Response           string `json:"response"`              // 生成的文本（生成模式）
	Done               bool   `json:"done"`                  // 是否完成
	DoneReason         string `json:"done_reason"`           // 完成原因（stop/length 等）
	TotalDuration      int64  `json:"total_duration"`        // 总耗时（纳秒）
	LoadDuration       int64  `json:"load_duration"`         // 模型加载耗时（纳秒）
	PromptEvalCount    int    `json:"prompt_eval_count"`     // 提示词 token 数量
	EvalCount          int    `json:"eval_count"`            // 生成的 token 数量
	PromptEvalDuration int64  `json:"prompt_eval_duration"`  // 提示词处理耗时（纳秒）
	EvalDuration       int64  `json:"eval_duration"`         // 生成耗时（纳秒）
}

// toUnix 将时间字符串转换为 Unix 时间戳。
// 支持 RFC3339 和 RFC3339Nano 格式。
// 参数:
//   - ts: 时间字符串
// 返回:
//   - int64: Unix 时间戳（秒）
func toUnix(ts string) int64 {
	if ts == "" {
		return time.Now().Unix()
	}
	// 尝试 RFC3339Nano 格式
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		// 尝试 RFC3339 格式
		t2, err2 := time.Parse(time.RFC3339, ts)
		if err2 == nil {
			return t2.Unix()
		}
		// 解析失败，返回当前时间
		return time.Now().Unix()
	}
	return t.Unix()
}

// ollamaStreamHandler 处理 Ollama 的流式响应。
// 将 Ollama 的 NDJSON 流式响应转换为 OpenAI 兼容的 SSE 格式。
// 处理流程：
// 1. 设置 SSE 头部
// 2. 发送开始响应
// 3. 逐行读取 Ollama 响应并转换为 OpenAI 格式
// 4. 处理思考内容和工具调用
// 5. 发送完成响应和使用量统计
// 参数:
//   - c: Gin 上下文
//   - info: Relay 信息
//   - resp: Ollama 的 HTTP 响应
// 返回:
//   - *dto.Usage: 使用量统计
//   - *types.NexusTokError: 错误信息（成功时为 nil）
func ollamaStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NexusTokError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("empty response"), types.ErrorCodeBadResponse, http.StatusBadRequest)
	}
	defer service.CloseResponseBodyGracefully(resp)

	// 设置 SSE 头部
	helper.SetEventStreamHeaders(c)
	scanner := bufio.NewScanner(resp.Body)
	usage := &dto.Usage{}
	var model = info.UpstreamModelName
	var responseId = common.GetUUID()
	var created = time.Now().Unix()
	var toolCallIndex int // 工具调用索引计数器

	// 发送开始响应（空 choices）
	start := helper.GenerateStartEmptyResponse(responseId, created, model, nil)
	if data, err := common.Marshal(start); err == nil {
		_ = helper.StringData(c, string(data))
	}

	// 逐行读取流式响应
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// 解析 JSON 数据块
		var chunk ollamaChatStreamChunk
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			logger.LogError(c, "ollama stream json decode error: "+err.Error()+" line="+line)
			return usage, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		if chunk.Model != "" {
			model = chunk.Model
		}
		created = toUnix(chunk.CreatedAt)

		if !chunk.Done {
			// 处理增量内容
			var content string
			if chunk.Message != nil {
				content = chunk.Message.Content // 聊天模式
			} else {
				content = chunk.Response // 生成模式
			}
			// 构建 OpenAI 格式的增量响应
			delta := dto.ChatCompletionsStreamResponse{
				Id:      responseId,
				Object:  "chat.completion.chunk",
				Created: created,
				Model:   model,
				Choices: []dto.ChatCompletionsStreamResponseChoice{{
					Index: 0,
					Delta: dto.ChatCompletionsStreamResponseChoiceDelta{Role: "assistant"},
				}},
			}
			if content != "" {
				delta.Choices[0].Delta.SetContentString(content)
			}
			// 处理思考内容（reasoning content）
			if chunk.Message != nil && len(chunk.Message.Thinking) > 0 {
				raw := strings.TrimSpace(string(chunk.Message.Thinking))
				if raw != "" && raw != "null" {
					// 解析 JSON 字符串以获取实际内容（去除引号）
					var thinkingContent string
					if err := json.Unmarshal(chunk.Message.Thinking, &thinkingContent); err == nil {
						delta.Choices[0].Delta.SetReasoningContent(thinkingContent)
					} else {
						// 回退到原始字符串（如果不是 JSON 字符串格式）
						delta.Choices[0].Delta.SetReasoningContent(raw)
					}
				}
			}
			// 处理工具调用
			if chunk.Message != nil && len(chunk.Message.ToolCalls) > 0 {
				delta.Choices[0].Delta.ToolCalls = make([]dto.ToolCallResponse, 0, len(chunk.Message.ToolCalls))
				for _, tc := range chunk.Message.ToolCalls {
					// 将参数转换为字符串
					argBytes, _ := json.Marshal(tc.Function.Arguments)
					toolId := fmt.Sprintf("call_%d", toolCallIndex)
					tr := dto.ToolCallResponse{ID: toolId, Type: "function", Function: dto.FunctionResponse{Name: tc.Function.Name, Arguments: string(argBytes)}}
					tr.SetIndex(toolCallIndex)
					toolCallIndex++
					delta.Choices[0].Delta.ToolCalls = append(delta.Choices[0].Delta.ToolCalls, tr)
				}
			}
			// 发送增量响应
			if data, err := common.Marshal(delta); err == nil {
				_ = helper.StringData(c, string(data))
			}
			continue
		}
		// 处理完成帧（done=true）
		// 提取使用量统计
		usage.PromptTokens = chunk.PromptEvalCount
		usage.CompletionTokens = chunk.EvalCount
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
		finishReason := chunk.DoneReason
		if finishReason == "" {
			finishReason = "stop"
		}
		// 发送停止响应
		if stop := helper.GenerateStopResponse(responseId, created, model, finishReason); stop != nil {
			if data, err := common.Marshal(stop); err == nil {
				_ = helper.StringData(c, string(data))
			}
		}
		// 发送使用量统计帧
		if final := helper.GenerateFinalUsageResponse(responseId, created, model, *usage); final != nil {
			if data, err := common.Marshal(final); err == nil {
				_ = helper.StringData(c, string(data))
			}
		}
		// 发送 [DONE] 标记
		helper.Done(c)
		break
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		logger.LogError(c, "ollama stream scan error: "+err.Error())
	}
	return usage, nil
}

// ollamaChatHandler 处理 Ollama 的非流式响应。
// 将 Ollama 的非流式响应转换为 OpenAI 兼容的 JSON 格式。
// Ollama 的非流式响应可能是单个 JSON 对象或多行 NDJSON 格式。
// 参数:
//   - c: Gin 上下文
//   - info: Relay 信息
//   - resp: Ollama 的 HTTP 响应
// 返回:
//   - *dto.Usage: 使用量统计
//   - *types.NexusTokError: 错误信息（成功时为 nil）
func ollamaChatHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NexusTokError) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	service.CloseResponseBodyGracefully(resp)
	raw := string(body)
	if common.DebugEnabled {
		println("ollama non-stream raw resp:", raw)
	}

	// 尝试按多行 NDJSON 格式解析
	lines := strings.Split(raw, "\n")
	var (
		aggContent       strings.Builder  // 聚合内容
		reasoningBuilder strings.Builder  // 聚合思考内容
		lastChunk        ollamaChatStreamChunk
		parsedAny        bool
	)
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		var ck ollamaChatStreamChunk
		if err := json.Unmarshal([]byte(ln), &ck); err != nil {
			// 如果只有一行且解析失败，可能是单个 JSON 对象
			if len(lines) == 1 {
				return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
			}
			continue
		}
		parsedAny = true
		lastChunk = ck
		// 提取思考内容
		if ck.Message != nil && len(ck.Message.Thinking) > 0 {
			raw := strings.TrimSpace(string(ck.Message.Thinking))
			if raw != "" && raw != "null" {
				// 解析 JSON 字符串以获取实际内容（去除引号）
				var thinkingContent string
				if err := json.Unmarshal(ck.Message.Thinking, &thinkingContent); err == nil {
					reasoningBuilder.WriteString(thinkingContent)
				} else {
					// 回退到原始字符串
					reasoningBuilder.WriteString(raw)
				}
			}
		}
		// 提取内容（聊天模式或生成模式）
		if ck.Message != nil && ck.Message.Content != "" {
			aggContent.WriteString(ck.Message.Content)
		} else if ck.Response != "" {
			aggContent.WriteString(ck.Response)
		}
	}

	// 如果没有按 NDJSON 解析成功，尝试作为单个 JSON 对象解析
	if !parsedAny {
		var single ollamaChatStreamChunk
		if err := json.Unmarshal(body, &single); err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		lastChunk = single
		if single.Message != nil {
			// 提取思考内容
			if len(single.Message.Thinking) > 0 {
				raw := strings.TrimSpace(string(single.Message.Thinking))
				if raw != "" && raw != "null" {
					// 解析 JSON 字符串以获取实际内容（去除引号）
					var thinkingContent string
					if err := json.Unmarshal(single.Message.Thinking, &thinkingContent); err == nil {
						reasoningBuilder.WriteString(thinkingContent)
					} else {
						// 回退到原始字符串
						reasoningBuilder.WriteString(raw)
					}
				}
			}
			aggContent.WriteString(single.Message.Content)
		} else {
			aggContent.WriteString(single.Response)
		}
	}

	// 构建 OpenAI 格式的完整响应
	model := lastChunk.Model
	if model == "" {
		model = info.UpstreamModelName
	}
	created := toUnix(lastChunk.CreatedAt)
	usage := &dto.Usage{PromptTokens: lastChunk.PromptEvalCount, CompletionTokens: lastChunk.EvalCount, TotalTokens: lastChunk.PromptEvalCount + lastChunk.EvalCount}
	content := aggContent.String()
	finishReason := lastChunk.DoneReason
	if finishReason == "" {
		finishReason = "stop"
	}

	// 构建消息对象
	msg := dto.Message{Role: "assistant", Content: contentPtr(content)}
	if rc := reasoningBuilder.String(); rc != "" {
		msg.ReasoningContent = &rc // 设置思考内容
	}
	// 构建完整响应
	full := dto.OpenAITextResponse{
		Id:      common.GetUUID(),
		Model:   model,
		Object:  "chat.completion",
		Created: created,
		Choices: []dto.OpenAITextResponseChoice{{
			Index:        0,
			Message:      msg,
			FinishReason: finishReason,
		}},
		Usage: *usage,
	}
	out, _ := common.Marshal(full)
	service.IOCopyBytesGracefully(c, resp, out)
	return usage, nil
}

// contentPtr 将字符串转换为指针。
// 空字符串返回 nil，非空字符串返回指针。
// 参数:
//   - s: 输入字符串
// 返回:
//   - *string: 字符串指针（空字符串时为 nil）
func contentPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
