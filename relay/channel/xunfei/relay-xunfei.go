// 讯飞星火大模型的核心中继处理逻辑。
// 负责 OpenAI 与讯飞格式之间的双向转换、WebSocket 连接管理、
// HMAC-SHA256 鉴权 URL 构建，以及流式/非流式响应处理。
package xunfei

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	// 项目内部依赖
	"github.com/c1cada/NexusTok/common"    // 通用工具函数（时间戳、日志等）
	"github.com/c1cada/NexusTok/constant"   // 常量定义（finish reason 等）
	"github.com/c1cada/NexusTok/dto"        // 数据传输对象
	"github.com/c1cada/NexusTok/relay/helper" // 中继辅助函数（SSE 头设置等）
	"github.com/c1cada/NexusTok/types"      // 错误类型
	"github.com/samber/lo"                  // 泛型工具库

	// 第三方依赖
	"github.com/gin-gonic/gin"        // HTTP 框架
	"github.com/gorilla/websocket"    // WebSocket 客户端
)

// 讯飞星火 API 文档参考：
// https://console.xfyun.cn/services/cbm
// https://www.xfyun.cn/doc/spark/Web.html

// requestOpenAI2Xunfei 将 OpenAI 格式的请求转换为讯飞星火格式。
//
// 特殊处理：
//   - 对于非 v3.5 版本的模型，system 消息会被转换为 user + assistant 的消息对，
//     因为早期版本不原生支持 system 角色。
//
// 参数：
//   - request: OpenAI 格式的聊天请求
//   - xunfeiAppId: 讯飞应用 ID
//   - domain: 模型领域标识（如 "generalv3"、"4.0Ultra"）
//
// 返回：讯飞格式的聊天请求指针。
func requestOpenAI2Xunfei(request dto.GeneralOpenAIRequest, xunfeiAppId string, domain string) *XunfeiChatRequest {
	messages := make([]XunfeiMessage, 0, len(request.Messages))
	// v3.5 版本原生支持 system 消息，其他版本需要转换
	shouldCovertSystemMessage := !strings.HasSuffix(request.Model, "3.5")
	for _, message := range request.Messages {
		if message.Role == "system" && shouldCovertSystemMessage {
			// 将 system 消息转换为 user 消息 + "Okay" 的 assistant 消息对
			messages = append(messages, XunfeiMessage{
				Role:    "user",
				Content: message.StringContent(),
			})
			messages = append(messages, XunfeiMessage{
				Role:    "assistant",
				Content: "Okay",
			})
		} else {
			messages = append(messages, XunfeiMessage{
				Role:    message.Role,
				Content: message.StringContent(),
			})
		}
	}
	xunfeiRequest := XunfeiChatRequest{}
	xunfeiRequest.Header.AppId = xunfeiAppId
	xunfeiRequest.Parameter.Chat.Domain = domain
	xunfeiRequest.Parameter.Chat.Temperature = request.Temperature
	xunfeiRequest.Parameter.Chat.TopK = lo.FromPtrOr(request.N, 0)
	xunfeiRequest.Parameter.Chat.MaxTokens = request.GetMaxTokens()
	xunfeiRequest.Payload.Message.Text = messages
	return &xunfeiRequest
}

// responseXunfei2OpenAI 将讯飞的非流式响应转换为 OpenAI 格式。
//
// 参数：
//   - response: 讯飞格式的响应
//
// 返回：OpenAI 格式的文本响应指针。
func responseXunfei2OpenAI(response *XunfeiChatResponse) *dto.OpenAITextResponse {
	// 如果响应中没有选项，填充一个空选项
	if len(response.Payload.Choices.Text) == 0 {
		response.Payload.Choices.Text = []XunfeiChatResponseTextItem{
			{
				Content: "",
			},
		}
	}
	choice := dto.OpenAITextResponseChoice{
		Index: 0,
		Message: dto.Message{
			Role:    "assistant",
			Content: response.Payload.Choices.Text[0].Content,
		},
		FinishReason: constant.FinishReasonStop,
	}
	fullTextResponse := dto.OpenAITextResponse{
		Object:  "chat.completion",
		Created: common.GetTimestamp(),
		Choices: []dto.OpenAITextResponseChoice{choice},
		Usage:   response.Payload.Usage.Text,
	}
	return &fullTextResponse
}

