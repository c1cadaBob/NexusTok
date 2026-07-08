// Package router - model-router.go
// 该文件集中注册模型管理、厂商元数据和模型部署路由。
//
// 模型管理路由先经过 AdminAuth，再按 authz 权限表执行 read/operate/write/
// sensitive_write 二次校验。路由表只改变授权层级，不改变现有 handler、
// 路径和业务语义，方便后续接入 Casbin 或用户级 override 时复用同一入口。
package router

import (
	"net/http"

	"github.com/c1cada/NexusTok/controller"
	"github.com/c1cada/NexusTok/middleware"
	"github.com/c1cada/NexusTok/service/authz"

	"github.com/gin-gonic/gin"
)

// registerModelRoutes 注册模型管理相关路由组。
func registerModelRoutes(apiRouter *gin.RouterGroup) {
	vendorRoute := apiRouter.Group("/vendors")
	vendorRoute.Use(middleware.AdminAuth())
	registerPermissionRoutes(vendorRoute, vendorPermissionRoutes)

	modelsRoute := apiRouter.Group("/models")
	modelsRoute.Use(middleware.AdminAuth())
	registerPermissionRoutes(modelsRoute, modelPermissionRoutes)

	deploymentsRoute := apiRouter.Group("/deployments")
	deploymentsRoute.Use(middleware.AdminAuth())
	registerPermissionRoutes(deploymentsRoute, deploymentPermissionRoutes)
}

var vendorPermissionRoutes = []permissionRoute{
	// 厂商查询
	{method: http.MethodGet, path: "/", permission: authz.ModelRead, handler: controller.GetAllVendors},
	{method: http.MethodGet, path: "/search", permission: authz.ModelRead, handler: controller.SearchVendors},
	{method: http.MethodGet, path: "/:id", permission: authz.ModelRead, handler: controller.GetVendorMeta},

	// 厂商元数据维护。删除会移除平台级厂商记录，按敏感写处理。
	{method: http.MethodPost, path: "/", permission: authz.ModelWrite, handler: controller.CreateVendorMeta},
	{method: http.MethodPut, path: "/", permission: authz.ModelWrite, handler: controller.UpdateVendorMeta},
	{method: http.MethodDelete, path: "/:id", permission: authz.ModelSensitiveWrite, handler: controller.DeleteVendorMeta},
}

var modelPermissionRoutes = []permissionRoute{
	// 上游模型同步。预览只执行外部读取和差异计算；真正同步会写入模型/厂商元数据。
	{method: http.MethodGet, path: "/sync_upstream/preview", permission: authz.ModelOperate, handler: controller.SyncUpstreamPreview},
	{method: http.MethodPost, path: "/sync_upstream", permission: authz.ModelWrite, handler: controller.SyncUpstreamModels},

	// 模型查询与定价查看
	{method: http.MethodGet, path: "/missing", permission: authz.ModelRead, handler: controller.GetMissingModels},
	{method: http.MethodGet, path: "/", permission: authz.ModelRead, handler: controller.GetAllModelsMeta},
	{method: http.MethodGet, path: "/search", permission: authz.ModelRead, handler: controller.SearchModelsMeta},
	{method: http.MethodGet, path: "/:id/pricing", permission: authz.ModelRead, handler: controller.GetModelPricingConfig},
	{method: http.MethodGet, path: "/:id", permission: authz.ModelRead, handler: controller.GetModelMeta},

	// 模型元数据与定价维护。定价配置会影响计费，但仍属于模型日常管理写权限。
	{method: http.MethodPut, path: "/:id/pricing", permission: authz.ModelWrite, handler: controller.UpdateModelPricingConfig},
	{method: http.MethodPost, path: "/", permission: authz.ModelWrite, handler: controller.CreateModelMeta},
	{method: http.MethodPut, path: "/", permission: authz.ModelWrite, handler: controller.UpdateModelMeta},
	{method: http.MethodDelete, path: "/:id", permission: authz.ModelSensitiveWrite, handler: controller.DeleteModelMeta},
}

var deploymentPermissionRoutes = []permissionRoute{
	// 部署设置与查询
	{method: http.MethodGet, path: "/settings", permission: authz.ModelRead, handler: controller.GetModelDeploymentSettings},
	{method: http.MethodGet, path: "/", permission: authz.ModelRead, handler: controller.GetAllDeployments},
	{method: http.MethodGet, path: "/search", permission: authz.ModelRead, handler: controller.SearchDeployments},
	{method: http.MethodGet, path: "/hardware-types", permission: authz.ModelRead, handler: controller.GetHardwareTypes},
	{method: http.MethodGet, path: "/locations", permission: authz.ModelRead, handler: controller.GetLocations},
	{method: http.MethodGet, path: "/available-replicas", permission: authz.ModelRead, handler: controller.GetAvailableReplicas},
	{method: http.MethodGet, path: "/check-name", permission: authz.ModelRead, handler: controller.CheckClusterNameAvailability},

	// 部署运行操作。连接测试、价格估算和扩容会触发外部 API 或改变运行容量，但不删除资源。
	{method: http.MethodPost, path: "/settings/test-connection", permission: authz.ModelOperate, handler: controller.TestIoNetConnection},
	{method: http.MethodPost, path: "/test-connection", permission: authz.ModelOperate, handler: controller.TestIoNetConnection},
	{method: http.MethodPost, path: "/price-estimation", permission: authz.ModelOperate, handler: controller.GetPriceEstimation},
	{method: http.MethodPost, path: "/:id/extend", permission: authz.ModelOperate, handler: controller.ExtendDeployment},

	// 部署生命周期维护
	{method: http.MethodPost, path: "/", permission: authz.ModelWrite, handler: controller.CreateDeployment},
	{method: http.MethodGet, path: "/:id", permission: authz.ModelRead, handler: controller.GetDeployment},
	{method: http.MethodGet, path: "/:id/logs", permission: authz.ModelRead, handler: controller.GetDeploymentLogs},
	{method: http.MethodGet, path: "/:id/containers", permission: authz.ModelRead, handler: controller.ListDeploymentContainers},
	{method: http.MethodGet, path: "/:id/containers/:container_id", permission: authz.ModelRead, handler: controller.GetContainerDetails},
	{method: http.MethodPut, path: "/:id", permission: authz.ModelWrite, handler: controller.UpdateDeployment},
	{method: http.MethodPut, path: "/:id/name", permission: authz.ModelWrite, handler: controller.UpdateDeploymentName},
	{method: http.MethodDelete, path: "/:id", permission: authz.ModelSensitiveWrite, handler: controller.DeleteDeployment},
}
