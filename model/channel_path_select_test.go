package model

import (
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/dto"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func advancedCustomPathTestConfig(path string) *dto.AdvancedCustomConfig {
	return &dto.AdvancedCustomConfig{
		Routes: []dto.AdvancedCustomRoute{
			{
				IncomingPath: path,
				UpstreamPath: "https://upstream.example" + path,
			},
		},
	}
}

func advancedCustomPathTestSettings(t *testing.T, path string) string {
	t.Helper()

	settingsBytes, err := common.Marshal(dto.ChannelOtherSettings{
		AdvancedCustom: advancedCustomPathTestConfig(path),
	})
	require.NoError(t, err)
	return string(settingsBytes)
}

func TestFilterChannelsByRequestPathForAdvancedCustomCache(t *testing.T) {
	oldChannelsIDM := channelsIDM
	oldAdvancedConfigs := channel2advancedCustomConfig
	t.Cleanup(func() {
		channelsIDM = oldChannelsIDM
		channel2advancedCustomConfig = oldAdvancedConfigs
	})

	channelsIDM = map[int]*Channel{
		1: {Id: 1, Type: constant.ChannelTypeAdvancedCustom},
		2: {Id: 2, Type: constant.ChannelTypeAdvancedCustom},
		3: {Id: 3, Type: constant.ChannelTypeOpenAI},
	}
	channel2advancedCustomConfig = map[int]*dto.AdvancedCustomConfig{
		1: advancedCustomPathTestConfig("/v1/chat/completions"),
		2: advancedCustomPathTestConfig("/v1/responses"),
	}

	filtered := filterChannelsByRequestPath([]int{1, 2, 3, 999}, "/v1/responses")

	require.Equal(t, []int{2, 3, 999}, filtered)
}

func TestFilterChannelsByRequestPathSkipsEmptyPath(t *testing.T) {
	oldChannelsIDM := channelsIDM
	oldAdvancedConfigs := channel2advancedCustomConfig
	t.Cleanup(func() {
		channelsIDM = oldChannelsIDM
		channel2advancedCustomConfig = oldAdvancedConfigs
	})

	channelsIDM = map[int]*Channel{
		1: {Id: 1, Type: constant.ChannelTypeAdvancedCustom},
	}
	channel2advancedCustomConfig = map[int]*dto.AdvancedCustomConfig{
		1: advancedCustomPathTestConfig("/v1/chat/completions"),
	}

	filtered := filterChannelsByRequestPath([]int{1}, "")

	require.Equal(t, []int{1}, filtered)
}

func TestGetChannelFiltersAdvancedCustomByRequestPathInDBFallback(t *testing.T) {
	oldDB := DB
	oldLogDB := LOG_DB
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	oldUsingSQLite := common.UsingSQLite
	oldUsingPostgreSQL := common.UsingPostgreSQL
	oldUsingMySQL := common.UsingMySQL
	oldCommonGroupCol := commonGroupCol
	oldCommonKeyCol := commonKeyCol
	oldCommonTrueVal := commonTrueVal
	oldCommonFalseVal := commonFalseVal

	common.MemoryCacheEnabled = false
	common.UsingSQLite = true
	common.UsingPostgreSQL = false
	common.UsingMySQL = false
	initCol()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))
	DB = db
	LOG_DB = db

	t.Cleanup(func() {
		DB = oldDB
		LOG_DB = oldLogDB
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
		common.UsingSQLite = oldUsingSQLite
		common.UsingPostgreSQL = oldUsingPostgreSQL
		common.UsingMySQL = oldUsingMySQL
		commonGroupCol = oldCommonGroupCol
		commonKeyCol = oldCommonKeyCol
		commonTrueVal = oldCommonTrueVal
		commonFalseVal = oldCommonFalseVal
	})

	priority := int64(0)
	require.NoError(t, db.Create(&Channel{
		Id:            1,
		Type:          constant.ChannelTypeAdvancedCustom,
		Status:        common.ChannelStatusEnabled,
		Name:          "advanced-chat",
		Models:        "gpt-path",
		Group:         "default",
		Priority:      &priority,
		OtherSettings: advancedCustomPathTestSettings(t, "/v1/chat/completions"),
	}).Error)
	require.NoError(t, db.Create(&Channel{
		Id:            2,
		Type:          constant.ChannelTypeAdvancedCustom,
		Status:        common.ChannelStatusEnabled,
		Name:          "advanced-responses",
		Models:        "gpt-path",
		Group:         "default",
		Priority:      &priority,
		OtherSettings: advancedCustomPathTestSettings(t, "/v1/responses"),
	}).Error)
	require.NoError(t, db.Create(&Ability{
		Group:     "default",
		Model:     "gpt-path",
		ChannelId: 1,
		Enabled:   true,
		Priority:  &priority,
	}).Error)
	require.NoError(t, db.Create(&Ability{
		Group:     "default",
		Model:     "gpt-path",
		ChannelId: 2,
		Enabled:   true,
		Priority:  &priority,
	}).Error)

	channel, err := GetChannel("default", "gpt-path", 0, "/v1/responses")

	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 2, channel.Id)
}
