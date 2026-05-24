// 本文件实现了 Cohere 渠道的核心中继逻辑。
// 包含将 OpenAI 请求转换为 Cohere 格式、将 Cohere 响应转换回 OpenAI 格式的处理函数。
// 支持聊天补全（流式和非流式）以及重排序两种请求类型。
package cohere

// 标准库导入
import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	// 项目内部包
	"github.com/c1cada/NexusTok/common"               // 公共工具函数
	"github.com/c1cada/NexusTok/dto"                   // 数据传输对象
	relaycommon "github.com/c1cada/NexusTok/relay/common" // relay 层公共工具
	"github.com/c1cada/NexusTok/relay/helper"          // relay 辅助函数
	"github.com/c1cada/NexusTok/service"               // 服务层
	"github.com/c1cada/NexusTok/types"                 // 类型定义

	// 第三方依赖
	"github.com/gin-gonic/gin"
	"github.com/samber/lo" // Go 工具库（提供 FromPtrOr 等）
)

// requestOpenAI2Cohere 将 OpenAI 格式的聊天请求转换为 Cohere 格式。
// 转换规则：
//   - OpenAI 的 messages 数组拆分为 chat_history（历史消息）和 message（当前用户消息）
//   - role 映射：assistant -> CHATBOT，system -> SYSTEM，其他 -> USER
//   - 如果未设置 max_tokens，默认为 4000
//   - 根据 CohereSafetySetting 全局配置设置安全模式
//
// 参数：textRequest - OpenAI 格式的请求体。
// 返回值：转换后的 Cohere 请求体指针。
func requestOpenAI2Cohere(textRequest dto.GeneralOpenAIRequest) *CohereRequest {
	cohereReq := CohereRequest{
		Model:       textRequest.Model,
		ChatHistory: []ChatHistory{},
		Message:     "",
		Stream:      lo.FromPtrOr(textRequest.Stream, false),
		MaxTokens:   textRequest.GetMaxTokens(),
	}
	// 设置安全模式
	if common.CohereSafetySetting != "NONE" {
		cohereReq.SafetyMode = common.CohereSafetySetting
	}
	// 默认最大 token 数
	if cohereReq.MaxTokens == 0 {
		cohereReq.MaxTokens = 4000
	}
	// 遍历消息列表，最后一条 user 消息作为当前 message，其余归入 chat_history
	for _, msg := range textRequest.Messages {
		if msg.Role == "user" {
			cohereReq.Message = msg.StringContent()
		} else {
			var role string
			if msg.Role == "assistant" {
				role = "CHATBOT"
			} else if msg.Role == "system" {
				role = "SYSTEM"
			} else {
				role = "USER"
			}
			cohereReq.ChatHistory = append(cohereReq.ChatHistory, ChatHistory{
				Role:    role,
				Message: msg.StringContent(),
			})
		}
	}

	return &cohereReq
}

// requestConvertRerank2Cohere 将通用重排序请求转换为 Cohere 重排序格式。
// 参数：rerankRequest - 通用重排序请求体。
// 返回值：转换后的 Cohere 重排序请求体指针。
func requestConvertRerank2Cohere(rerankRequest dto.RerankRequest) *CohereRerankRequest {
	topN := lo.FromPtrOr(rerankRequest.TopN, 1)
	if topN <= 0 {
		topN = 1
	}
	cohereReq := CohereRerankRequest{
		Query:           rerankRequest.Query,
		Documents:       rerankRequest.Documents,
		Model:           rerankRequest.Model,
		TopN:            topN,
		ReturnDocuments: true,
	}
	return &cohereReq
}

// stopReasonCohere2OpenAI 将 Cohere 的结束原因映射为 OpenAI 格式。
// 映射规则：
//   - COMPLETE -> "stop"（正常完成）
//   - MAX_TOKENS -> "max_tokens"（达到最大 token 限制）
//   - 其他值原样返回
//
// 参数：reason - Cohere 格式的结束原因。
// 返回值：OpenAI 格式的结束原因。
func stopReasonCohere2OpenAI(reason string) string {
	switch reason {
	case "COMPLETE":
		return "stop"
	case "MAX_TOKENS":
		return "max_tokens"
	default:
		return reason
	}
}

