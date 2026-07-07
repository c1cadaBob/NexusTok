// Package main 是 NexusTok 应用程序的入口包
// NexusTok 是一个大模型网关与 AI 资产管理系统，提供统一的 API 网关、
// 用户管理、计费、限流和管理后台等功能
package main

import (
	"bytes"    // 字节操作，用于处理嵌入的静态资源
	"embed"    // Go 1.16+ 嵌入文件系统功能
	"fmt"      // 格式化输出
	"log"      // 日志包
	"net/http" // HTTP 服务
	"os"       // 操作系统功能，环境变量读取
	"strconv"  // 字符串转换
	"strings"  // 字符串操作
	"time"     // 时间处理

	// 项目内部包导入
	"github.com/c1cada/NexusTok/common"                        // 公共工具包：配置、缓存、Redis、日志等
	"github.com/c1cada/NexusTok/constant"                      // 常量定义：API 类型、渠道类型等
	"github.com/c1cada/NexusTok/controller"                    // 控制器层：请求处理器
	"github.com/c1cada/NexusTok/i18n"                          // 国际化支持
	"github.com/c1cada/NexusTok/logger"                        // 日志配置
	"github.com/c1cada/NexusTok/middleware"                    // 中间件：认证、限流、CORS 等
	"github.com/c1cada/NexusTok/model"                         // 数据模型层：GORM ORM
	"github.com/c1cada/NexusTok/oauth"                         // OAuth 认证实现
	perfmetrics "github.com/c1cada/NexusTok/pkg/perf_metrics"  // 性能监控指标
	"github.com/c1cada/NexusTok/relay"                         // AI API 中继/代理层
	"github.com/c1cada/NexusTok/router"                        // 路由配置
	"github.com/c1cada/NexusTok/service"                       // 服务层：业务逻辑
	_ "github.com/c1cada/NexusTok/setting/performance_setting" // 性能设置（init 自动加载）
	"github.com/c1cada/NexusTok/setting/ratio_setting"         // 比率/定价设置

	// 第三方依赖包
	"github.com/bytedance/gopkg/util/gopool" // 字节跳动高性能协程池
	"github.com/gin-contrib/sessions"        // Gin 会话管理
	"github.com/gin-contrib/sessions/cookie" // 基于 Cookie 的会话存储
	"github.com/gin-gonic/gin"               // Gin Web 框架
	"github.com/joho/godotenv"               // .env 文件加载

	_ "net/http/pprof" // 性能分析工具（通过环境变量 ENABLE_PPROF 启用）
)

// 前端静态资源嵌入
// 使用 Go 1.16 的 embed 功能将前端构建产物嵌入到二进制文件中
// 这样部署时无需额外的前端文件，简化部署流程

//go:embed web/default/dist
var buildFS embed.FS // 默认主题的完整前端资源文件系统

//go:embed web/default/dist/index.html
var indexPage []byte // 默认主题的入口 HTML 页面（用于 SPA 路由回退）

//go:embed web/classic/dist
var classicBuildFS embed.FS // 经典主题的完整前端资源文件系统

//go:embed web/classic/dist/index.html
var classicIndexPage []byte // 经典主题的入口 HTML 页面

