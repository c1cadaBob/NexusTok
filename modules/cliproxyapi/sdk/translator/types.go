// translator - types.go
// 该文件定义了翻译管道的核心类型。
// 包括请求/响应的信封结构体、中间件类型和处理器类型，
// 用于在不同 AI 提供商的请求/响应格式之间进行转换。
// 管道（Pipeline）支持中间件链式处理，允许在翻译前后插入自定义逻辑。

// Package translator provides types and functions for converting chat requests and responses between different schemas.
package translator

import "context"

// RequestTransform 是将请求载荷从源格式转换为目标格式的函数类型。
// 它接收模型名称、原始 JSON 请求载荷和是否为流式请求的布尔标志，
// 返回转换后的请求载荷字节切片。
type RequestTransform func(model string, rawJSON []byte, stream bool) []byte

// ResponseStreamTransform 是将流式响应从源格式转换为目标格式的函数类型。
// 它接收上下文、模型名称、原始请求和转换后请求的 JSON、当前响应块的 JSON 以及可选参数，
// 返回包含转换后流式响应数据块的字节切片。
type ResponseStreamTransform func(ctx context.Context, model string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) [][]byte

// ResponseNonStreamTransform 是将非流式响应从源格式转换为目标格式的函数类型。
// 它接收上下文、模型名称、原始请求和转换后请求的 JSON、响应 JSON 以及可选参数，
// 返回转换后的响应字节切片。
type ResponseNonStreamTransform func(ctx context.Context, model string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) []byte

// ResponseTokenCountTransform 是将 token 计数从源格式转换为目标格式的函数类型。
// 它接收上下文和 token 计数（int64），返回转换后的 token 计数字节切片。
type ResponseTokenCountTransform func(ctx context.Context, count int64) []byte

// ResponseTransform 将流式响应、非流式响应和 token 计数的转换函数组合在一起。
// 这是一个完整的响应转换集合，用于注册到翻译注册表中。
type ResponseTransform struct {
	// Stream 是流式响应转换函数
	Stream ResponseStreamTransform
	// NonStream 是非流式响应转换函数
	NonStream ResponseNonStreamTransform
	// TokenCount 是 token 计数转换函数
	TokenCount ResponseTokenCountTransform
}
