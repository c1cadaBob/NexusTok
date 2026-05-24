// 本文件定义了 Google Gemini 渠道的模型列表、安全设置和渠道名称常量。
// Gemini 是 Google 的大语言模型系列，支持多种模型变体，
// 包括文本生成、图像生成、视频生成、嵌入和视觉理解等能力。
package gemini

// ModelList 定义了 Gemini 渠道支持的模型列表。
// 按功能分类：
//   - 稳定版本：经过充分测试的生产就绪模型
//   - 最新版本：始终指向最新发布的模型
//   - 预览版本：尚未正式发布的实验性模型（含 TTS、图像生成、机器人等）
//   - Gemma 系列：开源轻量级模型
//   - Embedding 模型：文本嵌入模型
//   - Imagen 模型：图像生成模型
//   - Veo 模型：视频生成模型
//   - 其他模型：如 AQA（自动问答）
var ModelList = []string{
	// 稳定版本
	"gemini-2.5-flash", "gemini-2.5-pro", "gemini-2.0-flash",
	"gemini-2.0-flash-001", "gemini-2.0-flash-lite-001", "gemini-2.0-flash-lite",
	"gemini-2.5-flash-lite",
	// 最新版本
	"gemini-flash-latest", "gemini-flash-lite-latest", "gemini-pro-latest",
	"gemini-2.5-flash-native-audio-latest",
	// 预览版本
	"gemini-2.5-flash-preview-tts", "gemini-2.5-pro-preview-tts",
	"gemini-2.5-flash-image", "gemini-2.5-flash-lite-preview-09-2025",
	"gemini-3-pro-preview", "gemini-3-flash-preview", "gemini-3.1-pro-preview",
	"gemini-3.1-pro-preview-customtools", "gemini-3.1-flash-lite-preview",
	"gemini-3-pro-image-preview", "nano-banana-pro-preview",
	"gemini-3.1-flash-image-preview", "gemini-robotics-er-1.5-preview",
	"gemini-2.5-computer-use-preview-10-2025", "deep-research-pro-preview-12-2025",
	"gemini-2.5-flash-native-audio-preview-09-2025", "gemini-2.5-flash-native-audio-preview-12-2025",
	// Gemma 开源模型系列
	"gemma-3-1b-it", "gemma-3-4b-it", "gemma-3-12b-it",
	"gemma-3-27b-it", "gemma-3n-e4b-it", "gemma-3n-e2b-it",
	// 文本嵌入模型
	"gemini-embedding-001", "gemini-embedding-2-preview",
	// Imagen 图像生成模型
	"imagen-4.0-generate-001", "imagen-4.0-ultra-generate-001",
	"imagen-4.0-fast-generate-001",
	// Veo 视频生成模型
	"veo-2.0-generate-001", "veo-3.0-generate-001", "veo-3.0-fast-generate-001",
	"veo-3.1-generate-preview", "veo-3.1-fast-generate-preview",
	// 其他模型
	"aqa",
}

// SafetySettingList 定义了 Gemini API 支持的安全过滤类别。
// 用于配置内容生成时的安全过滤级别。
var SafetySettingList = []string{
	"HARM_CATEGORY_HARASSMENT",       // 骚扰内容
	"HARM_CATEGORY_HATE_SPEECH",      // 仇恨言论
	"HARM_CATEGORY_SEXUALLY_EXPLICIT", // 色情内容
	"HARM_CATEGORY_DANGEROUS_CONTENT", // 危险内容
	//"HARM_CATEGORY_CIVIC_INTEGRITY", // 此项已弃用！
}

// ChannelName 定义了渠道名称标识符。
var ChannelName = "google gemini"