// main 是应用程序的入口函数
// 负责初始化所有资源、启动后台任务、配置 HTTP 服务器并监听端口
func main() {
	// 记录启动时间，用于计算启动耗时
	startTime := time.Now()

	// 初始化所有系统资源
	// 包括：环境变量、数据库、Redis、缓存、i18n、OAuth 等
	err := InitResources()
	if err != nil {
		// 初始化失败时记录致命日志并退出
		common.FatalLog("failed to initialize resources: " + err.Error())
		return
	}

	// 输出系统启动日志，包含版本号
	common.SysLog("NexusTok " + common.Version + " started")

	// 根据环境变量设置 Gin 运行模式
	// 非 debug 模式时使用 release 模式，减少日志输出提升性能
	if os.Getenv("GIN_MODE") != "debug" {
		gin.SetMode(gin.ReleaseMode)
	}

	// 如果启用了调试模式，输出调试日志
	if common.DebugEnabled {
		common.SysLog("running in debug mode")
	}

	// 延迟关闭数据库连接
	// 确保程序退出时正确释放数据库资源
	defer func() {
		err := model.CloseDB()
		if err != nil {
			common.FatalLog("failed to close database: " + err.Error())
		}
	}()

	// Redis 启用时自动启用内存缓存（兼容旧版本配置）
	if common.RedisEnabled {
		// for compatibility with old versions
		common.MemoryCacheEnabled = true
	}

	// 内存缓存初始化
	if common.MemoryCacheEnabled {
		common.SysLog("memory cache enabled")
		common.SysLog(fmt.Sprintf("sync frequency: %d seconds", common.SyncFrequency))

		// 初始化渠道缓存，带有 panic 恢复和重试机制
		// 防止缓存初始化失败导致整个服务无法启动
		func() {
			defer func() {
				if r := recover(); r != nil {
					common.SysLog(fmt.Sprintf("InitChannelCache panic: %v, retrying once", r))
					// 重试一次：修复渠道能力数据
					_, _, fixErr := model.FixAbility()
					if fixErr != nil {
						common.FatalLog(fmt.Sprintf("InitChannelCache failed: %s", fixErr.Error()))
					}
				}
			}()
			// 初始化渠道缓存，将数据库中的渠道数据加载到内存
			model.InitChannelCache()
		}()

		// 启动后台协程，定期同步渠道缓存
		// 确保缓存与数据库保持一致
		go model.SyncChannelCache(common.SyncFrequency)
	}

	// 启动后台协程，热更新系统配置选项
	// 定期从数据库加载最新配置，无需重启服务
	go model.SyncOptions(common.SyncFrequency)

	// 启动后台协程，更新配额数据看板
	// 用于前端展示实时的使用量统计
	go model.UpdateQuotaData()

	// 如果设置了渠道更新频率环境变量，启动自动更新渠道任务
	if os.Getenv("CHANNEL_UPDATE_FREQUENCY") != "" {
		frequency, err := strconv.Atoi(os.Getenv("CHANNEL_UPDATE_FREQUENCY"))
		if err != nil {
			common.FatalLog("failed to parse CHANNEL_UPDATE_FREQUENCY: " + err.Error())
		}
		// 启动自动更新渠道的后台任务
		go controller.AutomaticallyUpdateChannels(frequency)
	}

	// 启动 Codex 凭证自动刷新任务
	// 每 10 分钟检查一次，凭证将在 1 天内过期时自动刷新
	service.StartCodexCredentialAutoRefreshTask()

	// 启动账号池凭证自动刷新任务
	service.StartAccountPoolCredentialAutoRefreshTask()

	// 恢复服务重启前遗留的账号池后台检测任务
	service.StartPoolAccountCheckTaskRecovery()

	// 启动账号池自动可用性检测任务
	service.StartAccountPoolAutoCheckTask()

	// 启动订阅配额重置任务
	// 支持每日、每周、每月、自定义周期的配额重置
	service.StartSubscriptionQuotaResetTask()

	// 设置任务轮询适配器工厂函数
	// 打破 service -> relay 的导入循环依赖
	service.GetTaskAdaptorFunc = func(platform constant.TaskPlatform) service.TaskPollingAdaptor {
		a := relay.GetTaskAdaptor(platform)
		if a == nil {
			return nil
		}
		return a
	}

	// 启动 models.dev 模型目录每日同步任务
	// 每天凌晨只补齐本地缺失的模型和供应商，不覆盖管理员手动编辑
	controller.StartModelsDevSyncTask()

	// 如果是主节点且需要更新任务，启动批量更新协程
	if common.IsMasterNode && constant.UpdateTask {
		// 使用字节跳动的高性能协程池执行 Midjourney 任务批量更新
		gopool.Go(func() {
			controller.UpdateMidjourneyTaskBulk()
		})
		// 使用协程池执行通用任务批量更新
		gopool.Go(func() {
			controller.UpdateTaskBulk()
		})
	}

	// 如果启用了批量更新功能
	if os.Getenv("BATCH_UPDATE_ENABLED") == "true" {
		common.BatchUpdateEnabled = true
		common.SysLog("batch update enabled with interval " + strconv.Itoa(common.BatchUpdateInterval) + "s")
		// 初始化批量更新器
		model.InitBatchUpdater()
	}

	// 如果启用了 pprof 性能分析
	if os.Getenv("ENABLE_PPROF") == "true" {
		// 在 8005 端口启动 pprof HTTP 服务
		gopool.Go(func() {
			log.Println(http.ListenAndServe("0.0.0.0:8005", nil))
		})
		// 启动系统监控
		go common.Monitor()
		common.SysLog("pprof enabled")
	}

	// 启动 Pyroscope 性能分析（如果配置了）
	// Pyroscope 是一个持续性能分析平台
	err = common.StartPyroScope()
	if err != nil {
		common.SysError(fmt.Sprintf("start pyroscope error : %v", err))
	}

	// ========================================
	// 初始化 HTTP 服务器
	// ========================================

	// 创建 Gin 引擎实例
	server := gin.New()

	// 配置全局 panic 恢复中间件
	// 捕获所有未处理的 panic，返回友好的错误信息
	server.Use(gin.CustomRecovery(func(c *gin.Context, err any) {
		common.SysLog(fmt.Sprintf("panic detected: %v", err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Panic detected, error: %v. Please submit a issue here: https://github.com/c1cada/NexusTok", err),
				"type":    "nexustok_panic",
			},
		})
	}))

	// 注意：不能启用 gzip 压缩，否则会导致 SSE（Server-Sent Events）无法正常工作
	//server.Use(gzip.Gzip(gzip.DefaultCompression))

	// 注册请求 ID 中间件
	// 为每个请求生成唯一标识符，便于日志追踪和问题排查
	server.Use(middleware.RequestId())

	// 注册 Powered-By 响应头中间件
	server.Use(middleware.PoweredBy())

	// 注册国际化中间件
	// 根据请求头 Accept-Language 或查询参数设置语言
	server.Use(middleware.I18n())

	// 设置日志中间件
	// 记录请求日志，包括请求方法、路径、状态码、耗时等
	middleware.SetUpLogger(server)

	// 初始化会话存储
	// 使用 Cookie 存储会话数据，配置安全选项
	store := cookie.NewStore([]byte(common.SessionSecret))
	store.Options(sessions.Options{
		Path:     "/",                     // Cookie 路径
		MaxAge:   common.SessionMaxAge,    // 会话过期时间（秒）
		HttpOnly: true,                    // 仅 HTTP 访问，防止 XSS
		Secure:   false,                   // 是否仅 HTTPS（生产环境应设为 true）
		SameSite: http.SameSiteStrictMode, // SameSite 策略，防止 CSRF
	})
	// 注册会话中间件，会话名称为 "session"
	server.Use(sessions.Sessions("session", store))

	// 注入 Umami 网站分析脚本
	// Umami 是一个开源的网站分析工具
	InjectUmamiAnalytics()

	// 注入 Google Analytics 分析脚本
	InjectGoogleAnalytics()

	// 设置路由
	// 将所有 API 路由、静态文件路由注册到 Gin 引擎
	router.SetRouter(server, router.ThemeAssets{
		DefaultBuildFS:   buildFS,          // 默认主题资源
		DefaultIndexPage: indexPage,        // 默认主题入口页
		ClassicBuildFS:   classicBuildFS,   // 经典主题资源
		ClassicIndexPage: classicIndexPage, // 经典主题入口页
	})

	// 获取服务端口
	// 优先使用环境变量 PORT，否则使用配置文件中的端口
	var port = os.Getenv("PORT")
	if port == "" {
		port = strconv.Itoa(*common.Port)
	}

	// 输出启动成功日志
	common.LogStartupSuccess(startTime, port)

	// 启动 HTTP 服务器，监听指定端口
	err = server.Run(":" + port)
	if err != nil {
		// 服务器启动失败时记录致命日志
		common.FatalLog("failed to start HTTP server: " + err.Error())
	}
}

