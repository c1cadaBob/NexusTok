package upstreamaccount

import (
	"context"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAutomaticSyncTestDB(t *testing.T) {
	t.Helper()
	originDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelAccount{}, &model.Ability{}))
	model.DB = db
	t.Cleanup(func() {
		model.DB = originDB
	})
}

func TestAutomaticAccountConfigsPreserveLocalSettings(t *testing.T) {
	setupAutomaticSyncTestDB(t)

	channel := model.Channel{
		Key:           constant.ChannelCredentialModeAccountPool,
		Name:          "synced-channel",
		Status:        common.ChannelStatusEnabled,
		OtherSettings: `{"upstream_account_sync":{"platform":"new-api","base_url":"https://upstream.example"}}`,
		ChannelInfo: model.ChannelInfo{
			CredentialMode:     constant.ChannelCredentialModeAccountPool,
			AccountPoolEnabled: true,
		},
	}
	require.NoError(t, model.DB.Create(&channel).Error)

	account := model.ChannelAccount{
		ChannelId:     channel.Id,
		Name:          "local-name",
		Key:           "sk-local-key",
		Status:        common.ChannelStatusManuallyDisabled,
		Models:        "local-model",
		Group:         "local-group",
		Priority:      17,
		Weight:        23,
		OtherSettings: `{"upstream_account_sync":{"external_id":"external-1","key_digest":"legacy-digest"}}`,
	}
	require.NoError(t, model.DB.Create(&account).Error)

	configs, err := automaticAccountConfigs(channel.Id)
	require.NoError(t, err)
	require.Len(t, configs, 1)
	require.Equal(t, "external-1", configs[0].SyncID)
	require.Equal(t, "external-1", configs[0].ExternalID)
	require.Equal(t, account.Models, configs[0].Models)
	require.Equal(t, account.Group, configs[0].Group)
	require.NotNil(t, configs[0].Priority)
	require.Equal(t, account.Priority, *configs[0].Priority)
	require.NotNil(t, configs[0].Weight)
	require.Equal(t, account.Weight, *configs[0].Weight)
	require.Nil(t, configs[0].Enabled)
}

func TestRunUpstreamAccountSyncSkipsIneligibleChannels(t *testing.T) {
	setupAutomaticSyncTestDB(t)

	channels := []model.Channel{
		{
			Key:    "普通 key",
			Name:   "普通渠道",
			Status: common.ChannelStatusEnabled,
		},
		{
			Key:           constant.ChannelCredentialModeAccountPool,
			Name:          "禁用同步渠道",
			Status:        common.ChannelStatusManuallyDisabled,
			OtherSettings: `{"upstream_account_sync":{"platform":"new-api","base_url":"https://upstream.example"}}`,
			ChannelInfo: model.ChannelInfo{
				CredentialMode: constant.ChannelCredentialModeAccountPool,
			},
		},
		{
			Key:           constant.ChannelCredentialModeAccountPool,
			Name:          "无凭据同步渠道",
			Status:        common.ChannelStatusEnabled,
			OtherSettings: `{"upstream_account_sync":{"platform":"new-api","base_url":"https://upstream.example"}}`,
			ChannelInfo: model.ChannelInfo{
				CredentialMode: constant.ChannelCredentialModeAccountPool,
			},
		},
	}
	require.NoError(t, model.DB.Create(&channels).Error)

	progress := make([][2]int, 0)
	summary, err := RunUpstreamAccountSync(context.Background(), func(processed, total int) {
		progress = append(progress, [2]int{processed, total})
	})
	require.NoError(t, err)
	require.Equal(t, 3, summary.ScannedChannels)
	require.Equal(t, 0, summary.EligibleChannels)
	require.Equal(t, 3, summary.SkippedChannels)
	require.Equal(t, 0, summary.FailedChannels)
	require.NotEmpty(t, progress)
	require.Equal(t, [2]int{3, 3}, progress[len(progress)-1])
}
