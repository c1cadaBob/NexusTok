// Package dto - openai_response.go
// 该文件定义了 OpenAI 兼容 API 的响应数据传输对象（DTO）
//
// 主要结构体：
// - SimpleResponse：简单响应（仅包含用量和错误）
// - TextResponse/OpenAITextResponse：文本补全响应
// - OpenAIEmbeddingResponse/FlexibleEmbeddingResponse：嵌入响应
// - ChatCompletionsStreamResponse：聊天补全流式响应
// - OpenAIResponsesResponse：Responses API 响应
// - ResponsesStreamResponse：Responses API 流式响应
// - Usage：Token 用量统计
//
// 辅助函数：
// - GetOpenAIError：从动态错误类型中提取 OpenAI 标准错误
package dto

import (
	"encoding/json"
	"fmt"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/types"
)

// Responses API 输出类型常量
const (
	ResponsesOutputTypeImageGenerationCall = "image_generation_call" // 图像生成调用输出
)

// SimpleResponse 简单响应结构体（用于不需要完整响应的场景）
// Usage：Token 用量统计
// Error：错误信息
type SimpleResponse struct {
	Usage `json:"usage"`
	Error any `json:"error"`
}

// GetOpenAIError 从动态错误类型中提取OpenAIError结构
func (s *SimpleResponse) GetOpenAIError() *types.OpenAIError {
	return GetOpenAIError(s.Error)
}

// TextResponse 文本补全响应结构体（OpenAI 格式）
// Id：响应唯一标识
// Object：对象类型（如 "text_completion"）
// Created：创建时间戳
// Model：实际使用的模型名称
// Choices：生成的文本选项列表
// Usage：Token 用量统计
type TextResponse struct {
	Id      string                     `json:"id"`
	Object  string                     `json:"object"`
	Created int64                      `json:"created"`
	Model   string                     `json:"model"`
	Choices []OpenAITextResponseChoice `json:"choices"`
	Usage   `json:"usage"`
}

// OpenAITextResponseChoice 文本补全选项
// Index：选项索引
// Message：消息内容
// FinishReason：结束原因（stop/length/content_filter 等）
type OpenAITextResponseChoice struct {
	Index        int `json:"index"`
	Message      `json:"message"`
	FinishReason string `json:"finish_reason"`
}

// OpenAITextResponse OpenAI 文本补全响应
// Id：响应唯一标识
// Model：实际使用的模型名称
// Object：对象类型（如 "text_completion"）
// Created：创建时间戳（支持数字和字符串格式）
// Choices：生成的文本选项列表
// Error：错误信息（可选）
// Usage：Token 用量统计
type OpenAITextResponse struct {
	Id      string                     `json:"id"`
	Model   string                     `json:"model"`
	Object  string                     `json:"object"`
	Created any                        `json:"created"`
	Choices []OpenAITextResponseChoice `json:"choices"`
	Error   any                        `json:"error,omitempty"`
	Usage   `json:"usage"`
}

// GetOpenAIError 从动态错误类型中提取OpenAIError结构
func (o *OpenAITextResponse) GetOpenAIError() *types.OpenAIError {
	return GetOpenAIError(o.Error)
}

// OpenAIEmbeddingResponseItem OpenAI 嵌入结果项
// Object：对象类型（"embedding"）
// Index：在输入数组中的索引
// Embedding：嵌入向量（浮点数数组）
type OpenAIEmbeddingResponseItem struct {
	Object    string    `json:"object"`
	Index     int       `json:"index"`
	Embedding []float64 `json:"embedding"`
}

// OpenAIEmbeddingResponse OpenAI 嵌入响应
// Object：对象类型（"list"）
// Data：嵌入结果列表
// Model：实际使用的模型名称
// Usage：Token 用量统计
type OpenAIEmbeddingResponse struct {
	Object string                        `json:"object"`
	Data   []OpenAIEmbeddingResponseItem `json:"data"`
	Model  string                        `json:"model"`
	Usage  `json:"usage"`
}

