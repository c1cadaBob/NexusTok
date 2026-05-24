// managementasset - local.go
// 本文件提供管理面板静态资源文件的本地路径解析功能。
// 它负责定位管理面板 HTML 文件在文件系统中的存储位置，
// 支持通过环境变量覆盖默认路径，以及根据配置文件路径自动推导资源目录。
package managementasset

import (
	"os"
	"path/filepath"
	"strings"
)

// managementAssetName 是管理面板 HTML 资源文件的文件名常量。
const managementAssetName = "management.html"

// ManagementFileName 对外暴露的管理面板资源文件名。
const ManagementFileName = managementAssetName

// StaticDir 解析存储管理面板静态资源的本地目录路径。
// 解析优先级：
//  1. 如果设置了环境变量 MANAGEMENT_STATIC_PATH，则优先使用该路径；
//     如果该路径指向的恰好是 management.html 文件本身，则返回其所在目录。
//  2. 否则根据配置文件路径 configFilePath 推导：如果 configFilePath 是一个目录，
//     则在该目录下查找 static 子目录；如果是一个文件，则在其父目录下查找 static 子目录。
//
// 参数：
//   - configFilePath: 配置文件或配置目录的路径
//
// 返回值：
//   - string: 管理面板静态资源所在的目录路径，若无法解析则返回空字符串
func StaticDir(configFilePath string) string {
	if override := strings.TrimSpace(os.Getenv("MANAGEMENT_STATIC_PATH")); override != "" {
		cleaned := filepath.Clean(override)
		if strings.EqualFold(filepath.Base(cleaned), managementAssetName) {
			return filepath.Dir(cleaned)
		}
		return cleaned
	}

	configFilePath = strings.TrimSpace(configFilePath)
	if configFilePath == "" {
		return ""
	}

	base := filepath.Dir(configFilePath)
	fileInfo, err := os.Stat(configFilePath)
	if err == nil && fileInfo.IsDir() {
		base = configFilePath
	}

	return filepath.Join(base, "static")
}

// FilePath 解析管理面板资源文件在本地文件系统中的绝对路径。
// 解析优先级：
//  1. 如果设置了环境变量 MANAGEMENT_STATIC_PATH，则优先使用该路径；
//     如果该路径指向的恰好是 management.html 文件本身，则直接返回该路径。
//  2. 否则先通过 StaticDir 获取静态资源目录，再拼接文件名。
//
// 参数：
//   - configFilePath: 配置文件或配置目录的路径
//
// 返回值：
//   - string: 管理面板资源文件的绝对路径，若无法解析则返回空字符串
func FilePath(configFilePath string) string {
	if override := strings.TrimSpace(os.Getenv("MANAGEMENT_STATIC_PATH")); override != "" {
		cleaned := filepath.Clean(override)
		if strings.EqualFold(filepath.Base(cleaned), managementAssetName) {
			return cleaned
		}
		return filepath.Join(cleaned, ManagementFileName)
	}

	dir := StaticDir(configFilePath)
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, ManagementFileName)
}
