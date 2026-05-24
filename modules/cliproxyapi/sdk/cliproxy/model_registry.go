// cliproxy - model_registry.go
// 该文件提供模型注册表的公共 SDK 接口。
// 通过类型别名重新导出内部注册表的 ModelInfo 和 ModelRegistryHook，
// 并提供 ModelRegistry 接口和全局注册表访问函数。

package cliproxy

import "github.com/router-for-me/CLIProxyAPI/v7/internal/registry"

// ModelInfo 重新导出注册表的模型信息结构体。
type ModelInfo = registry.ModelInfo

// ModelRegistryHook 重新导出注册表的钩子接口，用于外部集成。
type ModelRegistryHook = registry.ModelRegistryHook

// ModelRegistry 描述外部调用方使用的注册表操作接口。
// 提供客户端注册/注销、模型配额管理、模型可用性查询等功能。
type ModelRegistry interface {
	// RegisterClient 为指定客户端注册模型列表
	RegisterClient(clientID, clientProvider string, models []*ModelInfo)
	// UnregisterClient 注销指定客户端的所有模型
	UnregisterClient(clientID string)
	// SetModelQuotaExceeded 标记指定客户端的模型配额已超限
	SetModelQuotaExceeded(clientID, modelID string)
	// ClearModelQuotaExceeded 清除指定客户端的模型配额超限状态
	ClearModelQuotaExceeded(clientID, modelID string)
	// ClientSupportsModel 检查指定客户端是否支持某个模型
	ClientSupportsModel(clientID, modelID string) bool
	// GetAvailableModels 获取指定处理器类型的可用模型列表
	GetAvailableModels(handlerType string) []map[string]any
	// GetAvailableModelsByProvider 获取指定提供商的可用模型列表
	GetAvailableModelsByProvider(provider string) []*ModelInfo
}

// GlobalModelRegistry 返回共享的全局注册表实例。
func GlobalModelRegistry() ModelRegistry {
	return registry.GetGlobalRegistry()
}

// SetGlobalModelRegistryHook 在共享全局注册表实例上注册可选的钩子。
func SetGlobalModelRegistryHook(hook ModelRegistryHook) {
	registry.GetGlobalRegistry().SetHook(hook)
}
