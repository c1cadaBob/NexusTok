// aws - relay-aws.go
// AWS Bedrock 渠道的核心中继处理逻辑实现。
// 本文件包含与 AWS Bedrock API 交互的底层函数，包括:
//   - AWS SDK 客户端的创建和初始化（支持 API Key 和 AKSK 两种认证方式）
//   - 请求的构建和发送（通过 InvokeModel / InvokeModelWithResponseStream API）
//   - 响应的解析和转换（包括 Claude 模型和 Nova 模型的非流式/流式响应处理）
//   - 跨区域推理支持（Cross-Region Inference）
//   - 请求透传（Pass-Through）功能支持
package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/relay/channel"
	"github.com/c1cada/NexusTok/relay/channel/claude"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/relay/helper"
	"github.com/c1cada/NexusTok/service"
	"github.com/c1cada/NexusTok/types"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"

	"github.com/c1cada/NexusTok/setting/model_setting"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	bedrockruntimeTypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/aws/smithy-go/auth/bearer"
)

// getAwsErrorStatusCode 从 AWS SDK 错误中提取 HTTP 状态码。
// AWS SDK 的错误可能实现了 HTTPStatusCode() int 接口，通过类型断言尝试提取。
// 如果无法确定状态码，默认返回 500 (Internal Server Error)。
//
// 参数:
//   - err: AWS SDK 返回的错误
//
// 返回: HTTP 状态码整数值。
func getAwsErrorStatusCode(err error) int {
	// Check for HTTP response error which contains status code
	var httpErr interface{ HTTPStatusCode() int }
	if errors.As(err, &httpErr) {
		return httpErr.HTTPStatusCode()
	}
	// Default to 500 if we can't determine the status code
	return http.StatusInternalServerError
}

// newAwsInvokeContext 创建 AWS Bedrock API 调用的上下文。
// 如果全局配置的 RelayTimeout 大于 0，则创建带超时的上下文；
// 否则返回无超时的 Background 上下文。
// 超时时间由 common.RelayTimeout 配置（单位: 秒）。
//
// 返回: 带超时的上下文和取消函数。
func newAwsInvokeContext() (context.Context, context.CancelFunc) {
	if common.RelayTimeout <= 0 {
		return context.Background(), func() {}
	}
	return context.WithTimeout(context.Background(), time.Duration(common.RelayTimeout)*time.Second)
}

// newAwsClient 创建并初始化 AWS Bedrock Runtime SDK 客户端。
// 根据 info.ApiKey 的格式决定认证方式:
//   - 格式 "<api-key>|<region>" (2 段): 使用 Bearer Token 认证（API Key 模式）。
//     api-key 作为 Bearer Token，region 作为 AWS 区域。
//   - 格式 "<access-key>|<secret-key>|<region>" (3 段): 使用 AWS AKSK 认证（AKSK 模式）。
//     access-key 和 secret-key 作为 AWS 静态凭证，region 作为 AWS 区域。
//   - 其他格式: 返回 "invalid aws secret key" 错误。
//
// 如果渠道配置了代理（Proxy），则通过代理创建 HTTP 客户端；否则使用默认 HTTP 客户端。
//
// 参数:
//   - c: Gin 上下文
//   - info: 中继请求的元数据信息，包含 API Key 和渠道配置
//
// 返回: AWS Bedrock Runtime 客户端和错误信息。
func newAwsClient(c *gin.Context, info *relaycommon.RelayInfo) (*bedrockruntime.Client, error) {
	var (
		httpClient *http.Client
		err        error
	)
	if info.ChannelSetting.Proxy != "" {
		httpClient, err = service.NewProxyHttpClient(info.ChannelSetting.Proxy)
		if err != nil {
			return nil, fmt.Errorf("new proxy http client failed: %w", err)
		}
	} else {
		httpClient = service.GetHttpClient()
	}

	awsSecret := strings.Split(info.ApiKey, "|")
	var client *bedrockruntime.Client
	switch len(awsSecret) {
	case 2:
		apiKey := awsSecret[0]
		region := awsSecret[1]
		client = bedrockruntime.New(bedrockruntime.Options{
			Region:                  region,
			BearerAuthTokenProvider: bearer.StaticTokenProvider{Token: bearer.Token{Value: apiKey}},
			HTTPClient:              httpClient,
		})
	case 3:
		ak := awsSecret[0]
		sk := awsSecret[1]
		region := awsSecret[2]
		client = bedrockruntime.New(bedrockruntime.Options{
			Region:      region,
			Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(ak, sk, "")),
			HTTPClient:  httpClient,
		})
	default:
		return nil, errors.New("invalid aws secret key")
	}

	return client, nil
}

