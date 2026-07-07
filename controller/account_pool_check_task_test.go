package controller

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type accountPoolCheckTaskPageAPIResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		Page     int                                 `json:"page"`
		PageSize int                                 `json:"page_size"`
		Total    int                                 `json:"total"`
		Items    []*service.AccountPoolCheckTaskView `json:"items"`
	} `json:"data"`
}

type accountPoolCheckTaskCleanupAPIResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		Deleted int `json:"deleted"`
	} `json:"data"`
}

func newAccountPoolCheckTaskListContext(t *testing.T, rawQuery string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	target := "/api/account-pool/check-tasks"
	if rawQuery != "" {
		target += "?" + rawQuery
	}
	ctx.Request = httptest.NewRequest(http.MethodGet, target, nil)
	return ctx, recorder
}

func newAccountPoolCheckTaskCleanupContext(t *testing.T, body any) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	payload, err := common.Marshal(body)
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/account-pool/check-tasks/cleanup", bytes.NewReader(payload))
	ctx.Request.Header.Set("Content-Type", "application/json")
	return ctx, recorder
}

func decodeAccountPoolCheckTaskPageResponse(t *testing.T, recorder *httptest.ResponseRecorder) accountPoolCheckTaskPageAPIResponse {
	t.Helper()
	var response accountPoolCheckTaskPageAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

func decodeAccountPoolCheckTaskCleanupResponse(t *testing.T, recorder *httptest.ResponseRecorder) accountPoolCheckTaskCleanupAPIResponse {
	t.Helper()
	var response accountPoolCheckTaskCleanupAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

func createControllerCheckTask(t *testing.T, group *model.AccountPoolGroup, status string, message string, createdAt int64, finishedAt int64) *model.PoolAccountCheckTask {
	t.Helper()
	task := &model.PoolAccountCheckTask{
		PoolGroupId:   group.Id,
		PoolGroupName: group.Name,
		Status:        status,
		Actor:         "controller-check-task-tester",
		RequestId:     "req-" + strconv.FormatInt(createdAt, 10),
		Total:         1,
		Message:       message,
		StartedTime:   createdAt,
		FinishedTime:  finishedAt,
		CreatedTime:   createdAt,
	}
	require.NoError(t, model.DB.Create(task).Error)
	return task
}

func TestListPoolAccountCheckTasksReturnsPagedHistory(t *testing.T) {
	setupAccountPoolBatchStatusTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.PoolAccountCheckTask{}))
	group := createBatchStatusGroup(t, "controller-check-task-main")
	otherGroup := createBatchStatusGroup(t, "controller-check-task-other")
	older := createControllerCheckTask(t, group, model.PoolAccountCheckTaskStatusCompleted, "older visible task", 1000, 1010)
	newer := createControllerCheckTask(t, group, model.PoolAccountCheckTaskStatusFailed, "needle visible task", 2000, 2010)
	_ = createControllerCheckTask(t, otherGroup, model.PoolAccountCheckTaskStatusCompleted, "other group task", 3000, 3010)

	ctx, recorder := newAccountPoolCheckTaskListContext(
		t,
		"pool_group_id="+strconv.Itoa(group.Id)+"&status=failed&search=needle&p=1&page_size=10",
	)
	ListPoolAccountCheckTasks(ctx)

	response := decodeAccountPoolCheckTaskPageResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	require.Equal(t, 1, response.Data.Page)
	require.Equal(t, 10, response.Data.PageSize)
	require.Equal(t, 1, response.Data.Total)
	require.Len(t, response.Data.Items, 1)
	require.Equal(t, newer.Id, response.Data.Items[0].ID)
	require.Equal(t, model.PoolAccountCheckTaskStatusFailed, response.Data.Items[0].Status)

	ctx, recorder = newAccountPoolCheckTaskListContext(
		t,
		"pool_group_id="+strconv.Itoa(group.Id)+"&p=2&page_size=1",
	)
	ListPoolAccountCheckTasks(ctx)

	response = decodeAccountPoolCheckTaskPageResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	require.Equal(t, 2, response.Data.Total)
	require.Len(t, response.Data.Items, 1)
	require.Equal(t, older.Id, response.Data.Items[0].ID)
}

