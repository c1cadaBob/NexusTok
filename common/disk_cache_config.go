// Package common - disk_cache_config.go
// 该文件实现了磁盘缓存的配置管理和统计信息追踪
//
// 磁盘缓存用于存储大请求体，避免占用过多内存
// 配置通过 performance_setting 包动态更新
// 统计信息通过原子操作保证并发安全
//
// 配置项：
// - Enabled: 是否启用磁盘缓存
// - ThresholdMB: 触发磁盘缓存的请求体大小阈值（MB）
// - MaxSizeMB: 磁盘缓存最大总大小（MB）
// - Path: 磁盘缓存目录
package common

import (
	"sync"
	"sync/atomic"
)

// DiskCacheConfig 磁盘缓存配置（由 performance_setting 包更新）
type DiskCacheConfig struct {
	Enabled     bool   // 是否启用磁盘缓存
	ThresholdMB int    // 触发磁盘缓存的请求体大小阈值（MB）
	MaxSizeMB   int    // 磁盘缓存最大总大小（MB）
	Path        string // 磁盘缓存目录（空字符串表示使用系统临时目录）
}

// 全局磁盘缓存配置（默认值）
var diskCacheConfig = DiskCacheConfig{
	Enabled:     false, // 默认禁用
	ThresholdMB: 10,    // 默认阈值 10MB
	MaxSizeMB:   1024,  // 默认最大 1GB
	Path:        "",    // 默认使用系统临时目录
}
var diskCacheConfigMu sync.RWMutex // 配置读写锁

// GetDiskCacheConfig 获取磁盘缓存配置
//
// 返回值：
//   - DiskCacheConfig: 当前配置的副本
func GetDiskCacheConfig() DiskCacheConfig {
	diskCacheConfigMu.RLock()
	defer diskCacheConfigMu.RUnlock()
	return diskCacheConfig
}

// SetDiskCacheConfig 设置磁盘缓存配置
//
// 参数：
//   - config: 新的配置
func SetDiskCacheConfig(config DiskCacheConfig) {
	diskCacheConfigMu.Lock()
	defer diskCacheConfigMu.Unlock()
	diskCacheConfig = config
}

// IsDiskCacheEnabled 是否启用磁盘缓存
//
// 返回值：
//   - bool: 是否启用
func IsDiskCacheEnabled() bool {
	diskCacheConfigMu.RLock()
	defer diskCacheConfigMu.RUnlock()
	return diskCacheConfig.Enabled
}

// GetDiskCacheThresholdBytes 获取磁盘缓存阈值（字节）
//
// 返回值：
//   - int64: 阈值（字节），通过位移运算将 MB 转换为字节
func GetDiskCacheThresholdBytes() int64 {
	diskCacheConfigMu.RLock()
	defer diskCacheConfigMu.RUnlock()
	return int64(diskCacheConfig.ThresholdMB) << 20 // MB → 字节
}

// GetDiskCacheMaxSizeBytes 获取磁盘缓存最大大小（字节）
//
// 返回值：
//   - int64: 最大大小（字节）
func GetDiskCacheMaxSizeBytes() int64 {
	diskCacheConfigMu.RLock()
	defer diskCacheConfigMu.RUnlock()
	return int64(diskCacheConfig.MaxSizeMB) << 20 // MB → 字节
}

// GetDiskCachePath 获取磁盘缓存目录
//
// 返回值：
//   - string: 缓存目录路径
func GetDiskCachePath() string {
	diskCacheConfigMu.RLock()
	defer diskCacheConfigMu.RUnlock()
	return diskCacheConfig.Path
}

// DiskCacheStats 磁盘缓存统计信息
//
// 用于监控缓存使用情况和性能
// 所有字段都通过原子操作读写，保证并发安全
type DiskCacheStats struct {
	ActiveDiskFiles         int64 `json:"active_disk_files"`          // 当前活跃的磁盘缓存文件数
	CurrentDiskUsageBytes   int64 `json:"current_disk_usage_bytes"`   // 当前磁盘缓存总大小（字节）
	ActiveMemoryBuffers     int64 `json:"active_memory_buffers"`      // 当前内存缓存数量
	CurrentMemoryUsageBytes int64 `json:"current_memory_usage_bytes"` // 当前内存缓存总大小（字节）
	DiskCacheHits           int64 `json:"disk_cache_hits"`            // 磁盘缓存命中次数
	MemoryCacheHits         int64 `json:"memory_cache_hits"`          // 内存缓存命中次数
	DiskCacheMaxBytes       int64 `json:"disk_cache_max_bytes"`       // 磁盘缓存最大限制（字节）
	DiskCacheThresholdBytes int64 `json:"disk_cache_threshold_bytes"` // 磁盘缓存阈值（字节）
}

var diskCacheStats DiskCacheStats // 全局缓存统计信息

