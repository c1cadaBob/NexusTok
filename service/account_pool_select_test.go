package service

import (
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func setupAccountPoolSelectTest(t *testing.T) {
	t.Helper()
	require.NoError(t, model.DB.AutoMigrate(&model.AccountPoolGroup{}, &model.PoolAccount{}))
	require.NoError(t, model.DB.Exec("DELETE FROM pool_accounts").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM account_pool_groups").Error)
	originalRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	poolAccountConcurrencyMu.Lock()
	poolAccountConcurrency = map[int]int{}
	poolAccountConcurrencyMu.Unlock()
	poolGroupConcurrencyMu.Lock()
	poolGroupConcurrency = map[int]int{}
	poolGroupConcurrencyMu.Unlock()
	poolGroupRateMu.Lock()
	poolGroupRateCounters = map[int]poolGroupRateCounter{}
	poolGroupRateMu.Unlock()
	t.Cleanup(func() {
		common.RedisEnabled = originalRedisEnabled
		poolAccountConcurrencyMu.Lock()
		poolAccountConcurrency = map[int]int{}
		poolAccountConcurrencyMu.Unlock()
		poolGroupConcurrencyMu.Lock()
		poolGroupConcurrency = map[int]int{}
		poolGroupConcurrencyMu.Unlock()
		poolGroupRateMu.Lock()
		poolGroupRateCounters = map[int]poolGroupRateCounter{}
		poolGroupRateMu.Unlock()
		_ = model.DB.Exec("DELETE FROM pool_accounts").Error
		_ = model.DB.Exec("DELETE FROM account_pool_groups").Error
	})
}

func newPoolSelectTestContext() *gin.Context {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	return c
}

func TestSelectPoolAccountRespectsGroupMaxConcurrency(t *testing.T) {
	setupAccountPoolSelectTest(t)
	group := &model.AccountPoolGroup{
		Name:           "codex-group-concurrency",
		Platform:       "codex",
		AuthType:       model.AccountPoolAuthTypeAPIKey,
		Source:         model.AccountPoolGroupSourceNative,
		Status:         common.ChannelStatusEnabled,
		Strategy:       model.AccountPoolStrategyRoundRobin,
		Group:          "default",
		MaxConcurrency: 1,
	}
	require.NoError(t, model.DB.Create(group).Error)
	accounts := []*model.PoolAccount{
		{
			PoolGroupId: group.Id,
			Name:        "account-a",
			Platform:    "codex",
			AuthType:    model.AccountPoolAuthTypeAPIKey,
			Credentials: "placeholder-a",
			Status:      common.ChannelStatusEnabled,
			Schedulable: true,
		},
		{
			PoolGroupId: group.Id,
			Name:        "account-b",
			Platform:    "codex",
			AuthType:    model.AccountPoolAuthTypeAPIKey,
			Credentials: "placeholder-b",
			Status:      common.ChannelStatusEnabled,
			Schedulable: true,
		},
	}
	require.NoError(t, model.DB.Create(&accounts).Error)
	channel := &model.Channel{
		ChannelInfo: model.ChannelInfo{
			AccountPoolGroupId: group.Id,
		},
	}

	firstCtx := newPoolSelectTestContext()
	selectedGroup, selectedAccount, err := SelectPoolAccount(firstCtx, channel, "gpt-4o", "default", 0)
	require.NoError(t, err)
	require.Equal(t, group.Id, selectedGroup.Id)
	require.NotNil(t, selectedAccount)
	require.True(t, common.GetContextKeyBool(firstCtx, constant.ContextKeyPoolGroupReserved))

	secondCtx := newPoolSelectTestContext()
	_, _, err = SelectPoolAccount(secondCtx, channel, "gpt-4o", "default", 0)
	require.True(t, errors.Is(err, ErrPoolAccountGroupConcurrencyExceeded))
	require.False(t, common.GetContextKeyBool(secondCtx, constant.ContextKeyPoolGroupReserved))

	ReleaseSelectedPoolAccount(firstCtx)
	require.False(t, common.GetContextKeyBool(firstCtx, constant.ContextKeyPoolGroupReserved))

	thirdCtx := newPoolSelectTestContext()
	_, selectedAccount, err = SelectPoolAccount(thirdCtx, channel, "gpt-4o", "default", 0)
	require.NoError(t, err)
	require.NotNil(t, selectedAccount)
	ReleaseSelectedPoolAccount(thirdCtx)
}

func TestSelectPoolAccountRespectsGroupRateLimit(t *testing.T) {
	setupAccountPoolSelectTest(t)
	group := createSelectablePoolGroup(t, "codex-group-rpm")
	group.RateLimitRpm = 1
	require.NoError(t, model.DB.Save(group).Error)
	account := createSelectablePoolAccount(t, group, "rpm-account")
	channel := &model.Channel{ChannelInfo: model.ChannelInfo{AccountPoolGroupId: group.Id}}

	firstCtx := newPoolSelectTestContext()
	_, selectedAccount, err := SelectPoolAccount(firstCtx, channel, "gpt-4o", "default", 0)
	require.NoError(t, err)
	require.Equal(t, account.Id, selectedAccount.Id)
	ReleaseSelectedPoolAccount(firstCtx)

	secondCtx := newPoolSelectTestContext()
	_, _, err = SelectPoolAccount(secondCtx, channel, "gpt-4o", "default", 0)
	require.True(t, errors.Is(err, ErrPoolAccountGroupRateLimitExceeded))
}

func TestReservePoolGroupUsageLimitRollsBackRateLimitWhenDailyRequestExceeded(t *testing.T) {
	setupAccountPoolSelectTest(t)
	group := createSelectablePoolGroup(t, "codex-group-rpm-rollback")
	group.RateLimitRpm = 1
	group.DailyRequestLimit = 1
	group.DailyRequestCount = 1
	group.DailyResetTime = model.AccountPoolDailyWindowStart(time.Now())
	require.NoError(t, model.DB.Save(group).Error)

	err := reservePoolGroupUsageLimit(group)
	require.True(t, errors.Is(err, model.ErrAccountPoolGroupDailyRequestLimitExceeded))
	require.True(t, reservePoolGroupRateLimit(group.Id, group.RateLimitRpm))
}

func TestSelectPoolAccountRespectsDailyRequestLimit(t *testing.T) {
	setupAccountPoolSelectTest(t)
	group := createSelectablePoolGroup(t, "codex-group-daily-requests")
	group.DailyRequestLimit = 1
	require.NoError(t, model.DB.Save(group).Error)
	createSelectablePoolAccount(t, group, "daily-request-account")
	channel := &model.Channel{ChannelInfo: model.ChannelInfo{AccountPoolGroupId: group.Id}}

	firstCtx := newPoolSelectTestContext()
	_, _, err := SelectPoolAccount(firstCtx, channel, "gpt-4o", "default", 0)
	require.NoError(t, err)
	ReleaseSelectedPoolAccount(firstCtx)

	secondCtx := newPoolSelectTestContext()
	_, _, err = SelectPoolAccount(secondCtx, channel, "gpt-4o", "default", 0)
	require.True(t, errors.Is(err, model.ErrAccountPoolGroupDailyRequestLimitExceeded))
}

func TestSelectPoolAccountRespectsDailyQuotaLimit(t *testing.T) {
	setupAccountPoolSelectTest(t)
	group := createSelectablePoolGroup(t, "codex-group-daily-quota")
	group.DailyQuotaLimit = 10
	group.DailyUsedQuota = 10
	group.DailyResetTime = model.AccountPoolDailyWindowStart(time.Now())
	require.NoError(t, model.DB.Save(group).Error)
	createSelectablePoolAccount(t, group, "daily-quota-account")
	channel := &model.Channel{ChannelInfo: model.ChannelInfo{AccountPoolGroupId: group.Id}}

	ctx := newPoolSelectTestContext()
	_, _, err := SelectPoolAccount(ctx, channel, "gpt-4o", "default", 0)
	require.True(t, errors.Is(err, model.ErrAccountPoolGroupDailyQuotaLimitExceeded))
}

func TestSelectPoolAccountResetsDailyUsageWindow(t *testing.T) {
	setupAccountPoolSelectTest(t)
	group := createSelectablePoolGroup(t, "codex-group-daily-reset")
	group.DailyRequestLimit = 1
	group.DailyQuotaLimit = 10
	group.DailyRequestCount = 1
	group.DailyUsedQuota = 10
	group.DailyResetTime = model.AccountPoolDailyWindowStart(time.Now().Add(-24 * time.Hour))
	require.NoError(t, model.DB.Save(group).Error)
	createSelectablePoolAccount(t, group, "daily-reset-account")
	channel := &model.Channel{ChannelInfo: model.ChannelInfo{AccountPoolGroupId: group.Id}}

	ctx := newPoolSelectTestContext()
	_, selectedAccount, err := SelectPoolAccount(ctx, channel, "gpt-4o", "default", 0)
	require.NoError(t, err)
	require.NotNil(t, selectedAccount)
	ReleaseSelectedPoolAccount(ctx)

	updated, err := model.GetAccountPoolGroupById(group.Id)
	require.NoError(t, err)
	require.Equal(t, int64(1), updated.DailyRequestCount)
	require.Equal(t, int64(0), updated.DailyUsedQuota)
	require.Equal(t, model.AccountPoolDailyWindowStart(time.Now()), updated.DailyResetTime)
}

func TestAddSelectedAccountUsedQuotaUpdatesPoolGroupQuota(t *testing.T) {
	setupAccountPoolSelectTest(t)
	group := createSelectablePoolGroup(t, "codex-group-quota-record")
	account := createSelectablePoolAccount(t, group, "quota-record-account")

	AddSelectedAccountUsedQuota(0, account.Id, 7)

	updated, err := model.GetAccountPoolGroupById(group.Id)
	require.NoError(t, err)
	require.Equal(t, int64(7), updated.UsedQuota)
	require.Equal(t, int64(7), updated.DailyUsedQuota)
	require.Equal(t, model.AccountPoolDailyWindowStart(time.Now()), updated.DailyResetTime)
}

func createSelectablePoolGroup(t *testing.T, name string) *model.AccountPoolGroup {
	t.Helper()
	group := &model.AccountPoolGroup{
		Name:     name,
		Platform: "codex",
		AuthType: model.AccountPoolAuthTypeAPIKey,
		Source:   model.AccountPoolGroupSourceNative,
		Status:   common.ChannelStatusEnabled,
		Strategy: model.AccountPoolStrategyRoundRobin,
		Group:    "default",
	}
	require.NoError(t, model.DB.Create(group).Error)
	return group
}

func createSelectablePoolAccount(t *testing.T, group *model.AccountPoolGroup, name string) *model.PoolAccount {
	t.Helper()
	account := &model.PoolAccount{
		PoolGroupId: group.Id,
		Name:        name,
		Platform:    group.Platform,
		AuthType:    group.AuthType,
		Credentials: "placeholder",
		Status:      common.ChannelStatusEnabled,
		Schedulable: true,
	}
	require.NoError(t, model.DB.Create(account).Error)
	return account
}

func TestSelectPoolAccountRespectsGroupDailyRequestLimit(t *testing.T) {
	setupAccountPoolSelectTest(t)
	group, channel := createPoolSelectGroupWithAccounts(t, model.AccountPoolGroup{
		Name:              "codex-daily-request",
		DailyRequestLimit: 1,
	})

	firstCtx := newPoolSelectTestContext()
	_, selectedAccount, err := SelectPoolAccount(firstCtx, channel, "gpt-4o", "default", 0)
	require.NoError(t, err)
	require.NotNil(t, selectedAccount)
	ReleaseSelectedPoolAccount(firstCtx)

	updatedGroup, err := model.GetAccountPoolGroupById(group.Id)
	require.NoError(t, err)
	require.EqualValues(t, 1, updatedGroup.DailyRequestCount)

	secondCtx := newPoolSelectTestContext()
	_, _, err = SelectPoolAccount(secondCtx, channel, "gpt-4o", "default", 0)
	require.True(t, errors.Is(err, model.ErrAccountPoolGroupDailyRequestLimitExceeded))
	require.False(t, common.GetContextKeyBool(secondCtx, constant.ContextKeyPoolGroupReserved))
}

func TestSelectPoolAccountSkipsGroupWhenDailyQuotaLimitReached(t *testing.T) {
	setupAccountPoolSelectTest(t)
	group, channel := createPoolSelectGroupWithAccounts(t, model.AccountPoolGroup{
		Name:            "codex-daily-quota",
		DailyQuotaLimit: 10,
	})
	var account model.PoolAccount
	require.NoError(t, model.DB.Where("pool_group_id = ?", group.Id).Order("id ASC").First(&account).Error)
	AddSelectedAccountUsedQuotaWithGroup(0, group.Id, account.Id, 10)

	updatedGroup, err := model.GetAccountPoolGroupById(group.Id)
	require.NoError(t, err)
	require.EqualValues(t, 10, updatedGroup.DailyUsedQuota)
	require.EqualValues(t, 10, updatedGroup.UsedQuota)

	ctx := newPoolSelectTestContext()
	_, _, err = SelectPoolAccount(ctx, channel, "gpt-4o", "default", 0)
	require.True(t, errors.Is(err, model.ErrAccountPoolGroupDailyQuotaLimitExceeded))
}

func TestAccountPoolGroupMaxConcurrencyFallsBackToSettings(t *testing.T) {
	group := &model.AccountPoolGroup{
		Settings: `{"max_concurrency":2}`,
	}

	require.Equal(t, 2, group.GetMaxConcurrency())

	group.MaxConcurrency = 3
	require.Equal(t, 3, group.GetMaxConcurrency())
}

func createPoolSelectGroupWithAccounts(t *testing.T, override model.AccountPoolGroup) (*model.AccountPoolGroup, *model.Channel) {
	t.Helper()
	group := &model.AccountPoolGroup{
		Name:     "codex-select",
		Platform: "codex",
		AuthType: model.AccountPoolAuthTypeAPIKey,
		Source:   model.AccountPoolGroupSourceNative,
		Status:   common.ChannelStatusEnabled,
		Strategy: model.AccountPoolStrategyRoundRobin,
		Group:    "default",
	}
	if override.Name != "" {
		group.Name = override.Name
	}
	group.MaxConcurrency = override.MaxConcurrency
	group.DailyRequestLimit = override.DailyRequestLimit
	group.DailyQuotaLimit = override.DailyQuotaLimit
	group.Settings = override.Settings
	require.NoError(t, model.DB.Create(group).Error)
	accounts := []*model.PoolAccount{
		{
			PoolGroupId: group.Id,
			Name:        "account-a",
			Platform:    "codex",
			AuthType:    model.AccountPoolAuthTypeAPIKey,
			Credentials: "placeholder-a",
			Status:      common.ChannelStatusEnabled,
			Schedulable: true,
		},
		{
			PoolGroupId: group.Id,
			Name:        "account-b",
			Platform:    "codex",
			AuthType:    model.AccountPoolAuthTypeAPIKey,
			Credentials: "placeholder-b",
			Status:      common.ChannelStatusEnabled,
			Schedulable: true,
		},
	}
	require.NoError(t, model.DB.Create(&accounts).Error)
	return group, &model.Channel{
		ChannelInfo: model.ChannelInfo{
			AccountPoolGroupId: group.Id,
		},
	}
}
