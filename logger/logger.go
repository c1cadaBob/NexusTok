// Package logger - logger.go
// 该文件实现了系统的日志记录功能
//
// 设计目标：
// - 提供统一的日志接口，支持 INFO、WARN、ERROR、DEBUG 四个日志级别
// - 支持日志文件轮转（当日志数量超过阈值时自动创建新文件）
// - 支持同时输出到控制台和文件
// - 提供配额格式化工具函数
//
// 日志格式：[级别] 时间 | 请求ID | 消息内容
// 日志文件命名：oneapi-YYYYMMDDHHmmss.log
package logger

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/setting/operation_setting"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
)

// 日志级别常量
const (
	loggerINFO  = "INFO"  // 信息级别：常规操作日志
	loggerWarn  = "WARN"  // 警告级别：潜在问题
	loggerError = "ERR"   // 错误级别：操作失败
	loggerDebug = "DEBUG" // 调试级别：详细调试信息（仅在调试模式下输出）
)

const maxLogCount = 1000000 // 单个日志文件最大日志条数，超过后触发轮转

var logCount int               // 当前日志文件已写入的日志条数
var setupLogLock sync.Mutex    // 日志设置互斥锁，防止并发设置
var setupLogWorking bool       // 是否正在设置日志（防止重入）
var currentLogPath string      // 当前日志文件路径
var currentLogPathMu sync.RWMutex // 日志路径读写锁
var currentLogFile *os.File    // 当前日志文件句柄

// GetCurrentLogPath 获取当前日志文件路径
//
// 返回值：
//   - string: 当前日志文件的完整路径
func GetCurrentLogPath() string {
	currentLogPathMu.RLock()
	defer currentLogPathMu.RUnlock()
	return currentLogPath
}

// SetupLogger 初始化日志系统
//
// 功能：
// 1. 如果配置了日志目录（LogDir），创建新的日志文件
// 2. 将 Gin 的默认输出重定向到同时输出到控制台和文件
// 3. 关闭旧的日志文件句柄
// 4. 日志文件命名格式：oneapi-YYYYMMDDHHmmss.log
//
// 注意：使用 TryLock 防止并发调用
func SetupLogger() {
	defer func() {
		setupLogWorking = false
	}()
	if *common.LogDir != "" {
		ok := setupLogLock.TryLock()
		if !ok {
			log.Println("setup log is already working")
			return
		}
		defer func() {
			setupLogLock.Unlock()
		}()
		// 生成日志文件路径
		logPath := filepath.Join(*common.LogDir, fmt.Sprintf("oneapi-%s.log", time.Now().Format("20060102150405")))
		fd, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Fatal("failed to open log file")
		}
		currentLogPathMu.Lock()
		oldFile := currentLogFile
		currentLogPath = logPath
		currentLogFile = fd
		currentLogPathMu.Unlock()

		// 将 Gin 输出重定向到同时输出到控制台和文件
		common.LogWriterMu.Lock()
		gin.DefaultWriter = io.MultiWriter(os.Stdout, fd)
		gin.DefaultErrorWriter = io.MultiWriter(os.Stderr, fd)
		if oldFile != nil {
			_ = oldFile.Close()
		}
		common.LogWriterMu.Unlock()
	}
}

// LogInfo 记录信息级别日志
//
// 参数：
//   - ctx: 上下文（用于提取请求 ID）
//   - msg: 日志消息
func LogInfo(ctx context.Context, msg string) {
	logHelper(ctx, loggerINFO, msg)
}

// LogWarn 记录警告级别日志
//
// 参数：
//   - ctx: 上下文
//   - msg: 日志消息
func LogWarn(ctx context.Context, msg string) {
	logHelper(ctx, loggerWarn, msg)
}

// LogError 记录错误级别日志
//
// 参数：
//   - ctx: 上下文
//   - msg: 日志消息
func LogError(ctx context.Context, msg string) {
	logHelper(ctx, loggerError, msg)
}

// LogDebug 记录调试级别日志
//
// 仅在调试模式（DebugEnabled=true）下输出
// 支持格式化字符串（类似 fmt.Sprintf）
//
// 参数：
//   - ctx: 上下文
//   - msg: 日志消息（支持格式化占位符）
//   - args: 格式化参数
func LogDebug(ctx context.Context, msg string, args ...any) {
	if common.DebugEnabled {
		if len(args) > 0 {
			msg = fmt.Sprintf(msg, args...)
		}
		logHelper(ctx, loggerDebug, msg)
	}
}

