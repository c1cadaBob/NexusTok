// Package middleware - rate-limit.go
// 该文件实现了请求频率限制（Rate Limiting）中间件
//
// 限流算法：滑动窗口（基于 Redis List 或内存计数器）
// 支持两种存储后端：
// 1. Redis: 适用于分布式部署，多个实例共享限流状态
// 2. 内存: 适用于单机部署，性能更好
//
// 限流粒度：
// - GlobalWebRateLimit: 全局 Web 请求限流（基于 IP）
// - GlobalAPIRateLimit: 全局 API 请求限流（基于 IP）
// - CriticalRateLimit: 关键操作限流（如登录、注册，基于 IP）
// - DownloadRateLimit: 下载请求限流（基于 IP）
// - UploadRateLimit: 上传请求限流（基于 IP）
// - SearchRateLimit: 搜索请求限流（基于用户 ID）
//
// Redis 限流实现：
// 使用 Redis List 存储请求时间戳，通过 LLEN 检查请求数量，
// 通过 LINDEX 获取最早的时间戳判断是否在时间窗口内
package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/c1cada/NexusTok/common" // 公共工具包
	"github.com/gin-gonic/gin"          // Gin 框架
)

// timeFormat 时间格式化模板
// 用于将时间戳存储为字符串格式（ISO 8601）
var timeFormat = "2006-01-02T15:04:05.000Z"

// inMemoryRateLimiter 内存限流器实例
// 用于单机部署时的请求限流
var inMemoryRateLimiter common.InMemoryRateLimiter

// defNext 默认的空中间件
// 当限流未启用时，直接放行请求
var defNext = func(c *gin.Context) {
	c.Next()
}

// redisRateLimiter Redis 限流实现
// 使用 Redis List 实现滑动窗口限流
//
// 算法：
// 1. 获取 List 长度（当前窗口内的请求数）
// 2. 如果未达到限制，直接记录请求
// 3. 如果已达到限制，检查最早请求的时间戳
// 4. 如果最早请求在时间窗口外，允许请求并更新 List
// 5. 如果最早请求在时间窗口内，拒绝请求
//
// 参数：
//   - c: Gin 上下文
//   - maxRequestNum: 时间窗口内最大请求数
//   - duration: 时间窗口大小（秒）
//   - mark: 限流标识符（用于区分不同类型的限流）
func redisRateLimiter(c *gin.Context, maxRequestNum int, duration int64, mark string) {
	ctx := context.Background()
	rdb := common.RDB
	key := "rateLimit:" + mark + c.ClientIP()
	listLength, err := rdb.LLen(ctx, key).Result()
	if err != nil {
		fmt.Println(err.Error())
		c.Status(http.StatusInternalServerError)
		c.Abort()
		return
	}
	if listLength < int64(maxRequestNum) {
		rdb.LPush(ctx, key, time.Now().Format(timeFormat))
		rdb.Expire(ctx, key, common.RateLimitKeyExpirationDuration)
	} else {
		oldTimeStr, _ := rdb.LIndex(ctx, key, -1).Result()
		oldTime, err := time.Parse(timeFormat, oldTimeStr)
		if err != nil {
			fmt.Println(err)
			c.Status(http.StatusInternalServerError)
			c.Abort()
			return
		}
		nowTimeStr := time.Now().Format(timeFormat)
		nowTime, err := time.Parse(timeFormat, nowTimeStr)
		if err != nil {
			fmt.Println(err)
			c.Status(http.StatusInternalServerError)
			c.Abort()
			return
		}
		// time.Since will return negative number!
		// See: https://stackoverflow.com/questions/50970900/why-is-time-since-returning-negative-durations-on-windows
		if int64(nowTime.Sub(oldTime).Seconds()) < duration {
			rdb.Expire(ctx, key, common.RateLimitKeyExpirationDuration)
			c.Status(http.StatusTooManyRequests)
			c.Abort()
			return
		} else {
			rdb.LPush(ctx, key, time.Now().Format(timeFormat))
			rdb.LTrim(ctx, key, 0, int64(maxRequestNum-1))
			rdb.Expire(ctx, key, common.RateLimitKeyExpirationDuration)
		}
	}
}

