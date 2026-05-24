// redisqueue - plugin.go
// 本文件实现了 Redis 队列的使用记录插件，负责将 AI API 请求的使用统计数据
// 序列化后入队，供管理面板实时消费和展示。插件在 init 阶段自动注册到核心使用统计系统。
package redisqueue

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	internallogging "github.com/router-for-me/CLIProxyAPI/v7/internal/logging"
	coreusage "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/usage"
)

// init 在包初始化时将 usageQueuePlugin 注册到核心使用统计插件系统。
func init() {
	coreusage.RegisterPlugin(&usageQueuePlugin{})
}

// usageQueuePlugin 是使用统计队列插件的核心结构体，实现了 coreusage.Plugin 接口。
// 当队列功能和使用统计开关均启用时，它会将每次请求的使用记录序列化为 JSON 并入队。
type usageQueuePlugin struct{}

// HandleUsage 处理一次 API 请求的使用记录。
// 它从记录中提取模型名称、提供商、认证类型、token 统计等信息，
// 构造 queuedUsageDetail 结构并序列化为 JSON 后推入队列。
//
// 参数：
//   - ctx: 请求上下文，用于提取请求 ID、端点等信息
//   - record: 核心使用统计数据记录
func (p *usageQueuePlugin) HandleUsage(ctx context.Context, record coreusage.Record) {
	if p == nil {
		return
	}
	if !Enabled() || !UsageStatisticsEnabled() {
		return
	}

	timestamp := record.RequestedAt
	if timestamp.IsZero() {
		timestamp = time.Now()
	}

	modelName := strings.TrimSpace(record.Model)
	if modelName == "" {
		modelName = "unknown"
	}
	aliasName := strings.TrimSpace(record.Alias)
	if aliasName == "" {
		aliasName = modelName
	}
	provider := strings.TrimSpace(record.Provider)
	if provider == "" {
		provider = "unknown"
	}
	authType := strings.TrimSpace(record.AuthType)
	if authType == "" {
		authType = "unknown"
	}
	apiKey := strings.TrimSpace(record.APIKey)
	requestID := strings.TrimSpace(internallogging.GetRequestID(ctx))
	reasoningEffort := strings.TrimSpace(record.ReasoningEffort)
	if reasoningEffort == "" {
		reasoningEffort = coreusage.ReasoningEffortFromContext(ctx)
	}

	tokens := tokenStats{
		InputTokens:         record.Detail.InputTokens,
		OutputTokens:        record.Detail.OutputTokens,
		ReasoningTokens:     record.Detail.ReasoningTokens,
		CachedTokens:        record.Detail.CachedTokens,
		CacheReadTokens:     record.Detail.CacheReadTokens,
		CacheCreationTokens: record.Detail.CacheCreationTokens,
		TotalTokens:         record.Detail.TotalTokens,
	}
	if tokens.TotalTokens == 0 {
		tokens.TotalTokens = tokens.InputTokens + tokens.OutputTokens + tokens.ReasoningTokens
	}
	if tokens.TotalTokens == 0 {
		tokens.TotalTokens = tokens.InputTokens + tokens.OutputTokens + tokens.ReasoningTokens + tokens.CachedTokens
	}

	failed := record.Failed
	if !failed {
		failed = !resolveSuccess(ctx)
	}
	fail := resolveFail(ctx, record, failed)

	detail := requestDetail{
		Timestamp:       timestamp,
		LatencyMs:       record.Latency.Milliseconds(),
		Source:          record.Source,
		AuthIndex:       record.AuthIndex,
		Tokens:          tokens,
		Failed:          failed,
		Fail:            fail,
		ResponseHeaders: record.ResponseHeaders,
	}

	payload, err := json.Marshal(queuedUsageDetail{
		requestDetail:   detail,
		Provider:        provider,
		Model:           modelName,
		Alias:           aliasName,
		Endpoint:        resolveEndpoint(ctx),
		AuthType:        authType,
		APIKey:          apiKey,
		RequestID:       requestID,
		ReasoningEffort: reasoningEffort,
	})
	if err != nil {
		return
	}
	Enqueue(payload)
}

// queuedUsageDetail 是入队的完整使用记录结构体，包含请求详情和扩展的元数据字段。
// 它嵌入了 requestDetail 并添加了提供商、模型、别名等请求级信息。
type queuedUsageDetail struct {
	requestDetail
	Provider        string `json:"provider"`
	Model           string `json:"model"`
	Alias           string `json:"alias"`
	Endpoint        string `json:"endpoint"`
	AuthType        string `json:"auth_type"`
	APIKey          string `json:"api_key"`
	RequestID       string `json:"request_id"`
	ReasoningEffort string `json:"reasoning_effort"`
}

// requestDetail 记录单次 API 请求的详细信息，包括时间戳、延迟、token 统计和失败信息。
type requestDetail struct {
	Timestamp       time.Time   `json:"timestamp"`
	LatencyMs       int64       `json:"latency_ms"`
	Source          string      `json:"source"`
	AuthIndex       string      `json:"auth_index"`
	Tokens          tokenStats  `json:"tokens"`
	Failed          bool        `json:"failed"`
	Fail            failDetail  `json:"fail"`
	ResponseHeaders http.Header `json:"response_headers,omitempty"`
}

type tokenStats struct {
	InputTokens         int64 `json:"input_tokens"`
	OutputTokens        int64 `json:"output_tokens"`
	ReasoningTokens     int64 `json:"reasoning_tokens"`
	CachedTokens        int64 `json:"cached_tokens"`
	CacheReadTokens     int64 `json:"cache_read_tokens"`
	CacheCreationTokens int64 `json:"cache_creation_tokens"`
	TotalTokens         int64 `json:"total_tokens"`
}

type failDetail struct {
	StatusCode int    `json:"status_code"`
	Body       string `json:"body"`
}

func resolveFail(ctx context.Context, record coreusage.Record, failed bool) failDetail {
	fail := failDetail{
		StatusCode: record.Fail.StatusCode,
		Body:       strings.TrimSpace(record.Fail.Body),
	}
	if !failed {
		return failDetail{StatusCode: 200}
	}
	if fail.StatusCode <= 0 {
		fail.StatusCode = internallogging.GetResponseStatus(ctx)
	}
	if fail.StatusCode <= 0 {
		fail.StatusCode = 500
	}
	return fail
}

func resolveSuccess(ctx context.Context) bool {
	status := internallogging.GetResponseStatus(ctx)
	if status == 0 {
		return true
	}
	return status < httpStatusBadRequest
}

func resolveEndpoint(ctx context.Context) string {
	return strings.TrimSpace(internallogging.GetEndpoint(ctx))
}

const httpStatusBadRequest = 400
