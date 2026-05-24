// 包 misc - antigravity_version.go
// 该文件提供了 Antigravity（Gemini CLI）版本管理功能。
// 包括从远程 API 获取最新版本、缓存版本信息、生成 User-Agent 字符串等。
package misc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// antigravityReleasesURL 是 Antigravity 版本发布 API 的 URL。
const antigravityReleasesURL = "https://antigravity-auto-updater-974169037036.us-central1.run.app/releases"

// antigravityFallbackVersion 是无法获取远程版本时的回退版本。
const antigravityFallbackVersion = "1.21.9"

// antigravityVersionCacheTTL 是版本缓存的过期时间（6 小时）。
const antigravityVersionCacheTTL = 6 * time.Hour

// antigravityFetchTimeout 是获取远程版本的 HTTP 请求超时时间。
const antigravityFetchTimeout = 10 * time.Second

// AntigravityNodeAPIClientUA 是 loadCodeAssist 请求中使用的 Node.js API 客户端 User-Agent 标识。
const AntigravityNodeAPIClientUA = "google-api-nodejs-client/10.3.0"

// AntigravityGoogAPIClientUA 是 Google API 客户端的 User-Agent 标识。
const AntigravityGoogAPIClientUA = "gl-node/22.21.1"

// antigravityRelease 表示 Antigravity 版本发布 API 的响应结构。
type antigravityRelease struct {
	// Version 是发布的版本号
	Version string `json:"version"`
	// ExecutionID 是构建执行的唯一标识符
	ExecutionID string `json:"execution_id"`
}

var (
	// cachedAntigravityVersion 是缓存的 Antigravity 最新版本号
	cachedAntigravityVersion = antigravityFallbackVersion
	// antigravityVersionMu 保护版本缓存的并发读写
	antigravityVersionMu sync.RWMutex
	// antigravityVersionExpiry 是缓存过期时间
	antigravityVersionExpiry time.Time
	// antigravityUpdaterOnce 确保版本更新器只启动一次
	antigravityUpdaterOnce sync.Once
)

// StartAntigravityVersionUpdater 启动后台 goroutine 定期刷新缓存的 Antigravity 版本。
// 与请求执行解耦，避免在版本查询时阻塞请求处理。
func StartAntigravityVersionUpdater(ctx context.Context) {
	antigravityUpdaterOnce.Do(func() {
		go runAntigravityVersionUpdater(ctx)
	})
}

// runAntigravityVersionUpdater 是版本更新器的主循环。
// 启动时立即刷新一次，之后按缓存 TTL 的一半间隔定期刷新。
func runAntigravityVersionUpdater(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}

	ticker := time.NewTicker(antigravityVersionCacheTTL / 2)
	defer ticker.Stop()

	log.Infof("periodic antigravity version refresh started (interval=%s)", antigravityVersionCacheTTL/2)

	refreshAntigravityVersion(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			refreshAntigravityVersion(ctx)
		}
	}
}

// refreshAntigravityVersion 从远程 API 获取最新版本并更新缓存。
// 获取失败时保留缓存值或使用回退版本。
func refreshAntigravityVersion(ctx context.Context) {
	version, errFetch := fetchAntigravityLatestVersion(ctx)

	antigravityVersionMu.Lock()
	defer antigravityVersionMu.Unlock()

	now := time.Now()

	if errFetch == nil {
		cachedAntigravityVersion = version
		antigravityVersionExpiry = now.Add(antigravityVersionCacheTTL)
		log.WithField("version", version).Info("fetched latest antigravity version")
		return
	}

	if cachedAntigravityVersion == "" || now.After(antigravityVersionExpiry) {
		cachedAntigravityVersion = antigravityFallbackVersion
		antigravityVersionExpiry = now.Add(antigravityVersionCacheTTL)
		log.WithError(errFetch).Warn("failed to refresh antigravity version, using fallback version")
		return
	}

	log.WithError(errFetch).Debug("failed to refresh antigravity version, keeping cached value")
}

// AntigravityLatestVersion 返回缓存的 Antigravity 最新版本。
// 缓存为空或过期时返回回退版本。
func AntigravityLatestVersion() string {
	antigravityVersionMu.RLock()
	if cachedAntigravityVersion != "" && time.Now().Before(antigravityVersionExpiry) {
		v := cachedAntigravityVersion
		antigravityVersionMu.RUnlock()
		return v
	}
	antigravityVersionMu.RUnlock()

	return antigravityFallbackVersion
}