// InjectUmamiAnalytics 注入 Umami 网站分析脚本到前端页面
// Umami 是一个开源、隐私友好的网站分析工具
// 该函数将 Umami 的跟踪脚本注入到 index.html 中
// 通过替换 HTML 中的占位符实现动态注入
func InjectUmamiAnalytics() {
	// 使用 strings.Builder 构建分析脚本 HTML
	analyticsInjectBuilder := &strings.Builder{}

	// 检查是否配置了 Umami 网站 ID
	if os.Getenv("UMAMI_WEBSITE_ID") != "" {
		umamiSiteID := os.Getenv("UMAMI_WEBSITE_ID")
		umamiScriptURL := os.Getenv("UMAMI_SCRIPT_URL")

		// 如果未配置自定义脚本 URL，使用 Umami 官方默认地址
		if umamiScriptURL == "" {
			umamiScriptURL = "https://analytics.umami.is/script.js"
		}

		// 构建 Umami 跟踪脚本的 HTML 标签
		// defer 属性确保脚本在页面解析完成后执行
		analyticsInjectBuilder.WriteString("<script defer src=\"")
		analyticsInjectBuilder.WriteString(umamiScriptURL)
		analyticsInjectBuilder.WriteString("\" data-website-id=\"")
		analyticsInjectBuilder.WriteString(umamiSiteID)
		analyticsInjectBuilder.WriteString("\"></script>")
	}

	// 添加 Umami 注释标记（用于标识注入位置）
	analyticsInjectBuilder.WriteString("<!--Umami NexusTok-->\n")

	// 将构建的 HTML 转换为字节数组
	analyticsInject := []byte(analyticsInjectBuilder.String())

	// 定义占位符，用于在 HTML 中定位注入点
	placeholder := []byte("<!--umami-->\n")

	// 将分析脚本注入到默认主题和经典主题的 index.html 中
	indexPage = bytes.ReplaceAll(indexPage, placeholder, analyticsInject)
	classicIndexPage = bytes.ReplaceAll(classicIndexPage, placeholder, analyticsInject)
}

