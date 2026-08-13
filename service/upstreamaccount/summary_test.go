package upstreamaccount

import (
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
	"github.com/stretchr/testify/require"
)

func TestSummarizeUpstreamAccountsUsesAccountLevelSnapshots(t *testing.T) {
	setupAutomaticSyncTestDB(t)

	snapshot := &Snapshot{
		Platform: PlatformNewAPI,
		BaseURL:  "https://upstream.example",
		Balance: &BalanceSnapshot{
			BalanceUSD: floatPtr(38.11),
			UsedUSD:    floatPtr(11.89),
			Source:     "dashboard",
		},
	}
	channel := model.Channel{
		Key:                constant.ChannelCredentialModeAccountPool,
		Name:               "synced-channel",
		Status:             common.ChannelStatusManuallyDisabled,
		Balance:            1,
		BalanceUpdatedTime: 123,
		OtherSettings:      mergeChannelSyncMetadata("", snapshot),
		ChannelInfo: model.ChannelInfo{
			CredentialMode:     constant.ChannelCredentialModeAccountPool,
			AccountPoolEnabled: true,
		},
	}
	require.NoError(t, model.DB.Create(&channel).Error)

	require.NoError(t, model.DB.Create(&model.ChannelAccount{
		ChannelId: channel.Id,
		Name:      "synced-key",
		Key:       "sk-a",
		UsedQuota: int64(999 * common.QuotaPerUnit),
	}).Error)

	summary, err := SummarizeUpstreamAccounts()
	require.NoError(t, err)
	require.Equal(t, 1, summary.SyncedChannelCount)
	require.InDelta(t, 38.11, summary.UpstreamBalanceUSD, 0.000001)
	require.InDelta(t, 11.89, summary.UpstreamUsedUSD, 0.000001)
	require.Equal(t, int64(11.89*common.QuotaPerUnit), summary.UpstreamUsedQuota)
	require.False(t, summary.Partial)
	require.Greater(t, summary.UpdatedAt, int64(0))
}

func TestSummarizeUpstreamAccountsFallsBackToSyncedKeyUsageForLegacyData(t *testing.T) {
	setupAutomaticSyncTestDB(t)

	channels := []model.Channel{
		{
			Key:                constant.ChannelCredentialModeAccountPool,
			Name:               "legacy-synced-channel",
			Status:             common.ChannelStatusEnabled,
			Balance:            7.5,
			BalanceUpdatedTime: 456,
			OtherSettings:      `{"upstream_account_sync":{"platform":"new-api","base_url":"https://legacy.example","synced_at":1}}`,
			ChannelInfo: model.ChannelInfo{
				CredentialMode:     constant.ChannelCredentialModeAccountPool,
				AccountPoolEnabled: true,
			},
		},
		{
			Key:           "plain-key",
			Name:          "plain-channel",
			Status:        common.ChannelStatusEnabled,
			Balance:       100,
			OtherSettings: `{}`,
		},
	}
	require.NoError(t, model.DB.Create(&channels).Error)

	require.NoError(t, model.DB.Create(&[]model.ChannelAccount{
		{ChannelId: channels[0].Id, Name: "a", Key: "sk-a", UsedQuota: int64(2 * common.QuotaPerUnit)},
		{ChannelId: channels[0].Id, Name: "b", Key: "sk-b", UsedQuota: int64(3 * common.QuotaPerUnit)},
		{ChannelId: channels[1].Id, Name: "plain", Key: "sk-plain", UsedQuota: int64(200 * common.QuotaPerUnit)},
	}).Error)

	summary, err := SummarizeUpstreamAccounts()
	require.NoError(t, err)
	require.Equal(t, 1, summary.SyncedChannelCount)
	require.InDelta(t, 7.5, summary.UpstreamBalanceUSD, 0.000001)
	require.InDelta(t, 5, summary.UpstreamUsedUSD, 0.000001)
	require.Equal(t, int64(5*common.QuotaPerUnit), summary.UpstreamUsedQuota)
	require.True(t, summary.Partial)
	require.Equal(t, int64(456), summary.UpdatedAt)
}

func TestSummarizeUpstreamAccountsIgnoresLocalAccountsInMixedSyncedChannel(t *testing.T) {
	setupAutomaticSyncTestDB(t)

	channel := model.Channel{
		Key:           constant.ChannelCredentialModeAccountPool,
		Name:          "mixed-synced-channel",
		Status:        common.ChannelStatusEnabled,
		Balance:       100,
		OtherSettings: `{"upstream_account_sync":{"platform":"sub2api","base_url":"https://mixed.example","ratio_conversion_config":{"paid_cny":1,"platform_usd_credit":10,"enabled":true}}}`,
		ChannelInfo: model.ChannelInfo{
			CredentialMode:     constant.ChannelCredentialModeAccountPool,
			AccountPoolEnabled: true,
		},
	}
	require.NoError(t, model.DB.Create(&channel).Error)

	snapshot := &Snapshot{
		Platform: PlatformSub2API,
		BaseURL:  "https://mixed.example",
		Keys: []SyncedKey{{
			ExternalID:   "upstream-key",
			Key:          "sk-upstream-key",
			QuotaUsedUSD: floatPtr(20),
		}},
	}
	ApplyRatioConversion(snapshot, RatioConversionConfig{
		PaidCNY:           1,
		PlatformUSDCredit: 10,
	})

	require.NoError(t, model.DB.Create(&[]model.ChannelAccount{
		{
			ChannelId:     channel.Id,
			Name:          "upstream-key",
			Key:           "sk-upstream-key",
			UsedQuota:     int64(20 * common.QuotaPerUnit),
			OtherSettings: mergeAccountSyncMetadata("", snapshot, snapshot.Keys[0]),
		},
		{
			ChannelId: channel.Id,
			Name:      "local-account",
			Key:       "sk-local-key",
			UsedQuota: int64(1000 * common.QuotaPerUnit),
		},
	}).Error)

	summary, err := SummarizeUpstreamAccounts()
	require.NoError(t, err)
	require.InDelta(t, 10, summary.UpstreamBalanceUSD, 0.000001)
	require.InDelta(t, 2, summary.UpstreamUsedUSD, 0.000001)
	require.Equal(t, int64(2*common.QuotaPerUnit), summary.UpstreamUsedQuota)
	require.True(t, summary.Partial)
}
