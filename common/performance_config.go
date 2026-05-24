// Package common - performance_config.go
// 该文件定义了性能监控配置
//
// 性能监控用于检测系统资源使用情况：
// - CPU 使用率
// - 内存使用率
// - 磁盘使用率
//
// 当资源使用率超过阈值时，系统会发出告警
// 配置通过原子操作保证并发安全
package common

import "sync/atomic"

// PerformanceMonitorConfig 性能监控配置
//
// 配置各项资源的监控阈值
// 当资源使用率超过阈值时触发告警
type PerformanceMonitorConfig struct {
	Enabled         bool // 是否启用性能监控
	CPUThreshold    int  // CPU 使用率阈值（百分比）
	MemoryThreshold int  // 内存使用率阈值（百分比）
	DiskThreshold   int  // 磁盘使用率阈值（百分比）
}

var performanceMonitorConfig atomic.Value // 全局性能监控配置（原子操作）

func init() {
	// 初始化默认配置
	performanceMonitorConfig.Store(PerformanceMonitorConfig{
		Enabled:         true, // 默认启用
		CPUThreshold:    90,   // CPU 阈值 90%
		MemoryThreshold: 90,   // 内存阈值 90%
		DiskThreshold:   90,   // 磁盘阈值 90%
	})
}

// GetPerformanceMonitorConfig 获取性能监控配置
//
// 返回值：
//   - PerformanceMonitorConfig: 当前配置
func GetPerformanceMonitorConfig() PerformanceMonitorConfig {
	return performanceMonitorConfig.Load().(PerformanceMonitorConfig)
}

// SetPerformanceMonitorConfig 设置性能监控配置
//
// 参数：
//   - config: 新的配置
func SetPerformanceMonitorConfig(config PerformanceMonitorConfig) {
	performanceMonitorConfig.Store(config)
}
