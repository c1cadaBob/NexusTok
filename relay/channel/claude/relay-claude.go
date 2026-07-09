// Package claude - relay-claude.go
//
// Claude 渠道中继适配器，负责在 OpenAI 格式与 Anthropic Claude 格式之间进行请求/响应的双向转换。
//
// 核心功能：
//   - RequestOpenAI2ClaudeMessage：将 OpenAI 格式的聊天补全请求转换为 Claude API 格式，包括消息、工具、
//     思维链（thinking）、Web 搜索等参数的映射。
//   - StreamResponseClaude2OpenAI / ResponseClaude2OpenAI：将 Claude 的流式/非流式响应转换为 OpenAI 格式。
//   - FormatClaudeResponseInfo：在流式传输过程中逐步累积 Claude 的 usage 信息。
//   - HandleStreamResponseData / HandleStreamFinalResponse：处理 Claude 流式 SSE 数据并向客户端转发。
//   - ClaudeStreamHandler / ClaudeHandler：流式与非流式响应的顶层入口函数。
//   - mapToolChoice：将 OpenAI 的 tool_choice 参数映射为 Claude 的 tool_choice 格式。
//
// 本文件同时支持两种中继格式（RelayFormatOpenAI 和 RelayFormatClaude），并处理 Bedrock 等上游
// 返回的 message_delta 缺少完整 usage 字段的兼容性问题。
package claude

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/logger"
	"github.com/c1cada/NexusTok/relay/channel/openrouter"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/relay/helper"
	"github.com/c1cada/NexusTok/relay/reasonmap"
	"github.com/c1cada/NexusTok/service"
	"github.com/c1cada/NexusTok/setting/model_setting"
	"github.com/c1cada/NexusTok/setting/reasoning"
	"github.com/c1cada/NexusTok/types"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Web 搜索工具的 max_uses 参数常量。
// Anthropic Claude 的 web_search 工具通过 max_uses 控制最大搜索次数，
// 对应 OpenAI 的 search_context_size（low/medium/high）映射到不同的搜索次数上限。
const (
	// WebSearchMaxUsesLow 对应 search_context_size="low"，最多执行 1 次搜索。
	WebSearchMaxUsesLow = 1
	// WebSearchMaxUsesMedium 对应 search_context_size="medium"，最多执行 5 次搜索。
	WebSearchMaxUsesMedium = 5
	// WebSearchMaxUsesHigh 对应 search_context_size="high"，最多执行 10 次搜索。
	WebSearchMaxUsesHigh = 10
)

// stopReasonClaude2OpenAI 将 Claude 的停止原因（stop_reason）转换为 OpenAI 的完成原因（finish_reason）。
// Claude 使用 "end_turn"、"max_tokens"、"tool_use"、"refusal" 等停止原因，
// OpenAI 使用 "stop"、"length"、"tool_calls"、"content_filter" 等完成原因。
// 该函数委托给 reasonmap 包中的映射表进行转换。
func stopReasonClaude2OpenAI(reason string) string {
	return reasonmap.ClaudeStopReasonToOpenAIFinishReason(reason)
}

// maybeMarkClaudeRefusal 检查 Claude 响应的停止原因是否为 "refusal"（拒绝），
// 如果是，则在 Gin 上下文中设置管理员拒绝原因标记，用于审计和计费排除。
// 该函数是安全的：当 gin.Context 为 nil 时直接返回，不会引发 panic。
func maybeMarkClaudeRefusal(c *gin.Context, stopReason string) {
	if c == nil {
		return
	}
	if strings.EqualFold(stopReason, "refusal") {
		common.SetContextKey(c, constant.ContextKeyAdminRejectReason, "claude_stop_reason=refusal")
	}
}

// mimeTypeFromOpenAIFileName 根据 OpenAI file 内容中的 filename 推断 MIME。
//
// OpenAI 兼容请求的 file_data 只有 base64 数据和 filename，没有独立
// MIME 字段。Claude 上游对内容块类型非常严格，因此这里必须先用文件名
// 扩展名识别可支持类型；未知扩展名返回 application/octet-stream，调用方
// 应跳过而不是错误地包装成 image。
func mimeTypeFromOpenAIFileName(fileName string) string {
	fileName = strings.TrimSpace(fileName)
	if fileName == "" {
		return "application/octet-stream"
	}
	if dot := strings.LastIndex(fileName, "."); dot >= 0 && dot+1 < len(fileName) {
		return service.GetMimeTypeByExtension(fileName[dot+1:])
	}
	return "application/octet-stream"
}

// stripBase64DataURLPrefix 移除 data URL 前缀，返回可直接解码的 base64 数据。
func stripBase64DataURLPrefix(data string) string {
	if strings.HasPrefix(data, "data:") {
		if idx := strings.Index(data, ","); idx >= 0 && idx+1 < len(data) {
			return data[idx+1:]
		}
	}
	return data
}

// convertOpenAIFileContentForClaude 将 OpenAI file 内容转换为 Claude 支持的内容块。
//
// 支持范围刻意保持保守：
//   - text/*：解码为 Claude text 内容块。
//   - application/pdf：转换为 Claude document 内容块。
//   - image/*：转换为 Claude image 内容块。
//   - 其他未知二进制：跳过，避免把无效 media_type 伪装成 image 发送给上游。
func convertOpenAIFileContentForClaude(c *gin.Context, file *dto.MessageFile) (*dto.ClaudeMediaMessage, bool, error) {
	if file == nil || file.FileData == "" {
		return nil, false, nil
	}

	mimeType := mimeTypeFromOpenAIFileName(file.FileName)
	if strings.HasPrefix(mimeType, "text/") {
		decoded, err := base64.StdEncoding.DecodeString(stripBase64DataURLPrefix(file.FileData))
		if err != nil {
			return nil, false, fmt.Errorf("decode text file failed: %w", err)
		}
		text := string(decoded)
		if text == "" {
			return nil, false, nil
		}
		return &dto.ClaudeMediaMessage{
			Type: "text",
			Text: common.GetPointer[string](text),
		}, true, nil
	}

	if !strings.HasPrefix(mimeType, "application/pdf") && !strings.HasPrefix(mimeType, "image/") {
		return nil, false, nil
	}

	source := types.NewFileSourceFromData(file.FileData, mimeType)
	base64Data, resolvedMimeType, err := service.GetBase64Data(c, source, "formatting file for Claude")
	if err != nil {
		return nil, false, fmt.Errorf("get file data failed: %w", err)
	}
	if resolvedMimeType == "" {
		resolvedMimeType = mimeType
	}

	claudeMediaMessage := &dto.ClaudeMediaMessage{
		Source: &dto.ClaudeMessageSource{
			Type:      "base64",
			MediaType: resolvedMimeType,
			Data:      base64Data,
		},
	}
	switch {
	case strings.HasPrefix(resolvedMimeType, "application/pdf"):
		claudeMediaMessage.Type = "document"
	case strings.HasPrefix(resolvedMimeType, "image/"):
		claudeMediaMessage.Type = "image"
	default:
		return nil, false, nil
	}

	return claudeMediaMessage, true, nil
}

