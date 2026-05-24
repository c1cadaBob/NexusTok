// auth - session_cache.go
// 该文件实现了基于 TTL 的会话到认证凭据的映射缓存，支持自动过期清理。
// 用于在会话亲和性选择中快速查找之前绑定的认证凭据，避免重复选择。
package auth

import (
	"sync"
	"time"
)

// sessionEntry 存储单个会话绑定的认证 ID 和过期时间。
type sessionEntry struct {
	authID    string    // 绑定的认证凭据 ID
	expiresAt time.Time // 绑定过期时间
}

// SessionCache 提供基于 TTL 的会话到认证凭据映射，支持自动清理过期条目。
type SessionCache struct {
	mu      sync.RWMutex              // 保护 entries 的读写锁
	entries map[string]sessionEntry    // 会话 ID 到认证条目的映射
	ttl     time.Duration             // 条目生存时间
	stopCh  chan struct{}              // 停止后台清理协程的信号通道
}

// NewSessionCache 创建指定 TTL 的会话缓存，并启动后台清理协程。
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

// Get 获取会话绑定的认证 ID，不刷新 TTL。
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

// GetAndRefresh 获取会话绑定的认证 ID 并刷新 TTL，用于活跃会话延长绑定生命周期。
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

// Set 绑定会话到指定认证 ID 并刷新 TTL。
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

// Invalidate 移除指定会话的绑定。
func (c *SessionCache) Invalidate(sessionID string) {
	if sessionID == "" {
		return
	}
	c.mu.Lock()
	delete(c.entries, sessionID)
	c.mu.Unlock()
}

// InvalidateAuth 移除所有绑定到指定认证 ID 的会话，用于认证凭据不可用时清理。
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

// Stop 终止后台清理协程。
func (c *SessionCache) Stop() {
	select {
	case <-c.stopCh:
	default:
		close(c.stopCh)
	}
}

// cleanupLoop 是后台清理协程的主循环，按 TTL 的一半间隔定期清理过期条目。
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

// cleanup 执行一次过期条目清理，遍历所有条目并删除已过期的。
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
