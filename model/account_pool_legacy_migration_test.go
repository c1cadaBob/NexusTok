package model

import (
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAccountPoolLegacyMigrationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB := DB
	oldLogDB := LOG_DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&AccountPoolGroup{}, &PoolAccount{}))
	DB = db
	LOG_DB = db
	t.Cleanup(func() {
		DB = oldDB
		LOG_DB = oldLogDB
	})
	return db
}

func TestMigrateLegacyCLIProxyAccountPoolGroupsConvertsLocalAccounts(t *testing.T) {
	db := setupAccountPoolLegacyMigrationTestDB(t)
	group := &AccountPoolGroup{
		Name:        "legacy-with-local-account",
		Platform:    "openai",
		AuthType:    AccountPoolAuthTypeOfficialOAuth,
		Source:      legacyCLIProxyAccountPoolGroupSource,
		ExternalKey: "external-main",
		Status:      common.ChannelStatusEnabled,
	}
	require.NoError(t, db.Create(group).Error)
	require.NoError(t, db.Create(&PoolAccount{
		PoolGroupId: group.Id,
		Name:        "local-account",
		Platform:    "openai",
		AuthType:    AccountPoolAuthTypeOfficialOAuth,
		Status:      common.ChannelStatusEnabled,
		Schedulable: true,
	}).Error)

	require.NoError(t, migrateLegacyCLIProxyAccountPoolGroups())
	require.NoError(t, migrateLegacyCLIProxyAccountPoolGroups())

	var migrated AccountPoolGroup
	require.NoError(t, db.First(&migrated, group.Id).Error)
	require.Equal(t, AccountPoolGroupSourceNative, migrated.Source)
	require.Equal(t, "", migrated.ExternalKey)
	require.Equal(t, common.ChannelStatusEnabled, migrated.Status)
}

func TestMigrateLegacyCLIProxyAccountPoolGroupsDisablesEmptyMirrors(t *testing.T) {
	db := setupAccountPoolLegacyMigrationTestDB(t)
	group := &AccountPoolGroup{
		Name:        "legacy-empty-mirror",
		Platform:    "external",
		AuthType:    AccountPoolAuthTypeOfficialOAuth,
		Source:      legacyCLIProxyAccountPoolGroupSource,
		ExternalKey: "external-empty",
		Status:      common.ChannelStatusEnabled,
	}
	require.NoError(t, db.Create(group).Error)

	require.NoError(t, migrateLegacyCLIProxyAccountPoolGroups())
	require.NoError(t, migrateLegacyCLIProxyAccountPoolGroups())

	var migrated AccountPoolGroup
	require.NoError(t, db.First(&migrated, group.Id).Error)
	require.Equal(t, legacyCLIProxyAccountPoolGroupSource, migrated.Source)
	require.Equal(t, "external-empty", migrated.ExternalKey)
	require.Equal(t, common.ChannelStatusManuallyDisabled, migrated.Status)
}
