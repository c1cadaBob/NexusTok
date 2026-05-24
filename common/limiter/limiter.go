// Package limiter - limiter.go
// 该文件实现了基于 Redis 的分布式令牌桶限流器
//
// 设计目标：
// - 使用 Redis Lua 脚本实现原子性的令牌桶算法
// - 支持分布式环境下的请求限流
// - 使用单例模式确保全局唯一实例
// - 支持通过选项模式灵活配置限流参数
//
// 令牌桶算法原理：
// - 桶以固定速率（Rate）填充令牌
// - 桶有最大容量（Capacity），超过容量的令牌会被丢弃
// - 每次请求消耗指定数量（Requested）的令牌
// - 如果桶中令牌不足，请求被拒绝
package limiter

import (
	"context"
	_ "embed"
	"fmt"
	"sync"

	"github.com/c1cada/NexusTok/common"
	"github.com/go-redis/redis/v8"
)

//go:embed lua/rate_limit.lua
var rateLimitScript string // 嵌入的 Lua 限流脚本

// RedisLimiter 基于 Redis 的分布式限流器
//
// 使用 Lua 脚本确保令牌桶操作的原子性
// 使用单例模式，通过 New 函数初始化
type RedisLimiter struct {
	client         *redis.Client // Redis 客户端实例
	limitScriptSHA string        // Lua 脚本的 SHA1 哈希值，用于 EVALSHA 调用
}

var (
	instance *RedisLimiter // 全局唯一实例
	once     sync.Once     // 确保只初始化一次
)

// New 创建或获取 RedisLimiter 单例实例
//
// 初始化流程：
// 1. 使用 sync.Once 确保只执行一次初始化
// 2. 将 Lua 限流脚本预加载到 Redis 服务器
// 3. 保存脚本的 SHA1 哈希值用于后续调用
//
// 参数：
//   - ctx: 上下文，用于控制 Redis 操作的超时和取消
//   - r: Redis 客户端实例
//
// 返回值：
//   - *RedisLimiter: 限流器实例
func New(ctx context.Context, r *redis.Client) *RedisLimiter {
	once.Do(func() {
		// 预加载 Lua 脚本到 Redis，获取 SHA1 哈希值
		limitSHA, err := r.ScriptLoad(ctx, rateLimitScript).Result()
		if err != nil {
			common.SysLog(fmt.Sprintf("Failed to load rate limit script: %v", err))
		}
		instance = &RedisLimiter{
			client:         r,
			limitScriptSHA: limitSHA,
		}
	})

	return instance
}

// Allow 判断请求是否被允许通过限流器
//
// 令牌桶算法实现：
// 1. 获取当前桶中的令牌数
// 2. 计算自上次请求以来应添加的令牌数
// 3. 如果桶中令牌数 >= 请求数，扣除令牌并返回 true
// 4. 否则返回 false（请求被限流）
//
// 参数：
//   - ctx: 上下文
//   - key: 限流键（通常包含用户 ID、IP 等标识）
//   - opts: 可选的配置选项（容量、速率、请求数）
//
// 返回值：
//   - bool: 请求是否被允许（true=允许，false=被限流）
//   - error: 执行错误
func (rl *RedisLimiter) Allow(ctx context.Context, key string, opts ...Option) (bool, error) {
	// 默认配置：容量 10 个令牌，每秒填充 1 个，每次请求消耗 1 个
	config := &Config{
		Capacity:  10,
		Rate:      1,
		Requested: 1,
	}

	// 应用选项模式，覆盖默认配置
	for _, opt := range opts {
		opt(config)
	}

	// 使用 EVALSHA 执行 Lua 脚本，确保原子性
	result, err := rl.client.EvalSha(
		ctx,
		rl.limitScriptSHA,
		[]string{key},
		config.Requested,
		config.Rate,
		config.Capacity,
	).Int()

	if err != nil {
		return false, fmt.Errorf("rate limit failed: %w", err)
	}
	return result == 1, nil
}

// Config 令牌桶配置结构体
//
// 使用选项模式（Functional Options Pattern）进行配置
type Config struct {
	Capacity  int64 // 桶容量（最大令牌数）
	Rate      int64 // 令牌填充速率（每秒添加的令牌数）
	Requested int64 // 本次请求需要消耗的令牌数
}

// Option 配置选项函数类型
// 用于实现函数式选项模式
type Option func(*Config)

// WithCapacity 设置桶容量
//
// 参数：
//   - c: 桶容量（最大令牌数）
//
// 返回值：
//   - Option: 配置选项函数
func WithCapacity(c int64) Option {
	return func(cfg *Config) { cfg.Capacity = c }
}

// WithRate 设置令牌填充速率
//
// 参数：
//   - r: 每秒添加的令牌数
//
// 返回值：
//   - Option: 配置选项函数
func WithRate(r int64) Option {
	return func(cfg *Config) { cfg.Rate = r }
}

// WithRequested 设置本次请求消耗的令牌数
//
// 参数：
//   - n: 请求数量
//
// 返回值：
//   - Option: 配置选项函数
func WithRequested(n int64) Option {
	return func(cfg *Config) { cfg.Requested = n }
}
