package service

import (
	"context"
	"testing"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/service/accountauth"
	"github.com/stretchr/testify/require"
)

type fakeCheckRefreshProvider struct{}

func (fakeCheckRefreshProvider) Name() string {
	return "codex-check-refresh"
}

func (fakeCheckRefreshProvider) DisplayName() string {
	return "Codex Check Refresh"
}

func (fakeCheckRefreshProvider) SupportsOAuth() bool {
	return true
}

func (fakeCheckRefreshProvider) SupportsDevice() bool {
	return false
}

func (fakeCheckRefreshProvider) RefreshLead() *time.Duration {
	lead := time.Minute
	return &lead
}

func (fakeCheckRefreshProvider) StartOAuth(context.Context, *model.AccountPoolGroup, accountauth.LoginStartRequest) (*accountauth.LoginStartResult, error) {
	return nil, nil
}

func (fakeCheckRefreshProvider) CompleteOAuth(context.Context, *model.AccountPoolGroup, accountauth.LoginCompleteRequest) (*accountauth.AccountCredential, error) {
	return nil, nil
}

func (fakeCheckRefreshProvider) StartDevice(context.Context, *model.AccountPoolGroup, accountauth.LoginStartRequest) (*accountauth.LoginStartResult, error) {
	return nil, nil
}

func (fakeCheckRefreshProvider) CompleteDevice(context.Context, *model.AccountPoolGroup, accountauth.LoginCompleteRequest) (*accountauth.AccountCredential, error) {
	return nil, nil
}

func (fakeCheckRefreshProvider) Refresh(_ context.Context, account *model.PoolAccount) (*accountauth.AccountCredential, error) {
	now := time.Now()
	return &accountauth.AccountCredential{
		Provider:        "codex-check-refresh",
		AuthType:        model.AccountPoolAuthTypeOfficialOAuth,
		Label:           account.Name,
		Credentials:     `{"access_token":"fresh-access-token","refresh_token":"fresh-refresh-token"}`,
		Summary:         "fresh-access-token",
		LastRefreshedAt: now,
		NextRefreshAt:   now.Add(time.Hour),
	}, nil
}

func (fakeCheckRefreshProvider) BuildChannelKey(*model.PoolAccount) (string, error) {
	return "fresh-access-token", nil
}

func (fakeCheckRefreshProvider) Summarize(raw string) string {
	return model.NormalizeAccountPoolCredentialSummary(raw)
}

