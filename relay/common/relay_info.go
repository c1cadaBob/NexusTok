// Package common - relay_info.go
// 本文件定义了中继处理的核心数据结构 RelayInfo 及其相关类型和工厂函数。
// RelayInfo 是整个请求中继过程中最核心的数据载体，贯穿请求的完整生命周期，
// 包含用户信息、令牌信息、渠道元数据、计费数据、流式状态等所有上下文信息。
//
// 本文件还定义了各种 GenRelayInfo* 工厂函数，用于根据不同请求格式
// （OpenAI、Claude、Gemini、Embedding、Responses 等）创建对应的 RelayInfo 实例。
package common

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/pkg/billingexpr"
	relayconstant "github.com/c1cada/NexusTok/relay/constant"
	"github.com/c1cada/NexusTok/setting/model_setting"
	"github.com/c1cada/NexusTok/types"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// ThinkingContentInfo 跟踪 Claude thinking（思考）内容的发送状态。
// 用于在流式响应中正确处理 thinking 内容块的生命周期。
type ThinkingContentInfo struct {
	IsFirstThinkingContent  bool // 是否为第一个 thinking 内容块
	SendLastThinkingContent bool // 是否需要发送最后一个 thinking 内容块
	HasSentThinkingContent  bool // 是否已发送过 thinking 内容
}

// 定义 Claude 消息类型的常量。
const (
	LastMessageTypeNone     = "none"     // 无消息
	LastMessageTypeText     = "text"     // 文本消息
	LastMessageTypeTools    = "tools"    // 工具调用消息
	LastMessageTypeThinking = "thinking" // 思考内容消息
)

// ClaudeConvertInfo 存储 Claude 响应格式转换过程中的状态信息。
// 用于在流式响应中跟踪 Claude 特有的消息块拼接和工具调用索引管理。
type ClaudeConvertInfo struct {
	LastMessagesType string     // 上一个消息块的类型（text/tools/thinking/none）
	Index            int        // 当前消息块的索引
	Usage            *dto.Usage // 累计使用量
	FinishReason     string     // 结束原因
	Done             bool       // 是否已完成

	ToolCallBaseIndex      int // 工具调用的基础索引（用于多轮对话中的索引偏移）
	ToolCallMaxIndexOffset int // 工具调用的最大索引偏移量
}

// RerankerInfo 存储 Rerank 请求的附加信息。
type RerankerInfo struct {
	Documents       []any // 待排序的文档列表
	ReturnDocuments bool  // 是否在响应中返回文档内容
}

// BuildInToolInfo 存储 Responses API 中内置工具的使用信息。
type BuildInToolInfo struct {
	ToolName          string // 工具名称（如 web_search_preview）
	CallCount         int    // 工具调用次数
	SearchContextSize string // 搜索上下文大小（如 medium、large）
}

// ResponsesUsageInfo 存储 Responses API 的使用统计信息。
type ResponsesUsageInfo struct {
	BuiltInTools map[string]*BuildInToolInfo // 内置工具的使用信息映射
}

// ChannelMeta 存储渠道的元数据信息。
// 在每次请求开始时由 InitChannelMeta 初始化，包含渠道类型、ID、
// 凭证模式、账号池信息、API 类型、版本等完整渠道配置。
type ChannelMeta struct {
	ChannelType          int                      // 渠道类型（如 OpenAI、Azure、Claude 等）
	ChannelId            int                      // 渠道 ID
	ChannelIsMultiKey    bool                     // 是否为多密钥渠道
	ChannelMultiKeyIndex int                      // 多密钥渠道的当前密钥索引
	CredentialMode       string                   // 凭证模式
	ChannelAccountPool   bool                     // 是否使用账号池
	ChannelAccountId     int                      // 账号池中的账号 ID
	ChannelAccountName   string                   // 账号名称
	PoolGroupId          int                      // 账号池分组 ID
	PoolGroupName        string                   // 账号池分组名称
	PoolAccountId        int                      // 账号池账号 ID
	PoolAccountName      string                   // 账号池账号名称
	PoolAccountAuthType  string                   // 账号池账号的认证类型
	ChannelBaseUrl       string                   // 渠道上游基础 URL
	ApiType              int                      // API 类型（如 OpenAI、Claude、Gemini 等）
	ApiVersion           string                   // API 版本（Azure 使用）
	ApiKey               string                   // API 密钥
	Organization         string                   // 组织标识（OpenAI 使用）
	ChannelCreateTime    int64                    // 渠道创建时间戳
	ParamOverride        map[string]interface{}   // 参数覆盖配置
	HeadersOverride      map[string]interface{}   // 请求头覆盖配置
	ChannelSetting       dto.ChannelSettings      // 渠道设置（如系统提示、passthrough 等）
	ChannelOtherSettings dto.ChannelOtherSettings // 渠道其他设置（如禁用字段过滤）
	UpstreamModelName    string                   // 上游模型名称（经过映射后）
	IsModelMapped        bool                     // 是否发生了模型映射
	SupportStreamOptions bool                     // 是否支持流式选项
}

// TokenCountMeta 存储 token 计数的元数据。
type TokenCountMeta struct {
	//promptTokens int
	estimatePromptTokens int // 预估的 prompt token 数量（在请求解析阶段估算）
}

