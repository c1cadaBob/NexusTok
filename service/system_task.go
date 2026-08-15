// Package service - system_task.go
// 该文件实现 SystemTask 后台任务执行器。SystemTask 把日志清理、批量检测、
// 模型同步等耗时管理动作统一成可观察、可续租、防重复的任务记录。
package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/logger"
	"github.com/c1cada/NexusTok/model"

	"github.com/bytedance/gopkg/util/gopool"
)

const (
	systemTaskRunnerIdleInterval       = 15 * time.Second
	systemTaskLockTTL                  = 60 * time.Second
	systemTaskSchedulerInterval        = 15 * time.Second
	systemTaskStaleLockCleanupInterval = 30 * time.Second
	logCleanupBatchSize                = 100
)

// SystemTaskHandler 执行某一类已被 runner 认领的系统任务。
//
// Run 拥有任务终态写入责任：无论成功或失败，handler 返回前都必须调用
// model.FinishSystemTask。handler 还必须尊重 ctx cancellation；runner 在租约丢失时
// 会取消 ctx，避免旧节点继续覆盖新 runner 的任务状态。
type SystemTaskHandler interface {
	Type() string
	Run(ctx context.Context, task *model.SystemTask, runnerID string)
}

// ScheduledSystemTaskHandler 表示可由调度器按周期创建的系统任务。
//
// 当前 NexusTok 先接入手动日志清理；保留该接口是为了后续把渠道测试、模型同步、
// Midjourney/异步任务轮询迁入同一 runner 时不再调整核心调度结构。
type ScheduledSystemTaskHandler interface {
	SystemTaskHandler
	Enabled() bool
	Interval() time.Duration
	NewPayload() any
}

var (
	systemTaskHandlersMu sync.RWMutex
	systemTaskHandlers   = map[string]SystemTaskHandler{}
	systemTaskRunnerOnce sync.Once
	systemTaskWakeup     = make(chan struct{}, 1)
)

// RegisterSystemTaskHandler 注册系统任务 handler。
//
// 同一 Type 重复注册会覆盖旧 handler，便于测试或后续模块替换实现。该函数可以在
// StartSystemTaskRunner 之前或之后调用；runner 每次 claim pass 都会读取最新快照。
func RegisterSystemTaskHandler(handler SystemTaskHandler) {
	if handler == nil || handler.Type() == "" {
		return
	}
	systemTaskHandlersMu.Lock()
	defer systemTaskHandlersMu.Unlock()
	systemTaskHandlers[handler.Type()] = handler
}

func registeredSystemTaskHandlers() []SystemTaskHandler {
	systemTaskHandlersMu.RLock()
	defer systemTaskHandlersMu.RUnlock()
	handlers := make([]SystemTaskHandler, 0, len(systemTaskHandlers))
	for _, handler := range systemTaskHandlers {
		handlers = append(handlers, handler)
	}
	return handlers
}

// StartSystemTaskRunner 启动主节点系统任务 runner。
//
// runner 负责三件事：清理过期租约、按需调度 ScheduledSystemTaskHandler、认领并执行
// pending 任务。非主节点不启动执行器，但仍可通过 API 查看任务状态和实例心跳。
func StartSystemTaskRunner() {
	systemTaskRunnerOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		runnerID := fmt.Sprintf("%s-%s", common.NodeName, common.GetRandomString(8))
		gopool.Go(func() {
			logger.LogInfo(context.Background(), fmt.Sprintf("system task runner started: runner=%s idle_interval=%s", runnerID, systemTaskRunnerIdleInterval))

			ticker := time.NewTicker(systemTaskRunnerIdleInterval)
			defer ticker.Stop()

			var lastScheduler time.Time
			var lastStaleLockCleanup time.Time
			runPass := func() {
				now := time.Now()
				if now.Sub(lastStaleLockCleanup) >= systemTaskStaleLockCleanupInterval {
					lastStaleLockCleanup = now
					if err := model.ExpireStaleSystemTaskLocks(common.GetTimestamp()); err != nil {
						logger.LogWarn(context.Background(), fmt.Sprintf("system task stale lock cleanup failed: %v", err))
					}
				}
				if now.Sub(lastScheduler) >= systemTaskSchedulerInterval {
					lastScheduler = now
					runSystemTaskScheduler()
				}
				runSystemTaskClaimPass(runnerID)
			}

			runPass()
			for {
				select {
				case <-ticker.C:
				case <-systemTaskWakeup:
				}
				runPass()
			}
		})
	})
}

