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
	"strings"
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
		// 这里不能直接使用 time.Since。某些 Windows 环境下单调时钟信息丢失或不一致时，
		// time.Since 可能返回负数，进而误判滑动窗口是否过期；显式解析格式化后的时间可以
		// 保持 Redis 存储值与当前时间的比较口径一致。
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
		// Init 内部会保证重复调用的安全性；这里在构造中间件时初始化一次，
		// 避免每个请求都重复创建清理协程，同时保持 Redis 不可用时的本地兜底能力。
		inMemoryRateLimiter.Init(common.RateLimitKeyExpirationDuration)
		return func(c *gin.Context) {
			memoryRateLimiter(c, maxRequestNum, duration, mark)
		}
	}
}

// isAccountPoolEmbeddedWebRateLimitExemptPath 判断当前 Web 路径是否属于账号池嵌入模块的受保护代理。
//
// CPAMC 嵌入 NexusTok 后会保留原模块的轮询行为：请求监控页会周期性访问
// /usage-service、/status、/v0/management/usage、/v0/management/api-key-aliases 等同源路径，
// 账号巡检页也会在一次批量巡检中连续访问管理代理。上述路由后续仍会经过
// accountPoolManagerSessionAuth 做管理员会话校验，并由 NexusTok 服务端注入内部管理密钥；
// 如果继续计入面向普通网页访问的 IP 级 GlobalWebRateLimit，管理员页面停留几分钟后
// 会把自己限流成 429，导致账号巡检和请求监控都无法稳定使用。
//
// 这里只豁免账号池嵌入模块的页面资源和 Usage Service 同源代理，不影响普通前端页面、
// 登录、注册、静态资源、Relay API 等路径的全局限流。
func isAccountPoolEmbeddedWebRateLimitExemptPath(path string) bool {
	normalizedPath := strings.TrimSpace(path)
	if normalizedPath == "" {
		return false
	}

	if strings.HasPrefix(normalizedPath, "/account-pool/manager") ||
		strings.HasPrefix(normalizedPath, "/usage-service") {
		return true
	}

	if normalizedPath == "/status" {
		return true
	}

	for _, prefix := range []string{
		"/v0/management/usage",
		"/v0/management/model-prices",
		"/v0/management/api-key-aliases",
	} {
		if normalizedPath == prefix || strings.HasPrefix(normalizedPath, prefix+"/") {
			return true
		}
	}

	return false
}

// isAccountPoolEmbeddedAPIRateLimitExemptPath 判断当前 API 路径是否属于账号池管理代理。
//
// /api/account-pool/management/* 是浏览器访问 CLIProxyAPI management API 的唯一受保护入口。
// 账号巡检会针对每个账号调用 /api-call，账号导入、禁用、删除也会经过该代理；这些请求都
// 已经由 middleware.AdminAuth 校验管理员权限，真正的 CLIProxyAPI management key 只在服务端
// 转发时注入。将它们从 IP 级 GlobalAPIRateLimit 中排除，可以避免一次正常批量巡检因为请求
// 数较多而被主项目误判为滥用，同时仍保留路由自身的管理员权限边界。
func isAccountPoolEmbeddedAPIRateLimitExemptPath(path string) bool {
	normalizedPath := strings.TrimSpace(path)
	return normalizedPath == "/api/account-pool/management" ||
		strings.HasPrefix(normalizedPath, "/api/account-pool/management/")
}

// GlobalWebRateLimit 全局 Web 请求限流中间件
// 对所有 Web 请求进行基于 IP 的频率限制
// 配置项：GLOBAL_WEB_RATE_LIMIT_ENABLE、GLOBAL_WEB_RATE_LIMIT_NUM、GLOBAL_WEB_RATE_LIMIT_DURATION
func GlobalWebRateLimit() func(c *gin.Context) {
	if common.GlobalWebRateLimitEnable {
		limiter := rateLimitFactory(common.GlobalWebRateLimitNum, common.GlobalWebRateLimitDuration, "GW")
		return func(c *gin.Context) {
			if isAccountPoolEmbeddedWebRateLimitExemptPath(c.Request.URL.Path) {
				c.Next()
				return
			}
			limiter(c)
		}
	}
	return defNext
}

// GlobalAPIRateLimit 全局 API 请求限流中间件
// 对所有 API 请求进行基于 IP 的频率限制
// 配置项：GLOBAL_API_RATE_LIMIT_ENABLE、GLOBAL_API_RATE_LIMIT_NUM、GLOBAL_API_RATE_LIMIT_DURATION
func GlobalAPIRateLimit() func(c *gin.Context) {
	if common.GlobalApiRateLimitEnable {
		limiter := rateLimitFactory(common.GlobalApiRateLimitNum, common.GlobalApiRateLimitDuration, "GA")
		return func(c *gin.Context) {
			if isAccountPoolEmbeddedAPIRateLimitExemptPath(c.Request.URL.Path) {
				c.Next()
				return
			}
			limiter(c)
		}
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

// userRateLimitFactory 创建按已认证用户 ID 计数的限流器。
//
// 与基于 ClientIP 的全局限流不同，该限流器把 key 绑定到登录后的用户 ID，
// 可以避免用户通过切换代理 IP 绕过搜索等用户级限制。调用方必须把它放在
// UserAuth 等认证中间件之后，否则上下文中还没有用户 ID，函数会返回 401。
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
	// Init 内部会保证重复调用的安全性；这里在构造按用户限流中间件时初始化一次，
	// 让内存模式与 Redis 模式具备相同的过期清理语义。
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

// userRedisRateLimiter 是 Redis 版本的用户级限流实现。
//
// 它与 redisRateLimiter 使用同样的滑动窗口算法，但接收调用方预先构造好的 key，
// 这样可以把 key 绑定到 user:{id} 而不是 ClientIP，满足按用户维度限流的需求。
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
