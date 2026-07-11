package authz

import (
	"errors"
	"fmt"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"gorm.io/gorm"
)

var errAuthzRoleAssigned = errors.New("authz role is assigned to users")

// AssignUserRoleInTx 在调用方事务内保存用户的授权角色模板。
//
// 空字符串和内置 Admin 都表示沿用系统 Admin 基线；自定义角色必须存在且启用。
// Root 角色永远不能分配给普通管理员，避免通过模板字段绕过系统角色边界。
func AssignUserRoleInTx(tx *gorm.DB, userID int, roleKey string) error {
	if userID <= 0 {
		return errAuthzUserIDRequired
	}
	if tx == nil {
		tx = model.DB
	}
	if tx == nil {
		return errAuthzDatabaseNotInitialized
	}
	normalized, err := normalizeAssignableRoleKeyInTx(tx, roleKey)
	if err != nil {
		return err
	}
	return tx.Model(&model.User{}).
		Where("id = ?", userID).
		Update("authz_role", normalized).Error
}

// NormalizeAssignableRoleKey 校验并规范化可分配给管理员用户的角色 key。
func NormalizeAssignableRoleKey(roleKey string) (string, error) {
	return normalizeAssignableRoleKeyInTx(model.DB, roleKey)
}

func normalizeAssignableRoleKeyInTx(tx *gorm.DB, roleKey string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(roleKey))
	if normalized == "" || normalized == BuiltInRoleAdmin {
		return "", nil
	}
	if normalized == BuiltInRoleRoot ||
		strings.HasPrefix(normalized, "role:") ||
		strings.HasPrefix(normalized, "user:") {
		return "", errAuthzRoleKeyReserved
	}
	if tx == nil {
		return "", errAuthzDatabaseNotInitialized
	}
	role, err := findPersistentRoleInTx(tx, normalized)
	if err != nil {
		return "", err
	}
	if role.BuiltIn {
		return "", errAuthzRoleBuiltInReadOnly
	}
	if !role.Enabled {
		return "", errAuthzRoleDisabled
	}
	return role.Key, nil
}

func findPersistentRoleInTx(tx *gorm.DB, roleKey string) (model.AuthzRole, error) {
	if tx == nil {
		tx = model.DB
	}
	if tx == nil {
		return model.AuthzRole{}, errAuthzDatabaseNotInitialized
	}
	var role model.AuthzRole
	err := tx.Where("key = ?", roleKey).First(&role).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.AuthzRole{}, errAuthzRoleNotFound
	}
	return role, err
}

func effectiveRoleForUser(userID int, systemRole int) *roleSpec {
	if userID > 0 && model.DB != nil {
		assignedRoleKey, actualSystemRole, ok := assignedRoleKeyForUser(userID)
		if !ok {
			return nil
		}
		if actualSystemRole < systemRole {
			systemRole = actualSystemRole
		}
		return effectiveRoleForAssignedKey(systemRole, assignedRoleKey)
	}
	return effectiveRoleForAssignedKey(systemRole, "")
}

func effectiveRoleForAssignedKey(systemRole int, assignedRoleKey string) *roleSpec {
	baseRole := roleForSystemRole(systemRole)
	if baseRole == nil || baseRole.superuser || baseRole.key != BuiltInRoleAdmin {
		return baseRole
	}
	if assignedRoleKey == "" || assignedRoleKey == BuiltInRoleAdmin {
		return baseRole
	}
	role, ok := runtimeAssignedRole(assignedRoleKey)
	if ok {
		return role
	}
	// 如果角色被手工删除、禁用或数据库暂时不可读，保留用户的系统 Admin 身份，
	// 但授权基线失败关闭，避免意外回退为完整 Admin 权限。
	return nil
}

func assignedRoleKeyForUser(userID int) (string, int, bool) {
	if userID <= 0 {
		return "", 0, true
	}
	if model.DB == nil {
		// 权限 catalog 的纯函数测试、早期启动探测和离线工具可能没有初始化 DB；
		// 这些路径无法存在用户级角色分配，按历史系统角色基线继续计算。
		return "", 0, true
	}
	var user model.User
	err := model.DB.Select("role", "authz_role").Where("id = ?", userID).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", 0, false
	}
	if err != nil {
		common.SysLog(fmt.Sprintf("authz: failed to load assigned role for user %d: %v", userID, err))
		return "", 0, false
	}
	return strings.ToLower(strings.TrimSpace(user.AuthzRole)), user.Role, true
}

