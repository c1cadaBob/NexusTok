// Package common - api_type.go
// 该文件实现了渠道类型（ChannelType）到 API 类型（APIType）的映射函数
//
// 渠道类型表示上游 AI 服务提供商的标识（如 OpenAI、Anthropic、Gemini 等）
// API 类型表示系统内部用于路由和适配器选择的标识
// 两者的映射关系是多对一的，即多个渠道类型可能映射到同一个 API 类型
//
// 映射覆盖的渠道类型包括：
// - 国际主流：OpenAI、Anthropic、Gemini、AWS Bedrock、Vertex AI、Mistral、DeepSeek 等
// - 国内主流：百度（v1/v2）、智谱（v3/v4）、阿里、讯飞、腾讯、火山引擎、Moonshot 等
// - 其他平台：Ollama、Perplexity、Cohere、Dify、Jina、Cloudflare、SiliconFlow 等
package common

import "github.com/c1cada/NexusTok/constant"

// ChannelType2APIType 将渠道类型转换为 API 类型
//
// 该函数在请求分发阶段被调用，用于确定使用哪个适配器处理请求
// 每个渠道类型对应一个上游 AI 服务提供商
// 返回的 API 类型决定了请求的路由和处理方式
//
// 参数：
//   - channelType: 渠道类型常量（来自 constant 包）
//
// 返回值：
//   - int: API 类型常量，如果渠道类型未映射则返回 APITypeOpenAI 作为默认值
//   - bool: 是否找到有效的映射关系，false 表示使用了默认的 OpenAI 类型
func ChannelType2APIType(channelType int) (int, bool) {
	// apiType 初始化为 -1，表示尚未找到匹配的映射
	apiType := -1

	// 根据渠道类型选择对应的 API 类型
	// 每个 case 对应一个上游 AI 服务提供商
	switch channelType {
	// ── 国际主流 AI 服务 ──────────────────────────────────────────────
	case constant.ChannelTypeOpenAI: // OpenAI（GPT-4、GPT-3.5 等）
		apiType = constant.APITypeOpenAI
	case constant.ChannelTypeAnthropic: // Anthropic（Claude 系列）
		apiType = constant.APITypeAnthropic
	case constant.ChannelTypeGemini: // Google Gemini
		apiType = constant.APITypeGemini
	case constant.ChannelTypeAws: // AWS Bedrock（托管 Claude、Llama 等）
		apiType = constant.APITypeAws
	case constant.ChannelTypeVertexAi: // Google Vertex AI
		apiType = constant.APITypeVertexAi
	case constant.ChannelTypeMistral: // Mistral AI
		apiType = constant.APITypeMistral
	case constant.ChannelTypeDeepSeek: // DeepSeek
		apiType = constant.APITypeDeepSeek
	case constant.ChannelTypePerplexity: // Perplexity AI
		apiType = constant.APITypePerplexity
	case constant.ChannelTypeCohere: // Cohere
		apiType = constant.APITypeCohere
	case constant.ChannelTypeXai: // xAI（Grok）
		apiType = constant.APITypeXai
	case constant.ChannelTypeOpenRouter: // OpenRouter（AI 模型聚合平台）
		apiType = constant.APITypeOpenRouter
	case constant.ChannelTypeReplicate: // Replicate（模型托管平台）
		apiType = constant.APITypeReplicate

	// ── 国内 AI 服务 ─────────────────────────────────────────────────
	case constant.ChannelTypeBaidu: // 百度文心一言（v1）
		apiType = constant.APITypeBaidu
	case constant.ChannelTypeBaiduV2: // 百度文心一言（v2）
		apiType = constant.APITypeBaiduV2
	case constant.ChannelTypeZhipu: // 智谱 AI（v3，ChatGLM）
		apiType = constant.APITypeZhipu
	case constant.ChannelTypeZhipu_v4: // 智谱 AI（v4）
		apiType = constant.APITypeZhipuV4
	case constant.ChannelTypeAli: // 阿里通义千问
		apiType = constant.APITypeAli
	case constant.ChannelTypeXunfei: // 讯飞星火
		apiType = constant.APITypeXunfei
	case constant.ChannelTypeTencent: // 腾讯混元
		apiType = constant.APITypeTencent
	case constant.ChannelTypeVolcEngine: // 火山引擎（豆包）
		apiType = constant.APITypeVolcEngine
	case constant.ChannelTypeMoonshot: // Moonshot（Kimi）
		apiType = constant.APITypeMoonshot
	case constant.ChannelTypeMiniMax: // MiniMax（海螺 AI）
		apiType = constant.APITypeMiniMax
	case constant.ChannelTypeCoze: // Coze（字节跳动 AI 平台）
		apiType = constant.APITypeCoze
	case constant.ChannelTypeJimeng: // 即梦 AI（图像生成）
		apiType = constant.APITypeJimeng

	// ── 本地部署和第三方平台 ──────────────────────────────────────────
	case constant.ChannelTypeOllama: // Ollama（本地模型部署）
		apiType = constant.APITypeOllama
	case constant.ChannelTypeXinference: // Xinference（本地模型推理框架）
		apiType = constant.APITypeXinference
	case constant.ChannelTypeDify: // Dify（AI 应用开发平台）
		apiType = constant.APITypeDify
	case constant.ChannelTypeSiliconFlow: // SiliconFlow（硅基流动）
		apiType = constant.APITypeSiliconFlow
	case constant.ChannelTypeMokaAI: // MokaAI
		apiType = constant.APITypeMokaAI

	// ── 其他服务 ──────────────────────────────────────────────────────
	case constant.ChannelTypePaLM: // Google PaLM（旧版，已被 Gemini 取代）
		apiType = constant.APITypePaLM
	case constant.ChannelTypeAIProxyLibrary: // AI Proxy Library
		apiType = constant.APITypeAIProxyLibrary
	case constant.ChannelTypeJina: // Jina AI（Embedding、Rerank）
		apiType = constant.APITypeJina
	case constant.ChannelCloudflare: // Cloudflare Workers AI
		apiType = constant.APITypeCloudflare
	case constant.ChannelTypeSubmodel: // 子模型（组合模型）
		apiType = constant.APITypeSubmodel
	case constant.ChannelTypeCodex: // Codex（代码生成）
		apiType = constant.APITypeCodex
	case constant.ChannelTypeAdvancedCustom: // Advanced Custom（按请求路径映射上游协议）
		apiType = constant.APITypeAdvancedCustom
	}

	// 如果没有找到匹配的渠道类型，默认返回 OpenAI API 类型
	// 这是一个安全的降级策略，因为 OpenAI 格式是最通用的
	if apiType == -1 {
		return constant.APITypeOpenAI, false
	}
	return apiType, true
}
