// account_pool_task_limit.go 实现账号池异步任务提交级限流。
// 该能力只约束 RelayTask 的提交请求，不表示完整持久化任务队列，
// 也不会把账号池账号占用延长到上游异步任务完成。
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"

	"github.com/gin-gonic/gin"
)

var (
	// ErrAccountPoolTaskConcurrencyExceeded 表示同一账号池组内同一任务类型的提交并发已满。
	ErrAccountPoolTaskConcurrencyExceeded = errors.New("账号池任务提交并发已满")
	// ErrAccountPoolTaskRateLimitExceeded 表示同一账号池组内同一任务类型的提交频率已达到上限。
	ErrAccountPoolTaskRateLimitExceeded = errors.New("账号池任务提交频率已达到上限")
	// ErrAccountPoolTaskWaitTimeout 表示等待任务提交并发槽位超过分组配置的安全超时时间。
	ErrAccountPoolTaskWaitTimeout = errors.New("等待账号池任务提交槽位超时")

	accountPoolTaskConcurrencyMu sync.Mutex
	accountPoolTaskConcurrency   = map[string]int{}

	accountPoolTaskRateMu       sync.Mutex
	accountPoolTaskRateCounters = map[string]poolGroupRateCounter{}

	// accountPoolTaskLimitRetryInterval 是任务提交并发满时的等待重试间隔；测试会临时缩短它。
	accountPoolTaskLimitRetryInterval = 100 * time.Millisecond
)

// AccountPoolTaskLimitOptions 描述一次异步任务提交限流检查所需的维度。
// 当前限流 key 使用账号池组 + platform + action，不按用户拆分，目的是保护共享账号池组
// 不被某一类异步任务提交流量打满。后续如要做多用户公平调度，需要单独评审。
type AccountPoolTaskLimitOptions struct {
	PoolGroupID int
	Platform    string
	Action      string
}

// ReserveAccountPoolTaskLimit 为异步任务提交预留任务级并发槽位并预占 RPM。
// 调用方应在任务参数校验通过、计费预扣费之前调用；如果返回 nil 错误且配置了任务并发，
// 必须在本次提交请求结束时调用 ReleaseAccountPoolTaskLimit，防止槽位泄漏。
func ReserveAccountPoolTaskLimit(c *gin.Context, opts AccountPoolTaskLimitOptions) (*model.AccountPoolGroup, error) {
	if opts.PoolGroupID <= 0 {
		return nil, nil
	}
	group, err := model.GetAccountPoolGroupById(opts.PoolGroupID)
	if err != nil {
		return nil, err
	}
	if group == nil || group.Status != common.ChannelStatusEnabled {
		return group, nil
	}
	if group.GetTaskMaxConcurrency() <= 0 && group.GetTaskRateLimitRpm() <= 0 {
		return group, nil
	}
	limitKey := BuildAccountPoolTaskLimitKey(group.Id, opts.Platform, opts.Action)
	if err := reserveAccountPoolTaskConcurrencyForRequest(c, group, limitKey); err != nil {
		return group, err
	}
	if !reserveAccountPoolTaskRateLimit(limitKey, group.GetTaskRateLimitRpm()) {
		ReleaseAccountPoolTaskLimit(c)
		return group, ErrAccountPoolTaskRateLimitExceeded
	}
	return group, nil
}

// ReleaseAccountPoolTaskLimit 释放当前请求预留的账号池任务提交并发槽位。
// 只有成功预留过并发槽位的请求才会写入上下文标记；无并发限制或仅 RPM 限制时不会释放任何内容。
func ReleaseAccountPoolTaskLimit(c *gin.Context) {
	if c == nil || !common.GetContextKeyBool(c, constant.ContextKeyPoolTaskReserved) {
		return
	}
	limitKey := common.GetContextKeyString(c, constant.ContextKeyPoolTaskLimitKey)
	if strings.TrimSpace(limitKey) != "" {
		releaseAccountPoolTaskConcurrency(limitKey)
	}
	common.SetContextKey(c, constant.ContextKeyPoolTaskReserved, false)
	common.SetContextKey(c, constant.ContextKeyPoolTaskLimitKey, "")
}

// BuildAccountPoolTaskLimitKey 构造任务提交限流键。
// 限流按 group + platform + action 生效，避免不同任务动作互相挤占，同时保持分组级保护语义。
func BuildAccountPoolTaskLimitKey(groupID int, platform string, action string) string {
	return fmt.Sprintf("%d:%s:%s",
		groupID,
		normalizeAccountPoolTaskLimitPart(platform, "unknown"),
		normalizeAccountPoolTaskLimitPart(action, "default"),
	)
}

func normalizeAccountPoolTaskLimitPart(value string, fallback string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, ":", "_")
	value = strings.ReplaceAll(value, " ", "_")
	if value == "" {
		return fallback
	}
	return value
}

