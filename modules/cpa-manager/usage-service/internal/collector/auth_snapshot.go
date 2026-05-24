// collector - auth_snapshot.go
// 认证快照（Auth Snapshot）解析与缓存模块。
// 该模块从上游 CPA 管理接口获取认证文件列表（auth-files），
// 解析每个认证文件的账号、标签、文件名、提供商、项目 ID 等元数据，
// 并通过带 TTL 的内存缓存避免频繁调用上游 API。
// 主要用于在使用量事件入库前补充 account_snapshot 等字段信息。
package collector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// authSnapshotCacheTTL 认证快照缓存有效期。
// 在此时间窗口内重复请求将直接使用缓存数据，不会调用上游 API。
const authSnapshotCacheTTL = 30 * time.Second

// authSnapshot 表示单个认证文件的快照元数据。
// 从上游 CPA 的 auth-files 接口解析而来，用于丰富使用量事件的账户信息。
type authSnapshot struct {
	Account      string // 账号标识（如邮箱地址），用于关联使用者身份
	Label        string // 认证文件的显示名称（优先取 label、name、email）
	FileName     string // 认证文件的文件名
	Provider     string // AI 提供商标识（如 codex、gemini 等）
	ProjectID    string // 项目 ID（如 Vertex AI 项目标识）
	CapturedAtMS int64  // 快照采集时间戳（毫秒级 Unix 时间戳）
}

// authSnapshotResolver 是认证快照的解析器和缓存管理器。
// 通过带互斥锁的内存缓存实现：
// - 缓存命中时直接返回缓存数据
// - 缓存未命中或过期时重新从上游 API 获取
// - 上游请求失败且数据来源未变时，仍返回过期缓存（graceful degradation）
type authSnapshotResolver struct {
	mu            sync.Mutex             // 保护以下字段的并发访问
	client        *http.Client           // 用于调用上游 auth-files 接口的 HTTP 客户端
	baseURL       string                 // 缓存数据来源的 CPA 基础 URL
	managementKey string                 // 缓存数据来源的管理密钥
	expiresAt     time.Time              // 缓存过期时间
	snapshots     map[string]authSnapshot // 以 auth_index 为键的快照缓存
}

// newAuthSnapshotResolver 创建一个新的认证快照解析器实例。
// 内部 HTTP 客户端设置 5 秒超时，避免上游 API 无响应时长时间阻塞。
func newAuthSnapshotResolver() *authSnapshotResolver {
	return &authSnapshotResolver{
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

// lookup 根据请求的 authIndex 集合查找对应的认证快照信息。
// 查询流程：
// 1. 校验上游 URL 和管理密钥是否有效
// 2. 检查缓存是否命中（相同来源且未过期）
// 3. 缓存命中则直接从缓存中提取匹配的快照
// 4. 缓存未命中则调用上游 API 获取最新数据
// 5. 上游请求失败时，如果数据来源未变则返回过期缓存
//
// 参数：
//   - ctx: 上下文，用于控制请求超时和取消
//   - cfg: 运行时配置，包含 CPA 上游 URL 和管理密钥
//   - authIndices: 需要查询的 auth_index 集合
//
// 返回值：以 auth_index 为键的快照映射，未找到的 key 不包含在结果中。
func (r *authSnapshotResolver) lookup(ctx context.Context, cfg RuntimeConfig, authIndices map[string]struct{}) map[string]authSnapshot {
	if r == nil || len(authIndices) == 0 {
		return nil
	}
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.CPAUpstreamURL), "/")
	managementKey := strings.TrimSpace(cfg.ManagementKey)
	if baseURL == "" || managementKey == "" {
		return nil
	}

	now := time.Now()
	r.mu.Lock()
	// 检查数据来源是否发生变化
	sameSource := r.baseURL == baseURL && r.managementKey == managementKey
	// 缓存未过期且来源一致时直接返回缓存数据
	if r.baseURL == baseURL && r.managementKey == managementKey && now.Before(r.expiresAt) {
		result := r.lookupLocked(authIndices)
		r.mu.Unlock()
		return result
	}
	r.mu.Unlock()

	// 缓存过期或来源变更，重新从上游获取
	snapshots, err := r.fetch(ctx, baseURL, managementKey)
	if err != nil {
		r.mu.Lock()
		var result map[string]authSnapshot
		// 获取失败但来源未变时，返回过期缓存（降级策略）
		if sameSource {
			result = r.lookupLocked(authIndices)
		}
		r.mu.Unlock()
		return result
	}

	// 更新缓存
	r.mu.Lock()
	r.baseURL = baseURL
	r.managementKey = managementKey
	r.expiresAt = now.Add(authSnapshotCacheTTL)
	r.snapshots = snapshots
	result := r.lookupLocked(authIndices)
	r.mu.Unlock()
	return result
}

// lookupLocked 在已持有锁的情况下从缓存中提取匹配的快照数据。
// 仅返回请求的 authIndices 中存在于缓存的条目。
func (r *authSnapshotResolver) lookupLocked(authIndices map[string]struct{}) map[string]authSnapshot {
	if len(r.snapshots) == 0 {
		return nil
	}
	result := make(map[string]authSnapshot, len(authIndices))
	for authIndex := range authIndices {
		if snapshot, ok := r.snapshots[authIndex]; ok {
			result[authIndex] = snapshot
		}
	}
	return result
}