// RequestOpenAI2ClaudeMessage 将 OpenAI 格式的聊天补全请求转换为 Claude API 格式。
//
// 该函数是请求转换的核心入口，处理以下映射：
//
//  1. 工具（Tools）：将 OpenAI 的 function calling 工具定义转换为 Claude 的 tool 格式，
//     包括 input_schema 的提取（type、properties、required 及额外字段）。
//
//  2. Web 搜索工具：如果请求中包含 web_search_options，自动注入 Claude 的 web_search_20250305 工具，
//     并将 search_context_size（low/medium/high）映射为 max_uses，同时处理 user_location 的转换。
//
//  3. 请求参数：映射 temperature、top_p、top_k、max_tokens、stream、stop_sequences 等参数。
//     如果客户端未指定 max_tokens，使用 Claude 配置中的默认值。
//
//  4. 思维链（Thinking）：支持三种模式：
//     a) Effort 后缀模式：如 claude-opus-4-6-low，从模型名提取 effort level，使用 adaptive thinking。
//     Opus 4.7 特殊处理：使用 summarized display 并清空 temperature/top_p/top_k。
//     b) -thinking 后缀模式：如 claude-3-5-sonnet-thinking，启用 extended thinking，
//     BudgetTokens 为 max_tokens 的可配置百分比（默认 80%）。
//     c) reasoning_effort 参数：直接映射为固定 BudgetTokens（low=1280, medium=2048, high=4096）。
//     d) reasoning 对象：通过 openrouter.RequestReasoning 结构的 max_tokens 覆盖 BudgetTokens。
//
//  5. 消息转换：
//     - system 消息：提取为 Claude 的 system 数组字段，支持纯文本和复合内容。
//     - 连续同角色消息合并（tool 角色除外）。
//     - 首条消息非 user 时自动插入占位 user 消息（Claude 要求首条消息必须为 user）。
//     - tool 角色消息转换为 user 消息 + tool_result 内容块。
//     - 图片和 PDF 文件：通过 base64 编码转换为 Claude 的 image/document 内容块。
//     - assistant 消息中的 tool_calls 转换为 tool_use 内容块。
//
// 参数：
//   - c: Gin 上下文，用于文件服务访问（图片/PDF 获取）
//   - textRequest: OpenAI 格式的请求体
//
// 返回值：
//   - *dto.ClaudeRequest: 转换后的 Claude 格式请求
//   - error: 转换过程中的错误，成功则为 nil
func RequestOpenAI2ClaudeMessage(c *gin.Context, textRequest dto.GeneralOpenAIRequest) (*dto.ClaudeRequest, error) {
	claudeTools := make([]any, 0, len(textRequest.Tools))

	for _, tool := range textRequest.Tools {
		if params, ok := tool.Function.Parameters.(map[string]any); ok {
			claudeTool := dto.Tool{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
			}
			claudeTool.InputSchema = make(map[string]interface{})
			if params["type"] != nil {
				claudeTool.InputSchema["type"] = params["type"].(string)
			}
			claudeTool.InputSchema["properties"] = params["properties"]
			claudeTool.InputSchema["required"] = params["required"]
			for s, a := range params {
				if s == "type" || s == "properties" || s == "required" {
					continue
				}
				claudeTool.InputSchema[s] = a
			}
			claudeTools = append(claudeTools, &claudeTool)
		}
	}

	// Web search tool
	// https://docs.anthropic.com/en/docs/agents-and-tools/tool-use/web-search-tool
	if textRequest.WebSearchOptions != nil {
		webSearchTool := dto.ClaudeWebSearchTool{
			Type: "web_search_20250305",
			Name: "web_search",
		}

		// 处理 user_location
		if textRequest.WebSearchOptions.UserLocation != nil {
			anthropicUserLocation := &dto.ClaudeWebSearchUserLocation{
				Type: "approximate", // 固定为 "approximate"
			}

			// 解析 UserLocation JSON
			var userLocationMap map[string]interface{}
			if err := common.Unmarshal(textRequest.WebSearchOptions.UserLocation, &userLocationMap); err == nil {
				// 检查是否有 approximate 字段
				if approximateData, ok := userLocationMap["approximate"].(map[string]interface{}); ok {
					if timezone, ok := approximateData["timezone"].(string); ok && timezone != "" {
						anthropicUserLocation.Timezone = timezone
					}
					if country, ok := approximateData["country"].(string); ok && country != "" {
						anthropicUserLocation.Country = country
					}
					if region, ok := approximateData["region"].(string); ok && region != "" {
						anthropicUserLocation.Region = region
					}
					if city, ok := approximateData["city"].(string); ok && city != "" {
						anthropicUserLocation.City = city
					}
				}
			}

			webSearchTool.UserLocation = anthropicUserLocation
		}

		// 处理 search_context_size 转换为 max_uses
		if textRequest.WebSearchOptions.SearchContextSize != "" {
			switch textRequest.WebSearchOptions.SearchContextSize {
			case "low":
				webSearchTool.MaxUses = WebSearchMaxUsesLow
			case "medium":
				webSearchTool.MaxUses = WebSearchMaxUsesMedium
			case "high":
				webSearchTool.MaxUses = WebSearchMaxUsesHigh
			}
		}

		claudeTools = append(claudeTools, &webSearchTool)
	}

	claudeRequest := dto.ClaudeRequest{
		Model:         textRequest.Model,
		StopSequences: nil,
		Temperature:   textRequest.Temperature,
		Tools:         claudeTools,
	}
	if maxTokens := textRequest.GetMaxTokens(); maxTokens > 0 {
		claudeRequest.MaxTokens = common.GetPointer(maxTokens)
	}
	if textRequest.TopP != nil {
		claudeRequest.TopP = common.GetPointer(*textRequest.TopP)
	}
	if textRequest.TopK != nil {
		claudeRequest.TopK = common.GetPointer(*textRequest.TopK)
	}
	if textRequest.IsStream(nil) {
		claudeRequest.Stream = common.GetPointer(true)
	}

	// 处理 tool_choice 和 parallel_tool_calls
	if textRequest.ToolChoice != nil || textRequest.ParallelTooCalls != nil {
		claudeToolChoice := mapToolChoice(textRequest.ToolChoice, textRequest.ParallelTooCalls)
		if claudeToolChoice != nil {
			claudeRequest.ToolChoice = claudeToolChoice
		}
	}

	if claudeRequest.MaxTokens == nil || *claudeRequest.MaxTokens == 0 {
		defaultMaxTokens := uint(model_setting.GetClaudeSettings().GetDefaultMaxTokens(textRequest.Model))
		claudeRequest.MaxTokens = &defaultMaxTokens
	}

	// ---- 思维链（Thinking）处理 ----
	// 优先级从高到低依次为：
	// 1. Effort 后缀模式（如 claude-opus-4-6-low）- 仅限 Opus 4.6/4.7
	// 2. -thinking 后缀模式（如 claude-3-5-sonnet-thinking）- 通用 Claude 模型
	// 3. reasoning_effort 参数 - 低/中/高三档
	// 4. reasoning 对象 - 自定义 max_tokens

	// 模式一：Effort 后缀模式。
	// 从模型名中提取 effort level（如 "low"、"medium"、"high"），使用 Claude 的 adaptive thinking。
	// 例如 claude-opus-4-6-low -> baseModel="claude-opus-4-6", effortLevel="low"
	if baseModel, effortLevel, ok := reasoning.TrimEffortSuffix(textRequest.Model); ok && effortLevel != "" &&
		(strings.HasPrefix(textRequest.Model, "claude-opus-4-6") || strings.HasPrefix(textRequest.Model, "claude-opus-4-7")) {
		claudeRequest.Model = baseModel
		claudeRequest.Thinking = &dto.Thinking{
			Type: "adaptive",
		}
		claudeRequest.OutputConfig = json.RawMessage(fmt.Sprintf(`{"effort":"%s"}`, effortLevel))
		if strings.HasPrefix(baseModel, "claude-opus-4-7") {
			// Opus 4.7 rejects non-default temperature/top_p/top_k with 400
			// and defaults display to "omitted"; restore the 4.6 visible summary.
			claudeRequest.Thinking.Display = "summarized"
			claudeRequest.Temperature = nil
			claudeRequest.TopP = nil
			claudeRequest.TopK = nil
		} else {
			claudeRequest.TopP = nil
			claudeRequest.Temperature = common.GetPointer[float64](1.0)
		}
		// 模式二：-thinking 后缀模式。
		// 当 ThinkingAdapterEnabled 配置启用且模型名以 "-thinking" 结尾时，自动启用 extended thinking。
		// 例如 claude-3-5-sonnet-thinking -> trimmedModel="claude-3-5-sonnet"
	} else if model_setting.GetClaudeSettings().ThinkingAdapterEnabled &&
		strings.HasSuffix(textRequest.Model, "-thinking") {

		trimmedModel := strings.TrimSuffix(textRequest.Model, "-thinking")
		if strings.HasPrefix(trimmedModel, "claude-opus-4-7") {
			// Opus 4.7 rejects thinking.type="enabled"; use adaptive at high effort.
			claudeRequest.Thinking = &dto.Thinking{Type: "adaptive", Display: "summarized"}
			claudeRequest.OutputConfig = json.RawMessage(`{"effort":"high"}`)
			claudeRequest.Temperature = nil
			claudeRequest.TopP = nil
			claudeRequest.TopK = nil
		} else {
			// 因为BudgetTokens 必须大于1024
			if claudeRequest.MaxTokens == nil || *claudeRequest.MaxTokens < 1280 {
				claudeRequest.MaxTokens = common.GetPointer[uint](1280)
			}

			// BudgetTokens 为 max_tokens 的 80%
			claudeRequest.Thinking = &dto.Thinking{
				Type:         "enabled",
				BudgetTokens: common.GetPointer[int](int(float64(*claudeRequest.MaxTokens) * model_setting.GetClaudeSettings().ThinkingAdapterBudgetTokensPercentage)),
			}
			// TODO: 临时处理
			// https://docs.anthropic.com/en/docs/build-with-claude/extended-thinking#important-considerations-when-using-extended-thinking
			claudeRequest.TopP = nil
			claudeRequest.Temperature = common.GetPointer[float64](1.0)
		}
		if !model_setting.ShouldPreserveThinkingSuffix(textRequest.Model) {
			claudeRequest.Model = trimmedModel
		}
	}

	// 模式三：reasoning_effort 参数。
	// 直接映射为固定 BudgetTokens：low=1280, medium=2048, high=4096。
	if textRequest.ReasoningEffort != "" {
		switch textRequest.ReasoningEffort {
		case "low":
			claudeRequest.Thinking = &dto.Thinking{
				Type:         "enabled",
				BudgetTokens: common.GetPointer[int](1280),
			}
		case "medium":
			claudeRequest.Thinking = &dto.Thinking{
				Type:         "enabled",
				BudgetTokens: common.GetPointer[int](2048),
			}
		case "high":
			claudeRequest.Thinking = &dto.Thinking{
				Type:         "enabled",
				BudgetTokens: common.GetPointer[int](4096),
			}
		}
	}

	// 模式四：reasoning 对象（来自 OpenRouter 等中间层）。
	// 通过 reasoning.max_tokens 字段直接设置 BudgetTokens，会覆盖前面模式三的设置。
	if textRequest.Reasoning != nil {
		var reasoning openrouter.RequestReasoning
		if err := common.Unmarshal(textRequest.Reasoning, &reasoning); err != nil {
			return nil, err
		}

		budgetTokens := reasoning.MaxTokens
		if budgetTokens > 0 {
			claudeRequest.Thinking = &dto.Thinking{
				Type:         "enabled",
				BudgetTokens: &budgetTokens,
			}
		}
	}

	if textRequest.Stop != nil {
		// stop maybe string/array string, convert to array string
		switch textRequest.Stop.(type) {
		case string:
			claudeRequest.StopSequences = []string{textRequest.Stop.(string)}
		case []interface{}:
			stopSequences := make([]string, 0)
			for _, stop := range textRequest.Stop.([]interface{}) {
				stopSequences = append(stopSequences, stop.(string))
			}
			claudeRequest.StopSequences = stopSequences
		}
	}
	// ---- 消息预处理阶段 ----
	// 将 OpenAI 消息列表进行规范化处理：
	// 1. 空角色默认填充为 "user"
	// 2. 保留 tool 角色的 tool_call_id 和 assistant 角色的 tool_calls
	// 3. 合并连续同角色消息（tool 角色除外），避免 Claude API 拒绝请求
	// 4. 空内容消息填充为 "..." 占位符（Claude 不接受空内容）
	formatMessages := make([]dto.Message, 0)
	lastMessage := dto.Message{
		Role: "tool",
	}
	for i, message := range textRequest.Messages {
		if message.Role == "" {
			textRequest.Messages[i].Role = "user"
		}
		fmtMessage := dto.Message{
			Role:    message.Role,
			Content: message.Content,
		}
		if message.Role == "tool" {
			fmtMessage.ToolCallId = message.ToolCallId
		}
		if message.Role == "assistant" && message.ToolCalls != nil {
			fmtMessage.ToolCalls = message.ToolCalls
		}
		if lastMessage.Role == message.Role && lastMessage.Role != "tool" {
			if lastMessage.IsStringContent() && message.IsStringContent() {
				fmtMessage.SetStringContent(strings.Trim(fmt.Sprintf("%s %s", lastMessage.StringContent(), message.StringContent()), "\""))
				// delete last message
				formatMessages = formatMessages[:len(formatMessages)-1]
			}
		}
		if fmtMessage.Content == nil || (fmtMessage.IsStringContent() && fmtMessage.StringContent() == "") {
			fmtMessage.SetStringContent("...")
		}
		formatMessages = append(formatMessages, fmtMessage)
		lastMessage = fmtMessage
	}

	// ---- Claude 消息格式转换阶段 ----
	// 将预处理后的 OpenAI 消息转换为 Claude 的消息格式：
	// - system 消息累积到 systemMessages 数组（Claude 的 system 是顶层字段，非消息）
	// - 非 system 消息转换为 ClaudeMessage 结构
	// - 首条消息非 user 时自动插入占位 user 消息（Claude 要求）
	// - tool 角色消息合并到前一条 user 消息中（Claude 的 tool_result 必须在 user 消息内）
	// - 图片/PDF 内容转换为 base64 编码的 image/document 内容块
	// - assistant 消息中的 tool_calls 转换为 tool_use 内容块
	claudeMessages := make([]dto.ClaudeMessage, 0)
	isFirstMessage := true
	// 初始化system消息数组，用于累积多个system消息
	var systemMessages []dto.ClaudeMediaMessage

	for _, message := range formatMessages {
		if message.Role == "system" {
			// 根据Claude API规范，system字段使用数组格式更有通用性
			if message.IsStringContent() {
				if text := message.StringContent(); text != "" {
					systemMessages = append(systemMessages, dto.ClaudeMediaMessage{
						Type: "text",
						Text: common.GetPointer[string](text),
					})
				}
			} else {
				// 支持复合内容的system消息（虽然不常见，但需要考虑完整性）
				for _, ctx := range message.ParseContent() {
					if ctx.Type == "text" && ctx.Text != "" {
						systemMessages = append(systemMessages, dto.ClaudeMediaMessage{
							Type: "text",
							Text: common.GetPointer[string](ctx.Text),
						})
					}
					// 未来可以在这里扩展对图片等其他类型的支持
				}
			}
		} else {
			if isFirstMessage {
				isFirstMessage = false
				if message.Role != "user" {
					// fix: first message is assistant, add user message
					claudeMessage := dto.ClaudeMessage{
						Role: "user",
						Content: []dto.ClaudeMediaMessage{
							{
								Type: "text",
								Text: common.GetPointer[string]("..."),
							},
						},
					}
					claudeMessages = append(claudeMessages, claudeMessage)
				}
			}
			claudeMessage := dto.ClaudeMessage{
				Role: message.Role,
			}
			if message.Role == "tool" {
				if len(claudeMessages) > 0 && claudeMessages[len(claudeMessages)-1].Role == "user" {
					lastMessage := claudeMessages[len(claudeMessages)-1]
					if content, ok := lastMessage.Content.(string); ok {
						lastMessage.Content = []dto.ClaudeMediaMessage{
							{
								Type: "text",
								Text: common.GetPointer[string](content),
							},
						}
					}
					lastMessage.Content = append(lastMessage.Content.([]dto.ClaudeMediaMessage), dto.ClaudeMediaMessage{
						Type:      "tool_result",
						ToolUseId: message.ToolCallId,
						Content:   message.Content,
					})
					claudeMessages[len(claudeMessages)-1] = lastMessage
					continue
				} else {
					claudeMessage.Role = "user"
					claudeMessage.Content = []dto.ClaudeMediaMessage{
						{
							Type:      "tool_result",
							ToolUseId: message.ToolCallId,
							Content:   message.Content,
						},
					}
				}
			} else if message.IsStringContent() && message.ToolCalls == nil {
				text := message.StringContent()
				if text == "" {
					text = "..."
				}
				claudeMessage.Content = text
			} else {
				claudeMediaMessages := make([]dto.ClaudeMediaMessage, 0)
				for _, mediaMessage := range message.ParseContent() {
					switch mediaMessage.Type {
					case "text":
						if mediaMessage.Text != "" {
							claudeMediaMessages = append(claudeMediaMessages, dto.ClaudeMediaMessage{
								Type: "text",
								Text: common.GetPointer[string](mediaMessage.Text),
							})
						}
					case dto.ContentTypeFile:
						claudeMediaMessage, ok, err := convertOpenAIFileContentForClaude(c, mediaMessage.GetFile())
						if err != nil {
							return nil, err
						}
						if ok && claudeMediaMessage != nil {
							claudeMediaMessages = append(claudeMediaMessages, *claudeMediaMessage)
						}
					default:
						source := mediaMessage.ToFileSource()
						if source == nil {
							continue
						}
						base64Data, mimeType, err := service.GetBase64Data(c, source, "formatting image for Claude")
						if err != nil {
							return nil, fmt.Errorf("get file data failed: %s", err.Error())
						}
						claudeMediaMessage := dto.ClaudeMediaMessage{
							Source: &dto.ClaudeMessageSource{
								Type: "base64",
							},
						}
						if strings.HasPrefix(mimeType, "application/pdf") {
							claudeMediaMessage.Type = "document"
						} else if strings.HasPrefix(mimeType, "image/") {
							claudeMediaMessage.Type = "image"
						} else {
							continue
						}

						claudeMediaMessage.Source.MediaType = mimeType
						claudeMediaMessage.Source.Data = base64Data
						claudeMediaMessages = append(claudeMediaMessages, claudeMediaMessage)
						continue
					}
				}

				if message.ToolCalls != nil {
					for _, toolCall := range message.ParseToolCalls() {
						inputObj := make(map[string]any)
						if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &inputObj); err != nil {
							common.SysLog("tool call function arguments is not a map[string]any: " + fmt.Sprintf("%v", toolCall.Function.Arguments))
							continue
						}
						claudeMediaMessages = append(claudeMediaMessages, dto.ClaudeMediaMessage{
							Type:  "tool_use",
							Id:    toolCall.ID,
							Name:  toolCall.Function.Name,
							Input: inputObj,
						})
					}
				}
				claudeMessage.Content = claudeMediaMessages
			}
			claudeMessages = append(claudeMessages, claudeMessage)
		}
	}

	// 设置累积的system消息
	if len(systemMessages) > 0 {
		claudeRequest.System = systemMessages
	}

	claudeRequest.Prompt = ""
	claudeRequest.Messages = claudeMessages
	return &claudeRequest, nil
}

