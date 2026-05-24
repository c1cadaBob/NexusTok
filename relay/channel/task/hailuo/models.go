// Package hailuo 实现海螺 AI 视频生成渠道的数据模型定义
package hailuo

// SubjectReference 主体参考图片结构体
// 用于主体参考视频生成模式
type SubjectReference struct {
	Type  string   `json:"type"`  // 主体类型，目前仅支持 "character"
	Image []string `json:"image"` // 主体参考图片数组（目前仅支持单张图片）
}

// VideoRequest 视频生成请求结构体
type VideoRequest struct {
	Model            string             `json:"model"`                        // 模型名称
	Prompt           string             `json:"prompt,omitempty"`             // 文本提示词
	PromptOptimizer  *bool              `json:"prompt_optimizer,omitempty"`   // 是否启用提示词优化
	FastPretreatment *bool              `json:"fast_pretreatment,omitempty"`  // 是否启用快速预处理
	Duration         *int               `json:"duration,omitempty"`           // 视频时长（秒）
	Resolution       string             `json:"resolution,omitempty"`         // 视频分辨率
	CallbackURL      string             `json:"callback_url,omitempty"`       // 回调通知 URL
	AigcWatermark    *bool              `json:"aigc_watermark,omitempty"`     // 是否添加 AIGC 水印
	FirstFrameImage  string             `json:"first_frame_image,omitempty"`  // 首帧图片（用于图片转视频）
	LastFrameImage   string             `json:"last_frame_image,omitempty"`   // 末帧图片（用于首尾帧视频）
	SubjectReference []SubjectReference `json:"subject_reference,omitempty"` // 主体参考（用于主体参考视频）
}

// VideoResponse 视频生成响应结构体
type VideoResponse struct {
	TaskID   string   `json:"task_id"`  // 任务 ID
	BaseResp BaseResp `json:"base_resp"` // 基础响应信息
}

// BaseResp 基础响应结构体
type BaseResp struct {
	StatusCode int    `json:"status_code"` // 状态码
	StatusMsg  string `json:"status_msg"`  // 状态消息
}

// QueryTaskRequest 查询任务请求结构体
type QueryTaskRequest struct {
	TaskID string `json:"task_id"` // 任务 ID
}

// QueryTaskResponse 查询任务响应结构体
type QueryTaskResponse struct {
	TaskID      string   `json:"task_id"`        // 任务 ID
	Status      string   `json:"status"`         // 任务状态
	FileID      string   `json:"file_id,omitempty"` // 文件 ID
	VideoWidth  int      `json:"video_width,omitempty"`  // 视频宽度
	VideoHeight int      `json:"video_height,omitempty"` // 视频高度
	BaseResp    BaseResp `json:"base_resp"`      // 基础响应信息
}

// ErrorInfo 错误信息结构体
type ErrorInfo struct {
	StatusCode int    `json:"status_code"` // 错误状态码
	StatusMsg  string `json:"status_msg"`  // 错误消息
}

// TaskStatusInfo 任务状态信息结构体
type TaskStatusInfo struct {
	TaskID    string `json:"task_id"`          // 任务 ID
	Status    string `json:"status"`           // 任务状态
	FileID    string `json:"file_id,omitempty"`    // 文件 ID
	VideoURL  string `json:"video_url,omitempty"`  // 视频 URL
	ErrorCode int    `json:"error_code,omitempty"` // 错误代码
	ErrorMsg  string `json:"error_msg,omitempty"`  // 错误消息
}

// ModelConfig 模型配置结构体
// 定义每个模型支持的参数范围
type ModelConfig struct {
	Name                 string   // 模型名称
	DefaultResolution    string   // 默认分辨率
	SupportedDurations   []int    // 支持的视频时长列表
	SupportedResolutions []string // 支持的分辨率列表
	HasPromptOptimizer   bool     // 是否支持提示词优化
	HasFastPretreatment  bool     // 是否支持快速预处理
}

// RetrieveFileResponse 文件检索响应结构体
type RetrieveFileResponse struct {
	File     FileObject `json:"file"`      // 文件对象
	BaseResp BaseResp   `json:"base_resp"` // 基础响应信息
}