// EnqueueSystemTask 创建一条按需执行的系统任务。
//
// 返回 created=false 表示同类型任务已有 pending/running 记录，此时直接返回现有任务。
func EnqueueSystemTask(taskType string, payload any) (*model.SystemTask, bool, error) {
	task, created, err := model.CreateSystemTaskIfAbsent(taskType, payload, nil)
	if err != nil {
		return nil, false, err
	}
	if created {
		notifySystemTaskRunner()
	}
	return task, created, nil
}

func notifySystemTaskRunner() {
	select {
	case systemTaskWakeup <- struct{}{}:
	default:
	}
}

func runSystemTaskClaimPass(runnerID string) {
	handlers := registeredSystemTaskHandlers()
	taskTypes := make([]string, 0, len(handlers))
	for _, handler := range handlers {
		taskTypes = append(taskTypes, handler.Type())
	}
	pendingTasks, err := model.FindEarliestPendingSystemTasks(taskTypes)
	if err != nil {
		logger.LogWarn(context.Background(), fmt.Sprintf("system task runner query failed: %v", err))
		return
	}
	for _, handler := range handlers {
		task := pendingTasks[handler.Type()]
		if task == nil {
			continue
		}
		claimedTask, claimed, err := model.ClaimSystemTask(task.ID, handler.Type(), runnerID, systemTaskLockUntil())
		if err != nil {
			logger.LogWarn(context.Background(), fmt.Sprintf("system task claim failed: %v", err))
			continue
		}
		if !claimed {
			continue
		}
		dispatchHandler := handler
		dispatchTask := claimedTask
		gopool.Go(func() {
			runWithLeaseHeartbeat(dispatchTask, runnerID, func(ctx context.Context) {
				dispatchHandler.Run(ctx, dispatchTask, runnerID)
			})
			// 同一 Type 可能存在多条使用不同 ActiveKey 排队的任务，例如账号池检测。
			// 单个 handler 完成后主动唤醒下一轮 claim，避免等待完整 idle interval 才处理队列下一项。
			notifySystemTaskRunner()
		})
	}
}

func runSystemTaskScheduler() {
	now := common.GetTimestamp()
	handlers := registeredSystemTaskHandlers()
	scheduledHandlers := make([]ScheduledSystemTaskHandler, 0, len(handlers))
	taskTypes := make([]string, 0, len(handlers))
	for _, handler := range handlers {
		scheduled, ok := handler.(ScheduledSystemTaskHandler)
		if !ok || !scheduled.Enabled() {
			continue
		}
		scheduledHandlers = append(scheduledHandlers, scheduled)
		taskTypes = append(taskTypes, scheduled.Type())
	}
	latestTasks, err := model.GetLatestSystemTasks(taskTypes)
	if err != nil {
		logger.LogWarn(context.Background(), fmt.Sprintf("system task scheduler query failed: %v", err))
		return
	}
	for _, scheduled := range scheduledHandlers {
		latest := latestTasks[scheduled.Type()]
		if latest != nil {
			if latest.Status == model.SystemTaskStatusPending || latest.Status == model.SystemTaskStatusRunning {
				continue
			}
			if now-latest.UpdatedAt < int64(scheduled.Interval().Seconds()) {
				continue
			}
		}
		if _, _, err := model.CreateSystemTaskIfAbsent(scheduled.Type(), scheduled.NewPayload(), nil); err != nil {
			logger.LogWarn(context.Background(), fmt.Sprintf("system task scheduler create failed: type=%s err=%v", scheduled.Type(), err))
		}
	}
}

