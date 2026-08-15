// Package controller - channel_test_internal_test.go
// 该文件包含渠道控制器内部测试（分层计费相关）
//
// 测试内容包括：
// - 分层计费配额计算（settleTestQuota）
// - 测试日志中分层计费信息注入（buildTestLogOther）
package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/pkg/billingexpr"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/service/upstreamaccount"
	"github.com/c1cada/NexusTok/setting/operation_setting"
	"github.com/c1cada/NexusTok/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSettleTestQuotaUsesTieredBilling(t *testing.T) {
	info := &relaycommon.RelayInfo{
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode:   "tiered_expr",
			ExprString:    `param("stream") == true ? tier("stream", p * 3) : tier("base", p * 2)`,
			ExprHash:      billingexpr.ExprHashString(`param("stream") == true ? tier("stream", p * 3) : tier("base", p * 2)`),
			GroupRatio:    1,
			EstimatedTier: "stream",
			QuotaPerUnit:  common.QuotaPerUnit,
			ExprVersion:   1,
		},
		BillingRequestInput: &billingexpr.RequestInput{
			Body: []byte(`{"stream":true}`),
		},
	}

	quota, result := settleTestQuota(info, types.PriceData{
		ModelRatio:      1,
		CompletionRatio: 2,
	}, &dto.Usage{
		PromptTokens: 1000,
	})

	require.Equal(t, 1500, quota)
	require.NotNil(t, result)
	require.Equal(t, "stream", result.MatchedTier)
}

func TestBuildTestLogOtherInjectsTieredInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	info := &relaycommon.RelayInfo{
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode: "tiered_expr",
			ExprString:  `tier("base", p * 2)`,
		},
		ChannelMeta: &relaycommon.ChannelMeta{},
	}
	priceData := types.PriceData{
		GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1},
	}
	usage := &dto.Usage{
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 12,
		},
	}

	other := buildTestLogOther(ctx, info, priceData, usage, &billingexpr.TieredResult{
		MatchedTier: "base",
	})

	require.Equal(t, "tiered_expr", other["billing_mode"])
	require.Equal(t, "base", other["matched_tier"])
	require.NotEmpty(t, other["expr_b64"])
}

func TestBuildTestLogOtherIncludesQuotaSaturation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{},
		QuotaClamp: &common.QuotaClamp{
			Op:      "QuotaRound",
			Kind:    common.QuotaClampOverflow,
			Clamped: common.MaxQuota,
		},
	}
	priceData := types.PriceData{
		GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1},
	}
	usage := &dto.Usage{}

	other := buildTestLogOther(ctx, info, priceData, usage, nil)

	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	saturation, ok := adminInfo["quota_saturation"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "QuotaRound", saturation["op"])
	require.Equal(t, common.QuotaClampOverflow, saturation["kind"])
}

func TestRecordManualChannelTestFailureLogDisablesSelectedSyncedAccount(t *testing.T) {
	db := setupChannelTestLogControllerTestDB(t)
	withUpstreamAccountKeyCheckSetting(t, operation_setting.UpstreamAccountKeyCheckSetting{
		Enabled:            false,
		IntervalMinutes:    30,
		FailureThreshold:   2,
		AutoRecoverEnabled: true,
	})

	channel, account := createUpstreamAccountKeyCheckFixture(t, "https://upstream.example", common.ChannelStatusEnabled, `{"upstream_account_sync":{"platform":"new-api","base_url":"https://upstream.example","external_id":"key-1","key_digest":"digest","ratio_conversion":0.2,"auto_check_failure_count":1}}`)
	user := createChannelTestLogUser(t, db)
	testCtx := createChannelTestResultContext(account)
	result := testResult{
		context:             testCtx,
		localErr:            errors.New("upstream rejected sk-secret"),
		newAPIError:         types.NewOpenAIError(errors.New("upstream rejected sk-secret"), types.ErrorCodeBadResponse, http.StatusInternalServerError),
		countForAutoDisable: true,
	}

	result = recordManualChannelTestFailureLog(testCtx, &channel, user.Id, "gpt-test", "openai", false, account.Id, 1.2, result)

	require.True(t, result.autoCheckFailureCounted)
	require.True(t, result.autoCheckDisabled)
	require.Equal(t, 2, result.autoCheckFailureCount)

	var stored model.ChannelAccount
	require.NoError(t, db.First(&stored, account.Id).Error)
	require.Equal(t, common.ChannelStatusAutoDisabled, stored.Status)
	metadata := upstreamaccount.ReadAccountAutoCheckMetadata(stored.OtherSettings)
	require.Equal(t, 2, metadata.FailureCount)
	require.True(t, metadata.DisabledByAutoCheck)
	require.NotContains(t, metadata.LastError, "sk-secret")

	var log model.Log
	require.NoError(t, model.LOG_DB.Where("type = ?", model.LogTypeConsume).Order("id desc").First(&log).Error)
	require.Equal(t, "模型测试", log.TokenName)
	require.Equal(t, 0, log.Quota)
	require.Equal(t, channel.Id, log.ChannelId)
	require.Equal(t, "gpt-test", log.ModelName)
	require.NotContains(t, log.Other, "sk-secret")
	var other map[string]any
	require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
	channelTest, ok := other["channel_test"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "failed", channelTest["status"])
	require.Equal(t, true, channelTest["counted_for_auto_disable"])
	require.Equal(t, true, channelTest["auto_disabled"])
	require.EqualValues(t, 2, channelTest["failure_count"])
}

