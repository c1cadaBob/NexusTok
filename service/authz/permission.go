// Package authz 提供 NexusTok 管理权限的只读 catalog。
//
// 当前阶段只暴露资源、动作和内置角色基线，供前端和后续路由权限表共用；
// 不在这里引入 Casbin 或改变现有 AdminAuth/RootAuth 行为，避免一次迁移影响
// 渠道、账号池、订阅等核心管理路径。
package authz

// Permission 标识某个资源上的一个动作。
type Permission struct {
	Resource string `json:"resource"`
	Action   string `json:"action"`
}

// PermissionsMap 是 resource -> action -> allowed 的权限矩阵。
type PermissionsMap map[string]map[string]bool

const (
	BuiltInRoleRoot  = "root"
	BuiltInRoleAdmin = "admin"
)

const (
	ActionRead           = "read"
	ActionOperate        = "operate"
	ActionWrite          = "write"
	ActionSensitiveWrite = "sensitive_write"
	ActionSecretView     = "secret_view"
)

// ActionDefinition 描述资源上的一个可授权动作。
type ActionDefinition struct {
	Action         string   `json:"action"`
	LabelKey       string   `json:"label_key"`
	DescriptionKey string   `json:"description_key"`
	DefaultRoles   []string `json:"-"`
}

// ResourceDefinition 描述一个管理资源及其可授权动作。
type ResourceDefinition struct {
	Resource string             `json:"resource"`
	LabelKey string             `json:"label_key"`
	Actions  []ActionDefinition `json:"actions"`
}

// RoleDescriptor 描述内置角色和它的基线授权矩阵。
type RoleDescriptor struct {
	Key       string         `json:"key"`
	Name      string         `json:"name"`
	BuiltIn   bool           `json:"built_in"`
	Superuser bool           `json:"superuser"`
	Grants    PermissionsMap `json:"grants"`
}

type roleSpec struct {
	key       string
	name      string
	builtIn   bool
	superuser bool
	sort      int
}

var builtInRoles = []roleSpec{
	{key: BuiltInRoleRoot, name: "Root", builtIn: true, superuser: true, sort: 0},
	{key: BuiltInRoleAdmin, name: "Admin", builtIn: true, superuser: false, sort: 10},
}

// Catalog 返回权限资源注册表的副本。
func Catalog() []ResourceDefinition {
	result := make([]ResourceDefinition, 0, len(registry))
	for _, resource := range registry {
		actions := make([]ActionDefinition, len(resource.Actions))
		copy(actions, resource.Actions)
		result = append(result, ResourceDefinition{
			Resource: resource.Resource,
			LabelKey: resource.LabelKey,
			Actions:  actions,
		})
	}
	return result
}

// Roles 返回内置角色及其基线授权矩阵。
func Roles() []RoleDescriptor {
	result := make([]RoleDescriptor, 0, len(builtInRoles))
	for _, role := range builtInRoles {
		result = append(result, RoleDescriptor{
			Key:       role.key,
			Name:      role.name,
			BuiltIn:   role.builtIn,
			Superuser: role.superuser,
			Grants:    roleGrants(role),
		})
	}
	return result
}

// Capabilities 按现有系统角色计算只读能力矩阵。
//
// 这个函数先服务于前端按钮显隐和后续灰度接入；完整 Authz enforcement 上线前，
// 它不参与任何服务端放行判断，避免与当前 AdminAuth/RootAuth 产生双重语义。
func Capabilities(systemRole int) PermissionsMap {
	role := roleForSystemRole(systemRole)
	if role == nil {
		return emptyGrants()
	}
	return roleGrants(*role)
}

func roleForSystemRole(systemRole int) *roleSpec {
	for i := range builtInRoles {
		role := &builtInRoles[i]
		switch {
		case systemRole >= 100 && role.key == BuiltInRoleRoot:
			return role
		case systemRole >= 10 && systemRole < 100 && role.key == BuiltInRoleAdmin:
			return role
		}
	}
	return nil
}

func roleGrants(role roleSpec) PermissionsMap {
	grants := make(PermissionsMap, len(registry))
	for _, resource := range registry {
		actions := make(map[string]bool, len(resource.Actions))
		for _, action := range resource.Actions {
			actions[action.Action] = role.superuser || actionHasRole(action, role.key)
		}
		grants[resource.Resource] = actions
	}
	return grants
}

func emptyGrants() PermissionsMap {
	grants := make(PermissionsMap, len(registry))
	for _, resource := range registry {
		actions := make(map[string]bool, len(resource.Actions))
		for _, action := range resource.Actions {
			actions[action.Action] = false
		}
		grants[resource.Resource] = actions
	}
	return grants
}

func actionHasRole(action ActionDefinition, roleKey string) bool {
	for _, r := range action.DefaultRoles {
		if r == roleKey {
			return true
		}
	}
	return false
}
