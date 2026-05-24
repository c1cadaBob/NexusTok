// 包 interfaces - client_models.go
// 该文件定义了 CLI Proxy API 服务器的客户端数据模型。
// 包括 Google Cloud 项目结构、Gemini 内容模型和生成配置等。
package interfaces

import (
	"time"
)

// GCPProject 表示 Google Cloud 项目列表请求的响应结构。
// 在获取 Google Cloud 账户的可用项目时使用此结构。
type GCPProject struct {
	// Projects 是用户可访问的 Google Cloud 项目列表
	Projects []GCPProjectProjects `json:"projects"`
}

// GCPProjectLabels 定义与 GCP 项目关联的标签。
// 这些标签可以包含项目用途或配置的元数据。
type GCPProjectLabels struct {
	// GenerativeLanguage 指示项目是否启用了生成式语言 API
	GenerativeLanguage string `json:"generative-language"`
}

// GCPProjectProjects 包含单个 Google Cloud 项目的详细信息。
// 包括标识信息、元数据和配置详情。
type GCPProjectProjects struct {
	// ProjectNumber 是项目的唯一数字标识符
	ProjectNumber string `json:"projectNumber"`

	// ProjectID 是项目的唯一字符串标识符
	ProjectID string `json:"projectId"`

	// LifecycleState 指示项目的当前状态（如 "ACTIVE"）
	LifecycleState string `json:"lifecycleState"`

	// Name 是项目的可读名称
	Name string `json:"name"`

	// Labels 包含与项目关联的元数据标签
	Labels GCPProjectLabels `json:"labels"`

	// CreateTime 是项目创建的时间戳
	CreateTime time.Time `json:"createTime"`
}

// Content 表示对话中的单条消息，包含角色和部件。
// 此结构模拟用户和 AI 模型之间的消息交换。
type Content struct {
	// Role 指示谁发送了消息（"user"、"model" 或 "tool"）
	Role string `json:"role"`

	// Parts 是组成消息的内容部件集合
	Parts []Part `json:"parts"`
}

// Part 表示消息中的一个独立内容片段。
// 部件可以是文本、内联数据（如图像）、函数调用或函数响应。
type Part struct {
	// Thought 指示是否为思考内容
	Thought bool `json:"thought,omitempty"`

	// Text 包含纯文本内容
	Text string `json:"text,omitempty"`

	// InlineData 包含 base64 编码的数据及其 MIME 类型（如图像）
	InlineData *InlineData `json:"inlineData,omitempty"`

	// ThoughtSignature 是某些部件附带的提供商要求的签名
	ThoughtSignature string `json:"thoughtSignature,omitempty"`

	// FunctionCall 表示模型请求的工具调用
	FunctionCall *FunctionCall `json:"functionCall,omitempty"`

	// FunctionResponse 表示工具执行的结果
	FunctionResponse *FunctionResponse `json:"functionResponse,omitempty"`
}

// InlineData 表示 base64 编码的数据及其 MIME 类型。
// 通常用于在请求中嵌入图像或其他二进制数据。
type InlineData struct {
	// MimeType 指定嵌入数据的媒体类型（如 "image/png"）
	MimeType string `json:"mime_type,omitempty"`

	// Data 包含 base64 编码的二进制数据
	Data string `json:"data,omitempty"`
}

// FunctionCall 表示模型请求的工具调用。
// 包括函数名称和模型想要执行的参数。
type FunctionCall struct {
	// ID 是要调用的函数的标识符
	ID string `json:"id,omitempty"`

	// Name 是要调用的函数的标识符
	Name string `json:"name"`

	// Args 包含传递给函数的参数
	Args map[string]interface{} `json:"args"`
}

// FunctionResponse 表示工具执行的结果。
// 在工具调用处理后发送回模型。
type FunctionResponse struct {
	// ID 是函数的标识符
	ID string `json:"id,omitempty"`

	// Name 是被调用的函数的标识符
	Name string `json:"name"`

	// Response 包含函数执行的结果数据
	Response map[string]interface{} `json:"response"`
}

// GenerateContentRequest 是 streamGenerateContent 端点的顶级请求结构。
// 此结构定义了从 AI 模型生成内容所需的所有参数。
type GenerateContentRequest struct {
	// SystemInstruction 提供指导模型行为的系统级指令
	SystemInstruction *Content `json:"systemInstruction,omitempty"`

	// Contents 是用户和模型之间的对话历史
	Contents []Content `json:"contents"`

	// Tools 定义模型可以调用的可用工具/函数
	Tools []ToolDeclaration `json:"tools,omitempty"`

	// GenerationConfig 包含控制模型生成行为的参数
	GenerationConfig `json:"generationConfig"`
}

// GenerationConfig 定义控制模型生成行为的参数。
// 这些参数影响模型响应的创造性、随机性和推理能力。
type GenerationConfig struct {
	// ThinkingConfig 指定模型"思考"过程的配置
	ThinkingConfig GenerationConfigThinkingConfig `json:"thinkingConfig,omitempty"`

	// Temperature 控制模型响应的随机性。
	// 值越接近 0 使响应更确定性，越接近 1 增加随机性。
	Temperature float64 `json:"temperature,omitempty"`

	// TopP 控制核采样，影响响应的多样性。
	// 将模型限制为仅考虑概率质量的前 P%。
	TopP float64 `json:"topP,omitempty"`

	// TopK 将模型限制为仅考虑前 K 个最可能的标记。
	// 这可以帮助控制生成文本的质量和多样性。
	TopK float64 `json:"topK,omitempty"`
}

// GenerationConfigThinkingConfig 指定模型"思考"过程的配置。
// 控制模型是否应将推理过程与最终答案一起输出。
type GenerationConfigThinkingConfig struct {
	// IncludeThoughts 确定模型是否应输出其推理过程。
	// 启用时，模型将在响应中包含其逐步思考。
	IncludeThoughts bool `json:"include_thoughts,omitempty"`
}

// ToolDeclaration 定义声明工具（如函数）的结构，
// 模型在内容生成期间可以调用这些工具。
type ToolDeclaration struct {
	// FunctionDeclarations 是模型可以调用的可用函数列表
	FunctionDeclarations []interface{} `json:"functionDeclarations"`
}