func TestRecordManualChannelTestFailureLogAutoSelectionDoesNotMutateAccount(t *testing.T) {
	db := setupChannelTestLogControllerTestDB(t)
	withUpstreamAccountKeyCheckSetting(t, operation_setting.UpstreamAccountKeyCheckSetting{
		Enabled:            false,
		IntervalMinutes:    30,
		FailureThreshold:   1,
		AutoRecoverEnabled: true,
	})

	channel, account := createUpstreamAccountKeyCheckFixture(t, "https://upstream.example", common.ChannelStatusEnabled, `{"upstream_account_sync":{"platform":"new-api","base_url":"https://upstream.example","external_id":"key-1","key_digest":"digest","ratio_conversion":0.2}}`)
	user := createChannelTestLogUser(t, db)
	testCtx := createChannelTestResultContext(account)
	result := testResult{
		context:             testCtx,
		localErr:            errors.New("upstream rejected sk-secret"),
		newAPIError:         types.NewOpenAIError(errors.New("upstream rejected sk-secret"), types.ErrorCodeBadResponse, http.StatusInternalServerError),
		countForAutoDisable: true,
	}

	result = recordManualChannelTestFailureLog(testCtx, &channel, user.Id, "gpt-test", "openai", false, 0, 1.2, result)

	require.False(t, result.autoCheckFailureCounted)
	require.False(t, result.autoCheckDisabled)

	var stored model.ChannelAccount
	require.NoError(t, db.First(&stored, account.Id).Error)
	require.Equal(t, common.ChannelStatusEnabled, stored.Status)
	require.Equal(t, 0, upstreamaccount.ReadAccountAutoCheckMetadata(stored.OtherSettings).FailureCount)

	var count int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("type = ?", model.LogTypeConsume).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestApplyManualChannelAccountTestSuccessClearsFailureCount(t *testing.T) {
	db := setupChannelTestLogControllerTestDB(t)
	withUpstreamAccountKeyCheckSetting(t, operation_setting.UpstreamAccountKeyCheckSetting{
		Enabled:            false,
		IntervalMinutes:    30,
		FailureThreshold:   2,
		AutoRecoverEnabled: true,
	})

	channel, account := createUpstreamAccountKeyCheckFixture(t, "https://upstream.example", common.ChannelStatusEnabled, `{"upstream_account_sync":{"platform":"new-api","base_url":"https://upstream.example","external_id":"key-1","key_digest":"digest","ratio_conversion":0.2,"auto_check_failure_count":1,"auto_check_last_error":"previous"}}`)

	failureCount, updated := applyManualChannelAccountTestSuccess(channel.Id, account.Id)

	require.True(t, updated)
	require.Equal(t, 0, failureCount)
	var stored model.ChannelAccount
	require.NoError(t, db.First(&stored, account.Id).Error)
	metadata := upstreamaccount.ReadAccountAutoCheckMetadata(stored.OtherSettings)
	require.Equal(t, 0, metadata.FailureCount)
	require.Equal(t, "success", metadata.LastStatus)
	require.NotZero(t, metadata.LastSuccessAt)
}

func TestShouldUseStreamForChannelTestForcesCodexStream(t *testing.T) {
	require.True(t, shouldUseStreamForChannelTest(&model.Channel{Type: constant.ChannelTypeCodex}, false))
	require.True(t, shouldUseStreamForChannelTest(&model.Channel{Type: constant.ChannelTypeCodex}, true))
	require.False(t, shouldUseStreamForChannelTest(&model.Channel{Type: constant.ChannelTypeOpenAI}, false))
	require.True(t, shouldUseStreamForChannelTest(&model.Channel{Type: constant.ChannelTypeOpenAI}, true))
}

func TestResolveChannelTestUserIDUsesRequestUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("id", 2)

	userID, err := resolveChannelTestUserID(ctx)

	require.NoError(t, err)
	require.Equal(t, 2, userID)
}

