package authz

import (
	"errors"
	"fmt"
	"sort"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"gorm.io/gorm"
)

var (
	errAuthzDatabaseNotInitialized = errors.New("authz database is not initialized")
	errAuthzUserIDRequired         = errors.New("authz user id is required")
)

// overrideEffects 保存已通过 catalog 校验的用户级覆盖结果。
type overrideEffects map[string]map[string]bool

// SetUserPermissions 保存指定用户的管理权限矩阵。
//
// 该函数只持久化与 Admin 基线不同的动作，避免把角色默认授权重复写入数据库。调用方
// 可以传入部分资源矩阵，函数只替换这些资源的 override，不影响其它资源的已有配置。
func SetUserPermissions(userID int, permissions PermissionsMap) error {
	if model.DB == nil {
		return errAuthzDatabaseNotInitialized
	}
	return model.DB.Transaction(func(tx *gorm.DB) error {
		return SetUserPermissionsInTx(tx, userID, permissions)
	})
}

// SetUserPermissionsInTx 在调用方事务内保存指定用户的权限矩阵。
func SetUserPermissionsInTx(tx *gorm.DB, userID int, permissions PermissionsMap) error {
	if userID <= 0 {
		return errAuthzUserIDRequired
	}
	if tx == nil {
		tx = model.DB
	}
	if tx == nil {
		return errAuthzDatabaseNotInitialized
	}

	for _, resource := range registry {
		actions, ok := permissions[resource.Resource]
		if !ok {
			continue
		}
		overrides := userOverrideRecords(userID, resource, actions)
		if err := model.ReplaceAuthzUserResourceOverridesInTx(tx, userID, resource.Resource, overrides); err != nil {
			return err
		}
	}
	return nil
}

// ClearUserPermissions 删除指定用户的全部管理权限 override。
func ClearUserPermissions(userID int) error {
	if model.DB == nil {
		return errAuthzDatabaseNotInitialized
	}
	return model.DB.Transaction(func(tx *gorm.DB) error {
		return ClearUserPermissionsInTx(tx, userID)
	})
}

// ClearUserPermissionsInTx 在调用方事务内删除指定用户的全部管理权限 override。
func ClearUserPermissionsInTx(tx *gorm.DB, userID int) error {
	if userID <= 0 {
		return errAuthzUserIDRequired
	}
	if tx == nil {
		tx = model.DB
	}
	if tx == nil {
		return errAuthzDatabaseNotInitialized
	}
	return model.ClearAuthzUserOverridesInTx(tx, userID)
}

// ClearUserAuthorization 是用户生命周期代码使用的语义化别名。
func ClearUserAuthorization(userID int) error {
	return ClearUserPermissions(userID)
}

// ClearUserAuthorizationInTx 是用户生命周期事务中使用的语义化别名。
func ClearUserAuthorizationInTx(tx *gorm.DB, userID int) error {
	return ClearUserPermissionsInTx(tx, userID)
}

// ExplicitUserPermissions 返回 Admin 基线叠加用户级 override 后的矩阵。
func ExplicitUserPermissions(userID int) PermissionsMap {
	return CapabilitiesForUser(userID, common.RoleAdminUser)
}

// ExplicitUserOverrides 只返回数据库中保存的显式 override。
func ExplicitUserOverrides(userID int) PermissionsMap {
	effects, err := loadUserOverrideEffects(userID)
	if err != nil {
		common.SysLog(fmt.Sprintf("authz: failed to load explicit user overrides for user %d: %v", userID, err))
		return PermissionsMap{}
	}
	result := make(PermissionsMap, len(effects))
	for resource, actions := range effects {
		result[resource] = make(map[string]bool, len(actions))
		for action, allowed := range actions {
			result[resource][action] = allowed
		}
	}
	return result
}

func userOverrideRecords(userID int, resource ResourceDefinition, actions map[string]bool) []model.AuthzUserOverride {
	records := make([]model.AuthzUserOverride, 0, len(actions))
	for _, action := range resource.Actions {
		desired, ok := actions[action.Action]
		if !ok {
			continue
		}
		adminBaseline := actionHasRole(action, BuiltInRoleAdmin)
		if allowed, ok := persistentAdminBaseline(resource.Resource, action.Action); ok {
			adminBaseline = allowed
		}
		if desired == adminBaseline {
			continue
		}
		effect := EffectDeny
		if desired {
			effect = EffectAllow
		}
		records = append(records, model.AuthzUserOverride{
			UserID:   userID,
			Resource: resource.Resource,
			Action:   action.Action,
			Effect:   effect,
		})
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].Action < records[j].Action
	})
	return records
}

func explicitUserOverride(userID int, permission Permission) (bool, bool) {
	return overrideAllows(safeUserOverrideEffects(userID), permission)
}

func overrideAllows(overrides overrideEffects, permission Permission) (bool, bool) {
	if len(overrides) == 0 {
		return false, false
	}
	actions, ok := overrides[permission.Resource]
	if !ok {
		return false, false
	}
	allowed, ok := actions[permission.Action]
	return allowed, ok
}

func safeUserOverrideEffects(userID int) overrideEffects {
	effects, err := loadUserOverrideEffects(userID)
	if err != nil {
		common.SysLog(fmt.Sprintf("authz: failed to load user overrides for user %d: %v", userID, err))
		return nil
	}
	return effects
}

func loadUserOverrideEffects(userID int) (overrideEffects, error) {
	if userID <= 0 || model.DB == nil {
		return nil, nil
	}
	records, err := model.GetAuthzUserOverrides(userID)
	if err != nil {
		return nil, err
	}
	effects := make(overrideEffects)
	for _, record := range records {
		permission := Permission{Resource: record.Resource, Action: record.Action}
		if !isKnownPermission(permission) {
			continue
		}
		var allowed bool
		switch record.Effect {
		case EffectAllow:
			allowed = true
		case EffectDeny:
			allowed = false
		default:
			continue
		}
		if effects[record.Resource] == nil {
			effects[record.Resource] = make(map[string]bool)
		}
		effects[record.Resource][record.Action] = allowed
	}
	return effects, nil
}

func isKnownPermission(permission Permission) bool {
	_, ok := findAction(permission)
	return ok
}
