// Package router - channel-router.go
// 该文件集中注册渠道管理路由。
//
// 渠道路由先经过 AdminAuth，再按 authz 权限表执行 read/operate/write/
// sensitive_write/secret_view 二次校验。原本 Root-only 或需要安全验证的路径
// 继续保留旧中间件，避免权限表灰度接入时放宽敏感边界。
package router

import (
	"net/http"

	"github.com/c1cada/NexusTok/controller"
	"github.com/c1cada/NexusTok/middleware"
	"github.com/c1cada/NexusTok/service/authz"

	"github.com/gin-gonic/gin"
)

type permissionRoute struct {
	method     string
	path       string
	permission authz.Permission
	before     []gin.HandlerFunc
	after      []gin.HandlerFunc
	handler    gin.HandlerFunc
}

// registerChannelRoutes 注册 /api/channel 路由组。
func registerChannelRoutes(apiRouter *gin.RouterGroup) {
	channelRoute := apiRouter.Group("/channel")
	channelRoute.Use(middleware.AdminAuth())

	for _, route := range channelPermissionRoutes {
		handlers := make([]gin.HandlerFunc, 0, len(route.before)+len(route.after)+2)
		handlers = append(handlers, route.before...)
		handlers = append(handlers, middleware.RequirePermission(route.permission))
		handlers = append(handlers, route.after...)
		handlers = append(handlers, route.handler)
		channelRoute.Handle(route.method, route.path, handlers...)
	}
}

