// coze - adaptor.go
// 本文件实现了 Coze 渠道的适配器（Adaptor），负责将统一的请求格式转换为 Coze API 所需的格式，
// 以及将 Coze 的响应转换回统一格式。Coze 是字节跳动旗下的 AI Bot 构建平台，
// 通过其 API 可以访问多种大语言模型。
// 适配器实现了 channel.Adaptor 接口，支持 OpenAI 格式的请求转换、流式和非流式响应处理。
package coze

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/relay/channel"
	"github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/types"

	"github.com/gin-gonic/gin"
)

// Adaptor 是 Coze 渠道的适配器结构体。
// 实现了 channel.Adaptor 接口，负责 Coze API 的请求转换和响应处理。
// Coze 的 API 流程较为特殊：非流式请求需要先创建聊天，然后轮询状态，最后获取消息详情。
type Adaptor struct {
}

// ConvertGeminiRequest 将 Gemini 格式请求转换为 Coze 格式。
// 当前未实现，Coze 渠道不支持 Gemini 格式的请求转换。
// 参数：
//   - c: Gin 上下文
//   - info: 中继请求信息
//   - request: Gemini 格式的聊天请求
//
// 返回值：始终返回错误，表示该方法未实现。
func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *common.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertAudioRequest 将音频请求转换为 Coze 格式。
// 当前未实现，Coze 渠道不支持音频请求转换。
// 参数：
//   - c: Gin 上下文
//   - info: 中继请求信息
//   - request: 音频请求
//
// 返回值：始终返回错误，表示该方法未实现。
func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *common.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, errors.New("not implemented")
}

// ConvertClaudeRequest 将 Claude 格式请求转换为 Coze 格式。
// 当前未实现，Coze 渠道不支持 Claude 格式的请求转换。
// 参数：
//   - c: Gin 上下文
//   - info: 中继请求信息
//   - request: Claude 格式的聊天请求
//
// 返回值：始终返回错误，表示该方法未实现。
func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *common.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	return nil, errors.New("not implemented")
}

// ConvertEmbeddingRequest 将 Embedding 请求转换为 Coze 格式。
// 当前未实现，Coze 渠道不支持 Embedding 请求转换。
// 参数：
//   - c: Gin 上下文
//   - info: 中继请求信息
//   - request: Embedding 请求
//
// 返回值：始终返回错误，表示该方法未实现。
func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *common.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return nil, errors.New("not implemented")
}

// ConvertImageRequest 将图片生成请求转换为 Coze 格式。
// 当前未实现，Coze 渠道不支持图片生成请求转换。
// 参数：
//   - c: Gin 上下文
//   - info: 中继请求信息
//   - request: 图片生成请求
//
// 返回值：始终返回错误，表示该方法未实现。
func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *common.RelayInfo, request dto.ImageRequest) (any, error) {
	return nil, errors.New("not implemented")
}

// ConvertOpenAIRequest 将 OpenAI 格式的聊天请求转换为 Coze 格式。
// 这是 Coze 渠道的核心请求转换方法，将通用的 OpenAI 请求格式转换为 Coze API 所需的请求结构。
// 转换过程由 convertCozeChatRequest 函数完成。
// 参数：
//   - c: Gin 上下文（包含 bot_id 等渠道特定信息）
//   - info: 中继请求信息
//   - request: OpenAI 格式的通用聊天请求
//
// 返回值：转换后的 CozeChatRequest 指针，或请求为空时返回错误。
func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *common.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	return convertCozeChatRequest(c, *request), nil
}

// ConvertOpenAIResponsesRequest 将 OpenAI Responses 格式请求转换为 Coze 格式。
// 当前未实现，Coze 渠道不支持 OpenAI Responses 格式的请求转换。
// 参数：
//   - c: Gin 上下文
//   - info: 中继请求信息
//   - request: OpenAI Responses 格式请求
//
// 返回值：始终返回错误，表示该方法未实现。
func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *common.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return nil, errors.New("not implemented")
}

// ConvertRerankRequest 将 Rerank 请求转换为 Coze 格式。
// 当前未实现，Coze 渠道不支持 Rerank 请求转换。
// 参数：
//   - c: Gin 上下文
//   - relayMode: 中继模式
//   - request: Rerank 请求
//
// 返回值：始终返回错误，表示该方法未实现。
func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, errors.New("not implemented")
}

