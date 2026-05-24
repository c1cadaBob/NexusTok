// helps - user_id_cache_test.go
// 用户 ID 缓存的单元测试。
// 测试以下功能：
// - TTL 内缓存命中：相同 API Key 返回相同的 user_id
// - TTL 过期后重新生成
// - 不同 API Key 返回不同的 user_id
// - 缓存命中时续期 TTL
package helps

import (
	"testing"
	"time"
)

// resetUserIDCache 是辅助函数，重置用户 ID 缓存到初始状态
func resetUserIDCache() {
	userIDCacheMu.Lock()
	userIDCache = make(map[string]userIDCacheEntry)
	userIDCacheMu.Unlock()
}

// TestCachedUserID_ReusesWithinTTL 测试 TTL 内缓存命中：相同 API Key 返回相同的 user_id
func TestCachedUserID_ReusesWithinTTL(t *testing.T) {
	resetUserIDCache()

	first := CachedUserID("api-key-1")
	second := CachedUserID("api-key-1")

	if first == "" {
		t.Fatal("expected generated user_id to be non-empty")
	}
	if first != second {
		t.Fatalf("expected cached user_id to be reused, got %q and %q", first, second)
	}
}

// TestCachedUserID_ExpiresAfterTTL 测试 TTL 过期后 user_id 被重新生成
func TestCachedUserID_ExpiresAfterTTL(t *testing.T) {
	resetUserIDCache()

	expiredID := CachedUserID("api-key-expired")
	cacheKey := userIDCacheKey("api-key-expired")
	userIDCacheMu.Lock()
	userIDCache[cacheKey] = userIDCacheEntry{
		value:  expiredID,
		expire: time.Now().Add(-time.Minute),
	}
	userIDCacheMu.Unlock()

	newID := CachedUserID("api-key-expired")
	if newID == expiredID {
		t.Fatalf("expected expired user_id to be replaced, got %q", newID)
	}
	if newID == "" {
		t.Fatal("expected regenerated user_id to be non-empty")
	}
}

// TestCachedUserID_IsScopedByAPIKey 测试不同 API Key 返回不同的 user_id
func TestCachedUserID_IsScopedByAPIKey(t *testing.T) {
	resetUserIDCache()

	first := CachedUserID("api-key-1")
	second := CachedUserID("api-key-2")

	if first == second {
		t.Fatalf("expected different API keys to have different user_ids, got %q", first)
	}
}

// TestCachedUserID_RenewsTTLOnHit 测试缓存命中时续期 TTL
func TestCachedUserID_RenewsTTLOnHit(t *testing.T) {
	resetUserIDCache()

	key := "api-key-renew"
	id := CachedUserID(key)
	cacheKey := userIDCacheKey(key)

	soon := time.Now()
	userIDCacheMu.Lock()
	userIDCache[cacheKey] = userIDCacheEntry{
		value:  id,
		expire: soon.Add(2 * time.Second),
	}
	userIDCacheMu.Unlock()

	if refreshed := CachedUserID(key); refreshed != id {
		t.Fatalf("expected cached user_id to be reused before expiry, got %q", refreshed)
	}

	userIDCacheMu.RLock()
	entry := userIDCache[cacheKey]
	userIDCacheMu.RUnlock()

	if entry.expire.Sub(soon) < 30*time.Minute {
		t.Fatalf("expected TTL to renew, got %v remaining", entry.expire.Sub(soon))
	}
}
