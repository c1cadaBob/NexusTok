package model

import (
	"testing"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAccountPoolHealthModelTest(t *testing.T) {
	t.Helper()
	oldDB := DB
	oldLogDB := LOG_DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	LOG_DB = db
	require.NoError(t, db.AutoMigrate(
		&AccountPoolGroup{},
		&PoolAccount{},
		&PoolAccountUsageLog{},
		&PoolAccountStateLog{},
	))
	t.Cleanup(func() {
		DB = oldDB
		LOG_DB = oldLogDB
	})
}

func createAccountPoolHealthGroup(t *testing.T, name string) *AccountPoolGroup {
	t.Helper()
	group := &AccountPoolGroup{
		Name:                           name,
		Platform:                       "codex",
		AuthType:                       AccountPoolAuthTypeAPIKey,
		Source:                         AccountPoolGroupSourceNative,
		Status:                         common.ChannelStatusEnabled,
		Strategy:                       AccountPoolStrategyRoundRobin,
		AutoCheckEnabled:               true,
		AutoCheckIntervalMinutes:       30,
		PreflightCheckMode:             AccountPoolPreflightCheckModeWarmup,
		PreflightCheckFreshnessMinutes: 120,
	}
	require.NoError(t, DB.Create(group).Error)
	return group
}

func createAccountPoolHealthAccount(t *testing.T, group *AccountPoolGroup, name string, updates func(*PoolAccount)) *PoolAccount {
	t.Helper()
	account := &PoolAccount{
		PoolGroupId:       group.Id,
		Name:              name,
		Platform:          group.Platform,
		AuthType:          group.AuthType,
		Credentials:       "encrypted-placeholder",
		CredentialSummary: "masked",
		Status:            common.ChannelStatusEnabled,
		Schedulable:       true,
	}
	if updates != nil {
		updates(account)
	}
	require.NoError(t, DB.Create(account).Error)
	return account
}

func createAccountPoolHealthUsageLog(t *testing.T, group *AccountPoolGroup, account *PoolAccount, createdAt int64, success bool) {
	t.Helper()
	require.NoError(t, LOG_DB.Create(&PoolAccountUsageLog{
		CreatedAt:           createdAt,
		PoolGroupId:         group.Id,
		PoolGroupName:       group.Name,
		PoolAccountId:       account.Id,
		PoolAccountName:     account.Name,
		PoolAccountAuthType: account.AuthType,
		ModelName:           "gpt-4o-mini",
		Success:             success,
	}).Error)
}

func createAccountPoolHealthStateLog(t *testing.T, group *AccountPoolGroup, account *PoolAccount, action string, createdAt int64) {
	t.Helper()
	require.NoError(t, LOG_DB.Create(&PoolAccountStateLog{
		CreatedAt:           createdAt,
		PoolGroupId:         group.Id,
		PoolGroupName:       group.Name,
		PoolAccountId:       account.Id,
		PoolAccountName:     account.Name,
		PoolAccountAuthType: account.AuthType,
		Action:              action,
		Source:              "test",
		Reason:              "health test",
		AfterStatus:         account.Status,
		AfterSchedulable:    account.Schedulable,
		AfterUnavailable:    account.Unavailable,
		AfterNextRetryTime:  account.NextRetryTime,
		AfterStatusMessage:  account.StatusMessage,
		AfterDisabledReason: account.DisabledReason,
	}).Error)
}

func accountPoolHealthGroupByName(summary *AccountPoolHealthSummary, name string) *AccountPoolGroupHealth {
	for _, group := range summary.Groups {
		if group != nil && group.Name == name {
			return group
		}
	}
	return nil
}

func accountPoolAbnormalByName(accounts []*AccountPoolAbnormalAccount, name string) *AccountPoolAbnormalAccount {
	for _, account := range accounts {
		if account != nil && account.Name == name {
			return account
		}
	}
	return nil
}

func TestGetAccountPoolHealthSummaryAggregatesNativeGroups(t *testing.T) {
	setupAccountPoolHealthModelTest(t)
	now := common.GetTimestamp()
	windowStart := AccountPoolDailyWindowStart(time.Now())
	mainGroup := createAccountPoolHealthGroup(t, "health-main")
	mainGroup.DailyRequestLimit = 10
	mainGroup.DailyRequestCount = 10
	mainGroup.DailyResetTime = windowStart
	require.NoError(t, DB.Save(mainGroup).Error)
	otherGroup := createAccountPoolHealthGroup(t, "health-other")
	legacyGroup := &AccountPoolGroup{
		Name:     "health-legacy",
		Platform: "codex",
		AuthType: AccountPoolAuthTypeAPIKey,
		Source:   "legacy-sidecar",
		Status:   common.ChannelStatusEnabled,
	}
	require.NoError(t, DB.Create(legacyGroup).Error)

	normal := createAccountPoolHealthAccount(t, mainGroup, "normal-account", func(account *PoolAccount) {
		account.SuccessCount = 8
		account.FailedCount = 2
		account.LastCheckedTime = now - 60
	})
	disabled := createAccountPoolHealthAccount(t, mainGroup, "disabled-account", func(account *PoolAccount) {
		account.Status = common.ChannelStatusManuallyDisabled
		account.Schedulable = false
		account.DisabledReason = "人工禁用"
		account.UpdatedTime = now - 3
	})
	cooling := createAccountPoolHealthAccount(t, mainGroup, "cooling-account", func(account *PoolAccount) {
		account.NextRetryTime = now + 300
		account.LastError = "今日额度耗尽"
		account.UpdatedTime = now - 2
	})
	unavailable := createAccountPoolHealthAccount(t, mainGroup, "unavailable-account", func(account *PoolAccount) {
		account.Unavailable = true
		account.StatusMessage = "登录已失效"
		account.UpdatedTime = now - 1
	})
	otherNormal := createAccountPoolHealthAccount(t, otherGroup, "other-normal", nil)
	_ = createAccountPoolHealthAccount(t, legacyGroup, "legacy-account", nil)

	createAccountPoolHealthUsageLog(t, mainGroup, normal, now-20, true)
	createAccountPoolHealthUsageLog(t, mainGroup, cooling, now-10, false)
	createAccountPoolHealthUsageLog(t, mainGroup, normal, windowStart-10, false)
	createAccountPoolHealthUsageLog(t, otherGroup, otherNormal, now-5, true)
	createAccountPoolHealthStateLog(t, mainGroup, disabled, "manual_disable", now-30)
	createAccountPoolHealthStateLog(t, otherGroup, otherNormal, "check_succeeded", now-20)

	summary, err := GetAccountPoolHealthSummary(AccountPoolHealthOptions{AbnormalLimit: 10, AuditLimit: 10})
	require.NoError(t, err)
	require.NotNil(t, summary)
	require.Len(t, summary.Groups, 2)
	require.Equal(t, 2, summary.Totals.GroupCount)
	require.Equal(t, 1, summary.Totals.LimitedGroupCount)
	require.Equal(t, int64(5), summary.Totals.TotalAccounts)
	require.Equal(t, int64(2), summary.Totals.AvailableAccounts)
	require.Equal(t, int64(2), summary.Totals.DisabledAccounts)
	require.Equal(t, int64(1), summary.Totals.CooldownAccounts)
	require.Equal(t, int64(1), summary.Totals.UnavailableAccounts)
	require.Equal(t, int64(3), summary.Totals.TodayRequests)
	require.Equal(t, int64(2), summary.Totals.TodaySuccesses)
	require.Equal(t, int64(1), summary.Totals.TodayFailures)
	require.InDelta(t, 2.0/3.0, summary.Totals.SuccessRate, 0.000001)
	require.InDelta(t, 2.0/5.0, summary.Totals.AvailabilityRate, 0.000001)

	mainHealth := accountPoolHealthGroupByName(summary, mainGroup.Name)
	require.NotNil(t, mainHealth)
	require.True(t, mainHealth.DailyLimitState.Limited)
	require.Equal(t, AccountPoolDailyLimitTypeRequest, mainHealth.DailyLimitState.LimitType)
	require.Equal(t, int64(4), mainHealth.Stats["total"])
	require.Equal(t, int64(1), mainHealth.Stats["enabled"])
	require.Equal(t, int64(2), mainHealth.TodayRequests)
	require.Equal(t, int64(1), mainHealth.TodayFailures)
	require.InDelta(t, 0.5, mainHealth.SuccessRate, 0.000001)
	require.InDelta(t, 0.25, mainHealth.AvailabilityRate, 0.000001)
	require.True(t, mainHealth.AutoCheckEnabled)
	require.Equal(t, AccountPoolPreflightCheckModeWarmup, mainHealth.PreflightCheckMode)

	require.Nil(t, accountPoolHealthGroupByName(summary, legacyGroup.Name))
	require.NotNil(t, accountPoolAbnormalByName(summary.RecentAbnormalAccounts, disabled.Name))
	require.Equal(t, "今日额度耗尽", accountPoolAbnormalByName(summary.RecentAbnormalAccounts, cooling.Name).Reason)
	require.Equal(t, "登录已失效", accountPoolAbnormalByName(summary.RecentAbnormalAccounts, unavailable.Name).Reason)
	require.Nil(t, accountPoolAbnormalByName(summary.RecentAbnormalAccounts, normal.Name))
	require.Len(t, summary.RecentStateLogs, 2)
}

func TestGetAccountPoolHealthSummaryFiltersPoolGroup(t *testing.T) {
	setupAccountPoolHealthModelTest(t)
	now := common.GetTimestamp()
	group := createAccountPoolHealthGroup(t, "filtered-main")
	otherGroup := createAccountPoolHealthGroup(t, "filtered-other")
	account := createAccountPoolHealthAccount(t, group, "filtered-account", nil)
	otherAccount := createAccountPoolHealthAccount(t, otherGroup, "filtered-other-account", nil)
	createAccountPoolHealthUsageLog(t, group, account, now-10, false)
	createAccountPoolHealthUsageLog(t, otherGroup, otherAccount, now-5, true)
	createAccountPoolHealthStateLog(t, group, account, "manual_disable", now-4)
	createAccountPoolHealthStateLog(t, otherGroup, otherAccount, "check_succeeded", now-3)

	summary, err := GetAccountPoolHealthSummary(AccountPoolHealthOptions{
		PoolGroupID:   group.Id,
		AbnormalLimit: 10,
		AuditLimit:    10,
	})
	require.NoError(t, err)
	require.Len(t, summary.Groups, 1)
	require.Equal(t, group.Id, summary.Groups[0].Id)
	require.Equal(t, int64(1), summary.Totals.TotalAccounts)
	require.Equal(t, int64(1), summary.Totals.TodayRequests)
	require.Equal(t, int64(1), summary.Totals.TodayFailures)
	require.Len(t, summary.RecentStateLogs, 1)
	require.Equal(t, group.Id, summary.RecentStateLogs[0].PoolGroupId)
}

func TestGetAccountPoolHealthSummaryUnknownGroupReturnsEmptyScope(t *testing.T) {
	setupAccountPoolHealthModelTest(t)
	now := common.GetTimestamp()
	group := createAccountPoolHealthGroup(t, "unknown-filter-source")
	account := createAccountPoolHealthAccount(t, group, "unknown-filter-account", nil)
	createAccountPoolHealthStateLog(t, group, account, "manual_disable", now-4)

	summary, err := GetAccountPoolHealthSummary(AccountPoolHealthOptions{
		PoolGroupID:   group.Id + 1000,
		AbnormalLimit: 10,
		AuditLimit:    10,
	})
	require.NoError(t, err)
	require.Empty(t, summary.Groups)
	require.Empty(t, summary.RecentAbnormalAccounts)
	require.Empty(t, summary.RecentStateLogs)
	require.Equal(t, int64(0), summary.Totals.TotalAccounts)
}