// AntigravityUserAgent 返回 Antigravity 请求的 User-Agent 字符串。
// 格式：antigravity/{version} darwin/arm64
func AntigravityUserAgent() string {
	return fmt.Sprintf("antigravity/%s darwin/arm64", AntigravityLatestVersion())
}

// antigravityBaseUserAgent 从 User-Agent 字符串中提取 Antigravity 基础部分。
// 移除附加的 Google API 客户端标识，返回纯净的 Antigravity UA。
func antigravityBaseUserAgent(userAgent string) string {
	userAgent = strings.TrimSpace(userAgent)
	if userAgent == "" {
		return AntigravityUserAgent()
	}
	lower := strings.ToLower(userAgent)
	if strings.HasPrefix(lower, "antigravity/") {
		if idx := strings.Index(lower, " google-api-nodejs-client/"); idx >= 0 {
			trimmed := strings.TrimSpace(userAgent[:idx])
			if trimmed != "" {
				return trimmed
			}
		}
	}
	return userAgent
}

// AntigravityRequestUserAgent 返回 generate/stream/model-list 请求使用的简短 Antigravity UA。
func AntigravityRequestUserAgent(userAgent string) string {
	return antigravityBaseUserAgent(userAgent)
}

// AntigravityLoadCodeAssistUserAgent 返回 loadCodeAssist 请求使用的长格式 Antigravity 控制平面 UA。
// 在基础 UA 后附加 Google API Node.js 客户端标识。
func AntigravityLoadCodeAssistUserAgent(userAgent string) string {
	userAgent = strings.TrimSpace(userAgent)
	if userAgent == "" {
		return AntigravityUserAgent() + " " + AntigravityNodeAPIClientUA
	}
	lower := strings.ToLower(userAgent)
	if !strings.HasPrefix(lower, "antigravity/") {
		return userAgent
	}
	if strings.Contains(lower, "google-api-nodejs-client/") {
		return userAgent
	}
	return antigravityBaseUserAgent(userAgent) + " " + AntigravityNodeAPIClientUA
}

// AntigravityVersionFromUserAgent 从 User-Agent 字符串中提取 Antigravity 版本号。
// 支持短格式和长格式 UA。无法提取时返回缓存的最新版本。
func AntigravityVersionFromUserAgent(userAgent string) string {
	base := antigravityBaseUserAgent(userAgent)
	lower := strings.ToLower(base)
	if !strings.HasPrefix(lower, "antigravity/") {
		return AntigravityLatestVersion()
	}
	rest := base[len("antigravity/"):]
	if idx := strings.IndexAny(rest, " \t"); idx >= 0 {
		rest = rest[:idx]
	}
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return AntigravityLatestVersion()
	}
	return rest
}

// fetchAntigravityLatestVersion 从远程 API 获取最新的 Antigravity 版本号。
// 返回列表中第一个发布的版本号。
func fetchAntigravityLatestVersion(ctx context.Context) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	client := &http.Client{Timeout: antigravityFetchTimeout}

	httpReq, errReq := http.NewRequestWithContext(ctx, http.MethodGet, antigravityReleasesURL, nil)
	if errReq != nil {
		return "", fmt.Errorf("build antigravity releases request: %w", errReq)
	}

	resp, errDo := client.Do(httpReq)
	if errDo != nil {
		return "", fmt.Errorf("fetch antigravity releases: %w", errDo)
	}
	defer func() {
		if errClose := resp.Body.Close(); errClose != nil {
			log.WithError(errClose).Warn("antigravity releases response body close error")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("antigravity releases API returned status %d", resp.StatusCode)
	}

	var releases []antigravityRelease
	if errDecode := json.NewDecoder(resp.Body).Decode(&releases); errDecode != nil {
		return "", fmt.Errorf("decode antigravity releases response: %w", errDecode)
	}

	if len(releases) == 0 {
		return "", errors.New("antigravity releases API returned empty list")
	}

	version := releases[0].Version
	if version == "" {
		return "", errors.New("antigravity releases API returned empty version")
	}

	return version, nil
}
