package service

import (
	"net/http/httptest"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func routingSelectTestWeight(weight uint) *uint {
	return &weight
}

func routingSelectTestPriority(priority int64) *int64 {
	return &priority
}

func newRoutingSelectTestContext() *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	return c
}

func TestSelectRoutingCandidateRespectsFourLevelOrder(t *testing.T) {
	t.Run("channel priority before credential priority", func(t *testing.T) {
		db := setupChannelAccountSelectTestDB(t)
		highPriority := model.Channel{
			Type:     constant.ChannelTypeOpenAI,
			Key:      "sk-high",
			Status:   common.ChannelStatusEnabled,
			Name:     "high-priority-channel",
			Models:   "gpt-route",
			Group:    "default",
			Priority: routingSelectTestPriority(10),
			Weight:   routingSelectTestWeight(0),
		}
		lowPriority := model.Channel{
			Type:     constant.ChannelTypeOpenAI,
			Status:   common.ChannelStatusEnabled,
			Name:     "low-priority-high-credential",
			Models:   "gpt-route",
			Group:    "default",
			Priority: routingSelectTestPriority(0),
			Weight:   routingSelectTestWeight(1000),
			ChannelInfo: model.ChannelInfo{
				CredentialMode:     constant.ChannelCredentialModeAccountPool,
				AccountPoolEnabled: true,
			},
		}
		require.NoError(t, db.Create(&highPriority).Error)
		require.NoError(t, db.Create(&lowPriority).Error)
		require.NoError(t, db.Create(&model.ChannelAccount{
			ChannelId: lowPriority.Id,
			Name:      "low-priority-key",
			Key:       "sk-low",
			Status:    common.ChannelStatusEnabled,
			Models:    "gpt-route",
			Group:     "default",
			Priority:  100,
			Weight:    100,
		}).Error)

		candidate, selectedGroup, err := SelectRoutingCandidate(&RetryParam{
			Ctx:        newRoutingSelectTestContext(),
			TokenGroup: "default",
			ModelName:  "gpt-route",
			Retry:      common.GetPointer(0),
		})

		require.NoError(t, err)
		require.Equal(t, "default", selectedGroup)
		require.NotNil(t, candidate)
		require.Equal(t, model.RoutingCredentialKindSingleKey, candidate.Kind)
		require.Equal(t, highPriority.Id, candidate.ChannelID)
		require.EqualValues(t, 10, candidate.Schedule.ChannelPriority)
		require.Equal(t, 0, candidate.Schedule.ChannelWeight)
		require.EqualValues(t, 0, candidate.Schedule.CredentialPriority)
		require.Equal(t, 0, candidate.Schedule.CredentialWeight)
	})

	t.Run("channel weight before credential priority", func(t *testing.T) {
		db := setupChannelAccountSelectTestDB(t)
		lowWeight := model.Channel{
			Type:     constant.ChannelTypeOpenAI,
			Key:      "sk-low-weight",
			Status:   common.ChannelStatusEnabled,
			Name:     "low-weight-channel",
			Models:   "gpt-route",
			Group:    "default",
			Priority: routingSelectTestPriority(5),
			Weight:   routingSelectTestWeight(10),
		}
		highWeight := model.Channel{
			Type:     constant.ChannelTypeOpenAI,
			Status:   common.ChannelStatusEnabled,
			Name:     "high-weight-high-credential",
			Models:   "gpt-route",
			Group:    "default",
			Priority: routingSelectTestPriority(5),
			Weight:   routingSelectTestWeight(20),
			ChannelInfo: model.ChannelInfo{
				CredentialMode:     constant.ChannelCredentialModeAccountPool,
				AccountPoolEnabled: true,
			},
		}
		require.NoError(t, db.Create(&lowWeight).Error)
		require.NoError(t, db.Create(&highWeight).Error)
		require.NoError(t, db.Create(&model.ChannelAccount{
			ChannelId: highWeight.Id,
			Name:      "high-weight-key",
			Key:       "sk-high",
			Status:    common.ChannelStatusEnabled,
			Models:    "gpt-route",
			Group:     "default",
			Priority:  100,
			Weight:    1,
		}).Error)

		candidate, selectedGroup, err := SelectRoutingCandidate(&RetryParam{
			Ctx:        newRoutingSelectTestContext(),
			TokenGroup: "default",
			ModelName:  "gpt-route",
			Retry:      common.GetPointer(0),
		})

		require.NoError(t, err)
		require.Equal(t, "default", selectedGroup)
		require.NotNil(t, candidate)
		require.Equal(t, model.RoutingCredentialKindChannelAccount, candidate.Kind)
		require.Equal(t, highWeight.Id, candidate.ChannelID)
		require.EqualValues(t, 5, candidate.Schedule.ChannelPriority)
		require.Equal(t, 20, candidate.Schedule.ChannelWeight)
		require.EqualValues(t, 100, candidate.Schedule.CredentialPriority)
		require.Equal(t, 1, candidate.Schedule.CredentialWeight)
	})

	t.Run("credential priority before credential weight", func(t *testing.T) {
		db := setupChannelAccountSelectTestDB(t)
		channel := model.Channel{
			Type:     constant.ChannelTypeOpenAI,
			Status:   common.ChannelStatusEnabled,
			Name:     "credential-priority-channel",
			Models:   "gpt-route",
			Group:    "default",
			Priority: routingSelectTestPriority(3),
			Weight:   routingSelectTestWeight(30),
			ChannelInfo: model.ChannelInfo{
				CredentialMode:     constant.ChannelCredentialModeAccountPool,
				AccountPoolEnabled: true,
			},
		}
		require.NoError(t, db.Create(&channel).Error)
		require.NoError(t, db.Create(&[]model.ChannelAccount{
			{
				ChannelId: channel.Id,
				Name:      "higher-priority",
				Key:       "sk-higher-priority",
				Status:    common.ChannelStatusEnabled,
				Models:    "gpt-route",
				Group:     "default",
				Priority:  2,
				Weight:    1,
			},
			{
				ChannelId: channel.Id,
				Name:      "lower-priority-higher-weight",
				Key:       "sk-lower-priority",
				Status:    common.ChannelStatusEnabled,
				Models:    "gpt-route",
				Group:     "default",
				Priority:  1,
				Weight:    100,
			},
		}).Error)

		candidate, selectedGroup, err := SelectRoutingCandidate(&RetryParam{
			Ctx:        newRoutingSelectTestContext(),
			TokenGroup: "default",
			ModelName:  "gpt-route",
			Retry:      common.GetPointer(0),
		})

		require.NoError(t, err)
		require.Equal(t, "default", selectedGroup)
		require.NotNil(t, candidate)
		require.Equal(t, model.RoutingCredentialKindChannelAccount, candidate.Kind)
		require.Equal(t, channel.Id, candidate.ChannelID)
		require.EqualValues(t, 2, candidate.Schedule.CredentialPriority)
		require.Equal(t, 1, candidate.Schedule.CredentialWeight)
	})

	t.Run("credential weight after前三层相同", func(t *testing.T) {
		db := setupChannelAccountSelectTestDB(t)
		channel := model.Channel{
			Type:     constant.ChannelTypeOpenAI,
			Status:   common.ChannelStatusEnabled,
			Name:     "credential-weight-channel",
			Models:   "gpt-route",
			Group:    "default",
			Priority: routingSelectTestPriority(3),
			Weight:   routingSelectTestWeight(30),
			ChannelInfo: model.ChannelInfo{
				CredentialMode:     constant.ChannelCredentialModeAccountPool,
				AccountPoolEnabled: true,
			},
		}
		require.NoError(t, db.Create(&channel).Error)
		require.NoError(t, db.Create(&[]model.ChannelAccount{
			{
				ChannelId: channel.Id,
				Name:      "lower-weight",
				Key:       "sk-lower-weight",
				Status:    common.ChannelStatusEnabled,
				Models:    "gpt-route",
				Group:     "default",
				Priority:  1,
				Weight:    10,
			},
			{
				ChannelId: channel.Id,
				Name:      "higher-weight",
				Key:       "sk-higher-weight",
				Status:    common.ChannelStatusEnabled,
				Models:    "gpt-route",
				Group:     "default",
				Priority:  1,
				Weight:    100,
			},
		}).Error)

		candidate, selectedGroup, err := SelectRoutingCandidate(&RetryParam{
			Ctx:        newRoutingSelectTestContext(),
			TokenGroup: "default",
			ModelName:  "gpt-route",
			Retry:      common.GetPointer(0),
		})

		require.NoError(t, err)
		require.Equal(t, "default", selectedGroup)
		require.NotNil(t, candidate)
		require.Equal(t, model.RoutingCredentialKindChannelAccount, candidate.Kind)
		require.Equal(t, channel.Id, candidate.ChannelID)
		require.EqualValues(t, 1, candidate.Schedule.CredentialPriority)
		require.Equal(t, 100, candidate.Schedule.CredentialWeight)
	})
}

