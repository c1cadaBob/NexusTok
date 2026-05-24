// Package jimeng 实现即梦 AI（字节跳动旗下）图片生成通道的适配器。
// 负责将 OpenAI 格式的请求转换为即梦 API 格式，并处理即梦的响应。
// 支持图片生成功能，其他功能（文本、嵌入等）暂未实现。
package jimeng

// 标准库导入
import (
	"encoding/json" // JSON 序列化/反序列化
	"errors"        // 错误创建
	"fmt"           // 格式化字符串
	"io"            // IO 读写接口
	"net/http"      // HTTP 客户端和响应处理

	// 项目内部依赖
	"github.com/c1cada/NexusTok/dto"                  // 数据传输对象定义
	"github.com/c1cada/NexusTok/relay/channel"          // 通道通用工具函数
	"github.com/c1cada/NexusTok/relay/channel/openai"   // OpenAI 通道处理器（用于文本流式/非流式响应）
	relaycommon "github.com/c1cada/NexusTok/relay/common"  // Relay 通用模块
	relayconstant "github.com/c1cada/NexusTok/relay/constant" // Relay 常量定义
	"github.com/c1cada/NexusTok/types"                 // 公共类型定义

	// 第三方依赖
	"github.com/gin-gonic/gin" // Gin Web 框架
)

// Adaptor 即梦 AI 通道适配器。
// 实现了 ChannelAdaptor 接口，负责请求格式转换和响应处理。
type Adaptor struct {
}

// ConvertGeminiRequest 将 Gemini 格式请求转换为即梦格式。
// 当前未实现，调用将返回错误。
func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertClaudeRequest 将 Claude 格式请求转换为即梦格式。
// 当前未实现，调用将返回错误。
func (a *Adaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	return nil, errors.New("not implemented")
}

// Init 初始化适配器。
// 即梦通道无需特殊初始化操作。
func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

// GetRequestURL 构建即梦 API 的请求 URL。
// 拼接基础地址和即梦 CVProcess API 路径，版本号为 2022-08-31。
// 参数:
//   - info: Relay 信息，包含渠道基础 URL
// 返回:
//   - string: 完整的 API 请求 URL
//   - error: 错误信息（当前始终返回 nil）
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return fmt.Sprintf("%s/?Action=CVProcess&Version=2022-08-31", info.ChannelBaseUrl), nil
}

// SetupRequestHeader 设置请求头。
// 当前未实现，即梦使用自定义签名认证方式（见 DoRequest 中的 Sign 调用）。
func (a *Adaptor) SetupRequestHeader(c *gin.Context, header *http.Header, info *relaycommon.RelayInfo) error {
	return errors.New("not implemented")
}

// ConvertOpenAIRequest 将 OpenAI 格式请求转换为即梦格式。
// 当前直接透传原始请求，不做格式转换。
// 参数:
//   - c: Gin 上下文
//   - info: Relay 信息
//   - request: OpenAI 格式的通用请求对象
// 返回:
//   - any: 转换后的请求体
//   - error: 错误信息
func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	return request, nil
}

// LogoInfo 即梦图片水印配置信息。
type LogoInfo struct {
	AddLogo         bool    `json:"add_logo,omitempty"`
	Position        int     `json:"position,omitempty"`
	Language        int     `json:"language,omitempty"`
	Opacity         float64 `json:"opacity,omitempty"`
	LogoTextContent string  `json:"logo_text_content,omitempty"`
}

// imageRequestPayload 即梦图片生成 API 的请求体结构体。
// 包含服务标识、提示词、尺寸、种子、超分辨率等配置项。
type imageRequestPayload struct {
	ReqKey     string   `json:"req_key"`                      // Service identifier, fixed value: jimeng_high_aes_general_v21_L
	Prompt     string   `json:"prompt"`                       // Prompt for image generation, supports both Chinese and English
	Seed       int64    `json:"seed,omitempty"`               // Random seed, default -1 (random)
	Width      int      `json:"width,omitempty"`              // Image width, default 512, range [256, 768]
	Height     int      `json:"height,omitempty"`             // Image height, default 512, range [256, 768]
	UsePreLLM  bool     `json:"use_pre_llm,omitempty"`        // Enable text expansion, default true
	UseSR      bool     `json:"use_sr,omitempty"`             // Enable super resolution, default true
	ReturnURL  bool     `json:"return_url,omitempty"`         // Whether to return image URL (valid for 24 hours)
	LogoInfo   LogoInfo `json:"logo_info,omitempty"`          // Watermark information
	ImageUrls  []string `json:"image_urls,omitempty"`         // Image URLs for input
	BinaryData []string `json:"binary_data_base64,omitempty"` // Base64 encoded binary data
}

