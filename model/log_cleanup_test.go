package model

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupLogCleanupTestDB(t *testing.T) {
	t.Helper()
	oldDB := DB
	oldLogDB := LOG_DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Log{}))
	DB = db
	LOG_DB = db
	t.Cleanup(func() {
		DB = oldDB
		LOG_DB = oldLogDB
	})
}

func TestCountAndDeleteOldLogBatch(t *testing.T) {
	setupLogCleanupTestDB(t)
	ctx := context.Background()

	require.NoError(t, LOG_DB.Create(&[]Log{
		{CreatedAt: 90, Type: LogTypeManage, Content: "old-a"},
		{CreatedAt: 95, Type: LogTypeManage, Content: "old-b"},
		{CreatedAt: 105, Type: LogTypeManage, Content: "new"},
	}).Error)

	count, err := CountOldLog(ctx, 100)
	require.NoError(t, err)
	require.Equal(t, int64(2), count)

	deleted, err := DeleteOldLogBatch(ctx, 100, 1)
	require.NoError(t, err)
	require.Equal(t, int64(1), deleted)

	count, err = CountOldLog(ctx, 100)
	require.NoError(t, err)
	require.Equal(t, int64(1), count)

	deleted, err = DeleteOldLog(ctx, 100, 1)
	require.NoError(t, err)
	require.Equal(t, int64(1), deleted)

	var remaining []Log
	require.NoError(t, LOG_DB.Order("created_at asc").Find(&remaining).Error)
	require.Len(t, remaining, 1)
	require.Equal(t, int64(105), remaining[0].CreatedAt)
}
