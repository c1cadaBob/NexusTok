// dify - constants.go
// 本文件定义了 Dify 渠道的模型列表和渠道名称常量。
// Dify 是一个开源的 LLM 应用开发平台，支持通过 API 调用各种 AI 应用。
// 与传统模型渠道不同，Dify 使用 Bot ID（应用 ID）而非模型名来标识具体应用，
// 因此模型列表通常为空。
package dify

// ModelList 定义了 Dify 渠道支持的模型列表。
// 由于 Dify 通过 Bot ID 标识应用，模型列表通常为空。
// 用户在请求时通过指定 Bot ID 来选择具体的 Dify 应用。
var ModelList []string

// ChannelName 定义了渠道名称标识符。
// 用于在系统中唯一标识 Dify 渠道，值为 "dify"。
var ChannelName = "dify"
