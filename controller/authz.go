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
