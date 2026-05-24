// subscription_reset_task.go
// 本文件实现了订阅配额重置的定时任务。
// 该任务定期执行以下维护操作：
// 1. 将到期的订阅标记为过期状态
// 2. 重置需要周期性重置的订阅配额
// 3. 清理过期的预消费记录（每 30 分钟执行一次）
// 仅在主节点上运行，避免多实例重复操作。

package service

import (
	// 标准库
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	// 项目内部包
	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/logger"
	"github.com/c1cada/NexusTok/model"

	// 第三方库：字节跳动的轻量级协程池
	"github.com/bytedance/gopkg/util/gopool"
)

// 订阅维护任务的配置常量
const (
	subscriptionResetTickInterval = 1 * time.Minute  // 定时任务执行间隔
	subscriptionResetBatchSize    = 300              // 每批处理的订阅数量
	subscriptionCleanupInterval   = 30 * time.Minute // 预消费记录清理间隔
)

// 使用 sync.Once 和 atomic 确保任务只启动一次且不会并发执行
var (
	subscriptionResetOnce    sync.Once    // 确保定时任务只初始化一次
	subscriptionResetRunning atomic.Bool  // 任务运行状态标记，防止并发执行
	subscriptionCleanupLast  atomic.Int64 // 上次清理预消费记录的时间戳（Unix 秒）
)

// StartSubscriptionQuotaResetTask 启动订阅配额重置定时任务
// 仅在主节点上运行，使用 sync.Once 确保只启动一次
// 启动后立即执行一次维护，之后按配置的间隔定期执行
func StartSubscriptionQuotaResetTask() {
	subscriptionResetOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		gopool.Go(func() {
			logger.LogInfo(context.Background(), fmt.Sprintf("subscription quota reset task started: tick=%s", subscriptionResetTickInterval))
			ticker := time.NewTicker(subscriptionResetTickInterval)
			defer ticker.Stop()

			runSubscriptionQuotaResetOnce()
			for range ticker.C {
				runSubscriptionQuotaResetOnce()
			}
		})
	})
}

// runSubscriptionQuotaResetOnce 执行一次订阅配额维护
// 使用 atomic.Bool 的 CAS 操作防止并发执行
// 执行流程：
// 1. 分批处理到期订阅的过期标记
// 2. 分批处理需要重置配额的订阅
// 3. 定期清理过期的预消费记录
func runSubscriptionQuotaResetOnce() {
	if !subscriptionResetRunning.CompareAndSwap(false, true) {
		return
	}
	defer subscriptionResetRunning.Store(false)

	ctx := context.Background()
	totalReset := 0   // 重置配额的订阅总数
	totalExpired := 0 // 标记过期的订阅总数

	// 第一步：分批处理到期订阅的过期标记
	for {
		n, err := model.ExpireDueSubscriptions(subscriptionResetBatchSize)
		if err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("subscription expire task failed: %v", err))
			return
		}
		if n == 0 {
			break
		}
		totalExpired += n
		if n < subscriptionResetBatchSize {
			break
		}
	}
	// 第二步：分批处理需要重置配额的订阅
	for {
		n, err := model.ResetDueSubscriptions(subscriptionResetBatchSize)
		if err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("subscription quota reset task failed: %v", err))
			return
		}
		if n == 0 {
			break
		}
		totalReset += n
		if n < subscriptionResetBatchSize {
			break
		}
	}

	// 第三步：定期清理过期的预消费记录（保留 7 天内的记录）
	lastCleanup := time.Unix(subscriptionCleanupLast.Load(), 0)
	if time.Since(lastCleanup) >= subscriptionCleanupInterval {
		if _, err := model.CleanupSubscriptionPreConsumeRecords(7 * 24 * 3600); err == nil {
			subscriptionCleanupLast.Store(time.Now().Unix()) // 更新上次清理时间
		}
	}
	if common.DebugEnabled && (totalReset > 0 || totalExpired > 0) {
		logger.LogDebug(ctx, "subscription maintenance: reset_count=%d, expired_count=%d", totalReset, totalExpired)
	}
}