// GetDiskCacheStats 获取缓存统计信息
//
// 使用原子操作读取所有统计字段，保证数据一致性
//
// 返回值：
//   - DiskCacheStats: 统计信息副本
func GetDiskCacheStats() DiskCacheStats {
	stats := DiskCacheStats{
		ActiveDiskFiles:         atomic.LoadInt64(&diskCacheStats.ActiveDiskFiles),
		CurrentDiskUsageBytes:   atomic.LoadInt64(&diskCacheStats.CurrentDiskUsageBytes),
		ActiveMemoryBuffers:     atomic.LoadInt64(&diskCacheStats.ActiveMemoryBuffers),
		CurrentMemoryUsageBytes: atomic.LoadInt64(&diskCacheStats.CurrentMemoryUsageBytes),
		DiskCacheHits:           atomic.LoadInt64(&diskCacheStats.DiskCacheHits),
		MemoryCacheHits:         atomic.LoadInt64(&diskCacheStats.MemoryCacheHits),
		DiskCacheMaxBytes:       GetDiskCacheMaxSizeBytes(),
		DiskCacheThresholdBytes: GetDiskCacheThresholdBytes(),
	}
	return stats
}

// IncrementDiskFiles 增加磁盘文件计数
//
// 在创建新的磁盘缓存文件时调用
//
// 参数：
//   - size: 文件大小（字节）
func IncrementDiskFiles(size int64) {
	atomic.AddInt64(&diskCacheStats.ActiveDiskFiles, 1)
	atomic.AddInt64(&diskCacheStats.CurrentDiskUsageBytes, size)
}

// DecrementDiskFiles 减少磁盘文件计数
//
// 在删除磁盘缓存文件时调用
// 使用下溢保护，防止计数变为负数
//
// 参数：
//   - size: 文件大小（字节）
func DecrementDiskFiles(size int64) {
	if atomic.AddInt64(&diskCacheStats.ActiveDiskFiles, -1) < 0 {
		atomic.StoreInt64(&diskCacheStats.ActiveDiskFiles, 0)
	}
	if atomic.AddInt64(&diskCacheStats.CurrentDiskUsageBytes, -size) < 0 {
		atomic.StoreInt64(&diskCacheStats.CurrentDiskUsageBytes, 0)
	}
}

// IncrementMemoryBuffers 增加内存缓存计数
//
// 在创建新的内存缓存时调用
//
// 参数：
//   - size: 缓存大小（字节）
func IncrementMemoryBuffers(size int64) {
	atomic.AddInt64(&diskCacheStats.ActiveMemoryBuffers, 1)
	atomic.AddInt64(&diskCacheStats.CurrentMemoryUsageBytes, size)
}

// DecrementMemoryBuffers 减少内存缓存计数
//
// 在释放内存缓存时调用
//
// 参数：
//   - size: 缓存大小（字节）
func DecrementMemoryBuffers(size int64) {
	atomic.AddInt64(&diskCacheStats.ActiveMemoryBuffers, -1)
	atomic.AddInt64(&diskCacheStats.CurrentMemoryUsageBytes, -size)
}

// IncrementDiskCacheHits 增加磁盘缓存命中次数
func IncrementDiskCacheHits() {
	atomic.AddInt64(&diskCacheStats.DiskCacheHits, 1)
}

// IncrementMemoryCacheHits 增加内存缓存命中次数
func IncrementMemoryCacheHits() {
	atomic.AddInt64(&diskCacheStats.MemoryCacheHits, 1)
}

// ResetDiskCacheStats 重置命中统计信息（不重置当前使用量）
func ResetDiskCacheStats() {
	atomic.StoreInt64(&diskCacheStats.DiskCacheHits, 0)
	atomic.StoreInt64(&diskCacheStats.MemoryCacheHits, 0)
}

// ResetDiskCacheUsage 重置磁盘缓存使用量统计（用于清理缓存后）
func ResetDiskCacheUsage() {
	atomic.StoreInt64(&diskCacheStats.ActiveDiskFiles, 0)
	atomic.StoreInt64(&diskCacheStats.CurrentDiskUsageBytes, 0)
}

// SyncDiskCacheStats 从实际磁盘状态同步统计信息
//
// 用于修正统计与实际不符的情况
// 遍历缓存目录，统计实际文件数量和总大小
func SyncDiskCacheStats() {
	fileCount, totalSize, err := GetDiskCacheInfo()
	if err != nil {
		return
	}
	atomic.StoreInt64(&diskCacheStats.ActiveDiskFiles, int64(fileCount))
	atomic.StoreInt64(&diskCacheStats.CurrentDiskUsageBytes, totalSize)
}

// IsDiskCacheAvailable 检查是否可以创建新的磁盘缓存
//
// 检查条件：
// 1. 磁盘缓存已启用
// 2. 当前使用量 + 新请求大小 <= 最大限制
//
// 参数：
//   - requestSize: 请求体大小（字节）
//
// 返回值：
//   - bool: 是否可以创建新的磁盘缓存
func IsDiskCacheAvailable(requestSize int64) bool {
	if !IsDiskCacheEnabled() {
		return false
	}
	maxBytes := GetDiskCacheMaxSizeBytes()
	currentUsage := atomic.LoadInt64(&diskCacheStats.CurrentDiskUsageBytes)
	return currentUsage+requestSize <= maxBytes
}
