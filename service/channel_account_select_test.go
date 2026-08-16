package service

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/setting/operation_setting"
	"github.com/c1cada/NexusTok/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelAccountSelectTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelAccount{}, &model.Ability{}))
	model.DB = db
	model.LOG_DB = db

	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
	})
	return db
}

func TestChannelAccountSupportsModelKeepsLegacyEmptyModels(t *testing.T) {
	channel := &model.Channel{Models: "gpt-channel"}
	account := &model.ChannelAccount{Models: ""}

	require.True(t, channelAccountSupportsModel(account, channel, "gpt-channel"))
	require.False(t, channelAccountSupportsModel(account, channel, "gpt-other"))
}

func TestChannelAccountSupportsModelRejectsEmptyModelsForUpstreamSync(t *testing.T) {
	channel := &model.Channel{
		Models:        "gpt-channel",
		OtherSettings: `{"upstream_account_sync":{"platform":"new-api","base_url":"https://upstream.example"}}`,
	}
	account := &model.ChannelAccount{Models: ""}

	require.False(t, channelAccountSupportsModel(account, channel, "gpt-channel"))
}

func TestSelectSpecificChannelAccountReportsEmptyModelsForUpstreamSync(t *testing.T) {
	db := setupChannelAccountSelectTestDB(t)
	channel := model.Channel{
		Type:          constant.ChannelTypeNewAPI,
		Status:        common.ChannelStatusEnabled,
		Name:          "synced-channel",
		Models:        "gpt-channel",
		Group:         "default",
		OtherSettings: `{"upstream_account_sync":{"platform":"new-api","base_url":"https://upstream.example"}}`,
		ChannelInfo: model.ChannelInfo{
			CredentialMode:     constant.ChannelCredentialModeAccountPool,
			AccountPoolEnabled: true,
		},
	}
	require.NoError(t, db.Create(&channel).Error)
	account := model.ChannelAccount{
		ChannelId:    channel.Id,
		Name:         "empty-model-key",
		Key:          "sk-empty-model",
		Status:       common.ChannelStatusEnabled,
		Models:       "",
		AccessGroups: "default",
	}
	require.NoError(t, db.Create(&account).Error)

	_, err := SelectSpecificChannelAccount(nil, &channel, "gpt-channel", "default", account.Id, 0)

	require.Error(t, err)
	require.Contains(t, err.Error(), "未配置可路由模型")
}

func TestSelectSpecificChannelAccountForTestAllowsDisabledSyncedAccount(t *testing.T) {
	db := setupChannelAccountSelectTestDB(t)
	channel := model.Channel{
		Type:          constant.ChannelTypeOpenAI,
		Status:        common.ChannelStatusEnabled,
		Name:          "synced-channel",
		Models:        "gpt-channel",
		Group:         "default",
		OtherSettings: `{"upstream_account_sync":{"platform":"new-api","base_url":"https://upstream.example"}}`,
		ChannelInfo: model.ChannelInfo{
			CredentialMode:     constant.ChannelCredentialModeAccountPool,
			AccountPoolEnabled: true,
		},
	}
	require.NoError(t, db.Create(&channel).Error)
	account := model.ChannelAccount{
		ChannelId:         channel.Id,
		Name:              "disabled-key",
		Key:               "sk-disabled",
		Status:            common.ChannelStatusManuallyDisabled,
		Models:            "gpt-channel",
		AccessGroups:      "default",
		RateLimitedUntil:  common.GetTimestamp() + 3600,
		TempDisabledUntil: common.GetTimestamp() + 3600,
	}
	require.NoError(t, db.Create(&account).Error)

	_, normalErr := SelectSpecificChannelAccount(nil, &channel, "gpt-channel", "default", account.Id, 0)
	selected, testErr := SelectSpecificChannelAccountForTest(nil, &channel, "gpt-channel", "default", account.Id, 0)

	require.Error(t, normalErr)
	require.NoError(t, testErr)
	require.Equal(t, account.Id, selected.Id)
}

