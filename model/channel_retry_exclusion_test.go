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

func retryExclusionTestWeight(weight uint) *uint {
	return &weight
}

func setupRetryExclusionChannelCache(t *testing.T, groupModelChannels map[string]map[string][]int, channels map[int]*Channel) {
	t.Helper()

	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true

	channelSyncLock.Lock()
	oldGroup2Model2Channels := group2model2channels
	oldChannelsIDM := channelsIDM
	oldAdvancedConfigs := channel2advancedCustomConfig
	group2model2channels = groupModelChannels
	channelsIDM = channels
	channel2advancedCustomConfig = map[int]*dto.AdvancedCustomConfig{}
	channelSyncLock.Unlock()

	t.Cleanup(func() {
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
		channelSyncLock.Lock()
		defer channelSyncLock.Unlock()
		group2model2channels = oldGroup2Model2Channels
		channelsIDM = oldChannelsIDM
		channel2advancedCustomConfig = oldAdvancedConfigs
	})
}

func TestGetRandomSatisfiedChannelWithExclusionsKeepsSamePriority(t *testing.T) {
	highPriority := int64(10)
	lowPriority := int64(1)
	setupRetryExclusionChannelCache(t, map[string]map[string][]int{
		"default": {
			"gpt-retry": {1, 2, 3},
		},
	}, map[int]*Channel{
		1: {Id: 1, Status: common.ChannelStatusEnabled, Type: constant.ChannelTypeOpenAI, Priority: &highPriority, Weight: retryExclusionTestWeight(100)},
		2: {Id: 2, Status: common.ChannelStatusEnabled, Type: constant.ChannelTypeOpenAI, Priority: &highPriority, Weight: retryExclusionTestWeight(100)},
		3: {Id: 3, Status: common.ChannelStatusEnabled, Type: constant.ChannelTypeOpenAI, Priority: &lowPriority, Weight: retryExclusionTestWeight(100)},
	})

	channel, err := GetRandomSatisfiedChannelWithExclusions("default", "gpt-retry", 1, "", []int{1})

	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 2, channel.Id)
}

func TestGetRandomSatisfiedChannelWithExclusionsFallsBackToLowerPriority(t *testing.T) {
	highPriority := int64(10)
	lowPriority := int64(1)
	setupRetryExclusionChannelCache(t, map[string]map[string][]int{
		"default": {
			"gpt-retry": {1, 2},
		},
	}, map[int]*Channel{
		1: {Id: 1, Status: common.ChannelStatusEnabled, Type: constant.ChannelTypeOpenAI, Priority: &highPriority, Weight: retryExclusionTestWeight(100)},
		2: {Id: 2, Status: common.ChannelStatusEnabled, Type: constant.ChannelTypeOpenAI, Priority: &lowPriority, Weight: retryExclusionTestWeight(100)},
	})

	channel, err := GetRandomSatisfiedChannelWithExclusions("default", "gpt-retry", 1, "", []int{1})

	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 2, channel.Id)
}

func TestGetRandomSatisfiedChannelSkipsDisabledCacheResidue(t *testing.T) {
	priority := int64(0)
	setupRetryExclusionChannelCache(t, map[string]map[string][]int{
		"default": {
			"gpt-retry": {1, 2},
		},
	}, map[int]*Channel{
		1: {Id: 1, Status: common.ChannelStatusAutoDisabled, Type: constant.ChannelTypeOpenAI, Priority: &priority, Weight: retryExclusionTestWeight(100)},
		2: {Id: 2, Status: common.ChannelStatusEnabled, Type: constant.ChannelTypeOpenAI, Priority: &priority, Weight: retryExclusionTestWeight(100)},
	})

	channel, err := GetRandomSatisfiedChannel("default", "gpt-retry", 0, "")

	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 2, channel.Id)
}

func TestCacheUpdateChannelStatusRemovesDuplicateChannelIds(t *testing.T) {
	priority := int64(0)
	setupRetryExclusionChannelCache(t, map[string]map[string][]int{
		"default": {
			"gpt-retry": {1, 2, 1, 1, 3},
		},
	}, map[int]*Channel{
		1: {Id: 1, Status: common.ChannelStatusEnabled, Type: constant.ChannelTypeOpenAI, Priority: &priority},
	})

	CacheUpdateChannelStatus(1, common.ChannelStatusAutoDisabled)

	require.Equal(t, []int{2, 3}, group2model2channels["default"]["gpt-retry"])
	require.Equal(t, common.ChannelStatusAutoDisabled, channelsIDM[1].Status)
}