func TestSelectRoutingCandidatePrefersHealthyCandidateWithinSameLayer(t *testing.T) {
	clearRoutingHealthCacheForTest(t)
	db := setupChannelAccountSelectTestDB(t)
	highHealthy := model.Channel{
		Type:     constant.ChannelTypeOpenAI,
		Key:      "sk-high-healthy",
		Status:   common.ChannelStatusEnabled,
		Name:     "high-layer-healthy",
		Models:   "gpt-route",
		Group:    "default",
		Priority: routingSelectTestPriority(10),
		Weight:   routingSelectTestWeight(10),
	}
	highCooling := model.Channel{
		Type:     constant.ChannelTypeOpenAI,
		Key:      "sk-high-cooling",
		Status:   common.ChannelStatusEnabled,
		Name:     "high-layer-cooling",
		Models:   "gpt-route",
		Group:    "default",
		Priority: routingSelectTestPriority(10),
		Weight:   routingSelectTestWeight(10),
	}
	lowHealthy := model.Channel{
		Type:     constant.ChannelTypeOpenAI,
		Key:      "sk-low-healthy",
		Status:   common.ChannelStatusEnabled,
		Name:     "low-layer-healthy",
		Models:   "gpt-route",
		Group:    "default",
		Priority: routingSelectTestPriority(0),
		Weight:   routingSelectTestWeight(1000),
	}
	require.NoError(t, db.Create(&highHealthy).Error)
	require.NoError(t, db.Create(&highCooling).Error)
	require.NoError(t, db.Create(&lowHealthy).Error)
	RecordChannelRoutingFailure("default", "gpt-route", highCooling.Id)

	candidate, selectedGroup, err := SelectRoutingCandidate(&RetryParam{
		Ctx:        newRoutingSelectTestContext(),
		TokenGroup: "default",
		ModelName:  "gpt-route",
		Retry:      common.GetPointer(0),
	})

	require.NoError(t, err)
	require.Equal(t, "default", selectedGroup)
	require.NotNil(t, candidate)
	require.Equal(t, highHealthy.Id, candidate.ChannelID)
	require.NotEqual(t, lowHealthy.Id, candidate.ChannelID)
}

