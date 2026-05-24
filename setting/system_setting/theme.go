// theme.go — 前端主题配置管理
// 职责：管理前端界面的主题配置，支持在不同前端主题之间切换。
// 通过 config.GlobalConfig 注册实现持久化存储，并在配置变更时
// 同步到 common 包供全局使用。

package system_setting

import (
	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/setting/config"
)

// ThemeSettings 主题配置结构体
type ThemeSettings struct {
	// Frontend 前端主题名称，如 "classic"、"default"
	Frontend string `json:"frontend"`
}

// themeSettings 是全局主题配置实例，默认使用 classic 主题
var themeSettings = ThemeSettings{
	Frontend: "classic",
}

// init 注册主题配置到全局配置管理系统，并同步到 common 包
func init() {
	config.GlobalConfig.Register("theme", &themeSettings)
	syncThemeToCommon()
}

// syncThemeToCommon 将当前主题配置同步到 common 包
func syncThemeToCommon() {
	common.SetTheme(themeSettings.Frontend)
}

// GetThemeSettings 获取当前主题配置的指针
// 返回值：指向当前主题配置的指针
func GetThemeSettings() *ThemeSettings {
	return &themeSettings
}

// UpdateAndSyncTheme 在数据库加载后同步主题配置到 common 包
// 通常在启动时或配置更新后调用
func UpdateAndSyncTheme() {
	syncThemeToCommon()
}
