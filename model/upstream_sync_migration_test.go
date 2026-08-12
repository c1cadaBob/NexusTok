package model

import (
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupUpstreamSyncMigrationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB := DB
	oldLogDB := LOG_DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &ChannelAccount{}, &Log{}))
	DB = db
	LOG_DB = db
	t.Cleanup(func() {
		DB = oldDB
		LOG_DB = oldLogDB
	})
	return db
}

func TestMigrateSyncedAccountModelsRunsOnlyOnce(t *testing.T) {
	db := setupUpstreamSyncMigrationTestDB(t)
	channel := Channel{
		Type:          constant.ChannelTypeNewAPI,
		Status:        common.ChannelStatusEnabled,
		Name:          "synced-channel",
		Models:        "gpt-old",
		Group:         "default",
		OtherSettings: `{"upstream_account_sync":{"platform":"new-api","base_url":"https://upstream.example"}}`,
	}
	require.NoError(t, db.Create(&channel).Error)
	account := ChannelAccount{
		ChannelId: channel.Id,
		Name:      "empty-model-key",
		Key:       "sk-empty",
		Status:    common.ChannelStatusEnabled,
		Models:    "",
	}
	require.NoError(t, db.Create(&account).Error)

	require.NoError(t, migrateSyncedAccountModels())

	var migrated ChannelAccount
	require.NoError(t, db.First(&migrated, account.Id).Error)
	require.Equal(t, "gpt-old", migrated.Models)

	require.NoError(t, db.Model(&migrated).Update("models", "").Error)
	require.NoError(t, migrateSyncedAccountModels())

	var cleared ChannelAccount
	require.NoError(t, db.First(&cleared, account.Id).Error)
	require.Equal(t, "", cleared.Models)
}

func TestMigrateSyncedAccountAccessGroupsRunsOnlyOnce(t *testing.T) {
	db := setupUpstreamSyncMigrationTestDB(t)
	channel := Channel{
		Type:          constant.ChannelTypeNewAPI,
		Status:        common.ChannelStatusEnabled,
		Name:          "synced-channel",
		Models:        "gpt-old",
		Group:         "vip",
		OtherSettings: `{"upstream_account_sync":{"platform":"new-api","base_url":"https://upstream.example"}}`,
	}
	require.NoError(t, db.Create(&channel).Error)
	account := ChannelAccount{
		ChannelId:    channel.Id,
		Name:         "empty-access-key",
		Key:          "sk-empty",
		Status:       common.ChannelStatusEnabled,
		Models:       "gpt-old",
		AccessGroups: "",
	}
	require.NoError(t, db.Create(&account).Error)

	require.NoError(t, migrateSyncedAccountAccessGroups())

	var migrated ChannelAccount
	require.NoError(t, db.First(&migrated, account.Id).Error)
	require.Equal(t, "vip", migrated.AccessGroups)

	require.NoError(t, db.Model(&migrated).Update("access_groups", "").Error)
	require.NoError(t, migrateSyncedAccountAccessGroups())

	var cleared ChannelAccount
	require.NoError(t, db.First(&cleared, account.Id).Error)
	require.Equal(t, "", cleared.AccessGroups)
}

func TestMigrateSyncedAccountLocalUsedQuotaRebuildsFromConsumeLogs(t *testing.T) {
	db := setupUpstreamSyncMigrationTestDB(t)
	synced := Channel{
		Type:          constant.ChannelTypeNewAPI,
		Status:        common.ChannelStatusEnabled,
		Name:          "synced-channel",
		UsedQuota:     999999,
		OtherSettings: `{"upstream_account_sync":{"platform":"new-api","base_url":"https://upstream.example"}}`,
	}
	plain := Channel{
		Type:      constant.ChannelTypeOpenAI,
		Status:    common.ChannelStatusEnabled,
		Name:      "plain-channel",
		UsedQuota: 888888,
	}
	marked := Channel{
		Type:          constant.ChannelTypeSub2API,
		Status:        common.ChannelStatusEnabled,
		Name:          "marked-channel",
		UsedQuota:     777777,
		OtherSettings: `{"upstream_account_sync":{"platform":"sub2api","base_url":"https://sub.example","migrations":{"local_used_quota_rebuilt":true}}}`,
	}
	require.NoError(t, db.Create(&synced).Error)
	require.NoError(t, db.Create(&plain).Error)
	require.NoError(t, db.Create(&marked).Error)
	require.NoError(t, LOG_DB.Create(&[]Log{
		{Type: LogTypeConsume, ChannelId: synced.Id, Quota: 120},
		{Type: LogTypeConsume, ChannelId: synced.Id, Quota: 30},
		{Type: LogTypeRefund, ChannelId: synced.Id, Quota: 900},
		{Type: LogTypeConsume, ChannelId: plain.Id, Quota: 111},
		{Type: LogTypeConsume, ChannelId: marked.Id, Quota: 222},
	}).Error)

	require.NoError(t, migrateSyncedAccountLocalUsedQuota())

	var gotSynced Channel
	require.NoError(t, db.First(&gotSynced, synced.Id).Error)
	require.Equal(t, int64(150), gotSynced.UsedQuota)
	require.True(t, syncedAccountMigrationDone(gotSynced.OtherSettings, "local_used_quota_rebuilt"))

	var gotPlain Channel
	require.NoError(t, db.First(&gotPlain, plain.Id).Error)
	require.Equal(t, int64(888888), gotPlain.UsedQuota)

	var gotMarked Channel
	require.NoError(t, db.First(&gotMarked, marked.Id).Error)
	require.Equal(t, int64(777777), gotMarked.UsedQuota)

	require.NoError(t, db.Model(&gotSynced).Update("used_quota", 42).Error)
	require.NoError(t, migrateSyncedAccountLocalUsedQuota())
	require.NoError(t, db.First(&gotSynced, synced.Id).Error)
	require.Equal(t, int64(42), gotSynced.UsedQuota)
}

func TestMigrateSyncedAccountLocalUsedQuotaUsesSeparateLogDB(t *testing.T) {
	oldDB := DB
	oldLogDB := LOG_DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	logDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}))
	require.NoError(t, logDB.AutoMigrate(&Log{}))
	DB = db
	LOG_DB = logDB
	t.Cleanup(func() {
		DB = oldDB
		LOG_DB = oldLogDB
	})

	channel := Channel{
		Type:          constant.ChannelTypeNewAPI,
		Status:        common.ChannelStatusEnabled,
		Name:          "separate-log-channel",
		UsedQuota:     987654,
		OtherSettings: `{"upstream_account_sync":{"platform":"new-api","base_url":"https://upstream.example"}}`,
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, logDB.Create(&[]Log{
		{Type: LogTypeConsume, ChannelId: channel.Id, Quota: 333},
		{Type: LogTypeConsume, ChannelId: channel.Id, Quota: 444},
	}).Error)

	require.NoError(t, migrateSyncedAccountLocalUsedQuota())

	var got Channel
	require.NoError(t, db.First(&got, channel.Id).Error)
	require.Equal(t, int64(777), got.UsedQuota)
	require.True(t, syncedAccountMigrationDone(got.OtherSettings, "local_used_quota_rebuilt"))
}
