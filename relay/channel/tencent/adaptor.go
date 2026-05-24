// Package tencent 实现了腾讯云混元大模型的渠道适配器。
// 负责将 OpenAI 格式的请求转换为腾讯云 API 格式，
// 处理 TC3-HMAC-SHA256 签名认证、流式和非流式响应转换等。
// 参考文档：https://cloud.tencent.com/document/product/1729/97732
package tencent

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	// 项目内部依赖
	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/relay/channel"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/types"

	// 第三方依赖
	"github.com/gin-gonic/gin"
)

// Adaptor 是腾讯云混元渠道的适配器结构体。
// 包含签名所需的元数据信息：签名串、应用 ID、操作类型、API 版本和时间戳。
type Adaptor struct {
	Sign      string // TC3-HMAC-SHA256 签名结果
	AppID     int64  // 腾讯云应用 ID
	Action    string // API 操作类型，如 "ChatCompletions"
	Version   string // API 版本号
	Timestamp int64  // 请求时间戳（秒）
}

// ConvertGeminiRequest 未实现，腾讯云渠道不支持 Gemini 格式请求。
func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertClaudeRequest 未实现，腾讯云渠道不支持 Claude 格式请求。
func (a *Adaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertAudioRequest 未实现，腾讯云渠道不支持音频请求。
func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertImageRequest 未实现，腾讯云渠道不支持图片生成请求。
func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// Init 初始化腾讯云适配器。
// 设置 API 操作类型为 "ChatCompletions"、版本为 "2023-09-01"，并记录当前时间戳。
// 参数:
//   - info: 中继请求的上下文信息
func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
	a.Action = "ChatCompletions"
	a.Version = "2023-09-01"
	a.Timestamp = common.GetTimestamp()
}

// GetRequestURL 构建腾讯云 API 的请求 URL。
// 腾讯云 API 采用统一端点，URL 为基础 URL 加上 "/"。
// 参数:
//   - info: 包含基础 URL 的中继信息
//
// 返回:
//   - string: 完整的请求 URL
//   - error: 始终返回 nil
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return fmt.Sprintf("%s/", info.ChannelBaseUrl), nil
}

// SetupRequestHeader 设置腾讯云 API 请求头。
// 包括通用 API 请求头、TC3 签名认证（Authorization）、操作类型（X-TC-Action）、
// API 版本（X-TC-Version）和时间戳（X-TC-Timestamp）。
// 参数:
//   - c: Gin 上下文
//   - req: HTTP 请求头指针
//   - info: 中继信息
//
// 返回:
//   - error: 始终返回 nil
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", a.Sign)
	req.Set("X-TC-Action", a.Action)
	req.Set("X-TC-Version", a.Version)
	req.Set("X-TC-Timestamp", strconv.FormatInt(a.Timestamp, 10))
	return nil
}

// ConvertOpenAIRequest 将 OpenAI 格式请求转换为腾讯云混元 API 格式。
// 流程：解析腾讯云配置（AppID|SecretId|SecretKey）-> 转换请求格式 -> 计算 TC3 签名。
// 参数:
//   - c: Gin 上下文，用于获取渠道密钥
//   - info: 中继信息
//   - request: OpenAI 格式的通用请求
//
// 返回:
//   - any: 转换后的腾讯云请求体
//   - error: 配置解析或签名计算失败时返回错误
func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	apiKey := common.GetContextKeyString(c, constant.ContextKeyChannelKey)
	apiKey = strings.TrimPrefix(apiKey, "Bearer ")
	appId, secretId, secretKey, err := parseTencentConfig(apiKey)
	a.AppID = appId
	if err != nil {
		return nil, err
	}
	tencentRequest := requestOpenAI2Tencent(a, *request)
	// we have to calculate the sign here
	a.Sign = getTencentSign(*tencentRequest, a, secretId, secretKey)
	return tencentRequest, nil
}

// ConvertRerankRequest 转换 Rerank 请求（当前返回 nil）。
func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

// ConvertEmbeddingRequest 未实现，腾讯云渠道暂不支持 Embedding 请求。
func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertOpenAIResponsesRequest 未实现，腾讯云渠道暂不支持 OpenAI Responses 请求。
func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	// TODO implement me
	return nil, errors.New("not implemented")
}

// DoRequest 执行实际的 HTTP API 请求，委托给通用的 DoApiRequest 方法。
func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

// DoResponse 处理腾讯云 API 的响应。
// 根据是否为流式请求，分别调用 tencentStreamHandler 或 tencentHandler。
// 参数:
//   - c: Gin 上下文
//   - resp: 上游 HTTP 响应
//   - info: 中继信息
//
// 返回:
//   - usage: token 使用量
//   - err: 处理过程中的错误信息
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NexusTokError) {
	if info.IsStream {
		usage, err = tencentStreamHandler(c, info, resp)
	} else {
		usage, err = tencentHandler(c, info, resp)
	}
	return
}

// GetModelList 返回腾讯云混元渠道支持的模型列表。
// 返回:
//   - []string: 模型名称切片
func (a *Adaptor) GetModelList() []string {
	return ModelList
}

// GetChannelName 返回渠道名称标识 "tencent"。
// 返回:
//   - string: 渠道名称
func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