// InjectGoogleAnalytics 注入 Google Analytics 4 (GA4) 分析脚本到前端页面
// Google Analytics 是 Google 提供的网站流量分析服务
// 该函数将 GA4 的 gtag.js 跟踪代码注入到 index.html 中
func InjectGoogleAnalytics() {
	// 使用 strings.Builder 构建分析脚本 HTML
	analyticsInjectBuilder := &strings.Builder{}

	// 检查是否配置了 Google Analytics ID
	if os.Getenv("GOOGLE_ANALYTICS_ID") != "" {
		gaID := os.Getenv("GOOGLE_ANALYTICS_ID")

		// 构建 Google Analytics 4 (gtag.js) 跟踪脚本
		// async 属性确保脚本异步加载，不阻塞页面渲染
		analyticsInjectBuilder.WriteString("<script async src=\"https://www.googletagmanager.com/gtag/js?id=")
		analyticsInjectBuilder.WriteString(gaID)
		analyticsInjectBuilder.WriteString("\"></script>")

		// 构建 gtag 初始化代码
		analyticsInjectBuilder.WriteString("<script>")
		// 初始化 dataLayer 数组（GA4 的数据层）
		analyticsInjectBuilder.WriteString("window.dataLayer = window.dataLayer || [];")
		// 定义 gtag 函数，将参数推送到 dataLayer
		analyticsInjectBuilder.WriteString("function gtag(){dataLayer.push(arguments);}")
		// 记录页面加载时间
		analyticsInjectBuilder.WriteString("gtag('js', new Date());")
		// 配置 GA4，传入跟踪 ID
		analyticsInjectBuilder.WriteString("gtag('config', '")
		analyticsInjectBuilder.WriteString(gaID)
		analyticsInjectBuilder.WriteString("');")
		analyticsInjectBuilder.WriteString("</script>")
	}

	// 添加 Google Analytics 注释标记（用于标识注入位置）
	analyticsInjectBuilder.WriteString("<!--Google Analytics NexusTok-->\n")

	// 将构建的 HTML 转换为字节数组
	analyticsInject := []byte(analyticsInjectBuilder.String())

	// 定义占位符，用于在 HTML 中定位注入点
	placeholder := []byte("<!--Google Analytics-->\n")

	// 将分析脚本注入到默认主题和经典主题的 index.html 中
	indexPage = bytes.ReplaceAll(indexPage, placeholder, analyticsInject)
	classicIndexPage = bytes.ReplaceAll(classicIndexPage, placeholder, analyticsInject)
}

