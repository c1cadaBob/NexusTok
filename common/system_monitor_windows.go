// Package common - system_monitor_windows.go
// 该文件实现了 Windows 平台的磁盘空间信息获取
//
// 使用 Windows API GetDiskFreeSpaceExW 获取磁盘空间信息
// 通过 go:build windows 构建标签限定仅 Windows 平台编译
//
// 实现方式：
// - 使用 syscall.NewLazyDLL 延迟加载 kernel32.dll
// - 使用 GetDiskFreeSpaceExW 进程获取磁盘空间
// - 使用 UTF16PtrFromString 转换路径为 Windows API 所需的 UTF-16 指针
//
//go:build windows

package common

import (
	"os"
	"syscall"
	"unsafe"
)

// GetDiskSpaceInfo 获取缓存目录所在磁盘的空间信息 (Windows)
//
// 获取流程：
// 1. 获取缓存目录路径（GetDiskCachePath）
// 2. 如果缓存目录为空，使用系统临时目录
// 3. 加载 kernel32.dll 并调用 GetDiskFreeSpaceExW
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

	// 延迟加载 kernel32.dll
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	// 获取 GetDiskFreeSpaceExW 函数引用
	getDiskFreeSpaceEx := kernel32.NewProc("GetDiskFreeSpaceExW")

	var freeBytesAvailable, totalBytes, totalFreeBytes uint64

	// 将路径转换为 Windows API 所需的 UTF-16 指针
	pathPtr, err := syscall.UTF16PtrFromString(cachePath)
	if err != nil {
		return info
	}

	// 调用 GetDiskFreeSpaceExW 获取磁盘空间信息
	// 参数：目录路径、可用字节数（调用者可用）、总字节数、总空闲字节数
	ret, _, _ := getDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFreeBytes)),
	)

	// 返回值为 0 表示调用失败
	if ret == 0 {
		return info
	}

	// 填充磁盘空间信息
	info.Total = totalBytes                           // 总空间
	info.Free = freeBytesAvailable                    // 调用者可用空间
	info.Used = totalBytes - totalFreeBytes           // 已用空间

	// 计算使用百分比
	if info.Total > 0 {
		info.UsedPercent = float64(info.Used) / float64(info.Total) * 100
	}

	return info
}
