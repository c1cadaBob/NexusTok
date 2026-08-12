package model

import (
	"testing"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/stretchr/testify/require"
)

func TestSumUsedQuotaReturnsZeroForEmptyResult(t *testing.T) {
	truncateTables(t)
	initCol()

	stat, err := SumUsedQuota(LogTypeUnknown, 100, 200, "", "missing", "", 0, "")
	require.NoError(t, err)
	require.Equal(t, Stat{}, stat)

	token := SumUsedToken(LogTypeUnknown, 100, 200, "", "missing", "")
	require.Zero(t, token)
}

func TestSumUsedQuotaUsesConsumeLogsAndCoalescesTokenTotals(t *testing.T) {
	truncateTables(t)
	initCol()
	now := time.Now().Unix()

	require.NoError(t, LOG_DB.Create(&[]Log{
		{
			UserId:           1,
			Username:         "alice",
			CreatedAt:        now,
			Type:             LogTypeConsume,
			ModelName:        "gpt-a",
			Quota:            100,
			PromptTokens:     10,
			CompletionTokens: 5,
			ChannelId:        3,
			Group:            "vip",
		},
		{
			UserId:           1,
			Username:         "alice",
			CreatedAt:        now - 120,
			Type:             LogTypeConsume,
			ModelName:        "gpt-a",
			Quota:            50,
			PromptTokens:     30,
			CompletionTokens: 20,
			ChannelId:        3,
			Group:            "vip",
		},
		{
			UserId:    1,
			Username:  "alice",
			CreatedAt: now,
			Type:      LogTypeManage,
			ModelName: "gpt-a",
			Quota:     999,
			ChannelId: 3,
			Group:     "vip",
		},
	}).Error)

	stat, err := SumUsedQuota(LogTypeUnknown, now-300, now+1, "gpt-a", "alice", "", 3, "vip")
	require.NoError(t, err)
	require.Equal(t, 150, stat.Quota)
	require.Equal(t, 1, stat.Rpm)
	require.Equal(t, 15, stat.Tpm)

	token := SumUsedToken(LogTypeUnknown, now-300, now+1, "gpt-a", "alice", "")
	require.Equal(t, 65, token)
}

func TestSumUpstreamCostUsesStandardQuotaAndLegacyFallback(t *testing.T) {
	truncateTables(t)
	initCol()
	now := time.Now().Unix()

	newLogOther := common.MapToJsonStr(map[string]interface{}{
		"admin_info": map[string]interface{}{
			"ratio_conversion":       0.5,
			"standard_billing_quota": 100.0,
		},
		"group_ratio": 2,
	})
	legacyLogOther := common.MapToJsonStr(map[string]interface{}{
		"admin_info": map[string]interface{}{
			"ratio_conversion": 0.25,
		},
		"user_group_ratio": 4,
		"group_ratio":      2,
	})
	invalidGroupOther := common.MapToJsonStr(map[string]interface{}{
		"admin_info": map[string]interface{}{
			"ratio_conversion": 0.25,
		},
		"group_ratio": 0,
	})
	missingRatioOther := common.MapToJsonStr(map[string]interface{}{
		"admin_info": map[string]interface{}{
			"standard_billing_quota": 100.0,
		},
		"group_ratio": 2,
	})
	negativeQuotaOther := common.MapToJsonStr(map[string]interface{}{
		"admin_info": map[string]interface{}{
			"ratio_conversion": 0.25,
		},
		"group_ratio": 2,
	})

	require.NoError(t, LOG_DB.Create(&[]Log{
		{
			UserId:    1,
			Username:  "alice",
			CreatedAt: now,
			Type:      LogTypeConsume,
			ModelName: "gpt-cost",
			Quota:     200,
			ChannelId: 8,
			Group:     "vip",
			TokenName: "token-a",
			Other:     newLogOther,
		},
		{
			UserId:    1,
			Username:  "alice",
			CreatedAt: now,
			Type:      LogTypeConsume,
			ModelName: "gpt-cost",
			Quota:     400,
			ChannelId: 8,
			Group:     "vip",
			TokenName: "token-a",
			Other:     legacyLogOther,
		},
		{
			UserId:    1,
			Username:  "alice",
			CreatedAt: now,
			Type:      LogTypeConsume,
			ModelName: "gpt-cost",
			Quota:     400,
			ChannelId: 8,
			Group:     "vip",
			TokenName: "token-a",
			Other:     invalidGroupOther,
		},
		{
			UserId:    1,
			Username:  "alice",
			CreatedAt: now,
			Type:      LogTypeConsume,
			ModelName: "gpt-cost",
			Quota:     400,
			ChannelId: 8,
			Group:     "vip",
			TokenName: "token-a",
			Other:     missingRatioOther,
		},
		{
			UserId:    1,
			Username:  "alice",
			CreatedAt: now,
			Type:      LogTypeConsume,
			ModelName: "gpt-cost",
			Quota:     -100,
			ChannelId: 8,
			Group:     "vip",
			TokenName: "token-a",
			Other:     negativeQuotaOther,
		},
		{
			UserId:    1,
			Username:  "alice",
			CreatedAt: now,
			Type:      LogTypeManage,
			ModelName: "gpt-cost",
			Quota:     999,
			ChannelId: 8,
			Group:     "vip",
			TokenName: "token-a",
			Other:     newLogOther,
		},
	}).Error)

	cost, err := SumUpstreamCost(now-60, now+60, "gpt-cost", "alice", "token-a", 8, "vip")
	require.NoError(t, err)
	// 新日志：100 * 0.5 = 50；旧日志：400 / user_group_ratio(4) * 0.25 = 25。
	require.InDelta(t, 75, cost, 0.000001)
}
