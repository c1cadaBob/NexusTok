// Package middleware - audit.go
// 该文件实现管理写操作的审计兜底。
//
// 审计挂载在 AdminAuth/RootAuth 鉴权成功之后，只观察 POST/PUT/PATCH/DELETE
// 管理接口的响应结果，并写入 LogTypeManage。为了避免泄露密钥、密码、OAuth token
// 或渠道配置，本兜底不读取请求体，只记录操作者、路由模板、路径参数、状态码和业务结果。
package middleware

import (
	"bytes"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"

	"github.com/gin-gonic/gin"
)

const auditResponseBodyMaxSize = 64 * 1024

// auditResponseWriter 包装 gin.ResponseWriter，并额外缓存有限长度的响应体。
// 缓存只用于解析 {"success": false} 这类业务失败响应；超过上限后只保留前缀，
// 以免密钥导出、大批量导出等响应占用过多内存。
type auditResponseWriter struct {
	gin.ResponseWriter
	body    *bytes.Buffer
	maxSize int
}

func (w *auditResponseWriter) Write(data []byte) (int, error) {
	if w.body.Len() < w.maxSize {
		remain := w.maxSize - w.body.Len()
		if remain >= len(data) {
			_, _ = w.body.Write(data)
		} else if remain > 0 {
			_, _ = w.body.Write(data[:remain])
		}
	}
	return w.ResponseWriter.Write(data)
}

func (w *auditResponseWriter) WriteString(data string) (int, error) {
	return w.Write([]byte(data))
}