// RelayInfo 是中继处理过程的核心数据载体，贯穿请求的完整生命周期。
// 包含用户身份信息、令牌配置、请求参数、渠道元数据、计费状态、
// 流式响应状态、格式转换链等所有上下文信息。
// 该结构体在请求开始时创建，在整个中继过程中被传递和修改，
// 最终在请求结束后用于计费结算和日志记录。
type RelayInfo struct {
	TokenId           int
	TokenKey          string
	TokenGroup        string
	UserId            int
	UsingGroup        string // 使用的分组，当auto跨分组重试时，会变动
	UserGroup         string // 用户所在分组
	TokenUnlimited    bool
	StartTime         time.Time
	FirstResponseTime time.Time
	isFirstResponse   bool
	//SendLastReasoningResponse bool
	IsStream               bool
	IsGeminiBatchEmbedding bool
	IsPlayground           bool
	UsePrice               bool
	RelayMode              int
	OriginModelName        string
	RequestURLPath         string
	RequestHeaders         map[string]string
	ShouldIncludeUsage     bool
	DisablePing            bool // 是否禁止向下游发送自定义 Ping
	ClientWs               *websocket.Conn
	TargetWs               *websocket.Conn
	InputAudioFormat       string
	OutputAudioFormat      string
	RealtimeTools          []dto.RealTimeTool
	IsFirstRequest         bool
	AudioUsage             bool
	ReasoningEffort        string
	UserSetting            dto.UserSetting
	UserEmail              string
	UserQuota              int
	RelayFormat            types.RelayFormat
	SendResponseCount      int
	ReceivedResponseCount  int
	FinalPreConsumedQuota  int // 最终预消耗的配额
	// ForcePreConsume 为 true 时禁用 BillingSession 的信任额度旁路，
	// 强制预扣全额。用于异步任务（视频/音乐生成等），因为请求返回后任务仍在运行，
	// 必须在提交前锁定全额。
	ForcePreConsume bool
	// Billing 是计费会话，封装了预扣费/结算/退款的统一生命周期。
	// 免费模型时为 nil。
	Billing BillingSettler
	// BillingSource indicates whether this request is billed from wallet quota or subscription.
	// "" or "wallet" => wallet; "subscription" => subscription
	BillingSource string
	// SubscriptionId is the user_subscriptions.id used when BillingSource == "subscription"
	SubscriptionId int
	// SubscriptionPreConsumed is the amount pre-consumed on subscription item (quota units or 1)
	SubscriptionPreConsumed int64
	// SubscriptionPostDelta is the post-consume delta applied to amount_used (quota units; can be negative).
	SubscriptionPostDelta int64
	// SubscriptionPlanId / SubscriptionPlanTitle are used for logging/UI display.
	SubscriptionPlanId    int
	SubscriptionPlanTitle string
	// RequestId is used for idempotent pre-consume/refund
	RequestId string
	// SubscriptionAmountTotal / SubscriptionAmountUsedAfterPreConsume are used to compute remaining in logs.
	SubscriptionAmountTotal               int64
	SubscriptionAmountUsedAfterPreConsume int64
	// UpstreamRequestBodySize 记录转换后上游请求体的字节数。
	// 当 handler 使用 NewOutboundJSONBody 将 JSON body 包装为 BodyStorage 时，
	// net/http 无法再自动识别具体长度；通用请求构造层会用该值回填 ContentLength。
	// 0 表示保持 net/http 默认判断，适用于透传、multipart、WebSocket 等路径。
	UpstreamRequestBodySize   int64
	IsClaudeBetaQuery         bool // /v1/messages?beta=true
	IsChannelTest             bool // channel test request
	RetryIndex                int
	LastError                 *types.NexusTokError
	RuntimeHeadersOverride    map[string]interface{}
	UseRuntimeHeadersOverride bool
	ParamOverrideAudit        []string

	PriceData types.PriceData

	// QuotaClamp 在本次请求的配额换算触发 int32 饱和保护时记录首个事件。
	// 后续消费/任务日志会把它写入 other.admin_info.quota_saturation，
	// 普通用户视图会移除 admin_info，避免暴露内部计费异常细节。
	QuotaClamp *common.QuotaClamp

	// TieredBillingSnapshot is a frozen snapshot of tiered billing rules
	// captured at pre-consume time. Non-nil only when billing mode is "tiered_expr".
	TieredBillingSnapshot *billingexpr.BillingSnapshot
	BillingRequestInput   *billingexpr.RequestInput

	Request dto.Request

	// RequestConversionChain records request format conversions in order, e.g.
	// ["openai", "openai_responses"] or ["openai", "claude"].
	RequestConversionChain []types.RelayFormat
	// 最终请求到上游的格式。可由 adaptor 显式设置；
	// 若为空，调用 GetFinalRequestRelayFormat 会回退到 RequestConversionChain 的最后一项或 RelayFormat。
	FinalRequestRelayFormat types.RelayFormat

	StreamStatus *StreamStatus

	ThinkingContentInfo
	TokenCountMeta
	*ClaudeConvertInfo
	*RerankerInfo
	*ResponsesUsageInfo
	*ChannelMeta
	*TaskRelayInfo
}

// NoteQuotaClamp 记录本次请求首次发生的配额饱和事件。
// 同一次请求可能在预扣、结算、附加费组合等多个阶段触发饱和；保留首次事件
// 能更接近根因，也避免后续兜底转换覆盖早期异常信号。
func (info *RelayInfo) NoteQuotaClamp(clamp *common.QuotaClamp) {
	if info == nil || clamp == nil || info.QuotaClamp != nil {
		return
	}
	info.QuotaClamp = clamp
}