// StreamResponseClaude2OpenAI 将 Claude 的流式 SSE 响应事件转换为 OpenAI 的 chat.completion.chunk 格式。
//
// Claude 的流式响应包含以下事件类型，分别映射为：
//   - "message_start"：消息开始事件，提取 response id、model，初始化空 delta。
//   - "content_block_start"：内容块开始事件，处理 text 块的首段文本和 tool_use 块的工具调用初始化。
//   - "content_block_delta"：内容块增量事件，处理以下子类型：
//   - text：文本增量
//   - input_json_delta：工具调用参数的 JSON 增量
//   - thinking_delta：思维链增量（映射为 reasoning_content）
//   - signature_delta：签名增量（加密内容，输出换行占位）
//   - "message_delta"：消息结束事件，提取 stop_reason 并转换为 OpenAI 的 finish_reason。
//   - "message_stop"：消息停止事件，返回 nil（由外部处理结束）。
//
// 工具调用（tool_use）通过 fcIdx 计算索引，兼容多工具并行调用场景。
// 当存在工具调用时，清除 delta.content 以兼容 LobeOpenAI 等衍生应用。
func StreamResponseClaude2OpenAI(claudeResponse *dto.ClaudeResponse) *dto.ChatCompletionsStreamResponse {
	var response dto.ChatCompletionsStreamResponse
	response.Object = "chat.completion.chunk"
	response.Model = claudeResponse.Model
	response.Choices = make([]dto.ChatCompletionsStreamResponseChoice, 0)
	tools := make([]dto.ToolCallResponse, 0)
	fcIdx := 0
	if claudeResponse.Index != nil {
		fcIdx = *claudeResponse.Index - 1
		if fcIdx < 0 {
			fcIdx = 0
		}
	}
	var choice dto.ChatCompletionsStreamResponseChoice
	if claudeResponse.Type == "message_start" {
		if claudeResponse.Message != nil {
			response.Id = claudeResponse.Message.Id
			response.Model = claudeResponse.Message.Model
		}
		//claudeUsage = &claudeResponse.Message.Usage
		choice.Delta.SetContentString("")
		choice.Delta.Role = "assistant"
	} else if claudeResponse.Type == "content_block_start" {
		if claudeResponse.ContentBlock != nil {
			// 如果是文本块，尽可能发送首段文本（若存在）
			if claudeResponse.ContentBlock.Type == "text" && claudeResponse.ContentBlock.Text != nil {
				choice.Delta.SetContentString(*claudeResponse.ContentBlock.Text)
			}
			if claudeResponse.ContentBlock.Type == "tool_use" {
				tools = append(tools, dto.ToolCallResponse{
					Index: common.GetPointer(fcIdx),
					ID:    claudeResponse.ContentBlock.Id,
					Type:  "function",
					Function: dto.FunctionResponse{
						Name:      claudeResponse.ContentBlock.Name,
						Arguments: "",
					},
				})
			}
		} else {
			return nil
		}
	} else if claudeResponse.Type == "content_block_delta" {
		if claudeResponse.Delta != nil {
			choice.Delta.Content = claudeResponse.Delta.Text
			switch claudeResponse.Delta.Type {
			case "input_json_delta":
				tools = append(tools, dto.ToolCallResponse{
					Type:  "function",
					Index: common.GetPointer(fcIdx),
					Function: dto.FunctionResponse{
						Arguments: *claudeResponse.Delta.PartialJson,
					},
				})
			case "signature_delta":
				// 加密的不处理
				signatureContent := "\n"
				choice.Delta.ReasoningContent = &signatureContent
			case "thinking_delta":
				choice.Delta.ReasoningContent = claudeResponse.Delta.Thinking
			}
		}
	} else if claudeResponse.Type == "message_delta" {
		if claudeResponse.Delta != nil && claudeResponse.Delta.StopReason != nil {
			finishReason := stopReasonClaude2OpenAI(*claudeResponse.Delta.StopReason)
			if finishReason != "null" {
				choice.FinishReason = &finishReason
			}
		}
		//claudeUsage = &claudeResponse.Usage
	} else if claudeResponse.Type == "message_stop" {
		return nil
	} else {
		return nil
	}
	if len(tools) > 0 {
		choice.Delta.Content = nil // compatible with other OpenAI derivative applications, like LobeOpenAICompatibleFactory ...
		choice.Delta.ToolCalls = tools
	}
	response.Choices = append(response.Choices, choice)

	return &response
}

