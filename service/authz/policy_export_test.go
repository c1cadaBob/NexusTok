package authz

import (
	"strings"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExportPersistentPoliciesReturnsStablePortableSnapshot(t *testing.T) {
	setupAuthzOverrideTestDB(t)
	require.NoError(t, SeedPersistentPolicies())
	require.NoError(t, SetUserPermissions(42, PermissionsMap{
		ResourceChannel: {
			ActionSensitiveWrite: true,
		},
	}))

	export, err := ExportPersistentPolicies()
	require.NoError(t, err)
	require.NotNil(t, export)

	assert.Equal(t, persistentPolicyExportVersion, export.Version)
	assert.NotZero(t, export.GeneratedAt)
	require.Len(t, export.Roles, 2)
	assert.Equal(t, BuiltInRoleRoot, export.Roles[0].Key)
	assert.Equal(t, BuiltInRoleAdmin, export.Roles[1].Key)
	assert.True(t, export.Roles[0].BuiltIn)
	assert.True(t, export.Roles[1].Enabled)

	assert.True(t, hasExportPolicy(export.Policies, RoleSubject(BuiltInRoleAdmin), ResourceChannel, ActionRead, EffectAllow))
	assert.True(t, hasExportPolicy(export.Policies, RoleSubject(BuiltInRoleAdmin), ResourceChannel, ActionWrite, EffectAllow))
	assert.True(t, hasExportPolicy(export.Policies, UserSubject(42), ResourceChannel, ActionSensitiveWrite, EffectAllow))
	assertPoliciesSorted(t, export.Policies)

	payload, err := common.Marshal(export)
	require.NoError(t, err)
	exportJSON := string(payload)
	assert.NotContains(t, exportJSON, `"id"`)
	assert.NotContains(t, exportJSON, `"created_at"`)
	assert.NotContains(t, exportJSON, `"updated_at"`)
}

func hasExportPolicy(policies []PersistentPolicyExportRule, subject string, resource string, action string, effect string) bool {
	for _, policy := range policies {
		if policy.Ptype == "p" &&
			policy.V0 == subject &&
			policy.V1 == resource &&
			policy.V2 == action &&
			policy.V3 == effect {
			return true
		}
	}
	return false
}

func assertPoliciesSorted(t *testing.T, policies []PersistentPolicyExportRule) {
	t.Helper()
	for i := 1; i < len(policies); i++ {
		previous := exportPolicySortKey(policies[i-1])
		current := exportPolicySortKey(policies[i])
		assert.LessOrEqual(t, previous, current)
	}
}

func exportPolicySortKey(policy PersistentPolicyExportRule) string {
	return strings.Join([]string{
		policy.Ptype,
		policy.V0,
		policy.V1,
		policy.V2,
		policy.V3,
		policy.V4,
		policy.V5,
	}, "\x00")
}
