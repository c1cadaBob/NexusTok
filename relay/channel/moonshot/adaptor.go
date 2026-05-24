// Package moonshot 实现 Moonshot (Kimi) AI 提供商的适配器。
// Moonshot 兼容 OpenAI 和 Claude 的 API 格式，本适配器负责：
// - 请求格式转换（OpenAI/Claude 格式到上游格式）
// - 响应格式转换（上游格式到 OpenAI/Claude 格式）
// - 请求 URL 构建（根据 RelayFormat 和 RelayMode 选择正确端点）
// - 请求头设置（Bearer Token 认证）
package moonshot

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	// 项目内部包
	channelconstant "github.com/c1cada/NexusTok/constant"           // 渠道常量定义
	"github.com/c1cada/NexusTok/dto"                                 // 数据传输对象
	"github.com/c1cada/NexusTok/relay/channel"                       // 渠道通用工具
	"github.com/c1cada/NexusTok/relay/channel/claude"                // Claude 适配器（用于 Claude 格式请求转换）
	"github.com/c1cada/NexusTok/relay/channel/openai"                // OpenAI 适配器（用于 OpenAI 格式请求转换）
	relaycommon "github.com/c1cada/NexusTok/relay/common"            // Relay 通用信息
	"github.com/c1cada/NexusTok/relay/constant"                      // Relay 常量（RelayMode 等）
	"github.com/c1cada/NexusTok/types"                               // 类型定义（错误类型等）

	// 第三方依赖
	"github.com/gin-gonic/gin"                                       // Gin Web 框架
)

// Adaptor 是 Moonshot 渠道的适配器实现。
// Moonshot 支持 OpenAI 和 Claude 两种 API 格式，通过 RelayFormat 字段区分。
type Adaptor struct {
}

// ConvertGeminiRequest 将 Gemini 格式请求转换为 Moonshot 上游格式。
// Moonshot 不支持 Gemini 格式，返回未实现错误。
// 参数:
//   - c: Gin 上下文
//   - info: Relay 信息（包含渠道配置、模型信息等）
//   - request: Gemini 格式的聊天请求
// 返回:
//   - any: 转换后的请求体（此处不会返回）
//   - error: 始终返回 "not implemented" 错误
func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertClaudeRequest 将 Claude 格式请求转换为 Moonshot 上游格式。
// 直接委托给 Claude 适配器处理，因为 Moonshot 兼容 Claude API。
// 参数:
//   - c: Gin 上下文
//   - info: Relay 信息
//   - req: Claude 格式请求
// 返回:
//   - any: 转换后的请求体
//   - error: 转换过程中的错误
func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, req *dto.ClaudeRequest) (any, error) {
	adaptor := claude.Adaptor{}
	return adaptor.ConvertClaudeRequest(c, info, req)
}

// ConvertAudioRequest 将音频请求转换为 Moonshot 上游格式。
// Moonshot 不支持音频请求，返回未实现错误。
// 参数:
//   - c: Gin 上下文
//   - info: Relay 信息
//   - request: 音频请求
// 返回:
//   - io.Reader: 请求体（此处不会返回）
//   - error: 始终返回 "not supported" 错误
func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//TODO implement me
	return nil, errors.New("not supported")
}

// ConvertImageRequest 将图片请求转换为 Moonshot 上游格式。
// 委托给 OpenAI 适配器处理，因为 Moonshot 兼容 OpenAI 图片 API。
// 参数:
//   - c: Gin 上下文
//   - info: Relay 信息
//   - request: 图片请求
// 返回:
//   - any: 转换后的请求体
//   - error: 转换过程中的错误
func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	adaptor := openai.Adaptor{}
	return adaptor.ConvertImageRequest(c, info, request)
}

// Init 初始化适配器。
// Moonshot 适配器无需特殊初始化逻辑。
// 参数:
//   - info: Relay 信息
func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

