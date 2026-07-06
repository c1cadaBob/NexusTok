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

func createBatchDeleteAuthFile(t *testing.T, groupID int, accountID int, name string) *model.AccountPoolAuthFile {
	t.Helper()
	authFile := &model.AccountPoolAuthFile{
		Name:              name,
		SourcePlatform:    model.AccountPoolAuthFileFormatNative,
		Format:            model.AccountPoolAuthFileFormatNative,
		Provider:          "codex",
		Platform:          "codex",
		AuthType:          model.AccountPoolAuthTypeAPIKey,
		PoolGroupId:       groupID,
		PoolAccountId:     accountID,
		Status:            common.ChannelStatusEnabled,
		FileDigest:        name + "-digest",
		EncryptedContent:  "encrypted-auth-file-content",
		CredentialSummary: "sk-***test",
	}
	require.NoError(t, model.DB.Create(authFile).Error)
	return authFile
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

func TestDeletePoolAccountDetachesLinkedAuthFile(t *testing.T) {
	setupAccountPoolBatchStatusTest(t)
	group := createBatchStatusGroup(t, "single-delete-detach")
	account := createBatchStatusAccount(t, group.Id, "single-delete-detach-account")
	authFile := createBatchDeleteAuthFile(t, group.Id, account.Id, "single-delete-detach-auth")
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/api/account-pool/accounts/"+strconv.Itoa(account.Id), nil)
	ctx.Params = gin.Params{{Key: "account_id", Value: strconv.Itoa(account.Id)}}
	ctx.Set("username", "single-detach-tester")

	DeletePoolAccount(ctx)

	var apiResponse struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &apiResponse))
	require.True(t, apiResponse.Success, apiResponse.Message)
	err := model.DB.Where("id = ?", account.Id).First(&model.PoolAccount{}).Error
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	var updatedAuthFile model.AccountPoolAuthFile
	require.NoError(t, model.DB.Where("id = ?", authFile.Id).First(&updatedAuthFile).Error)
	require.Zero(t, updatedAuthFile.PoolAccountId)
	require.Equal(t, common.ChannelStatusManuallyDisabled, updatedAuthFile.Status)

	logs, total, err := model.GetPoolAccountStateLogs(model.PoolAccountStateLogFilter{
		PoolAccountId: account.Id,
		Action:        model.PoolAccountStateActionManualDelete,
		Search:        "管理员删除账号",
		Limit:         10,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, logs, 1)
	require.Equal(t, "single-detach-tester", logs[0].Actor)
	require.Equal(t, common.ChannelStatusEnabled, logs[0].BeforeStatus)
	require.Equal(t, 0, logs[0].AfterStatus)
}

func TestDeletePoolAccountKeepsAuthFileWhenOtherGroupUsesIt(t *testing.T) {
	setupAccountPoolBatchStatusTest(t)
	sourceGroup := createBatchStatusGroup(t, "single-delete-shared-source")
	targetGroup := createBatchStatusGroup(t, "single-delete-shared-target")
	sourceAccount := createBatchStatusAccount(t, sourceGroup.Id, "single-delete-shared-source-account")
	targetAccount := createBatchStatusAccount(t, targetGroup.Id, "single-delete-shared-target-account")
	authFile := createBatchDeleteAuthFile(t, sourceGroup.Id, sourceAccount.Id, "single-delete-shared-auth")
	require.NoError(t, model.DB.Model(sourceAccount).Update("auth_file_id", authFile.Id).Error)
	require.NoError(t, model.DB.Model(targetAccount).Update("auth_file_id", authFile.Id).Error)
	sourceAccount.AuthFileId = authFile.Id
	targetAccount.AuthFileId = authFile.Id

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/api/account-pool/accounts/"+strconv.Itoa(targetAccount.Id), nil)
	ctx.Params = gin.Params{{Key: "account_id", Value: strconv.Itoa(targetAccount.Id)}}
	ctx.Set("username", "single-shared-detach-tester")

	DeletePoolAccount(ctx)

	var apiResponse struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &apiResponse))
	require.True(t, apiResponse.Success, apiResponse.Message)
	err := model.DB.Where("id = ?", targetAccount.Id).First(&model.PoolAccount{}).Error
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	var updatedAuthFile model.AccountPoolAuthFile
	require.NoError(t, model.DB.Where("id = ?", authFile.Id).First(&updatedAuthFile).Error)
	require.Equal(t, sourceAccount.Id, updatedAuthFile.PoolAccountId)
	require.Equal(t, common.ChannelStatusEnabled, updatedAuthFile.Status)
}

func TestBatchDeletePoolAccountsDetachesLinkedAuthFiles(t *testing.T) {
	setupAccountPoolBatchStatusTest(t)
	group := createBatchStatusGroup(t, "batch-delete-detach")
	accountA := createBatchStatusAccount(t, group.Id, "batch-delete-detach-a")
	accountB := createBatchStatusAccount(t, group.Id, "batch-delete-detach-b")
	authFileA := createBatchDeleteAuthFile(t, group.Id, accountA.Id, "batch-delete-detach-auth-a")
	authFileB := createBatchDeleteAuthFile(t, group.Id, accountB.Id, "batch-delete-detach-auth-b")

	ctx, recorder := newAccountPoolBatchDeleteContext(t, group.Id, gin.H{
		"account_ids": []int{accountA.Id, accountB.Id},
		"reason":      "批量删除解除关联测试",
	})
	BatchDeletePoolAccounts(ctx)

	response := decodeAccountPoolBatchDeleteResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	require.Equal(t, 2, response.Data.Deleted)
	for _, authFileID := range []int{authFileA.Id, authFileB.Id} {
		var authFile model.AccountPoolAuthFile
		require.NoError(t, model.DB.Where("id = ?", authFileID).First(&authFile).Error)
		require.Zero(t, authFile.PoolAccountId)
		require.Equal(t, common.ChannelStatusManuallyDisabled, authFile.Status)
	}
}

func TestDeleteAccountPoolAuthFileWritesLinkedAccountDeleteLog(t *testing.T) {
	setupAccountPoolBatchStatusTest(t)
	group := createBatchStatusGroup(t, "delete-auth-file-log")
	account := createBatchStatusAccount(t, group.Id, "delete-auth-file-account")
	authFile := createBatchDeleteAuthFile(t, group.Id, account.Id, "delete-auth-file")
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/api/account-pool/auth-files/"+strconv.Itoa(authFile.Id)+"?delete_account=true", nil)
	ctx.Params = gin.Params{{Key: "auth_file_id", Value: strconv.Itoa(authFile.Id)}}
	ctx.Set("username", "auth-file-delete-tester")

	DeleteAccountPoolAuthFile(ctx)

	var apiResponse struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &apiResponse))
	require.True(t, apiResponse.Success, apiResponse.Message)
	err := model.DB.Where("id = ?", authFile.Id).First(&model.AccountPoolAuthFile{}).Error
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	err = model.DB.Where("id = ?", account.Id).First(&model.PoolAccount{}).Error
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	logs, total, err := model.GetPoolAccountStateLogs(model.PoolAccountStateLogFilter{
		PoolAccountId: account.Id,
		Action:        model.PoolAccountStateActionManualDelete,
		Search:        "删除认证文件时同步删除关联账号",
		Limit:         10,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, logs, 1)
	require.Equal(t, "auth-file-delete-tester", logs[0].Actor)
	require.Equal(t, account.Name, logs[0].PoolAccountName)
	require.Equal(t, common.ChannelStatusEnabled, logs[0].BeforeStatus)
	require.Equal(t, 0, logs[0].AfterStatus)
}
