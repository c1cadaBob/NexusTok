// Package moonshot 的常量定义文件。
// 定义 Moonshot (Kimi) 渠道支持的模型列表和渠道名称。
package moonshot

// ModelList 是 Moonshot 支持的模型名称列表。
// 包含 Kimi K2 系列的各个版本：
//   - kimi-k2.5: Kimi K2.5 主版本
//   - kimi-k2-0905-preview: Kimi K2 预览版
//   - kimi-k2-turbo-preview: Kimi K2 Turbo 预览版（更快响应）
//   - kimi-k2-thinking: Kimi K2 思考版（支持推理链）
//   - kimi-k2-thinking-turbo: Kimi K2 思考 Turbo 版（更快的推理）
var ModelList = []string{
	"kimi-k2.5",
	"kimi-k2-0905-preview",
	"kimi-k2-turbo-preview",
	"kimi-k2-thinking",
	"kimi-k2-thinking-turbo",
}

// ChannelName 是 Moonshot 渠道的标识名称。
var ChannelName = "moonshot"
