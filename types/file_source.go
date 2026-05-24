// Package types - file_source.go
// 该文件定义了文件类型和文件源相关类型
//
// 主要类型：
// - FileType：文件类型常量（image、audio、video、file）
//
// 用途：
// - 在多模态请求中标识文件类型
// - 用于文件处理和路由决策
package types

import (
	"fmt"
	"image"
	"os"
	"strings"
	"sync"
)

// FileSource 统一的文件来源抽象接口
// 支持 URL 和 base64 两种来源，提供懒加载和缓存机制
type FileSource interface {
	IsURL() bool
	GetIdentifier() string
	GetRawData() string
	ClearRawData()

	SetCache(data *CachedFileData)
	GetCache() *CachedFileData
	HasCache() bool
	ClearCache()

	IsRegistered() bool
	SetRegistered(registered bool)
	Mu() *sync.Mutex
}

// baseFileSource 共享的缓存/锁/清理注册状态
// 作为 URLSource 和 Base64Source 的嵌入结构体，提供通用的缓存管理功能
type baseFileSource struct {
	cachedData  *CachedFileData // 缓存的文件数据
	cacheLoaded bool            // 缓存是否已加载
	registered  bool            // 是否已注册到清理列表
	mu          sync.Mutex      // 并发锁，保护缓存操作
}

// SetCache 设置缓存的文件数据
//
// 参数：
//   - data: 要缓存的文件数据
func (b *baseFileSource) SetCache(data *CachedFileData) {
	b.cachedData = data
	b.cacheLoaded = true
}

// GetCache 获取缓存的文件数据
//
// 返回值：
//   - *CachedFileData: 缓存的文件数据，可能为 nil
func (b *baseFileSource) GetCache() *CachedFileData {
	return b.cachedData
}

// HasCache 检查是否有可用的缓存数据
//
// 返回值：
//   - bool: 有缓存数据返回 true，否则返回 false
func (b *baseFileSource) HasCache() bool {
	return b.cacheLoaded && b.cachedData != nil
}

// ClearCache 清除缓存数据并释放相关资源
// 对于磁盘缓存，会关闭文件句柄并删除临时文件
func (b *baseFileSource) ClearCache() {
	if b.cachedData != nil {
		b.cachedData.Close()
	}
	b.cachedData = nil
	b.cacheLoaded = false
}

// IsRegistered 检查是否已注册到清理列表
//
// 返回值：
//   - bool: 已注册返回 true，否则返回 false
func (b *baseFileSource) IsRegistered() bool {
	return b.registered
}

// SetRegistered 设置是否已注册到清理列表
//
// 参数：
//   - registered: 注册状态
func (b *baseFileSource) SetRegistered(registered bool) {
	b.registered = registered
}

// Mu 获取互斥锁的指针
// 用于外部代码进行细粒度的并发控制
//
// 返回值：
//   - *sync.Mutex: 互斥锁指针
func (b *baseFileSource) Mu() *sync.Mutex {
	return &b.mu
}

// ---------------------------------------------------------------------------
// URLSource — URL 来源的 FileSource 实现
// ---------------------------------------------------------------------------

// URLSource URL 来源的 FileSource 实现
// 用于处理通过 HTTP/HTTPS URL 引用的文件
type URLSource struct {
	baseFileSource
	URL string // 文件的 URL 地址
}

// IsURL 判断是否为 URL 来源
// 返回 true 表示数据来源为 URL
func (u *URLSource) IsURL() bool { return true }

// GetIdentifier 获取文件标识符（用于日志和调试）
// 超过 100 字符的 URL 会被截断并添加 "..." 后缀
//
// 返回值：
//   - string: 文件标识符
func (u *URLSource) GetIdentifier() string {
	if len(u.URL) > 100 {
		return u.URL[:100] + "..."
	}
	return u.URL
}

// GetRawData 获取原始数据（URL 地址字符串）
//
// 返回值：
//   - string: URL 地址
func (u *URLSource) GetRawData() string { return u.URL }

