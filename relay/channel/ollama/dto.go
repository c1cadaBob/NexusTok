// Package ollama 的数据传输对象 (DTO) 定义文件。
// 定义了 Ollama API 请求和响应的各种结构体，包括：
// - 聊天请求/响应格式
// - 生成请求格式
// - 嵌入请求/响应格式
// - 模型管理请求/响应格式
package ollama

import (
	"encoding/json" // 用于 json.RawMessage 类型定义
)

// OllamaChatMessage 表示 Ollama 聊天消息格式。
// 支持多模态内容（文本+图片）和工具调用。
type OllamaChatMessage struct {
	Role      string           `json:"role"`                 // 消息角色：system/user/assistant/tool
	Content   string           `json:"content,omitempty"`    // 消息文本内容
	Images    []string         `json:"images,omitempty"`     // 图片列表（base64 编码）
	ToolCalls []OllamaToolCall `json:"tool_calls,omitempty"` // 工具调用列表（assistant 消息）
	ToolName  string           `json:"tool_name,omitempty"`  // 工具名称（tool 消息）
	Thinking  json.RawMessage  `json:"thinking,omitempty"`   // 思考内容（用于推理模型）
}

// OllamaToolFunction 定义工具函数的结构。
type OllamaToolFunction struct {
	Name        string      `json:"name"`                   // 函数名称
	Description string      `json:"description,omitempty"`  // 函数描述
	Parameters  interface{} `json:"parameters,omitempty"`   // 函数参数定义（JSON Schema 格式）
}

// OllamaTool 定义工具的结构。
type OllamaTool struct {
	Type     string             `json:"type"`     // 工具类型（通常为 "function"）
	Function OllamaToolFunction `json:"function"` // 工具函数定义
}

// OllamaToolCall 表示工具调用请求。
type OllamaToolCall struct {
	Function struct {
		Name      string      `json:"name"`      // 调用的函数名称
		Arguments interface{} `json:"arguments"` // 函数参数（JSON 对象）
	} `json:"function"` // 函数调用详情
}

// OllamaChatRequest 是 Ollama 聊天 API 的请求格式。
// 对应 Ollama 的 /api/chat 端点。
type OllamaChatRequest struct {
	Model     string              `json:"model"`               // 模型名称
	Messages  []OllamaChatMessage `json:"messages"`            // 消息列表
	Tools     interface{}         `json:"tools,omitempty"`     // 可用工具列表
	Format    interface{}         `json:"format,omitempty"`    // 响应格式（如 JSON Schema）
	Stream    bool                `json:"stream,omitempty"`    // 是否使用流式响应
	Options   map[string]any      `json:"options,omitempty"`   // 模型参数（temperature、top_p 等）
	KeepAlive interface{}         `json:"keep_alive,omitempty"` // 模型保持加载的时间
	Think     json.RawMessage     `json:"think,omitempty"`     // 是否启用思考模式
}

// OllamaGenerateRequest 是 Ollama 生成 API 的请求格式。
// 对应 Ollama 的 /api/generate 端点。
type OllamaGenerateRequest struct {
	Model     string          `json:"model"`               // 模型名称
	Prompt    string          `json:"prompt,omitempty"`     // 提示文本
	Suffix    string          `json:"suffix,omitempty"`     // 后缀文本（用于代码补全）
	Images    []string        `json:"images,omitempty"`     // 图片列表（base64 编码）
	Format    interface{}     `json:"format,omitempty"`     // 响应格式
	Stream    bool            `json:"stream,omitempty"`     // 是否使用流式响应
	Options   map[string]any  `json:"options,omitempty"`    // 模型参数
	KeepAlive interface{}     `json:"keep_alive,omitempty"` // 模型保持加载的时间
	Think     json.RawMessage `json:"think,omitempty"`      // 是否启用思考模式
}

// OllamaEmbeddingRequest 是 Ollama 嵌入 API 的请求格式。
// 对应 Ollama 的 /api/embed 端点。
type OllamaEmbeddingRequest struct {
	Model      string         `json:"model"`                // 模型名称
	Input      interface{}    `json:"input"`                // 输入文本（字符串或字符串数组）
	Options    map[string]any `json:"options,omitempty"`    // 模型参数
	Dimensions int            `json:"dimensions,omitempty"` // 嵌入向量维度（可选）
}

// OllamaEmbeddingResponse 是 Ollama 嵌入 API 的响应格式。
type OllamaEmbeddingResponse struct {
	Error           string      `json:"error,omitempty"`          // 错误信息
	Model           string      `json:"model"`                    // 使用的模型名称
	Embeddings      [][]float64 `json:"embeddings"`               // 嵌入向量列表
	PromptEvalCount int         `json:"prompt_eval_count,omitempty"` // 提示词 token 数量
}

// OllamaTagsResponse 是 Ollama 模型列表 API 的响应格式。
// 对应 Ollama 的 /api/tags 端点。
type OllamaTagsResponse struct {
	Models []OllamaModel `json:"models"` // 已安装的模型列表
}

// OllamaModel 表示一个已安装的 Ollama 模型。
type OllamaModel struct {
	Name       string            `json:"name"`                // 模型名称
	Size       int64             `json:"size"`                // 模型大小（字节）
	Digest     string            `json:"digest,omitempty"`    // 模型摘要
	ModifiedAt string            `json:"modified_at"`         // 最后修改时间
	Details    OllamaModelDetail `json:"details,omitempty"`   // 模型详细信息
}

// OllamaModelDetail 包含模型的详细信息。
type OllamaModelDetail struct {
	ParentModel       string   `json:"parent_model,omitempty"`       // 父模型名称
	Format            string   `json:"format,omitempty"`             // 模型格式（如 GGUF）
	Family            string   `json:"family,omitempty"`             // 模型家族（如 llama）
	Families          []string `json:"families,omitempty"`           // 模型家族列表
	ParameterSize     string   `json:"parameter_size,omitempty"`     // 参数规模（如 7B、13B）
	QuantizationLevel string   `json:"quantization_level,omitempty"` // 量化级别（如 Q4_0）
}

// OllamaPullRequest 是 Ollama 拉取模型 API 的请求格式。
// 对应 Ollama 的 /api/pull 端点。
type OllamaPullRequest struct {
	Name   string `json:"name"`             // 要拉取的模型名称
	Stream bool   `json:"stream,omitempty"` // 是否使用流式响应
}

// OllamaPullResponse 是 Ollama 拉取模型 API 的响应格式。
type OllamaPullResponse struct {
	Status    string `json:"status"`               // 拉取状态
	Digest    string `json:"digest,omitempty"`     // 模型摘要
	Total     int64  `json:"total,omitempty"`      // 总大小（字节）
	Completed int64  `json:"completed,omitempty"`  // 已完成大小（字节）
}

// OllamaDeleteRequest 是 Ollama 删除模型 API 的请求格式。
// 对应 Ollama 的 /api/delete 端点。
type OllamaDeleteRequest struct {
	Name string `json:"name"` // 要删除的模型名称
}
