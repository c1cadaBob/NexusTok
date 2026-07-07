// Package model - system_task.go
// 该文件定义系统任务的持久化模型。SystemTask 用于承载日志清理、渠道测试、
// 模型更新、异步任务轮询等后台运维动作，让这些动作可以被 Root 管理员观察、
// 防重复触发，并在多节点部署中通过租约锁避免并发执行同一类任务。
package model

import (
	"errors"

	"github.com/c1cada/NexusTok/common"

	"gorm.io/gorm"
)

// SystemTaskStatus 表示系统后台任务的生命周期状态。
type SystemTaskStatus string

const (
	SystemTaskStatusPending   SystemTaskStatus = "pending"
	SystemTaskStatusRunning   SystemTaskStatus = "running"
	SystemTaskStatusSucceeded SystemTaskStatus = "succeeded"
	SystemTaskStatusFailed    SystemTaskStatus = "failed"

	SystemTaskTypeLogCleanup       = "log_cleanup"
	SystemTaskTypeChannelTest      = "channel_test"
	SystemTaskTypeModelUpdate      = "model_update"
	SystemTaskTypeMidjourneyPoll   = "midjourney_poll"
	SystemTaskTypeAsyncTaskPoll    = "async_task_poll"
	SystemTaskTypeAccountPoolCheck = "account_pool_check"
)

var (
	ErrSystemTaskLockLost          = errors.New("system task lock lost")
	errSystemTaskTypeRequired      = errors.New("system task type is required")
	errSystemTaskActiveKeyTooLong  = errors.New("system task active key is too long")
	errSystemTaskTerminalStatus    = errors.New("system task finish status must be succeeded or failed")
	errSystemTaskRunnerIDRequired  = errors.New("system task runner id is required")
	errSystemTaskLockUntilRequired = errors.New("system task lock_until is required")
)

// SystemTask 保存一条可观察的后台任务记录。
//
// Payload、State、Result 使用 TEXT 存放 JSON 字符串，避免 JSONB/JSON 列类型导致
// SQLite、MySQL 5.7 和 PostgreSQL 的迁移差异。ActiveKey 在 pending/running 阶段
// 保存任务类型，并在终态清空；唯一索引用于从数据库层面阻止同类型任务重复活跃。
type SystemTask struct {
	ID        int64            `json:"id" gorm:"primaryKey"`
	TaskID    string           `json:"task_id" gorm:"type:varchar(64);uniqueIndex"`
	Type      string           `json:"type" gorm:"type:varchar(64);index"`
	Status    SystemTaskStatus `json:"status" gorm:"type:varchar(32);index"`
	ActiveKey *string          `json:"active_key,omitempty" gorm:"type:varchar(64);uniqueIndex"`
	Payload   string           `json:"payload" gorm:"type:text"`
	State     string           `json:"state" gorm:"type:text"`
	Result    string           `json:"result" gorm:"type:text"`
	Error     string           `json:"error" gorm:"type:text"`
	LockedBy  string           `json:"locked_by" gorm:"type:varchar(128);index"`
	CreatedAt int64            `json:"created_at" gorm:"bigint;index"`
	UpdatedAt int64            `json:"updated_at" gorm:"bigint;index"`
}

// SystemTaskLock 保存某一类系统任务当前的执行租约。
//
// Type 是主键，表示同一任务类型在任意时刻最多只有一个有效租约。LockedUntil 使用
// Unix 秒时间戳，过期后新 runner 可以抢占并将旧任务标记为失败，避免节点宕机后任务
// 永久停留在 running。
type SystemTaskLock struct {
	Type        string `json:"type" gorm:"type:varchar(64);primaryKey"`
	TaskID      string `json:"task_id" gorm:"type:varchar(64);index"`
	LockedBy    string `json:"locked_by" gorm:"type:varchar(128);index"`
	LockedUntil int64  `json:"locked_until" gorm:"bigint;index"`
	UpdatedAt   int64  `json:"updated_at" gorm:"bigint;index"`
}

