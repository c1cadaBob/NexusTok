// thinking - validate.go
// 该文件实现了思考配置的验证和规范化逻辑。
// 包括模型能力检测、配置模式自动转换（预算/级别互转）、级别和预算的范围钳制、
// 提供商家族判断（同族/跨族）以及日志记录等核心功能。

// Package thinking provides unified thinking configuration processing logic.
package thinking

import (
	"fmt"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	log "github.com/sirupsen/logrus"
)

// ValidateConfig 根据模型能力验证并规范化思考配置。
//
// 该函数执行全面的验证流程：
//   - 检查模型是否支持思考功能
//   - 根据模型能力在预算（Budget）和级别（Level）格式之间自动转换
//   - 验证请求的级别是否在模型支持的级别列表中
//   - 将预算值钳制到模型允许的范围内
//   - 当为仅支持级别的模型将预算转换为级别时，将派生的标准级别钳制到最近的支持级别
//   - 当配置来自模型后缀时，禁用严格的预算验证（钳制而非报错）
//
// 参数：
//   - config: 待验证的思考配置
//   - modelInfo: 模型注册表信息，包含 ThinkingSupport 属性（nil 表示不支持思考）
//   - fromFormat: 源提供商格式（用于确定严格验证规则）
//   - toFormat: 目标提供商格式
//   - fromSuffix: 配置是否来自模型后缀
//
// 返回值：
//   - 钳制值后的规范化 ThinkingConfig
//   - 验证失败时返回 ThinkingError（ErrThinkingNotSupported、ErrLevelNotSupported 等）
//
// 自动转换行为：
//   - 仅预算模型 + 级别配置 → 级别转换为预算
//   - 仅级别模型 + 预算配置 → 预算转换为级别
//   - 混合模型 → 保留原始格式
func ValidateConfig(config ThinkingConfig, modelInfo *registry.ModelInfo, fromFormat, toFormat string, fromSuffix bool) (*ThinkingConfig, error) {
	fromFormat, toFormat = strings.ToLower(strings.TrimSpace(fromFormat)), strings.ToLower(strings.TrimSpace(toFormat))
	model := "unknown"
	support := (*registry.ThinkingSupport)(nil)
	if modelInfo != nil {
		if modelInfo.ID != "" {
			model = modelInfo.ID
		}
		support = modelInfo.Thinking
	}

	if support == nil {
		if config.Mode != ModeNone {
			return nil, NewThinkingErrorWithModel(ErrThinkingNotSupported, "thinking not supported for this model", model)
		}
		return &config, nil
	}

	// allowClampUnsupported 决定是否钳制不支持的级别而非返回错误。
	// 当跨提供商家族（如 openai→gemini、claude→gemini）且目标模型支持离散级别时生效。
	// 同一家族内的转换需要严格验证。
	toCapability := detectModelCapability(modelInfo)
	toHasLevelSupport := toCapability == CapabilityLevelOnly || toCapability == CapabilityHybrid
	allowClampUnsupported := toHasLevelSupport && !isSameProviderFamily(fromFormat, toFormat)

	// strictBudget 决定是否强制执行严格的预算范围验证。
	// 当满足以下条件时生效：(1) 配置来自请求体（非后缀），(2) 源格式已知，
	// (3) 源和目标在同一提供商家族内。跨家族或基于后缀的配置将被钳制而非拒绝，以提高互操作性。
	strictBudget := !fromSuffix && fromFormat != "" && isSameProviderFamily(fromFormat, toFormat)
	budgetDerivedFromLevel := false

	capability := detectModelCapability(modelInfo)
	switch capability {
	case CapabilityBudgetOnly:
		if config.Mode == ModeLevel {
			if config.Level == LevelAuto {
				break
			}
			budget, ok := ConvertLevelToBudget(string(config.Level))
			if !ok {
				return nil, NewThinkingError(ErrUnknownLevel, fmt.Sprintf("unknown level: %s", config.Level))
			}
			config.Mode = ModeBudget
			config.Budget = budget
			config.Level = ""
			budgetDerivedFromLevel = true
		}
	case CapabilityLevelOnly:
		if config.Mode == ModeBudget {
			level, ok := ConvertBudgetToLevel(config.Budget)
			if !ok {
				return nil, NewThinkingError(ErrUnknownLevel, fmt.Sprintf("budget %d cannot be converted to a valid level", config.Budget))
			}
			// 当为仅支持级别的模型将预算转换为级别时，将派生的标准级别钳制到最近的支持级别。
		// 特殊值（none/auto）保持不变。
		config.Mode = ModeLevel
			config.Level = clampLevel(ThinkingLevel(level), modelInfo, toFormat)
			config.Budget = 0
		}
	case CapabilityHybrid:
	}

	if config.Mode == ModeLevel && config.Level == LevelNone {
		config.Mode = ModeNone
		config.Budget = 0
		config.Level = ""
	}
	if config.Mode == ModeLevel && config.Level == LevelAuto {
		config.Mode = ModeAuto
		config.Budget = -1
		config.Level = ""
	}
	if config.Mode == ModeBudget && config.Budget == 0 {
		config.Mode = ModeNone
		config.Level = ""
	}

	if len(support.Levels) > 0 && config.Mode == ModeLevel {
		if !isLevelSupported(string(config.Level), support.Levels) {
			if allowClampUnsupported {
				config.Level = clampLevel(config.Level, modelInfo, toFormat)
			}
			if !isLevelSupported(string(config.Level), support.Levels) {
				// User explicitly specified an unsupported level - return error
				// (budget-derived levels may be clamped based on source format)
				validLevels := normalizeLevels(support.Levels)
				message := fmt.Sprintf("level %q not supported, valid levels: %s", strings.ToLower(string(config.Level)), strings.Join(validLevels, ", "))
				return nil, NewThinkingError(ErrLevelNotSupported, message)
			}
		}
	}

	if strictBudget && config.Mode == ModeBudget && !budgetDerivedFromLevel {
		min, max := support.Min, support.Max
		if min != 0 || max != 0 {
			if config.Budget < min || config.Budget > max || (config.Budget == 0 && !support.ZeroAllowed) {
				message := fmt.Sprintf("budget %d out of range [%d,%d]", config.Budget, min, max)
				return nil, NewThinkingError(ErrBudgetOutOfRange, message)
			}
		}
	}

	// 将 ModeAuto 转换为中等值（当不支持动态思考时）
	if config.Mode == ModeAuto && !support.DynamicAllowed {
		config = convertAutoToMidRange(config, support, toFormat, model)
	}

	if config.Mode == ModeNone && toFormat == "claude" {
		// Claude 支持通过 thinking.type="disabled" 显式禁用。
		// 保留 Budget=0 以便应用器可以省略 budget_tokens。
		config.Budget = 0
		config.Level = ""
	} else {
		switch config.Mode {
		case ModeBudget, ModeAuto, ModeNone:
			config.Budget = clampBudget(config.Budget, modelInfo, toFormat)
		}

		// ModeNone 钳制后 Budget > 0：为仅级别/混合模型设置最低级别
		// 确保 Apply 层无需访问 support.Levels
		if config.Mode == ModeNone && config.Budget > 0 && len(support.Levels) > 0 {
			config.Level = ThinkingLevel(support.Levels[0])
		}
	}

	return &config, nil
}

