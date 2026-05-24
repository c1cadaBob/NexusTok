// redisqueue - usage_toggle.go
// 本文件提供使用统计队列的独立开关控制。
// 通过原子布尔值实现线程安全的开关切换，与队列功能开关分离，
// 允许在队列功能启用的情况下单独控制是否记录使用数据。
package redisqueue

import "sync/atomic"

// usageStatisticsEnabled 是使用统计功能的原子开关。
// 默认为启用状态。
var usageStatisticsEnabled atomic.Bool

// init 将使用统计开关的初始值设为 true（启用）。
func init() {
	usageStatisticsEnabled.Store(true)
}

// SetUsageStatisticsEnabled toggles whether usage records are enqueued into the redisqueue payload buffer.
// This is controlled by the config field `usage-statistics-enabled` and the corresponding management API.
func SetUsageStatisticsEnabled(enabled bool) { usageStatisticsEnabled.Store(enabled) }

// UsageStatisticsEnabled reports whether the usage queue plugin should publish records.
func UsageStatisticsEnabled() bool { return usageStatisticsEnabled.Load() }
