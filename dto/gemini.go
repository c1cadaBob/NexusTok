// Package dto - gemini.go
// 该文件定义了 Google Gemini API 的数据传输对象
//
// 主要结构体：
// - GeminiChatRequest/Response：Gemini 聊天对话请求和响应
// - GeminiChatContent/Part：消息内容和部件（文本、内联数据、函数调用等）
// - GeminiChatGenerationConfig：生成配置（温度、Top-P、输出 token 上限等）
// - GeminiThinkingConfig：思考模式配置（思维预算、思维级别等）
// - GeminiImageRequest/Response：Imagen 图像生成请求和响应
// - GeminiEmbeddingRequest/Response：文本嵌入请求和响应
//
// 兼容性说明：
// - 所有结构体同时支持 snake_case 和 camelCase JSON 字段名
// - 通过自定义 UnmarshalJSON 方法实现自动兼容
// - 原生 Gemini API 使用 camelCase，但部分客户端使用 snake_case
package dto

import (
	"encoding/json"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/logger"
	"github.com/c1cada/NexusTok/types"

	"github.com/gin-gonic/gin"
)

// GeminiChatRequest Gemini 聊天请求结构体
// 支持单条请求和批量请求（通过 Requests 字段）
// Contents：对话内容列表（多轮对话消息）
// SafetySettings：安全过滤设置
// GenerationConfig：生成参数配置
// Tools：可用工具列表（函数声明、Google 搜索、代码执行等）
// ToolConfig：工具调用配置（函数调用模式等）
// SystemInstructions：系统指令（角色设定、行为约束等）
// CachedContent：缓存内容引用（用于减少重复上下文传输）
type GeminiChatRequest struct {
	Requests           []GeminiChatRequest        `json:"requests,omitempty"` // For batch requests
	Contents           []GeminiChatContent        `json:"contents"`
	SafetySettings     []GeminiChatSafetySettings `json:"safetySettings,omitempty"`
	GenerationConfig   GeminiChatGenerationConfig `json:"generationConfig,omitempty"`
	Tools              json.RawMessage            `json:"tools,omitempty"`
	ToolConfig         *ToolConfig                `json:"toolConfig,omitempty"`
	SystemInstructions *GeminiChatContent         `json:"systemInstruction,omitempty"`
	CachedContent      string                     `json:"cachedContent,omitempty"`
}

// UnmarshalJSON allows GeminiChatRequest to accept both snake_case and camelCase fields.
func (r *GeminiChatRequest) UnmarshalJSON(data []byte) error {
	type Alias GeminiChatRequest
	var aux struct {
		Alias
		SystemInstructionSnake *GeminiChatContent `json:"system_instruction,omitempty"`
	}

	if err := common.Unmarshal(data, &aux); err != nil {
		return err
	}

	*r = GeminiChatRequest(aux.Alias)

	if aux.SystemInstructionSnake != nil {
		r.SystemInstructions = aux.SystemInstructionSnake
	}

	return nil
}

// ToolConfig 工具调用配置
// FunctionCallingConfig：函数调用配置（模式和允许的函数名列表）
// RetrievalConfig：检索配置（地理位置和语言偏好）
// IncludeServerSideToolInvocations：是否包含服务端工具调用结果
type ToolConfig struct {
	FunctionCallingConfig *FunctionCallingConfig `json:"functionCallingConfig,omitempty"`
	RetrievalConfig       *RetrievalConfig       `json:"retrievalConfig,omitempty"`
	IncludeServerSideToolInvocations *bool       `json:"includeServerSideToolInvocations,omitempty"`
}

// FunctionCallingConfig 函数调用配置
// Mode：调用模式（AUTO/ANY/NONE）
// AllowedFunctionNames：允许调用的函数名白名单
type FunctionCallingConfig struct {
	Mode                 FunctionCallingConfigMode `json:"mode,omitempty"`
	AllowedFunctionNames []string                  `json:"allowedFunctionNames,omitempty"`
}
// FunctionCallingConfigMode 函数调用模式类型
// 支持 AUTO（自动决定）、ANY（强制调用）、NONE（禁止调用）
type FunctionCallingConfigMode string