// ResponseClaude2OpenAI 将 Claude 的非流式（一次性）完整响应转换为 OpenAI 的 chat.completion 格式。
//
// 处理逻辑：
//   - 遍历 claudeResponse.Content 中的所有内容块，按类型分别处理：
//   - "tool_use"：提取工具调用的 id、name 和 input 参数（JSON 序列化为字符串）。
//   - "thinking"：提取思维链明文内容，映射为 reasoning_content 字段。
//   - "text"：提取最终响应文本。
//   - 如果 content[0] 包含 Thinking 字段，单独提取为 choice 级别的 reasoning_content。
//   - 将 Claude 的 stop_reason 通过 stopReasonClaude2OpenAI 转换为 finish_reason。
//   - 生成标准的 OpenAI 响应结构，包括 id（使用 Claude 原始 id）、object、created、model、choices。
func ResponseClaude2OpenAI(claudeResponse *dto.ClaudeResponse) *dto.OpenAITextResponse {
	choices := make([]dto.OpenAITextResponseChoice, 0)
	fullTextResponse := dto.OpenAITextResponse{
		Id:      fmt.Sprintf("chatcmpl-%s", common.GetUUID()),
		Object:  "chat.completion",
		Created: common.GetTimestamp(),
	}
	var responseText string
	var responseThinking string
	if len(claudeResponse.Content) > 0 {
		responseText = claudeResponse.Content[0].GetText()
		if claudeResponse.Content[0].Thinking != nil {
			responseThinking = *claudeResponse.Content[0].Thinking
		}
	}
	tools := make([]dto.ToolCallResponse, 0)
	thinkingContent := ""

	fullTextResponse.Id = claudeResponse.Id
	for _, message := range claudeResponse.Content {
		switch message.Type {
		case "tool_use":
			args, _ := json.Marshal(message.Input)
			tools = append(tools, dto.ToolCallResponse{
				ID:   message.Id,
				Type: "function", // compatible with other OpenAI derivative applications
				Function: dto.FunctionResponse{
					Name:      message.Name,
					Arguments: string(args),
				},
			})
		case "thinking":
			// 加密的不管， 只输出明文的推理过程
			if message.Thinking != nil {
				thinkingContent = *message.Thinking
			}
		case "text":
			responseText = message.GetText()
		}
	}
	choice := dto.OpenAITextResponseChoice{
		Index: 0,
		Message: dto.Message{
			Role: "assistant",
		},
		FinishReason: stopReasonClaude2OpenAI(claudeResponse.StopReason),
	}
	choice.SetStringContent(responseText)
	if len(responseThinking) > 0 {
		choice.ReasoningContent = &responseThinking
	}
	if len(tools) > 0 {
		choice.Message.SetToolCalls(tools)
	}
	if thinkingContent != "" {
		choice.Message.ReasoningContent = &thinkingContent
	}
	fullTextResponse.Model = claudeResponse.Model
	choices = append(choices, choice)
	fullTextResponse.Choices = choices
	return &fullTextResponse
}