// DoRequest 执行向 Coze API 发送请求的操作。
// Coze API 的请求流程因是否为流式模式而不同：
//   - 流式模式：直接发送请求并返回响应流
//   - 非流式模式：采用三步流程：
//  1. 发送创建聊天请求（/v3/chat）
//  2. 轮询检查聊天是否完成（/v3/chat/retrieve），每秒检查一次
//  3. 获取聊天消息详情（/v3/chat/message/list）
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继请求信息
//   - requestBody: 请求体的 io.Reader
//
// 返回值：上游 API 的响应（any 类型）和可能的错误。
func (a *Adaptor) DoRequest(c *gin.Context, info *common.RelayInfo, requestBody io.Reader) (any, error) {
	if info.IsStream {
		return channel.DoApiRequest(a, c, info, requestBody)
	}
	// 首先发送创建消息请求，成功后再发送获取消息请求
	// 发送创建消息请求
	resp, err := channel.DoApiRequest(a, c, info, requestBody)
	if err != nil {
		return nil, err
	}
	// 解析 resp
	var cozeResponse CozeChatResponse
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(respBody, &cozeResponse)
	if cozeResponse.Code != 0 {
		return nil, errors.New(cozeResponse.Msg)
	}
	// 将会话 ID 和聊天 ID 保存到上下文中，供后续轮询和获取详情使用
	c.Set("coze_conversation_id", cozeResponse.Data.ConversationId)
	c.Set("coze_chat_id", cozeResponse.Data.Id)
	// 轮询检查消息是否完成
	for {
		err, isComplete := checkIfChatComplete(a, c, info)
		if err != nil {
			return nil, err
		} else {
			if isComplete {
				break
			}
		}
		time.Sleep(time.Second * 1)
	}
	// 发送获取消息请求
	return getChatDetail(a, c, info)
}

// DoResponse 处理 Coze API 的响应，根据是否为流式模式分别调用不同的处理函数。
// 流式模式调用 cozeChatStreamHandler，非流式模式调用 cozeChatHandler。
// 参数：
//   - c: Gin 上下文
//   - resp: 上游 Coze API 的 HTTP 响应
//   - info: 中继请求信息
//
// 返回值：
//   - usage: token 用量信息（any 类型）
//   - err: 可能的 NexusTok 错误
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *common.RelayInfo) (usage any, err *types.NexusTokError) {
	if info.IsStream {
		usage, err = cozeChatStreamHandler(c, info, resp)
	} else {
		usage, err = cozeChatHandler(c, info, resp)
	}
	return
}

// GetChannelName 返回渠道名称标识符。
// 返回值：渠道名称字符串 "coze"。
func (a *Adaptor) GetChannelName() string {
	return ChannelName
}

// GetModelList 返回 Coze 渠道支持的模型列表。
// 返回值：包含所有支持模型名称的字符串切片。
func (a *Adaptor) GetModelList() []string {
	return ModelList
}

// GetRequestURL 构建 Coze API 的请求 URL。
// Coze 的聊天 API 端点为 /v3/chat。
// 参数：
//   - info: 中继请求信息（包含渠道的基础 URL）
//
// 返回值：完整的请求 URL 字符串和可能的错误。
func (a *Adaptor) GetRequestURL(info *common.RelayInfo) (string, error) {
	return fmt.Sprintf("%s/v3/chat", info.ChannelBaseUrl), nil
}

// Init 初始化 Coze 适配器。
// 当前为空实现，Coze 渠道不需要额外的初始化操作。
// 参数：
//   - info: 中继请求信息
func (a *Adaptor) Init(info *common.RelayInfo) {

}

// SetupRequestHeader 设置发送到 Coze API 的 HTTP 请求头。
// 设置通用的 API 请求头，并添加 Bearer Token 认证头。
// 参数：
//   - c: Gin 上下文
//   - req: 要设置的 HTTP 请求头指针
//   - info: 中继请求信息（包含 API Key）
//
// 返回值：始终返回 nil。
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *common.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", "Bearer "+info.ApiKey)
	return nil
}