// SystemTaskResponse 是前端和外部 API 使用的系统任务视图。
type SystemTaskResponse struct {
	ID        int64            `json:"id"`
	TaskID    string           `json:"task_id"`
	Type      string           `json:"type"`
	Status    SystemTaskStatus `json:"status"`
	ActiveKey *string          `json:"active_key,omitempty"`
	Payload   any              `json:"payload"`
	State     any              `json:"state"`
	Result    any              `json:"result"`
	Error     string           `json:"error"`
	LockedBy  string           `json:"locked_by"`
	CreatedAt int64            `json:"created_at"`
	UpdatedAt int64            `json:"updated_at"`
}

// BeforeCreate 填充系统任务创建和更新时间戳。
func (task *SystemTask) BeforeCreate(_ *gorm.DB) error {
	now := common.GetTimestamp()
	if task.CreatedAt == 0 {
		task.CreatedAt = now
	}
	if task.UpdatedAt == 0 {
		task.UpdatedAt = now
	}
	return nil
}

// BeforeCreate 填充系统任务租约更新时间戳。
func (lock *SystemTaskLock) BeforeCreate(_ *gorm.DB) error {
	if lock.UpdatedAt == 0 {
		lock.UpdatedAt = common.GetTimestamp()
	}
	return nil
}

// GenerateSystemTaskID 生成外部可见的系统任务 ID。
func GenerateSystemTaskID() (string, error) {
	key, err := common.GenerateRandomCharsKey(32)
	if err != nil {
		return "", err
	}
	return "systask_" + key, nil
}

// CreateSystemTask 创建一条 pending 系统任务。
//
// 同类型任务处于 pending/running 时会占用 ActiveKey，数据库唯一索引会阻止重复创建。
// 调用方应在捕获唯一索引错误后查询 GetActiveSystemTask，以便返回已存在任务。
func CreateSystemTask(taskType string, payload any, state any) (*SystemTask, error) {
	return CreateSystemTaskWithActiveKey(taskType, taskType, payload, state)
}

// CreateSystemTaskWithActiveKey 创建一条 pending 系统任务，并使用调用方指定的 ActiveKey。
//
// ActiveKey 只负责 pending/running 阶段的去重范围；任务执行租约仍按 Type 维度获取。
// 这让账号池检测这类“允许多条同类型任务排队、但同一时刻只能执行一条”的场景可以
// 为每个业务任务使用独立 ActiveKey，同时继续复用 SystemTaskLock 的跨节点互斥能力。
func CreateSystemTaskWithActiveKey(taskType string, activeKey string, payload any, state any) (*SystemTask, error) {
	if taskType == "" {
		return nil, errSystemTaskTypeRequired
	}
	if activeKey == "" {
		activeKey = taskType
	}
	if len(activeKey) > 64 {
		return nil, errSystemTaskActiveKeyTooLong
	}
	taskID, err := GenerateSystemTaskID()
	if err != nil {
		return nil, err
	}
	payloadText, err := marshalSystemTaskJSON(payload)
	if err != nil {
		return nil, err
	}
	stateText, err := marshalSystemTaskJSON(state)
	if err != nil {
		return nil, err
	}

	task := &SystemTask{
		TaskID:    taskID,
		Type:      taskType,
		Status:    SystemTaskStatusPending,
		ActiveKey: &activeKey,
		Payload:   payloadText,
		State:     stateText,
	}
	if err := DB.Create(task).Error; err != nil {
		return nil, err
	}
	return task, nil
}

// GetSystemTaskByTaskID 根据外部任务 ID 查询任务，不存在时返回 nil。
func GetSystemTaskByTaskID(taskID string) (*SystemTask, error) {
	var task SystemTask
	if err := DB.Where("task_id = ?", taskID).First(&task).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &task, nil
}

// GetActiveSystemTask 查询某一类型最近一条 pending/running 任务。
func GetActiveSystemTask(taskType string) (*SystemTask, error) {
	var task SystemTask
	err := DB.Where("type = ? AND status IN ?", taskType, activeSystemTaskStatuses()).
		Order("id desc").
		First(&task).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &task, nil
}

