// Package middleware - rate-limit.go
// 该文件实现了请求频率限制（Rate Limiting）中间件
//
// 限流算法：Redis 固定窗口 Lua 计数器或内存计数器
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
// 使用 Lua 脚本在单次 Redis 调用中完成计数、过期和限流判断，减少高并发下的网络往返。
package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common" // 公共工具包
	"github.com/gin-gonic/gin"          // Gin 框架
	"github.com/go-redis/redis/v8"      // Redis 脚本执行
)

// timeFormat 是 Redis List 型限流仍在复用的时间格式。
// 全局入口限流已切换为 Lua 固定窗口，模型成功请求限流仍使用该格式保存滑动窗口时间戳。
const timeFormat = "2006-01-02T15:04:05.000Z"

// redisFixedWindowRateLimitScript 使用单条 Lua 脚本完成 INCR + EXPIRE + 判断。
//
// 旧实现每次 Redis 限流需要 LLEN/LPUSH/EXPIRE/LINDEX/LTRIM 等多次往返；在多节点高并发
// 部署中，限流本身会放大 Redis RTT。固定窗口脚本牺牲一点窗口边界平滑度，换取原子性和
// 单次 Redis 调用，更适合全局入口限流这种“保护服务容量”的场景。
var redisFixedWindowRateLimitScript = redis.NewScript(`
local current = redis.call("INCR", KEYS[1])
if current == 1 then
  redis.call("EXPIRE", KEYS[1], tonumber(ARGV[2]))
end
if current > tonumber(ARGV[1]) then
  return 0
end
return 1
`)

// inMemoryRateLimiter 内存限流器实例
// 用于单机部署时的请求限流
var inMemoryRateLimiter common.InMemoryRateLimiter

// defNext 默认的空中间件
// 当限流未启用时，直接放行请求
var defNext = func(c *gin.Context) {
	c.Next()
}

var staticAssetWebRateLimitExemptPaths = map[string]struct{}{
	"/favicon.ico":          {},
	"/logo.png":             {},
	"/robots.txt":           {},
	"/pay-apple.png":        {},
	"/pay-card.png":         {},
	"/pay-google.png":       {},
	"/azure_model_name.png": {},
	"/cover-4.webp":         {},
	"/ratio.png":            {},
}

var staticAssetWebRateLimitExemptExts = []string{
	".js",
	".css",
	".map",
	".woff",
	".woff2",
	".ttf",
	".png",
	".jpg",
	".jpeg",
	".webp",
	".svg",
	".ico",
	".txt",
}

var apiRateLimitExemptPaths = map[string]struct{}{
	"/api/channel/upstream-account/capture-helper.user.js": {},
}

var apiRateLimitExemptPrefixes = []string{
	"/api/channel/upstream-account/capture-session/",
}

var apiRateLimitExemptSuffixes = []string{
	"/userscript.user.js",
}

// writeRateLimitResponse 返回带正文和 Retry-After 的 429。
//
// 旧限流实现只写状态码，浏览器打开 userscript 下载链接时会被 Tampermonkey/Chrome
// 表现成 ERR_INVALID_RESPONSE，管理员看不到真实原因。这里统一给出可读响应，同时
// 仍避免暴露内部限流 key 或 Redis 状态。
func writeRateLimitResponse(c *gin.Context, retryAfterSeconds int64) {
	if retryAfterSeconds <= 0 {
		retryAfterSeconds = 60
	}
	c.Header("Retry-After", fmt.Sprintf("%d", retryAfterSeconds))
	c.Header("Cache-Control", "no-store")
	c.Header("Content-Type", "application/json; charset=utf-8")
	c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
		"success": false,
		"message": "Too many requests. Please retry later.",
	})
}

