// Package model - authz_role.go
// 该文件保存管理权限角色模板的持久化记录。
//
// 设计约束：
// - 表结构对齐 new-api 的 authz_roles，便于后续角色模板 UI 和策略导入导出继续演进。
// - 字段只使用字符串、布尔、整数和 bigint 时间戳，保证 SQLite、MySQL 和 PostgreSQL 兼容。
// - 运行时授权仍由 service/authz 校验 catalog 后消费，模型层不理解具体权限语义。
package model

import (
	"github.com/c1cada/NexusTok/common"
	"gorm.io/gorm"
)

// AuthzRole 表示一个可授权角色模板。
//
// Root/Admin 是系统内置运行时角色；自定义角色可作为系统 Admin 用户的授权基线，
// 具体权限语义仍由 service/authz 校验 catalog 与 casbin_rule 后消费。
type AuthzRole struct {
	ID          int64  `json:"id" gorm:"primaryKey"`
	Key         string `json:"key" gorm:"type:varchar(64);uniqueIndex;not null"`
	Name        string `json:"name" gorm:"type:varchar(100);not null"`
	Description string `json:"description" gorm:"type:text"`
	BuiltIn     bool   `json:"built_in" gorm:"index"`
	Enabled     bool   `json:"enabled" gorm:"index"`
	Sort        int    `json:"sort" gorm:"index"`
	CreatedAt   int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt   int64  `json:"updated_at" gorm:"bigint"`
}

// TableName 固定表名，保持与 new-api 的持久化策略表兼容。
func (AuthzRole) TableName() string {
	return "authz_roles"
}

// BeforeCreate 填充角色模板的创建和更新时间戳。
func (role *AuthzRole) BeforeCreate(_ *gorm.DB) error {
	now := common.GetTimestamp()
	if role.CreatedAt == 0 {
		role.CreatedAt = now
	}
	if role.UpdatedAt == 0 {
		role.UpdatedAt = now
	}
	return nil
}

// BeforeUpdate 刷新角色模板的更新时间戳。
func (role *AuthzRole) BeforeUpdate(_ *gorm.DB) error {
	role.UpdatedAt = common.GetTimestamp()
	return nil
}
