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

type cacheClaim struct {
	token     string
	expiresAt time.Time
}

type HybridCacheConfig[V any] struct {
	Namespace Namespace

	// Redis is used when RedisEnabled returns true (or RedisEnabled is nil) and Redis is not nil.
	Redis        *redis.Client
	RedisCodec   ValueCodec[V]
	RedisEnabled func() bool

	// Memory builds a hot cache used when Redis is disabled. Keys stored in memory are fully namespaced.
	Memory func() *hot.HotCache[string, V]
}

// HybridCache is a small helper that uses Redis when enabled, otherwise falls back to in-memory hot cache.
type HybridCache[V any] struct {
	ns Namespace

	redis        *redis.Client
	redisCodec   ValueCodec[V]
	redisEnabled func() bool

	memOnce sync.Once
	memInit func() *hot.HotCache[string, V]
	mem     *hot.HotCache[string, V]

	claimMu sync.Mutex
	claims  map[string]cacheClaim
}

func NewHybridCache[V any](cfg HybridCacheConfig[V]) *HybridCache[V] {
	return &HybridCache[V]{
		ns:           cfg.Namespace,
		redis:        cfg.Redis,
		redisCodec:   cfg.RedisCodec,
		redisEnabled: cfg.RedisEnabled,
		memInit:      cfg.Memory,
	}
}

func (c *HybridCache[V]) FullKey(key string) string {
	return c.ns.FullKey(key)
}

func (c *HybridCache[V]) redisOn() bool {
	if c.redis == nil || c.redisCodec == nil {
		return false
	}
	if c.redisEnabled == nil {
		return true
	}
	return c.redisEnabled()
}

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

// TryClaim 为一个调用方原子占用缓存键，直到 ttl 到期。
// Redis 使用 SET NX，使不同应用节点共享同一占用状态。
func (c *HybridCache[V]) TryClaim(key, token string, ttl time.Duration) (bool, error) {
	full := c.claimFullKey(key)
	token = strings.TrimSpace(token)
	if full == "" || token == "" || ttl <= 0 {
		return false, nil
	}

	if c.redisOn() {
		ctx, cancel := context.WithTimeout(context.Background(), defaultRedisOpTimeout)
		defer cancel()
		return c.redis.SetNX(ctx, full, token, ttl).Result()
	}

	now := time.Now()
	c.claimMu.Lock()
	defer c.claimMu.Unlock()
	if c.claims == nil {
		c.claims = make(map[string]cacheClaim)
	}
	if existing, ok := c.claims[full]; ok {
		if existing.expiresAt.After(now) {
			return false, nil
		}
		delete(c.claims, full)
	}
	c.claims[full] = cacheClaim{
		token:     token,
		expiresAt: now.Add(ttl),
	}
	return true, nil
}

// ReleaseClaim 仅在 token 仍匹配占用者时释放占用。
// Redis 使用 Lua 比较删除，保证校验和删除的原子性。
func (c *HybridCache[V]) ReleaseClaim(key, token string) (bool, error) {
	full := c.claimFullKey(key)
	token = strings.TrimSpace(token)
	if full == "" || token == "" {
		return false, nil
	}

	if c.redisOn() {
		ctx, cancel := context.WithTimeout(context.Background(), defaultRedisOpTimeout)
		defer cancel()
		result, err := c.redis.Eval(ctx, `
			if redis.call("GET", KEYS[1]) == ARGV[1] then
				return redis.call("DEL", KEYS[1])
			end
			return 0
		`, []string{full}, token).Int()
		if err != nil {
			return false, err
		}
		return result > 0, nil
	}

	c.claimMu.Lock()
	defer c.claimMu.Unlock()
	claim, ok := c.claims[full]
	if !ok {
		return false, nil
	}
	if !claim.expiresAt.After(time.Now()) {
		delete(c.claims, full)
		return false, nil
	}
	if claim.token != token {
		return false, nil
	}
	delete(c.claims, full)
	return true, nil
}

// ClaimMatches 判断当前占用是否仍由指定 token 持有。
func (c *HybridCache[V]) ClaimMatches(key, token string) (bool, error) {
	full := c.claimFullKey(key)
	token = strings.TrimSpace(token)
	if full == "" || token == "" {
		return false, nil
	}

	if c.redisOn() {
		ctx, cancel := context.WithTimeout(context.Background(), defaultRedisOpTimeout)
		defer cancel()
		value, err := c.redis.Get(ctx, full).Result()
		if errors.Is(err, redis.Nil) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return value == token, nil
	}

	c.claimMu.Lock()
	defer c.claimMu.Unlock()
	claim, ok := c.claims[full]
	if !ok {
		return false, nil
	}
	if !claim.expiresAt.After(time.Now()) {
		delete(c.claims, full)
		return false, nil
	}
	return claim.token == token, nil
}

// DeleteClaim 不校验占用者直接删除占用，仅供已校验外层缓存记录的清理路径使用。
func (c *HybridCache[V]) DeleteClaim(key string) error {
	full := c.claimFullKey(key)
	if full == "" {
		return nil
	}

	if c.redisOn() {
		ctx, cancel := context.WithTimeout(context.Background(), defaultRedisOpTimeout)
		defer cancel()
		return c.redis.Del(ctx, full).Err()
	}

	c.claimMu.Lock()
	delete(c.claims, full)
	c.claimMu.Unlock()
	return nil
}

func (c *HybridCache[V]) claimFullKey(key string) string {
	full := c.ns.FullKey(key)
	if full == "" {
		return ""
	}
	return full + ":claim"
}

// Keys returns keys with valid values. In Redis, it returns all matching keys.
func (c *HybridCache[V]) Keys() ([]string, error) {
	if c.redisOn() {
		return c.scanKeys(c.ns.MatchPattern())
	}
	return c.memCache().Keys(), nil
}

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

// DeleteMany accepts either fully namespaced keys or raw keys and deletes them.
// It returns a map keyed by fully namespaced keys.
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

func (c *HybridCache[V]) Capacity() (mainCacheCapacity int, missingCacheCapacity int) {
	if c.redisOn() {
		return 0, 0
	}
	return c.memCache().Capacity()
}

func (c *HybridCache[V]) Algorithm() (mainCacheAlgorithm string, missingCacheAlgorithm string) {
	if c.redisOn() {
		return "redis", ""
	}
	return c.memCache().Algorithm()
}
