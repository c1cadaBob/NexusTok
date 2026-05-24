// 包 auth - types.go
// 该文件定义了认证系统的核心数据类型，包括 Auth 凭据结构体、QuotaState 配额状态、
// ModelState 模型状态，以及相关的克隆、索引、过期时间解析等功能。
package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	baseauth "github.com/router-for-me/CLIProxyAPI/v7/internal/auth"
)

// PostAuthHook 定义在 Auth 记录创建后但持久化到存储前调用的钩子函数。
// 允许基于外部上下文修改 Auth 记录（如注入元数据）。
type PostAuthHook func(context.Context, *Auth) error

// RequestInfo 持有从 HTTP 请求中提取的信息。
// 它被注入到传递给 PostAuthHook 的上下文中。
type RequestInfo struct {
	Query   url.Values // 请求查询参数
	Headers http.Header // 请求头
}

type requestInfoKey struct{} // 上下文键类型，用于存储 RequestInfo

// WithRequestInfo 返回附加了 RequestInfo 的新上下文。
func WithRequestInfo(ctx context.Context, info *RequestInfo) context.Context {
	return context.WithValue(ctx, requestInfoKey{}, info)
}

// GetRequestInfo 从上下文中检索 RequestInfo（如果存在）。
func GetRequestInfo(ctx context.Context) *RequestInfo {
	if val, ok := ctx.Value(requestInfoKey{}).(*RequestInfo); ok {
		return val
	}
	return nil
}

// Auth 封装了与单个凭据关联的运行时状态和元数据。
type Auth struct {
	ID               string                 `json:"id"`                // ID 在重启间唯一标识认证记录
	Index            string                 `json:"-"`                 // Index 是从认证元数据派生的稳定运行时标识（不持久化）
	Provider         string                 `json:"provider"`          // Provider 是上游提供商键（如 "gemini"、"claude"）
	Prefix           string                 `json:"prefix,omitempty"`  // Prefix 可选地为模型路由添加命名空间前缀
	FileName         string                 `json:"-"`                 // FileName 存储认证文件的相对或绝对路径
	Storage          baseauth.TokenStorage  `json:"-"`                 // Storage 持有登录流程中使用的令牌持久化实现
	Label            string                 `json:"label,omitempty"`   // Label 是可选的人类可读标签，用于日志记录
	Status           Status                 `json:"status"`            // Status 是 AuthManager 管理的生命周期状态
	StatusMessage    string                 `json:"status_message,omitempty"` // StatusMessage 持有当前状态的简短描述
	Disabled         bool                   `json:"disabled"`          // Disabled 表示认证被运营者有意禁用
	Unavailable      bool                   `json:"unavailable"`       // Unavailable 标记临时的提供商不可用（如配额超限）
	ProxyURL         string                 `json:"proxy_url,omitempty"` // ProxyURL 如果提供，覆盖此认证的全局代理设置
	Attributes       map[string]string      `json:"attributes,omitempty"` // Attributes 存储执行器所需的提供商特定元数据（不可变配置）
	Metadata         map[string]any         `json:"metadata,omitempty"`   // Metadata 存储运行时可变的提供商状态（如令牌、Cookie）
	Quota            QuotaState             `json:"quota"`             // Quota 捕获负载均衡器使用的最近配额信息
	LastError        *Error                 `json:"last_error,omitempty"` // LastError 存储执行或刷新时遇到的最后一次失败
	CreatedAt        time.Time              `json:"created_at"`        // CreatedAt 是创建时间戳（UTC）
	UpdatedAt        time.Time              `json:"updated_at"`        // UpdatedAt 是最后修改时间戳（UTC）
	LastRefreshedAt  time.Time              `json:"last_refreshed_at"` // LastRefreshedAt 记录最后一次成功刷新的时间（UTC）
	NextRefreshAfter time.Time              `json:"next_refresh_after"` // NextRefreshAfter 是应重新触发刷新的最早时间
	NextRetryAfter   time.Time              `json:"next_retry_after"`  // NextRetryAfter 是应重新触发重试的最早时间
	ModelStates      map[string]*ModelState `json:"model_states,omitempty"` // ModelStates 跟踪每个模型的运行时可用性数据

	Runtime any `json:"-"` // Runtime 携带执行期间使用的不可序列化数据（仅内存中）

	Success int64 `json:"-"` // 成功请求计数
	Failed  int64 `json:"-"` // 失败请求计数

	recentRequests recentRequestRing `json:"-"` // 最近请求的环形缓冲区
	indexAssigned  bool              `json:"-"` // 索引是否已分配
}