// memoryRateLimiter 内存限流实现
// 使用内存计数器实现限流，适用于单机部署
//
// 参数：
//   - c: Gin 上下文
//   - maxRequestNum: 时间窗口内最大请求数
//   - duration: 时间窗口大小（秒）
//   - mark: 限流标识符
func memoryRateLimiter(c *gin.Context, maxRequestNum int, duration int64, mark string) {
	key := mark + c.ClientIP()
	if !inMemoryRateLimiter.Request(key, maxRequestNum, duration) {
		c.Status(http.StatusTooManyRequests)
		c.Abort()
		return
	}
}

// rateLimitFactory 限流器工厂函数
// 根据是否启用 Redis 选择对应的限流实现
//
// 参数：
//   - maxRequestNum: 时间窗口内最大请求数
//   - duration: 时间窗口大小（秒）
//   - mark: 限流标识符
//
// 返回值：
//   - func(c *gin.Context): 限流中间件函数
func rateLimitFactory(maxRequestNum int, duration int64, mark string) func(c *gin.Context) {
	if common.RedisEnabled {
		return func(c *gin.Context) {
			redisRateLimiter(c, maxRequestNum, duration, mark)
		}
	} else {
		// It's safe to call multi times.
		inMemoryRateLimiter.Init(common.RateLimitKeyExpirationDuration)
		return func(c *gin.Context) {
			memoryRateLimiter(c, maxRequestNum, duration, mark)
		}
	}
}

// GlobalWebRateLimit 全局 Web 请求限流中间件
// 对所有 Web 请求进行基于 IP 的频率限制
// 配置项：GLOBAL_WEB_RATE_LIMIT_ENABLE、GLOBAL_WEB_RATE_LIMIT_NUM、GLOBAL_WEB_RATE_LIMIT_DURATION
func GlobalWebRateLimit() func(c *gin.Context) {
	if common.GlobalWebRateLimitEnable {
		return rateLimitFactory(common.GlobalWebRateLimitNum, common.GlobalWebRateLimitDuration, "GW")
	}
	return defNext
}

// GlobalAPIRateLimit 全局 API 请求限流中间件
// 对所有 API 请求进行基于 IP 的频率限制
// 配置项：GLOBAL_API_RATE_LIMIT_ENABLE、GLOBAL_API_RATE_LIMIT_NUM、GLOBAL_API_RATE_LIMIT_DURATION
func GlobalAPIRateLimit() func(c *gin.Context) {
	if common.GlobalApiRateLimitEnable {
		return rateLimitFactory(common.GlobalApiRateLimitNum, common.GlobalApiRateLimitDuration, "GA")
	}
	return defNext
}

// CriticalRateLimit 关键操作限流中间件
// 对敏感操作（如登录、注册、密码重置）进行基于 IP 的频率限制
// 配置项：CRITICAL_RATE_LIMIT_ENABLE、CRITICAL_RATE_LIMIT_NUM、CRITICAL_RATE_LIMIT_DURATION
func CriticalRateLimit() func(c *gin.Context) {
	if common.CriticalRateLimitEnable {
		return rateLimitFactory(common.CriticalRateLimitNum, common.CriticalRateLimitDuration, "CT")
	}
	return defNext
}

// DownloadRateLimit 下载请求限流中间件
// 对文件下载请求进行基于 IP 的频率限制
// 配置项：DOWNLOAD_RATE_LIMIT_NUM、DOWNLOAD_RATE_LIMIT_DURATION
func DownloadRateLimit() func(c *gin.Context) {
	return rateLimitFactory(common.DownloadRateLimitNum, common.DownloadRateLimitDuration, "DW")
}