// InitChannelMeta 从 Gin 上下文中初始化渠道元数据（ChannelMeta）。
// 读取上下文中存储的渠道类型、ID、凭证模式、账号池信息、API 配置、
// 参数覆盖、请求头覆盖等信息，并设置到 RelayInfo 的 ChannelMeta 字段中。
// 同时根据渠道类型设置特殊的 ApiVersion（如 Azure 的 API 版本、Vertex 的区域）。
// 初始化完成后还会重置请求对象的模型名称为渠道映射后的模型名。
//
// 参数：
//   - c: Gin 上下文，包含中间件设置的各种渠道信息
func (info *RelayInfo) InitChannelMeta(c *gin.Context) {
	channelType := common.GetContextKeyInt(c, constant.ContextKeyChannelType)
	paramOverride := common.GetContextKeyStringMap(c, constant.ContextKeyChannelParamOverride)
	headerOverride := common.GetContextKeyStringMap(c, constant.ContextKeyChannelHeaderOverride)
	apiType, _ := common.ChannelType2APIType(channelType)
	channelMeta := &ChannelMeta{
		ChannelType:          channelType,
		ChannelId:            common.GetContextKeyInt(c, constant.ContextKeyChannelId),
		ChannelIsMultiKey:    common.GetContextKeyBool(c, constant.ContextKeyChannelIsMultiKey),
		ChannelMultiKeyIndex: common.GetContextKeyInt(c, constant.ContextKeyChannelMultiKeyIndex),
		CredentialMode:       common.GetContextKeyString(c, constant.ContextKeyChannelCredentialMode),
		ChannelAccountPool:   common.GetContextKeyBool(c, constant.ContextKeyChannelAccountPool),
		ChannelAccountId:     common.GetContextKeyInt(c, constant.ContextKeyChannelAccountId),
		ChannelAccountName:   common.GetContextKeyString(c, constant.ContextKeyChannelAccountName),
		PoolGroupId:          common.GetContextKeyInt(c, constant.ContextKeyPoolGroupId),
		PoolGroupName:        common.GetContextKeyString(c, constant.ContextKeyPoolGroupName),
		PoolAccountId:        common.GetContextKeyInt(c, constant.ContextKeyPoolAccountId),
		PoolAccountName:      common.GetContextKeyString(c, constant.ContextKeyPoolAccountName),
		PoolAccountAuthType:  common.GetContextKeyString(c, constant.ContextKeyPoolAccountAuthType),
		ChannelBaseUrl:       common.GetContextKeyString(c, constant.ContextKeyChannelBaseUrl),
		ApiType:              apiType,
		ApiVersion:           c.GetString("api_version"),
		ApiKey:               common.GetContextKeyString(c, constant.ContextKeyChannelKey),
		Organization:         c.GetString("channel_organization"),
		ChannelCreateTime:    c.GetInt64("channel_create_time"),
		ParamOverride:        paramOverride,
		HeadersOverride:      headerOverride,
		UpstreamModelName:    common.GetContextKeyString(c, constant.ContextKeyOriginalModel),
		IsModelMapped:        false,
		SupportStreamOptions: false,
	}

	if channelType == constant.ChannelTypeAzure {
		channelMeta.ApiVersion = GetAPIVersion(c)
	}
	if channelType == constant.ChannelTypeVertexAi {
		channelMeta.ApiVersion = c.GetString("region")
	}

	channelSetting, ok := common.GetContextKeyType[dto.ChannelSettings](c, constant.ContextKeyChannelSetting)
	if ok {
		channelMeta.ChannelSetting = channelSetting
	}

	channelOtherSettings, ok := common.GetContextKeyType[dto.ChannelOtherSettings](c, constant.ContextKeyChannelOtherSetting)
	if ok {
		channelMeta.ChannelOtherSettings = channelOtherSettings
	}

	if streamSupportedChannels[channelMeta.ChannelType] {
		channelMeta.SupportStreamOptions = true
	}

	info.ChannelMeta = channelMeta

	// reset some fields based on channel meta
	// 重置某些字段，例如模型名称等
	if info.Request != nil {
		info.Request.SetModelName(info.OriginModelName)
	}
}

// ToString 生成 RelayInfo 的可读字符串表示，用于日志记录和调试。
// 包含请求格式、中继模式、流式状态、模型名称、用户信息（邮箱脱敏）、
// 令牌信息（密钥脱敏）、时间指标、音频/实时信息、价格数据、渠道元数据（密钥脱敏）等。
// 支持 nil 接收者调用，返回 "RelayInfo<nil>"。
func (info *RelayInfo) ToString() string {
	if info == nil {
		return "RelayInfo<nil>"
	}

	// Basic info
	b := &strings.Builder{}
	fmt.Fprintf(b, "RelayInfo{ ")
	fmt.Fprintf(b, "RelayFormat: %s, ", info.RelayFormat)
	fmt.Fprintf(b, "RelayMode: %d, ", info.RelayMode)
	fmt.Fprintf(b, "IsStream: %t, ", info.IsStream)
	fmt.Fprintf(b, "IsPlayground: %t, ", info.IsPlayground)
	fmt.Fprintf(b, "RequestURLPath: %q, ", info.RequestURLPath)
	fmt.Fprintf(b, "OriginModelName: %q, ", info.OriginModelName)
	fmt.Fprintf(b, "EstimatePromptTokens: %d, ", info.estimatePromptTokens)
	fmt.Fprintf(b, "ShouldIncludeUsage: %t, ", info.ShouldIncludeUsage)
	fmt.Fprintf(b, "DisablePing: %t, ", info.DisablePing)
	fmt.Fprintf(b, "SendResponseCount: %d, ", info.SendResponseCount)
	fmt.Fprintf(b, "FinalPreConsumedQuota: %d, ", info.FinalPreConsumedQuota)

	// User & token info (mask secrets)
	fmt.Fprintf(b, "User{ Id: %d, Email: %q, Group: %q, UsingGroup: %q, Quota: %d }, ",
		info.UserId, common.MaskEmail(info.UserEmail), info.UserGroup, info.UsingGroup, info.UserQuota)
	fmt.Fprintf(b, "Token{ Id: %d, Unlimited: %t, Key: ***masked*** }, ", info.TokenId, info.TokenUnlimited)

	// Time info
	latencyMs := info.FirstResponseTime.Sub(info.StartTime).Milliseconds()
	fmt.Fprintf(b, "Timing{ Start: %s, FirstResponse: %s, LatencyMs: %d }, ",
		info.StartTime.Format(time.RFC3339Nano), info.FirstResponseTime.Format(time.RFC3339Nano), latencyMs)

	// Audio / realtime
	if info.InputAudioFormat != "" || info.OutputAudioFormat != "" || len(info.RealtimeTools) > 0 || info.AudioUsage {
		fmt.Fprintf(b, "Realtime{ AudioUsage: %t, InFmt: %q, OutFmt: %q, Tools: %d }, ",
			info.AudioUsage, info.InputAudioFormat, info.OutputAudioFormat, len(info.RealtimeTools))
	}

	// Reasoning
	if info.ReasoningEffort != "" {
		fmt.Fprintf(b, "ReasoningEffort: %q, ", info.ReasoningEffort)
	}

	// Price data (non-sensitive)
	if info.PriceData.UsePrice {
		fmt.Fprintf(b, "PriceData{ %s }, ", info.PriceData.ToSetting())
	}

	// Channel metadata (mask ApiKey)
	if info.ChannelMeta != nil {
		cm := info.ChannelMeta
		fmt.Fprintf(b, "ChannelMeta{ Type: %d, Id: %d, CredentialMode: %q, IsMultiKey: %t, MultiKeyIndex: %d, AccountPool: %t, AccountId: %d, AccountName: %q, BaseURL: %q, ApiType: %d, ApiVersion: %q, Organization: %q, CreateTime: %d, UpstreamModelName: %q, IsModelMapped: %t, SupportStreamOptions: %t, ApiKey: ***masked*** }, ",
			cm.ChannelType, cm.ChannelId, cm.CredentialMode, cm.ChannelIsMultiKey, cm.ChannelMultiKeyIndex, cm.ChannelAccountPool, cm.ChannelAccountId, cm.ChannelAccountName, cm.ChannelBaseUrl, cm.ApiType, cm.ApiVersion, cm.Organization, cm.ChannelCreateTime, cm.UpstreamModelName, cm.IsModelMapped, cm.SupportStreamOptions)
	}

	// Responses usage info (non-sensitive)
	if info.ResponsesUsageInfo != nil && len(info.ResponsesUsageInfo.BuiltInTools) > 0 {
		fmt.Fprintf(b, "ResponsesTools{ ")
		first := true
		for name, tool := range info.ResponsesUsageInfo.BuiltInTools {
			if !first {
				fmt.Fprintf(b, ", ")
			}
			first = false
			if tool != nil {
				fmt.Fprintf(b, "%s: calls=%d", name, tool.CallCount)
			} else {
				fmt.Fprintf(b, "%s: calls=0", name)
			}
		}
		fmt.Fprintf(b, " }, ")
	}

	fmt.Fprintf(b, "}")
	return b.String()
}