// RetrievalConfig 检索配置
// LatLng：地理位置坐标（用于本地化检索）
// LanguageCode：语言代码偏好
type RetrievalConfig struct {
	LatLng       *LatLng `json:"latLng,omitempty"`
	LanguageCode string  `json:"languageCode,omitempty"`
}

// LatLng 地理位置坐标
// Latitude：纬度
// Longitude：经度
type LatLng struct {
	Latitude  *float64 `json:"latitude,omitempty"`
	Longitude *float64 `json:"longitude,omitempty"`
}

// GetTokenCountMeta 获取 Gemini 聊天请求的 Token 计数元数据
// 遍历所有内容中的文本部分和内联数据（图片/音频/视频/文件）
// 返回拼接的输入文本、文件元数据列表和最大输出 token 数
func (r *GeminiChatRequest) GetTokenCountMeta() *types.TokenCountMeta {
	var files []*types.FileMeta = make([]*types.FileMeta, 0)

	var maxTokens int

	if r.GenerationConfig.MaxOutputTokens != nil && *r.GenerationConfig.MaxOutputTokens > 0 {
		maxTokens = int(*r.GenerationConfig.MaxOutputTokens)
	}

	var inputTexts []string
	for _, content := range r.Contents {
		for _, part := range content.Parts {
			if part.Text != "" {
				inputTexts = append(inputTexts, part.Text)
			}
			if source := part.InlineData.ToFileSource(); source != nil {
				mimeType := part.InlineData.MimeType
				var fileType types.FileType
				if strings.HasPrefix(mimeType, "image/") {
					fileType = types.FileTypeImage
				} else if strings.HasPrefix(mimeType, "audio/") {
					fileType = types.FileTypeAudio
				} else if strings.HasPrefix(mimeType, "video/") {
					fileType = types.FileTypeVideo
				} else {
					fileType = types.FileTypeFile
				}
				files = append(files, &types.FileMeta{
					FileType: fileType,
					Source:   source,
				})
			}
		}
	}

	inputText := strings.Join(inputTexts, "\n")
	return &types.TokenCountMeta{
		CombineText: inputText,
		Files:       files,
		MaxTokens:   maxTokens,
	}
}

// IsStream 判断 Gemini 请求是否为流式模式
// 通过两种方式检测：
// 1. URL 查询参数 alt=sse
// 2. URL 路径包含 streamGenerateContent
func (r *GeminiChatRequest) IsStream(c *gin.Context) bool {
	if c.Query("alt") == "sse" {
		return true
	}
	// Native Gemini API uses URL action to indicate streaming:
	// /v1beta/models/{model}:streamGenerateContent
	if strings.Contains(c.Request.URL.Path, "streamGenerateContent") {
		return true
	}
	return false
}

// SetModelName Gemini 请求没有 model 字段（模型在 URL 路径中），此方法为空实现
func (r *GeminiChatRequest) SetModelName(modelName string) {
	// GeminiChatRequest does not have a model field, so this method does nothing.
}

// GetTools 解析并返回工具列表
// 支持 JSON 数组和 JSON 对象两种格式：
// - 数组格式：直接解析为工具列表
// - 对象格式：解析为单个工具并包装为列表
func (r *GeminiChatRequest) GetTools() []GeminiChatTool {
	var tools []GeminiChatTool
	if strings.HasPrefix(string(r.Tools), "[") {
		// is array
		if err := common.Unmarshal(r.Tools, &tools); err != nil {
			logger.LogError(nil, "error_unmarshalling_tools: "+err.Error())
			return nil
		}
	} else if strings.HasPrefix(string(r.Tools), "{") {
		// is object
		singleTool := GeminiChatTool{}
		if err := common.Unmarshal(r.Tools, &singleTool); err != nil {
			logger.LogError(nil, "error_unmarshalling_single_tool: "+err.Error())
			return nil
		}
		tools = []GeminiChatTool{singleTool}
	}
	return tools
}

// SetTools 设置工具列表并序列化为 JSON
// 空列表时设置为空 JSON 数组 "[]"
func (r *GeminiChatRequest) SetTools(tools []GeminiChatTool) {
	if len(tools) == 0 {
		r.Tools = json.RawMessage("[]")
		return
	}

	// Marshal the tools to JSON
	data, err := common.Marshal(tools)
	if err != nil {
		logger.LogError(nil, "error_marshalling_tools: "+err.Error())
		return
	}
	r.Tools = data
}

