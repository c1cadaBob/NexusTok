// account_pool_auto_check_task.go 实现账号池分组的自动可用性检测调度。
//
// 该任务只负责按分组配置判断“是否应该创建检测任务”。真正的账号检测、结果持久化、
// 账号状态更新和状态日志记录全部复用 account_pool_check.go 中的后台检测任务队列。
// 这样自动检测和管理员手动检测共享同一套安全上限、凭据脱敏和审计链路。
package service

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/logger"
	"github.com/c1cada/NexusTok/model"
)

const (
	accountPoolAutoCheckTickInterval = time.Minute
	accountPoolAutoCheckBatchSize    = 100
	accountPoolAutoCheckRetryDelay   = 10 * time.Minute
	accountPoolAutoCheckActor        = "system:auto_check"
)

var (
	accountPoolAutoCheckOnce    sync.Once
	accountPoolAutoCheckRunning atomic.Bool
)

// StartAccountPoolAutoCheckTask 启动账号池自动可用性检测调度任务。
// 任务仅在主节点运行，避免多实例部署时多个进程同时为同一分组创建检测任务。
func StartAccountPoolAutoCheckTask() {
	accountPoolAutoCheckOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		gopool.Go(func() {
			logger.LogInfo(context.Background(), fmt.Sprintf("account pool auto-check task started: tick=%s", accountPoolAutoCheckTickInterval))
			ticker := time.NewTicker(accountPoolAutoCheckTickInterval)
			defer ticker.Stop()
			runAccountPoolAutoCheckOnce()
			for range ticker.C {
				runAccountPoolAutoCheckOnce()
			}
		})
	})
}

// runAccountPoolAutoCheckOnce 执行一次自动检测扫描。
// 扫描只处理已启用、原生来源且到达 auto_check_next_time 的分组；每个分组最多创建一个
// 后台检测任务。创建成功或跳过空分组后都会推进 next_time，避免同一分组被重复扫描。
func runAccountPoolAutoCheckOnce() {
	if !accountPoolAutoCheckRunning.CompareAndSwap(false, true) {
		return
	}
	defer accountPoolAutoCheckRunning.Store(false)

	ctx := context.Background()
	now := common.GetTimestamp()
	groups, err := loadDueAccountPoolAutoCheckGroups(now, accountPoolAutoCheckBatchSize)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("account pool auto-check: query groups failed: %v", err))
		return
	}
	for _, group := range groups {
		if group == nil || !shouldRunAccountPoolAutoCheck(group, now) {
			continue
		}
		if err := startAccountPoolAutoCheckForGroup(ctx, group, now); err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("account pool auto-check: group_id=%d failed: %v", group.Id, err))
			deferAccountPoolAutoCheckGroup(group.Id, now+int64(accountPoolAutoCheckRetryDelay.Seconds()))
		}
	}
}

func loadDueAccountPoolAutoCheckGroups(now int64, limit int) ([]*model.AccountPoolGroup, error) {
	if limit <= 0 {
		limit = accountPoolAutoCheckBatchSize
	}
	var groups []*model.AccountPoolGroup
	err := model.DB.
		Where(
			"source = ? AND status = ? AND auto_check_enabled = ? AND (auto_check_next_time = 0 OR auto_check_next_time <= ?)",
			model.AccountPoolGroupSourceNative,
			common.ChannelStatusEnabled,
			true,
			now,
		).
		Order("id asc").
		Limit(limit).
		Find(&groups).Error
	return groups, err
}

// shouldRunAccountPoolAutoCheck 判断分组是否满足自动检测的基本条件。
// 该函数同时作为查询条件之外的防线，便于未来调用方绕过查询函数时仍不会调度禁用分组
// 或遗留外部来源分组。
func shouldRunAccountPoolAutoCheck(group *model.AccountPoolGroup, now int64) bool {
	if group == nil || !group.AutoCheckEnabled {
		return false
	}
	if group.Status != common.ChannelStatusEnabled {
		return false
	}
	if group.Source != "" && group.Source != model.AccountPoolGroupSourceNative {
		return false
	}
	return group.AutoCheckNextTime <= 0 || group.AutoCheckNextTime <= now
}

func startAccountPoolAutoCheckForGroup(ctx context.Context, group *model.AccountPoolGroup, now int64) error {
	if group == nil || group.Id <= 0 {
		return fmt.Errorf("account pool group is required")
	}
	accountCount, err := countPoolAccountsForAutoCheck(group.Id)
	if err != nil {
		return err
	}
	nextTime := nextAccountPoolAutoCheckTime(now, group.GetAutoCheckIntervalMinutes())
	if accountCount <= 0 {
		if err := updateAccountPoolAutoCheckSchedule(group.Id, map[string]interface{}{
			"auto_check_next_time": nextTime,
		}); err != nil {
			return err
		}
		if common.DebugEnabled {
			logger.LogDebug(ctx, "account pool auto-check: group_id=%d skipped empty group", group.Id)
		}
		return nil
	}
	task, err := StartPoolAccountCheckTask(AccountPoolCheckTaskOptions{
		PoolGroupID: group.Id,
		Limit:       group.GetAutoCheckLimit(),
		Actor:       accountPoolAutoCheckActor,
		RequestID:   accountPoolAutoCheckRequestID(group.Id, now),
	})
	if err != nil {
		return err
	}
	if err := updateAccountPoolAutoCheckSchedule(group.Id, map[string]interface{}{
		"auto_check_last_time":    now,
		"auto_check_next_time":    nextTime,
		"auto_check_last_task_id": task.ID,
	}); err != nil {
		return err
	}
	if common.DebugEnabled {
		logger.LogDebug(ctx, "account pool auto-check: group_id=%d task_id=%d total=%d", group.Id, task.ID, task.Total)
	}
	return nil
}

func countPoolAccountsForAutoCheck(groupID int) (int64, error) {
	var count int64
	err := model.DB.Model(&model.PoolAccount{}).Where("pool_group_id = ?", groupID).Count(&count).Error
	return count, err
}

func updateAccountPoolAutoCheckSchedule(groupID int, updates map[string]interface{}) error {
	if groupID <= 0 || len(updates) == 0 {
		return nil
	}
	if err := model.DB.Model(&model.AccountPoolGroup{}).Where("id = ?", groupID).Updates(updates).Error; err != nil {
		logger.LogWarn(context.Background(), fmt.Sprintf("account pool auto-check: group_id=%d update schedule failed: %v", groupID, err))
		return err
	}
	return nil
}

func deferAccountPoolAutoCheckGroup(groupID int, nextTime int64) {
	_ = updateAccountPoolAutoCheckSchedule(groupID, map[string]interface{}{
		"auto_check_next_time": nextTime,
	})
}

func nextAccountPoolAutoCheckTime(now int64, intervalMinutes int) int64 {
	intervalMinutes = model.NormalizeAccountPoolAutoCheckIntervalMinutes(intervalMinutes)
	return now + int64(intervalMinutes)*60
}

func accountPoolAutoCheckRequestID(groupID int, now int64) string {
	return fmt.Sprintf("account-pool-auto-check-%d-%d", groupID, now)
}
