package upstreamaccount

import (
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAttachChannelMinimumRatiosUsesConvertedRatioAndDisabledAccounts(t *testing.T) {
	withTestChannelMinimumRatioDB(t)

	channelA := createMinimumRatioChannel(t, "ratio-a")
	channelB := createMinimumRatioChannel(t, "ratio-b")
	channelC := createMinimumRatioChannel(t, "ratio-c")

	createMinimumRatioAccount(t, channelA.Id, common.ChannelStatusManuallyDisabled, map[string]any{
		"ratio_conversion": 0.35,
		"effective_ratio":  0.8,
		"group_ratio":      0.2,
	})
	createMinimumRatioAccount(t, channelA.Id, common.ChannelStatusEnabled, map[string]any{
		"effective_ratio": 0.6,
	})
	createMinimumRatioAccount(t, channelB.Id, common.ChannelStatusEnabled, map[string]any{
		"model_ratios": map[string]float64{
			"gpt-a": 1.4,
			"gpt-b": 0.7,
		},
	})

	channels := []*model.Channel{channelA, channelB, channelC}
	require.NoError(t, AttachChannelMinimumRatios(channels))

	require.NotNil(t, channelA.MinimumRatio)
	require.InDelta(t, 0.35, *channelA.MinimumRatio, 0.000001)
	require.NotNil(t, channelB.MinimumRatio)
	require.InDelta(t, 0.7, *channelB.MinimumRatio, 0.000001)
	require.Nil(t, channelC.MinimumRatio)
}

func TestSortChannelsByMinimumRatioPutsEmptyLast(t *testing.T) {
	low := &model.Channel{Id: 10, MinimumRatio: minimumRatioPtr(0.5)}
	high := &model.Channel{Id: 20, MinimumRatio: minimumRatioPtr(1.2)}
	sameHighID := &model.Channel{Id: 30, MinimumRatio: minimumRatioPtr(0.5)}
	empty := &model.Channel{Id: 40}
	channels := []*model.Channel{empty, high, low, sameHighID}

	SortChannelsByMinimumRatio(channels, false)
	require.Equal(t, []int{30, 10, 20, 40}, minimumRatioChannelIDs(channels))

	SortChannelsByMinimumRatio(channels, true)
	require.Equal(t, []int{20, 30, 10, 40}, minimumRatioChannelIDs(channels))
}

func withTestChannelMinimumRatioDB(t *testing.T) {
	t.Helper()
	oldDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelAccount{}))
	model.DB = db
	t.Cleanup(func() {
		model.DB = oldDB
	})
}

func createMinimumRatioChannel(t *testing.T, name string) *model.Channel {
	t.Helper()
	channel := &model.Channel{
		Name:   name,
		Key:    "placeholder",
		Status: common.ChannelStatusEnabled,
	}
	require.NoError(t, model.DB.Create(channel).Error)
	return channel
}

func createMinimumRatioAccount(
	t *testing.T,
	channelID int,
	status int,
	metadata map[string]any,
) {
	t.Helper()
	syncMetadata := map[string]any{
		"platform": "new-api",
		"base_url": "https://upstream.example",
	}
	for key, value := range metadata {
		syncMetadata[key] = value
	}
	settingsBytes, err := common.Marshal(map[string]any{
		upstreamAccountSyncMetadataKey: syncMetadata,
	})
	require.NoError(t, err)
	account := &model.ChannelAccount{
		ChannelId:     channelID,
		Name:          "synced-key",
		Key:           "sk-test",
		Status:        status,
		OtherSettings: string(settingsBytes),
	}
	require.NoError(t, model.DB.Create(account).Error)
}

func minimumRatioPtr(value float64) *float64 {
	return &value
}

func minimumRatioChannelIDs(channels []*model.Channel) []int {
	ids := make([]int, 0, len(channels))
	for _, channel := range channels {
		ids = append(ids, channel.Id)
	}
	return ids
}
