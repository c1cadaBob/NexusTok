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
)

type accountPoolBatchExportAPIResponse struct {
	Success bool                         `json:"success"`
	Message string                       `json:"message"`
	Data    poolAccountBatchExportResult `json:"data"`
}

func newAccountPoolBatchExportContext(t *testing.T, groupID int, body any) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	payload, err := common.Marshal(body)
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/account-pool/groups/"+strconv.Itoa(groupID)+"/accounts/export", bytes.NewReader(payload))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(groupID)}}
	ctx.Set("username", "batch-export-tester")
	return ctx, recorder
}

func decodeAccountPoolBatchExportResponse(t *testing.T, recorder *httptest.ResponseRecorder) accountPoolBatchExportAPIResponse {
	t.Helper()
	var response accountPoolBatchExportAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

func TestBatchExportPoolAccountsExportsSelectedSafeSnapshot(t *testing.T) {
	setupAccountPoolBatchStatusTest(t)
	group := createBatchStatusGroup(t, "batch-export-main")
	otherGroup := createBatchStatusGroup(t, "batch-export-other")
	accountA := createBatchStatusAccount(t, group.Id, "batch-export-a")
	accountB := createBatchStatusAccount(t, group.Id, "batch-export-b")
	otherAccount := createBatchStatusAccount(t, otherGroup.Id, "batch-export-other")
	baseURL := "https://api.example.test/path?token=hidden-base-url-token"
	headerOverride := `{"Authorization":"Bearer hidden-header-token"}`
	require.NoError(t, model.DB.Model(accountA).Updates(map[string]interface{}{
		"credentials":           "encrypted-hidden-credential",
		"credential_metadata":   `{"refresh_token":"hidden-refresh-token"}`,
		"credential_attributes": `{"cookie":"hidden-cookie-value"}`,
		"credential_summary":    "sk-***safe-summary",
		"proxy":                 "http://user:hidden-proxy-password@proxy.example.test",
		"base_url":              baseURL,
		"header_override":       headerOverride,
		"daily_request_count":   7,
		"daily_used_quota":      99,
		"success_count":         11,
		"failed_count":          2,
		"last_error":            "上游最近错误",
		"temp_disabled_until":   common.GetTimestamp() + 60,
		"status_code_mapping":   `{"429":"cooldown"}`,
		"quota_snapshot":        `{"quota":"hidden-quota-detail"}`,
		"model_states":          `{"model":"hidden-model-state"}`,
		"recent_requests":       `["hidden-request-id"]`,
		"credential_provider":   "openai",
		"credential_label":      "safe label",
		"daily_limit_action":    model.AccountPoolDailyLimitActionDisable,
		"rate_limit_rpm":        12,
		"daily_request_limit":   100,
		"daily_quota_limit":     200,
		"max_concurrency":       3,
		"weight":                4,
		"priority":              5,
		"unavailable":           true,
		"status_message":        "临时不可用",
		"disabled_reason":       "导出测试",
		"last_checked_time":     common.GetTimestamp(),
		"last_refreshed_time":   common.GetTimestamp(),
		"next_refresh_time":     common.GetTimestamp() + 3600,
		"next_retry_time":       common.GetTimestamp() + 120,
	}).Error)

	ctx, recorder := newAccountPoolBatchExportContext(t, group.Id, gin.H{
		"account_ids": []int{accountB.Id, otherAccount.Id, accountA.Id, accountA.Id, 0},
	})
	BatchExportPoolAccounts(ctx)

	rawBody := recorder.Body.String()
	require.NotContains(t, rawBody, "encrypted-hidden-credential")
	require.NotContains(t, rawBody, "hidden-refresh-token")
	require.NotContains(t, rawBody, "hidden-cookie-value")
	require.NotContains(t, rawBody, "hidden-proxy-password")
	require.NotContains(t, rawBody, "hidden-base-url-token")
	require.NotContains(t, rawBody, "hidden-header-token")
	require.NotContains(t, rawBody, "hidden-quota-detail")
	require.NotContains(t, rawBody, "hidden-model-state")
	require.NotContains(t, rawBody, "hidden-request-id")

	response := decodeAccountPoolBatchExportResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	require.Equal(t, "nexustok_account_pool_safe_export_v1", response.Data.Format)
	require.False(t, response.Data.CredentialsExported)
	require.Equal(t, 3, response.Data.Total)
	require.Equal(t, 2, response.Data.Exported)
	require.Equal(t, 1, response.Data.Skipped)
	require.Equal(t, []int{otherAccount.Id}, response.Data.SkippedAccountIDs)
	require.Len(t, response.Data.Accounts, 2)
	require.Equal(t, accountB.Id, response.Data.Accounts[0].ID)
	require.Equal(t, accountA.Id, response.Data.Accounts[1].ID)
	require.Equal(t, "sk-***safe-summary", response.Data.Accounts[1].CredentialSummary)
	require.True(t, response.Data.Accounts[1].ProxyConfigured)
	require.True(t, response.Data.Accounts[1].BaseURLConfigured)
	require.True(t, response.Data.Accounts[1].HasHeaderOverride)
	require.Equal(t, int64(11), response.Data.Accounts[1].SuccessCount)
	require.Contains(t, response.Data.SensitiveFieldsRedacted, "credentials")
	require.Contains(t, response.Data.SensitiveFieldsRedacted, "header_override")
}

func TestBatchExportPoolAccountsExportsWholeGroupWhenIDsOmitted(t *testing.T) {
	setupAccountPoolBatchStatusTest(t)
	group := createBatchStatusGroup(t, "batch-export-all")
	accountA := createBatchStatusAccount(t, group.Id, "batch-export-all-a")
	accountB := createBatchStatusAccount(t, group.Id, "batch-export-all-b")
	otherGroup := createBatchStatusGroup(t, "batch-export-all-other")
	_ = createBatchStatusAccount(t, otherGroup.Id, "batch-export-all-other")

	ctx, recorder := newAccountPoolBatchExportContext(t, group.Id, gin.H{})
	BatchExportPoolAccounts(ctx)

	response := decodeAccountPoolBatchExportResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	require.Equal(t, 2, response.Data.Total)
	require.Equal(t, 2, response.Data.Exported)
	require.Equal(t, 0, response.Data.Skipped)
	require.Len(t, response.Data.Accounts, 2)
	require.Equal(t, accountA.Id, response.Data.Accounts[0].ID)
	require.Equal(t, accountB.Id, response.Data.Accounts[1].ID)
}