func TestSelectRoutingCandidateKeepsLeastDegradedCandidateWithinSameLayer(t *testing.T) {
	clearRoutingHealthCacheForTest(t)
	db := setupChannelAccountSelectTestDB(t)
	better := model.Channel{
		Type:     constant.ChannelTypeOpenAI,
		Key:      "sk-better",
		Status:   common.ChannelStatusEnabled,
		Name:     "better-high-layer",
		Models:   "gpt-route",
		Group:    "default",
		Priority: routingSelectTestPriority(10),
		Weight:   routingSelectTestWeight(10),
	}
	worse := model.Channel{
		Type:     constant.ChannelTypeOpenAI,
		Key:      "sk-worse",
		Status:   common.ChannelStatusEnabled,
		Name:     "worse-high-layer",
		Models:   "gpt-route",
		Group:    "default",
		Priority: routingSelectTestPriority(10),
		Weight:   routingSelectTestWeight(10),
	}
	low := model.Channel{
		Type:     constant.ChannelTypeOpenAI,
		Key:      "sk-low",
		Status:   common.ChannelStatusEnabled,
		Name:     "low-layer",
		Models:   "gpt-route",
		Group:    "default",
		Priority: routingSelectTestPriority(0),
		Weight:   routingSelectTestWeight(1000),
	}
	require.NoError(t, db.Create(&better).Error)
	require.NoError(t, db.Create(&worse).Error)
	require.NoError(t, db.Create(&low).Error)
	RecordChannelRoutingFailure("default", "gpt-route", better.Id)
	RecordChannelRoutingFailure("default", "gpt-route", worse.Id)
	RecordChannelRoutingFailure("default", "gpt-route", worse.Id)

	candidate, selectedGroup, err := SelectRoutingCandidate(&RetryParam{
		Ctx:        newRoutingSelectTestContext(),
		TokenGroup: "default",
		ModelName:  "gpt-route",
		Retry:      common.GetPointer(0),
	})

	require.NoError(t, err)
	require.Equal(t, "default", selectedGroup)
	require.NotNil(t, candidate)
	require.Equal(t, better.Id, candidate.ChannelID)
	require.NotEqual(t, low.Id, candidate.ChannelID)
}

