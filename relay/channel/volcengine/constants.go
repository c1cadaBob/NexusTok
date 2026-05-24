// Package volcengine 的常量定义文件。
// 定义了火山引擎渠道支持的模型列表和渠道名称。
// 火山引擎是字节跳动的云服务平台，提供豆包系列大模型的推理服务。

package volcengine

// ModelList 是火山引擎渠道支持的模型列表。
// 包含豆包系列的大语言模型（Doubao-pro/lite）、嵌入模型、
// 图片生成模型（Seedream）、视频生成模型（Seedance）和推理模型（Seed-thinking）。
var ModelList = []string{
	"Doubao-pro-128k",                  // 豆包专业版，128K 上下文
	"Doubao-pro-32k",                   // 豆包专业版，32K 上下文
	"Doubao-pro-4k",                    // 豆包专业版，4K 上下文
	"Doubao-lite-128k",                 // 豆包轻量版，128K 上下文
	"Doubao-lite-32k",                  // 豆包轻量版，32K 上下文
	"Doubao-lite-4k",                   // 豆包轻量版，4K 上下文
	"Doubao-embedding",                 // 豆包嵌入模型
	"doubao-seedream-4-0-250828",       // 豆包 Seedream 图片生成模型（带前缀）
	"seedream-4-0-250828",              // Seedream 图片生成模型
	"doubao-seedance-1-0-pro-250528",   // 豆包 Seedance 视频生成模型（带前缀）
	"seedance-1-0-pro-250528",          // Seedance 视频生成模型
	"doubao-seed-1-6-thinking-250715",  // 豆包 Seed 推理模型（带前缀）
	"seed-1-6-thinking-250715",         // Seed 推理模型
}

// ChannelName 渠道名称标识，用于路由和日志中识别火山引擎渠道。
var ChannelName = "volcengine"
