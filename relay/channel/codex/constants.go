// 本文件定义了 Codex 渠道的模型列表和渠道名称常量。
// 包含 GPT-5 系列及其 Codex 变体模型。
package codex

import (
	"github.com/c1cada/NexusTok/setting/ratio_setting" // 模型比例/后缀设置
	"github.com/samber/lo"                               // Go 工具库
)

// baseModelList 定义了 Codex 渠道支持的基础模型列表。
// 包含 GPT-5 及其各种 Codex 代码生成变体。
var baseModelList = []string{
	"gpt-5", "gpt-5-codex", "gpt-5-codex-mini",
	"gpt-5.1", "gpt-5.1-codex", "gpt-5.1-codex-max", "gpt-5.1-codex-mini",
	"gpt-5.2", "gpt-5.2-codex", "gpt-5.3-codex", "gpt-5.3-codex-spark",
	"gpt-5.4",
}

// ModelList 是最终的模型列表，在基础模型列表基础上追加了带有紧凑模型后缀的变体。
// 紧凑模型后缀用于计费/配比场景中的模型标识。
var ModelList = withCompactModelSuffix(baseModelList)

// ChannelName 定义了渠道名称标识符。
const ChannelName = "codex"

// withCompactModelSuffix 为模型列表中的每个模型追加带紧凑后缀的变体，并去重。
// 例如 "gpt-5" 会额外生成 "gpt-5<compact-suffix>" 形式的模型名。
// 参数：models - 原始模型名列表。
// 返回值：包含原始模型和带后缀模型的去重列表。
func withCompactModelSuffix(models []string) []string {
	out := make([]string, 0, len(models)*2)
	out = append(out, models...)
	out = append(out, lo.Map(models, func(model string, _ int) string {
		return ratio_setting.WithCompactModelSuffix(model)
	})...)
	return lo.Uniq(out)
}
