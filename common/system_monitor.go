// Package common - system_monitor.go
// 该文件实现了系统资源监控功能
//
// 监控指标：
// - CPU 使用率：使用 gopsutil/cpu 获取整体 CPU 使用百分比
// - 内存使用率：使用 gopsutil/mem 获取虚拟内存使用百分比
// - 磁盘使用率：使用平台特定的 GetDiskSpaceInfo 获取磁盘空间信息
//
// 工作机制：
// - 启动后台协程，每 5 秒采集一次系统状态
// - 使用 atomic.Value 无锁存储最新状态，保证读取的高性能
// - 可通过 PerformanceMonitorConfig 配置开关控制是否启用
//
// 使用场景：
// - 系统状态仪表盘展示
// - 自动告警（CPU/内存/磁盘超阈值）
// - pprof 自动触发（CPU > 80%）
package common

import (
	"sync/atomic"
	"time"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/mem"
)

// DiskSpaceInfo 磁盘空间信息
//
// 用于存储磁盘使用情况的统计数据，由平台特定的 GetDiskSpaceInfo 函数填充
type DiskSpaceInfo struct {
	// Total 磁盘总空间（字节）
	Total uint64 `json:"total"`
	// Free 磁盘可用空间（字节）
	Free uint64 `json:"free"`
	// Used 磁盘已用空间（字节）
	Used uint64 `json:"used"`
	// UsedPercent 磁盘使用百分比（0-100）
	UsedPercent float64 `json:"used_percent"`
}

// SystemStatus 系统状态信息
//
// 存储 CPU、内存、磁盘的使用率快照
// 通过 atomic.Value 无锁存储，支持高并发读取
type SystemStatus struct {
	// CPUUsage CPU 使用百分比（0-100）
	CPUUsage float64
	// MemoryUsage 内存使用百分比（0-100）
	MemoryUsage float64
	// DiskUsage 磁盘使用百分比（0-100）
	DiskUsage float64
}

// latestSystemStatus 使用 atomic.Value 存储最新的系统状态
//
// 设计原因：
// - atomic.Value 提供无锁读取，适合高频查询场景
// - 避免在每次查询时重新采集系统指标
// - 后台协程定期更新，前端/监控接口直接读取
var latestSystemStatus atomic.Value

// init 初始化系统状态为零值
func init() {
	latestSystemStatus.Store(SystemStatus{})
}

// StartSystemMonitor 启动系统资源监控的后台协程
//
// 监控逻辑：
// 1. 检查 PerformanceMonitorConfig 是否启用监控
// 2. 如果未启用，每 30 秒检查一次配置
// 3. 如果启用，每 5 秒采集一次系统状态
//
// 该函数通常在服务启动时调用一次，后台协程会持续运行
func StartSystemMonitor() {
	go func() {
		for {
			// 获取性能监控配置
			config := GetPerformanceMonitorConfig()
			if !config.Enabled {
				// 监控未启用，等待较长时间后重新检查
				time.Sleep(30 * time.Second)
				continue
			}

			// 采集并更新系统状态
			updateSystemStatus()
			// 5 秒采集间隔
			time.Sleep(5 * time.Second)
		}
	}()
}

// updateSystemStatus 采集当前系统资源使用情况并更新状态
//
// 采集指标：
// - CPU 使用率：cpu.Percent(0, false) 返回自上次调用以来的整体使用率
// - 内存使用率：mem.VirtualMemory() 返回虚拟内存统计信息
// - 磁盘使用率：GetDiskSpaceInfo() 返回缓存目录所在磁盘的空间信息
//
// 注意：CPU 使用率在首次调用时可能不准确，后续调用会逐渐正常
func updateSystemStatus() {
	var status SystemStatus

	// 采集 CPU 使用率
	// cpu.Percent(0, false) 返回自上次调用以来的 CPU 使用率
	// 第一个参数 0 表示不阻塞等待采样周期
	// 第二个 false 表示返回整体使用率（非每个核心）
	percents, err := cpu.Percent(0, false)
	if err == nil && len(percents) > 0 {
		status.CPUUsage = percents[0]
	}

	// 采集内存使用率
	memInfo, err := mem.VirtualMemory()
	if err == nil {
		status.MemoryUsage = memInfo.UsedPercent
	}

	// 采集磁盘使用率
	diskInfo := GetDiskSpaceInfo()
	if diskInfo.Total > 0 {
		status.DiskUsage = diskInfo.UsedPercent
	}

	// 原子更新系统状态
	latestSystemStatus.Store(status)
}

// GetSystemStatus 获取最新的系统状态快照
//
// 返回值：
//   - SystemStatus: 包含 CPU、内存、磁盘使用率的状态结构体
//
// 该方法是无锁的，适合高频调用（如前端轮询）
func GetSystemStatus() SystemStatus {
	return latestSystemStatus.Load().(SystemStatus)
}
