// Package openrouter 的常量定义文件。
// 定义 OpenRouter 渠道支持的模型列表和渠道名称。
// OpenRouter 是一个 AI 模型聚合平台，提供对多种模型的统一访问。
// 模型列表为空是因为 OpenRouter 支持动态模型，用户可以自行配置。
package openrouter

// ModelList 是 OpenRouter 支持的模型名称列表。
// OpenRouter 支持动态模型发现，因此列表为空。
// 用户可以在配置中指定任何 OpenRouter 支持的模型。
var ModelList = []string{}

// ChannelName 是 OpenRouter 渠道的标识名称。
var ChannelName = "openrouter"
