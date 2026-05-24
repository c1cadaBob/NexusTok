// logging - request_logger.go
// 该文件为 SDK 消费者提供请求日志记录的公共接口。
// 通过类型别名重新导出内部日志记录实现，包括文件日志记录器和流式日志写入器，
// 并提供带有默认配置的工厂函数。

// Package logging re-exports request logging primitives for SDK consumers.
package logging

import internallogging "github.com/router-for-me/CLIProxyAPI/v7/internal/logging"

// defaultErrorLogsMaxFiles 定义错误日志文件的最大保留数量。
const defaultErrorLogsMaxFiles = 10

// RequestLogger 定义了 HTTP 请求和响应日志记录的接口。
// 所有日志记录器实现都必须遵循此接口规范。
type RequestLogger = internallogging.RequestLogger

// StreamingLogWriter 处理流式响应数据块的实时日志记录。
// 用于在流式传输过程中逐块记录响应内容。
type StreamingLogWriter = internallogging.StreamingLogWriter

// FileRequestLogger 基于文件系统的请求日志记录器实现。
// 将请求和响应数据持久化存储到文件中。
type FileRequestLogger = internallogging.FileRequestLogger

// NewFileRequestLogger 创建一个使用默认错误日志保留策略（10个文件）的文件请求日志记录器。
//
// 参数：
//   - enabled: 是否启用日志记录
//   - logsDir: 日志文件存储目录
//   - configDir: 配置文件目录，用于确定日志存储位置
//
// 返回值：
//   - *FileRequestLogger: 文件请求日志记录器实例
func NewFileRequestLogger(enabled bool, logsDir string, configDir string) *FileRequestLogger {
	return internallogging.NewFileRequestLogger(enabled, logsDir, configDir, defaultErrorLogsMaxFiles)
}

// NewFileRequestLoggerWithOptions 创建一个支持自定义错误日志保留数量的文件请求日志记录器。
//
// 参数：
//   - enabled: 是否启用日志记录
//   - logsDir: 日志文件存储目录
//   - configDir: 配置文件目录
//   - errorLogsMaxFiles: 错误日志文件的最大保留数量
//
// 返回值：
//   - *FileRequestLogger: 文件请求日志记录器实例
func NewFileRequestLoggerWithOptions(enabled bool, logsDir string, configDir string, errorLogsMaxFiles int) *FileRequestLogger {
	return internallogging.NewFileRequestLogger(enabled, logsDir, configDir, errorLogsMaxFiles)
}
