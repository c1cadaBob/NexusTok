// 包 browser 提供跨平台的浏览器打开功能。
// 该文件封装了底层操作系统的命令，提供统一的接口用于在默认浏览器中打开 URL。
// 支持 macOS、Windows 和 Linux 系统，当首选库不可用时会自动回退到平台特定命令。
package browser

import (
	"fmt"
	"os/exec"
	"runtime"

	log "github.com/sirupsen/logrus"
	"github.com/skratchdot/open-golang/open"
)

// OpenURL 在默认浏览器中打开指定的 URL。
// 首先尝试使用跨平台的 open-golang 库，如果失败则回退到平台特定的命令。
//
// 参数：
//   - url: 要打开的 URL
//
// 返回：
//   - error: 无法打开 URL 时返回的错误，成功时返回 nil
func OpenURL(url string) error {
	fmt.Printf("Attempting to open URL in browser: %s\n", url)

	// 尝试使用 open-golang 库打开
	err := open.Run(url)
	if err == nil {
		log.Debug("Successfully opened URL using open-golang library")
		return nil
	}

	log.Debugf("open-golang failed: %v, trying platform-specific commands", err)

	// 回退到平台特定命令
	return openURLPlatformSpecific(url)
}

// openURLPlatformSpecific 使用操作系统特定命令打开 URL 的辅助函数。
// 作为 OpenURL 的回退机制，在 open-golang 库失败时使用。
//
// 参数：
//   - url: 要打开的 URL
//
// 返回：
//   - error: 无法打开 URL 时返回的错误，成功时返回 nil
func openURLPlatformSpecific(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin": // macOS
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "linux":
		// 按优先级顺序尝试常见的 Linux 浏览器
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
// 验证当前操作系统上是否存在必要的浏览器打开命令。
//
// 返回：
//   - bool: 如果可以打开浏览器返回 true，否则返回 false
func IsAvailable() bool {
	// 首先检查 open-golang 库是否可用
	testErr := open.Run("about:blank")
	if testErr == nil {
		return true
	}

	// 检查平台特定的命令
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

// GetPlatformInfo 返回包含当前平台浏览器打开能力详细信息的映射。
// 包括操作系统、架构和可用命令等信息。
//
// 返回：
//   - map[string]interface{}: 包含平台特定浏览器支持信息的映射
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
