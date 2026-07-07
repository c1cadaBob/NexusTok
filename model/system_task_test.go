package model

import (
	"errors"
	"testing"

	"github.com/c1cada/NexusTok/common"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type testSystemTaskPayload struct {
	TargetTimestamp int64 `json:"target_timestamp"`
	BatchSize       int   `json:"batch_size"`
}

type testSystemTaskState struct {
	Processed int `json:"processed"`
	Total     int `json:"total"`
}

func setupSystemTaskTestDB(t *testing.T) {
	t.Helper()
	originDB := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&SystemTask{}, &SystemTaskLock{}))
	DB = db
	t.Cleanup(func() { DB = originDB })
}

func createLegacyPendingSystemTask(t *testing.T, taskType string) *SystemTask {
	t.Helper()
	taskID, err := GenerateSystemTaskID()
	require.NoError(t, err)
	task := &SystemTask{
		TaskID: taskID,
		Type:   taskType,
		Status: SystemTaskStatusPending,
	}
	require.NoError(t, DB.Create(task).Error)
	return task
}

func TestSystemTaskCreateDecodeAndActiveLifecycle(t *testing.T) {
	setupSystemTaskTestDB(t)

	payload := testSystemTaskPayload{TargetTimestamp: 1000, BatchSize: 100}
	state := testSystemTaskState{Processed: 1, Total: 10}
	task, err := CreateSystemTask(SystemTaskTypeLogCleanup, payload, state)
	require.NoError(t, err)
	require.NotNil(t, task.ActiveKey)
	require.Equal(t, SystemTaskTypeLogCleanup, *task.ActiveKey)

	var decodedPayload testSystemTaskPayload
	require.NoError(t, task.DecodePayload(&decodedPayload))
	require.Equal(t, payload, decodedPayload)

	var decodedState testSystemTaskState
	require.NoError(t, task.DecodeState(&decodedState))
	require.Equal(t, state, decodedState)

	activeTask, err := GetActiveSystemTask(SystemTaskTypeLogCleanup)
	require.NoError(t, err)
	require.NotNil(t, activeTask)
	require.Equal(t, task.TaskID, activeTask.TaskID)

	_, err = CreateSystemTask(SystemTaskTypeLogCleanup, payload, state)
	require.Error(t, err)

	claimedTask, claimed, err := ClaimSystemTask(task.ID, SystemTaskTypeLogCleanup, "runner-a", common.GetTimestamp()+60)
	require.NoError(t, err)
	require.True(t, claimed)
	require.Equal(t, SystemTaskStatusRunning, claimedTask.Status)

	require.NoError(t, UpdateSystemTaskState(task.TaskID, "runner-a", testSystemTaskState{Processed: 10, Total: 10}))
	require.NoError(t, FinishSystemTask(task.TaskID, "runner-a", SystemTaskStatusSucceeded, map[string]int64{"deleted_count": 0}, ""))

	finishedTask, err := GetSystemTaskByTaskID(task.TaskID)
	require.NoError(t, err)
	require.NotNil(t, finishedTask)
	require.Equal(t, SystemTaskStatusSucceeded, finishedTask.Status)
	require.Nil(t, finishedTask.ActiveKey)
	require.Equal(t, "runner-a", finishedTask.LockedBy)

	var result map[string]int64
	require.NoError(t, finishedTask.DecodeResult(&result))
	require.Equal(t, int64(0), result["deleted_count"])

	activeTask, err = GetActiveSystemTask(SystemTaskTypeLogCleanup)
	require.NoError(t, err)
	require.Nil(t, activeTask)

	nextTask, err := CreateSystemTask(SystemTaskTypeLogCleanup, nil, nil)
	require.NoError(t, err)
	require.NotEmpty(t, nextTask.TaskID)
}

func TestSystemTaskExpiredLockCanBeReclaimed(t *testing.T) {
	setupSystemTaskTestDB(t)

	first, err := CreateSystemTask(SystemTaskTypeChannelTest, nil, nil)
	require.NoError(t, err)
	_, claimed, err := ClaimSystemTask(first.ID, SystemTaskTypeChannelTest, "runner-a", common.GetTimestamp()+60)
	require.NoError(t, err)
	require.True(t, claimed)

	require.NoError(t, DB.Model(&SystemTaskLock{}).
		Where("type = ?", SystemTaskTypeChannelTest).
		Update("locked_until", common.GetTimestamp()-1).Error)

	second := createLegacyPendingSystemTask(t, SystemTaskTypeChannelTest)
	claimedTask, claimed, err := ClaimSystemTask(second.ID, SystemTaskTypeChannelTest, "runner-b", common.GetTimestamp()+60)
	require.NoError(t, err)
	require.True(t, claimed)
	require.Equal(t, second.TaskID, claimedTask.TaskID)

	reloadedFirst, err := GetSystemTaskByTaskID(first.TaskID)
	require.NoError(t, err)
	require.Equal(t, SystemTaskStatusFailed, reloadedFirst.Status)
	require.Nil(t, reloadedFirst.ActiveKey)
	require.Equal(t, "task lease expired", reloadedFirst.Error)
}