// convertAutoToMidRange 在不支持动态思考时将 ModeAuto 转换为中等固定值。
//
// 该函数处理模型不支持动态/自动思考的情况。自动模式会根据模型能力被静默转换为固定值：
//   - 仅级别模型：转换为 ModeLevel，级别设为 LevelMedium
//   - 预算模型：转换为 ModeBudget，预算设为 (Min + Max) / 2
//
// 日志记录：
//   - 转换发生时记录 Debug 级别日志
//   - 字段：original_mode、clamped_to、reason
func convertAutoToMidRange(config ThinkingConfig, support *registry.ThinkingSupport, provider, model string) ThinkingConfig {
	// 对于仅级别模型（有 Levels 但没有 Min/Max 范围），使用 ModeLevel 和 medium 级别
	if len(support.Levels) > 0 && support.Min == 0 && support.Max == 0 {
		config.Mode = ModeLevel
		config.Level = LevelMedium
		config.Budget = 0
		log.WithFields(log.Fields{
			"provider":      provider,
			"model":         model,
			"original_mode": "auto",
			"clamped_to":    string(LevelMedium),
		}).Debug("thinking: mode converted, dynamic not allowed, using medium level |")
		return config
	}

	// 对于预算模型，使用中等预算值
	mid := (support.Min + support.Max) / 2
	if mid <= 0 && support.ZeroAllowed {
		config.Mode = ModeNone
		config.Budget = 0
	} else if mid <= 0 {
		config.Mode = ModeBudget
		config.Budget = support.Min
	} else {
		config.Mode = ModeBudget
		config.Budget = mid
	}
	log.WithFields(log.Fields{
		"provider":      provider,
		"model":         model,
		"original_mode": "auto",
		"clamped_to":    config.Budget,
	}).Debug("thinking: mode converted, dynamic not allowed |")
	return config
}

// standardLevelOrder 定义了思考级别从低到高的标准排序。
var standardLevelOrder = []ThinkingLevel{LevelMinimal, LevelLow, LevelMedium, LevelHigh, LevelXHigh, LevelMax}

