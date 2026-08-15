// account_pool_check.go 实现原生账号池账号的人工可用性检测。
// 检测入口服务于管理员后台：单账号检测用于定位具体凭据问题，分组批量检测用于
// 快速刷新一组账号的健康状态。检测会复用热路径的凭证构造逻辑并更新账号运行统计，
// 但不会写入通用消费日志，避免把管理操作误认为真实用户请求。
package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/service/accountauth"
	"gorm.io/gorm"
)

const (
	accountPoolCheckTimeout                 = 30 * time.Second
	accountPoolCheckBatchLimit              = 100
	accountPoolCheckRetryDelay              = 5 * time.Minute
	accountPoolCheckRecoveryLimit           = 100
	accountPoolCheckTaskListLimit           = 100
	accountPoolCheckTaskCleanupDefaultLimit = 500
	accountPoolCheckTaskCleanupMaxLimit     = 1000

	accountPoolCheckTaskRecoveredMessage   = "account check task recovered and requeued after service restart"
	accountPoolCheckTaskInterruptedMessage = "account check task failed because service restarted while it was running"
	accountPoolCheckTaskRunningMessage     = "account check task is running"
	accountPoolCheckTaskCompletedMessage   = "account check task completed"
)

var (
	accountPoolCheckRecoveryOnce sync.Once
)

// AccountPoolCheckResult 描述一次账号可用性检测的结果。
// checked=true 表示检测流程确实执行过；success=false 时 message 保存脱敏后的失败原因。
type AccountPoolCheckResult struct {
	AccountID     int    `json:"account_id"`
	AccountName   string `json:"account_name"`
	PoolGroupID   int    `json:"pool_group_id"`
	Provider      string `json:"provider"`
	Checked       bool   `json:"checked"`
	Success       bool   `json:"success"`
	Message       string `json:"message"`
	CheckedAt     int64  `json:"checked_at"`
	Refreshed     bool   `json:"refreshed"`
	NextRetryTime int64  `json:"next_retry_time,omitempty"`
}

// AccountPoolBatchCheckResult 汇总分组批量检测结果。
type AccountPoolBatchCheckResult struct {
	Total   int                       `json:"total"`
	Checked int                       `json:"checked"`
	Success int                       `json:"success"`
	Failed  int                       `json:"failed"`
	Skipped int                       `json:"skipped"`
	Items   []*AccountPoolCheckResult `json:"items"`
}

// AccountPoolCheckTaskOptions 描述创建后台检测任务时的筛选条件和审计上下文。
type AccountPoolCheckTaskOptions struct {
	PoolGroupID int
	AccountIDs  []int
	Limit       int
	Actor       string
	RequestID   string
}

// AccountPoolCheckTaskFilter 描述检测任务历史列表筛选条件。
// StartTimestamp 和 EndTimestamp 作用于任务创建时间，便于管理员追溯一段时间内由
// 手动检测或自动检测创建的任务。Search 会匹配任务 ID、分组名、操作者、请求 ID 和消息。
type AccountPoolCheckTaskFilter struct {
	PoolGroupID    int
	Status         string
	Actor          string
	StartTimestamp int64
	EndTimestamp   int64
	Search         string
	StartIdx       int
	Limit          int
}

// AccountPoolCheckTaskRetentionOptions 描述检测任务历史保留清理条件。
// 清理只允许作用于 completed / failed 终态任务，queued / running 代表队列或 worker
// 仍可能持有该任务，不能被保留策略删除。
type AccountPoolCheckTaskRetentionOptions struct {
	PoolGroupID     int
	BeforeTimestamp int64
	Statuses        []string
	Limit           int
}

// AccountPoolCheckTaskView 是后台检测任务面向 API 和前端的脱敏视图。
type AccountPoolCheckTaskView struct {
	ID            int                       `json:"id"`
	PoolGroupID   int                       `json:"pool_group_id"`
	PoolGroupName string                    `json:"pool_group_name"`
	Status        string                    `json:"status"`
	Actor         string                    `json:"actor,omitempty"`
	RequestID     string                    `json:"request_id,omitempty"`
	AccountIDs    []int                     `json:"account_ids"`
	Total         int                       `json:"total"`
	Checked       int                       `json:"checked"`
	Success       int                       `json:"success"`
	Failed        int                       `json:"failed"`
	Skipped       int                       `json:"skipped"`
	Message       string                    `json:"message"`
	Items         []*AccountPoolCheckResult `json:"items"`
	StartedTime   int64                     `json:"started_time"`
	FinishedTime  int64                     `json:"finished_time"`
	CreatedTime   int64                     `json:"created_time"`
	UpdatedTime   int64                     `json:"updated_time"`
}

// AccountPoolCheckSystemTaskPayload 记录 SystemTask 与账号池检测任务的绑定关系。
//
// 一个 `account_pool_check` 系统任务只执行一个 PoolAccountCheckTask；多个检测任务
// 可以各自创建 pending 系统任务排队，但执行租约仍按 `account_pool_check` 类型串行获取。
type AccountPoolCheckSystemTaskPayload struct {
	CheckTaskID int `json:"check_task_id"`
}

