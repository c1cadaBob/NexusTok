// Package replicate 的常量定义文件。
// 定义了 Replicate 渠道的名称标识和支持的模型列表。
package replicate

const (
	// ChannelName 渠道名称标识，用于路由和日志中识别 Replicate 渠道。
	ChannelName = "replicate"
	// ModelFlux11Pro Replicate 平台默认的图片生成模型，来自 Black Forest Labs 的 Flux 1.1 Pro。
	ModelFlux11Pro = "black-forest-labs/flux-1.1-pro"
)

// ModelList 是 Replicate 渠道当前支持的模型列表。
var ModelList = []string{
	ModelFlux11Pro,
}
