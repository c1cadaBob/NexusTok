// Package common - body_storage.go
// 该文件实现了请求体（Request Body）的存储抽象层
//
// 设计目标：
// - 提供统一的请求体存储接口，支持内存和磁盘两种存储后端
// - 根据请求体大小自动选择存储策略（小请求用内存，大请求用磁盘）
// - 支持请求体的重复读取（ReadSeeker 接口）和资源释放（Closer 接口）
// - 内置资源计数器，用于系统监控和内存管理
//
// 存储策略：
// - 内存存储（memoryStorage）：适用于小请求体，读写速度快
// - 磁盘存储（diskStorage）：适用于大请求体，避免占用过多内存
// - 切换阈值由 GetDiskCacheThresholdBytes() 配置
//
// 使用场景：
// - API 请求转发前的请求体缓存
// - 请求体大小验证
// - 请求体重复读取（如重试、日志记录等）
package common

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// BodyStorage 请求体存储接口
//
// 该接口组合了 io.ReadSeeker 和 io.Closer，提供：
// - Read: 读取请求体数据
// - Seek: 移动读取位置（支持重复读取）
// - Close: 释放存储资源
// - Bytes: 获取全部内容的字节切片
// - Size: 获取数据大小
// - IsDisk: 判断是否为磁盘存储
type BodyStorage interface {
	io.ReadSeeker
	io.Closer
	// Bytes 获取全部内容的字节切片
	Bytes() ([]byte, error)
	// Size 获取数据大小（字节）
	Size() int64
	// IsDisk 是否是磁盘存储（true=磁盘，false=内存）
	IsDisk() bool
}

// ErrStorageClosed 存储已关闭错误
// 当尝试操作已关闭的存储时返回此错误
var ErrStorageClosed = fmt.Errorf("body storage is closed")

// memoryStorage 内存存储实现
//
// 使用 bytes.Reader 作为底层读取器，支持 Read 和 Seek 操作
// 使用原子操作和互斥锁确保并发安全
// 通过引用计数器跟踪内存使用量
type memoryStorage struct {
	data   []byte        // 原始数据
	reader *bytes.Reader // 底层读取器（支持 Read 和 Seek）
	size   int64         // 数据大小（字节）
	closed int32         // 关闭标志（0=打开，1=关闭），使用原子操作
	mu     sync.Mutex    // 互斥锁，保护并发访问
}

// newMemoryStorage 创建内存存储实例
//
// 初始化流程：
// 1. 计算数据大小
// 2. 增加全局内存缓冲区计数器
// 3. 创建 bytes.Reader 用于支持 Read 和 Seek
//
// 参数：
//   - data: 要存储的字节数据
//
// 返回值：
//   - *memoryStorage: 内存存储实例
func newMemoryStorage(data []byte) *memoryStorage {
	size := int64(len(data))
	IncrementMemoryBuffers(size) // 增加全局内存缓冲区计数
	return &memoryStorage{
		data:   data,
		reader: bytes.NewReader(data),
		size:   size,
	}
}

// Read 从内存存储读取数据
//
// 实现 io.Reader 接口
// 使用互斥锁保护并发访问
// 如果存储已关闭，返回 ErrStorageClosed 错误
//
// 参数：
//   - p: 读取缓冲区
//
// 返回值：
//   - n: 实际读取的字节数
//   - err: 读取错误
func (m *memoryStorage) Read(p []byte) (n int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if atomic.LoadInt32(&m.closed) == 1 {
		return 0, ErrStorageClosed
	}
	return m.reader.Read(p)
}

// Seek 移动内存存储的读取位置
//
// 实现 io.Seeker 接口
// 使用互斥锁保护并发访问
// 如果存储已关闭，返回 ErrStorageClosed 错误
//
// 参数：
//   - offset: 偏移量
//   - whence: 起始位置（io.SeekStart/SeekCurrent/SeekEnd）
//
// 返回值：
//   - int64: 新的读取位置
//   - error: 定位错误
func (m *memoryStorage) Seek(offset int64, whence int) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if atomic.LoadInt32(&m.closed) == 1 {
		return 0, ErrStorageClosed
	}
	return m.reader.Seek(offset, whence)
}

