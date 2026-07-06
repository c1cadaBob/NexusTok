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

type accountPoolHealthAPIResponse struct {
	Success bool                            `json:"success"`
	Message string                          `json:"message"`
	Data    *model.AccountPoolHealthSummary `json:"data"`
}

func newAccountPoolHealthContext(t *testing.T, rawQuery string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	target := "/api/account-pool/health"
	if rawQuery != "" {
		target += "?" + rawQuery
	}
	ctx.Request = httptest.NewRequest(http.MethodGet, target, nil)
	return ctx, recorder
}

func decodeAccountPoolHealthResponse(t *testing.T, recorder *httptest.ResponseRecorder) accountPoolHealthAPIResponse {
	t.Helper()
	var response accountPoolHealthAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

func TestGetAccountPoolHealthReturnsSummary(t *testing.T) {
	setupAccountPoolBatchStatusTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.PoolAccountUsageLog{}))
	group := createBatchStatusGroup(t, "controller-health-main")
	account := createBatchStatusAccount(t, group.Id, "controller-health-account")
	now := common.GetTimestamp()
	require.NoError(t, model.LOG_DB.Create(&model.PoolAccountUsageLog{
		CreatedAt:           now,
		PoolGroupId:         group.Id,
		PoolGroupName:       group.Name,
		PoolAccountId:       account.Id,
		PoolAccountName:     account.Name,
		PoolAccountAuthType: account.AuthType,
		Success:             true,
	}).Error)

	ctx, recorder := newAccountPoolHealthContext(
		t,
		"pool_group_id="+strconv.Itoa(group.Id)+"&abnormal_limit=5&audit_limit=5",
	)
	GetAccountPoolHealth(ctx)

	response := decodeAccountPoolHealthResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	require.NotNil(t, response.Data)
	require.Len(t, response.Data.Groups, 1)
	require.Equal(t, group.Id, response.Data.Groups[0].Id)
	require.Equal(t, int64(1), response.Data.Totals.TotalAccounts)
	require.Equal(t, int64(1), response.Data.Totals.AvailableAccounts)
	require.Equal(t, int64(1), response.Data.Totals.TodayRequests)
	require.Equal(t, int64(1), response.Data.Totals.TodaySuccesses)
	require.Equal(t, int64(0), response.Data.Totals.TodayFailures)
}