// doAwsClientRequest 通过 AWS SDK 客户端执行 Bedrock API 请求（AKSK 模式）。
// 处理流程:
//   1. 创建 AWS SDK 客户端（通过 newAwsClient）
//   2. 获取对应的 AWS 模型 ID
//   3. 检查是否支持跨区域推理（Cross-Region Inference），如支持则转换模型 ID
//   4. 设置请求头（包括 header override）
//   5. 根据模型类型构建不同的请求体:
//   - Nova 模型: 解码请求体为 NovaRequest 格式，构造 InvokeModelInput
//   - Claude 模型: 通过 formatRequest 格式化请求体，然后根据是否流式请求:
//   - 流式请求: 构造 InvokeModelWithResponseStreamInput
//   - 非流式请求: 构造 InvokeModelInput
//
// 参数:
//   - c: Gin 上下文
//   - info: 中继请求的元数据信息
//   - a: AWS 适配器实例，用于保存客户端和请求状态
//   - requestBody: 原始请求体的 io.Reader
//
// 返回: nil 和 nil（请求体已保存在适配器中，由 DoResponse 处理实际调用）或错误信息。
func doAwsClientRequest(c *gin.Context, info *relaycommon.RelayInfo, a *Adaptor, requestBody io.Reader) (any, error) {
	awsCli, err := newAwsClient(c, info)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeChannelAwsClientError)
	}
	a.AwsClient = awsCli

	// 获取对应的AWS模型ID
	awsModelId := getAwsModelID(info.UpstreamModelName)

	awsRegionPrefix := getAwsRegionPrefix(awsCli.Options().Region)
	canCrossRegion := awsModelCanCrossRegion(awsModelId, awsRegionPrefix)
	if canCrossRegion {
		awsModelId = awsModelCrossRegion(awsModelId, awsRegionPrefix)
	}

	// init empty request.header
	requestHeader := http.Header{}
	a.SetupRequestHeader(c, &requestHeader, info)
	headerOverride, err := channel.ResolveHeaderOverride(info, c)
	if err != nil {
		return nil, err
	}
	for key, value := range headerOverride {
		requestHeader.Set(key, value)
	}

	if isNovaModel(awsModelId) {
		var novaReq *NovaRequest
		err = common.DecodeJson(requestBody, &novaReq)
		if err != nil {
			return nil, types.NewError(errors.Wrap(err, "decode nova request fail"), types.ErrorCodeBadRequestBody)
		}

		// 使用InvokeModel API，但使用Nova格式的请求体
		awsReq := &bedrockruntime.InvokeModelInput{
			ModelId:     aws.String(awsModelId),
			Accept:      aws.String("application/json"),
			ContentType: aws.String("application/json"),
		}

		reqBody, err := common.Marshal(novaReq)
		if err != nil {
			return nil, types.NewError(errors.Wrap(err, "marshal nova request"), types.ErrorCodeBadResponseBody)
		}
		awsReq.Body = reqBody
		a.AwsReq = awsReq
		return nil, nil
	} else {
		awsClaudeReq, err := formatRequest(requestBody, requestHeader)
		if err != nil {
			return nil, types.NewError(errors.Wrap(err, "format aws request fail"), types.ErrorCodeBadRequestBody)
		}

		if info.IsStream {
			awsReq := &bedrockruntime.InvokeModelWithResponseStreamInput{
				ModelId:     aws.String(awsModelId),
				Accept:      aws.String("application/json"),
				ContentType: aws.String("application/json"),
			}
			awsReq.Body, err = buildAwsRequestBody(c, info, awsClaudeReq)
			if err != nil {
				return nil, types.NewError(errors.Wrap(err, "marshal aws request fail"), types.ErrorCodeBadRequestBody)
			}
			a.AwsReq = awsReq
			return nil, nil
		} else {
			awsReq := &bedrockruntime.InvokeModelInput{
				ModelId:     aws.String(awsModelId),
				Accept:      aws.String("application/json"),
				ContentType: aws.String("application/json"),
			}
			awsReq.Body, err = buildAwsRequestBody(c, info, awsClaudeReq)
			if err != nil {
				return nil, types.NewError(errors.Wrap(err, "marshal aws request fail"), types.ErrorCodeBadRequestBody)
			}
			a.AwsReq = awsReq
			return nil, nil
		}
	}
}

