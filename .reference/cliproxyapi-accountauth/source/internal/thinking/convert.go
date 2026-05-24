// 包 thinking - convert.go
// 该文件提供了思考级别和预算之间的转换功能。
// 包括级别到预算的映射、预算到级别的映射、模型能力检测等。
package thinking

import (
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
)

// levelToBudgetMap 定义了标准级别到预算的映射表。
// 所有键为小写；查找时应使用 strings.ToLower。
var levelToBudgetMap = map[string]int{
	"none":    0,
	"auto":    -1,
	"minimal": 512,
	"low":     1024,
	"medium":  8192,
	"high":    24576,
	"xhigh":   32768,
	// "max" is used by Claude adaptive thinking effort. We map it to a large budget
	// and rely on per-model clamping when converting to budget-only providers.
	"max": 128000,
}

// ConvertLevelToBudget converts a thinking level to a budget value.
//
// This is a semantic conversion that maps discrete levels to numeric budgets.
// Level matching is case-insensitive.
//
// Level → Budget mapping:
//   - none    → 0
//   - auto    → -1
//   - minimal → 512
//   - low     → 1024
//   - medium  → 8192
//   - high    → 24576
//   - xhigh   → 32768
//   - max     → 128000
//
// Returns:
//   - budget: The converted budget value
//   - ok: true if level is valid, false otherwise
func ConvertLevelToBudget(level string) (int, bool) {
	budget, ok := levelToBudgetMap[strings.ToLower(level)]
	return budget, ok
}

// BudgetThreshold 常量定义了每个思考级别的上限。
// 用于 ConvertBudgetToLevel 的范围映射。
const (
	// ThresholdMinimal 是 "minimal" 级别的上限（1-512）
	ThresholdMinimal = 512
	// ThresholdLow 是 "low" 级别的上限（513-1024）
	ThresholdLow = 1024
	// ThresholdMedium 是 "medium" 级别的上限（1025-8192）
	ThresholdMedium = 8192
	// ThresholdHigh 是 "high" 级别的上限（8193-24576）
	ThresholdHigh = 24576
)

// ConvertBudgetToLevel converts a budget value to the nearest thinking level.
//
// This is a semantic conversion that maps numeric budgets to discrete levels.
// Uses threshold-based mapping for range conversion.
//
// Budget → Level thresholds:
//   - -1        → auto
//   - 0         → none
//   - 1-512     → minimal
//   - 513-1024  → low
//   - 1025-8192 → medium
//   - 8193-24576 → high
//   - 24577+    → xhigh
//
// Returns:
//   - level: The converted thinking level string
//   - ok: true if budget is valid, false for invalid negatives (< -1)
func ConvertBudgetToLevel(budget int) (string, bool) {
	switch {
	case budget < -1:
		// Invalid negative values
		return "", false
	case budget == -1:
		return string(LevelAuto), true
	case budget == 0:
		return string(LevelNone), true
	case budget <= ThresholdMinimal:
		return string(LevelMinimal), true
	case budget <= ThresholdLow:
		return string(LevelLow), true
	case budget <= ThresholdMedium:
		return string(LevelMedium), true
	case budget <= ThresholdHigh:
		return string(LevelHigh), true
	default:
		return string(LevelXHigh), true
	}
}

// HasLevel reports whether the given target level exists in the levels slice.
// Matching is case-insensitive with leading/trailing whitespace trimmed.
func HasLevel(levels []string, target string) bool {
	for _, level := range levels {
		if strings.EqualFold(strings.TrimSpace(level), target) {
			return true
		}
	}
	return false
}

// MapToClaudeEffort maps a generic thinking level string to a Claude adaptive
// thinking effort value (low/medium/high/max).
//
// supportsMax indicates whether the target model supports "max" effort.
// Returns the mapped effort and true if the level is valid, or ("", false) otherwise.
func MapToClaudeEffort(level string, supportsMax bool) (string, bool) {
	level = strings.ToLower(strings.TrimSpace(level))
	switch level {
	case "":
		return "", false
	case "minimal":
		return "low", true
	case "low", "medium", "high":
		return level, true
	case "xhigh", "max":
		if supportsMax {
			return "max", true
		}
		return "high", true
	case "auto":
		return "high", true
	default:
		return "", false
	}
}

// ModelCapability 描述模型的思考格式支持能力。
type ModelCapability int

const (
	// CapabilityUnknown 表示 modelInfo 为 nil（透传行为，内部使用）。
	CapabilityUnknown ModelCapability = iota - 1
	// CapabilityNone 表示模型不支持思考（Thinking 为 nil）。
	CapabilityNone
	// CapabilityBudgetOnly 表示模型仅支持数字预算。
	CapabilityBudgetOnly
	// CapabilityLevelOnly 表示模型仅支持离散级别。
	CapabilityLevelOnly
	// CapabilityHybrid 表示模型同时支持预算和级别。
	CapabilityHybrid
)

// detectModelCapability 检测模型的思考格式支持能力。
//
// 根据模型的 ThinkingSupport 配置分类：
//   - CapabilityNone: Thinking 为 nil（不支持思考）
//   - CapabilityBudgetOnly: 有 Min/Max 但无 Levels（Claude、Gemini 2.5）
//   - CapabilityLevelOnly: 有 Levels 但无 Min/Max（OpenAI、Codex、Kimi）
//   - CapabilityHybrid: 同时有 Min/Max 和 Levels（Gemini 3）
func detectModelCapability(modelInfo *registry.ModelInfo) ModelCapability {
	if modelInfo == nil {
		return CapabilityUnknown // sentinel for "passthrough" behavior
	}
	if modelInfo.Thinking == nil {
		return CapabilityNone
	}
	support := modelInfo.Thinking
	hasBudget := support.Min > 0 || support.Max > 0
	hasLevels := len(support.Levels) > 0

	switch {
	case hasBudget && hasLevels:
		return CapabilityHybrid
	case hasBudget:
		return CapabilityBudgetOnly
	case hasLevels:
		return CapabilityLevelOnly
	default:
		return CapabilityNone
	}
}