// GeminiThinkingConfig Gemini 思考模式配置
// 控制模型在生成回复前是否进行内部推理
// IncludeThoughts：是否在响应中包含思考过程
// ThinkingBudget：思考预算（分配给推理的 token 数量上限）
// ThinkingLevel：思考级别（与 ThinkingBudget 冲突时的备选配置）
type GeminiThinkingConfig struct {
	IncludeThoughts bool `json:"includeThoughts,omitempty"`
	ThinkingBudget  *int `json:"thinkingBudget,omitempty"`
	// TODO Conflict with thinkingbudget.
	ThinkingLevel string `json:"thinkingLevel,omitempty"`
}

// UnmarshalJSON allows GeminiThinkingConfig to accept both snake_case and camelCase fields.
func (c *GeminiThinkingConfig) UnmarshalJSON(data []byte) error {
	type Alias GeminiThinkingConfig
	var aux struct {
		Alias
		IncludeThoughtsSnake *bool  `json:"include_thoughts,omitempty"`
		ThinkingBudgetSnake  *int   `json:"thinking_budget,omitempty"`
		ThinkingLevelSnake   string `json:"thinking_level,omitempty"`
	}

	if err := common.Unmarshal(data, &aux); err != nil {
		return err
	}

	*c = GeminiThinkingConfig(aux.Alias)

	if aux.IncludeThoughtsSnake != nil {
		c.IncludeThoughts = *aux.IncludeThoughtsSnake
	}

	if aux.ThinkingBudgetSnake != nil {
		c.ThinkingBudget = aux.ThinkingBudgetSnake
	}

	if aux.ThinkingLevelSnake != "" {
		c.ThinkingLevel = aux.ThinkingLevelSnake
	}

	return nil
}

// SetThinkingBudget 设置思考预算
// budget：分配给模型内部推理的 token 数量上限
func (c *GeminiThinkingConfig) SetThinkingBudget(budget int) {
	c.ThinkingBudget = &budget
}

// GeminiInlineData Gemini 内联数据结构体
// 用于传输 Base64 编码的二进制数据（图片、音频、视频等）
// MimeType：MIME 类型（如 image/png、audio/mp3 等）
// Data：Base64 编码的数据内容
type GeminiInlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

// ToFileSource 将内联数据转换为通用文件源
// 返回 nil 表示数据为空，非空时创建基于 Base64 数据的文件源
func (d *GeminiInlineData) ToFileSource() types.FileSource {
	if d == nil || d.Data == "" {
		return nil
	}
	return types.NewFileSourceFromData(d.Data, d.MimeType)
}

// UnmarshalJSON custom unmarshaler for GeminiInlineData to support snake_case and camelCase for MimeType
func (g *GeminiInlineData) UnmarshalJSON(data []byte) error {
	type Alias GeminiInlineData // Use type alias to avoid recursion
	var aux struct {
		Alias
		MimeTypeSnake string `json:"mime_type"`
	}

	if err := common.Unmarshal(data, &aux); err != nil {
		return err
	}

	*g = GeminiInlineData(aux.Alias) // Copy other fields if any in future

	// Prioritize snake_case if present
	if aux.MimeTypeSnake != "" {
		g.MimeType = aux.MimeTypeSnake
	} else if aux.MimeType != "" { // Fallback to camelCase from Alias
		g.MimeType = aux.MimeType
	}
	// g.Data would be populated by aux.Alias.Data
	return nil
}

// FunctionCall 函数调用结构体
// FunctionName：被调用的函数名称
// Arguments：函数参数（JSON 对象或已解析的 map）
type FunctionCall struct {
	FunctionName string `json:"name"`
	Arguments    any    `json:"args"`
}

