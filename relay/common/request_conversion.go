// Package common 提供了中继层的通用工具函数和数据结构。
// 本文件负责根据请求对象的 Go 类型推断其对应的中继格式（RelayFormat），
// 并将推断结果追加到 RelayInfo 的请求转换链中。
package common

import (
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/types"
)

// GuessRelayFormatFromRequest 根据请求对象的具体 Go 类型推断其中继格式。
// 通过 type switch 匹配请求对象的类型，返回对应的 RelayFormat 常量。
//
// 参数：
//   - req: 请求对象，支持指针和值类型的双重匹配
//
// 返回值：
//   - types.RelayFormat: 推断出的中继格式（如 OpenAI、Claude、Gemini 等）
//   - bool: 是否成功推断出格式
func GuessRelayFormatFromRequest(req any) (types.RelayFormat, bool) {
	switch req.(type) {
	case *dto.GeneralOpenAIRequest, dto.GeneralOpenAIRequest:
		return types.RelayFormatOpenAI, true
	case *dto.OpenAIResponsesRequest, dto.OpenAIResponsesRequest:
		return types.RelayFormatOpenAIResponses, true
	case *dto.ClaudeRequest, dto.ClaudeRequest:
		return types.RelayFormatClaude, true
	case *dto.GeminiChatRequest, dto.GeminiChatRequest:
		return types.RelayFormatGemini, true
	case *dto.EmbeddingRequest, dto.EmbeddingRequest:
		return types.RelayFormatEmbedding, true
	case *dto.RerankRequest, dto.RerankRequest:
		return types.RelayFormatRerank, true
	case *dto.ImageRequest, dto.ImageRequest:
		return types.RelayFormatOpenAIImage, true
	case *dto.AudioRequest, dto.AudioRequest:
		return types.RelayFormatOpenAIAudio, true
	default:
		return "", false
	}
}

// AppendRequestConversionFromRequest 根据请求对象类型推断中继格式并追加到转换链。
// 这是 GuessRelayFormatFromRequest 的便捷封装，用于在请求处理流程中
// 自动记录请求经历了哪些格式转换。
//
// 参数：
//   - info: 中继信息对象，其 RequestConversionChain 将被追加新格式
//   - req: 请求对象
func AppendRequestConversionFromRequest(info *RelayInfo, req any) {
	if info == nil {
		return
	}
	format, ok := GuessRelayFormatFromRequest(req)
	if !ok {
		return
	}
	info.AppendRequestConversion(format)
}
