// Package hailuo 实现海螺 AI（MiniMax）视频生成渠道的适配器
// 支持文本转视频、图片转视频等多种视频生成模式
// API 文档: https://platform.minimaxi.com/docs/api-reference/video-generation-intro
package hailuo

// ChannelName 渠道名称标识
const (
	ChannelName = "hailuo-video"
)

// ModelList 支持的海螺视频生成模型列表
// 包含 Hailuo 2.3 系列、Director 系列和场景转视频模型
var ModelList = []string{
	"MiniMax-Hailuo-2.3",      // Hailuo 2.3 标准版
	"MiniMax-Hailuo-2.3-Fast", // Hailuo 2.3 快速版
	"MiniMax-Hailuo-02",       // Hailuo 02 版本
	"T2V-01-Director",         // 文本转视频（导演模式）
	"T2V-01",                  // 文本转视频
	"I2V-01-Director",         // 图片转视频（导演模式）
	"I2V-01-live",             // 图片转视频（直播模式）
	"I2V-01",                  // 图片转视频
	"S2V-01",                  // 场景转视频
}

// API 端点常量
const (
	TextToVideoEndpoint = "/v1/video_generation"      // 文本转视频端点
	QueryTaskEndpoint   = "/v1/query/video_generation" // 查询任务端点
)

// API 响应状态码
const (
	StatusSuccess    = 0    // 成功
	StatusRateLimit  = 1002 // 速率限制
	StatusAuthFailed = 1004 // 认证失败
	StatusNoBalance  = 1008 // 余额不足
	StatusSensitive  = 1026 // 内容敏感
	StatusParamError = 2013 // 参数错误
	StatusInvalidKey = 2049 // 无效密钥
)

// 任务状态常量
const (
	TaskStatusPreparing  = "Preparing"  // 准备中
	TaskStatusQueueing   = "Queueing"   // 排队中
	TaskStatusProcessing = "Processing" // 处理中
	TaskStatusSuccess    = "Success"    // 成功
	TaskStatusFailed     = "Fail"       // 失败
)

// 视频分辨率常量
const (
	Resolution512P  = "512P"  // 512P 分辨率
	Resolution720P  = "720P"  // 720P 分辨率
	Resolution768P  = "768P"  // 768P 分辨率
	Resolution1080P = "1080P" // 1080P 分辨率
)

// 默认配置常量
const (
	DefaultDuration   = 6         // 默认视频时长（秒）
	DefaultResolution = Resolution720P // 默认分辨率
)