// logHelper 日志记录辅助函数
//
// 功能：
// 1. 从上下文提取请求 ID
// 2. 格式化日志消息
// 3. 写入对应的输出流（INFO 写入 stdout，其他写入 stderr）
// 4. 检查日志数量，超过阈值时触发日志轮转
//
// 参数：
//   - ctx: 上下文
//   - level: 日志级别
//   - msg: 日志消息
func logHelper(ctx context.Context, level string, msg string) {
	id := ctx.Value(common.RequestIdKey)
	if id == nil {
		id = "SYSTEM"
	}
	now := time.Now()
	common.LogWriterMu.RLock()
	writer := gin.DefaultErrorWriter
	if level == loggerINFO {
		writer = gin.DefaultWriter
	}
	_, _ = fmt.Fprintf(writer, "[%s] %v | %s | %s \n", level, now.Format("2006/01/02 - 15:04:05"), id, msg)
	common.LogWriterMu.RUnlock()
	logCount++ // 不需要精确计数，无需加锁
	if logCount > maxLogCount && !setupLogWorking {
		logCount = 0
		setupLogWorking = true
		gopool.Go(func() {
			SetupLogger()
		})
	}
}

// LogQuota 将配额值格式化为带单位的字符串
//
// 根据系统配置的配额显示类型，转换为不同的货币格式：
// - CNY：人民币格式（¥xxx）
// - Custom：自定义货币格式
// - Tokens：点数格式
// - USD（默认）：美元格式（＄xxx）
//
// 参数：
//   - quota: 配额值（内部单位）
//
// 返回值：
//   - string: 格式化后的配额字符串
func LogQuota(quota int) string {
	q := float64(quota)
	switch operation_setting.GetQuotaDisplayType() {
	case operation_setting.QuotaDisplayTypeCNY:
		usd := q / common.QuotaPerUnit
		cny := usd * operation_setting.USDExchangeRate
		return fmt.Sprintf("¥%.6f 额度", cny)
	case operation_setting.QuotaDisplayTypeCustom:
		usd := q / common.QuotaPerUnit
		rate := operation_setting.GetGeneralSetting().CustomCurrencyExchangeRate
		symbol := operation_setting.GetGeneralSetting().CustomCurrencySymbol
		if symbol == "" {
			symbol = "¤"
		}
		if rate <= 0 {
			rate = 1
		}
		v := usd * rate
		return fmt.Sprintf("%s%.6f 额度", symbol, v)
	case operation_setting.QuotaDisplayTypeTokens:
		return fmt.Sprintf("%d 点额度", quota)
	default: // USD
		return fmt.Sprintf("＄%.6f 额度", q/common.QuotaPerUnit)
	}
}

// FormatQuota 将配额值格式化为纯数字字符串（不带单位）
//
// 与 LogQuota 类似，但不附加"额度"后缀
// 用于 UI 显示等需要精确控制格式的场景
//
// 参数：
//   - quota: 配额值（内部单位）
//
// 返回值：
//   - string: 格式化后的配额字符串
func FormatQuota(quota int) string {
	q := float64(quota)
	switch operation_setting.GetQuotaDisplayType() {
	case operation_setting.QuotaDisplayTypeCNY:
		usd := q / common.QuotaPerUnit
		cny := usd * operation_setting.USDExchangeRate
		return fmt.Sprintf("¥%.6f", cny)
	case operation_setting.QuotaDisplayTypeCustom:
		usd := q / common.QuotaPerUnit
		rate := operation_setting.GetGeneralSetting().CustomCurrencyExchangeRate
		symbol := operation_setting.GetGeneralSetting().CustomCurrencySymbol
		if symbol == "" {
			symbol = "¤"
		}
		if rate <= 0 {
			rate = 1
		}
		v := usd * rate
		return fmt.Sprintf("%s%.6f", symbol, v)
	case operation_setting.QuotaDisplayTypeTokens:
		return fmt.Sprintf("%d", quota)
	default:
		return fmt.Sprintf("＄%.6f", q/common.QuotaPerUnit)
	}
}

// LogJson 以 JSON 格式记录对象（仅供测试使用）
//
// 将对象序列化为 JSON 字符串后以 DEBUG 级别输出
//
// 参数：
//   - ctx: 上下文
//   - msg: 附加消息
//   - obj: 要记录的对象
func LogJson(ctx context.Context, msg string, obj any) {
	jsonStr, err := common.Marshal(obj)
	if err != nil {
		LogError(ctx, fmt.Sprintf("json marshal failed: %s", err.Error()))
		return
	}
	LogDebug(ctx, fmt.Sprintf("%s | %s", msg, string(jsonStr)))
}