// streamSupportedChannels 定义了支持流式选项（StreamOptions）的渠道类型映射。
// 只有在此映射中的渠道类型才会在请求中携带 StreamOptions 参数。
var streamSupportedChannels = map[int]bool{
	constant.ChannelTypeOpenAI:         true,
	constant.ChannelTypeAnthropic:      true,
	constant.ChannelTypeAws:            true,
	constant.ChannelTypeGemini:         true,
	constant.ChannelCloudflare:         true,
	constant.ChannelTypeAzure:          true,
	constant.ChannelTypeVolcEngine:     true,
	constant.ChannelTypeOllama:         true,
	constant.ChannelTypeXai:            true,
	constant.ChannelTypeDeepSeek:       true,
	constant.ChannelTypeBaiduV2:        true,
	constant.ChannelTypeZhipu_v4:       true,
	constant.ChannelTypeAli:            true,
	constant.ChannelTypeSubmodel:       true,
	constant.ChannelTypeCodex:          true,
	constant.ChannelTypeMoonshot:       true,
	constant.ChannelTypeMiniMax:        true,
	constant.ChannelTypeSiliconFlow:    true,
	constant.ChannelTypeAdvancedCustom: true,
}

// GenRelayInfoWs 创建 WebSocket 实时通信请求的 RelayInfo 实例。
// 默认设置音频格式为 pcm16，标记为首次请求。
//
// 参数：
//   - c: Gin 上下文
//   - ws: 客户端的 WebSocket 连接
//
// 返回值：
//   - *RelayInfo: 初始化后的中继信息
func GenRelayInfoWs(c *gin.Context, ws *websocket.Conn) *RelayInfo {
	info := genBaseRelayInfo(c, nil)
	info.RelayFormat = types.RelayFormatOpenAIRealtime
	info.ClientWs = ws
	info.InputAudioFormat = "pcm16"
	info.OutputAudioFormat = "pcm16"
	info.IsFirstRequest = true
	return info
}

// GenRelayInfoClaude 创建 Claude 格式请求的 RelayInfo 实例。
// 初始化 Claude 特有的转换信息（ClaudeConvertInfo）和 beta 查询标志。
//
// 参数：
//   - c: Gin 上下文
//   - request: 请求对象
//
// 返回值：
//   - *RelayInfo: 初始化后的中继信息
func GenRelayInfoClaude(c *gin.Context, request dto.Request) *RelayInfo {
	info := genBaseRelayInfo(c, request)
	info.RelayFormat = types.RelayFormatClaude
	info.ShouldIncludeUsage = false
	info.ClaudeConvertInfo = &ClaudeConvertInfo{
		LastMessagesType: LastMessageTypeNone,
	}
	info.IsClaudeBetaQuery = c.Query("beta") == "true"
	return info
}

// GenRelayInfoRerank 创建 Rerank（文档重排序）请求的 RelayInfo 实例。
// 初始化 RerankerInfo，包含文档列表和是否返回文档内容的配置。
func GenRelayInfoRerank(c *gin.Context, request *dto.RerankRequest) *RelayInfo {
	info := genBaseRelayInfo(c, request)
	info.RelayMode = relayconstant.RelayModeRerank
	info.RelayFormat = types.RelayFormatRerank
	info.RerankerInfo = &RerankerInfo{
		Documents:       request.Documents,
		ReturnDocuments: request.GetReturnDocuments(),
	}
	return info
}

// GenRelayInfoOpenAIAudio 创建 OpenAI 音频请求的 RelayInfo 实例。
func GenRelayInfoOpenAIAudio(c *gin.Context, request dto.Request) *RelayInfo {
	info := genBaseRelayInfo(c, request)
	info.RelayFormat = types.RelayFormatOpenAIAudio
	return info
}

// GenRelayInfoEmbedding 创建 Embedding（文本向量化）请求的 RelayInfo 实例。
func GenRelayInfoEmbedding(c *gin.Context, request dto.Request) *RelayInfo {
	info := genBaseRelayInfo(c, request)
	info.RelayFormat = types.RelayFormatEmbedding
	return info
}

