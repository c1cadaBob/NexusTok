// Package cachex - hybrid_cache.go
// 该文件实现了混合缓存（HybridCache）功能
//
// 核心功能：
// - 本地内存缓存 + Redis 双层缓存
// - 支持缓存命名空间隔离
// - 支持自定义值编解码器
// - 支持缓存过期时间配置
//
// 缓存策略：
// - 读取时优先从本地内存缓存获取
// - 本地缓存未命中时从 Redis 获取
// - 写入时同时更新本地缓存和 Redis
package cachex

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/samber/hot"
)

const (
	defaultRedisOpTimeout   = 2 * time.Second
	defaultRedisScanTimeout = 30 * time.Second
	defaultRedisDelTimeout  = 10 * time.Second
)

// HybridCacheConfig 混合缓存的配置结构体
// 使用泛型参数 V 表示缓存值的类型
type HybridCacheConfig[V any] struct {
	Namespace Namespace // 缓存命名空间，用于键隔离

	// Redis 配置（当 Redis 启用时使用）
	Redis        *redis.Client    // Redis 客户端实例
	RedisCodec   ValueCodec[V]    // Redis 值编解码器
	RedisEnabled func() bool      // 动态检查 Redis 是否启用的回调函数

	// 内存缓存配置（当 Redis 未启用时作为降级方案）
	Memory func() *hot.HotCache[string, V] // 内存缓存工厂函数
}

// HybridCache 实现本地内存缓存 + Redis 的双层缓存策略
// 当 Redis 可用时使用 Redis 作为后端存储；当 Redis 不可用时降级为本地内存缓存
// 泛型参数 V 表示缓存值的类型
type HybridCache[V any] struct {
	ns Namespace // 缓存命名空间

	redis        *redis.Client         // Redis 客户端
	redisCodec   ValueCodec[V]         // Redis 值编解码器
	redisEnabled func() bool           // Redis 启用状态检查回调

	memOnce sync.Once                  // 内存缓存初始化的单次执行保证
	memInit func() *hot.HotCache[string, V] // 内存缓存工厂函数
	mem     *hot.HotCache[string, V]  // 内存缓存实例
}

// NewHybridCache 创建一个新的混合缓存实例
// 根据配置决定使用 Redis 还是本地内存缓存作为后端
func NewHybridCache[V any](cfg HybridCacheConfig[V]) *HybridCache[V] {
	return &HybridCache[V]{
		ns:           cfg.Namespace,
		redis:        cfg.Redis,
		redisCodec:   cfg.RedisCodec,
		redisEnabled: cfg.RedisEnabled,
		memInit:      cfg.Memory,
	}
}

// FullKey 将原始键转换为带命名空间前缀的完整键
func (c *HybridCache[V]) FullKey(key string) string {
	return c.ns.FullKey(key)
}

// redisOn 检查 Redis 是否可用
// 需要同时满足：Redis 客户端非空、编解码器非空、启用回调返回 true（或回调为 nil）
func (c *HybridCache[V]) redisOn() bool {
	if c.redis == nil || c.redisCodec == nil {
		return false
	}
	if c.redisEnabled == nil {
		return true
	}
	return c.redisEnabled()
}

// memCache 获取或初始化内存缓存实例
// 使用 sync.Once 保证只初始化一次
func (c *HybridCache[V]) memCache() *hot.HotCache[string, V] {
	c.memOnce.Do(func() {
		if c.memInit == nil {
			c.mem = hot.NewHotCache[string, V](hot.LRU, 1).Build()
			return
		}
		c.mem = c.memInit()
	})
	return c.mem
}

// Get 从缓存中获取值
// 优先从 Redis 获取；如果 Redis 不可用，从本地内存缓存获取
// 返回值：
//   - value：缓存的值（未找到时为零值）
//   - found：是否找到
//   - err：读取错误
func (c *HybridCache[V]) Get(key string) (value V, found bool, err error) {
	full := c.ns.FullKey(key)
	if full == "" {
		var zero V
		return zero, false, nil
	}

	if c.redisOn() {
		ctx, cancel := context.WithTimeout(context.Background(), defaultRedisOpTimeout)
		defer cancel()

		raw, e := c.redis.Get(ctx, full).Result()
		if e == nil {
			v, decErr := c.redisCodec.Decode(raw)
			if decErr != nil {
				var zero V
				return zero, false, decErr
			}
			return v, true, nil
		}
		if errors.Is(e, redis.Nil) {
			var zero V
			return zero, false, nil
		}
		var zero V
		return zero, false, e
	}

	return c.memCache().Get(full)
}

// SetWithTTL 设置缓存值（带过期时间）
// 如果 Redis 可用，写入 Redis 并设置 TTL；否则写入本地内存缓存
// 参数：
//   - key：缓存键（不含命名空间前缀）
//   - v：要缓存的值
//   - ttl：过期时间
func (c *HybridCache[V]) SetWithTTL(key string, v V, ttl time.Duration) error {
	full := c.ns.FullKey(key)
	if full == "" {
		return nil
	}

	if c.redisOn() {
		raw, err := c.redisCodec.Encode(v)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), defaultRedisOpTimeout)
		defer cancel()
		return c.redis.Set(ctx, full, raw, ttl).Err()
	}

	c.memCache().SetWithTTL(full, v, ttl)
	return nil
}

