package controller

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

func setupMidjourneyPollingTest(t *testing.T) {
	t.Helper()
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Midjourney{},
		&model.User{},
		&model.Log{},
		&model.SystemTask{},
		&model.SystemTaskLock{},
	))
	model.DB = db
	model.LOG_DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
	})
}

func TestRunMidjourneyPollingOnceFixesNullMjID(t *testing.T) {
	setupMidjourneyPollingTest(t)

	task := &model.Midjourney{
		UserId:   701,
		Status:   "SUBMITTED",
		Progress: "0%",
	}
	require.NoError(t, model.DB.Create(task).Error)

	summary, err := RunMidjourneyPollingOnce(context.Background(), nil)
	require.NoError(t, err)
	require.Equal(t, 1, summary.Pending)
	require.Equal(t, 1, summary.NullTaskIDs)
	require.Empty(t, summary.Channels)

	var reloaded model.Midjourney
	require.NoError(t, model.DB.Where("id = ?", task.Id).First(&reloaded).Error)
	require.Equal(t, "FAILURE", reloaded.Status)
	require.Equal(t, "100%", reloaded.Progress)
}

func TestMidjourneyPollHandlerFinishesSystemTaskOnEmptyQueue(t *testing.T) {
	setupMidjourneyPollingTest(t)

	task, err := model.CreateSystemTask(model.SystemTaskTypeMidjourneyPoll, nil, nil)
	require.NoError(t, err)
	claimedTask, claimed, err := model.ClaimSystemTask(task.ID, model.SystemTaskTypeMidjourneyPoll, "runner-midjourney-poll", common.GetTimestamp()+60)
	require.NoError(t, err)
	require.True(t, claimed)

	midjourneyPollHandler{}.Run(context.Background(), claimedTask, "runner-midjourney-poll")

	finished, err := model.GetSystemTaskByTaskID(task.TaskID)
	require.NoError(t, err)
	require.NotNil(t, finished)
	require.Equal(t, model.SystemTaskStatusSucceeded, finished.Status)
	require.Nil(t, finished.ActiveKey)

	var result MidjourneyPollSummary
	require.NoError(t, finished.DecodeResult(&result))
	require.Equal(t, 0, result.Pending)
	require.Equal(t, 0, result.Updated)
}

func TestUpdateMidjourneyTaskBulkQueuesSystemTask(t *testing.T) {
	setupMidjourneyPollingTest(t)
	oldMaster := common.IsMasterNode
	oldUpdateTask := constant.UpdateTask
	common.IsMasterNode = true
	constant.UpdateTask = true
	t.Cleanup(func() {
		common.IsMasterNode = oldMaster
		constant.UpdateTask = oldUpdateTask
	})

	UpdateMidjourneyTaskBulk()

	task, err := model.GetActiveSystemTask(model.SystemTaskTypeMidjourneyPoll)
	require.NoError(t, err)
	require.NotNil(t, task)
	require.Equal(t, model.SystemTaskStatusPending, task.Status)
}