// isGlobalAPIRateLimitExemptPath 判断全局 API 限流是否应放行。
//
// 采集助手脚本下载是 Tampermonkey 安装入口，已经在路由层使用独立下载限流保护；
// 如果继续计入 GlobalAPIRateLimit，生产环境里后台页面请求峰值会把 `.user.js`
// 安装请求误伤成 429，导致脚本根本无法安装。动态一次性脚本仍要靠 capture_id、
// install_token 和 TTL 校验，不因这里豁免全局限流而公开敏感会话。
func isGlobalAPIRateLimitExemptPath(path string) bool {
	normalizedPath := strings.TrimSpace(path)
	if normalizedPath == "" {
		return false
	}
	if _, ok := apiRateLimitExemptPaths[normalizedPath]; ok {
		return true
	}
	for _, prefix := range apiRateLimitExemptPrefixes {
		if !strings.HasPrefix(normalizedPath, prefix) {
			continue
		}
		for _, suffix := range apiRateLimitExemptSuffixes {
			if strings.HasSuffix(normalizedPath, suffix) {
				return true
			}
		}
	}
	return false
}

// redisRateLimiter Redis 限流实现。
// 使用 Redis Lua 固定窗口计数器，在一次 Redis 调用中完成计数和判断。
//
// 算法：
// 1. 根据当前时间和 duration 生成窗口 key；
// 2. Lua 脚本执行 INCR，首次写入时设置过期时间；
// 3. 当前窗口计数超过 maxRequestNum 时拒绝请求。
//
// 参数：
//   - c: Gin 上下文
//   - maxRequestNum: 时间窗口内最大请求数
//   - duration: 时间窗口大小（秒）
//   - mark: 限流标识符（用于区分不同类型的限流）
func redisRateLimiter(c *gin.Context, maxRequestNum int, duration int64, mark string) {
	key := buildRedisRateLimitKey("rateLimit:"+mark+c.ClientIP(), duration, time.Now())
	allowed, err := redisFixedWindowAllow(context.Background(), key, maxRequestNum, duration)
	if err != nil {
		fmt.Println(err.Error())
		c.Status(http.StatusInternalServerError)
		c.Abort()
		return
	}
	if !allowed {
		writeRateLimitResponse(c, redisRateLimitExpireSeconds(duration))
		return
	}
}

// buildRedisRateLimitKey 按固定窗口生成 Redis key。
//
// duration 小于等于 0 时使用 1 秒窗口兜底，避免错误环境变量导致除零。窗口序号进入 key，
// 因此每个窗口只保留一个计数器，过期后由 Redis 自动清理。
func buildRedisRateLimitKey(baseKey string, duration int64, now time.Time) string {
	if duration <= 0 {
		duration = 1
	}
	return fmt.Sprintf("%s:%d", baseKey, now.Unix()/duration)
}

// redisRateLimitExpireSeconds 计算固定窗口 key 的过期时间。
//
// 过期时间略长于窗口本身，确保窗口边界附近的请求仍能看到当前计数；同时不沿用旧 List
// 实现的长过期时间，避免每个时间窗口生成的 key 在 Redis 中滞留过久。
func redisRateLimitExpireSeconds(duration int64) int64 {
	if duration <= 0 {
		duration = 1
	}
	expireSeconds := duration + 60
	if expireSeconds < 60 {
		return 60
	}
	return expireSeconds
}

