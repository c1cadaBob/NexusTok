package model

import (
	"testing"

	"github.com/c1cadaBob/NexusTok/common"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestCloseDBHandlesUninitializedDatabases(t *testing.T) {
	previousDB := DB
	previousLogDB := LOG_DB
	t.Cleanup(func() {
		DB = previousDB
		LOG_DB = previousLogDB
	})

	DB = nil
	LOG_DB = nil

	require.NoError(t, CloseDB())
	assert.NotPanics(t, func() {
		require.NoError(t, closeDB(nil))
	})
}

func TestCloseDBClosesDistinctDatabaseHandles(t *testing.T) {
	previousDB := DB
	previousLogDB := LOG_DB
	t.Cleanup(func() {
		DB = previousDB
		LOG_DB = previousLogDB
	})

	mainDB, err := gorm.Open(sqlite.Open("file:close_db_main_"+common.GetUUID()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	logDB, err := gorm.Open(sqlite.Open("file:close_db_log_"+common.GetUUID()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	DB = mainDB
	LOG_DB = logDB

	require.NoError(t, CloseDB())
	assert.EqualError(t, mainDB.Exec("SELECT 1").Error, "sql: database is closed")
	assert.EqualError(t, logDB.Exec("SELECT 1").Error, "sql: database is closed")
}

func TestCloseDBClosesSharedDatabaseHandleOnce(t *testing.T) {
	previousDB := DB
	previousLogDB := LOG_DB
	t.Cleanup(func() {
		DB = previousDB
		LOG_DB = previousLogDB
	})

	db, err := gorm.Open(sqlite.Open("file:close_db_shared_"+common.GetUUID()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	LOG_DB = db

	require.NoError(t, CloseDB())
	assert.EqualError(t, db.Exec("SELECT 1").Error, "sql: database is closed")
}