// AccountPoolCheckSystemTaskResult 是系统任务完成后保存的检测摘要。
//
// 账号级明细仍只保存在 PoolAccountCheckTask.ResultsJSON 中，SystemTask 结果只放聚合计数，
// 方便 `/system-info` 做全局观测时避免透出账号名称、provider 或错误细节。
type AccountPoolCheckSystemTaskResult struct {
	CheckTaskID int    `json:"check_task_id"`
	Status      string `json:"status"`
	Total       int    `json:"total"`
	Checked     int    `json:"checked"`
	Success     int    `json:"success"`
	Failed      int    `json:"failed"`
	Skipped     int    `json:"skipped"`
	Message     string `json:"message"`
}

// AccountPoolCheckTaskRecoveryResult 描述服务启动时对遗留后台检测任务的恢复结果。
// QueuedRecovered 代表已重新创建 SystemTask 执行入口的 queued 任务数量；RunningArchived
// 代表因旧进程中断而归档为 failed 的 running 任务数量。running 任务不自动重跑，是为了
// 避免重复写入账号状态日志或二次刷新 OAuth 凭据这类有副作用的操作。
type AccountPoolCheckTaskRecoveryResult struct {
	QueuedRecovered int
	RunningArchived int
}

// StartPoolAccountCheckTaskRecovery 恢复服务重启前遗留的后台检测任务。
// 检测任务的执行入口由 SystemTask 持久化承载；服务重启后 queued 任务会重新确保对应
// 系统任务存在，running 任务则代表旧进程已经中断。该入口只在主节点执行一次，避免
// 多实例同时恢复同一批任务。
func StartPoolAccountCheckTaskRecovery() {
	accountPoolCheckRecoveryOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		result, err := recoverPoolAccountCheckTasks()
		if err != nil {
			common.SysLog(fmt.Sprintf("failed to recover pool account check tasks: %v", err))
			return
		}
		if result.QueuedRecovered > 0 || result.RunningArchived > 0 {
			common.SysLog(fmt.Sprintf(
				"pool account check task recovery completed: queued_recovered=%d, running_archived=%d",
				result.QueuedRecovered,
				result.RunningArchived,
			))
		}
	})
}

// CheckPoolAccount 手动检测单个原生账号池账号。
// 检测策略按强度递进：
// 1. 解密凭据并确认非空，这是所有账号可被调度的最低前提；
// 2. 如果账号 provider 支持 Refresh，则实际调用 refresh_token 刷新，验证官方 OAuth 凭据仍可用；
// 3. 如果 provider 不支持刷新，则调用 BuildChannelKey 做本地凭据构造校验。
func CheckPoolAccount(ctx context.Context, accountID int) (*AccountPoolCheckResult, error) {
	if accountID <= 0 {
		return nil, fmt.Errorf("account_id is required")
	}
	account, err := model.GetPoolAccountById(accountID)
	if err != nil {
		return nil, err
	}
	group, err := model.GetAccountPoolGroupById(account.PoolGroupId)
	if err != nil {
		return nil, err
	}
	if err := ensureNativeAccountPoolGroup(group); err != nil {
		return nil, err
	}
	return checkPoolAccount(ctx, account)
}

// CheckPoolAccountsByIDs 批量检测指定账号 ID。
// 该入口用于前端“检测当前页/当前选择”的交互，避免误点时一次检测整个大分组。
// 所有账号必须属于同一个分组；如果 groupID > 0，还会校验账号归属该分组。
func CheckPoolAccountsByIDs(ctx context.Context, groupID int, accountIDs []int) (*AccountPoolBatchCheckResult, error) {
	accountIDs = normalizePoolAccountCheckIDs(accountIDs)
	if len(accountIDs) == 0 {
		return &AccountPoolBatchCheckResult{Items: []*AccountPoolCheckResult{}}, nil
	}
	if len(accountIDs) > accountPoolCheckBatchLimit {
		accountIDs = accountIDs[:accountPoolCheckBatchLimit]
	}
	var accounts []*model.PoolAccount
	query := model.DB.Where("id IN ?", accountIDs)
	if groupID > 0 {
		group, err := model.GetAccountPoolGroupById(groupID)
		if err != nil {
			return nil, err
		}
		if err := ensureNativeAccountPoolGroup(group); err != nil {
			return nil, err
		}
		query = query.Where("pool_group_id = ?", groupID)
	}
	if err := query.Order("id ASC").Find(&accounts).Error; err != nil {
		return nil, err
	}
	return checkPoolAccountList(ctx, accounts), nil
}

// CheckPoolAccountsInGroup 批量检测一个账号池分组内的账号。
// limit <= 0 时使用安全默认值；limit 最大不超过 accountPoolCheckBatchLimit，防止一次请求
// 对大量 OAuth 账号发起刷新导致管理接口长期占用。
func CheckPoolAccountsInGroup(ctx context.Context, groupID int, limit int) (*AccountPoolBatchCheckResult, error) {
	if groupID <= 0 {
		return nil, fmt.Errorf("pool group id is required")
	}
	if limit <= 0 || limit > accountPoolCheckBatchLimit {
		limit = accountPoolCheckBatchLimit
	}
	group, err := model.GetAccountPoolGroupById(groupID)
	if err != nil {
		return nil, err
	}
	if err := ensureNativeAccountPoolGroup(group); err != nil {
		return nil, err
	}
	var accounts []*model.PoolAccount
	if err := model.DB.Where("pool_group_id = ?", groupID).Order("id ASC").Limit(limit).Find(&accounts).Error; err != nil {
		return nil, err
	}
	return checkPoolAccountList(ctx, accounts), nil
}

