// dify - dto.go
// 本文件定义了 Dify 渠道的数据传输对象（DTO）。
// 包含请求和响应的结构体定义，用于与 Dify API 进行数据交互。
// Dify 支持流式（streaming）和阻塞（blocking）两种响应模式，
// 以及工作流（workflow）调试信息的透传。
package dify

import (
	"github.com/c1cada/NexusTok/dto"
)

// DifyChatRequest 表示发送到 Dify 聊天 API 的请求结构体。
// 对应 Dify API 的 /v1/chat-messages 端点。
type DifyChatRequest struct {
	Inputs           map[string]interface{} `json:"inputs"`              // 输入变量（用于 Workflow 类型应用）
	Query            string                 `json:"query"`               // 用户查询文本
	ResponseMode     string                 `json:"response_mode"`       // 响应模式："blocking"（阻塞）或 "streaming"（流式）
	User             string                 `json:"user"`                // 用户标识
	AutoGenerateName bool                   `json:"auto_generate_name"`  // 是否自动生成会话名称
	Files            []DifyFile             `json:"files"`               // 附件文件列表（如图片）
}

// DifyFile 表示发送到 Dify API 的文件附件。
// 支持两种传输模式：本地文件上传和远程 URL 引用。
type DifyFile struct {
	Type         string `json:"type"`                    // 文件类型（如 "image"）
	TransferMode string `json:"transfer_mode"`           // 传输模式："local_file"（本地文件）或 "remote_url"（远程 URL）
	URL          string `json:"url,omitempty"`           // 远程文件 URL（remote_url 模式使用）
	UploadFileId string `json:"upload_file_id,omitempty"` // 上传后的文件 ID（local_file 模式使用）
}

// DifyMetaData 表示 Dify API 响应中的元数据。
// 包含 token 用量统计信息。
type DifyMetaData struct {
	Usage dto.Usage `json:"usage"` // token 用量信息（输入、输出、总计）
}

// DifyData 表示 Dify 流式响应中工作流相关的数据。
// 用于调试模式下展示工作流和节点的执行信息。
type DifyData struct {
	WorkflowId string `json:"workflow_id"` // 工作流 ID
	NodeId     string `json:"node_id"`     // 节点 ID
	NodeType   string `json:"node_type"`   // 节点类型（如 "llm"、"code" 等）
	Status     string `json:"status"`      // 执行状态（如 "running"、"succeeded"、"failed" 等）
}

// DifyChatCompletionResponse 表示 Dify 非流式聊天响应。
// 对应 blocking 模式下的完整响应。
type DifyChatCompletionResponse struct {
	ConversationId string       `json:"conversation_id"` // 对话 ID
	Answer         string       `json:"answer"`          // 回答文本
	CreateAt       int64        `json:"create_at"`       // 创建时间戳
	MetaData       DifyMetaData `json:"metadata"`        // 元数据（含用量信息）
}

// DifyChunkChatCompletionResponse 表示 Dify 流式聊天响应中的单个事件块。
// 对应 streaming 模式下的 SSE 事件。
type DifyChunkChatCompletionResponse struct {
	Event          string       `json:"event"`           // 事件类型（如 "message"、"message_end"、"workflow_started" 等）
	ConversationId string       `json:"conversation_id"` // 对话 ID
	Answer         string       `json:"answer"`          // 增量回答文本
	Data           DifyData     `json:"data"`            // 工作流相关数据
	MetaData       DifyMetaData `json:"metadata"`        // 元数据（含用量信息）
}
