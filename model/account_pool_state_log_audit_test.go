package model

import (
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAccountPoolStateLogAuditModelTest(t *testing.T) {
	t.Helper()
	oldDB := DB
	oldLogDB := LOG_DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	LOG_DB = db
	require.NoError(t, db.AutoMigrate(&AccountPoolGroup{}, &PoolAccount{}, &PoolAccountStateLog{}))
	t.Cleanup(func() {
		DB = oldDB
		LOG_DB = oldLogDB
	})
}

func createAccountPoolStateLogAuditGroup(t *testing.T, name string) *AccountPoolGroup {
	t.Helper()
	group := &AccountPoolGroup{
		Name:     name,
		Platform: "codex",
		AuthType: AccountPoolAuthTypeAPIKey,
		Source:   AccountPoolGroupSourceNative,
		Status:   common.ChannelStatusEnabled,
		Strategy: AccountPoolStrategyRoundRobin,
	}
	require.NoError(t, DB.Create(group).Error)
	return group
}

func createAccountPoolStateLogAuditAccount(t *testing.T, group *AccountPoolGroup, name string) *PoolAccount {
	t.Helper()
	account := &PoolAccount{
		PoolGroupId:       group.Id,
		Name:              name,
		Platform:          group.Platform,
		AuthType:          group.AuthType,
		Credentials:       "encrypted-placeholder",
		CredentialSummary: "masked",
		Status:            common.ChannelStatusEnabled,
		Schedulable:       true,
	}
	require.NoError(t, DB.Create(account).Error)
	return account
}

func createAccountPoolStateAuditLog(t *testing.T, group *AccountPoolGroup, account *PoolAccount, action string, source string, actor string, reason string, requestID string, createdAt int64) {
	t.Helper()
	require.NoError(t, LOG_DB.Create(&PoolAccountStateLog{
		CreatedAt:           createdAt,
		PoolGroupId:         group.Id,
		PoolGroupName:       group.Name,
		PoolAccountId:       account.Id,
		PoolAccountName:     account.Name,
		PoolAccountAuthType: account.AuthType,
		Action:              action,
		Source:              source,
		Actor:               actor,
		Reason:              reason,
		RequestId:           requestID,
		BeforeStatus:        common.ChannelStatusEnabled,
		AfterStatus:         common.ChannelStatusManuallyDisabled,
		BeforeSchedulable:   true,
		AfterSchedulable:    false,
	}).Error)
}

func TestGetPoolAccountStateLogAuditSummaryAggregatesFilteredLogs(t *testing.T) {
	setupAccountPoolStateLogAuditModelTest(t)
	now := common.GetTimestamp()
	group := createAccountPoolStateLogAuditGroup(t, "audit-main")
	otherGroup := createAccountPoolStateLogAuditGroup(t, "audit-other")
	accountA := createAccountPoolStateLogAuditAccount(t, group, "audit-a")
	accountB := createAccountPoolStateLogAuditAccount(t, group, "audit-b")
	accountC := createAccountPoolStateLogAuditAccount(t, group, "audit-c")
	otherAccount := createAccountPoolStateLogAuditAccount(t, otherGroup, "audit-other")

	createAccountPoolStateAuditLog(t, group, accountA, PoolAccountStateActionManualStatus, "admin", "alice", "批量禁用账号", "bulk-req", now-30)
	createAccountPoolStateAuditLog(t, group, accountB, PoolAccountStateActionManualStatus, "admin", "alice", "批量禁用账号", "bulk-req", now-20)
	createAccountPoolStateAuditLog(t, group, accountC, PoolAccountStateActionRelayError, "relay", "", "上游 429", "relay-req", now-10)
	createAccountPoolStateAuditLog(t, otherGroup, otherAccount, PoolAccountStateActionManualDelete, "admin", "bob", "其它分组删除", "other-req", now-5)

	summary, err := GetPoolAccountStateLogAuditSummary(PoolAccountStateLogFilter{PoolGroupId: group.Id})
	require.NoError(t, err)
	require.NotNil(t, summary)
	require.Equal(t, int64(3), summary.Total)
	require.Equal(t, int64(2), summary.ManualTotal)
	require.Equal(t, int64(1), summary.AutomaticTotal)
	require.Equal(t, int64(3), summary.AffectedAccounts)
	require.NotEmpty(t, summary.ActionStats)
	require.Equal(t, PoolAccountStateActionManualStatus, summary.ActionStats[0].Key)
	require.Equal(t, int64(2), summary.ActionStats[0].Total)
	require.NotEmpty(t, summary.SourceStats)
	require.Equal(t, "admin", summary.SourceStats[0].Key)
	require.Equal(t, int64(2), summary.SourceStats[0].Total)
	require.Len(t, summary.ActorStats, 1)
	require.Equal(t, "alice", summary.ActorStats[0].Key)
	require.Equal(t, int64(2), summary.ActorStats[0].Total)
	require.Len(t, summary.RecentBulkOperations, 1)
	require.Equal(t, "bulk-req", summary.RecentBulkOperations[0].RequestId)
	require.Equal(t, 2, summary.RecentBulkOperations[0].AccountCount)
	require.Len(t, summary.RecentBulkOperations[0].SampleAccounts, 2)
	require.Equal(t, accountB.Id, summary.RecentBulkOperations[0].SampleAccounts[0].Id)
	require.Equal(t, accountA.Id, summary.RecentBulkOperations[0].SampleAccounts[1].Id)
}

func TestGetPoolAccountStateLogsFiltersByRequestIdAndSearch(t *testing.T) {
	setupAccountPoolStateLogAuditModelTest(t)
	now := common.GetTimestamp()
	group := createAccountPoolStateLogAuditGroup(t, "audit-request")
	accountA := createAccountPoolStateLogAuditAccount(t, group, "audit-request-a")
	accountB := createAccountPoolStateLogAuditAccount(t, group, "audit-request-b")

	createAccountPoolStateAuditLog(t, group, accountA, PoolAccountStateActionManualStatus, "admin", "alice", "first request", "req-alpha", now-20)
	createAccountPoolStateAuditLog(t, group, accountB, PoolAccountStateActionManualDelete, "admin", "alice", "second request", "req-beta", now-10)

	logs, total, err := GetPoolAccountStateLogs(PoolAccountStateLogFilter{
		RequestId: "req-alpha",
		Limit:     10,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, logs, 1)
	require.Equal(t, "req-alpha", logs[0].RequestId)
	require.Equal(t, accountA.Id, logs[0].PoolAccountId)

	logs, total, err = GetPoolAccountStateLogs(PoolAccountStateLogFilter{
		Search: "req-beta",
		Limit:  10,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, logs, 1)
	require.Equal(t, "req-beta", logs[0].RequestId)
	require.Equal(t, accountB.Id, logs[0].PoolAccountId)
}
