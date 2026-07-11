// Package controller - authz.go
// 该文件暴露管理权限 catalog，供前端权限编辑器和后续细粒度路由授权共用。
package controller

import (
	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/service/authz"
	"github.com/gin-gonic/gin"
)

// GetPermissionCatalog 返回权限资源、动作以及内置角色基线授权矩阵。
//
// 当前接口只读，不改变现有 AdminAuth/RootAuth 的服务端放行逻辑；它先让默认前端
// 能基于稳定 schema 做按钮显隐和权限编辑器原型，后续再逐步接入 Casbin/路由权限表。
func GetPermissionCatalog(c *gin.Context) {
	common.ApiSuccess(c, gin.H{
		"resources": authz.Catalog(),
		"roles":     authz.Roles(),
	})
}

// ExportPermissionPolicies 导出持久化权限策略快照。
//
// 该接口面向 Root 管理员的权限审计和备份场景，只读返回 authz_roles 与
// casbin_rule 中的可迁移字段，不暴露数据库自增 ID，也不触发任何策略写入。
func ExportPermissionPolicies(c *gin.Context) {
	export, err := authz.ExportPersistentPolicies()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, export)
}
