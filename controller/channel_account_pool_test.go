// Package controller - channel_account_pool_test.go
// 该文件包含渠道账号池模式验证的单元测试
//
// 测试内容包括：
// - 全局账号池模式必须选择账号池组
// - 全局账号池模式允许空 Key 和 BaseURL
package controller

import (
	"errors"
	"testing"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAccountPoolChannelTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	require.NoError(t, db.AutoMigrate(&model.AccountPoolGroup{}, &model.Channel{}))
}

func TestValidateChannelGlobalAccountPoolRequiresGroup(t *testing.T) {
	setupAccountPoolChannelTestDB(t)

	channel := &model.Channel{
		Type:   constant.ChannelTypeOpenAI,
		Key:    "",
		Models: "gpt-4o-mini",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			CredentialMode: constant.ChannelCredentialModeGlobalAccountPool,
		},
	}

	err := validateChannel(channel, true)
	require.Error(t, err)
	require.Contains(t, err.Error(), "账号池模式必须选择账号池组")
}

func TestValidateChannelGlobalAccountPoolAllowsEmptyKeyAndBaseURL(t *testing.T) {
	setupAccountPoolChannelTestDB(t)

	group := &model.AccountPoolGroup{
		Name:     "codex-main",
		Platform: "codex",
		AuthType: model.AccountPoolAuthTypeOfficialOAuth,
		Source:   model.AccountPoolGroupSourceNative,
		Status:   common.ChannelStatusEnabled,
	}
	require.NoError(t, model.DB.Create(group).Error)

	channel := &model.Channel{
		Type:   constant.ChannelTypeOpenAI,
		Key:    "",
		Models: "gpt-4o-mini",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			CredentialMode:     constant.ChannelCredentialModeGlobalAccountPool,
			AccountPoolGroupId: group.Id,
		},
	}

	require.NoError(t, validateChannel(channel, true))
}

func TestAccountPoolGroupOptionResponseReturnsNativeGroupsWithAccounts(t *testing.T) {
	nativeGroup := &model.AccountPoolGroup{
		Id:       1,
		Name:     "local-only",
		Platform: "codex",
		AuthType: model.AccountPoolAuthTypeAPIKey,
		Source:   model.AccountPoolGroupSourceNative,
		Status:   common.ChannelStatusEnabled,
		Stats:    map[string]int64{"total": 3, "enabled": 3},
	}
	item, ok := accountPoolGroupOptionResponse(nativeGroup)
	require.True(t, ok)
	require.Equal(t, nativeGroup.Id, item["id"])
	require.Equal(t, model.AccountPoolGroupSourceNative, item["source"])

	legacyGroup := &model.AccountPoolGroup{
		Id:          2,
		Name:        "empty-remote",
		Platform:    "codex",
		AuthType:    model.AccountPoolAuthTypeOfficialOAuth,
		Source:      model.AccountPoolGroupSourceCLIProxyAPI,
		ExternalKey: "empty-remote",
		Status:      common.ChannelStatusEnabled,
		Stats:       map[string]int64{"total": 0, "enabled": 0},
	}
	item, ok = accountPoolGroupOptionResponse(legacyGroup)
	require.False(t, ok)
	require.Nil(t, item)

	activeLegacyGroup := &model.AccountPoolGroup{
		Id:          4,
		Name:        "remote-main",
		Platform:    "codex",
		AuthType:    model.AccountPoolAuthTypeOfficialOAuth,
		Source:      model.AccountPoolGroupSourceCLIProxyAPI,
		ExternalKey: "remote-main",
		Status:      common.ChannelStatusEnabled,
		Stats:       map[string]int64{"total": 2, "enabled": 1},
	}
	item, ok = accountPoolGroupOptionResponse(activeLegacyGroup)
	require.False(t, ok)
	require.Nil(t, item)

	activeNativeGroup := &model.AccountPoolGroup{
		Id:          3,
		Name:        "native-main",
		Platform:    "codex",
		AuthType:    model.AccountPoolAuthTypeOfficialOAuth,
		Source:      model.AccountPoolGroupSourceNative,
		ExternalKey: "",
		Status:      common.ChannelStatusEnabled,
		Stats:       map[string]int64{"total": 2, "enabled": 1},
	}
	item, ok = accountPoolGroupOptionResponse(activeNativeGroup)
	require.True(t, ok)
	require.Equal(t, activeNativeGroup.Id, item["id"])
	require.Equal(t, model.AccountPoolGroupSourceNative, item["source"])
}

