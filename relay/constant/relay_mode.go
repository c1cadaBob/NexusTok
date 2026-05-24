// Package constant 定义了中继层使用的各种常量。
// 本文件定义了中继模式（RelayMode）常量及其从 URL 路径到模式的映射函数。
// 中继模式用于标识请求的类型（如聊天补全、嵌入、图像生成等），
// 以便后续路由到对应的处理逻辑。
package constant

import (
	"net/http"
	"strings"
)

// 中继模式常量定义，使用 iota 自增。
// 每个常量代表一种 API 请求类型。
const (
	RelayModeUnknown = iota // 未知模式
	RelayModeChatCompletions // 聊天补全（/v1/chat/completions）
	RelayModeCompletions     // 文本补全（/v1/completions）
	RelayModeEmbeddings      // 文本嵌入（/v1/embeddings）
	RelayModeModerations     // 内容审核（/v1/moderations）
	RelayModeImagesGenerations // 图像生成（/v1/images/generations）
	RelayModeImagesEdits     // 图像编辑（/v1/images/edits）
	RelayModeEdits           // 文本编辑（/v1/edits）

	// Midjourney 相关模式
	RelayModeMidjourneyImagine           // Midjourney 想象/生成
	RelayModeMidjourneyDescribe          // Midjourney 描述
	RelayModeMidjourneyBlend             // Midjourney 混合
	RelayModeMidjourneyChange            // Midjourney 变换
	RelayModeMidjourneySimpleChange      // Midjourney 简单变换
	RelayModeMidjourneyNotify            // Midjourney 通知回调
	RelayModeMidjourneyTaskFetch         // Midjourney 任务查询
	RelayModeMidjourneyTaskImageSeed     // Midjourney 图像种子查询
	RelayModeMidjourneyTaskFetchByCondition // Midjourney 按条件查询任务
	RelayModeMidjourneyAction            // Midjourney 动作（midjourney plus）
	RelayModeMidjourneyModal             // Midjourney 模态框（midjourney plus）
	RelayModeMidjourneyShorten           // Midjourney 缩短提示词（midjourney plus）
	RelayModeSwapFace                    // 换脸功能（midjourney plus）
	RelayModeMidjourneyUpload            // Midjourney 上传图片（midjourney plus）
	RelayModeMidjourneyVideo             // Midjourney 视频生成
	RelayModeMidjourneyEdits             // Midjourney 图片编辑

	// 音频相关模式
	RelayModeAudioSpeech        // 文本转语音 TTS
	RelayModeAudioTranscription // 语音转文本 Whisper
	RelayModeAudioTranslation   // 语音翻译 Whisper

	// Suno 音乐生成相关模式
	RelayModeSunoFetch      // Suno 任务列表查询
	RelayModeSunoFetchByID  // Suno 按 ID 查询任务
	RelayModeSunoSubmit     // Suno 提交生成任务

	// 视频生成相关模式
	RelayModeVideoFetchByID // 按 ID 查询视频任务
	RelayModeVideoSubmit    // 提交视频生成任务

	RelayModeRerank    // 重排序（Rerank）
	RelayModeResponses // OpenAI Responses API
	RelayModeRealtime  // 实时通信 API
	RelayModeGemini    // Gemini 原生 API

	RelayModeResponsesCompact // OpenAI Responses 精简模式
)

// Path2RelayMode 根据请求 URL 路径解析对应的中继模式。
// 按路径前缀进行匹配，优先匹配更具体的路径（如 /v1/chat/completions）。
// 对于以 /mj 开头的路径，会进一步委托给 Path2RelayModeMidjourney 处理。
//
// 参数：
//   - path: 请求的 URL 路径
//
// 返回值：
//   - int: 对应的中继模式常量；未匹配则返回 RelayModeUnknown
func Path2RelayMode(path string) int {
	relayMode := RelayModeUnknown
	if strings.HasPrefix(path, "/v1/chat/completions") || strings.HasPrefix(path, "/pg/chat/completions") {
		relayMode = RelayModeChatCompletions
	} else if strings.HasPrefix(path, "/v1/completions") {
		relayMode = RelayModeCompletions
	} else if strings.HasPrefix(path, "/v1/embeddings") {
		relayMode = RelayModeEmbeddings
	} else if strings.HasSuffix(path, "embeddings") {
		relayMode = RelayModeEmbeddings
	} else if strings.HasPrefix(path, "/v1/moderations") {
		relayMode = RelayModeModerations
	} else if strings.HasPrefix(path, "/v1/images/generations") {
		relayMode = RelayModeImagesGenerations
	} else if strings.HasPrefix(path, "/v1/images/edits") {
		relayMode = RelayModeImagesEdits
	} else if strings.HasPrefix(path, "/v1/edits") {
		relayMode = RelayModeEdits
	} else if strings.HasPrefix(path, "/v1/responses/compact") {
		relayMode = RelayModeResponsesCompact
	} else if strings.HasPrefix(path, "/v1/responses") {
		relayMode = RelayModeResponses
	} else if strings.HasPrefix(path, "/v1/audio/speech") {
		relayMode = RelayModeAudioSpeech
	} else if strings.HasPrefix(path, "/v1/audio/transcriptions") {
		relayMode = RelayModeAudioTranscription
	} else if strings.HasPrefix(path, "/v1/audio/translations") {
		relayMode = RelayModeAudioTranslation
	} else if strings.HasPrefix(path, "/v1/rerank") {
		relayMode = RelayModeRerank
	} else if strings.HasPrefix(path, "/v1/realtime") {
		relayMode = RelayModeRealtime
	} else if strings.HasPrefix(path, "/v1beta/models") || strings.HasPrefix(path, "/v1/models") {
		relayMode = RelayModeGemini
	} else if strings.HasPrefix(path, "/mj") {
		relayMode = Path2RelayModeMidjourney(path)
	}
	return relayMode
}