// FlexibleEmbeddingResponseItem 灵活嵌入结果项（支持不同格式的向量）
// Object：对象类型
// Index：索引
// Embedding：嵌入向量（any 类型，支持数组和 Base64 字符串）
type FlexibleEmbeddingResponseItem struct {
	Object    string `json:"object"`
	Index     int    `json:"index"`
	Embedding any    `json:"embedding"`
}

// FlexibleEmbeddingResponse 灵活嵌入响应（支持不同格式的向量）
// Object：对象类型
// Data：嵌入结果列表
// Model：实际使用的模型名称
// Usage：Token 用量统计
type FlexibleEmbeddingResponse struct {
	Object string                          `json:"object"`
	Data   []FlexibleEmbeddingResponseItem `json:"data"`
	Model  string                          `json:"model"`
	Usage  `json:"usage"`
}

// ChatCompletionsStreamResponseChoice 聊天补全流式响应选项
// Delta：增量内容（流式响应的核心）
// Logprobs：对数概率
// FinishReason：结束原因（stop/length/tool_calls/content_filter 等）
// Index：选项索引
type ChatCompletionsStreamResponseChoice struct {
	Delta        ChatCompletionsStreamResponseChoiceDelta `json:"delta,omitempty"`
	Logprobs     *any                                     `json:"logprobs"`
	FinishReason *string                                  `json:"finish_reason"`
	Index        int                                      `json:"index"`
}

// ChatCompletionsStreamResponseChoiceDelta 流式响应增量内容
// Content：文本内容增量
// ReasoningContent/Reasoning：推理内容增量（o1/o3 等推理模型）
// Role：消息角色（仅首个 chunk 包含）
// ToolCalls：工具调用增量（函数调用场景）
type ChatCompletionsStreamResponseChoiceDelta struct {
	Content          *string            `json:"content,omitempty"`
	ReasoningContent *string            `json:"reasoning_content,omitempty"`
	Reasoning        *string            `json:"reasoning,omitempty"`
	Role             string             `json:"role,omitempty"`
	ToolCalls        []ToolCallResponse `json:"tool_calls,omitempty"`
}

// SetContentString 设置文本内容（指针方式，区分空字符串和未设置）
func (c *ChatCompletionsStreamResponseChoiceDelta) SetContentString(s string) {
	c.Content = &s
}

// GetContentString 获取文本内容
// 如果 Content 为 nil 返回空字符串
func (c *ChatCompletionsStreamResponseChoiceDelta) GetContentString() string {
	if c.Content == nil {
		return ""
	}
	return *c.Content
}

// GetReasoningContent 获取推理内容
// 优先返回 ReasoningContent，其次返回 Reasoning（兼容不同上游格式）
func (c *ChatCompletionsStreamResponseChoiceDelta) GetReasoningContent() string {
	if c.ReasoningContent == nil && c.Reasoning == nil {
		return ""
	}
	if c.ReasoningContent != nil {
		return *c.ReasoningContent
	}
	return *c.Reasoning
}

// SetReasoningContent 设置推理内容
func (c *ChatCompletionsStreamResponseChoiceDelta) SetReasoningContent(s string) {
	c.ReasoningContent = &s
	//c.Reasoning = &s
}

// ToolCallResponse 工具调用响应
// Index：工具调用索引（仅流式响应中非 nil）
// ID：工具调用唯一标识
// Type：工具类型（"function"）
// Function：函数调用详情
type ToolCallResponse struct {
	// Index is not nil only in chat completion chunk object
	Index    *int             `json:"index,omitempty"`
	ID       string           `json:"id,omitempty"`
	Type     any              `json:"type"`
	Function FunctionResponse `json:"function"`
}

// SetIndex 设置工具调用索引
func (c *ToolCallResponse) SetIndex(i int) {
	c.Index = &i
}

// FunctionResponse 函数响应
// Description：函数描述（请求时使用）
// Name：函数名称
// Parameters：函数参数定义（请求时使用）
// Arguments：函数调用参数 JSON 字符串（响应时使用）
type FunctionResponse struct {
	Description string `json:"description,omitempty"`
	Name        string `json:"name,omitempty"`
	// call function with arguments in JSON format
	Parameters any    `json:"parameters,omitempty"` // request
	Arguments  string `json:"arguments"`            // response
}

