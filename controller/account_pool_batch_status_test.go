package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type accountPoolBatchStatusAPIResponse struct {
	Success bool                         `json:"success"`
	Message string                       `json:"message"`
	Data    poolAccountBatchStatusResult `json:"data"`
}

func setupAccountPoolBatchStatusTest(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.AccountPoolGroup{}, &model.PoolAccount{}, &model.AccountPoolAuthFile{}, &model.PoolAccountStateLog{}))
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
	})
}

func newAccountPoolBatchStatusContext(t *testing.T, groupID int, body any) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	payload, err := common.Marshal(body)
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/account-pool/groups/"+strconv.Itoa(groupID)+"/accounts/status", bytes.NewReader(payload))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(groupID)}}
	ctx.Set("username", "batch-status-tester")
	return ctx, recorder
}

func decodeAccountPoolBatchStatusResponse(t *testing.T, recorder *httptest.ResponseRecorder) accountPoolBatchStatusAPIResponse {
	t.Helper()
	var response accountPoolBatchStatusAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

func createBatchStatusGroup(t *testing.T, name string) *model.AccountPoolGroup {
	t.Helper()
	group := &model.AccountPoolGroup{
		Name:     name,
		Platform: "codex",
		AuthType: model.AccountPoolAuthTypeAPIKey,
		Source:   model.AccountPoolGroupSourceNative,
		Status:   common.ChannelStatusEnabled,
	}
	require.NoError(t, model.DB.Create(group).Error)
	return group
}

func createBatchStatusAccount(t *testing.T, groupID int, name string) *model.PoolAccount {
	t.Helper()
	account := &model.PoolAccount{
		PoolGroupId:       groupID,
		Name:              name,
		Platform:          "codex",
		AuthType:          model.AccountPoolAuthTypeAPIKey,
		Credentials:       "encrypted-test",
		CredentialSummary: "test-key",
		Status:            common.ChannelStatusEnabled,
		Schedulable:       true,
	}
	require.NoError(t, model.DB.Create(account).Error)
	return account
}

func TestBatchUpdatePoolAccountStatusDisablesOnlyAccountsInGroup(t *testing.T) {
	setupAccountPoolBatchStatusTest(t)
	group := createBatchStatusGroup(t, "batch-status-main")
	otherGroup := createBatchStatusGroup(t, "batch-status-other")
	accountA := createBatchStatusAccount(t, group.Id, "batch-status-a")
	accountB := createBatchStatusAccount(t, group.Id, "batch-status-b")
	otherAccount := createBatchStatusAccount(t, otherGroup.Id, "batch-status-other")

	ctx, recorder := newAccountPoolBatchStatusContext(t, group.Id, gin.H{
		"account_ids": []int{accountA.Id, accountB.Id, otherAccount.Id},
		"status":      common.ChannelStatusManuallyDisabled,
		"schedulable": false,
		"reason":      "批量禁用测试",
	})
	BatchUpdatePoolAccountStatus(ctx)

	response := decodeAccountPoolBatchStatusResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	require.Equal(t, 3, response.Data.Total)
	require.Equal(t, 2, response.Data.Updated)
	require.Equal(t, 1, response.Data.Skipped)
	require.Equal(t, 0, response.Data.Failed)

	updatedA, err := model.GetPoolAccountById(accountA.Id)
	require.NoError(t, err)
	require.Equal(t, common.ChannelStatusManuallyDisabled, updatedA.Status)
	require.False(t, updatedA.Schedulable)
	updatedB, err := model.GetPoolAccountById(accountB.Id)
	require.NoError(t, err)
	require.Equal(t, common.ChannelStatusManuallyDisabled, updatedB.Status)
	require.False(t, updatedB.Schedulable)
	unchanged, err := model.GetPoolAccountById(otherAccount.Id)
	require.NoError(t, err)
	require.Equal(t, common.ChannelStatusEnabled, unchanged.Status)
	require.True(t, unchanged.Schedulable)

	logs, total, err := model.GetPoolAccountStateLogs(model.PoolAccountStateLogFilter{
		PoolGroupId: group.Id,
		Action:      model.PoolAccountStateActionManualStatus,
		Search:      "批量禁用测试",
		Limit:       10,
	})
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Len(t, logs, 2)
	for _, log := range logs {
		require.Equal(t, "admin", log.Source)
		require.Equal(t, "batch-status-tester", log.Actor)
		require.Equal(t, common.ChannelStatusEnabled, log.BeforeStatus)
		require.Equal(t, common.ChannelStatusManuallyDisabled, log.AfterStatus)
	}
}

func TestBatchUpdatePoolAccountStatusClearCooldownRestoresTemporaryState(t *testing.T) {
	setupAccountPoolBatchStatusTest(t)
	group := createBatchStatusGroup(t, "batch-clear-cooldown")
	account := createBatchStatusAccount(t, group.Id, "batch-cooling")
	nextRetry := common.GetTimestamp() + 3600
	require.NoError(t, model.DB.Model(account).Updates(map[string]interface{}{
		"unavailable":         true,
		"status_message":      "temporary cooling",
		"last_error":          "temporary cooling",
		"disabled_reason":     "temporary cooling",
		"rate_limited_until":  nextRetry,
		"overload_until":      nextRetry,
		"temp_disabled_until": nextRetry,
		"next_retry_time":     nextRetry,
	}).Error)

	ctx, recorder := newAccountPoolBatchStatusContext(t, group.Id, gin.H{
		"account_ids":    []int{account.Id},
		"clear_cooldown": true,
		"reason":         "批量清冷却测试",
	})
	BatchUpdatePoolAccountStatus(ctx)

	response := decodeAccountPoolBatchStatusResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	require.Equal(t, 1, response.Data.Updated)
	require.Equal(t, 0, response.Data.Failed)

	updated, err := model.GetPoolAccountById(account.Id)
	require.NoError(t, err)
	require.Equal(t, common.ChannelStatusEnabled, updated.Status)
	require.True(t, updated.Schedulable)
	require.False(t, updated.Unavailable)
	require.Zero(t, updated.RateLimitedUntil)
	require.Zero(t, updated.OverloadUntil)
	require.Zero(t, updated.TempDisabledUntil)
	require.Zero(t, updated.NextRetryTime)
	require.Empty(t, updated.LastError)
	require.Empty(t, updated.StatusMessage)

	logs, total, err := model.GetPoolAccountStateLogs(model.PoolAccountStateLogFilter{
		PoolGroupId: group.Id,
		Action:      model.PoolAccountStateActionManualClearCooldown,
		Search:      "批量清冷却测试",
		Limit:       10,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, logs, 1)
	require.True(t, logs[0].BeforeUnavailable)
	require.False(t, logs[0].AfterUnavailable)
	require.Equal(t, int64(0), logs[0].AfterNextRetryTime)
}

func TestBatchUpdatePoolAccountStatusRejectsEmptyAccountIDs(t *testing.T) {
	setupAccountPoolBatchStatusTest(t)
	group := createBatchStatusGroup(t, "batch-status-empty")

	ctx, recorder := newAccountPoolBatchStatusContext(t, group.Id, gin.H{
		"account_ids": []int{},
		"status":      common.ChannelStatusEnabled,
	})
	BatchUpdatePoolAccountStatus(ctx)

	response := decodeAccountPoolBatchStatusResponse(t, recorder)
	require.False(t, response.Success)
	require.Contains(t, response.Message, "account_ids is required")
}
