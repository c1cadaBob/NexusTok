package service

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func setupAccountPoolUsageLogTest(t *testing.T) {
	t.Helper()
	require.NoError(t, model.DB.AutoMigrate(&model.PoolAccount{}, &model.PoolAccountUsageLog{}))
	require.NoError(t, model.DB.Exec("DELETE FROM pool_accounts").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM pool_account_usage_logs").Error)
	t.Cleanup(func() {
		_ = model.DB.Exec("DELETE FROM pool_accounts").Error
		_ = model.DB.Exec("DELETE FROM pool_account_usage_logs").Error
	})
}

func newAccountPoolUsageLogGinContext() *gin.Context {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(string(constant.ContextKeyUserName), "alice")
	c.Set("token_name", "admin-token")
	c.Set(common.RequestIdKey, "req-account-pool")
	c.Set(common.UpstreamRequestIdKey, "upstream-account-pool")
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	common.SetContextKey(c, constant.ContextKeyChannelName, "native-channel")
	return c
}

func TestRecordRelayPoolAccountUsageSuccess(t *testing.T) {
	setupAccountPoolUsageLogTest(t)
	c := newAccountPoolUsageLogGinContext()
	relayInfo := &relaycommon.RelayInfo{
		UserId:          7,
		TokenId:         9,
		UsingGroup:      "default",
		OriginModelName: "gpt-4o-mini",
		RequestId:       "req-from-relay",
		RetryIndex:      1,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId:           3,
			PoolGroupId:         5,
			PoolGroupName:       "codex-main",
			PoolAccountId:       11,
			PoolAccountName:     "codex-a",
			PoolAccountAuthType: model.AccountPoolAuthTypeOfficialOAuth,
		},
	}

	RecordRelayPoolAccountUsageSuccess(c, relayInfo, 12, 8, 123, 2)

	logs, total, err := model.GetPoolAccountUsageLogs(model.PoolAccountUsageLogFilter{
		PoolAccountId: 11,
		Success:       common.GetPointer(true),
		Limit:         10,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, logs, 1)
	require.True(t, logs[0].Success)
	require.Equal(t, 123, logs[0].Quota)
	require.Equal(t, 20, logs[0].PromptTokens+logs[0].CompletionTokens)
	require.Equal(t, "codex-main", logs[0].PoolGroupName)
	require.Equal(t, "native-channel", logs[0].ChannelName)
	require.Equal(t, "alice", logs[0].Username)
	require.Equal(t, "admin-token", logs[0].TokenName)
	require.Equal(t, "upstream-account-pool", logs[0].UpstreamRequestId)
}

func TestProcessPoolAccountErrorRecordsUsageFailure(t *testing.T) {
	setupAccountPoolUsageLogTest(t)
	account := &model.PoolAccount{
		Id:             22,
		PoolGroupId:    6,
		Name:           "codex-b",
		Platform:       "codex",
		AuthType:       model.AccountPoolAuthTypeAPIKey,
		Credentials:    "encrypted-placeholder",
		Status:         common.ChannelStatusEnabled,
		Schedulable:    true,
		RecentRequests: "",
	}
	require.NoError(t, model.DB.Create(account).Error)

	c := newAccountPoolUsageLogGinContext()
	common.SetContextKey(c, constant.ContextKeyPoolGroupId, 6)
	common.SetContextKey(c, constant.ContextKeyPoolGroupName, "codex-fallback")
	common.SetContextKey(c, constant.ContextKeyPoolAccountId, account.Id)
	common.SetContextKey(c, constant.ContextKeyPoolAccountName, account.Name)
	common.SetContextKey(c, constant.ContextKeyPoolAccountAuthType, account.AuthType)
	common.SetContextKey(c, constant.ContextKeyChannelId, 4)
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "gpt-4o-mini")
	channelError := *types.NewChannelError(4, constant.ChannelTypeOpenAI, "native-channel", false, "sk-test", true)
	channelError.AccountPool = true
	channelError.PoolGroupId = 6
	channelError.PoolGroupName = "codex-fallback"
	channelError.PoolAccountId = account.Id
	channelError.PoolAccountName = account.Name
	channelError.PoolAccountAuthType = account.AuthType
	poolErr := types.NewOpenAIError(errors.New("invalid api key sk-test"), types.ErrorCodeBadResponseStatusCode, http.StatusUnauthorized)

	ProcessPoolAccountError(c, channelError, poolErr)

	logs, total, queryErr := model.GetPoolAccountUsageLogs(model.PoolAccountUsageLogFilter{
		PoolAccountId: account.Id,
		Success:       common.GetPointer(false),
		Limit:         10,
	})
	require.NoError(t, queryErr)
	require.Equal(t, int64(1), total)
	require.Len(t, logs, 1)
	require.False(t, logs[0].Success)
	require.Equal(t, http.StatusUnauthorized, logs[0].StatusCode)
	require.Equal(t, string(types.ErrorCodeBadResponseStatusCode), logs[0].ErrorCode)
	require.NotEmpty(t, logs[0].ErrorMessage)

	updated, getErr := model.GetPoolAccountById(account.Id)
	require.NoError(t, getErr)
	require.Equal(t, int64(1), updated.FailedCount)
	require.NotEmpty(t, updated.LastError)
}
