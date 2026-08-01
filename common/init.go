// Package common - init.go
// 该文件负责系统初始化和环境变量配置
//
// 初始化流程：
// 1. 解析命令行参数（--port, --version, --help, --log-dir）
// 2. 加载环境变量配置
// 3. 验证会话密钥安全性
// 4. 初始化日志目录
// 5. 配置 TLS 安全选项
// 6. 初始化 Redis 客户端（如果启用）
// 7. 初始化数据库连接
package common

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/constant" // 常量定义
)

// 命令行参数定义
var (
	Port         = flag.Int("port", 3000, "the listening port")                  // 监听端口
	PrintVersion = flag.Bool("version", false, "print version and exit")         // 打印版本
	PrintHelp    = flag.Bool("help", false, "print help and exit")               // 打印帮助
	LogDir       = flag.String("log-dir", "./logs", "specify the log directory") // 日志目录
)

// printHelp 打印帮助信息
func printHelp() {
	fmt.Println("NexusTok (Based on OneAPI) " + Version + " - The next-generation LLM gateway and AI asset management system supports multiple languages.")
	fmt.Println("Original Project: OneAPI by JustSong - https://github.com/songquanpeng/one-api")
	fmt.Println("Maintainer: c1cada - https://github.com/c1cada/NexusTok")
	fmt.Println("Usage: nexustok [--port <port>] [--log-dir <log directory>] [--version] [--help]")
}