// Keys 返回所有有效缓存键
// Redis 模式下使用 SCAN 命令遍历匹配的键；内存模式下返回本地缓存的所有键
func (c *HybridCache[V]) Keys() ([]string, error) {
	if c.redisOn() {
		return c.scanKeys(c.ns.MatchPattern())
	}
	return c.memCache().Keys(), nil
}

// scanKeys 使用 Redis SCAN 命令遍历匹配指定模式的所有键
// 使用游标迭代，每次获取 1000 个键，避免阻塞 Redis
func (c *HybridCache[V]) scanKeys(match string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultRedisScanTimeout)
	defer cancel()

	var cursor uint64
	keys := make([]string, 0, 1024)
	for {
		k, next, err := c.redis.Scan(ctx, cursor, match, 1000).Result()
		if err != nil {
			return keys, err
		}
		keys = append(keys, k...)
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return keys, nil
}

// Purge 清除当前命名空间下的所有缓存
// Redis 模式下使用 SCAN + UNLINK 批量删除；内存模式下直接清除本地缓存
func (c *HybridCache[V]) Purge() error {
	if c.redisOn() {
		keys, err := c.scanKeys(c.ns.MatchPattern())
		if err != nil {
			return err
		}
		if len(keys) == 0 {
			return nil
		}
		_, err = c.DeleteMany(keys)
		return err
	}

	c.memCache().Purge()
	return nil
}

// DeleteByPrefix 删除指定前缀下的所有缓存键
// 自动添加命名空间前缀，支持嵌套前缀（如 "user:123"）
// 返回实际删除的键数量
func (c *HybridCache[V]) DeleteByPrefix(prefix string) (int, error) {
	fullPrefix := c.ns.FullKey(prefix)
	if fullPrefix == "" {
		return 0, nil
	}
	if !strings.HasSuffix(fullPrefix, ":") {
		fullPrefix += ":"
	}

	if c.redisOn() {
		match := fullPrefix + "*"
		keys, err := c.scanKeys(match)
		if err != nil {
			return 0, err
		}
		if len(keys) == 0 {
			return 0, nil
		}

		res, err := c.DeleteMany(keys)
		if err != nil {
			return 0, err
		}
		deleted := 0
		for _, ok := range res {
			if ok {
				deleted++
			}
		}
		return deleted, nil
	}

	// In memory, we filter keys and bulk delete.
	allKeys := c.memCache().Keys()
	keys := make([]string, 0, 128)
	for _, k := range allKeys {
		if strings.HasPrefix(k, fullPrefix) {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return 0, nil
	}
	res, _ := c.DeleteMany(keys)
	deleted := 0
	for _, ok := range res {
		if ok {
			deleted++
		}
	}
	return deleted, nil
}

// DeleteMany 批量删除缓存键
// 接受完整命名空间键或原始键，自动转换为完整键
// Redis 模式下使用 Pipeline + UNLINK 非阻塞删除（比 DEL 更高效）
// 返回值：以完整键为 key 的 map，value 表示该键是否被成功删除
func (c *HybridCache[V]) DeleteMany(keys []string) (map[string]bool, error) {
	res := make(map[string]bool, len(keys))
	if len(keys) == 0 {
		return res, nil
	}

	fullKeys := make([]string, 0, len(keys))
	for _, k := range keys {
		k = c.ns.FullKey(k)
		if k == "" {
			continue
		}
		fullKeys = append(fullKeys, k)
	}
	if len(fullKeys) == 0 {
		return res, nil
	}

	if c.redisOn() {
		ctx, cancel := context.WithTimeout(context.Background(), defaultRedisDelTimeout)
		defer cancel()

		pipe := c.redis.Pipeline()
		cmds := make([]*redis.IntCmd, 0, len(fullKeys))
		for _, k := range fullKeys {
			// UNLINK is non-blocking vs DEL for large key batches.
			cmds = append(cmds, pipe.Unlink(ctx, k))
		}
		_, err := pipe.Exec(ctx)
		if err != nil && !errors.Is(err, redis.Nil) {
			return res, err
		}
		for i, cmd := range cmds {
			deleted := cmd != nil && cmd.Err() == nil && cmd.Val() > 0
			res[fullKeys[i]] = deleted
		}
		return res, nil
	}

	return c.memCache().DeleteMany(fullKeys), nil
}

// Capacity 返回缓存的容量配置
// Redis 模式下返回 (0, 0)（容量由 Redis 管理）
// 内存模式下返回本地缓存的主缓存和缺失缓存容量
func (c *HybridCache[V]) Capacity() (mainCacheCapacity int, missingCacheCapacity int) {
	if c.redisOn() {
		return 0, 0
	}
	return c.memCache().Capacity()
}

// Algorithm 返回缓存使用的淘汰算法名称
// Redis 模式下返回 ("redis", "")
// 内存模式下返回本地缓存的算法名称
func (c *HybridCache[V]) Algorithm() (mainCacheAlgorithm string, missingCacheAlgorithm string) {
	if c.redisOn() {
		return "redis", ""
	}
	return c.memCache().Algorithm()
}
