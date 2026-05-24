// modules - modules.go
// 可插拔路由模块系统。
// 该模块提供了一个扩展 API 服务器的框架，允许通过独立的模块添加路由和功能，
// 而无需修改核心路由逻辑。支持 V1（旧版）和 V2（推荐）两种模块接口。
package modules

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/api/handlers"
)

// Context encapsulates the dependencies exposed to routing modules during
// registration. Modules can use the Gin engine to attach routes, the shared
// BaseAPIHandler for constructing SDK-specific handlers, and the resolved
// authentication middleware for protecting routes that require API keys.
type Context struct {
	Engine         *gin.Engine
	BaseHandler    *handlers.BaseAPIHandler
	Config         *config.Config
	AuthMiddleware gin.HandlerFunc
}

// RouteModule represents a pluggable routing module that can register routes
// and handle configuration updates independently of the core server.
//
// DEPRECATED: Use RouteModuleV2 for new modules. This interface is kept for
// backwards compatibility and will be removed in a future version.
type RouteModule interface {
	// Name returns a human-readable identifier for the module
	Name() string

	// Register sets up routes and handlers for this module.
	// It receives the Gin engine, base handlers, and current configuration.
	// Returns an error if registration fails (errors are logged but don't stop the server).
	Register(engine *gin.Engine, baseHandler *handlers.BaseAPIHandler, cfg *config.Config) error

	// OnConfigUpdated is called when the configuration is reloaded.
	// Modules can respond to configuration changes here.
	// Returns an error if the update cannot be applied.
	OnConfigUpdated(cfg *config.Config) error
}

// RouteModuleV2 represents a pluggable bundle of routes that can integrate with
// the API server without modifying its core routing logic. Implementations can
// attach routes during Register and react to configuration updates via
// OnConfigUpdated.
//
// This is the preferred interface for new modules. It uses Context for cleaner
// dependency injection and supports idempotent registration.
type RouteModuleV2 interface {
	// Name returns a unique identifier for logging and diagnostics.
	Name() string

	// Register wires the module's routes into the provided Gin engine. Modules
	// should treat multiple calls as idempotent and avoid duplicate route
	// registration when invoked more than once.
	Register(ctx Context) error

	// OnConfigUpdated notifies the module when the server configuration changes
	// via hot reload. Implementations can refresh cached state or emit warnings.
	OnConfigUpdated(cfg *config.Config) error
}

// RegisterModule is a helper that registers a module using either the V1 or V2
// interface. This allows gradual migration from V1 to V2 without breaking
// existing modules.
//
// Example usage:
//
//	ctx := modules.Context{
//	    Engine:         engine,
//	    BaseHandler:    baseHandler,
//	    Config:         cfg,
//	    AuthMiddleware: authMiddleware,
//	}
//	if err := modules.RegisterModule(ctx, ampModule); err != nil {
//	    log.Errorf("Failed to register module: %v", err)
//	}
// RegisterModule 辅助函数，自动检测模块实现的接口版本（V1 或 V2）并注册。
// 优先尝试 V2 接口，如果模块未实现则降级到 V1 接口。
// 如果模块两个接口都未实现，返回错误。
func RegisterModule(ctx Context, mod interface{}) error {
	// Try V2 interface first (preferred)
	if v2, ok := mod.(RouteModuleV2); ok {
		return v2.Register(ctx)
	}

	// Fall back to V1 interface for backwards compatibility
	if v1, ok := mod.(RouteModule); ok {
		return v1.Register(ctx.Engine, ctx.BaseHandler, ctx.Config)
	}

	return fmt.Errorf("unsupported module type %T (must implement RouteModule or RouteModuleV2)", mod)
}
