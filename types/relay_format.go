// Package types - relay_format.go
// 该文件定义了中继格式（RelayFormat）类型和常量
//
// 主要类型：
// - RelayFormat：中继格式类型
//
// 常量定义：
// - RelayFormatOpenAI：OpenAI 格式
// - RelayFormatClaude：Claude 格式
// - RelayFormatGemini：Gemini 格式
// - RelayFormatOpenAIResponses：OpenAI Responses 格式
//
// 用途：
// - 标识请求/响应的格式类型
// - 在中继层进行格式转换
package types

type RelayFormat string

const (
	RelayFormatOpenAI                    RelayFormat = "openai"
	RelayFormatClaude                                = "claude"
	RelayFormatGemini                                = "gemini"
	RelayFormatOpenAIResponses                       = "openai_responses"
	RelayFormatOpenAIResponsesCompaction             = "openai_responses_compaction"
	RelayFormatOpenAIAudio                           = "openai_audio"
	RelayFormatOpenAIImage                           = "openai_image"
	RelayFormatOpenAIRealtime                        = "openai_realtime"
	RelayFormatRerank                                = "rerank"
	RelayFormatEmbedding                             = "embedding"

	RelayFormatTask    = "task"
	RelayFormatMjProxy = "mj_proxy"
)