// auditRouteActions 将「METHOD + 路由模板」映射为稳定 action。
// action 是语言无关标识，前端可用它做本地化展示；未命中的管理写接口会回退为 generic，
// 仍然记录 method、route、path、status 和 success，避免因为漏配 action 而没有审计。
var auditRouteActions = map[string]string{
	// 用户管理
	"POST /api/user/topup/complete":                    "user.topup_complete",
	"POST /api/user/":                                  "user.create",
	"POST /api/user/manage":                            "user.manage",
	"PUT /api/user/":                                   "user.update",
	"DELETE /api/user/:id":                             "user.delete",
	"DELETE /api/user/:id/reset_passkey":               "user.reset_passkey",
	"DELETE /api/user/:id/oauth/bindings/:provider_id": "user.oauth_unbind",
	"DELETE /api/user/:id/bindings/:binding_type":      "user.binding_clear",
	"DELETE /api/user/:id/2fa":                         "user.2fa_disable",

	// 订阅管理
	"POST /api/subscription/admin/plans":                             "subscription.plan_create",
	"PUT /api/subscription/admin/plans/:id":                          "subscription.plan_update",
	"PATCH /api/subscription/admin/plans/:id":                        "subscription.plan_status_update",
	"POST /api/subscription/admin/bind":                              "subscription.bind",
	"POST /api/subscription/admin/users/:id/subscriptions":           "subscription.user_create",
	"POST /api/subscription/admin/user_subscriptions/:id/invalidate": "subscription.user_invalidate",
	"DELETE /api/subscription/admin/user_subscriptions/:id":          "subscription.user_delete",

	// 系统设置、性能和后台任务
	"PUT /api/option/":                          "option.update",
	"POST /api/option/payment_compliance":       "option.payment_compliance",
	"DELETE /api/option/channel_affinity_cache": "option.clear_affinity_cache",
	"POST /api/option/rest_model_ratio":         "option.reset_model_ratio",
	"POST /api/option/migrate_console_setting":  "option.migrate_console_setting",
	"POST /api/performance/reset_stats":         "performance.reset_stats",
	"DELETE /api/performance/disk_cache":        "performance.clear_disk_cache",
	"POST /api/performance/gc":                  "performance.gc",
	"DELETE /api/performance/logs":              "performance.clear_logs",
	"POST /api/ratio_sync/fetch":                "ratio_sync.fetch",
	"POST /api/system-task/log-cleanup":         "system_task.log_cleanup",
	"DELETE /api/log/":                          "log.clear",

	// 权限治理
	"POST /api/authz/roles":              "authz.role_create",
	"PUT /api/authz/roles/:key":          "authz.role_update",
	"DELETE /api/authz/roles/:key":       "authz.role_delete",
	"PUT /api/authz/roles/:key/policies": "authz.role_policies_update",
	"POST /api/authz/policies/import":    "authz.policies_import",

	// 自定义 OAuth 提供商
	"POST /api/custom-oauth-provider/discovery": "custom_oauth.discovery",
	"POST /api/custom-oauth-provider/":          "custom_oauth.create",
	"PUT /api/custom-oauth-provider/:id":        "custom_oauth.update",
	"DELETE /api/custom-oauth-provider/:id":     "custom_oauth.delete",

	// 账号池
	"POST /api/account-pool/auth-files":                          "account_pool.auth_file_create",
	"POST /api/account-pool/auth-files/import":                   "account_pool.auth_file_import",
	"PUT /api/account-pool/auth-files/:auth_file_id":             "account_pool.auth_file_update",
	"DELETE /api/account-pool/auth-files/:auth_file_id":          "account_pool.auth_file_delete",
	"POST /api/account-pool/check-tasks/cleanup":                 "account_pool.check_task_cleanup",
	"POST /api/account-pool/groups":                              "account_pool.group_create",
	"PUT /api/account-pool/groups/:id":                           "account_pool.group_update",
	"DELETE /api/account-pool/groups/:id":                        "account_pool.group_delete",
	"POST /api/account-pool/groups/:id/oauth/:provider/start":    "account_pool.oauth_start",
	"POST /api/account-pool/groups/:id/oauth/:provider/complete": "account_pool.oauth_complete",
	"POST /api/account-pool/groups/:id/device/:provider/start":   "account_pool.device_start",
	"POST /api/account-pool/groups/:id/accounts":                 "account_pool.account_create",
	"POST /api/account-pool/groups/:id/accounts/batch":           "account_pool.account_batch_create",
	"POST /api/account-pool/groups/:id/accounts/attach":          "account_pool.account_attach",
	"POST /api/account-pool/groups/:id/accounts/status":          "account_pool.account_batch_status",
	"POST /api/account-pool/groups/:id/accounts/export":          "account_pool.account_batch_export",
	"POST /api/account-pool/groups/:id/accounts/delete":          "account_pool.account_batch_delete",
	"POST /api/account-pool/groups/:id/accounts/check":           "account_pool.account_batch_check",
	"POST /api/account-pool/groups/:id/accounts/check-tasks":     "account_pool.account_check_task",
	"POST /api/account-pool/login-sessions/:session_id/cancel":   "account_pool.login_session_cancel",
	"PUT /api/account-pool/accounts/:account_id":                 "account_pool.account_update",
	"DELETE /api/account-pool/accounts/:account_id":              "account_pool.account_delete",
	"POST /api/account-pool/accounts/:account_id/status":         "account_pool.account_status",
	"POST /api/account-pool/accounts/:account_id/check":          "account_pool.account_check",
	"POST /api/account-pool/accounts/:account_id/refresh":        "account_pool.account_refresh",
	"POST /api/account-pool/accounts/:account_id/runtime/reset":  "account_pool.account_runtime_reset",
	"POST /api/account-pool/oauth/codex/start":                   "account_pool.codex_oauth_start",
	"POST /api/account-pool/oauth/codex/complete":                "account_pool.codex_oauth_complete",

	// 渠道和渠道账号
	"POST /api/channel/":                                "channel.create",
	"PUT /api/channel/":                                 "channel.update",
	"DELETE /api/channel/disabled":                      "channel.delete_disabled",
	"DELETE /api/channel/:id":                           "channel.delete",
	"POST /api/channel/batch":                           "channel.delete_batch",
	"POST /api/channel/:id/key":                         "channel.key_view",
	"POST /api/channel/:id/accounts":                    "channel_account.create",
	"POST /api/channel/:id/accounts/batch":              "channel_account.batch_create",
	"POST /api/channel/:id/accounts/import-multikey":    "channel_account.import_multikey",
	"PUT /api/channel/:id/accounts/:account_id":         "channel_account.update",
	"DELETE /api/channel/:id/accounts/:account_id":      "channel_account.delete",
	"POST /api/channel/:id/accounts/:account_id/status": "channel_account.status",
	"POST /api/channel/tag/disabled":                    "channel.tag_disable",
	"POST /api/channel/tag/enabled":                     "channel.tag_enable",
	"PUT /api/channel/tag":                              "channel.tag_edit",
	"POST /api/channel/batch/tag":                       "channel.tag_batch_set",
	"POST /api/channel/fix":                             "channel.fix",
	"POST /api/channel/fetch_models":                    "channel.fetch_models",
	"POST /api/channel/codex/oauth/start":               "channel.codex_oauth_start",
	"POST /api/channel/codex/oauth/complete":            "channel.codex_oauth_complete",
	"POST /api/channel/:id/codex/oauth/start":           "channel.codex_oauth_start_for_channel",
	"POST /api/channel/:id/codex/oauth/complete":        "channel.codex_oauth_complete_for_channel",
	"POST /api/channel/:id/codex/refresh":               "channel.codex_refresh",
	"POST /api/channel/:id/codex/usage/reset":           "channel.codex_usage_reset",
	"POST /api/channel/ollama/pull":                     "channel.ollama_pull",
	"POST /api/channel/ollama/pull/stream":              "channel.ollama_pull_stream",
	"DELETE /api/channel/ollama/delete":                 "channel.ollama_delete",
	"POST /api/channel/copy/:id":                        "channel.copy",
	"POST /api/channel/multi_key/manage":                "channel.multi_key_manage",
	"POST /api/channel/upstream_updates/apply":          "channel.upstream_apply",
	"POST /api/channel/upstream_updates/apply_all":      "channel.upstream_apply_all",
	"POST /api/channel/upstream_updates/detect":         "channel.upstream_detect",
	"POST /api/channel/upstream_updates/detect_all":     "channel.upstream_detect_all",

	// 兑换码、预填分组、厂商、模型和部署
	"POST /api/redemption/":                          "redemption.create",
	"PUT /api/redemption/":                           "redemption.update",
	"DELETE /api/redemption/invalid":                 "redemption.delete_invalid",
	"DELETE /api/redemption/:id":                     "redemption.delete",
	"POST /api/prefill_group/":                       "prefill_group.create",
	"PUT /api/prefill_group/":                        "prefill_group.update",
	"DELETE /api/prefill_group/:id":                  "prefill_group.delete",
	"POST /api/vendors/":                             "vendor.create",
	"PUT /api/vendors/":                              "vendor.update",
	"DELETE /api/vendors/:id":                        "vendor.delete",
	"POST /api/models/sync_upstream":                 "model.sync_upstream",
	"PUT /api/models/:id/pricing":                    "model.pricing_update",
	"POST /api/models/":                              "model.create",
	"PUT /api/models/":                               "model.update",
	"DELETE /api/models/:id":                         "model.delete",
	"POST /api/deployments/settings/test-connection": "deployment.settings_test_connection",
	"POST /api/deployments/test-connection":          "deployment.test_connection",
	"POST /api/deployments/price-estimation":         "deployment.price_estimation",
	"POST /api/deployments/":                         "deployment.create",
	"PUT /api/deployments/:id":                       "deployment.update",
	"PUT /api/deployments/:id/name":                  "deployment.rename",
	"POST /api/deployments/:id/extend":               "deployment.extend",
	"DELETE /api/deployments/:id":                    "deployment.delete",
}

