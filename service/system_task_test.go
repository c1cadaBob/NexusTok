package service

import (
	"context"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupSystemTaskServiceTestDB(t *testing.T) {
	t.Helper()
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.SystemTask{}, &model.SystemTaskLock{}))
	model.DB = db
	model.LOG_DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
	})
}

func TestStartLogCleanupTaskRunsThroughSystemTask(t *testing.T) {
	setupSystemTaskServiceTestDB(t)
	require.NoError(t, model.LOG_DB.Create(&[]model.Log{
		{CreatedAt: 80, Type: model.LogTypeManage, Content: "old-a"},
		{CreatedAt: 90, Type: model.LogTypeManage, Content: "old-b"},
		{CreatedAt: 110, Type: model.LogTypeManage, Content: "new"},
	}).Error)

	task, err := StartLogCleanupTask(100)
	require.NoError(t, err)
	require.Equal(t, model.SystemTaskTypeLogCleanup, task.Type)

	claimedTask, claimed, err := model.ClaimSystemTask(task.ID, model.SystemTaskTypeLogCleanup, "runner-test", common.GetTimestamp()+60)
	require.NoError(t, err)
	require.True(t, claimed)

	runLogCleanupTask(context.Background(), claimedTask, "runner-test")

	finished, err := model.GetSystemTaskByTaskID(task.TaskID)
	require.NoError(t, err)
	require.NotNil(t, finished)
	require.Equal(t, model.SystemTaskStatusSucceeded, finished.Status)
	require.Nil(t, finished.ActiveKey)

	var state LogCleanupState
	require.NoError(t, finished.DecodeState(&state))
	require.Equal(t, int64(2), state.Total)
	require.Equal(t, int64(2), state.Processed)
	require.Equal(t, int64(0), state.Remaining)
	require.Equal(t, 100, state.Progress)

	var result LogCleanupResult
	require.NoError(t, finished.DecodeResult(&result))
	require.Equal(t, int64(2), result.DeletedCount)

	var logs []model.Log
	require.NoError(t, model.LOG_DB.Order("created_at asc").Find(&logs).Error)
	require.Len(t, logs, 1)
	require.Equal(t, int64(110), logs[0].CreatedAt)
}

func TestStartLogCleanupTaskReturnsActiveTask(t *testing.T) {
	setupSystemTaskServiceTestDB(t)

	first, err := StartLogCleanupTask(100)
	require.NoError(t, err)
	second, err := StartLogCleanupTask(200)
	require.NoError(t, err)

	require.Equal(t, first.TaskID, second.TaskID)
}
