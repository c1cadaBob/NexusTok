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
	accountPoolCheckTimeout       = 30 * time.Second
	accountPoolCheckBatchLimit    = 100
	accountPoolCheckRetryDelay    = 5 * time.Minute
	accountPoolCheckTaskQueueSize = 100
)

var (
	accountPoolCheckTaskQueue      = make(chan int, accountPoolCheckTaskQueueSize)
	accountPoolCheckTaskWorkerOnce sync.Once
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
		if _, err := model.GetAccountPoolGroupById(groupID); err != nil {
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
	if _, err := model.GetAccountPoolGroupById(groupID); err != nil {
		return nil, err
	}
	var accounts []*model.PoolAccount
	if err := model.DB.Where("pool_group_id = ?", groupID).Order("id ASC").Limit(limit).Find(&accounts).Error; err != nil {
		return nil, err
	}
	return checkPoolAccountList(ctx, accounts), nil
}

// StartPoolAccountCheckTask 创建后台检测任务并立即启动 worker。
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
	if err := enqueuePoolAccountCheckTask(task.Id); err != nil {
		return nil, err
	}
	return PoolAccountCheckTaskPublicView(task), nil
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

func runPoolAccountCheckTask(taskID int) {
	if taskID <= 0 {
		return
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			common.SysLog(fmt.Sprintf("pool account check task panic: task_id=%d, error=%v", taskID, recovered))
			failPoolAccountCheckTask(taskID, fmt.Sprintf("account check task failed: %v", recovered))
		}
	}()
	startedAt := common.GetTimestamp()
	if err := model.DB.Model(&model.PoolAccountCheckTask{}).
		Where("id = ?", taskID).
		Updates(map[string]interface{}{
			"status":       model.PoolAccountCheckTaskStatusRunning,
			"started_time": startedAt,
			"message":      "account check task is running",
		}).Error; err != nil {
		common.SysLog(fmt.Sprintf("failed to start pool account check task: task_id=%d, error=%v", taskID, err))
		failPoolAccountCheckTask(taskID, fmt.Sprintf("account check task failed to start: %v", err))
		return
	}
	task := &model.PoolAccountCheckTask{}
	if err := model.DB.Where("id = ?", taskID).First(task).Error; err != nil {
		common.SysLog(fmt.Sprintf("failed to load pool account check task: task_id=%d, error=%v", taskID, err))
		failPoolAccountCheckTask(taskID, fmt.Sprintf("account check task failed to load: %v", err))
		return
	}
	accountIDs := splitAccountPoolCheckTaskIDs(task.AccountIds)
	result := &AccountPoolBatchCheckResult{
		Total: len(accountIDs),
		Items: make([]*AccountPoolCheckResult, 0, len(accountIDs)),
	}
	for _, accountID := range accountIDs {
		item := runPoolAccountCheckTaskItem(task.PoolGroupId, accountID)
		result.Items = append(result.Items, item)
		accumulatePoolAccountCheckTaskResult(result, item)
		recordPoolAccountCheckTaskState(task, item)
		if err := updatePoolAccountCheckTaskProgress(taskID, model.PoolAccountCheckTaskStatusRunning, result, "account check task is running", 0); err != nil {
			common.SysLog(fmt.Sprintf("failed to update pool account check task progress: task_id=%d, error=%v", taskID, err))
		}
	}
	message := "account check task completed"
	if result.Failed > 0 {
		message = "account check task completed with failed accounts"
	}
	if err := updatePoolAccountCheckTaskProgress(taskID, model.PoolAccountCheckTaskStatusCompleted, result, message, common.GetTimestamp()); err != nil {
		common.SysLog(fmt.Sprintf("failed to finish pool account check task: task_id=%d, error=%v", taskID, err))
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

func runPoolAccountCheckTaskItem(groupID int, accountID int) *AccountPoolCheckResult {
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
	item, err := checkPoolAccount(context.Background(), account)
	if err != nil {
		item = accountPoolCheckErrorResult(account, err)
	}
	return item
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

func enqueuePoolAccountCheckTask(taskID int) error {
	if taskID <= 0 {
		return fmt.Errorf("check task id is required")
	}
	accountPoolCheckTaskWorkerOnce.Do(func() {
		go runPoolAccountCheckTaskWorker()
	})
	select {
	case accountPoolCheckTaskQueue <- taskID:
		return nil
	default:
		message := "account check task queue is full"
		_ = model.DB.Model(&model.PoolAccountCheckTask{}).
			Where("id = ?", taskID).
			Updates(map[string]interface{}{
				"status":        model.PoolAccountCheckTaskStatusFailed,
				"message":       message,
				"finished_time": common.GetTimestamp(),
			}).Error
		return fmt.Errorf("%s", message)
	}
}

func runPoolAccountCheckTaskWorker() {
	for taskID := range accountPoolCheckTaskQueue {
		runPoolAccountCheckTask(taskID)
	}
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
	if ok && account.AuthType == model.AccountPoolAuthTypeOfficialOAuth && provider.RefreshLead() != nil {
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
