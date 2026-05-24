// interfaces - api_handler.go
// 本文件定义了所有 API 处理器必须实现的核心接口。
// 提供处理器类型标识和支持模型查询的通用契约。
package interfaces

// APIHandler 定义了所有 API 处理器必须实现的接口。
// 该接口提供了识别处理器类型和支持模型查询的方法，
// 用于不同 AI 服务端点的请求/响应处理。
type APIHandler interface {
	// HandlerType 返回此 API 处理器的类型标识符。
	// 用于确定使用哪个请求/响应转换器。
	HandlerType() string

	// Models 返回此 API 处理器支持的模型列表。
	// 每个模型以包含模型元数据的 map 表示。
	Models() []map[string]any
}
