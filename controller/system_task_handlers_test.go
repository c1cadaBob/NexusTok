package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/service/upstreamaccount"
	"github.com/c1cada/NexusTok/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupSystemTaskHandlerTestDB(t *testing.T) {
	t.Helper()
	originDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Channel{},
		&model.ChannelAccount{},
		&model.Ability{},
		&model.SystemTask{},
		&model.SystemTaskLock{},
	))
	model.DB = db
	t.Cleanup(func() {
		model.DB = originDB
	})
}

func upstreamAccountSyncSettingsForHandlerTest(t *testing.T, baseURL string) string {
	t.Helper()
	encryptedPassword, err := common.EncryptSensitiveString("secret")
	require.NoError(t, err)
	settings := map[string]any{
		"upstream_account_sync": map[string]any{
			"platform": "new-api",
			"base_url": baseURL,
			"credentials": map[string]any{
				"platform": "new-api",
				"base_url": baseURL,
				"username": "admin",
				"password": encryptedPassword,
			},
		},
	}
	bytes, err := common.Marshal(settings)
	require.NoError(t, err)
	return string(bytes)
}

func TestUpstreamAccountSyncHandlerSkipsPendingTaskWhenDisabled(t *testing.T) {
	setupSystemTaskHandlerTestDB(t)
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount.Add(1)
		http.Error(w, "automatic sync should be skipped", http.StatusInternalServerError)
	}))
	defer server.Close()

	channel := model.Channel{
		Type:          constant.ChannelTypeOpenAI,
		Key:           constant.ChannelCredentialModeAccountPool,
		Name:          "disabled-auto-sync-channel",
		Status:        common.ChannelStatusEnabled,
		OtherSettings: upstreamAccountSyncSettingsForHandlerTest(t, server.URL),
		ChannelInfo: model.ChannelInfo{
			CredentialMode:     constant.ChannelCredentialModeAccountPool,
			AccountPoolEnabled: true,
		},
	}
	require.NoError(t, model.DB.Create(&channel).Error)

	setting := operation_setting.GetUpstreamAccountSyncSetting()
	oldSetting := *setting
	*setting = operation_setting.UpstreamAccountSyncSetting{
		Enabled:  false,
		Interval: 1,
		Unit:     operation_setting.UpstreamAccountSyncUnitHour,
	}
	t.Cleanup(func() {
		*setting = oldSetting
	})

	task, err := model.CreateSystemTask(model.SystemTaskTypeUpstreamAccountSync, nil, nil)
	require.NoError(t, err)
	claimedTask, claimed, err := model.ClaimSystemTask(
		task.ID,
		model.SystemTaskTypeUpstreamAccountSync,
		"runner-upstream-account-sync",
		common.GetTimestamp()+60,
	)
	require.NoError(t, err)
	require.True(t, claimed)

	upstreamAccountSyncHandler{}.Run(context.Background(), claimedTask, "runner-upstream-account-sync")

	finished, err := model.GetSystemTaskByTaskID(task.TaskID)
	require.NoError(t, err)
	require.NotNil(t, finished)
	require.Equal(t, model.SystemTaskStatusSucceeded, finished.Status)
	require.Nil(t, finished.ActiveKey)
	require.Equal(t, int32(0), requestCount.Load())

	var summary upstreamaccount.UpstreamAccountSyncSummary
	require.NoError(t, finished.DecodeResult(&summary))
	require.True(t, summary.Skipped)
	require.Equal(t, "上游账号自动同步已关闭", summary.SkipReason)
	require.Zero(t, summary.ScannedChannels)
}
