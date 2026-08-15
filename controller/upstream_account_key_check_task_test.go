package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/service/upstreamaccount"
	"github.com/c1cada/NexusTok/setting/operation_setting"
	"github.com/stretchr/testify/require"
)

func TestUpstreamAccountKeyCheckDisablesAfterFailureThreshold(t *testing.T) {
	db := setupChannelAccountMutationTestDB(t)
	withUpstreamAccountKeyCheckSetting(t, operation_setting.UpstreamAccountKeyCheckSetting{
		Enabled:            true,
		IntervalMinutes:    30,
		FailureThreshold:   2,
		AutoRecoverEnabled: true,
	})

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/models", r.URL.Path)
		http.Error(w, `{"error":{"message":"bad key sk-secret"}}`, http.StatusUnauthorized)
	}))
	defer upstream.Close()

	channel, account := createUpstreamAccountKeyCheckFixture(t, upstream.URL, common.ChannelStatusEnabled, `{"upstream_account_sync":{"platform":"new-api","base_url":"`+upstream.URL+`","external_id":"key-1","key_digest":"digest","ratio_conversion":0.2}}`)

	first, err := runUpstreamAccountKeyCheckTask(context.Background(), nil)
	require.NoError(t, err)
	require.Equal(t, 1, first.FailedAccounts)
	require.Equal(t, 0, first.DisabledAccounts)

	var afterFirst model.ChannelAccount
	require.NoError(t, db.First(&afterFirst, account.Id).Error)
	require.Equal(t, common.ChannelStatusEnabled, afterFirst.Status)
	require.Equal(t, 1, upstreamaccount.ReadAccountAutoCheckMetadata(afterFirst.OtherSettings).FailureCount)

	second, err := runUpstreamAccountKeyCheckTask(context.Background(), nil)
	require.NoError(t, err)
	require.Equal(t, 1, second.DisabledAccounts)

	var disabled model.ChannelAccount
	require.NoError(t, db.First(&disabled, account.Id).Error)
	require.Equal(t, channel.Id, disabled.ChannelId)
	require.Equal(t, common.ChannelStatusAutoDisabled, disabled.Status)
	metadata := upstreamaccount.ReadAccountAutoCheckMetadata(disabled.OtherSettings)
	require.Equal(t, 2, metadata.FailureCount)
	require.True(t, metadata.DisabledByAutoCheck)
	require.NotContains(t, metadata.LastError, "sk-secret")
}

