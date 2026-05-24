// aws - adaptor.go
// AWS Bedrock 渠道适配器实现。
// 本文件定义了 AWS Bedrock 渠道的核心适配器结构体 Adaptor 及其实现的所有接口方法，
// 负责将上游（OpenAI / Claude / Gemini 等）请求格式转换为 AWS Bedrock 所需的格式，
// 并根据客户端认证模式（API Key 或 AKSK）分发请求和处理响应。
// 支持 Amazon Nova 系列模型以及 Claude 系列模型的请求转发与响应解析。
package aws

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/relay/channel"
	"github.com/c1cada/NexusTok/relay/channel/claude"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/service"
	"github.com/c1cada/NexusTok/types"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/pkg/errors"

	"github.com/gin-gonic/gin"
)

// ClientMode 定义 AWS 客户端的认证模式类型。
type ClientMode int

const (
	// ClientModeApiKey 表示使用 API Key 方式进行认证（Bearer Token）。
	// API Key 格式为 "<api-key>|<region>"。
	ClientModeApiKey ClientMode = iota + 1

	// ClientModeAKSK 表示使用 AWS Access Key / Secret Key 方式进行认证（AWS Signature V4）。
	// AKSK 格式为 "<access-key>|<secret-key>|<region>"。
	ClientModeAKSK
)

// Adaptor 是 AWS Bedrock 渠道的适配器结构体。
// 实现了 channel.Adaptor 接口，负责请求格式转换、HTTP 请求构建、
// 认证头设置以及响应的解析和转发。
type Adaptor struct {
	// ClientMode 当前使用的客户端认证模式（API Key 或 AKSK）。
	ClientMode ClientMode
	// AwsClient AWS Bedrock Runtime SDK 客户端实例，用于通过 AKSK 方式发起请求。
	AwsClient *bedrockruntime.Client
	// AwsModelId 当前请求对应的 AWS Bedrock 模型 ID。
	AwsModelId string
	// AwsReq 构建好的 AWS 请求体，可以是 InvokeModelInput 或 InvokeModelWithResponseStreamInput。
	AwsReq any
	// IsNova 标识当前请求是否为 Amazon Nova 模型请求。
	// Nova 模型使用不同的请求/响应格式，需要特殊处理。
	IsNova bool
}

// ConvertGeminiRequest 将 Gemini 格式的聊天请求转换为 AWS Bedrock 格式。
// 当前尚未实现，调用将返回 "not implemented" 错误。
// 参数:
//   - c: Gin 上下文
//   - info: 中继请求的元数据信息
//   - request: Gemini 格式的聊天请求
//
// 返回: 转换后的请求体和错误信息。
func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertClaudeRequest 将 Claude 格式的请求转换为 AWS Bedrock 兼容的 Claude 请求。
// 主要处理逻辑:
//   - 遍历请求中的所有消息，检查消息内容是否包含 URL 类型的图片源
//   - 对于 URL 类型的图片源，通过统一文件服务将图片下载并转换为 base64 编码
//   - 将原始的 URL 类型替换为 base64 类型（AWS Bedrock 不支持直接使用 URL 图片源）
//
// 参数:
//   - c: Gin 上下文
//   - info: 中继请求的元数据信息
//   - request: Claude 格式的请求体
//
// 返回: 转换后的请求体（可能包含图片源替换）和错误信息。
func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	for i, message := range request.Messages {
		updated := false
		if !message.IsStringContent() {
			content, err := message.ParseContent()
			if err != nil {
				return nil, errors.Wrap(err, "failed to parse message content")
			}
			for i2, mediaMessage := range content {
				if mediaMessage.Source != nil {
					if mediaMessage.Source.Type == "url" {
						// 使用统一的文件服务获取图片数据
						source := types.NewURLFileSource(mediaMessage.Source.Url)
						base64Data, mimeType, err := service.GetBase64Data(c, source, "formatting image for Claude")
						if err != nil {
							return nil, fmt.Errorf("get file base64 from url failed: %s", err.Error())
						}
						mediaMessage.Source.MediaType = mimeType
						mediaMessage.Source.Data = base64Data
						mediaMessage.Source.Url = ""
						mediaMessage.Source.Type = "base64"
						content[i2] = mediaMessage
						updated = true
					}
				}
			}
			if updated {
				message.SetContent(content)
			}
		}
		if updated {
			request.Messages[i] = message
		}
	}
	return request, nil
}