// streamResponseXunfei2OpenAI 将讯飞的流式响应转换为 OpenAI SSE 格式。
//
// 参数：
//   - xunfeiResponse: 讯飞格式的流式响应
//
// 返回：OpenAI 格式的流式响应 chunk。
func streamResponseXunfei2OpenAI(xunfeiResponse *XunfeiChatResponse) *dto.ChatCompletionsStreamResponse {
	if len(xunfeiResponse.Payload.Choices.Text) == 0 {
		xunfeiResponse.Payload.Choices.Text = []XunfeiChatResponseTextItem{
			{
				Content: "",
			},
		}
	}
	var choice dto.ChatCompletionsStreamResponseChoice
	choice.Delta.SetContentString(xunfeiResponse.Payload.Choices.Text[0].Content)
	// Status == 2 表示这是最后一条消息
	if xunfeiResponse.Payload.Choices.Status == 2 {
		choice.FinishReason = &constant.FinishReasonStop
	}
	response := dto.ChatCompletionsStreamResponse{
		Object:  "chat.completion.chunk",
		Created: common.GetTimestamp(),
		Model:   "SparkDesk",
		Choices: []dto.ChatCompletionsStreamResponseChoice{choice},
	}
	return &response
}

// buildXunfeiAuthUrl 构建讯飞星火 WebSocket 鉴权 URL。
//
// 鉴权流程：
//  1. 解析目标 URL，提取 host 和 path
//  2. 使用当前 UTC 时间生成签名字符串
//  3. 使用 HMAC-SHA256 和 apiSecret 对签名字符串进行签名
//  4. 将签名结果编码为 Base64，构造 Authorization 头
//  5. 将 host、date、authorization 作为查询参数附加到 URL
//
// 参数：
//   - hostUrl: 讯飞 WebSocket 服务的基础 URL
//   - apiKey: 讯飞 API Key
//   - apiSecret: 讯飞 API Secret
//
// 返回：带鉴权参数的完整 WebSocket URL。
func buildXunfeiAuthUrl(hostUrl string, apiKey, apiSecret string) string {
	// HMAC-SHA256 签名辅助函数
	HmacWithShaToBase64 := func(algorithm, data, key string) string {
		mac := hmac.New(sha256.New, []byte(key))
		mac.Write([]byte(data))
		encodeData := mac.Sum(nil)
		return base64.StdEncoding.EncodeToString(encodeData)
	}
	ul, err := url.Parse(hostUrl)
	if err != nil {
		fmt.Println(err)
	}
	// 使用 RFC1123 格式的 UTC 时间
	date := time.Now().UTC().Format(time.RFC1123)
	// 构造签名字符串：包含 host、date 和请求行
	signString := []string{"host: " + ul.Host, "date: " + date, "GET " + ul.Path + " HTTP/1.1"}
	sign := strings.Join(signString, "\n")
	sha := HmacWithShaToBase64("hmac-sha256", sign, apiSecret)
	// 构造 Authorization 头的值
	authUrl := fmt.Sprintf("hmac username=\"%s\", algorithm=\"%s\", headers=\"%s\", signature=\"%s\"", apiKey,
		"hmac-sha256", "host date request-line", sha)
	authorization := base64.StdEncoding.EncodeToString([]byte(authUrl))
	// 将鉴权信息作为查询参数
	v := url.Values{}
	v.Add("host", ul.Host)
	v.Add("date", date)
	v.Add("authorization", authorization)
	callUrl := hostUrl + "?" + v.Encode()
	return callUrl
}

