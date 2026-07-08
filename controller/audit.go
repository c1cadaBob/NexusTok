// Package controller - audit.go
// 该文件提供 controller 内可复用的管理审计辅助函数。
//
// 全局兜底审计由 middleware/audit.go 自动完成；当某个 handler 能拿到更准确的业务语义
// （例如目标用户、资源名称、变更前后状态）时，可以调用这里的 helper 写入更精细的审计日志，
// 并标记当前请求已审计，避免兜底中间件重复记录。
package controller

import (
	"fmt"
	"os"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"

	"github.com/gin-gonic/gin"
)

// auditContentTemplates 将稳定 action 映射为英文兜底模板。
// 数据库存储仍以 action+params 为主；Content 只服务导出、旧前端或解析失败时的人类可读兜底。
var auditContentTemplates = map[string]string{
	"user.create":        "Created user ${username}",
	"user.update":        "Updated user ${username} (ID: ${id})",
	"user.delete":        "Deleted user ${username} (ID: ${id})",
	"user.manage":        "Managed user ${username} (ID: ${id})",
	"user.binding_clear": "Cleared ${binding_type} binding for user ${username}",
	"user.2fa_disable":   "Force-disabled two-factor authentication for user ${target_user_id}",
	"option.update":      "Updated system setting ${key}",
	"channel.create":     "Created channel ${name}",
	"channel.update":     "Updated channel ${name} (ID: ${id})",
	"channel.delete":     "Deleted channel ${name} (ID: ${id})",
	"channel.key_view":   "Viewed channel key (ID: ${id})",
	"redemption.create":  "Created redemption codes ${name}",
	"redemption.update":  "Updated redemption code ${id}",
	"redemption.delete":  "Deleted redemption code ${id}",
	"subscription.bind":  "Bound subscription for user ${target_user_id}",
}

// auditContentEN 根据 action 模板渲染英文兜底内容；未登记 action 时返回 action 本身。
func auditContentEN(action string, params map[string]interface{}) string {
	tmpl, ok := auditContentTemplates[action]
	if !ok {
		return action
	}
	return os.Expand(tmpl, func(key string) string {
		if value, exists := params[key]; exists {
			return fmt.Sprintf("%v", value)
		}
		return ""
	})
}

// auditOperatorInfo 从 Gin 上下文中提取操作者身份。
// 该结构写入 Other.admin_info，普通用户日志视图会整体剥离。
func auditOperatorInfo(c *gin.Context) map[string]interface{} {
	return map[string]interface{}{
		"admin_id":       c.GetInt("id"),
		"admin_username": c.GetString("username"),
		"admin_role":     c.GetInt("role"),
		"auth_method":    auditAuthMethod(c),
	}
}

func auditAuthMethod(c *gin.Context) string {
	if c.GetBool("use_access_token") {
		return "access_token"
	}
	return "session"
}

// markAuditLogged 标记当前请求已经写入手动审计日志。
func markAuditLogged(c *gin.Context) {
	common.SetContextKey(c, constant.ContextKeyAuditLogged, true)
}

// recordManageAudit 记录归属于当前操作者的管理操作审计。
func recordManageAudit(c *gin.Context, action string, params map[string]interface{}) {
	recordManageAuditFor(c, c.GetInt("id"), action, params)
}

// recordManageAuditFor 记录管理操作审计，并在 params 中补充目标用户 ID。
// 日志归属仍然是实际操作者；被操作用户或资源只作为结构化参数保存。
func recordManageAuditFor(c *gin.Context, targetUserId int, action string, params map[string]interface{}) {
	if params == nil {
		params = map[string]interface{}{}
	}
	operatorUserId := c.GetInt("id")
	if _, exists := params["target_user_id"]; !exists && targetUserId > 0 && targetUserId != operatorUserId {
		params["target_user_id"] = targetUserId
	}
	model.RecordOperationAuditLog(model.OperationAuditLogParams{
		UserId:    operatorUserId,
		Content:   auditContentEN(action, params),
		Ip:        c.ClientIP(),
		Action:    action,
		Params:    params,
		AdminInfo: auditOperatorInfo(c),
	})
	markAuditLogged(c)
}

// recordUserSecurityAudit 记录普通用户自己的安全敏感操作。
// 这类日志没有管理员操作者，因此不写 admin_info，也不依赖 AdminAuth/RootAuth 兜底。
func recordUserSecurityAudit(c *gin.Context, userId int, action string, params map[string]interface{}) {
	if params == nil {
		params = map[string]interface{}{}
	}
	model.RecordOperationAuditLog(model.OperationAuditLogParams{
		UserId:  userId,
		Content: auditContentEN(action, params),
		Ip:      c.ClientIP(),
		Action:  action,
		Params:  params,
	})
}