// ConvertAudioRequest 将音频请求转换为 AWS Bedrock 格式。
// 当前尚未实现，调用将返回 "not implemented" 错误。
// 参数:
//   - c: Gin 上下文
//   - info: 中继请求的元数据信息
//   - request: 音频请求体
//
// 返回: 转换后的 io.Reader 请求体和错误信息。
func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertImageRequest 将图片生成请求转换为 AWS Bedrock 格式。
// 当前尚未实现，调用将返回 "not implemented" 错误。
// 参数:
//   - c: Gin 上下文
//   - info: 中继请求的元数据信息
//   - request: 图片请求体
//
// 返回: 转换后的请求体和错误信息。
func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// Init 在请求处理前对适配器进行初始化。
// 当前 AWS 适配器无需特殊的初始化逻辑，为空实现。
// 参数:
//   - info: 中继请求的元数据信息。
func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

// GetRequestURL 根据认证模式构建 AWS Bedrock 的请求 URL。
// 认证模式由 ChannelOtherSettings.AwsKeyType 决定:
//   - API Key 模式: 构造 HTTPS URL 直接访问 Bedrock Runtime 的 converse 端点，
//     格式为 "https://bedrock-runtime.<region>.amazonaws.com/model/<modelId>/converse"。
//     API Key 格式必须为 "<api-key>|<region>"。
//   - AKSK 模式: 不需要构造 URL，直接通过 AWS SDK 客户端发起请求，返回空字符串。
//
// 参数:
//   - info: 中继请求的元数据信息，包含 API Key 和渠道配置。
//
// 返回: 请求 URL 字符串和错误信息。
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info.ChannelOtherSettings.AwsKeyType == dto.AwsKeyTypeApiKey {
		awsModelId := getAwsModelID(info.UpstreamModelName)
		a.ClientMode = ClientModeApiKey
		awsSecret := strings.Split(info.ApiKey, "|")
		if len(awsSecret) != 2 {
			return "", errors.New("invalid aws api key, should be in format of <api-key>|<region>")
		}
		return fmt.Sprintf("https://bedrock-runtime.%s.amazonaws.com/model/%s/converse", awsModelId, awsSecret[1]), nil
	} else {
		a.ClientMode = ClientModeAKSK
		return "", nil
	}
}

// SetupRequestHeader 设置发往 AWS Bedrock 的 HTTP 请求头。
// 首先调用 Claude 通用的请求头处理函数（如 anthropic-beta 等头），
// 然后在 API Key 模式下额外添加 Bearer 认证头。
// AKSK 模式下认证由 AWS SDK 自动处理，无需手动设置认证头。
//
// 参数:
//   - c: Gin 上下文
//   - req: 待设置的 HTTP 请求头指针
//   - info: 中继请求的元数据信息
//
// 返回: 错误信息（当前始终返回 nil）。
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	claude.CommonClaudeHeadersOperation(c, req, info)
	if a.ClientMode == ClientModeApiKey {
		req.Set("Authorization", "Bearer "+info.ApiKey)
	}
	return nil
}

// ConvertOpenAIRequest 将 OpenAI 格式的请求转换为 AWS Bedrock 支持的格式。
// 处理逻辑:
//   - 如果请求体为 nil，直接返回错误
//   - 判断目标模型是否为 Amazon Nova 模型:
//   - 如果是 Nova 模型: 调用 convertToNovaRequest 将 OpenAI 格式转换为 Nova 请求格式，
//     并设置 IsNova 标记为 true
//   - 如果不是 Nova 模型: 调用 Claude 的转换函数将 OpenAI 格式转换为 Claude 格式，
//     并更新上游模型名称为 Claude 模型名称
//
// 参数:
//   - c: Gin 上下文
//   - info: 中继请求的元数据信息
//   - request: OpenAI 格式的通用请求体
//
// 返回: 转换后的请求体和错误信息。
func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	// 检查是否为Nova模型
	if isNovaModel(request.Model) {
		novaReq := convertToNovaRequest(request)
		a.IsNova = true
		return novaReq, nil
	}

	// 原有的Claude模型处理逻辑
	claudeReq, err := claude.RequestOpenAI2ClaudeMessage(c, *request)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert openai request to claude request")
	}
	info.UpstreamModelName = claudeReq.Model
	return claudeReq, err
}

