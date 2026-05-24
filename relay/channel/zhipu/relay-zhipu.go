// 智谱 ChatGLM 的核心中继处理逻辑。
// 负责 JWT 鉴权令牌的生成与缓存、OpenAI 与智谱格式之间的双向转换，
// 以及流式（SSE）/ 非流式响应的处理。
package zhipu

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	// 项目内部依赖
	"github.com/c1cada/NexusTok/common"              // 通用工具函数
	"github.com/c1cada/NexusTok/constant"             // 常量定义
	"github.com/c1cada/NexusTok/dto"                  // 数据传输对象
	relaycommon "github.com/c1cada/NexusTok/relay/common" // 中继层通用结构体
	"github.com/c1cada/NexusTok/relay/helper"          // 中继辅助函数
	"github.com/c1cada/NexusTok/service"               // 服务层工具
	"github.com/c1cada/NexusTok/types"                 // 错误类型
	"github.com/samber/lo"                            // 泛型工具库

	// 第三方依赖
	"github.com/gin-gonic/gin"          // HTTP 框架
	"github.com/golang-jwt/jwt/v5"      // JWT 库
)

// 智谱 API 文档参考：
// https://open.bigmodel.cn/doc/api#chatglm_std
// https://open.bigmodel.cn/api/paas/v3/model-api/chatglm_std/invoke
// https://open.bigmodel.cn/api/paas/v3/model-api/chatglm_std/sse-invoke

// zhipuTokens JWT 令牌缓存，使用 sync.Map 保证并发安全。
var zhipuTokens sync.Map

// expSeconds JWT 令牌有效期（秒），默认 24 小时。
var expSeconds int64 = 24 * 3600

// getZhipuToken 获取智谱 API 的 JWT 鉴权令牌。
// 首先检查缓存中是否有未过期的令牌，如果有则直接返回；
// 否则根据 API Key（格式为 "id.secret"）生成新的 JWT 令牌并缓存。
//
// 参数：
//   - apikey: 智谱 API Key，格式为 "{id}.{secret}"
//
// 返回：JWT 令牌字符串，如果 key 格式无效则返回空字符串。
func getZhipuToken(apikey string) string {
	// 检查缓存中是否有未过期的令牌
	data, ok := zhipuTokens.Load(apikey)
	if ok {
		tokenData := data.(zhipuTokenData)
		if time.Now().Before(tokenData.ExpiryTime) {
			return tokenData.Token
		}
	}

	// 解析 API Key：格式为 "id.secret"
	split := strings.Split(apikey, ".")
	if len(split) != 2 {
		common.SysLog("invalid zhipu key: " + apikey)
		return ""
	}

	id := split[0]
	secret := split[1]

	// 计算过期时间（毫秒级时间戳）
	expMillis := time.Now().Add(time.Duration(expSeconds)*time.Second).UnixNano() / 1e6
	expiryTime := time.Now().Add(time.Duration(expSeconds) * time.Second)

	// 当前时间戳（毫秒）
	timestamp := time.Now().UnixNano() / 1e6

	// 构造 JWT Claims
	payload := jwt.MapClaims{
		"api_key":   id,
		"exp":       expMillis,
		"timestamp": timestamp,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, payload)

	// 设置 JWT 头部
	token.Header["alg"] = "HS256"
	token.Header["sign_type"] = "SIGN"

	// 使用 secret 签名
	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		return ""
	}

	// 缓存令牌
	zhipuTokens.Store(apikey, zhipuTokenData{
		Token:      tokenString,
		ExpiryTime: expiryTime,
	})

	return tokenString
}