// StartPoolAccountCheckTask 创建后台检测任务并确保 SystemTask 执行入口存在。
//
// 任务创建阶段会固定本次要检测的账号 ID 快照。这样即使管理员随后增删账号，任务进度
// 仍然可复现；执行时如果账号已被删除，会作为 skipped 记录在结果中。
func StartPoolAccountCheckTask(opts AccountPoolCheckTaskOptions) (*AccountPoolCheckTaskView, error) {
	if opts.PoolGroupID <= 0 {
		return nil, fmt.Errorf("pool group id is required")
	}
	group, err := model.GetAccountPoolGroupById(opts.PoolGroupID)
	if err != nil {
		return nil, err
	}
	if err := ensureNativeAccountPoolGroup(group); err != nil {
		return nil, err
	}
	accountIDs, err := loadPoolAccountIDsForCheckTask(opts.PoolGroupID, opts.AccountIDs, opts.Limit)
	if err != nil {
		return nil, err
	}
	task := &model.PoolAccountCheckTask{
		PoolGroupId:   group.Id,
		PoolGroupName: group.Name,
		Status:        model.PoolAccountCheckTaskStatusQueued,
		Actor:         opts.Actor,
		RequestId:     opts.RequestID,
		AccountIds:    joinAccountPoolCheckTaskIDs(accountIDs),
		Total:         len(accountIDs),
	}
	if err := model.DB.Create(task).Error; err != nil {
		return nil, err
	}
	if _, _, err := ensurePoolAccountCheckSystemTask(task.Id); err != nil {
		failPoolAccountCheckTask(task.Id, fmt.Sprintf("account check task failed to enqueue: %v", err))
		return nil, err
	}
	return PoolAccountCheckTaskPublicView(task), nil
}

func recoverPoolAccountCheckTasks() (AccountPoolCheckTaskRecoveryResult, error) {
	result := AccountPoolCheckTaskRecoveryResult{}
	now := common.GetTimestamp()
	archived, err := archiveInterruptedPoolAccountCheckTasks(now)
	if err != nil {
		return result, err
	}
	result.RunningArchived = archived

	recovered, err := requeuePendingPoolAccountCheckTasks()
	if err != nil {
		return result, err
	}
	result.QueuedRecovered = recovered
	return result, nil
}

func archiveInterruptedPoolAccountCheckTasks(now int64) (int, error) {
	if now <= 0 {
		now = common.GetTimestamp()
	}
	tx := model.DB.Model(&model.PoolAccountCheckTask{}).
		Where("status = ?", model.PoolAccountCheckTaskStatusRunning).
		Updates(map[string]interface{}{
			"status":        model.PoolAccountCheckTaskStatusFailed,
			"message":       accountPoolCheckTaskInterruptedMessage,
			"finished_time": now,
		})
	if tx.Error != nil {
		return 0, tx.Error
	}
	return int(tx.RowsAffected), nil
}

func requeuePendingPoolAccountCheckTasks() (int, error) {
	var tasks []*model.PoolAccountCheckTask
	if err := model.DB.
		Where("status = ?", model.PoolAccountCheckTaskStatusQueued).
		Order("id ASC").
		Limit(accountPoolCheckRecoveryLimit).
		Find(&tasks).Error; err != nil {
		return 0, err
	}
	recovered := 0
	for _, task := range tasks {
		if task == nil || task.Id <= 0 {
			continue
		}
		if err := releaseClaimedPoolAccountCheckSystemTask(task.Id); err != nil {
			return recovered, err
		}
		if err := markPoolAccountCheckTaskRecovered(task.Id); err != nil {
			return recovered, err
		}
		if _, _, err := ensurePoolAccountCheckSystemTask(task.Id); err != nil {
			return recovered, err
		}
		recovered++
	}
	return recovered, nil
}

func releaseClaimedPoolAccountCheckSystemTask(taskID int) error {
	activeTask, err := model.GetActiveSystemTaskByActiveKey(poolAccountCheckSystemTaskActiveKey(taskID))
	if err != nil || activeTask == nil {
		return err
	}
	if activeTask.Status != model.SystemTaskStatusRunning {
		return nil
	}
	if err := model.MarkSystemTaskLeaseExpired(activeTask.TaskID); err != nil {
		return err
	}
	return model.ReleaseSystemTaskLock(activeTask.TaskID, activeTask.LockedBy)
}

func markPoolAccountCheckTaskRecovered(taskID int) error {
	if taskID <= 0 {
		return fmt.Errorf("check task id is required")
	}
	return model.DB.Model(&model.PoolAccountCheckTask{}).
		Where("id = ? AND status = ?", taskID, model.PoolAccountCheckTaskStatusQueued).
		Updates(map[string]interface{}{
			"message":       accountPoolCheckTaskRecoveredMessage,
			"started_time":  0,
			"finished_time": 0,
		}).Error
}

// GetPoolAccountCheckTask 查询后台检测任务并返回脱敏视图。
func GetPoolAccountCheckTask(taskID int) (*AccountPoolCheckTaskView, error) {
	if taskID <= 0 {
		return nil, fmt.Errorf("check task id is required")
	}
	task := &model.PoolAccountCheckTask{}
	if err := model.DB.Where("id = ?", taskID).First(task).Error; err != nil {
		return nil, err
	}
	return PoolAccountCheckTaskPublicView(task), nil
}

