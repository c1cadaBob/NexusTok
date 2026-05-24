// coze - dto.go
// 本文件定义了 Coze 渠道的数据传输对象（DTO）。
// 包含与 Coze 聊天 API 交互所需的请求和响应结构体。
// Coze API 支持流式和非流式两种响应模式，非流式模式需要通过轮询机制获取最终结果。
// 所有结构体均使用 JSON 标签进行序列化/反序列化映射。
package coze

import "encoding/json"

// CozeError 表示 Coze API 返回的错误信息结构体。
// 当 API 调用出错时，Coze 会返回此结构体描述错误详情。
type CozeError struct {
	Code    int    `json:"code"`    // 错误码，非零值表示出错
	Message string `json:"message"` // 错误描述信息
}

// CozeEnterMessage 表示发送到 Coze API 的单条消息。
// 用于构建聊天请求的附加消息列表，支持多种内容类型。
type CozeEnterMessage struct {
	Role        string          `json:"role"`                   // 消息角色（如 user、assistant）
	Type        string          `json:"type,omitempty"`         // 消息类型
	Content     any             `json:"content,omitempty"`      // 消息内容，可以是字符串或结构化数据
	MetaData    json.RawMessage `json:"meta_data,omitempty"`    // 元数据，JSON 格式的附加信息
	ContentType string          `json:"content_type,omitempty"` // 内容类型（如 "text"）
}

// CozeChatRequest 表示发送到 Coze 聊天 API 的请求结构体。
// 对应 Coze API 的 /v3/chat 端点，用于创建新的聊天会话。
type CozeChatRequest struct {
	BotId              string             `json:"bot_id"`                          // Bot 唯一标识，指定要与之对话的 Bot
	UserId             json.RawMessage    `json:"user_id"`                         // 用户标识，JSON 格式
	AdditionalMessages []CozeEnterMessage `json:"additional_messages,omitempty"`   // 附加消息列表，包含用户的输入消息
	Stream             bool               `json:"stream,omitempty"`               // 是否启用流式响应
	CustomVariables    json.RawMessage    `json:"custom_variables,omitempty"`     // 自定义变量，JSON 格式
	AutoSaveHistory    bool               `json:"auto_save_history,omitempty"`    // 是否自动保存聊天历史
	MetaData           json.RawMessage    `json:"meta_data,omitempty"`            // 元数据，JSON 格式的附加信息
	ExtraParams        json.RawMessage    `json:"extra_params,omitempty"`         // 额外参数，JSON 格式
	ShortcutCommand    json.RawMessage    `json:"shortcut_command,omitempty"`     // 快捷命令，JSON 格式
	Parameters         json.RawMessage    `json:"parameters,omitempty"`           // 请求参数，JSON 格式
}

// CozeChatResponse 表示 Coze 聊天 API 的响应结构体。
// 用于解析创建聊天会话的响应，以及轮询聊天状态的响应。
type CozeChatResponse struct {
	Code int                  `json:"code"` // 响应状态码，0 表示成功
	Msg  string               `json:"msg"`  // 响应消息
	Data CozeChatResponseData `json:"data"` // 响应数据
}

// CozeChatResponseData 表示 Coze 聊天响应的数据部分。
// 包含聊天会话的基本信息、状态和 token 用量统计。
type CozeChatResponseData struct {
	Id             string        `json:"id"`              // 聊天会话 ID
	ConversationId string        `json:"conversation_id"` // 对话 ID，用于关联同一对话的多次交互
	BotId          string        `json:"bot_id"`          // Bot ID
	CreatedAt      int64         `json:"created_at"`      // 创建时间戳（秒）
	LastError      CozeError     `json:"last_error"`      // 最后的错误信息（如果有）
	Status         string        `json:"status"`          // 会话状态（如 "created"、"completed"、"failed" 等）
	Usage          CozeChatUsage `json:"usage"`           // token 用量信息
}

// CozeChatUsage 表示 Coze 聊天 API 的 token 用量统计。
// 用于统计单次聊天交互的 token 消耗情况。
type CozeChatUsage struct {
	TokenCount  int `json:"token_count"`  // 总 token 数量（输入 + 输出）
	OutputCount int `json:"output_count"` // 输出 token 数量
	InputCount  int `json:"input_count"`  // 输入 token 数量
}

// CozeChatDetailResponse 表示 Coze 聊天详情 API 的响应结构体。
// 对应 /v3/chat/message/list 端点，用于获取聊天过程中产生的消息详情列表。
// 包含聊天过程中 Bot 生成的所有消息（如回答、思考过程等）。
type CozeChatDetailResponse struct {
	Data   []CozeChatV3MessageDetail `json:"data"`   // 消息详情列表
	Code   int                       `json:"code"`   // 响应状态码，0 表示成功
	Msg    string                    `json:"msg"`    // 响应消息
	Detail CozeResponseDetail        `json:"detail"` // 响应详情（含日志 ID）
}

// CozeChatV3MessageDetail 表示 Coze 聊天中单条消息的详细信息。
// 包含消息的内容、角色、类型、时间戳等完整信息。
// 支持推理内容（ReasoningContent）字段，用于展示模型的思考链过程。
type CozeChatV3MessageDetail struct {
	Id               string          `json:"id"`                 // 消息 ID
	Role             string          `json:"role"`               // 消息角色（如 "assistant"）
	Type             string          `json:"type"`               // 消息类型（如 "answer"、"reasoning" 等）
	BotId            string          `json:"bot_id"`             // Bot ID
	ChatId           string          `json:"chat_id"`            // 聊天 ID
	Content          json.RawMessage `json:"content"`            // 消息内容，JSON 格式
	MetaData         json.RawMessage `json:"meta_data"`          // 元数据
	CreatedAt        int64           `json:"created_at"`         // 创建时间戳（秒）
	SectionId        string          `json:"section_id"`         // 段落 ID
	UpdatedAt        int64           `json:"updated_at"`         // 更新时间戳（秒）
	ContentType      string          `json:"content_type"`       // 内容类型（如 "text"）
	ConversationId   string          `json:"conversation_id"`    // 对话 ID
	ReasoningContent string          `json:"reasoning_content"`  // 推理内容（思考链），展示模型的推理过程
}

// CozeResponseDetail 表示 Coze API 响应的详细信息。
// 包含用于问题追踪的日志 ID。
type CozeResponseDetail struct {
	Logid string `json:"logid"` // 日志追踪 ID，可用于问题排查
}