// buildAwsRequestBody 构建 AWS Bedrock 请求的 JSON 序列化负载。
// 支持两种模式:
//   - 透传模式（Pass-Through）: 当全局设置 PassThroughRequestEnabled 或渠道设置
//     PassThroughBodyEnabled 为 true 时，直接从请求体存储中获取原始 JSON，
//     删除 "model" 和 "stream" 字段后透传给上游。这允许客户端直接控制请求参数。
//   - 标准模式: 将格式化后的 awsClaudeReq 结构体序列化为 JSON。
//
// 参数:
//   - c: Gin 上下文
//   - info: 中继请求的元数据信息
//   - awsClaudeReq: 格式化后的请求体（标准模式使用）
//
// 返回: JSON 序列化后的字节数组和错误信息。
func buildAwsRequestBody(c *gin.Context, info *relaycommon.RelayInfo, awsClaudeReq any) ([]byte, error) {
	if model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled {
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			return nil, errors.Wrap(err, "get request body for pass-through fail")
		}
		body, err := storage.Bytes()
		if err != nil {
			return nil, errors.Wrap(err, "get request body bytes fail")
		}
		var data map[string]interface{}
		if err := common.Unmarshal(body, &data); err != nil {
			return nil, errors.Wrap(err, "pass-through unmarshal request body fail")
		}
		delete(data, "model")
		delete(data, "stream")
		return common.Marshal(data)
	}
	return common.Marshal(awsClaudeReq)
}

// getAwsRegionPrefix 从 AWS 区域 ID 中提取区域前缀。
// 例如 "us-east-1" 提取出 "us"，"ap-northeast-1" 提取出 "ap"。
// 区域前缀用于判断是否支持跨区域推理以及选择跨区域模型前缀。
//
// 参数:
//   - awsRegionId: AWS 区域 ID（如 "us-east-1"）
//
// 返回: 区域前缀字符串。
func getAwsRegionPrefix(awsRegionId string) string {
	parts := strings.Split(awsRegionId, "-")
	regionPrefix := ""
	if len(parts) > 0 {
		regionPrefix = parts[0]
	}
	return regionPrefix
}

// awsModelCanCrossRegion 判断指定的 AWS 模型在给定区域前缀下是否支持跨区域推理。
// 通过查询 awsModelCanCrossRegionMap 映射表来确定。
// 跨区域推理允许在一个区域中调用另一个区域中可用的模型。
//
// 参数:
//   - awsModelId: AWS Bedrock 模型 ID
//   - awsRegionPrefix: AWS 区域前缀（如 "us"、"ap"）
//
// 返回: 是否支持跨区域推理。
func awsModelCanCrossRegion(awsModelId, awsRegionPrefix string) bool {
	regionSet, exists := awsModelCanCrossRegionMap[awsModelId]
	return exists && regionSet[awsRegionPrefix]
}

// awsModelCrossRegion 为支持跨区域推理的模型添加区域前缀。
// 通过查询 awsRegionCrossModelPrefixMap 映射表获取对应区域前缀的模型前缀，
// 然后将前缀和原始模型 ID 用 "." 连接，形成跨区域模型 ID。
// 例如: 区域前缀 "us" + 模型 ID "anthropic.claude-3-sonnet-20240229-v1:0"
//   - 变为 "us.anthropic.claude-3-sonnet-20240229-v1:0"
//
// 参数:
//   - awsModelId: 原始 AWS Bedrock 模型 ID
//   - awsRegionPrefix: AWS 区域前缀
//
// 返回: 跨区域模型 ID，如果找不到对应前缀则返回原始模型 ID。
func awsModelCrossRegion(awsModelId, awsRegionPrefix string) string {
	modelPrefix, find := awsRegionCrossModelPrefixMap[awsRegionPrefix]
	if !find {
		return awsModelId
	}
	return modelPrefix + "." + awsModelId
}

