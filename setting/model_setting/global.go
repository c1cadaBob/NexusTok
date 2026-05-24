// global.go — 全局模型配置管理
// 职责：定义和管理全局级别的模型配置，包括透传请求开关、思维链后缀模型黑名单、
// 以及 ChatCompletions 到 Responses API 的转换策略等跨模型的通用设置。
// 通过 config.GlobalConfig 注册实现持久化存储。

package model_setting

import (
	"slices"
	"strings"

	"github.com/c1cada/NexusTok/setting/config"
)

// ChatCompletionsToResponsesPolicy 定义 ChatCompletions 请求自动转换为 Responses API 的策略配置。
// 可按渠道 ID、渠道类型、模型名称模式等维度控制转换行为。
type ChatCompletionsToResponsesPolicy struct {
	Enabled       bool     `json:"enabled"`                  // 是否启用转换策略
	AllChannels   bool     `json:"all_channels"`             // 是否对所有渠道生效
	ChannelIDs    []int    `json:"channel_ids,omitempty"`    // 指定生效的渠道 ID 列表
	ChannelTypes  []int    `json:"channel_types,omitempty"`  // 指定生效的渠道类型列表
	ModelPatterns []string `json:"model_patterns,omitempty"` // 指定生效的模型名称模式列表
}

// IsChannelEnabled 判断指定渠道是否启用 ChatCompletions 到 Responses 的转换。
// 依次检查：策略总开关 -> 全渠道开关 -> 渠道 ID 匹配 -> 渠道类型匹配。
func (p ChatCompletionsToResponsesPolicy) IsChannelEnabled(channelID int, channelType int) bool {
	if !p.Enabled {
		return false
	}
	if p.AllChannels {
		return true
	}

	if channelID > 0 && len(p.ChannelIDs) > 0 && slices.Contains(p.ChannelIDs, channelID) {
		return true
	}
	if channelType > 0 && len(p.ChannelTypes) > 0 && slices.Contains(p.ChannelTypes, channelType) {
		return true
	}
	return false
}

// GlobalSettings 定义全局模型配置结构体，包含跨模型的通用设置。
type GlobalSettings struct {
	PassThroughRequestEnabled        bool                             `json:"pass_through_request_enabled"`        // 是否启用请求透传模式
	ThinkingModelBlacklist           []string                         `json:"thinking_model_blacklist"`            // 需保留 thinking/-nothinking 等后缀的模型黑名单
	ChatCompletionsToResponsesPolicy ChatCompletionsToResponsesPolicy `json:"chat_completions_to_responses_policy"` // ChatCompletions 转 Responses API 的策略
}

// 默认配置
var defaultOpenaiSettings = GlobalSettings{
	PassThroughRequestEnabled: false,
	ThinkingModelBlacklist: []string{
		"moonshotai/kimi-k2-thinking",
		"kimi-k2-thinking",
	},
	ChatCompletionsToResponsesPolicy: ChatCompletionsToResponsesPolicy{
		Enabled:     false,
		AllChannels: true,
	},
}

// 全局实例
var globalSettings = defaultOpenaiSettings

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("global", &globalSettings)
}

// GetGlobalSettings 获取当前全局配置的指针。
func GetGlobalSettings() *GlobalSettings {
	return &globalSettings
}

// ShouldPreserveThinkingSuffix 判断模型是否配置为保留 thinking/-nothinking/-low/-high/-medium 后缀
func ShouldPreserveThinkingSuffix(modelName string) bool {
	target := strings.TrimSpace(modelName)
	if target == "" {
		return false
	}

	for _, entry := range globalSettings.ThinkingModelBlacklist {
		if strings.TrimSpace(entry) == target {
			return true
		}
	}
	return false
}