// GetActiveSystemTaskByActiveKey 查询指定 ActiveKey 当前对应的 pending/running 任务。
//
// 该入口用于业务任务和 SystemTask 之间的一对一绑定，例如账号池检测任务重启恢复时，
// 如果旧的系统任务记录仍处于活动状态，就直接复用它而不是创建重复执行入口。
func GetActiveSystemTaskByActiveKey(activeKey string) (*SystemTask, error) {
	if activeKey == "" {
		return nil, nil
	}
	var task SystemTask
	err := DB.Where("active_key = ? AND status IN ?", activeKey, activeSystemTaskStatuses()).
		Order("id desc").
		First(&task).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &task, nil
}

// FindPendingSystemTasks 按创建顺序查询某一类型等待执行的任务。
func FindPendingSystemTasks(taskType string, limit int) ([]*SystemTask, error) {
	if limit <= 0 {
		limit = 1
	}
	var tasks []*SystemTask
	err := DB.Where("type = ? AND status = ?", taskType, SystemTaskStatusPending).
		Order("id asc").
		Limit(limit).
		Find(&tasks).Error
	return tasks, err
}

// FindEarliestPendingSystemTasks 查询每个任务类型最早的一条 pending 任务。
func FindEarliestPendingSystemTasks(taskTypes []string) (map[string]*SystemTask, error) {
	tasksByType := map[string]*SystemTask{}
	if len(taskTypes) == 0 {
		return tasksByType, nil
	}

	subQuery := DB.Model(&SystemTask{}).
		Select("MIN(id)").
		Where("type IN ? AND status = ?", taskTypes, SystemTaskStatusPending).
		Group("type")
	var tasks []*SystemTask
	if err := DB.Where("id IN (?)", subQuery).Find(&tasks).Error; err != nil {
		return nil, err
	}
	for _, task := range tasks {
		tasksByType[task.Type] = task
	}
	return tasksByType, nil
}

// ListSystemTasks 按创建时间倒序列出系统任务，限制最大返回数量，避免 Root 页面误取大量历史。
func ListSystemTasks(limit int) ([]*SystemTask, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	var tasks []*SystemTask
	err := DB.Order("id desc").Limit(limit).Find(&tasks).Error
	return tasks, err
}

// GetLatestSystemTask 查询某一类型最近一条任意状态任务，不存在时返回 nil。
func GetLatestSystemTask(taskType string) (*SystemTask, error) {
	var task SystemTask
	err := DB.Where("type = ?", taskType).Order("id desc").First(&task).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &task, nil
}

// GetLatestSystemTasks 查询每个任务类型最近一条任意状态任务。
func GetLatestSystemTasks(taskTypes []string) (map[string]*SystemTask, error) {
	tasksByType := map[string]*SystemTask{}
	if len(taskTypes) == 0 {
		return tasksByType, nil
	}

	subQuery := DB.Model(&SystemTask{}).
		Select("MAX(id)").
		Where("type IN ?", taskTypes).
		Group("type")
	var tasks []*SystemTask
	if err := DB.Where("id IN (?)", subQuery).Find(&tasks).Error; err != nil {
		return nil, err
	}
	for _, task := range tasks {
		tasksByType[task.Type] = task
	}
	return tasksByType, nil
}

// ClaimSystemTask 尝试把 pending 任务切换为 running，并获取对应类型的执行租约。
//
// 返回 claimed=false 表示任务已被其他 runner 抢先处理，调用方可以安全跳过。若发现同类型
// 旧租约已过期，会先将旧 running 任务标记为 failed，再把租约转移给当前任务。
func ClaimSystemTask(id int64, taskType string, runnerID string, lockUntil int64) (*SystemTask, bool, error) {
	if runnerID == "" {
		return nil, false, errSystemTaskRunnerIDRequired
	}
	if lockUntil == 0 {
		return nil, false, errSystemTaskLockUntilRequired
	}
	now := common.GetTimestamp()
	var task SystemTask
	if err := DB.Where("id = ? AND type = ? AND status = ?", id, taskType, SystemTaskStatusPending).First(&task).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}

	acquired, expiredTaskID, err := acquireSystemTaskLock(taskType, task.TaskID, runnerID, now, lockUntil)
	if err != nil || !acquired {
		return nil, acquired, err
	}
	if expiredTaskID != "" && expiredTaskID != task.TaskID {
		if err := MarkSystemTaskLeaseExpired(expiredTaskID); err != nil {
			_ = ReleaseSystemTaskLock(task.TaskID, runnerID)
			return nil, false, err
		}
	}

	result := DB.Model(&SystemTask{}).
		Where("id = ? AND type = ? AND status = ?", id, taskType, SystemTaskStatusPending).
		Updates(map[string]any{
			"status":     SystemTaskStatusRunning,
			"locked_by":  runnerID,
			"updated_at": now,
		})
	if result.Error != nil {
		_ = ReleaseSystemTaskLock(task.TaskID, runnerID)
		return nil, false, result.Error
	}
	if result.RowsAffected == 0 {
		_ = ReleaseSystemTaskLock(task.TaskID, runnerID)
		return nil, false, nil
	}

	if err := DB.Where("id = ?", id).First(&task).Error; err != nil {
		_ = ReleaseSystemTaskLock(task.TaskID, runnerID)
		return nil, false, err
	}
	return &task, true, nil
}

