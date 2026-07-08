package model

import (
	"testing"
	"time"

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