func TestSelectChannelAccountForUpstreamSyncPrefersPriorityThenWeight(t *testing.T) {
	db := setupChannelAccountSelectTestDB(t)
	channel := model.Channel{
		Type:          constant.ChannelTypeOpenAI,
		Status:        common.ChannelStatusEnabled,
		Name:          "synced-channel",
		Models:        "gpt-channel",
		Group:         "default",
		OtherSettings: `{"upstream_account_sync":{"platform":"new-api","base_url":"https://upstream.example"}}`,
		ChannelInfo: model.ChannelInfo{
			CredentialMode:     constant.ChannelCredentialModeAccountPool,
			AccountPoolEnabled: true,
		},
	}
	require.NoError(t, db.Create(&channel).Error)
	cheap := model.ChannelAccount{
		ChannelId:    channel.Id,
		Name:         "cheap-key",
		Key:          "sk-cheap",
		Status:       common.ChannelStatusEnabled,
		Models:       "gpt-channel",
		AccessGroups: "default",
		Priority:     1,
		Weight:       150,
		OtherSettings: `{"upstream_account_sync":{"platform":"new-api","base_url":"https://upstream.example",
			"external_id":"cheap","key_digest":"cheap","ratio_conversion":0.5}}`,
	}
	expensive := model.ChannelAccount{
		ChannelId:    channel.Id,
		Name:         "expensive-key",
		Key:          "sk-expensive",
		Status:       common.ChannelStatusEnabled,
		Models:       "gpt-channel",
		AccessGroups: "default",
		Priority:     1,
		Weight:       80,
		OtherSettings: `{"upstream_account_sync":{"platform":"new-api","base_url":"https://upstream.example",
			"external_id":"expensive","key_digest":"expensive","ratio_conversion":1.2}}`,
	}
	higherPriority := model.ChannelAccount{
		ChannelId:    channel.Id,
		Name:         "higher-priority-key",
		Key:          "sk-priority",
		Status:       common.ChannelStatusEnabled,
		Models:       "gpt-channel",
		AccessGroups: "default",
		Priority:     2,
		Weight:       10,
		OtherSettings: `{"upstream_account_sync":{"platform":"new-api","base_url":"https://upstream.example",
			"external_id":"priority","key_digest":"priority","ratio_conversion":2}}`,
	}
	manual := model.ChannelAccount{
		ChannelId:    channel.Id,
		Name:         "manual-high-weight-key",
		Key:          "sk-manual",
		Status:       common.ChannelStatusEnabled,
		Models:       "gpt-channel",
		AccessGroups: "default",
		Priority:     9,
		Weight:       999,
	}
	require.NoError(t, db.Create(&cheap).Error)
	require.NoError(t, db.Create(&expensive).Error)
	require.NoError(t, db.Create(&higherPriority).Error)
	require.NoError(t, db.Create(&manual).Error)

	selected, err := SelectChannelAccount(nil, &channel, "gpt-channel", "default", 0)
	require.NoError(t, err)
	require.Equal(t, higherPriority.Id, selected.Id)

	require.NoError(t, db.Model(&model.ChannelAccount{}).Where("id = ?", higherPriority.Id).Update("status", common.ChannelStatusManuallyDisabled).Error)
	selected, err = SelectChannelAccount(nil, &channel, "gpt-channel", "default", 0)
	require.NoError(t, err)
	require.Equal(t, cheap.Id, selected.Id)
}

