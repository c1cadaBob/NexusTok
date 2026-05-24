// Package dto - suno.go
// 该文件定义了 Suno AI 音乐生成 API 的数据传输对象
//
// 主要结构体：
// - SunoSubmitReq：Suno 音乐生成请求（v3 API）
// - SunoDataResponse：Suno 任务响应（包含状态和生成的歌曲数据）
// - SunoSong：Suno 生成的歌曲（音频、视频、图像 URL 等）
// - SunoMetadata：歌曲元数据（标签、提示词等）
// - SunoLyrics：歌词信息
// - SunoGoAPISubmitReq/GoAPITaskResponse：GoAPI 兼容的请求和响应格式
//
// 任务状态：submitted -> queueing -> processing -> success/failed
// 任务类型：song（歌曲生成）、lyrics（歌词生成）、description-mode（描述模式）
package dto

import (
	"encoding/json"
)

// SunoSubmitReq Suno 音乐生成请求（v3 API）
// GptDescriptionPrompt：GPT 描述提示词（用于 AI 描述模式生成歌曲）
// Prompt：音乐生成提示词（歌词或风格描述）
// Mv：模型版本（如 "v3"、"v3.5" 等）
// Title：歌曲标题
// Tags：风格标签（如 "pop, rock, upbeat"）
// ContinueAt：续写起始时间点（秒，用于歌曲续写）
// TaskID：任务 ID（续写时使用）
// ContinueClipId：续写片段 ID（续写时使用）
// MakeInstrumental：是否生成纯音乐（无歌词）
type SunoSubmitReq struct {
	GptDescriptionPrompt string  `json:"gpt_description_prompt,omitempty"`
	Prompt               string  `json:"prompt,omitempty"`
	Mv                   string  `json:"mv,omitempty"`
	Title                string  `json:"title,omitempty"`
	Tags                 string  `json:"tags,omitempty"`
	ContinueAt           float64 `json:"continue_at,omitempty"`
	TaskID               string  `json:"task_id,omitempty"`
	ContinueClipId       string  `json:"continue_clip_id,omitempty"`
	MakeInstrumental     bool    `json:"make_instrumental"`
}

// SunoDataResponse Suno 任务响应
// TaskID：任务 ID
// Action：任务类型（song/lyrics/description-mode）
// Status：任务状态（submitted/queueing/processing/success/failed）
// FailReason：失败原因（失败时）
// SubmitTime/StartTime/FinishTime：提交/开始/完成时间戳
// Data：生成的数据（JSON 格式，包含歌曲列表）
type SunoDataResponse struct {
	TaskID     string          `json:"task_id" gorm:"type:varchar(50);index"`
	Action     string          `json:"action" gorm:"type:varchar(40);index"` // 任务类型, song, lyrics, description-mode
	Status     string          `json:"status" gorm:"type:varchar(20);index"` // 任务状态, submitted, queueing, processing, success, failed
	FailReason string          `json:"fail_reason"`
	SubmitTime int64           `json:"submit_time" gorm:"index"`
	StartTime  int64           `json:"start_time" gorm:"index"`
	FinishTime int64           `json:"finish_time" gorm:"index"`
	Data       json.RawMessage `json:"data" gorm:"type:json"`
}

// SunoSong Suno 生成的歌曲
// ID：歌曲唯一标识
// VideoURL：视频 URL（MV）
// AudioURL：音频 URL
// ImageURL：封面图像 URL
// ImageLargeURL：大尺寸封面图像 URL
// MajorModelVersion：主模型版本
// ModelName：模型名称
// Status：歌曲状态
// Title：歌曲标题
// Text：歌词文本
// Metadata：歌曲元数据
type SunoSong struct {
	ID                string       `json:"id"`
	VideoURL          string       `json:"video_url"`
	AudioURL          string       `json:"audio_url"`
	ImageURL          string       `json:"image_url"`
	ImageLargeURL     string       `json:"image_large_url"`
	MajorModelVersion string       `json:"major_model_version"`
	ModelName         string       `json:"model_name"`
	Status            string       `json:"status"`
	Title             string       `json:"title"`
	Text              string       `json:"text"`
	Metadata          SunoMetadata `json:"metadata"`
}

// SunoMetadata Suno 歌曲元数据
// Tags：风格标签
// Prompt：生成提示词
// GPTDescriptionPrompt：GPT 描述提示词
// AudioPromptID：音频提示 ID（续写场景）
// Duration：歌曲时长
// ErrorType/ErrorMessage：错误信息（失败时）
type SunoMetadata struct {
	Tags                 string      `json:"tags"`
	Prompt               string      `json:"prompt"`
	GPTDescriptionPrompt interface{} `json:"gpt_description_prompt"`
	AudioPromptID        interface{} `json:"audio_prompt_id"`
	Duration             interface{} `json:"duration"`
	ErrorType            interface{} `json:"error_type"`
	ErrorMessage         interface{} `json:"error_message"`
}

// SunoLyrics Suno 歌词
// ID：歌词唯一标识
// Status：状态
// Title：歌曲标题
// Text：歌词文本
type SunoLyrics struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Title  string `json:"title"`
	Text   string `json:"text"`
}

// SunoGoAPISubmitReq GoAPI 兼容的 Suno 音乐生成请求
// CustomMode：是否使用自定义模式（true 时需要手动指定提示词和标签）
// Input：输入参数
// NotifyHook：通知回调 URL
type SunoGoAPISubmitReq struct {
	CustomMode bool `json:"custom_mode"`

	Input SunoGoAPISubmitReqInput `json:"input"`

	NotifyHook string `json:"notify_hook,omitempty"`
}

// SunoGoAPISubmitReqInput GoAPI 兼容的 Suno 输入参数
// GptDescriptionPrompt：GPT 描述提示词
// Prompt：音乐生成提示词
// Mv：模型版本
// Title：歌曲标题
// Tags：风格标签
// ContinueAt：续写起始时间点
// TaskID：任务 ID
// ContinueClipId：续写片段 ID
// MakeInstrumental：是否生成纯音乐
type SunoGoAPISubmitReqInput struct {
	GptDescriptionPrompt string  `json:"gpt_description_prompt"`
	Prompt               string  `json:"prompt"`
	Mv                   string  `json:"mv"`
	Title                string  `json:"title"`
	Tags                 string  `json:"tags"`
	ContinueAt           float64 `json:"continue_at"`
	TaskID               string  `json:"task_id"`
	ContinueClipId       string  `json:"continue_clip_id"`
	MakeInstrumental     bool    `json:"make_instrumental"`
}

// GoAPITaskResponse GoAPI 任务响应（泛型）
// Code：响应状态码
// Message：响应消息
// Data：响应数据（泛型）
// ErrorMessage：错误消息（失败时）
type GoAPITaskResponse[T any] struct {
	Code         int    `json:"code"`
	Message      string `json:"message"`
	Data         T      `json:"data"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// GoAPITaskResponseData GoAPI 任务提交响应数据
// TaskID：任务 ID
type GoAPITaskResponseData struct {
	TaskID string `json:"task_id"`
}

// GoAPIFetchResponseData GoAPI 任务查询响应数据
// TaskID：任务 ID
// Status：任务状态
// Input：输入参数
// Clips：生成的歌曲片段映射（key 为片段 ID）
type GoAPIFetchResponseData struct {
	TaskID string              `json:"task_id"`
	Status string              `json:"status"`
	Input  string              `json:"input"`
	Clips  map[string]SunoSong `json:"clips"`
}