// GenRelayInfoResponses 创建 OpenAI Responses API 请求的 RelayInfo 实例。
// 初始化 ResponsesUsageInfo，解析请求中的内置工具列表（如 web_search_preview），
// 并记录每个工具的搜索上下文大小等配置。
func GenRelayInfoResponses(c *gin.Context, request *dto.OpenAIResponsesRequest) *RelayInfo {
	info := genBaseRelayInfo(c, request)
	info.RelayMode = relayconstant.RelayModeResponses
	info.RelayFormat = types.RelayFormatOpenAIResponses

	info.ResponsesUsageInfo = &ResponsesUsageInfo{
		BuiltInTools: make(map[string]*BuildInToolInfo),
	}
	if len(request.Tools) > 0 {
		for _, tool := range request.GetToolsMap() {
			toolType := common.Interface2String(tool["type"])
			info.ResponsesUsageInfo.BuiltInTools[toolType] = &BuildInToolInfo{
				ToolName:  toolType,
				CallCount: 0,
			}
			switch toolType {
			case dto.BuildInToolWebSearchPreview:
				searchContextSize := common.Interface2String(tool["search_context_size"])
				if searchContextSize == "" {
					searchContextSize = "medium"
				}
				info.ResponsesUsageInfo.BuiltInTools[toolType].SearchContextSize = searchContextSize
			}
		}
	}
	return info
}

// GenRelayInfoGemini 创建 Gemini 格式请求的 RelayInfo 实例。
func GenRelayInfoGemini(c *gin.Context, request dto.Request) *RelayInfo {
	info := genBaseRelayInfo(c, request)
	info.RelayFormat = types.RelayFormatGemini
	info.ShouldIncludeUsage = false

	return info
}

// GenRelayInfoImage 创建图片生成请求的 RelayInfo 实例。
func GenRelayInfoImage(c *gin.Context, request dto.Request) *RelayInfo {
	info := genBaseRelayInfo(c, request)
	info.RelayFormat = types.RelayFormatOpenAIImage
	return info
}

// GenRelayInfoOpenAI 创建 OpenAI 格式请求的 RelayInfo 实例。
func GenRelayInfoOpenAI(c *gin.Context, request dto.Request) *RelayInfo {
	info := genBaseRelayInfo(c, request)
	info.RelayFormat = types.RelayFormatOpenAI
	return info
}

// genBaseRelayInfo 是所有 GenRelayInfo* 函数的基础实现。
// 从 Gin 上下文中提取用户、令牌、请求等基本信息，构建基础的 RelayInfo 实例。
// 处理 Playground 路径前缀、流式检测、请求 ID 生成等逻辑。
//
// 参数：
//   - c: Gin 上下文
//   - request: 请求对象（WebSocket 模式下可为 nil）
//
// 返回值：
//   - *RelayInfo: 基础中继信息实例
func genBaseRelayInfo(c *gin.Context, request dto.Request) *RelayInfo {

	//channelType := common.GetContextKeyInt(c, constant.ContextKeyChannelType)
	//channelId := common.GetContextKeyInt(c, constant.ContextKeyChannelId)
	//paramOverride := common.GetContextKeyStringMap(c, constant.ContextKeyChannelParamOverride)

	tokenGroup := common.GetContextKeyString(c, constant.ContextKeyTokenGroup)
	// 当令牌分组为空时，表示使用用户分组
	if tokenGroup == "" {
		tokenGroup = common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	}

	startTime := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime)
	if startTime.IsZero() {
		startTime = time.Now()
	}

	isStream := false

	if request != nil {
		isStream = request.IsStream(c)
	}
	c.Set(string(constant.ContextKeyIsStream), isStream)

	// firstResponseTime = time.Now() - 1 second

	reqId := common.GetContextKeyString(c, common.RequestIdKey)
	if reqId == "" {
		reqId = common.GetTimeString() + common.GetRandomString(8)
	}
	info := &RelayInfo{
		Request: request,

		RequestId:  reqId,
		UserId:     common.GetContextKeyInt(c, constant.ContextKeyUserId),
		UsingGroup: common.GetContextKeyString(c, constant.ContextKeyUsingGroup),
		UserGroup:  common.GetContextKeyString(c, constant.ContextKeyUserGroup),
		UserQuota:  common.GetContextKeyInt(c, constant.ContextKeyUserQuota),
		UserEmail:  common.GetContextKeyString(c, constant.ContextKeyUserEmail),

		OriginModelName: common.GetContextKeyString(c, constant.ContextKeyOriginalModel),

		TokenId:        common.GetContextKeyInt(c, constant.ContextKeyTokenId),
		TokenKey:       common.GetContextKeyString(c, constant.ContextKeyTokenKey),
		TokenUnlimited: common.GetContextKeyBool(c, constant.ContextKeyTokenUnlimited),
		TokenGroup:     tokenGroup,

		isFirstResponse: true,
		RelayMode:       relayconstant.Path2RelayMode(c.Request.URL.Path),
		RequestURLPath:  c.Request.URL.String(),
		RequestHeaders:  cloneRequestHeaders(c),
		IsStream:        isStream,

		StartTime:         startTime,
		FirstResponseTime: startTime.Add(-time.Second),
		ThinkingContentInfo: ThinkingContentInfo{
			IsFirstThinkingContent:  true,
			SendLastThinkingContent: false,
		},
		TokenCountMeta: TokenCountMeta{
			//promptTokens: common.GetContextKeyInt(c, constant.ContextKeyPromptTokens),
			estimatePromptTokens: common.GetContextKeyInt(c, constant.ContextKeyEstimatedTokens),
		},
	}

	if info.RelayMode == relayconstant.RelayModeUnknown {
		info.RelayMode = c.GetInt("relay_mode")
	}

	if strings.HasPrefix(c.Request.URL.Path, "/pg") {
		info.IsPlayground = true
		info.RequestURLPath = strings.TrimPrefix(info.RequestURLPath, "/pg")
		info.RequestURLPath = "/v1" + info.RequestURLPath
	}
	userSetting, ok := common.GetContextKeyType[dto.UserSetting](c, constant.ContextKeyUserSetting)
	if ok {
		info.UserSetting = userSetting
	}

	return info
}

