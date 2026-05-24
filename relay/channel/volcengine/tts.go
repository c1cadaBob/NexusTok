// 火山引擎 TTS（Text-to-Speech，语音合成）实现文件。
// 提供了将文本转换为语音的功能，支持 HTTP 和 WebSocket 两种通信方式。
// 包含请求/响应 DTO 定义、OpenAI 语音参数映射、HTTP/WS 响应处理等。
package volcengine

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	// 项目内部依赖
	"github.com/c1cada/NexusTok/dto"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/types"

	// 第三方依赖
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// VolcengineTTSRequest 是火山引擎 TTS API 的请求结构体。
type VolcengineTTSRequest struct {
	App     VolcengineTTSApp     `json:"app"`     // 应用信息（AppID、Token、集群）
	User    VolcengineTTSUser    `json:"user"`    // 用户信息
	Audio   VolcengineTTSAudio   `json:"audio"`   // 音频配置
	Request VolcengineTTSReqInfo `json:"request"` // 请求参数
}

// VolcengineTTSApp 是 TTS 请求中的应用信息。
type VolcengineTTSApp struct {
	AppID   string `json:"appid"`   // 应用 ID
	Token   string `json:"token"`   // 访问令牌
	Cluster string `json:"cluster"` // 集群标识
}

// VolcengineTTSUser 是 TTS 请求中的用户信息。
type VolcengineTTSUser struct {
	UID string `json:"uid"` // 用户唯一标识
}

// VolcengineTTSAudio 是 TTS 请求中的音频配置。
type VolcengineTTSAudio struct {
	VoiceType        string  `json:"voice_type"`                   // 音色类型标识
	Encoding         string  `json:"encoding"`                     // 音频编码格式（mp3、wav、ogg_opus、pcm）
	SpeedRatio       float64 `json:"speed_ratio"`                  // 语速比例
	Rate             int     `json:"rate"`                         // 采样率
	Bitrate          int     `json:"bitrate,omitempty"`            // 比特率
	LoudnessRatio    float64 `json:"loudness_ratio,omitempty"`     // 音量比例
	EnableEmotion    bool    `json:"enable_emotion,omitempty"`     // 是否启用情感
	Emotion          string  `json:"emotion,omitempty"`            // 情感类型
	EmotionScale     float64 `json:"emotion_scale,omitempty"`      // 情感强度
	ExplicitLanguage string  `json:"explicit_language,omitempty"`  // 显式语言
	ContextLanguage  string  `json:"context_language,omitempty"`   // 上下文语言
}

// VolcengineTTSReqInfo 是 TTS 请求中的请求参数。
type VolcengineTTSReqInfo struct {
	ReqID           string                   `json:"reqid"`                     // 请求唯一标识
	Text            string                   `json:"text"`                      // 待合成的文本内容
	Operation       string                   `json:"operation"`                 // 操作类型（如 "query"）
	Model           string                   `json:"model,omitempty"`           // 模型名称
	TextType        string                   `json:"text_type,omitempty"`       // 文本类型（如 "ssml"）
	SilenceDuration float64                  `json:"silence_duration,omitempty"` // 静音时长
	WithTimestamp   interface{}              `json:"with_timestamp,omitempty"`   // 是否返回时间戳
	ExtraParam      *VolcengineTTSExtraParam `json:"extra_param,omitempty"`     // 额外参数
}