// Close 关闭内存存储并释放资源
//
// 使用 CompareAndSwap 确保只执行一次关闭操作
// 关闭后会减少全局内存缓冲区计数器
//
// 返回值：
//   - error: 始终返回 nil
func (m *memoryStorage) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if atomic.CompareAndSwapInt32(&m.closed, 0, 1) {
		DecrementMemoryBuffers(m.size) // 减少全局内存缓冲区计数
	}
	return nil
}

// Bytes 获取内存存储的全部内容
//
// 返回原始数据的引用（非副本）
// 如果存储已关闭，返回 ErrStorageClosed 错误
//
// 返回值：
//   - []byte: 全部内容的字节切片
//   - error: 操作错误
func (m *memoryStorage) Bytes() ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if atomic.LoadInt32(&m.closed) == 1 {
		return nil, ErrStorageClosed
	}
	return m.data, nil
}

// Size 获取内存存储的数据大小
//
// 返回值：
//   - int64: 数据大小（字节）
func (m *memoryStorage) Size() int64 {
	return m.size
}

// IsDisk 判断是否为磁盘存储
//
// 返回值：
//   - bool: 始终返回 false（内存存储）
func (m *memoryStorage) IsDisk() bool {
	return false
}

// diskStorage 磁盘存储实现
//
// 使用临时文件作为底层存储，支持 Read、Seek 和 Close 操作
// 使用原子操作和互斥锁确保并发安全
// 通过引用计数器跟踪磁盘使用量
type diskStorage struct {
	file     *os.File   // 底层文件句柄
	filePath string     // 临时文件路径
	size     int64      // 数据大小（字节）
	closed   int32      // 关闭标志（0=打开，1=关闭），使用原子操作
	mu       sync.Mutex // 互斥锁，保护并发访问
}

// newDiskStorage 从字节数据创建磁盘存储实例
//
// 创建流程：
// 1. 使用统一的缓存目录管理创建临时文件
// 2. 将数据写入临时文件
// 3. 重置文件指针到开头
// 4. 增加全局磁盘文件计数器
//
// 参数：
//   - data: 要存储的字节数据
//   - cachePath: 缓存目录路径（当前未使用，由 CreateDiskCacheFile 统一管理）
//
// 返回值：
//   - *diskStorage: 磁盘存储实例
//   - error: 创建错误
func newDiskStorage(data []byte, cachePath string) (*diskStorage, error) {
	// 使用统一的缓存目录管理创建临时文件
	filePath, file, err := CreateDiskCacheFile(DiskCacheTypeBody)
	if err != nil {
		return nil, err
	}

	// 写入数据
	n, err := file.Write(data)
	if err != nil {
		file.Close()
		os.Remove(filePath)
		return nil, fmt.Errorf("failed to write to temp file: %w", err)
	}

	// 重置文件指针到开头，以便后续 Read 操作
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		file.Close()
		os.Remove(filePath)
		return nil, fmt.Errorf("failed to seek temp file: %w", err)
	}

	size := int64(n)
	IncrementDiskFiles(size) // 增加全局磁盘文件计数

	return &diskStorage{
		file:     file,
		filePath: filePath,
		size:     size,
	}, nil
}

