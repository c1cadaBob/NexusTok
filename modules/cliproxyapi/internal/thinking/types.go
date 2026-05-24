// thinking - types.go
// 该文件定义了统一思考（Thinking）配置处理的核心类型。
// 包括思考模式（预算/级别/禁用/自动）、思考级别、统一配置结构体、
// 模型名称后缀解析结果，以及提供商特定的应用器接口。
// 支持 Claude、Gemini、OpenAI、Codex、Antigravity、Kimi、xAI 等提供商。

// Package thinking provides unified thinking configuration processing.
//
// This package offers a unified interface for parsing, validating, and applying
// thinking configurations across various AI providers (Claude, Gemini, OpenAI, Codex, Antigravity, Kimi, xAI).
package thinking

import "github.com/router-for-me/CLIProxyAPI/v7/internal/registry"

// ThinkingMode 表示思考配置模式的类型。
type ThinkingMode int

const (
	// ModeBudget 使用数字预算模式（对应后缀 "(1000)" 等）
	ModeBudget ThinkingMode = iota
	// ModeLevel 使用离散级别模式（对应后缀 "(high)" 等）
	ModeLevel
	// ModeNone 禁用思考（对应后缀 "(none)" 或 budget=0）
	ModeNone
	// ModeAuto 自动/动态思考（对应后缀 "(auto)" 或 budget=-1）
	ModeAuto
)

// String 返回 ThinkingMode 的字符串表示。
func (m ThinkingMode) String() string {
	switch m {
	case ModeBudget:
		return "budget"
	case ModeLevel:
		return "level"
	case ModeNone:
		return "none"
	case ModeAuto:
		return "auto"
	default:
		return "unknown"
	}
}

// ThinkingLevel 表示离散的思考级别。
type ThinkingLevel string

const (
	// LevelNone 禁用思考
	LevelNone ThinkingLevel = "none"
	// LevelAuto 启用自动/动态思考
	LevelAuto ThinkingLevel = "auto"
	// LevelMinimal 设置最小思考力度
	LevelMinimal ThinkingLevel = "minimal"
	// LevelLow 设置低思考力度
	LevelLow ThinkingLevel = "low"
	// LevelMedium 设置中等思考力度
	LevelMedium ThinkingLevel = "medium"
	// LevelHigh 设置高思考力度
	LevelHigh ThinkingLevel = "high"
	// LevelXHigh 设置超高思考力度
	LevelXHigh ThinkingLevel = "xhigh"
	// LevelMax 设置最大思考力度。
	// 目前用于 Claude 4.6 自适应思考（opus 支持 "max"）。
	LevelMax ThinkingLevel = "max"
)

// ThinkingConfig 表示统一的思考配置。
// 用于在组件之间传递思考配置信息。
// 根据 Mode 的不同，Budget 或 Level 字段生效：
//   - ModeNone: Budget=0，Level 被忽略
//   - ModeAuto: Budget=-1，Level 被忽略
//   - ModeBudget: Budget 为正整数，Level 被忽略
//   - ModeLevel: Budget 被忽略，Level 为有效级别
type ThinkingConfig struct {
	// Mode 指定配置模式
	Mode ThinkingMode
	// Budget 是思考预算（token 数量），仅在 ModeBudget 模式下生效。
	// 特殊值：0 表示禁用，-1 表示自动
	Budget int
	// Level 是思考级别，仅在 ModeLevel 模式下生效
	Level ThinkingLevel
}

// SuffixResult 表示从模型名称中解析思考后缀的结果。
// 思考后缀的格式为 model-name(value)，其中 value 可以是数字预算
//（如 "16384"）或级别名称（如 "high"）。
type SuffixResult struct {
	// ModelName 是移除后缀后的模型名称。
	// 如果未找到后缀，则等于原始输入。
	ModelName string

	// HasSuffix 指示是否找到有效后缀。
	HasSuffix bool

	// RawSuffix 是括号内的内容（不含括号）。
	// 如果 HasSuffix 为 false，则为空字符串。
	RawSuffix string
}

// ProviderApplier 定义了提供商特定的思考配置应用器接口。
// 实现此接口的类型负责将统一的 ThinkingConfig 转换为提供商特定格式
// 并应用到请求体中。
//
// 实现要求：
//   - Apply 方法必须是幂等的
//   - 不得修改输入的 config 或 modelInfo
//   - 返回请求体的修改副本
//   - 对于不支持的配置返回适当的 ThinkingError
type ProviderApplier interface {
	// Apply 将思考配置应用到请求体中。
	//
	// 参数：
	//   - body: 原始请求体 JSON
	//   - config: 统一思考配置
	//   - modelInfo: 模型注册表信息，包含 ThinkingSupport 属性
	//
	// 返回值：
	//   - 修改后的请求体 JSON
	//   - 如果配置无效或不支持则返回 ThinkingError
	Apply(body []byte, config ThinkingConfig, modelInfo *registry.ModelInfo) ([]byte, error)
}