// FileObject 文件对象结构体
type FileObject struct {
	FileID      int64  `json:"file_id"`      // 文件 ID
	Bytes       int64  `json:"bytes"`        // 文件大小（字节）
	CreatedAt   int64  `json:"created_at"`   // 创建时间戳
	Filename    string `json:"filename"`     // 文件名
	Purpose     string `json:"purpose"`      // 文件用途
	DownloadURL string `json:"download_url"` // 下载 URL
}

// GetModelConfig 获取指定模型的配置信息
// 如果模型未找到，返回默认配置
//
// 参数：
//   - model: 模型名称
//
// 返回值：
//   - ModelConfig: 模型配置信息
func GetModelConfig(model string) ModelConfig {
	configs := map[string]ModelConfig{
		"MiniMax-Hailuo-2.3": {
			Name:                 "MiniMax-Hailuo-2.3",
			DefaultResolution:    Resolution768P,
			SupportedDurations:   []int{6, 10},
			SupportedResolutions: []string{Resolution768P, Resolution1080P},
			HasPromptOptimizer:   true,
			HasFastPretreatment:  true,
		},
		"MiniMax-Hailuo-2.3-Fast": {
			Name:                 "MiniMax-Hailuo-2.3-Fast",
			DefaultResolution:    Resolution768P,
			SupportedDurations:   []int{6, 10},
			SupportedResolutions: []string{Resolution768P, Resolution1080P},
			HasPromptOptimizer:   true,
			HasFastPretreatment:  true,
		},
		"MiniMax-Hailuo-02": {
			Name:                 "MiniMax-Hailuo-02",
			DefaultResolution:    Resolution768P,
			SupportedDurations:   []int{6, 10},
			SupportedResolutions: []string{Resolution512P, Resolution768P, Resolution1080P},
			HasPromptOptimizer:   true,
			HasFastPretreatment:  true,
		},
		"T2V-01-Director": {
			Name:                 "T2V-01-Director",
			DefaultResolution:    Resolution768P,
			SupportedDurations:   []int{6},
			SupportedResolutions: []string{Resolution768P, Resolution1080P},
			HasPromptOptimizer:   true,
			HasFastPretreatment:  false,
		},
		"T2V-01": {
			Name:                 "T2V-01",
			DefaultResolution:    Resolution720P,
			SupportedDurations:   []int{6},
			SupportedResolutions: []string{Resolution720P},
			HasPromptOptimizer:   true,
			HasFastPretreatment:  false,
		},
		"I2V-01-Director": {
			Name:                 "I2V-01-Director",
			DefaultResolution:    Resolution720P,
			SupportedDurations:   []int{6},
			SupportedResolutions: []string{Resolution720P, Resolution1080P},
			HasPromptOptimizer:   true,
			HasFastPretreatment:  false,
		},
		"I2V-01-live": {
			Name:                 "I2V-01-live",
			DefaultResolution:    Resolution720P,
			SupportedDurations:   []int{6},
			SupportedResolutions: []string{Resolution720P, Resolution1080P},
			HasPromptOptimizer:   true,
			HasFastPretreatment:  false,
		},
		"I2V-01": {
			Name:                 "I2V-01",
			DefaultResolution:    Resolution720P,
			SupportedDurations:   []int{6},
			SupportedResolutions: []string{Resolution720P, Resolution1080P},
			HasPromptOptimizer:   true,
			HasFastPretreatment:  false,
		},
		"S2V-01": {
			Name:                 "S2V-01",
			DefaultResolution:    Resolution720P,
			SupportedDurations:   []int{6},
			SupportedResolutions: []string{Resolution720P},
			HasPromptOptimizer:   true,
			HasFastPretreatment:  false,
		},
	}

	if config, exists := configs[model]; exists {
		return config
	}

	// 返回默认配置
	return ModelConfig{
		Name:                 model,
		DefaultResolution:    DefaultResolution,
		SupportedDurations:   []int{6},
		SupportedResolutions: []string{DefaultResolution},
		HasPromptOptimizer:   true,
		HasFastPretreatment:  false,
	}
}