func TestSelectRoutingCandidateReSelectsWithinSameLayerBeforeLowerLayer(t *testing.T) {
	db := setupChannelAccountSelectTestDB(t)
	firstChannel := model.Channel{
		Type:     constant.ChannelTypeOpenAI,
		Key:      "sk-first",
		Status:   common.ChannelStatusEnabled,
		Name:     "first-high-layer",
		Models:   "gpt-route",
		Group:    "default",
		Priority: routingSelectTestPriority(10),
		Weight:   routingSelectTestWeight(10),
	}
	secondChannel := model.Channel{
		Type:     constant.ChannelTypeOpenAI,
		Key:      "sk-second",
		Status:   common.ChannelStatusEnabled,
		Name:     "second-high-layer",
		Models:   "gpt-route",
		Group:    "default",
		Priority: routingSelectTestPriority(10),
		Weight:   routingSelectTestWeight(10),
	}
	lowChannel := model.Channel{
		Type:     constant.ChannelTypeOpenAI,
		Key:      "sk-low",
		Status:   common.ChannelStatusEnabled,
		Name:     "low-layer",
		Models:   "gpt-route",
		Group:    "default",
		Priority: routingSelectTestPriority(0),
		Weight:   routingSelectTestWeight(1000),
	}
	require.NoError(t, db.Create(&firstChannel).Error)
	require.NoError(t, db.Create(&secondChannel).Error)
	require.NoError(t, db.Create(&lowChannel).Error)

	ctx := newRoutingSelectTestContext()
	first, _, err := SelectRoutingCandidate(&RetryParam{
		Ctx:        ctx,
		TokenGroup: "default",
		ModelName:  "gpt-route",
		Retry:      common.GetPointer(0),
	})
	require.NoError(t, err)
	require.NotNil(t, first)
	AddExcludedRoutingCandidate(ctx, first)

	second, _, err := SelectRoutingCandidate(&RetryParam{
		Ctx:        ctx,
		TokenGroup: "default",
		ModelName:  "gpt-route",
		Retry:      common.GetPointer(0),
	})
	require.NoError(t, err)
	require.NotNil(t, second)
	require.NotEqual(t, first.ChannelID, second.ChannelID)
	require.NotEqual(t, lowChannel.Id, second.ChannelID)
}

