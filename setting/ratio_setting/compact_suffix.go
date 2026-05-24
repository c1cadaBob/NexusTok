// compact_suffix.go — 紧凑模型后缀工具
// 职责：定义和操作模型名称的 "-openai-compact" 后缀，
// 用于标识使用紧凑格式的 OpenAI 兼容模型变体。

package ratio_setting

import "strings"

// CompactModelSuffix 紧凑模型的后缀标识
const CompactModelSuffix = "-openai-compact"

// CompactWildcardModelKey 通配符形式的紧凑模型键，用于匹配所有紧凑模型
const CompactWildcardModelKey = "*" + CompactModelSuffix

// WithCompactModelSuffix 为模型名添加紧凑后缀
// 如果模型名已包含该后缀则原样返回，避免重复添加
// 参数：
//   - modelName: 原始模型名称
//
// 返回值：添加后缀后的模型名称
func WithCompactModelSuffix(modelName string) string {
	if strings.HasSuffix(modelName, CompactModelSuffix) {
		return modelName
	}
	return modelName + CompactModelSuffix
}