// getAwsModelID 将请求中的模型名称映射为 AWS Bedrock 的模型 ID。
// 通过查询 awsModelIDMap 映射表进行转换。
// 如果映射表中不存在对应模型名称，则直接返回原始模型名称（允许用户直接传入 AWS 模型 ID）。
//
// 参数:
//   - requestModel: 请求中的模型名称（如 "claude-3-sonnet"）
//
// 返回: AWS Bedrock 模型 ID（如 "anthropic.claude-3-sonnet-20240229-v1:0"）。
func getAwsModelID(requestModel string) string {
	if awsModelIDName, ok := awsModelIDMap[requestModel]; ok {
		return awsModelIDName
	}
	return requestModel
}

// awsHandler 处理 AWS Bedrock Claude 模型的非流式（同步）响应。
// 处理流程:
//   1. 创建带超时的上下文
//   2. 通过 AWS SDK 的 InvokeModel API 发起同步调用
//   3. 如果调用失败，提取 HTTP 状态码并返回 OpenAI 格式的错误
//   4. 初始化 Claude 响应信息结构体（包含响应 ID、创建时间、模型名称等）
//   5. 将上游的 Content-Type 头复制到客户端响应头
//   6. 委托 claude.HandleClaudeResponseData 解析响应体并写入客户端
//
// 参数:
//   - c: Gin 上下文
//   - info: 中继请求的元数据信息
//   - a: AWS 适配器实例，包含已构建的请求体（AwsReq）
//
// 返回: NexusTok 错误（如有）和 Token 用量信息。
func awsHandler(c *gin.Context, info *relaycommon.RelayInfo, a *Adaptor) (*types.NexusTokError, *dto.Usage) {

	ctx, cancel := newAwsInvokeContext()
	defer cancel()

	awsResp, err := a.AwsClient.InvokeModel(ctx, a.AwsReq.(*bedrockruntime.InvokeModelInput))
	if err != nil {
		statusCode := getAwsErrorStatusCode(err)
		return types.NewOpenAIError(errors.Wrap(err, "InvokeModel"), types.ErrorCodeAwsInvokeError, statusCode), nil
	}

	claudeInfo := &claude.ClaudeResponseInfo{
		ResponseId:   helper.GetResponseID(c),
		Created:      common.GetTimestamp(),
		Model:        info.UpstreamModelName,
		ResponseText: strings.Builder{},
		Usage:        &dto.Usage{},
	}

	// 复制上游 Content-Type 到客户端响应头
	if awsResp.ContentType != nil && *awsResp.ContentType != "" {
		c.Writer.Header().Set("Content-Type", *awsResp.ContentType)
	}

	handlerErr := claude.HandleClaudeResponseData(c, info, claudeInfo, nil, awsResp.Body)
	if handlerErr != nil {
		return handlerErr, nil
	}
	return nil, claudeInfo.Usage
}

