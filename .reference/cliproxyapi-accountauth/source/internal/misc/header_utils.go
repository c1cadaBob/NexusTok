// 包 misc - header_utils.go
// 该文件提供了 HTTP 头操作的辅助函数。
// 包括 Gemini CLI User-Agent 生成、代理/指纹头清理、头确保设置等功能。
package misc

import (
	"fmt"
	"net/http"
	"runtime"
	"strings"
)

// GeminiCLIVersion 是上游请求 User-Agent 中报告的 Gemini CLI 版本字符串。
const GeminiCLIVersion = "0.34.0"

// GeminiCLIApiClientHeader 是发送到 Gemini CLI 上游的 X-Goog-Api-Client 头值。
const GeminiCLIApiClientHeader = "google-genai-sdk/1.41.0 gl-node/v22.19.0"

// geminiCLIOS 将 Go 运行时 OS 名称映射为 Gemini CLI 使用的 Node.js 风格平台字符串。
func geminiCLIOS() string {
	switch runtime.GOOS {
	case "windows":
		return "win32"
	default:
		return runtime.GOOS
	}
}

// geminiCLIArch 将 Go 运行时架构名称映射为 Gemini CLI 使用的 Node.js 风格架构字符串。
func geminiCLIArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x64"
	case "386":
		return "x86"
	default:
		return runtime.GOARCH
	}
}

// GeminiCLIUserAgent 返回匹配 Gemini CLI 格式的 User-Agent 字符串。
// model 参数包含在 UA 中；不适用时传入 "" 或 "unknown"。
//
// 参数：
//   - model: 模型名称
//
// 返回：
//   - string: 格式化的 User-Agent 字符串
func GeminiCLIUserAgent(model string) string {
	if model == "" {
		model = "unknown"
	}
	return fmt.Sprintf("GeminiCLI/%s/%s (%s; %s; terminal)", GeminiCLIVersion, model, geminiCLIOS(), geminiCLIArch())
}

// ScrubProxyAndFingerprintHeaders 移除出站请求中可能暴露代理基础设施、客户端身份或浏览器指纹的所有头。
// 确保发送到上游服务的请求看起来像是直接从原生客户端发出，而非来自反向代理后面的第三方客户端。
//
// 参数：
//   - req: 要清理的 HTTP 请求
func ScrubProxyAndFingerprintHeaders(req *http.Request) {
	if req == nil {
		return
	}

	// --- Proxy tracing headers ---
	req.Header.Del("X-Forwarded-For")
	req.Header.Del("X-Forwarded-Host")
	req.Header.Del("X-Forwarded-Proto")
	req.Header.Del("X-Forwarded-Port")
	req.Header.Del("X-Real-IP")
	req.Header.Del("Forwarded")
	req.Header.Del("Via")

	// --- Client identity headers ---
	req.Header.Del("X-Title")
	req.Header.Del("X-Stainless-Lang")
	req.Header.Del("X-Stainless-Package-Version")
	req.Header.Del("X-Stainless-Os")
	req.Header.Del("X-Stainless-Arch")
	req.Header.Del("X-Stainless-Runtime")
	req.Header.Del("X-Stainless-Runtime-Version")
	req.Header.Del("Http-Referer")
	req.Header.Del("Referer")

	// --- Browser / Chromium fingerprint headers ---
	// These are sent by Electron-based clients (e.g. CherryStudio) using the
	// Fetch API, but NOT by Node.js https module (which Antigravity uses).
	req.Header.Del("Sec-Ch-Ua")
	req.Header.Del("Sec-Ch-Ua-Mobile")
	req.Header.Del("Sec-Ch-Ua-Platform")
	req.Header.Del("Sec-Fetch-Mode")
	req.Header.Del("Sec-Fetch-Site")
	req.Header.Del("Sec-Fetch-Dest")
	req.Header.Del("Priority")

	// --- Encoding negotiation ---
	// Antigravity (Node.js) sends "gzip, deflate, br" by default;
	// Electron-based clients may add "zstd" which is a fingerprint mismatch.
	req.Header.Del("Accept-Encoding")
}

// EnsureHeader 确保目标头映射中存在指定的头。
// 按优先级检查多个来源：源头、现有目标头、默认值。
// 仅在头不存在且值非空时设置。
//
// 参数：
//   - target: 要修改的目标头映射
//   - source: 优先检查的源头映射（可为 nil）
//   - key: 头键名
//   - defaultValue: 无其他来源时使用的默认值
func EnsureHeader(target http.Header, source http.Header, key, defaultValue string) {
	if target == nil {
		return
	}
	if source != nil {
		if val := strings.TrimSpace(source.Get(key)); val != "" {
			target.Set(key, val)
			return
		}
	}
	if strings.TrimSpace(target.Get(key)) != "" {
		return
	}
	if val := strings.TrimSpace(defaultValue); val != "" {
		target.Set(key, val)
	}
}