func TestSelectChannelsForAutomaticTestPassiveRecoveryOnlyUsesAutoDisabled(t *testing.T) {
	channels := []*model.Channel{
		{Id: 1, Status: common.ChannelStatusEnabled},
		{Id: 2, Status: common.ChannelStatusAutoDisabled},
		{Id: 3, Status: common.ChannelStatusManuallyDisabled},
	}

	selected := selectChannelsForAutomaticTest(channels, operation_setting.ChannelTestModePassiveRecovery)

	require.Len(t, selected, 1)
	require.Equal(t, 2, selected[0].Id)
}

func TestSelectChannelsForAutomaticTestScheduledSkipsManualDisabled(t *testing.T) {
	channels := []*model.Channel{
		{Id: 1, Status: common.ChannelStatusEnabled},
		{Id: 2, Status: common.ChannelStatusAutoDisabled},
		{Id: 3, Status: common.ChannelStatusManuallyDisabled},
	}

	selected := selectChannelsForAutomaticTest(channels, operation_setting.ChannelTestModeScheduledAll)

	require.Len(t, selected, 2)
	require.Equal(t, 1, selected[0].Id)
	require.Equal(t, 2, selected[1].Id)
}

func TestTestAllChannelsRejectsExistingActiveTask(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.SystemTask{}, &model.SystemTaskLock{}))

	existing, err := model.CreateSystemTask(model.SystemTaskTypeChannelTest, nil, nil)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/test", nil)

	TestAllChannels(ctx)

	require.Equal(t, http.StatusConflict, recorder.Code)
	require.Contains(t, recorder.Body.String(), existing.TaskID)
	require.Contains(t, recorder.Body.String(), "已有通道测试任务正在运行或等待中")
}

func setupChannelTestLogControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := setupChannelAccountMutationTestDB(t)
	oldLogDB := model.LOG_DB
	oldLogConsumeEnabled := common.LogConsumeEnabled
	oldRedisEnabled := common.RedisEnabled
	model.LOG_DB = db
	common.LogConsumeEnabled = true
	common.RedisEnabled = false
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Log{}))
	t.Cleanup(func() {
		model.LOG_DB = oldLogDB
		common.LogConsumeEnabled = oldLogConsumeEnabled
		common.RedisEnabled = oldRedisEnabled
	})
	return db
}

func createChannelTestLogUser(t *testing.T, db *gorm.DB) model.User {
	t.Helper()
	user := model.User{
		Username: "channel-test-admin",
		Password: "password",
		Role:     common.RoleRootUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
		Quota:    int(common.QuotaPerUnit),
	}
	require.NoError(t, db.Create(&user).Error)
	return user
}

func createChannelTestResultContext(account model.ChannelAccount) *gin.Context {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Set("group", "default")
	common.SetContextKey(ctx, constant.ContextKeyChannelCredentialMode, constant.ChannelCredentialModeAccountPool)
	common.SetContextKey(ctx, constant.ContextKeyChannelAccountPool, true)
	common.SetContextKey(ctx, constant.ContextKeyChannelAccountId, account.Id)
	common.SetContextKey(ctx, constant.ContextKeyChannelAccountName, account.Name)
	return ctx
}
