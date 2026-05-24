// Package middleware - email-verification-rate-limit.go
// 该文件实现了邮箱验证码发送频率限制中间件
//
// 功能说明：
// - 基于客户端 IP 地址限制邮箱验证码的发送频率
// - 支持 Redis 和内存两种限流后端
// - Redis 可用时使用 Redis 进行分布式限流
// - Redis 不可用时回退到内存限流
//
// 限流规则：
// - 30 秒内同一 IP 最多发送 2 次验证码
// - 超出限制后返回 429 Too Many Requests
package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/c1cada/NexusTok/common"

	"github.com/gin-gonic/gin"
)

const (
	// EmailVerificationRateLimitMark 邮箱验证限流标识前缀
	// 用于 Redis 键和内存限流键的命名空间
	EmailVerificationRateLimitMark = "EV"

	// EmailVerificationMaxRequests 时间窗口内最大请求数
	// 30 秒内最多允许 2 次验证码发送请求
	EmailVerificationMaxRequests = 2 // 30秒内最多2次

	// EmailVerificationDuration 限流时间窗口（秒）
	EmailVerificationDuration = 30 // 30秒时间窗口
)

// redisEmailVerificationRateLimiter 基于 Redis 的邮箱验证限流器
// 使用 Redis 的 INCR 和 EXPIRE 命令实现滑动窗口限流
//
// 工作流程：
// 1. 以客户端 IP 为键，使用 INCR 原子递增计数器
// 2. 首次请求时设置键的过期时间（30 秒）
// 3. 计数器超过阈值时返回 429 错误，附带剩余等待时间
// 4. Redis 操作失败时自动回退到内存限流
func redisEmailVerificationRateLimiter(c *gin.Context) {
	ctx := context.Background()
	rdb := common.RDB
	key := "emailVerification:" + EmailVerificationRateLimitMark + ":" + c.ClientIP()

	count, err := rdb.Incr(ctx, key).Result()
	if err != nil {
		// fallback
		memoryEmailVerificationRateLimiter(c)
		return
	}

	// 第一次设置键时设置过期时间
	if count == 1 {
		_ = rdb.Expire(ctx, key, time.Duration(EmailVerificationDuration)*time.Second).Err()
	}

	// 检查是否超出限制
	if count <= int64(EmailVerificationMaxRequests) {
		c.Next()
		return
	}

	// 获取剩余等待时间
	ttl, err := rdb.TTL(ctx, key).Result()
	waitSeconds := int64(EmailVerificationDuration)
	if err == nil && ttl > 0 {
		waitSeconds = int64(ttl.Seconds())
	}

	c.JSON(http.StatusTooManyRequests, gin.H{
		"success": false,
		"message": fmt.Sprintf("发送过于频繁，请等待 %d 秒后再试", waitSeconds),
	})
	c.Abort()
}

// memoryEmailVerificationRateLimiter 基于内存的邮箱验证限流器
// 当 Redis 不可用时作为回退方案使用
// 使用全局内存限流器 inMemoryRateLimiter 进行限流判断
func memoryEmailVerificationRateLimiter(c *gin.Context) {
	key := EmailVerificationRateLimitMark + ":" + c.ClientIP()

	if !inMemoryRateLimiter.Request(key, EmailVerificationMaxRequests, EmailVerificationDuration) {
		c.JSON(http.StatusTooManyRequests, gin.H{
			"success": false,
			"message": "发送过于频繁，请稍后再试",
		})
		c.Abort()
		return
	}

	c.Next()
}

// EmailVerificationRateLimit 邮箱验证限流中间件工厂函数
// 根据 Redis 是否可用自动选择限流后端：
// - Redis 可用：使用 redisEmailVerificationRateLimiter（分布式限流）
// - Redis 不可用：使用 memoryEmailVerificationRateLimiter（单实例限流）
func EmailVerificationRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		if common.RedisEnabled {
			redisEmailVerificationRateLimiter(c)
		} else {
			inMemoryRateLimiter.Init(common.RateLimitKeyExpirationDuration)
			memoryEmailVerificationRateLimiter(c)
		}
	}
}
