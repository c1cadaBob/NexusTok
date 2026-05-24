// Package types - file_data.go
// 该文件定义了本地文件数据结构
//
// 主要类型：
// - LocalFileData：本地文件数据（MIME 类型、Base64 数据、URL、大小）
//
// 用途：
// - 在请求处理中传递文件数据
// - 支持图片、音频、视频等多种文件类型
package types

// LocalFileData 本地文件数据结构
// 用于在请求处理中传递文件数据
// 支持图片、音频、视频等多种文件类型
type LocalFileData struct {
	MimeType   string // 文件的 MIME 类型（如 image/png、audio/mp3）
	Base64Data string // Base64 编码的文件数据
	Url        string // 文件的 URL 地址（如果是远程文件）
	Size       int64  // 文件大小（字节）
}