func TestAccountPoolGroupResponseIncludesDailyLimitState(t *testing.T) {
	now := time.Now()
	group := &model.AccountPoolGroup{
		Id:                8,
		Name:              "daily-request-limited",
		Platform:          "codex",
		AuthType:          model.AccountPoolAuthTypeAPIKey,
		Source:            model.AccountPoolGroupSourceNative,
		Status:            common.ChannelStatusEnabled,
		DailyRequestLimit: 2,
		DailyRequestCount: 2,
		DailyQuotaLimit:   10,
		DailyUsedQuota:    3,
		DailyLimitAction:  model.AccountPoolDailyLimitActionDisable,
		DailyResetTime:    model.AccountPoolDailyWindowStart(now),
	}

	item := accountPoolGroupResponse(group)
	state, ok := item["daily_limit_state"].(model.AccountPoolDailyLimitState)
	require.True(t, ok)
	require.True(t, state.Limited)
	require.Equal(t, model.AccountPoolDailyLimitTypeRequest, state.LimitType)
	require.Equal(t, model.AccountPoolGroupDailyRequestLimitStatusMessage, state.Reason)
	require.Greater(t, state.NextResetTime, state.WindowStart)
	require.EqualValues(t, 2, item["daily_request_count"])
	require.EqualValues(t, 3, item["daily_used_quota"])
	require.Equal(t, model.AccountPoolDailyLimitActionDisable, item["daily_limit_action"])
}

func TestAccountPoolGroupResponseIncludesAutoCheckSettings(t *testing.T) {
	group := &model.AccountPoolGroup{
		Id:                       18,
		Name:                     "auto-check-response",
		Platform:                 "codex",
		AuthType:                 model.AccountPoolAuthTypeAPIKey,
		Source:                   model.AccountPoolGroupSourceNative,
		Status:                   common.ChannelStatusEnabled,
		AutoCheckEnabled:         true,
		AutoCheckIntervalMinutes: 15,
		AutoCheckLimit:           3,
		AutoCheckLastTime:        1700000000,
		AutoCheckNextTime:        1700000900,
		AutoCheckLastTaskId:      99,
	}

	item := accountPoolGroupResponse(group)
	require.Equal(t, true, item["auto_check_enabled"])
	require.Equal(t, 15, item["auto_check_interval_minutes"])
	require.Equal(t, 3, item["auto_check_limit"])
	require.EqualValues(t, 1700000000, item["auto_check_last_time"])
	require.EqualValues(t, 1700000900, item["auto_check_next_time"])
	require.Equal(t, 99, item["auto_check_last_task_id"])

	option, ok := accountPoolGroupOptionResponse(&model.AccountPoolGroup{
		Id:                       19,
		Name:                     "auto-check-option",
		Platform:                 "codex",
		AuthType:                 model.AccountPoolAuthTypeAPIKey,
		Source:                   model.AccountPoolGroupSourceNative,
		Status:                   common.ChannelStatusEnabled,
		AutoCheckEnabled:         true,
		AutoCheckIntervalMinutes: 20,
		AutoCheckLimit:           5,
		AutoCheckLastTaskId:      100,
		Stats:                    map[string]int64{"total": 1, "enabled": 1},
	})
	require.True(t, ok)
	require.Equal(t, true, option["auto_check_enabled"])
	require.Equal(t, 20, option["auto_check_interval_minutes"])
	require.Equal(t, 5, option["auto_check_limit"])
	require.Equal(t, 100, option["auto_check_last_task_id"])
}

func TestAccountPoolGroupResponseIncludesPreflightCheckSettings(t *testing.T) {
	group := &model.AccountPoolGroup{
		Id:                             20,
		Name:                           "preflight-response",
		Platform:                       "codex",
		AuthType:                       model.AccountPoolAuthTypeAPIKey,
		Source:                         model.AccountPoolGroupSourceNative,
		Status:                         common.ChannelStatusEnabled,
		PreflightCheckMode:             model.AccountPoolPreflightCheckModeRequireRecent,
		PreflightCheckFreshnessMinutes: 30,
		PreflightCheckLimit:            4,
	}

	item := accountPoolGroupResponse(group)
	require.Equal(t, model.AccountPoolPreflightCheckModeRequireRecent, item["preflight_check_mode"])
	require.Equal(t, 30, item["preflight_check_freshness_minutes"])
	require.Equal(t, 4, item["preflight_check_limit"])

	option, ok := accountPoolGroupOptionResponse(&model.AccountPoolGroup{
		Id:                             21,
		Name:                           "preflight-option",
		Platform:                       "codex",
		AuthType:                       model.AccountPoolAuthTypeAPIKey,
		Source:                         model.AccountPoolGroupSourceNative,
		Status:                         common.ChannelStatusEnabled,
		PreflightCheckMode:             model.AccountPoolPreflightCheckModeWarmup,
		PreflightCheckFreshnessMinutes: 90,
		PreflightCheckLimit:            6,
		Stats:                          map[string]int64{"total": 1, "enabled": 1},
	})
	require.True(t, ok)
	require.Equal(t, model.AccountPoolPreflightCheckModeWarmup, option["preflight_check_mode"])
	require.Equal(t, 90, option["preflight_check_freshness_minutes"])
	require.Equal(t, 6, option["preflight_check_limit"])
}