// UpdateSystemTaskState 更新 running 任务的进度状态。
//
// 更新前会确认同一 runner 仍持有未过期租约；租约丢失时返回 ErrSystemTaskLockLost，
// 防止旧节点在被抢占后继续覆盖新节点写入的进度。
func UpdateSystemTaskState(taskID string, lockedBy string, state any) error {
	stateText, err := marshalSystemTaskJSON(state)
	if err != nil {
		return err
	}
	now := common.GetTimestamp()
	result := DB.Model(&SystemTask{}).
		Where("task_id = ? AND status = ? AND locked_by = ?", taskID, SystemTaskStatusRunning, lockedBy).
		Where("EXISTS (SELECT 1 FROM system_task_locks WHERE system_task_locks.task_id = system_tasks.task_id AND system_task_locks.locked_by = ? AND system_task_locks.locked_until >= ?)", lockedBy, now).
		Updates(map[string]any{
			"state":      stateText,
			"updated_at": now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrSystemTaskLockLost
	}
	return nil
}

// RenewSystemTaskLock 延长 runner 当前持有的任务租约。
func RenewSystemTaskLock(taskID string, lockedBy string, lockUntil int64) error {
	now := common.GetTimestamp()
	result := DB.Model(&SystemTaskLock{}).
		Where("task_id = ? AND locked_by = ? AND locked_until >= ?", taskID, lockedBy, now).
		Updates(map[string]any{
			"locked_until": lockUntil,
			"updated_at":   now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrSystemTaskLockLost
	}
	return nil
}

// MarkSystemTaskLeaseExpired 将租约过期的 running 任务标记为失败并释放 ActiveKey。
func MarkSystemTaskLeaseExpired(taskID string) error {
	return DB.Model(&SystemTask{}).
		Where("task_id = ? AND status = ?", taskID, SystemTaskStatusRunning).
		Updates(map[string]any{
			"status":     SystemTaskStatusFailed,
			"active_key": nil,
			"error":      "task lease expired",
			"updated_at": common.GetTimestamp(),
		}).Error
}

// ExpireStaleSystemTaskLocks 清理所有过期租约，并把对应 running 任务置为失败。
func ExpireStaleSystemTaskLocks(now int64) error {
	var locks []*SystemTaskLock
	if err := DB.Where("locked_until < ?", now).Find(&locks).Error; err != nil {
		return err
	}
	for _, lock := range locks {
		if err := MarkSystemTaskLeaseExpired(lock.TaskID); err != nil {
			return err
		}
		result := DB.Where("type = ? AND task_id = ? AND locked_by = ? AND locked_until < ?", lock.Type, lock.TaskID, lock.LockedBy, now).
			Delete(&SystemTaskLock{})
		if result.Error != nil {
			return result.Error
		}
	}
	return nil
}

// ReleaseSystemTaskLock 释放 runner 当前持有的任务租约。
func ReleaseSystemTaskLock(taskID string, lockedBy string) error {
	return DB.Where("task_id = ? AND locked_by = ?", taskID, lockedBy).Delete(&SystemTaskLock{}).Error
}

// FinishSystemTask 将 running 任务写入终态结果，并释放 ActiveKey 与租约。
func FinishSystemTask(taskID string, lockedBy string, status SystemTaskStatus, resultPayload any, errorMessage string) error {
	if status != SystemTaskStatusSucceeded && status != SystemTaskStatusFailed {
		return errSystemTaskTerminalStatus
	}
	resultText, err := marshalSystemTaskJSON(resultPayload)
	if err != nil {
		return err
	}
	now := common.GetTimestamp()
	result := DB.Model(&SystemTask{}).
		Where("task_id = ? AND status = ? AND locked_by = ?", taskID, SystemTaskStatusRunning, lockedBy).
		Where("EXISTS (SELECT 1 FROM system_task_locks WHERE system_task_locks.task_id = system_tasks.task_id AND system_task_locks.locked_by = ? AND system_task_locks.locked_until >= ?)", lockedBy, now).
		Updates(map[string]any{
			"status":     status,
			"active_key": nil,
			"result":     resultText,
			"error":      errorMessage,
			"updated_at": now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrSystemTaskLockLost
	}
	return ReleaseSystemTaskLock(taskID, lockedBy)
}

// DecodePayload 将任务输入 JSON 解码到指定结构。
func (task *SystemTask) DecodePayload(v any) error {
	return decodeSystemTaskJSONString(task.Payload, v)
}

// DecodeState 将任务进度 JSON 解码到指定结构。
func (task *SystemTask) DecodeState(v any) error {
	return decodeSystemTaskJSONString(task.State, v)
}

// DecodeResult 将任务结果 JSON 解码到指定结构。
func (task *SystemTask) DecodeResult(v any) error {
	return decodeSystemTaskJSONString(task.Result, v)
}

// ToResponse 将数据库记录转换成 API 响应视图。
func (task *SystemTask) ToResponse() SystemTaskResponse {
	return SystemTaskResponse{
		ID:        task.ID,
		TaskID:    task.TaskID,
		Type:      task.Type,
		Status:    task.Status,
		ActiveKey: task.ActiveKey,
		Payload:   decodeSystemTaskJSONValue(task.Payload),
		State:     decodeSystemTaskJSONValue(task.State),
		Result:    decodeSystemTaskJSONValue(task.Result),
		Error:     task.Error,
		LockedBy:  task.LockedBy,
		CreatedAt: task.CreatedAt,
		UpdatedAt: task.UpdatedAt,
	}
}

func acquireSystemTaskLock(taskType string, taskID string, lockedBy string, now int64, lockUntil int64) (bool, string, error) {
	lock := &SystemTaskLock{
		Type:        taskType,
		TaskID:      taskID,
		LockedBy:    lockedBy,
		LockedUntil: lockUntil,
		UpdatedAt:   now,
	}
	if err := DB.Create(lock).Error; err == nil {
		return true, "", nil
	}

	var existing SystemTaskLock
	err := DB.Where("type = ?", taskType).First(&existing).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, "", nil
		}
		return false, "", err
	}
	if existing.LockedUntil >= now {
		return false, "", nil
	}

	result := DB.Model(&SystemTaskLock{}).
		Where("type = ? AND locked_until < ?", taskType, now).
		Updates(map[string]any{
			"task_id":      taskID,
			"locked_by":    lockedBy,
			"locked_until": lockUntil,
			"updated_at":   now,
		})
	if result.Error != nil {
		return false, "", result.Error
	}
	if result.RowsAffected == 0 {
		return false, "", nil
	}
	return true, existing.TaskID, nil
}

func activeSystemTaskStatuses() []string {
	return []string{string(SystemTaskStatusPending), string(SystemTaskStatusRunning)}
}

func marshalSystemTaskJSON(v any) (string, error) {
	if v == nil {
		return "", nil
	}
	data, err := common.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func decodeSystemTaskJSONString(data string, v any) error {
	if data == "" {
		return nil
	}
	return common.UnmarshalJsonStr(data, v)
}

func decodeSystemTaskJSONValue(data string) any {
	if data == "" {
		return nil
	}
	var value any
	if err := common.UnmarshalJsonStr(data, &value); err != nil {
		return data
	}
	return value
}
