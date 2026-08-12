package upstreamaccount

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMergeChannelSyncMetadataPreservesBalanceSnapshotWhenRefreshOmitsBalance(t *testing.T) {
	previous := mergeChannelSyncMetadata("", &Snapshot{
		Platform: "new-api",
		BaseURL:  "https://upstream.example",
		Balance: &BalanceSnapshot{
			BalanceUSD: floatPtr(38.11),
			UsedUSD:    floatPtr(11.89),
			Source:     "dashboard",
		},
	})

	refreshed := mergeChannelSyncMetadata(previous, &Snapshot{
		Platform: "new-api",
		BaseURL:  "https://upstream.example",
		Keys: []SyncedKey{{
			Name: "key-a",
			Key:  "sk-a",
		}},
	})

	snapshot, ok := ReadChannelAccountBalanceSnapshot(refreshed)
	require.True(t, ok)
	require.NotNil(t, snapshot.BalanceUSD)
	require.NotNil(t, snapshot.UsedUSD)
	require.InDelta(t, 38.11, *snapshot.BalanceUSD, 0.000001)
	require.InDelta(t, 11.89, *snapshot.UsedUSD, 0.000001)
}
