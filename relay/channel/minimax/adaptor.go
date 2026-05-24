// MiniMax 通道适配器文件。
// 实现了 relay/channel 接口，负责将各种格式的请求转换为 MiniMax API 兼容格式，
// 并将 MiniMax 的响应转换为 OpenAI/Claude 兼容格式。
// 支持的请求类型包括：对话补全（OpenAI/Claude 格式）、音频合成（TTS）、图片生成。
// MiniMax 通道特性：
//   - 对话补全使用 MiniMax 自有端点（/v1/text/chatcompletion_v2），透传 OpenAI 格式请求
//   - 支持 Claude 格式请求的代理（委托给 claude.Adaptor）
//   - 支持语音合成（TTS），委托给内部 handleTTSResponse 处理
//   - 支持图片生成，委托给内部 miniMaxImageHandler 处理
package minimax

// 标准库导入
import (
	"bytes"   // 字节缓冲区，用于构建请求体
	"encoding/json" // JSON 序列化/反序列化
	"errors"  // 错误创建
	"fmt"     // 格式化字符串
	"io"      // IO 读写接口
	"net/http" // HTTP 请求和响应处理

	// 项目内部依赖
	"github.com/c1cada/NexusTok/dto"                        // 数据传输对象（请求/响应结构体）
	"github.com/c1cada/NexusTok/relay/channel"              // 通道通用操作（API 请求头设置、请求发送）
	"github.com/c1cada/NexusTok/relay/channel/claude"       // Claude 通道适配器（用于 Claude 格式代理）
	"github.com/c1cada/NexusTok/relay/channel/openai"       // OpenAI 通道适配器（用于默认响应处理）
	relaycommon "github.com/c1cada/NexusTok/relay/common"   // Relay 通用模块（RelayInfo 等）
	"github.com/c1cada/NexusTok/relay/constant"             // Relay 常量（RelayMode 定义）
	"github.com/c1cada/NexusTok/types"                      // 公共类型定义（NexusTokError 等）

	// 第三方依赖
	"github.com/gin-gonic/gin" // Gin Web 框架
	"github.com/samber/lo"     // Go 泛型工具库（指针取值等）
)

// Adaptor MiniMax 通道适配器结构体。
// 实现了 channel.Adaptor 接口，提供以下核心能力：
//   - 多种请求格式转换: OpenAI、Claude、Audio、Image
//   - 请求 URL 构建: 根据 RelayMode 选择不同的 MiniMax API 端点
//   - 请求头设置: 标准 API 请求头 + Bearer 认证
//   - 响应处理: 根据请求模式和格式分发到不同的响应处理器
type Adaptor struct {
}

// ConvertGeminiRequest 将 Gemini 格式的请求转换为 MiniMax 格式。
// 当前 MiniMax 通道不支持 Gemini 格式请求，始终返回 "not implemented" 错误。
// 参数:
//   - c: Gin 上下文
//   - info: Relay 信息
//   - req: Gemini 格式的对话请求
// 返回:
//   - any: nil（未实现）
//   - error: "not implemented" 错误
func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	return nil, errors.New("not implemented")
}

// ConvertClaudeRequest 将 Claude 格式的请求转换为 MiniMax 格式。
// 委托给 Claude 通道适配器进行处理，因为 MiniMax 支持 Claude API 格式的代理。
// 参数:
//   - c: Gin 上下文
//   - info: Relay 信息，包含基础 URL、API 密钥等
//   - req: Claude 格式的对话请求
// 返回:
//   - any: Claude 格式的请求体（由 claude.Adaptor 转换）
//   - error: 转换过程中的错误
func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, req *dto.ClaudeRequest) (any, error) {
	adaptor := claude.Adaptor{}
	return adaptor.ConvertClaudeRequest(c, info, req)
}

