package model

import (
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAccountPoolLogDBUsesPrimaryWhenGeneralLogDBIsClickHouse(t *testing.T) {
	oldDB := DB
	oldLogDB := LOG_DB
	oldLogDatabaseType := common.LogDatabaseType()
	t.Cleanup(func() {
		DB = oldDB
		LOG_DB = oldLogDB
		common.SetLogDatabaseType(oldLogDatabaseType)
	})

	primaryDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	clickHousePlaceholderDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = primaryDB
	LOG_DB = clickHousePlaceholderDB
	common.SetLogDatabaseType(common.DatabaseTypeClickHouse)

	got, err := accountPoolLogDB()

	require.NoError(t, err)
	require.Same(t, primaryDB, got)
}

func TestAccountPoolLogDBUsesIndependentLogDBForStandardDatabases(t *testing.T) {
	oldDB := DB
	oldLogDB := LOG_DB
	oldLogDatabaseType := common.LogDatabaseType()
	t.Cleanup(func() {
		DB = oldDB
		LOG_DB = oldLogDB
		common.SetLogDatabaseType(oldLogDatabaseType)
	})

	primaryDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	logDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = primaryDB
	LOG_DB = logDB
	common.SetLogDatabaseType(common.DatabaseTypeMySQL)

	got, err := accountPoolLogDB()

	require.NoError(t, err)
	require.Same(t, logDB, got)
}
