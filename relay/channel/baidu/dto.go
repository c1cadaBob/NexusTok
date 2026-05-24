// Package baidu 定义百度文心一言 API 的数据传输对象（DTO）。
// 包含请求和响应的结构体定义，用于与百度文心一言 API 进行 JSON 序列化/反序列化。
package baidu

// 标准库导入

// 项目内部导入

// BaiduMessage 表示百度文心一言 API 的消息结构。
// 每条消息包含角色（role）和内容（content）两个字段。
type BaiduMessage struct {
	Role    string `json:"role"`    // 消息角色：user（用户）、assistant（助手）
	Content string `json:"content"` // 消息内容
}

// BaiduChatRequest 表示百度文心一言对话 API 的请求结构。
// 包含消息列表、模型参数、流式输出控制等配置项。
type BaiduChatRequest struct {
	Messages        []BaiduMessage  `json:"messages"`                    // 对话消息列表
	Temperature     *float64        `json:"temperature,omitempty"`       // 采样温度，控制输出随机性
	TopP            float64         `json:"top_p,omitempty"`             // 核采样概率阈值
	PenaltyScore    float64         `json:"penalty_score,omitempty"`     // 重复惩罚分数（1.0-2.0）
	Stream          bool            `json:"stream,omitempty"`            // 是否启用流式输出
	System          string          `json:"system,omitempty"`            // 系统提示词
	DisableSearch   bool            `json:"disable_search,omitempty"`    // 是否禁用搜索功能
	EnableCitation  bool            `json:"enable_citation,omitempty"`   // 是否启用引用功能
	MaxOutputTokens *int            `json:"max_output_tokens,omitempty"` // 最大输出 token 数
	UserId          json.RawMessage `json:"user_id,omitempty"`           // 用户唯一标识
}

// Error 表示百度文心一言 API 返回的错误信息结构。
type Error struct {
	ErrorCode int    `json:"error_code"` // 错误码
	ErrorMsg  string `json:"error_msg"`  // 错误描述信息
}

// BaiduChatResponse 表示百度文心一言对话 API 的非流式响应结构。
// 包含对话结果、是否截断标记、历史清理标记和 token 使用量。
type BaiduChatResponse struct {
	Id               string    `json:"id"`                 // 请求唯一标识
	Object           string    `json:"object"`             // 对象类型
	Created          int64     `json:"created"`            // 创建时间戳
	Result           string    `json:"result"`             // 对话返回的结果文本
	IsTruncated      bool      `json:"is_truncated"`       // 结果是否被截断
	NeedClearHistory bool      `json:"need_clear_history"` // 是否需要清理对话历史
	Usage            dto.Usage `json:"usage"`              // token 使用量统计
	Error                                        // 内嵌错误信息
}

// BaiduChatStreamResponse 表示百度文心一言对话 API 的流式响应结构。
// 在非流式响应基础上增加了句子 ID 和结束标记。
type BaiduChatStreamResponse struct {
	BaiduChatResponse                    // 内嵌非流式响应结构
	SentenceId int  `json:"sentence_id"` // 句子 ID，用于标识流式输出中的每个片段
	IsEnd      bool `json:"is_end"`      // 是否为最后一个流式响应片段
}

// BaiduEmbeddingRequest 表示百度文心一言向量化 API 的请求结构。
type BaiduEmbeddingRequest struct {
	Input []string `json:"input"` // 待向量化的文本列表
}

// BaiduEmbeddingData 表示百度文心一言向量化 API 返回的单条向量数据。
type BaiduEmbeddingData struct {
	Object    string    `json:"object"`    // 对象类型
	Embedding []float64 `json:"embedding"` // 向量数据（浮点数数组）
	Index     int       `json:"index"`     // 在输入列表中的索引位置
}

// BaiduEmbeddingResponse 表示百度文心一言向量化 API 的响应结构。
type BaiduEmbeddingResponse struct {
	Id      string               `json:"id"`      // 请求唯一标识
	Object  string               `json:"object"`  // 对象类型
	Created int64                `json:"created"` // 创建时间戳
	Data    []BaiduEmbeddingData `json:"data"`    // 向量数据列表
	Usage   dto.Usage            `json:"usage"`   // token 使用量统计
	Error                                         // 内嵌错误信息
}

// BaiduAccessToken 表示百度 API 的 OAuth Access Token 信息。
// 用于百度文心一言 API 的身份认证。
type BaiduAccessToken struct {
	AccessToken      string    `json:"access_token"`                 // 访问令牌
	Error            string    `json:"error,omitempty"`              // 错误类型
	ErrorDescription string    `json:"error_description,omitempty"`  // 错误描述
	ExpiresIn        int64     `json:"expires_in,omitempty"`         // 令牌有效期（秒）
	ExpiresAt        time.Time `json:"-"`                            // 令牌过期时间（不参与 JSON 序列化）
}

// BaiduTokenResponse 表示百度 Token 接口的响应结构。
type BaiduTokenResponse struct {
	ExpiresIn   int    `json:"expires_in"`   // 令牌有效期（秒）
	AccessToken string `json:"access_token"` // 访问令牌
}