// xunfeiStreamHandler 处理讯飞的流式聊天请求。
// 通过 WebSocket 发送请求，逐块接收响应并通过 SSE 推送给客户端。
//
// 参数：
//   - c: Gin 上下文
//   - textRequest: OpenAI 格式的聊天请求
//   - appId: 讯飞应用 ID
//   - apiSecret: 讯飞 API Secret
//   - apiKey: 讯飞 API Key
//
// 返回：token 使用量统计和可能的错误。
func xunfeiStreamHandler(c *gin.Context, textRequest dto.GeneralOpenAIRequest, appId string, apiSecret string, apiKey string) (*dto.Usage, *types.NexusTokError) {
	// 获取模型对应的领域标识和鉴权 URL
	domain, authUrl := getXunfeiAuthUrl(c, apiKey, apiSecret, textRequest.Model)
	// 建立 WebSocket 连接并发送请求
	dataChan, stopChan, err := xunfeiMakeRequest(textRequest, domain, authUrl, appId)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeDoRequestFailed)
	}
	// 设置 SSE 响应头
	helper.SetEventStreamHeaders(c)
	var usage dto.Usage
	// 使用 Gin 的 Stream 方法以 SSE 格式推送数据
	c.Stream(func(w io.Writer) bool {
		select {
		case xunfeiResponse := <-dataChan:
			// 累加 token 使用量
			usage.PromptTokens += xunfeiResponse.Payload.Usage.Text.PromptTokens
			usage.CompletionTokens += xunfeiResponse.Payload.Usage.Text.CompletionTokens
			usage.TotalTokens += xunfeiResponse.Payload.Usage.Text.TotalTokens
			// 将讯飞响应转换为 OpenAI 流式格式
			response := streamResponseXunfei2OpenAI(&xunfeiResponse)
			jsonResponse, err := json.Marshal(response)
			if err != nil {
				common.SysLog("error marshalling stream response: " + err.Error())
				return true
			}
			c.Render(-1, common.CustomEvent{Data: "data: " + string(jsonResponse)})
			return true
		case <-stopChan:
			// 发送 SSE 结束标记
			c.Render(-1, common.CustomEvent{Data: "data: [DONE]"})
			return false
		}
	})
	return &usage, nil
}

// xunfeiHandler 处理讯飞的非流式聊天请求。
// 通过 WebSocket 发送请求，收集所有响应块后合并为完整响应返回。
//
// 参数：
//   - c: Gin 上下文
//   - textRequest: OpenAI 格式的聊天请求
//   - appId: 讯飞应用 ID
//   - apiSecret: 讯飞 API Secret
//   - apiKey: 讯飞 API Key
//
// 返回：token 使用量统计和可能的错误。
func xunfeiHandler(c *gin.Context, textRequest dto.GeneralOpenAIRequest, appId string, apiSecret string, apiKey string) (*dto.Usage, *types.NexusTokError) {
	domain, authUrl := getXunfeiAuthUrl(c, apiKey, apiSecret, textRequest.Model)
	dataChan, stopChan, err := xunfeiMakeRequest(textRequest, domain, authUrl, appId)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeDoRequestFailed)
	}
	var usage dto.Usage
	var content string
	var xunfeiResponse XunfeiChatResponse
	stop := false
	// 循环接收所有 WebSocket 消息，直到收到停止信号
	for !stop {
		select {
		case xunfeiResponse = <-dataChan:
			if len(xunfeiResponse.Payload.Choices.Text) == 0 {
				continue
			}
			// 拼接所有分片内容
			content += xunfeiResponse.Payload.Choices.Text[0].Content
			// 累加 token 使用量
			usage.PromptTokens += xunfeiResponse.Payload.Usage.Text.PromptTokens
			usage.CompletionTokens += xunfeiResponse.Payload.Usage.Text.CompletionTokens
			usage.TotalTokens += xunfeiResponse.Payload.Usage.Text.TotalTokens
		case stop = <-stopChan:
		}
	}
	if len(xunfeiResponse.Payload.Choices.Text) == 0 {
		xunfeiResponse.Payload.Choices.Text = []XunfeiChatResponseTextItem{
			{
				Content: "",
			},
		}
	}
	// 将合并后的内容写入响应
	xunfeiResponse.Payload.Choices.Text[0].Content = content

	response := responseXunfei2OpenAI(&xunfeiResponse)
	jsonResponse, err := json.Marshal(response)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	_, _ = c.Writer.Write(jsonResponse)
	return &usage, nil
}