// GeminiFunctionResponse 函数执行结果响应
// Name：函数名称
// Response：函数执行结果（键值对形式）
// WillContinue：是否继续执行（用于多步函数调用）
// Scheduling：调度信息
// Parts：附加内容部件
// ID：响应标识
type GeminiFunctionResponse struct {
	Name         string                 `json:"name"`
	Response     map[string]interface{} `json:"response"`
	WillContinue json.RawMessage        `json:"willContinue,omitempty"`
	Scheduling   json.RawMessage        `json:"scheduling,omitempty"`
	Parts        json.RawMessage        `json:"parts,omitempty"`
	ID           json.RawMessage        `json:"id,omitempty"`
}

// GeminiPartExecutableCode 可执行代码结构体
// Language：编程语言（如 PYTHON、JAVASCRIPT 等）
// Code：代码内容
type GeminiPartExecutableCode struct {
	Language string `json:"language,omitempty"`
	Code     string `json:"code,omitempty"`
}

// GeminiPartCodeExecutionResult 代码执行结果
// Outcome：执行结果状态
// Output：执行输出内容
type GeminiPartCodeExecutionResult struct {
	Outcome string `json:"outcome,omitempty"`
	Output  string `json:"output,omitempty"`
}

// GeminiFileData 文件数据引用
// MimeType：MIME 类型
// FileUri：文件 URI（通常是 Google Cloud Storage 地址）
type GeminiFileData struct {
	MimeType string `json:"mimeType,omitempty"`
	FileUri  string `json:"fileUri,omitempty"`
}

// GeminiPart Gemini 消息内容部件
// 每个部件是消息中的一个独立内容单元
// Text：文本内容
// Thought：是否为模型的思考过程（推理内容）
// InlineData：内联二进制数据（图片、音频等）
// FunctionCall：函数调用请求
// ThoughtSignature：思考过程签名
// FunctionResponse：函数执行结果
// MediaResolution：媒体分辨率配置
// VideoMetadata：视频元数据
// FileData：外部文件引用
// ExecutableCode：可执行代码（代码执行功能）
// CodeExecutionResult：代码执行结果
type GeminiPart struct {
	Text             string                  `json:"text,omitempty"`
	Thought          bool                    `json:"thought,omitempty"`
	InlineData       *GeminiInlineData       `json:"inlineData,omitempty"`
	FunctionCall     *FunctionCall           `json:"functionCall,omitempty"`
	ThoughtSignature json.RawMessage         `json:"thoughtSignature,omitempty"`
	FunctionResponse *GeminiFunctionResponse `json:"functionResponse,omitempty"`
	// Optional. Media resolution for the input media.
	MediaResolution     json.RawMessage                `json:"mediaResolution,omitempty"`
	VideoMetadata       json.RawMessage                `json:"videoMetadata,omitempty"`
	FileData            *GeminiFileData                `json:"fileData,omitempty"`
	ExecutableCode      *GeminiPartExecutableCode      `json:"executableCode,omitempty"`
	CodeExecutionResult *GeminiPartCodeExecutionResult `json:"codeExecutionResult,omitempty"`
}

// UnmarshalJSON custom unmarshaler for GeminiPart to support snake_case and camelCase for InlineData
func (p *GeminiPart) UnmarshalJSON(data []byte) error {
	// Alias to avoid recursion during unmarshalling
	type Alias GeminiPart
	var aux struct {
		Alias
		InlineDataSnake *GeminiInlineData `json:"inline_data,omitempty"` // snake_case variant
	}

	if err := common.Unmarshal(data, &aux); err != nil {
		return err
	}

	// Assign fields from alias
	*p = GeminiPart(aux.Alias)

	// Prioritize snake_case for InlineData if present
	if aux.InlineDataSnake != nil {
		p.InlineData = aux.InlineDataSnake
	} else if aux.InlineData != nil { // Fallback to camelCase from Alias
		p.InlineData = aux.InlineData
	}
	// Other fields like Text, FunctionCall etc. are already populated via aux.Alias

	return nil
}

// GeminiChatContent Gemini 对话内容
// Role：消息角色（user/model/system）
// Parts：内容部件列表（文本、图片、函数调用等）
type GeminiChatContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []GeminiPart `json:"parts"`
}

// GeminiChatSafetySettings Gemini 安全过滤设置
// Category：安全类别（如 HARM_CATEGORY_HARASSMENT、HARM_CATEGORY_HATE_SPEECH 等）
// Threshold：过滤阈值（BLOCK_NONE/LOW/MEDIUM/HIGH 等）
type GeminiChatSafetySettings struct {
	Category  string `json:"category"`
	Threshold string `json:"threshold"`
}

