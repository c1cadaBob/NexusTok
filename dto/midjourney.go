// Package dto - midjourney.go
// 该文件定义了 Midjourney 图像生成 API 的数据传输对象
//
// 主要结构体：
// - SwapFaceRequest：换脸请求（源图和目标图的 Base64 编码）
// - MidjourneyRequest：Midjourney 生成请求（提示词、动作、种子等）
// - MidjourneyResponse：Midjourney 响应
// - MidjourneyDto：Midjourney 任务详情（包含状态、进度、图像 URL 等）
// - MidjourneyWithoutStatus：Midjourney 任务记录（不含状态，用于数据库存储）
// - ActionButton：Midjourney 操作按钮（放大、变换、变体等）
// - Properties：Midjourney 属性（最终提示词）
package dto

//type SimpleMjRequest struct {
//	Prompt   string `json:"prompt"`
//	CustomId string `json:"customId"`
//	Action   string `json:"action"`
//	Content  string `json:"content"`
//}

// SwapFaceRequest Midjourney 换脸请求
// SourceBase64：源人脸图像的 Base64 编码
// TargetBase64：目标图像的 Base64 编码
type SwapFaceRequest struct {
	SourceBase64 string `json:"sourceBase64"`
	TargetBase64 string `json:"targetBase64"`
}

// MidjourneyRequest Midjourney 生成请求
// Prompt：图像生成提示词
// CustomId：自定义标识符
// BotType：机器人类型
// NotifyHook：通知回调 URL
// Action：操作类型（IMAGINE/UPSCALE/VARIATION 等）
// Index：操作索引（如放大第几张图）
// State：状态标识
// TaskId：任务 ID（用于查询、取消等操作）
// Base64Array：Base64 编码的图像数组（用于垫图）
// Content：内容（用于描述操作）
// MaskBase64：遮罩图像的 Base64 编码（用于局部重绘）
type MidjourneyRequest struct {
	Prompt      string   `json:"prompt"`
	CustomId    string   `json:"customId"`
	BotType     string   `json:"botType"`
	NotifyHook  string   `json:"notifyHook"`
	Action      string   `json:"action"`
	Index       int      `json:"index"`
	State       string   `json:"state"`
	TaskId      string   `json:"taskId"`
	Base64Array []string `json:"base64Array"`
	Content     string   `json:"content"`
	MaskBase64  string   `json:"maskBase64"`
}

// MidjourneyResponse Midjourney API 响应
// Code：响应状态码
// Description：响应描述
// Properties：属性（扩展信息）
// Result：结果（通常是任务 ID 或图像 URL）
type MidjourneyResponse struct {
	Code        int         `json:"code"`
	Description string      `json:"description"`
	Properties  interface{} `json:"properties"`
	Result      string      `json:"result"`
}

// MidjourneyUploadResponse Midjourney 上传响应
// Code：响应状态码
// Description：响应描述
// Result：上传结果 URL 列表
type MidjourneyUploadResponse struct {
	Code        int      `json:"code"`
	Description string   `json:"description"`
	Result      []string `json:"result"`
}

// MidjourneyResponseWithStatusCode 带 HTTP 状态码的 Midjourney 响应
// StatusCode：HTTP 状态码
// Response：Midjourney 响应内容
type MidjourneyResponseWithStatusCode struct {
	StatusCode int `json:"statusCode"`
	Response   MidjourneyResponse
}

// MidjourneyDto Midjourney 任务详情
// MjId：Midjourney 任务 ID
// Action：操作类型
// CustomId：自定义标识符
// BotType：机器人类型
// Prompt/PromptEn：提示词（中文/英文）
// Description：任务描述
// State：状态标识
// SubmitTime/StartTime/FinishTime：提交/开始/完成时间戳
// ImageUrl/VideoUrl：生成的图像/视频 URL
// VideoUrls：视频 URL 列表
// Status：任务状态（SUBMITTED/IN_PROGRESS/FAILURE/SUCCESS 等）
// Progress：进度百分比
// FailReason：失败原因
// Buttons：操作按钮（放大、变换等）
// MaskBase64：遮罩图像
// Properties：属性（最终提示词等）
type MidjourneyDto struct {
	MjId        string      `json:"id"`
	Action      string      `json:"action"`
	CustomId    string      `json:"customId"`
	BotType     string      `json:"botType"`
	Prompt      string      `json:"prompt"`
	PromptEn    string      `json:"promptEn"`
	Description string      `json:"description"`
	State       string      `json:"state"`
	SubmitTime  int64       `json:"submitTime"`
	StartTime   int64       `json:"startTime"`
	FinishTime  int64       `json:"finishTime"`
	ImageUrl    string      `json:"imageUrl"`
	VideoUrl    string      `json:"videoUrl"`
	VideoUrls   []ImgUrls   `json:"videoUrls"`
	Status      string      `json:"status"`
	Progress    string      `json:"progress"`
	FailReason  string      `json:"failReason"`
	Buttons     any         `json:"buttons"`
	MaskBase64  string      `json:"maskBase64"`
	Properties  *Properties `json:"properties"`
}

// ImgUrls 图像 URL 结构体
// Url：图像 URL 地址
type ImgUrls struct {
	Url string `json:"url"`
}

// MidjourneyStatus Midjourney 状态结构体
// Status：状态码
type MidjourneyStatus struct {
	Status int `json:"status"`
}
// MidjourneyWithoutStatus Midjourney 任务记录（不含状态字段）
// 用于数据库存储，包含完整的任务信息
// Id：数据库自增 ID
// Code：响应状态码
// UserId：用户 ID
// Action：操作类型
// MjId：Midjourney 任务 ID
// Prompt/PromptEn：提示词（中文/英文）
// Description：任务描述
// State：状态标识
// SubmitTime/StartTime/FinishTime：提交/开始/完成时间戳
// ImageUrl：生成的图像 URL
// Progress：进度百分比
// FailReason：失败原因
// ChannelId：使用的渠道 ID
type MidjourneyWithoutStatus struct {
	Id          int    `json:"id"`
	Code        int    `json:"code"`
	UserId      int    `json:"user_id" gorm:"index"`
	Action      string `json:"action"`
	MjId        string `json:"mj_id" gorm:"index"`
	Prompt      string `json:"prompt"`
	PromptEn    string `json:"prompt_en"`
	Description string `json:"description"`
	State       string `json:"state"`
	SubmitTime  int64  `json:"submit_time"`
	StartTime   int64  `json:"start_time"`
	FinishTime  int64  `json:"finish_time"`
	ImageUrl    string `json:"image_url"`
	Progress    string `json:"progress"`
	FailReason  string `json:"fail_reason"`
	ChannelId   int    `json:"channel_id"`
}

// ActionButton Midjourney 操作按钮
// CustomId：自定义标识符
// Emoji：按钮图标
// Label：按钮标签
// Type：按钮类型
// Style：按钮样式
type ActionButton struct {
	CustomId any `json:"customId"`
	Emoji    any `json:"emoji"`
	Label    any `json:"label"`
	Type     any `json:"type"`
	Style    any `json:"style"`
}

// Properties Midjourney 属性结构体
// FinalPrompt：最终生成的提示词（Midjourney 处理后的完整提示词）
// FinalZhPrompt：最终生成的中文提示词
type Properties struct {
	FinalPrompt   string `json:"finalPrompt"`
	FinalZhPrompt string `json:"finalZhPrompt"`
}
