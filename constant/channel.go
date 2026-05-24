// Package constant - channel.go
// 该文件定义了渠道类型相关的常量
// 渠道类型用于标识不同的 AI 服务提供商
//
// 渠道类型的作用：
// 1. 路由分发：根据渠道类型选择对应的适配器
// 2. 计费统计：按渠道类型统计使用量和费用
// 3. 状态监控：监控各渠道的可用性
// 4. 模型管理：管理各渠道支持的模型
package constant

// ========================================
// 渠道类型常量
// ========================================

const (
	ChannelTypeUnknown        = 0  // 未知类型
	ChannelTypeOpenAI         = 1  // OpenAI（GPT-4、GPT-3.5 等）
	ChannelTypeMidjourney     = 2  // Midjourney（图像生成）
	ChannelTypeAzure          = 3  // Azure OpenAI
	ChannelTypeOllama         = 4  // Ollama（本地模型）
	ChannelTypeMidjourneyPlus = 5  // Midjourney Plus
	ChannelTypeOpenAIMax      = 6  // OpenAI Max
	ChannelTypeOhMyGPT        = 7  // OhMyGPT
	ChannelTypeCustom         = 8  // 自定义渠道
	ChannelTypeAILS           = 9  // AILS
	ChannelTypeAIProxy        = 10 // AI Proxy
	ChannelTypePaLM           = 11 // Google PaLM
	ChannelTypeAPI2GPT        = 12 // API2GPT
	ChannelTypeAIGC2D         = 13 // AIGC2D
	ChannelTypeAnthropic      = 14 // Anthropic（Claude）
	ChannelTypeBaidu          = 15 // 百度（文心一言）
	ChannelTypeZhipu          = 16 // 智谱（GLM-4）
	ChannelTypeAli            = 17 // 阿里（通义千问）
	ChannelTypeXunfei         = 18 // 讯飞（星火）
	ChannelType360            = 19 // 360（智脑）
	ChannelTypeOpenRouter     = 20 // OpenRouter
	ChannelTypeAIProxyLibrary = 21 // AI Proxy Library
	ChannelTypeFastGPT        = 22 // FastGPT
	ChannelTypeTencent        = 23 // 腾讯（混元）
	ChannelTypeGemini         = 24 // Google Gemini
	ChannelTypeMoonshot       = 25 // 月之暗面（Kimi）
	ChannelTypeZhipu_v4       = 26 // 智谱 V4
	ChannelTypePerplexity     = 27 // Perplexity
	ChannelTypeLingYiWanWu    = 31 // 零一万物（Yi）
	ChannelTypeAws            = 33 // AWS（Bedrock）
	ChannelTypeCohere         = 34 // Cohere
	ChannelTypeMiniMax        = 35 // MiniMax
	ChannelTypeSunoAPI        = 36 // Suno API（音乐生成）
	ChannelTypeDify           = 37 // Dify
	ChannelTypeJina           = 38 // Jina
	ChannelCloudflare         = 39 // Cloudflare
	ChannelTypeSiliconFlow    = 40 // SiliconFlow
	ChannelTypeVertexAi       = 41 // Google Vertex AI
	ChannelTypeMistral        = 42 // Mistral
	ChannelTypeDeepSeek       = 43 // DeepSeek
	ChannelTypeMokaAI         = 44 // MokaAI
	ChannelTypeVolcEngine     = 45 // 火山引擎（豆包）
	ChannelTypeBaiduV2        = 46 // 百度 V2
	ChannelTypeXinference     = 47 // Xinference
	ChannelTypeXai            = 48 // xAI（Grok）
	ChannelTypeCoze           = 49 // Coze
	ChannelTypeKling          = 50 // Kling（快手视频生成）
	ChannelTypeJimeng         = 51 // 即梦（字节跳动视频生成）
	ChannelTypeVidu           = 52 // Vidu
	ChannelTypeSubmodel       = 53 // Submodel
	ChannelTypeDoubaoVideo    = 54 // 豆包视频
	ChannelTypeSora           = 55 // Sora（OpenAI 视频生成）
	ChannelTypeReplicate      = 56 // Replicate
	ChannelTypeCodex          = 57 // Codex（OpenAI Codex）
	ChannelTypeDummy          // 仅用于计数，不要在此之后添加渠道
)

