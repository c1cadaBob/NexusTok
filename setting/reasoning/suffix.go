// suffix.go — 推理模型后缀解析工具
// 职责：解析 AI 模型名称中的推理努力等级（Effort Level）后缀。
// 支持多种推理模型的后缀格式，包括通用 Claude 后缀、OpenAI 推理后缀
// 和 DeepSeek V4 思维模式后缀。

package reasoning

import (
	"strings"

	"github.com/samber/lo"
)

// EffortSuffixes 通用的推理努力等级后缀列表（用于 Claude 等模型）
// 例如：claude-opus-4-6-high, claude-opus-4-6-low
var EffortSuffixes = []string{"-max", "-xhigh", "-high", "-medium", "-low", "-minimal"}

// OpenAIEffortSuffixes OpenAI 模型的推理努力等级后缀列表
// 例如：o3-mini-high, o3-mini-low
var OpenAIEffortSuffixes = []string{"-high", "-minimal", "-low", "-medium", "-none", "-xhigh"}

// DeepSeekV4EffortSuffixes DeepSeek V4 模型的思维模式后缀列表
// -none 表示禁用思维，-max 表示启用最大思维
var DeepSeekV4EffortSuffixes = []string{"-none", "-max"}

// TrimEffortSuffix 从模型名中剥离推理努力等级后缀
// 使用通用 EffortSuffixes 列表进行匹配
// 参数：
//   - modelName: 原始模型名称
//
// 返回值：
//   - string: 剥离后缀后的基础模型名
//   - string: 剥离的后缀（不含前缀 "-"）
//   - bool: 是否成功匹配到后缀
func TrimEffortSuffix(modelName string) (string, string, bool) {
	return TrimEffortSuffixWithSuffixes(modelName, EffortSuffixes)
}

// TrimEffortSuffixWithSuffixes 从模型名中剥离指定后缀列表中的后缀
// 参数：
//   - modelName: 原始模型名称
//   - suffixes: 待匹配的后缀列表
//
// 返回值：
//   - string: 剥离后缀后的基础模型名
//   - string: 剥离的后缀（不含前缀 "-"）
//   - bool: 是否成功匹配到后缀
func TrimEffortSuffixWithSuffixes(modelName string, suffixes []string) (string, string, bool) {
	suffix, found := lo.Find(suffixes, func(s string) bool {
		return strings.HasSuffix(modelName, s)
	})
	if !found {
		return modelName, "", false
	}
	return strings.TrimSuffix(modelName, suffix), strings.TrimPrefix(suffix, "-"), true
}

// ParseOpenAIReasoningEffortFromModelSuffix 从 OpenAI 模型名中解析推理努力等级
// 参数：
//   - modelName: 模型名称，如 "o3-mini-high"
//
// 返回值：
//   - string: 解析出的努力等级（如 "high"），未匹配时为空字符串
//   - string: 基础模型名（如 "o3-mini"），未匹配时为原始模型名
func ParseOpenAIReasoningEffortFromModelSuffix(modelName string) (string, string) {
	baseModel, effort, ok := TrimEffortSuffixWithSuffixes(modelName, OpenAIEffortSuffixes)
	if !ok {
		return "", modelName
	}
	return effort, baseModel
}

// ParseDeepSeekV4ThinkingSuffix 从 DeepSeek V4 模型名中解析思维模式配置
// 参数：
//   - modelName: 模型名称，如 "deepseek-v4-chat-max"
//
// 返回值：
//   - baseModel: 基础模型名
//   - thinkingType: 思维模式类型，"disabled"（禁用）或 "enabled"（启用）
//   - effort: 思维努力等级（仅 "max" 时有值）
//   - ok: 是否成功解析
func ParseDeepSeekV4ThinkingSuffix(modelName string) (baseModel string, thinkingType string, effort string, ok bool) {
	baseModel, suffix, ok := TrimEffortSuffixWithSuffixes(modelName, DeepSeekV4EffortSuffixes)
	if !ok || !strings.HasPrefix(baseModel, "deepseek-v4-") {
		return modelName, "", "", false
	}
	switch suffix {
	case "none":
		return baseModel, "disabled", "", true // 禁用思维模式
	case "max":
		return baseModel, "enabled", "max", true // 启用最大思维模式
	default:
		return modelName, "", "", false
	}
}
