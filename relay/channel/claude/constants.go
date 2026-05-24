// Package claude 实现 Anthropic Claude AI 平台的渠道适配器常量定义。
// 包含该渠道支持的模型列表和渠道名称标识。
package claude

// ModelList 定义 Claude 渠道支持的模型列表。
// 包含 Claude 3/3.5/4 系列模型，涵盖 Opus、Sonnet、Haiku 不同能力级别，
// 以及带 "-thinking" 后缀的扩展思维版本。
var ModelList = []string{
	"claude-3-sonnet-20240229",           // Claude 3 Sonnet 基础版
	"claude-3-opus-20240229",             // Claude 3 Opus 旗舰版
	"claude-3-haiku-20240307",            // Claude 3 Haiku 快速版
	"claude-3-5-haiku-20241022",          // Claude 3.5 Haiku 快速版
	"claude-haiku-4-5-20251001",          // Claude Haiku 4.5 快速版
	"claude-3-5-sonnet-20240620",         // Claude 3.5 Sonnet 基础版
	"claude-3-5-sonnet-20241022",         // Claude 3.5 Sonnet 更新版
	"claude-3-7-sonnet-20250219",         // Claude 3.7 Sonnet
	"claude-3-7-sonnet-20250219-thinking", // Claude 3.7 Sonnet 扩展思维版
	"claude-sonnet-4-20250514",           // Claude Sonnet 4
	"claude-sonnet-4-20250514-thinking",  // Claude Sonnet 4 扩展思维版
	"claude-opus-4-20250514",             // Claude Opus 4
	"claude-opus-4-20250514-thinking",    // Claude Opus 4 扩展思维版
	"claude-opus-4-1-20250805",           // Claude Opus 4.1
	"claude-opus-4-1-20250805-thinking",  // Claude Opus 4.1 扩展思维版
	"claude-sonnet-4-5-20250929",         // Claude Sonnet 4.5
	"claude-sonnet-4-5-20250929-thinking", // Claude Sonnet 4.5 扩展思维版
	"claude-opus-4-5-20251101",           // Claude Opus 4.5
	"claude-opus-4-5-20251101-thinking",  // Claude Opus 4.5 扩展思维版
	"claude-opus-4-6",                    // Claude Opus 4.6 默认级别
	"claude-opus-4-6-max",               // Claude Opus 4.6 最大级别
	"claude-opus-4-6-high",              // Claude Opus 4.6 高级别
	"claude-opus-4-6-medium",            // Claude Opus 4.6 中级别
	"claude-opus-4-6-low",               // Claude Opus 4.6 低级别
	"claude-sonnet-4-6",                 // Claude Sonnet 4.6
	"claude-opus-4-7",                   // Claude Opus 4.7 默认级别
	"claude-opus-4-7-max",               // Claude Opus 4.7 最大级别
	"claude-opus-4-7-xhigh",             // Claude Opus 4.7 超高级别
	"claude-opus-4-7-high",              // Claude Opus 4.7 高级别
	"claude-opus-4-7-medium",            // Claude Opus 4.7 中级别
	"claude-opus-4-7-low",               // Claude Opus 4.7 低级别
	"claude-opus-4-7-thinking",          // Claude Opus 4.7 扩展思维版
}

// ChannelName 定义 Claude 渠道的名称标识，用于渠道注册和路由匹配。
var ChannelName = "claude"