// GeminiChatTool Gemini 工具定义
// GoogleSearch：Google 搜索工具
// GoogleSearchRetrieval：Google 搜索检索工具
// CodeExecution：代码执行工具
// FunctionDeclarations：函数声明列表
// URLContext：URL 上下文工具
type GeminiChatTool struct {
	GoogleSearch          any `json:"googleSearch,omitempty"`
	GoogleSearchRetrieval any `json:"googleSearchRetrieval,omitempty"`
	CodeExecution         any `json:"codeExecution,omitempty"`
	FunctionDeclarations  any `json:"functionDeclarations,omitempty"`
	URLContext            any `json:"urlContext,omitempty"`
}

// GeminiChatGenerationConfig Gemini 生成配置
// Temperature/TopP/TopK：采样参数
// MaxOutputTokens：最大输出 token 数
// CandidateCount：生成候选回复数量
// StopSequences：停止序列
// ResponseMimeType/ResponseSchema：结构化输出配置
// PresencePenalty/FrequencyPenalty：惩罚参数
// ResponseLogprobs/Logprobs：对数概率配置
// EnableEnhancedCivicAnswers：增强公民问答
// MediaResolution：媒体分辨率
// Seed：随机种子
// ResponseModalities：响应模态（TEXT/IMAGE/AUDIO 等）
// ThinkingConfig：思考模式配置
// SpeechConfig/ImageConfig：语音/图像生成配置（透传）
type GeminiChatGenerationConfig struct {
	Temperature                *float64              `json:"temperature,omitempty"`
	TopP                       *float64              `json:"topP,omitempty"`
	TopK                       *float64              `json:"topK,omitempty"`
	MaxOutputTokens            *uint                 `json:"maxOutputTokens,omitempty"`
	CandidateCount             *int                  `json:"candidateCount,omitempty"`
	StopSequences              []string              `json:"stopSequences,omitempty"`
	ResponseMimeType           string                `json:"responseMimeType,omitempty"`
	ResponseSchema             any                   `json:"responseSchema,omitempty"`
	ResponseJsonSchema         json.RawMessage       `json:"responseJsonSchema,omitempty"`
	PresencePenalty            *float32              `json:"presencePenalty,omitempty"`
	FrequencyPenalty           *float32              `json:"frequencyPenalty,omitempty"`
	ResponseLogprobs           *bool                 `json:"responseLogprobs,omitempty"`
	Logprobs                   *int32                `json:"logprobs,omitempty"`
	EnableEnhancedCivicAnswers *bool                 `json:"enableEnhancedCivicAnswers,omitempty"`
	MediaResolution            MediaResolution       `json:"mediaResolution,omitempty"`
	Seed                       *int64                `json:"seed,omitempty"`
	ResponseModalities         []string              `json:"responseModalities,omitempty"`
	ThinkingConfig             *GeminiThinkingConfig `json:"thinkingConfig,omitempty"`
	SpeechConfig               json.RawMessage       `json:"speechConfig,omitempty"` // RawMessage to allow flexible speech config
	ImageConfig                json.RawMessage       `json:"imageConfig,omitempty"`  // RawMessage to allow flexible image config
}