const (
	recentRequestBucketSeconds int64 = 10 * 60 // 每个请求桶的时间跨度（10 分钟）
	recentRequestBucketCount         = 20      // 环形缓冲区中的桶数量
)

// recentRequestBucket 存储单个时间桶内的请求统计。
type recentRequestBucket struct {
	bucketID int64 // 桶标识（时间戳除以桶跨度）
	success  int64 // 成功请求数
	failed   int64 // 失败请求数
}

// recentRequestRing 是最近请求统计的环形缓冲区。
type recentRequestRing struct {
	buckets [recentRequestBucketCount]recentRequestBucket
}

// RecentRequestBucket 是最近请求桶的可序列化表示，用于 API 返回。
type RecentRequestBucket struct {
	Time    string `json:"time"`    // 时间范围标签（如 "15:00-15:10"）
	Success int64  `json:"success"` // 成功请求数
	Failed  int64  `json:"failed"`  // 失败请求数
}

// QuotaState 包含凭据的限制器跟踪数据。
type QuotaState struct {
	Exceeded     bool      `json:"exceeded"`                // Exceeded 表示凭据最近遇到了配额错误
	Reason       string    `json:"reason,omitempty"`        // Reason 提供可选的提供商特定人类可读描述
	NextRecoverAt time.Time `json:"next_recover_at"`         // NextRecoverAt 是凭据可能再次可用的时间
	BackoffLevel int       `json:"backoff_level,omitempty"` // BackoffLevel 存储用于速率限制的渐进冷却指数
}

// ModelState 捕获认证条目下特定模型的执行状态。
type ModelState struct {
	Status         Status     `json:"status"`                    // Status 反映此模型的生命周期状态
	StatusMessage  string     `json:"status_message,omitempty"`  // StatusMessage 提供状态的可选简短描述
	Unavailable    bool       `json:"unavailable"`               // Unavailable 反映模型是否被临时阻止重试
	NextRetryAfter time.Time  `json:"next_retry_after"`          // NextRetryAfter 定义每模型的重试时间
	LastError      *Error     `json:"last_error,omitempty"`      // LastError 记录此模型观察到的最新错误
	Quota          QuotaState `json:"quota"`                     // Quota 保留此模型遇到速率限制时的配额信息
	UpdatedAt      time.Time  `json:"updated_at"`                // UpdatedAt 跟踪此模型状态的最后更新时间戳
}

// recentRequestBucketID 根据当前时间计算请求桶标识。
func recentRequestBucketID(now time.Time) int64 {
	if now.IsZero() {
		return 0
	}
	return now.Unix() / recentRequestBucketSeconds
}

// recentRequestBucketIndex 计算桶标识在环形缓冲区中的索引。
func recentRequestBucketIndex(bucketID int64) int {
	mod := bucketID % int64(recentRequestBucketCount)
	if mod < 0 {
		mod += int64(recentRequestBucketCount)
	}
	return int(mod)
}

// formatRecentRequestBucketLabel 格式化请求桶的时间范围标签（如 "15:00-15:10"）。
func formatRecentRequestBucketLabel(bucketID int64) string {
	start := time.Unix(bucketID*recentRequestBucketSeconds, 0).In(time.Local)
	end := start.Add(time.Duration(recentRequestBucketSeconds) * time.Second)
	return start.Format("15:04") + "-" + end.Format("15:04")
}

