// Package router - channel-router.go
// 该文件集中注册渠道管理路由。
//
// 当前仍保持既有 AdminAuth/RootAuth 行为不变；先把渠道路由从 api-router.go
// 抽出，后续接入细粒度 Authz 时可以在这里替换为 read/operate/write/
// sensitive_write 权限表，而不继续扩大主路由文件。
package router

import (
	"github.com/c1cada/NexusTok/controller"
	"github.com/c1cada/NexusTok/middleware"

	"github.com/gin-gonic/gin"
)

// registerChannelRoutes 注册 /api/channel 路由组。
func registerChannelRoutes(apiRouter *gin.RouterGroup) {
	channelRoute := apiRouter.Group("/channel")
	channelRoute.Use(middleware.AdminAuth())
	{
		// 渠道查询
		channelRoute.GET("/", controller.GetAllChannels)                  // 获取所有渠道
		channelRoute.GET("/search", controller.SearchChannels)            // 搜索渠道
		channelRoute.GET("/models", controller.ChannelListModels)         // 获取渠道模型列表
		channelRoute.GET("/models_enabled", controller.EnabledListModels) // 获取启用的模型列表
		channelRoute.GET("/:id", controller.GetChannel)                   // 获取单个渠道

		// 渠道账号管理
		channelRoute.GET("/:id/accounts", controller.ListChannelAccounts)                              // 获取渠道账号列表
		channelRoute.POST("/:id/accounts", controller.CreateChannelAccount)                            // 创建渠道账号
		channelRoute.POST("/:id/accounts/batch", controller.BatchCreateChannelAccounts)                // 批量创建渠道账号
		channelRoute.POST("/:id/accounts/import-multikey", controller.ImportMultiKeyToChannelAccounts) // 导入多密钥
		channelRoute.GET("/:id/accounts/:account_id", controller.GetChannelAccount)                    // 获取单个渠道账号
		channelRoute.PUT("/:id/accounts/:account_id", controller.UpdateChannelAccount)                 // 更新渠道账号
		channelRoute.DELETE("/:id/accounts/:account_id", controller.DeleteChannelAccount)              // 删除渠道账号
		channelRoute.POST("/:id/accounts/:account_id/status", controller.UpdateChannelAccountStatus)   // 更新渠道账号状态

		// 渠道密钥获取（需要 root 权限和安全验证）
		channelRoute.POST("/:id/key", middleware.RootAuth(), middleware.CriticalRateLimit(), middleware.DisableCache(), middleware.SecureVerificationRequired(), controller.GetChannelKey)

		// 渠道测试和余额更新
		channelRoute.GET("/test", controller.TestAllChannels)                    // 测试所有渠道
		channelRoute.GET("/test/:id", controller.TestChannel)                    // 测试单个渠道
		channelRoute.GET("/update_balance", controller.UpdateAllChannelsBalance) // 更新所有渠道余额
		channelRoute.GET("/update_balance/:id", controller.UpdateChannelBalance) // 更新单个渠道余额

		// 渠道增删改
		channelRoute.POST("/", controller.AddChannel)                      // 添加渠道
		channelRoute.PUT("/", controller.UpdateChannel)                    // 更新渠道
		channelRoute.DELETE("/disabled", controller.DeleteDisabledChannel) // 删除禁用的渠道
		channelRoute.DELETE("/:id", controller.DeleteChannel)              // 删除渠道
		channelRoute.POST("/batch", controller.DeleteChannelBatch)         // 批量删除渠道

		// 渠道标签管理
		channelRoute.POST("/tag/disabled", controller.DisableTagChannels) // 禁用标签渠道
		channelRoute.POST("/tag/enabled", controller.EnableTagChannels)   // 启用标签渠道
		channelRoute.PUT("/tag", controller.EditTagChannels)              // 编辑标签渠道
		channelRoute.POST("/batch/tag", controller.BatchSetChannelTag)    // 批量设置渠道标签
		channelRoute.GET("/tag/models", controller.GetTagModels)          // 获取标签模型

		// 渠道能力修复
		channelRoute.POST("/fix", controller.FixChannelsAbilities)

		// 上游模型获取
		channelRoute.GET("/fetch_models/:id", controller.FetchUpstreamModels)             // 获取上游模型列表
		channelRoute.POST("/fetch_models", middleware.RootAuth(), controller.FetchModels) // 获取模型列表（需要 root）

		// Codex OAuth
		channelRoute.POST("/codex/oauth/start", controller.StartCodexOAuth)                                 // 开始 Codex OAuth
		channelRoute.POST("/codex/oauth/complete", controller.CompleteCodexOAuth)                           // 完成 Codex OAuth
		channelRoute.POST("/:id/codex/oauth/start", controller.StartCodexOAuthForChannel)                   // 为指定渠道开始 Codex OAuth
		channelRoute.POST("/:id/codex/oauth/complete", controller.CompleteCodexOAuthForChannel)             // 为指定渠道完成 Codex OAuth
		channelRoute.POST("/:id/codex/refresh", controller.RefreshCodexChannelCredential)                   // 刷新 Codex 渠道凭证
		channelRoute.GET("/:id/codex/usage", controller.GetCodexChannelUsage)                               // 获取 Codex 渠道使用情况
		channelRoute.GET("/:id/codex/usage/reset-credits", controller.GetCodexChannelRateLimitResetCredits) // 获取 Codex 用量重置额度
		channelRoute.POST("/:id/codex/usage/reset", controller.ResetCodexChannelUsage)                      // 重置 Codex 用量

		// Ollama 模型管理
		channelRoute.POST("/ollama/pull", controller.OllamaPullModel)              // 拉取 Ollama 模型
		channelRoute.POST("/ollama/pull/stream", controller.OllamaPullModelStream) // 流式拉取 Ollama 模型
		channelRoute.DELETE("/ollama/delete", controller.OllamaDeleteModel)        // 删除 Ollama 模型
		channelRoute.GET("/ollama/version/:id", controller.OllamaVersion)          // 获取 Ollama 版本

		// 渠道复制
		channelRoute.POST("/copy/:id", controller.CopyChannel)

		// 多密钥管理
		channelRoute.POST("/multi_key/manage", controller.ManageMultiKeys)

		// 上游模型更新
		channelRoute.POST("/upstream_updates/apply", controller.ApplyChannelUpstreamModelUpdates)          // 应用上游模型更新
		channelRoute.POST("/upstream_updates/apply_all", controller.ApplyAllChannelUpstreamModelUpdates)   // 应用所有上游模型更新
		channelRoute.POST("/upstream_updates/detect", controller.DetectChannelUpstreamModelUpdates)        // 检测上游模型更新
		channelRoute.POST("/upstream_updates/detect_all", controller.DetectAllChannelUpstreamModelUpdates) // 检测所有上游模型更新
	}
}