func TestSelectRoutingCandidateForChannelUsesExplicitGroupWhenContextIsAuto(t *testing.T) {
	db := setupChannelAccountSelectTestDB(t)
	priority := int64(10)
	weight := uint(100)
	channel := model.Channel{
		Type:     constant.ChannelTypeOpenAI,
		Key:      "sk-auto-group",
		Status:   common.ChannelStatusEnabled,
		Name:     "auto-context-channel",
		Models:   "gpt-route",
		Group:    "default",
		Priority: &priority,
		Weight:   &weight,
	}
	require.NoError(t, db.Create(&channel).Error)

	ctx := newRoutingSelectTestContext()
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "auto")
	candidate, selectedGroup, err := SelectRoutingCandidateForChannel(&RetryParam{
		Ctx:        ctx,
		TokenGroup: "default",
		ModelName:  "gpt-route",
		Retry:      common.GetPointer(0),
	}, channel.Id)

	require.NoError(t, err)
	require.Equal(t, "default", selectedGroup)
	require.NotNil(t, candidate)
	require.Equal(t, channel.Id, candidate.ChannelID)
	require.Equal(t, model.RoutingCredentialKindSingleKey, candidate.Kind)
}

func TestSelectRoutingCandidateForChannelPrefersSyncedManagedWeight(t *testing.T) {
	db := setupChannelAccountSelectTestDB(t)
	priority := int64(10)
	weight := uint(100)
	channel := model.Channel{
		Type:          constant.ChannelTypeOpenAI,
		Status:        common.ChannelStatusEnabled,
		Name:          "synced-affinity-channel",
		Models:        "gpt-route",
		Group:         "default",
		Priority:      &priority,
		Weight:        &weight,
		OtherSettings: `{"upstream_account_sync":{"platform":"new-api","base_url":"https://upstream.example"}}`,
		ChannelInfo: model.ChannelInfo{
			CredentialMode:     constant.ChannelCredentialModeAccountPool,
			AccountPoolEnabled: true,
		},
	}
	require.NoError(t, db.Create(&channel).Error)
	cheap := model.ChannelAccount{
		ChannelId:     channel.Id,
		Name:          "cheap-key",
		Key:           "sk-cheap",
		Status:        common.ChannelStatusEnabled,
		Models:        "gpt-route",
		AccessGroups:  "default",
		Priority:      0,
		Weight:        150,
		OtherSettings: `{"upstream_account_sync":{"platform":"new-api","base_url":"https://upstream.example","external_id":"cheap","key_digest":"cheap","ratio_conversion":0.5}}`,
	}
	expensive := model.ChannelAccount{
		ChannelId:     channel.Id,
		Name:          "expensive-key",
		Key:           "sk-expensive",
		Status:        common.ChannelStatusEnabled,
		Models:        "gpt-route",
		AccessGroups:  "default",
		Priority:      0,
		Weight:        80,
		OtherSettings: `{"upstream_account_sync":{"platform":"new-api","base_url":"https://upstream.example","external_id":"expensive","key_digest":"expensive","ratio_conversion":1.2}}`,
	}
	require.NoError(t, db.Create(&cheap).Error)
	require.NoError(t, db.Create(&expensive).Error)

	candidate, selectedGroup, err := SelectRoutingCandidateForChannel(&RetryParam{
		Ctx:         newRoutingSelectTestContext(),
		TokenGroup:  "default",
		ModelName:   "gpt-route",
		RequestPath: "/v1/chat/completions",
		Retry:       common.GetPointer(0),
	}, channel.Id)

	require.NoError(t, err)
	require.Equal(t, "default", selectedGroup)
	require.NotNil(t, candidate)
	require.Equal(t, cheap.Id, candidate.ChannelAccountID)
	require.Equal(t, 150, candidate.Schedule.CredentialWeight)
}

