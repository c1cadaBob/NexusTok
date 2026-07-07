package service

import (
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func setupAccountPoolTaskLimitTest(t *testing.T) {
	t.Helper()
	require.NoError(t, model.DB.AutoMigrate(&model.AccountPoolGroup{}))
	require.NoError(t, model.DB.Exec("DELETE FROM account_pool_groups").Error)

	originalRedisEnabled := common.RedisEnabled
	originalRetryInterval := accountPoolTaskLimitRetryInterval
	common.RedisEnabled = false
	accountPoolTaskLimitRetryInterval = 5 * time.Millisecond
	resetAccountPoolTaskLimitState()

	t.Cleanup(func() {
		common.RedisEnabled = originalRedisEnabled
		accountPoolTaskLimitRetryInterval = originalRetryInterval
		resetAccountPoolTaskLimitState()
		_ = model.DB.Exec("DELETE FROM account_pool_groups").Error
	})
}

func resetAccountPoolTaskLimitState() {
	accountPoolTaskConcurrencyMu.Lock()
	accountPoolTaskConcurrency = map[string]int{}
	accountPoolTaskConcurrencyMu.Unlock()
	accountPoolTaskRateMu.Lock()
	accountPoolTaskRateCounters = map[string]poolGroupRateCounter{}
	accountPoolTaskRateMu.Unlock()
}

func newAccountPoolTaskLimitContext() *gin.Context {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/v1/videos", nil)
	return c
}

func createTaskLimitGroup(t *testing.T, group *model.AccountPoolGroup) *model.AccountPoolGroup {
	t.Helper()
	if group.Name == "" {
		group.Name = "task-limit-group"
	}
	if group.Platform == "" {
		group.Platform = "codex"
	}
	if group.AuthType == "" {
		group.AuthType = model.AccountPoolAuthTypeAPIKey
	}
	if group.Source == "" {
		group.Source = model.AccountPoolGroupSourceNative
	}
	if group.Status == 0 {
		group.Status = common.ChannelStatusEnabled
	}
	require.NoError(t, model.DB.Create(group).Error)
	return group
}

func TestReserveAccountPoolTaskLimitNoopsWhenDisabled(t *testing.T) {
	setupAccountPoolTaskLimitTest(t)
	group := createTaskLimitGroup(t, &model.AccountPoolGroup{})
	ctx := newAccountPoolTaskLimitContext()

	loaded, err := ReserveAccountPoolTaskLimit(ctx, AccountPoolTaskLimitOptions{
		PoolGroupID: group.Id,
		Platform:    "video",
		Action:      "submit",
	})

	require.NoError(t, err)
	require.Equal(t, group.Id, loaded.Id)
	ReleaseAccountPoolTaskLimit(ctx)
	require.Empty(t, accountPoolTaskConcurrency)
}

func TestReserveAccountPoolTaskLimitConcurrencyReserveAndRelease(t *testing.T) {
	setupAccountPoolTaskLimitTest(t)
	group := createTaskLimitGroup(t, &model.AccountPoolGroup{
		TaskMaxConcurrency: 1,
		TaskLimitAction:    model.AccountPoolTaskLimitActionFail,
	})
	first := newAccountPoolTaskLimitContext()

	_, err := ReserveAccountPoolTaskLimit(first, AccountPoolTaskLimitOptions{
		PoolGroupID: group.Id,
		Platform:    "Video Task",
		Action:      "Submit:HD",
	})
	require.NoError(t, err)
	require.Equal(t, 1, accountPoolTaskConcurrency[BuildAccountPoolTaskLimitKey(group.Id, "Video Task", "Submit:HD")])

	second := newAccountPoolTaskLimitContext()
	_, err = ReserveAccountPoolTaskLimit(second, AccountPoolTaskLimitOptions{
		PoolGroupID: group.Id,
		Platform:    "Video Task",
		Action:      "Submit:HD",
	})
	require.True(t, errors.Is(err, ErrAccountPoolTaskConcurrencyExceeded))

	ReleaseAccountPoolTaskLimit(first)
	require.Empty(t, accountPoolTaskConcurrency)

	_, err = ReserveAccountPoolTaskLimit(second, AccountPoolTaskLimitOptions{
		PoolGroupID: group.Id,
		Platform:    "Video Task",
		Action:      "Submit:HD",
	})
	require.NoError(t, err)
	ReleaseAccountPoolTaskLimit(second)
}

func TestReserveAccountPoolTaskLimitWaitsForReleasedConcurrency(t *testing.T) {
	setupAccountPoolTaskLimitTest(t)
	group := createTaskLimitGroup(t, &model.AccountPoolGroup{
		TaskMaxConcurrency:   1,
		TaskLimitAction:      model.AccountPoolTaskLimitActionWait,
		TaskLimitWaitSeconds: 1,
	})
	first := newAccountPoolTaskLimitContext()
	_, err := ReserveAccountPoolTaskLimit(first, AccountPoolTaskLimitOptions{
		PoolGroupID: group.Id,
		Platform:    "video",
		Action:      "submit",
	})
	require.NoError(t, err)

	done := make(chan error, 1)
	second := newAccountPoolTaskLimitContext()
	go func() {
		_, waitErr := ReserveAccountPoolTaskLimit(second, AccountPoolTaskLimitOptions{
			PoolGroupID: group.Id,
			Platform:    "video",
			Action:      "submit",
		})
		done <- waitErr
	}()

	time.Sleep(20 * time.Millisecond)
	ReleaseAccountPoolTaskLimit(first)
	require.NoError(t, <-done)
	ReleaseAccountPoolTaskLimit(second)
}

func TestReserveAccountPoolTaskLimitWaitTimeout(t *testing.T) {
	setupAccountPoolTaskLimitTest(t)
	group := createTaskLimitGroup(t, &model.AccountPoolGroup{
		TaskMaxConcurrency:   1,
		TaskLimitAction:      model.AccountPoolTaskLimitActionWait,
		TaskLimitWaitSeconds: 1,
	})
	accountPoolTaskLimitRetryInterval = 2 * time.Millisecond
	first := newAccountPoolTaskLimitContext()
	_, err := ReserveAccountPoolTaskLimit(first, AccountPoolTaskLimitOptions{
		PoolGroupID: group.Id,
		Platform:    "video",
		Action:      "submit",
	})
	require.NoError(t, err)

	second := newAccountPoolTaskLimitContext()
	started := time.Now()
	_, err = ReserveAccountPoolTaskLimit(second, AccountPoolTaskLimitOptions{
		PoolGroupID: group.Id,
		Platform:    "video",
		Action:      "submit",
	})

	require.True(t, errors.Is(err, ErrAccountPoolTaskWaitTimeout))
	require.GreaterOrEqual(t, time.Since(started), 900*time.Millisecond)
	ReleaseAccountPoolTaskLimit(first)
}

func TestReserveAccountPoolTaskLimitRateLimitExceeded(t *testing.T) {
	setupAccountPoolTaskLimitTest(t)
	group := createTaskLimitGroup(t, &model.AccountPoolGroup{
		TaskRateLimitRpm: 1,
	})
	first := newAccountPoolTaskLimitContext()
	_, err := ReserveAccountPoolTaskLimit(first, AccountPoolTaskLimitOptions{
		PoolGroupID: group.Id,
		Platform:    "video",
		Action:      "submit",
	})
	require.NoError(t, err)

	second := newAccountPoolTaskLimitContext()
	_, err = ReserveAccountPoolTaskLimit(second, AccountPoolTaskLimitOptions{
		PoolGroupID: group.Id,
		Platform:    "video",
		Action:      "submit",
	})

	require.True(t, errors.Is(err, ErrAccountPoolTaskRateLimitExceeded))
}

func TestBuildAccountPoolTaskLimitKeyNormalizesParts(t *testing.T) {
	require.Equal(t, "7:video_task:submit_hd", BuildAccountPoolTaskLimitKey(7, " Video Task ", "Submit:HD"))
	require.Equal(t, "7:unknown:default", BuildAccountPoolTaskLimitKey(7, " ", ""))
}
