// grok.go — Grok 模型配置管理
// 职责：定义和管理 Grok（xAI）模型的专属配置，包括违规扣费功能开关
// 和扣费金额等设置。通过 config.GlobalConfig 注册实现持久化存储。

package model_setting

import "github.com/c1cada/NexusTok/setting/config"

// GrokSettings 定义 Grok 模型的配置结构体
type GrokSettings struct {
	// ViolationDeductionEnabled 控制是否启用违规扣费功能
	ViolationDeductionEnabled bool `json:"violation_deduction_enabled"`
	// ViolationDeductionAmount 设置每次违规的扣费金额（美元）
	ViolationDeductionAmount float64 `json:"violation_deduction_amount"`
}

// defaultGrokSettings 是 Grok 配置的默认值
var defaultGrokSettings = GrokSettings{
	ViolationDeductionEnabled: true,
	ViolationDeductionAmount:  0.05,
}

// grokSettings 是当前生效的 Grok 配置实例
var grokSettings = defaultGrokSettings

// init 注册 Grok 配置到全局配置管理系统
func init() {
	config.GlobalConfig.Register("grok", &grokSettings)
}

// GetGrokSettings 获取当前 Grok 配置的指针
// 返回值：指向当前 Grok 配置的指针
func GetGrokSettings() *GrokSettings {
	return &grokSettings
}
