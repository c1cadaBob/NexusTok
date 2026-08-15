package service

import (
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
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
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelAccount{}))
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
