// Package baidu_v2 实现百度文心一言 V2 版本（基于火山引擎）的渠道适配器常量定义。
// V2 版本采用 OpenAI 兼容的 API 格式，相较于 V1 版本支持更多模型和功能。
package baidu_v2

// ModelList 定义百度文心一言 V2 渠道支持的模型列表。
// 包含 ERNIE 4.0/3.5 系列、Speed/Lite 轻量版、角色扮演模型以及 DeepSeek 系列模型。
var ModelList = []string{
	"ernie-4.0-8k-latest",            // ERNIE 4.0 8K 最新版
	"ernie-4.0-8k-preview",           // ERNIE 4.0 8K 预览版
	"ernie-4.0-8k",                   // ERNIE 4.0 8K 基础版
	"ernie-4.0-turbo-8k-latest",      // ERNIE 4.0 Turbo 8K 最新版
	"ernie-4.0-turbo-8k-preview",     // ERNIE 4.0 Turbo 8K 预览版
	"ernie-4.0-turbo-8k",             // ERNIE 4.0 Turbo 8K 基础版
	"ernie-4.0-turbo-128k",           // ERNIE 4.0 Turbo 128K 长上下文版
	"ernie-3.5-8k-preview",           // ERNIE 3.5 8K 预览版
	"ernie-3.5-8k",                   // ERNIE 3.5 8K 基础版
	"ernie-3.5-128k",                 // ERNIE 3.5 128K 长上下文版
	"ernie-speed-8k",                 // ERNIE Speed 8K 快速版
	"ernie-speed-128k",               // ERNIE Speed 128K 长上下文快速版
	"ernie-speed-pro-128k",           // ERNIE Speed Pro 128K 增强快速版
	"ernie-lite-8k",                  // ERNIE Lite 8K 轻量版
	"ernie-lite-pro-128k",            // ERNIE Lite Pro 128K 增强轻量版
	"ernie-tiny-8k",                  // ERNIE Tiny 8K 极速版
	"ernie-char-8k",                  // ERNIE Char 8K 角色扮演模型
	"ernie-char-fiction-8k",          // ERNIE Char Fiction 8K 虚构角色扮演模型
	"ernie-novel-8k",                 // ERNIE Novel 8K 小说创作模型
	"deepseek-v3",                    // DeepSeek V3 对话模型
	"deepseek-r1",                    // DeepSeek R1 推理模型
	"deepseek-r1-distill-qwen-32b",   // DeepSeek R1 蒸馏版 Qwen 32B
	"deepseek-r1-distill-qwen-14b",   // DeepSeek R1 蒸馏版 Qwen 14B
}

// ChannelName 定义百度文心一言 V2 渠道的名称标识。
// 使用 "volcengine" 作为渠道名，因为 V2 版本基于火山引擎平台。
var ChannelName = "volcengine"