// ConvertAudioRequest 将 OpenAI 格式的音频请求转换为 MiniMax TTS 请求格式。
// 仅支持 RelayModeAudioSpeech 模式。转换逻辑：
// 1. 提取语音 ID、语速、输出格式等参数
// 2. 构建 MiniMaxTTSRequest 结构体（包含语音设置和音频设置）
// 3. 如果请求携带 Metadata（厂商自定义字段），则反序列化并合并到请求中
// 4. 将请求序列化为 JSON 并设置 response_format 到上下文中
//
// 参数:
//   - c: Gin 上下文，用于存储 response_format 等中间状态
//   - info: Relay 信息，包含模型名称等
//   - request: OpenAI 格式的音频请求
// 返回:
//   - io.Reader: MiniMax TTS 请求的 JSON 字节流
//   - error: 非支持的 RelayMode 或序列化错误
func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	if info.RelayMode != constant.RelayModeAudioSpeech {
		return nil, errors.New("unsupported audio relay mode")
	}

	voiceID := request.Voice
	speed := lo.FromPtrOr(request.Speed, 0.0)
	outputFormat := request.ResponseFormat

	minimaxRequest := MiniMaxTTSRequest{
		Model: info.OriginModelName,
		Text:  request.Input,
		VoiceSetting: VoiceSetting{
			VoiceID: voiceID,
			Speed:   speed,
		},
		AudioSetting: &AudioSetting{
			Format: outputFormat,
		},
		OutputFormat: outputFormat,
	}

	// 同步扩展字段的厂商自定义metadata
	if len(request.Metadata) > 0 {
		if err := json.Unmarshal(request.Metadata, &minimaxRequest); err != nil {
			return nil, fmt.Errorf("error unmarshalling metadata to minimax request: %w", err)
		}
	}

	jsonData, err := json.Marshal(minimaxRequest)
	if err != nil {
		return nil, fmt.Errorf("error marshalling minimax request: %w", err)
	}
	if outputFormat != "hex" {
		outputFormat = "url"
	}

	c.Set("response_format", outputFormat)

	// Debug: log the request structure
	// fmt.Printf("MiniMax TTS Request: %s\n", string(jsonData))

	return bytes.NewReader(jsonData), nil
}

// ConvertImageRequest 将 OpenAI 格式的图片生成请求转换为 MiniMax 格式。
// 仅支持 RelayModeImagesGenerations 模式。
// 委托给 oaiImage2MiniMaxImageRequest 函数进行格式转换，处理模型名、提示词、
// 宽高比、响应格式等参数的映射。
// 参数:
//   - c: Gin 上下文
//   - info: Relay 信息，包含 RelayMode 和原始模型名称
//   - request: OpenAI 格式的图片生成请求
// 返回:
//   - any: MiniMaxImageRequest 结构体
//   - error: 不支持的 RelayMode 时返回错误
func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	if info.RelayMode != constant.RelayModeImagesGenerations {
		return nil, fmt.Errorf("unsupported image relay mode: %d", info.RelayMode)
	}
	return oaiImage2MiniMaxImageRequest(request), nil
}

// Init 初始化 MiniMax 通道适配器。
// 当前 MiniMax 通道无需特殊的初始化逻辑，方法体为空。
// 参数:
//   - info: Relay 信息
func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

// GetRequestURL 获取 MiniMax API 的请求 URL。
// 委托给包级别的 GetRequestURL 函数，根据 RelayMode 和 RelayFormat
// 构建对应的 API 端点地址。
// 参数:
//   - info: Relay 信息，包含基础 URL 和请求模式
// 返回:
//   - string: 完整的 API 请求 URL
//   - error: 不支持的模式时返回错误
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return GetRequestURL(info)
}

// SetupRequestHeader 设置发送给 MiniMax API 的请求头。
// 调用通道通用的 SetupApiRequestHeader 设置标准请求头（Content-Type 等），
// 并添加 Bearer Token 认证头。
// 参数:
//   - c: Gin 上下文
//   - req: 待设置的 HTTP 请求头
//   - info: Relay 信息，包含 API 密钥
// 返回:
//   - error: 始终返回 nil
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", "Bearer "+info.ApiKey)
	return nil
}