func TestUpstreamAccountKeyCheckRecoversAutoDisabledAccount(t *testing.T) {
	db := setupChannelAccountMutationTestDB(t)
	withUpstreamAccountKeyCheckSetting(t, operation_setting.UpstreamAccountKeyCheckSetting{
		Enabled:            true,
		IntervalMinutes:    30,
		FailureThreshold:   2,
		AutoRecoverEnabled: true,
	})

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/models", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-recovered"}]}`))
	}))
	defer upstream.Close()

	settings := `{"upstream_account_sync":{"platform":"new-api","base_url":"` + upstream.URL + `","external_id":"key-1","key_digest":"digest","ratio_conversion":0.2,"auto_check_failure_count":2,"auto_check_disabled_by_auto_check":true,"auto_check_disabled_at":123}}`
	_, account := createUpstreamAccountKeyCheckFixture(t, upstream.URL, common.ChannelStatusAutoDisabled, settings)

	summary, err := runUpstreamAccountKeyCheckTask(context.Background(), nil)
	require.NoError(t, err)
	require.Equal(t, 1, summary.SucceededAccounts)
	require.Equal(t, 1, summary.RecoveredAccounts)

	var recovered model.ChannelAccount
	require.NoError(t, db.First(&recovered, account.Id).Error)
	require.Equal(t, common.ChannelStatusEnabled, recovered.Status)
	metadata := upstreamaccount.ReadAccountAutoCheckMetadata(recovered.OtherSettings)
	require.Equal(t, 0, metadata.FailureCount)
	require.False(t, metadata.DisabledByAutoCheck)
	require.NotZero(t, metadata.LastSuccessAt)
}

func TestUpstreamAccountKeyCheckSkipsManuallyDisabledAccounts(t *testing.T) {
	db := setupChannelAccountMutationTestDB(t)
	withUpstreamAccountKeyCheckSetting(t, operation_setting.UpstreamAccountKeyCheckSetting{
		Enabled:            true,
		IntervalMinutes:    30,
		FailureThreshold:   1,
		AutoRecoverEnabled: true,
	})

	requests := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-should-not-run"}]}`))
	}))
	defer upstream.Close()

	_, account := createUpstreamAccountKeyCheckFixture(t, upstream.URL, common.ChannelStatusManuallyDisabled, `{"upstream_account_sync":{"platform":"new-api","base_url":"`+upstream.URL+`","external_id":"key-1","key_digest":"digest","ratio_conversion":0.2,"auto_check_failure_count":2,"auto_check_disabled_by_auto_check":true,"auto_check_disabled_at":123}}`)

	summary, err := runUpstreamAccountKeyCheckTask(context.Background(), nil)
	require.NoError(t, err)
	require.Equal(t, 1, summary.ScannedAccounts)
	require.Equal(t, 0, summary.EligibleAccounts)
	require.Equal(t, 1, summary.SkippedAccounts)
	require.Equal(t, 0, requests)

	var stored model.ChannelAccount
	require.NoError(t, db.First(&stored, account.Id).Error)
	require.Equal(t, common.ChannelStatusManuallyDisabled, stored.Status)
	require.True(t, upstreamaccount.ReadAccountAutoCheckMetadata(stored.OtherSettings).DisabledByAutoCheck)
}

func TestUpdateChannelAccountStatusClearsAutoCheckMarkerOnManualRestore(t *testing.T) {
	db := setupChannelAccountMutationTestDB(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-restored"}]}`))
	}))
	defer upstream.Close()

	channel, account := createUpstreamAccountKeyCheckFixture(t, upstream.URL, common.ChannelStatusAutoDisabled, `{"upstream_account_sync":{"platform":"new-api","base_url":"`+upstream.URL+`","external_id":"key-1","key_digest":"digest","ratio_conversion":0.2,"auto_check_failure_count":2,"auto_check_disabled_by_auto_check":true,"auto_check_disabled_at":123,"auto_check_last_error":"masked failure"}}`)
	ctx, recorder := createChannelAccountMutationRequest(t, http.MethodPost, channel.Id, account.Id, map[string]any{
		"status": common.ChannelStatusEnabled,
		"reason": "manual restore",
	})

	UpdateChannelAccountStatus(ctx)

	requireChannelAccountMutationSucceeded(t, recorder)
	var restored model.ChannelAccount
	require.NoError(t, db.First(&restored, account.Id).Error)
	require.Equal(t, common.ChannelStatusEnabled, restored.Status)
	metadata := upstreamaccount.ReadAccountAutoCheckMetadata(restored.OtherSettings)
	require.Equal(t, 0, metadata.FailureCount)
	require.False(t, metadata.DisabledByAutoCheck)
	require.Equal(t, "manual", metadata.LastStatus)
}