// ChatCompletionsStreamResponse 聊天补全流式响应
// Id：响应唯一标识
// Object：对象类型（"chat.completion.chunk"）
// Created：创建时间戳
// Model：实际使用的模型名称
// SystemFingerprint：系统指纹（用于检测配置变更）
// Choices：生成的选项列表（通常只有一个）
// Usage：Token 用量统计（仅最后一个 chunk 包含）
type ChatCompletionsStreamResponse struct {
	Id                string                                `json:"id"`
	Object            string                                `json:"object"`
	Created           int64                                 `json:"created"`
	Model             string                                `json:"model"`
	SystemFingerprint *string                               `json:"system_fingerprint"`
	Choices           []ChatCompletionsStreamResponseChoice `json:"choices"`
	Usage             *Usage                                `json:"usage"`
}

// IsFinished 判断流式响应是否已完成
// 当第一个选项的 FinishReason 非空时返回 true
func (c *ChatCompletionsStreamResponse) IsFinished() bool {
	if len(c.Choices) == 0 {
		return false
	}
	return c.Choices[0].FinishReason != nil && *c.Choices[0].FinishReason != ""
}

// IsToolCall 判断流式响应是否包含工具调用
// 当第一个选项的 ToolCalls 非空时返回 true
func (c *ChatCompletionsStreamResponse) IsToolCall() bool {
	if len(c.Choices) == 0 {
		return false
	}
	return len(c.Choices[0].Delta.ToolCalls) > 0
}

// GetFirstToolCall 获取第一个工具调用
// 如果没有工具调用返回 nil
func (c *ChatCompletionsStreamResponse) GetFirstToolCall() *ToolCallResponse {
	if c.IsToolCall() {
		return &c.Choices[0].Delta.ToolCalls[0]
	}
	return nil
}

// ClearToolCalls 清除工具调用的详细信息（保留结构，清空内容）
// 用于在流式响应中移除重复的工具调用元数据
func (c *ChatCompletionsStreamResponse) ClearToolCalls() {
	if !c.IsToolCall() {
		return
	}
	for choiceIdx := range c.Choices {
		for callIdx := range c.Choices[choiceIdx].Delta.ToolCalls {
			c.Choices[choiceIdx].Delta.ToolCalls[callIdx].ID = ""
			c.Choices[choiceIdx].Delta.ToolCalls[callIdx].Type = nil
			c.Choices[choiceIdx].Delta.ToolCalls[callIdx].Function.Name = ""
		}
	}
}

// Copy 深拷贝流式响应（Choices 使用新切片，其他字段共享）
func (c *ChatCompletionsStreamResponse) Copy() *ChatCompletionsStreamResponse {
	choices := make([]ChatCompletionsStreamResponseChoice, len(c.Choices))
	copy(choices, c.Choices)
	return &ChatCompletionsStreamResponse{
		Id:                c.Id,
		Object:            c.Object,
		Created:           c.Created,
		Model:             c.Model,
		SystemFingerprint: c.SystemFingerprint,
		Choices:           choices,
		Usage:             c.Usage,
	}
}

// GetSystemFingerprint 获取系统指纹
// 如果 SystemFingerprint 为 nil 返回空字符串
func (c *ChatCompletionsStreamResponse) GetSystemFingerprint() string {
	if c.SystemFingerprint == nil {
		return ""
	}
	return *c.SystemFingerprint
}

// SetSystemFingerprint 设置系统指纹
func (c *ChatCompletionsStreamResponse) SetSystemFingerprint(s string) {
	c.SystemFingerprint = &s
}

// ChatCompletionsStreamResponseSimple 简化版流式响应（用于内部处理）
// Choices：选项列表
// Usage：Token 用量统计
type ChatCompletionsStreamResponseSimple struct {
	Choices []ChatCompletionsStreamResponseChoice `json:"choices"`
	Usage   *Usage                                `json:"usage"`
}

