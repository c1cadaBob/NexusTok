package service

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelAccountErrorUpdatesAffectCapabilities(t *testing.T) {
	assert.True(t, channelAccountErrorUpdatesAffectCapabilities(map[string]interface{}{
		"status": common.ChannelStatusAutoDisabled,
	}))
	assert.True(t, channelAccountErrorUpdatesAffectCapabilities(map[string]interface{}{
		"rate_limited_until": int64(100),
	}))
	assert.True(t, channelAccountErrorUpdatesAffectCapabilities(map[string]interface{}{
		"overload_until": int64(100),
	}))
	assert.True(t, channelAccountErrorUpdatesAffectCapabilities(map[string]interface{}{
		"temp_disabled_until": int64(100),
	}))
	assert.False(t, channelAccountErrorUpdatesAffectCapabilities(map[string]interface{}{
		"last_error": "upstream rejected request",
	}))
}

// TestProcessChannelAccountErrorCoolsServiceUnavailableAccount 覆盖 503 的账户冷却：
// 失败凭据必须被当前请求排除、持久化为短冷却，并让同渠道备用凭据立即可被选中。
func TestProcessChannelAccountErrorCoolsServiceUnavailableAccount(t *testing.T) {
	db := setupChannelAccountSelectTestDB(t)
	gin.SetMode(gin.TestMode)

	channel := model.Channel{
		Type:   constant.ChannelTypeOpenAI,
		Status: common.ChannelStatusEnabled,
		Name:   "service-unavailable-channel",
		Models: "gpt-service-unavailable",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			CredentialMode:     constant.ChannelCredentialModeAccountPool,
			AccountPoolEnabled: true,
		},
	}
	require.NoError(t, db.Create(&channel).Error)
	failed := model.ChannelAccount{
		ChannelId: channel.Id,
		Name:      "failed-account",
		Key:       "sk-secret-503",
		Status:    common.ChannelStatusEnabled,
		Models:    "gpt-service-unavailable",
		Group:     "default",
		Priority:  10,
		Weight:    100,
	}
	backup := model.ChannelAccount{
		ChannelId: channel.Id,
		Name:      "backup-account",
		Key:       "sk-backup-503",
		Status:    common.ChannelStatusEnabled,
		Models:    "gpt-service-unavailable",
		Group:     "default",
		Priority:  10,
		Weight:    100,
	}
	require.NoError(t, db.Create(&failed).Error)
	require.NoError(t, db.Create(&backup).Error)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	before := time.Now()
	upstreamErr := types.NewOpenAIError(
		errors.New("Service temporarily unavailable for sk-secret-503"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusServiceUnavailable,
	)
	ProcessChannelAccountError(ctx, types.ChannelError{
		ChannelId:        channel.Id,
		ChannelAccountId: failed.Id,
	}, upstreamErr)

	var stored model.ChannelAccount
	require.NoError(t, db.First(&stored, failed.Id).Error)
	require.InDelta(t, before.Add(defaultChannelAccountServiceUnavailableCooldown).Unix(), stored.OverloadUntil, 2)
	require.NotContains(t, stored.LastError, "sk-secret-503")
	require.True(t, GetExcludedChannelAccountIds(ctx)[failed.Id])

	selected, err := SelectChannelAccount(ctx, &channel, "gpt-service-unavailable", "default", 0)
	require.NoError(t, err)
	require.Equal(t, backup.Id, selected.Id)
	ReleaseSelectedChannelAccount(ctx)
}

// TestProcessChannelAccountErrorHonorsServiceUnavailableRetryAfter 验证 503 优先遵循
// Retry-After 的秒数与 HTTP 日期格式，而没有响应头时才使用 60 秒默认冷却。
func TestProcessChannelAccountErrorHonorsServiceUnavailableRetryAfter(t *testing.T) {
	testCases := []struct {
		name       string
		retryAfter string
		expected   func(time.Time) int64
	}{
		{
			name:       "seconds",
			retryAfter: "120",
			expected: func(now time.Time) int64 {
				return now.Add(120 * time.Second).Unix()
			},
		},
		{
			name: "http date",
			expected: func(now time.Time) int64 {
				return now.Add(180 * time.Second).UTC().Unix()
			},
		},
		{
			name:       "fallback",
			retryAfter: "",
			expected: func(now time.Time) int64 {
				return now.Add(defaultChannelAccountServiceUnavailableCooldown).Unix()
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			db := setupChannelAccountSelectTestDB(t)
			gin.SetMode(gin.TestMode)

			channel := model.Channel{
				Type:   constant.ChannelTypeOpenAI,
				Status: common.ChannelStatusEnabled,
				Name:   "retry-after-channel",
				Models: "gpt-retry-after",
				Group:  "default",
			}
			require.NoError(t, db.Create(&channel).Error)
			account := model.ChannelAccount{
				ChannelId: channel.Id,
				Name:      "retry-after-account",
				Key:       "sk-retry-after",
				Status:    common.ChannelStatusEnabled,
				Models:    "gpt-retry-after",
				Group:     "default",
			}
			require.NoError(t, db.Create(&account).Error)

			now := time.Now()
			retryAfter := testCase.retryAfter
			if testCase.name == "http date" {
				retryAfter = now.Add(180 * time.Second).UTC().Format(http.TimeFormat)
			}
			upstreamErr := types.NewOpenAIError(
				errors.New("service temporarily unavailable"),
				types.ErrorCodeBadResponseStatusCode,
				http.StatusServiceUnavailable,
			)
			upstreamErr.RetryAfter = retryAfter
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ProcessChannelAccountError(ctx, types.ChannelError{
				ChannelId:        channel.Id,
				ChannelAccountId: account.Id,
			}, upstreamErr)

			var stored model.ChannelAccount
			require.NoError(t, db.First(&stored, account.Id).Error)
			require.InDelta(t, testCase.expected(now), stored.OverloadUntil, 2)
		})
	}
}
