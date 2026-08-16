package upstreamaccount

import (
	"context"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRefreshSyncedAccountSchedulingSuggestionsOnlyTouchesSyncedAccounts(t *testing.T) {
	oldDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ChannelAccount{}))
	model.DB = db
	t.Cleanup(func() {
		model.DB = oldDB
	})

	cheapRatio := 0.8
	syncedEnabled := model.ChannelAccount{
		ChannelId:     10,
		Name:          "cheap",
		Status:        common.ChannelStatusEnabled,
		Priority:      9,
		Weight:        1,
		OtherSettings: mergeAccountSyncMetadata("", &Snapshot{Platform: PlatformNewAPI, BaseURL: "https://upstream.example"}, SyncedKey{ExternalID: "cheap", RatioConversion: cheapRatio}),
	}
	syncedDisabled := model.ChannelAccount{
		ChannelId:     11,
		Name:          "disabled",
		Status:        common.ChannelStatusManuallyDisabled,
		Priority:      8,
		Weight:        2,
		OtherSettings: mergeAccountSyncMetadata("", &Snapshot{Platform: PlatformSub2API, BaseURL: "https://sub.example"}, SyncedKey{ExternalID: "disabled"}),
	}
	ordinary := model.ChannelAccount{
		ChannelId: 12,
		Name:      "ordinary",
		Status:    common.ChannelStatusEnabled,
		Priority:  7,
		Weight:    3,
	}
	require.NoError(t, db.Create(&syncedEnabled).Error)
	require.NoError(t, db.Create(&syncedDisabled).Error)
	require.NoError(t, db.Create(&ordinary).Error)

	var progressCalls int
	summary, affected, err := RefreshSyncedAccountSchedulingSuggestions(context.Background(), func(processed, total int) {
		progressCalls++
		require.LessOrEqual(t, processed, total)
	})

	require.NoError(t, err)
	require.Equal(t, 2, summary.ScannedAccounts)
	require.Equal(t, 2, summary.UpdatedAccounts)
	require.Equal(t, 2, summary.AffectedChannels)
	require.ElementsMatch(t, []int{10, 11}, summary.ChannelIDs)
	require.Contains(t, affected, 10)
	require.Contains(t, affected, 11)
	require.Greater(t, progressCalls, 0)

	var refreshedCheap model.ChannelAccount
	require.NoError(t, db.First(&refreshedCheap, syncedEnabled.Id).Error)
	require.Equal(t, int64(0), refreshedCheap.Priority)
	require.Equal(t, 120, refreshedCheap.Weight)

	var refreshedDisabled model.ChannelAccount
	require.NoError(t, db.First(&refreshedDisabled, syncedDisabled.Id).Error)
	require.Equal(t, int64(0), refreshedDisabled.Priority)
	require.Equal(t, 100, refreshedDisabled.Weight)

	var refreshedOrdinary model.ChannelAccount
	require.NoError(t, db.First(&refreshedOrdinary, ordinary.Id).Error)
	require.Equal(t, int64(7), refreshedOrdinary.Priority)
	require.Equal(t, 3, refreshedOrdinary.Weight)
}
