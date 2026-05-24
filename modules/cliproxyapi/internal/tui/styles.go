// Package tui 提供基于终端的管理界面。
// 本文件定义了 TUI 的样式常量和颜色配置。
//
// Package tui provides a terminal-based management interface for CLIProxyAPI.
package tui

import "github.com/charmbracelet/lipgloss"

// 颜色调色板定义
// Color palette
var (
	colorPrimary   = lipgloss.Color("#7C3AED") // 紫罗兰色 - violet
	colorSecondary = lipgloss.Color("#6366F1") // 靛蓝色 - indigo
	colorSuccess   = lipgloss.Color("#22C55E") // 绿色 - green
	colorWarning   = lipgloss.Color("#EAB308") // 黄色 - yellow
	colorError     = lipgloss.Color("#EF4444") // 红色 - red
	colorInfo      = lipgloss.Color("#3B82F6") // 蓝色 - blue
	colorMuted     = lipgloss.Color("#6B7280") // 灰色 - gray
	colorBg        = lipgloss.Color("#1E1E2E") // 深色背景 - dark bg
	colorSurface   = lipgloss.Color("#313244") // 稍浅表面色 - slightly lighter
	colorText      = lipgloss.Color("#CDD6F4") // 浅色文本 - light text
	colorSubtext   = lipgloss.Color("#A6ADC8") // 暗色文本 - dimmer text
	colorBorder    = lipgloss.Color("#45475A") // 边框色 - border
	colorHighlight = lipgloss.Color("#F5C2E7") // 粉色高亮 - pink highlight
)

// 标签栏样式定义
// Tab bar styles
var (
	tabActiveStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(colorPrimary).
			Padding(0, 2)

	tabInactiveStyle = lipgloss.NewStyle().
				Foreground(colorSubtext).
				Background(colorSurface).
				Padding(0, 2)

	tabBarStyle = lipgloss.NewStyle().
			Background(colorSurface).
			PaddingLeft(1).
			PaddingBottom(0)
)

// Content styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorHighlight).
			MarginBottom(1)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(colorSubtext).
			Italic(true)

	labelStyle = lipgloss.NewStyle().
			Foreground(colorInfo).
			Bold(true).
			Width(24)

	valueStyle = lipgloss.NewStyle().
			Foreground(colorText)

	sectionStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(1, 2)

	errorStyle = lipgloss.NewStyle().
			Foreground(colorError).
			Bold(true)

	successStyle = lipgloss.NewStyle().
			Foreground(colorSuccess)

	warningStyle = lipgloss.NewStyle().
			Foreground(colorWarning)

	statusBarStyle = lipgloss.NewStyle().
			Foreground(colorSubtext).
			Background(colorSurface).
			PaddingLeft(1).
			PaddingRight(1)

	helpStyle = lipgloss.NewStyle().
			Foreground(colorMuted)
)

// Log level styles
var (
	logDebugStyle = lipgloss.NewStyle().Foreground(colorMuted)
	logInfoStyle  = lipgloss.NewStyle().Foreground(colorInfo)
	logWarnStyle  = lipgloss.NewStyle().Foreground(colorWarning)
	logErrorStyle = lipgloss.NewStyle().Foreground(colorError)
)

// Table styles
var (
	tableHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorHighlight).
				BorderBottom(true).
				BorderStyle(lipgloss.NormalBorder()).
				BorderForeground(colorBorder)

	tableCellStyle = lipgloss.NewStyle().
			Foreground(colorText).
			PaddingRight(2)

	tableSelectedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(colorPrimary).
				Bold(true)
)

func logLevelStyle(level string) lipgloss.Style {
	switch level {
	case "debug":
		return logDebugStyle
	case "info":
		return logInfoStyle
	case "warn", "warning":
		return logWarnStyle
	case "error", "fatal", "panic":
		return logErrorStyle
	default:
		return logInfoStyle
	}
}
