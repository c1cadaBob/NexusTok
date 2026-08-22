package model

import (
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func routingCandidateTestWeight(weight uint) *uint {
	return &weight
}

func routingCandidateTestPriority(priority int64) *int64 {
	return &priority
}

func setupRoutingCandidateTestDB(t *testing.T, memoryCache bool) *gorm.DB {
	t.Helper()
	oldDB := DB
	oldLogDB := LOG_DB
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	oldGroup2Model2Channels := group2model2channels
	oldChannelsIDM := channelsIDM
	oldAdvancedConfigs := channel2advancedCustomConfig
	oldAbilitySchedules := channelAbilitySchedules
	oldRoutingCandidateCache := routingCandidateCache

	common.MemoryCacheEnabled = memoryCache
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}, &ChannelAccount{}, &AccountPoolGroup{}, &PoolAccount{}))
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
		channelAbilitySchedules = oldAbilitySchedules
		routingCandidateCache = oldRoutingCandidateCache
	})
	return db
}

func TestRoutingCandidatesGenerateCredentialKindsAndSchedules(t *testing.T) {
	db := setupRoutingCandidateTestDB(t, true)
	channels := []Channel{
		{
			Id:       1,
			Type:     constant.ChannelTypeOpenAI,
			Key:      "sk-single",
			Status:   common.ChannelStatusEnabled,
			Name:     "single",
			Models:   "gpt-candidate",
			Group:    "default",
			Priority: routingCandidateTestPriority(2),
			Weight:   routingCandidateTestWeight(10),
		},
		{
			Id:       2,
			Type:     constant.ChannelTypeOpenAI,
			Key:      "sk-multi-a\nsk-multi-b",
			Status:   common.ChannelStatusEnabled,
			Name:     "multi",
			Models:   "gpt-candidate",
			Group:    "default",
			Priority: routingCandidateTestPriority(1),
			Weight:   routingCandidateTestWeight(20),
			ChannelInfo: ChannelInfo{
				CredentialMode: constant.ChannelCredentialModeMultiKey,
				IsMultiKey:     true,
				MultiKeyMode:   constant.MultiKeyModePolling,
			},
		},
		{
			Id:       3,
			Type:     constant.ChannelTypeOpenAI,
			Status:   common.ChannelStatusEnabled,
			Name:     "channel-account",
			Models:   "gpt-candidate",
			Group:    "default",
			Priority: routingCandidateTestPriority(0),
			Weight:   routingCandidateTestWeight(5),
			ChannelInfo: ChannelInfo{
				CredentialMode:     constant.ChannelCredentialModeAccountPool,
				AccountPoolEnabled: true,
			},
		},
		{
			Id:       4,
			Type:     constant.ChannelTypeOpenAI,
			Status:   common.ChannelStatusEnabled,
			Name:     "pool-account",
			Models:   "gpt-candidate",
			Group:    "default",
			Priority: routingCandidateTestPriority(3),
			Weight:   routingCandidateTestWeight(1),
			ChannelInfo: ChannelInfo{
				CredentialMode:     constant.ChannelCredentialModeGlobalAccountPool,
				AccountPoolEnabled: true,
				AccountPoolGroupId: 1,
			},
		},
	}
	require.NoError(t, db.Create(&channels).Error)
	require.NoError(t, db.Create(&ChannelAccount{
		ChannelId: 3,
		Name:      "account-key",
		Key:       "sk-account",
		Status:    common.ChannelStatusEnabled,
		Models:    "gpt-candidate",
		Group:     "default",
		Priority:  7,
		Weight:    100,
	}).Error)
	require.NoError(t, db.Create(&AccountPoolGroup{
		Id:       1,
		Name:     "pool-group",
		Platform: "openai",
		AuthType: modelAccountPoolAuthTypeAPIKeyForTest(),
		Source:   AccountPoolGroupSourceNative,
		Status:   common.ChannelStatusEnabled,
		Strategy: AccountPoolStrategyWeighted,
		Models:   "gpt-candidate",
		Group:    "default",
	}).Error)
	require.NoError(t, db.Create(&PoolAccount{
		Id:          1,
		PoolGroupId: 1,
		Name:        "pool-key",
		Platform:    "openai",
		AuthType:    AccountPoolAuthTypeAPIKey,
		Credentials: "encrypted-placeholder",
		Status:      common.ChannelStatusEnabled,
		Schedulable: true,
		Models:      "gpt-candidate",
		Group:       "default",
		Priority:    4,
		Weight:      9,
	}).Error)

	InitChannelCache()
	candidates, err := GetRoutingCandidatesWithExclusions("default", "gpt-candidate", "", nil, nil)
	require.NoError(t, err)
	require.Len(t, candidates, 5)

	byKind := map[RoutingCredentialKind][]*RoutingCandidate{}
	for _, candidate := range candidates {
		byKind[candidate.Kind] = append(byKind[candidate.Kind], candidate)
	}
	require.Len(t, byKind[RoutingCredentialKindSingleKey], 1)
	require.Len(t, byKind[RoutingCredentialKindMultiKey], 2)
	require.Len(t, byKind[RoutingCredentialKindChannelAccount], 1)
	require.Len(t, byKind[RoutingCredentialKindPoolAccount], 1)
	require.EqualValues(t, 7, byKind[RoutingCredentialKindChannelAccount][0].Schedule.EffectivePriority)
	require.Equal(t, 105, byKind[RoutingCredentialKindChannelAccount][0].Schedule.EffectiveWeight)
	require.EqualValues(t, 7, byKind[RoutingCredentialKindPoolAccount][0].Schedule.EffectivePriority)
	require.Equal(t, 10, byKind[RoutingCredentialKindPoolAccount][0].Schedule.EffectiveWeight)
}

func TestRoutingScheduleUsesAdditiveFormulaAndZeroWeight(t *testing.T) {
	schedule := NewRoutingSchedule(3, 10, 2, 195)
	require.EqualValues(t, 5, schedule.EffectivePriority)
	require.Equal(t, 205, schedule.EffectiveWeight)

	zero := NewRoutingSchedule(0, 0, 0, 0)
	require.EqualValues(t, 0, zero.EffectivePriority)
	require.Equal(t, 0, zero.EffectiveWeight)
}

func modelAccountPoolAuthTypeAPIKeyForTest() string {
	return AccountPoolAuthTypeAPIKey
}