// ClaudeResponseInfo 用于在 Claude 流式响应处理过程中累积和追踪响应信息。
//
// 在流式传输过程中，Claude 的响应被拆分为多个 SSE 事件，该结构体作为上下文状态，
// 逐步收集各事件中的数据，最终形成完整的 usage 统计和响应文本。
//
// 字段说明：
//   - ResponseId: 响应唯一标识，来自 message_start 事件中的 message.id。
//   - Created: 响应创建时间戳（Unix 秒），在 ClaudeStreamHandler 中初始化。
//   - Model: 实际使用的模型名称，来自 message_start 事件中的 message.model。
//   - ResponseText: 累积的响应文本（包含 thinking 和 text 内容），用于最终的 usage 估算兜底。
//   - Usage: 累积的 token 用量统计，包含 prompt_tokens、completion_tokens、缓存相关字段等。
//   - Done: 标记响应是否已完成（收到 message_delta 事件时置为 true）。
type ClaudeResponseInfo struct {
	ResponseId   string
	Created      int64
	Model        string
	ResponseText strings.Builder
	Usage        *dto.Usage
	Done         bool
}

// cacheCreationTokensForOpenAIUsage 计算用于 OpenAI 风格 usage 显示的缓存创建 token 总数。
//
// 缓存创建 token 有两种来源：
//   - 分拆缓存（split cache）：ClaudeCacheCreation5mTokens + ClaudeCacheCreation1hTokens，
//     分别代表 5 分钟和 1 小时过期的缓存创建 token 数。
//   - 聚合缓存（aggregate cache）：PromptTokensDetails.CachedCreationTokens，
//     是上游返回的缓存创建 token 总数。
//
// 优先使用分拆缓存之和；如果分拆缓存为 0，则回退到聚合缓存。
// 如果聚合缓存大于分拆缓存之和（可能包含其他类型的缓存创建 token），则使用聚合缓存。
func cacheCreationTokensForOpenAIUsage(usage *dto.Usage) int {
	if usage == nil {
		return 0
	}
	splitCacheCreationTokens := usage.ClaudeCacheCreation5mTokens + usage.ClaudeCacheCreation1hTokens
	if splitCacheCreationTokens == 0 {
		return usage.PromptTokensDetails.CachedCreationTokens
	}
	if usage.PromptTokensDetails.CachedCreationTokens > splitCacheCreationTokens {
		return usage.PromptTokensDetails.CachedCreationTokens
	}
	return splitCacheCreationTokens
}

// buildOpenAIStyleUsageFromClaudeUsage 将 Claude 的 usage 统计转换为 OpenAI 风格的 usage 格式。
//
// 转换逻辑：
//   - 归一化缓存创建 token 的分拆（5m + 1h）与聚合值之间的关系。
//   - 计算总输入 token = prompt_tokens + cached_tokens + cache_creation_tokens。
//   - 同时设置 PromptTokens 和 InputTokens（OpenAI 新旧两种字段名）。
//   - 计算 TotalTokens = totalInputTokens + completionTokens。
//   - 设置 UsageSemantic = "openai"、UsageSource = "anthropic"，用于下游标识用量来源。
//
// 返回值为 usage 的浅拷贝，不会修改原始 usage 对象。
func buildOpenAIStyleUsageFromClaudeUsage(usage *dto.Usage) dto.Usage {
	if usage == nil {
		return dto.Usage{}
	}
	clone := *usage
	clone.ClaudeCacheCreation5mTokens, clone.ClaudeCacheCreation1hTokens = service.NormalizeCacheCreationSplit(
		usage.PromptTokensDetails.CachedCreationTokens,
		usage.ClaudeCacheCreation5mTokens,
		usage.ClaudeCacheCreation1hTokens,
	)
	cacheCreationTokens := cacheCreationTokensForOpenAIUsage(usage)
	totalInputTokens := usage.PromptTokens + usage.PromptTokensDetails.CachedTokens + cacheCreationTokens
	clone.PromptTokens = totalInputTokens
	clone.InputTokens = totalInputTokens
	clone.TotalTokens = totalInputTokens + usage.CompletionTokens
	clone.UsageSemantic = "openai"
	clone.UsageSource = "anthropic"
	return clone
}

// buildMessageDeltaPatchUsage 构建用于修补 message_delta 事件 usage 字段的 ClaudeUsage 对象。
//
// 背景：某些上游（如 AWS Bedrock）的 message_delta 事件中 usage 可能缺少 input_tokens、
// cache_read_input_tokens、cache_creation_input_tokens 等字段，但这些字段在 message_start
// 事件中已经返回过。此函数将 message_start 已获取的 usage 数据与 message_delta 的 usage 合并，
// 生成完整的 usage 数据，用于后续的 JSON 补丁操作。
//
// 合并规则：
//   - 如果 message_delta 的 InputTokens 为 0 但 claudeInfo 中有 PromptTokens，则使用 claudeInfo 的值。
//   - cache_read_input_tokens 和 cache_creation_input_tokens 同理。
//   - 缓存创建的 5m/1h 分拆值进行归一化处理。
func buildMessageDeltaPatchUsage(claudeResponse *dto.ClaudeResponse, claudeInfo *ClaudeResponseInfo) *dto.ClaudeUsage {
	usage := &dto.ClaudeUsage{}
	if claudeResponse != nil && claudeResponse.Usage != nil {
		*usage = *claudeResponse.Usage
	}

	if claudeInfo == nil || claudeInfo.Usage == nil {
		return usage
	}

	if usage.InputTokens == 0 && claudeInfo.Usage.PromptTokens > 0 {
		usage.InputTokens = claudeInfo.Usage.PromptTokens
	}
	if usage.CacheReadInputTokens == 0 && claudeInfo.Usage.PromptTokensDetails.CachedTokens > 0 {
		usage.CacheReadInputTokens = claudeInfo.Usage.PromptTokensDetails.CachedTokens
	}
	if usage.CacheCreationInputTokens == 0 && claudeInfo.Usage.PromptTokensDetails.CachedCreationTokens > 0 {
		usage.CacheCreationInputTokens = claudeInfo.Usage.PromptTokensDetails.CachedCreationTokens
	}
	cacheCreation5m := 0
	cacheCreation1h := 0
	if usage.CacheCreation != nil {
		cacheCreation5m = usage.CacheCreation.Ephemeral5mInputTokens
		cacheCreation1h = usage.CacheCreation.Ephemeral1hInputTokens
	} else {
		cacheCreation5m = claudeInfo.Usage.ClaudeCacheCreation5mTokens
		cacheCreation1h = claudeInfo.Usage.ClaudeCacheCreation1hTokens
	}
	cacheCreation5m, cacheCreation1h = service.NormalizeCacheCreationSplit(
		usage.CacheCreationInputTokens,
		cacheCreation5m,
		cacheCreation1h,
	)
	if usage.CacheCreation == nil && (cacheCreation5m > 0 || cacheCreation1h > 0) {
		usage.CacheCreation = &dto.ClaudeCacheCreationUsage{}
	}
	if usage.CacheCreation != nil {
		usage.CacheCreation.Ephemeral5mInputTokens = cacheCreation5m
		usage.CacheCreation.Ephemeral1hInputTokens = cacheCreation1h
	}
	return usage
}

