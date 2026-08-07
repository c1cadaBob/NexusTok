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
	require.NoError(t, db.AutoMigrate(&Channel{}, &ChannelAccount{}))
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
