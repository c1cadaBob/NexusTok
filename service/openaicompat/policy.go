// policy.go 提供 Chat Completions 到 Responses API 转换的策略判断逻辑。
// 根据渠道 ID、渠道类型和模型名称，决定是否将 Chat Completions 请求
// 自动转换为 Responses API 格式。
package openaicompat

import "github.com/c1cada/NexusTok/setting/model_setting" // 模型策略配置

// ShouldChatCompletionsUseResponsesPolicy 根据指定策略判断是否应使用 Responses API。
// 同时检查渠道是否启用和模型是否匹配策略中的模式列表。
//
// 参数：
//   - policy: Chat Completions 到 Responses 的转换策略配置
//   - channelID: 渠道 ID
//   - channelType: 渠道类型
//   - model: 模型名称
//
// 返回：
//   - bool: true 表示应转换为 Responses API 格式
func ShouldChatCompletionsUseResponsesPolicy(policy model_setting.ChatCompletionsToResponsesPolicy, channelID int, channelType int, model string) bool {
	// 首先检查该渠道是否在策略的启用列表中
	if !policy.IsChannelEnabled(channelID, channelType) {
		return false
	}
	// 然后检查模型名称是否匹配策略中的正则模式
	return matchAnyRegex(policy.ModelPatterns, model)
}

// ShouldChatCompletionsUseResponsesGlobal 使用全局策略判断是否应使用 Responses API。
// 从全局配置中获取策略，委托给 ShouldChatCompletionsUseResponsesPolicy。
//
// 参数：
//   - channelID: 渠道 ID
//   - channelType: 渠道类型
//   - model: 模型名称
//
// 返回：
//   - bool: true 表示应转换为 Responses API 格式
func ShouldChatCompletionsUseResponsesGlobal(channelID int, channelType int, model string) bool {
	return ShouldChatCompletionsUseResponsesPolicy(
		model_setting.GetGlobalSettings().ChatCompletionsToResponsesPolicy,
		channelID,
		channelType,
		model,
	)
}
