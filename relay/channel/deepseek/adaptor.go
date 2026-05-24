// Package deepseek 实现了 DeepSeek 渠道适配器。
// DeepSeek 是深度求索公司的大语言模型服务，支持 OpenAI 兼容格式
// 和 Claude 格式的请求。其特色功能包括思维链（Thinking）推理模式，
// 通过模型名后缀（如 -none/-max）控制推理深度。
package deepseek

// 标准库导入
import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	// 项目内部包
	"github.com/c1cada/NexusTok/common"                    // 公共工具函数
	"github.com/c1cada/NexusTok/dto"                        // 数据传输对象
	"github.com/c1cada/NexusTok/relay/channel"              // 通用渠道工具
	"github.com/c1cada/NexusTok/relay/channel/claude"       // Claude 渠道适配器（用于 Claude 格式请求）
	"github.com/c1cada/NexusTok/relay/channel/openai"       // OpenAI 渠道适配器（用于响应处理）
	relaycommon "github.com/c1cada/NexusTok/relay/common"  // relay 层公共工具
	"github.com/c1cada/NexusTok/relay/constant"             // relay 层常量
	"github.com/c1cada/NexusTok/setting/reasoning"          // 推理/思维链配置
	"github.com/c1cada/NexusTok/types"

	// 第三方依赖
	"github.com/gin-gonic/gin"
)

// Adaptor 是 DeepSeek 渠道的适配器，实现了 channel.Adaptor 接口。
// 支持 OpenAI 兼容格式和 Claude 格式的请求，以及思维链推理功能。
type Adaptor struct {
}

// ConvertGeminiRequest DeepSeek 渠道不支持 Gemini 请求格式。
func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertClaudeRequest 将 Claude 格式请求转换为 DeepSeek 兼容格式。
// 委托给 Claude 适配器进行基础转换，然后应用 DeepSeek V4 的思维链后缀处理。
// 参数：c - Gin 上下文，info - 中继请求信息，req - Claude 请求体。
// 返回值：转换后的请求体和可能的错误。
func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, req *dto.ClaudeRequest) (any, error) {
	adaptor := claude.Adaptor{}
	convertedRequest, err := adaptor.ConvertClaudeRequest(c, info, req)
	if err != nil {
		return nil, err
	}
	claudeRequest, ok := convertedRequest.(*dto.ClaudeRequest)
	if !ok {
		return convertedRequest, nil
	}
	// 应用 DeepSeek V4 思维链后缀（如 -none/-max）
	if err := applyDeepSeekV4ClaudeThinkingSuffix(info, claudeRequest); err != nil {
		return nil, err
	}
	return claudeRequest, nil
}

// ConvertAudioRequest DeepSeek 渠道不支持音频请求。
func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertImageRequest DeepSeek 渠道不支持图像请求。
func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// Init 初始化适配器，DeepSeek 渠道无需额外初始化。
func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

// GetRequestURL 构建发送到 DeepSeek 上游服务的请求 URL。
// 根据请求格式和 relay 模式返回不同的端点：
//   - Claude 格式：/anthropic/v1/messages
//   - Completions 模式（FIM 补全）：/beta/completions
//   - 默认（OpenAI 格式）：/v1/chat/completions
//
// 参数：info - 中继请求信息。
// 返回值：完整的请求 URL 和可能的错误。
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	fimBaseUrl := info.ChannelBaseUrl
	switch info.RelayFormat {
	case types.RelayFormatClaude:
		return fmt.Sprintf("%s/anthropic/v1/messages", info.ChannelBaseUrl), nil
	default:
		// FIM（Fill-in-the-Middle）补全需要 /beta 前缀
		if !strings.HasSuffix(info.ChannelBaseUrl, "/beta") {
			fimBaseUrl += "/beta"
		}
		switch info.RelayMode {
		case constant.RelayModeCompletions:
			return fmt.Sprintf("%s/completions", fimBaseUrl), nil
		default:
			return fmt.Sprintf("%s/v1/chat/completions", info.ChannelBaseUrl), nil
		}
	}
}

// SetupRequestHeader 设置发送到 DeepSeek 上游服务的 HTTP 请求头。
// 设置通用请求头和 Bearer Token 认证。
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", "Bearer "+info.ApiKey)
	return nil
}