// cloneRequestHeaders 从 Gin 上下文中克隆请求头映射。
// 将所有非空的请求头键值对复制到新的映射中，键保持原始大小写。
// 用于在参数覆盖的条件判断中引用请求头信息。
//
// 参数：
//   - c: Gin 上下文
//
// 返回值：
//   - map[string]string: 请求头映射，无请求头时返回 nil
func cloneRequestHeaders(c *gin.Context) map[string]string {
	if c == nil || c.Request == nil {
		return nil
	}
	if len(c.Request.Header) == 0 {
		return nil
	}
	headers := make(map[string]string, len(c.Request.Header))
	for key := range c.Request.Header {
		value := strings.TrimSpace(c.Request.Header.Get(key))
		if value == "" {
			continue
		}
		headers[key] = value
	}
	if len(headers) == 0 {
		return nil
	}
	return headers
}

// GenRelayInfo 是 RelayInfo 的统一工厂函数。
// 根据 relayFormat 参数选择对应的 GenRelayInfo* 函数创建实例。
// 支持的格式包括：OpenAI、OpenAIAudio、OpenAIImage、OpenAIRealtime（WebSocket）、
// Claude、Rerank、Gemini、Embedding、OpenAIResponses、OpenAIResponsesCompaction、Task、MjProxy。
//
// 参数：
//   - c: Gin 上下文
//   - relayFormat: 请求的中继格式类型
//   - request: 请求对象
//   - ws: WebSocket 连接（仅 OpenAIRealtime 格式使用）
//
// 返回值：
//   - *RelayInfo: 初始化后的中继信息
//   - error: 创建过程中的错误（如格式不支持或类型断言失败）
func GenRelayInfo(c *gin.Context, relayFormat types.RelayFormat, request dto.Request, ws *websocket.Conn) (*RelayInfo, error) {
	var info *RelayInfo
	var err error
	switch relayFormat {
	case types.RelayFormatOpenAI:
		info = GenRelayInfoOpenAI(c, request)
	case types.RelayFormatOpenAIAudio:
		info = GenRelayInfoOpenAIAudio(c, request)
	case types.RelayFormatOpenAIImage:
		info = GenRelayInfoImage(c, request)
	case types.RelayFormatOpenAIRealtime:
		info = GenRelayInfoWs(c, ws)
	case types.RelayFormatClaude:
		info = GenRelayInfoClaude(c, request)
	case types.RelayFormatRerank:
		if request, ok := request.(*dto.RerankRequest); ok {
			info = GenRelayInfoRerank(c, request)
			break
		}
		err = errors.New("request is not a RerankRequest")
	case types.RelayFormatGemini:
		info = GenRelayInfoGemini(c, request)
	case types.RelayFormatEmbedding:
		info = GenRelayInfoEmbedding(c, request)
	case types.RelayFormatOpenAIResponses:
		if request, ok := request.(*dto.OpenAIResponsesRequest); ok {
			info = GenRelayInfoResponses(c, request)
			break
		}
		err = errors.New("request is not a OpenAIResponsesRequest")
	case types.RelayFormatOpenAIResponsesCompaction:
		if request, ok := request.(*dto.OpenAIResponsesCompactionRequest); ok {
			return GenRelayInfoResponsesCompaction(c, request), nil
		}
		return nil, errors.New("request is not a OpenAIResponsesCompactionRequest")
	case types.RelayFormatTask:
		info = genBaseRelayInfo(c, nil)
		info.TaskRelayInfo = &TaskRelayInfo{}
	case types.RelayFormatMjProxy:
		info = genBaseRelayInfo(c, nil)
		info.TaskRelayInfo = &TaskRelayInfo{}
	default:
		err = errors.New("invalid relay format")
	}

	if err != nil {
		return nil, err
	}
	if info == nil {
		return nil, errors.New("failed to build relay info")
	}

	info.InitRequestConversionChain()
	return info, nil
}

// InitRequestConversionChain 初始化请求格式转换链。
// 若转换链为空，则以当前 RelayFormat 作为链的起始节点。
func (info *RelayInfo) InitRequestConversionChain() {
	if info == nil {
		return
	}
	if len(info.RequestConversionChain) > 0 {
		return
	}
	if info.RelayFormat == "" {
		return
	}
	info.RequestConversionChain = []types.RelayFormat{info.RelayFormat}
}

// AppendRequestConversion 向请求格式转换链中追加一个新的格式。
// 如果链为空则初始化；如果新格式与链尾相同则不重复追加。
// 用于记录请求在中继过程中经历的格式转换路径（如 openai -> claude）。
func (info *RelayInfo) AppendRequestConversion(format types.RelayFormat) {
	if info == nil {
		return
	}
	if format == "" {
		return
	}
	if len(info.RequestConversionChain) == 0 {
		info.RequestConversionChain = []types.RelayFormat{format}
		return
	}
	last := info.RequestConversionChain[len(info.RequestConversionChain)-1]
	if last == format {
		return
	}
	info.RequestConversionChain = append(info.RequestConversionChain, format)
}

// GetFinalRequestRelayFormat 获取最终发送到上游的请求格式。
// 按优先级返回：
//  1. 显式设置的 FinalRequestRelayFormat（由 adaptor 设置）。
//  2. 转换链的最后一个格式。
//  3. 原始 RelayFormat。
func (info *RelayInfo) GetFinalRequestRelayFormat() types.RelayFormat {
	if info == nil {
		return ""
	}
	if info.FinalRequestRelayFormat != "" {
		return info.FinalRequestRelayFormat
	}
	if n := len(info.RequestConversionChain); n > 0 {
		return info.RequestConversionChain[n-1]
	}
	return info.RelayFormat
}

// GenRelayInfoResponsesCompaction 创建 Responses 压缩请求的 RelayInfo 实例。
func GenRelayInfoResponsesCompaction(c *gin.Context, request *dto.OpenAIResponsesCompactionRequest) *RelayInfo {
	info := genBaseRelayInfo(c, request)
	if info.RelayMode == relayconstant.RelayModeUnknown {
		info.RelayMode = relayconstant.RelayModeResponsesCompact
	}
	info.RelayFormat = types.RelayFormatOpenAIResponsesCompaction
	return info
}