func setupAccountPoolCheckTest(t *testing.T) {
	t.Helper()
	require.NoError(t, model.DB.AutoMigrate(&model.AccountPoolGroup{}, &model.PoolAccount{}))
	require.NoError(t, model.DB.Exec("DELETE FROM pool_accounts").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM account_pool_groups").Error)
	t.Cleanup(func() {
		_ = model.DB.Exec("DELETE FROM pool_accounts").Error
		_ = model.DB.Exec("DELETE FROM account_pool_groups").Error
	})
}

func createCheckTestGroup(t *testing.T) *model.AccountPoolGroup {
	t.Helper()
	group := &model.AccountPoolGroup{
		Name:     "codex-check",
		Platform: "codex",
		AuthType: model.AccountPoolAuthTypeAPIKey,
		Source:   model.AccountPoolGroupSourceNative,
		Status:   common.ChannelStatusEnabled,
		Strategy: model.AccountPoolStrategyRoundRobin,
	}
	require.NoError(t, model.DB.Create(group).Error)
	return group
}

func encryptedCheckCredential(t *testing.T, raw string) string {
	t.Helper()
	encrypted, err := common.EncryptSensitiveString(raw)
	require.NoError(t, err)
	return encrypted
}

func TestCheckPoolAccountMarksBasicCredentialAvailable(t *testing.T) {
	setupAccountPoolCheckTest(t)
	group := createCheckTestGroup(t)
	account := &model.PoolAccount{
		PoolGroupId:        group.Id,
		Name:               "check-ok",
		Platform:           "codex",
		AuthType:           model.AccountPoolAuthTypeAPIKey,
		CredentialProvider: "codex",
		Credentials:        encryptedCheckCredential(t, `{"api_key":"sk-test"}`),
		Status:             common.ChannelStatusAutoDisabled,
		Schedulable:        false,
		Unavailable:        true,
		LastError:          "old error",
		NextRetryTime:      common.GetTimestamp() + 3600,
	}
	require.NoError(t, model.DB.Create(account).Error)

	result, err := CheckPoolAccount(context.Background(), account.Id)

	require.NoError(t, err)
	require.True(t, result.Checked)
	require.True(t, result.Success)
	require.False(t, result.Refreshed)
	require.NotZero(t, result.CheckedAt)

	updated, err := model.GetPoolAccountById(account.Id)
	require.NoError(t, err)
	require.Equal(t, common.ChannelStatusEnabled, updated.Status)
	require.True(t, updated.Schedulable)
	require.False(t, updated.Unavailable)
	require.Empty(t, updated.LastError)
	require.Zero(t, updated.NextRetryTime)
	require.NotZero(t, updated.LastCheckedTime)
	require.EqualValues(t, 1, updated.SuccessCount)
}

func TestCheckPoolAccountMarksInvalidCredentialUnavailable(t *testing.T) {
	setupAccountPoolCheckTest(t)
	group := createCheckTestGroup(t)
	account := &model.PoolAccount{
		PoolGroupId:        group.Id,
		Name:               "check-bad",
		Platform:           "codex",
		AuthType:           model.AccountPoolAuthTypeOfficialOAuth,
		CredentialProvider: "codex",
		Credentials:        encryptedCheckCredential(t, `{"access_token":"missing-refresh-token"}`),
		Status:             common.ChannelStatusEnabled,
		Schedulable:        true,
	}
	require.NoError(t, model.DB.Create(account).Error)

	result, err := CheckPoolAccount(context.Background(), account.Id)

	require.NoError(t, err)
	require.True(t, result.Checked)
	require.False(t, result.Success)
	require.NotEmpty(t, result.Message)
	require.NotZero(t, result.NextRetryTime)

	updated, err := model.GetPoolAccountById(account.Id)
	require.NoError(t, err)
	require.True(t, updated.Unavailable)
	require.NotEmpty(t, updated.LastError)
	require.NotZero(t, updated.NextRetryTime)
	require.NotZero(t, updated.LastCheckedTime)
	require.EqualValues(t, 1, updated.FailedCount)
}

func TestCheckPoolAccountKeepsManualDisabledAccountUnschedulableAfterRefresh(t *testing.T) {
	setupAccountPoolCheckTest(t)
	accountauth.RegisterProvider(fakeCheckRefreshProvider{})
	group := createCheckTestGroup(t)
	group.Platform = "codex-check-refresh"
	group.AuthType = model.AccountPoolAuthTypeOfficialOAuth
	require.NoError(t, model.DB.Save(group).Error)
	account := &model.PoolAccount{
		PoolGroupId:        group.Id,
		Name:               "check-refresh-disabled",
		Platform:           "codex-check-refresh",
		AuthType:           model.AccountPoolAuthTypeOfficialOAuth,
		CredentialProvider: "codex-check-refresh",
		Credentials:        encryptedCheckCredential(t, `{"access_token":"old","refresh_token":"refresh"}`),
		Status:             common.ChannelStatusManuallyDisabled,
		Schedulable:        false,
		Unavailable:        true,
		LastError:          "old refresh error",
	}
	require.NoError(t, model.DB.Create(account).Error)

	result, err := CheckPoolAccount(context.Background(), account.Id)

	require.NoError(t, err)
	require.True(t, result.Checked)
	require.True(t, result.Success)
	require.True(t, result.Refreshed)

	updated, err := model.GetPoolAccountById(account.Id)
	require.NoError(t, err)
	require.Equal(t, common.ChannelStatusManuallyDisabled, updated.Status)
	require.False(t, updated.Schedulable)
	require.False(t, updated.Unavailable)
	require.Empty(t, updated.LastError)
	require.NotZero(t, updated.LastCheckedTime)
	require.EqualValues(t, 1, updated.SuccessCount)
}

func TestCheckPoolAccountsInGroupSummarizesResults(t *testing.T) {
	setupAccountPoolCheckTest(t)
	group := createCheckTestGroup(t)
	accounts := []*model.PoolAccount{
		{
			PoolGroupId:        group.Id,
			Name:               "check-ok",
			Platform:           "codex",
			AuthType:           model.AccountPoolAuthTypeAPIKey,
			CredentialProvider: "codex",
			Credentials:        encryptedCheckCredential(t, `{"api_key":"sk-test"}`),
			Status:             common.ChannelStatusEnabled,
			Schedulable:        true,
		},
		{
			PoolGroupId:        group.Id,
			Name:               "check-bad",
			Platform:           "codex",
			AuthType:           model.AccountPoolAuthTypeOfficialOAuth,
			CredentialProvider: "codex",
			Credentials:        encryptedCheckCredential(t, `{"access_token":"missing-refresh-token"}`),
			Status:             common.ChannelStatusEnabled,
			Schedulable:        true,
		},
	}
	require.NoError(t, model.DB.Create(&accounts).Error)

	result, err := CheckPoolAccountsInGroup(context.Background(), group.Id, 10)

	require.NoError(t, err)
	require.Equal(t, 2, result.Total)
	require.Equal(t, 2, result.Checked)
	require.Equal(t, 1, result.Success)
	require.Equal(t, 1, result.Failed)
	require.Equal(t, 0, result.Skipped)
	require.Len(t, result.Items, 2)
}