// VolcengineTTSExtraParam 是 TTS 请求的额外参数配置。
type VolcengineTTSExtraParam struct {
	DisableMarkdownFilter      bool                      `json:"disable_markdown_filter,omitempty"`       // 是否禁用 Markdown 过滤
	EnableLatexTn              bool                      `json:"enable_latex_tn,omitempty"`               // 是否启用 LaTeX 文本归一化
	MuteCutThreshold           string                    `json:"mute_cut_threshold,omitempty"`            // 静音切割阈值
	MuteCutRemainMs            string                    `json:"mute_cut_remain_ms,omitempty"`            // 静音切割保留毫秒数
	DisableEmojiFilter         bool                      `json:"disable_emoji_filter,omitempty"`          // 是否禁用 Emoji 过滤
	UnsupportedCharRatioThresh float64                   `json:"unsupported_char_ratio_thresh,omitempty"` // 不支持字符比例阈值
	AigcWatermark              bool                      `json:"aigc_watermark,omitempty"`                // 是否添加 AIGC 水印
	CacheConfig                *VolcengineTTSCacheConfig `json:"cache_config,omitempty"`                  // 缓存配置
}

// VolcengineTTSCacheConfig 是 TTS 缓存配置。
type VolcengineTTSCacheConfig struct {
	TextType int  `json:"text_type,omitempty"` // 文本类型
	UseCache bool `json:"use_cache,omitempty"` // 是否使用缓存
}

// VolcengineTTSResponse 是火山引擎 TTS API 的响应结构体。
type VolcengineTTSResponse struct {
	ReqID    string                     `json:"reqid"`              // 请求 ID
	Code     int                        `json:"code"`               // 状态码（3000 表示成功）
	Message  string                     `json:"message"`            // 响应消息
	Sequence int                        `json:"sequence"`           // 序列号（负数表示最后一包）
	Data     string                     `json:"data"`               // base64 编码的音频数据
	Addition *VolcengineTTSAdditionInfo `json:"addition,omitempty"` // 附加信息
}

// VolcengineTTSAdditionInfo 是 TTS 响应中的附加信息。
type VolcengineTTSAdditionInfo struct {
	Duration string `json:"duration"` // 音频时长
}

// openAIToVolcengineVoiceMap 是 OpenAI 语音名称到火山引擎音色类型的映射表。
// 将 OpenAI TTS API 的标准音色映射为火山引擎的音色标识。
var openAIToVolcengineVoiceMap = map[string]string{
	"alloy":   "zh_male_M392_conversation_wvae_bigtts", // 合金音色 -> 中文男性对话音色
	"echo":    "zh_male_wenhao_mars_bigtts",            // 回声音色 -> 中文男性文浩音色
	"fable":   "zh_female_tianmei_mars_bigtts",         // 寓言音色 -> 中文女性甜美音色
	"onyx":    "zh_male_zhibi_mars_bigtts",             // 缟玛瑙音色 -> 中文男性知彼音色
	"nova":    "zh_female_shuangkuaisisi_mars_bigtts",  // 新星音色 -> 中文女性爽快音色
	"shimmer": "zh_female_cancan_mars_bigtts",          // 微光音色 -> 中文女性灿灿音色
}

// responseFormatToEncodingMap 是 OpenAI 响应格式到火山引擎音频编码的映射表。
var responseFormatToEncodingMap = map[string]string{
	"mp3":  "mp3",      // MP3 格式
	"opus": "ogg_opus", // Opus 编码（OGG 容器）
	"aac":  "mp3",      // AAC 降级为 MP3
	"flac": "mp3",      // FLAC 降级为 MP3
	"wav":  "wav",      // WAV 格式
	"pcm":  "pcm",      // PCM 原始音频
}

// parseVolcengineAuth 解析火山引擎的 API Key 配置。
// 格式为 "AppID|AccessToken"，两个字段用竖线分隔。
// 参数:
//   - apiKey: API Key 配置字符串
// 返回:
//   - appID: 应用 ID
//   - token: 访问令牌
//   - err: 格式不正确时返回错误
	parts := strings.Split(apiKey, "|")
	if len(parts) != 2 {
		return "", "", errors.New("invalid api key format, expected: appid|access_token")
	}
	return parts[0], parts[1], nil
}

