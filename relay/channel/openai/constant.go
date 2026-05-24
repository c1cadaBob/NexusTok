// Package openai 的常量定义文件。
// 定义 OpenAI 渠道支持的模型列表和渠道名称。
// 包含 OpenAI 的完整模型产品线：
// - GPT-3.5 系列
// - GPT-4 系列
// - GPT-4o 系列
// - o1/o3/o4 推理系列
// - GPT-5 系列
// - 音频/实时系列
// - 嵌入模型
// - 图片生成模型
// - 语音模型
// - 视频生成模型
package openai

// ModelList 是 OpenAI 支持的模型名称列表。
// 包含所有可用的模型，按系列分组：
//   - GPT-3.5: gpt-3.5-turbo 及其变体
//   - GPT-4: gpt-4 及其变体
//   - GPT-4o: gpt-4o 及其变体（多模态）
//   - o1/o3/o4: 推理系列模型
//   - GPT-5: 最新一代模型
//   - 音频/实时: 音频和实时对话模型
//   - 嵌入: text-embedding 系列
//   - 图片: dall-e 和 gpt-image 系列
//   - 语音: whisper 和 tts 系列
//   - 视频: sora 系列
var ModelList = []string{
	// GPT-3.5 系列
	"gpt-3.5-turbo", "gpt-3.5-turbo-0613", "gpt-3.5-turbo-1106", "gpt-3.5-turbo-0125",
	"gpt-3.5-turbo-16k", "gpt-3.5-turbo-16k-0613",
	"gpt-3.5-turbo-instruct", "gpt-3.5-turbo-instruct-0914",
	// GPT-4 系列
	"gpt-4", "gpt-4-0613", "gpt-4-1106-preview", "gpt-4-0125-preview",
	"gpt-4-32k", "gpt-4-32k-0613",
	"gpt-4-turbo-preview", "gpt-4-turbo", "gpt-4-turbo-2024-04-09",
	"gpt-4-vision-preview",
	// GPT-4o 系列（多模态）
	"chatgpt-4o-latest",
	"gpt-4o", "gpt-4o-2024-05-13", "gpt-4o-2024-08-06", "gpt-4o-2024-11-20",
	"gpt-4o-transcribe", "gpt-4o-transcribe-diarize",
	"gpt-4o-search-preview", "gpt-4o-search-preview-2025-03-11",
	"gpt-4o-mini", "gpt-4o-mini-2024-07-18",
	"gpt-4o-mini-transcribe", "gpt-4o-mini-transcribe-2025-03-20", "gpt-4o-mini-transcribe-2025-12-15",
	"gpt-4o-mini-tts", "gpt-4o-mini-tts-2025-03-20", "gpt-4o-mini-tts-2025-12-15",
	"gpt-4o-mini-search-preview", "gpt-4o-mini-search-preview-2025-03-11",
	// GPT-4.5 系列
	"gpt-4.5-preview", "gpt-4.5-preview-2025-02-27",
	// GPT-4.1 系列
	"gpt-4.1", "gpt-4.1-2025-04-14",
	"gpt-4.1-mini", "gpt-4.1-mini-2025-04-14",
	"gpt-4.1-nano", "gpt-4.1-nano-2025-04-14",
	// o1 推理系列
	"o1", "o1-2024-12-17",
	"o1-preview", "o1-preview-2024-09-12",
	"o1-mini", "o1-mini-2024-09-12",
	"o1-pro", "o1-pro-2025-03-19",
	// o3 推理系列
	"o3-mini", "o3-mini-2025-01-31",
	"o3-mini-high", "o3-mini-2025-01-31-high",
	"o3-mini-low", "o3-mini-2025-01-31-low",
	"o3-mini-medium", "o3-mini-2025-01-31-medium",
	"o3", "o3-2025-04-16",
	"o3-pro", "o3-pro-2025-06-10",
	"o3-deep-research", "o3-deep-research-2025-06-26",
	// o4 推理系列
	"o4-mini", "o4-mini-2025-04-16",
	"o4-mini-deep-research", "o4-mini-deep-research-2025-06-26",
	// GPT-5 系列
	"gpt-5", "gpt-5-2025-08-07", "gpt-5-chat-latest",
	"gpt-5-mini", "gpt-5-mini-2025-08-07",
	"gpt-5-nano", "gpt-5-nano-2025-08-07",
	"gpt-5-codex",
	"gpt-5-pro", "gpt-5-pro-2025-10-06",
	"gpt-5-search-api", "gpt-5-search-api-2025-10-14",
	"gpt-5.1", "gpt-5.1-2025-11-13", "gpt-5.1-chat-latest",
	"gpt-5.1-codex", "gpt-5.1-codex-mini", "gpt-5.1-codex-max",
	"gpt-5.2", "gpt-5.2-2025-12-11", "gpt-5.2-chat-latest",
	"gpt-5.2-pro", "gpt-5.2-pro-2025-12-11",
	"gpt-5.2-codex",
	"gpt-5.3-chat-latest",
	"gpt-5.3-codex",
	"gpt-5.4", "gpt-5.4-2026-03-05",
	"gpt-5.4-pro", "gpt-5.4-pro-2026-03-05",
	// 音频/实时系列
	"gpt-4o-audio-preview", "gpt-4o-audio-preview-2024-10-01", "gpt-4o-audio-preview-2024-12-17", "gpt-4o-audio-preview-2025-06-03",
	"gpt-4o-realtime-preview", "gpt-4o-realtime-preview-2024-10-01", "gpt-4o-realtime-preview-2024-12-17", "gpt-4o-realtime-preview-2025-06-03",
	"gpt-4o-mini-realtime-preview", "gpt-4o-mini-realtime-preview-2024-12-17",
	"gpt-4o-mini-audio-preview", "gpt-4o-mini-audio-preview-2024-12-17",
	"gpt-audio", "gpt-audio-2025-08-28",
	"gpt-audio-mini", "gpt-audio-mini-2025-10-06", "gpt-audio-mini-2025-12-15",
	"gpt-audio-1.5",
	"gpt-realtime", "gpt-realtime-2025-08-28",
	"gpt-realtime-mini", "gpt-realtime-mini-2025-10-06", "gpt-realtime-mini-2025-12-15",
	"gpt-realtime-1.5",
	// 嵌入模型
	"text-embedding-ada-002", "text-embedding-3-small", "text-embedding-3-large",
	// 文本模型
	"text-curie-001", "text-babbage-001", "text-ada-001",
	// 内容审核模型
	"text-moderation-latest", "text-moderation-stable",
	"omni-moderation-latest", "omni-moderation-2024-09-26",
	// 编辑模型
	"text-davinci-edit-001",
	"davinci-002", "babbage-002",
	// 图片生成模型
	"dall-e-2", "dall-e-3",
	"gpt-image-1", "gpt-image-1-mini", "gpt-image-1.5",
	"chatgpt-image-latest",
	// 语音模型
	"whisper-1",
	"tts-1", "tts-1-1106", "tts-1-hd", "tts-1-hd-1106",
	// 计算机使用模型
	"computer-use-preview", "computer-use-preview-2025-03-11",
	// 视频生成模型
	"sora-2", "sora-2-pro",
}

// ChannelName 是 OpenAI 渠道的标识名称。
var ChannelName = "openai"
