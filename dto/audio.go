// Package dto - audio.go
// 该文件定义了音频相关 API 的数据传输对象
//
// 主要结构体：
// - AudioRequest：音频生成/语音合成请求（支持 TTS、Whisper 等）
// - AudioResponse：音频转文本响应
// - WhisperVerboseJSONResponse：Whisper 详细 JSON 格式响应（包含时间戳分段信息）
// - Segment：音频转录的时间片段（包含起止时间、文本、token 等）
//
// 兼容性说明：
// - 支持 vllm-omni 扩展字段（task_type、language、ref_audio 等）
// - Speed 字段使用指针类型以区分"未设置"和"设置为 0"的情况
package dto

import (
	"encoding/json"
	"strings"

	"github.com/c1cada/NexusTok/types"

	"github.com/gin-gonic/gin"
)

// AudioRequest 音频生成请求结构体
// 支持 TTS（文本转语音）和 ASR（语音转文本/Whisper）两种场景
// Model：目标模型名称（如 tts-1、whisper-1 等）
// Input：输入文本（TTS 场景）或待转录音频描述（ASR 场景由上游适配）
// Voice：语音角色名称（TTS 场景，如 alloy、echo 等）
// Instructions：语音风格指令（部分 TTS 模型支持）
// ResponseFormat：响应格式（如 mp3、opus、wav、verbose_json 等）
// Speed：语速倍率（指针类型，nil 表示使用默认值）
// StreamFormat：流式格式（"sse" 表示启用服务端事件流）
// Metadata：扩展元数据（透传给上游）
// vllm-omni 扩展字段：TaskType、Language、RefAudio、RefText 等用于语音克隆场景
type AudioRequest struct {
	Model          string          `json:"model"`
	Input          string          `json:"input"`
	Voice          string          `json:"voice"`
	Instructions   string          `json:"instructions,omitempty"`
	ResponseFormat string          `json:"response_format,omitempty"`
	Speed          *float64        `json:"speed,omitempty"`
	StreamFormat   string          `json:"stream_format,omitempty"`
	Metadata       json.RawMessage `json:"metadata,omitempty"`
	// vllm-omini
	TaskType                json.RawMessage `json:"task_type,omitempty"`
	Language                json.RawMessage `json:"language,omitempty"`
	RefAudio                json.RawMessage `json:"ref_audio,omitempty"`
	RefText                 json.RawMessage `json:"ref_text,omitempty"`
	XVectorOnlyMode         json.RawMessage `json:"x_vector_only_mode,omitempty"`
	MaxNewTokens            json.RawMessage `json:"max_new_tokens,omitempty"`
	InitialCodecChunkFrames json.RawMessage `json:"initial_codec_chunk_frames,omitempty"`
	// TODO：ensure that the logic remains correct after the stream is started.
	//Stream                  json.RawMessage `json:"stream,omitempty"`
}

// GetTokenCountMeta 获取音频请求的 Token 计数元数据
// 对于 GPT 系列模型使用 Tokenizer 方式精确计算，其他模型使用文本字符数估算
func (r *AudioRequest) GetTokenCountMeta() *types.TokenCountMeta {
	meta := &types.TokenCountMeta{
		CombineText: r.Input,
		TokenType:   types.TokenTypeTextNumber,
	}
	if strings.Contains(r.Model, "gpt") {
		meta.TokenType = types.TokenTypeTokenizer
	}
	return meta
}

// IsStream 判断音频请求是否为流式模式
// 当 StreamFormat 为 "sse" 时返回 true，表示以 Server-Sent Events 方式流式返回音频
func (r *AudioRequest) IsStream(c *gin.Context) bool {
	return r.StreamFormat == "sse"
}

// SetModelName 设置音频请求的模型名称
// 仅在 modelName 非空时更新，用于上游模型映射或路由替换
func (r *AudioRequest) SetModelName(modelName string) {
	if modelName != "" {
		r.Model = modelName
	}
}

// AudioResponse 音频转文本的简单响应结构体
// Text：识别出的文本内容
type AudioResponse struct {
	Text string `json:"text"`
}

// WhisperVerboseJSONResponse Whisper 详细 JSON 格式响应
// 包含完整的转录信息：任务类型、语言检测、音频时长、全文文本和时间片段列表
// Task：任务类型（transcribe 或 translate）
// Language：检测到的语言代码
// Duration：音频总时长（秒）
// Text：完整转录文本
// Segments：时间片段列表（包含每个片段的详细信息）
type WhisperVerboseJSONResponse struct {
	Task     string    `json:"task,omitempty"`
	Language string    `json:"language,omitempty"`
	Duration float64   `json:"duration,omitempty"`
	Text     string    `json:"text,omitempty"`
	Segments []Segment `json:"segments,omitempty"`
}

// Segment 音频转录的时间片段
// 每个片段对应音频中的一段连续语音
// Id：片段序号
// Seek：音频中的字节偏移量
// Start/End：片段的起止时间（秒）
// Text：该片段的转录文本
// Tokens：该片段对应的 token ID 列表
// Temperature：解码温度
// AvgLogprob：平均对数概率（越接近 0 越可信）
// CompressionRatio：压缩比（用于检测重复/低质量片段）
// NoSpeechProb：无语音概率（用于过滤静音段）
type Segment struct {
	Id               int     `json:"id"`
	Seek             int     `json:"seek"`
	Start            float64 `json:"start"`
	End              float64 `json:"end"`
	Text             string  `json:"text"`
	Tokens           []int   `json:"tokens"`
	Temperature      float64 `json:"temperature"`
	AvgLogprob       float64 `json:"avg_logprob"`
	CompressionRatio float64 `json:"compression_ratio"`
	NoSpeechProb     float64 `json:"no_speech_prob"`
}