// ListPoolAccountCheckTasks 分页查询后台检测任务历史。
// 响应统一走 PoolAccountCheckTaskPublicView，确保数据库中的 results_json 原始字段不会被
// 直接暴露给前端；即使旧数据里混入未知字段，转换到强类型结果时也会被丢弃。
func ListPoolAccountCheckTasks(filter AccountPoolCheckTaskFilter) ([]*AccountPoolCheckTaskView, int64, error) {
	if filter.Limit <= 0 {
		filter.Limit = 20
	}
	if filter.Limit > accountPoolCheckTaskListLimit {
		filter.Limit = accountPoolCheckTaskListLimit
	}
	query := model.DB.Model(&model.PoolAccountCheckTask{})
	if filter.PoolGroupID > 0 {
		query = query.Where("pool_group_id = ?", filter.PoolGroupID)
	}
	if status := normalizePoolAccountCheckTaskStatus(filter.Status); status != "" {
		query = query.Where("status = ?", status)
	}
	if strings.TrimSpace(filter.Actor) != "" {
		query = query.Where("actor = ?", strings.TrimSpace(filter.Actor))
	}
	if filter.StartTimestamp > 0 {
		query = query.Where("created_time >= ?", filter.StartTimestamp)
	}
	if filter.EndTimestamp > 0 {
		query = query.Where("created_time <= ?", filter.EndTimestamp)
	}
	if strings.TrimSpace(filter.Search) != "" {
		search := strings.TrimSpace(filter.Search)
		like := "%" + search + "%"
		query = query.Where("(pool_group_name LIKE ? OR actor LIKE ? OR request_id LIKE ? OR message LIKE ? OR id = ?)", like, like, like, like, parsePositiveInt(search))
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	tasks := []*model.PoolAccountCheckTask{}
	if err := query.
		Order("created_time DESC").
		Order("id DESC").
		Limit(filter.Limit).
		Offset(filter.StartIdx).
		Find(&tasks).Error; err != nil {
		return nil, 0, err
	}
	views := make([]*AccountPoolCheckTaskView, 0, len(tasks))
	for _, task := range tasks {
		views = append(views, PoolAccountCheckTaskPublicView(task))
	}
	return views, total, nil
}

// CleanupPoolAccountCheckTasks 删除符合保留策略的历史检测任务。
// 为兼容 SQLite、MySQL 和 PostgreSQL，清理会先按条件查询有限数量的 ID，再按 ID 集合删除；
// 不直接依赖数据库方言不一致的 DELETE LIMIT。
func CleanupPoolAccountCheckTasks(opts AccountPoolCheckTaskRetentionOptions) (int, error) {
	statuses := normalizePoolAccountCheckTaskCleanupStatuses(opts.Statuses)
	if len(opts.Statuses) == 0 {
		statuses = []string{
			model.PoolAccountCheckTaskStatusCompleted,
			model.PoolAccountCheckTaskStatusFailed,
		}
	} else if len(statuses) == 0 {
		return 0, nil
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = accountPoolCheckTaskCleanupDefaultLimit
	}
	if limit > accountPoolCheckTaskCleanupMaxLimit {
		limit = accountPoolCheckTaskCleanupMaxLimit
	}
	if opts.BeforeTimestamp <= 0 {
		opts.BeforeTimestamp = common.GetTimestamp() - int64((7 * 24 * time.Hour).Seconds())
	}
	query := model.DB.Model(&model.PoolAccountCheckTask{}).
		Where("status IN ?", statuses).
		Where("finished_time > 0 AND finished_time < ?", opts.BeforeTimestamp)
	if opts.PoolGroupID > 0 {
		query = query.Where("pool_group_id = ?", opts.PoolGroupID)
	}
	var taskIDs []int
	if err := query.
		Order("finished_time ASC").
		Order("id ASC").
		Limit(limit).
		Pluck("id", &taskIDs).Error; err != nil {
		return 0, err
	}
	if len(taskIDs) == 0 {
		return 0, nil
	}
	tx := model.DB.Where("id IN ?", taskIDs).Delete(&model.PoolAccountCheckTask{})
	if tx.Error != nil {
		return 0, tx.Error
	}
	return int(tx.RowsAffected), nil
}

// PoolAccountCheckTaskPublicView 将任务模型转换为 API 响应；ResultsJSON 只包含脱敏检测结果。
func PoolAccountCheckTaskPublicView(task *model.PoolAccountCheckTask) *AccountPoolCheckTaskView {
	if task == nil {
		return &AccountPoolCheckTaskView{Items: []*AccountPoolCheckResult{}}
	}
	items := []*AccountPoolCheckResult{}
	if strings.TrimSpace(task.ResultsJSON) != "" {
		if err := common.UnmarshalJsonStr(task.ResultsJSON, &items); err != nil {
			items = []*AccountPoolCheckResult{}
		}
	}
	return &AccountPoolCheckTaskView{
		ID:            task.Id,
		PoolGroupID:   task.PoolGroupId,
		PoolGroupName: task.PoolGroupName,
		Status:        task.Status,
		Actor:         task.Actor,
		RequestID:     task.RequestId,
		AccountIDs:    splitAccountPoolCheckTaskIDs(task.AccountIds),
		Total:         task.Total,
		Checked:       task.Checked,
		Success:       task.Success,
		Failed:        task.Failed,
		Skipped:       task.Skipped,
		Message:       task.Message,
		Items:         items,
		StartedTime:   task.StartedTime,
		FinishedTime:  task.FinishedTime,
		CreatedTime:   task.CreatedTime,
		UpdatedTime:   task.UpdatedTime,
	}
}

func checkPoolAccountList(ctx context.Context, accounts []*model.PoolAccount) *AccountPoolBatchCheckResult {
	result := &AccountPoolBatchCheckResult{
		Total: len(accounts),
		Items: make([]*AccountPoolCheckResult, 0, len(accounts)),
	}
	for _, account := range accounts {
		item, err := checkPoolAccount(ctx, account)
		if err != nil {
			item = accountPoolCheckErrorResult(account, err)
		}
		result.Items = append(result.Items, item)
		if item == nil || !item.Checked {
			result.Skipped++
			continue
		}
		result.Checked++
		if item.Success {
			result.Success++
		} else {
			result.Failed++
		}
	}
	return result
}

func loadPoolAccountIDsForCheckTask(groupID int, requestedIDs []int, limit int) ([]int, error) {
	accountIDs := normalizePoolAccountCheckIDs(requestedIDs)
	if len(accountIDs) > accountPoolCheckBatchLimit {
		accountIDs = accountIDs[:accountPoolCheckBatchLimit]
	}
	if len(accountIDs) > 0 {
		return accountIDs, nil
	}
	if limit <= 0 || limit > accountPoolCheckBatchLimit {
		limit = accountPoolCheckBatchLimit
	}
	var accounts []*model.PoolAccount
	if err := model.DB.
		Where("pool_group_id = ?", groupID).
		Order("id ASC").
		Limit(limit).
		Find(&accounts).Error; err != nil {
		return nil, err
	}
	result := make([]int, 0, len(accounts))
	for _, account := range accounts {
		if account != nil {
			result = append(result, account.Id)
		}
	}
	return result, nil
}

func ensurePoolAccountCheckSystemTask(taskID int) (*model.SystemTask, bool, error) {
	if taskID <= 0 {
		return nil, false, fmt.Errorf("check task id is required")
	}
	activeKey := poolAccountCheckSystemTaskActiveKey(taskID)
	activeTask, err := model.GetActiveSystemTaskByActiveKey(activeKey)
	if err != nil {
		return nil, false, err
	}
	if activeTask != nil {
		return activeTask, false, nil
	}

	view, err := GetPoolAccountCheckTask(taskID)
	if err != nil {
		return nil, false, err
	}
	task, created, err := model.CreateSystemTaskWithActiveKeyIfAbsent(
		model.SystemTaskTypeAccountPoolCheck,
		activeKey,
		AccountPoolCheckSystemTaskPayload{CheckTaskID: taskID},
		SystemTaskProgress{
			Total:     view.Total,
			Processed: view.Checked + view.Skipped,
			Progress:  accountPoolCheckProgress(view.Checked+view.Skipped, view.Total),
		},
	)
	if err != nil {
		return nil, false, err
	}
	if created {
		notifySystemTaskRunner()
	}
	return task, created, nil
}

func poolAccountCheckSystemTaskActiveKey(taskID int) string {
	return fmt.Sprintf("%s:%d", model.SystemTaskTypeAccountPoolCheck, taskID)
}

// RunPoolAccountCheckSystemTask 执行一条已排队的账号池检测任务。
//
// 该函数由 account_pool_check SystemTask handler 调用；PoolAccountCheckTask 继续作为账号池
// 页面和专用历史接口的业务任务记录，SystemTask 负责跨节点认领、租约、进度和全局观测。
// report 回调只写入聚合进度，账号级明细仍通过 PoolAccountCheckTask 的脱敏视图查询。
func RunPoolAccountCheckSystemTask(ctx context.Context, taskID int, report func(processed, total int)) (summary AccountPoolCheckSystemTaskResult, err error) {
	if taskID <= 0 {
		return summary, fmt.Errorf("check task id is required")
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			common.SysLog(fmt.Sprintf("pool account check task panic: task_id=%d, error=%v", taskID, recovered))
			err = fmt.Errorf("account check task failed: %v", recovered)
			failPoolAccountCheckTask(taskID, err.Error())
		}
	}()

	task, terminal, err := startPoolAccountCheckTaskRun(taskID)
	if err != nil {
		failPoolAccountCheckTask(taskID, fmt.Sprintf("account check task failed to start: %v", err))
		return summary, err
	}
	if terminal {
		return poolAccountCheckSystemTaskResult(task), nil
	}
	accountIDs := splitAccountPoolCheckTaskIDs(task.AccountIds)
	result := &AccountPoolBatchCheckResult{
		Total: len(accountIDs),
		Items: make([]*AccountPoolCheckResult, 0, len(accountIDs)),
	}
	if report != nil {
		report(0, result.Total)
	}
	for _, accountID := range accountIDs {
		if ctxErr := ctx.Err(); ctxErr != nil {
			message := fmt.Sprintf("account check task cancelled: %v", ctxErr)
			failPoolAccountCheckTask(taskID, message)
			return poolAccountCheckSystemTaskResultFromBatch(taskID, model.PoolAccountCheckTaskStatusFailed, result, message), ctxErr
		}
		item := runPoolAccountCheckTaskItem(ctx, task.PoolGroupId, accountID)
		result.Items = append(result.Items, item)
		accumulatePoolAccountCheckTaskResult(result, item)
		recordPoolAccountCheckTaskState(task, item)
		processed := len(result.Items)
		if err := updatePoolAccountCheckTaskProgress(taskID, model.PoolAccountCheckTaskStatusRunning, result, accountPoolCheckTaskRunningMessage, 0); err != nil {
			common.SysLog(fmt.Sprintf("failed to update pool account check task progress: task_id=%d, error=%v", taskID, err))
		}
		if report != nil {
			report(processed, result.Total)
		}
	}
	message := accountPoolCheckTaskCompletedMessage
	if result.Failed > 0 {
		message = "account check task completed with failed accounts"
	}
	if err := updatePoolAccountCheckTaskProgress(taskID, model.PoolAccountCheckTaskStatusCompleted, result, message, common.GetTimestamp()); err != nil {
		common.SysLog(fmt.Sprintf("failed to finish pool account check task: task_id=%d, error=%v", taskID, err))
		return poolAccountCheckSystemTaskResultFromBatch(taskID, model.PoolAccountCheckTaskStatusFailed, result, err.Error()), err
	}
	if report != nil {
		report(result.Total, result.Total)
	}
	return poolAccountCheckSystemTaskResultFromBatch(taskID, model.PoolAccountCheckTaskStatusCompleted, result, message), nil
}

