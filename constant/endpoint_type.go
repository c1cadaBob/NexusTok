// Package constant - endpoint_type.go
// 该文件定义了端点类型常量
//
// 端点类型用于标识不同的 API 端点规范
// 每个渠道可以支持多种端点类型，用于处理不同类型的请求
//
// 端点类型的作用：
// - 请求路由：根据端点类型选择对应的处理逻辑
// - 适配器选择：不同端点类型可能使用不同的请求/响应格式
// - 模型映射：某些端点类型有特殊的模型名称处理
package constant

// EndpointType 端点类型
type EndpointType string

const (
	// EndpointTypeOpenAI OpenAI Chat Completions 端点
	// 处理 /v1/chat/completions 请求
	EndpointTypeOpenAI EndpointType = "openai"

	// EndpointTypeOpenAIResponse OpenAI Responses 端点
	// 处理 /v1/responses 请求（完整响应模式）
	EndpointTypeOpenAIResponse EndpointType = "openai-response"

	// EndpointTypeOpenAIResponseCompact OpenAI Responses 紧凑端点
	// 处理 /v1/responses 请求（紧凑响应模式）
	EndpointTypeOpenAIResponseCompact EndpointType = "openai-response-compact"

	// EndpointTypeAnthropic Anthropic Messages 端点
	// 处理 /v1/messages 请求
	EndpointTypeAnthropic EndpointType = "anthropic"

	// EndpointTypeGemini Gemini GenerateContent 端点
	// 处理 Gemini API 的内容生成请求
	EndpointTypeGemini EndpointType = "gemini"

	// EndpointTypeJinaRerank Jina Rerank 端点
	// 处理 Jina 的重排序请求
	EndpointTypeJinaRerank EndpointType = "jina-rerank"

	// EndpointTypeImageGeneration 图像生成端点
	// 处理图像生成请求（DALL-E、Midjourney 等）
	EndpointTypeImageGeneration EndpointType = "image-generation"

	// EndpointTypeEmbeddings 文本嵌入端点
	// 处理 /v1/embeddings 请求
	EndpointTypeEmbeddings EndpointType = "embeddings"

	// EndpointTypeOpenAIVideo OpenAI 视频生成端点
	// 处理 Sora 等视频生成请求
	EndpointTypeOpenAIVideo EndpointType = "openai-video"

	// 以下端点类型已注释，暂未启用
	//EndpointTypeMidjourney     EndpointType = "midjourney-proxy"
	//EndpointTypeSuno           EndpointType = "suno-proxy"
	//EndpointTypeKling          EndpointType = "kling"
	//EndpointTypeJimeng         EndpointType = "jimeng"
)
