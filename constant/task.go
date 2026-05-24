// Package constant - task.go
// 该文件定义了异步任务相关的常量
//
// 异步任务用于处理需要较长时间的操作：
// - Suno 音乐生成
// - Midjourney 图像生成
// - 其他需要轮询结果的 AI 生成任务
//
// 任务生命周期：
// 1. 创建任务（提交到上游服务）
// 2. 轮询任务状态
// 3. 获取任务结果
// 4. 返回结果给用户
package constant

// TaskPlatform 任务平台类型
type TaskPlatform string

const (
	// TaskPlatformSuno Suno 音乐生成平台
	TaskPlatformSuno TaskPlatform = "suno"
	// TaskPlatformMidjourney Midjourney 图像生成平台
	TaskPlatformMidjourney = "mj"
)

// Suno 操作类型常量
const (
	// SunoActionMusic 音乐生成操作
	SunoActionMusic = "MUSIC"
	// SunoActionLyrics 歌词生成操作
	SunoActionLyrics = "LYRICS"
)

// 通用任务操作类型常量
const (
	// TaskActionGenerate 通用生成操作
	TaskActionGenerate = "generate"
	// TaskActionTextGenerate 文本生成操作
	TaskActionTextGenerate = "textGenerate"
	// TaskActionFirstTailGenerate 首尾生成操作
	TaskActionFirstTailGenerate = "firstTailGenerate"
	// TaskActionReferenceGenerate 参考生成操作
	TaskActionReferenceGenerate = "referenceGenerate"
	// TaskActionRemix 混音/重混操作
	TaskActionRemix = "remixGenerate"
)

// SunoModel2Action Suno 模型名称到操作类型的映射
// 用于将 API 请求中的模型名称转换为实际的 Suno 操作
var SunoModel2Action = map[string]string{
	"suno_music":  SunoActionMusic,
	"suno_lyrics": SunoActionLyrics,
}