//func (info *RelayInfo) SetPromptTokens(promptTokens int) {
//	info.promptTokens = promptTokens
//}

// SetEstimatePromptTokens 设置预估的 prompt token 数量。
func (info *RelayInfo) SetEstimatePromptTokens(promptTokens int) {
	info.estimatePromptTokens = promptTokens
}

// GetEstimatePromptTokens 获取预估的 prompt token 数量。
func (info *RelayInfo) GetEstimatePromptTokens() int {
	return info.estimatePromptTokens
}

// SetFirstResponseTime 记录首次响应时间。
// 使用 sync.Once 语义，仅第一次调用生效。用于计算请求的首字节延迟（TTFB）。
func (info *RelayInfo) SetFirstResponseTime() {
	if info.isFirstResponse {
		info.FirstResponseTime = time.Now()
		info.isFirstResponse = false
	}
}

// HasSendResponse 判断是否已收到上游的首个响应。
// 通过比较 FirstResponseTime 和 StartTime 判断。
func (info *RelayInfo) HasSendResponse() bool {
	return info.FirstResponseTime.After(info.StartTime)
}

// TaskRelayInfo 存储异步任务（Task）请求的附加信息。
// 包含任务操作类型、原始任务 ID、公开任务 ID 和计费控制等。
type TaskRelayInfo struct {
	Action       string // 任务操作类型（如 generate、remix、video 等）
	OriginTaskID string // 原始任务 ID（remix/continuation 操作时使用）
	// PublicTaskID 是提交时预生成的 task_xxxx 格式公开 ID，
	// 供 DoResponse 在返回给客户端时使用（避免暴露上游真实 ID）。
	PublicTaskID string

	ConsumeQuota bool // 是否需要消费配额

	// LockedChannel holds the full channel object when the request is bound to
	// a specific channel (e.g., remix on origin task's channel). Stored as any
	// to avoid an import cycle with model; callers type-assert to *model.Channel.
	LockedChannel any // 锁定的渠道对象（remix 操作时绑定到原任务渠道）
}

// TaskSubmitReq 是异步任务提交请求的数据结构。
// 支持 JSON 反序列化，其中 duration 和 metadata 字段支持多种格式（数字/字符串）。
type TaskSubmitReq struct {
	Prompt         string                 `json:"prompt"`
	Model          string                 `json:"model,omitempty"`
	Mode           string                 `json:"mode,omitempty"`
	Image          string                 `json:"image,omitempty"`
	Images         []string               `json:"images,omitempty"`
	Size           string                 `json:"size,omitempty"`
	Duration       int                    `json:"duration,omitempty"`
	Seconds        string                 `json:"seconds,omitempty"`
	InputReference string                 `json:"input_reference,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// GetPrompt 获取任务提交请求的提示词。
func (t *TaskSubmitReq) GetPrompt() string {
	return t.Prompt
}

// HasImage 判断任务提交请求是否包含图片输入。
func (t *TaskSubmitReq) HasImage() bool {
	return len(t.Images) > 0
}

// UnmarshalJSON 自定义 JSON 反序列化，处理 duration 和 metadata 字段的多格式兼容。
// duration 支持 int 和 string 两种格式；metadata 支持直接对象和 JSON 字符串两种格式。
func (t *TaskSubmitReq) UnmarshalJSON(data []byte) error {
	type Alias TaskSubmitReq
	aux := &struct {
		Metadata json.RawMessage `json:"metadata,omitempty"`
		Duration json.RawMessage `json:"duration,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(t),
	}

	if err := common.Unmarshal(data, &aux); err != nil {
		return err
	}

	if len(aux.Duration) > 0 {
		var durationInt int
		if err := common.Unmarshal(aux.Duration, &durationInt); err == nil {
			t.Duration = durationInt
		} else {
			var durationStr string
			if err := common.Unmarshal(aux.Duration, &durationStr); err == nil && durationStr != "" {
				if v, err := strconv.Atoi(durationStr); err == nil {
					t.Duration = v
				}
			}
		}
	}

	if len(aux.Metadata) > 0 {
		var metadataStr string
		if err := common.Unmarshal(aux.Metadata, &metadataStr); err == nil && metadataStr != "" {
			var metadataObj map[string]interface{}
			if err := common.Unmarshal([]byte(metadataStr), &metadataObj); err == nil {
				t.Metadata = metadataObj
				return nil
			}
		}

		var metadataObj map[string]interface{}
		if err := common.Unmarshal(aux.Metadata, &metadataObj); err == nil {
			t.Metadata = metadataObj
		}
	}

	return nil
}

// UnmarshalMetadata 将 TaskSubmitReq 的 metadata 字段反序列化为指定类型。
// 用于各 adaptor 将通用的 metadata 映射为特定平台的参数结构。
func (t *TaskSubmitReq) UnmarshalMetadata(v any) error {
	metadata := t.Metadata
	if metadata != nil {
		metadataBytes, err := common.Marshal(metadata)
		if err != nil {
			return fmt.Errorf("marshal metadata failed: %w", err)
		}
		err = common.Unmarshal(metadataBytes, v)
		if err != nil {
			return fmt.Errorf("unmarshal metadata to target failed: %w", err)
		}
	}
	return nil
}

// TaskInfo 表示异步任务的状态信息。
// 由上游任务适配器返回，包含任务状态、进度、结果 URL 等。
type TaskInfo struct {
	Code             int    `json:"code"`
	TaskID           string `json:"task_id"`
	Status           string `json:"status"`
	Reason           string `json:"reason,omitempty"`
	Url              string `json:"url,omitempty"`
	RemoteUrl        string `json:"remote_url,omitempty"`
	Progress         string `json:"progress,omitempty"`
	CompletionTokens int    `json:"completion_tokens,omitempty"` // 用于按倍率计费
	TotalTokens      int    `json:"total_tokens,omitempty"`      // 用于按倍率计费
}

