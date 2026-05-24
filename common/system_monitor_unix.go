// Package common - system_monitor_unix.go
// 该文件实现了 Unix/Linux/macOS 平台的磁盘空间信息获取
//
// 使用 golang.org/x/sys/unix 包调用系统调用 statfs 获取磁盘空间信息
// 通过 go:build !windows 构建标签排除 Windows 平台
//
// 兼容性：
// - 显式将 stat.Bsize 转换为 uint64，兼容 FreeBSD（其字段类型为 int64）
// - 使用 stat.Bavail（非特权用户可用空间）而非 stat.Bfree（所有可用空间）
//
//go:build !windows

package common

import (
	"os"

	"golang.org/x/sys/unix"
)

// GetDiskSpaceInfo 获取缓存目录所在磁盘的空间信息 (Unix/Linux/macOS)
//
// 获取流程：
// 1. 获取缓存目录路径（GetDiskCachePath）
// 2. 如果缓存目录为空，使用系统临时目录
// 3. 调用 unix.Statfs 获取文件系统统计信息
// 4. 计算总空间、可用空间、已用空间和使用百分比
//
// 返回值：
//   - DiskSpaceInfo: 磁盘空间信息结构体
func GetDiskSpaceInfo() DiskSpaceInfo {
	// 获取缓存目录路径
	cachePath := GetDiskCachePath()
	if cachePath == "" {
		// 降级到系统临时目录
		cachePath = os.TempDir()
	}

	info := DiskSpaceInfo{}

	// 调用 statfs 系统调用获取文件系统信息
	var stat unix.Statfs_t
	err := unix.Statfs(cachePath, &stat)
	if err != nil {
		return info
	}

	// 计算磁盘空间
	// 显式转换为 uint64 以兼容 FreeBSD（其 Bsize 字段类型为 int64）
	bsize := uint64(stat.Bsize)
	info.Total = uint64(stat.Blocks) * bsize      // 总空间 = 块数 * 块大小
	info.Free = uint64(stat.Bavail) * bsize         // 可用空间（非特权用户可用）
	info.Used = info.Total - uint64(stat.Bfree)*bsize // 已用空间 = 总空间 - 空闲空间

	// 计算使用百分比
	if info.Total > 0 {
		info.UsedPercent = float64(info.Used) / float64(info.Total) * 100
	}

	return info
}
