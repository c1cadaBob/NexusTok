// Package constant - env.go
// 该文件定义了环境变量控制的全局配置变量
//
// 这些变量在服务启动时从环境变量读取，运行期间保持不变
// 用于控制各种功能的开关和参数
//
// 配置分类：
// - 流式传输配置
// - 文件处理配置
// - Token 计数配置
// - 任务配置
// - 日志配置
// - 安全配置
package constant

// StreamingTimeout 流式传输超时时间（秒）
// 超过此时间未收到上游数据，将断开连接
var StreamingTimeout int

// DifyDebug 是否启用 Dify 调试模式
// 启用后会输出 Dify API 的详细请求/响应日志
var DifyDebug bool

// MaxFileDownloadMB 最大文件下载大小（MB）
// 超过此大小的文件下载将被拒绝
var MaxFileDownloadMB int

// StreamScannerMaxBufferMB 流式扫描器最大缓冲区大小（MB）
// 用于处理流式响应时的缓冲区限制
var StreamScannerMaxBufferMB int

// ForceStreamOption 是否强制使用流式选项
// 启用后，所有请求都会尝试使用流式传输
var ForceStreamOption bool

// CountToken 是否启用 Token 计数
// 启用后，会在本地计算请求/响应的 Token 数量
var CountToken bool

// GetMediaToken 是否在流式模式下获取媒体 Token
var GetMediaToken bool

// GetMediaTokenNotStream 是否在非流式模式下获取媒体 Token
var GetMediaTokenNotStream bool

// UpdateTask 是否启用任务状态更新
// 用于异步任务（如图像生成）的状态跟踪
var UpdateTask bool

// MaxRequestBodyMB 最大请求体大小（MB）
// 超过此大小的请求将被拒绝
var MaxRequestBodyMB int

// AzureDefaultAPIVersion Azure OpenAI 默认 API 版本
// 例如：2024-02-15-preview
var AzureDefaultAPIVersion string

// NotifyLimitCount 通知限制数量
// 在指定时间窗口内允许的最大通知次数
var NotifyLimitCount int

// NotificationLimitDurationMinute 通知限制时间窗口（分钟）
// 与 NotifyLimitCount 配合使用
var NotificationLimitDurationMinute int

// GenerateDefaultToken 是否自动生成默认 Token
// 新用户注册时是否自动创建默认 API Token
var GenerateDefaultToken bool

// ErrorLogEnabled 是否启用错误日志
// 启用后，会记录详细的错误日志到数据库
var ErrorLogEnabled bool

// TaskQueryLimit 任务查询限制
// 查询异步任务列表时的最大返回数量
var TaskQueryLimit int

// TaskTimeoutMinutes 任务超时时间（分钟）
// 超过此时间未完成的任务将被标记为失败
var TaskTimeoutMinutes int

// TaskPricePatches 任务价格补丁（临时变量，未来版本将移除）
// 用于 Sora 等特殊任务的价格覆盖
var TaskPricePatches []string

// TrustedRedirectDomains 可信重定向域名列表
// 用于验证重定向 URL 的安全性
// 支持子域名匹配（如 "example.com" 匹配 "sub.example.com"）
var TrustedRedirectDomains []string