// FailTaskInfo 创建一个失败状态的 TaskInfo。
//
// 参数：
//   - reason: 失败原因
//
// 返回值：
//   - *TaskInfo: 失败状态的任务信息
func FailTaskInfo(reason string) *TaskInfo {
	return &TaskInfo{
		Status: "FAILURE",
		Reason: reason,
	}
}

// RemoveDisabledFields 从请求 JSON 数据中移除渠道设置中禁用的字段。
// 这些字段可能带来安全风险或额外计费，需要根据渠道配置决定是否过滤。
//
// 支持过滤的字段：
//   - service_tier: 服务层级字段，可能导致额外计费（OpenAI、Claude、Responses API 支持）
//   - inference_geo: Claude 数据驻留推理区域字段（仅 Claude 支持，默认过滤）
//   - speed: Claude 推理速度模式字段（仅 Claude 支持，默认过滤）
//   - store: 数据存储授权字段，涉及用户隐私（仅 OpenAI、Responses API 支持，默认允许透传）
//   - safety_identifier: 安全标识符，用于向 OpenAI 报告违规用户（仅 OpenAI 支持，默认过滤）
//   - stream_options.include_obfuscation: 响应流混淆控制字段（仅 OpenAI Responses API 支持）
//
// 当全局或渠道级的 passthrough 模式启用时，跳过所有过滤。
//
// 参数：
//   - jsonData: 原始请求体 JSON
//   - channelOtherSettings: 渠道的其他设置（控制各字段的过滤行为）
//   - channelPassThroughEnabled: 渠道级 passthrough 是否启用
//
// 返回值：
//   - []byte: 过滤后的 JSON
//   - error: 处理过程中的错误
func RemoveDisabledFields(jsonData []byte, channelOtherSettings dto.ChannelOtherSettings, channelPassThroughEnabled bool) ([]byte, error) {
	if model_setting.GetGlobalSettings().PassThroughRequestEnabled || channelPassThroughEnabled {
		return jsonData, nil
	}

	var data map[string]interface{}
	if err := common.Unmarshal(jsonData, &data); err != nil {
		common.SysError("RemoveDisabledFields Unmarshal error :" + err.Error())
		return jsonData, nil
	}

	// 默认移除 service_tier，除非明确允许（避免额外计费风险）
	if !channelOtherSettings.AllowServiceTier {
		if _, exists := data["service_tier"]; exists {
			delete(data, "service_tier")
		}
	}

	// 默认移除 inference_geo，除非明确允许（避免在未授权情况下透传数据驻留区域）
	if !channelOtherSettings.AllowInferenceGeo {
		if _, exists := data["inference_geo"]; exists {
			delete(data, "inference_geo")
		}
	}

	// 默认移除 speed，除非明确允许（避免意外切换 Claude 推理速度模式）
	if !channelOtherSettings.AllowSpeed {
		if _, exists := data["speed"]; exists {
			delete(data, "speed")
		}
	}

	// 默认允许 store 透传，除非明确禁用（禁用可能影响 Codex 使用）
	if channelOtherSettings.DisableStore {
		if _, exists := data["store"]; exists {
			delete(data, "store")
		}
	}

	// 默认移除 safety_identifier，除非明确允许（保护用户隐私，避免向 OpenAI 报告用户信息）
	if !channelOtherSettings.AllowSafetyIdentifier {
		if _, exists := data["safety_identifier"]; exists {
			delete(data, "safety_identifier")
		}
	}

	// 默认移除 stream_options.include_obfuscation，除非明确允许（避免关闭响应流混淆保护）
	if !channelOtherSettings.AllowIncludeObfuscation {
		if streamOptionsAny, exists := data["stream_options"]; exists {
			if streamOptions, ok := streamOptionsAny.(map[string]interface{}); ok {
				if _, includeExists := streamOptions["include_obfuscation"]; includeExists {
					delete(streamOptions, "include_obfuscation")
				}
				if len(streamOptions) == 0 {
					delete(data, "stream_options")
				} else {
					data["stream_options"] = streamOptions
				}
			}
		}
	}

	jsonDataAfter, err := common.Marshal(data)
	if err != nil {
		common.SysError("RemoveDisabledFields Marshal error :" + err.Error())
		return jsonData, nil
	}
	return jsonDataAfter, nil
}

// RemoveGeminiDisabledFields 从 Gemini 请求 JSON 中移除不兼容的字段。
// 当前主要处理 functionResponse.id 字段，因为 Vertex AI 不支持该字段。
// 仅在 Gemini 设置中启用了 RemoveFunctionResponseIdEnabled 时生效。
//
// 参数：
//   - jsonData: 原始请求体 JSON
//
// 返回值：
//   - []byte: 处理后的 JSON
//   - error: 处理过程中的错误
func RemoveGeminiDisabledFields(jsonData []byte) ([]byte, error) {
	if !model_setting.GetGeminiSettings().RemoveFunctionResponseIdEnabled {
		return jsonData, nil
	}

	var data map[string]interface{}
	if err := common.Unmarshal(jsonData, &data); err != nil {
		common.SysError("RemoveGeminiDisabledFields Unmarshal error: " + err.Error())
		return jsonData, nil
	}

	// Process contents array
	// Handle both camelCase (functionResponse) and snake_case (function_response)
	if contents, ok := data["contents"].([]interface{}); ok {
		for _, content := range contents {
			if contentMap, ok := content.(map[string]interface{}); ok {
				if parts, ok := contentMap["parts"].([]interface{}); ok {
					for _, part := range parts {
						if partMap, ok := part.(map[string]interface{}); ok {
							// Check functionResponse (camelCase)
							if funcResp, ok := partMap["functionResponse"].(map[string]interface{}); ok {
								delete(funcResp, "id")
							}
							// Check function_response (snake_case)
							if funcResp, ok := partMap["function_response"].(map[string]interface{}); ok {
								delete(funcResp, "id")
							}
						}
					}
				}
			}
		}
	}

	jsonDataAfter, err := common.Marshal(data)
	if err != nil {
		common.SysError("RemoveGeminiDisabledFields Marshal error: " + err.Error())
		return jsonData, nil
	}
	return jsonDataAfter, nil
}