// requestOpenAI2Zhipu 将 OpenAI 格式的请求转换为智谱格式。
//
// 特殊处理：
//   - system 消息会被转换为 system + "Okay" 的 user 消息对，
//     以符合智谱的消息格式要求。
//
// 参数：
//   - request: OpenAI 格式的聊天请求
//
// 返回：智谱格式的请求指针。
func requestOpenAI2Zhipu(request dto.GeneralOpenAIRequest) *ZhipuRequest {
	messages := make([]ZhipuMessage, 0, len(request.Messages))
	for _, message := range request.Messages {
		if message.Role == "system" {
			// 将 system 消息保留在 system 角色，但追加一个 "Okay" 的 user 消息
			messages = append(messages, ZhipuMessage{
				Role:    "system",
				Content: message.StringContent(),
			})
			messages = append(messages, ZhipuMessage{
				Role:    "user",
				Content: "Okay",
			})
		} else {
			messages = append(messages, ZhipuMessage{
				Role:    message.Role,
				Content: message.StringContent(),
			})
		}
	}
	return &ZhipuRequest{
		Prompt:      messages,
		Temperature: request.Temperature,
		TopP:        lo.FromPtrOr(request.TopP, 0),
		Incremental: false,
	}
}

// responseZhipu2OpenAI 将智谱的非流式响应转换为 OpenAI 格式。
//
// 参数：
//   - response: 智谱格式的响应
//
// 返回：OpenAI 格式的文本响应指针。
func responseZhipu2OpenAI(response *ZhipuResponse) *dto.OpenAITextResponse {
	fullTextResponse := dto.OpenAITextResponse{
		Id:      response.Data.TaskId,
		Object:  "chat.completion",
		Created: common.GetTimestamp(),
		Choices: make([]dto.OpenAITextResponseChoice, 0, len(response.Data.Choices)),
		Usage:   response.Data.Usage,
	}
	for i, choice := range response.Data.Choices {
		openaiChoice := dto.OpenAITextResponseChoice{
			Index: i,
			Message: dto.Message{
				Role:    choice.Role,
				Content: strings.Trim(choice.Content, "\""), // 去除可能存在的引号
			},
			FinishReason: "",
		}
		// 最后一个选项标记为 stop
		if i == len(response.Data.Choices)-1 {
			openaiChoice.FinishReason = "stop"
		}
		fullTextResponse.Choices = append(fullTextResponse.Choices, openaiChoice)
	}
	return &fullTextResponse
}

// streamResponseZhipu2OpenAI 将智谱的流式数据响应转换为 OpenAI SSE 格式。
//
// 参数：
//   - zhipuResponse: 智谱流式响应的文本内容
//
// 返回：OpenAI 格式的流式响应 chunk。
func streamResponseZhipu2OpenAI(zhipuResponse string) *dto.ChatCompletionsStreamResponse {
	var choice dto.ChatCompletionsStreamResponseChoice
	choice.Delta.SetContentString(zhipuResponse)
	response := dto.ChatCompletionsStreamResponse{
		Object:  "chat.completion.chunk",
		Created: common.GetTimestamp(),
		Model:   "chatglm",
		Choices: []dto.ChatCompletionsStreamResponseChoice{choice},
	}
	return &response
}

// streamMetaResponseZhipu2OpenAI 将智谱流式响应的元数据（结束时的汇总信息）转换为 OpenAI 格式。
// 元数据包含 token 使用量等信息，在流式传输的最后一帧发送。
//
// 参数：
//   - zhipuResponse: 智谱流式元数据响应
//
// 返回：OpenAI 格式的流式响应 chunk 和 token 使用量。
func streamMetaResponseZhipu2OpenAI(zhipuResponse *ZhipuStreamMetaResponse) (*dto.ChatCompletionsStreamResponse, *dto.Usage) {
	var choice dto.ChatCompletionsStreamResponseChoice
	choice.Delta.SetContentString("")
	choice.FinishReason = &constant.FinishReasonStop
	response := dto.ChatCompletionsStreamResponse{
		Id:      zhipuResponse.RequestId,
		Object:  "chat.completion.chunk",
		Created: common.GetTimestamp(),
		Model:   "chatglm",
		Choices: []dto.ChatCompletionsStreamResponseChoice{choice},
	}
	return &response, &zhipuResponse.Usage
}