// ConvertOpenAIRequest 将 OpenAI 格式的对话请求转换为 MiniMax 格式。
// MiniMax 的对话补全 API 兼容 OpenAI 格式，因此直接透传原始请求。
// 参数:
//   - c: Gin 上下文
//   - info: Relay 信息
//   - request: OpenAI 格式的通用对话请求
// 返回:
//   - any: 原始请求对象（无需转换）
//   - error: 请求为 nil 时返回错误
func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	return request, nil
}

// ConvertRerankRequest 将重排序请求转换为 MiniMax 格式。
// 当前 MiniMax 通道不支持重排序功能，返回 nil。
// 参数:
//   - c: Gin 上下文
//   - relayMode: Relay 模式
//   - request: 重排序请求
// 返回:
//   - any: nil（不支持）
//   - error: nil
func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

// ConvertEmbeddingRequest 将 Embedding 请求转换为 MiniMax 格式。
// 直接透传原始请求，无需格式转换。
// 参数:
//   - c: Gin 上下文
//   - info: Relay 信息
//   - request: OpenAI 格式的 Embedding 请求
// 返回:
//   - any: 原始请求对象
//   - error: nil
func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return request, nil
}

// ConvertOpenAIResponsesRequest 将 OpenAI Responses 格式的请求转换为 MiniMax 格式。
// 当前 MiniMax 通道不支持 OpenAI Responses 格式，始终返回 "not implemented" 错误。
// 参数:
//   - c: Gin 上下文
//   - info: Relay 信息
//   - request: OpenAI Responses 格式的请求
// 返回:
//   - any: nil（未实现）
//   - error: "not implemented" 错误
func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return nil, errors.New("not implemented")
}

// DoRequest 执行发送给 MiniMax API 的 HTTP 请求。
// 委托给通道通用的 DoApiRequest 函数，使用适配器自身作为回调处理器。
// 参数:
//   - c: Gin 上下文
//   - info: Relay 信息，包含目标 URL、超时配置等
//   - requestBody: 请求体的 io.Reader
// 返回:
//   - any: HTTP 响应对象
//   - error: 请求发送过程中的错误
func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

// DoResponse 处理 MiniMax API 的 HTTP 响应，并转换为客户端可识别的格式。
// 根据请求模式和格式分发到不同的处理器：
//   - RelayModeAudioSpeech: 委托给 handleTTSResponse 处理语音合成响应
//   - RelayModeImagesGenerations: 委托给 miniMaxImageHandler 处理图片生成响应
//   - RelayFormatClaude: 委托给 Claude 适配器处理 Claude 格式响应
//   - 默认: 委托给 OpenAI 适配器处理标准 OpenAI 格式响应
//
// 参数:
//   - c: Gin 上下文，用于写入响应
//   - resp: MiniMax API 返回的 HTTP 响应
//   - info: Relay 信息，包含请求模式和格式
// 返回:
//   - usage: token 使用量信息
//   - err: 错误信息（NexusTokError 类型）
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NexusTokError) {
	if info.RelayMode == constant.RelayModeAudioSpeech {
		return handleTTSResponse(c, resp, info)
	}
	if info.RelayMode == constant.RelayModeImagesGenerations {
		return miniMaxImageHandler(c, resp, info)
	}

	switch info.RelayFormat {
	case types.RelayFormatClaude:
		adaptor := claude.Adaptor{}
		return adaptor.DoResponse(c, resp, info)
	default:
		adaptor := openai.Adaptor{}
		return adaptor.DoResponse(c, resp, info)
	}
}

// GetModelList 获取 MiniMax 通道支持的所有模型列表。
// 返回 constants.go 中定义的 ModelList，包含对话模型、语音合成模型和图片生成模型。
// 返回:
//   - []string: 支持的模型名称列表
func (a *Adaptor) GetModelList() []string {
	return ModelList
}

// GetChannelName 获取通道名称。
// 返回 constants.go 中定义的 ChannelName，值为 "minimax"。
// 返回:
//   - string: 通道名称标识
func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
