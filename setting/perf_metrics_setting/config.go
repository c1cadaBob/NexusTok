// config.go — 性能指标采集配置管理
// 职责：管理性能指标（Performance Metrics）采集功能的配置，
// 包括功能开关、数据刷新间隔、聚合时间桶大小和数据保留天数。
// 通过 config.GlobalConfig 注册实现持久化存储。

package perf_metrics_setting

import "github.com/c1cada/NexusTok/setting/config"

// PerfMetricsSetting 性能指标采集配置结构体
type PerfMetricsSetting struct {
	// Enabled 控制是否启用性能指标采集功能
	Enabled bool `json:"enabled"`
	// FlushInterval 数据刷新到存储的间隔时间（单位：分钟）
	FlushInterval int `json:"flush_interval"`
	// BucketTime 聚合时间桶的大小，可选值: "minute"、"5min"、"hour"
	BucketTime string `json:"bucket_time"`
	// RetentionDays 数据保留天数，0 表示永不删除
	RetentionDays int `json:"retention_days"`
}

// perfMetricsSetting 是全局性能指标配置实例，默认启用
var perfMetricsSetting = PerfMetricsSetting{
	Enabled:       true,     // 默认启用
	FlushInterval: 5,        // 默认 5 分钟刷新一次
	BucketTime:    "hour",   // 默认按小时聚合
	RetentionDays: 0,        // 默认永不删除
}

// init 注册性能指标配置到全局配置管理系统
func init() {
	config.GlobalConfig.Register("perf_metrics_setting", &perfMetricsSetting)
}

// GetSetting 获取当前性能指标配置的副本
// 返回值：当前配置的值拷贝
func GetSetting() PerfMetricsSetting {
	return perfMetricsSetting
}

// GetBucketSeconds 将 BucketTime 配置转换为秒数
// 返回值：时间桶的秒数，未知值默认回退到 3600（1 小时）
func GetBucketSeconds() int64 {
	switch perfMetricsSetting.BucketTime {
	case "minute":
		return 60
	case "5min":
		return 300
	case "hour":
		return 3600
	default:
		return 3600
	}
}

// GetFlushIntervalMinutes 获取刷新间隔（分钟），确保最小值为 1
// 返回值：刷新间隔分钟数
func GetFlushIntervalMinutes() int {
	if perfMetricsSetting.FlushInterval < 1 {
		return 1
	}
	return perfMetricsSetting.FlushInterval
}