func runPoolAccountCheckTask(taskID int) {
	if _, err := RunPoolAccountCheckSystemTask(context.Background(), taskID, nil); err != nil {
		common.SysLog(fmt.Sprintf("pool account check task failed: task_id=%d, error=%v", taskID, err))
	}
}

func startPoolAccountCheckTaskRun(taskID int) (*model.PoolAccountCheckTask, bool, error) {
	startedAt := common.GetTimestamp()
	tx := model.DB.Model(&model.PoolAccountCheckTask{}).
		Where("id = ? AND status = ?", taskID, model.PoolAccountCheckTaskStatusQueued).
		Updates(map[string]interface{}{
			"status":       model.PoolAccountCheckTaskStatusRunning,
			"started_time": startedAt,
			"message":      accountPoolCheckTaskRunningMessage,
		})
	if tx.Error != nil {
		return nil, false, tx.Error
	}
	task := &model.PoolAccountCheckTask{}
	if err := model.DB.Where("id = ?", taskID).First(task).Error; err != nil {
		return nil, false, err
	}
	if tx.RowsAffected > 0 {
		return task, false, nil
	}
	switch task.Status {
	case model.PoolAccountCheckTaskStatusCompleted, model.PoolAccountCheckTaskStatusFailed:
		return task, true, nil
	case model.PoolAccountCheckTaskStatusRunning:
		return task, false, fmt.Errorf("account check task is already running")
	default:
		return task, false, fmt.Errorf("account check task is not queued: status=%s", task.Status)
	}
}

