// 包 interfaces 定义了 CLI Proxy API 服务器的核心接口和共享结构。
// 这些接口为应用程序的不同组件提供了通用契约，
// 如 AI 服务客户端、API 处理器和数据模型。
package interfaces

// APIHandler 定义了所有 API 处理器必须实现的接口。
// 该接口提供了用于识别处理器类型和检索不同 AI 服务端点支持的模型的方法。
type APIHandler interface {
	// HandlerType 返回此 API 处理器的类型标识符。
	// 用于确定使用哪个请求/响应翻译器。
	HandlerType() string

	// Models 返回此 API 处理器支持的模型列表。
	// 每个模型表示为包含模型元数据的映射。
	Models() []map[string]any
}