func TestCleanupPoolAccountCheckTasksControllerKeepsActiveTasks(t *testing.T) {
	setupAccountPoolBatchStatusTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.PoolAccountCheckTask{}))
	group := createBatchStatusGroup(t, "controller-check-task-cleanup")
	now := common.GetTimestamp()
	completed := createControllerCheckTask(t, group, model.PoolAccountCheckTaskStatusCompleted, "cleanup completed", now-500, now-400)
	failed := createControllerCheckTask(t, group, model.PoolAccountCheckTaskStatusFailed, "cleanup failed", now-450, now-350)
	queued := createControllerCheckTask(t, group, model.PoolAccountCheckTaskStatusQueued, "cleanup queued", now-440, now-340)
	running := createControllerCheckTask(t, group, model.PoolAccountCheckTaskStatusRunning, "cleanup running", now-430, now-330)

	ctx, recorder := newAccountPoolCheckTaskCleanupContext(t, gin.H{
		"pool_group_id":    group.Id,
		"before_timestamp": now,
		"statuses":         []string{"completed", "failed", "queued", "running"},
		"limit":            10,
	})
	CleanupPoolAccountCheckTasks(ctx)

	response := decodeAccountPoolCheckTaskCleanupResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	require.Equal(t, 2, response.Data.Deleted)

	for _, taskID := range []int{completed.Id, failed.Id} {
		err := model.DB.Where("id = ?", taskID).First(&model.PoolAccountCheckTask{}).Error
		require.Error(t, err)
		require.Equal(t, gorm.ErrRecordNotFound, err)
	}
	for _, taskID := range []int{queued.Id, running.Id} {
		require.NoError(t, model.DB.Where("id = ?", taskID).First(&model.PoolAccountCheckTask{}).Error)
	}
}

func TestAccountPoolCheckSystemTaskHandlerCompletesDomainTask(t *testing.T) {
	setupAccountPoolBatchStatusTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.PoolAccountCheckTask{}, &model.SystemTask{}, &model.SystemTaskLock{}))
	group := createBatchStatusGroup(t, "controller-check-task-handler")
	account := createBatchStatusAccount(t, group.Id, "controller-check-task-handler-account")
	encryptedCredential, err := common.EncryptSensitiveString(`{"api_key":"sk-controller-handler"}`)
	require.NoError(t, err)
	account.Credentials = encryptedCredential
	account.CredentialProvider = "codex"
	require.NoError(t, model.DB.Save(account).Error)

	checkTask, err := service.StartPoolAccountCheckTask(service.AccountPoolCheckTaskOptions{
		PoolGroupID: group.Id,
		AccountIDs:  []int{account.Id},
		Actor:       "controller-system-task-handler",
		RequestID:   "req-controller-system-task-handler",
	})
	require.NoError(t, err)

	systemTask, err := model.GetActiveSystemTaskByActiveKey("account_pool_check:" + strconv.Itoa(checkTask.ID))
	require.NoError(t, err)
	require.NotNil(t, systemTask)
	claimedTask, claimed, err := model.ClaimSystemTask(systemTask.ID, model.SystemTaskTypeAccountPoolCheck, "runner-account-pool-check", common.GetTimestamp()+60)
	require.NoError(t, err)
	require.True(t, claimed)

	accountPoolCheckHandler{}.Run(context.Background(), claimedTask, "runner-account-pool-check")

	finishedSystemTask, err := model.GetSystemTaskByTaskID(systemTask.TaskID)
	require.NoError(t, err)
	require.NotNil(t, finishedSystemTask)
	require.Equal(t, model.SystemTaskStatusSucceeded, finishedSystemTask.Status)
	require.Nil(t, finishedSystemTask.ActiveKey)

	var result service.AccountPoolCheckSystemTaskResult
	require.NoError(t, finishedSystemTask.DecodeResult(&result))
	require.Equal(t, checkTask.ID, result.CheckTaskID)
	require.Equal(t, model.PoolAccountCheckTaskStatusCompleted, result.Status)
	require.Equal(t, 1, result.Success)

	finishedCheckTask, err := service.GetPoolAccountCheckTask(checkTask.ID)
	require.NoError(t, err)
	require.Equal(t, model.PoolAccountCheckTaskStatusCompleted, finishedCheckTask.Status)
	require.Equal(t, 1, finishedCheckTask.Success)
}