// CompletionsStreamResponse 旧版文本补全流式响应
// Choices：选项列表
// Text：生成的文本
// FinishReason：结束原因
type CompletionsStreamResponse struct {
	Choices []struct {
		Text         string `json:"text"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

// Usage Token 用量统计
// PromptTokens：输入提示词 token 数
// CompletionTokens：输出补全 token 数
// TotalTokens：总 token 数
// PromptCacheHitTokens：缓存命中 token 数
// UsageSemantic：用量语义标识
// UsageSource：用量来源标识
// PromptTokensDetails：输入 token 详细分类（缓存、文本、音频、图像）
// CompletionTokenDetails：输出 token 详细分类（文本、音频、图像、推理）
// InputTokens/OutputTokens：Claude 格式的输入/输出 token 数
// InputTokensDetails：Claude 格式的输入 token 详情
// ClaudeCacheCreation5mTokens/ClaudeCacheCreation1hTokens：Claude 缓存创建 token 数
// Cost：OpenRouter 等平台的费用信息
type Usage struct {
	PromptTokens         int    `json:"prompt_tokens"`
	CompletionTokens     int    `json:"completion_tokens"`
	TotalTokens          int    `json:"total_tokens"`
	PromptCacheHitTokens int    `json:"prompt_cache_hit_tokens,omitempty"`
	UsageSemantic        string `json:"usage_semantic,omitempty"`
	UsageSource          string `json:"usage_source,omitempty"`

	PromptTokensDetails    InputTokenDetails  `json:"prompt_tokens_details"`
	CompletionTokenDetails OutputTokenDetails `json:"completion_tokens_details"`
	InputTokens            int                `json:"input_tokens"`
	OutputTokens           int                `json:"output_tokens"`
	InputTokensDetails     *InputTokenDetails `json:"input_tokens_details"`

	// claude cache 1h
	ClaudeCacheCreation5mTokens int `json:"claude_cache_creation_5_m_tokens"`
	ClaudeCacheCreation1hTokens int `json:"claude_cache_creation_1_h_tokens"`

	// OpenRouter Params
	Cost any `json:"cost,omitempty"`
}

// OpenAIVideoResponse OpenAI 视频文件响应
// Id：文件唯一标识
// Object：对象类型（"file"）
// Bytes：文件大小（字节）
// CreatedAt：创建时间戳
// ExpiresAt：过期时间戳
// Filename：文件名
// Purpose：用途（如 "fine-tune"、"video_generation" 等）
type OpenAIVideoResponse struct {
	Id        string `json:"id" example:"file-abc123"`
	Object    string `json:"object" example:"file"`
	Bytes     int64  `json:"bytes" example:"120000"`
	CreatedAt int64  `json:"created_at" example:"1677610602"`
	ExpiresAt int64  `json:"expires_at" example:"1677614202"`
	Filename  string `json:"filename" example:"mydata.jsonl"`
	Purpose   string `json:"purpose" example:"fine-tune"`
}

// InputTokenDetails 输入 Token 详细分类
// CachedTokens：缓存命中 token 数
// CachedCreationTokens：缓存创建 token 数
// TextTokens：文本 token 数
// AudioTokens：音频 token 数
// ImageTokens：图像 token 数
type InputTokenDetails struct {
	CachedTokens         int `json:"cached_tokens"`
	CachedCreationTokens int `json:"cached_creation_tokens,omitempty"`
	TextTokens           int `json:"text_tokens"`
	AudioTokens          int `json:"audio_tokens"`
	ImageTokens          int `json:"image_tokens"`
}

// OutputTokenDetails 输出 Token 详细分类
// TextTokens：文本 token 数
// AudioTokens：音频 token 数
// ImageTokens：图像 token 数
// ReasoningTokens：推理 token 数（o1/o3 等推理模型）
type OutputTokenDetails struct {
	TextTokens      int `json:"text_tokens"`
	AudioTokens     int `json:"audio_tokens"`
	ImageTokens     int `json:"image_tokens"`
	ReasoningTokens int `json:"reasoning_tokens"`
}

// OpenAIResponsesResponse OpenAI Responses API 响应
// ID：响应唯一标识
// Object：对象类型（"response"）
// CreatedAt：创建时间戳
// Status：响应状态
// Error：错误信息（可选）
// IncompleteDetails：未完成详情（如 token 限制）
// Instructions：系统指令
// MaxOutputTokens：最大输出 token 数
// Model：实际使用的模型名称
// Output：输出列表（消息、工具调用、图像生成等）
// ParallelToolCalls：是否并行工具调用
// PreviousResponseID：前置响应 ID
// Reasoning：推理配置
// Store：是否存储
// Temperature：温度
// ToolChoice：工具选择策略
// Tools：工具列表
// TopP：Top P 采样
// Truncation：截断配置
// Usage：Token 用量统计
// User：用户标识
// Metadata：扩展元数据
type OpenAIResponsesResponse struct {
	ID                 string             `json:"id"`
	Object             string             `json:"object"`
	CreatedAt          int                `json:"created_at"`
	Status             json.RawMessage    `json:"status"`
	Error              any                `json:"error,omitempty"`
	IncompleteDetails  *IncompleteDetails `json:"incomplete_details,omitempty"`
	Instructions       json.RawMessage    `json:"instructions"`
	MaxOutputTokens    int                `json:"max_output_tokens"`
	Model              string             `json:"model"`
	Output             []ResponsesOutput  `json:"output"`
	ParallelToolCalls  bool               `json:"parallel_tool_calls"`
	PreviousResponseID json.RawMessage    `json:"previous_response_id"`
	Reasoning          *Reasoning         `json:"reasoning"`
	Store              bool               `json:"store"`
	Temperature        float64            `json:"temperature"`
	ToolChoice         json.RawMessage    `json:"tool_choice"`
	Tools              []map[string]any   `json:"tools"`
	TopP               float64            `json:"top_p"`
	Truncation         json.RawMessage    `json:"truncation"`
	Usage              *Usage             `json:"usage"`
	User               json.RawMessage    `json:"user"`
	Metadata           json.RawMessage    `json:"metadata"`
}

// GetOpenAIError 从动态错误类型中提取OpenAIError结构
func (o *OpenAIResponsesResponse) GetOpenAIError() *types.OpenAIError {
	return GetOpenAIError(o.Error)
}

// HasImageGenerationCall 检查响应中是否包含图像生成调用输出
func (o *OpenAIResponsesResponse) HasImageGenerationCall() bool {
	if len(o.Output) == 0 {
		return false
	}
	for _, output := range o.Output {
		if output.Type == ResponsesOutputTypeImageGenerationCall {
			return true
		}
	}
	return false
}

// GetQuality 获取图像生成的质量参数
// 遍历输出找到图像生成调用并返回其质量设置
func (o *OpenAIResponsesResponse) GetQuality() string {
	if len(o.Output) == 0 {
		return ""
	}
	for _, output := range o.Output {
		if output.Type == ResponsesOutputTypeImageGenerationCall {
			return output.Quality
		}
	}
	return ""
}

// GetSize 获取图像生成的尺寸参数
// 遍历输出找到图像生成调用并返回其尺寸设置
func (o *OpenAIResponsesResponse) GetSize() string {
	if len(o.Output) == 0 {
		return ""
	}
	for _, output := range o.Output {
		if output.Type == ResponsesOutputTypeImageGenerationCall {
			return output.Size
		}
	}
	return ""
}

// IncompleteDetails 响应未完成详情
// Reasoning：未完成原因
type IncompleteDetails struct {
	Reasoning string `json:"reasoning"`
}

// ResponsesOutput Responses API 输出项
// Type：输出类型（"message"/"function_call"/"image_generation_call" 等）
// ID：输出唯一标识
// Status：输出状态
// Role：消息角色（"assistant"）
// Content：内容列表（文本、推理摘要等）
// Quality/Size：图像生成参数（仅 image_generation_call 类型）
// CallId：工具调用 ID（仅 function_call 类型）
// Name：函数名称（仅 function_call 类型）
// Arguments：函数调用参数（仅 function_call 类型）
type ResponsesOutput struct {
	Type      string                   `json:"type"`
	ID        string                   `json:"id"`
	Status    string                   `json:"status"`
	Role      string                   `json:"role"`
	Content   []ResponsesOutputContent `json:"content"`
	Quality   string                   `json:"quality"`
	Size      string                   `json:"size"`
	CallId    string                   `json:"call_id,omitempty"`
	Name      string                   `json:"name,omitempty"`
	Arguments json.RawMessage          `json:"arguments,omitempty"`
}

// ArgumentsString 返回函数调用参数的字符串形式
// 用于 Chat Completions 格式兼容
func (r *ResponsesOutput) ArgumentsString() string {
	if r == nil {
		return ""
	}
	return ResponsesArgumentsString(r.Arguments)
}

// ResponsesArgumentsString 将 json.RawMessage 格式的参数转换为字符串
// 用于 Chat Completions 格式兼容
func ResponsesArgumentsString(arguments json.RawMessage) string {
	return common.JsonRawMessageToString(arguments)
}

// ResponsesOutputContent Responses API 输出内容
// Type：内容类型（"output_text"/"reasoning_summary" 等）
// Text：文本内容
// Annotations：注释列表（引用、来源等）
type ResponsesOutputContent struct {
	Type        string        `json:"type"`
	Text        string        `json:"text"`
	Annotations []interface{} `json:"annotations"`
}

// ResponsesReasoningSummaryPart 推理摘要部分
// Type：部分类型（"summary_text"）
// Text：摘要文本
type ResponsesReasoningSummaryPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// 内置工具类型常量
const (
	BuildInToolWebSearchPreview = "web_search_preview" // Web 搜索预览工具
	BuildInToolFileSearch       = "file_search"        // 文件搜索工具
)

// 内置调用类型常量
const (
	BuildInCallWebSearchCall = "web_search_call" // Web 搜索调用
)

// Responses 流式事件类型常量
const (
	ResponsesOutputTypeItemAdded = "response.output_item.added" // 输出项添加事件
	ResponsesOutputTypeItemDone  = "response.output_item.done"  // 输出项完成事件
)

// ResponsesStreamResponse Responses API 流式响应
// Type：事件类型（如 "response.created"/"response.output_item.added" 等）
// Response：完整响应对象（仅在 response.created/response.completed 等事件中包含）
// Delta：增量文本内容
// Item：输出项（在 output_item 事件中包含）
// OutputIndex：输出索引
// ContentIndex：内容索引
// SummaryIndex：摘要索引
// ItemID：输出项 ID
// Part：推理摘要部分
type ResponsesStreamResponse struct {
	Type     string                   `json:"type"`
	Response *OpenAIResponsesResponse `json:"response,omitempty"`
	Delta    string                   `json:"delta,omitempty"`
	Item     *ResponsesOutput         `json:"item,omitempty"`
	// - response.function_call_arguments.delta
	// - response.function_call_arguments.done
	OutputIndex  *int                           `json:"output_index,omitempty"`
	ContentIndex *int                           `json:"content_index,omitempty"`
	SummaryIndex *int                           `json:"summary_index,omitempty"`
	ItemID       string                         `json:"item_id,omitempty"`
	Part         *ResponsesReasoningSummaryPart `json:"part,omitempty"`
}

// GetOpenAIError 从动态错误类型中提取 OpenAI 标准错误结构
// 支持以下格式：
// - types.OpenAIError 或 *types.OpenAIError：直接返回
// - map[string]interface{}：从 map 中提取字段构建
// - string：包装为简单错误
// - 其他类型：转换为字符串后包装
func GetOpenAIError(errorField any) *types.OpenAIError {
	if errorField == nil {
		return nil
	}

	switch err := errorField.(type) {
	case types.OpenAIError:
		return &err
	case *types.OpenAIError:
		return err
	case map[string]interface{}:
		// 处理从JSON解析来的map结构
		openaiErr := &types.OpenAIError{}
		if errType, ok := err["type"].(string); ok {
			openaiErr.Type = errType
		}
		if errMsg, ok := err["message"].(string); ok {
			openaiErr.Message = errMsg
		}
		if errParam, ok := err["param"].(string); ok {
			openaiErr.Param = errParam
		}
		if errCode, ok := err["code"]; ok {
			openaiErr.Code = errCode
		}
		return openaiErr
	case string:
		// 处理简单字符串错误
		return &types.OpenAIError{
			Type:    "error",
			Message: err,
		}
	default:
		// 未知类型，尝试转换为字符串
		return &types.OpenAIError{
			Type:    "unknown_error",
			Message: fmt.Sprintf("%v", err),
		}
	}
}