// newDiskStorageFromReader 从 Reader 创建磁盘存储实例（用于大请求的流式处理）
//
// 创建流程：
// 1. 使用统一的缓存目录管理创建临时文件
// 2. 从 reader 流式读取数据并写入文件（限制最大读取量）
// 3. 如果数据超过最大限制，返回 ErrRequestBodyTooLarge 错误
// 4. 重置文件指针到开头
// 5. 增加全局磁盘文件计数器
//
// 参数：
//   - reader: 数据源读取器
//   - maxBytes: 最大允许的数据大小（字节）
//   - cachePath: 缓存目录路径（当前未使用）
//
// 返回值：
//   - *diskStorage: 磁盘存储实例
//   - error: 创建错误
func newDiskStorageFromReader(reader io.Reader, maxBytes int64, cachePath string) (*diskStorage, error) {
	// 使用统一的缓存目录管理创建临时文件
	filePath, file, err := CreateDiskCacheFile(DiskCacheTypeBody)
	if err != nil {
		return nil, err
	}

	// 从 reader 读取并写入文件
	// LimitReader 限制最大读取量为 maxBytes+1，用于检测是否超过限制
	written, err := io.Copy(file, io.LimitReader(reader, maxBytes+1))
	if err != nil {
		file.Close()
		os.Remove(filePath)
		return nil, fmt.Errorf("failed to write to temp file: %w", err)
	}

	// 检查数据是否超过最大限制
	if written > maxBytes {
		file.Close()
		os.Remove(filePath)
		return nil, ErrRequestBodyTooLarge
	}

	// 重置文件指针到开头
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		file.Close()
		os.Remove(filePath)
		return nil, fmt.Errorf("failed to seek temp file: %w", err)
	}

	IncrementDiskFiles(written) // 增加全局磁盘文件计数

	return &diskStorage{
		file:     file,
		filePath: filePath,
		size:     written,
	}, nil
}

// Read 从磁盘存储读取数据
//
// 实现 io.Reader 接口
// 使用互斥锁保护并发访问
// 如果存储已关闭，返回 ErrStorageClosed 错误
//
// 参数：
//   - p: 读取缓冲区
//
// 返回值：
//   - n: 实际读取的字节数
//   - err: 读取错误
func (d *diskStorage) Read(p []byte) (n int, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if atomic.LoadInt32(&d.closed) == 1 {
		return 0, ErrStorageClosed
	}
	return d.file.Read(p)
}

// Seek 移动磁盘存储的读取位置
//
// 实现 io.Seeker 接口
// 使用互斥锁保护并发访问
// 如果存储已关闭，返回 ErrStorageClosed 错误
//
// 参数：
//   - offset: 偏移量
//   - whence: 起始位置（io.SeekStart/SeekCurrent/SeekEnd）
//
// 返回值：
//   - int64: 新的读取位置
//   - error: 定位错误
func (d *diskStorage) Seek(offset int64, whence int) (int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if atomic.LoadInt32(&d.closed) == 1 {
		return 0, ErrStorageClosed
	}
	return d.file.Seek(offset, whence)
}

// Close 关闭磁盘存储并释放资源
//
// 使用 CompareAndSwap 确保只执行一次关闭操作
// 关闭流程：
// 1. 关闭文件句柄
// 2. 删除临时文件
// 3. 减少全局磁盘文件计数器
//
// 返回值：
//   - error: 始终返回 nil
func (d *diskStorage) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if atomic.CompareAndSwapInt32(&d.closed, 0, 1) {
		d.file.Close()
		os.Remove(d.filePath)
		DecrementDiskFiles(d.size) // 减少全局磁盘文件计数
	}
	return nil
}

// Bytes 获取磁盘存储的全部内容
//
// 读取流程：
// 1. 保存当前文件指针位置
// 2. 移动到文件开头
// 3. 读取全部内容
// 4. 恢复原来的文件指针位置
//
// 返回值：
//   - []byte: 全部内容的字节切片
//   - error: 读取错误
func (d *diskStorage) Bytes() ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if atomic.LoadInt32(&d.closed) == 1 {
		return nil, ErrStorageClosed
	}

	// 保存当前位置
	currentPos, err := d.file.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}

	// 移动到开头
	if _, err := d.file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	// 读取全部内容
	data := make([]byte, d.size)
	_, err = io.ReadFull(d.file, data)
	if err != nil {
		return nil, err
	}

	// 恢复位置
	if _, err := d.file.Seek(currentPos, io.SeekStart); err != nil {
		return nil, err
	}

	return data, nil
}

// Size 获取磁盘存储的数据大小
//
// 返回值：
//   - int64: 数据大小（字节）
func (d *diskStorage) Size() int64 {
	return d.size
}

// IsDisk 判断是否为磁盘存储
//
// 返回值：
//   - bool: 始终返回 true（磁盘存储）
func (d *diskStorage) IsDisk() bool {
	return true
}