// awsStreamHandler 处理 AWS Bedrock Claude 模型的流式（SSE）响应。
// 处理流程:
//   1. 创建带超时的上下文
//   2. 通过 AWS SDK 的 InvokeModelWithResponseStream API 发起流式调用
//   3. 如果调用失败，提取 HTTP 状态码并返回 OpenAI 格式的错误
//   4. 获取响应流并确保在函数返回时关闭
//   5. 初始化 Claude 响应信息结构体
//   6. 遍历流中的事件:
//   - ResponseStreamMemberChunk: 收到数据块，记录首次响应时间并处理响应数据
//   - UnknownUnionMember: 未知事件类型，返回错误
//   - 其他: 未知或空事件类型，返回错误
//   7. 流结束后调用 claude.HandleStreamFinalResponse 发送最终响应
//
// 参数:
//   - c: Gin 上下文
//   - info: 中继请求的元数据信息
//   - a: AWS 适配器实例，包含已构建的请求体（AwsReq）
//
// 返回: NexusTok 错误（如有）和 Token 用量信息。
func awsStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, a *Adaptor) (*types.NexusTokError, *dto.Usage) {
	ctx, cancel := newAwsInvokeContext()
	defer cancel()

	awsResp, err := a.AwsClient.InvokeModelWithResponseStream(ctx, a.AwsReq.(*bedrockruntime.InvokeModelWithResponseStreamInput))
	if err != nil {
		statusCode := getAwsErrorStatusCode(err)
		return types.NewOpenAIError(errors.Wrap(err, "InvokeModelWithResponseStream"), types.ErrorCodeAwsInvokeError, statusCode), nil
	}
	stream := awsResp.GetStream()
	defer stream.Close()

	claudeInfo := &claude.ClaudeResponseInfo{
		ResponseId:   helper.GetResponseID(c),
		Created:      common.GetTimestamp(),
		Model:        info.UpstreamModelName,
		ResponseText: strings.Builder{},
		Usage:        &dto.Usage{},
	}

	for event := range stream.Events() {
		switch v := event.(type) {
		case *bedrockruntimeTypes.ResponseStreamMemberChunk:
			info.SetFirstResponseTime()
			respErr := claude.HandleStreamResponseData(c, info, claudeInfo, string(v.Value.Bytes))
			if respErr != nil {
				return respErr, nil
			}
		case *bedrockruntimeTypes.UnknownUnionMember:
			fmt.Println("unknown tag:", v.Tag)
			return types.NewError(errors.New("unknown response type"), types.ErrorCodeInvalidRequest), nil
		default:
			fmt.Println("union is nil or unknown type")
			return types.NewError(errors.New("nil or unknown response type"), types.ErrorCodeInvalidRequest), nil
		}
	}

	claude.HandleStreamFinalResponse(c, info, claudeInfo)
	return nil, claudeInfo.Usage
}

// handleNovaRequest 处理 Amazon Nova 模型的非流式请求。
// 处理流程:
//   1. 创建带超时的上下文
//   2. 通过 AWS SDK 的 InvokeModel API 发起同步调用
//   3. 如果调用失败，提取 HTTP 状态码并返回 OpenAI 格式的错误
//   4. 解析 Nova 模型返回的 JSON 响应体，提取:
//   - output.message.content[].text: 生成的文本内容
//   - usage: Token 使用量（inputTokens、outputTokens、totalTokens）
//   5. 将 Nova 格式的响应转换为 OpenAI 兼容的格式（OpenAITextResponse）
//   6. 通过 Gin 的 JSON 方法将响应写入客户端
//
// 参数:
//   - c: Gin 上下文
//   - info: 中继请求的元数据信息
//   - a: AWS 适配器实例，包含已构建的请求体（AwsReq）
//
// 返回: NexusTok 错误（如有）和 Token 用量信息。
func handleNovaRequest(c *gin.Context, info *relaycommon.RelayInfo, a *Adaptor) (*types.NexusTokError, *dto.Usage) {

	ctx, cancel := newAwsInvokeContext()
	defer cancel()

	awsResp, err := a.AwsClient.InvokeModel(ctx, a.AwsReq.(*bedrockruntime.InvokeModelInput))
	if err != nil {
		statusCode := getAwsErrorStatusCode(err)
		return types.NewOpenAIError(errors.Wrap(err, "InvokeModel"), types.ErrorCodeAwsInvokeError, statusCode), nil
	}

	// 解析Nova响应
	var novaResp struct {
		Output struct {
			Message struct {
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"message"`
		} `json:"output"`
		Usage struct {
			InputTokens  int `json:"inputTokens"`
			OutputTokens int `json:"outputTokens"`
			TotalTokens  int `json:"totalTokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(awsResp.Body, &novaResp); err != nil {
		return types.NewError(errors.Wrap(err, "unmarshal nova response"), types.ErrorCodeBadResponseBody), nil
	}

	// 构造OpenAI格式响应
	response := dto.OpenAITextResponse{
		Id:      helper.GetResponseID(c),
		Object:  "chat.completion",
		Created: common.GetTimestamp(),
		Model:   info.UpstreamModelName,
		Choices: []dto.OpenAITextResponseChoice{{
			Index: 0,
			Message: dto.Message{
				Role:    "assistant",
				Content: novaResp.Output.Message.Content[0].Text,
			},
			FinishReason: "stop",
		}},
		Usage: dto.Usage{
			PromptTokens:     novaResp.Usage.InputTokens,
			CompletionTokens: novaResp.Usage.OutputTokens,
			TotalTokens:      novaResp.Usage.TotalTokens,
		},
	}

	c.JSON(http.StatusOK, response)
	return nil, &response.Usage
}
