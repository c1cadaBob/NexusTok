package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthzUserOverrideReplaceAndClear(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&AuthzUserOverride{}))
	require.NoError(t, DB.Exec("DELETE FROM authz_user_overrides").Error)
	t.Cleanup(func() {
		require.NoError(t, DB.Exec("DELETE FROM authz_user_overrides").Error)
	})

	require.NoError(t, ReplaceAuthzUserResourceOverridesInTx(DB, 42, "channel", []AuthzUserOverride{
		{UserID: 42, Resource: "channel", Action: "read", Effect: "deny"},
		{UserID: 42, Resource: "channel", Action: "sensitive_write", Effect: "allow"},
	}))

	records, err := GetAuthzUserOverrides(42)
	require.NoError(t, err)
	require.Len(t, records, 2)
	assert.Equal(t, "read", records[0].Action)
	assert.Equal(t, "sensitive_write", records[1].Action)
	assert.NotZero(t, records[0].CreatedAt)
	assert.NotZero(t, records[0].UpdatedAt)

	require.NoError(t, ReplaceAuthzUserResourceOverridesInTx(DB, 42, "channel", []AuthzUserOverride{
		{UserID: 42, Resource: "channel", Action: "operate", Effect: "deny"},
	}))

	records, err = GetAuthzUserOverrides(42)
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, "operate", records[0].Action)

	require.NoError(t, ClearAuthzUserOverridesInTx(DB, 42))
	records, err = GetAuthzUserOverrides(42)
	require.NoError(t, err)
	assert.Empty(t, records)
}
