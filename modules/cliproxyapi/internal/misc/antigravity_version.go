// Package misc provides miscellaneous utility functions for the CLI Proxy API server.
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

// antigravityReleasesURL 是 Antigravity 版本发布 API 的地址。
const (
	antigravityReleasesURL     = "https://antigravity-auto-updater-974169037036.us-central1.run.app/releases"
	// antigravityFallbackVersion 是无法获取最新版本时使用的回退版本号。
	antigravityFallbackVersion = "1.21.9"
	// antigravityVersionCacheTTL 是版本缓存的过期时间。
	antigravityVersionCacheTTL = 6 * time.Hour
	// antigravityFetchTimeout 是获取版本信息的 HTTP 请求超时时间。
	antigravityFetchTimeout    = 10 * time.Second
	// AntigravityNodeAPIClientUA 是 Node.js API 客户端的 User-Agent 标识。
	AntigravityNodeAPIClientUA = "google-api-nodejs-client/10.3.0"
	// AntigravityGoogAPIClientUA 是 Google API 客户端的 User-Agent 标识。
	AntigravityGoogAPIClientUA = "gl-node/22.21.1"
)

// antigravityRelease 表示 Antigravity 版本发布 API 返回的单条发布记录。
type antigravityRelease struct {
	Version     string `json:"version"`     // 版本号
	ExecutionID string `json:"execution_id"` // 执行 ID
}

var (
	// cachedAntigravityVersion 缓存的 Antigravity 最新版本号。
	cachedAntigravityVersion = antigravityFallbackVersion
	// antigravityVersionMu 保护版本缓存的读写锁。
	antigravityVersionMu     sync.RWMutex
	// antigravityVersionExpiry 版本缓存的过期时间点。
	antigravityVersionExpiry time.Time
	// antigravityUpdaterOnce 确保版本更新器只启动一次。
	antigravityUpdaterOnce   sync.Once
)

// StartAntigravityVersionUpdater starts a background goroutine that periodically refreshes the cached antigravity version.
// This is intentionally decoupled from request execution to avoid blocking executors on version lookups.
func StartAntigravityVersionUpdater(ctx context.Context) {
	antigravityUpdaterOnce.Do(func() {
		go runAntigravityVersionUpdater(ctx)
	})
}

// runAntigravityVersionUpdater 是版本更新器的后台循环，按缓存 TTL 的一半间隔定期刷新版本信息。
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

// refreshAntigravityVersion 从远程 API 获取最新版本并更新缓存，
// 获取失败时若缓存已过期则使用回退版本。
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

// AntigravityLatestVersion returns the cached antigravity version refreshed by StartAntigravityVersionUpdater.
// It falls back to antigravityFallbackVersion if the cache is empty or stale.
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

// AntigravityUserAgent returns the User-Agent string for antigravity requests
// using the latest version fetched from the releases API.
func AntigravityUserAgent() string {
	return fmt.Sprintf("antigravity/%s darwin/arm64", AntigravityLatestVersion())
}

// antigravityBaseUserAgent 从 User-Agent 字符串中提取 Antigravity 基础部分，
// 去除附加的 google-api-nodejs-client 等后缀。
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

// AntigravityRequestUserAgent returns the short Antigravity runtime UA used by
// generate/stream/model-list requests.
func AntigravityRequestUserAgent(userAgent string) string {
	return antigravityBaseUserAgent(userAgent)
}

// AntigravityLoadCodeAssistUserAgent returns the long Antigravity control-plane
// UA used by loadCodeAssist requests.
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

// AntigravityVersionFromUserAgent extracts the Antigravity version prefix from
// either the short or long Antigravity UA forms.
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

// fetchAntigravityLatestVersion 从 Antigravity 发布 API 获取最新版本号，
// 返回第一个发布记录的版本字符串。
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