// ConvertOpenAIRequest 将 OpenAI 格式请求转换为 DeepSeek 兼容格式。
// 主要处理思维链后缀（如 deepseek-v4-pro-max -> deepseek-v4-pro + thinking）。
// 参数：c - Gin 上下文，info - 中继请求信息，request - OpenAI 请求体。
func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	if err := applyDeepSeekV4OpenAIThinkingSuffix(info, request); err != nil {
		return nil, err
	}

	return request, nil
}

// applyDeepSeekV4OpenAIThinkingSuffix 处理 DeepSeek V4 模型的思维链后缀。
// 解析模型名中的思维链后缀（如 -none/-max），并设置相应的 thinking 和 reasoning_effort 参数。
// 例如：deepseek-v4-pro-max -> base=deepseek-v4-pro, type=enabled, effort=high。
//
// 参数：
//   - info: 中继请求信息（用于更新上游模型名和推理深度）
//   - request: OpenAI 请求体（用于设置 thinking 和 reasoning_effort）
//
// 返回值：可能的错误。
func applyDeepSeekV4OpenAIThinkingSuffix(info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) error {
	modelName := request.Model
	if info != nil && info.ChannelMeta != nil && info.UpstreamModelName != "" {
		modelName = info.UpstreamModelName
	}
	// 解析模型名中的思维链后缀
	baseModel, thinkingType, effort, ok := reasoning.ParseDeepSeekV4ThinkingSuffix(modelName)
	if !ok {
		return nil // 无后缀，无需处理
	}
	thinking, err := common.Marshal(map[string]string{
		"type": thinkingType,
	})
	if err != nil {
		return fmt.Errorf("error marshalling thinking: %w", err)
	}
	// 更新模型名和思维链参数
	request.Model = baseModel
	request.THINKING = thinking
	request.ReasoningEffort = effort
	if info != nil {
		if info.ChannelMeta != nil {
			info.UpstreamModelName = baseModel
		}
		info.ReasoningEffort = effort
	}
	return nil
}

// applyDeepSeekV4ClaudeThinkingSuffix 处理 DeepSeek V4 模型在 Claude 格式下的思维链后缀。
// 与 OpenAI 版本类似，但设置 Claude 格式的 Thinking 和 OutputConfig 字段。
//
// 参数：
//   - info: 中继请求信息
//   - request: Claude 请求体
//
// 返回值：可能的错误。
func applyDeepSeekV4ClaudeThinkingSuffix(info *relaycommon.RelayInfo, request *dto.ClaudeRequest) error {
	modelName := request.Model
	if info != nil && info.ChannelMeta != nil && info.UpstreamModelName != "" {
		modelName = info.UpstreamModelName
	}
	baseModel, thinkingType, effort, ok := reasoning.ParseDeepSeekV4ThinkingSuffix(modelName)
	if !ok {
		return nil // 无后缀，无需处理
	}
	request.Model = baseModel
	request.Thinking = &dto.Thinking{Type: thinkingType}
	if effort == "" {
		request.OutputConfig = nil
	} else {
		outputConfig, err := common.Marshal(map[string]string{
			"effort": effort,
		})
		if err != nil {
			return fmt.Errorf("error marshalling output_config: %w", err)
		}
		request.OutputConfig = outputConfig
	}
	if info != nil {
		if info.ChannelMeta != nil {
			info.UpstreamModelName = baseModel
		}
		info.ReasoningEffort = effort
	}
	return nil
}

// ConvertRerankRequest DeepSeek 渠道不支持重排序请求。
func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

// ConvertEmbeddingRequest DeepSeek 渠道不支持 Embedding 请求。
func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertOpenAIResponsesRequest DeepSeek 渠道不支持 OpenAI Responses API。
func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	// TODO implement me
	return nil, errors.New("not implemented")
}

// DoRequest 发送 API 请求到上游 DeepSeek 服务。
func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

// DoResponse 处理上游 DeepSeek 服务的响应。
// 根据请求格式分发到不同的处理器：
//   - Claude 格式：委托给 Claude 适配器处理
//   - 默认（OpenAI 格式）：委托给 OpenAI 适配器处理
//
// 返回值：usage 用量信息和可能的错误。
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

// GetModelList 返回 DeepSeek 渠道支持的模型列表。
func (a *Adaptor) GetModelList() []string {
	return ModelList
}

// GetChannelName 返回渠道名称 "deepseek"。
func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