// shouldSkipClaudeMessageDeltaUsagePatch 判断是否应该跳过对 message_delta usage 的补丁操作。
//
// 当全局的 PassThroughRequestEnabled 启用（透传模式）或渠道级别的 PassThroughBodyEnabled 启用时，
// 上游响应体将被原样透传给客户端，因此不需要也不应修改 usage 数据。
//
// 参数：
//   - info: 中继信息，包含渠道级别的配置
//
// 返回值：
//   - true: 跳过补丁（透传模式）
//   - false: 需要执行补丁
func shouldSkipClaudeMessageDeltaUsagePatch(info *relaycommon.RelayInfo) bool {
	if model_setting.GetGlobalSettings().PassThroughRequestEnabled {
		return true
	}
	if info == nil {
		return false
	}
	return info.ChannelSetting.PassThroughBodyEnabled
}

// patchClaudeMessageDeltaUsageData 使用 sjson 对 message_delta 事件的原始 JSON 字符串进行补丁操作，
// 填充上游缺失的 usage 字段。
//
// 补丁的字段包括：
//   - usage.input_tokens
//   - usage.cache_read_input_tokens
//   - usage.cache_creation_input_tokens
//   - usage.cache_creation.ephemeral_5m_input_tokens
//   - usage.cache_creation.ephemeral_1h_input_tokens
//
// 仅在上游未返回该字段或值为 0 时才进行填充，避免覆盖上游的有效数据。
// 使用 sjson/gjson 库直接操作 JSON 字符串，避免完整的反序列化/序列化开销。
func patchClaudeMessageDeltaUsageData(data string, usage *dto.ClaudeUsage) string {
	if data == "" || usage == nil {
		return data
	}

	data = setMessageDeltaUsageInt(data, "usage.input_tokens", usage.InputTokens)
	data = setMessageDeltaUsageInt(data, "usage.cache_read_input_tokens", usage.CacheReadInputTokens)
	data = setMessageDeltaUsageInt(data, "usage.cache_creation_input_tokens", usage.CacheCreationInputTokens)

	if usage.CacheCreation != nil {
		data = setMessageDeltaUsageInt(data, "usage.cache_creation.ephemeral_5m_input_tokens", usage.CacheCreation.Ephemeral5mInputTokens)
		data = setMessageDeltaUsageInt(data, "usage.cache_creation.ephemeral_1h_input_tokens", usage.CacheCreation.Ephemeral1hInputTokens)
	}

	return data
}

// setMessageDeltaUsageInt 是 patchClaudeMessageDeltaUsageData 的辅助函数，
// 用于在 JSON 字符串中设置单个整数值。
//
// 仅在以下条件同时满足时才写入：
//   - localValue > 0（本地值有效）
//   - 上游字段不存在或上游值为 0（不覆盖已有值）
//
// 使用 gjson.Get 读取上游值，sjson.Set 写入补丁值。
// 任何错误均静默处理（返回原始 data），确保不中断流式传输。
func setMessageDeltaUsageInt(data string, path string, localValue int) string {
	if localValue <= 0 {
		return data
	}

	upstreamValue := gjson.Get(data, path)
	if upstreamValue.Exists() && upstreamValue.Int() > 0 {
		return data
	}

	patchedData, err := sjson.Set(data, path, localValue)
	if err != nil {
		return data
	}
	return patchedData
}

// FormatClaudeResponseInfo 在 Claude 流式传输过程中逐步累积响应信息到 claudeInfo。
//
// 该函数是流式 usage 追踪的核心，根据 Claude SSE 事件类型分别处理：
//
//   - "message_start"：消息开始，提取 response id、model、以及初始 usage（包含 input_tokens、
//     cache_read_input_tokens、cache_creation_input_tokens 和分拆的 5m/1h 缓存 token）。
//
//   - "content_block_delta"：内容块增量，将 text 和 thinking 内容追加到 ResponseText 中，
//     用于后续的 usage 估算兜底。
//
//   - "message_delta"：消息结束事件，提取最终 usage（仅覆盖非零值，避免丢失 message_start 的数据），
//     计算 TotalTokens，并将 Done 标记为 true。
//
//   - "content_block_start"：内容块开始事件，仅用于同步 oaiResponse 的元数据，不累积 usage。
//
// 参数：
//   - claudeResponse: 当前 SSE 事件的解析结果
//   - oaiResponse: OpenAI 格式的流式响应（可选），用于同步 id/created/model 等元数据
//   - claudeInfo: 累积的状态结构体（不可为 nil）
//
// 返回值：
//   - true: 事件被成功处理
//   - false: claudeInfo 为 nil 或事件类型不被识别
func FormatClaudeResponseInfo(claudeResponse *dto.ClaudeResponse, oaiResponse *dto.ChatCompletionsStreamResponse, claudeInfo *ClaudeResponseInfo) bool {
	if claudeInfo == nil {
		return false
	}
	if claudeInfo.Usage == nil {
		claudeInfo.Usage = &dto.Usage{}
	}
	if claudeResponse.Type == "message_start" {
		if claudeResponse.Message != nil {
			claudeInfo.ResponseId = claudeResponse.Message.Id
			claudeInfo.Model = claudeResponse.Message.Model
		}

		// message_start, 获取usage
		if claudeResponse.Message != nil && claudeResponse.Message.Usage != nil {
			claudeInfo.Usage.PromptTokens = claudeResponse.Message.Usage.InputTokens
			claudeInfo.Usage.UsageSemantic = "anthropic"
			claudeInfo.Usage.PromptTokensDetails.CachedTokens = claudeResponse.Message.Usage.CacheReadInputTokens
			claudeInfo.Usage.PromptTokensDetails.CachedCreationTokens = claudeResponse.Message.Usage.CacheCreationInputTokens
			claudeInfo.Usage.ClaudeCacheCreation5mTokens = claudeResponse.Message.Usage.GetCacheCreation5mTokens()
			claudeInfo.Usage.ClaudeCacheCreation1hTokens = claudeResponse.Message.Usage.GetCacheCreation1hTokens()
			claudeInfo.Usage.CompletionTokens = claudeResponse.Message.Usage.OutputTokens
		}
	} else if claudeResponse.Type == "content_block_delta" {
		if claudeResponse.Delta != nil {
			if claudeResponse.Delta.Text != nil {
				claudeInfo.ResponseText.WriteString(*claudeResponse.Delta.Text)
			}
			if claudeResponse.Delta.Thinking != nil {
				claudeInfo.ResponseText.WriteString(*claudeResponse.Delta.Thinking)
			}
		}
	} else if claudeResponse.Type == "message_delta" {
		// 最终的usage获取
		if claudeResponse.Usage != nil {
			claudeInfo.Usage.UsageSemantic = "anthropic"
			if claudeResponse.Usage.InputTokens > 0 {
				// 不叠加，只取最新的
				claudeInfo.Usage.PromptTokens = claudeResponse.Usage.InputTokens
			}
			if claudeResponse.Usage.CacheReadInputTokens > 0 {
				claudeInfo.Usage.PromptTokensDetails.CachedTokens = claudeResponse.Usage.CacheReadInputTokens
			}
			if claudeResponse.Usage.CacheCreationInputTokens > 0 {
				claudeInfo.Usage.PromptTokensDetails.CachedCreationTokens = claudeResponse.Usage.CacheCreationInputTokens
			}
			if cacheCreation5m := claudeResponse.Usage.GetCacheCreation5mTokens(); cacheCreation5m > 0 {
				claudeInfo.Usage.ClaudeCacheCreation5mTokens = cacheCreation5m
			}
			if cacheCreation1h := claudeResponse.Usage.GetCacheCreation1hTokens(); cacheCreation1h > 0 {
				claudeInfo.Usage.ClaudeCacheCreation1hTokens = cacheCreation1h
			}
			if claudeResponse.Usage.OutputTokens > 0 {
				claudeInfo.Usage.CompletionTokens = claudeResponse.Usage.OutputTokens
			}
			claudeInfo.Usage.TotalTokens = claudeInfo.Usage.PromptTokens + claudeInfo.Usage.CompletionTokens
		}

		// 判断是否完整
		claudeInfo.Done = true
	} else if claudeResponse.Type == "content_block_start" {
	} else {
		return false
	}
	if oaiResponse != nil {
		oaiResponse.Id = claudeInfo.ResponseId
		oaiResponse.Created = claudeInfo.Created
		oaiResponse.Model = claudeInfo.Model
	}
	return true
}

