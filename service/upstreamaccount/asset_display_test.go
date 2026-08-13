package upstreamaccount

import (
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/stretchr/testify/require"
)

func TestBuildChannelAssetDisplayAppliesConversionFactor(t *testing.T) {
	snapshot := &Snapshot{
		Platform: PlatformSub2API,
		BaseURL:  "https://sub2api.example",
		Balance: &BalanceSnapshot{
			BalanceUSD: floatPtr(269.510572),
			UsedUSD:    floatPtr(965.10803),
			Source:     "sub2api:user/profile",
		},
	}
	ApplyRatioConversion(snapshot, RatioConversionConfig{
		PaidCNY:           1,
		PlatformUSDCredit: 10,
	})
	channel := &model.Channel{
		OtherSettings: mergeChannelSyncMetadata("", snapshot),
		Balance:       269.510572,
		UsedQuota:     123456,
	}

	display := BuildChannelAssetDisplay(channel, nil)

	require.True(t, display.HasBalance)
	require.True(t, display.HasUsed)
	require.InDelta(t, 0.1, display.ConversionFactor, 0.000001)
	require.NotNil(t, display.BalanceUSD)
	require.NotNil(t, display.UsedUSD)
	require.NotNil(t, display.UsedQuota)
	require.InDelta(t, 26.9510572, *display.BalanceUSD, 0.000001)
	require.InDelta(t, 96.510803, *display.UsedUSD, 0.000001)
	require.Equal(t, int64(48255402), *display.UsedQuota)
}

func TestBuildChannelAssetDisplayFallsBackToAccountConversionConfig(t *testing.T) {
	channel := &model.Channel{
		OtherSettings: `{"upstream_account_sync":{"platform":"new-api","base_url":"https://newapi.example","balance_snapshot":{"balance_usd":100,"used_usd":50}}}`,
		Balance:       100,
	}
	accountSnapshot := &Snapshot{
		Platform: PlatformNewAPI,
		BaseURL:  "https://newapi.example",
		Keys: []SyncedKey{{
			ExternalID:        "key-1",
			Key:               "sk-key",
			QuotaUsedUSD:      floatPtr(5),
			QuotaRemainingUSD: floatPtr(95),
		}},
	}
	ApplyRatioConversion(accountSnapshot, RatioConversionConfig{
		PaidCNY:           2,
		PlatformUSDCredit: 20,
	})
	accounts := []model.ChannelAccount{{
		OtherSettings: mergeAccountSyncMetadata("", accountSnapshot, accountSnapshot.Keys[0]),
	}}

	display := BuildChannelAssetDisplay(channel, accounts)

	require.InDelta(t, 0.1, display.ConversionFactor, 0.000001)
	require.NotNil(t, display.BalanceUSD)
	require.NotNil(t, display.UsedUSD)
	require.InDelta(t, 10, *display.BalanceUSD, 0.000001)
	require.InDelta(t, 5, *display.UsedUSD, 0.000001)
}

func TestBuildChannelAssetDisplayIgnoresLocalAccountsWhenSyncMetadataExists(t *testing.T) {
	channel := &model.Channel{
		OtherSettings: `{"upstream_account_sync":{"platform":"sub2api","base_url":"https://sub2api.example","balance_snapshot":{"balance_usd":100}}}`,
		Balance:       100,
	}
	snapshot := &Snapshot{
		Platform: PlatformSub2API,
		BaseURL:  "https://sub2api.example",
		Keys: []SyncedKey{{
			ExternalID:   "upstream-key",
			Key:          "sk-upstream-key",
			QuotaUsedUSD: floatPtr(2),
		}},
	}
	ApplyRatioConversion(snapshot, RatioConversionConfig{
		PaidCNY:           1,
		PlatformUSDCredit: 10,
	})

	accounts := []model.ChannelAccount{
		{
			OtherSettings: mergeAccountSyncMetadata("", snapshot, snapshot.Keys[0]),
			UsedQuota:     int64(2 * common.QuotaPerUnit),
		},
		{
			Name:      "local-account",
			UsedQuota: int64(1000 * common.QuotaPerUnit),
		},
	}

	display := BuildChannelAssetDisplay(channel, accounts)

	require.True(t, display.HasUsed)
	require.NotNil(t, display.UsedUSD)
	require.InDelta(t, 0.2, *display.UsedUSD, 0.000001)
}

func TestBuildAccountAssetDisplayAppliesConversionToSyncedKeySnapshots(t *testing.T) {
	snapshot := &Snapshot{
		Platform: PlatformSub2API,
		BaseURL:  "https://sub2api.example",
		Keys: []SyncedKey{{
			ExternalID:        "key-1",
			Key:               "sk-key",
			QuotaLimitUSD:     floatPtr(269.510572),
			QuotaUsedUSD:      floatPtr(965.10803),
			QuotaRemainingUSD: floatPtr(26.9510572),
		}},
	}
	ApplyRatioConversion(snapshot, RatioConversionConfig{
		PaidCNY:           1,
		PlatformUSDCredit: 10,
	})
	settings := mergeAccountSyncMetadata("", snapshot, snapshot.Keys[0])

	display := BuildAccountAssetDisplay(settings, 1)

	require.True(t, display.HasUsed)
	require.True(t, display.HasRemaining)
	require.InDelta(t, 0.1, display.ConversionFactor, 0.000001)
	require.NotNil(t, display.UsedUSD)
	require.NotNil(t, display.RemainingUSD)
	require.NotNil(t, display.UsedQuota)
	require.NotNil(t, display.RemainingQuota)
	require.InDelta(t, 96.510803, *display.UsedUSD, 0.000001)
	require.InDelta(t, 2.69510572, *display.RemainingUSD, 0.000001)
	require.Equal(t, int64(48255402), *display.UsedQuota)
}

func TestBuildAccountAssetDisplayKeepsPlainAccountsWithoutUpstreamFields(t *testing.T) {
	display := BuildAccountAssetDisplay(`{}`, int64(5*common.QuotaPerUnit))

	require.False(t, display.HasUsed)
	require.False(t, display.HasRemaining)
	require.Nil(t, display.UsedQuota)
	require.Nil(t, display.RemainingQuota)
}

func TestBuildAccountAssetDisplayDoesNotInventRemainingForUnlimitedKey(t *testing.T) {
	snapshot := &Snapshot{
		Platform: PlatformSub2API,
		BaseURL:  "https://sub2api.example",
		Keys: []SyncedKey{{
			ExternalID:        "unlimited-key",
			Key:               "sk-unlimited-key",
			QuotaUsedUSD:      floatPtr(3),
			QuotaRemainingUSD: nil,
			QuotaLimitUSD:     nil,
			Unlimited:         true,
		}},
	}
	ApplyRatioConversion(snapshot, RatioConversionConfig{
		PaidCNY:           1,
		PlatformUSDCredit: 10,
	})

	display := BuildAccountAssetDisplay(
		mergeAccountSyncMetadata("", snapshot, snapshot.Keys[0]),
		0,
	)

	require.True(t, display.HasUsed)
	require.False(t, display.HasRemaining)
	require.Nil(t, display.RemainingQuota)
}
