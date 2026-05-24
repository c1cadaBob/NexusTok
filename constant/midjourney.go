// Package constant - midjourney.go
// 该文件定义了 Midjourney 图像生成服务相关的常量
//
// Midjourney 是一个 AI 图像生成服务，支持多种操作：
// - 文本生成图像（Imagine）
// - 图像描述（Describe）
// - 图像混合（Blend）
// - 图像放大（Upscale）
// - 图像变体（Variation）
// - 局部重绘（InPaint）
// - 缩放（Zoom）
// - 平移（Pan）
// - 换脸（SwapFace）
// - 视频生成（Video）
package constant

// Midjourney 错误码常量
const (
	// MjErrorUnknown 未知错误
	MjErrorUnknown = 5
	// MjRequestError 请求错误
	MjRequestError = 4
)

// Midjourney 操作类型常量
const (
	// MjActionImagine 文本生成图像
	MjActionImagine = "IMAGINE"
	// MjActionDescribe 图像描述
	MjActionDescribe = "DESCRIBE"
	// MjActionBlend 图像混合
	MjActionBlend = "BLEND"
	// MjActionUpscale 图像放大
	MjActionUpscale = "UPSCALE"
	// MjActionVariation 图像变体
	MjActionVariation = "VARIATION"
	// MjActionReRoll 重新生成
	MjActionReRoll = "REROLL"
	// MjActionInPaint 局部重绘
	MjActionInPaint = "INPAINT"
	// MjActionModal 模态操作
	MjActionModal = "MODAL"
	// MjActionZoom 缩放操作
	MjActionZoom = "ZOOM"
	// MjActionCustomZoom 自定义缩放
	MjActionCustomZoom = "CUSTOM_ZOOM"
	// MjActionShorten 缩短提示词
	MjActionShorten = "SHORTEN"
	// MjActionHighVariation 高强度变体
	MjActionHighVariation = "HIGH_VARIATION"
	// MjActionLowVariation 低强度变体
	MjActionLowVariation = "LOW_VARIATION"
	// MjActionPan 平移操作
	MjActionPan = "PAN"
	// MjActionSwapFace 换脸操作
	MjActionSwapFace = "SWAP_FACE"
	// MjActionUpload 上传图像
	MjActionUpload = "UPLOAD"
	// MjActionVideo 视频生成
	MjActionVideo = "VIDEO"
	// MjActionEdits 图像编辑
	MjActionEdits = "EDITS"
)

// MidjourneyModel2Action Midjourney 模型名称到操作类型的映射
// 用于将 API 请求中的模型名称转换为实际的 Midjourney 操作
var MidjourneyModel2Action = map[string]string{
	"mj_imagine":        MjActionImagine,
	"mj_describe":       MjActionDescribe,
	"mj_blend":          MjActionBlend,
	"mj_upscale":        MjActionUpscale,
	"mj_variation":      MjActionVariation,
	"mj_reroll":         MjActionReRoll,
	"mj_modal":          MjActionModal,
	"mj_inpaint":        MjActionInPaint,
	"mj_zoom":           MjActionZoom,
	"mj_custom_zoom":    MjActionCustomZoom,
	"mj_shorten":        MjActionShorten,
	"mj_high_variation": MjActionHighVariation,
	"mj_low_variation":  MjActionLowVariation,
	"mj_pan":            MjActionPan,
	"swap_face":         MjActionSwapFace,
	"mj_upload":         MjActionUpload,
	"mj_video":          MjActionVideo,
	"mj_edits":          MjActionEdits,
}