func runWithLeaseHeartbeat(task *model.SystemTask, runnerID string, fn func(ctx context.Context)) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	interval := systemTaskLockTTL / 3
	if interval <= 0 {
		interval = systemTaskLockTTL
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if err := model.RenewSystemTaskLock(task.TaskID, runnerID, systemTaskLockUntil()); err != nil {
					cancel()
					return
				}
			}
		}
	}()

	fn(ctx)
	close(done)
}

func systemTaskLockUntil() int64 {
	return common.GetTimestamp() + int64(systemTaskLockTTL.Seconds())
}

// LogCleanupPayload 是日志清理任务的输入。
type LogCleanupPayload struct {
	TargetTimestamp int64 `json:"target_timestamp"`
	BatchSize       int   `json:"batch_size"`
}

// LogCleanupState 是日志清理任务的进度状态。
type LogCleanupState struct {
	Total     int64 `json:"total"`
	Processed int64 `json:"processed"`
	Progress  int   `json:"progress"`
	Remaining int64 `json:"remaining"`
}

// LogCleanupResult 是日志清理任务的终态结果。
type LogCleanupResult struct {
	DeletedCount int64 `json:"deleted_count"`
}

type logCleanupHandler struct{}

func (logCleanupHandler) Type() string {
	return model.SystemTaskTypeLogCleanup
}

func (logCleanupHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	runLogCleanupTask(ctx, task, runnerID)
}

func init() {
	RegisterSystemTaskHandler(logCleanupHandler{})
}

// StartLogCleanupTask 创建日志清理系统任务。
//
// 如果已有同类型 pending/running 任务，直接返回现有任务，避免 Root 重复点击造成
// 多个后台删除任务并发执行。
func StartLogCleanupTask(targetTimestamp int64) (*model.SystemTask, error) {
	if targetTimestamp <= 0 {
		return nil, errors.New("target timestamp is required")
	}
	task, created, err := model.CreateSystemTaskIfAbsent(model.SystemTaskTypeLogCleanup, LogCleanupPayload{
		TargetTimestamp: targetTimestamp,
		BatchSize:       logCleanupBatchSize,
	}, LogCleanupState{})
	if err != nil {
		return nil, err
	}
	if created {
		notifySystemTaskRunner()
	}
	return task, nil
}

func runLogCleanupTask(ctx context.Context, task *model.SystemTask, runnerID string) {
	payload := LogCleanupPayload{}
	if err := task.DecodePayload(&payload); err != nil {
		failSystemTask(task, runnerID, err)
		return
	}
	if payload.TargetTimestamp <= 0 {
		failSystemTask(task, runnerID, errors.New("target timestamp is required"))
		return
	}
	if payload.BatchSize <= 0 {
		payload.BatchSize = logCleanupBatchSize
	}

	state := LogCleanupState{}
	if err := task.DecodeState(&state); err != nil {
		failSystemTask(task, runnerID, err)
		return
	}

	for {
		remaining, err := model.CountOldLog(ctx, payload.TargetTimestamp)
		if err != nil {
			failSystemTask(task, runnerID, err)
			return
		}
		syncLogCleanupStateFromRemaining(&state, remaining)
		if err := model.UpdateSystemTaskState(task.TaskID, runnerID, state); err != nil {
			logSystemTaskLockError(ctx, task, err)
			return
		}
		if state.Remaining == 0 {
			break
		}

		progressed := false
		for state.Remaining > 0 {
			rowsAffected, err := model.DeleteOldLogBatch(ctx, payload.TargetTimestamp, payload.BatchSize)
			if err != nil {
				failSystemTask(task, runnerID, err)
				return
			}
			if rowsAffected == 0 {
				break
			}
			progressed = true
			state.Processed += rowsAffected
			if state.Total < state.Processed {
				state.Total = state.Processed
			}
			if state.Remaining > rowsAffected {
				state.Remaining -= rowsAffected
			} else {
				state.Remaining = 0
			}
			state.Progress = logCleanupProgress(state.Processed, state.Total)
			if err := model.UpdateSystemTaskState(task.TaskID, runnerID, state); err != nil {
				logSystemTaskLockError(ctx, task, err)
				return
			}
		}
		if !progressed {
			failSystemTask(task, runnerID, errors.New("no log rows were deleted"))
			return
		}
	}

	state.Remaining = 0
	state.Progress = 100
	if state.Total < state.Processed {
		state.Total = state.Processed
	}
	if err := model.UpdateSystemTaskState(task.TaskID, runnerID, state); err != nil {
		logSystemTaskLockError(ctx, task, err)
		return
	}
	result := LogCleanupResult{DeletedCount: state.Processed}
	if err := model.FinishSystemTask(task.TaskID, runnerID, model.SystemTaskStatusSucceeded, result, ""); err != nil {
		logSystemTaskLockError(ctx, task, err)
	}
}

