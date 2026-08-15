package model

import (
	"strings"

	"github.com/c1cada/NexusTok/setting/ratio_setting"
)

// SplitCommaValues 将逗号分隔的配置值拆分为去空白后的切片。
// 渠道模型、账号模型和分组配置都沿用逗号分隔格式；集中处理可以避免各处对空值、
// 首尾逗号和多余空格产生不一致解释。
func SplitCommaValues(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

// MatchesModelList 判断模型名称是否匹配配置列表中的任一项。
// 该函数是渠道账号实际调度和后台展示聚合的共同语义来源，支持精确匹配、
// ratio_setting.FormatMatchingModelName 规范化匹配、"*" 全匹配，以及 "prefix-*"
// 这类前缀通配，避免展示层和 Relay 选号层对同一个同步密钥得出不同结论。
func MatchesModelList(models []string, modelName string) bool {
	modelName = strings.TrimSpace(modelName)
	canonicalModel := ratio_setting.FormatMatchingModelName(modelName)
	for _, candidate := range models {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if candidate == "*" || candidate == modelName || candidate == canonicalModel {
			return true
		}
		normalizedCandidate := ratio_setting.FormatMatchingModelName(candidate)
		if normalizedCandidate == modelName || normalizedCandidate == canonicalModel {
			return true
		}
		if strings.HasSuffix(candidate, "*") {
			prefix := strings.TrimSuffix(candidate, "*")
			if strings.HasPrefix(modelName, prefix) || strings.HasPrefix(canonicalModel, prefix) {
				return true
			}
		}
	}
	return false
}
