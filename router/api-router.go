// Package router - api-router.go
// 该文件定义了所有 API 路由，包括：
// - 公开接口（无需认证）
// - 用户接口（需要用户认证）
// - 管理员接口（需要管理员权限）
// - 根管理员接口（需要 root 权限）
package router

import (
	"github.com/c1cada/NexusTok/controller" // 控制器层：请求处理器
	"github.com/c1cada/NexusTok/middleware" // 中间件：认证、限流等

	// 导入 oauth 包以通过 init() 注册 OAuth 提供商
	_ "github.com/c1cada/NexusTok/oauth"

	"github.com/gin-contrib/gzip" // gzip 压缩中间件
	"github.com/gin-gonic/gin"    // Gin Web 框架
)

// SetApiRouter 设置所有 API 路由
// API 路由前缀为 /api，提供 RESTful 风格的接口
// 路由按功能分组，每个组有不同的权限要求
//
// 路由层级：
// 1. 公开接口 - 无需认证（如登录、注册、状态查询）
// 2. 用户接口 - 需要有效的用户 token（如个人信息、token 管理）
// 3. 管理员接口 - 需要管理员权限（如用户管理、渠道管理）
// 4. 根管理员接口 - 需要 root 权限（如系统配置、性能管理）
func SetApiRouter(router *gin.Engine) {
	// 创建 /api 路由组
	apiRouter := router.Group("/api")

	// 注册路由标签中间件，标记这是 API 路由
	// 用于日志记录和监控区分
	apiRouter.Use(middleware.RouteTag("api"))

	// 注册 gzip 压缩中间件
	// 压缩响应体，减少网络传输量
	apiRouter.Use(gzip.Gzip(gzip.DefaultCompression))

	// 注册请求体存储清理中间件
	// 清理临时存储的请求体文件
	apiRouter.Use(middleware.BodyStorageCleanup())

	// 注册全局 API 限流中间件
	// 防止 API 被滥用
	apiRouter.Use(middleware.GlobalAPIRateLimit())
	anonymousRequestBodyLimit := middleware.AnonymousRequestBodyLimit()
	{
		// ========================================
		// 公开接口 - 无需认证
		// ========================================

		// 系统设置相关
		apiRouter.GET("/setup", controller.GetSetup)                              // 获取系统设置状态
		apiRouter.POST("/setup", anonymousRequestBodyLimit, controller.PostSetup) // 提交初始设置

		// 系统状态相关
		apiRouter.GET("/status", controller.GetStatus)                  // 获取系统状态
		apiRouter.GET("/uptime/status", controller.GetUptimeKumaStatus) // 获取 Uptime Kuma 监控状态

		// 模型列表（需要用户认证）
		apiRouter.GET("/models", middleware.UserAuth(), controller.DashboardListModels)

		// 状态测试（需要管理员权限）
		apiRouter.GET("/status/test", middleware.AdminAuth(), controller.TestStatus)

		// 公开信息接口
		apiRouter.GET("/notice", controller.GetNotice)                     // 获取系统公告
		apiRouter.GET("/user-agreement", controller.GetUserAgreement)      // 获取用户协议
		apiRouter.GET("/privacy-policy", controller.GetPrivacyPolicy)      // 获取隐私政策
		apiRouter.GET("/about", controller.GetAbout)                       // 获取关于信息
		apiRouter.GET("/home_page_content", controller.GetHomePageContent) // 获取首页内容

		// 定价信息：访问策略跟随 HeaderNavModules.pricing，已登录用户可看到分组价格
		apiRouter.GET("/pricing", middleware.HeaderNavModuleAuth("pricing"), controller.GetPricing)

		// 性能监控指标路由组
		perfMetricsRoute := apiRouter.Group("/perf-metrics")
		perfMetricsRoute.Use(middleware.HeaderNavModulePublicOrUserAuth("pricing")) // pricing 辅助数据，公开策略跟随模型广场
		{
			perfMetricsRoute.GET("/summary", controller.GetPerfMetricsSummary) // 获取性能指标摘要
			perfMetricsRoute.GET("", controller.GetPerfMetrics)                // 获取详细性能指标
		}

		// 排行榜：访问策略跟随 HeaderNavModules.rankings
		apiRouter.GET("/rankings", middleware.HeaderNavModuleAuth("rankings"), controller.GetRankings)

		// 邮箱验证（需要频率限制和 Turnstile 验证）
		apiRouter.GET("/verification", middleware.EmailVerificationRateLimit(), middleware.TurnstileCheck(), controller.SendEmailVerification)

		// 密码重置相关
		apiRouter.GET("/reset_password", middleware.CriticalRateLimit(), middleware.TurnstileCheck(), controller.SendPasswordResetEmail)
		apiRouter.POST("/user/reset", middleware.CriticalRateLimit(), anonymousRequestBodyLimit, controller.ResetPassword)

		// ========================================
		// OAuth 路由 - 特定路由必须在 :provider 通配符之前
		// ========================================

		// OAuth 状态码生成
		apiRouter.GET("/oauth/state", middleware.CriticalRateLimit(), controller.GenerateOAuthCode)

		// 邮箱绑定
		apiRouter.POST("/oauth/email/bind", middleware.CriticalRateLimit(), anonymousRequestBodyLimit, controller.EmailBind)

		// 非标准 OAuth（微信、Telegram）- 保持原有路由
		apiRouter.GET("/oauth/wechat", middleware.CriticalRateLimit(), controller.WeChatAuth)
		apiRouter.POST("/oauth/wechat/bind", middleware.CriticalRateLimit(), anonymousRequestBodyLimit, controller.WeChatBind)
		apiRouter.GET("/oauth/telegram/login", middleware.CriticalRateLimit(), controller.TelegramLogin)
		apiRouter.GET("/oauth/telegram/bind", middleware.CriticalRateLimit(), controller.TelegramBind)

		// 标准 OAuth 提供商（GitHub、Discord、OIDC、LinuxDO）- 统一路由
		apiRouter.GET("/oauth/:provider", middleware.CriticalRateLimit(), controller.HandleOAuth)

		// 比率配置
		apiRouter.GET("/ratio_config", middleware.CriticalRateLimit(), controller.GetRatioConfig)

		// ========================================
		// 支付网关 Webhook 回调（无需认证）
		// ========================================
		apiRouter.POST("/stripe/webhook", anonymousRequestBodyLimit, controller.StripeWebhook)              // Stripe 支付回调
		apiRouter.POST("/creem/webhook", anonymousRequestBodyLimit, controller.CreemWebhook)                // Creem 支付回调
		apiRouter.POST("/waffo/webhook", anonymousRequestBodyLimit, controller.WaffoWebhook)                // Waffo 支付回调
		apiRouter.POST("/waffo-pancake/webhook", anonymousRequestBodyLimit, controller.WaffoPancakeWebhook) // Waffo Pancake 支付回调

		// 通用安全验证路由（需要用户认证）
		apiRouter.POST("/verify", middleware.UserAuth(), middleware.CriticalRateLimit(), controller.UniversalVerify)

		// ========================================
		// 用户路由组 - /api/user
		// ========================================
		userRoute := apiRouter.Group("/user")
		{
			// 用户认证相关（公开接口，但有频率限制）
			userRoute.POST("/register", middleware.CriticalRateLimit(), anonymousRequestBodyLimit, middleware.TurnstileCheck(), controller.Register) // 用户注册
			userRoute.POST("/login", middleware.CriticalRateLimit(), anonymousRequestBodyLimit, middleware.TurnstileCheck(), controller.Login)       // 用户登录
			userRoute.POST("/login/2fa", middleware.CriticalRateLimit(), anonymousRequestBodyLimit, controller.Verify2FALogin)                       // 两步验证登录

			// Passkey（WebAuthn）登录
			userRoute.POST("/passkey/login/begin", middleware.CriticalRateLimit(), anonymousRequestBodyLimit, controller.PasskeyLoginBegin)   // 开始 Passkey 登录
			userRoute.POST("/passkey/login/finish", middleware.CriticalRateLimit(), anonymousRequestBodyLimit, controller.PasskeyLoginFinish) // 完成 Passkey 登录

			// 用户登出
			userRoute.GET("/logout", controller.Logout)

			// 易支付回调（无需认证）
			userRoute.POST("/epay/notify", anonymousRequestBodyLimit, controller.EpayNotify)
			userRoute.GET("/epay/notify", controller.EpayNotify)

			// 获取用户组列表（公开）
			userRoute.GET("/groups", controller.GetUserGroups)

			// ========================================
			// 用户自身操作路由 - 需要用户认证
			// ========================================
			selfRoute := userRoute.Group("/")
			selfRoute.Use(middleware.UserAuth()) // 所有子路由都需要用户认证
			{
				// 用户信息
				selfRoute.GET("/self/groups", controller.GetUserGroups) // 获取当前用户的组
				selfRoute.GET("/self", controller.GetSelf)              // 获取当前用户信息
				selfRoute.GET("/models", controller.GetUserModels)      // 获取用户可用模型
				selfRoute.PUT("/self", controller.UpdateSelf)           // 更新用户信息
				selfRoute.DELETE("/self", controller.DeleteSelf)        // 删除用户账户

				// 访问令牌
				selfRoute.GET("/token", controller.GenerateAccessToken) // 生成访问令牌

				// Passkey（WebAuthn）管理
				selfRoute.GET("/passkey", controller.PasskeyStatus)                          // 获取 Passkey 状态
				selfRoute.POST("/passkey/register/begin", controller.PasskeyRegisterBegin)   // 开始注册 Passkey
				selfRoute.POST("/passkey/register/finish", controller.PasskeyRegisterFinish) // 完成注册 Passkey
				selfRoute.POST("/passkey/verify/begin", controller.PasskeyVerifyBegin)       // 开始验证 Passkey
				selfRoute.POST("/passkey/verify/finish", controller.PasskeyVerifyFinish)     // 完成验证 Passkey
				selfRoute.DELETE("/passkey", controller.PasskeyDelete)                       // 删除 Passkey

				// 推荐码和充值
				selfRoute.GET("/aff", controller.GetAffCode)                               // 获取推荐码
				selfRoute.GET("/topup/info", controller.GetTopUpInfo)                      // 获取充值信息
				selfRoute.GET("/topup/self", controller.GetUserTopUps)                     // 获取用户充值记录
				selfRoute.POST("/topup", middleware.CriticalRateLimit(), controller.TopUp) // 用户充值

				// 支付相关
				selfRoute.POST("/pay", middleware.CriticalRateLimit(), controller.RequestEpay)             // 请求易支付
				selfRoute.POST("/amount", controller.RequestAmount)                                        // 请求金额
				selfRoute.POST("/stripe/pay", middleware.CriticalRateLimit(), controller.RequestStripePay) // 请求 Stripe 支付
				selfRoute.POST("/stripe/amount", controller.RequestStripeAmount)                           // 请求 Stripe 金额
				selfRoute.POST("/creem/pay", middleware.CriticalRateLimit(), controller.RequestCreemPay)   // 请求 Creem 支付
				selfRoute.POST("/waffo/amount", controller.RequestWaffoAmount)                             // 请求 Waffo 金额
				selfRoute.POST("/waffo/pay", middleware.CriticalRateLimit(), controller.RequestWaffoPay)   // 请求 Waffo 支付

				// 推荐配额转移
				selfRoute.POST("/aff_transfer", controller.TransferAffQuota)

				// 用户设置
				selfRoute.PUT("/setting", controller.UpdateUserSetting)

				// 两步验证（2FA）路由
				selfRoute.GET("/2fa/status", controller.Get2FAStatus)                 // 获取 2FA 状态
				selfRoute.POST("/2fa/setup", controller.Setup2FA)                     // 设置 2FA
				selfRoute.POST("/2fa/enable", controller.Enable2FA)                   // 启用 2FA
				selfRoute.POST("/2fa/disable", controller.Disable2FA)                 // 禁用 2FA
				selfRoute.POST("/2fa/backup_codes", controller.RegenerateBackupCodes) // 重新生成备用码

				// 签到路由
				selfRoute.GET("/checkin", controller.GetCheckinStatus)                        // 获取签到状态
				selfRoute.POST("/checkin", middleware.TurnstileCheck(), controller.DoCheckin) // 执行签到

				// 自定义 OAuth 绑定
				selfRoute.GET("/oauth/bindings", controller.GetUserOAuthBindings)              // 获取用户 OAuth 绑定列表
				selfRoute.DELETE("/oauth/bindings/:provider_id", controller.UnbindCustomOAuth) // 解绑 OAuth
			}

			// ========================================
			// 管理员路由组 - 需要管理员权限
			// ========================================
			adminRoute := userRoute.Group("/")
			adminRoute.Use(middleware.AdminAuth()) // 所有子路由都需要管理员权限
			{
				// 用户管理
				adminRoute.GET("/", controller.GetAllUsers)                                                // 获取所有用户
				adminRoute.GET("/topup", controller.GetAllTopUps)                                          // 获取所有充值记录
				adminRoute.POST("/topup/complete", controller.AdminCompleteTopUp)                          // 管理员完成充值
				adminRoute.GET("/search", controller.SearchUsers)                                          // 搜索用户
				adminRoute.GET("/:id/oauth/bindings", controller.GetUserOAuthBindingsByAdmin)              // 获取用户 OAuth 绑定（管理员）
				adminRoute.DELETE("/:id/oauth/bindings/:provider_id", controller.UnbindCustomOAuthByAdmin) // 解绑用户 OAuth（管理员）
				adminRoute.DELETE("/:id/bindings/:binding_type", controller.AdminClearUserBinding)         // 清除用户绑定（管理员）
				adminRoute.GET("/:id", controller.GetUser)                                                 // 获取单个用户
				adminRoute.POST("/", controller.CreateUser)                                                // 创建用户
				adminRoute.POST("/manage", controller.ManageUser)                                          // 管理用户
				adminRoute.PUT("/", controller.UpdateUser)                                                 // 更新用户
				adminRoute.DELETE("/:id", controller.DeleteUser)                                           // 删除用户
				adminRoute.DELETE("/:id/reset_passkey", controller.AdminResetPasskey)                      // 重置用户 Passkey

				// 管理员 2FA 路由
				adminRoute.GET("/2fa/stats", controller.Admin2FAStats)    // 获取 2FA 统计
				adminRoute.DELETE("/:id/2fa", controller.AdminDisable2FA) // 禁用用户 2FA
			}
		}

		// ========================================
		// 订阅计费路由组 - /api/subscription
		// ========================================
		// 用户订阅路由（需要用户认证）
		subscriptionRoute := apiRouter.Group("/subscription")
		subscriptionRoute.Use(middleware.UserAuth())
		{
			subscriptionRoute.GET("/plans", controller.GetSubscriptionPlans)                                                 // 获取订阅计划列表
			subscriptionRoute.GET("/self", controller.GetSubscriptionSelf)                                                   // 获取当前用户订阅
			subscriptionRoute.PUT("/self/preference", controller.UpdateSubscriptionPreference)                               // 更新订阅偏好
			subscriptionRoute.POST("/balance/pay", middleware.CriticalRateLimit(), controller.SubscriptionRequestBalancePay) // 余额购买订阅
			subscriptionRoute.POST("/epay/pay", middleware.CriticalRateLimit(), controller.SubscriptionRequestEpay)          // 易支付订阅
			subscriptionRoute.POST("/stripe/pay", middleware.CriticalRateLimit(), controller.SubscriptionRequestStripePay)   // Stripe 订阅
			subscriptionRoute.POST("/creem/pay", middleware.CriticalRateLimit(), controller.SubscriptionRequestCreemPay)     // Creem 订阅
		}

		// 管理员订阅管理路由（需要管理员权限）
		subscriptionAdminRoute := apiRouter.Group("/subscription/admin")
		subscriptionAdminRoute.Use(middleware.AdminAuth())
		{
			subscriptionAdminRoute.GET("/plans", controller.AdminListSubscriptionPlans)              // 获取订阅计划列表（管理员）
			subscriptionAdminRoute.POST("/plans", controller.AdminCreateSubscriptionPlan)            // 创建订阅计划
			subscriptionAdminRoute.PUT("/plans/:id", controller.AdminUpdateSubscriptionPlan)         // 更新订阅计划
			subscriptionAdminRoute.PATCH("/plans/:id", controller.AdminUpdateSubscriptionPlanStatus) // 更新订阅计划状态
			subscriptionAdminRoute.POST("/bind", controller.AdminBindSubscription)                   // 绑定订阅

			// 用户订阅管理（管理员）
			subscriptionAdminRoute.GET("/users/:id/subscriptions", controller.AdminListUserSubscriptions)                 // 获取用户订阅列表
			subscriptionAdminRoute.POST("/users/:id/subscriptions", controller.AdminCreateUserSubscription)               // 创建用户订阅
			subscriptionAdminRoute.POST("/user_subscriptions/:id/invalidate", controller.AdminInvalidateUserSubscription) // 使用户订阅失效
			subscriptionAdminRoute.DELETE("/user_subscriptions/:id", controller.AdminDeleteUserSubscription)              // 删除用户订阅
		}

		// 订阅支付回调（无需认证）
		apiRouter.POST("/subscription/epay/notify", anonymousRequestBodyLimit, controller.SubscriptionEpayNotify)
		apiRouter.GET("/subscription/epay/notify", controller.SubscriptionEpayNotify)
		apiRouter.GET("/subscription/epay/return", controller.SubscriptionEpayReturn)
		apiRouter.POST("/subscription/epay/return", anonymousRequestBodyLimit, controller.SubscriptionEpayReturn)

		// ========================================
		// 系统选项路由组 - /api/option（需要 root 权限）
		// ========================================
		optionRoute := apiRouter.Group("/option")
		optionRoute.Use(middleware.RootAuth())
		{
			optionRoute.GET("/", controller.GetOptions)                                         // 获取所有选项
			optionRoute.PUT("/", controller.UpdateOption)                                       // 更新选项
			optionRoute.POST("/payment_compliance", controller.ConfirmPaymentCompliance)        // 确认支付合规
			optionRoute.GET("/channel_affinity_cache", controller.GetChannelAffinityCacheStats) // 获取渠道亲和缓存统计
			optionRoute.DELETE("/channel_affinity_cache", controller.ClearChannelAffinityCache) // 清除渠道亲和缓存
			optionRoute.POST("/rest_model_ratio", controller.ResetModelRatio)                   // 重置模型比率
			optionRoute.POST("/migrate_console_setting", controller.MigrateConsoleSetting)      // 迁移控制台设置（旧键，下个版本删除）
		}

		// ========================================
		// 自定义 OAuth 提供商管理路由组 - /api/custom-oauth-provider（需要 root 权限）
		// ========================================
		customOAuthRoute := apiRouter.Group("/custom-oauth-provider")
		customOAuthRoute.Use(middleware.RootAuth())
		{
			customOAuthRoute.POST("/discovery", controller.FetchCustomOAuthDiscovery) // 获取 OAuth Discovery
			customOAuthRoute.GET("/", controller.GetCustomOAuthProviders)             // 获取所有自定义 OAuth 提供商
			customOAuthRoute.GET("/:id", controller.GetCustomOAuthProvider)           // 获取单个自定义 OAuth 提供商
			customOAuthRoute.POST("/", controller.CreateCustomOAuthProvider)          // 创建自定义 OAuth 提供商
			customOAuthRoute.PUT("/:id", controller.UpdateCustomOAuthProvider)        // 更新自定义 OAuth 提供商
			customOAuthRoute.DELETE("/:id", controller.DeleteCustomOAuthProvider)     // 删除自定义 OAuth 提供商
		}

		// ========================================
		// 性能监控路由组 - /api/performance（需要 root 权限）
		// ========================================
		performanceRoute := apiRouter.Group("/performance")
		performanceRoute.Use(middleware.RootAuth())
		{
			performanceRoute.GET("/stats", controller.GetPerformanceStats)          // 获取性能统计
			performanceRoute.DELETE("/disk_cache", controller.ClearDiskCache)       // 清除磁盘缓存
			performanceRoute.POST("/reset_stats", controller.ResetPerformanceStats) // 重置性能统计
			performanceRoute.POST("/gc", controller.ForceGC)                        // 强制 GC
			performanceRoute.GET("/logs", controller.GetLogFiles)                   // 获取日志文件
			performanceRoute.DELETE("/logs", controller.CleanupLogFiles)            // 清理日志文件
		}

		// ========================================
		// 比率同步路由组 - /api/ratio_sync（需要 root 权限）
		// ========================================
		ratioSyncRoute := apiRouter.Group("/ratio_sync")
		ratioSyncRoute.Use(middleware.RootAuth())
		{
			ratioSyncRoute.GET("/channels", controller.GetSyncableChannels) // 获取可同步渠道
			ratioSyncRoute.POST("/fetch", controller.FetchUpstreamRatios)   // 获取上游比率
		}

		registerAccountPoolRoutes(apiRouter)
		registerChannelRoutes(apiRouter)
		registerAuthzRoutes(apiRouter)
		registerSystemTaskRoutes(apiRouter)
		registerSystemInfoRoutes(apiRouter)

		// ========================================
		// Token 路由组 - /api/token（需要用户认证）
		// 用于管理 API 访问令牌
		// ========================================
		tokenRoute := apiRouter.Group("/token")
		tokenRoute.Use(middleware.UserAuth())
		{
			tokenRoute.GET("/", controller.GetAllTokens)                                                                            // 获取所有 token
			tokenRoute.GET("/search", middleware.SearchRateLimit(), controller.SearchTokens)                                        // 搜索 token
			tokenRoute.GET("/:id", controller.GetToken)                                                                             // 获取单个 token
			tokenRoute.POST("/:id/key", middleware.CriticalRateLimit(), middleware.DisableCache(), controller.GetTokenKey)          // 获取 token 密钥
			tokenRoute.POST("/", controller.AddToken)                                                                               // 添加 token
			tokenRoute.PUT("/", controller.UpdateToken)                                                                             // 更新 token
			tokenRoute.DELETE("/:id", controller.DeleteToken)                                                                       // 删除 token
			tokenRoute.POST("/batch", controller.DeleteTokenBatch)                                                                  // 批量删除 token
			tokenRoute.POST("/batch/keys", middleware.CriticalRateLimit(), middleware.DisableCache(), controller.GetTokenKeysBatch) // 批量获取 token 密钥
		}

		// ========================================
		// 使用量路由组 - /api/usage
		// ========================================
		usageRoute := apiRouter.Group("/usage")
		usageRoute.Use(middleware.CORS(), middleware.CriticalRateLimit()) // CORS 和频率限制
		{
			// Token 使用量查询（只读 token 认证）
			tokenUsageRoute := usageRoute.Group("/token")
			tokenUsageRoute.Use(middleware.TokenAuthReadOnly()) // 使用只读 token 认证
			{
				tokenUsageRoute.GET("/", controller.GetTokenUsage) // 获取 token 使用量
			}
		}

		// ========================================
		// 兑换码路由组 - /api/redemption（需要管理员权限）
		// ========================================
		redemptionRoute := apiRouter.Group("/redemption")
		redemptionRoute.Use(middleware.AdminAuth())
		{
			redemptionRoute.GET("/", controller.GetAllRedemptions)                 // 获取所有兑换码
			redemptionRoute.GET("/search", controller.SearchRedemptions)           // 搜索兑换码
			redemptionRoute.GET("/:id", controller.GetRedemption)                  // 获取单个兑换码
			redemptionRoute.POST("/", controller.AddRedemption)                    // 添加兑换码
			redemptionRoute.PUT("/", controller.UpdateRedemption)                  // 更新兑换码
			redemptionRoute.DELETE("/invalid", controller.DeleteInvalidRedemption) // 删除无效兑换码
			redemptionRoute.DELETE("/:id", controller.DeleteRedemption)            // 删除兑换码
		}

		// ========================================
		// 日志路由组 - /api/log
		// ========================================
		logRoute := apiRouter.Group("/log")
		// 管理员接口
		logRoute.GET("/", middleware.AdminAuth(), controller.GetAllLogs)                                                    // 获取所有日志
		logRoute.DELETE("/", middleware.AdminAuth(), controller.DeleteHistoryLogs)                                          // 删除历史日志
		logRoute.GET("/stat", middleware.AdminAuth(), controller.GetLogsStat)                                               // 获取日志统计
		logRoute.GET("/self/stat", middleware.UserAuth(), controller.GetLogsSelfStat)                                       // 获取用户日志统计
		logRoute.GET("/channel_affinity_usage_cache", middleware.AdminAuth(), controller.GetChannelAffinityUsageCacheStats) // 获取渠道亲和使用缓存统计
		logRoute.GET("/search", middleware.AdminAuth(), controller.SearchAllLogs)                                           // 搜索所有日志
		logRoute.GET("/self", middleware.UserAuth(), controller.GetUserLogs)                                                // 获取用户日志
		logRoute.GET("/self/search", middleware.UserAuth(), middleware.SearchRateLimit(), controller.SearchUserLogs)        // 搜索用户日志

		// ========================================
		// 数据路由组 - /api/data
		// ========================================
		dataRoute := apiRouter.Group("/data")
		dataRoute.GET("/", middleware.AdminAuth(), controller.GetAllQuotaDates)              // 获取所有配额日期
		dataRoute.GET("/users", middleware.AdminAuth(), controller.GetQuotaDatesByUser)      // 按用户获取配额日期
		dataRoute.GET("/self", middleware.UserAuth(), controller.GetUserQuotaDates)          // 获取用户配额日期
		dataRoute.GET("/flow", middleware.AdminAuth(), controller.GetAllFlowQuotaDates)      // 获取流量账本聚合数据
		dataRoute.GET("/flow/self", middleware.UserAuth(), controller.GetUserFlowQuotaDates) // 获取当前用户流量账本聚合数据

		// Token 日志查询（需要 CORS 和频率限制）
		logRoute.Use(middleware.CORS(), middleware.CriticalRateLimit())
		{
			logRoute.GET("/token", middleware.TokenAuthReadOnly(), controller.GetLogByKey) // 通过 token 获取日志
		}

		// ========================================
		// 分组路由组 - /api/group（需要管理员权限）
		// ========================================
		groupRoute := apiRouter.Group("/group")
		groupRoute.Use(middleware.AdminAuth())
		{
			groupRoute.GET("/", controller.GetGroups) // 获取所有分组
		}

		// ========================================
		// 预填充分组路由组 - /api/prefill_group（需要管理员权限）
		// ========================================
		prefillGroupRoute := apiRouter.Group("/prefill_group")
		prefillGroupRoute.Use(middleware.AdminAuth())
		{
			prefillGroupRoute.GET("/", controller.GetPrefillGroups)         // 获取所有预填充分组
			prefillGroupRoute.POST("/", controller.CreatePrefillGroup)      // 创建预填充分组
			prefillGroupRoute.PUT("/", controller.UpdatePrefillGroup)       // 更新预填充分组
			prefillGroupRoute.DELETE("/:id", controller.DeletePrefillGroup) // 删除预填充分组
		}

		// ========================================
		// Midjourney 路由组 - /api/mj
		// ========================================
		mjRoute := apiRouter.Group("/mj")
		mjRoute.GET("/self", middleware.UserAuth(), controller.GetUserMidjourney) // 获取用户 Midjourney 任务
		mjRoute.GET("/", middleware.AdminAuth(), controller.GetAllMidjourney)     // 获取所有 Midjourney 任务

		// ========================================
		// 任务路由组 - /api/task
		// ========================================
		taskRoute := apiRouter.Group("/task")
		{
			taskRoute.GET("/self", middleware.UserAuth(), controller.GetUserTask) // 获取用户任务
			taskRoute.GET("/", middleware.AdminAuth(), controller.GetAllTask)     // 获取所有任务
		}

		// ========================================
		// 厂商路由组 - /api/vendors（需要管理员权限）
		// ========================================
		vendorRoute := apiRouter.Group("/vendors")
		vendorRoute.Use(middleware.AdminAuth())
		{
			vendorRoute.GET("/", controller.GetAllVendors)          // 获取所有厂商
			vendorRoute.GET("/search", controller.SearchVendors)    // 搜索厂商
			vendorRoute.GET("/:id", controller.GetVendorMeta)       // 获取单个厂商元数据
			vendorRoute.POST("/", controller.CreateVendorMeta)      // 创建厂商元数据
			vendorRoute.PUT("/", controller.UpdateVendorMeta)       // 更新厂商元数据
			vendorRoute.DELETE("/:id", controller.DeleteVendorMeta) // 删除厂商元数据
		}

		// ========================================
		// 模型路由组 - /api/models（需要管理员权限）
		// ========================================
		modelsRoute := apiRouter.Group("/models")
		modelsRoute.Use(middleware.AdminAuth())
		{
			modelsRoute.GET("/sync_upstream/preview", controller.SyncUpstreamPreview) // 预览上游同步
			modelsRoute.POST("/sync_upstream", controller.SyncUpstreamModels)         // 同步上游模型
			modelsRoute.GET("/missing", controller.GetMissingModels)                  // 获取缺失模型
			modelsRoute.GET("/", controller.GetAllModelsMeta)                         // 获取所有模型元数据
			modelsRoute.GET("/search", controller.SearchModelsMeta)                   // 搜索模型元数据
			modelsRoute.GET("/:id/pricing", controller.GetModelPricingConfig)         // 获取单个模型定价配置
			modelsRoute.PUT("/:id/pricing", controller.UpdateModelPricingConfig)      // 更新单个模型定价配置
			modelsRoute.GET("/:id", controller.GetModelMeta)                          // 获取单个模型元数据
			modelsRoute.POST("/", controller.CreateModelMeta)                         // 创建模型元数据
			modelsRoute.PUT("/", controller.UpdateModelMeta)                          // 更新模型元数据
			modelsRoute.DELETE("/:id", controller.DeleteModelMeta)                    // 删除模型元数据
		}

		// ========================================
		// 部署路由组 - /api/deployments（需要管理员权限）
		// 用于管理模型部署
		// ========================================
		deploymentsRoute := apiRouter.Group("/deployments")
		deploymentsRoute.Use(middleware.AdminAuth())
		{
			// 部署设置
			deploymentsRoute.GET("/settings", controller.GetModelDeploymentSettings)           // 获取部署设置
			deploymentsRoute.POST("/settings/test-connection", controller.TestIoNetConnection) // 测试连接

			// 部署管理
			deploymentsRoute.GET("/", controller.GetAllDeployments)                      // 获取所有部署
			deploymentsRoute.GET("/search", controller.SearchDeployments)                // 搜索部署
			deploymentsRoute.POST("/test-connection", controller.TestIoNetConnection)    // 测试连接
			deploymentsRoute.GET("/hardware-types", controller.GetHardwareTypes)         // 获取硬件类型
			deploymentsRoute.GET("/locations", controller.GetLocations)                  // 获取可用区域
			deploymentsRoute.GET("/available-replicas", controller.GetAvailableReplicas) // 获取可用副本数
			deploymentsRoute.POST("/price-estimation", controller.GetPriceEstimation)    // 获取价格估算
			deploymentsRoute.GET("/check-name", controller.CheckClusterNameAvailability) // 检查集群名称可用性
			deploymentsRoute.POST("/", controller.CreateDeployment)                      // 创建部署

			// 单个部署操作
			deploymentsRoute.GET("/:id", controller.GetDeployment)                                // 获取单个部署
			deploymentsRoute.GET("/:id/logs", controller.GetDeploymentLogs)                       // 获取部署日志
			deploymentsRoute.GET("/:id/containers", controller.ListDeploymentContainers)          // 获取部署容器列表
			deploymentsRoute.GET("/:id/containers/:container_id", controller.GetContainerDetails) // 获取容器详情
			deploymentsRoute.PUT("/:id", controller.UpdateDeployment)                             // 更新部署
			deploymentsRoute.PUT("/:id/name", controller.UpdateDeploymentName)                    // 更新部署名称
			deploymentsRoute.POST("/:id/extend", controller.ExtendDeployment)                     // 扩展部署
			deploymentsRoute.DELETE("/:id", controller.DeleteDeployment)                          // 删除部署
		}

	}
}
