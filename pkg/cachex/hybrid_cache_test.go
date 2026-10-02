package cachex

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/samber/hot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newClaimTestRedisCache(t *testing.T, client *redis.Client) *HybridCache[string] {
	t.Helper()
	return NewHybridCache[string](HybridCacheConfig[string]{
		Namespace:  Namespace("claim-test"),
		Redis:      client,
		RedisCodec: StringCodec{},
	})
}

func newClaimTestMemoryCache() *HybridCache[string] {
	return NewHybridCache[string](HybridCacheConfig[string]{
		Namespace: Namespace("claim-test"),
		Memory: func() *hot.HotCache[string, string] {
			return hot.NewHotCache[string, string](hot.LRU, 32).WithTTL(time.Minute).Build()
		},
	})
}

func TestHybridCacheClaimsCoordinateThroughRedis(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		require.NoError(t, client.Close())
	})

	cacheA := newClaimTestRedisCache(t, client)
	cacheB := newClaimTestRedisCache(t, client)

	claimed, err := cacheA.TryClaim("capture-1", "token-a", time.Minute)
	require.NoError(t, err)
	assert.True(t, claimed)

	claimed, err = cacheB.TryClaim("capture-1", "token-b", time.Minute)
	require.NoError(t, err)
	assert.False(t, claimed)

	released, err := cacheB.ReleaseClaim("capture-1", "token-b")
	require.NoError(t, err)
	assert.False(t, released)

	matches, err := cacheA.ClaimMatches("capture-1", "token-a")
	require.NoError(t, err)
	assert.True(t, matches)

	released, err = cacheA.ReleaseClaim("capture-1", "token-a")
	require.NoError(t, err)
	assert.True(t, released)

	claimed, err = cacheB.TryClaim("capture-1", "token-b", time.Minute)
	require.NoError(t, err)
	assert.True(t, claimed)
}

func TestHybridCacheClaimsCoordinateInMemory(t *testing.T) {
	cache := newClaimTestMemoryCache()

	claimed, err := cache.TryClaim("capture-2", "token-a", time.Minute)
	require.NoError(t, err)
	assert.True(t, claimed)

	claimed, err = cache.TryClaim("capture-2", "token-b", time.Minute)
	require.NoError(t, err)
	assert.False(t, claimed)

	released, err := cache.ReleaseClaim("capture-2", "token-b")
	require.NoError(t, err)
	assert.False(t, released)

	released, err = cache.ReleaseClaim("capture-2", "token-a")
	require.NoError(t, err)
	assert.True(t, released)

	claimed, err = cache.TryClaim("capture-2", "token-b", time.Minute)
	require.NoError(t, err)
	assert.True(t, claimed)
}