// fetch 从上游 CPA 的 /v0/management/auth-files 接口获取所有认证文件信息，
// 并解析为以 auth_index 为键的快照映射。
// 解析逻辑支持多种字段命名风格（snake_case、camelCase、kebab-case）以兼容不同版本的 CPA。
//
// 参数：
//   - ctx: 上下文，用于控制请求超时和取消
//   - baseURL: CPA 上游基础 URL
//   - managementKey: 管理密钥，通过 Bearer token 方式传递
//
// 返回值：以 auth_index 为键的快照映射；如果 auth_index 为空则跳过该条目。
func (r *authSnapshotResolver) fetch(ctx context.Context, baseURL string, managementKey string) (map[string]authSnapshot, error) {
	endpoint, err := authFilesEndpoint(baseURL)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+managementKey)

	client := r.client
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	// 校验 HTTP 响应状态码
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1024))
		return nil, errors.New("auth files request failed: " + res.Status)
	}

	// 解析响应 JSON
	var payload authFilesPayload
	decoder := json.NewDecoder(res.Body)
	if err := decoder.Decode(&payload); err != nil {
		return nil, err
	}

	// 遍历文件列表，提取每个认证文件的元数据
	capturedAt := time.Now().UnixMilli()
	snapshots := make(map[string]authSnapshot, len(payload.Files))
	for _, file := range payload.Files {
		// 读取 auth_index，支持多种字段命名
		authIndex := readAuthFileString(file, "auth_index", "authIndex", "auth-index")
		if authIndex == "" {
			continue
		}
		// 提取账号标识，排除疑似密钥的值
		account := firstSafeAccount(
			readAuthFileString(file, "account"),
			readAuthFileString(file, "email"),
		)
		// 提取显示标签，按优先级取值
		label := firstNonEmpty(
			readAuthFileString(file, "label"),
			readAuthFileString(file, "name"),
			readAuthFileString(file, "email"),
			account,
		)
		fileName := readAuthFileString(file, "name")
		// 提取提供商标识
		provider := firstNonEmpty(
			readAuthFileString(file, "provider"),
			readAuthFileString(file, "type"),
		)
		// 提取项目 ID，兼容 Gemini 虚拟项目字段
		projectID := firstNonEmpty(
			readAuthFileString(file, "project_id", "projectId"),
			readAuthFileString(file, "gemini_virtual_project", "geminiVirtualProject"),
		)
		// 如果账号为空，回退到标签或文件名
		if account == "" {
			account = firstNonEmpty(label, fileName)
		}
		snapshots[authIndex] = authSnapshot{
			Account:      account,
			Label:        label,
			FileName:     fileName,
			Provider:     provider,
			ProjectID:    projectID,
			CapturedAtMS: capturedAt,
		}
	}
	return snapshots, nil
}

// authFilesPayload 表示上游 auth-files 接口的 JSON 响应结构。
type authFilesPayload struct {
	Files []map[string]any `json:"files"` // 认证文件列表，每个元素为一个 map
}

// authFilesEndpoint 根据基础 URL 构建 auth-files 接口的完整请求地址。
// 自动处理协议前缀（缺少时默认添加 http://）和尾部斜杠清理。
func authFilesEndpoint(baseURL string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		return "", errors.New("upstream URL is empty")
	}
	if !strings.Contains(base, "://") {
		base = "http://" + base
	}
	parsed, err := url.Parse(base + "/v0/management/auth-files")
	if err != nil {
		return "", err
	}
	return parsed.String(), nil
}

// readAuthFileString 从认证文件 map 中按 keys 顺序查找第一个非空字符串值。
// 支持多个 key 以兼容不同命名风格的字段（如 snake_case、camelCase、kebab-case）。
// 返回值经过 TrimSpace 处理。
func readAuthFileString(file map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := file[key]
		if !ok || value == nil {
			continue
		}
		text := strings.TrimSpace(toString(value))
		if text != "" {
			return text
		}
	}
	return ""
}

// toString 将任意类型的值转换为字符串。
// 支持 string、json.Number 和其他类型的默认格式化。
func toString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	default:
		return fmt.Sprint(value)
	}
}

// firstNonEmpty 从多个字符串值中返回第一个非空（去除空白后）的值。
// 所有值均经过 TrimSpace 处理。
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// firstSafeAccount 从多个候选值中返回第一个非空且不像密钥的值。
// 用于防止将 API 密钥误作为账号标识使用。跳过空值和疑似密钥的值。
func firstSafeAccount(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" || looksLikeSecret(trimmed) {
			continue
		}
		return trimmed
	}
	return ""
}

// looksLikeSecret 判断给定的字符串值是否看起来像一个 API 密钥或秘密信息。
// 判断依据：
// - 包含 @ 的视为邮箱，不是密钥
// - 包含空格或路径分隔符的不是密钥
// - 以 sk- 或 AIza 开头的是密钥
// - 长度在 32-512 之间的可能是密钥
func looksLikeSecret(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || strings.Contains(trimmed, "@") {
		return false
	}
	if strings.ContainsAny(trimmed, " /\\") {
		return false
	}
	return strings.HasPrefix(trimmed, "sk-") ||
		strings.HasPrefix(trimmed, "AIza") ||
		(len(trimmed) >= 32 && len(trimmed) <= 512)
}