// ConvertRerankRequest 将重排序请求转换为 AWS Bedrock 格式。
// 当前为空实现，AWS Bedrock 渠道暂不支持重排序功能。
//
// 参数:
//   - c: Gin 上下文
//   - relayMode: 中继模式
//   - request: 重排序请求体
//
// 返回: nil 和 nil（不做任何转换）。
func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

// ConvertEmbeddingRequest 将文本嵌入请求转换为 AWS Bedrock 格式。
// 当前尚未实现，调用将返回 "not implemented" 错误。
//
// 参数:
//   - c: Gin 上下文
//   - info: 中继请求的元数据信息
//   - request: 嵌入请求体
//
// 返回: 转换后的请求体和错误信息。
func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertOpenAIResponsesRequest 将 OpenAI Responses 格式的请求转换为 AWS Bedrock 格式。
// 当前尚未实现，调用将返回 "not implemented" 错误。
//
// 参数:
//   - c: Gin 上下文
//   - info: 中继请求的元数据信息
//   - request: OpenAI Responses 格式的请求体
//
// 返回: 转换后的请求体和错误信息。
func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	// TODO implement me
	return nil, errors.New("not implemented")
}

// DoRequest 根据当前认证模式发起实际的 HTTP 请求。
// 分发逻辑:
//   - API Key 模式: 使用通用的 channel.DoApiRequest 发起标准 HTTP 请求，
//     通过 URL 和 Bearer Token 进行认证
//   - AKSK 模式: 使用 doAwsClientRequest 通过 AWS SDK 客户端发起请求，
//     由 AWS SDK 自动处理签名认证
//
// 参数:
//   - c: Gin 上下文
//   - info: 中继请求的元数据信息
//   - requestBody: 请求体的 io.Reader
//
// 返回: 原始响应和错误信息。
func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	if a.ClientMode == ClientModeApiKey {
		return channel.DoApiRequest(a, c, info, requestBody)
	} else {
		return doAwsClientRequest(c, info, a, requestBody)
	}
}

// DoResponse 处理 AWS Bedrock 返回的响应，并将结果写入客户端。
// 分发逻辑:
//   - API Key 模式: 委托给 Claude 适配器处理（Bedrock API Key 模式的响应格式与 Claude 原生格式一致）
//   - AKSK 模式:
//   - Nova 模型: 调用 handleNovaRequest 解析 Nova 格式响应并转换为 OpenAI 格式
//   - Claude 模型流式请求: 调用 awsStreamHandler 处理流式响应
//   - Claude 模型非流式请求: 调用 awsHandler 处理普通响应
//
// 参数:
//   - c: Gin 上下文
//   - resp: 上游返回的 HTTP 响应（API Key 模式使用，AKSK 模式下可能为 nil）
//   - info: 中继请求的元数据信息
//
// 返回: Token 用量信息和 NexusTok 错误信息。
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NexusTokError) {
	if a.ClientMode == ClientModeApiKey {
		claudeAdaptor := claude.Adaptor{}
		usage, err = claudeAdaptor.DoResponse(c, resp, info)
	} else {
		if a.IsNova {
			err, usage = handleNovaRequest(c, info, a)
		} else {
			if info.IsStream {
				err, usage = awsStreamHandler(c, info, a)
			} else {
				err, usage = awsHandler(c, info, a)
			}
		}
	}
	return
}

// GetModelList 返回当前 AWS 渠道支持的所有模型名称列表。
// 遍历 awsModelIDMap 中的所有键（即模型名称），组成列表返回。
//
// 返回: 支持的模型名称字符串切片。
func (a *Adaptor) GetModelList() (models []string) {
	for n := range awsModelIDMap {
		models = append(models, n)
	}

	return
}

// GetChannelName 返回当前渠道的名称标识。
// 返回值为常量 ChannelName（即 "aws"），用于在路由分发和日志中标识渠道类型。
//
// 返回: 渠道名称字符串。
func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
