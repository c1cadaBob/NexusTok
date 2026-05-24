// Package dto - video.go
// 该文件定义了视频生成 API 的数据传输对象
//
// 主要结构体：
// - VideoRequest：视频生成请求（支持文生视频和图生视频）
// - VideoResponse：视频任务提交响应
// - VideoTaskResponse：视频任务查询响应
// - VideoTaskMetadata：视频任务元数据（实际生成的视频参数）
// - VideoTaskError：视频任务错误信息
//
// 支持的厂商参数通过 Metadata map 透传
package dto

// VideoRequest 视频生成请求
// Model：目标模型/风格 ID（如 kling-v1、sora 等）
// Prompt：文本提示词
// Image：输入图像（URL 或 Base64，用于图生视频）
// Duration：视频时长（秒）
// Width/Height：视频宽高
// Fps：帧率
// Seed：随机种子（用于结果可复现）
// N：生成视频数量
// ResponseFormat：响应格式（url/b64_json）
// User：用户标识
// Metadata：扩展元数据（厂商特定参数，如 negative_prompt、style、quality_level 等）
type VideoRequest struct {
	Model          string         `json:"model,omitempty" example:"kling-v1"`                                                                                                                                    // Model/style ID
	Prompt         string         `json:"prompt,omitempty" example:"宇航员站起身走了"`                                                                                                                                   // Text prompt
	Image          string         `json:"image,omitempty" example:"https://h2.inkwai.com/bs2/upload-ylab-stunt/se/ai_portal_queue_mmu_image_upscale_aiweb/3214b798-e1b4-4b00-b7af-72b5b0417420_raw_image_0.jpg"` // Image input (URL/Base64)
	Duration       float64        `json:"duration" example:"5.0"`                                                                                                                                                // Video duration (seconds)
	Width          int            `json:"width" example:"512"`                                                                                                                                                   // Video width
	Height         int            `json:"height" example:"512"`                                                                                                                                                  // Video height
	Fps            int            `json:"fps,omitempty" example:"30"`                                                                                                                                            // Video frame rate
	Seed           int            `json:"seed,omitempty" example:"20231234"`                                                                                                                                     // Random seed
	N              int            `json:"n,omitempty" example:"1"`                                                                                                                                               // Number of videos to generate
	ResponseFormat string         `json:"response_format,omitempty" example:"url"`                                                                                                                               // Response format
	User           string         `json:"user,omitempty" example:"user-1234"`                                                                                                                                    // User identifier
	Metadata       map[string]any `json:"metadata,omitempty"`                                                                                                                                                    // Vendor-specific/custom params (e.g. negative_prompt, style, quality_level, etc.)
}

// VideoResponse 视频生成提交任务后的响应
type VideoResponse struct {
	TaskId string `json:"task_id"`
	Status string `json:"status"`
}

// VideoTaskResponse 查询视频生成任务状态的响应
type VideoTaskResponse struct {
	TaskId   string             `json:"task_id" example:"abcd1234efgh"` // 任务ID
	Status   string             `json:"status" example:"succeeded"`     // 任务状态
	Url      string             `json:"url,omitempty"`                  // 视频资源URL（成功时）
	Format   string             `json:"format,omitempty" example:"mp4"` // 视频格式
	Metadata *VideoTaskMetadata `json:"metadata,omitempty"`             // 结果元数据
	Error    *VideoTaskError    `json:"error,omitempty"`                // 错误信息（失败时）
}

// VideoTaskMetadata 视频任务元数据
type VideoTaskMetadata struct {
	Duration float64 `json:"duration" example:"5.0"`  // 实际生成的视频时长
	Fps      int     `json:"fps" example:"30"`        // 实际帧率
	Width    int     `json:"width" example:"512"`     // 实际宽度
	Height   int     `json:"height" example:"512"`    // 实际高度
	Seed     int     `json:"seed" example:"20231234"` // 使用的随机种子
}

// VideoTaskError 视频任务错误信息
type VideoTaskError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