func TestInitChannelCacheCreatesMissingGroupMapFromChannel(t *testing.T) {
	oldDB := DB
	oldLogDB := LOG_DB
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	oldGroup2Model2Channels := group2model2channels
	oldChannelsIDM := channelsIDM
	oldAdvancedConfigs := channel2advancedCustomConfig

	common.MemoryCacheEnabled = true

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))
	DB = db
	LOG_DB = db

	t.Cleanup(func() {
		DB = oldDB
		LOG_DB = oldLogDB
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
		channelSyncLock.Lock()
		defer channelSyncLock.Unlock()
		group2model2channels = oldGroup2Model2Channels
		channelsIDM = oldChannelsIDM
		channel2advancedCustomConfig = oldAdvancedConfigs
	})

	priority := int64(0)
	require.NoError(t, db.Create(&Channel{
		Id:       1,
		Type:     constant.ChannelTypeOpenAI,
		Status:   common.ChannelStatusEnabled,
		Name:     "synced-account-channel",
		Models:   " gpt-retry, ",
		Group:    " synced-group, ",
		Priority: &priority,
	}).Error)

	require.NotPanics(t, InitChannelCache)
	require.Equal(t, []int{1}, group2model2channels["synced-group"]["gpt-retry"])
	require.Contains(t, channelsIDM, 1)
}

func TestSyncChannelAccountPoolCapabilitiesUsesAccountPairs(t *testing.T) {
	oldDB := DB
	oldLogDB := LOG_DB
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}, &ChannelAccount{}))
	DB = db
	LOG_DB = db

	t.Cleanup(func() {
		DB = oldDB
		LOG_DB = oldLogDB
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
	})

	priority := int64(0)
	channel := Channel{
		Type:     constant.ChannelTypeOpenAI,
		Status:   common.ChannelStatusEnabled,
		Name:     "account-pair-channel",
		Models:   "gpt-a,gpt-b",
		Group:    "default,vip",
		Priority: &priority,
		ChannelInfo: ChannelInfo{
			CredentialMode:     constant.ChannelCredentialModeAccountPool,
			AccountPoolEnabled: true,
			AccountPoolMode:    constant.ChannelAccountPoolModePolling,
		},
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&[]ChannelAccount{
		{
			ChannelId: channel.Id,
			Name:      "default-key",
			Key:       "sk-default",
			Status:    common.ChannelStatusEnabled,
			Models:    "gpt-a",
			Group:     "default",
		},
		{
			ChannelId: channel.Id,
			Name:      "vip-key",
			Key:       "sk-vip",
			Status:    common.ChannelStatusEnabled,
			Models:    "gpt-b",
			Group:     "vip",
		},
		{
			ChannelId: channel.Id,
			Name:      "disabled-key",
			Key:       "sk-disabled",
			Status:    common.ChannelStatusManuallyDisabled,
			Models:    "gpt-disabled",
			Group:     "disabled",
		},
	}).Error)

	require.NoError(t, SyncChannelAccountPoolCapabilities(channel.Id, nil))

	var abilities []Ability
	require.NoError(t, db.Order(commonGroupCol).Order("model").Find(&abilities).Error)
	require.Len(t, abilities, 2)
	require.Equal(t, "default", abilities[0].Group)
	require.Equal(t, "gpt-a", abilities[0].Model)
	require.Equal(t, "vip", abilities[1].Group)
	require.Equal(t, "gpt-b", abilities[1].Model)

	var refreshed Channel
	require.NoError(t, db.First(&refreshed, channel.Id).Error)
	require.Equal(t, "gpt-a,gpt-b", refreshed.Models)
	require.Equal(t, "default,vip", refreshed.Group)
}

