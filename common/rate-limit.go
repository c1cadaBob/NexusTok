// Package common - rate-limit.go
// 该文件实现了基于滑动窗口的内存限流器
//
// 限流算法：滑动窗口（Sliding Window）
// - 使用时间戳队列记录每个 Key 的请求时间
// - 窗口大小由 duration 参数控制
// - 窗口内的请求数量由 maxRequestNum 参数控制
//
// 数据结构：
// - store: map[string]*[]int64，Key 为限流键，Value 为时间戳队列
// - 队列按时间排序（旧 → 新）
// - 使用互斥锁保护并发访问
//
// 使用场景：
// - API 请求频率限制
// - 登录尝试限制
// - 搜索请求限制
package common

import (
	"sync"
	"time"
)

// InMemoryRateLimiter 内存限流器
//
// 使用滑动窗口算法实现限流
// 适用于单节点部署或不需要分布式限流的场景
type InMemoryRateLimiter struct {
	store              map[string]*[]int64 // 存储：Key → 时间戳队列
	mutex              sync.Mutex          // 互斥锁，保护并发访问
	expirationDuration time.Duration       // 过期时间（用于清理过期数据）
}

// Init 初始化限流器
//
// 如果限流器未初始化，则进行初始化
// 启动后台清理协程，定期清理过期数据
//
// 参数：
//   - expirationDuration: 过期时间（数据在过期后会被清理）
func (l *InMemoryRateLimiter) Init(expirationDuration time.Duration) {
	if l.store == nil {
		l.mutex.Lock()
		if l.store == nil {
			l.store = make(map[string]*[]int64)
			l.expirationDuration = expirationDuration
			if expirationDuration > 0 {
				go l.clearExpiredItems()
			}
		}
		l.mutex.Unlock()
	}
}

// clearExpiredItems 清理过期数据
//
// 后台协程，每隔 expirationDuration 清理一次过期数据
// 清理条件：队列为空或队列中最新时间戳已过期
func (l *InMemoryRateLimiter) clearExpiredItems() {
	for {
		time.Sleep(l.expirationDuration)
		l.mutex.Lock()
		now := time.Now().Unix()
		for key := range l.store {
			queue := l.store[key]
			size := len(*queue)
			if size == 0 || now-(*queue)[size-1] > int64(l.expirationDuration.Seconds()) {
				delete(l.store, key)
			}
		}
		l.mutex.Unlock()
	}
}

// Request 发起限流请求
//
// 滑动窗口限流逻辑：
// 1. 如果队列未满（len < maxRequestNum），直接放行
// 2. 如果队列已满，检查队首时间戳是否已过期：
//    - 已过期：移除队首，放行新请求
//    - 未过期：拒绝请求
//
// 参数：
//   - key: 限流键（如 IP 地址、用户 ID 等）
//   - maxRequestNum: 窗口内最大请求数
//   - duration: 窗口大小（秒）
//
// 返回值：
//   - bool: 是否允许请求（true=允许，false=限流）
func (l *InMemoryRateLimiter) Request(key string, maxRequestNum int, duration int64) bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	// 队列格式：[old <-- new]
	queue, ok := l.store[key]
	now := time.Now().Unix()
	if ok {
		if len(*queue) < maxRequestNum {
			// 队列未满，直接放行
			*queue = append(*queue, now)
			return true
		} else {
			// 队列已满，检查队首是否已过期
			if now-(*queue)[0] >= duration {
				// 队首已过期，移除队首，放行新请求
				*queue = (*queue)[1:]
				*queue = append(*queue, now)
				return true
			} else {
				// 队首未过期，拒绝请求
				return false
			}
		}
	} else {
		// Key 不存在，创建新队列
		s := make([]int64, 0, maxRequestNum)
		l.store[key] = &s
		*(l.store[key]) = append(*(l.store[key]), now)
	}
	return true
}