// TestSelectRoutingCandidatePrefersLowestConvertedRatioAcrossChannels 验证同一调度层的
// 同步账号按真实转换倍率升序选择，而不是继续由旧的密钥权重决定胜负。失败候选进入
// 请求级排除集后，下一次选择必须从全渠道剩余候选中继续按最低倍率选择。
func TestSelectRoutingCandidatePrefersLowestConvertedRatioAcrossChannels(t *testing.T) {
	db := setupChannelAccountSelectTestDB(t)
	priority := int64(10)
	weight := uint(100)
	channels := []model.Channel{
		{
			Type:          constant.ChannelTypeOpenAI,
			Status:        common.ChannelStatusEnabled,
			Name:          "expensive-synced-channel",
			Models:        "gpt-route",
			Group:         "default",
			Priority:      &priority,
			Weight:        &weight,
			OtherSettings: `{"upstream_account_sync":{"platform":"new-api","base_url":"https://upstream.example"}}`,
			ChannelInfo: model.ChannelInfo{
				CredentialMode:     constant.ChannelCredentialModeAccountPool,
				AccountPoolEnabled: true,
			},
		},
		{
			Type:          constant.ChannelTypeOpenAI,
			Status:        common.ChannelStatusEnabled,
			Name:          "cheap-synced-channel",
			Models:        "gpt-route",
			Group:         "default",
			Priority:      &priority,
			Weight:        &weight,
			OtherSettings: `{"upstream_account_sync":{"platform":"new-api","base_url":"https://upstream.example"}}`,
			ChannelInfo: model.ChannelInfo{
				CredentialMode:     constant.ChannelCredentialModeAccountPool,
				AccountPoolEnabled: true,
			},
		},
	}
	require.NoError(t, db.Create(&channels).Error)

	expensive := model.ChannelAccount{
		ChannelId:     channels[0].Id,
		Name:          "expensive-high-weight-key",
		Key:           "sk-expensive",
		Status:        common.ChannelStatusEnabled,
		Models:        "gpt-route",
		AccessGroups:  "default",
		Priority:      1,
		Weight:        100,
		OtherSettings: `{"upstream_account_sync":{"platform":"new-api","base_url":"https://upstream.example","ratio_conversion":1.2}}`,
	}
	cheap := model.ChannelAccount{
		ChannelId:     channels[1].Id,
		Name:          "cheap-low-weight-key",
		Key:           "sk-cheap",
		Status:        common.ChannelStatusEnabled,
		Models:        "gpt-route",
		AccessGroups:  "default",
		Priority:      1,
		Weight:        10,
		OtherSettings: `{"upstream_account_sync":{"platform":"new-api","base_url":"https://upstream.example","ratio_conversion":0.5}}`,
	}
	require.NoError(t, db.Create(&expensive).Error)
	require.NoError(t, db.Create(&cheap).Error)

	ctx := newRoutingSelectTestContext()
	first, selectedGroup, err := SelectRoutingCandidate(&RetryParam{
		Ctx:        ctx,
		TokenGroup: "default",
		ModelName:  "gpt-route",
		Retry:      common.GetPointer(0),
	})
	require.NoError(t, err)
	require.Equal(t, "default", selectedGroup)
	require.NotNil(t, first)
	require.Equal(t, channels[1].Id, first.ChannelID)
	require.Equal(t, cheap.Id, first.ChannelAccountID)
	require.True(t, first.HasConvertedRatio)
	require.InDelta(t, 0.5, first.ConvertedRatio, 0.000001)

	serviceExcluded := first.Clone()
	AddExcludedRoutingCandidate(ctx, serviceExcluded)
	second, _, err := SelectRoutingCandidate(&RetryParam{
		Ctx:        ctx,
		TokenGroup: "default",
		ModelName:  "gpt-route",
		Retry:      common.GetPointer(0),
	})
	require.NoError(t, err)
	require.NotNil(t, second)
	require.Equal(t, channels[0].Id, second.ChannelID)
	require.Equal(t, expensive.Id, second.ChannelAccountID)
}