// UnmarshalJSON allows GeminiChatGenerationConfig to accept both snake_case and camelCase fields.
func (c *GeminiChatGenerationConfig) UnmarshalJSON(data []byte) error {
	type Alias GeminiChatGenerationConfig
	var aux struct {
		Alias
		TopPSnake                       *float64              `json:"top_p,omitempty"`
		TopKSnake                       *float64              `json:"top_k,omitempty"`
		MaxOutputTokensSnake            *uint                 `json:"max_output_tokens,omitempty"`
		CandidateCountSnake             *int                  `json:"candidate_count,omitempty"`
		StopSequencesSnake              []string              `json:"stop_sequences,omitempty"`
		ResponseMimeTypeSnake           string                `json:"response_mime_type,omitempty"`
		ResponseSchemaSnake             any                   `json:"response_schema,omitempty"`
		ResponseJsonSchemaSnake         json.RawMessage       `json:"response_json_schema,omitempty"`
		PresencePenaltySnake            *float32              `json:"presence_penalty,omitempty"`
		FrequencyPenaltySnake           *float32              `json:"frequency_penalty,omitempty"`
		ResponseLogprobsSnake           *bool                 `json:"response_logprobs,omitempty"`
		EnableEnhancedCivicAnswersSnake *bool                 `json:"enable_enhanced_civic_answers,omitempty"`
		MediaResolutionSnake            MediaResolution       `json:"media_resolution,omitempty"`
		ResponseModalitiesSnake         []string              `json:"response_modalities,omitempty"`
		ThinkingConfigSnake             *GeminiThinkingConfig `json:"thinking_config,omitempty"`
		SpeechConfigSnake               json.RawMessage       `json:"speech_config,omitempty"`
		ImageConfigSnake                json.RawMessage       `json:"image_config,omitempty"`
	}

	if err := common.Unmarshal(data, &aux); err != nil {
		return err
	}

	*c = GeminiChatGenerationConfig(aux.Alias)

	// Prioritize snake_case if present
	if aux.TopPSnake != nil {
		c.TopP = aux.TopPSnake
	}
	if aux.TopKSnake != nil {
		c.TopK = aux.TopKSnake
	}
	if aux.MaxOutputTokensSnake != nil {
		c.MaxOutputTokens = aux.MaxOutputTokensSnake
	}
	if aux.CandidateCountSnake != nil {
		c.CandidateCount = aux.CandidateCountSnake
	}
	if len(aux.StopSequencesSnake) > 0 {
		c.StopSequences = aux.StopSequencesSnake
	}
	if aux.ResponseMimeTypeSnake != "" {
		c.ResponseMimeType = aux.ResponseMimeTypeSnake
	}
	if aux.ResponseSchemaSnake != nil {
		c.ResponseSchema = aux.ResponseSchemaSnake
	}
	if len(aux.ResponseJsonSchemaSnake) > 0 {
		c.ResponseJsonSchema = aux.ResponseJsonSchemaSnake
	}
	if aux.PresencePenaltySnake != nil {
		c.PresencePenalty = aux.PresencePenaltySnake
	}
	if aux.FrequencyPenaltySnake != nil {
		c.FrequencyPenalty = aux.FrequencyPenaltySnake
	}
	if aux.ResponseLogprobsSnake != nil {
		c.ResponseLogprobs = aux.ResponseLogprobsSnake
	}
	if aux.EnableEnhancedCivicAnswersSnake != nil {
		c.EnableEnhancedCivicAnswers = aux.EnableEnhancedCivicAnswersSnake
	}
	if aux.MediaResolutionSnake != "" {
		c.MediaResolution = aux.MediaResolutionSnake
	}
	if len(aux.ResponseModalitiesSnake) > 0 {
		c.ResponseModalities = aux.ResponseModalitiesSnake
	}
	if aux.ThinkingConfigSnake != nil {
		c.ThinkingConfig = aux.ThinkingConfigSnake
	}
	if len(aux.SpeechConfigSnake) > 0 {
		c.SpeechConfig = aux.SpeechConfigSnake
	}
	if len(aux.ImageConfigSnake) > 0 {
		c.ImageConfig = aux.ImageConfigSnake
	}

	return nil
}

// MediaResolution 媒体分辨率类型
type MediaResolution string

// GeminiChatCandidate Gemini 生成候选回复
// Content：回复内容
// FinishReason：结束原因（STOP/MAX_TOKENS/SAFETY 等）
// Index：候选索引
// SafetyRatings：安全评级列表
type GeminiChatCandidate struct {
	Content       GeminiChatContent        `json:"content"`
	FinishReason  *string                  `json:"finishReason"`
	Index         int64                    `json:"index"`
	SafetyRatings []GeminiChatSafetyRating `json:"safetyRatings"`
}

// GeminiChatSafetyRating Gemini 安全评级
// Category：安全类别
// Probability：危害概率等级（NEGLIGIBLE/LOW/MEDIUM/HIGH）
type GeminiChatSafetyRating struct {
	Category    string `json:"category"`
	Probability string `json:"probability"`
}