// ClearRawData 清除原始数据
// 对于 URL 来源，此操作为空操作（URL 不需要清除）
func (u *URLSource) ClearRawData() {}

// ---------------------------------------------------------------------------
// Base64Source — Base64 内联数据来源的 FileSource 实现
// ---------------------------------------------------------------------------

// Base64Source Base64 内联数据来源的 FileSource 实现
// 用于处理通过 Base64 编码内嵌在请求中的文件数据
type Base64Source struct {
	baseFileSource
	Base64Data string // Base64 编码的文件数据
	MimeType   string // 文件的 MIME 类型（如 image/png）
}

// IsURL 判断是否为 URL 来源
// 返回 false 表示数据来源为 Base64 内联数据
func (b *Base64Source) IsURL() bool { return false }

// GetIdentifier 获取文件标识符（用于日志和调试）
// 超过 50 字符的 Base64 数据会被截断并添加 "..." 后缀
//
// 返回值：
//   - string: 文件标识符，格式为 "base64:..."
func (b *Base64Source) GetIdentifier() string {
	if len(b.Base64Data) > 50 {
		return "base64:" + b.Base64Data[:50] + "..."
	}
	return "base64:" + b.Base64Data
}

// GetRawData 获取原始数据（Base64 编码字符串）
//
// 返回值：
//   - string: Base64 编码的文件数据
func (b *Base64Source) GetRawData() string { return b.Base64Data }

// ClearRawData 清除原始数据以释放内存
// 仅当 Base64 数据超过 1024 字节时才清除，避免频繁清除小数据
func (b *Base64Source) ClearRawData() {
	if len(b.Base64Data) > 1024 {
		b.Base64Data = ""
	}
}

// ---------------------------------------------------------------------------
// 构造函数
// ---------------------------------------------------------------------------

// NewURLFileSource 创建 URL 来源的 FileSource
//
// 参数：
//   - url: 文件的 URL 地址
//
// 返回值：
//   - *URLSource: URL 来源对象
func NewURLFileSource(url string) *URLSource {
	return &URLSource{URL: url}
}

// NewBase64FileSource 创建 Base64 来源的 FileSource
//
// 参数：
//   - base64Data: Base64 编码的文件数据
//   - mimeType: 文件的 MIME 类型
//
// 返回值：
//   - *Base64Source: Base64 来源对象
func NewBase64FileSource(base64Data string, mimeType string) *Base64Source {
	return &Base64Source{
		Base64Data: base64Data,
		MimeType:   mimeType,
	}
}

// NewFileSourceFromData 根据数据内容自动判断创建哪种 FileSource
// 如果数据以 "http://" 或 "https://" 开头，则创建 URLSource
// 否则创建 Base64Source
//
// 参数：
//   - data: 文件数据（URL 或 Base64 编码数据）
//   - mimeType: 文件的 MIME 类型（仅 Base64 来源使用）
//
// 返回值：
//   - FileSource: 文件来源对象
func NewFileSourceFromData(data string, mimeType string) FileSource {
	if strings.HasPrefix(data, "http://") || strings.HasPrefix(data, "https://") {
		return NewURLFileSource(data)
	}
	return NewBase64FileSource(data, mimeType)
}

// ---------------------------------------------------------------------------
// CachedFileData — 缓存的文件数据（支持内存和磁盘两种模式）
// ---------------------------------------------------------------------------