// TestSelectRoutingCandidateDoesNotLetCostOverrideHigherPriorityLayers 验证转换倍率只在
// 渠道优先级、渠道权重和密钥优先级均相同后生效，低成本候选不能跨层级反超。
func TestSelectRoutingCandidateDoesNotLetCostOverrideHigherPriorityLayers(t *testing.T) {
	tests := []struct {
		name        string
		highChannel model.Channel
		lowChannel  model.Channel
		highAccount model.ChannelAccount
		lowAccount  model.ChannelAccount
	}{
		{
			name: "channel priority remains first",
			highChannel: model.Channel{
				Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Name: "higher-channel-priority",
				Models: "gpt-route", Group: "default", Priority: routingSelectTestPriority(2), Weight: routingSelectTestWeight(1),
				OtherSettings: `{"upstream_account_sync":{"platform":"new-api","base_url":"https://upstream.example"}}`,
				ChannelInfo:   model.ChannelInfo{CredentialMode: constant.ChannelCredentialModeAccountPool, AccountPoolEnabled: true},
			},
			lowChannel: model.Channel{
				Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Name: "lower-channel-priority",
				Models: "gpt-route", Group: "default", Priority: routingSelectTestPriority(1), Weight: routingSelectTestWeight(100),
				OtherSettings: `{"upstream_account_sync":{"platform":"new-api","base_url":"https://upstream.example"}}`,
				ChannelInfo:   model.ChannelInfo{CredentialMode: constant.ChannelCredentialModeAccountPool, AccountPoolEnabled: true},
			},
			highAccount: model.ChannelAccount{Name: "high-priority-costly", Key: "sk-high-priority", Status: common.ChannelStatusEnabled, Models: "gpt-route", AccessGroups: "default", Priority: 1, Weight: 1, OtherSettings: `{"upstream_account_sync":{"ratio_conversion":2}}`},
			lowAccount:  model.ChannelAccount{Name: "low-priority-cheap", Key: "sk-low-priority", Status: common.ChannelStatusEnabled, Models: "gpt-route", AccessGroups: "default", Priority: 99, Weight: 100, OtherSettings: `{"upstream_account_sync":{"ratio_conversion":0.1}}`},
		},
		{
			name: "channel weight remains second",
			highChannel: model.Channel{
				Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Name: "higher-channel-weight",
				Models: "gpt-route", Group: "default", Priority: routingSelectTestPriority(1), Weight: routingSelectTestWeight(100),
				OtherSettings: `{"upstream_account_sync":{"platform":"new-api","base_url":"https://upstream.example"}}`,
				ChannelInfo:   model.ChannelInfo{CredentialMode: constant.ChannelCredentialModeAccountPool, AccountPoolEnabled: true},
			},
			lowChannel: model.Channel{
				Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Name: "lower-channel-weight",
				Models: "gpt-route", Group: "default", Priority: routingSelectTestPriority(1), Weight: routingSelectTestWeight(1),
				OtherSettings: `{"upstream_account_sync":{"platform":"new-api","base_url":"https://upstream.example"}}`,
				ChannelInfo:   model.ChannelInfo{CredentialMode: constant.ChannelCredentialModeAccountPool, AccountPoolEnabled: true},
			},
			highAccount: model.ChannelAccount{Name: "high-weight-costly", Key: "sk-high-weight", Status: common.ChannelStatusEnabled, Models: "gpt-route", AccessGroups: "default", Priority: 1, Weight: 1, OtherSettings: `{"upstream_account_sync":{"ratio_conversion":2}}`},
			lowAccount:  model.ChannelAccount{Name: "low-weight-cheap", Key: "sk-low-weight", Status: common.ChannelStatusEnabled, Models: "gpt-route", AccessGroups: "default", Priority: 99, Weight: 100, OtherSettings: `{"upstream_account_sync":{"ratio_conversion":0.1}}`},
		},
		{
			name: "credential priority remains third",
			highChannel: model.Channel{
				Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Name: "higher-credential-priority",
				Models: "gpt-route", Group: "default", Priority: routingSelectTestPriority(1), Weight: routingSelectTestWeight(100),
				OtherSettings: `{"upstream_account_sync":{"platform":"new-api","base_url":"https://upstream.example"}}`,
				ChannelInfo:   model.ChannelInfo{CredentialMode: constant.ChannelCredentialModeAccountPool, AccountPoolEnabled: true},
			},
			lowChannel: model.Channel{
				Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Name: "lower-credential-priority",
				Models: "gpt-route", Group: "default", Priority: routingSelectTestPriority(1), Weight: routingSelectTestWeight(100),
				OtherSettings: `{"upstream_account_sync":{"platform":"new-api","base_url":"https://upstream.example"}}`,
				ChannelInfo:   model.ChannelInfo{CredentialMode: constant.ChannelCredentialModeAccountPool, AccountPoolEnabled: true},
			},
			highAccount: model.ChannelAccount{Name: "high-credential-costly", Key: "sk-high-credential", Status: common.ChannelStatusEnabled, Models: "gpt-route", AccessGroups: "default", Priority: 2, Weight: 1, OtherSettings: `{"upstream_account_sync":{"ratio_conversion":2}}`},
			lowAccount:  model.ChannelAccount{Name: "low-credential-cheap", Key: "sk-low-credential", Status: common.ChannelStatusEnabled, Models: "gpt-route", AccessGroups: "default", Priority: 1, Weight: 100, OtherSettings: `{"upstream_account_sync":{"ratio_conversion":0.1}}`},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			db := setupChannelAccountSelectTestDB(t)
			require.NoError(t, db.Create(&testCase.highChannel).Error)
			require.NoError(t, db.Create(&testCase.lowChannel).Error)
			testCase.highAccount.ChannelId = testCase.highChannel.Id
			testCase.lowAccount.ChannelId = testCase.lowChannel.Id
			require.NoError(t, db.Create(&testCase.highAccount).Error)
			require.NoError(t, db.Create(&testCase.lowAccount).Error)

			candidate, _, err := SelectRoutingCandidate(&RetryParam{
				Ctx:        newRoutingSelectTestContext(),
				TokenGroup: "default",
				ModelName:  "gpt-route",
				Retry:      common.GetPointer(0),
			})
			require.NoError(t, err)
			require.NotNil(t, candidate)
			require.Equal(t, testCase.highChannel.Id, candidate.ChannelID)
			require.Equal(t, testCase.highAccount.Id, candidate.ChannelAccountID)
		})
	}
}