func TestSyncChannelAccountPoolCapabilitiesUsesAccessGroupsForUpstreamSync(t *testing.T) {
	oldDB := DB
	oldLogDB := LOG_DB
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}, &ChannelAccount{}))
	DB = db
	LOG_DB = db

	t.Cleanup(func() {
		DB = oldDB
		LOG_DB = oldLogDB
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
	})

	priority := int64(0)
	channel := Channel{
		Type:          constant.ChannelTypeOpenAI,
		Status:        common.ChannelStatusEnabled,
		Name:          "upstream-sync-channel",
		Models:        "gpt-a,gpt-b",
		Group:         "default,vip",
		Priority:      &priority,
		OtherSettings: `{"upstream_account_sync":{"platform":"new-api","base_url":"https://upstream.example","synced_at":1}}`,
		ChannelInfo: ChannelInfo{
			CredentialMode:     constant.ChannelCredentialModeAccountPool,
			AccountPoolEnabled: true,
			AccountPoolMode:    constant.ChannelAccountPoolModePolling,
		},
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&[]ChannelAccount{
		{
			ChannelId:    channel.Id,
			Name:         "upstream-default-key",
			Key:          "sk-default",
			Status:       common.ChannelStatusEnabled,
			Models:       "gpt-a",
			Group:        "upstream-default",
			AccessGroups: "default",
		},
		{
			ChannelId:    channel.Id,
			Name:         "upstream-vip-key",
			Key:          "sk-vip",
			Status:       common.ChannelStatusEnabled,
			Models:       "gpt-b",
			Group:        "upstream-vip",
			AccessGroups: "vip",
		},
	}).Error)

	require.NoError(t, SyncChannelAccountPoolCapabilities(channel.Id, nil))

	var abilities []Ability
	require.NoError(t, db.Order(commonGroupCol).Order("model").Find(&abilities).Error)
	require.Len(t, abilities, 2)
	abilityPairs := make([]string, 0, len(abilities))
	for _, ability := range abilities {
		abilityPairs = append(abilityPairs, ability.Group+":"+ability.Model)
	}
	require.Equal(t, []string{
		"default:gpt-a",
		"vip:gpt-b",
	}, abilityPairs)

	var refreshed Channel
	require.NoError(t, db.First(&refreshed, channel.Id).Error)
	require.Equal(t, "gpt-a,gpt-b", refreshed.Models)
	require.Equal(t, "default,vip", refreshed.Group)
}

func TestGetChannelWithExclusionsFiltersChannelStatusInDBFallback(t *testing.T) {
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

	highPriority := int64(10)
	lowPriority := int64(1)
	require.NoError(t, db.Create(&Channel{
		Id:       1,
		Type:     constant.ChannelTypeOpenAI,
		Status:   common.ChannelStatusAutoDisabled,
		Name:     "disabled-high-priority",
		Models:   "gpt-retry",
		Group:    "default",
		Priority: &highPriority,
	}).Error)
	require.NoError(t, db.Create(&Channel{
		Id:       2,
		Type:     constant.ChannelTypeOpenAI,
		Status:   common.ChannelStatusEnabled,
		Name:     "enabled-low-priority",
		Models:   "gpt-retry",
		Group:    "default",
		Priority: &lowPriority,
	}).Error)
	require.NoError(t, db.Create(&Ability{
		Group:     "default",
		Model:     "gpt-retry",
		ChannelId: 1,
		Enabled:   true,
		Priority:  &highPriority,
		Weight:    100,
	}).Error)
	require.NoError(t, db.Create(&Ability{
		Group:     "default",
		Model:     "gpt-retry",
		ChannelId: 2,
		Enabled:   true,
		Priority:  &lowPriority,
		Weight:    100,
	}).Error)

	channel, err := GetChannel("default", "gpt-retry", 0, "")

	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 2, channel.Id)
}

func TestGetChannelWithExclusionsKeepsSamePriorityInDBFallback(t *testing.T) {
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

	highPriority := int64(10)
	lowPriority := int64(1)
	for _, channel := range []*Channel{
		{Id: 1, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Name: "failed-high", Models: "gpt-retry", Group: "default", Priority: &highPriority},
		{Id: 2, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Name: "healthy-high", Models: "gpt-retry", Group: "default", Priority: &highPriority},
		{Id: 3, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Name: "healthy-low", Models: "gpt-retry", Group: "default", Priority: &lowPriority},
	} {
		require.NoError(t, db.Create(channel).Error)
	}
	for _, ability := range []*Ability{
		{Group: "default", Model: "gpt-retry", ChannelId: 1, Enabled: true, Priority: &highPriority, Weight: 100},
		{Group: "default", Model: "gpt-retry", ChannelId: 2, Enabled: true, Priority: &highPriority, Weight: 100},
		{Group: "default", Model: "gpt-retry", ChannelId: 3, Enabled: true, Priority: &lowPriority, Weight: 100},
	} {
		require.NoError(t, db.Create(ability).Error)
	}

	channel, err := GetChannelWithExclusions("default", "gpt-retry", 1, "", []int{1})

	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 2, channel.Id)
}