// HandleStreamResponseData 处理单条 Claude 流式 SSE 数据，并根据中继格式转发给客户端。
//
// 处理流程：
//  1. 将原始 JSON 字符串解析为 dto.ClaudeResponse。
//  2. 检查错误响应：如果上游返回了 Claude 错误（error 类型），直接返回对应的 NexusTokError。
//  3. 检查 refusal 拒绝：如果 stop_reason 为 "refusal"，在上下文中标记拒绝原因。
//  4. 根据中继格式分别处理：
//     - RelayFormatClaude：直接透传 Claude 原始格式，但对 message_delta 的 usage 进行补丁
//     （解决 Bedrock 等上游缺失字段的问题），然后通过 helper.ClaudeChunkData 发送。
//     - RelayFormatOpenAI：调用 StreamResponseClaude2OpenAI 转换为 OpenAI 格式，
//     通过 FormatClaudeResponseInfo 累积 usage，最后通过 helper.ObjectData 发送。
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继信息（包含中继格式、渠道设置等）
//   - claudeInfo: 流式 usage 累积状态
//   - data: 原始 JSON 字符串
//
// 返回值：
//   - *types.NexusTokError: 处理错误，成功则为 nil
func HandleStreamResponseData(c *gin.Context, info *relaycommon.RelayInfo, claudeInfo *ClaudeResponseInfo, data string) *types.NexusTokError {
	var claudeResponse dto.ClaudeResponse
	err := common.UnmarshalJsonStr(data, &claudeResponse)
	if err != nil {
		common.SysLog("error unmarshalling stream response: " + err.Error())
		return types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	if claudeError := claudeResponse.GetClaudeError(); claudeError != nil && claudeError.Type != "" {
		return types.WithClaudeError(*claudeError, http.StatusInternalServerError)
	}
	if claudeResponse.StopReason != "" {
		maybeMarkClaudeRefusal(c, claudeResponse.StopReason)
	}
	if claudeResponse.Delta != nil && claudeResponse.Delta.StopReason != nil {
		maybeMarkClaudeRefusal(c, *claudeResponse.Delta.StopReason)
	}
	if info.RelayFormat == types.RelayFormatClaude {
		FormatClaudeResponseInfo(&claudeResponse, nil, claudeInfo)

		if claudeResponse.Type == "message_start" {
			// message_start, 获取usage
			if claudeResponse.Message != nil {
				info.UpstreamModelName = claudeResponse.Message.Model
			}
		} else if claudeResponse.Type == "message_delta" {
			// 确保 message_delta 的 usage 包含完整的 input_tokens 和 cache 相关字段
			// 解决 AWS Bedrock 等上游返回的 message_delta 缺少这些字段的问题
			if !shouldSkipClaudeMessageDeltaUsagePatch(info) {
				data = patchClaudeMessageDeltaUsageData(data, buildMessageDeltaPatchUsage(&claudeResponse, claudeInfo))
			}
		}
		helper.ClaudeChunkData(c, claudeResponse, data)
	} else if info.RelayFormat == types.RelayFormatOpenAI {
		response := StreamResponseClaude2OpenAI(&claudeResponse)

		if !FormatClaudeResponseInfo(&claudeResponse, response, claudeInfo) {
			return nil
		}

		err = helper.ObjectData(c, response)
		if err != nil {
			logger.LogError(c, "send_stream_response_failed: "+err.Error())
		}
	}
	return nil
}

// HandleStreamFinalResponse 在 Claude 流式传输结束后处理最终的 usage 统计和结束标记。
//
// 处理逻辑：
//  1. 检查 usage 完整性：如果 completion_tokens 为 0 或 Done 为 false，
//     说明上游可能未正确返回 usage（某些上游场景下常见）。
//     此时通过 service.ResponseText2Usage 基于累积的响应文本进行 token 估算作为兜底，
//     仅补充缺失字段，不覆盖 message_start 已获取的 cache 字段。
//  2. 设置 UsageSemantic = "anthropic" 标识用量来源。
//  3. 根据中继格式分别处理：
//     - RelayFormatClaude：无需额外操作（流式数据已在 HandleStreamResponseData 中逐块发送）。
//     - RelayFormatOpenAI：如果需要包含 usage（ShouldIncludeUsage），将 Claude usage 转换为
//     OpenAI 格式并发送最终的 usage chunk，然后发送 [DONE] 结束标记。
func HandleStreamFinalResponse(c *gin.Context, info *relaycommon.RelayInfo, claudeInfo *ClaudeResponseInfo) {
	if claudeInfo.Usage.PromptTokens == 0 {
		//上游出错
	}
	if claudeInfo.Usage.CompletionTokens == 0 || !claudeInfo.Done {
		if common.DebugEnabled {
			common.SysLog("claude response usage is not complete, maybe upstream error")
		}
		// 只补缺失字段，不整份覆盖——保留 message_start 已拿到的 cache 字段
		fallback := service.ResponseText2Usage(c, claudeInfo.ResponseText.String(), info.UpstreamModelName, info.GetEstimatePromptTokens())
		if claudeInfo.Usage.CompletionTokens == 0 ||
			(!claudeInfo.Done && fallback.CompletionTokens > claudeInfo.Usage.CompletionTokens) {
			claudeInfo.Usage.CompletionTokens = fallback.CompletionTokens
		}
		if claudeInfo.Usage.PromptTokens == 0 {
			claudeInfo.Usage.PromptTokens = fallback.PromptTokens
		}
		claudeInfo.Usage.TotalTokens = claudeInfo.Usage.PromptTokens + claudeInfo.Usage.CompletionTokens
	}
	if claudeInfo.Usage != nil {
		claudeInfo.Usage.UsageSemantic = "anthropic"
	}

	if info.RelayFormat == types.RelayFormatClaude {
		//
	} else if info.RelayFormat == types.RelayFormatOpenAI {
		if info.ShouldIncludeUsage {
			openAIUsage := buildOpenAIStyleUsageFromClaudeUsage(claudeInfo.Usage)
			response := helper.GenerateFinalUsageResponse(claudeInfo.ResponseId, claudeInfo.Created, info.UpstreamModelName, openAIUsage)
			err := helper.ObjectData(c, response)
			if err != nil {
				common.SysLog("send final response failed: " + err.Error())
			}
		}
		helper.Done(c)
	}
}

// ClaudeStreamHandler 是 Claude 流式响应处理的顶层入口函数。
//
// 完整的流式处理流程：
//  1. 初始化 ClaudeResponseInfo，填充 response id、created timestamp、model 等元数据。
//  2. 使用 helper.StreamScannerHandler 逐行读取 SSE 数据流。
//  3. 对每条 SSE 数据调用 HandleStreamResponseData 进行解析、转换和转发。
//  4. 如果处理过程中出错，通过 sr.Stop(err) 停止扫描。
//  5. 流式传输结束后调用 HandleStreamFinalResponse 处理最终 usage 和结束标记。
//
// 参数：
//   - c: Gin 上下文
//   - resp: 上游 Claude API 的 HTTP 响应
//   - info: 中继信息
//
// 返回值：
//   - *dto.Usage: 最终的 token 用量统计
//   - *types.NexusTokError: 处理错误，成功则为 nil
func ClaudeStreamHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NexusTokError) {
	claudeInfo := &ClaudeResponseInfo{
		ResponseId:   helper.GetResponseID(c),
		Created:      common.GetTimestamp(),
		Model:        info.UpstreamModelName,
		ResponseText: strings.Builder{},
		Usage:        &dto.Usage{},
	}
	var err *types.NexusTokError
	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		err = HandleStreamResponseData(c, info, claudeInfo, data)
		if err != nil {
			sr.Stop(err)
		}
	})
	if err != nil {
		return nil, err
	}

	HandleStreamFinalResponse(c, info, claudeInfo)
	return claudeInfo.Usage, nil
}

