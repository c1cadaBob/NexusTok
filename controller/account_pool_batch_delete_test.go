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
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type accountPoolBatchDeleteAPIResponse struct {
	Success bool                         `json:"success"`
	Message string                       `json:"message"`
	Data    poolAccountBatchDeleteResult `json:"data"`
}

func newAccountPoolBatchDeleteContext(t *testing.T, groupID int, body any) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	payload, err := common.Marshal(body)
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/account-pool/groups/"+strconv.Itoa(groupID)+"/accounts/delete", bytes.NewReader(payload))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(groupID)}}
	ctx.Set("username", "batch-delete-tester")
	return ctx, recorder
}

func decodeAccountPoolBatchDeleteResponse(t *testing.T, recorder *httptest.ResponseRecorder) accountPoolBatchDeleteAPIResponse {
	t.Helper()
	var response accountPoolBatchDeleteAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

func TestBatchDeletePoolAccountsDeletesOnlyAccountsInGroup(t *testing.T) {
	setupAccountPoolBatchStatusTest(t)
	group := createBatchStatusGroup(t, "batch-delete-main")
	otherGroup := createBatchStatusGroup(t, "batch-delete-other")
	accountA := createBatchStatusAccount(t, group.Id, "batch-delete-a")
	accountB := createBatchStatusAccount(t, group.Id, "batch-delete-b")
	otherAccount := createBatchStatusAccount(t, otherGroup.Id, "batch-delete-other")

	ctx, recorder := newAccountPoolBatchDeleteContext(t, group.Id, gin.H{
		"account_ids": []int{accountA.Id, accountB.Id, otherAccount.Id, accountA.Id, 0},
		"reason":      "批量删除测试",
	})
	BatchDeletePoolAccounts(ctx)

	response := decodeAccountPoolBatchDeleteResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	require.Equal(t, 3, response.Data.Total)
	require.Equal(t, 2, response.Data.Deleted)
	require.Equal(t, 1, response.Data.Skipped)
	require.Equal(t, 0, response.Data.Failed)

	err := model.DB.Where("id = ?", accountA.Id).First(&model.PoolAccount{}).Error
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	err = model.DB.Where("id = ?", accountB.Id).First(&model.PoolAccount{}).Error
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	unchanged, err := model.GetPoolAccountById(otherAccount.Id)
	require.NoError(t, err)
	require.Equal(t, otherGroup.Id, unchanged.PoolGroupId)

	logs, total, err := model.GetPoolAccountStateLogs(model.PoolAccountStateLogFilter{
		PoolGroupId: group.Id,
		Action:      model.PoolAccountStateActionManualDelete,
		Search:      "批量删除测试",
		Limit:       10,
	})
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Len(t, logs, 2)
	for _, log := range logs {
		require.Equal(t, "admin", log.Source)
		require.Equal(t, "batch-delete-tester", log.Actor)
		require.Equal(t, common.ChannelStatusEnabled, log.BeforeStatus)
		require.True(t, log.BeforeSchedulable)
		require.Equal(t, 0, log.AfterStatus)
		require.False(t, log.AfterSchedulable)
	}
}

func TestBatchDeletePoolAccountsRejectsEmptyAccountIDs(t *testing.T) {
	setupAccountPoolBatchStatusTest(t)
	group := createBatchStatusGroup(t, "batch-delete-empty")

	ctx, recorder := newAccountPoolBatchDeleteContext(t, group.Id, gin.H{
		"account_ids": []int{},
	})
	BatchDeletePoolAccounts(ctx)

	response := decodeAccountPoolBatchDeleteResponse(t, recorder)
	require.False(t, response.Success)
	require.Contains(t, response.Message, "account_ids is required")
}

func TestDeletePoolAccountWritesStateLog(t *testing.T) {
	setupAccountPoolBatchStatusTest(t)
	group := createBatchStatusGroup(t, "single-delete-log")
	account := createBatchStatusAccount(t, group.Id, "single-delete-account")
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/api/account-pool/accounts/"+strconv.Itoa(account.Id), nil)
	ctx.Params = gin.Params{{Key: "account_id", Value: strconv.Itoa(account.Id)}}
	ctx.Set("username", "single-delete-tester")

	DeletePoolAccount(ctx)

	var apiResponse struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &apiResponse))
	require.True(t, apiResponse.Success, apiResponse.Message)
	err := model.DB.Where("id = ?", account.Id).First(&model.PoolAccount{}).Error
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	logs, total, err := model.GetPoolAccountStateLogs(model.PoolAccountStateLogFilter{
		PoolAccountId: account.Id,
		Action:        model.PoolAccountStateActionManualDelete,
		Search:        "管理员删除账号",
		Limit:         10,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, logs, 1)
	require.Equal(t, group.Id, logs[0].PoolGroupId)
	require.Equal(t, account.Name, logs[0].PoolAccountName)
	require.Equal(t, "single-delete-tester", logs[0].Actor)
	require.Equal(t, common.ChannelStatusEnabled, logs[0].BeforeStatus)
	require.Equal(t, 0, logs[0].AfterStatus)
}