func TestUpstreamAccountKeyCheckHonorsRatioThreshold(t *testing.T) {
	db := setupChannelAccountMutationTestDB(t)
	withUpstreamAccountKeyCheckSetting(t, operation_setting.UpstreamAccountKeyCheckSetting{
		Enabled:            true,
		IntervalMinutes:    30,
		RatioThreshold:     1,
		FailureThreshold:   1,
		AutoRecoverEnabled: true,
	})
	requests := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-low"}]}`))
	}))
	defer upstream.Close()

	channel, lowRatio := createUpstreamAccountKeyCheckFixture(t, upstream.URL, common.ChannelStatusEnabled, `{"upstream_account_sync":{"platform":"new-api","base_url":"`+upstream.URL+`","external_id":"low","key_digest":"low","ratio_conversion":0.5}}`)
	highRatio := model.ChannelAccount{
		ChannelId:     channel.Id,
		Name:          "high-ratio",
		Key:           "sk-high",
		Status:        common.ChannelStatusEnabled,
		Models:        "gpt-high",
		Group:         "default",
		AccessGroups:  "default",
		OtherSettings: `{"upstream_account_sync":{"platform":"new-api","base_url":"` + upstream.URL + `","external_id":"high","key_digest":"high","ratio_conversion":2}}`,
	}
	require.NoError(t, db.Create(&highRatio).Error)

	summary, err := runUpstreamAccountKeyCheckTask(context.Background(), nil)
	require.NoError(t, err)
	require.Equal(t, 2, summary.ScannedAccounts)
	require.Equal(t, 1, summary.EligibleAccounts)
	require.Equal(t, 1, summary.SkippedAccounts)
	require.Equal(t, 1, summary.SucceededAccounts)
	require.Equal(t, 1, requests)

	var checked model.ChannelAccount
	require.NoError(t, db.First(&checked, lowRatio.Id).Error)
	require.NotZero(t, upstreamaccount.ReadAccountAutoCheckMetadata(checked.OtherSettings).LastSuccessAt)
}

func TestUpstreamAccountKeyCheckRatioThresholdSkipsMissingConvertedRatio(t *testing.T) {
	setupChannelAccountMutationTestDB(t)
	withUpstreamAccountKeyCheckSetting(t, operation_setting.UpstreamAccountKeyCheckSetting{
		Enabled:            true,
		IntervalMinutes:    30,
		RatioThreshold:     1,
		FailureThreshold:   1,
		AutoRecoverEnabled: true,
	})
	requests := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-missing-ratio"}]}`))
	}))
	defer upstream.Close()

	_, _ = createUpstreamAccountKeyCheckFixture(t, upstream.URL, common.ChannelStatusEnabled, `{"upstream_account_sync":{"platform":"new-api","base_url":"`+upstream.URL+`","external_id":"missing","key_digest":"missing","effective_ratio":0.5}}`)

	summary, err := runUpstreamAccountKeyCheckTask(context.Background(), nil)
	require.NoError(t, err)
	require.Equal(t, 1, summary.ScannedAccounts)
	require.Equal(t, 0, summary.EligibleAccounts)
	require.Equal(t, 1, summary.SkippedAccounts)
	require.Equal(t, 0, requests)
}

func createUpstreamAccountKeyCheckFixture(t *testing.T, baseURL string, accountStatus int, accountSettings string) (model.Channel, model.ChannelAccount) {
	t.Helper()
	db := model.DB
	channel := model.Channel{
		Type:          constant.ChannelTypeOpenAI,
		Key:           constant.ChannelCredentialModeAccountPool,
		Name:          "key-check-channel",
		Status:        common.ChannelStatusEnabled,
		BaseURL:       &baseURL,
		Models:        "gpt-test",
		Group:         "default",
		OtherSettings: `{"upstream_account_sync":{"platform":"new-api","base_url":"` + baseURL + `"}}`,
		ChannelInfo: model.ChannelInfo{
			CredentialMode:     constant.ChannelCredentialModeAccountPool,
			AccountPoolEnabled: true,
		},
	}
	require.NoError(t, db.Create(&channel).Error)
	account := model.ChannelAccount{
		ChannelId:      channel.Id,
		Name:           "key-check-account",
		Key:            "sk-secret",
		Status:         accountStatus,
		Models:         "gpt-test",
		Group:          "default",
		AccessGroups:   "default",
		OtherSettings:  accountSettings,
		DisabledReason: "previous",
	}
	require.NoError(t, db.Create(&account).Error)
	return channel, account
}

func withUpstreamAccountKeyCheckSetting(t *testing.T, next operation_setting.UpstreamAccountKeyCheckSetting) {
	t.Helper()
	setting := operation_setting.GetUpstreamAccountKeyCheckSetting()
	previous := *setting
	*setting = next
	t.Cleanup(func() {
		*setting = previous
	})
}
