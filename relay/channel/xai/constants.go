// Package xai 的常量定义文件。
// 定义了 xAI（Grok）渠道支持的模型列表和渠道名称。

package xai

// ModelList 是 xAI 渠道支持的模型列表。
// 包含以下类别：
// - 语言模型：Grok-4/3/2 系列，支持推理和非推理模式
// - 搜索增强变体：模型名带 "-search" 后缀，支持联网搜索
// - 推理努力变体：grok-3-mini 支持 "-high"/"-low" 推理深度
// - 图片生成模型：Grok Imagine 系列
// - 视频生成模型：Grok Imagine Video
var ModelList = []string{
	// 语言模型
	"grok-4-1-fast-reasoning",
	"grok-4-1-fast-non-reasoning",
	"grok-code-fast-1",
	"grok-4-fast-reasoning",
	"grok-4-fast-non-reasoning",
	"grok-4-0709",
	"grok-3-mini",
	"grok-3",
	"grok-2-vision-1212",
	// 搜索增强变体（支持联网搜索）
	"grok-4-1-fast-reasoning-search",
	"grok-4-1-fast-non-reasoning-search",
	"grok-4-fast-reasoning-search",
	"grok-4-fast-non-reasoning-search",
	"grok-4-0709-search",
	"grok-3-mini-search",
	"grok-3-search",
	// grok-3-mini 推理努力变体（控制推理深度）
	"grok-3-mini-high", "grok-3-mini-low",
	// 图片生成模型
	"grok-imagine-image-pro",
	"grok-imagine-image",
	"grok-2-image-1212",
	// 视频生成模型
	"grok-imagine-video",
}

// ChannelName 渠道名称标识，用于路由和日志中识别 xAI 渠道。
var ChannelName = "xai"
