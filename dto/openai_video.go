// Package dto - openai_video.go
// 该文件定义了 OpenAI 视频生成 API 的数据传输对象
//
// 主要结构体：
// - OpenAIVideo：视频任务结构体（包含状态、进度、时长等信息）
// - OpenAIVideoError：视频任务错误信息
//
// 视频状态常量：
// - VideoStatusUnknown：未知状态
// - VideoStatusQueued：排队中
// - VideoStatusInProgress：生成中
// - VideoStatusCompleted：已完成
// - VideoStatusFailed：失败
package dto

import (
	"strconv"
	"strings"
)

// 视频任务状态常量
const (
	VideoStatusUnknown    = "unknown"     // 未知状态
	VideoStatusQueued     = "queued"      // 排队中
	VideoStatusInProgress = "in_progress" // 生成中
	VideoStatusCompleted  = "completed"   // 已完成
	VideoStatusFailed     = "failed"      // 失败
)

// OpenAIVideo OpenAI 视频任务结构体
// ID：视频唯一标识
// TaskID：任务 ID（兼容旧接口，待废弃）
// Object：对象类型（"video"）
// Model：使用的模型名称
// Status：任务状态（使用 VideoStatus 常量）
// Progress：生成进度（0-100 的整数）
// CreatedAt：创建时间戳
// CompletedAt：完成时间戳
// ExpiresAt：过期时间戳
// Seconds：视频时长
// Size：视频尺寸
// RemixedFromVideoID：混剪来源视频 ID
// Error：错误信息（失败时）
// Metadata：扩展元数据
type OpenAIVideo struct {
	ID                 string            `json:"id"`
	TaskID             string            `json:"task_id,omitempty"` //兼容旧接口 待废弃
	Object             string            `json:"object"`
	Model              string            `json:"model"`
	Status             string            `json:"status"` // Should use VideoStatus constants: VideoStatusQueued, VideoStatusInProgress, VideoStatusCompleted, VideoStatusFailed
	Progress           int               `json:"progress"`
	CreatedAt          int64             `json:"created_at"`
	CompletedAt        int64             `json:"completed_at,omitempty"`
	ExpiresAt          int64             `json:"expires_at,omitempty"`
	Seconds            string            `json:"seconds,omitempty"`
	Size               string            `json:"size,omitempty"`
	RemixedFromVideoID string            `json:"remixed_from_video_id,omitempty"`
	Error              *OpenAIVideoError `json:"error,omitempty"`
	Metadata           map[string]any    `json:"metadata,omitempty"`
}

// SetProgressStr 设置进度（字符串格式，自动去除 % 后缀并转换为整数）
func (m *OpenAIVideo) SetProgressStr(progress string) {
	progress = strings.TrimSuffix(progress, "%")
	m.Progress, _ = strconv.Atoi(progress)
}
// SetMetadata 设置元数据键值对
// 如果 Metadata 为 nil，会自动初始化
func (m *OpenAIVideo) SetMetadata(k string, v any) {
	if m.Metadata == nil {
		m.Metadata = make(map[string]any)
	}
	m.Metadata[k] = v
}
// NewOpenAIVideo 创建新的视频任务实例
// 默认状态为 VideoStatusQueued（排队中）
func NewOpenAIVideo() *OpenAIVideo {
	return &OpenAIVideo{
		Object: "video",
		Status: VideoStatusQueued,
	}
}

// OpenAIVideoError 视频任务错误信息
// Message：错误消息
// Code：错误代码
type OpenAIVideoError struct {
	Message string `json:"message"`
	Code    string `json:"code"`
}