// redisFixedWindowAllow 执行 Redis 固定窗口限流判断。
func redisFixedWindowAllow(ctx context.Context, key string, maxRequestNum int, duration int64) (bool, error) {
	if maxRequestNum <= 0 {
		return false, nil
	}
	result, err := redisFixedWindowRateLimitScript.Run(
		ctx,
		common.RDB,
		[]string{key},
		maxRequestNum,
		redisRateLimitExpireSeconds(duration),
	).Int()
	if err != nil {
		return false, err
	}
	return result == 1, nil
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
		writeRateLimitResponse(c, redisRateLimitExpireSeconds(duration))
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

// isStaticAssetWebRateLimitExemptPath 判断当前 Web 路径是否是前端构建出的静态资源。
//
// default 主题的 Rsbuild 构建会把路由模块拆成大量 /static/js/async/*.js 分片，
// classic 主题的 Vite 构建也会生成 /assets/* 资源。浏览器首次打开或连续切换页面时，
// 这些哈希资源会在短时间内集中请求；如果全部计入默认 60 次 / 180 秒的 IP 级
// GlobalWebRateLimit，静态分片会被 429 拦截，TanStack Router 的动态 import 随后会
// 失败并渲染通用 500 页面。静态资源没有业务写入能力，且由文件名哈希和缓存头控制版本，
// 因此这里仅把明确的构建产物与根级站点资源从 Web 页面访问限流中排除。
//
// 动态页面、认证页、后台页面和 API 路径仍然保留原有全局限流；后端接口自身也继续走
// GlobalAPIRateLimit，不受这里的静态资源豁免影响。
func isStaticAssetWebRateLimitExemptPath(path string) bool {
	normalizedPath := strings.TrimSpace(path)
	if normalizedPath == "" {
		return false
	}

	if strings.HasPrefix(normalizedPath, "/api") ||
		strings.HasPrefix(normalizedPath, "/v1") ||
		strings.HasPrefix(normalizedPath, "/dashboard") {
		return false
	}

	if strings.HasPrefix(normalizedPath, "/static/") ||
		strings.HasPrefix(normalizedPath, "/assets/") {
		return true
	}

	if _, ok := staticAssetWebRateLimitExemptPaths[normalizedPath]; ok {
		return true
	}

	lowerPath := strings.ToLower(normalizedPath)
	for _, ext := range staticAssetWebRateLimitExemptExts {
		if strings.HasSuffix(lowerPath, ext) {
			return true
		}
	}

	return false
}

// GlobalWebRateLimit 全局 Web 请求限流中间件
// 对所有 Web 请求进行基于 IP 的频率限制
// 配置项：GLOBAL_WEB_RATE_LIMIT_ENABLE、GLOBAL_WEB_RATE_LIMIT_NUM、GLOBAL_WEB_RATE_LIMIT_DURATION
func GlobalWebRateLimit() func(c *gin.Context) {
	if common.GlobalWebRateLimitEnable {
		limiter := rateLimitFactory(common.GlobalWebRateLimitNum, common.GlobalWebRateLimitDuration, "GW")
		return func(c *gin.Context) {
			if isStaticAssetWebRateLimitExemptPath(c.Request.URL.Path) {
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
			if isGlobalAPIRateLimitExemptPath(c.Request.URL.Path) {
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

// HelperDownloadRateLimit 专门保护采集助手下载接口。
//
// 采集助手是公开安装入口，但它会被浏览器在短时间内重复请求，且应该比普通下载
// 更宽松地对待 API 峰值。这里使用独立限流标记，避免全局 API 限流把 helper 下载
// 误伤成空响应，同时仍保留明确的 Retry-After 语义。
func HelperDownloadRateLimit() func(c *gin.Context) {
	return rateLimitFactory(common.DownloadRateLimitNum, common.DownloadRateLimitDuration, "HDW")
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
			writeRateLimitResponse(c, redisRateLimitExpireSeconds(duration))
			return
		}
	}
}

// userRedisRateLimiter 是 Redis 版本的用户级限流实现。
//
// 它与 redisRateLimiter 使用同样的固定窗口 Lua 计数器，但接收调用方预先构造好的 key，
// 这样可以把 key 绑定到 user:{id} 而不是 ClientIP，满足按用户维度限流的需求。
func userRedisRateLimiter(c *gin.Context, maxRequestNum int, duration int64, key string) {
	windowKey := buildRedisRateLimitKey(key, duration, time.Now())
	allowed, err := redisFixedWindowAllow(context.Background(), windowKey, maxRequestNum, duration)
	if err != nil {
		fmt.Println(err.Error())
		c.Status(http.StatusInternalServerError)
		c.Abort()
		return
	}
	if !allowed {
		writeRateLimitResponse(c, redisRateLimitExpireSeconds(duration))
		return
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