// cohereStreamHandler 处理 Cohere 流式聊天响应。
// 将 Cohere 的逐行 JSON 流转换为 OpenAI 兼容的 SSE（Server-Sent Events）格式。
// 主要流程：
//  1. 使用 Scanner 按行读取响应体
//  2. 将每行 JSON 解析为 CohereResponse
//  3. 转换为 OpenAI ChatCompletionsStreamResponse 格式
//  4. 对于结束事件，提取用量信息
//  5. 通过 Gin 的 Stream 方法以 SSE 格式发送到客户端
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继请求信息
//   - resp: 上游 HTTP 响应
//
// 返回值：usage 用量信息和可能的错误。
func cohereStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NexusTokError) {
	responseId := helper.GetResponseID(c)       // 生成响应 ID
	createdTime := common.GetTimestamp()         // 获取创建时间戳
	usage := &dto.Usage{}
	responseText := ""                           // 累积响应文本（用于估算 token）
	scanner := bufio.NewScanner(resp.Body)
	// 自定义 Scanner 的 Split 函数，按换行符分割
	scanner.Split(func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		if atEOF && len(data) == 0 {
			return 0, nil, nil
		}
		if i := strings.Index(string(data), "\n"); i >= 0 {
			return i + 1, data[0:i], nil
		}
		if atEOF {
			return len(data), data, nil
		}
		return 0, nil, nil
	})
	dataChan := make(chan string)
	stopChan := make(chan bool)
	// 在 goroutine 中读取数据行并发送到 channel
	go func() {
		for scanner.Scan() {
			data := scanner.Text()
			dataChan <- data
		}
		stopChan <- true
	}()
	helper.SetEventStreamHeaders(c) // 设置 SSE 响应头
	isFirst := true
	c.Stream(func(w io.Writer) bool {
		select {
		case data := <-dataChan:
			// 记录首个响应时间
			if isFirst {
				isFirst = false
				info.FirstResponseTime = time.Now()
			}
			data = strings.TrimSuffix(data, "\r") // 移除 Windows 换行符
			var cohereResp CohereResponse
			err := json.Unmarshal([]byte(data), &cohereResp)
			if err != nil {
				common.SysLog("error unmarshalling stream response: " + err.Error())
				return true
			}
			// 构建 OpenAI 格式的流式响应
			var openaiResp dto.ChatCompletionsStreamResponse
			openaiResp.Id = responseId
			openaiResp.Created = createdTime
			openaiResp.Object = "chat.completion.chunk"
			openaiResp.Model = info.UpstreamModelName
			if cohereResp.IsFinished {
				// 结束事件：设置完成原因和用量信息
				finishReason := stopReasonCohere2OpenAI(cohereResp.FinishReason)
				openaiResp.Choices = []dto.ChatCompletionsStreamResponseChoice{
					{
						Delta:        dto.ChatCompletionsStreamResponseChoiceDelta{},
						Index:        0,
						FinishReason: &finishReason,
					},
				}
				if cohereResp.Response != nil {
					usage.PromptTokens = cohereResp.Response.Meta.BilledUnits.InputTokens
					usage.CompletionTokens = cohereResp.Response.Meta.BilledUnits.OutputTokens
				}
			} else {
				// 中间事件：包含文本增量
				openaiResp.Choices = []dto.ChatCompletionsStreamResponseChoice{
					{
						Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
							Role:    "assistant",
							Content: &cohereResp.Text,
						},
						Index: 0,
					},
				}
				responseText += cohereResp.Text // 累积文本用于估算
			}
			jsonStr, err := json.Marshal(openaiResp)
			if err != nil {
				common.SysLog("error marshalling stream response: " + err.Error())
				return true
			}
			// 以 SSE 格式发送数据
			c.Render(-1, common.CustomEvent{Data: "data: " + string(jsonStr)})
			return true
		case <-stopChan:
			// 流结束，发送 [DONE] 信号
			c.Render(-1, common.CustomEvent{Data: "data: [DONE]"})
			return false
		}
	})
	// 如果上游未返回 token 用量信息，则基于响应文本估算
	if usage.PromptTokens == 0 {
		usage = service.ResponseText2Usage(c, responseText, info.UpstreamModelName, info.GetEstimatePromptTokens())
	}
	return usage, nil
}