func TestPoolAccountResponseIncludesDailyLimitAction(t *testing.T) {
	account := &model.PoolAccount{
		Id:               12,
		PoolGroupId:      8,
		Name:             "daily-limit-action-account",
		Platform:         "codex",
		AuthType:         model.AccountPoolAuthTypeAPIKey,
		Status:           common.ChannelStatusEnabled,
		Schedulable:      true,
		DailyLimitAction: model.AccountPoolDailyLimitActionDisable,
	}

	item := poolAccountResponse(account)
	require.Equal(t, model.AccountPoolDailyLimitActionDisable, item["daily_limit_action"])
}

func TestAccountPoolGroupResponseUsesCurrentDailyWindowForStaleUsage(t *testing.T) {
	now := time.Now()
	group := &model.AccountPoolGroup{
		Id:                9,
		Name:              "stale-daily-limit",
		Platform:          "codex",
		AuthType:          model.AccountPoolAuthTypeAPIKey,
		Source:            model.AccountPoolGroupSourceNative,
		Status:            common.ChannelStatusEnabled,
		DailyRequestLimit: 1,
		DailyRequestCount: 1,
		DailyQuotaLimit:   10,
		DailyUsedQuota:    10,
		DailyResetTime:    model.AccountPoolDailyWindowStart(now.Add(-24 * time.Hour)),
	}

	item := accountPoolGroupResponse(group)
	state, ok := item["daily_limit_state"].(model.AccountPoolDailyLimitState)
	require.True(t, ok)
	require.False(t, state.Limited)
	require.Empty(t, state.LimitType)
	require.Equal(t, model.AccountPoolDailyWindowStart(now), state.WindowStart)
	require.EqualValues(t, 0, item["daily_request_count"])
	require.EqualValues(t, 0, item["daily_used_quota"])
	require.EqualValues(t, model.AccountPoolDailyWindowStart(now), item["daily_reset_time"])
}

func TestAccountPoolGroupRequestSettingsDropsLegacyMaxConcurrencyWhenExplicit(t *testing.T) {
	zero := 0
	settings := accountPoolGroupRequestSettings(accountPoolGroupUpsertRequest{
		Settings:       `{"max_concurrency":2,"retry_interval":30}`,
		MaxConcurrency: &zero,
	})

	require.JSONEq(t, `{"retry_interval":30}`, settings)

	group, err := buildAccountPoolGroupFromRequest(accountPoolGroupUpsertRequest{
		Name:           "native-limit",
		Platform:       "codex",
		Settings:       `{"max_concurrency":2}`,
		MaxConcurrency: &zero,
	})

	require.NoError(t, err)
	require.Equal(t, 0, group.MaxConcurrency)
	require.Empty(t, group.Settings)
	require.Equal(t, 0, group.GetMaxConcurrency())
}

func TestAccountPoolGroupRequestSettingsKeepsLegacyMaxConcurrencyWhenFieldAbsent(t *testing.T) {
	group, err := buildAccountPoolGroupFromRequest(accountPoolGroupUpsertRequest{
		Name:     "legacy-limit",
		Platform: "codex",
		Settings: `{"max_concurrency":2}`,
	})

	require.NoError(t, err)
	require.Equal(t, 0, group.MaxConcurrency)
	require.JSONEq(t, `{"max_concurrency":2}`, group.Settings)
	require.Equal(t, 2, group.GetMaxConcurrency())
}

