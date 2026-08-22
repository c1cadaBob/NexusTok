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

func TestSelectRoutingCandidateHonorsCredentialPriorityAcrossChannels(t *testing.T) {
	db := setupChannelAccountSelectTestDB(t)
	lowChannel := model.Channel{
		Type:     constant.ChannelTypeOpenAI,
		Key:      "sk-low-channel",
		Status:   common.ChannelStatusEnabled,
		Name:     "low-channel-high-weight",
		Models:   "gpt-route",
		Group:    "default",
		Priority: routingSelectTestPriority(0),
		Weight:   routingSelectTestWeight(1000),
	}
	require.NoError(t, db.Create(&lowChannel).Error)
	highKeyChannel := model.Channel{
		Type:     constant.ChannelTypeOpenAI,
		Status:   common.ChannelStatusEnabled,
		Name:     "high-key-channel",
		Models:   "gpt-route",
		Group:    "default",
		Priority: routingSelectTestPriority(0),
		Weight:   routingSelectTestWeight(0),
		ChannelInfo: model.ChannelInfo{
			CredentialMode:     constant.ChannelCredentialModeAccountPool,
			AccountPoolEnabled: true,
		},
	}
	require.NoError(t, db.Create(&highKeyChannel).Error)
	account := model.ChannelAccount{
		ChannelId: highKeyChannel.Id,
		Name:      "priority-key",
		Key:       "sk-priority",
		Status:    common.ChannelStatusEnabled,
		Models:    "gpt-route",
		Group:     "default",
		Priority:  1,
		Weight:    195,
	}
	require.NoError(t, db.Create(&account).Error)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	candidate, selectedGroup, err := SelectRoutingCandidate(&RetryParam{
		Ctx:        c,
		TokenGroup: "default",
		ModelName:  "gpt-route",
		Retry:      common.GetPointer(0),
	})

	require.NoError(t, err)
	require.Equal(t, "default", selectedGroup)
	require.NotNil(t, candidate)
	require.Equal(t, model.RoutingCredentialKindChannelAccount, candidate.Kind)
	require.Equal(t, highKeyChannel.Id, candidate.ChannelID)
	require.Equal(t, account.Id, candidate.ChannelAccountID)
	require.EqualValues(t, 1, candidate.Schedule.EffectivePriority)
	require.Equal(t, 195, candidate.Schedule.EffectiveWeight)
}

func TestSelectRoutingCandidateExcludesFailedCandidateBeforeChannel(t *testing.T) {
	db := setupChannelAccountSelectTestDB(t)
	channel := model.Channel{
		Type:     constant.ChannelTypeOpenAI,
		Status:   common.ChannelStatusEnabled,
		Name:     "account-channel",
		Models:   "gpt-route",
		Group:    "default",
		Priority: routingSelectTestPriority(0),
		Weight:   routingSelectTestWeight(0),
		ChannelInfo: model.ChannelInfo{
			CredentialMode:     constant.ChannelCredentialModeAccountPool,
			AccountPoolEnabled: true,
		},
	}
	require.NoError(t, db.Create(&channel).Error)
	accounts := []model.ChannelAccount{
		{ChannelId: channel.Id, Name: "first", Key: "sk-first", Status: common.ChannelStatusEnabled, Models: "gpt-route", Group: "default", Priority: 1, Weight: 10},
		{ChannelId: channel.Id, Name: "second", Key: "sk-second", Status: common.ChannelStatusEnabled, Models: "gpt-route", Group: "default", Priority: 1, Weight: 10},
	}
	require.NoError(t, db.Create(&accounts).Error)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	first, _, err := SelectRoutingCandidate(&RetryParam{Ctx: c, TokenGroup: "default", ModelName: "gpt-route", Retry: common.GetPointer(0)})
	require.NoError(t, err)
	require.NotNil(t, first)
	AddExcludedRoutingCandidate(c, first)

	second, _, err := SelectRoutingCandidate(&RetryParam{Ctx: c, TokenGroup: "default", ModelName: "gpt-route", Retry: common.GetPointer(0)})
	require.NoError(t, err)
	require.NotNil(t, second)
	require.Equal(t, channel.Id, second.ChannelID)
	require.NotEqual(t, first.ChannelAccountID, second.ChannelAccountID)
	require.Empty(t, GetExcludedChannelIds(c))
}