// Path2RelayModeMidjourney 根据 Midjourney 相关路径解析具体的中继模式。
// 通过路径后缀匹配各种 Midjourney 操作（如 imagine、blend、describe 等）。
//
// 参数：
//   - path: 以 /mj 开头的请求路径
//
// 返回值：
//   - int: 对应的 Midjourney 中继模式常量
func Path2RelayModeMidjourney(path string) int {
	relayMode := RelayModeUnknown
	if strings.HasSuffix(path, "/mj/submit/action") {
		// midjourney plus
		relayMode = RelayModeMidjourneyAction
	} else if strings.HasSuffix(path, "/mj/submit/modal") {
		// midjourney plus
		relayMode = RelayModeMidjourneyModal
	} else if strings.HasSuffix(path, "/mj/submit/shorten") {
		// midjourney plus
		relayMode = RelayModeMidjourneyShorten
	} else if strings.HasSuffix(path, "/mj/insight-face/swap") {
		// midjourney plus
		relayMode = RelayModeSwapFace
	} else if strings.HasSuffix(path, "/submit/upload-discord-images") {
		// midjourney plus
		relayMode = RelayModeMidjourneyUpload
	} else if strings.HasSuffix(path, "/mj/submit/imagine") {
		relayMode = RelayModeMidjourneyImagine
	} else if strings.HasSuffix(path, "/mj/submit/video") {
		relayMode = RelayModeMidjourneyVideo
	} else if strings.HasSuffix(path, "/mj/submit/edits") {
		relayMode = RelayModeMidjourneyEdits
	} else if strings.HasSuffix(path, "/mj/submit/blend") {
		relayMode = RelayModeMidjourneyBlend
	} else if strings.HasSuffix(path, "/mj/submit/describe") {
		relayMode = RelayModeMidjourneyDescribe
	} else if strings.HasSuffix(path, "/mj/notify") {
		relayMode = RelayModeMidjourneyNotify
	} else if strings.HasSuffix(path, "/mj/submit/change") {
		relayMode = RelayModeMidjourneyChange
	} else if strings.HasSuffix(path, "/mj/submit/simple-change") {
		relayMode = RelayModeMidjourneyChange
	} else if strings.HasSuffix(path, "/fetch") {
		relayMode = RelayModeMidjourneyTaskFetch
	} else if strings.HasSuffix(path, "/image-seed") {
		relayMode = RelayModeMidjourneyTaskImageSeed
	} else if strings.HasSuffix(path, "/list-by-condition") {
		relayMode = RelayModeMidjourneyTaskFetchByCondition
	}
	return relayMode
}

// Path2RelaySuno 根据 HTTP 方法和路径解析 Suno 相关的中继模式。
// Suno API 使用 POST /fetch 查询任务列表、GET /fetch/{id} 查询单个任务、/submit/ 提交任务。
//
// 参数：
//   - method: HTTP 请求方法（GET/POST）
//   - path: 请求的 URL 路径
//
// 返回值：
//   - int: 对应的 Suno 中继模式常量
func Path2RelaySuno(method, path string) int {
	relayMode := RelayModeUnknown
	if method == http.MethodPost && strings.HasSuffix(path, "/fetch") {
		relayMode = RelayModeSunoFetch
	} else if method == http.MethodGet && strings.Contains(path, "/fetch/") {
		relayMode = RelayModeSunoFetchByID
	} else if strings.Contains(path, "/submit/") {
		relayMode = RelayModeSunoSubmit
	}
	return relayMode
}
