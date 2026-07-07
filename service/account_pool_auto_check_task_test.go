package service

import (
	"testing"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/stretchr/testify/require"
)

func TestRunAccountPoolAutoCheckOnceStartsDueGroupTask(t *testing.T) {
	setupAccountPoolCheckTest(t)
	now := common.GetTimestamp()
	group := &model.AccountPoolGroup{
		Name:                     "auto-check-due",
		Platform:                 "codex",
		AuthType:                 model.AccountPoolAuthTypeAPIKey,
		Source:                   model.AccountPoolGroupSourceNative,
		Status:                   common.ChannelStatusEnabled,
		AutoCheckEnabled:         true,
		AutoCheckIntervalMinutes: 30,
		AutoCheckLimit:           2,
		AutoCheckNextTime:        now - 1,
	}
	require.NoError(t, model.DB.Create(group).Error)
	accounts := []*model.PoolAccount{
		{
			PoolGroupId:        group.Id,
			Name:               "auto-check-ok-a",
			Platform:           "codex",
			AuthType:           model.AccountPoolAuthTypeAPIKey,
			CredentialProvider: "codex",
			Credentials:        encryptedCheckCredential(t, `{"api_key":"sk-auto-a"}`),
			Status:             common.ChannelStatusEnabled,
			Schedulable:        true,
		},
		{
			PoolGroupId:        group.Id,
			Name:               "auto-check-ok-b",
			Platform:           "codex",
			AuthType:           model.AccountPoolAuthTypeAPIKey,
			CredentialProvider: "codex",
			Credentials:        encryptedCheckCredential(t, `{"api_key":"sk-auto-b"}`),
			Status:             common.ChannelStatusEnabled,
			Schedulable:        true,
		},
		{
			PoolGroupId:        group.Id,
			Name:               "auto-check-ok-c",
			Platform:           "codex",
			AuthType:           model.AccountPoolAuthTypeAPIKey,
			CredentialProvider: "codex",
			Credentials:        encryptedCheckCredential(t, `{"api_key":"sk-auto-c"}`),
			Status:             common.ChannelStatusEnabled,
			Schedulable:        true,
		},
	}
	require.NoError(t, model.DB.Create(&accounts).Error)

	runAccountPoolAutoCheckOnce()

	var updated model.AccountPoolGroup
	require.NoError(t, model.DB.Where("id = ?", group.Id).First(&updated).Error)
	require.NotZero(t, updated.AutoCheckLastTaskId)
	require.GreaterOrEqual(t, updated.AutoCheckLastTime, now)
	require.GreaterOrEqual(t, updated.AutoCheckNextTime, now+int64(30*time.Minute/time.Second)-1)

	task, err := GetPoolAccountCheckTask(updated.AutoCheckLastTaskId)
	require.NoError(t, err)
	require.Equal(t, accountPoolAutoCheckActor, task.Actor)
	require.Contains(t, task.RequestID, "account-pool-auto-check-")
	require.Equal(t, 2, task.Total)
	require.Equal(t, []int{accounts[0].Id, accounts[1].Id}, task.AccountIDs)
	require.Equal(t, model.PoolAccountCheckTaskStatusQueued, task.Status)

	systemTask, err := model.GetActiveSystemTaskByActiveKey(poolAccountCheckSystemTaskActiveKey(task.ID))
	require.NoError(t, err)
	require.NotNil(t, systemTask)
	require.Equal(t, model.SystemTaskTypeAccountPoolCheck, systemTask.Type)
}

func TestRunAccountPoolAutoCheckOnceSkipsDisabledAndNotDueGroups(t *testing.T) {
	setupAccountPoolCheckTest(t)
	now := common.GetTimestamp()
	disabled := &model.AccountPoolGroup{
		Name:                     "auto-check-disabled",
		Platform:                 "codex",
		AuthType:                 model.AccountPoolAuthTypeAPIKey,
		Source:                   model.AccountPoolGroupSourceNative,
		Status:                   common.ChannelStatusEnabled,
		AutoCheckEnabled:         false,
		AutoCheckIntervalMinutes: 15,
		AutoCheckLimit:           1,
		AutoCheckNextTime:        now - 1,
	}
	notDue := &model.AccountPoolGroup{
		Name:                     "auto-check-not-due",
		Platform:                 "codex",
		AuthType:                 model.AccountPoolAuthTypeAPIKey,
		Source:                   model.AccountPoolGroupSourceNative,
		Status:                   common.ChannelStatusEnabled,
		AutoCheckEnabled:         true,
		AutoCheckIntervalMinutes: 15,
		AutoCheckLimit:           1,
		AutoCheckNextTime:        now + 3600,
	}
	require.NoError(t, model.DB.Create(disabled).Error)
	require.NoError(t, model.DB.Create(notDue).Error)

	runAccountPoolAutoCheckOnce()

	var taskCount int64
	require.NoError(t, model.DB.Model(&model.PoolAccountCheckTask{}).Count(&taskCount).Error)
	require.Zero(t, taskCount)
}

func TestRunAccountPoolAutoCheckOnceDefersEmptyGroup(t *testing.T) {
	setupAccountPoolCheckTest(t)
	now := common.GetTimestamp()
	group := &model.AccountPoolGroup{
		Name:                     "auto-check-empty",
		Platform:                 "codex",
		AuthType:                 model.AccountPoolAuthTypeAPIKey,
		Source:                   model.AccountPoolGroupSourceNative,
		Status:                   common.ChannelStatusEnabled,
		AutoCheckEnabled:         true,
		AutoCheckIntervalMinutes: 20,
		AutoCheckLimit:           10,
		AutoCheckNextTime:        now - 1,
	}
	require.NoError(t, model.DB.Create(group).Error)

	runAccountPoolAutoCheckOnce()

	var updated model.AccountPoolGroup
	require.NoError(t, model.DB.Where("id = ?", group.Id).First(&updated).Error)
	require.Zero(t, updated.AutoCheckLastTaskId)
	require.Zero(t, updated.AutoCheckLastTime)
	require.GreaterOrEqual(t, updated.AutoCheckNextTime, now+int64(20*time.Minute/time.Second)-1)

	var taskCount int64
	require.NoError(t, model.DB.Model(&model.PoolAccountCheckTask{}).Count(&taskCount).Error)
	require.Zero(t, taskCount)
}

func TestShouldRunAccountPoolAutoCheckGuardsNativeEnabledDueGroups(t *testing.T) {
	now := common.GetTimestamp()
	require.True(t, shouldRunAccountPoolAutoCheck(&model.AccountPoolGroup{
		Source:            model.AccountPoolGroupSourceNative,
		Status:            common.ChannelStatusEnabled,
		AutoCheckEnabled:  true,
		AutoCheckNextTime: now,
	}, now))
	require.False(t, shouldRunAccountPoolAutoCheck(&model.AccountPoolGroup{
		Source:            model.AccountPoolGroupSourceCLIProxyAPI,
		Status:            common.ChannelStatusEnabled,
		AutoCheckEnabled:  true,
		AutoCheckNextTime: now,
	}, now))
	require.False(t, shouldRunAccountPoolAutoCheck(&model.AccountPoolGroup{
		Source:            model.AccountPoolGroupSourceNative,
		Status:            common.ChannelStatusEnabled,
		AutoCheckEnabled:  true,
		AutoCheckNextTime: now + 60,
	}, now))
}