var channelPermissionRoutes = []permissionRoute{
	// 渠道查询
	{method: http.MethodGet, path: "/", permission: authz.ChannelRead, handler: controller.GetAllChannels},
	{method: http.MethodGet, path: "/search", permission: authz.ChannelRead, handler: controller.SearchChannels},
	{method: http.MethodGet, path: "/models", permission: authz.ChannelRead, handler: controller.ChannelListModels},
	{method: http.MethodGet, path: "/models_enabled", permission: authz.ChannelRead, handler: controller.EnabledListModels},
	{method: http.MethodGet, path: "/:id", permission: authz.ChannelRead, handler: controller.GetChannel},

	// 渠道账号管理。账号新增、导入、更新和删除会触碰凭证材料，按敏感写处理。
	{method: http.MethodGet, path: "/:id/accounts", permission: authz.ChannelRead, handler: controller.ListChannelAccounts},
	{method: http.MethodPost, path: "/:id/accounts", permission: authz.ChannelSensitiveWrite, handler: controller.CreateChannelAccount},
	{method: http.MethodPost, path: "/:id/accounts/batch", permission: authz.ChannelSensitiveWrite, handler: controller.BatchCreateChannelAccounts},
	{method: http.MethodPost, path: "/:id/accounts/import-multikey", permission: authz.ChannelSensitiveWrite, handler: controller.ImportMultiKeyToChannelAccounts},
	{method: http.MethodGet, path: "/:id/accounts/:account_id", permission: authz.ChannelRead, handler: controller.GetChannelAccount},
	{method: http.MethodPut, path: "/:id/accounts/:account_id", permission: authz.ChannelSensitiveWrite, handler: controller.UpdateChannelAccount},
	{method: http.MethodDelete, path: "/:id/accounts/:account_id", permission: authz.ChannelSensitiveWrite, handler: controller.DeleteChannelAccount},
	{method: http.MethodPost, path: "/:id/accounts/:account_id/status", permission: authz.ChannelOperate, handler: controller.UpdateChannelAccountStatus},

	// 渠道密钥获取同时需要 Root、限流、禁缓存和安全验证，权限表只作为额外审计边界。
	{
		method:     http.MethodPost,
		path:       "/:id/key",
		permission: authz.ChannelSecretView,
		before:     []gin.HandlerFunc{middleware.RootAuth()},
		after:      []gin.HandlerFunc{middleware.CriticalRateLimit(), middleware.DisableCache(), middleware.SecureVerificationRequired()},
		handler:    controller.GetChannelKey,
	},

	// 渠道测试和余额更新
	{method: http.MethodGet, path: "/test", permission: authz.ChannelOperate, handler: controller.TestAllChannels},
	{method: http.MethodGet, path: "/test/:id", permission: authz.ChannelOperate, handler: controller.TestChannel},
	{method: http.MethodGet, path: "/update_balance", permission: authz.ChannelOperate, handler: controller.UpdateAllChannelsBalance},
	{method: http.MethodGet, path: "/update_balance/:id", permission: authz.ChannelOperate, handler: controller.UpdateChannelBalance},

	// 渠道增删改
	{method: http.MethodPost, path: "/", permission: authz.ChannelSensitiveWrite, handler: controller.AddChannel},
	{method: http.MethodPut, path: "/", permission: authz.ChannelWrite, handler: controller.UpdateChannel},
	{method: http.MethodDelete, path: "/disabled", permission: authz.ChannelSensitiveWrite, handler: controller.DeleteDisabledChannel},
	{method: http.MethodDelete, path: "/:id", permission: authz.ChannelSensitiveWrite, handler: controller.DeleteChannel},
	{method: http.MethodPost, path: "/batch", permission: authz.ChannelSensitiveWrite, handler: controller.DeleteChannelBatch},

	// 渠道标签管理
	{method: http.MethodPost, path: "/tag/disabled", permission: authz.ChannelOperate, handler: controller.DisableTagChannels},
	{method: http.MethodPost, path: "/tag/enabled", permission: authz.ChannelOperate, handler: controller.EnableTagChannels},
	{method: http.MethodPut, path: "/tag", permission: authz.ChannelWrite, handler: controller.EditTagChannels},
	{method: http.MethodPost, path: "/batch/tag", permission: authz.ChannelWrite, handler: controller.BatchSetChannelTag},
	{method: http.MethodGet, path: "/tag/models", permission: authz.ChannelRead, handler: controller.GetTagModels},

	// 渠道能力修复
	{method: http.MethodPost, path: "/fix", permission: authz.ChannelOperate, handler: controller.FixChannelsAbilities},

	// 上游模型获取
	{method: http.MethodGet, path: "/fetch_models/:id", permission: authz.ChannelOperate, handler: controller.FetchUpstreamModels},
	{method: http.MethodPost, path: "/fetch_models", permission: authz.ChannelSensitiveWrite, before: []gin.HandlerFunc{middleware.RootAuth()}, handler: controller.FetchModels},

	// Codex OAuth 和用量维护
	{method: http.MethodPost, path: "/codex/oauth/start", permission: authz.ChannelSensitiveWrite, handler: controller.StartCodexOAuth},
	{method: http.MethodPost, path: "/codex/oauth/complete", permission: authz.ChannelSensitiveWrite, handler: controller.CompleteCodexOAuth},
	{method: http.MethodPost, path: "/:id/codex/oauth/start", permission: authz.ChannelSensitiveWrite, handler: controller.StartCodexOAuthForChannel},
	{method: http.MethodPost, path: "/:id/codex/oauth/complete", permission: authz.ChannelSensitiveWrite, handler: controller.CompleteCodexOAuthForChannel},
	{method: http.MethodPost, path: "/:id/codex/refresh", permission: authz.ChannelSensitiveWrite, handler: controller.RefreshCodexChannelCredential},
	{method: http.MethodGet, path: "/:id/codex/usage", permission: authz.ChannelRead, handler: controller.GetCodexChannelUsage},
	{method: http.MethodGet, path: "/:id/codex/usage/reset-credits", permission: authz.ChannelRead, handler: controller.GetCodexChannelRateLimitResetCredits},
	{method: http.MethodPost, path: "/:id/codex/usage/reset", permission: authz.ChannelOperate, handler: controller.ResetCodexChannelUsage},

	// Ollama 模型管理
	{method: http.MethodPost, path: "/ollama/pull", permission: authz.ChannelSensitiveWrite, handler: controller.OllamaPullModel},
	{method: http.MethodPost, path: "/ollama/pull/stream", permission: authz.ChannelSensitiveWrite, handler: controller.OllamaPullModelStream},
	{method: http.MethodDelete, path: "/ollama/delete", permission: authz.ChannelSensitiveWrite, handler: controller.OllamaDeleteModel},
	{method: http.MethodGet, path: "/ollama/version/:id", permission: authz.ChannelOperate, handler: controller.OllamaVersion},

	// 渠道复制和多密钥管理
	{method: http.MethodPost, path: "/copy/:id", permission: authz.ChannelSensitiveWrite, handler: controller.CopyChannel},
	{method: http.MethodPost, path: "/multi_key/manage", permission: authz.ChannelSensitiveWrite, handler: controller.ManageMultiKeys},

	// 上游模型更新
	{method: http.MethodPost, path: "/upstream_updates/apply", permission: authz.ChannelWrite, handler: controller.ApplyChannelUpstreamModelUpdates},
	{method: http.MethodPost, path: "/upstream_updates/apply_all", permission: authz.ChannelWrite, handler: controller.ApplyAllChannelUpstreamModelUpdates},
	{method: http.MethodPost, path: "/upstream_updates/detect", permission: authz.ChannelOperate, handler: controller.DetectChannelUpstreamModelUpdates},
	{method: http.MethodPost, path: "/upstream_updates/detect_all", permission: authz.ChannelOperate, handler: controller.DetectAllChannelUpstreamModelUpdates},
}