// recordRecentRequest 记录一次最近请求到环形缓冲区。
func (a *Auth) recordRecentRequest(now time.Time, success bool) {
	if a == nil {
		return
	}
	bucketID := recentRequestBucketID(now)
	idx := recentRequestBucketIndex(bucketID)
	bucket := &a.recentRequests.buckets[idx]
	if bucket.bucketID != bucketID {
		bucket.bucketID = bucketID
		bucket.success = 0
		bucket.failed = 0
	}
	if success {
		bucket.success++
		return
	}
	bucket.failed++
}

// RecentRequestsSnapshot 返回最近请求统计的快照，用于 API 返回。
func (a *Auth) RecentRequestsSnapshot(now time.Time) []RecentRequestBucket {
	out := make([]RecentRequestBucket, 0, recentRequestBucketCount)
	if a == nil {
		return out
	}

	currentBucketID := recentRequestBucketID(now)
	for i := recentRequestBucketCount - 1; i >= 0; i-- {
		bucketID := currentBucketID - int64(i)
		idx := recentRequestBucketIndex(bucketID)
		bucket := a.recentRequests.buckets[idx]
		entry := RecentRequestBucket{
			Time: formatRecentRequestBucketLabel(bucketID),
		}
		if bucket.bucketID == bucketID {
			entry.Success = bucket.success
			entry.Failed = bucket.failed
		}
		out = append(out, entry)
	}

	return out
}

// Clone 浅拷贝 Auth 结构体，复制 map 以避免意外修改。
func (a *Auth) Clone() *Auth {
	if a == nil {
		return nil
	}
	copyAuth := *a
	if len(a.Attributes) > 0 {
		copyAuth.Attributes = make(map[string]string, len(a.Attributes))
		for key, value := range a.Attributes {
			copyAuth.Attributes[key] = value
		}
	}
	if len(a.Metadata) > 0 {
		copyAuth.Metadata = make(map[string]any, len(a.Metadata))
		for key, value := range a.Metadata {
			copyAuth.Metadata[key] = value
		}
	}
	if len(a.ModelStates) > 0 {
		copyAuth.ModelStates = make(map[string]*ModelState, len(a.ModelStates))
		for key, state := range a.ModelStates {
			copyAuth.ModelStates[key] = state.Clone()
		}
	}
	copyAuth.Runtime = a.Runtime
	return &copyAuth
}

// stableAuthIndex 根据种子字符串生成稳定的认证索引（SHA256 前 8 字节的十六进制）。
func stableAuthIndex(seed string) string {
	seed = strings.TrimSpace(seed)
	if seed == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(sum[:8])
}

