// Package browser 提供跨平台的浏览器打开功能，用于在默认浏览器中打开 URL。
// 它抽象了底层操作系统命令，提供了统一的简单接口。
//
// Package browser provides cross-platform functionality for opening URLs in the default web browser.
// It abstracts the underlying operating system commands and provides a simple interface.
package browser

import (
	"fmt"
	"os/exec"
	"runtime"

	log "github.com/sirupsen/logrus"
	"github.com/skratchdot/open-golang/open"
)

// OpenURL 在默认浏览器中打开指定的 URL。
// 首先尝试使用跨平台库 open-golang，如果失败则回退到操作系统特定的命令。
//
// 参数:
//   - url: 要打开的 URL 地址
//
// 返回值:
//   - error: 无法打开 URL 时返回错误，成功时返回 nil
//
// OpenURL opens the specified URL in the default web browser.
func OpenURL(url string) error {
	fmt.Printf("Attempting to open URL in browser: %s\n", url)

	// Try using the open-golang library first
	err := open.Run(url)
	if err == nil {
		log.Debug("Successfully opened URL using open-golang library")
		return nil
	}

	log.Debugf("open-golang failed: %v, trying platform-specific commands", err)

	// Fallback to platform-specific commands
	return openURLPlatformSpecific(url)
}

// openURLPlatformSpecific 是一个辅助函数，使用操作系统特定的命令打开 URL。
// 作为 OpenURL 的回退机制。
//
// 参数:
//   - url: 要打开的 URL 地址
//
// 返回值:
//   - error: 无法打开 URL 时返回错误，成功时返回 nil
//
// openURLPlatformSpecific is a helper function that opens a URL using OS-specific commands.
func openURLPlatformSpecific(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin": // macOS
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "linux":
		// Try common Linux browsers in order of preference
		browsers := []string{"xdg-open", "x-www-browser", "www-browser", "firefox", "chromium", "google-chrome"}
		for _, browser := range browsers {
			if _, err := exec.LookPath(browser); err == nil {
				cmd = exec.Command(browser, url)
				break
			}
		}
		if cmd == nil {
			return fmt.Errorf("no suitable browser found on Linux system")
		}
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}

	log.Debugf("Running command: %s %v", cmd.Path, cmd.Args[1:])
	err := cmd.Start()
	if err != nil {
		return fmt.Errorf("failed to start browser command: %w", err)
	}

	log.Debug("Successfully opened URL using platform-specific command")
	return nil
}

// IsAvailable 检查系统是否有可用的命令来打开浏览器。
// 它会验证当前操作系统上必要的命令是否存在。
//
// 返回值:
//   - true: 如果可以打开浏览器
//   - false: 如果没有可用的浏览器命令
//
// IsAvailable checks if the system has a command available to open a web browser.
func IsAvailable() bool {
	// First check if open-golang can work
	testErr := open.Run("about:blank")
	if testErr == nil {
		return true
	}

	// Check platform-specific commands
	switch runtime.GOOS {
	case "darwin":
		_, err := exec.LookPath("open")
		return err == nil
	case "windows":
		_, err := exec.LookPath("rundll32")
		return err == nil
	case "linux":
		browsers := []string{"xdg-open", "x-www-browser", "www-browser", "firefox", "chromium", "google-chrome"}
		for _, browser := range browsers {
			if _, err := exec.LookPath(browser); err == nil {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// GetPlatformInfo 返回一个 map，包含当前平台的浏览器打开能力详情，
// 包括操作系统、架构和可用命令信息。
//
// 返回值:
//   - map[string]interface{}: 包含平台特定浏览器支持信息的 map
//
// GetPlatformInfo returns a map containing details about the current platform's
// browser opening capabilities.
func GetPlatformInfo() map[string]interface{} {
	info := map[string]interface{}{
		"os":        runtime.GOOS,
		"arch":      runtime.GOARCH,
		"available": IsAvailable(),
	}

	switch runtime.GOOS {
	case "darwin":
		info["default_command"] = "open"
	case "windows":
		info["default_command"] = "rundll32"
	case "linux":
		browsers := []string{"xdg-open", "x-www-browser", "www-browser", "firefox", "chromium", "google-chrome"}
		var availableBrowsers []string
		for _, browser := range browsers {
			if _, err := exec.LookPath(browser); err == nil {
				availableBrowsers = append(availableBrowsers, browser)
			}
		}
		info["available_browsers"] = availableBrowsers
		if len(availableBrowsers) > 0 {
			info["default_command"] = availableBrowsers[0]
		}
	}

	return info
}