func failPoolAccountCheckTask(taskID int, message string) {
	if taskID <= 0 {
		return
	}
	_ = model.DB.Model(&model.PoolAccountCheckTask{}).
		Where("id = ?", taskID).
		Updates(map[string]interface{}{
			"status":        model.PoolAccountCheckTaskStatusFailed,
			"message":       common.MaskSensitiveInfo(message),
			"finished_time": common.GetTimestamp(),
		}).Error
}

func runPoolAccountCheckTaskItem(ctx context.Context, groupID int, accountID int) *AccountPoolCheckResult {
	account := &model.PoolAccount{}
	if err := model.DB.Where("id = ? AND pool_group_id = ?", accountID, groupID).First(account).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return &AccountPoolCheckResult{
				AccountID:   accountID,
				PoolGroupID: groupID,
				Checked:     false,
				Success:     false,
				Message:     "account not found in group",
				CheckedAt:   common.GetTimestamp(),
			}
		}
		return accountPoolCheckErrorResult(&model.PoolAccount{Id: accountID, PoolGroupId: groupID}, err)
	}
	item, err := checkPoolAccount(ctx, account)
	if err != nil {
		item = accountPoolCheckErrorResult(account, err)
	}
	return item
}

func poolAccountCheckSystemTaskResult(task *model.PoolAccountCheckTask) AccountPoolCheckSystemTaskResult {
	if task == nil {
		return AccountPoolCheckSystemTaskResult{}
	}
	return AccountPoolCheckSystemTaskResult{
		CheckTaskID: task.Id,
		Status:      task.Status,
		Total:       task.Total,
		Checked:     task.Checked,
		Success:     task.Success,
		Failed:      task.Failed,
		Skipped:     task.Skipped,
		Message:     task.Message,
	}
}

func poolAccountCheckSystemTaskResultFromBatch(taskID int, status string, result *AccountPoolBatchCheckResult, message string) AccountPoolCheckSystemTaskResult {
	if result == nil {
		result = &AccountPoolBatchCheckResult{Items: []*AccountPoolCheckResult{}}
	}
	return AccountPoolCheckSystemTaskResult{
		CheckTaskID: taskID,
		Status:      status,
		Total:       result.Total,
		Checked:     result.Checked,
		Success:     result.Success,
		Failed:      result.Failed,
		Skipped:     result.Skipped,
		Message:     common.MaskSensitiveInfo(message),
	}
}

