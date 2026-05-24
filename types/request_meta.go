// Package types - request_meta.go
// 该文件定义了请求元数据相关类型
//
// 用途：
// - 在请求处理链中传递元数据信息
// - 支持请求追踪和日志记录
package types

// FileType 文件类型常量
// 用于在多模态请求中标识不同类型的文件
type FileType string

const (
	FileTypeImage FileType = "image" // 图片文件类型
	FileTypeAudio FileType = "audio" // 音频文件类型
	FileTypeVideo FileType = "video" // 视频文件类型
	FileTypeFile  FileType = "file"  // 通用文件类型
)

// TokenType Token 计数类型常量
// 用于标识不同的 Token 计数方式
type TokenType string

const (
	TokenTypeTextNumber TokenType = "text_number" // 文本/数字 Token（按字符数估算）
	TokenTypeTokenizer  TokenType = "tokenizer"   // 分词器 Token（使用专用分词器计算）
	TokenTypeImage      TokenType = "image"       // 图片 Token（按图片尺寸计算）
)

// TokenCountMeta Token 计数元数据
// 用于在请求处理过程中传递 Token 计数的详细信息
type TokenCountMeta struct {
	TokenType     TokenType   `json:"token_type,omitempty"`     // Token 计数类型
	CombineText   string      `json:"combine_text,omitempty"`   // 所有消息合并后的文本
	ToolsCount    int         `json:"tools_count,omitempty"`    // 工具/函数定义数量
	NameCount     int         `json:"name_count,omitempty"`     // 消息中的名称数量
	MessagesCount int         `json:"messages_count,omitempty"` // 消息数量
	Files         []*FileMeta `json:"files,omitempty"`          // 文件列表，每个文件包含类型和内容
	MaxTokens     int         `json:"max_tokens,omitempty"`     // 请求允许的最大 Token 数

	ImagePriceRatio float64 `json:"image_ratio,omitempty"` // 图片价格倍率
	//IsStreaming   bool        `json:"is_streaming,omitempty"`   // 是否为流式请求
}

// FileMeta 文件元数据
// 描述请求中的单个文件信息
type FileMeta struct {
	FileType           // 文件类型（image、audio、video、file）
	Source FileSource  // 统一的文件来源（URL 或 base64）
	Detail string      // 图片细节级别（low/high/auto）
}

// NewFileMeta 创建新的 FileMeta
func NewFileMeta(fileType FileType, source FileSource) *FileMeta {
	return &FileMeta{
		FileType: fileType,
		Source:   source,
	}
}

// NewImageFileMeta 创建图片类型的 FileMeta
func NewImageFileMeta(source FileSource, detail string) *FileMeta {
	return &FileMeta{
		FileType: FileTypeImage,
		Source:   source,
		Detail:   detail,
	}
}

// GetIdentifier 获取文件标识符（用于日志）
func (f *FileMeta) GetIdentifier() string {
	if f.Source != nil {
		return f.Source.GetIdentifier()
	}
	return "unknown"
}

// IsURL 判断是否是 URL 来源
func (f *FileMeta) IsURL() bool {
	return f.Source != nil && f.Source.IsURL()
}

// GetRawData 获取原始数据（兼容旧代码）
// Deprecated: 请使用 Source.GetRawData()
func (f *FileMeta) GetRawData() string {
	if f.Source != nil {
		return f.Source.GetRawData()
	}
	return ""
}

// RequestMeta 请求元数据
// 在请求处理链中传递请求的元数据信息
// 用于日志记录、计费统计和请求追踪
type RequestMeta struct {
	OriginalModelName string `json:"original_model_name"` // 原始请求的模型名称（映射前）
	UserUsingGroup    string `json:"user_using_group"`    // 用户当前使用的用户组
	PromptTokens      int    `json:"prompt_tokens"`       // 提示词 Token 数量
	PreConsumedQuota  int    `json:"pre_consumed_quota"`  // 预消耗的额度
}
