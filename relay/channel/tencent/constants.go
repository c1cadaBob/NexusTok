// Package tencent 的常量定义文件。
// 定义了腾讯云混元渠道支持的模型列表和渠道名称。

package tencent

// ModelList 是腾讯云混元渠道支持的模型列表。
// 包含混元系列的大语言模型，支持不同上下文长度和能力级别。
var ModelList = []string{
	"hunyuan-lite",          // 轻量版，适合简单对话
	"hunyuan-standard",      // 标准版
	"hunyuan-standard-256K", // 标准版，支持 256K 上下文
	"hunyuan-pro",           // 专业版，最高能力
}

// ChannelName 渠道名称标识，用于路由和日志中识别腾讯云渠道。
var ChannelName = "tencent"