// TestSelectRoutingCandidateUsesWeightFallbackForMissingConvertedRatio 验证同层存在无成本
// 元数据的候选时继续使用原有密钥权重。这样普通手动账号、Multi-Key 与历史同步数据
// 不会因为无法比较真实成本而被新版调度静默排除。
func TestSelectRoutingCandidateUsesWeightFallbackForMissingConvertedRatio(t *testing.T) {
	db := setupChannelAccountSelectTestDB(t)
	channel := model.Channel{
		Type:          constant.ChannelTypeOpenAI,
		Status:        common.ChannelStatusEnabled,
		Name:          "mixed-cost-metadata-channel",
		Models:        "gpt-route",
		Group:         "default",
		Priority:      routingSelectTestPriority(1),
		Weight:        routingSelectTestWeight(100),
		OtherSettings: `{"upstream_account_sync":{"platform":"new-api","base_url":"https://upstream.example"}}`,
		ChannelInfo: model.ChannelInfo{
			CredentialMode:     constant.ChannelCredentialModeAccountPool,
			AccountPoolEnabled: true,
		},
	}
	require.NoError(t, db.Create(&channel).Error)
	knownCost := model.ChannelAccount{
		ChannelId:     channel.Id,
		Name:          "known-cheap-key",
		Key:           "sk-known-cheap",
		Status:        common.ChannelStatusEnabled,
		Models:        "gpt-route",
		AccessGroups:  "default",
		Priority:      1,
		Weight:        10,
		OtherSettings: `{"upstream_account_sync":{"ratio_conversion":0.1}}`,
	}
	legacyManual := model.ChannelAccount{
		ChannelId:    channel.Id,
		Name:         "legacy-manual-key",
		Key:          "sk-legacy-manual",
		Status:       common.ChannelStatusEnabled,
		Models:       "gpt-route",
		AccessGroups: "default",
		Priority:     1,
		Weight:       100,
	}
	require.NoError(t, db.Create(&knownCost).Error)
	require.NoError(t, db.Create(&legacyManual).Error)

	candidate, _, err := SelectRoutingCandidate(&RetryParam{
		Ctx:        newRoutingSelectTestContext(),
		TokenGroup: "default",
		ModelName:  "gpt-route",
		Retry:      common.GetPointer(0),
	})
	require.NoError(t, err)
	require.NotNil(t, candidate)
	require.Equal(t, legacyManual.Id, candidate.ChannelAccountID)
	require.False(t, candidate.HasConvertedRatio)
}