func accountPoolCheckProgress(processed int, total int) int {
	if total <= 0 {
		return 100
	}
	if processed <= 0 {
		return 0
	}
	if processed >= total {
		return 100
	}
	return processed * 100 / total
}

func accumulatePoolAccountCheckTaskResult(result *AccountPoolBatchCheckResult, item *AccountPoolCheckResult) {
	if result == nil || item == nil {
		return
	}
	if !item.Checked {
		result.Skipped++
		return
	}
	result.Checked++
	if item.Success {
		result.Success++
	} else {
		result.Failed++
	}
}

func updatePoolAccountCheckTaskProgress(taskID int, status string, result *AccountPoolBatchCheckResult, message string, finishedAt int64) error {
	if result == nil {
		result = &AccountPoolBatchCheckResult{Items: []*AccountPoolCheckResult{}}
	}
	data, err := common.Marshal(result.Items)
	if err != nil {
		return err
	}
	updates := map[string]interface{}{
		"status":       status,
		"total":        result.Total,
		"checked":      result.Checked,
		"success":      result.Success,
		"failed":       result.Failed,
		"skipped":      result.Skipped,
		"message":      message,
		"results_json": string(data),
	}
	if finishedAt > 0 {
		updates["finished_time"] = finishedAt
	}
	return model.DB.Model(&model.PoolAccountCheckTask{}).Where("id = ?", taskID).Updates(updates).Error
}

func recordPoolAccountCheckTaskState(task *model.PoolAccountCheckTask, item *AccountPoolCheckResult) {
	if task == nil || item == nil || !item.Checked || item.AccountID <= 0 {
		return
	}
	action := model.PoolAccountStateActionCheckFailed
	if item.Success {
		action = model.PoolAccountStateActionCheckSucceeded
	}
	model.RecordPoolAccountStateLog(model.PoolAccountStateLogRecord{
		PoolAccountId: item.AccountID,
		Action:        action,
		Source:        "admin",
		Actor:         task.Actor,
		Reason:        item.Message,
		RequestId:     task.RequestId,
	})
}

func checkPoolAccount(ctx context.Context, account *model.PoolAccount) (*AccountPoolCheckResult, error) {
	if account == nil {
		return nil, fmt.Errorf("account is required")
	}
	checkedAt := common.GetTimestamp()
	result := &AccountPoolCheckResult{
		AccountID:   account.Id,
		AccountName: account.Name,
		PoolGroupID: account.PoolGroupId,
		Provider:    account.GetCredentialProvider(),
		Checked:     true,
		CheckedAt:   checkedAt,
	}
	if strings.TrimSpace(account.Credentials) == "" {
		err := fmt.Errorf("account credential is empty")
		markPoolAccountCheckFailed(account, checkedAt, err)
		result.Message = err.Error()
		result.NextRetryTime = checkedAt + int64(accountPoolCheckRetryDelay.Seconds())
		return result, nil
	}
	raw, err := account.GetDecryptedCredentials()
	if err != nil {
		markPoolAccountCheckFailed(account, checkedAt, err)
		result.Message = common.MaskSensitiveInfo(err.Error())
		result.NextRetryTime = checkedAt + int64(accountPoolCheckRetryDelay.Seconds())
		return result, nil
	}
	if strings.TrimSpace(raw) == "" {
		err = fmt.Errorf("account credential is empty")
		markPoolAccountCheckFailed(account, checkedAt, err)
		result.Message = err.Error()
		result.NextRetryTime = checkedAt + int64(accountPoolCheckRetryDelay.Seconds())
		return result, nil
	}
	provider, ok := accountauth.DefaultManager().Provider(result.Provider)
	checkCtx, cancel := context.WithTimeout(ctx, accountPoolCheckTimeout)
	defer cancel()
	if ok && account.AuthType == model.AccountPoolAuthTypeOfficialOAuth && provider.RefreshLead() != nil && poolAccountCredentialAllowsRefresh(account) {
		credential, refreshErr := provider.Refresh(checkCtx, account)
		if refreshErr != nil {
			markPoolAccountCheckFailed(account, checkedAt, refreshErr)
			result.Message = common.MaskSensitiveInfo(refreshErr.Error())
			result.NextRetryTime = checkedAt + int64(accountPoolCheckRetryDelay.Seconds())
			return result, nil
		}
		if err := updatePoolAccountCredentialFromAuth(account, credential); err != nil {
			markPoolAccountCheckFailed(account, checkedAt, err)
			result.Message = common.MaskSensitiveInfo(err.Error())
			result.NextRetryTime = checkedAt + int64(accountPoolCheckRetryDelay.Seconds())
			return result, nil
		}
		if err := markPoolAccountCheckSucceeded(account, checkedAt, "credential refreshed and account is available"); err != nil {
			return nil, err
		}
		result.Success = true
		result.Refreshed = true
		result.Message = "credential refreshed and account is available"
		return result, nil
	}
	if _, err := BuildPoolAccountChannelKey(account); err != nil {
		markPoolAccountCheckFailed(account, checkedAt, err)
		result.Message = common.MaskSensitiveInfo(err.Error())
		result.NextRetryTime = checkedAt + int64(accountPoolCheckRetryDelay.Seconds())
		return result, nil
	}
	if err := markPoolAccountCheckSucceeded(account, checkedAt, "credential is available"); err != nil {
		return nil, err
	}
	result.Success = true
	result.Message = "credential is available"
	return result, nil
}

