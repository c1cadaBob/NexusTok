// Package tui - browser.go
// 提供浏览器打开功能，用于在用户的默认浏览器中打开 URL。
package tui

import (
	"os/exec"
	"runtime"
)

// openBrowser 在用户的默认浏览器中打开指定的 URL。
// 支持 macOS、Linux 和 Windows 操作系统。
//
// 参数:
//   - url: 要打开的 URL 地址
//
// 返回值:
//   - error: 打开失败时返回错误
//
// openBrowser opens the specified URL in the user's default browser.
func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "linux":
		return exec.Command("xdg-open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}
