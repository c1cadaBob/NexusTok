// Package common - disk_cache.go
// 该文件实现了磁盘缓存的文件操作
//
// 提供磁盘缓存文件的创建、读写、删除和清理功能
// 使用统一的缓存目录管理，支持按类型分类存储
//
// 缓存类型：
// - body: 请求体缓存（用于大请求的临时存储）
// - file: 文件数据缓存（用于文件上传等场景）
//
// 文件命名规则：{type}-{uuid}-{timestamp}.tmp
package common

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// DiskCacheType 磁盘缓存类型
type DiskCacheType string

const (
	DiskCacheTypeBody DiskCacheType = "body" // 请求体缓存
	DiskCacheTypeFile DiskCacheType = "file" // 文件数据缓存
)

// 统一的缓存目录名
const diskCacheDir = "nexustok-body-cache"

// GetDiskCacheDir 获取统一的磁盘缓存目录
//
// 每次调用都会重新计算，以响应配置变化
// 如果未配置缓存路径，使用系统临时目录
//
// 返回值：
//   - string: 缓存目录的完整路径
func GetDiskCacheDir() string {
	cachePath := GetDiskCachePath()
	if cachePath == "" {
		cachePath = os.TempDir()
	}
	return filepath.Join(cachePath, diskCacheDir)
}

// EnsureDiskCacheDir 确保缓存目录存在
//
// 如果目录不存在则创建，权限为 0755
//
// 返回值：
//   - error: 创建错误
func EnsureDiskCacheDir() error {
	dir := GetDiskCacheDir()
	return os.MkdirAll(dir, 0755)
}

// CreateDiskCacheFile 创建磁盘缓存文件
//
// 文件命名规则：{type}-{uuid前8位}-{时间戳}.tmp
// 使用 O_EXCL 标志确保文件不存在时才创建，防止冲突
//
// 参数：
//   - cacheType: 缓存类型（body/file）
//
// 返回值：
//   - string: 文件路径
//   - *os.File: 文件句柄
//   - error: 创建错误
func CreateDiskCacheFile(cacheType DiskCacheType) (string, *os.File, error) {
	if err := EnsureDiskCacheDir(); err != nil {
		return "", nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	dir := GetDiskCacheDir()
	// 文件名格式：{type}-{uuid前8位}-{时间戳}.tmp
	filename := fmt.Sprintf("%s-%s-%d.tmp", cacheType, uuid.New().String()[:8], time.Now().UnixNano())
	filePath := filepath.Join(dir, filename)

	// 使用 O_EXCL 标志确保文件不存在时才创建
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_RDWR|os.O_EXCL, 0600)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create cache file: %w", err)
	}

	return filePath, file, nil
}

// WriteDiskCacheFile 写入数据到磁盘缓存文件
//
// 创建文件并写入数据，写入完成后关闭文件
// 如果写入失败，自动清理已创建的文件
//
// 参数：
//   - cacheType: 缓存类型
//   - data: 要写入的数据
//
// 返回值：
//   - string: 文件路径
//   - error: 写入错误
func WriteDiskCacheFile(cacheType DiskCacheType, data []byte) (string, error) {
	filePath, file, err := CreateDiskCacheFile(cacheType)
	if err != nil {
		return "", err
	}

	_, err = file.Write(data)
	if err != nil {
		file.Close()
		os.Remove(filePath)
		return "", fmt.Errorf("failed to write cache file: %w", err)
	}

	if err := file.Close(); err != nil {
		os.Remove(filePath)
		return "", fmt.Errorf("failed to close cache file: %w", err)
	}

	return filePath, nil
}

// WriteDiskCacheFileString 写入字符串到磁盘缓存文件
//
// 参数：
//   - cacheType: 缓存类型
//   - data: 要写入的字符串
//
// 返回值：
//   - string: 文件路径
//   - error: 写入错误
func WriteDiskCacheFileString(cacheType DiskCacheType, data string) (string, error) {
	return WriteDiskCacheFile(cacheType, []byte(data))
}

// ReadDiskCacheFile 读取磁盘缓存文件
//
// 参数：
//   - filePath: 文件路径
//
// 返回值：
//   - []byte: 文件内容
//   - error: 读取错误
func ReadDiskCacheFile(filePath string) ([]byte, error) {
	return os.ReadFile(filePath)
}

// ReadDiskCacheFileString 读取磁盘缓存文件为字符串
//
// 参数：
//   - filePath: 文件路径
//
// 返回值：
//   - string: 文件内容
//   - error: 读取错误
func ReadDiskCacheFileString(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// RemoveDiskCacheFile 删除磁盘缓存文件
//
// 参数：
//   - filePath: 文件路径
//
// 返回值：
//   - error: 删除错误
func RemoveDiskCacheFile(filePath string) error {
	return os.Remove(filePath)
}

// CleanupOldDiskCacheFiles 清理旧的缓存文件
//
// 遍历缓存目录，删除超过指定存活时间的文件
// 删除时会更新全局统计信息
//
// 注意：此函数只删除文件，不更新统计中的文件大小（因为无法得知每个文件的原始大小）
//
// 参数：
//   - maxAge: 文件最大存活时间
//
// 返回值：
//   - error: 清理错误（目录不存在不算错误）
func CleanupOldDiskCacheFiles(maxAge time.Duration) error {
	dir := GetDiskCacheDir()

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 目录不存在，无需清理
		}
		return err
	}

	now := time.Now()
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if now.Sub(info.ModTime()) > maxAge {
			// 注意：后台清理任务删除文件时，由于无法得知原始 base64Size，
			// 只能按磁盘文件大小扣减。这在目前 base64 存储模式下是准确的。
			if err := os.Remove(filepath.Join(dir, entry.Name())); err == nil {
				DecrementDiskFiles(info.Size())
			}
		}
	}
	return nil
}

// GetDiskCacheInfo 获取磁盘缓存目录信息
//
// 遍历缓存目录，统计文件数量和总大小
//
// 返回值：
//   - fileCount: 文件数量
//   - totalSize: 总大小（字节）
//   - err: 读取错误
func GetDiskCacheInfo() (fileCount int, totalSize int64, err error) {
	dir := GetDiskCacheDir()

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, nil
		}
		return 0, 0, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		fileCount++
		totalSize += info.Size()
	}
	return fileCount, totalSize, nil
}

// ShouldUseDiskCache 判断是否应该使用磁盘缓存
//
// 判断条件：
// 1. 磁盘缓存已启用
// 2. 数据大小 >= 阈值
// 3. 磁盘缓存可用（未超过最大限制）
//
// 参数：
//   - dataSize: 数据大小（字节）
//
// 返回值：
//   - bool: 是否应该使用磁盘缓存
func ShouldUseDiskCache(dataSize int64) bool {
	if !IsDiskCacheEnabled() {
		return false
	}
	threshold := GetDiskCacheThresholdBytes()
	if dataSize < threshold {
		return false
	}
	return IsDiskCacheAvailable(dataSize)
}