// CachedFileData 缓存的文件数据
// 支持内存和磁盘两种缓存模式：
// - 内存模式：适用于小文件，数据存储在 base64Data 字段中
// - 磁盘模式：适用于大文件，数据存储在临时文件中
type CachedFileData struct {
	base64Data  string        // 内存中的 base64 数据（小文件）
	MimeType    string        // MIME 类型
	Size        int64         // 文件大小（字节）
	DiskSize    int64         // 磁盘缓存实际占用大小（字节，通常是 base64 长度）
	ImageConfig *image.Config // 图片配置（如果是图片）
	ImageFormat string        // 图片格式（如 png、jpeg）

	diskPath        string     // 磁盘缓存文件路径（大文件）
	isDisk          bool       // 是否使用磁盘缓存模式
	diskMu          sync.Mutex // 磁盘操作锁（保护磁盘文件的读取和删除）
	diskClosed      bool       // 是否已关闭/清理
	statDecremented bool       // 是否已扣减统计计数

	OnClose func(size int64) // 关闭回调函数，用于通知调用方释放资源
}

// NewMemoryCachedData 创建内存缓存的文件数据
// 适用于小文件，数据直接存储在内存中
//
// 参数：
//   - base64Data: Base64 编码的文件数据
//   - mimeType: 文件的 MIME 类型
//   - size: 文件大小（字节）
//
// 返回值：
//   - *CachedFileData: 内存缓存对象
func NewMemoryCachedData(base64Data string, mimeType string, size int64) *CachedFileData {
	return &CachedFileData{
		base64Data: base64Data,
		MimeType:   mimeType,
		Size:       size,
		isDisk:     false,
	}
}

// NewDiskCachedData 创建磁盘缓存的文件数据
// 适用于大文件，数据存储在磁盘临时文件中
//
// 参数：
//   - diskPath: 磁盘缓存文件路径
//   - mimeType: 文件的 MIME 类型
//   - size: 文件大小（字节）
//
// 返回值：
//   - *CachedFileData: 磁盘缓存对象
func NewDiskCachedData(diskPath string, mimeType string, size int64) *CachedFileData {
	return &CachedFileData{
		diskPath: diskPath,
		MimeType: mimeType,
		Size:     size,
		isDisk:   true,
	}
}

// GetBase64Data 获取 Base64 编码的文件数据
// 内存缓存模式直接返回数据，磁盘缓存模式从文件读取
//
// 返回值：
//   - string: Base64 编码的文件数据
//   - error: 读取错误，磁盘缓存已关闭或读取失败时返回错误
func (c *CachedFileData) GetBase64Data() (string, error) {
	if !c.isDisk {
		return c.base64Data, nil
	}

	c.diskMu.Lock()
	defer c.diskMu.Unlock()

	if c.diskClosed {
		return "", fmt.Errorf("disk cache already closed")
	}

	data, err := os.ReadFile(c.diskPath)
	if err != nil {
		return "", fmt.Errorf("failed to read from disk cache: %w", err)
	}
	return string(data), nil
}

// SetBase64Data 设置 Base64 编码的文件数据
// 仅对内存缓存模式有效，磁盘缓存模式下此操作为空操作
//
// 参数：
//   - data: Base64 编码的文件数据
func (c *CachedFileData) SetBase64Data(data string) {
	if !c.isDisk {
		c.base64Data = data
	}
}

// IsDisk 判断是否使用磁盘缓存模式
//
// 返回值：
//   - bool: 磁盘缓存模式返回 true，内存缓存模式返回 false
func (c *CachedFileData) IsDisk() bool {
	return c.isDisk
}

// Close 关闭缓存并释放资源
// - 内存模式：清空 base64Data 字段
// - 磁盘模式：删除临时文件并调用 OnClose 回调通知调用方
// 可多次调用，重复调用为空操作
//
// 返回值：
//   - error: 关闭错误（如文件删除失败），成功返回 nil
func (c *CachedFileData) Close() error {
	if !c.isDisk {
		c.base64Data = ""
		return nil
	}

	c.diskMu.Lock()
	defer c.diskMu.Unlock()

	if c.diskClosed {
		return nil
	}

	c.diskClosed = true
	if c.diskPath != "" {
		err := os.Remove(c.diskPath)
		if err == nil && !c.statDecremented && c.OnClose != nil {
			c.OnClose(c.DiskSize)
			c.statDecremented = true
		}
		return err
	}
	return nil
}