func syncLogCleanupStateFromRemaining(state *LogCleanupState, remaining int64) {
	if state.Total <= 0 {
		state.Total = remaining
		state.Processed = 0
	} else {
		processedFromRemaining := state.Total - remaining
		if processedFromRemaining > state.Processed {
			state.Processed = processedFromRemaining
		}
	}
	if state.Processed < 0 {
		state.Processed = 0
	}
	state.Remaining = remaining
	state.Progress = logCleanupProgress(state.Processed, state.Total)
}

func logCleanupProgress(processed int64, total int64) int {
	if total <= 0 {
		return 100
	}
	if processed <= 0 {
		return 0
	}
	if processed >= total {
		return 100
	}
	return int(processed * 100 / total)
}

// SystemTaskProgress 是可按百分比汇报进度的系统任务通用状态。
//
// 渠道批量测试、模型同步等任务都会以“已处理数量/总数量”推进；统一状态结构可以让
// 前端任务面板不用理解每一类任务的私有字段，也能稳定展示进度条。
type SystemTaskProgress struct {
	Total     int `json:"total"`
	Processed int `json:"processed"`
	Progress  int `json:"progress"`
}

// NewSystemTaskProgressReporter 创建绑定到 running 任务的进度写入回调。
//
// handler 在循环处理任务项时传入 processed/total；该回调会节流写入数据库，始终保留
// 首次变化和最终 100% 状态。租约丢失时写入错误只作为 best-effort 忽略，runner 的
// 心跳协程会取消 handler ctx，真正的停止语义由 handler 自己检查 ctx 完成。
func NewSystemTaskProgressReporter(task *model.SystemTask, runnerID string) func(processed, total int) {
	const minWriteInterval = 2 * time.Second
	var (
		lastWriteAt  time.Time
		lastProgress = -1
	)
	return func(processed, total int) {
		progress := 100
		if total > 0 {
			progress = processed * 100 / total
		}
		if progress < 0 {
			progress = 0
		}
		if progress > 100 {
			progress = 100
		}

		if progress < 100 {
			if progress == lastProgress {
				return
			}
			if !lastWriteAt.IsZero() && time.Since(lastWriteAt) < minWriteInterval {
				return
			}
		}
		lastProgress = progress
		lastWriteAt = time.Now()

		state := SystemTaskProgress{
			Total:     total,
			Processed: processed,
			Progress:  progress,
		}
		_ = model.UpdateSystemTaskState(task.TaskID, runnerID, state)
	}
}

func failSystemTask(task *model.SystemTask, runnerID string, err error) {
	logger.LogWarn(context.Background(), fmt.Sprintf("system task %s failed: %v", task.TaskID, err))
	if finishErr := model.FinishSystemTask(task.TaskID, runnerID, model.SystemTaskStatusFailed, nil, err.Error()); finishErr != nil {
		logger.LogWarn(context.Background(), fmt.Sprintf("system task %s failed to save failure state: %v", task.TaskID, finishErr))
	}
}

func logSystemTaskLockError(ctx context.Context, task *model.SystemTask, err error) {
	if errors.Is(err, model.ErrSystemTaskLockLost) {
		logger.LogWarn(ctx, fmt.Sprintf("system task %s lock lost", task.TaskID))
		return
	}
	logger.LogWarn(ctx, fmt.Sprintf("system task %s update failed: %v", task.TaskID, err))
}