func runtimeAssignedRole(roleKey string) (*roleSpec, bool) {
	roleKey = strings.ToLower(strings.TrimSpace(roleKey))
	if roleKey == "" || roleKey == BuiltInRoleAdmin {
		role, ok := builtInRoleByKey(BuiltInRoleAdmin)
		return role, ok
	}
	if roleKey == BuiltInRoleRoot || model.DB == nil {
		return nil, false
	}
	role, err := findPersistentRole(roleKey)
	if err != nil || !role.Enabled || role.BuiltIn {
		if err != nil {
			common.SysLog(fmt.Sprintf("authz: assigned role %q is unavailable: %v", roleKey, err))
		}
		return nil, false
	}
	return &roleSpec{
		key:         role.Key,
		name:        role.Name,
		description: role.Description,
		builtIn:     false,
		superuser:   false,
		sort:        role.Sort,
	}, true
}

func userPermissionBaselineResolverInTx(tx *gorm.DB, userID int) func(resource string, action string) (bool, bool) {
	assignedRoleKey, ok := assignedRoleKeyForUserInTx(tx, userID)
	if !ok {
		return func(_ string, _ string) (bool, bool) {
			return false, true
		}
	}
	if assignedRoleKey == "" || assignedRoleKey == BuiltInRoleAdmin {
		return func(resource string, action string) (bool, bool) {
			return persistentAdminBaselineInTx(tx, resource, action)
		}
	}
	role, ok := runtimeAssignedRoleInTx(tx, assignedRoleKey)
	if !ok {
		return func(_ string, _ string) (bool, bool) {
			return false, true
		}
	}
	grants, ok, err := loadPersistentRoleGrantsFromDBWithConn(tx, *role)
	if err != nil {
		common.SysLog(fmt.Sprintf("authz: failed to load grants for assigned role %q: %v", assignedRoleKey, err))
		return func(_ string, _ string) (bool, bool) {
			return false, true
		}
	}
	if !ok {
		return func(_ string, _ string) (bool, bool) {
			return false, true
		}
	}
	return func(resource string, action string) (bool, bool) {
		return grantsAllow(grants, resource, action)
	}
}

func assignedRoleKeyForUserInTx(tx *gorm.DB, userID int) (string, bool) {
	if userID <= 0 {
		return "", true
	}
	if tx == nil {
		tx = model.DB
	}
	if tx == nil {
		return "", false
	}
	var user model.User
	err := tx.Select("authz_role").Where("id = ?", userID).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// 兼容历史单元测试直接保存用户 override 的场景：没有用户行时沿用 Admin 基线。
		return "", true
	}
	if err != nil {
		common.SysLog(fmt.Sprintf("authz: failed to load assigned role for user %d in transaction: %v", userID, err))
		return "", false
	}
	return strings.ToLower(strings.TrimSpace(user.AuthzRole)), true
}

func runtimeAssignedRoleInTx(tx *gorm.DB, roleKey string) (*roleSpec, bool) {
	roleKey = strings.ToLower(strings.TrimSpace(roleKey))
	if roleKey == "" || roleKey == BuiltInRoleAdmin {
		role, ok := builtInRoleByKey(BuiltInRoleAdmin)
		return role, ok
	}
	if roleKey == BuiltInRoleRoot {
		return nil, false
	}
	role, err := findPersistentRoleInTx(tx, roleKey)
	if err != nil || !role.Enabled || role.BuiltIn {
		return nil, false
	}
	return &roleSpec{
		key:         role.Key,
		name:        role.Name,
		description: role.Description,
		builtIn:     false,
		superuser:   false,
		sort:        role.Sort,
	}, true
}

// ensureRoleUnassignedInTx 确认自定义授权角色当前没有被任何用户引用。
//
// 删除、禁用或导入替换角色都会改变已分配管理员的授权基线，因此必须在同一事务内
// 检查 users.authz_role。调用方传入内置 Admin/Root 时不做处理，因为它们不是可分配
// 的自定义模板；自定义角色一旦存在引用就返回稳定错误，交由 controller/API 展示。
func ensureRoleUnassignedInTx(tx *gorm.DB, roleKey string) error {
	roleKey = strings.ToLower(strings.TrimSpace(roleKey))
	if roleKey == "" || roleKey == BuiltInRoleAdmin || roleKey == BuiltInRoleRoot {
		return nil
	}
	if tx == nil {
		tx = model.DB
	}
	if tx == nil {
		return errAuthzDatabaseNotInitialized
	}
	var assignedCount int64
	if err := tx.Model(&model.User{}).
		Where("authz_role = ?", roleKey).
		Count(&assignedCount).Error; err != nil {
		return err
	}
	if assignedCount > 0 {
		return errAuthzRoleAssigned
	}
	return nil
}