// CreateBodyStorage 根据数据大小创建合适的存储
//
// 存储策略选择逻辑：
// 1. 如果启用了磁盘缓存且数据大小 >= 阈值且磁盘可用 → 使用磁盘存储
// 2. 否则 → 使用内存存储
// 3. 如果磁盘存储创建失败 → 回退到内存存储
//
// 参数：
//   - data: 要存储的字节数据
//
// 返回值：
//   - BodyStorage: 存储实例（内存或磁盘）
//   - error: 创建错误
func CreateBodyStorage(data []byte) (BodyStorage, error) {
	size := int64(len(data))
	threshold := GetDiskCacheThresholdBytes()

	// 检查是否应该使用磁盘缓存
	if IsDiskCacheEnabled() &&
		size >= threshold &&
		IsDiskCacheAvailable(size) {
		storage, err := newDiskStorage(data, GetDiskCachePath())
		if err != nil {
			// 如果磁盘存储失败，回退到内存存储
			SysError(fmt.Sprintf("failed to create disk storage, falling back to memory: %v", err))
			return newMemoryStorage(data), nil
		}
		return storage, nil
	}

	return newMemoryStorage(data), nil
}

// CreateBodyStorageFromReader 从 Reader 创建存储（用于大请求的流式处理）
//
// 存储策略选择逻辑：
// 1. 如果启用了磁盘缓存且 Content-Length >= 阈值且磁盘可用 → 直接使用磁盘存储（流式写入）
// 2. 否则 → 先读取到内存，再根据大小选择存储方式
//
// 注意：如果磁盘存储失败且 reader 已被消费，无法安全回退到内存存储
//
// 参数：
//   - reader: 数据源读取器
//   - contentLength: 请求头中的 Content-Length（可能为 -1 表示未知）
//   - maxBytes: 最大允许的数据大小（字节）
//
// 返回值：
//   - BodyStorage: 存储实例
//   - error: 创建错误
func CreateBodyStorageFromReader(reader io.Reader, contentLength int64, maxBytes int64) (BodyStorage, error) {
	threshold := GetDiskCacheThresholdBytes()

	// 如果启用了磁盘缓存且内容长度超过阈值，直接使用磁盘存储
	if IsDiskCacheEnabled() &&
		contentLength > 0 &&
		contentLength >= threshold &&
		IsDiskCacheAvailable(contentLength) {
		storage, err := newDiskStorageFromReader(reader, maxBytes, GetDiskCachePath())
		if err != nil {
			if IsRequestBodyTooLargeError(err) {
				return nil, err // 请求体过大，直接返回错误
			}
			// 磁盘存储失败，reader 已被消费，无法安全回退
			// 直接返回错误而非尝试回退（因为 reader 数据已丢失）
			return nil, fmt.Errorf("disk storage creation failed: %w", err)
		}
		IncrementDiskCacheHits() // 记录磁盘缓存命中
		return storage, nil
	}

	// 使用内存读取
	data, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, ErrRequestBodyTooLarge
	}

	storage, err := CreateBodyStorage(data)
	if err != nil {
		return nil, err
	}
	// 记录缓存命中统计
	if !storage.IsDisk() {
		IncrementMemoryCacheHits() // 内存缓存命中
	} else {
		IncrementDiskCacheHits() // 磁盘缓存命中
	}
	return storage, nil
}

// ReaderOnly 包装 io.Reader 以隐藏 io.Closer 接口
//
// 这个辅助函数用于防止 http.NewRequest 类型断言 io.ReadCloser
// 从而避免意外关闭底层的 BodyStorage
//
// 使用场景：
// - 将 BodyStorage 传递给 http.NewRequest 时，如果不希望请求完成后自动关闭存储
//
// 参数：
//   - r: 原始读取器
//
// 返回值：
//   - io.Reader: 包装后的读取器（只暴露 Read 方法）
func ReaderOnly(r io.Reader) io.Reader {
	return struct{ io.Reader }{r}
}

// CleanupOldCacheFiles 清理旧的缓存文件（用于启动时清理残留）
//
// 调用统一的缓存管理函数，清理超过 5 分钟的旧缓存文件
// 在系统启动时调用，清理上次运行可能遗留的临时文件
func CleanupOldCacheFiles() {
	CleanupOldDiskCacheFiles(5 * time.Minute)
}
