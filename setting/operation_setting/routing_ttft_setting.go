package operation_setting

import "github.com/c1cada/NexusTok/setting/config"

// RoutingTTFTSetting 控制候选首包延迟保护以及它对亲和命中路径的影响。
// 配置通过系统 option 持久化，不需要新增数据库表结构。
type RoutingTTFTSetting struct {
	// Enabled 控制普通候选选路是否启用 TTFT 冷却保护。
	Enabled bool `json:"enabled"`
	// ApplyToAffinity 控制亲和命中时是否也过滤处于 TTFT 冷却期的候选。
	ApplyToAffinity bool `json:"apply_to_affinity"`
	// ThresholdMs 首包延迟阈值，达到该值的样本会参与慢候选判定。
	ThresholdMs int `json:"threshold_ms"`
	// CooldownSeconds 慢候选进入冷却后的持续时间。
	CooldownSeconds int `json:"cooldown_seconds"`
	// MinSamples 触发冷却前要求的最少样本数。
	MinSamples int `json:"min_samples"`
}

var routingTTFTSetting = RoutingTTFTSetting{
	Enabled:         true,
	ApplyToAffinity: true,
	ThresholdMs:     800,
	CooldownSeconds: 90,
	MinSamples:      2,
}

func init() {
	config.GlobalConfig.Register("routing_ttft_setting", &routingTTFTSetting)
}

// GetRoutingTTFTSetting 返回当前候选首包延迟配置。
func GetRoutingTTFTSetting() *RoutingTTFTSetting {
	return &routingTTFTSetting
}