// zhipuStreamHandler 处理智谱的流式聊天请求。
// 使用 SSE（Server-Sent Events）格式推送响应数据。
//
// 智谱的流式响应格式：
//   - "data:" 前缀：实际的响应内容块
//   - "meta:" 前缀：元数据（包含 token 使用量等汇总信息）
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继信息
//   - resp: 智谱 API 的 HTTP 响应
//
// 返回：token 使用量统计和可能的错误。
func zhipuStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NexusTokError) {
	var usage *dto.Usage
	scanner := bufio.NewScanner(resp.Body)
	scanner.Split(bufio.ScanLines)
	// 使用 channel 分离 data 和 meta 数据流
	dataChan := make(chan string)
	metaChan := make(chan string)
	stopChan := make(chan bool)
	// 启动 goroutine 解析 SSE 流
	go func() {
		for scanner.Scan() {
			data := scanner.Text()
			lines := strings.Split(data, "\n")
			for i, line := range lines {
				if len(line) < 5 {
					continue
				}
				// 根据前缀分发到不同的 channel
				if line[:5] == "data:" {
					dataChan <- line[5:]
					if i != len(lines)-1 {
						dataChan <- "\n"
					}
				} else if line[:5] == "meta:" {
					metaChan <- line[5:]
				}
			}
		}
		stopChan <- true
	}()
	// 设置 SSE 响应头
	helper.SetEventStreamHeaders(c)
	c.Stream(func(w io.Writer) bool {
		select {
		case data := <-dataChan:
			// 处理普通的流式数据
			response := streamResponseZhipu2OpenAI(data)
			jsonResponse, err := json.Marshal(response)
			if err != nil {
				common.SysLog("error marshalling stream response: " + err.Error())
				return true
			}
			c.Render(-1, common.CustomEvent{Data: "data: " + string(jsonResponse)})
			return true
		case data := <-metaChan:
			// 处理元数据（流式结束时的汇总信息）
			var zhipuResponse ZhipuStreamMetaResponse
			err := json.Unmarshal([]byte(data), &zhipuResponse)
			if err != nil {
				common.SysLog("error unmarshalling stream response: " + err.Error())
				return true
			}
			response, zhipuUsage := streamMetaResponseZhipu2OpenAI(&zhipuResponse)
			jsonResponse, err := json.Marshal(response)
			if err != nil {
				common.SysLog("error marshalling stream response: " + err.Error())
				return true
			}
			usage = zhipuUsage
			c.Render(-1, common.CustomEvent{Data: "data: " + string(jsonResponse)})
			return true
		case <-stopChan:
			// 发送 SSE 结束标记
			c.Render(-1, common.CustomEvent{Data: "data: [DONE]"})
			return false
		}
	})
	service.CloseResponseBodyGracefully(resp)
	return usage, nil
}

// zhipuHandler 处理智谱的非流式聊天请求。
// 读取完整的响应体，解析后转换为 OpenAI 格式返回。
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继信息
//   - resp: 智谱 API 的 HTTP 响应
//
// 返回：token 使用量统计和可能的错误。
func zhipuHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NexusTokError) {
	var zhipuResponse ZhipuResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	service.CloseResponseBodyGracefully(resp)
	err = json.Unmarshal(responseBody, &zhipuResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	// 检查智谱 API 返回的错误状态
	if !zhipuResponse.Success {
		return nil, types.WithOpenAIError(types.OpenAIError{
			Message: zhipuResponse.Msg,
			Code:    zhipuResponse.Code,
		}, resp.StatusCode)
	}
	fullTextResponse := responseZhipu2OpenAI(&zhipuResponse)
	jsonResponse, err := json.Marshal(fullTextResponse)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, err = c.Writer.Write(jsonResponse)
	return &fullTextResponse.Usage, nil
}
