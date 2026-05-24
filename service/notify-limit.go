// notify-limit.go
// 本文件实现了通知频率限制功能，用于防止用户在短时间内发送过多通知。
// 支持两种存储后端：
// 1. Redis：适用于分布式部署，使用 Redis 的键值存储和原子递增操作
// 2. 内存（sync.Map）：适用于单机部署或 Redis 不可用的场景
// 限制按用户 ID + 通知类型 + 小时粒度进行计数。

package service

import (
	// 标准库
	"fmt"
	"strconv"
	"sync"
	"time"

	// 项目内部包
	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"

	// 第三方库：字节跳动的轻量级协程池
	"github.com/bytedance/gopkg/util/gopool"
)

// notifyLimitStore 用于在 Redis 不可用时提供基于内存的速率限制
var (
	notifyLimitStore sync.Map  // 内存限制计数存储，key 为 "{userId}:{notifyType}:{小时}"
	cleanupOnce      sync.Once // 确保清理任务只启动一次
)

// limitCount 表示限制计数的内部结构
type limitCount struct {
	Count     int       // 当前计数
	Timestamp time.Time // 计数开始时间
}

// getDuration 获取通知限制的时间窗口时长
// 从配置中读取分钟数并转换为 time.Duration
// 返回值:
//   - time.Duration: 限制时间窗口时长
func getDuration() time.Duration {
	minute := constant.NotificationLimitDurationMinute
	return time.Duration(minute) * time.Minute
}

// startCleanupTask 启动后台清理任务，定期清理过期的内存限制记录
// 每小时执行一次，删除超过限制时间窗口的记录
func startCleanupTask() {
	gopool.Go(func() {
		for {
			time.Sleep(time.Hour) // 每小时清理一次
			now := time.Now()
			// 遍历所有记录，删除超过时间窗口的条目
			notifyLimitStore.Range(func(key, value interface{}) bool {
				if limit, ok := value.(limitCount); ok {
					if now.Sub(limit.Timestamp) >= getDuration() {
						notifyLimitStore.Delete(key) // 已过期，删除
					}
				}
				return true
			})
		}
	})
}

// CheckNotificationLimit 检查用户是否超出通知发送频率限制
// 根据 Redis 是否可用自动选择存储后端
// 参数:
//   - userId: 用户 ID
//   - notifyType: 通知类型（如 "email"、"sms" 等）
// 返回值:
//   - bool: 是否允许发送通知（true=允许，false=已超限）
//   - error: 检查过程中发生错误时返回
func CheckNotificationLimit(userId int, notifyType string) (bool, error) {
	if common.RedisEnabled {
		return checkRedisLimit(userId, notifyType)
	}
	return checkMemoryLimit(userId, notifyType)
}

// checkRedisLimit 使用 Redis 检查通知频率限制
// 使用 Redis 键格式：notify_limit:{userId}:{notifyType}:{小时时间戳}
// 限制按小时粒度计数
// 参数:
//   - userId: 用户 ID
//   - notifyType: 通知类型
// 返回值:
//   - bool: 是否允许发送通知
//   - error: Redis 操作失败时返回错误
func checkRedisLimit(userId int, notifyType string) (bool, error) {
	// 构建 Redis 键：按用户+类型+小时进行计数
	key := fmt.Sprintf("notify_limit:%d:%s:%s", userId, notifyType, time.Now().Format("2006010215"))

	// 获取当前计数
	count, err := common.RedisGet(key)
	if err != nil && err.Error() != "redis: nil" {
		return false, fmt.Errorf("failed to get notification count: %w", err)
	}

	// 如果键不存在，初始化计数为 1 并设置过期时间
	if count == "" {
		err = common.RedisSet(key, "1", getDuration())
		return true, err
	}

	currentCount, _ := strconv.Atoi(count)
	limit := constant.NotifyLimitCount

	// 检查是否已达到限制
	if currentCount >= limit {
		return false, nil
	}

	// 未超限，递增计数
	err = common.RedisIncr(key, 1)
	if err != nil {
		return false, fmt.Errorf("failed to increment notification count: %w", err)
	}

	return true, nil
}

// checkMemoryLimit 使用内存检查通知频率限制（Redis 不可用时的回退方案）
// 使用 sync.Map 存储限制计数，键格式：{userId}:{notifyType}:{小时时间戳}
// 参数:
//   - userId: 用户 ID
//   - notifyType: 通知类型
// 返回值:
//   - bool: 是否允许发送通知
//   - error: 始终返回 nil（内存操作不会失败）
func checkMemoryLimit(userId int, notifyType string) (bool, error) {
	// 确保清理任务已启动（仅执行一次）
	cleanupOnce.Do(startCleanupTask)

	// 构建内存存储键：按用户+类型+小时进行计数
	key := fmt.Sprintf("%d:%s:%s", userId, notifyType, time.Now().Format("2006010215"))
	now := time.Now()

	// 获取当前限制计数，如果不存在则初始化
	var currentLimit limitCount
	if value, ok := notifyLimitStore.Load(key); ok {
		currentLimit = value.(limitCount)
		// 检查条目是否已过期
		if now.Sub(currentLimit.Timestamp) >= getDuration() {
			currentLimit = limitCount{Count: 0, Timestamp: now} // 已过期，重置计数
		}
	} else {
		currentLimit = limitCount{Count: 0, Timestamp: now} // 新条目，初始化
	}

	// 递增计数
	currentLimit.Count++

	// 获取限制阈值
	limit := constant.NotifyLimitCount

	// 存储更新后的计数
	notifyLimitStore.Store(key, currentLimit)

	return currentLimit.Count <= limit, nil // 计数未超限则允许发送
}