// GeminiChatPromptFeedback Gemini 提示词反馈
// SafetyRatings：提示词的安全评级列表
// BlockReason：阻止原因（如果因安全问题被阻止）
type GeminiChatPromptFeedback struct {
	SafetyRatings []GeminiChatSafetyRating `json:"safetyRatings"`
	BlockReason   *string                  `json:"blockReason,omitempty"`
}

// GeminiChatResponse Gemini 聊天响应
// Candidates：候选回复列表
// PromptFeedback：提示词反馈（安全检查结果）
// UsageMetadata：Token 使用量元数据
type GeminiChatResponse struct {
	Candidates     []GeminiChatCandidate     `json:"candidates"`
	PromptFeedback *GeminiChatPromptFeedback `json:"promptFeedback,omitempty"`
	UsageMetadata  GeminiUsageMetadata       `json:"usageMetadata"`
}

// GeminiUsageMetadata Gemini Token 使用量元数据
// PromptTokenCount：输入提示词 token 数
// ToolUsePromptTokenCount：工具调用提示词 token 数
// CandidatesTokenCount：候选回复 token 数
// TotalTokenCount：总 token 数
// ThoughtsTokenCount：思考过程 token 数
// CachedContentTokenCount：缓存内容 token 数
// PromptTokensDetails/ToolUsePromptTokensDetails/CandidatesTokensDetails：按模态分类的详细 token 数
type GeminiUsageMetadata struct {
	PromptTokenCount           int                         `json:"promptTokenCount"`
	ToolUsePromptTokenCount    int                         `json:"toolUsePromptTokenCount"`
	CandidatesTokenCount       int                         `json:"candidatesTokenCount"`
	TotalTokenCount            int                         `json:"totalTokenCount"`
	ThoughtsTokenCount         int                         `json:"thoughtsTokenCount"`
	CachedContentTokenCount    int                         `json:"cachedContentTokenCount"`
	PromptTokensDetails        []GeminiPromptTokensDetails `json:"promptTokensDetails"`
	ToolUsePromptTokensDetails []GeminiPromptTokensDetails `json:"toolUsePromptTokensDetails"`
	CandidatesTokensDetails    []GeminiPromptTokensDetails `json:"candidatesTokensDetails"`
}

// GeminiPromptTokensDetails Gemini 按模态分类的 Token 详情
// Modality：模态类型（如 TEXT、IMAGE、AUDIO 等）
// TokenCount：该模态的 token 数量
type GeminiPromptTokensDetails struct {
	Modality   string `json:"modality"`
	TokenCount int    `json:"tokenCount"`
}

// Imagen 相关结构体

// GeminiImageRequest Imagen 图像生成请求
// Instances：输入实例列表（包含提示词）
// Parameters：生成参数（数量、比例、人物生成等）
type GeminiImageRequest struct {
	Instances  []GeminiImageInstance `json:"instances"`
	Parameters GeminiImageParameters `json:"parameters"`
}

// GeminiImageInstance Imagen 图像生成输入实例
// Prompt：图像生成提示词
type GeminiImageInstance struct {
	Prompt string `json:"prompt"`
}

// GeminiImageParameters Imagen 图像生成参数
// SampleCount：生成图像数量
// AspectRatio：宽高比（如 "1:1"、"16:9" 等）
// PersonGeneration：人物生成策略（ALLOW/DONT_ALLOW 等）
// ImageSize：图像尺寸
type GeminiImageParameters struct {
	SampleCount      int    `json:"sampleCount,omitempty"`
	AspectRatio      string `json:"aspectRatio,omitempty"`
	PersonGeneration string `json:"personGeneration,omitempty"`
	ImageSize        string `json:"imageSize,omitempty"`
}

// GeminiImageResponse Imagen 图像生成响应
// Predictions：生成的图像预测结果列表
type GeminiImageResponse struct {
	Predictions []GeminiImagePrediction `json:"predictions"`
}

