package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/relay/channel/task/taskcommon"
	relaycommon "github.com/c1cada/NexusTok/relay/common"

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
		&model.Channel{},
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

type taskPollingSleepTestAdaptor struct{}

func (taskPollingSleepTestAdaptor) Init(_ *relaycommon.RelayInfo) {}

func (taskPollingSleepTestAdaptor) FetchTask(_ string, _ string, _ map[string]any, _ string) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"status":"SUBMITTED"}`)),
	}, nil
}

func (taskPollingSleepTestAdaptor) ParseTaskResult(_ []byte) (*relaycommon.TaskInfo, error) {
	return &relaycommon.TaskInfo{Status: string(model.TaskStatusSubmitted)}, nil
}

func (taskPollingSleepTestAdaptor) AdjustBillingOnComplete(_ *model.Task, _ *relaycommon.TaskInfo) int {
	return 0
}

func seedTaskPollingSleepChannel(t *testing.T, settings dto.ChannelOtherSettings) (*model.Channel, []string, map[string]*model.Task) {
	t.Helper()

	settingsBytes, err := common.Marshal(settings)
	require.NoError(t, err)
	baseURL := "https://upstream.example"
	channel := &model.Channel{
		Type:          constant.ChannelTypeVidu,
		Key:           "sk-test",
		Name:          "polling-sleep-test",
		BaseURL:       &baseURL,
		Models:        "vidu",
		Group:         "default",
		Status:        common.ChannelStatusEnabled,
		OtherSettings: string(settingsBytes),
	}
	require.NoError(t, model.DB.Create(channel).Error)

	taskIds := []string{"upstream-task-1", "upstream-task-2"}
	taskM := make(map[string]*model.Task, len(taskIds))
	for _, taskID := range taskIds {
		task := &model.Task{
			TaskID:    taskID,
			Platform:  constant.TaskPlatform("vidu"),
			UserId:    901,
			ChannelId: channel.Id,
			Status:    model.TaskStatusSubmitted,
			Progress:  taskcommon.ProgressSubmitted,
		}
		require.NoError(t, model.DB.Create(task).Error)
		taskM[taskID] = task
	}

	return channel, taskIds, taskM
}

func TestUpdateVideoTasksKeepsDefaultPollingSleep(t *testing.T) {
	setupAsyncTaskPollingTest(t)
	oldAdaptorFunc := GetTaskAdaptorFunc
	GetTaskAdaptorFunc = func(platform constant.TaskPlatform) TaskPollingAdaptor {
		require.Equal(t, constant.TaskPlatform("vidu"), platform)
		return taskPollingSleepTestAdaptor{}
	}
	t.Cleanup(func() {
		GetTaskAdaptorFunc = oldAdaptorFunc
	})

	channel, taskIds, taskM := seedTaskPollingSleepChannel(t, dto.ChannelOtherSettings{})

	startedAt := time.Now()
	require.NoError(t, updateVideoTasks(context.Background(), constant.TaskPlatform("vidu"), channel.Id, taskIds, taskM))

	require.GreaterOrEqual(t, time.Since(startedAt), 900*time.Millisecond)
}

func TestUpdateVideoTasksDefaultPollingSleepRespondsToContextCancel(t *testing.T) {
	setupAsyncTaskPollingTest(t)
	oldAdaptorFunc := GetTaskAdaptorFunc
	GetTaskAdaptorFunc = func(platform constant.TaskPlatform) TaskPollingAdaptor {
		require.Equal(t, constant.TaskPlatform("vidu"), platform)
		return taskPollingSleepTestAdaptor{}
	}
	t.Cleanup(func() {
		GetTaskAdaptorFunc = oldAdaptorFunc
	})

	channel, taskIds, taskM := seedTaskPollingSleepChannel(t, dto.ChannelOtherSettings{})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	startedAt := time.Now()
	err := updateVideoTasks(ctx, constant.TaskPlatform("vidu"), channel.Id, taskIds, taskM)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(startedAt), 500*time.Millisecond)
}

func TestUpdateVideoTasksCanDisablePollingSleepPerChannel(t *testing.T) {
	setupAsyncTaskPollingTest(t)
	oldAdaptorFunc := GetTaskAdaptorFunc
	GetTaskAdaptorFunc = func(platform constant.TaskPlatform) TaskPollingAdaptor {
		require.Equal(t, constant.TaskPlatform("vidu"), platform)
		return taskPollingSleepTestAdaptor{}
	}
	t.Cleanup(func() {
		GetTaskAdaptorFunc = oldAdaptorFunc
	})

	channel, taskIds, taskM := seedTaskPollingSleepChannel(t, dto.ChannelOtherSettings{
		DisableTaskPollingSleep: true,
	})

	startedAt := time.Now()
	require.NoError(t, updateVideoTasks(context.Background(), constant.TaskPlatform("vidu"), channel.Id, taskIds, taskM))

	require.Less(t, time.Since(startedAt), 500*time.Millisecond)
}