// InitEnv 初始化环境配置
// 解析命令行参数和环境变量，设置全局配置
//
// 处理的配置项：
// - VERSION: 版本号（环境变量覆盖）
// - SESSION_SECRET: 会话密钥（必须修改默认值）
// - SESSION_MAX_AGE: 会话最大存活时间
// - CRYPTO_SECRET: 加密密钥
// - SQLITE_PATH: SQLite 数据库路径
// - LOG_DIR: 日志目录
// - DEBUG: 调试模式
// - MEMORY_CACHE_ENABLED: 内存缓存
// - NODE_TYPE: 节点类型（master/slave）
// - NODE_NAME: 节点名称
// - TLS_INSECURE_SKIP_VERIFY: TLS 跳过验证
// - REDIS_CONN_STRING: Redis 连接字符串
// - SQL_DSN: 数据库连接字符串
func InitEnv() {
	flag.Parse()

	envVersion := os.Getenv("VERSION")
	if envVersion != "" {
		Version = envVersion
	}

	if *PrintVersion {
		fmt.Println(Version)
		os.Exit(0)
	}

	if *PrintHelp {
		printHelp()
		os.Exit(0)
	}

	sessionSecret, err := resolveSessionSecret()
	if err != nil {
		if errors.Is(err, errDefaultSessionSecret) {
			log.Println("WARNING: SESSION_SECRET is set to the default value 'random_string', please change it to a random string.")
			log.Println("警告：SESSION_SECRET被设置为默认值'random_string'，请修改为随机字符串。")
			log.Fatal("Please set SESSION_SECRET to a random string.")
		}
		log.Printf("WARNING: failed to load persistent session secret, using in-memory fallback: %v", err)
		log.Printf("警告：无法加载持久化会话密钥，将使用进程内临时密钥：%v", err)
	} else {
		SessionSecret = sessionSecret
	}
	SessionMaxAge = resolveSessionMaxAge(SessionMaxAge)
	if err := InitSessionCookieSettings(); err != nil {
		log.Fatal(err)
	}
	if os.Getenv("CRYPTO_SECRET") != "" {
		CryptoSecret = os.Getenv("CRYPTO_SECRET")
	} else {
		CryptoSecret = SessionSecret
	}
	if os.Getenv("SQLITE_PATH") != "" {
		SQLitePath = os.Getenv("SQLITE_PATH")
	}
	if *LogDir != "" {
		var err error
		*LogDir, err = filepath.Abs(*LogDir)
		if err != nil {
			log.Fatal(err)
		}
		if _, err := os.Stat(*LogDir); os.IsNotExist(err) {
			err = os.Mkdir(*LogDir, 0777)
			if err != nil {
				log.Fatal(err)
			}
		}
	}

	// Initialize variables from constants.go that were using environment variables
	DebugEnabled = os.Getenv("DEBUG") == "true"
	MemoryCacheEnabled = os.Getenv("MEMORY_CACHE_ENABLED") == "true"
	IsMasterNode = os.Getenv("NODE_TYPE") != "slave"
	initNodeNameIdentity()
	TLSInsecureSkipVerify = GetEnvOrDefaultBool("TLS_INSECURE_SKIP_VERIFY", false)
	SMTPStartTLSEnabled = GetEnvOrDefaultBool("SMTP_STARTTLS_ENABLE", GetEnvOrDefaultBool("SMTP_STARTTLS_ENABLED", false))
	SMTPInsecureSkipVerify = GetEnvOrDefaultBool("SMTP_INSECURE_SKIP_VERIFY", GetEnvOrDefaultBool("SMTP_TLS_INSECURE_SKIP_VERIFY", false))
	if TLSInsecureSkipVerify {
		if tr, ok := http.DefaultTransport.(*http.Transport); ok && tr != nil {
			if tr.TLSClientConfig != nil {
				tr.TLSClientConfig.InsecureSkipVerify = true
			} else {
				tr.TLSClientConfig = InsecureTLSConfig
			}
		}
	}

	// Parse requestInterval and set RequestInterval
	requestInterval, _ = strconv.Atoi(os.Getenv("POLLING_INTERVAL"))
	RequestInterval = time.Duration(requestInterval) * time.Second

	// Initialize variables with GetEnvOrDefault
	SyncFrequency = GetEnvOrDefault("SYNC_FREQUENCY", 60)
	BatchUpdateInterval = GetEnvOrDefault("BATCH_UPDATE_INTERVAL", 5)
	RelayTimeout = GetEnvOrDefault("RELAY_TIMEOUT", 0)
	RelayIdleConnTimeout = GetEnvOrDefault("RELAY_IDLE_CONN_TIMEOUT", 90)
	RelayMaxIdleConns = GetEnvOrDefault("RELAY_MAX_IDLE_CONNS", 500)
	RelayMaxIdleConnsPerHost = GetEnvOrDefault("RELAY_MAX_IDLE_CONNS_PER_HOST", 100)
	RelayMaxConnsPerHost = GetEnvOrDefault("RELAY_MAX_CONNS_PER_HOST", 0)
	RelayResponseHeaderTimeout = GetEnvOrDefault("RELAY_RESPONSE_HEADER_TIMEOUT", 0)
	RelayProxyClientCacheTTL = GetEnvOrDefault("RELAY_PROXY_CLIENT_CACHE_TTL", 900)
	RelayProxyClientCacheMaxSize = GetEnvOrDefault("RELAY_PROXY_CLIENT_CACHE_MAX_SIZE", 4096)

	// Initialize string variables with GetEnvOrDefaultString
	GeminiSafetySetting = GetEnvOrDefaultString("GEMINI_SAFETY_SETTING", "BLOCK_NONE")
	CohereSafetySetting = GetEnvOrDefaultString("COHERE_SAFETY_SETTING", "NONE")

	// Initialize rate limit variables
	GlobalApiRateLimitEnable = GetEnvOrDefaultBool("GLOBAL_API_RATE_LIMIT_ENABLE", true)
	GlobalApiRateLimitNum = GetEnvOrDefault("GLOBAL_API_RATE_LIMIT", 180)
	GlobalApiRateLimitDuration = int64(GetEnvOrDefault("GLOBAL_API_RATE_LIMIT_DURATION", 180))

	GlobalWebRateLimitEnable = GetEnvOrDefaultBool("GLOBAL_WEB_RATE_LIMIT_ENABLE", true)
	GlobalWebRateLimitNum = GetEnvOrDefault("GLOBAL_WEB_RATE_LIMIT", 60)
	GlobalWebRateLimitDuration = int64(GetEnvOrDefault("GLOBAL_WEB_RATE_LIMIT_DURATION", 180))

	CriticalRateLimitEnable = GetEnvOrDefaultBool("CRITICAL_RATE_LIMIT_ENABLE", true)
	CriticalRateLimitNum = GetEnvOrDefault("CRITICAL_RATE_LIMIT", 20)
	CriticalRateLimitDuration = int64(GetEnvOrDefault("CRITICAL_RATE_LIMIT_DURATION", 20*60))

	SearchRateLimitEnable = GetEnvOrDefaultBool("SEARCH_RATE_LIMIT_ENABLE", true)
	SearchRateLimitNum = GetEnvOrDefault("SEARCH_RATE_LIMIT", 10)
	SearchRateLimitDuration = int64(GetEnvOrDefault("SEARCH_RATE_LIMIT_DURATION", 60))
	initConstantEnv()
}

