package model

import (
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupMultiKeyStatusTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	oldDB := DB
	oldLogDB := LOG_DB
	oldMemoryCacheEnabled := common.MemoryCacheEnabled

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))
	DB = db
	LOG_DB = db
	common.MemoryCacheEnabled = false

	t.Cleanup(func() {
		DB = oldDB
		LOG_DB = oldLogDB
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
	})

	return db
}

func createMultiKeyStatusChannel(t *testing.T, db *gorm.DB, channelID int) *Channel {
	t.Helper()

	priority := int64(0)
	channel := &Channel{
		Id:       channelID,
		Type:     constant.ChannelTypeOpenAI,
		Status:   common.ChannelStatusEnabled,
		Name:     "multi-key-status",
		Key:      "sk-a\nsk-b",
		Models:   "gpt-4o",
		Group:    "default",
		Priority: &priority,
		ChannelInfo: ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 2,
			MultiKeyMode: constant.MultiKeyModePolling,
		},
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&Ability{
		Group:     "default",
		Model:     "gpt-4o",
		ChannelId: channel.Id,
		Enabled:   true,
		Priority:  &priority,
		Weight:    100,
	}).Error)
	return channel
}

func TestUpdateChannelStatusMultiKeyUnknownKeyDoesNotDisableFirstKey(t *testing.T) {
	db := setupMultiKeyStatusTestDB(t)
	channel := createMultiKeyStatusChannel(t, db, 101)

	changed := UpdateChannelStatus(channel.Id, "sk-missing", common.ChannelStatusAutoDisabled, "upstream rejected unknown key")

	require.True(t, changed)
	var stored Channel
	require.NoError(t, db.First(&stored, channel.Id).Error)
	require.Equal(t, common.ChannelStatusAutoDisabled, stored.Status)
	require.Empty(t, stored.ChannelInfo.MultiKeyStatusList)
	require.Empty(t, stored.ChannelInfo.MultiKeyDisabledReason)

	var ability Ability
	require.NoError(t, db.Where("channel_id = ?", channel.Id).First(&ability).Error)
	require.False(t, ability.Enabled)
}

func TestUpdateChannelStatusMultiKeyAllKeysDisabledRemovesCacheCandidate(t *testing.T) {
	db := setupMultiKeyStatusTestDB(t)
	channel := createMultiKeyStatusChannel(t, db, 102)

	setupRetryExclusionChannelCache(t, map[string]map[string][]int{
		"default": {
			"gpt-4o": {channel.Id},
		},
	}, map[int]*Channel{
		channel.Id: {
			Id:       channel.Id,
			Type:     channel.Type,
			Status:   common.ChannelStatusEnabled,
			Name:     channel.Name,
			Key:      channel.Key,
			Models:   channel.Models,
			Group:    channel.Group,
			Priority: channel.Priority,
			ChannelInfo: ChannelInfo{
				IsMultiKey:   true,
				MultiKeySize: 2,
				MultiKeyMode: constant.MultiKeyModePolling,
			},
		},
	})

	require.True(t, UpdateChannelStatus(channel.Id, "sk-a", common.ChannelStatusAutoDisabled, "first key failed"))

	var afterFirst Channel
	require.NoError(t, db.First(&afterFirst, channel.Id).Error)
	require.Equal(t, common.ChannelStatusEnabled, afterFirst.Status)
	require.Equal(t, common.ChannelStatusAutoDisabled, afterFirst.ChannelInfo.MultiKeyStatusList[0])
	require.True(t, channelIDInCache("default", "gpt-4o", channel.Id))

	require.True(t, UpdateChannelStatus(channel.Id, "sk-b", common.ChannelStatusAutoDisabled, "second key failed"))

	var afterSecond Channel
	require.NoError(t, db.First(&afterSecond, channel.Id).Error)
	require.Equal(t, common.ChannelStatusAutoDisabled, afterSecond.Status)
	require.Equal(t, common.ChannelStatusAutoDisabled, afterSecond.ChannelInfo.MultiKeyStatusList[0])
	require.Equal(t, common.ChannelStatusAutoDisabled, afterSecond.ChannelInfo.MultiKeyStatusList[1])

	var ability Ability
	require.NoError(t, db.Where("channel_id = ?", channel.Id).First(&ability).Error)
	require.False(t, ability.Enabled)
	require.False(t, channelIDInCache("default", "gpt-4o", channel.Id))
}

func TestUpdateChannelStatusMultiKeyChannelLevelEnableClearsKeyDisabledState(t *testing.T) {
	db := setupMultiKeyStatusTestDB(t)
	channel := createMultiKeyStatusChannel(t, db, 103)
	require.True(t, UpdateChannelStatus(channel.Id, "sk-a", common.ChannelStatusAutoDisabled, "first key failed"))
	require.True(t, UpdateChannelStatus(channel.Id, "sk-b", common.ChannelStatusAutoDisabled, "second key failed"))

	require.True(t, UpdateChannelStatus(channel.Id, "", common.ChannelStatusEnabled, "manual recovery"))

	var stored Channel
	require.NoError(t, db.First(&stored, channel.Id).Error)
	require.Equal(t, common.ChannelStatusEnabled, stored.Status)
	require.Empty(t, stored.ChannelInfo.MultiKeyStatusList)
	require.Empty(t, stored.ChannelInfo.MultiKeyDisabledReason)
	require.Empty(t, stored.ChannelInfo.MultiKeyDisabledTime)

	var ability Ability
	require.NoError(t, db.Where("channel_id = ?", channel.Id).First(&ability).Error)
	require.True(t, ability.Enabled)
}

func channelIDInCache(group string, model string, channelID int) bool {
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	for _, cachedID := range group2model2channels[group][model] {
		if cachedID == channelID {
			return true
		}
	}
	return false
}
