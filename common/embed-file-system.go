// Package common - embed-file-system.go
// 该文件实现了嵌入式文件系统（embed.FS）的 HTTP 静态文件服务
//
// 使用 Go 1.16+ 的 embed 功能将前端构建产物嵌入到二进制文件中
// 支持两种前端主题的运行时切换（default 和 classic）
//
// 参考：https://github.com/gin-contrib/static/issues/19
package common

import (
	"embed"
	"io/fs"
	"net/http"
	"os"

	"github.com/gin-contrib/static"
)

// embedFileSystem 嵌入式文件系统适配器
//
// 实现 static.ServeFileSystem 接口
// 将 embed.FS 包装为 HTTP 文件系统
type embedFileSystem struct {
	http.FileSystem
}

// Exists 检查文件是否存在
//
// 实现 static.ServeFileSystem 接口
// 通过尝试打开文件来判断是否存在
//
// 参数：
//   - prefix: 路径前缀（未使用）
//   - path: 文件路径
//
// 返回值：
//   - bool: 文件是否存在
func (e *embedFileSystem) Exists(prefix string, path string) bool {
	_, err := e.Open(path)
	if err != nil {
		return false
	}
	return true
}

// Open 打开嵌入式文件
//
// 实现 http.FileSystem 接口
// 特殊处理根路径 "/"：返回 ErrNotExist，使请求转到 NoRouter 处理器
// NoRouter 处理器会使用替换的 index.html（包含分析代码等）
//
// 参数：
//   - name: 文件路径
//
// 返回值：
//   - http.File: 文件句柄
//   - error: 打开错误
func (e *embedFileSystem) Open(name string) (http.File, error) {
	if name == "/" {
		// 根路径返回不存在，让 NoRouter 处理器处理
		// 这样可以使用替换的 index.html（包含分析代码等）
		return nil, os.ErrNotExist
	}
	return e.FileSystem.Open(name)
}

// EmbedFolder 将嵌入式文件系统中的子目录包装为静态文件服务
//
// 使用 fs.Sub 获取子目录的文件系统
// 用于将嵌入的前端构建产物提供为静态文件服务
//
// 参数：
//   - fsEmbed: 嵌入式文件系统
//   - targetPath: 子目录路径（如 "web/dist"）
//
// 返回值：
//   - static.ServeFileSystem: 静态文件服务接口
func EmbedFolder(fsEmbed embed.FS, targetPath string) static.ServeFileSystem {
	efs, err := fs.Sub(fsEmbed, targetPath)
	if err != nil {
		panic(err)
	}
	return &embedFileSystem{
		FileSystem: http.FS(efs),
	}
}

// themeAwareFileSystem 主题感知文件系统
//
// 根据当前主题（通过 GetTheme() 获取）委托给对应的嵌入式文件系统
// 实现运行时主题切换，无需重启服务器
type themeAwareFileSystem struct {
	defaultFS static.ServeFileSystem // default 主题的文件系统
	classicFS static.ServeFileSystem // classic 主题的文件系统
}

// Exists 检查文件是否存在（主题感知）
//
// 根据当前主题选择对应的文件系统进行检查
//
// 参数：
//   - prefix: 路径前缀
//   - path: 文件路径
//
// 返回值：
//   - bool: 文件是否存在
func (t *themeAwareFileSystem) Exists(prefix string, path string) bool {
	if GetTheme() == "classic" {
		return t.classicFS.Exists(prefix, path)
	}
	return t.defaultFS.Exists(prefix, path)
}

// Open 打开文件（主题感知）
//
// 根据当前主题选择对应的文件系统打开文件
//
// 参数：
//   - name: 文件路径
//
// 返回值：
//   - http.File: 文件句柄
//   - error: 打开错误
func (t *themeAwareFileSystem) Open(name string) (http.File, error) {
	if GetTheme() == "classic" {
		return t.classicFS.Open(name)
	}
	return t.defaultFS.Open(name)
}

// NewThemeAwareFS 创建主题感知文件系统
//
// 参数：
//   - defaultFS: default 主题的文件系统
//   - classicFS: classic 主题的文件系统
//
// 返回值：
//   - static.ServeFileSystem: 主题感知文件系统
func NewThemeAwareFS(defaultFS, classicFS static.ServeFileSystem) static.ServeFileSystem {
	return &themeAwareFileSystem{defaultFS: defaultFS, classicFS: classicFS}
}
