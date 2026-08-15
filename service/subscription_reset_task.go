// subscription_reset_task.go
// 本文件实现了订阅配额维护任务。
// 该任务定期执行以下维护操作：
// 1. 将到期的订阅标记为过期状态
// 2. 重置需要周期性重置的订阅配额
// 3. 清理过期的预消费记录（每 30 分钟执行一次）
// 调度和执行统一接入 SystemTask，避免多实例重复操作，并让 Root 可以观察任务历史。

package service

import (
	// 标准库
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	// 项目内部包
	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/logger"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/setting/operation_setting"
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

// SubscriptionMaintenanceState 是订阅维护任务的可观察进度状态。
//
// 订阅维护无法在执行前可靠统计所有待处理行数，因此状态按阶段推进：先处理过期订阅，
// 再重置周期额度，最后按清理间隔删除预消费幂等记录。计数字段在每批处理后写入，便于
// Root 在系统任务面板看到任务是否仍在推进。
type SubscriptionMaintenanceState struct {
	Phase          string `json:"phase"`
	Expired        int    `json:"expired"`
	Reset          int    `json:"reset"`
	CleanupRan     bool   `json:"cleanup_ran"`
	CleanupDeleted int64  `json:"cleanup_deleted"`
	Progress       int    `json:"progress"`
}

// SubscriptionMaintenanceResult 是订阅维护任务完成后的聚合结果。
type SubscriptionMaintenanceResult struct {
	Expired        int   `json:"expired"`
	Reset          int   `json:"reset"`
	CleanupRan     bool  `json:"cleanup_ran"`
	CleanupDeleted int64 `json:"cleanup_deleted"`
	Skipped        bool  `json:"skipped,omitempty"`
}

type subscriptionMaintenanceHandler struct{}

func (subscriptionMaintenanceHandler) Type() string {
	return model.SystemTaskTypeSubscriptionMaintenance
}

func (subscriptionMaintenanceHandler) Enabled() bool {
	return operation_setting.GetSystemTaskSetting().SubscriptionMaintenanceEnabled
}

func (subscriptionMaintenanceHandler) Interval() time.Duration {
	return subscriptionResetTickInterval
}

func (subscriptionMaintenanceHandler) NewPayload() any {
	return nil
}

func (subscriptionMaintenanceHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	if !operation_setting.GetSystemTaskSetting().SubscriptionMaintenanceEnabled {
		if err := model.FinishSystemTask(
			task.TaskID,
			runnerID,
			model.SystemTaskStatusSucceeded,
			map[string]any{
				"skipped":     true,
				"skip_reason": "订阅维护已关闭",
			},
			"",
		); err != nil {
			logSystemTaskLockError(ctx, task, err)
		}
		return
	}
	result, err := RunSubscriptionMaintenanceOnce(ctx, func(state SubscriptionMaintenanceState) error {
		return model.UpdateSystemTaskState(task.TaskID, runnerID, state)
	})
	if err != nil {
		if errors.Is(err, model.ErrSystemTaskLockLost) {
			logSystemTaskLockError(ctx, task, err)
			return
		}
		if finishErr := model.FinishSystemTask(task.TaskID, runnerID, model.SystemTaskStatusFailed, result, err.Error()); finishErr != nil {
			logSystemTaskLockError(ctx, task, finishErr)
		}
		return
	}
	if err := model.FinishSystemTask(task.TaskID, runnerID, model.SystemTaskStatusSucceeded, result, ""); err != nil {
		logSystemTaskLockError(ctx, task, err)
	}
}

func init() {
	RegisterSystemTaskHandler(subscriptionMaintenanceHandler{})
}

// StartSubscriptionQuotaResetTask 兼容旧启动入口，并创建一次订阅维护系统任务。
//
// 周期调度由 subscriptionMaintenanceHandler 交给 SystemTask scheduler 完成；这里仅在
// 主节点启动时确保有一条首次执行的 pending 任务。这样订阅过期、额度重置和预消费记录
// 清理都会进入统一任务历史，并复用数据库租约处理多节点互斥。
func StartSubscriptionQuotaResetTask() {
	subscriptionResetOnce.Do(func() {
		if !common.IsMasterNode ||
			!operation_setting.GetSystemTaskSetting().SubscriptionMaintenanceEnabled {
			return
		}
		task, created, err := enqueueSubscriptionMaintenanceSystemTask()
		if err != nil {
			logger.LogWarn(context.Background(), fmt.Sprintf("subscription maintenance system task enqueue failed: %v", err))
			return
		}
		if created {
			logger.LogInfo(context.Background(), fmt.Sprintf("subscription maintenance system task queued: task_id=%s tick=%s", task.TaskID, subscriptionResetTickInterval))
		}
	})
}

func enqueueSubscriptionMaintenanceSystemTask() (*model.SystemTask, bool, error) {
	activeTask, err := model.GetActiveSystemTask(model.SystemTaskTypeSubscriptionMaintenance)
	if err != nil {
		return nil, false, err
	}
	if activeTask != nil {
		return activeTask, false, nil
	}

	latestTask, err := model.GetLatestSystemTask(model.SystemTaskTypeSubscriptionMaintenance)
	if err != nil {
		return nil, false, err
	}
	if latestTask != nil && common.GetTimestamp()-latestTask.UpdatedAt < int64(subscriptionResetTickInterval.Seconds()) {
		return latestTask, false, nil
	}

	task, created, err := model.CreateSystemTaskIfAbsent(model.SystemTaskTypeSubscriptionMaintenance, nil, nil)
	if err != nil {
		return nil, false, err
	}
	if created {
		notifySystemTaskRunner()
	}
	return task, created, nil
}

// RunSubscriptionMaintenanceOnce 执行一次订阅配额维护。
//
// 使用 atomic.Bool 的 CAS 操作保留进程内防重入保护；跨节点互斥由 SystemTaskLock 提供。
// 执行流程：
// 1. 分批处理到期订阅的过期标记
// 2. 分批处理需要重置配额的订阅
// 3. 定期清理过期的预消费记录
func RunSubscriptionMaintenanceOnce(ctx context.Context, report func(SubscriptionMaintenanceState) error) (SubscriptionMaintenanceResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	result := SubscriptionMaintenanceResult{}
	if !subscriptionResetRunning.CompareAndSwap(false, true) {
		result.Skipped = true
		return result, nil
	}
	defer subscriptionResetRunning.Store(false)

	// 第一步：分批处理到期订阅的过期标记
	state := SubscriptionMaintenanceState{Phase: "expire", Progress: 5}
	if err := reportSubscriptionMaintenanceState(report, state); err != nil {
		return result, err
	}
	for {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		n, err := model.ExpireDueSubscriptions(subscriptionResetBatchSize)
		if err != nil {
			return result, err
		}
		if n == 0 {
			break
		}
		result.Expired += n
		state.Expired = result.Expired
		state.Progress = 30
		if err := reportSubscriptionMaintenanceState(report, state); err != nil {
			return result, err
		}
		if n < subscriptionResetBatchSize {
			break
		}
	}

	// 第二步：分批处理需要重置配额的订阅
	state.Phase = "reset"
	state.Progress = 55
	if err := reportSubscriptionMaintenanceState(report, state); err != nil {
		return result, err
	}
	for {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		n, err := model.ResetDueSubscriptions(subscriptionResetBatchSize)
		if err != nil {
			return result, err
		}
		if n == 0 {
			break
		}
		result.Reset += n
		state.Reset = result.Reset
		state.Progress = 75
		if err := reportSubscriptionMaintenanceState(report, state); err != nil {
			return result, err
		}
		if n < subscriptionResetBatchSize {
			break
		}
	}

	// 第三步：定期清理过期的预消费记录（保留 7 天内的记录）
	state.Phase = "cleanup"
	state.Progress = 90
	if err := reportSubscriptionMaintenanceState(report, state); err != nil {
		return result, err
	}
	lastCleanup := time.Unix(subscriptionCleanupLast.Load(), 0)
	if time.Since(lastCleanup) >= subscriptionCleanupInterval {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		deleted, err := model.CleanupSubscriptionPreConsumeRecords(7 * 24 * 3600)
		if err != nil {
			return result, err
		}
		result.CleanupRan = true
		result.CleanupDeleted = deleted
		subscriptionCleanupLast.Store(time.Now().Unix()) // 更新上次清理时间
	}

	state.Phase = "finished"
	state.Expired = result.Expired
	state.Reset = result.Reset
	state.CleanupRan = result.CleanupRan
	state.CleanupDeleted = result.CleanupDeleted
	state.Progress = 100
	if err := reportSubscriptionMaintenanceState(report, state); err != nil {
		return result, err
	}
	if common.DebugEnabled && (result.Reset > 0 || result.Expired > 0 || result.CleanupDeleted > 0) {
		logger.LogDebug(ctx, "subscription maintenance: reset_count=%d, expired_count=%d, cleanup_deleted=%d", result.Reset, result.Expired, result.CleanupDeleted)
	}
	return result, nil
}

func reportSubscriptionMaintenanceState(report func(SubscriptionMaintenanceState) error, state SubscriptionMaintenanceState) error {
	if report == nil {
		return nil
	}
	return report(state)
}

// runSubscriptionQuotaResetOnce 保留给旧内部调用路径使用，实际启动调度已迁入 SystemTask。
func runSubscriptionQuotaResetOnce() {
	if _, err := RunSubscriptionMaintenanceOnce(context.Background(), nil); err != nil {
		logger.LogWarn(context.Background(), fmt.Sprintf("subscription maintenance task failed: %v", err))
	}
}
