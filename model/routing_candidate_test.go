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
	require.EqualValues(t, 0, byKind[RoutingCredentialKindSingleKey][0].Schedule.CredentialPriority)
	require.Equal(t, 0, byKind[RoutingCredentialKindSingleKey][0].Schedule.CredentialWeight)
	require.EqualValues(t, 0, byKind[RoutingCredentialKindMultiKey][0].Schedule.CredentialPriority)
	require.Equal(t, 0, byKind[RoutingCredentialKindMultiKey][0].Schedule.CredentialWeight)
	require.EqualValues(t, 0, byKind[RoutingCredentialKindMultiKey][1].Schedule.CredentialPriority)
	require.Equal(t, 0, byKind[RoutingCredentialKindMultiKey][1].Schedule.CredentialWeight)
	require.EqualValues(t, 0, byKind[RoutingCredentialKindChannelAccount][0].Schedule.ChannelPriority)
	require.Equal(t, 5, byKind[RoutingCredentialKindChannelAccount][0].Schedule.ChannelWeight)
	require.EqualValues(t, 7, byKind[RoutingCredentialKindChannelAccount][0].Schedule.CredentialPriority)
	require.Equal(t, 100, byKind[RoutingCredentialKindChannelAccount][0].Schedule.CredentialWeight)
	require.EqualValues(t, 3, byKind[RoutingCredentialKindPoolAccount][0].Schedule.ChannelPriority)
	require.Equal(t, 1, byKind[RoutingCredentialKindPoolAccount][0].Schedule.ChannelWeight)
	require.EqualValues(t, 4, byKind[RoutingCredentialKindPoolAccount][0].Schedule.CredentialPriority)
	require.Equal(t, 9, byKind[RoutingCredentialKindPoolAccount][0].Schedule.CredentialWeight)
}

func TestRoutingCandidatesUseAbilityScheduleForAccountPool(t *testing.T) {
	db := setupRoutingCandidateTestDB(t, true)
	channelPriority := int64(0)
	channelWeight := uint(0)
	channels := []Channel{
		{
			Id:       22,
			Type:     constant.ChannelTypeOpenAI,
			Status:   common.ChannelStatusEnabled,
			Name:     "fast-account-pool",
			Models:   "gpt-5.5",
			Group:    "default",
			Priority: &channelPriority,
			Weight:   &channelWeight,
			ChannelInfo: ChannelInfo{
				CredentialMode:     constant.ChannelCredentialModeAccountPool,
				AccountPoolEnabled: true,
			},
		},
		{
			Id:       27,
			Type:     constant.ChannelTypeOpenAI,
			Status:   common.ChannelStatusEnabled,
			Name:     "slow-account-pool",
			Models:   "gpt-5.5",
			Group:    "default",
			Priority: &channelPriority,
			Weight:   &channelWeight,
			ChannelInfo: ChannelInfo{
				CredentialMode:     constant.ChannelCredentialModeAccountPool,
				AccountPoolEnabled: true,
			},
		},
	}
	require.NoError(t, db.Create(&channels).Error)
	require.NoError(t, db.Create(&[]ChannelAccount{
		{
			ChannelId: 22,
			Name:      "fast-key",
			Key:       "sk-fast",
			Status:    common.ChannelStatusEnabled,
			Models:    "gpt-5.5",
			Group:     "default",
			Priority:  0,
			Weight:    120,
		},
		{
			ChannelId: 27,
			Name:      "slow-key",
			Key:       "sk-slow",
			Status:    common.ChannelStatusEnabled,
			Models:    "gpt-5.5",
			Group:     "default",
			Priority:  1,
			Weight:    200,
		},
	}).Error)

	priorityOne := int64(1)
	priorityZero := int64(0)
	require.NoError(t, db.Create(&[]Ability{
		{
			Group:     "default",
			Model:     "gpt-5.5",
			ChannelId: 22,
			Enabled:   true,
			Priority:  &priorityOne,
			Weight:    220,
		},
		{
			Group:     "default",
			Model:     "gpt-5.5",
			ChannelId: 27,
			Enabled:   true,
			Priority:  &priorityZero,
			Weight:    50,
		},
	}).Error)

	InitChannelCache()
	candidates, err := GetRoutingCandidatesWithExclusions("default", "gpt-5.5", "", nil, nil)
	require.NoError(t, err)
	require.Len(t, candidates, 2)

	byChannel := map[int]*RoutingCandidate{}
	for _, candidate := range candidates {
		byChannel[candidate.ChannelID] = candidate
	}
	require.EqualValues(t, 1, byChannel[22].Schedule.ChannelPriority)
	require.Equal(t, 220, byChannel[22].Schedule.ChannelWeight)
	require.EqualValues(t, 0, byChannel[22].Schedule.CredentialPriority)
	require.Equal(t, 120, byChannel[22].Schedule.CredentialWeight)
	require.EqualValues(t, 0, byChannel[27].Schedule.ChannelPriority)
	require.Equal(t, 50, byChannel[27].Schedule.ChannelWeight)
	require.EqualValues(t, 1, byChannel[27].Schedule.CredentialPriority)
	require.Equal(t, 200, byChannel[27].Schedule.CredentialWeight)
}

func TestRoutingScheduleComparesLexicographically(t *testing.T) {
	channelPriorityWins := NewRoutingSchedule(3, 0, 0, 0)
	lowerChannelPriority := NewRoutingSchedule(2, 999, 999, 999)
	require.Equal(t, 1, channelPriorityWins.Compare(lowerChannelPriority))

	channelWeightWins := NewRoutingSchedule(3, 10, 0, 0)
	higherCredentialPriority := NewRoutingSchedule(3, 9, 999, 999)
	require.Equal(t, 1, channelWeightWins.Compare(higherCredentialPriority))

	credentialPriorityWins := NewRoutingSchedule(3, 10, 2, 1)
	lowerCredentialWeight := NewRoutingSchedule(3, 10, 2, 0)
	require.Equal(t, 1, credentialPriorityWins.Compare(lowerCredentialWeight))

	require.True(t, channelPriorityWins.SameLayer(NewRoutingSchedule(3, 0, 0, 0)))
}

func modelAccountPoolAuthTypeAPIKeyForTest() string {
	return AccountPoolAuthTypeAPIKey
}
