// Package middleware - model-rate-limit.go
// 该文件实现了模型请求频率限制中间件
//
// 与全局限流不同，模型限流是基于用户 ID 的细粒度限流
// 支持两种限制维度：
// 1. 总请求数限制：限制时间窗口内的总请求次数（包括失败请求）
// 2. 成功请求数限制：限制时间窗口内的成功请求次数
//
// 限流算法：
// - 总请求数：使用令牌桶算法（Token Bucket）实现
// - 成功请求数：使用滑动窗口算法实现
//
// 存储后端：
// - Redis: 使用 go-redis 的 List 和令牌桶实现
// - 内存: 使用 InMemoryRateLimiter 实现
//
// 配置项：
// - MODEL_REQUEST_RATE_LIMIT_ENABLED: 是否启用模型请求限流
// - MODEL_REQUEST_RATE_LIMIT_COUNT: 总请求数限制
// - MODEL_REQUEST_RATE_LIMIT_SUCCESS_COUNT: 成功请求数限制
// - MODEL_REQUEST_RATE_LIMIT_DURATION_MINUTES: 时间窗口（分钟）
package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/c1cada/NexusTok/common"           // 公共工具包
	"github.com/c1cada/NexusTok/common/limiter"    // 令牌桶限流器
	"github.com/c1cada/NexusTok/constant"          // 常量定义
	"github.com/c1cada/NexusTok/setting"           // 配置管理

	"github.com/gin-gonic/gin"   // Gin 框架
	"github.com/go-redis/redis/v8" // Redis 客户端
)

// 限流标识符常量
const (
	ModelRequestRateLimitCountMark        = "MRRL"  // 总请求数限流标识
	ModelRequestRateLimitSuccessCountMark = "MRRLS" // 成功请求数限流标识
)

// checkRedisRateLimit 检查 Redis 中的请求限制
// 使用滑动窗口算法检查是否超过限流阈值
//
// 参数：
//   - ctx: 上下文
//   - rdb: Redis 客户端
//   - key: Redis 键名
//   - maxCount: 最大请求数（0 表示不限制）
//   - duration: 时间窗口大小（秒）
//
// 返回值：
//   - bool: 是否允许请求
//   - error: 检查错误
func checkRedisRateLimit(ctx context.Context, rdb *redis.Client, key string, maxCount int, duration int64) (bool, error) {
	// 如果maxCount为0，表示不限制
	if maxCount == 0 {
		return true, nil
	}

	// 获取当前计数
	length, err := rdb.LLen(ctx, key).Result()
	if err != nil {
		return false, err
	}

	// 如果未达到限制，允许请求
	if length < int64(maxCount) {
		return true, nil
	}

	// 检查时间窗口
	oldTimeStr, _ := rdb.LIndex(ctx, key, -1).Result()
	oldTime, err := time.Parse(timeFormat, oldTimeStr)
	if err != nil {
		return false, err
	}

	nowTimeStr := time.Now().Format(timeFormat)
	nowTime, err := time.Parse(timeFormat, nowTimeStr)
	if err != nil {
		return false, err
	}
	// 如果在时间窗口内已达到限制，拒绝请求
	subTime := nowTime.Sub(oldTime).Seconds()
	if int64(subTime) < duration {
		rdb.Expire(ctx, key, time.Duration(setting.ModelRequestRateLimitDurationMinutes)*time.Minute)
		return false, nil
	}

	return true, nil
}

// recordRedisRequest 记录 Redis 请求
// 将当前时间戳推入 List 头部，并截断 List 到最大长度
//
// 参数：
//   - ctx: 上下文
//   - rdb: Redis 客户端
//   - key: Redis 键名
//   - maxCount: 最大请求数（用于截断 List）
func recordRedisRequest(ctx context.Context, rdb *redis.Client, key string, maxCount int) {
	// 如果maxCount为0，不记录请求
	if maxCount == 0 {
		return
	}

	now := time.Now().Format(timeFormat)
	rdb.LPush(ctx, key, now)
	rdb.LTrim(ctx, key, 0, int64(maxCount-1))
	rdb.Expire(ctx, key, time.Duration(setting.ModelRequestRateLimitDurationMinutes)*time.Minute)
}

