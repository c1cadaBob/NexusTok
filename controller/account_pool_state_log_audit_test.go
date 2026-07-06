package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type accountPoolStateLogAuditSummaryAPIResponse struct {
	Success bool                                   `json:"success"`
	Message string                                 `json:"message"`
	Data    *model.PoolAccountStateLogAuditSummary `json:"data"`
}

type accountPoolStateLogAuditExportAPIResponse struct {
	Success bool                                 `json:"success"`
	Message string                               `json:"message"`
	Data    poolAccountStateLogAuditExportResult `json:"data"`
}

func newAccountPoolStateLogAuditContext(t *testing.T, path string, rawQuery string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	target := path
	if rawQuery != "" {
		target += "?" + rawQuery
	}
	ctx.Request = httptest.NewRequest(http.MethodGet, target, nil)
	return ctx, recorder
}

func decodeAccountPoolStateLogAuditSummaryResponse(t *testing.T, recorder *httptest.ResponseRecorder) accountPoolStateLogAuditSummaryAPIResponse {
	t.Helper()
	var response accountPoolStateLogAuditSummaryAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

func decodeAccountPoolStateLogAuditExportResponse(t *testing.T, recorder *httptest.ResponseRecorder) accountPoolStateLogAuditExportAPIResponse {
	t.Helper()
	var response accountPoolStateLogAuditExportAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

func createControllerStateAuditLog(t *testing.T, group *model.AccountPoolGroup, account *model.PoolAccount, action string, source string, actor string, requestID string, createdAt int64) {
	t.Helper()
	require.NoError(t, model.LOG_DB.Create(&model.PoolAccountStateLog{
		CreatedAt:           createdAt,
		PoolGroupId:         group.Id,
		PoolGroupName:       group.Name,
		PoolAccountId:       account.Id,
		PoolAccountName:     account.Name,
		PoolAccountAuthType: account.AuthType,
		Action:              action,
		Source:              source,
		Actor:               actor,
		Reason:              "controller audit test",
		RequestId:           requestID,
		BeforeStatus:        common.ChannelStatusEnabled,
		AfterStatus:         common.ChannelStatusManuallyDisabled,
		BeforeSchedulable:   true,
		AfterSchedulable:    false,
	}).Error)
}

func TestGetAccountPoolStateLogAuditSummaryReturnsFilteredSummary(t *testing.T) {
	setupAccountPoolBatchStatusTest(t)
	group := createBatchStatusGroup(t, "controller-audit-main")
	otherGroup := createBatchStatusGroup(t, "controller-audit-other")
	accountA := createBatchStatusAccount(t, group.Id, "controller-audit-a")
	accountB := createBatchStatusAccount(t, group.Id, "controller-audit-b")
	otherAccount := createBatchStatusAccount(t, otherGroup.Id, "controller-audit-other")
	now := common.GetTimestamp()
	createControllerStateAuditLog(t, group, accountA, model.PoolAccountStateActionManualStatus, "admin", "alice", "bulk-controller", now-20)
	createControllerStateAuditLog(t, group, accountB, model.PoolAccountStateActionManualStatus, "admin", "alice", "bulk-controller", now-10)
	createControllerStateAuditLog(t, otherGroup, otherAccount, model.PoolAccountStateActionRelayError, "relay", "", "other-controller", now-5)

	ctx, recorder := newAccountPoolStateLogAuditContext(
		t,
		"/api/account-pool/state-logs/audit-summary",
		"pool_group_id="+strconv.Itoa(group.Id),
	)
	GetAccountPoolStateLogAuditSummary(ctx)

	response := decodeAccountPoolStateLogAuditSummaryResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	require.NotNil(t, response.Data)
	require.Equal(t, int64(2), response.Data.Total)
	require.Equal(t, int64(2), response.Data.ManualTotal)
	require.Equal(t, int64(0), response.Data.AutomaticTotal)
	require.Len(t, response.Data.RecentBulkOperations, 1)
	require.Equal(t, "bulk-controller", response.Data.RecentBulkOperations[0].RequestId)
	require.Equal(t, 2, response.Data.RecentBulkOperations[0].AccountCount)
}

func TestExportAccountPoolStateLogsReturnsSafeFilteredSnapshot(t *testing.T) {
	setupAccountPoolBatchStatusTest(t)
	group := createBatchStatusGroup(t, "controller-audit-export-main")
	otherGroup := createBatchStatusGroup(t, "controller-audit-export-other")
	account := createBatchStatusAccount(t, group.Id, "controller-audit-export-a")
	otherAccount := createBatchStatusAccount(t, otherGroup.Id, "controller-audit-export-other")
	now := common.GetTimestamp()
	createControllerStateAuditLog(t, group, account, model.PoolAccountStateActionManualDelete, "admin", "alice", "export-main", now-20)
	createControllerStateAuditLog(t, otherGroup, otherAccount, model.PoolAccountStateActionManualDelete, "admin", "alice", "export-other", now-10)

	ctx, recorder := newAccountPoolStateLogAuditContext(
		t,
		"/api/account-pool/state-logs/export",
		"pool_group_id="+strconv.Itoa(group.Id)+"&limit=10",
	)
	ExportAccountPoolStateLogs(ctx)

	rawBody := recorder.Body.String()
	require.NotContains(t, rawBody, "encrypted-test")
	response := decodeAccountPoolStateLogAuditExportResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	require.Equal(t, "nexustok_account_pool_state_audit_export_v1", response.Data.Format)
	require.Equal(t, int64(1), response.Data.Total)
	require.Equal(t, 1, response.Data.Exported)
	require.Equal(t, 10, response.Data.Limit)
	require.Len(t, response.Data.Logs, 1)
	require.Equal(t, account.Id, response.Data.Logs[0].PoolAccountID)
	require.Contains(t, response.Data.SensitiveFieldsRedacted, "credentials")
}
