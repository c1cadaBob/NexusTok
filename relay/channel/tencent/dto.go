// Package tencent 的数据传输对象（DTO）定义文件。
// 定义了腾讯云混元 API 的请求和响应结构体，
// 包含聊天消息格式、请求参数、错误信息、token 使用量和响应选项等。
// 腾讯云混元 API 文档：https://cloud.tencent.com/document/product/1729/97732
package tencent

// TencentMessage 腾讯云混元聊天消息格式。
// 包含消息角色（Role）和文本内容（Content），用于构建对话上下文。
type TencentMessage struct {
	Role    string `json:"Role"`    // 消息角色："system"（系统）、"user"（用户）、"assistant"（助手）
	Content string `json:"Content"` // 消息文本内容
}

// TencentChatRequest 腾讯云混元聊天请求结构体。
// 包含模型选择、对话消息列表、流式开关以及生成参数（温度、Top-P 等）。
type TencentChatRequest struct {
	// 模型名称，可选值包括 hunyuan-lite、hunyuan-standard、hunyuan-standard-256K、hunyuan-pro。
	// 各模型介绍请阅读 [产品概述](https://cloud.tencent.com/document/product/1729/104753) 中的说明。
	//
	// 注意：
	// 不同的模型计费不同，请根据 [购买指南](https://cloud.tencent.com/document/product/1729/97731) 按需调用。
	Model *string `json:"Model"`
	// 聊天上下文信息。
	// 说明：
	// 1. 长度最多为 40，按对话时间从旧到新在数组中排列。
	// 2. Message.Role 可选值：system、user、assistant。
	// 其中，system 角色可选，如存在则必须位于列表的最开始。user 和 assistant 需交替出现（一问一答），以 user 提问开始和结束，且 Content 不能为空。Role 的顺序示例：[system（可选） user assistant user assistant user ...]。
	// 3. Messages 中 Content 总长度不能超过模型输入长度上限（可参考 [产品概述](https://cloud.tencent.com/document/product/1729/104753) 文档），超过则会截断最前面的内容，只保留尾部内容。
	Messages []*TencentMessage `json:"Messages"`
	// 流式调用开关。
	// 说明：
	// 1. 未传值时默认为非流式调用（false）。
	// 2. 流式调用时以 SSE 协议增量返回结果（返回值取 Choices[n].Delta 中的值，需要拼接增量数据才能获得完整结果）。
	// 3. 非流式调用时：
	// 调用方式与普通 HTTP 请求无异。
	// 接口响应耗时较长，**如需更低时延建议设置为 true**。
	// 只返回一次最终结果（返回值取 Choices[n].Message 中的值）。
	//
	// 注意：
	// 通过 SDK 调用时，流式和非流式调用需用**不同的方式**获取返回值，具体参考 SDK 中的注释或示例（在各语言 SDK 代码仓库的 examples/hunyuan/v20230901/ 目录中）。
	Stream *bool `json:"Stream,omitempty"`
	// 说明：
	// 1. 影响输出文本的多样性，取值越大，生成文本的多样性越强。
	// 2. 取值区间为 [0.0, 1.0]，未传值时使用各模型推荐值。
	// 3. 非必要不建议使用，不合理的取值会影响效果。
	TopP *float64 `json:"TopP,omitempty"`
	// 说明：
	// 1. 较高的数值会使输出更加随机，而较低的数值会使其更加集中和确定。
	// 2. 取值区间为 [0.0, 2.0]，未传值时使用各模型推荐值。
	// 3. 非必要不建议使用，不合理的取值会影响效果。
	Temperature *float64 `json:"Temperature,omitempty"`
}

// TencentError 腾讯云 API 错误信息结构体。
// 当请求失败时，响应中会包含此结构体描述错误原因。
type TencentError struct {
	Code    int    `json:"Code"`    // 错误码，0 表示成功，非 0 表示错误
	Message string `json:"Message"` // 错误描述信息
}

// TencentUsage 腾讯云 API 的 token 使用量统计结构体。
// 记录输入、输出和总计的 token 消耗数量。
type TencentUsage struct {
	PromptTokens     int `json:"PromptTokens"`     // 输入（提示词）token 数量
	CompletionTokens int `json:"CompletionTokens"` // 输出（补全）token 数量
	TotalTokens      int `json:"TotalTokens"`      // 总 token 数量
}

// TencentResponseChoices 腾讯云 API 响应中的单个选项结构体。
// 同步模式通过 Messages 字段返回内容，流式模式通过 Delta 字段返回增量内容。
type TencentResponseChoices struct {
	FinishReason string         `json:"FinishReason,omitempty"` // 流式结束标志位，为 "stop" 则表示尾包
	Messages     TencentMessage `json:"Message,omitempty"`      // 内容，同步模式返回内容，流模式为 null 输出 content 内容总数最多支持 1024token。
	Delta        TencentMessage `json:"Delta,omitempty"`        // 内容，流模式返回内容，同步模式为 null 输出 content 内容总数最多支持 1024token。
}

// TencentChatResponse 腾讯云混元聊天响应结构体。
// 包含生成结果选项、创建时间、会话 ID、token 使用量和错误信息等。
type TencentChatResponse struct {
	Choices []TencentResponseChoices `json:"Choices,omitempty"` // 结果
	Created int64                    `json:"Created,omitempty"` // unix 时间戳的字符串
	Id      string                   `json:"Id,omitempty"`      // 会话 id
	Usage   TencentUsage             `json:"Usage,omitempty"`   // token 数量
	Error   TencentError             `json:"Error,omitempty"`   // 错误信息 注意：此字段可能返回 null，表示取不到有效值
	Note    string                   `json:"Note,omitempty"`    // 注释
	ReqID   string                   `json:"Req_id,omitempty"`  // 唯一请求 Id，每次请求都会返回。用于反馈接口入参
}

// TencentChatResponseSB 腾讯云 API 响应的顶层包装结构体。
// 腾讯云 API 的实际响应数据嵌套在 Response 字段中。
type TencentChatResponseSB struct {
	Response TencentChatResponse `json:"Response,omitempty"` // 实际的聊天响应数据
}