// ChannelBaseURLs 渠道基础 URL 列表
// 索引对应渠道类型常量
var ChannelBaseURLs = []string{
	"",                                    // 0 - Unknown
	"https://api.openai.com",              // 1 - OpenAI
	"https://oa.api2d.net",                // 2 - Midjourney
	"",                                    // 3 - Azure
	"http://localhost:11434",              // 4 - Ollama
	"https://api.openai-sb.com",           // 5 - MidjourneyPlus
	"https://api.openaimax.com",           // 6 - OpenAIMax
	"https://api.ohmygpt.com",             // 7 - OhMyGPT
	"",                                    // 8 - Custom
	"https://api.caipacity.com",           // 9 - AILS
	"https://api.aiproxy.io",              // 10 - AIProxy
	"",                                    // 11 - PaLM
	"https://api.api2gpt.com",             // 12 - API2GPT
	"https://api.aigc2d.com",              // 13 - AIGC2D
	"https://api.anthropic.com",           // 14 - Anthropic
	"https://aip.baidubce.com",            // 15 - Baidu
	"https://open.bigmodel.cn",            // 16 - Zhipu
	"https://dashscope.aliyuncs.com",      // 17 - Ali
	"",                                    // 18 - Xunfei
	"https://api.360.cn",                  // 19 - 360
	"https://openrouter.ai/api",           // 20 - OpenRouter
	"https://api.aiproxy.io",              // 21 - AIProxyLibrary
	"https://fastgpt.run/api/openapi",     // 22 - FastGPT
	"https://hunyuan.tencentcloudapi.com", // 23 - Tencent
	"https://generativelanguage.googleapis.com", // 24 - Gemini
	"https://api.moonshot.cn",                   // 25 - Moonshot
	"https://open.bigmodel.cn",                  // 26 - ZhipuV4
	"https://api.perplexity.ai",                 // 27 - Perplexity
	"",                                          // 28
	"",                                          // 29
	"",                                          // 30
	"https://api.lingyiwanwu.com",               // 31 - LingYiWanWu
	"",                                          // 32
	"",                                          // 33 - AWS
	"https://api.cohere.ai",                     // 34 - Cohere
	"https://api.minimax.chat",                  // 35 - MiniMax
	"",                                          // 36 - SunoAPI
	"https://api.dify.ai",                       // 37 - Dify
	"https://api.jina.ai",                       // 38 - Jina
	"https://api.cloudflare.com",                // 39 - Cloudflare
	"https://api.siliconflow.cn",                // 40 - SiliconFlow
	"",                                          // 41 - VertexAI
	"https://api.mistral.ai",                    // 42 - Mistral
	"https://api.deepseek.com",                  // 43 - DeepSeek
	"https://api.moka.ai",                       // 44 - MokaAI
	"https://ark.cn-beijing.volces.com",         // 45 - VolcEngine
	"https://qianfan.baidubce.com",              // 46 - BaiduV2
	"",                                          // 47 - Xinference
	"https://api.x.ai",                          // 48 - xAI
	"https://api.coze.cn",                       // 49 - Coze
	"https://api.klingai.com",                   // 50 - Kling
	"https://visual.volcengineapi.com",          // 51 - Jimeng
	"https://api.vidu.cn",                       // 52 - Vidu
	"https://llm.submodel.ai",                   // 53 - Submodel
	"https://ark.cn-beijing.volces.com",         // 54 - DoubaoVideo
	"https://api.openai.com",                    // 55 - Sora
	"https://api.replicate.com",                 // 56 - Replicate
	"https://chatgpt.com",                       // 57 - Codex
}

