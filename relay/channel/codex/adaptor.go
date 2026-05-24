// Package codex 实现了 Codex 渠道适配器。
// Codex 是 OpenAI 的代码生成服务，通过 ChatGPT 后端 API 提供服务。
// 该适配器支持 /v1/responses 端点（包括紧凑模式），不支持传统的
// /v1/chat/completions、/v1/embeddings、/v1/rerank 等端点。
package codex

// 标准库导入
import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	// 项目内部包
	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/relay/channel"
	"github.com/c1cada/NexusTok/relay/channel/openai"       // OpenAI 渠道的响应处理逻辑
	relaycommon "github.com/c1cada/NexusTok/relay/common"   // relay 层公共工具
	relayconstant "github.com/c1cada/NexusTok/relay/constant" // relay 层常量定义
	"github.com/c1cada/NexusTok/types"

	// 第三方依赖
	"github.com/gin-gonic/gin"
)

// Adaptor 是 Codex 渠道的适配器，实现了 channel.Adaptor 接口。
// 仅支持 OpenAI Responses API 端点，其他端点均返回不支持错误。
type Adaptor struct {
}

// ConvertGeminiRequest Codex 渠道不支持 Gemini 请求格式，直接返回错误。
func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	return nil, errors.New("codex channel: endpoint not supported")
}

// ConvertClaudeRequest Codex 渠道不支持 Claude /v1/messages 端点。
func (a *Adaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	return nil, errors.New("codex channel: /v1/messages endpoint not supported")
}

// ConvertAudioRequest Codex 渠道不支持音频请求。
func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, errors.New("codex channel: endpoint not supported")
}

// ConvertImageRequest Codex 渠道不支持图像请求。
func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	return nil, errors.New("codex channel: endpoint not supported")
}

// Init 初始化适配器，Codex 渠道无需额外初始化。
func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

// ConvertOpenAIRequest Codex 渠道不支持 /v1/chat/completions 端点。
func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	return nil, errors.New("codex channel: /v1/chat/completions endpoint not supported")
}

// ConvertRerankRequest Codex 渠道不支持 /v1/rerank 端点。
func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, errors.New("codex channel: /v1/rerank endpoint not supported")
}

// ConvertEmbeddingRequest Codex 渠道不支持 /v1/embeddings 端点。
func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return nil, errors.New("codex channel: /v1/embeddings endpoint not supported")
}

// ConvertOpenAIResponsesRequest 将 OpenAI Responses 请求转换为 Codex 后端所需的格式。
// 主要处理逻辑：
//  1. 如果渠道配置了 systemPrompt，将其注入到 instructions 字段中
//  2. 如果启用了 systemPromptOverride，则将渠道系统提示与原始提示合并
//  3. 确保 instructions 字段始终存在（默认为空字符串）
//  4. 非紧凑模式下，强制设置 store=false 并移除 max_output_tokens 和 temperature
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继请求信息，包含渠道配置
//   - request: OpenAI Responses 请求体
//
// 返回值：转换后的请求体和可能的错误
func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	isCompact := info != nil && info.RelayMode == relayconstant.RelayModeResponsesCompact

	if info != nil && info.ChannelSetting.SystemPrompt != "" {
		systemPrompt := info.ChannelSetting.SystemPrompt

		if len(request.Instructions) == 0 {
			// 没有原始 instructions，直接使用渠道系统提示
			if b, err := common.Marshal(systemPrompt); err == nil {
				request.Instructions = b
			} else {
				return nil, err
			}
		} else if info.ChannelSetting.SystemPromptOverride {
			// 启用了系统提示覆盖模式
			var existing string
			if err := common.Unmarshal(request.Instructions, &existing); err == nil {
				existing = strings.TrimSpace(existing)
				if existing == "" {
					if b, err := common.Marshal(systemPrompt); err == nil {
						request.Instructions = b
					} else {
						return nil, err
					}
				} else {
					// 将渠道系统提示与原始提示合并
					if b, err := common.Marshal(systemPrompt + "\n" + existing); err == nil {
						request.Instructions = b
					} else {
						return nil, err
					}
				}
			} else {
				if b, err := common.Marshal(systemPrompt); err == nil {
					request.Instructions = b
				} else {
					return nil, err
				}
			}
		}
	}
	// Codex 后端要求 instructions 字段必须存在，
	// 与 Codex CLI 行为保持一致，默认设置为空字符串
	if len(request.Instructions) == 0 {
		request.Instructions = json.RawMessage(`""`)
	}

	if isCompact {
		return request, nil
	}
	// Codex 要求 store 必须为 false
	request.Store = json.RawMessage("false")
	// 移除 max_output_tokens 和 temperature 参数
	request.MaxOutputTokens = nil
	request.Temperature = nil
	return request, nil
}

