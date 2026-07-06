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
	require.NoError(t, model.DB.AutoMigrate(&model.AccountPoolGroup{}, &model.PoolAccount{}, &model.PoolAccountStateLog{}))
	require.NoError(t, model.DB.Exec("DELETE FROM pool_account_state_logs").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM pool_accounts").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM account_pool_groups").Error)
	originalRedisEnabled := common.RedisEnabled
	originalRandomIntn := poolAccountRandomIntn
	common.RedisEnabled = false
	poolAccountConcurrencyMu.Lock()
	poolAccountConcurrency = map[int]int{}
	poolAccountConcurrencyMu.Unlock()
	poolAccountCursorMu.Lock()
	poolAccountCursors = map[string]uint64{}
	poolAccountCursorMu.Unlock()
	poolAccountRateMu.Lock()
	poolAccountRateCounters = map[int]poolGroupRateCounter{}
	poolAccountRateMu.Unlock()
	poolGroupConcurrencyMu.Lock()
	poolGroupConcurrency = map[int]int{}
	poolGroupConcurrencyMu.Unlock()
	poolGroupRateMu.Lock()
	poolGroupRateCounters = map[int]poolGroupRateCounter{}
	poolGroupRateMu.Unlock()
	t.Cleanup(func() {
		common.RedisEnabled = originalRedisEnabled
		poolAccountRandomIntn = originalRandomIntn
		poolAccountConcurrencyMu.Lock()
		poolAccountConcurrency = map[int]int{}
		poolAccountConcurrencyMu.Unlock()
		poolAccountCursorMu.Lock()
		poolAccountCursors = map[string]uint64{}
		poolAccountCursorMu.Unlock()
		poolAccountRateMu.Lock()
		poolAccountRateCounters = map[int]poolGroupRateCounter{}
		poolAccountRateMu.Unlock()
		poolGroupConcurrencyMu.Lock()
		poolGroupConcurrency = map[int]int{}
		poolGroupConcurrencyMu.Unlock()
		poolGroupRateMu.Lock()
		poolGroupRateCounters = map[int]poolGroupRateCounter{}
		poolGroupRateMu.Unlock()
		_ = model.DB.Exec("DELETE FROM pool_account_state_logs").Error
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

func TestSelectPoolAccountPrefersHigherSuccessRate(t *testing.T) {
	setupAccountPoolSelectTest(t)
	group := createSelectablePoolGroup(t, "codex-success-rate")
	group.Strategy = model.AccountPoolStrategySuccessRate
	require.NoError(t, model.DB.Save(group).Error)
	lowSuccessAccount := createSelectablePoolAccount(t, group, "success-rate-low")
	lowSuccessAccount.SuccessCount = 1
	lowSuccessAccount.FailedCount = 9
	require.NoError(t, model.DB.Save(lowSuccessAccount).Error)
	highSuccessAccount := createSelectablePoolAccount(t, group, "success-rate-high")
	highSuccessAccount.SuccessCount = 8
	highSuccessAccount.FailedCount = 2
	require.NoError(t, model.DB.Save(highSuccessAccount).Error)
	channel := &model.Channel{ChannelInfo: model.ChannelInfo{AccountPoolGroupId: group.Id}}

	ctx := newPoolSelectTestContext()
	_, selectedAccount, err := SelectPoolAccount(ctx, channel, "gpt-4o", "default", 0)
	require.NoError(t, err)
	require.Equal(t, highSuccessAccount.Id, selectedAccount.Id)
	ReleaseSelectedPoolAccount(ctx)
}

func TestSelectPoolAccountSuccessRateRespectsPriority(t *testing.T) {
	setupAccountPoolSelectTest(t)
	group := createSelectablePoolGroup(t, "codex-success-rate-priority")
	group.Strategy = model.AccountPoolStrategySuccessRate
	require.NoError(t, model.DB.Save(group).Error)
	highSuccessAccount := createSelectablePoolAccount(t, group, "success-rate-lower-priority")
	highSuccessAccount.SuccessCount = 20
	highSuccessAccount.FailedCount = 0
	highSuccessAccount.Priority = 0
	require.NoError(t, model.DB.Save(highSuccessAccount).Error)
	highPriorityAccount := createSelectablePoolAccount(t, group, "success-rate-higher-priority")
	highPriorityAccount.SuccessCount = 1
	highPriorityAccount.FailedCount = 9
	highPriorityAccount.Priority = 10
	require.NoError(t, model.DB.Save(highPriorityAccount).Error)
	channel := &model.Channel{ChannelInfo: model.ChannelInfo{AccountPoolGroupId: group.Id}}

	ctx := newPoolSelectTestContext()
	_, selectedAccount, err := SelectPoolAccount(ctx, channel, "gpt-4o", "default", 0)
	require.NoError(t, err)
	require.Equal(t, highPriorityAccount.Id, selectedAccount.Id)
	ReleaseSelectedPoolAccount(ctx)
}

func TestSortPoolAccountCandidatesSuccessRateTieBreakers(t *testing.T) {
	accounts := []*model.PoolAccount{
		{Id: 1, SuccessCount: 1, FailedCount: 1},
		{Id: 2, SuccessCount: 4, FailedCount: 4},
		{Id: 3, SuccessCount: 0, FailedCount: 0},
	}

	sortPoolAccountCandidates(accounts, model.AccountPoolStrategySuccessRate)

	require.Equal(t, 2, accounts[0].Id)
	require.Equal(t, 1, accounts[1].Id)
	require.Equal(t, 3, accounts[2].Id)
}

func TestSelectPoolAccountRandomUsesRandomIndex(t *testing.T) {
	setupAccountPoolSelectTest(t)
	group := createSelectablePoolGroup(t, "codex-random-index")
	group.Strategy = model.AccountPoolStrategyRandom
	require.NoError(t, model.DB.Save(group).Error)
	firstAccount := createSelectablePoolAccount(t, group, "random-account-a")
	secondAccount := createSelectablePoolAccount(t, group, "random-account-b")
	poolAccountRandomIntn = func(n int) int {
		require.Equal(t, 2, n)
		return 1
	}
	channel := &model.Channel{ChannelInfo: model.ChannelInfo{AccountPoolGroupId: group.Id}}

	ctx := newPoolSelectTestContext()
	_, selectedAccount, err := SelectPoolAccount(ctx, channel, "gpt-4o", "default", 0)
	require.NoError(t, err)
	require.Equal(t, secondAccount.Id, selectedAccount.Id)
	require.NotEqual(t, firstAccount.Id, selectedAccount.Id)
	ReleaseSelectedPoolAccount(ctx)
}

func TestSelectPoolAccountRandomRespectsPriority(t *testing.T) {
	setupAccountPoolSelectTest(t)
	group := createSelectablePoolGroup(t, "codex-random-priority")
	group.Strategy = model.AccountPoolStrategyRandom
	require.NoError(t, model.DB.Save(group).Error)
	lowPriorityAccount := createSelectablePoolAccount(t, group, "random-low-priority")
	lowPriorityAccount.Priority = 0
	require.NoError(t, model.DB.Save(lowPriorityAccount).Error)
	highPriorityAccount := createSelectablePoolAccount(t, group, "random-high-priority")
	highPriorityAccount.Priority = 10
	require.NoError(t, model.DB.Save(highPriorityAccount).Error)
	poolAccountRandomIntn = func(n int) int {
		require.Equal(t, 1, n)
		return 0
	}
	channel := &model.Channel{ChannelInfo: model.ChannelInfo{AccountPoolGroupId: group.Id}}

	ctx := newPoolSelectTestContext()
	_, selectedAccount, err := SelectPoolAccount(ctx, channel, "gpt-4o", "default", 0)
	require.NoError(t, err)
	require.Equal(t, highPriorityAccount.Id, selectedAccount.Id)
	ReleaseSelectedPoolAccount(ctx)
}

func TestSelectPoolAccountWeightedHonorsWeights(t *testing.T) {
	setupAccountPoolSelectTest(t)
	group := createSelectablePoolGroup(t, "codex-weighted-sequence")
	group.Strategy = model.AccountPoolStrategyWeighted
	require.NoError(t, model.DB.Save(group).Error)
	firstAccount := createSelectablePoolAccount(t, group, "weighted-account-a")
	firstAccount.Weight = 2
	require.NoError(t, model.DB.Save(firstAccount).Error)
	secondAccount := createSelectablePoolAccount(t, group, "weighted-account-b")
	secondAccount.Weight = 1
	require.NoError(t, model.DB.Save(secondAccount).Error)
	channel := &model.Channel{ChannelInfo: model.ChannelInfo{AccountPoolGroupId: group.Id}}

	selectedIDs := make([]int, 0, 6)
	for i := 0; i < 6; i++ {
		ctx := newPoolSelectTestContext()
		_, selectedAccount, err := SelectPoolAccount(ctx, channel, "gpt-4o", "default", 0)
		require.NoError(t, err)
		selectedIDs = append(selectedIDs, selectedAccount.Id)
		ReleaseSelectedPoolAccount(ctx)
	}

	require.Equal(t, []int{
		firstAccount.Id,
		firstAccount.Id,
		secondAccount.Id,
		firstAccount.Id,
		firstAccount.Id,
		secondAccount.Id,
	}, selectedIDs)
}

func TestSelectPoolAccountWeightedSkipsDailyQuotaLimitedAccount(t *testing.T) {
	setupAccountPoolSelectTest(t)
	group := createSelectablePoolGroup(t, "codex-weighted-quota-skip")
	group.Strategy = model.AccountPoolStrategyWeighted
	require.NoError(t, model.DB.Save(group).Error)
	limitedAccount := createSelectablePoolAccount(t, group, "weighted-quota-limited")
	limitedAccount.Weight = 100
	limitedAccount.DailyQuotaLimit = 10
	limitedAccount.DailyUsedQuota = 10
	limitedAccount.DailyResetTime = model.AccountPoolDailyWindowStart(time.Now())
	require.NoError(t, model.DB.Save(limitedAccount).Error)
	fallbackAccount := createSelectablePoolAccount(t, group, "weighted-quota-fallback")
	fallbackAccount.Weight = 1
	require.NoError(t, model.DB.Save(fallbackAccount).Error)
	channel := &model.Channel{ChannelInfo: model.ChannelInfo{AccountPoolGroupId: group.Id}}

	ctx := newPoolSelectTestContext()
	_, selectedAccount, err := SelectPoolAccount(ctx, channel, "gpt-4o", "default", 0)
	require.NoError(t, err)
	require.Equal(t, fallbackAccount.Id, selectedAccount.Id)
	ReleaseSelectedPoolAccount(ctx)
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

	updatedAccount, err := model.GetPoolAccountById(account.Id)
	require.NoError(t, err)
	require.Equal(t, int64(7), updatedAccount.UsedQuota)
	require.Equal(t, int64(7), updatedAccount.DailyUsedQuota)
	require.Equal(t, model.AccountPoolDailyWindowStart(time.Now()), updatedAccount.DailyResetTime)

	updated, err := model.GetAccountPoolGroupById(group.Id)
	require.NoError(t, err)
	require.Equal(t, int64(7), updated.UsedQuota)
	require.Equal(t, int64(7), updated.DailyUsedQuota)
	require.Equal(t, model.AccountPoolDailyWindowStart(time.Now()), updated.DailyResetTime)
}

func TestSelectPoolAccountRespectsAccountRateLimit(t *testing.T) {
	setupAccountPoolSelectTest(t)
	group := createSelectablePoolGroup(t, "codex-account-rpm")
	account := createSelectablePoolAccount(t, group, "rpm-account")
	account.RateLimitRpm = 1
	require.NoError(t, model.DB.Save(account).Error)
	channel := &model.Channel{ChannelInfo: model.ChannelInfo{AccountPoolGroupId: group.Id}}

	firstCtx := newPoolSelectTestContext()
	_, selectedAccount, err := SelectPoolAccount(firstCtx, channel, "gpt-4o", "default", 0)
	require.NoError(t, err)
	require.Equal(t, account.Id, selectedAccount.Id)
	ReleaseSelectedPoolAccount(firstCtx)

	secondCtx := newPoolSelectTestContext()
	_, _, err = SelectPoolAccount(secondCtx, channel, "gpt-4o", "default", 0)
	require.True(t, errors.Is(err, ErrPoolAccountRateLimitExceeded))

	updatedAccount, err := model.GetPoolAccountById(account.Id)
	require.NoError(t, err)
	require.EqualValues(t, 1, updatedAccount.DailyRequestCount)
}

func TestSelectPoolAccountSwitchesWhenAccountRateLimitExceeded(t *testing.T) {
	setupAccountPoolSelectTest(t)
	group := createSelectablePoolGroup(t, "codex-account-rpm-switch")
	group.Strategy = model.AccountPoolStrategyFillFirst
	require.NoError(t, model.DB.Save(group).Error)
	firstAccount := createSelectablePoolAccount(t, group, "rpm-account-a")
	firstAccount.RateLimitRpm = 1
	require.NoError(t, model.DB.Save(firstAccount).Error)
	secondAccount := createSelectablePoolAccount(t, group, "rpm-account-b")
	channel := &model.Channel{ChannelInfo: model.ChannelInfo{AccountPoolGroupId: group.Id}}

	firstCtx := newPoolSelectTestContext()
	_, selectedAccount, err := SelectPoolAccount(firstCtx, channel, "gpt-4o", "default", 0)
	require.NoError(t, err)
	require.Equal(t, firstAccount.Id, selectedAccount.Id)
	ReleaseSelectedPoolAccount(firstCtx)

	secondCtx := newPoolSelectTestContext()
	_, selectedAccount, err = SelectPoolAccount(secondCtx, channel, "gpt-4o", "default", 0)
	require.NoError(t, err)
	require.Equal(t, secondAccount.Id, selectedAccount.Id)
	ReleaseSelectedPoolAccount(secondCtx)
}

func TestSelectPoolAccountRespectsAccountDailyRequestLimit(t *testing.T) {
	setupAccountPoolSelectTest(t)
	group := createSelectablePoolGroup(t, "codex-account-daily-request")
	account := createSelectablePoolAccount(t, group, "daily-request-account")
	account.DailyRequestLimit = 1
	require.NoError(t, model.DB.Save(account).Error)
	channel := &model.Channel{ChannelInfo: model.ChannelInfo{AccountPoolGroupId: group.Id}}

	firstCtx := newPoolSelectTestContext()
	_, selectedAccount, err := SelectPoolAccount(firstCtx, channel, "gpt-4o", "default", 0)
	require.NoError(t, err)
	require.Equal(t, account.Id, selectedAccount.Id)
	ReleaseSelectedPoolAccount(firstCtx)

	secondCtx := newPoolSelectTestContext()
	_, _, err = SelectPoolAccount(secondCtx, channel, "gpt-4o", "default", 0)
	require.True(t, errors.Is(err, model.ErrPoolAccountDailyRequestLimitExceeded))

	updatedAccount, err := model.GetPoolAccountById(account.Id)
	require.NoError(t, err)
	require.True(t, updatedAccount.Unavailable)
	require.Equal(t, model.PoolAccountDailyRequestLimitStatusMessage, updatedAccount.StatusMessage)
	require.Equal(t, model.PoolAccountDailyRequestLimitStatusMessage, updatedAccount.LastError)
	require.Equal(t, model.AccountPoolNextDailyWindowStart(time.Now()), updatedAccount.NextRetryTime)

	stats, err := model.CountPoolAccountsByGroupIDs([]int{group.Id})
	require.NoError(t, err)
	require.EqualValues(t, 1, stats[group.Id]["cooldown"])
	require.EqualValues(t, 1, stats[group.Id]["unavailable"])
	require.EqualValues(t, 0, stats[group.Id]["enabled"])
}

func TestSelectPoolAccountDailyRequestLimitCanAutoDisable(t *testing.T) {
	setupAccountPoolSelectTest(t)
	group := createSelectablePoolGroup(t, "codex-account-daily-request-disable")
	group.DailyLimitAction = model.AccountPoolDailyLimitActionDisable
	require.NoError(t, model.DB.Save(group).Error)
	account := createSelectablePoolAccount(t, group, "daily-request-disable-account")
	account.DailyRequestLimit = 1
	require.NoError(t, model.DB.Save(account).Error)
	channel := &model.Channel{ChannelInfo: model.ChannelInfo{AccountPoolGroupId: group.Id}}

	firstCtx := newPoolSelectTestContext()
	_, selectedAccount, err := SelectPoolAccount(firstCtx, channel, "gpt-4o", "default", 0)
	require.NoError(t, err)
	require.Equal(t, account.Id, selectedAccount.Id)
	ReleaseSelectedPoolAccount(firstCtx)

	secondCtx := newPoolSelectTestContext()
	_, _, err = SelectPoolAccount(secondCtx, channel, "gpt-4o", "default", 0)
	require.True(t, errors.Is(err, model.ErrPoolAccountDailyRequestLimitExceeded))

	updatedAccount, err := model.GetPoolAccountById(account.Id)
	require.NoError(t, err)
	require.Equal(t, common.ChannelStatusAutoDisabled, updatedAccount.Status)
	require.False(t, updatedAccount.Schedulable)
	require.True(t, updatedAccount.Unavailable)
	require.Equal(t, model.PoolAccountDailyRequestLimitAutoDisabledStatusMessage, updatedAccount.StatusMessage)
	require.Equal(t, model.PoolAccountDailyRequestLimitAutoDisabledStatusMessage, updatedAccount.LastError)
	require.Equal(t, model.PoolAccountDailyRequestLimitAutoDisabledStatusMessage, updatedAccount.DisabledReason)
	require.EqualValues(t, 0, updatedAccount.NextRetryTime)

	stats, err := model.CountPoolAccountsByGroupIDs([]int{group.Id})
	require.NoError(t, err)
	require.EqualValues(t, 1, stats[group.Id]["disabled"])
	require.EqualValues(t, 0, stats[group.Id]["cooldown"])

	stateLogs, total, err := model.GetPoolAccountStateLogs(model.PoolAccountStateLogFilter{
		PoolAccountId: account.Id,
		Action:        model.PoolAccountStateActionDailyLimitDisabled,
		Limit:         10,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, stateLogs, 1)
	require.Equal(t, "daily_limit", stateLogs[0].Source)
	require.Equal(t, model.PoolAccountDailyRequestLimitAutoDisabledStatusMessage, stateLogs[0].Reason)
}

func TestSelectPoolAccountSkipsAccountDailyRequestLimit(t *testing.T) {
	setupAccountPoolSelectTest(t)
	group := createSelectablePoolGroup(t, "codex-account-daily-request-skip")
	group.Strategy = model.AccountPoolStrategyFillFirst
	require.NoError(t, model.DB.Save(group).Error)
	limitedAccount := createSelectablePoolAccount(t, group, "daily-request-limited-account")
	limitedAccount.DailyRequestLimit = 1
	limitedAccount.DailyRequestCount = 1
	limitedAccount.DailyResetTime = model.AccountPoolDailyWindowStart(time.Now())
	require.NoError(t, model.DB.Save(limitedAccount).Error)
	fallbackAccount := createSelectablePoolAccount(t, group, "daily-request-fallback-account")
	channel := &model.Channel{ChannelInfo: model.ChannelInfo{AccountPoolGroupId: group.Id}}

	ctx := newPoolSelectTestContext()
	_, selectedAccount, err := SelectPoolAccount(ctx, channel, "gpt-4o", "default", 0)
	require.NoError(t, err)
	require.Equal(t, fallbackAccount.Id, selectedAccount.Id)
	ReleaseSelectedPoolAccount(ctx)
}

func TestSelectPoolAccountRespectsAccountDailyQuotaLimit(t *testing.T) {
	setupAccountPoolSelectTest(t)
	group := createSelectablePoolGroup(t, "codex-account-daily-quota")
	account := createSelectablePoolAccount(t, group, "daily-quota-account")
	account.DailyQuotaLimit = 10
	account.DailyUsedQuota = 10
	account.DailyResetTime = model.AccountPoolDailyWindowStart(time.Now())
	require.NoError(t, model.DB.Save(account).Error)
	channel := &model.Channel{ChannelInfo: model.ChannelInfo{AccountPoolGroupId: group.Id}}

	ctx := newPoolSelectTestContext()
	_, _, err := SelectPoolAccount(ctx, channel, "gpt-4o", "default", 0)
	require.True(t, errors.Is(err, model.ErrPoolAccountDailyQuotaLimitExceeded))
}

func TestSelectPoolAccountSkipsAccountDailyQuotaLimit(t *testing.T) {
	setupAccountPoolSelectTest(t)
	group := createSelectablePoolGroup(t, "codex-account-daily-quota-skip")
	group.Strategy = model.AccountPoolStrategyFillFirst
	require.NoError(t, model.DB.Save(group).Error)
	limitedAccount := createSelectablePoolAccount(t, group, "daily-quota-limited-account")
	limitedAccount.DailyQuotaLimit = 10
	limitedAccount.DailyUsedQuota = 10
	limitedAccount.DailyResetTime = model.AccountPoolDailyWindowStart(time.Now())
	require.NoError(t, model.DB.Save(limitedAccount).Error)
	fallbackAccount := createSelectablePoolAccount(t, group, "daily-quota-fallback-account")
	channel := &model.Channel{ChannelInfo: model.ChannelInfo{AccountPoolGroupId: group.Id}}

	ctx := newPoolSelectTestContext()
	_, selectedAccount, err := SelectPoolAccount(ctx, channel, "gpt-4o", "default", 0)
	require.NoError(t, err)
	require.Equal(t, fallbackAccount.Id, selectedAccount.Id)
	ReleaseSelectedPoolAccount(ctx)
}

func TestSelectPoolAccountResetsAccountDailyUsageWindow(t *testing.T) {
	setupAccountPoolSelectTest(t)
	group := createSelectablePoolGroup(t, "codex-account-daily-reset")
	account := createSelectablePoolAccount(t, group, "daily-reset-account")
	account.DailyRequestLimit = 1
	account.DailyQuotaLimit = 10
	account.DailyRequestCount = 1
	account.DailyUsedQuota = 10
	account.DailyResetTime = model.AccountPoolDailyWindowStart(time.Now().Add(-24 * time.Hour))
	account.Unavailable = true
	account.StatusMessage = model.PoolAccountDailyRequestLimitStatusMessage
	account.LastError = model.PoolAccountDailyRequestLimitStatusMessage
	account.DisabledReason = model.PoolAccountDailyRequestLimitStatusMessage
	account.NextRetryTime = model.AccountPoolNextDailyWindowStart(time.Now().Add(-24 * time.Hour))
	require.NoError(t, model.DB.Save(account).Error)
	channel := &model.Channel{ChannelInfo: model.ChannelInfo{AccountPoolGroupId: group.Id}}

	ctx := newPoolSelectTestContext()
	_, selectedAccount, err := SelectPoolAccount(ctx, channel, "gpt-4o", "default", 0)
	require.NoError(t, err)
	require.Equal(t, account.Id, selectedAccount.Id)
	ReleaseSelectedPoolAccount(ctx)

	updatedAccount, err := model.GetPoolAccountById(account.Id)
	require.NoError(t, err)
	require.Equal(t, int64(1), updatedAccount.DailyRequestCount)
	require.Equal(t, int64(0), updatedAccount.DailyUsedQuota)
	require.Equal(t, model.AccountPoolDailyWindowStart(time.Now()), updatedAccount.DailyResetTime)
	require.False(t, updatedAccount.Unavailable)
	require.Equal(t, "", updatedAccount.StatusMessage)
	require.Equal(t, "", updatedAccount.LastError)
	require.Equal(t, int64(0), updatedAccount.NextRetryTime)
}

func TestAddSelectedAccountUsedQuotaMarksAccountDailyQuotaCooling(t *testing.T) {
	setupAccountPoolSelectTest(t)
	group := createSelectablePoolGroup(t, "codex-account-quota-cooling")
	account := createSelectablePoolAccount(t, group, "daily-quota-cooling-account")
	account.DailyQuotaLimit = 10
	require.NoError(t, model.DB.Save(account).Error)

	AddSelectedAccountUsedQuota(0, account.Id, 10)

	updatedAccount, err := model.GetPoolAccountById(account.Id)
	require.NoError(t, err)
	require.EqualValues(t, 10, updatedAccount.DailyUsedQuota)
	require.True(t, updatedAccount.Unavailable)
	require.Equal(t, model.PoolAccountDailyQuotaLimitStatusMessage, updatedAccount.StatusMessage)
	require.Equal(t, model.PoolAccountDailyQuotaLimitStatusMessage, updatedAccount.LastError)
	require.Equal(t, model.AccountPoolNextDailyWindowStart(time.Now()), updatedAccount.NextRetryTime)

	channel := &model.Channel{ChannelInfo: model.ChannelInfo{AccountPoolGroupId: group.Id}}
	ctx := newPoolSelectTestContext()
	_, _, err = SelectPoolAccount(ctx, channel, "gpt-4o", "default", 0)
	require.True(t, errors.Is(err, model.ErrPoolAccountDailyQuotaLimitExceeded))

	require.NoError(t, model.DB.Model(&model.PoolAccount{}).Where("id = ?", account.Id).Updates(map[string]interface{}{
		"daily_reset_time": model.AccountPoolDailyWindowStart(time.Now().Add(-24 * time.Hour)),
	}).Error)
	recoveredCtx := newPoolSelectTestContext()
	_, selectedAccount, err := SelectPoolAccount(recoveredCtx, channel, "gpt-4o", "default", 0)
	require.NoError(t, err)
	require.Equal(t, account.Id, selectedAccount.Id)
	ReleaseSelectedPoolAccount(recoveredCtx)
}

func TestAddSelectedAccountUsedQuotaCanAutoDisableOnDailyQuotaLimit(t *testing.T) {
	setupAccountPoolSelectTest(t)
	group := createSelectablePoolGroup(t, "codex-account-quota-disable")
	account := createSelectablePoolAccount(t, group, "daily-quota-disable-account")
	account.DailyQuotaLimit = 10
	account.DailyLimitAction = model.AccountPoolDailyLimitActionDisable
	require.NoError(t, model.DB.Save(account).Error)

	AddSelectedAccountUsedQuota(0, account.Id, 10)

	updatedAccount, err := model.GetPoolAccountById(account.Id)
	require.NoError(t, err)
	require.EqualValues(t, 10, updatedAccount.DailyUsedQuota)
	require.Equal(t, common.ChannelStatusAutoDisabled, updatedAccount.Status)
	require.False(t, updatedAccount.Schedulable)
	require.True(t, updatedAccount.Unavailable)
	require.Equal(t, model.PoolAccountDailyQuotaLimitAutoDisabledStatusMessage, updatedAccount.StatusMessage)
	require.EqualValues(t, 0, updatedAccount.NextRetryTime)
}

func TestReservePoolAccountUsageLimitRollsBackRateLimitWhenDailyRequestExceeded(t *testing.T) {
	setupAccountPoolSelectTest(t)
	group := createSelectablePoolGroup(t, "codex-account-rpm-rollback")
	account := createSelectablePoolAccount(t, group, "rpm-rollback-account")
	account.RateLimitRpm = 1
	account.DailyRequestLimit = 1
	account.DailyRequestCount = 1
	account.DailyResetTime = model.AccountPoolDailyWindowStart(time.Now())
	require.NoError(t, model.DB.Save(account).Error)

	err := reservePoolAccountUsageLimit(account)
	require.True(t, errors.Is(err, model.ErrPoolAccountDailyRequestLimitExceeded))
	require.True(t, reservePoolAccountRateLimit(account.Id, account.RateLimitRpm))
}

func TestSelectPoolAccountRollsBackAccountUsageWhenGroupLimitFails(t *testing.T) {
	setupAccountPoolSelectTest(t)
	group := createSelectablePoolGroup(t, "codex-account-group-rpm-rollback")
	group.RateLimitRpm = 1
	require.NoError(t, model.DB.Save(group).Error)
	account := createSelectablePoolAccount(t, group, "group-rpm-rollback-account")
	account.RateLimitRpm = 1
	require.NoError(t, model.DB.Save(account).Error)
	channel := &model.Channel{ChannelInfo: model.ChannelInfo{AccountPoolGroupId: group.Id}}

	require.True(t, reservePoolGroupRateLimit(group.Id, group.RateLimitRpm))

	ctx := newPoolSelectTestContext()
	_, _, err := SelectPoolAccount(ctx, channel, "gpt-4o", "default", 0)
	require.True(t, errors.Is(err, ErrPoolAccountGroupRateLimitExceeded))
	require.False(t, common.GetContextKeyBool(ctx, constant.ContextKeyPoolGroupReserved))

	updatedAccount, err := model.GetPoolAccountById(account.Id)
	require.NoError(t, err)
	require.EqualValues(t, 0, updatedAccount.DailyRequestCount)
	require.True(t, reservePoolAccountRateLimit(account.Id, account.RateLimitRpm))
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