func initConstantEnv() {
	constant.StreamingTimeout = GetEnvOrDefault("STREAMING_TIMEOUT", 300)
	constant.DifyDebug = GetEnvOrDefaultBool("DIFY_DEBUG", true)
	constant.MaxFileDownloadMB = GetEnvOrDefault("MAX_FILE_DOWNLOAD_MB", 64)
	constant.StreamScannerMaxBufferMB = GetEnvOrDefault("STREAM_SCANNER_MAX_BUFFER_MB", 128)
	// MaxRequestBodyMB 请求体最大大小（解压后），用于防止超大请求/zip bomb导致内存暴涨
	constant.MaxRequestBodyMB = GetEnvOrDefault("MAX_REQUEST_BODY_MB", 128)
	// AnonymousRequestBodyLimitKB 限制匿名 POST 入口的请求体大小，避免登录、注册、Webhook 等入口被超大请求压垮。
	constant.AnonymousRequestBodyLimitKB = GetEnvOrDefault("ANONYMOUS_REQUEST_BODY_LIMIT_KB", 512)
	// ForceStreamOption 覆盖请求参数，强制返回usage信息
	constant.ForceStreamOption = GetEnvOrDefaultBool("FORCE_STREAM_OPTION", true)
	constant.CountToken = GetEnvOrDefaultBool("CountToken", true)
	constant.GetMediaToken = GetEnvOrDefaultBool("GET_MEDIA_TOKEN", true)
	constant.GetMediaTokenNotStream = GetEnvOrDefaultBool("GET_MEDIA_TOKEN_NOT_STREAM", false)
	constant.UpdateTask = GetEnvOrDefaultBool("UPDATE_TASK", true)
	constant.AzureDefaultAPIVersion = GetEnvOrDefaultString("AZURE_DEFAULT_API_VERSION", "2025-04-01-preview")
	constant.NotifyLimitCount = GetEnvOrDefault("NOTIFY_LIMIT_COUNT", 2)
	constant.NotificationLimitDurationMinute = GetEnvOrDefault("NOTIFICATION_LIMIT_DURATION_MINUTE", 10)
	// GenerateDefaultToken 是否生成初始令牌，默认关闭。
	constant.GenerateDefaultToken = GetEnvOrDefaultBool("GENERATE_DEFAULT_TOKEN", false)
	// 是否启用错误日志
	constant.ErrorLogEnabled = GetEnvOrDefaultBool("ERROR_LOG_ENABLED", false)
	// 任务轮询时查询的最大数量
	constant.TaskQueryLimit = GetEnvOrDefault("TASK_QUERY_LIMIT", 1000)
	// 异步任务超时时间（分钟），超过此时间未完成的任务将被标记为失败并退款。0 表示禁用。
	constant.TaskTimeoutMinutes = GetEnvOrDefault("TASK_TIMEOUT_MINUTES", 1440)

	soraPatchStr := GetEnvOrDefaultString("TASK_PRICE_PATCH", "")
	if soraPatchStr != "" {
		var taskPricePatches []string
		soraPatches := strings.Split(soraPatchStr, ",")
		for _, patch := range soraPatches {
			trimmedPatch := strings.TrimSpace(patch)
			if trimmedPatch != "" {
				taskPricePatches = append(taskPricePatches, trimmedPatch)
			}
		}
		constant.TaskPricePatches = taskPricePatches
	}

	// Initialize trusted redirect domains for URL validation
	trustedDomainsStr := GetEnvOrDefaultString("TRUSTED_REDIRECT_DOMAINS", "")
	var trustedDomains []string
	domains := strings.Split(trustedDomainsStr, ",")
	for _, domain := range domains {
		trimmedDomain := strings.TrimSpace(domain)
		if trimmedDomain != "" {
			// Normalize domain to lowercase
			trustedDomains = append(trustedDomains, strings.ToLower(trimmedDomain))
		}
	}
	constant.TrustedRedirectDomains = trustedDomains
}
