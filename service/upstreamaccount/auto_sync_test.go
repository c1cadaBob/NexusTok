package upstreamaccount

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelAccount{}, &model.Ability{}, &model.Task{}))
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
	require.NotNil(t, configs[0].Models)
	require.Equal(t, account.Models, *configs[0].Models)
	require.Equal(t, account.Group, configs[0].Group)
	require.NotNil(t, configs[0].Priority)
	require.Equal(t, account.Priority, *configs[0].Priority)
	require.NotNil(t, configs[0].Weight)
	require.Equal(t, account.Weight, *configs[0].Weight)
	require.Nil(t, configs[0].Enabled)
}

func TestAutomaticAccountConfigsUsesSyncMaskForLegacyAccounts(t *testing.T) {
	setupAutomaticSyncTestDB(t)

	channel := model.Channel{
		Key:           constant.ChannelCredentialModeAccountPool,
		Name:          "legacy-synced-channel",
		Status:        common.ChannelStatusEnabled,
		OtherSettings: `{"upstream_account_sync":{"platform":"new-api","base_url":"https://upstream.example"}}`,
		ChannelInfo: model.ChannelInfo{
			CredentialMode:     constant.ChannelCredentialModeAccountPool,
			AccountPoolEnabled: true,
		},
	}
	require.NoError(t, model.DB.Create(&channel).Error)
	account := model.ChannelAccount{
		ChannelId: channel.Id,
		Key:       "sk-legacy-account-key",
	}
	require.NoError(t, model.DB.Create(&account).Error)

	configs, err := automaticAccountConfigs(channel.Id)
	require.NoError(t, err)
	require.Len(t, configs, 1)
	require.Equal(t, maskKey(account.Key), configs[0].SyncID)
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

func TestRunUpstreamAccountSyncPreservesExistingRatioConversionConfig(t *testing.T) {
	setupAutomaticSyncTestDB(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/api/v1/") && r.URL.Path != "/api/v1/auth/login" {
			require.Equal(t, "Bearer auto-token", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/api/v1/auth/login":
			var body map[string]string
			require.NoError(t, common.DecodeJson(r.Body, &body))
			require.Equal(t, "alice@example.com", body["email"])
			require.Equal(t, "secret", body["password"])
			_, _ = w.Write([]byte(`{"code":0,"data":{"access_token":"auto-token","refresh_token":"auto-refresh","expires_in":3600,"user":{"id":5,"email":"alice@example.com","balance":1}}}`))
		case "/api/v1/auth/me":
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":5,"email":"alice@example.com","balance":10}}`))
		case "/api/v1/user/profile":
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":5,"email":"alice@example.com","balance":12.5}}`))
		case "/api/v1/groups/available":
			_, _ = w.Write([]byte(`{"code":0,"data":[{"id":3,"name":"vip","platform":"openai","rate_multiplier":10}]}`))
		case "/api/v1/groups/rates":
			_, _ = w.Write([]byte(`{"code":0,"data":{"3":10}}`))
		case "/api/v1/usage/dashboard/stats":
			_, _ = w.Write([]byte(`{"code":0,"data":{"total_actual_cost":1}}`))
		case "/api/v1/keys":
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":9,"name":"auto-key","key":"sk-auto-key-new","status":"active","group_id":3,"group":{"id":3,"name":"vip"},"models":["gpt-4o"],"quota":20,"quota_used":3}],"total":1}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	channel := model.Channel{
		Key:    constant.ChannelCredentialModeAccountPool,
		Name:   "auto-ratio-channel",
		Status: common.ChannelStatusEnabled,
		OtherSettings: mergeChannelSyncMetadataWithCredential(
			"",
			&Snapshot{Platform: PlatformSub2API, BaseURL: server.URL},
			Credential{
				Platform: PlatformSub2API,
				BaseURL:  server.URL,
				Email:    "alice@example.com",
				AuthMode: AuthModePassword,
				Password: "secret",
			},
		),
		ChannelInfo: model.ChannelInfo{
			CredentialMode:     constant.ChannelCredentialModeAccountPool,
			AccountPoolEnabled: true,
		},
	}
	require.NoError(t, model.DB.Create(&channel).Error)

	oldKey := SyncedKey{
		ExternalID: "9",
		Name:       "auto-key",
		Key:        "sk-auto-key-old",
		MaskedKey:  "sk-auto-key-old",
		GroupID:    "3",
		GroupName:  "vip",
		Models:     []string{"gpt-4o"},
		GroupRatio: floatPtr(10),
	}
	existingSnapshot := &Snapshot{
		Platform: PlatformSub2API,
		BaseURL:  server.URL,
		Keys:     []SyncedKey{oldKey},
	}
	ApplyRatioConversion(existingSnapshot, RatioConversionConfig{
		PaidCNY:           1,
		PlatformUSDCredit: 1,
	})
	require.NoError(t, model.DB.Create(&model.ChannelAccount{
		ChannelId:     channel.Id,
		Name:          "auto-key",
		Key:           "sk-auto-key-old",
		Status:        common.ChannelStatusEnabled,
		Models:        "gpt-4o",
		Group:         "vip",
		AccessGroups:  "default",
		OtherSettings: mergeAccountSyncMetadata("", existingSnapshot, existingSnapshot.Keys[0]),
	}).Error)

	summary, err := RunUpstreamAccountSync(
		context.Background(),
		nil,
		WithSystemTaskLog("systask_sync_test", 100),
	)
	require.NoError(t, err)
	require.Equal(t, 1, summary.EligibleChannels)
	require.Equal(t, 1, summary.SucceededChannels)
	require.Equal(t, 1, summary.UpdatedAccounts)

	var refreshed model.ChannelAccount
	require.NoError(t, model.DB.Where("channel_id = ?", channel.Id).First(&refreshed).Error)
	metadata := ReadAccountSyncDisplayMetadata(refreshed.OtherSettings)
	require.NotNil(t, metadata.RatioConversionConfig)
	require.Equal(t, float64(1), metadata.RatioConversionConfig.PaidCNY)
	require.Equal(t, float64(1), metadata.RatioConversionConfig.PlatformUSDCredit)
	require.InDelta(t, 10, metadata.RatioConversion, 0.000001)
}

func TestRunUpstreamAccountSyncWritesPerChannelTaskLogsAndIsIdempotent(t *testing.T) {
	setupAutomaticSyncTestDB(t)

	channels := []model.Channel{
		{
			Key:    "normal-key",
			Name:   "普通渠道",
			Status: common.ChannelStatusEnabled,
		},
		{
			Key:           constant.ChannelCredentialModeAccountPool,
			Name:          "禁用同步渠道",
			Status:        common.ChannelStatusManuallyDisabled,
			OtherSettings: `{"upstream_account_sync":{"platform":"new-api","base_url":"https://upstream.example"}}`,
			ChannelInfo: model.ChannelInfo{
				CredentialMode:     constant.ChannelCredentialModeAccountPool,
				AccountPoolEnabled: true,
			},
		},
		{
			Key:           constant.ChannelCredentialModeAccountPool,
			Name:          "缺少凭据同步渠道",
			Status:        common.ChannelStatusEnabled,
			OtherSettings: `{"upstream_account_sync":{"platform":"new-api","base_url":"https://upstream.example"}}`,
			ChannelInfo: model.ChannelInfo{
				CredentialMode:     constant.ChannelCredentialModeAccountPool,
				AccountPoolEnabled: true,
			},
		},
	}
	require.NoError(t, model.DB.Create(&channels).Error)

	run := func() {
		_, err := RunUpstreamAccountSync(
			context.Background(),
			nil,
			WithSystemTaskLog("systask_idempotent_test", 200),
		)
		require.NoError(t, err)
	}
	run()
	run()

	var tasks []model.Task
	require.NoError(t, model.DB.
		Where("task_id = ? AND action = ?", "systask_idempotent_test", constant.TaskActionUpstreamAccountSync).
		Order("channel_id").
		Find(&tasks).Error)
	require.Len(t, tasks, 2)
	require.Equal(t, model.TaskStatusSkipped, tasks[0].Status)
	require.Equal(t, model.TaskStatusSkipped, tasks[1].Status)
	require.Equal(t, constant.TaskPlatformSystem, tasks[0].Platform)
	require.Equal(t, 0, tasks[0].UserId)
	require.Equal(t, tasks[0].TaskID, tasks[1].TaskID)
	require.Equal(t, channels[1].Id, tasks[0].ChannelId)
	require.Equal(t, channels[2].Id, tasks[1].ChannelId)
	require.Contains(t, tasks[0].FailReason, "渠道已禁用")
	require.Contains(t, tasks[1].FailReason, "没有保存上游账号凭据")

	logs := model.TaskGetAllTasks(0, 20, model.SyncTaskQueryParams{
		TaskID: "systask_idempotent_test",
	})
	require.Len(t, logs, 2)
	channelNames := make(map[int]string, len(logs))
	for _, log := range logs {
		channelNames[log.ChannelId] = log.ChannelName
	}
	require.Equal(t, "禁用同步渠道", channelNames[channels[1].Id])
	require.Equal(t, "缺少凭据同步渠道", channelNames[channels[2].Id])
}

func TestRunUpstreamAccountSyncMarksPendingChannelLogsWhenCanceled(t *testing.T) {
	setupAutomaticSyncTestDB(t)

	channel := model.Channel{
		Key:           constant.ChannelCredentialModeAccountPool,
		Name:          "取消同步渠道",
		Status:        common.ChannelStatusEnabled,
		OtherSettings: `{"upstream_account_sync":{"platform":"new-api","base_url":"https://upstream.example"}}`,
		ChannelInfo: model.ChannelInfo{
			CredentialMode:     constant.ChannelCredentialModeAccountPool,
			AccountPoolEnabled: true,
		},
	}
	require.NoError(t, model.DB.Create(&channel).Error)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := RunUpstreamAccountSync(ctx, nil, WithSystemTaskLog("systask_canceled_test", 300))
	require.ErrorIs(t, err, context.Canceled)

	var task model.Task
	require.NoError(t, model.DB.
		Where("task_id = ? AND channel_id = ?", "systask_canceled_test", channel.Id).
		First(&task).Error)
	require.Equal(t, model.TaskStatus("FAILURE"), task.Status)
	require.Equal(t, "100%", task.Progress)
	require.Contains(t, task.FailReason, "context canceled")
}