func TestProcessChannelAccountErrorDisablesSyncedKeyAfterThreshold(t *testing.T) {
	db := setupChannelAccountSelectTestDB(t)
	withChannelAccountKeyCheckThreshold(t, 3)
	channel, account := createSyncedChannelAccountFixture(t, db, common.ChannelStatusEnabled, `{"upstream_account_sync":{"platform":"new-api","base_url":"https://upstream.example","external_id":"key-1","key_digest":"digest","ratio_conversion":0.2,"auto_check_failure_count":2}}`)
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	upstreamErr := types.NewOpenAIError(errors.New("upstream rejected sk-secret"), types.ErrorCodeBadResponse, http.StatusBadGateway)

	ProcessChannelAccountError(ctx, types.ChannelError{
		ChannelId:        channel.Id,
		ChannelAccountId: account.Id,
		AutoBan:          true,
	}, upstreamErr)

	var stored model.ChannelAccount
	require.NoError(t, db.First(&stored, account.Id).Error)
	require.Equal(t, common.ChannelStatusAutoDisabled, stored.Status)
	require.NotContains(t, stored.LastError, "sk-secret")
	metadata := readSyncedChannelAccountAutoCheckMetadata(stored.OtherSettings)
	require.Equal(t, 3, metadata.FailureCount)
	require.True(t, metadata.DisabledByAutoCheck)
	require.NotContains(t, metadata.LastError, "sk-secret")
	require.True(t, ConsumeSyncedChannelAccountAutoDisabledRetrySignal(ctx))
	require.False(t, ConsumeSyncedChannelAccountAutoDisabledRetrySignal(ctx))
}

func TestMarkSelectedChannelAccountRequestSuccessClearsSyncedKeyFailure(t *testing.T) {
	db := setupChannelAccountSelectTestDB(t)
	channel, account := createSyncedChannelAccountFixture(t, db, common.ChannelStatusEnabled, `{"upstream_account_sync":{"platform":"new-api","base_url":"https://upstream.example","external_id":"key-1","key_digest":"digest","ratio_conversion":0.2,"auto_check_failure_count":1,"auto_check_last_error":"previous","auto_check_last_status":"failed"}}`)
	require.NoError(t, db.Model(&model.ChannelAccount{}).Where("id = ?", account.Id).Update("last_error", "previous").Error)
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyChannelAccountPool, true)
	common.SetContextKey(ctx, constant.ContextKeyChannelId, channel.Id)
	common.SetContextKey(ctx, constant.ContextKeyChannelAccountId, account.Id)

	MarkSelectedChannelAccountRequestSuccess(ctx)

	var stored model.ChannelAccount
	require.NoError(t, db.First(&stored, account.Id).Error)
	require.Equal(t, "", stored.LastError)
	metadata := readSyncedChannelAccountAutoCheckMetadata(stored.OtherSettings)
	require.Equal(t, 0, metadata.FailureCount)
	require.Equal(t, "", metadata.LastError)
	require.Equal(t, "success", metadata.LastStatus)
	require.NotZero(t, metadata.LastSuccessAt)
}

func withChannelAccountKeyCheckThreshold(t *testing.T, threshold int) {
	t.Helper()
	setting := operation_setting.GetUpstreamAccountKeyCheckSetting()
	old := *setting
	setting.FailureThreshold = threshold
	t.Cleanup(func() {
		*setting = old
	})
}

func createSyncedChannelAccountFixture(t *testing.T, db *gorm.DB, status int, settings string) (model.Channel, model.ChannelAccount) {
	t.Helper()
	channel := model.Channel{
		Type:          constant.ChannelTypeOpenAI,
		Status:        common.ChannelStatusEnabled,
		Name:          "synced-channel",
		Models:        "gpt-channel",
		Group:         "default",
		OtherSettings: `{"upstream_account_sync":{"platform":"new-api","base_url":"https://upstream.example"}}`,
		ChannelInfo: model.ChannelInfo{
			CredentialMode:     constant.ChannelCredentialModeAccountPool,
			AccountPoolEnabled: true,
		},
	}
	require.NoError(t, db.Create(&channel).Error)
	account := model.ChannelAccount{
		ChannelId:     channel.Id,
		Name:          "synced-key",
		Key:           "sk-secret",
		Status:        status,
		Models:        "gpt-channel",
		AccessGroups:  "default",
		OtherSettings: settings,
	}
	require.NoError(t, db.Create(&account).Error)
	return channel, account
}
