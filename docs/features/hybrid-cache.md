# 混合缓存系统

混合缓存（HybridCache）提供 Redis + 本地内存 LRU 的双层缓存抽象，用于渠道亲和性、Token 缓存、用户缓存等场景。

## 架构概览

```
读取请求
  → 本地内存 LRU 缓存查找
  → 命中？返回
  → 未命中？Redis 查找
  → 命中？写入本地缓存，返回
  → 未命中？返回零值

写入请求
  → 写入本地内存缓存
  → 写入 Redis
  → 两层同步更新
```

## 核心组件

### HybridCache

泛型双层缓存实现：

```go
type HybridCache[V any] struct {
    ns         Namespace                    // 缓存命名空间
    redis      *redis.Client               // Redis 客户端
    redisCodec ValueCodec[V]               // Redis 值编解码器
    redisEnabled func() bool               // Redis 启用状态检查
    mem        *hot.HotCache[string, V]    // 内存缓存实例
}
```

### 缓存配置

```go
type HybridCacheConfig[V any] struct {
    Namespace    Namespace                  // 缓存命名空间
    Redis        *redis.Client              // Redis 客户端
    RedisCodec   ValueCodec[V]              // Redis 编解码器
    RedisEnabled func() bool                // Redis 启用检查
    Memory       func() *hot.HotCache[string, V]  // 内存缓存工厂
}
```

## 命名空间

缓存键通过命名空间隔离，避免不同功能的缓存键冲突：

| 命名空间 | 用途 |
|----------|------|
| `nexustok:channel_affinity:v2` | 渠道亲和性缓存 |
| `nexustok:channel_affinity_usage_cache_stats:v2` | 亲和性使用统计 |
| `nexustok:token_cache` | Token 缓存 |
| `nexustok:user_cache` | 用户缓存 |
| `nexustok:channel_cache` | 渠道缓存 |

## 值编解码器

`ValueCodec` 接口定义了值的序列化和反序列化：

```go
type ValueCodec[V any] interface {
    Encode(v V) ([]byte, error)
    Decode(data []byte) (V, error)
}
```

支持自定义编解码器，用于类型安全的序列化。

## 降级策略

当 Redis 不可用时，自动降级为纯内存缓存：

- `RedisEnabled` 回调返回 `false` 时，跳过 Redis 操作
- 所有读写仅在本地内存缓存中进行
- 适用于单实例部署或 Redis 故障场景

## 超时配置

| 操作 | 默认超时 | 说明 |
|------|----------|------|
| Redis 读写 | 2 秒 | `defaultRedisOpTimeout` |
| Redis 扫描 | 30 秒 | `defaultRedisScanTimeout` |
| Redis 删除 | 10 秒 | `defaultRedisDelTimeout` |

## 使用场景

### 渠道亲和性

存储亲和性绑定关系（亲和值 → 渠道 ID），支持 TTL 过期。

### Token/用户缓存

缓存热点数据，减少数据库查询：
- Token 信息缓存
- 用户信息缓存
- 渠道配置缓存

### 多实例部署

在多实例部署中：
- Redis 作为共享缓存层，确保跨实例一致性
- 本地内存缓存减少 Redis 访问频率
- 缓存失效通过 Redis 发布/订阅同步

## 关键文件

| 文件 | 说明 |
|------|------|
| `pkg/cachex/hybrid_cache.go` | 混合缓存核心实现 |
| `pkg/cachex/namespace.go` | 命名空间管理 |
| `pkg/cachex/codec.go` | 值编解码器接口 |