// DoRequest 发送 API 请求到上游 Codex 服务。
// 参数：c - Gin 上下文，info - 中继请求信息，requestBody - 请求体 reader。
// 返回值：上游响应和可能的错误。
func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

// DoResponse 处理上游 Codex 服务的响应。
// 仅支持 Responses 和 ResponsesCompact 模式，其他模式返回不支持错误。
// 紧凑模式使用 OaiResponsesCompactionHandler 处理，
// 流式模式使用 OaiResponsesStreamHandler 处理，
// 非流式模式使用 OaiResponsesHandler 处理。
//
// 参数：
//   - c: Gin 上下文
//   - resp: 上游 HTTP 响应
//   - info: 中继请求信息
//
// 返回值：usage 用量信息和可能的错误
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NexusTokError) {
	if info.RelayMode != relayconstant.RelayModeResponses && info.RelayMode != relayconstant.RelayModeResponsesCompact {
		return nil, types.NewError(errors.New("codex channel: endpoint not supported"), types.ErrorCodeInvalidRequest)
	}

	if info.RelayMode == relayconstant.RelayModeResponsesCompact {
		return openai.OaiResponsesCompactionHandler(c, resp)
	}

	if info.IsStream {
		return openai.OaiResponsesStreamHandler(c, info, resp)
	}
	return openai.OaiResponsesHandler(c, info, resp)
}

// GetModelList 返回 Codex 渠道支持的模型列表。
func (a *Adaptor) GetModelList() []string {
	return ModelList
}

// GetChannelName 返回渠道名称 "codex"。
func (a *Adaptor) GetChannelName() string {
	return ChannelName
}

// GetRequestURL 构建发送到上游 Codex 服务的请求 URL。
// 仅支持 Responses 和 ResponsesCompact 两种 relay 模式。
// ResponsesCompact 模式使用 /backend-api/codex/responses/compact 路径，
// 普通 Responses 模式使用 /backend-api/codex/responses 路径。
//
// 参数：info - 中继请求信息
// 返回值：完整的请求 URL 和可能的错误
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info.RelayMode != relayconstant.RelayModeResponses && info.RelayMode != relayconstant.RelayModeResponsesCompact {
		return "", errors.New("codex channel: only /v1/responses and /v1/responses/compact are supported")
	}
	path := "/backend-api/codex/responses"
	if info.RelayMode == relayconstant.RelayModeResponsesCompact {
		path = "/backend-api/codex/responses/compact"
	}
	return relaycommon.GetFullRequestURL(info.ChannelBaseUrl, path, info.ChannelType), nil
}

// SetupRequestHeader 设置发送到 Codex 上游服务的 HTTP 请求头。
// 主要操作：
//  1. 调用通用 API 请求头设置
//  2. 解析 JSON 格式的 OAuth 密钥，提取 access_token 和 account_id
//  3. 设置 Authorization、chatgpt-account-id 等必填头
//  4. 设置 OpenAI-Beta 和 originator 头（如果未设置）
//  5. 强制设置 Content-Type 为 application/json（上游对 Content-Type 格式要求严格）
//  6. 根据是否流式请求设置 Accept 头
//
// 参数：
//   - c: Gin 上下文
//   - req: 要设置的请求头
//   - info: 中继请求信息
//
// 返回值：可能的错误
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)

	// 验证 API Key 必须是 JSON 格式
	key := strings.TrimSpace(info.ApiKey)
	if !strings.HasPrefix(key, "{") {
		return errors.New("codex channel: key must be a JSON object")
	}

	// 解析 OAuth 密钥 JSON
	oauthKey, err := ParseOAuthKey(key)
	if err != nil {
		return err
	}

	accessToken := strings.TrimSpace(oauthKey.AccessToken)
	accountID := strings.TrimSpace(oauthKey.AccountID)

	// 验证必填字段
	if accessToken == "" {
		return errors.New("codex channel: access_token is required")
	}
	if accountID == "" {
		return errors.New("codex channel: account_id is required")
	}

	// 设置认证和账户头
	req.Set("Authorization", "Bearer "+accessToken)
	req.Set("chatgpt-account-id", accountID)

	// 设置 Codex 特有的请求头
	if req.Get("OpenAI-Beta") == "" {
		req.Set("OpenAI-Beta", "responses=experimental")
	}
	if req.Get("originator") == "" {
		req.Set("originator", "codex_cli_rs")
	}

	// chatgpt.com 后端 API 对 Content-Type 格式要求严格，
	// 客户端可能省略或包含参数（如 charset=utf-8），会被上游拒绝，
	// 因此强制设置精确的媒体类型
	req.Set("Content-Type", "application/json")
	if info.IsStream {
		req.Set("Accept", "text/event-stream")
	} else if req.Get("Accept") == "" {
		req.Set("Accept", "application/json")
	}

	return nil
}
