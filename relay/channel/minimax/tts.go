// MiniMax 语音合成（TTS）处理文件。
// 负责将 MiniMax 的 TTS 响应转换为可返回的音频数据。
// 支持两种音频返回方式：
//   - URL 重定向: 当音频数据为 HTTP 链接时，直接重定向客户端
//   - 二进制数据: 当音频数据为 hex 编码时，解码后以音频流返回
// 同时包含 MiniMax 对话补全响应的处理逻辑。
package minimax

// 标准库导入
import (
	"encoding/hex" // hex 编解码（用于音频数据解码）
	"encoding/json" // JSON 序列化/反序列化
	"errors"       // 错误创建
	"fmt"          // 格式化字符串
	"io"           // IO 读写接口
	"net/http"     // HTTP 响应处理
	"strings"      // 字符串操作

	// 项目内部依赖
	"github.com/c1cada/NexusTok/dto"                    // 数据传输对象定义
	relaycommon "github.com/c1cada/NexusTok/relay/common"  // Relay 通用模块
	"github.com/c1cada/NexusTok/service"               // 服务层工具函数
	"github.com/c1cada/NexusTok/types"                 // 公共类型定义

	// 第三方依赖
	"github.com/gin-gonic/gin" // Gin Web 框架
)

// MiniMaxTTSRequest MiniMax 语音合成 API 的请求结构体。
// 包含模型、文本、语音设置、音频设置、音色权重等配置。
type MiniMaxTTSRequest struct {
	Model             string             `json:"model"`
	Text              string             `json:"text"`
	Stream            bool               `json:"stream,omitempty"`
	StreamOptions     *StreamOptions     `json:"stream_options,omitempty"`
	VoiceSetting      VoiceSetting       `json:"voice_setting"`
	PronunciationDict *PronunciationDict `json:"pronunciation_dict,omitempty"`
	AudioSetting      *AudioSetting      `json:"audio_setting,omitempty"`
	TimbreWeights     []TimbreWeight     `json:"timbre_weights,omitempty"`
	LanguageBoost     string             `json:"language_boost,omitempty"`
	VoiceModify       *VoiceModify       `json:"voice_modify,omitempty"`
	SubtitleEnable    bool               `json:"subtitle_enable,omitempty"`
	OutputFormat      string             `json:"output_format,omitempty"`
	AigcWatermark     bool               `json:"aigc_watermark,omitempty"`
}

// StreamOptions 流式传输选项。
type StreamOptions struct {
	ExcludeAggregatedAudio bool `json:"exclude_aggregated_audio,omitempty"`
}

// VoiceSetting 语音设置，包含音色 ID、语速、音量、音调等参数。
type VoiceSetting struct {
	VoiceID           string  `json:"voice_id"`
	Speed             float64 `json:"speed,omitempty"`
	Vol               float64 `json:"vol,omitempty"`
	Pitch             int     `json:"pitch,omitempty"`
	Emotion           string  `json:"emotion,omitempty"`
	TextNormalization bool    `json:"text_normalization,omitempty"`
	LatexRead         bool    `json:"latex_read,omitempty"`
}

// PronunciationDict 发音词典，用于自定义特定词语的发音。
type PronunciationDict struct {
	Tone []string `json:"tone,omitempty"`
}

// AudioSetting 音频输出设置，包含采样率、比特率、格式、声道等参数。
type AudioSetting struct {
	SampleRate int    `json:"sample_rate,omitempty"`
	Bitrate    int    `json:"bitrate,omitempty"`
	Format     string `json:"format,omitempty"`
	Channel    int    `json:"channel,omitempty"`
	ForceCbr   bool   `json:"force_cbr,omitempty"`
}

// TimbreWeight 音色权重，用于多音色混合场景。
type TimbreWeight struct {
	VoiceID string `json:"voice_id"`
	Weight  int    `json:"weight"`
}

// VoiceModify 语音变调/变声参数。
type VoiceModify struct {
	Pitch        int    `json:"pitch,omitempty"`
	Intensity    int    `json:"intensity,omitempty"`
	Timbre       int    `json:"timbre,omitempty"`
	SoundEffects string `json:"sound_effects,omitempty"`
}

// MiniMaxTTSResponse MiniMax TTS API 的响应结构体。
type MiniMaxTTSResponse struct {
	Data      MiniMaxTTSData   `json:"data"`
	ExtraInfo MiniMaxExtraInfo `json:"extra_info"`
	TraceID   string           `json:"trace_id"`
	BaseResp  MiniMaxBaseResp  `json:"base_resp"`
}

// MiniMaxTTSData TTS 响应中的音频数据。
type MiniMaxTTSData struct {
	Audio  string `json:"audio"`
	Status int    `json:"status"`
}

// MiniMaxExtraInfo TTS 响应中的额外信息，包含使用字符数。
type MiniMaxExtraInfo struct {
	UsageCharacters int64 `json:"usage_characters"`
}