func TestAccountPoolGroupRequestNormalizesAutoCheckSettings(t *testing.T) {
	enabled := true
	tooLargeLimit := 200
	zeroInterval := 0
	group, err := buildAccountPoolGroupFromRequest(accountPoolGroupUpsertRequest{
		Name:                     "auto-check-request",
		Platform:                 "codex",
		AutoCheckEnabled:         &enabled,
		AutoCheckIntervalMinutes: &zeroInterval,
		AutoCheckLimit:           &tooLargeLimit,
	})

	require.NoError(t, err)
	require.True(t, group.AutoCheckEnabled)
	require.Equal(t, model.AccountPoolAutoCheckDefaultIntervalMinutes, group.AutoCheckIntervalMinutes)
	require.Equal(t, model.AccountPoolAutoCheckMaxLimit, group.AutoCheckLimit)

	disabled := false
	negativeLimit := -1
	updates, err := accountPoolGroupUpdateMap(accountPoolGroupUpsertRequest{
		AutoCheckEnabled: &disabled,
		AutoCheckLimit:   &negativeLimit,
	})
	require.NoError(t, err)
	require.Equal(t, false, updates["auto_check_enabled"])
	require.Equal(t, model.AccountPoolAutoCheckDefaultLimit, updates["auto_check_limit"])
}

func TestAccountPoolGroupRequestNormalizesPreflightCheckSettings(t *testing.T) {
	tooLargeLimit := 200
	zeroFreshness := 0
	group, err := buildAccountPoolGroupFromRequest(accountPoolGroupUpsertRequest{
		Name:                           "preflight-request",
		Platform:                       "codex",
		PreflightCheckMode:             model.AccountPoolPreflightCheckModeRequireRecent,
		PreflightCheckFreshnessMinutes: &zeroFreshness,
		PreflightCheckLimit:            &tooLargeLimit,
	})

	require.NoError(t, err)
	require.Equal(t, model.AccountPoolPreflightCheckModeRequireRecent, group.PreflightCheckMode)
	require.Equal(t, model.AccountPoolPreflightCheckDefaultFreshnessMinutes, group.PreflightCheckFreshnessMinutes)
	require.Equal(t, model.AccountPoolPreflightCheckMaxLimit, group.PreflightCheckLimit)

	negativeLimit := -1
	freshness := 45
	updates, err := accountPoolGroupUpdateMap(accountPoolGroupUpsertRequest{
		PreflightCheckMode:             model.AccountPoolPreflightCheckModeWarmup,
		PreflightCheckFreshnessMinutes: &freshness,
		PreflightCheckLimit:            &negativeLimit,
	})
	require.NoError(t, err)
	require.Equal(t, model.AccountPoolPreflightCheckModeWarmup, updates["preflight_check_mode"])
	require.Equal(t, 45, updates["preflight_check_freshness_minutes"])
	require.Equal(t, model.AccountPoolPreflightCheckDefaultLimit, updates["preflight_check_limit"])
}

func TestValidateChannelGlobalAccountPoolRejectsCLIProxyGroup(t *testing.T) {
	setupAccountPoolChannelTestDB(t)

	group := &model.AccountPoolGroup{
		Name:        "remote-only",
		Platform:    "codex",
		AuthType:    model.AccountPoolAuthTypeAPIKey,
		Source:      model.AccountPoolGroupSourceCLIProxyAPI,
		ExternalKey: "remote-only",
		Status:      common.ChannelStatusEnabled,
	}
	require.NoError(t, model.DB.Create(group).Error)

	channel := &model.Channel{
		Type:   constant.ChannelTypeOpenAI,
		Key:    "",
		Models: "gpt-4o-mini",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			CredentialMode:     constant.ChannelCredentialModeGlobalAccountPool,
			AccountPoolGroupId: group.Id,
		},
	}

	err := validateChannel(channel, true)
	require.Error(t, err)
	require.Contains(t, err.Error(), "账号池模式只能选择原生账号池组")
}

func TestFormatChannelTestFailureMessageForGlobalAccountPoolAuthUnavailable(t *testing.T) {
	channel := &model.Channel{
		Type:   constant.ChannelTypeOpenAI,
		Key:    constant.ChannelCredentialModeGlobalAccountPool,
		Models: "gpt-5.4,gpt-5.5",
		ChannelInfo: model.ChannelInfo{
			CredentialMode:     constant.ChannelCredentialModeGlobalAccountPool,
			AccountPoolGroupId: 2,
		},
	}

	message := formatChannelTestFailureMessage(
		channel,
		"gpt-5.5",
		errors.New(`bad response status code 401, message: {"detail":"Unauthorized"}, body: {"error":{"message":"{\"detail\":\"Unauthorized\"}","type":"authentication_error","code":"auth_unavailable"}}`),
	)

	require.Contains(t, message, "gpt-5.5")
	require.Contains(t, message, "全局账号池")
	require.Contains(t, message, "账号池组 ID：2")
	require.Contains(t, message, "重新登录或刷新授权")
	require.NotContains(t, message, "bad response status code")
	require.NotContains(t, message, "auth_unavailable")
}
