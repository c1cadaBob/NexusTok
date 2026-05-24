// 包 auth - session_cache.go
// 该文件实现了基于 TTL 的会话到认证的映射缓存。
// SessionCache 提供自动过期清理机制，用于会话亲和性选择器的会话绑定管理。
package auth

import (
	"sync"
	"time"
)

// sessionEntry 存储带有过期时间的认证绑定。
type sessionEntry struct {
	authID    string    // 绑定的认证 ID
	expiresAt time.Time // 过期时间
}

// SessionCache 提供基于 TTL 的会话到认证映射，支持自动清理过期条目。
type SessionCache struct {
	mu      sync.RWMutex              // 保护 entries 的读写锁
	entries map[string]sessionEntry    // 会话 ID 到认证绑定的映射
	ttl     time.Duration             // 条目的生存时间
	stopCh  chan struct{}              // 停止信号通道
}

// NewSessionCache 创建具有指定 TTL 的缓存。
// 后台 goroutine 定期清理过期条目。
//
// 参数:
//   - ttl: 条目的生存时间（<=0 时默认 30 分钟）
//
// 返回:
//   - *SessionCache: 会话缓存实例
func NewSessionCache(ttl time.Duration) *SessionCache {
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	c := &SessionCache{
		entries: make(map[string]sessionEntry),
		ttl:     ttl,
		stopCh:  make(chan struct{}),
	}
	go c.cleanupLoop()
	return c
}

// Get 检索绑定到会话的认证 ID（如果仍然有效）。
// 访问时不会刷新 TTL。
//
// 参数:
//   - sessionID: 会话标识
//
// 返回:
//   - string: 绑定的认证 ID
//   - bool: 是否找到有效绑定
func (c *SessionCache) Get(sessionID string) (string, bool) {
	if sessionID == "" {
		return "", false
	}
	c.mu.RLock()
	entry, ok := c.entries[sessionID]
	c.mu.RUnlock()
	if !ok {
		return "", false
	}
	if time.Now().After(entry.expiresAt) {
		c.mu.Lock()
		delete(c.entries, sessionID)
		c.mu.Unlock()
		return "", false
	}
	return entry.authID, true
}

// GetAndRefresh 检索绑定到会话的认证 ID 并在命中时刷新 TTL。
// 这会延长活跃会话的绑定生存时间。
//
// 参数:
//   - sessionID: 会话标识
//
// 返回:
//   - string: 绑定的认证 ID
//   - bool: 是否找到有效绑定
func (c *SessionCache) GetAndRefresh(sessionID string) (string, bool) {
	if sessionID == "" {
		return "", false
	}
	now := time.Now()
	c.mu.Lock()
	entry, ok := c.entries[sessionID]
	if !ok {
		c.mu.Unlock()
		return "", false
	}
	if now.After(entry.expiresAt) {
		delete(c.entries, sessionID)
		c.mu.Unlock()
		return "", false
	}
	// Refresh TTL on successful access
	entry.expiresAt = now.Add(c.ttl)
	c.entries[sessionID] = entry
	c.mu.Unlock()
	return entry.authID, true
}

// Set 将会话绑定到认证 ID 并刷新 TTL。
//
// 参数:
//   - sessionID: 会话标识
//   - authID: 认证 ID
func (c *SessionCache) Set(sessionID, authID string) {
	if sessionID == "" || authID == "" {
		return
	}
	c.mu.Lock()
	c.entries[sessionID] = sessionEntry{
		authID:    authID,
		expiresAt: time.Now().Add(c.ttl),
	}
	c.mu.Unlock()
}

// Invalidate 移除特定的会话绑定。
//
// 参数:
//   - sessionID: 要移除的会话标识
func (c *SessionCache) Invalidate(sessionID string) {
	if sessionID == "" {
		return
	}
	c.mu.Lock()
	delete(c.entries, sessionID)
	c.mu.Unlock()
}

// InvalidateAuth 移除绑定到特定认证 ID 的所有会话。
// 当认证变为不可用时使用。
//
// 参数:
//   - authID: 要移除的认证 ID
func (c *SessionCache) InvalidateAuth(authID string) {
	if authID == "" {
		return
	}
	c.mu.Lock()
	for sid, entry := range c.entries {
		if entry.authID == authID {
			delete(c.entries, sid)
		}
	}
	c.mu.Unlock()
}

// Stop 终止后台清理 goroutine。
func (c *SessionCache) Stop() {
	select {
	case <-c.stopCh:
	default:
		close(c.stopCh)
	}
}

// cleanupLoop 是后台清理循环，每隔 TTL/2 清理一次过期条目。
func (c *SessionCache) cleanupLoop() {
	ticker := time.NewTicker(c.ttl / 2)
	defer ticker.Stop()
	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.cleanup()
		}
	}
}

// cleanup 执行一次过期条目的清理。
func (c *SessionCache) cleanup() {
	now := time.Now()
	c.mu.Lock()
	for sid, entry := range c.entries {
		if now.After(entry.expiresAt) {
			delete(c.entries, sid)
		}
	}
	c.mu.Unlock()
}
