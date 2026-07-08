// Package router - account_pool-router.go
// 该文件集中注册 NexusTok 原生账号池管理路由。
//
// 账号池承载真实上游凭证和账号生命周期。路由先经过 AdminAuth，再按
// account_pool 资源的 read/operate/write/sensitive_write 权限做二次校验；
// 当前 authz 基线仍是 Root/Admin 角色，后续接入 Casbin 时只需替换 authz.Can。
package router

import (
	"net/http"

	"github.com/c1cada/NexusTok/controller"
	"github.com/c1cada/NexusTok/middleware"
	"github.com/c1cada/NexusTok/service/authz"

	"github.com/gin-gonic/gin"
)

// registerAccountPoolRoutes 注册 /api/account-pool 路由组。
func registerAccountPoolRoutes(apiRouter *gin.RouterGroup) {
	accountPoolRoute := apiRouter.Group("/account-pool")
	accountPoolRoute.Use(middleware.AdminAuth())
	registerPermissionRoutes(accountPoolRoute, accountPoolPermissionRoutes)
}

var accountPoolPermissionRoutes = []permissionRoute{
	// 提供商、健康、日志和脱敏审计查询
	{method: http.MethodGet, path: "/providers", permission: authz.AccountPoolRead, handler: controller.ListAccountPoolProviders},
	{method: http.MethodGet, path: "/health", permission: authz.AccountPoolRead, handler: controller.GetAccountPoolHealth},
	{method: http.MethodGet, path: "/usage-logs", permission: authz.AccountPoolRead, handler: controller.ListAccountPoolUsageLogs},
	{method: http.MethodGet, path: "/state-logs/audit-summary", permission: authz.AccountPoolRead, handler: controller.GetAccountPoolStateLogAuditSummary},
	{method: http.MethodGet, path: "/state-logs", permission: authz.AccountPoolRead, handler: controller.ListAccountPoolStateLogs},
	{method: http.MethodGet, path: "/state-logs/export", permission: authz.AccountPoolOperate, handler: controller.ExportAccountPoolStateLogs},
	{method: http.MethodGet, path: "/check-tasks", permission: authz.AccountPoolRead, handler: controller.ListPoolAccountCheckTasks},
	{method: http.MethodPost, path: "/check-tasks/cleanup", permission: authz.AccountPoolOperate, handler: controller.CleanupPoolAccountCheckTasks},
	{method: http.MethodGet, path: "/check-tasks/:check_task_id", permission: authz.AccountPoolRead, handler: controller.GetPoolAccountCheckTask},

	// 认证文件管理。创建、导入、更新和删除都会触碰凭证材料，按敏感写处理。
	{method: http.MethodGet, path: "/auth-files", permission: authz.AccountPoolRead, handler: controller.ListAccountPoolAuthFiles},
	{method: http.MethodPost, path: "/auth-files", permission: authz.AccountPoolSensitiveWrite, handler: controller.CreateAccountPoolAuthFile},
	{method: http.MethodPost, path: "/auth-files/import", permission: authz.AccountPoolSensitiveWrite, handler: controller.ImportAccountPoolAuthFiles},
	{method: http.MethodGet, path: "/auth-files/:auth_file_id", permission: authz.AccountPoolRead, handler: controller.GetAccountPoolAuthFile},
	{method: http.MethodPut, path: "/auth-files/:auth_file_id", permission: authz.AccountPoolSensitiveWrite, handler: controller.UpdateAccountPoolAuthFile},
	{method: http.MethodDelete, path: "/auth-files/:auth_file_id", permission: authz.AccountPoolSensitiveWrite, handler: controller.DeleteAccountPoolAuthFile},

	// 分组管理。分组配置本身是普通写，删除分组会影响账号生命周期，按敏感写处理。
	{method: http.MethodGet, path: "/groups", permission: authz.AccountPoolRead, handler: controller.ListAccountPoolGroups},
	{method: http.MethodPost, path: "/groups", permission: authz.AccountPoolWrite, handler: controller.CreateAccountPoolGroup},
	{method: http.MethodGet, path: "/groups/options", permission: authz.AccountPoolRead, handler: controller.ListAccountPoolGroupOptions},
	{method: http.MethodGet, path: "/groups/:id", permission: authz.AccountPoolRead, handler: controller.GetAccountPoolGroup},
	{method: http.MethodPut, path: "/groups/:id", permission: authz.AccountPoolWrite, handler: controller.UpdateAccountPoolGroup},
	{method: http.MethodDelete, path: "/groups/:id", permission: authz.AccountPoolSensitiveWrite, handler: controller.DeleteAccountPoolGroup},

	// OAuth 和设备授权流程会产生或更新账号凭证，统一归入敏感写。
	{method: http.MethodPost, path: "/groups/:id/oauth/:provider/start", permission: authz.AccountPoolSensitiveWrite, handler: controller.StartAccountPoolProviderOAuth},
	{method: http.MethodPost, path: "/groups/:id/oauth/:provider/complete", permission: authz.AccountPoolSensitiveWrite, handler: controller.CompleteAccountPoolProviderOAuth},
	{method: http.MethodPost, path: "/groups/:id/device/:provider/start", permission: authz.AccountPoolSensitiveWrite, handler: controller.StartAccountPoolProviderDevice},
	{method: http.MethodGet, path: "/login-sessions/:session_id", permission: authz.AccountPoolRead, handler: controller.GetAccountPoolLoginSession},
	{method: http.MethodPost, path: "/login-sessions/:session_id/cancel", permission: authz.AccountPoolOperate, handler: controller.CancelAccountPoolLoginSession},

	// 分组内账号管理。检测、状态和脱敏导出属于操作；账号生命周期和凭证变更属于敏感写。
	{method: http.MethodGet, path: "/groups/:id/accounts", permission: authz.AccountPoolRead, handler: controller.ListPoolAccounts},
	{method: http.MethodPost, path: "/groups/:id/accounts", permission: authz.AccountPoolSensitiveWrite, handler: controller.CreatePoolAccount},
	{method: http.MethodPost, path: "/groups/:id/accounts/batch", permission: authz.AccountPoolSensitiveWrite, handler: controller.BatchCreatePoolAccounts},
	{method: http.MethodPost, path: "/groups/:id/accounts/attach", permission: authz.AccountPoolSensitiveWrite, handler: controller.AttachPoolAccountsToGroup},
	{method: http.MethodPost, path: "/groups/:id/accounts/status", permission: authz.AccountPoolOperate, handler: controller.BatchUpdatePoolAccountStatus},
	{method: http.MethodPost, path: "/groups/:id/accounts/export", permission: authz.AccountPoolOperate, handler: controller.BatchExportPoolAccounts},
	{method: http.MethodPost, path: "/groups/:id/accounts/delete", permission: authz.AccountPoolSensitiveWrite, handler: controller.BatchDeletePoolAccounts},
	{method: http.MethodPost, path: "/groups/:id/accounts/check", permission: authz.AccountPoolOperate, handler: controller.CheckPoolAccountsInGroup},
	{method: http.MethodPost, path: "/groups/:id/accounts/check-tasks", permission: authz.AccountPoolOperate, handler: controller.StartPoolAccountCheckTask},

	// 单账号管理
	{method: http.MethodGet, path: "/accounts/:account_id", permission: authz.AccountPoolRead, handler: controller.GetPoolAccount},
	{method: http.MethodPut, path: "/accounts/:account_id", permission: authz.AccountPoolSensitiveWrite, handler: controller.UpdatePoolAccount},
	{method: http.MethodDelete, path: "/accounts/:account_id", permission: authz.AccountPoolSensitiveWrite, handler: controller.DeletePoolAccount},
	{method: http.MethodPost, path: "/accounts/:account_id/status", permission: authz.AccountPoolOperate, handler: controller.UpdatePoolAccountStatus},
	{method: http.MethodPost, path: "/accounts/:account_id/check", permission: authz.AccountPoolOperate, handler: controller.CheckPoolAccount},
	{method: http.MethodPost, path: "/accounts/:account_id/refresh", permission: authz.AccountPoolSensitiveWrite, handler: controller.RefreshPoolAccountCredential},
	{method: http.MethodPost, path: "/accounts/:account_id/runtime/reset", permission: authz.AccountPoolOperate, handler: controller.ResetPoolAccountRuntime},

	// Codex OAuth 会创建账号池凭证，归入敏感写。
	{method: http.MethodPost, path: "/oauth/codex/start", permission: authz.AccountPoolSensitiveWrite, handler: controller.StartAccountPoolCodexOAuth},
	{method: http.MethodPost, path: "/oauth/codex/complete", permission: authz.AccountPoolSensitiveWrite, handler: controller.CompleteAccountPoolCodexOAuth},
}