// GetRequestURL 构建上游请求的完整 URL。
// 根据 RelayFormat（Claude/OpenAI）和 RelayMode（聊天/嵌入/补全/Rerank）选择正确的端点。
// 支持特殊渠道基础 URL（ChannelSpecialBases）覆盖。
// 参数:
//   - info: Relay 信息（包含 ChannelBaseUrl、RelayFormat、RelayMode 等）
// 返回:
//   - string: 完整的上游请求 URL
//   - error: URL 构建过程中的错误（此处始终返回 nil）
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	baseURL := info.ChannelBaseUrl
	// 检查是否有特殊渠道基础 URL 配置
	if specialPlan, ok := channelconstant.ChannelSpecialBases[baseURL]; ok {
		// Claude 格式请求使用 Claude 专用基础 URL
		if info.RelayFormat == types.RelayFormatClaude {
			return fmt.Sprintf("%s/v1/messages", specialPlan.ClaudeBaseURL), nil
		}
		// OpenAI 格式请求使用 OpenAI 专用基础 URL
		if info.RelayFormat == types.RelayFormatOpenAI {
			return fmt.Sprintf("%s/chat/completions", specialPlan.OpenAIBaseURL), nil
		}
	}

	// 根据 RelayFormat 选择端点
	switch info.RelayFormat {
	case types.RelayFormatClaude:
		// Claude 格式使用 /anthropic/v1/messages 端点
		return fmt.Sprintf("%s/anthropic/v1/messages", info.ChannelBaseUrl), nil
	default:
		// OpenAI 格式根据 RelayMode 选择端点
		if info.RelayMode == constant.RelayModeRerank {
			return fmt.Sprintf("%s/v1/rerank", info.ChannelBaseUrl), nil
		} else if info.RelayMode == constant.RelayModeEmbeddings {
			return fmt.Sprintf("%s/v1/embeddings", info.ChannelBaseUrl), nil
		} else if info.RelayMode == constant.RelayModeChatCompletions {
			return fmt.Sprintf("%s/v1/chat/completions", info.ChannelBaseUrl), nil
		} else if info.RelayMode == constant.RelayModeCompletions {
			return fmt.Sprintf("%s/v1/completions", info.ChannelBaseUrl), nil
		}
		// 默认使用聊天补全端点
		return fmt.Sprintf("%s/v1/chat/completions", info.ChannelBaseUrl), nil
	}
}

// SetupRequestHeader 设置上游请求的 HTTP 头部。
// 包含通用 API 请求头和 Bearer Token 认证头。
// 参数:
//   - c: Gin 上下文
//   - req: HTTP 请求头指针
//   - info: Relay 信息（包含 ApiKey 等）
// 返回:
//   - error: 始终返回 nil
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", fmt.Sprintf("Bearer %s", info.ApiKey))
	return nil
}

// ConvertOpenAIRequest 将 OpenAI 格式请求转换为 Moonshot 上游格式。
// Moonshot 兼容 OpenAI API，直接返回原请求。
// 参数:
//   - c: Gin 上下文
//   - info: Relay 信息
//   - request: OpenAI 格式请求
// 返回:
//   - any: 原请求体
//   - error: 始终返回 nil
func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	return request, nil
}

// ConvertOpenAIResponsesRequest 将 OpenAI Responses 格式请求转换为上游格式。
// Moonshot 尚未支持 Responses API，返回未实现错误。
// 参数:
//   - c: Gin 上下文
//   - info: Relay 信息
//   - request: OpenAI Responses 格式请求
// 返回:
//   - any: 转换后的请求体（此处不会返回）
//   - error: 始终返回 "not implemented" 错误
func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	// TODO implement me
	return nil, errors.New("not implemented")
}

// DoRequest 执行上游 HTTP 请求。
// 委托给通用的 channel.DoApiRequest 方法处理。
// 参数:
//   - c: Gin 上下文
//   - info: Relay 信息
//   - requestBody: 请求体的 io.Reader
// 返回:
//   - any: 原始 HTTP 响应
//   - error: 请求过程中的错误
func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

// ConvertRerankRequest 将 Rerank 请求转换为上游格式。
// Moonshot 兼容 Rerank API，直接返回原请求。
// 参数:
//   - c: Gin 上下文
//   - relayMode: Relay 模式
//   - request: Rerank 请求
// 返回:
//   - any: 原请求体
//   - error: 始终返回 nil
func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return request, nil
}

// ConvertEmbeddingRequest 将嵌入请求转换为上游格式。
// Moonshot 兼容嵌入 API，直接返回原请求。
// 参数:
//   - c: Gin 上下文
//   - info: Relay 信息
//   - request: 嵌入请求
// 返回:
//   - any: 原请求体
//   - error: 始终返回 nil
func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return request, nil
}

// DoResponse 处理上游响应并转换为客户端格式。
// 根据 RelayFormat 选择不同的响应处理器：
//   - Claude 格式：使用 Claude 适配器处理
//   - OpenAI 格式：使用 OpenAI 适配器处理
//
// 参数:
//   - c: Gin 上下文
//   - resp: 上游 HTTP 响应
//   - info: Relay 信息
// 返回:
//   - any: 使用量统计
//   - *types.NexusTokError: 错误信息（成功时为 nil）
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NexusTokError) {
	switch info.RelayFormat {
	case types.RelayFormatClaude:
		adaptor := claude.Adaptor{}
		return adaptor.DoResponse(c, resp, info)
	default:
		adaptor := openai.Adaptor{}
		return adaptor.DoResponse(c, resp, info)
	}
}

// GetModelList 返回 Moonshot 支持的模型列表。
// 返回:
//   - []string: 模型名称列表
func (a *Adaptor) GetModelList() []string {
	return ModelList
}

// GetChannelName 返回渠道名称。
// 返回:
//   - string: 渠道名称 "moonshot"
func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