// redisRateLimitHandler Redis 限流处理器
// 使用 Redis 实现双维度限流：
// 1. 成功请求数限制（滑动窗口）
// 2. 总请求数限制（令牌桶）
//
// 处理流程：
// 1. 检查成功请求数限制
// 2. 检查总请求数限制（当 totalMaxCount > 0 时）
// 3. 处理请求
// 4. 如果请求成功，记录成功请求
//
// 参数：
//   - duration: 时间窗口大小（秒）
//   - totalMaxCount: 总请求数限制（0 表示不限制）
//   - successMaxCount: 成功请求数限制
//
// 返回值：
//   - gin.HandlerFunc: 限流中间件函数
func redisRateLimitHandler(duration int64, totalMaxCount, successMaxCount int) gin.HandlerFunc {
	return func(c *gin.Context) {
		userId := strconv.Itoa(c.GetInt("id"))
		ctx := context.Background()
		rdb := common.RDB

		// 1. 检查成功请求数限制
		successKey := fmt.Sprintf("rateLimit:%s:%s", ModelRequestRateLimitSuccessCountMark, userId)
		allowed, err := checkRedisRateLimit(ctx, rdb, successKey, successMaxCount, duration)
		if err != nil {
			fmt.Println("检查成功请求数限制失败:", err.Error())
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "rate_limit_check_failed")
			return
		}
		if !allowed {
			abortWithOpenAiMessage(c, http.StatusTooManyRequests, fmt.Sprintf("您已达到请求数限制：%d分钟内最多请求%d次", setting.ModelRequestRateLimitDurationMinutes, successMaxCount))
			return
		}

		//2.检查总请求数限制并记录总请求（当totalMaxCount为0时会自动跳过，使用令牌桶限流器
		if totalMaxCount > 0 {
			totalKey := fmt.Sprintf("rateLimit:%s", userId)
			// 初始化
			tb := limiter.New(ctx, rdb)
			allowed, err = tb.Allow(
				ctx,
				totalKey,
				limiter.WithCapacity(int64(totalMaxCount)*duration),
				limiter.WithRate(int64(totalMaxCount)),
				limiter.WithRequested(duration),
			)

			if err != nil {
				fmt.Println("检查总请求数限制失败:", err.Error())
				abortWithOpenAiMessage(c, http.StatusInternalServerError, "rate_limit_check_failed")
				return
			}

			if !allowed {
				abortWithOpenAiMessage(c, http.StatusTooManyRequests, fmt.Sprintf("您已达到总请求数限制：%d分钟内最多请求%d次，包括失败次数，请检查您的请求是否正确", setting.ModelRequestRateLimitDurationMinutes, totalMaxCount))
			}
		}

		// 4. 处理请求
		c.Next()

		// 5. 如果请求成功，记录成功请求
		if c.Writer.Status() < 400 {
			recordRedisRequest(ctx, rdb, successKey, successMaxCount)
		}
	}
}

// memoryRateLimitHandler 内存限流处理器
// 使用内存计数器实现限流，适用于单机部署
//
// 处理流程：
// 1. 检查总请求数限制（当 totalMaxCount > 0 时）
// 2. 检查成功请求数限制
// 3. 处理请求
// 4. 如果请求成功，记录成功请求
//
// 参数：
//   - duration: 时间窗口大小（秒）
//   - totalMaxCount: 总请求数限制（0 表示不限制）
//   - successMaxCount: 成功请求数限制
//
// 返回值：
//   - gin.HandlerFunc: 限流中间件函数
func memoryRateLimitHandler(duration int64, totalMaxCount, successMaxCount int) gin.HandlerFunc {
	inMemoryRateLimiter.Init(time.Duration(setting.ModelRequestRateLimitDurationMinutes) * time.Minute)

	return func(c *gin.Context) {
		userId := strconv.Itoa(c.GetInt("id"))
		totalKey := ModelRequestRateLimitCountMark + userId
		successKey := ModelRequestRateLimitSuccessCountMark + userId

		// 1. 检查总请求数限制（当totalMaxCount为0时跳过）
		if totalMaxCount > 0 && !inMemoryRateLimiter.Request(totalKey, totalMaxCount, duration) {
			c.Status(http.StatusTooManyRequests)
			c.Abort()
			return
		}

		// 2. 检查成功请求数限制
		// 使用一个临时key来检查限制，这样可以避免实际记录
		checkKey := successKey + "_check"
		if !inMemoryRateLimiter.Request(checkKey, successMaxCount, duration) {
			c.Status(http.StatusTooManyRequests)
			c.Abort()
			return
		}

		// 3. 处理请求
		c.Next()

		// 4. 如果请求成功，记录到实际的成功请求计数中
		if c.Writer.Status() < 400 {
			inMemoryRateLimiter.Request(successKey, successMaxCount, duration)
		}
	}
}

// ModelRequestRateLimit 模型请求限流中间件
// 基于用户 ID 的细粒度限流，支持总请求数和成功请求数两个维度
//
// 执行流程：
// 1. 检查是否启用限流
// 2. 计算限流参数（全局配置 + 分组配置覆盖）
// 3. 根据存储类型选择限流处理器（Redis 或内存）
//
// 分组限流配置优先级：分组配置 > 全局配置
//
// 返回值：
//   - func(c *gin.Context): 限流中间件函数
func ModelRequestRateLimit() func(c *gin.Context) {
	return func(c *gin.Context) {
		// 在每个请求时检查是否启用限流
		if !setting.ModelRequestRateLimitEnabled {
			c.Next()
			return
		}

		// 计算限流参数
		duration := int64(setting.ModelRequestRateLimitDurationMinutes * 60)
		totalMaxCount := setting.ModelRequestRateLimitCount
		successMaxCount := setting.ModelRequestRateLimitSuccessCount

		// 获取分组
		group := common.GetContextKeyString(c, constant.ContextKeyTokenGroup)
		if group == "" {
			group = common.GetContextKeyString(c, constant.ContextKeyUserGroup)
		}

		//获取分组的限流配置
		groupTotalCount, groupSuccessCount, found := setting.GetGroupRateLimit(group)
		if found {
			totalMaxCount = groupTotalCount
			successMaxCount = groupSuccessCount
		}

		// 根据存储类型选择并执行限流处理器
		if common.RedisEnabled {
			redisRateLimitHandler(duration, totalMaxCount, successMaxCount)(c)
		} else {
			memoryRateLimitHandler(duration, totalMaxCount, successMaxCount)(c)
		}
	}
}