func reserveAccountPoolTaskConcurrencyForRequest(c *gin.Context, group *model.AccountPoolGroup, limitKey string) error {
	if group == nil || group.GetTaskMaxConcurrency() <= 0 {
		return nil
	}
	if reserveAccountPoolTaskConcurrency(limitKey, group.GetTaskMaxConcurrency()) {
		common.SetContextKey(c, constant.ContextKeyPoolTaskReserved, true)
		common.SetContextKey(c, constant.ContextKeyPoolTaskLimitKey, limitKey)
		return nil
	}
	if group.GetTaskLimitAction() != model.AccountPoolTaskLimitActionWait {
		return ErrAccountPoolTaskConcurrencyExceeded
	}
	deadline := time.Now().Add(time.Duration(group.GetTaskLimitWaitSeconds()) * time.Second)
	for time.Now().Before(deadline) {
		if waitErr := waitForNextAccountPoolTaskLimitAttempt(c, deadline); waitErr != nil {
			return waitErr
		}
		if reserveAccountPoolTaskConcurrency(limitKey, group.GetTaskMaxConcurrency()) {
			common.SetContextKey(c, constant.ContextKeyPoolTaskReserved, true)
			common.SetContextKey(c, constant.ContextKeyPoolTaskLimitKey, limitKey)
			return nil
		}
	}
	return fmt.Errorf("%w: %v", ErrAccountPoolTaskWaitTimeout, ErrAccountPoolTaskConcurrencyExceeded)
}

func waitForNextAccountPoolTaskLimitAttempt(c *gin.Context, deadline time.Time) error {
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return ErrAccountPoolTaskWaitTimeout
	}
	interval := accountPoolTaskLimitRetryInterval
	if interval <= 0 {
		interval = 100 * time.Millisecond
	}
	if remaining < interval {
		interval = remaining
	}
	timer := time.NewTimer(interval)
	defer timer.Stop()
	if c == nil || c.Request == nil || c.Request.Context() == nil {
		<-timer.C
		return nil
	}
	select {
	case <-timer.C:
		return nil
	case <-c.Request.Context().Done():
		return c.Request.Context().Err()
	}
}

func reserveAccountPoolTaskConcurrency(limitKey string, maxConcurrency int) bool {
	if strings.TrimSpace(limitKey) == "" || maxConcurrency <= 0 {
		return true
	}
	if common.RedisEnabled && common.RDB != nil {
		key := fmt.Sprintf("nexustok:account_pool:task_concurrency:%s", limitKey)
		value, err := common.RDB.Incr(context.Background(), key).Result()
		if err == nil {
			if value == 1 {
				_ = common.RDB.Expire(context.Background(), key, 10*time.Minute).Err()
			}
			if value <= int64(maxConcurrency) {
				return true
			}
			_ = common.RDB.Decr(context.Background(), key).Err()
			return false
		}
		common.SysLog(fmt.Sprintf("failed to reserve account pool task concurrency in redis, fallback to memory: key=%s, error=%v", limitKey, err))
	}
	accountPoolTaskConcurrencyMu.Lock()
	defer accountPoolTaskConcurrencyMu.Unlock()
	if accountPoolTaskConcurrency[limitKey] >= maxConcurrency {
		return false
	}
	accountPoolTaskConcurrency[limitKey]++
	return true
}

func releaseAccountPoolTaskConcurrency(limitKey string) {
	if strings.TrimSpace(limitKey) == "" {
		return
	}
	if common.RedisEnabled && common.RDB != nil {
		key := fmt.Sprintf("nexustok:account_pool:task_concurrency:%s", limitKey)
		if value, err := common.RDB.Decr(context.Background(), key).Result(); err == nil {
			if value <= 0 {
				_ = common.RDB.Del(context.Background(), key).Err()
			}
			return
		}
	}
	accountPoolTaskConcurrencyMu.Lock()
	defer accountPoolTaskConcurrencyMu.Unlock()
	if accountPoolTaskConcurrency[limitKey] <= 1 {
		delete(accountPoolTaskConcurrency, limitKey)
		return
	}
	accountPoolTaskConcurrency[limitKey]--
}

func reserveAccountPoolTaskRateLimit(limitKey string, rpm int) bool {
	if strings.TrimSpace(limitKey) == "" || rpm <= 0 {
		return true
	}
	window := time.Now().Unix() / 60
	if common.RedisEnabled && common.RDB != nil {
		key := fmt.Sprintf("nexustok:account_pool:task_rate:%s:%d", limitKey, window)
		value, err := common.RDB.Incr(context.Background(), key).Result()
		if err == nil {
			if value == 1 {
				_ = common.RDB.Expire(context.Background(), key, 2*time.Minute).Err()
			}
			if value <= int64(rpm) {
				return true
			}
			_ = common.RDB.Decr(context.Background(), key).Err()
			return false
		}
		common.SysLog(fmt.Sprintf("failed to reserve account pool task rate limit in redis, fallback to memory: key=%s, error=%v", limitKey, err))
	}
	accountPoolTaskRateMu.Lock()
	defer accountPoolTaskRateMu.Unlock()
	counter := accountPoolTaskRateCounters[limitKey]
	if counter.window != window {
		counter = poolGroupRateCounter{window: window}
	}
	if counter.count >= rpm {
		accountPoolTaskRateCounters[limitKey] = counter
		return false
	}
	counter.count++
	accountPoolTaskRateCounters[limitKey] = counter
	return true
}