// indexSeed 计算认证记录的索引种子字符串。
// 优先使用文件路径，其次使用 API 密钥，最后使用 ID。
func (a *Auth) indexSeed() string {
	if a == nil {
		return ""
	}

	provider := strings.ToLower(strings.TrimSpace(a.Provider))
	compatName := ""
	baseURL := ""
	apiKey := ""
	filePath := ""
	if a.Attributes != nil {
		compatName = strings.TrimSpace(a.Attributes["compat_name"])
		baseURL = strings.TrimSpace(a.Attributes["base_url"])
		apiKey = strings.TrimSpace(a.Attributes["api_key"])
		filePath = strings.TrimSpace(a.Attributes["path"])
		if filePath == "" {
			filePath = strings.TrimSpace(a.Attributes["source"])
		}
	}

	if filePath == "" {
		filePath = strings.TrimSpace(a.FileName)
	}
	if filePath == "" {
		filePath = strings.TrimSpace(a.ID)
	}

	if filePath != "" && strings.HasSuffix(strings.ToLower(filePath), ".json") {
		abs, errAbs := filepath.Abs(filePath)
		if errAbs == nil && strings.TrimSpace(abs) != "" {
			filePath = abs
		}
		filePath = filepath.Clean(filePath)

		authType := ""
		if a.Metadata != nil {
			if rawType, ok := a.Metadata["type"].(string); ok {
				authType = strings.TrimSpace(rawType)
			}
		}
		if authType == "" {
			authType = strings.TrimSpace(provider)
		}
		authType = strings.ToLower(strings.TrimSpace(authType))
		if authType != "" {
			return authType + ":" + filePath
		}
	}

	apiPrefix := ""
	if apiKey != "" {
		switch {
		case compatName != "" || strings.EqualFold(provider, "openai-compatibility"):
			apiPrefix = "openai-compatibility"
		case strings.EqualFold(provider, "gemini"):
			apiPrefix = "gemini-api-key"
		case strings.EqualFold(provider, "codex"):
			apiPrefix = "codex-api-key"
		case strings.EqualFold(provider, "claude"):
			apiPrefix = "claude-api-key"
		}
	}
	if apiPrefix != "" {
		return apiPrefix + ":" + strings.TrimSpace(baseURL) + "+" + strings.TrimSpace(apiKey)
	}

	if id := strings.TrimSpace(a.ID); id != "" {
		return "id:" + id
	}

	return ""
}

// EnsureIndex 返回从认证文件名或凭据标识派生的稳定索引。
func (a *Auth) EnsureIndex() string {
	if a == nil {
		return ""
	}
	if a.indexAssigned && a.Index != "" {
		return a.Index
	}

	seed := a.indexSeed()
	if seed == "" {
		return ""
	}

	idx := stableAuthIndex(seed)
	a.Index = idx
	a.indexAssigned = true
	return idx
}

// Clone 复制模型状态，包括嵌套的错误详情。
func (m *ModelState) Clone() *ModelState {
	if m == nil {
		return nil
	}
	copyState := *m
	if m.LastError != nil {
		copyState.LastError = &Error{
			Code:       m.LastError.Code,
			Message:    m.LastError.Message,
			Retryable:  m.LastError.Retryable,
			HTTPStatus: m.LastError.HTTPStatus,
		}
	}
	return &copyState
}

// ProxyInfo 返回代理信息描述字符串，用于日志记录。
func (a *Auth) ProxyInfo() string {
	if a == nil {
		return ""
	}
	proxyStr := strings.TrimSpace(a.ProxyURL)
	if proxyStr == "" {
		return ""
	}
	if idx := strings.Index(proxyStr, "://"); idx > 0 {
		return "via " + proxyStr[:idx] + " proxy"
	}
	return "via proxy"
}

// DisableCoolingOverride 返回认证范围的禁用冷却覆盖值。
// 从元数据键 "disable_cooling"（或旧版 "disable-cooling"）读取。
// 注意：此覆盖为"仅 true"模式。当元数据值为 false 时视为"未设置"，
// 以便全局禁用冷却标志仍然生效。
//
// 返回:
//   - bool: 覆盖值
//   - bool: 是否存在覆盖
func (a *Auth) DisableCoolingOverride() (bool, bool) {
	if a == nil || a.Metadata == nil {
		return false, false
	}
	if val, ok := a.Metadata["disable_cooling"]; ok {
		if parsed, okParse := parseBoolAny(val); okParse {
			if !parsed {
				return false, false
			}
			return parsed, true
		}
	}
	if val, ok := a.Metadata["disable-cooling"]; ok {
		if parsed, okParse := parseBoolAny(val); okParse {
			if !parsed {
				return false, false
			}
			return parsed, true
		}
	}
	return false, false
}