// InitResources 初始化应用程序所需的所有资源
// 按照依赖顺序依次初始化各个组件：
// 1. 环境变量和日志
// 2. 数据库连接
// 3. 系统配置
// 4. 缓存（Redis/内存）
// 5. 国际化
// 6. OAuth 提供商
//
// 返回值：
//   - error: 初始化过程中的错误，成功返回 nil
func InitResources() error {
	// 加载 .env 文件中的环境变量
	// 如果 .env 文件不存在，使用系统环境变量
	err := godotenv.Load(".env")
	if err != nil {
		// 仅在调试模式下输出警告，不影响正常启动
		if common.DebugEnabled {
			common.SysLog("No .env file found, using default environment variables. If needed, please create a .env file and set the relevant variables.")
		}
	}

	// 初始化环境变量配置
	// 从环境变量读取并设置各种系统配置项
	common.InitEnv()

	// 设置日志系统
	// 配置日志输出格式、级别和目标
	logger.SetupLogger()

	// 初始化模型比率设置
	// 加载不同 AI 模型的定价比率配置
	ratio_setting.InitRatioSettings()

	// 初始化 HTTP 客户端
	// 配置超时、代理等选项，用于调用上游 AI API
	service.InitHttpClient()

	// 初始化 Token 编码器
	// 加载 BPE 等分词器，用于计算 token 数量
	service.InitTokenEncoders()

	// 初始化主数据库连接
	// 支持 SQLite、MySQL、PostgreSQL 三种数据库
	err = model.InitDB()
	if err != nil {
		common.FatalLog("failed to initialize database: " + err.Error())
		return err
	}

	// 检查数据库设置状态
	// 如果是首次运行，可能需要初始化表结构
	model.CheckSetup()

	// 初始化系统选项配置
	// 必须在数据库初始化之后执行
	// 从数据库加载所有配置项到内存
	model.InitOptionMap()

	// 清理旧的磁盘缓存文件
	// 删除过期的缓存文件，释放磁盘空间
	common.CleanupOldCacheFiles()

	// 初始化模型定价数据
	// 从数据库或配置文件加载各模型的定价信息
	model.GetPricing()

	// 初始化日志数据库连接
	// 日志可能使用单独的数据库，与主数据库分离
	err = model.InitLogDB()
	if err != nil {
		return err
	}

	// 初始化 Redis 客户端连接
	// Redis 用于分布式缓存、会话存储、限流计数器等
	err = common.InitRedisClient()
	if err != nil {
		return err
	}

	// 初始化性能监控指标
	perfmetrics.Init()

	// 启动系统监控
	// 监控 CPU、内存、磁盘等系统资源使用情况
	common.StartSystemMonitor()

	// 启动系统实例心跳上报
	// 用于 Root 查看当前部署节点、主从角色、版本和资源快照
	service.StartSystemInstanceReporter()

	// 启动系统任务执行器
	// 用于异步执行日志清理等耗时后台任务，并通过数据库租约避免多节点重复执行。
	service.StartSystemTaskRunner()

	// 初始化国际化 (i18n) 支持
	// 加载多语言翻译文件，配置语言检测
	err = i18n.Init()
	if err != nil {
		// i18n 初始化失败不是致命错误，记录警告继续运行
		common.SysError("failed to initialize i18n: " + err.Error())
		// Don't return error, i18n is not critical
	} else {
		// 输出支持的语言列表
		common.SysLog("i18n initialized with languages: " + strings.Join(i18n.SupportedLanguages(), ", "))
	}

	// 注册用户语言加载器（延迟加载）
	// 当需要获取用户语言偏好时，从数据库查询
	i18n.SetUserLangLoader(model.GetUserLanguage)

	// 从数据库加载自定义 OAuth 提供商配置
	// 支持动态添加 OAuth 登录方式（如 GitHub、Discord、OIDC 等）
	err = oauth.LoadCustomProviders()
	if err != nil {
		// OAuth 加载失败不是致命错误，记录警告继续运行
		common.SysError("failed to load custom OAuth providers: " + err.Error())
		// Don't return error, custom OAuth is not critical
	}

	// 所有资源初始化完成，返回 nil 表示成功
	return nil
}