// ChannelTypeNames 渠道类型名称映射
// 用于日志显示和前端展示
var ChannelTypeNames = map[int]string{
	ChannelTypeUnknown:        "Unknown",
	ChannelTypeOpenAI:         "OpenAI",
	ChannelTypeMidjourney:     "Midjourney",
	ChannelTypeAzure:          "Azure",
	ChannelTypeOllama:         "Ollama",
	ChannelTypeMidjourneyPlus: "MidjourneyPlus",
	ChannelTypeOpenAIMax:      "OpenAIMax",
	ChannelTypeOhMyGPT:        "OhMyGPT",
	ChannelTypeCustom:         "Custom",
	ChannelTypeAILS:           "AILS",
	ChannelTypeAIProxy:        "AIProxy",
	ChannelTypePaLM:           "PaLM",
	ChannelTypeAPI2GPT:        "API2GPT",
	ChannelTypeAIGC2D:         "AIGC2D",
	ChannelTypeAnthropic:      "Anthropic",
	ChannelTypeBaidu:          "Baidu",
	ChannelTypeZhipu:          "Zhipu",
	ChannelTypeAli:            "Ali",
	ChannelTypeXunfei:         "Xunfei",
	ChannelType360:            "360",
	ChannelTypeOpenRouter:     "OpenRouter",
	ChannelTypeAIProxyLibrary: "AIProxyLibrary",
	ChannelTypeFastGPT:        "FastGPT",
	ChannelTypeTencent:        "Tencent",
	ChannelTypeGemini:         "Gemini",
	ChannelTypeMoonshot:       "Moonshot",
	ChannelTypeZhipu_v4:       "ZhipuV4",
	ChannelTypePerplexity:     "Perplexity",
	ChannelTypeLingYiWanWu:    "LingYiWanWu",
	ChannelTypeAws:            "AWS",
	ChannelTypeCohere:         "Cohere",
	ChannelTypeMiniMax:        "MiniMax",
	ChannelTypeSunoAPI:        "SunoAPI",
	ChannelTypeDify:           "Dify",
	ChannelTypeJina:           "Jina",
	ChannelCloudflare:         "Cloudflare",
	ChannelTypeSiliconFlow:    "SiliconFlow",
	ChannelTypeVertexAi:       "VertexAI",
	ChannelTypeMistral:        "Mistral",
	ChannelTypeDeepSeek:       "DeepSeek",
	ChannelTypeMokaAI:         "MokaAI",
	ChannelTypeVolcEngine:     "VolcEngine",
	ChannelTypeBaiduV2:        "BaiduV2",
	ChannelTypeXinference:     "Xinference",
	ChannelTypeXai:            "xAI",
	ChannelTypeCoze:           "Coze",
	ChannelTypeKling:          "Kling",
	ChannelTypeJimeng:         "Jimeng",
	ChannelTypeVidu:           "Vidu",
	ChannelTypeSubmodel:       "Submodel",
	ChannelTypeDoubaoVideo:    "DoubaoVideo",
	ChannelTypeSora:           "Sora",
	ChannelTypeReplicate:      "Replicate",
	ChannelTypeCodex:          "Codex",
}

// GetChannelTypeName 获取渠道类型名称
//
// 参数：
//   - channelType: 渠道类型常量
//
// 返回值：
//   - string: 渠道类型名称，未知类型返回 "Unknown"
func GetChannelTypeName(channelType int) string {
	if name, ok := ChannelTypeNames[channelType]; ok {
		return name
	}
	return "Unknown"
}

// ChannelSpecialBase 渠道特殊基础 URL 结构体
// 某些渠道可能有不同的 Claude 和 OpenAI 基础 URL
type ChannelSpecialBase struct {
	ClaudeBaseURL string // Claude API 基础 URL
	OpenAIBaseURL string // OpenAI API 基础 URL
}

// ChannelSpecialBases 渠道特殊基础 URL 映射
// 键为渠道标识符，值为特殊基础 URL 配置
var ChannelSpecialBases = map[string]ChannelSpecialBase{
	"glm-coding-plan": {
		ClaudeBaseURL: "https://open.bigmodel.cn/api/anthropic",
		OpenAIBaseURL: "https://open.bigmodel.cn/api/coding/paas/v4",
	},
	"glm-coding-plan-international": {
		ClaudeBaseURL: "https://api.z.ai/api/anthropic",
		OpenAIBaseURL: "https://api.z.ai/api/coding/paas/v4",
	},
	"kimi-coding-plan": {
		ClaudeBaseURL: "https://api.kimi.com/coding",
		OpenAIBaseURL: "https://api.kimi.com/coding/v1",
	},
	"doubao-coding-plan": {
		ClaudeBaseURL: "https://ark.cn-beijing.volces.com/api/coding",
		OpenAIBaseURL: "https://ark.cn-beijing.volces.com/api/coding/v3",
	},
}