// ToolPrefixDisabled 返回是否应跳过 proxy_ 工具名称前缀。
// 为 true 时，工具名称将原样发送给 Anthropic。
// 从元数据键 "tool_prefix_disabled"（或 "tool-prefix-disabled"）读取。
func (a *Auth) ToolPrefixDisabled() bool {
	if a == nil || a.Metadata == nil {
		return false
	}
	for _, key := range []string{"tool_prefix_disabled", "tool-prefix-disabled"} {
		if val, ok := a.Metadata[key]; ok {
			if parsed, okParse := parseBoolAny(val); okParse {
				return parsed
			}
		}
	}
	return false
}

// RequestRetryOverride 返回认证文件范围的请求重试覆盖值。
// 从元数据键 "request_retry"（或旧版 "request-retry"）读取。
func (a *Auth) RequestRetryOverride() (int, bool) {
	if a == nil || a.Metadata == nil {
		return 0, false
	}
	if val, ok := a.Metadata["request_retry"]; ok {
		if parsed, okParse := parseIntAny(val); okParse {
			if parsed < 0 {
				parsed = 0
			}
			return parsed, true
		}
	}
	if val, ok := a.Metadata["request-retry"]; ok {
		if parsed, okParse := parseIntAny(val); okParse {
			if parsed < 0 {
				parsed = 0
			}
			return parsed, true
		}
	}
	return 0, false
}

// parseBoolAny 从任意类型值中解析布尔值。
// 支持 bool、string、float64 和 json.Number 类型。
func parseBoolAny(val any) (bool, bool) {
	switch typed := val.(type) {
	case bool:
		return typed, true
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return false, false
		}
		parsed, err := strconv.ParseBool(trimmed)
		if err != nil {
			return false, false
		}
		return parsed, true
	case float64:
		return typed != 0, true
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return false, false
		}
		return parsed != 0, true
	default:
		return false, false
	}
}

// parseIntAny 从任意类型值中解析整数值。
// 支持 int、int32、int64、float64、json.Number 和 string 类型。
func parseIntAny(val any) (int, bool) {
	switch typed := val.(type) {
	case int:
		return typed, true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return 0, false
		}
		return int(parsed), true
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return 0, false
		}
		parsed, err := strconv.Atoi(trimmed)
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

// AccountInfo 返回认证账户的类型和标识信息。
// 优先检查 OAuth 邮箱，其次检查 API 密钥。
func (a *Auth) AccountInfo() (string, string) {
	if a == nil {
		return "", ""
	}
	// For Gemini CLI, include project ID in the OAuth account info if present.
	if strings.ToLower(a.Provider) == "gemini-cli" {
		if a.Metadata != nil {
			email, _ := a.Metadata["email"].(string)
			email = strings.TrimSpace(email)
			if email != "" {
				if p, ok := a.Metadata["project_id"].(string); ok {
					p = strings.TrimSpace(p)
					if p != "" {
						return "oauth", email + " (" + p + ")"
					}
				}
				return "oauth", email
			}
		}
	}

	// Check metadata for email first (OAuth-style auth)
	if a.Metadata != nil {
		if v, ok := a.Metadata["email"].(string); ok {
			email := strings.TrimSpace(v)
			if email != "" {
				return "oauth", email
			}
		}
	}
	// Fall back to API key (API-key auth)
	if a.Attributes != nil {
		if v := a.Attributes["api_key"]; v != "" {
			return "api_key", v
		}
	}
	return "", ""
}

// ExpirationTime 尝试从元数据中提取凭据过期时间戳。
// 检查常见键如 "expired"、"expire"、"expires_at"，以及嵌套的 "token" 对象，
// 以保持与旧版认证文件格式的兼容性。
func (a *Auth) ExpirationTime() (time.Time, bool) {
	if a == nil {
		return time.Time{}, false
	}
	if ts, ok := expirationFromMap(a.Metadata); ok {
		return ts, true
	}
	return time.Time{}, false
}

var (
	refreshLeadMu        sync.RWMutex                          // 保护刷新前导时间工厂映射的读写锁
	refreshLeadFactories = make(map[string]func() *time.Duration) // 提供商到刷新前导时间工厂的映射
)

