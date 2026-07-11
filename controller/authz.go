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

// ListPermissionRoles 返回持久化角色模板及当前策略矩阵。
//
// 角色矩阵会暴露管理面的完整授权边界，因此路由层必须保持 Root-only，并叠加
// system_setting.secret_view 权限分类；controller 只负责转发 service 的稳定 DTO。
func ListPermissionRoles(c *gin.Context) {
	roles, err := authz.PersistentRoles()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"roles": roles,
	})
}

// UpdatePermissionRolePolicies 校验并可选替换指定角色策略。
//
// 请求默认 dry-run；真正写库需要显式传入 `dry_run:false`。Root 角色策略在
// service 层保持只读，避免把 superuser 语义降级成普通 allow 列表。
func UpdatePermissionRolePolicies(c *gin.Context) {
	var req authz.RolePolicyUpdateRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiError(c, err)
		return
	}

	result, err := authz.UpdateRolePolicies(c.Param("key"), req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

// ImportPermissionPolicies 校验并可选导入持久化权限策略快照。
//
// 导入是高风险写操作：默认只 dry-run；真正写库必须由 Root 显式传入
// `dry_run:false`，replace 模式还需要确认字符串。service 层负责事务写入、
// 用户 override 同步和策略快照 reload。
func ImportPermissionPolicies(c *gin.Context) {
	var req authz.PersistentPolicyImportRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiError(c, err)
		return
	}

	result, err := authz.ImportPersistentPolicies(req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}