// clampLevel 将给定级别钳制到最近的支持级别。
// 相等距离时优先选择较低级别。
func clampLevel(level ThinkingLevel, modelInfo *registry.ModelInfo, provider string) ThinkingLevel {
	model := "unknown"
	var supported []string
	if modelInfo != nil {
		if modelInfo.ID != "" {
			model = modelInfo.ID
		}
		if modelInfo.Thinking != nil {
			supported = modelInfo.Thinking.Levels
		}
	}

	if len(supported) == 0 || isLevelSupported(string(level), supported) {
		return level
	}

	pos := levelIndex(string(level))
	if pos == -1 {
		return level
	}
	bestIdx, bestDist := -1, len(standardLevelOrder)+1

	for _, s := range supported {
		if idx := levelIndex(strings.TrimSpace(s)); idx != -1 {
			if dist := abs(pos - idx); dist < bestDist || (dist == bestDist && idx < bestIdx) {
				bestIdx, bestDist = idx, dist
			}
		}
	}

	if bestIdx >= 0 {
		clamped := standardLevelOrder[bestIdx]
		log.WithFields(log.Fields{
			"provider":       provider,
			"model":          model,
			"original_value": string(level),
			"clamped_to":     string(clamped),
		}).Debug("thinking: level clamped |")
		return clamped
	}
	return level
}

// clampBudget 将预算值钳制到模型支持的范围内。
func clampBudget(value int, modelInfo *registry.ModelInfo, provider string) int {
	model := "unknown"
	support := (*registry.ThinkingSupport)(nil)
	if modelInfo != nil {
		if modelInfo.ID != "" {
			model = modelInfo.ID
		}
		support = modelInfo.Thinking
	}
	if support == nil {
		return value
	}

	// 自动值（-1）直接通过，不进行钳制。
	if value == -1 {
		return value
	}

	min, max := support.Min, support.Max
	if value == 0 && !support.ZeroAllowed {
		log.WithFields(log.Fields{
			"provider":       provider,
			"model":          model,
			"original_value": value,
			"clamped_to":     min,
			"min":            min,
			"max":            max,
		}).Warn("thinking: budget zero not allowed |")
		return min
	}

	// 某些模型仅支持级别，不定义数值预算范围。
	if min == 0 && max == 0 {
		return value
	}

	if value < min {
		if value == 0 && support.ZeroAllowed {
			return 0
		}
		logClamp(provider, model, value, min, min, max)
		return min
	}
	if value > max {
		logClamp(provider, model, value, max, min, max)
		return max
	}
	return value
}

// isLevelSupported 检查给定级别是否在支持的级别列表中。
func isLevelSupported(level string, supported []string) bool {
	for _, s := range supported {
		if strings.EqualFold(level, strings.TrimSpace(s)) {
			return true
		}
	}
	return false
}

// levelIndex 返回给定级别在标准排序中的索引，未找到返回 -1。
func levelIndex(level string) int {
	for i, l := range standardLevelOrder {
		if strings.EqualFold(level, string(l)) {
			return i
		}
	}
	return -1
}

// normalizeLevels 将级别列表中的所有项转为小写并去除首尾空白。
func normalizeLevels(levels []string) []string {
	out := make([]string, len(levels))
	for i, l := range levels {
		out[i] = strings.ToLower(strings.TrimSpace(l))
	}
	return out
}

// isBudgetCapableProvider 返回提供商是否支持基于预算的思考。
// 这些提供商也可能支持基于级别的思考（混合模型）。
func isBudgetCapableProvider(provider string) bool {
	switch provider {
	case "gemini", "gemini-cli", "antigravity", "claude":
		return true
	default:
		return false
	}
}

// isGeminiFamily 返回提供商是否属于 Gemini 家族。
func isGeminiFamily(provider string) bool {
	switch provider {
	case "gemini", "gemini-cli", "antigravity":
		return true
	default:
		return false
	}
}

// isOpenAIFamily 返回提供商是否属于 OpenAI 家族。
func isOpenAIFamily(provider string) bool {
	switch provider {
	case "openai", "openai-response", "codex", "xai":
		return true
	default:
		return false
	}
}

// isSameProviderFamily 判断两个提供商是否属于同一家族。
func isSameProviderFamily(from, to string) bool {
	if from == to {
		return true
	}
	return (isGeminiFamily(from) && isGeminiFamily(to)) ||
		(isOpenAIFamily(from) && isOpenAIFamily(to))
}

// abs 返回整数的绝对值。
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// logClamp 记录预算钳制操作的调试日志。
func logClamp(provider, model string, original, clampedTo, min, max int) {
	log.WithFields(log.Fields{
		"provider":       provider,
		"model":          model,
		"original_value": original,
		"min":            min,
		"max":            max,
		"clamped_to":     clampedTo,
	}).Debug("thinking: budget clamped |")
}