// mapVoiceType 将 OpenAI 音色名称映射为火山引擎音色标识。
// 如果映射表中不存在，则直接返回原始值（支持直接传入火山引擎音色标识）。
// 参数:
//   - openAIVoice: OpenAI 音色名称
// 返回:
//   - string: 火山引擎音色标识
	if voice, ok := openAIToVolcengineVoiceMap[openAIVoice]; ok {
		return voice
	}
	return openAIVoice
}

// mapEncoding 将 OpenAI 响应格式映射为火山引擎音频编码格式。
// 不支持的格式默认降级为 mp3。
// 参数:
//   - responseFormat: OpenAI 响应格式（如 "mp3"、"opus" 等）
// 返回:
//   - string: 火山引擎编码格式
	if encoding, ok := responseFormatToEncodingMap[responseFormat]; ok {
		return encoding
	}
	return "mp3"
}

// getContentTypeByEncoding 根据音频编码格式返回对应的 HTTP Content-Type。
// 参数:
//   - encoding: 音频编码格式
// 返回:
//   - string: MIME 类型字符串
	contentTypeMap := map[string]string{
		"mp3":      "audio/mpeg",
		"ogg_opus": "audio/ogg",
		"wav":      "audio/wav",
		"pcm":      "audio/pcm",
	}
	if ct, ok := contentTypeMap[encoding]; ok {
		return ct
	}
	return "application/octet-stream"
}

// handleTTSResponse 处理火山引擎 TTS 的 HTTP 非流式响应。
// 流程：读取响应体 -> 解析 JSON -> 检查状态码（3000 为成功） ->
// base64 解码音频数据 -> 设置 Content-Type 并返回音频数据。
// 参数:
//   - c: Gin 上下文
//   - resp: 上游 HTTP 响应
//   - info: 中继信息
//   - encoding: 音频编码格式
// 返回:
//   - usage: token 使用量
//   - err: 处理过程中的错误信息
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, types.NewErrorWithStatusCode(
			errors.New("failed to read volcengine response"),
			types.ErrorCodeReadResponseBodyFailed,
			http.StatusInternalServerError,
		)
	}
	defer resp.Body.Close()

	var volcResp VolcengineTTSResponse
	if unmarshalErr := json.Unmarshal(body, &volcResp); unmarshalErr != nil {
		return nil, types.NewErrorWithStatusCode(
			errors.New("failed to parse volcengine response"),
			types.ErrorCodeBadResponseBody,
			http.StatusInternalServerError,
		)
	}

	if volcResp.Code != 3000 {
		return nil, types.NewErrorWithStatusCode(
			errors.New(volcResp.Message),
			types.ErrorCodeBadResponse,
			http.StatusBadRequest,
		)
	}

	audioData, decodeErr := base64.StdEncoding.DecodeString(volcResp.Data)
	if decodeErr != nil {
		return nil, types.NewErrorWithStatusCode(
			errors.New("failed to decode audio data"),
			types.ErrorCodeBadResponseBody,
			http.StatusInternalServerError,
		)
	}

	contentType := getContentTypeByEncoding(encoding)
	c.Header("Content-Type", contentType)
	c.Data(http.StatusOK, contentType, audioData)

	usage = &dto.Usage{
		PromptTokens:     info.GetEstimatePromptTokens(),
		CompletionTokens: 0,
		TotalTokens:      info.GetEstimatePromptTokens(),
	}

	return usage, nil
}

// generateRequestID 生成唯一的请求 ID（UUID v4）。
// 返回:
//   - string: UUID 格式的请求 ID
	return uuid.New().String()
}

