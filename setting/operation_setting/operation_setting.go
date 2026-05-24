// operation_setting.go — 运维配置管理
// 职责：管理运维相关的全局配置，包括演示站点开关、自用模式开关
// 以及渠道自动禁用关键词列表。当上游返回包含特定错误关键词的响应时，
// 系统可自动禁用对应渠道，避免无效请求。

package operation_setting

import "strings"

// DemoSiteEnabled 控制是否启用演示站点模式（限制部分功能）
var DemoSiteEnabled = false

// SelfUseModeEnabled 控制是否启用自用模式（面向个人使用的简化模式）
var SelfUseModeEnabled = false

// AutomaticDisableKeywords 自动禁用渠道的关键词列表
// 当上游响应中包含以下关键词时，系统会自动禁用对应渠道
var AutomaticDisableKeywords = []string{
	"Your credit balance is too low",
	"This organization has been disabled.",
	"You exceeded your current quota",
	"Permission denied",
	"The security token included in the request is invalid",
	"Operation not allowed",
	"Your account is not authorized",
}

// AutomaticDisableKeywordsToString 将关键词列表转换为换行分隔的字符串
// 返回值：每个关键词占一行的字符串
func AutomaticDisableKeywordsToString() string {
	return strings.Join(AutomaticDisableKeywords, "\n")
}

// AutomaticDisableKeywordsFromString 从换行分隔的字符串解析关键词列表
// 会自动去除空白字符并转为小写
// 参数：
//   - s: 换行分隔的关键词字符串
func AutomaticDisableKeywordsFromString(s string) {
	AutomaticDisableKeywords = []string{}
	ak := strings.Split(s, "\n")
	for _, k := range ak {
		k = strings.TrimSpace(k)
		k = strings.ToLower(k)
		if k != "" {
			AutomaticDisableKeywords = append(AutomaticDisableKeywords, k)
		}
	}
}
