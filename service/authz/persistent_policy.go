package authz

import (
	"errors"
	"strconv"

	"github.com/c1cada/NexusTok/model"
	"gorm.io/gorm"
)

// RoleSubject 返回角色在策略表中的 subject 名称。
//
// 与 new-api 保持 `role:<key>` 约定，后续接入 Casbin adapter、自定义角色或导入
// 导出工具时，可以直接复用同一张 `casbin_rule` 表。
func RoleSubject(roleKey string) string {
	return "role:" + roleKey
}

// UserSubject 返回用户在策略表中的 subject 名称。
//
// 本轮用户级 override 仍保留在 `authz_user_overrides` 中；该函数先作为兼容命名
// 暴露，避免后续迁移用户策略时再引入另一套 subject 规则。
func UserSubject(userID int) string {
	return "user:" + strconv.Itoa(userID)
}

// SeedPersistentPolicies 初始化内置角色模板和 Admin 默认策略。
//
// 该函数只写 Root/Admin 内置角色及其默认策略。Root 是 superuser，不需要写全量
// allow 规则；Admin 策略来自 catalog 中的 DefaultRoles。调用方应只在主节点执行，
// 避免多节点同时重置内置策略。
func SeedPersistentPolicies() error {
	if model.DB == nil {
		return errAuthzDatabaseNotInitialized
	}

	return model.DB.Transaction(func(tx *gorm.DB) error {
		if err := seedBuiltInRoles(tx); err != nil {
			return err
		}
		if err := resetBuiltInRolePolicies(tx); err != nil {
			return err
		}
		return seedDefaultRolePolicies(tx)
	})
}

func seedBuiltInRoles(tx *gorm.DB) error {
	for _, spec := range builtInRoles {
		nextRole := model.AuthzRole{
			Key:         spec.key,
			Name:        spec.name,
			Description: spec.description,
			BuiltIn:     spec.builtIn,
			Enabled:     true,
			Sort:        spec.sort,
		}

		var existing model.AuthzRole
		err := tx.Where("key = ?", spec.key).First(&existing).Error
		switch {
		case err == nil:
			if err := tx.Model(&existing).Updates(map[string]interface{}{
				"name":        nextRole.Name,
				"description": nextRole.Description,
				"built_in":    nextRole.BuiltIn,
				"enabled":     nextRole.Enabled,
				"sort":        nextRole.Sort,
			}).Error; err != nil {
				return err
			}
		case errors.Is(err, gorm.ErrRecordNotFound):
			if err := tx.Create(&nextRole).Error; err != nil {
				return err
			}
		default:
			return err
		}
	}
	return nil
}

func resetBuiltInRolePolicies(tx *gorm.DB) error {
	subjects := make([]string, 0, len(builtInRoles))
	for _, spec := range builtInRoles {
		subjects = append(subjects, RoleSubject(spec.key))
	}
	return tx.Where("ptype = ? AND v0 IN ?", "p", subjects).
		Delete(&model.CasbinRule{}).Error
}

func seedDefaultRolePolicies(tx *gorm.DB) error {
	rules := make([]model.CasbinRule, 0)
	for _, spec := range builtInRoles {
		if spec.superuser {
			continue
		}
		subject := RoleSubject(spec.key)
		for _, resource := range registry {
			for _, action := range resource.Actions {
				if !actionHasRole(action, spec.key) {
					continue
				}
				rules = append(rules, model.CasbinRule{
					Ptype: "p",
					V0:    subject,
					V1:    resource.Resource,
					V2:    action.Action,
					V3:    EffectAllow,
				})
			}
		}
	}
	if len(rules) == 0 {
		return nil
	}
	return tx.Create(&rules).Error
}

func persistentRoleAllows(role roleSpec, permission Permission) (bool, bool) {
	grants, ok := persistentRoleGrants(role)
	if !ok {
		return false, false
	}
	actions, ok := grants[permission.Resource]
	if !ok {
		return false, true
	}
	allowed, ok := actions[permission.Action]
	if !ok {
		return false, true
	}
	return allowed, true
}

func persistentRoleGrants(role roleSpec) (PermissionsMap, bool) {
	if role.superuser || model.DB == nil {
		return nil, false
	}

	var records []model.CasbinRule
	err := model.DB.Where("ptype = ? AND v0 = ?", "p", RoleSubject(role.key)).
		Order("v1 ASC, v2 ASC").
		Find(&records).Error
	if err != nil || len(records) == 0 {
		return nil, false
	}

	grants := emptyGrants()
	validAllowCount := 0
	for _, record := range records {
		if record.V3 != "" && record.V3 != EffectAllow {
			continue
		}
		permission := Permission{Resource: record.V1, Action: record.V2}
		if !isKnownPermission(permission) {
			continue
		}
		grants[record.V1][record.V2] = true
		validAllowCount++
	}

	// 如果策略表存在但该角色没有任何可用 allow 策略，视为策略缺失并回退静态
	// 基线。这样迁移中断、表被清空或只残留未知策略时不会把 Admin 瞬间降为全拒绝。
	if validAllowCount == 0 {
		return nil, false
	}
	return grants, true
}

func persistentAdminBaseline(resource string, action string) (bool, bool) {
	adminRole, ok := builtInRoleByKey(BuiltInRoleAdmin)
	if !ok {
		return false, false
	}
	grants, ok := persistentRoleGrants(*adminRole)
	if !ok {
		return false, false
	}
	actions, ok := grants[resource]
	if !ok {
		return false, true
	}
	allowed, ok := actions[action]
	if !ok {
		return false, true
	}
	return allowed, true
}
