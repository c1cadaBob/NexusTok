package upstreamaccount

import (
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAttachChannelMinimumRatiosIgnoresDisabledAccounts(t *testing.T) {
	withTestChannelMinimumRatioDB(t)

	channelA := createMinimumRatioChannel(t, "ratio-a")
	channelB := createMinimumRatioChannel(t, "ratio-b")
	channelC := createMinimumRatioChannel(t, "ratio-c")
	channelD := createMinimumRatioChannel(t, "ratio-disabled-only")

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
	createMinimumRatioAccount(t, channelD.Id, common.ChannelStatusAutoDisabled, map[string]any{
		"ratio_conversion": 0.1,
	})

	channels := []*model.Channel{channelA, channelB, channelC, channelD}
	require.NoError(t, AttachChannelMinimumRatios(channels))

	require.NotNil(t, channelA.MinimumRatio)
	require.InDelta(t, 0.6, *channelA.MinimumRatio, 0.000001)
	require.NotNil(t, channelB.MinimumRatio)
	require.InDelta(t, 0.7, *channelB.MinimumRatio, 0.000001)
	require.Nil(t, channelC.MinimumRatio)
	require.Nil(t, channelD.MinimumRatio)
}

func TestAttachChannelMinimumRatiosForModelOnlyUsesMatchingAccounts(t *testing.T) {
	withTestChannelMinimumRatioDB(t)

	channelA := createMinimumRatioChannel(t, "ratio-a")
	channelB := createMinimumRatioChannel(t, "ratio-b")
	channelC := createMinimumRatioChannel(t, "ratio-c")

	createMinimumRatioAccountWithModels(t, channelA.Id, common.ChannelStatusEnabled, "gpt-4o,gpt-5", map[string]any{
		"ratio_conversion": 0.6,
	})
	createMinimumRatioAccountWithModels(t, channelA.Id, common.ChannelStatusEnabled, "claude-*", map[string]any{
		"ratio_conversion": 0.2,
	})
	createMinimumRatioAccountWithModels(t, channelB.Id, common.ChannelStatusEnabled, "gpt-*", map[string]any{
		"ratio_conversion": 0.35,
	})
	createMinimumRatioAccountWithModels(t, channelC.Id, common.ChannelStatusEnabled, "gpt-4.1", map[string]any{
		"ratio_conversion": 0.1,
	})
	createMinimumRatioAccountWithModels(t, channelC.Id, common.ChannelStatusManuallyDisabled, "gpt-5", map[string]any{
		"ratio_conversion": 0.05,
	})

	channels := []*model.Channel{channelA, channelB, channelC}
	require.NoError(t, AttachChannelMinimumRatiosForModel(channels, "gpt-5"))

	require.NotNil(t, channelA.MinimumRatio)
	require.InDelta(t, 0.6, *channelA.MinimumRatio, 0.000001)
	require.NotNil(t, channelB.MinimumRatio)
	require.InDelta(t, 0.35, *channelB.MinimumRatio, 0.000001)
	require.Nil(t, channelC.MinimumRatio)
}

func TestCollectChannelMinimumRatioModelsOnlyUsesSyncedAccounts(t *testing.T) {
	withTestChannelMinimumRatioDB(t)

	channel := createMinimumRatioChannel(t, "ratio-models")
	createMinimumRatioAccountWithModels(t, channel.Id, common.ChannelStatusEnabled, "gpt-5,Claude-3,gpt-5", map[string]any{
		"ratio_conversion": 0.6,
	})
	require.NoError(t, model.DB.Create(&model.ChannelAccount{
		ChannelId: channel.Id,
		Name:      "plain-account",
		Key:       "sk-plain",
		Status:    common.ChannelStatusEnabled,
		Models:    "plain-model",
	}).Error)
	createMinimumRatioAccountWithModels(t, channel.Id, common.ChannelStatusAutoDisabled, "disabled-model", map[string]any{
		"ratio_conversion": 0.1,
	})

	models, err := CollectChannelMinimumRatioModels([]*model.Channel{channel})
	require.NoError(t, err)
	require.Equal(t, []string{"Claude-3", "gpt-5"}, models)
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
	createMinimumRatioAccountWithModels(t, channelID, status, "", metadata)
}

func createMinimumRatioAccountWithModels(
	t *testing.T,
	channelID int,
	status int,
	models string,
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
		Models:        models,
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
