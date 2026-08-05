package model

import (
	"testing"

	"github.com/c1cada/NexusTok/common"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSystemInstanceUpsertAndList(t *testing.T) {
	originDB := DB
	defer func() { DB = originDB }()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&SystemInstance{}))
	DB = db
	now := common.GetTimestamp()

	require.NoError(t, UpsertSystemInstance("node-a", map[string]any{
		"version": "v1",
	}, now, now+10))
	require.NoError(t, UpsertSystemInstance("node-a", map[string]any{
		"version": "v2",
	}, now, now+30))

	instances, err := ListSystemInstances()
	require.NoError(t, err)
	require.Len(t, instances, 1)
	require.Equal(t, "node-a", instances[0].NodeName)
	require.Equal(t, now+30, instances[0].LastSeenAt)

	response := instances[0].ToResponse(now + 40)
	require.Equal(t, SystemInstanceStatusOnline, response.Status)
	require.Equal(t, now, response.StartedAt)
	require.Equal(t, SystemInstanceStaleAfterSeconds, response.StaleAfterSeconds)

	info, ok := response.Info.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "v2", info["version"])
}

func TestListSystemInstancesDeletesExpiredRecords(t *testing.T) {
	originDB := DB
	defer func() { DB = originDB }()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&SystemInstance{}))
	DB = db

	now := common.GetTimestamp()
	require.NoError(t, UpsertSystemInstance(
		"node-expired",
		map[string]any{"version": "old"},
		now-SystemInstanceExpireAfterSeconds-1,
		now-SystemInstanceExpireAfterSeconds-1,
	))
	require.NoError(t, UpsertSystemInstance(
		"node-current",
		map[string]any{"version": "current"},
		now,
		now,
	))

	instances, err := ListSystemInstances()
	require.NoError(t, err)
	require.Len(t, instances, 1)
	require.Equal(t, "node-current", instances[0].NodeName)
}

func TestSystemInstanceToResponseMarksStale(t *testing.T) {
	instance := &SystemInstance{
		NodeName:   "node-stale",
		LastSeenAt: 100,
		StartedAt:  90,
	}

	response := instance.ToResponse(100 + SystemInstanceStaleAfterSeconds + 1)

	require.Equal(t, SystemInstanceStatusStale, response.Status)
	require.Equal(t, int64(100), response.LastSeenAt)
}

func TestSystemInstanceDecodeKeepsInvalidInfo(t *testing.T) {
	instance := &SystemInstance{
		NodeName: "node-invalid",
		Info:     "{invalid",
	}

	response := instance.ToResponse(common.GetTimestamp())

	require.Equal(t, "{invalid", response.Info)
}
