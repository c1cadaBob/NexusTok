package authz

import (
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCatalogIncludesNexusTokCoreResources(t *testing.T) {
	catalog := Catalog()
	require.Len(t, catalog, 6)

	resources := make(map[string]ResourceDefinition, len(catalog))
	for _, resource := range catalog {
		resources[resource.Resource] = resource
		assert.NotEmpty(t, resource.LabelKey)
		assert.NotEmpty(t, resource.Actions)
	}

	for _, name := range []string{"channel", "account_pool", "user", "model", "subscription", "system_setting"} {
		_, ok := resources[name]
		assert.True(t, ok, "resource %s should be registered", name)
	}

	assert.Equal(t, "Channel Management", resources["channel"].LabelKey)
	assertActionExists(t, resources["channel"], ActionSensitiveWrite)
	assertActionExists(t, resources["account_pool"], ActionSecretView)
	assertActionExists(t, resources["system_setting"], ActionSecretView)
}

func TestRolesExposeRootAndAdminBaselines(t *testing.T) {
	roles := Roles()
	require.Len(t, roles, 2)

	root := requireRole(t, roles, BuiltInRoleRoot)
	admin := requireRole(t, roles, BuiltInRoleAdmin)

	assert.True(t, root.Superuser)
	assert.True(t, root.Grants["channel"][ActionSensitiveWrite])
	assert.True(t, root.Grants["system_setting"][ActionSecretView])

	assert.False(t, admin.Superuser)
	assert.True(t, admin.Grants["channel"][ActionRead])
	assert.True(t, admin.Grants["channel"][ActionOperate])
	assert.True(t, admin.Grants["channel"][ActionWrite])
	assert.False(t, admin.Grants["channel"][ActionSensitiveWrite])
	assert.False(t, admin.Grants["channel"][ActionSecretView])
	assert.True(t, admin.Grants["account_pool"][ActionRead])
	assert.False(t, admin.Grants["system_setting"][ActionWrite])
}

func TestCapabilitiesFollowExistingSystemRoles(t *testing.T) {
	root := Capabilities(common.RoleRootUser)
	admin := Capabilities(common.RoleAdminUser)
	user := Capabilities(common.RoleCommonUser)

	assert.True(t, root["subscription"][ActionSensitiveWrite])
	assert.True(t, admin["subscription"][ActionWrite])
	assert.False(t, admin["subscription"][ActionSensitiveWrite])
	assert.False(t, user["channel"][ActionRead])
	assert.False(t, user["system_setting"][ActionRead])
}

func assertActionExists(t *testing.T, resource ResourceDefinition, action string) {
	t.Helper()
	for _, item := range resource.Actions {
		if item.Action == action {
			return
		}
	}
	t.Fatalf("action %s should exist on resource %s", action, resource.Resource)
}

func requireRole(t *testing.T, roles []RoleDescriptor, key string) RoleDescriptor {
	t.Helper()
	for _, role := range roles {
		if role.Key == key {
			return role
		}
	}
	t.Fatalf("role %s should exist", key)
	return RoleDescriptor{}
}
