package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
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
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
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

type taskPollingParallelTestAdaptor struct {
	mu           sync.Mutex
	taskIDs      []string
	blockTaskID  string
	blockStarted chan struct{}
	releaseBlock chan struct{}
	blockOnce    sync.Once
}

func (a *taskPollingParallelTestAdaptor) Init(_ *relaycommon.RelayInfo) {}

func (a *taskPollingParallelTestAdaptor) FetchTask(_ string, _ string, body map[string]any, _ string) (*http.Response, error) {
	taskID, _ := body["task_id"].(string)
	if taskID == a.blockTaskID && a.releaseBlock != nil {
		a.blockOnce.Do(func() {
			if a.blockStarted != nil {
				close(a.blockStarted)
			}
		})
		<-a.releaseBlock
	}

	a.mu.Lock()
	a.taskIDs = append(a.taskIDs, taskID)
	a.mu.Unlock()

	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"status":"SUBMITTED"}`)),
	}, nil
}

func (a *taskPollingParallelTestAdaptor) ParseTaskResult(_ []byte) (*relaycommon.TaskInfo, error) {
	return &relaycommon.TaskInfo{Status: string(model.TaskStatusSubmitted)}, nil
}

func (a *taskPollingParallelTestAdaptor) AdjustBillingOnComplete(_ *model.Task, _ *relaycommon.TaskInfo) int {
	return 0
}

func (a *taskPollingParallelTestAdaptor) fetchedTaskIDs() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.taskIDs...)
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

func seedTaskPollingParallelChannel(t *testing.T, channelID int, settings dto.ChannelOtherSettings) *model.Channel {
	t.Helper()

	settingsBytes, err := common.Marshal(settings)
	require.NoError(t, err)
	baseURL := "https://upstream.example"
	channel := &model.Channel{
		Id:            channelID,
		Type:          constant.ChannelTypeVidu,
		Key:           "sk-test",
		Name:          "polling-parallel-test",
		BaseURL:       &baseURL,
		Models:        "vidu",
		Group:         "default",
		Status:        common.ChannelStatusEnabled,
		OtherSettings: string(settingsBytes),
	}
	require.NoError(t, model.DB.Create(channel).Error)
	return channel
}

func seedTaskPollingParallelTask(t *testing.T, channelID int, publicTaskID string, upstreamTaskID string) *model.Task {
	t.Helper()

	task := &model.Task{
		TaskID:    publicTaskID,
		Platform:  constant.TaskPlatform("vidu"),
		UserId:    901,
		ChannelId: channelID,
		Status:    model.TaskStatusSubmitted,
		Progress:  taskcommon.ProgressSubmitted,
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: upstreamTaskID,
		},
	}
	require.NoError(t, model.DB.Create(task).Error)
	return task
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

func TestUpdateVideoTasksDefaultSleepDoesNotBlockOtherChannels(t *testing.T) {
	setupAsyncTaskPollingTest(t)
	seedTaskPollingParallelChannel(t, 201, dto.ChannelOtherSettings{})
	seedTaskPollingParallelChannel(t, 202, dto.ChannelOtherSettings{})
	firstChannelFirst := seedTaskPollingParallelTask(t, 201, "task-public-201-a", "upstream-201-a")
	firstChannelSecond := seedTaskPollingParallelTask(t, 201, "task-public-201-b", "upstream-201-b")
	secondChannelFirst := seedTaskPollingParallelTask(t, 202, "task-public-202-a", "upstream-202-a")
	secondChannelSecond := seedTaskPollingParallelTask(t, 202, "task-public-202-b", "upstream-202-b")

	adaptor := &taskPollingParallelTestAdaptor{}
	oldAdaptorFunc := GetTaskAdaptorFunc
	GetTaskAdaptorFunc = func(platform constant.TaskPlatform) TaskPollingAdaptor {
		require.Equal(t, constant.TaskPlatform("vidu"), platform)
		return adaptor
	}
	t.Cleanup(func() {
		GetTaskAdaptorFunc = oldAdaptorFunc
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := UpdateVideoTasks(ctx, constant.TaskPlatform("vidu"), map[int][]string{
		201: {
			firstChannelFirst.GetUpstreamTaskID(),
			firstChannelSecond.GetUpstreamTaskID(),
		},
		202: {
			secondChannelFirst.GetUpstreamTaskID(),
			secondChannelSecond.GetUpstreamTaskID(),
		},
	}, map[string]*model.Task{
		firstChannelFirst.GetUpstreamTaskID():   firstChannelFirst,
		firstChannelSecond.GetUpstreamTaskID():  firstChannelSecond,
		secondChannelFirst.GetUpstreamTaskID():  secondChannelFirst,
		secondChannelSecond.GetUpstreamTaskID(): secondChannelSecond,
	})

	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.ElementsMatch(t, []string{"upstream-201-a", "upstream-202-a"}, adaptor.fetchedTaskIDs())
}

func TestUpdateVideoTasksSlowChannelDoesNotBlockOtherChannels(t *testing.T) {
	setupAsyncTaskPollingTest(t)
	seedTaskPollingParallelChannel(t, 251, dto.ChannelOtherSettings{})
	seedTaskPollingParallelChannel(t, 252, dto.ChannelOtherSettings{DisableTaskPollingSleep: true})
	slowTask := seedTaskPollingParallelTask(t, 251, "task-public-slow", "upstream-slow")
	fastFirst := seedTaskPollingParallelTask(t, 252, "task-public-fast-a", "upstream-fast-a")
	fastSecond := seedTaskPollingParallelTask(t, 252, "task-public-fast-b", "upstream-fast-b")

	adaptor := &taskPollingParallelTestAdaptor{
		blockTaskID:  slowTask.GetUpstreamTaskID(),
		blockStarted: make(chan struct{}),
		releaseBlock: make(chan struct{}),
	}
	var releaseOnce sync.Once
	releaseBlockedTask := func() {
		releaseOnce.Do(func() {
			close(adaptor.releaseBlock)
		})
	}
	t.Cleanup(releaseBlockedTask)
	oldAdaptorFunc := GetTaskAdaptorFunc
	GetTaskAdaptorFunc = func(platform constant.TaskPlatform) TaskPollingAdaptor {
		require.Equal(t, constant.TaskPlatform("vidu"), platform)
		return adaptor
	}
	t.Cleanup(func() {
		GetTaskAdaptorFunc = oldAdaptorFunc
	})

	errCh := make(chan error, 1)
	go func() {
		errCh <- UpdateVideoTasks(context.Background(), constant.TaskPlatform("vidu"), map[int][]string{
			251: {slowTask.GetUpstreamTaskID()},
			252: {
				fastFirst.GetUpstreamTaskID(),
				fastSecond.GetUpstreamTaskID(),
			},
		}, map[string]*model.Task{
			slowTask.GetUpstreamTaskID():   slowTask,
			fastFirst.GetUpstreamTaskID():  fastFirst,
			fastSecond.GetUpstreamTaskID(): fastSecond,
		})
	}()

	select {
	case <-adaptor.blockStarted:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("慢渠道没有进入阻塞状态")
	}

	require.Eventually(t, func() bool {
		return len(adaptor.fetchedTaskIDs()) == 2
	}, 500*time.Millisecond, 10*time.Millisecond)
	require.ElementsMatch(t, []string{"upstream-fast-a", "upstream-fast-b"}, adaptor.fetchedTaskIDs())

	releaseBlockedTask()
	require.NoError(t, <-errCh)
	require.ElementsMatch(t, []string{"upstream-slow", "upstream-fast-a", "upstream-fast-b"}, adaptor.fetchedTaskIDs())
}

func TestUpdateVideoTasksMixedChannelSleepSettings(t *testing.T) {
	setupAsyncTaskPollingTest(t)
	seedTaskPollingParallelChannel(t, 301, dto.ChannelOtherSettings{})
	seedTaskPollingParallelChannel(t, 302, dto.ChannelOtherSettings{DisableTaskPollingSleep: true})
	sleepyFirst := seedTaskPollingParallelTask(t, 301, "task-public-sleepy-a", "upstream-sleepy-a")
	sleepySecond := seedTaskPollingParallelTask(t, 301, "task-public-sleepy-b", "upstream-sleepy-b")
	fastFirst := seedTaskPollingParallelTask(t, 302, "task-public-fast-mixed-a", "upstream-fast-mixed-a")
	fastSecond := seedTaskPollingParallelTask(t, 302, "task-public-fast-mixed-b", "upstream-fast-mixed-b")

	adaptor := &taskPollingParallelTestAdaptor{}
	oldAdaptorFunc := GetTaskAdaptorFunc
	GetTaskAdaptorFunc = func(platform constant.TaskPlatform) TaskPollingAdaptor {
		require.Equal(t, constant.TaskPlatform("vidu"), platform)
		return adaptor
	}
	t.Cleanup(func() {
		GetTaskAdaptorFunc = oldAdaptorFunc
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := UpdateVideoTasks(ctx, constant.TaskPlatform("vidu"), map[int][]string{
		301: {
			sleepyFirst.GetUpstreamTaskID(),
			sleepySecond.GetUpstreamTaskID(),
		},
		302: {
			fastFirst.GetUpstreamTaskID(),
			fastSecond.GetUpstreamTaskID(),
		},
	}, map[string]*model.Task{
		sleepyFirst.GetUpstreamTaskID():  sleepyFirst,
		sleepySecond.GetUpstreamTaskID(): sleepySecond,
		fastFirst.GetUpstreamTaskID():    fastFirst,
		fastSecond.GetUpstreamTaskID():   fastSecond,
	})

	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.ElementsMatch(t, []string{"upstream-sleepy-a", "upstream-fast-mixed-a", "upstream-fast-mixed-b"}, adaptor.fetchedTaskIDs())
}