// RegisterRefreshLeadProvider 注册提供商的刷新前导时间工厂函数。
func RegisterRefreshLeadProvider(provider string, factory func() *time.Duration) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" || factory == nil {
		return
	}
	refreshLeadMu.Lock()
	refreshLeadFactories[provider] = factory
	refreshLeadMu.Unlock()
}

// expireKeys 是过期时间元数据键的列表。
var expireKeys = [...]string{"expired", "expire", "expires_at", "expiresAt", "expiry", "expires"}

// expirationFromMap 从元数据映射中递归查找过期时间。
// 支持直接键和嵌套在 "token"/"Token" 对象中的键。
func expirationFromMap(meta map[string]any) (time.Time, bool) {
	if meta == nil {
		return time.Time{}, false
	}
	for _, key := range expireKeys {
		if v, ok := meta[key]; ok {
			if ts, ok1 := parseTimeValue(v); ok1 {
				return ts, true
			}
		}
	}
	for _, nestedKey := range []string{"token", "Token"} {
		if nested, ok := meta[nestedKey]; ok {
			switch val := nested.(type) {
			case map[string]any:
				if ts, ok1 := expirationFromMap(val); ok1 {
					return ts, true
				}
			case map[string]string:
				temp := make(map[string]any, len(val))
				for k, v := range val {
					temp[k] = v
				}
				if ts, ok1 := expirationFromMap(temp); ok1 {
					return ts, true
				}
			}
		}
	}
	return time.Time{}, false
}

// ProviderRefreshLead 获取提供商的刷新前导时间。
// 优先从运行时对象获取，其次从注册的工厂函数获取。
func ProviderRefreshLead(provider string, runtime any) *time.Duration {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if runtime != nil {
		if eval, ok := runtime.(interface{ RefreshLead() *time.Duration }); ok {
			if lead := eval.RefreshLead(); lead != nil && *lead > 0 {
				return lead
			}
		}
	}
	refreshLeadMu.RLock()
	factory := refreshLeadFactories[provider]
	refreshLeadMu.RUnlock()
	if factory == nil {
		return nil
	}
	if lead := factory(); lead != nil && *lead > 0 {
		return lead
	}
	return nil
}

// parseTimeValue 从任意类型值中解析时间。
// 支持多种时间格式字符串和 Unix 时间戳（秒/毫秒）。
func parseTimeValue(v any) (time.Time, bool) {
	switch value := v.(type) {
	case string:
		s := strings.TrimSpace(value)
		if s == "" {
			return time.Time{}, false
		}
		layouts := []string{
			time.RFC3339,
			time.RFC3339Nano,
			"2006-01-02 15:04:05",
			"2006-01-02 15:04",
			"2006-01-02T15:04:05Z07:00",
		}
		for _, layout := range layouts {
			if ts, err := time.Parse(layout, s); err == nil {
				return ts, true
			}
		}
		if unix, err := strconv.ParseInt(s, 10, 64); err == nil {
			return normaliseUnix(unix), true
		}
	case float64:
		return normaliseUnix(int64(value)), true
	case int64:
		return normaliseUnix(value), true
	case json.Number:
		if i, err := value.Int64(); err == nil {
			return normaliseUnix(i), true
		}
		if f, err := value.Float64(); err == nil {
			return normaliseUnix(int64(f)), true
		}
	}
	return time.Time{}, false
}

// normaliseUnix 将 Unix 时间戳规范化为 time.Time。
// 启发式：大于 1e12 的值视为毫秒精度。
func normaliseUnix(raw int64) time.Time {
	if raw <= 0 {
		return time.Time{}
	}
	// Heuristic: treat values with millisecond precision (>1e12) accordingly.
	if raw > 1_000_000_000_000 {
		return time.UnixMilli(raw)
	}
	return time.Unix(raw, 0)
}
