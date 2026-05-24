// Package common - constants.go
// 该文件定义了系统的全局常量和配置变量
// 包括系统信息、功能开关、配额配置、会话配置等
package common

import (
	"crypto/tls"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// ========================================
// 系统信息
// ========================================

var StartTime = time.Now().Unix() // 系统启动时间（Unix 时间戳，秒）
var Version = "v0.0.0"            // 系统版本号（构建时自动替换，无需手动修改）
var SystemName = "NexusTok"       // 系统名称
var Footer = ""                   // 页脚文本
var Logo = ""                     // Logo URL
var TopUpLink = ""                // 充值链接

// ========================================
// 前端主题配置
// ========================================

// themeValue 存储当前前端主题（原子操作，支持并发读写）
var themeValue atomic.Value

// init 初始化默认主题为 "classic"
func init() {
	themeValue.Store("classic")
}

// GetTheme 获取当前前端主题
//
// 返回值：
//   - string: 主题名称（"default" 或 "classic"）
func GetTheme() string {
	return themeValue.Load().(string)
}

// SetTheme 更新前端主题（原子操作）
// 只接受 "default" 和 "classic" 两个值，其他值会被忽略
//
// 参数：
//   - t: 主题名称
func SetTheme(t string) {
	if t == "default" || t == "classic" {
		themeValue.Store(t)
	}
}

// ThemeAwarePath 主题感知路径重写
// 当使用 "default" 主题时，将旧的 /console/* 路径重写为新路径
// 对于 "classic" 主题，路径保持不变
//
// 路径映射：
// - /console/topup -> /wallet
// - /console/log -> /usage-logs
// - /console/personal -> /profile
//
// 参数：
//   - suffix: 原始路径
//
// 返回值：
//   - string: 重写后的路径
func ThemeAwarePath(suffix string) string {
	if GetTheme() != "default" {
		return suffix
	}

	switch {
	case strings.HasPrefix(suffix, "/console/topup"):
		return strings.Replace(suffix, "/console/topup", "/wallet", 1)
	case strings.HasPrefix(suffix, "/console/log"):
		return strings.Replace(suffix, "/console/log", "/usage-logs", 1)
	case strings.HasPrefix(suffix, "/console/personal"):
		return strings.Replace(suffix, "/console/personal", "/profile", 1)
	}

	return suffix
}

// ========================================
// 功能开关和配额配置
// ========================================

var QuotaPerUnit = 500 * 1000.0 // 每单位配额对应的 token 数量（$0.002 / 1K tokens），用于配额和金额的换算

// 保留旧变量以兼容历史逻辑，实际展示由 general_setting.quota_display_type 控制
var DisplayInCurrencyEnabled = true  // 是否以货币形式显示配额（如 $1.00）
var DisplayTokenStatEnabled = true   // 是否显示 token 统计信息
var DrawingEnabled = true            // 是否启用绘图功能（DALL-E、Midjourney 等）
var TaskEnabled = true               // 是否启用任务功能（异步任务如 Midjourney、Suno 等）
var DataExportEnabled = true         // 是否启用数据导出功能
var DataExportInterval = 5           // 数据导出间隔（分钟）
var DataExportDefaultTime = "hour"   // 默认导出时间范围（hour/day/week/month）
var DefaultCollapseSidebar = false   // 默认是否折叠侧边栏

// 注意：包含 "Secret" 或 "Token" 的选项不会通过 GetOptions 返回（安全考虑）

// ========================================
// 会话和加密配置
// ========================================

var SessionSecret = uuid.New().String()  // 会话密钥（随机生成，用于 JWT 签名等）
var CryptoSecret = uuid.New().String()   // 加密密钥（随机生成，用于 HMAC 等加密操作）
var SessionMaxAge = 90 * 24 * 60 * 60   // 会话最大存活时间（秒，默认 90 天）

// ========================================
// 全局配置映射
// ========================================

var OptionMap map[string]string    // 全局选项映射（从数据库加载的系统配置）
var OptionMapRWMutex sync.RWMutex // 选项映射读写锁（保护并发访问）

var ItemsPerPage = 10    // 每页显示的条目数（分页参数）
var MaxRecentItems = 1000 // 最大最近条目数

// ── 认证和注册开关 ──────────────────────────────────────────────────
var PasswordLoginEnabled = true       // 是否启用密码登录
var PasswordRegisterEnabled = true    // 是否启用密码注册
var EmailVerificationEnabled = false  // 是否启用邮箱验证
var GitHubOAuthEnabled = false        // 是否启用 GitHub OAuth 登录
var LinuxDOOAuthEnabled = false       // 是否启用 LinuxDO OAuth 登录
var WeChatAuthEnabled = false         // 是否启用微信登录
var TelegramOAuthEnabled = false      // 是否启用 Telegram 登录
var TurnstileCheckEnabled = false     // 是否启用 Cloudflare Turnstile 验证
var RegisterEnabled = true            // 是否启用注册功能

// ── 邮箱限制配置 ────────────────────────────────────────────────────
var EmailDomainRestrictionEnabled = false // 是否启用邮箱域名限制（只允许白名单域名注册）
var EmailAliasRestrictionEnabled = false  // 是否启用邮箱别名限制（禁止使用 + 别名）
var EmailDomainWhitelist = []string{      // 允许注册的邮箱域名白名单
	"gmail.com",
	"163.com",
	"126.com",
	"qq.com",
	"outlook.com",
	"hotmail.com",
	"icloud.com",
	"yahoo.com",
	"foxmail.com",
}
var EmailLoginAuthServerList = []string{ // 邮箱登录认证服务器列表
	"smtp.sendcloud.net",
	"smtp.azurecomm.net",
}

var DebugEnabled bool        // 是否启用调试模式（输出详细日志）
var MemoryCacheEnabled bool  // 是否启用内存缓存

var LogConsumeEnabled = true // 是否启用消费日志记录

var TLSInsecureSkipVerify bool                              // 是否跳过 TLS 证书验证
var InsecureTLSConfig = &tls.Config{InsecureSkipVerify: true} // 不安全的 TLS 配置（用于测试或自签名证书）

// ── SMTP 邮件配置 ───────────────────────────────────────────────────
var SMTPServer = ""           // SMTP 服务器地址
var SMTPPort = 587            // SMTP 端口（默认 587，SSL 时为 465）
var SMTPSSLEnabled = false    // 是否启用 SMTP SSL
var SMTPForceAuthLogin = false // 是否强制 SMTP AUTH LOGIN 认证
var SMTPAccount = ""          // SMTP 账号
var SMTPFrom = ""             // SMTP 发件人地址
var SMTPToken = ""            // SMTP 密码/令牌

// ── OAuth 配置 ──────────────────────────────────────────────────────
var GitHubClientId = ""         // GitHub OAuth Client ID
var GitHubClientSecret = ""     // GitHub OAuth Client Secret
var LinuxDOClientId = ""        // LinuxDO OAuth Client ID
var LinuxDOClientSecret = ""    // LinuxDO OAuth Client Secret
var LinuxDOMinimumTrustLevel = 0 // LinuxDO 最低信任等级要求

// ── 微信登录配置 ────────────────────────────────────────────────────
var WeChatServerAddress = ""           // 微信服务器地址
var WeChatServerToken = ""             // 微信服务器令牌
var WeChatAccountQRCodeImageURL = ""   // 微信公众号二维码图片 URL

// ── Cloudflare Turnstile 配置 ───────────────────────────────────────
var TurnstileSiteKey = ""   // Turnstile 站点密钥（前端使用）
var TurnstileSecretKey = "" // Turnstile 密钥（后端验证使用）

// ── Telegram 登录配置 ───────────────────────────────────────────────
var TelegramBotToken = "" // Telegram Bot Token
var TelegramBotName = ""  // Telegram Bot 用户名

// ── 配额和渠道配置 ──────────────────────────────────────────────────
var QuotaForNewUser = 0                   // 新用户注册赠送配额
var QuotaForInviter = 0                   // 邀请人获得的配额奖励
var QuotaForInvitee = 0                   // 被邀请人获得的配额奖励
var ChannelDisableThreshold = 5.0         // 渠道自动禁用阈值（错误率超过此值自动禁用）
var AutomaticDisableChannelEnabled = false // 是否启用渠道自动禁用
var AutomaticEnableChannelEnabled = false  // 是否启用渠道自动启用（恢复后自动启用）
var QuotaRemindThreshold = 1000           // 配额提醒阈值（低于此值发送提醒）
var PreConsumedQuota = 500                // 预扣配额（流式请求开始时预扣）

var RetryTimes = 0 // 请求失败重试次数（0 表示不重试）

//var RootUserEmail = ""

var IsMasterNode bool // 是否为主节点（多节点部署时标识主节点）

// NodeName 节点名称，从 NODE_NAME 环境变量读取；
// 用于审计日志中标识节点身份，在容器/K8s 部署时比自动探测到的容器内网 IP 更具可读性。
var NodeName = ""

var requestInterval int           // 请求间隔（内部使用）
var RequestInterval time.Duration // 请求间隔（时间间隔格式）

var SyncFrequency int // 同步频率（秒），用于缓存过期和数据同步

var BatchUpdateEnabled = false // 是否启用批量更新
var BatchUpdateInterval int    // 批量更新间隔（秒）

var RelayTimeout int // 中继请求超时时间（秒）

var RelayMaxIdleConns int        // HTTP 客户端最大空闲连接数
var RelayMaxIdleConnsPerHost int // HTTP 客户端每主机最大空闲连接数

var GeminiSafetySetting string // Gemini 安全设置（JSON 格式）

// https://docs.cohere.com/docs/safety-modes Type; NONE/CONTEXTUAL/STRICT
var CohereSafetySetting string // Cohere 安全设置（NONE/CONTEXTUAL/STRICT）

const (
	RequestIdKey         = "X-Oneapi-Request-Id"   // 请求 ID 键（存储在 Gin 上下文中）
	UpstreamRequestIdKey = "X-Upstream-Request-Id"  // 上游请求 ID 键（从上游响应头提取）
)

// 用户角色常量
const (
	RoleGuestUser  = 0   // 访客用户
	RoleCommonUser = 1   // 普通用户
	RoleAdminUser  = 10  // 管理员
	RoleRootUser   = 100 // 超级管理员
)

// IsValidateRole 验证角色是否有效
//
// 参数：
//   - role: 角色值
//
// 返回值：
//   - bool: 是否为有效的角色值
func IsValidateRole(role int) bool {
	return role == RoleGuestUser || role == RoleCommonUser || role == RoleAdminUser || role == RoleRootUser
}

var (
	FileUploadPermission    = RoleGuestUser // 文件上传权限（默认访客即可）
	FileDownloadPermission  = RoleGuestUser // 文件下载权限
	ImageUploadPermission   = RoleGuestUser // 图片上传权限
	ImageDownloadPermission = RoleGuestUser // 图片下载权限
)

// 限流配置（时间单位为秒）
// 不应大于 RateLimitKeyExpirationDuration
var (
	GlobalApiRateLimitEnable   bool // 是否启用全局 API 限流
	GlobalApiRateLimitNum      int  // 全局 API 限流数量（请求数）
	GlobalApiRateLimitDuration int64 // 全局 API 限流时间窗口（秒）

	GlobalWebRateLimitEnable   bool // 是否启用全局 Web 限流
	GlobalWebRateLimitNum      int  // 全局 Web 限流数量
	GlobalWebRateLimitDuration int64 // 全局 Web 限流时间窗口（秒）

	CriticalRateLimitEnable   bool      // 是否启用关键操作限流
	CriticalRateLimitNum            = 20 // 关键操作限流数量
	CriticalRateLimitDuration int64 = 20 * 60 // 关键操作限流时间窗口（20 分钟）

	UploadRateLimitNum            = 10 // 上传限流数量
	UploadRateLimitDuration int64 = 60 // 上传限流时间窗口（60 秒）

	DownloadRateLimitNum            = 10 // 下载限流数量
	DownloadRateLimitDuration int64 = 60 // 下载限流时间窗口（60 秒）

	// Per-user search rate limit (applies after authentication, keyed by user ID)
	SearchRateLimitEnable         = true // 是否启用搜索限流
	SearchRateLimitNum            = 10   // 搜索限流数量
	SearchRateLimitDuration int64 = 60   // 搜索限流时间窗口（60 秒）
)

var RateLimitKeyExpirationDuration = 20 * time.Minute // 限流 Key 过期时间（20 分钟）

// 用户状态常量
const (
	UserStatusEnabled  = 1 // 用户状态：启用（不要使用 0，0 是默认值！）
	UserStatusDisabled = 2 // 用户状态：禁用（也不要使用 0）
)

// Token 状态常量
const (
	TokenStatusEnabled   = 1 // Token 状态：启用（不要使用 0，0 是默认值！）
	TokenStatusDisabled  = 2 // Token 状态：禁用（也不要使用 0）
	TokenStatusExpired   = 3 // Token 状态：已过期
	TokenStatusExhausted = 4 // Token 状态：配额已耗尽
)

// 兑换码状态常量
const (
	RedemptionCodeStatusEnabled  = 1 // 兑换码状态：可用（不要使用 0，0 是默认值！）
	RedemptionCodeStatusDisabled = 2 // 兑换码状态：禁用（也不要使用 0）
	RedemptionCodeStatusUsed     = 3 // 兑换码状态：已使用（也不要使用 0）
)

// 渠道状态常量
const (
	ChannelStatusUnknown          = 0 // 渠道状态：未知
	ChannelStatusEnabled          = 1 // 渠道状态：启用（不要使用 0，0 是默认值！）
	ChannelStatusManuallyDisabled = 2 // 渠道状态：手动禁用（也不要使用 0）
	ChannelStatusAutoDisabled     = 3 // 渠道状态：自动禁用（错误率过高时自动禁用）
)

// 充值状态常量
const (
	TopUpStatusPending = "pending" // 充值状态：待处理
	TopUpStatusSuccess = "success" // 充值状态：成功
	TopUpStatusFailed  = "failed"  // 充值状态：失败
	TopUpStatusExpired = "expired" // 充值状态：已过期
)