// HandleClaudeResponseData 处理 Claude 的非流式（一次性）完整响应数据。
//
// 处理流程：
//  1. 将原始字节数据解析为 dto.ClaudeResponse。
//  2. 检查错误响应和 refusal 拒绝标记。
//  3. 提取 usage 统计：input_tokens、output_tokens、cache 相关字段、分拆的 5m/1h 缓存 token。
//  4. 根据中继格式分别处理：
//     - RelayFormatOpenAI：调用 ResponseClaude2OpenAI 转换为 OpenAI 格式，
//     附带转换后的 usage，然后 JSON 序列化。
//     - RelayFormatClaude：直接使用原始响应数据。
//  5. 特殊处理：如果响应中包含 web_search_requests（服务器端工具用量），
//     设置到 gin.Context 中用于计费。
//  6. 通过 service.IOCopyBytesGracefully 将响应数据写入客户端。
func HandleClaudeResponseData(c *gin.Context, info *relaycommon.RelayInfo, claudeInfo *ClaudeResponseInfo, httpResp *http.Response, data []byte) *types.NexusTokError {
	var claudeResponse dto.ClaudeResponse
	err := common.Unmarshal(data, &claudeResponse)
	if err != nil {
		return types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	if claudeError := claudeResponse.GetClaudeError(); claudeError != nil && claudeError.Type != "" {
		return types.WithClaudeError(*claudeError, http.StatusInternalServerError)
	}
	maybeMarkClaudeRefusal(c, claudeResponse.StopReason)
	if claudeInfo.Usage == nil {
		claudeInfo.Usage = &dto.Usage{}
	}
	if claudeResponse.Usage != nil {
		claudeInfo.Usage.PromptTokens = claudeResponse.Usage.InputTokens
		claudeInfo.Usage.CompletionTokens = claudeResponse.Usage.OutputTokens
		claudeInfo.Usage.TotalTokens = claudeResponse.Usage.InputTokens + claudeResponse.Usage.OutputTokens
		claudeInfo.Usage.UsageSemantic = "anthropic"
		claudeInfo.Usage.PromptTokensDetails.CachedTokens = claudeResponse.Usage.CacheReadInputTokens
		claudeInfo.Usage.PromptTokensDetails.CachedCreationTokens = claudeResponse.Usage.CacheCreationInputTokens
		claudeInfo.Usage.ClaudeCacheCreation5mTokens = claudeResponse.Usage.GetCacheCreation5mTokens()
		claudeInfo.Usage.ClaudeCacheCreation1hTokens = claudeResponse.Usage.GetCacheCreation1hTokens()
	}
	var responseData []byte
	switch info.RelayFormat {
	case types.RelayFormatOpenAI:
		openaiResponse := ResponseClaude2OpenAI(&claudeResponse)
		openaiResponse.Usage = buildOpenAIStyleUsageFromClaudeUsage(claudeInfo.Usage)
		responseData, err = json.Marshal(openaiResponse)
		if err != nil {
			return types.NewError(err, types.ErrorCodeBadResponseBody)
		}
	case types.RelayFormatClaude:
		responseData = data
	}

	if claudeResponse.Usage != nil && claudeResponse.Usage.ServerToolUse != nil && claudeResponse.Usage.ServerToolUse.WebSearchRequests > 0 {
		c.Set("claude_web_search_requests", claudeResponse.Usage.ServerToolUse.WebSearchRequests)
	}

	service.IOCopyBytesGracefully(c, httpResp, responseData)
	return nil
}

// ClaudeHandler 是 Claude 非流式响应处理的顶层入口函数。
//
// 处理流程：
//  1. 延迟关闭上游响应体（defer service.CloseResponseBodyGracefully）。
//  2. 初始化 ClaudeResponseInfo。
//  3. 使用 io.ReadAll 一次性读取整个响应体。
//  4. 调试模式下打印响应体内容。
//  5. 委托给 HandleClaudeResponseData 进行解析、转换和响应。
//
// 参数：
//   - c: Gin 上下文
//   - resp: 上游 Claude API 的 HTTP 响应
//   - info: 中继信息
//
// 返回值：
//   - *dto.Usage: 最终的 token 用量统计
//   - *types.NexusTokError: 处理错误，成功则为 nil
func ClaudeHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NexusTokError) {
	defer service.CloseResponseBodyGracefully(resp)

	claudeInfo := &ClaudeResponseInfo{
		ResponseId:   helper.GetResponseID(c),
		Created:      common.GetTimestamp(),
		Model:        info.UpstreamModelName,
		ResponseText: strings.Builder{},
		Usage:        &dto.Usage{},
	}
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	if common.DebugEnabled {
		println("responseBody: ", string(responseBody))
	}
	handleErr := HandleClaudeResponseData(c, info, claudeInfo, resp, responseBody)
	if handleErr != nil {
		return nil, handleErr
	}
	return claudeInfo.Usage, nil
}

// mapToolChoice 将 OpenAI 格式的 tool_choice 参数映射为 Claude 格式的 ClaudeToolChoice。
//
// OpenAI 与 Claude 的 tool_choice 映射关系：
//   - "auto"      -> type: "auto"        （模型自动决定是否调用工具）
//   - "required"  -> type: "any"          （强制模型至少调用一个工具）
//   - "none"      -> type: "none"         （禁止调用任何工具）
//   - {"function":{"name":"xxx"}} -> type: "tool", name: "xxx"（强制调用指定工具）
//
// 并行工具调用（parallel_tool_calls）处理：
//   - 如果 tool_choice 为 nil 但 parallelToolCalls 有值，创建默认 auto 类型。
//   - 将 OpenAI 的 parallel_tool_calls（true=允许并行）反转为 Claude 的
//     disable_parallel_tool_use（true=禁止并行）。
//   - 特殊情况：当 tool_choice.type 为 "none" 时，不设置 disable_parallel_tool_use，
//     因为 Anthropic schema 不允许 none 类型携带额外字段。
func mapToolChoice(toolChoice any, parallelToolCalls *bool) *dto.ClaudeToolChoice {
	var claudeToolChoice *dto.ClaudeToolChoice

	// 处理 tool_choice 字符串值
	if toolChoiceStr, ok := toolChoice.(string); ok {
		switch toolChoiceStr {
		case "auto":
			claudeToolChoice = &dto.ClaudeToolChoice{
				Type: "auto",
			}
		case "required":
			claudeToolChoice = &dto.ClaudeToolChoice{
				Type: "any",
			}
		case "none":
			claudeToolChoice = &dto.ClaudeToolChoice{
				Type: "none",
			}
		}
	} else if toolChoiceMap, ok := toolChoice.(map[string]interface{}); ok {
		// 处理 tool_choice 对象值
		if function, ok := toolChoiceMap["function"].(map[string]interface{}); ok {
			if toolName, ok := function["name"].(string); ok {
				claudeToolChoice = &dto.ClaudeToolChoice{
					Type: "tool",
					Name: toolName,
				}
			}
		}
	}

	// 处理 parallel_tool_calls
	if parallelToolCalls != nil {
		if claudeToolChoice == nil {
			// 如果没有 tool_choice，但有 parallel_tool_calls，创建默认的 auto 类型
			claudeToolChoice = &dto.ClaudeToolChoice{
				Type: "auto",
			}
		}

		// Anthropic schema: tool_choice.type=none does not accept extra fields.
		// When tools are disabled, parallel_tool_calls is irrelevant, so we drop it.
		if claudeToolChoice.Type != "none" {
			// 如果 parallel_tool_calls 为 true，则 disable_parallel_tool_use 为 false
			claudeToolChoice.DisableParallelToolUse = !*parallelToolCalls
		}
	}

	return claudeToolChoice
}
