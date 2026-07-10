// Package constant - api_type.go
// 该文件定义了 API 类型常量
//
// API 类型用于标识不同的 AI 服务提供商的 API 接口规范
// 每个 API 类型对应一种特定的请求/响应格式
//
// 与渠道类型的区别：
// - 渠道类型（ChannelType）：标识具体的 AI 服务提供商
// - API 类型（APIType）：标识 API 接口规范
// 多个渠道可能使用同一种 API 类型（如多个 OpenAI 兼容的代理服务）
//
// 使用场景：
// - 适配器选择：根据 API 类型选择对应的请求转换器
// - 请求格式化：将统一的请求格式转换为特定 API 的格式
// - 响应解析：将特定 API 的响应转换为统一格式
package constant

const (
	APITypeOpenAI         = iota // OpenAI API（GPT 系列模型）
	APITypeAnthropic             // Anthropic API（Claude 系列模型）
	APITypePaLM                  // Google PaLM API
	APITypeBaidu                 // 百度文心一言 API
	APITypeZhipu                 // 智谱 GLM API
	APITypeAli                   // 阿里通义千问 API
	APITypeXunfei                // 讯飞星火 API
	APITypeAIProxyLibrary        // AI Proxy Library API
	APITypeTencent               // 腾讯混元 API
	APITypeGemini                // Google Gemini API
	APITypeZhipuV4               // 智谱 GLM V4 API
	APITypeOllama                // Ollama 本地模型 API
	APITypePerplexity            // Perplexity API
	APITypeAws                   // AWS Bedrock API
	APITypeCohere                // Cohere API
	APITypeDify                  // Dify API
	APITypeJina                  // Jina API
	APITypeCloudflare            // Cloudflare Workers AI API
	APITypeSiliconFlow           // SiliconFlow API
	APITypeVertexAi              // Google Vertex AI API
	APITypeMistral               // Mistral API
	APITypeDeepSeek              // DeepSeek API
	APITypeMokaAI                // MokaAI API
	APITypeVolcEngine            // 火山引擎（豆包）API
	APITypeBaiduV2               // 百度 V2 API
	APITypeOpenRouter            // OpenRouter API
	APITypeXinference            // Xinference API
	APITypeXai                   // xAI（Grok）API
	APITypeCoze                  // Coze API
	APITypeJimeng                // 即梦 API
	APITypeMoonshot              // 月之暗面（Kimi）API
	APITypeSubmodel              // Submodel API
	APITypeMiniMax               // MiniMax API
	APITypeReplicate             // Replicate API
	APITypeCodex                 // Codex API
	APITypeAdvancedCustom        // Advanced Custom API（按路由映射到不同上游协议）
	APITypeDummy                 // 仅用于计数，不要在此之后添加 API 类型
)
