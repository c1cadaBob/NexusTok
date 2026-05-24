// api - options.go
// 该文件暴露 HTTP 服务器选项的辅助函数，用于嵌入 CLIProxyAPI。
// 包装内部服务器选项类型，使外部项目可以配置嵌入式 HTTP 服务器
// 而无需导入内部包。

// Package api exposes server option helpers for embedding CLIProxyAPI.
//
// It wraps internal server option types so external projects can configure the embedded
// HTTP server without importing internal packages.
package api

import (
	"time"

	"github.com/gin-gonic/gin"
	internalapi "github.com/router-for-me/CLIProxyAPI/v7/internal/api"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/api/handlers"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/logging"
)

// ServerOption 自定义 HTTP 服务器构建选项。
type ServerOption = internalapi.ServerOption

// WithMiddleware 在服务器构建期间追加额外的 Gin 中间件。
func WithMiddleware(mw ...gin.HandlerFunc) ServerOption { return internalapi.WithMiddleware(mw...) }

// WithEngineConfigurator 允许调用方在中间件设置之前修改 Gin 引擎。
func WithEngineConfigurator(fn func(*gin.Engine)) ServerOption {
	return internalapi.WithEngineConfigurator(fn)
}

// WithRouterConfigurator 在默认路由注册之后追加回调。
func WithRouterConfigurator(fn func(*gin.Engine, *handlers.BaseAPIHandler, *config.Config)) ServerOption {
	return internalapi.WithRouterConfigurator(fn)
}

// WithLocalManagementPassword 存储仅用于本地主机管理请求的运行时管理密码。
func WithLocalManagementPassword(password string) ServerOption {
	return internalapi.WithLocalManagementPassword(password)
}

// WithKeepAliveEndpoint 启用带超时和回调的保活端点。
func WithKeepAliveEndpoint(timeout time.Duration, onTimeout func()) ServerOption {
	return internalapi.WithKeepAliveEndpoint(timeout, onTimeout)
}

// WithRequestLoggerFactory 自定义请求日志记录器的创建方式。
func WithRequestLoggerFactory(factory func(*config.Config, string) logging.RequestLogger) ServerOption {
	return internalapi.WithRequestLoggerFactory(factory)
}