func markPoolAccountCheckSucceeded(account *model.PoolAccount, checkedAt int64, message string) error {
	if account == nil {
		return nil
	}
	recentRequests := accountauth.RecordRecentRequest(account.RecentRequests, time.Unix(checkedAt, 0), true)
	model.RecordPoolAccountRequest(account.Id, true, recentRequests)
	updates := map[string]interface{}{
		"last_checked_time":   checkedAt,
		"unavailable":         false,
		"status_message":      message,
		"last_error":          "",
		"rate_limited_until":  0,
		"overload_until":      0,
		"temp_disabled_until": 0,
		"next_retry_time":     0,
		"disabled_reason":     "",
	}
	if account.Status == common.ChannelStatusAutoDisabled {
		updates["status"] = common.ChannelStatusEnabled
		updates["schedulable"] = true
	} else if account.Status == common.ChannelStatusEnabled {
		updates["schedulable"] = true
	} else if account.Status == common.ChannelStatusManuallyDisabled {
		// 手动禁用代表管理员的显式调度意图。即使检测或 OAuth 刷新成功，
		// 也只刷新健康状态，不把账号重新放回可调度候选集。
		updates["schedulable"] = false
	}
	return model.UpdatePoolAccountErrorState(account.Id, updates)
}

func markPoolAccountCheckFailed(account *model.PoolAccount, checkedAt int64, err error) {
	if account == nil || err == nil {
		return
	}
	reason := common.MaskSensitiveInfo(err.Error())
	nextRetry := checkedAt + int64(accountPoolCheckRetryDelay.Seconds())
	recentRequests := accountauth.RecordRecentRequest(account.RecentRequests, time.Unix(checkedAt, 0), false)
	model.RecordPoolAccountRequest(account.Id, false, recentRequests)
	updates := map[string]interface{}{
		"last_checked_time": checkedAt,
		"unavailable":       true,
		"status_message":    reason,
		"last_error":        reason,
		"next_retry_time":   nextRetry,
		"disabled_reason":   reason,
	}
	if updateErr := model.UpdatePoolAccountErrorState(account.Id, updates); updateErr != nil {
		common.SysLog(fmt.Sprintf("failed to update pool account check state: account_id=%d, error=%v", account.Id, updateErr))
	}
}

func accountPoolCheckErrorResult(account *model.PoolAccount, err error) *AccountPoolCheckResult {
	checkedAt := common.GetTimestamp()
	result := &AccountPoolCheckResult{
		Checked:   false,
		Success:   false,
		CheckedAt: checkedAt,
		Message:   common.MaskSensitiveInfo(err.Error()),
	}
	if account != nil {
		result.AccountID = account.Id
		result.AccountName = account.Name
		result.PoolGroupID = account.PoolGroupId
		result.Provider = account.GetCredentialProvider()
	}
	return result
}

func normalizePoolAccountCheckIDs(accountIDs []int) []int {
	if len(accountIDs) == 0 {
		return nil
	}
	seen := map[int]bool{}
	result := make([]int, 0, len(accountIDs))
	for _, accountID := range accountIDs {
		if accountID <= 0 || seen[accountID] {
			continue
		}
		seen[accountID] = true
		result = append(result, accountID)
	}
	return result
}

func joinAccountPoolCheckTaskIDs(accountIDs []int) string {
	if len(accountIDs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(accountIDs))
	for _, accountID := range accountIDs {
		if accountID > 0 {
			parts = append(parts, strconv.Itoa(accountID))
		}
	}
	return strings.Join(parts, ",")
}

func splitAccountPoolCheckTaskIDs(value string) []int {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '\n' || r == ';'
	})
	ids := make([]int, 0, len(parts))
	seen := map[int]bool{}
	for _, part := range parts {
		id, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids
}

func normalizePoolAccountCheckTaskStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case model.PoolAccountCheckTaskStatusQueued:
		return model.PoolAccountCheckTaskStatusQueued
	case model.PoolAccountCheckTaskStatusRunning:
		return model.PoolAccountCheckTaskStatusRunning
	case model.PoolAccountCheckTaskStatusCompleted:
		return model.PoolAccountCheckTaskStatusCompleted
	case model.PoolAccountCheckTaskStatusFailed:
		return model.PoolAccountCheckTaskStatusFailed
	default:
		return ""
	}
}

func normalizePoolAccountCheckTaskCleanupStatuses(statuses []string) []string {
	if len(statuses) == 0 {
		return nil
	}
	seen := map[string]bool{}
	result := make([]string, 0, len(statuses))
	for _, status := range statuses {
		normalized := normalizePoolAccountCheckTaskStatus(status)
		if normalized == "" || seen[normalized] {
			continue
		}
		if normalized != model.PoolAccountCheckTaskStatusCompleted && normalized != model.PoolAccountCheckTaskStatusFailed {
			continue
		}
		seen[normalized] = true
		result = append(result, normalized)
	}
	return result
}

func parsePositiveInt(value string) int {
	id, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || id <= 0 {
		return 0
	}
	return id
}