// UploadRateLimit 上传请求限流中间件
// 对文件上传请求进行基于 IP 的频率限制
// 配置项：UPLOAD_RATE_LIMIT_NUM、UPLOAD_RATE_LIMIT_DURATION
func UploadRateLimit() func(c *gin.Context) {
	return rateLimitFactory(common.UploadRateLimitNum, common.UploadRateLimitDuration, "UP")
}

// userRateLimitFactory creates a rate limiter keyed by authenticated user ID
// instead of client IP, making it resistant to proxy rotation attacks.
// Must be used AFTER authentication middleware (UserAuth).
func userRateLimitFactory(maxRequestNum int, duration int64, mark string) func(c *gin.Context) {
	if common.RedisEnabled {
		return func(c *gin.Context) {
			userId := c.GetInt("id")
			if userId == 0 {
				c.Status(http.StatusUnauthorized)
				c.Abort()
				return
			}
			key := fmt.Sprintf("rateLimit:%s:user:%d", mark, userId)
			userRedisRateLimiter(c, maxRequestNum, duration, key)
		}
	}
	// It's safe to call multi times.
	inMemoryRateLimiter.Init(common.RateLimitKeyExpirationDuration)
	return func(c *gin.Context) {
		userId := c.GetInt("id")
		if userId == 0 {
			c.Status(http.StatusUnauthorized)
			c.Abort()
			return
		}
		key := fmt.Sprintf("%s:user:%d", mark, userId)
		if !inMemoryRateLimiter.Request(key, maxRequestNum, duration) {
			c.Status(http.StatusTooManyRequests)
			c.Abort()
			return
		}
	}
}

// userRedisRateLimiter is like redisRateLimiter but accepts a pre-built key
// (to support user-ID-based keys).
func userRedisRateLimiter(c *gin.Context, maxRequestNum int, duration int64, key string) {
	ctx := context.Background()
	rdb := common.RDB
	listLength, err := rdb.LLen(ctx, key).Result()
	if err != nil {
		fmt.Println(err.Error())
		c.Status(http.StatusInternalServerError)
		c.Abort()
		return
	}
	if listLength < int64(maxRequestNum) {
		rdb.LPush(ctx, key, time.Now().Format(timeFormat))
		rdb.Expire(ctx, key, common.RateLimitKeyExpirationDuration)
	} else {
		oldTimeStr, _ := rdb.LIndex(ctx, key, -1).Result()
		oldTime, err := time.Parse(timeFormat, oldTimeStr)
		if err != nil {
			fmt.Println(err)
			c.Status(http.StatusInternalServerError)
			c.Abort()
			return
		}
		nowTimeStr := time.Now().Format(timeFormat)
		nowTime, err := time.Parse(timeFormat, nowTimeStr)
		if err != nil {
			fmt.Println(err)
			c.Status(http.StatusInternalServerError)
			c.Abort()
			return
		}
		if int64(nowTime.Sub(oldTime).Seconds()) < duration {
			rdb.Expire(ctx, key, common.RateLimitKeyExpirationDuration)
			c.Status(http.StatusTooManyRequests)
			c.Abort()
			return
		} else {
			rdb.LPush(ctx, key, time.Now().Format(timeFormat))
			rdb.LTrim(ctx, key, 0, int64(maxRequestNum-1))
			rdb.Expire(ctx, key, common.RateLimitKeyExpirationDuration)
		}
	}
}

// SearchRateLimit returns a per-user rate limiter for search endpoints.
// Configurable via SEARCH_RATE_LIMIT_ENABLE / SEARCH_RATE_LIMIT / SEARCH_RATE_LIMIT_DURATION.
func SearchRateLimit() func(c *gin.Context) {
	if !common.SearchRateLimitEnable {
		return defNext
	}
	return userRateLimitFactory(common.SearchRateLimitNum, common.SearchRateLimitDuration, "SR")
}
