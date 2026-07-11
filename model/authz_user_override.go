// Package model - authz_user_override.go
// 该文件保存管理权限的用户级 override 记录，并同步维护 Casbin 兼容策略镜像。
//
// 设计约束：
//   - 表结构只使用整数、短字符串和 bigint 时间戳，确保 SQLite、MySQL 和 PostgreSQL 兼容。
//   - resource/action/effect 保持为普通字符串，不使用 JSON/JSONB，避免数据库方言差异。
//   - 业务层负责校验 resource/action 是否属于 authz catalog，以及 effect 是否为 allow/deny。
//   - casbin_rule 中的 user:<id> policy 是兼容镜像，便于后续完整 Casbin runtime、
//     策略导入导出和角色编辑器复用同一张策略表。
package model

import (
	"errors"
	"strconv"

	"github.com/c1cada/NexusTok/common"
	"gorm.io/gorm"
)

var errAuthzUserOverrideResourceRequired = errors.New("authz user override resource is required")

// AuthzUserOverride 表示某个用户在某个管理资源动作上的显式 allow/deny 覆盖。
//
// 唯一索引保证同一用户、资源、动作最多只有一条 override。该模型不保存“角色基线”，
// 只保存与 Admin 基线不同的显式差异，便于后续 UI、审计和导入导出保持语义清晰。
type AuthzUserOverride struct {
	ID        int64  `json:"id" gorm:"primaryKey"`
	UserID    int    `json:"user_id" gorm:"uniqueIndex:idx_authz_user_override_unique,priority:1;index"`
	Resource  string `json:"resource" gorm:"type:varchar(64);uniqueIndex:idx_authz_user_override_unique,priority:2;index"`
	Action    string `json:"action" gorm:"type:varchar(64);uniqueIndex:idx_authz_user_override_unique,priority:3;index"`
	Effect    string `json:"effect" gorm:"type:varchar(16);index"`
	CreatedAt int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt int64  `json:"updated_at" gorm:"bigint"`
}

// BeforeCreate 填充 override 的创建和更新时间戳。
func (override *AuthzUserOverride) BeforeCreate(_ *gorm.DB) error {
	now := common.GetTimestamp()
	if override.CreatedAt == 0 {
		override.CreatedAt = now
	}
	if override.UpdatedAt == 0 {
		override.UpdatedAt = now
	}
	return nil
}

// BeforeUpdate 刷新 override 的更新时间戳。
func (override *AuthzUserOverride) BeforeUpdate(_ *gorm.DB) error {
	override.UpdatedAt = common.GetTimestamp()
	return nil
}

// GetAuthzUserOverrides 读取指定用户的全部权限 override。
func GetAuthzUserOverrides(userID int) ([]AuthzUserOverride, error) {
	if DB == nil || userID <= 0 {
		return nil, nil
	}
	var overrides []AuthzUserOverride
	err := DB.Where("user_id = ?", userID).
		Order("resource ASC, action ASC").
		Find(&overrides).Error
	return overrides, err
}

// ReplaceAuthzUserResourceOverridesInTx 在事务内替换指定用户某个资源的 override。
//
// 调用方传入的 overrides 必须已经完成权限 catalog 与 effect 校验。本函数只负责数据库
// 原子替换：先删除该资源旧记录，再同步替换 casbin_rule 镜像，最后写入新记录；
// 如果任一步失败，事务回滚后旧记录不会丢失。
func ReplaceAuthzUserResourceOverridesInTx(tx *gorm.DB, userID int, resource string, overrides []AuthzUserOverride) error {
	if tx == nil {
		tx = DB
	}
	if tx == nil || userID <= 0 {
		return nil
	}
	if resource == "" {
		return errAuthzUserOverrideResourceRequired
	}
	if err := tx.Where("user_id = ? AND resource = ?", userID, resource).
		Delete(&AuthzUserOverride{}).Error; err != nil {
		return err
	}
	if err := replaceAuthzUserCasbinPoliciesInTx(tx, userID, resource, overrides); err != nil {
		return err
	}
	if len(overrides) == 0 {
		return nil
	}
	return tx.Create(&overrides).Error
}

// ClearAuthzUserOverridesInTx 在事务内删除指定用户的全部权限 override。
func ClearAuthzUserOverridesInTx(tx *gorm.DB, userID int) error {
	if tx == nil {
		tx = DB
	}
	if tx == nil || userID <= 0 {
		return nil
	}
	if err := tx.Where("user_id = ?", userID).Delete(&AuthzUserOverride{}).Error; err != nil {
		return err
	}
	return clearAuthzUserCasbinPoliciesInTx(tx, userID)
}

func replaceAuthzUserCasbinPoliciesInTx(tx *gorm.DB, userID int, resource string, overrides []AuthzUserOverride) error {
	// 用户级策略在 Casbin 兼容表中使用 p, user:<id>, <resource>, <action>, <effect>。
	// 当前授权读取仍以 AuthzUserOverride 为准，这里的镜像只负责让后续 enforcer
	// 或导入导出能力可以无缝消费同一份策略数据。
	if err := tx.Where("ptype = ? AND v0 = ? AND v1 = ?", "p", authzUserPolicySubject(userID), resource).
		Delete(&CasbinRule{}).Error; err != nil {
		return err
	}
	if len(overrides) == 0 {
		return nil
	}
	rules := make([]CasbinRule, 0, len(overrides))
	for _, override := range overrides {
		rules = append(rules, CasbinRule{
			Ptype: "p",
			V0:    authzUserPolicySubject(userID),
			V1:    override.Resource,
			V2:    override.Action,
			V3:    override.Effect,
		})
	}
	return tx.Create(&rules).Error
}

func clearAuthzUserCasbinPoliciesInTx(tx *gorm.DB, userID int) error {
	return tx.Where("ptype = ? AND v0 = ?", "p", authzUserPolicySubject(userID)).
		Delete(&CasbinRule{}).Error
}

func authzUserPolicySubject(userID int) string {
	return "user:" + strconv.Itoa(userID)
}