// xunfeiMakeRequest 建立 WebSocket 连接并发送讯飞请求。
// 启动一个 goroutine 持续读取 WebSocket 消息并通过 channel 返回。
//
// 参数：
//   - textRequest: OpenAI 格式的请求
//   - domain: 模型领域标识
//   - authUrl: 已鉴权的 WebSocket URL
//   - appId: 讯飞应用 ID
//
// 返回：
//   - dataChan: 接收讯飞响应的 channel
//   - stopChan: 接收完成信号的 channel
//   - error: 建立连接或发送请求时的错误
func xunfeiMakeRequest(textRequest dto.GeneralOpenAIRequest, domain, authUrl, appId string) (chan XunfeiChatResponse, chan bool, error) {
	// 建立 WebSocket 连接，5 秒握手超时
	d := websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}
	conn, resp, err := d.Dial(authUrl, nil)
	if err != nil || resp.StatusCode != 101 {
		return nil, nil, err
	}

	// 将 OpenAI 请求转换为讯飞格式并发送
	data := requestOpenAI2Xunfei(textRequest, appId, domain)
	err = conn.WriteJSON(data)
	if err != nil {
		return nil, nil, err
	}

	dataChan := make(chan XunfeiChatResponse)
	stopChan := make(chan bool)
	// 启动后台 goroutine 持续读取 WebSocket 消息
	go func() {
		defer func() {
			conn.Close()
		}()
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				common.SysLog("error reading stream response: " + err.Error())
				break
			}
			var response XunfeiChatResponse
			err = json.Unmarshal(msg, &response)
			if err != nil {
				common.SysLog("error unmarshalling stream response: " + err.Error())
				break
			}
			dataChan <- response
			// Status == 2 表示最后一条消息，结束循环
			if response.Payload.Choices.Status == 2 {
				if err != nil {
					common.SysLog("error closing websocket connection: " + err.Error())
				}
				break
			}
		}
		stopChan <- true
	}()

	return dataChan, stopChan, nil
}

// apiVersion2domain 将 API 版本号转换为讯飞模型领域标识。
//
// 版本与领域映射关系：
//   - v1.1 -> "lite"（轻量版）
//   - v2.1 -> "generalv2"（通用 v2）
//   - v3.1 -> "generalv3"（通用 v3）
//   - v3.5 -> "generalv3.5"（通用 v3.5）
//   - v4.0 -> "4.0Ultra"（Ultra 版）
//   - 其他  -> "general" + 版本号
func apiVersion2domain(apiVersion string) string {
	switch apiVersion {
	case "v1.1":
		return "lite"
	case "v2.1":
		return "generalv2"
	case "v3.1":
		return "generalv3"
	case "v3.5":
		return "generalv3.5"
	case "v4.0":
		return "4.0Ultra"
	}
	return "general" + apiVersion
}

// getXunfeiAuthUrl 获取讯飞的鉴权 WebSocket URL 和模型领域标识。
//
// 参数：
//   - c: Gin 上下文
//   - apiKey: 讯飞 API Key
//   - apiSecret: 讯飞 API Secret
//   - modelName: 模型名称
//
// 返回：领域标识和鉴权后的 WebSocket URL。
func getXunfeiAuthUrl(c *gin.Context, apiKey string, apiSecret string, modelName string) (string, string) {
	apiVersion := getAPIVersion(c, modelName)
	domain := apiVersion2domain(apiVersion)
	authUrl := buildXunfeiAuthUrl(fmt.Sprintf("wss://spark-api.xf-yun.com/%s/chat", apiVersion), apiKey, apiSecret)
	return domain, authUrl
}

// getAPIVersion 获取讯飞 API 版本号。
// 按优先级依次尝试以下来源：
//  1. URL 查询参数 "api-version"
//  2. 模型名称中提取（如 "SparkDesk-v3.1" -> "v3.1"）
//  3. Gin 上下文中设置的 "api_version"
//  4. 默认值 "v1.1"
func getAPIVersion(c *gin.Context, modelName string) string {
	// 优先从 URL 查询参数获取
	query := c.Request.URL.Query()
	apiVersion := query.Get("api-version")
	if apiVersion != "" {
		return apiVersion
	}
	// 从模型名称中提取版本号（格式：SparkDesk-vX.X）
	parts := strings.Split(modelName, "-")
	if len(parts) == 2 {
		apiVersion = parts[1]
		return apiVersion

	}
	// 从上下文获取
	apiVersion = c.GetString("api_version")
	if apiVersion != "" {
		return apiVersion
	}
	// 使用默认版本
	apiVersion = "v1.1"
	common.SysLog("api_version not found, using default: " + apiVersion)
	return apiVersion
}
