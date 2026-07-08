// Package controller - user_authz.go
// 该文件承载用户管理复合动作的权限分类。
//
// `/api/user/manage` 兼容历史请求体，单一路由会根据 action 执行启停、删除、
// 升降级和额度调整等不同动作。路由级权限只能先按 user.operate 放行，真正会改变
// 账号生命周期、管理员身份或钱包额度的动作必须在 handler 内额外校验 user.sensitive_write。
package controller

import (
	"net/http"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/i18n"
	"github.com/c1cada/NexusTok/service/authz"

	"github.com/gin-gonic/gin"
)

// userManageActionNeedsSensitiveWrite 判断 ManageUser 请求是否触碰高风险用户资产。
func userManageActionNeedsSensitiveWrite(req ManageRequest) bool {
	switch req.Action {
	case "delete", "promote", "demote", "add_quota":
		return true
	default:
		return false
	}
}

// userCanSensitiveWrite 判断当前操作者是否拥有用户敏感写权限。
func userCanSensitiveWrite(c *gin.Context) bool {
	return authz.Can(c.GetInt("id"), c.GetInt("role"), authz.UserSensitiveWrite)
}

// requireUserSensitiveWrite 在复合 handler 内执行用户敏感写二次校验。
func requireUserSensitiveWrite(c *gin.Context) bool {
	if userCanSensitiveWrite(c) {
		return true
	}
	c.JSON(http.StatusForbidden, gin.H{
		"success": false,
		"message": common.TranslateMessage(c, i18n.MsgAuthInsufficientPrivilege),
	})
	c.Abort()
	return false
}