func TestSystemTaskLockPreventsConcurrentClaim(t *testing.T) {
	setupSystemTaskTestDB(t)

	first, err := CreateSystemTask(SystemTaskTypeModelUpdate, nil, nil)
	require.NoError(t, err)
	_, claimed, err := ClaimSystemTask(first.ID, SystemTaskTypeModelUpdate, "runner-a", common.GetTimestamp()+60)
	require.NoError(t, err)
	require.True(t, claimed)

	second := createLegacyPendingSystemTask(t, SystemTaskTypeModelUpdate)
	_, claimed, err = ClaimSystemTask(second.ID, SystemTaskTypeModelUpdate, "runner-b", common.GetTimestamp()+60)
	require.NoError(t, err)
	require.False(t, claimed)

	reloadedSecond, err := GetSystemTaskByTaskID(second.TaskID)
	require.NoError(t, err)
	require.Equal(t, SystemTaskStatusPending, reloadedSecond.Status)
}

func TestExpireStaleSystemTaskLocksFailsOldRun(t *testing.T) {
	setupSystemTaskTestDB(t)

	task, err := CreateSystemTask(SystemTaskTypeAsyncTaskPoll, nil, nil)
	require.NoError(t, err)
	_, claimed, err := ClaimSystemTask(task.ID, SystemTaskTypeAsyncTaskPoll, "runner-a", common.GetTimestamp()+60)
	require.NoError(t, err)
	require.True(t, claimed)

	require.NoError(t, DB.Model(&SystemTaskLock{}).
		Where("type = ?", SystemTaskTypeAsyncTaskPoll).
		Update("locked_until", common.GetTimestamp()-1).Error)

	require.NoError(t, ExpireStaleSystemTaskLocks(common.GetTimestamp()))

	reloaded, err := GetSystemTaskByTaskID(task.TaskID)
	require.NoError(t, err)
	require.Equal(t, SystemTaskStatusFailed, reloaded.Status)
	require.Nil(t, reloaded.ActiveKey)

	var lockCount int64
	require.NoError(t, DB.Model(&SystemTaskLock{}).Where("task_id = ?", task.TaskID).Count(&lockCount).Error)
	require.Equal(t, int64(0), lockCount)
}

func TestSystemTaskListLimitAndLatestQueries(t *testing.T) {
	setupSystemTaskTestDB(t)

	require.NoError(t, DB.Create(&SystemTask{
		TaskID: "manual-old",
		Type:   "type_a",
		Status: SystemTaskStatusSucceeded,
	}).Error)
	for i := 0; i < 105; i++ {
		_, err := CreateSystemTask("task_limit_"+common.GetUUID(), nil, nil)
		require.NoError(t, err)
	}

	tasks, err := ListSystemTasks(500)
	require.NoError(t, err)
	require.Len(t, tasks, 100)

	latest, err := GetLatestSystemTask("type_a")
	require.NoError(t, err)
	require.NotNil(t, latest)
	require.Equal(t, "manual-old", latest.TaskID)

	latestByType, err := GetLatestSystemTasks([]string{"type_a", "missing"})
	require.NoError(t, err)
	require.Len(t, latestByType, 1)
	require.Equal(t, "manual-old", latestByType["type_a"].TaskID)

	pendingByType, err := FindEarliestPendingSystemTasks([]string{"type_a", "missing"})
	require.NoError(t, err)
	require.Empty(t, pendingByType)
}

func TestSystemTaskResponseKeepsInvalidJSON(t *testing.T) {
	task := &SystemTask{
		TaskID:  "invalid-json",
		Type:    SystemTaskTypeLogCleanup,
		Status:  SystemTaskStatusFailed,
		Payload: "{invalid",
		State:   "not-json",
		Result:  "",
	}

	response := task.ToResponse()

	require.Equal(t, "{invalid", response.Payload)
	require.Equal(t, "not-json", response.State)
	require.Nil(t, response.Result)
}

func TestSystemTaskLockLostWhenLeaseMissing(t *testing.T) {
	setupSystemTaskTestDB(t)

	task, err := CreateSystemTask(SystemTaskTypeMidjourneyPoll, nil, nil)
	require.NoError(t, err)
	_, claimed, err := ClaimSystemTask(task.ID, SystemTaskTypeMidjourneyPoll, "runner-a", common.GetTimestamp()+60)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, ReleaseSystemTaskLock(task.TaskID, "runner-a"))

	err = UpdateSystemTaskState(task.TaskID, "runner-a", map[string]int{"processed": 1})
	require.True(t, errors.Is(err, ErrSystemTaskLockLost))

	err = FinishSystemTask(task.TaskID, "runner-a", SystemTaskStatusSucceeded, nil, "")
	require.True(t, errors.Is(err, ErrSystemTaskLockLost))
}