// ConvertImageRequest 将 OpenAI 格式的图片请求转换为即梦格式。
// 构建即梦专用的图片生成请求体，支持通过 ExtraFields 传递即梦特有参数。
// 参数:
//   - c: Gin 上下文
//   - info: Relay 信息
//   - request: OpenAI 格式的图片生成请求
// 返回:
//   - any: 即梦格式的请求体 (imageRequestPayload)
//   - error: 错误信息
func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	payload := imageRequestPayload{
		ReqKey: request.Model,
		Prompt: request.Prompt,
	}
	if request.ResponseFormat == "" || request.ResponseFormat == "url" {
		payload.ReturnURL = true // Default to returning image URLs
	}

	if len(request.ExtraFields) > 0 {
		if err := json.Unmarshal(request.ExtraFields, &payload); err != nil {
			return nil, fmt.Errorf("failed to unmarshal extra fields: %w", err)
		}
	}

	return payload, nil
}

// ConvertRerankRequest Rerank 请求转换（即梦不支持）。
func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, errors.New("not implemented")
}

// ConvertEmbeddingRequest Embedding 请求转换（即梦不支持）。
func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return nil, errors.New("not implemented")
}

// ConvertAudioRequest 音频请求转换（即梦不支持）。
func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, errors.New("not implemented")
}

// ConvertOpenAIResponsesRequest OpenAI Responses API 请求转换（即梦不支持）。
func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return nil, errors.New("not implemented")
}

// DoRequest 执行向即梦 API 的 HTTP 请求。
// 流程：构建请求 URL -> 创建 HTTP 请求 -> 添加签名认证 -> 发送请求。
// 参数:
//   - c: Gin 上下文
//   - info: Relay 信息
//   - requestBody: 请求体的 IO Reader
// 返回:
//   - any: HTTP 响应对象
//   - error: 错误信息
func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	fullRequestURL, err := a.GetRequestURL(info)
	if err != nil {
		return nil, fmt.Errorf("get request url failed: %w", err)
	}
	req, err := http.NewRequest(c.Request.Method, fullRequestURL, requestBody)
	if err != nil {
		return nil, fmt.Errorf("new request failed: %w", err)
	}
	err = Sign(c, req, info.ApiKey)
	if err != nil {
		return nil, fmt.Errorf("setup request header failed: %w", err)
	}
	resp, err := channel.DoRequest(c, req, info)
	if err != nil {
		return nil, fmt.Errorf("do request failed: %w", err)
	}
	return resp, nil
}

// DoResponse 处理即梦 API 的响应。
// 根据 RelayMode 分发到不同的处理器：
//   - 图片生成模式: 使用 jimengImageHandler 处理
//   - 流式文本模式: 委托给 OpenAI 流式处理器
//   - 非流式文本模式: 委托给 OpenAI 非流式处理器
//
// 参数:
//   - c: Gin 上下文
//   - resp: 即梦 API 返回的 HTTP 响应
//   - info: Relay 信息
// 返回:
//   - usage: token 使用量信息
//   - err: 错误信息
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NexusTokError) {
	if info.RelayMode == relayconstant.RelayModeImagesGenerations {
		usage, err = jimengImageHandler(c, resp, info)
	} else if info.IsStream {
		usage, err = openai.OaiStreamHandler(c, info, resp)
	} else {
		usage, err = openai.OpenaiHandler(c, info, resp)
	}
	return
}

// GetModelList 返回即梦支持的模型列表。
func (a *Adaptor) GetModelList() []string {
	return ModelList
}

// GetChannelName 返回通道名称。
func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