// handleTTSWebSocketResponse 处理火山引擎 TTS 的 WebSocket 流式响应。
// 流程：建立 WebSocket 连接 -> 发送完整请求 -> 循环接收消息 ->
// 处理音频数据（写入客户端）-> 检测结束标志（Sequence < 0）。
// 支持的消息类型：错误消息、前端结果（跳过）、音频数据（写入客户端）。
// 参数:
//   - c: Gin 上下文
//   - requestURL: WebSocket 连接地址
//   - volcRequest: 火山引擎 TTS 请求体
//   - info: 中继信息
//   - encoding: 音频编码格式
// 返回:
//   - usage: token 使用量
//   - err: 处理过程中的错误信息
	_, token, parseErr := parseVolcengineAuth(info.ApiKey)
	if parseErr != nil {
		return nil, types.NewErrorWithStatusCode(
			parseErr,
			types.ErrorCodeChannelInvalidKey,
			http.StatusUnauthorized,
		)
	}

	header := http.Header{}
	header.Set("Authorization", fmt.Sprintf("Bearer;%s", token))

	conn, resp, dialErr := websocket.DefaultDialer.DialContext(context.Background(), requestURL, header)
	if dialErr != nil {
		if resp != nil {
			return nil, types.NewErrorWithStatusCode(
				fmt.Errorf("failed to connect to websocket: %w, status: %d", dialErr, resp.StatusCode),
				types.ErrorCodeBadResponseStatusCode,
				http.StatusBadGateway,
			)
		}
		return nil, types.NewErrorWithStatusCode(
			fmt.Errorf("failed to connect to websocket: %w", dialErr),
			types.ErrorCodeBadResponseStatusCode,
			http.StatusBadGateway,
		)
	}
	defer conn.Close()

	payload, marshalErr := json.Marshal(volcRequest)
	if marshalErr != nil {
		return nil, types.NewErrorWithStatusCode(
			fmt.Errorf("failed to marshal request: %w", marshalErr),
			types.ErrorCodeBadRequestBody,
			http.StatusInternalServerError,
		)
	}

	if sendErr := FullClientRequest(conn, payload); sendErr != nil {
		return nil, types.NewErrorWithStatusCode(
			fmt.Errorf("failed to send request: %w", sendErr),
			types.ErrorCodeBadRequestBody,
			http.StatusInternalServerError,
		)
	}

	contentType := getContentTypeByEncoding(encoding)
	c.Header("Content-Type", contentType)
	c.Header("Transfer-Encoding", "chunked")

	for {
		msg, recvErr := ReceiveMessage(conn)
		if recvErr != nil {
			if websocket.IsCloseError(recvErr, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				break
			}
			return nil, types.NewErrorWithStatusCode(
				fmt.Errorf("failed to receive message: %w", recvErr),
				types.ErrorCodeBadResponse,
				http.StatusInternalServerError,
			)
		}

		switch msg.MsgType {
		case MsgTypeError:
			return nil, types.NewErrorWithStatusCode(
				fmt.Errorf("received error from server: code=%d, %s", msg.ErrorCode, string(msg.Payload)),
				types.ErrorCodeBadResponse,
				http.StatusBadRequest,
			)
		case MsgTypeFrontEndResultServer:
			continue
		case MsgTypeAudioOnlyServer:
			if len(msg.Payload) > 0 {
				if _, writeErr := c.Writer.Write(msg.Payload); writeErr != nil {
					return nil, types.NewErrorWithStatusCode(
						fmt.Errorf("failed to write audio data: %w", writeErr),
						types.ErrorCodeBadResponse,
						http.StatusInternalServerError,
					)
				}
				c.Writer.Flush()
			}

			if msg.Sequence < 0 {
				c.Status(http.StatusOK)
				usage = &dto.Usage{
					PromptTokens:     info.GetEstimatePromptTokens(),
					CompletionTokens: 0,
					TotalTokens:      info.GetEstimatePromptTokens(),
				}
				return usage, nil
			}
		default:
			continue
		}
	}

	c.Status(http.StatusOK)
	usage = &dto.Usage{
		PromptTokens:     info.GetEstimatePromptTokens(),
		CompletionTokens: 0,
		TotalTokens:      info.GetEstimatePromptTokens(),
	}
	return usage, nil
}