// MiniMaxBaseResp MiniMax API 的基础响应状态。
// status_code 为 0 表示成功，非 0 表示错误。
type MiniMaxBaseResp struct {
	StatusCode int64  `json:"status_code"`
	StatusMsg  string `json:"status_msg"`
}

// getContentTypeByFormat 根据音频格式返回对应的 HTTP Content-Type。
// 支持 mp3、wav、flac、aac、pcm 格式，默认返回 mp3 的 Content-Type。
// 参数:
//   - format: 音频格式字符串
// 返回:
//   - string: 对应的 MIME 类型
func getContentTypeByFormat(format string) string {
	contentTypeMap := map[string]string{
		"mp3":  "audio/mpeg",
		"wav":  "audio/wav",
		"flac": "audio/flac",
		"aac":  "audio/aac",
		"pcm":  "audio/pcm",
	}
	if ct, ok := contentTypeMap[format]; ok {
		return ct
	}
	return "audio/mpeg" // default to mp3
}

// handleTTSResponse 处理 MiniMax TTS API 的 HTTP 响应。
// 流程：读取响应体 -> 解析 JSON -> 检查错误状态 -> 处理音频数据。
// 音频数据处理方式：
//   - HTTP URL: 通过 302 重定向将客户端导向音频地址
//   - Hex 编码: 解码为二进制音频数据并直接返回
//
// 参数:
//   - c: Gin 上下文，用于写入响应
//   - resp: MiniMax API 返回的 HTTP 响应
//   - info: Relay 信息，用于估算 token 使用量
// 返回:
//   - usage: token 使用量信息（使用字符数作为 token 数）
//   - err: 错误信息
func handleTTSResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NexusTokError) {
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, types.NewErrorWithStatusCode(
			fmt.Errorf("failed to read minimax response: %w", readErr),
			types.ErrorCodeReadResponseBodyFailed,
			http.StatusInternalServerError,
		)
	}
	defer resp.Body.Close()

	// Parse response
	var minimaxResp MiniMaxTTSResponse
	if unmarshalErr := json.Unmarshal(body, &minimaxResp); unmarshalErr != nil {
		return nil, types.NewErrorWithStatusCode(
			fmt.Errorf("failed to unmarshal minimax TTS response: %w", unmarshalErr),
			types.ErrorCodeBadResponseBody,
			http.StatusInternalServerError,
		)
	}

	// Check base_resp status code
	if minimaxResp.BaseResp.StatusCode != 0 {
		return nil, types.NewErrorWithStatusCode(
			fmt.Errorf("minimax TTS error: %d - %s", minimaxResp.BaseResp.StatusCode, minimaxResp.BaseResp.StatusMsg),
			types.ErrorCodeBadResponse,
			http.StatusBadRequest,
		)
	}

	// Check if we have audio data
	if minimaxResp.Data.Audio == "" {
		return nil, types.NewErrorWithStatusCode(
			fmt.Errorf("no audio data in minimax TTS response"),
			types.ErrorCodeBadResponse,
			http.StatusBadRequest,
		)
	}

	if strings.HasPrefix(minimaxResp.Data.Audio, "http") {
		c.Redirect(http.StatusFound, minimaxResp.Data.Audio)
	} else {
		// Handle hex-encoded audio data
		audioData, decodeErr := hex.DecodeString(minimaxResp.Data.Audio)
		if decodeErr != nil {
			return nil, types.NewErrorWithStatusCode(
				fmt.Errorf("failed to decode hex audio data: %w", decodeErr),
				types.ErrorCodeBadResponse,
				http.StatusInternalServerError,
			)
		}

		// Determine content type - default to mp3
		contentType := "audio/mpeg"

		c.Data(http.StatusOK, contentType, audioData)
	}

	usage = &dto.Usage{
		PromptTokens:     info.GetEstimatePromptTokens(),
		CompletionTokens: 0,
		TotalTokens:      int(minimaxResp.ExtraInfo.UsageCharacters),
	}

	return usage, nil
}

// handleChatCompletionResponse 处理 MiniMax 对话补全 API 的 HTTP 响应。
// 直接透传上游响应，复制允许的响应头并返回 JSON 数据。
// 参数:
//   - c: Gin 上下文，用于写入响应
//   - resp: MiniMax API 返回的 HTTP 响应
//   - info: Relay 信息
// 返回:
//   - usage: nil（对话补全的 usage 包含在响应体中）
//   - err: nil（错误通过 HTTP 状态码传递）
func handleChatCompletionResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NexusTokError) {
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, types.NewErrorWithStatusCode(
			errors.New("failed to read minimax response"),
			types.ErrorCodeReadResponseBodyFailed,
			http.StatusInternalServerError,
		)
	}
	defer resp.Body.Close()

	// Set response headers
	for key, values := range resp.Header {
		if !service.ShouldCopyUpstreamHeader(c, key, values) {
			continue
		}
		for _, value := range values {
			c.Header(key, value)
		}
	}

	c.Data(resp.StatusCode, "application/json", body)
	return nil, nil
}
