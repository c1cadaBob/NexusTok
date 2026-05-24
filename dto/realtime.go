// Package dto - realtime.go
// 该文件定义了 OpenAI Realtime API（实时语音对话）的数据传输对象
//
// 主要结构体：
// - RealtimeEvent：实时事件（统一的事件格式）
// - RealtimeSession：实时会话配置（模态、语音、音频格式等）
// - RealtimeItem：实时对话项（消息、工具调用等）
// - RealtimeContent：实时内容（文本、音频、转录等）
// - RealtimeResponse/RealtimeUsage：实时响应和用量统计
//
// 事件类型常量：
// 客户端事件：error、session.update、conversation.item.create、response.create、input_audio_buffer.append
// 服务端事件：response.done、session.updated/created、response.audio.delta 等
package dto

import "github.com/c1cada/NexusTok/types"

// 客户端发送的实时事件类型常量
const (
	RealtimeEventTypeError              = "error"                      // 错误事件
	RealtimeEventTypeSessionUpdate      = "session.update"             // 会话配置更新
	RealtimeEventTypeConversationCreate = "conversation.item.create"   // 创建对话项
	RealtimeEventTypeResponseCreate     = "response.create"            // 请求生成响应
	RealtimeEventInputAudioBufferAppend = "input_audio_buffer.append"  // 追加音频数据
)

// 服务端返回的实时事件类型常量
const (
	RealtimeEventTypeResponseDone                   = "response.done"                    // 响应完成
	RealtimeEventTypeSessionUpdated                 = "session.updated"                  // 会话已更新
	RealtimeEventTypeSessionCreated                 = "session.created"                  // 会话已创建
	RealtimeEventResponseAudioDelta                 = "response.audio.delta"             // 音频增量
	RealtimeEventResponseAudioTranscriptionDelta    = "response.audio_transcript.delta"  // 音频转录增量
	RealtimeEventResponseFunctionCallArgumentsDelta = "response.function_call_arguments.delta" // 函数调用参数增量
	RealtimeEventResponseFunctionCallArgumentsDone  = "response.function_call_arguments.done"  // 函数调用参数完成
	RealtimeEventConversationItemCreated            = "conversation.item.created"        // 对话项已创建
)

// RealtimeEvent 实时事件（统一的事件格式）
// EventId：事件唯一标识
// Type：事件类型（使用上述常量）
// Session：会话配置（session.update/created/updated 事件）
// Item：对话项（conversation.item.create/created 事件）
// Error：错误信息（error 事件）
// Response：响应信息（response.done 事件）
// Delta：增量文本内容（文本相关 delta 事件）
// Audio：增量音频数据（Base64 编码，response.audio.delta 事件）
type RealtimeEvent struct {
	EventId string `json:"event_id"`
	Type    string `json:"type"`
	//PreviousItemId string `json:"previous_item_id"`
	Session  *RealtimeSession   `json:"session,omitempty"`
	Item     *RealtimeItem      `json:"item,omitempty"`
	Error    *types.OpenAIError `json:"error,omitempty"`
	Response *RealtimeResponse  `json:"response,omitempty"`
	Delta    string             `json:"delta,omitempty"`
	Audio    string             `json:"audio,omitempty"`
}

// RealtimeResponse 实时响应
// Usage：Token 用量统计
type RealtimeResponse struct {
	Usage *RealtimeUsage `json:"usage"`
}

// RealtimeUsage 实时 Token 用量统计
// TotalTokens：总 token 数
// InputTokens：输入 token 数
// OutputTokens：输出 token 数
// InputTokenDetails：输入 token 详细分类
// OutputTokenDetails：输出 token 详细分类
type RealtimeUsage struct {
	TotalTokens        int                `json:"total_tokens"`
	InputTokens        int                `json:"input_tokens"`
	OutputTokens       int                `json:"output_tokens"`
	InputTokenDetails  InputTokenDetails  `json:"input_token_details"`
	OutputTokenDetails OutputTokenDetails `json:"output_token_details"`
}

// RealtimeSession 实时会话配置
// Modalities：支持的模态列表（如 ["text", "audio"]）
// Instructions：系统指令
// Voice：语音角色（如 "alloy"、"echo" 等）
// InputAudioFormat：输入音频格式（如 "pcm16"、"g711_ulaw" 等）
// OutputAudioFormat：输出音频格式
// InputAudioTranscription：输入音频转录配置
// TurnDetection：轮次检测配置（VAD 等）
// Tools：可用工具列表
// ToolChoice：工具选择策略
// Temperature：温度参数
type RealtimeSession struct {
	Modalities              []string                `json:"modalities"`
	Instructions            string                  `json:"instructions"`
	Voice                   string                  `json:"voice"`
	InputAudioFormat        string                  `json:"input_audio_format"`
	OutputAudioFormat       string                  `json:"output_audio_format"`
	InputAudioTranscription InputAudioTranscription `json:"input_audio_transcription"`
	TurnDetection           interface{}             `json:"turn_detection"`
	Tools                   []RealTimeTool          `json:"tools"`
	ToolChoice              string                  `json:"tool_choice"`
	Temperature             float64                 `json:"temperature"`
	//MaxResponseOutputTokens int                     `json:"max_response_output_tokens"`
}

// InputAudioTranscription 输入音频转录配置
// Model：转录模型名称（如 "whisper-1"）
type InputAudioTranscription struct {
	Model string `json:"model"`
}

// RealTimeTool 实时工具定义
// Type：工具类型（"function"）
// Name：函数名称
// Description：函数描述
// Parameters：函数参数定义（JSON Schema）
type RealTimeTool struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

// RealtimeItem 实时对话项
// Id：对话项唯一标识
// Type：类型（"message"/"function_call"/"function_call_output" 等）
// Status：状态（"completed"/"in_progress" 等）
// Role：角色（"user"/"assistant"/"system"）
// Content：内容列表
// Name：函数名称（function_call 类型）
// ToolCalls：工具调用信息
// CallId：工具调用 ID（function_call_output 类型）
type RealtimeItem struct {
	Id        string            `json:"id"`
	Type      string            `json:"type"`
	Status    string            `json:"status"`
	Role      string            `json:"role"`
	Content   []RealtimeContent `json:"content"`
	Name      *string           `json:"name,omitempty"`
	ToolCalls any               `json:"tool_calls,omitempty"`
	CallId    string            `json:"call_id,omitempty"`
}
// RealtimeContent 实时内容
// Type：内容类型（"text"/"audio"/"input_audio" 等）
// Text：文本内容
// Audio：音频数据（Base64 编码）
// Transcript：音频转录文本
type RealtimeContent struct {
	Type       string `json:"type"`
	Text       string `json:"text,omitempty"`
	Audio      string `json:"audio,omitempty"` // Base64-encoded audio bytes.
	Transcript string `json:"transcript,omitempty"`
}
