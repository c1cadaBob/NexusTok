// Package baidu 实现百度文心一言（ERNIE）AI 平台的渠道适配器常量定义。
// 包含该渠道支持的模型列表和渠道名称标识。
package baidu

// ModelList 定义百度文心一言渠道支持的模型列表。
// 包含 ERNIE 系列对话模型（如 ERNIE-4.0、ERNIE-3.5）和向量化模型（如 Embedding-V1、bge-large）。
var ModelList = []string{
	"ERNIE-4.0-8K",          // 文心一言 4.0 8K 上下文对话模型
	"ERNIE-3.5-8K",          // 文心一言 3.5 8K 上下文对话模型
	"ERNIE-3.5-8K-0205",     // 文心一言 3.5 8K 0205 版本
	"ERNIE-3.5-8K-1222",     // 文心一言 3.5 8K 1222 版本
	"ERNIE-Bot-8K",          // ERNIE Bot 8K 上下文对话模型
	"ERNIE-3.5-4K-0205",     // 文心一言 3.5 4K 0205 版本
	"ERNIE-Speed-8K",        // ERNIE Speed 8K 快速对话模型
	"ERNIE-Speed-128K",      // ERNIE Speed 128K 长上下文模型
	"ERNIE-Lite-8K-0922",    // ERNIE Lite 8K 0922 轻量版
	"ERNIE-Lite-8K-0308",    // ERNIE Lite 8K 0308 轻量版
	"ERNIE-Tiny-8K",         // ERNIE Tiny 8K 极速版
	"BLOOMZ-7B",             // BLOOMZ 7B 开源对话模型
	"Embedding-V1",          // 百度文心文本向量化模型 V1
	"bge-large-zh",          // BGE Large 中文向量化模型
	"bge-large-en",          // BGE Large 英文向量化模型
	"tao-8k",                // TAO 8K 向量化模型
}

// ChannelName 定义百度文心一言渠道的名称标识，用于渠道注册和路由匹配。
var ChannelName = "baidu"
