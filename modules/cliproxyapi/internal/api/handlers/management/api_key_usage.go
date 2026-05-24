// management - api_key_usage.go
// API Key 使用统计端点。
// 该模块提供查询所有内存中 API Key 类型认证记录的使用统计数据，
// 按提供者（provider）分组，按 "base_url|api_key" 键索引，
// 包含成功/失败计数和最近请求的时间桶分布。
package management

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

// apiKeyUsageEntry 表示单个 API Key 的使用统计数据。
// 包含累计的成功/失败次数和按时间桶分布的最近请求统计。
type apiKeyUsageEntry struct {
	Success        int64                          `json:"success"`         // 累计成功请求数
	Failed         int64                          `json:"failed"`          // 累计失败请求数
	RecentRequests []coreauth.RecentRequestBucket `json:"recent_requests"` // 按时间桶分布的最近请求统计
}

// mergeRecentRequestBuckets 合并两个最近请求时间桶数组。
// 逐桶累加成功和失败计数。当两个数组长度不同时，只合并公共部分。
func mergeRecentRequestBuckets(dst, src []coreauth.RecentRequestBucket) []coreauth.RecentRequestBucket {
	if len(dst) == 0 {
		return src
	}
	if len(src) == 0 {
		return dst
	}
	if len(dst) != len(src) {
		n := len(dst)
		if len(src) < n {
			n = len(src)
		}
		for i := 0; i < n; i++ {
			dst[i].Success += src[i].Success
			dst[i].Failed += src[i].Failed
		}
		return dst
	}
	for i := range dst {
		dst[i].Success += src[i].Success
		dst[i].Failed += src[i].Failed
	}
	return dst
}

// GetAPIKeyUsage 返回所有内存中 API Key 类型认证记录的使用统计数据。
// 响应按提供者名称分组，每个提供者内按 "base_url|api_key" 复合键索引。
// 每个条目包含累计的成功/失败次数和按时间桶分布的最近请求统计。
// 如果同一复合键有多条认证记录，会自动合并统计数据。
func (h *Handler) GetAPIKeyUsage(c *gin.Context) {
	if h == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "handler not initialized"})
		return
	}

	h.mu.Lock()
	manager := h.authManager
	h.mu.Unlock()
	if manager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "core auth manager unavailable"})
		return
	}

	now := time.Now()
	out := make(map[string]map[string]apiKeyUsageEntry)
	for _, auth := range manager.List() {
		if auth == nil {
			continue
		}
		kind, apiKey := auth.AccountInfo()
		if !strings.EqualFold(strings.TrimSpace(kind), "api_key") {
			continue
		}
		apiKey = strings.TrimSpace(apiKey)
		if apiKey == "" {
			continue
		}
		baseURL := ""
		if auth.Attributes != nil {
			baseURL = strings.TrimSpace(auth.Attributes["base_url"])
			if baseURL == "" {
				baseURL = strings.TrimSpace(auth.Attributes["base-url"])
			}
		}
		compositeKey := baseURL + "|" + apiKey
		provider := strings.ToLower(strings.TrimSpace(auth.Provider))
		if provider == "" {
			provider = "unknown"
		}

		recent := auth.RecentRequestsSnapshot(now)
		providerBucket, ok := out[provider]
		if !ok {
			providerBucket = make(map[string]apiKeyUsageEntry)
			out[provider] = providerBucket
		}
		if existing, exists := providerBucket[compositeKey]; exists {
			existing.Success += auth.Success
			existing.Failed += auth.Failed
			existing.RecentRequests = mergeRecentRequestBuckets(existing.RecentRequests, recent)
			providerBucket[compositeKey] = existing
			continue
		}
		providerBucket[compositeKey] = apiKeyUsageEntry{
			Success:        auth.Success,
			Failed:         auth.Failed,
			RecentRequests: recent,
		}
	}

	c.JSON(http.StatusOK, out)
}