// cohereHandler 处理 Cohere 非流式聊天响应。
// 将 Cohere 的完整 JSON 响应转换为 OpenAI TextResponse 格式并写入客户端。
// 参数：
//   - c: Gin 上下文
//   - info: 中继请求信息
//   - resp: 上游 HTTP 响应
//
// 返回值：usage 用量信息和可能的错误。
func cohereHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NexusTokError) {
	createdTime := common.GetTimestamp()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	service.CloseResponseBodyGracefully(resp) // 优雅关闭响应体
	var cohereResp CohereResponseResult
	err = json.Unmarshal(responseBody, &cohereResp)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	// 构建用量信息
	usage := dto.Usage{}
	usage.PromptTokens = cohereResp.Meta.BilledUnits.InputTokens
	usage.CompletionTokens = cohereResp.Meta.BilledUnits.OutputTokens
	usage.TotalTokens = cohereResp.Meta.BilledUnits.InputTokens + cohereResp.Meta.BilledUnits.OutputTokens

	// 构建 OpenAI 格式的响应
	var openaiResp dto.TextResponse
	openaiResp.Id = cohereResp.ResponseId
	openaiResp.Created = createdTime
	openaiResp.Object = "chat.completion"
	openaiResp.Model = info.UpstreamModelName
	openaiResp.Usage = usage

	openaiResp.Choices = []dto.OpenAITextResponseChoice{
		{
			Index:        0,
			Message:      dto.Message{Content: cohereResp.Text, Role: "assistant"},
			FinishReason: stopReasonCohere2OpenAI(cohereResp.FinishReason),
		},
	}

	jsonResponse, err := json.Marshal(openaiResp)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	// 写入响应
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, _ = c.Writer.Write(jsonResponse)
	return &usage, nil
}

// cohereRerankHandler 处理 Cohere 重排序响应。
// 将 Cohere 重排序结果转换为通用 RerankResponse 格式。
// 如果上游未返回输入 token 数，则使用请求阶段的估算值。
// 参数：
//   - c: Gin 上下文
//   - resp: 上游 HTTP 响应
//   - info: 中继请求信息
//
// 返回值：usage 用量信息和可能的错误。
func cohereRerankHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NexusTokError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	service.CloseResponseBodyGracefully(resp)
	var cohereResp CohereRerankResponseResult
	err = json.Unmarshal(responseBody, &cohereResp)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	// 构建用量信息，重排序请求无输出 token
	usage := dto.Usage{}
	if cohereResp.Meta.BilledUnits.InputTokens == 0 {
		// 上游未返回 token 数，使用估算值
		usage.PromptTokens = info.GetEstimatePromptTokens()
		usage.CompletionTokens = 0
		usage.TotalTokens = info.GetEstimatePromptTokens()
	} else {
		usage.PromptTokens = cohereResp.Meta.BilledUnits.InputTokens
		usage.CompletionTokens = cohereResp.Meta.BilledUnits.OutputTokens
		usage.TotalTokens = cohereResp.Meta.BilledUnits.InputTokens + cohereResp.Meta.BilledUnits.OutputTokens
	}

	// 构建通用重排序响应
	var rerankResp dto.RerankResponse
	rerankResp.Results = cohereResp.Results
	rerankResp.Usage = usage

	jsonResponse, err := json.Marshal(rerankResp)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	// 写入响应
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, err = c.Writer.Write(jsonResponse)
	return &usage, nil
}
