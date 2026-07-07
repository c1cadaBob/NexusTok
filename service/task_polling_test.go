package service

import (
	"context"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAsyncTaskPollingTest(t *testing.T) {
	t.Helper()
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldTaskQueryLimit := constant.TaskQueryLimit
	oldTaskTimeoutMinutes := constant.TaskTimeoutMinutes
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Task{},
		&model.User{},
		&model.Log{},
		&model.SystemTask{},
		&model.SystemTaskLock{},
	))
	model.DB = db
	model.LOG_DB = db
	constant.TaskQueryLimit = 1000
	constant.TaskTimeoutMinutes = 1440
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		constant.TaskQueryLimit = oldTaskQueryLimit
		constant.TaskTimeoutMinutes = oldTaskTimeoutMinutes
	})
}

func TestRunAsyncTaskPollingOnceFixesNullUpstreamTask(t *testing.T) {
	setupAsyncTaskPollingTest(t)

	task := &model.Task{
		UserId:     901,
		Platform:   constant.TaskPlatformSuno,
		Status:     model.TaskStatusSubmitted,
		Progress:   "0%",
		SubmitTime: common.GetTimestamp(),
	}
	require.NoError(t, model.DB.Create(task).Error)

	summary, err := RunAsyncTaskPollingOnce(context.Background(), nil)
	require.NoError(t, err)
	require.Equal(t, 1, summary.Pending)
	require.Equal(t, 1, summary.NullTaskIDs)
	require.Equal(t, 0, summary.DispatchCount)

	var reloaded model.Task
	require.NoError(t, model.DB.Where("id = ?", task.ID).First(&reloaded).Error)
	require.Equal(t, model.TaskStatus(model.TaskStatusFailure), reloaded.Status)
	require.Equal(t, "100%", reloaded.Progress)
}

func TestAsyncTaskPollHandlerFinishesSystemTaskOnEmptyQueue(t *testing.T) {
	setupAsyncTaskPollingTest(t)

	task, err := model.CreateSystemTask(model.SystemTaskTypeAsyncTaskPoll, nil, nil)
	require.NoError(t, err)
	claimedTask, claimed, err := model.ClaimSystemTask(task.ID, model.SystemTaskTypeAsyncTaskPoll, "runner-async-task-poll", common.GetTimestamp()+60)
	require.NoError(t, err)
	require.True(t, claimed)

	asyncTaskPollHandler{}.Run(context.Background(), claimedTask, "runner-async-task-poll")

	finished, err := model.GetSystemTaskByTaskID(task.TaskID)
	require.NoError(t, err)
	require.NotNil(t, finished)
	require.Equal(t, model.SystemTaskStatusSucceeded, finished.Status)
	require.Nil(t, finished.ActiveKey)

	var state SystemTaskProgress
	require.NoError(t, finished.DecodeState(&state))
	require.Equal(t, 100, state.Progress)

	var result AsyncTaskPollSummary
	require.NoError(t, finished.DecodeResult(&result))
	require.Equal(t, 0, result.Pending)
	require.Equal(t, 0, result.DispatchCount)
}

func TestTaskPollingLoopQueuesSystemTask(t *testing.T) {
	setupAsyncTaskPollingTest(t)
	oldMaster := common.IsMasterNode
	oldUpdateTask := constant.UpdateTask
	common.IsMasterNode = true
	constant.UpdateTask = true
	t.Cleanup(func() {
		common.IsMasterNode = oldMaster
		constant.UpdateTask = oldUpdateTask
	})

	TaskPollingLoop()

	task, err := model.GetActiveSystemTask(model.SystemTaskTypeAsyncTaskPoll)
	require.NoError(t, err)
	require.NotNil(t, task)
	require.Equal(t, model.SystemTaskStatusPending, task.Status)
}