// beginAdminAudit 在管理写操作进入业务 handler 前包装响应写入器。
// 返回的 bool 表示本层是否拥有审计包装所有权；嵌套 RootAuth 不会再次包装，避免双写日志。
func beginAdminAudit(c *gin.Context) (*auditResponseWriter, bool) {
	method := c.Request.Method
	if method != "POST" && method != "PUT" && method != "PATCH" && method != "DELETE" {
		return nil, false
	}
	if common.GetContextKeyBool(c, constant.ContextKeyAuditActive) {
		return nil, false
	}

	writer := &auditResponseWriter{
		ResponseWriter: c.Writer,
		body:           bytes.NewBuffer(nil),
		maxSize:        auditResponseBodyMaxSize,
	}
	common.SetContextKey(c, constant.ContextKeyAuditActive, true)
	c.Writer = writer
	return writer, true
}

// finishAdminAudit 在业务 handler 完成后写入兜底审计日志。
// 如果 handler 已通过 ContextKeyAuditLogged 标记手动审计，则跳过兜底，避免同一次操作重复记录。
func finishAdminAudit(c *gin.Context, writer *auditResponseWriter, owner bool) {
	if !owner {
		return
	}
	defer common.SetContextKey(c, constant.ContextKeyAuditActive, false)
	if writer == nil {
		return
	}
	if common.GetContextKeyBool(c, constant.ContextKeyAuditLogged) {
		return
	}

	method := c.Request.Method
	route := c.FullPath()
	if route == "" {
		route = c.Request.URL.Path
	}
	action := auditRouteActions[method+" "+route]
	if action == "" {
		action = "generic"
	}

	routeParams := map[string]string{}
	for _, param := range c.Params {
		routeParams[param.Key] = param.Value
	}

	opParams := map[string]interface{}{}
	if action == "generic" {
		opParams["method"] = method
		opParams["route"] = route
	}

	status := writer.Status()
	success := auditResponseSuccess(status, writer.body.Bytes())
	adminInfo := map[string]interface{}{
		"admin_id":       c.GetInt("id"),
		"admin_username": c.GetString("username"),
		"admin_role":     c.GetInt("role"),
		"auth_method":    auditAuthMethod(c),
	}
	auditInfo := map[string]interface{}{
		"method":  method,
		"route":   route,
		"path":    c.Request.URL.Path,
		"status":  status,
		"success": success,
	}
	if len(routeParams) > 0 {
		auditInfo["params"] = routeParams
	}

	model.RecordOperationAuditLog(model.OperationAuditLogParams{
		UserId:    c.GetInt("id"),
		Content:   method + " " + route,
		Ip:        c.ClientIP(),
		Action:    action,
		Params:    opParams,
		AdminInfo: adminInfo,
		AuditInfo: auditInfo,
	})
}

func auditAuthMethod(c *gin.Context) string {
	if c.GetBool("use_access_token") {
		return "access_token"
	}
	return "session"
}

// auditResponseSuccess 优先读取 JSON 响应体中的 success 字段；如果不存在或无法解析，
// 则退回到 HTTP 状态码判断。显式 success=false 必须被识别为失败，即使 HTTP 状态为 200。
func auditResponseSuccess(status int, body []byte) bool {
	if status >= 400 {
		return false
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		var response struct {
			Success *bool `json:"success"`
		}
		if err := common.Unmarshal(trimmed, &response); err == nil && response.Success != nil {
			return *response.Success
		}
	}
	return status < 400
}
