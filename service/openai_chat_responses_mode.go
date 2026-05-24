// openai_chat_responses_mode.go
// 本文件提供了判断是否应将 Chat Completions 请求路由到 Responses API 的策略函数。
// 支持两种判断模式：
// 1. 基于策略（Policy）：根据配置的策略规则判断
// 2. 基于全局配置（Global）：根据全局设置判断
// 这些函数是对 openaicompat 包中实现的薄封装，便于 service 层直接调用。

package service

import (
	// 项目内部包
	"github.com/c1cada/NexusTok/service/openaicompat"
	"github.com/c1cada/NexusTok/setting/model_setting"
)

// ShouldChatCompletionsUseResponsesPolicy 根据策略判断是否应使用 Responses API
// 支持按渠道 ID、渠道类型和模型名称进行匹配
// 参数:
//   - policy: Chat Completions 到 Responses 的路由策略配置
//   - channelID: 渠道 ID
//   - channelType: 渠道类型
//   - model: 模型名称
// 返回值:
//   - bool: 是否应使用 Responses API
func ShouldChatCompletionsUseResponsesPolicy(policy model_setting.ChatCompletionsToResponsesPolicy, channelID int, channelType int, model string) bool {
	return openaicompat.ShouldChatCompletionsUseResponsesPolicy(policy, channelID, channelType, model)
}

// ShouldChatCompletionsUseResponsesGlobal 根据全局配置判断是否应使用 Responses API
// 用于判断指定渠道和模型是否应全局启用 Responses API 模式
// 参数:
//   - channelID: 渠道 ID
//   - channelType: 渠道类型
//   - model: 模型名称
// 返回值:
//   - bool: 是否应全局使用 Responses API
func ShouldChatCompletionsUseResponsesGlobal(channelID int, channelType int, model string) bool {
	return openaicompat.ShouldChatCompletionsUseResponsesGlobal(channelID, channelType, model)
}