// GeminiImagePrediction Imagen 图像预测结果
// MimeType：图像 MIME 类型（如 image/png）
// BytesBase64Encoded：Base64 编码的图像数据
// RaiFilteredReason：RAI（Responsible AI）过滤原因（如果被过滤）
// SafetyAttributes：安全属性
type GeminiImagePrediction struct {
	MimeType           string `json:"mimeType"`
	BytesBase64Encoded string `json:"bytesBase64Encoded"`
	RaiFilteredReason  string `json:"raiFilteredReason,omitempty"`
	SafetyAttributes   any    `json:"safetyAttributes,omitempty"`
}

// 嵌入相关结构体

// GeminiEmbeddingRequest Gemini 文本嵌入请求
// Model：嵌入模型名称
// Content：输入内容
// TaskType：任务类型（RETRIEVAL_QUERY/RETRIEVAL_DOCUMENT 等）
// Title：文档标题（可选，用于提高检索质量）
// OutputDimensionality：输出向量维度（可选，用于降维）
type GeminiEmbeddingRequest struct {
	Model                string            `json:"model,omitempty"`
	Content              GeminiChatContent `json:"content"`
	TaskType             string            `json:"taskType,omitempty"`
	Title                string            `json:"title,omitempty"`
	OutputDimensionality int               `json:"outputDimensionality,omitempty"`
}

// IsStream Gemini 嵌入请求不支持流式输出，始终返回 false
func (r *GeminiEmbeddingRequest) IsStream(c *gin.Context) bool {
	// Gemini embedding requests are not streamed
	return false
}

// GetTokenCountMeta 获取 Gemini 嵌入请求的 Token 计数元数据
// 提取内容中的所有文本部分并拼接返回
func (r *GeminiEmbeddingRequest) GetTokenCountMeta() *types.TokenCountMeta {
	var inputTexts []string
	for _, part := range r.Content.Parts {
		if part.Text != "" {
			inputTexts = append(inputTexts, part.Text)
		}
	}
	inputText := strings.Join(inputTexts, "\n")
	return &types.TokenCountMeta{
		CombineText: inputText,
	}
}

// SetModelName 设置 Gemini 嵌入请求的模型名称
// 仅在 modelName 非空时更新
func (r *GeminiEmbeddingRequest) SetModelName(modelName string) {
	if modelName != "" {
		r.Model = modelName
	}
}

// GeminiBatchEmbeddingRequest Gemini 批量嵌入请求
// Requests：嵌入请求列表
type GeminiBatchEmbeddingRequest struct {
	Requests []*GeminiEmbeddingRequest `json:"requests"`
}

// IsStream Gemini 批量嵌入请求不支持流式输出，始终返回 false
func (r *GeminiBatchEmbeddingRequest) IsStream(c *gin.Context) bool {
	// Gemini batch embedding requests are not streamed
	return false
}

// GetTokenCountMeta 获取 Gemini 批量嵌入请求的 Token 计数元数据
// 遍历所有子请求，提取文本并拼接返回
func (r *GeminiBatchEmbeddingRequest) GetTokenCountMeta() *types.TokenCountMeta {
	var inputTexts []string
	for _, request := range r.Requests {
		meta := request.GetTokenCountMeta()
		if meta != nil && meta.CombineText != "" {
			inputTexts = append(inputTexts, meta.CombineText)
		}
	}
	inputText := strings.Join(inputTexts, "\n")
	return &types.TokenCountMeta{
		CombineText: inputText,
	}
}

// SetModelName 设置 Gemini 批量嵌入请求的模型名称
// 遍历所有子请求并设置模型名称
func (r *GeminiBatchEmbeddingRequest) SetModelName(modelName string) {
	if modelName != "" {
		for _, req := range r.Requests {
			req.SetModelName(modelName)
		}
	}
}

// GeminiEmbeddingResponse Gemini 单条嵌入响应
// Embedding：嵌入向量结果
type GeminiEmbeddingResponse struct {
	Embedding ContentEmbedding `json:"embedding"`
}

// GeminiBatchEmbeddingResponse Gemini 批量嵌入响应
// Embeddings：嵌入向量结果列表
type GeminiBatchEmbeddingResponse struct {
	Embeddings []*ContentEmbedding `json:"embeddings"`
}

// ContentEmbedding 内容嵌入向量
// Values：浮点数向量值
type ContentEmbedding struct {
	Values []float64 `json:"values"`
}
