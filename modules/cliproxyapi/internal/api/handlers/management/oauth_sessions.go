// management - oauth_sessions.go
// OAuth 会话状态管理模块。
// 该模块实现了基于内存的 OAuth 会话状态机，用于跟踪 OAuth 认证流程的生命周期。
// 会话状态包括：pending（等待回调）、completed（已完成）、error（出错）。
// 支持基于 TTL 的自动过期清理，防止内存泄漏。
package management

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// OAuth 会话相关的常量定义。
const (
	oauthSessionTTL     = 10 * time.Minute // OAuth 会话的默认存活时间
	maxOAuthStateLength = 128              // state 参数的最大允许长度
)

// OAuth 会话相关的错误定义。
var (
	errInvalidOAuthState      = errors.New("invalid oauth state")          // state 参数格式无效
	errUnsupportedOAuthFlow   = errors.New("unsupported oauth provider")   // 不支持的 OAuth 提供者
	errOAuthSessionNotPending = errors.New("oauth session is not pending") // 会话不在 pending 状态
)

// oauthSession 表示一个 OAuth 会话的状态信息。
// 会话的状态转换：pending -> completed（成功）或 error（失败）。
type oauthSession struct {
	Provider  string    // OAuth 提供者名称（如 anthropic、gemini、codex）
	Status    string    // 会话状态：空字符串表示 pending，非空表示错误信息
	CreatedAt time.Time // 会话创建时间
	ExpiresAt time.Time // 会话过期时间，超过此时间将被自动清理
}

// oauthSessionStore 是线程安全的 OAuth 会话存储。
// 使用 map 存储以 state 为键的会话信息，通过 RWMutex 保护并发访问。
type oauthSessionStore struct {
	mu       sync.RWMutex          // 读写锁，保护 sessions 的并发访问
	ttl      time.Duration         // 会话默认存活时间
	sessions map[string]oauthSession // 以 state 为键的会话映射表
}

// newOAuthSessionStore 创建一个新的 OAuth 会话存储实例。
// 如果传入的 ttl <= 0，则使用默认的 oauthSessionTTL。
func newOAuthSessionStore(ttl time.Duration) *oauthSessionStore {
	if ttl <= 0 {
		ttl = oauthSessionTTL
	}
	return &oauthSessionStore{
		ttl:      ttl,
		sessions: make(map[string]oauthSession),
	}
}

// purgeExpiredLocked 清理所有已过期的会话记录。
// 调用前必须已持有写锁。遍历所有会话，删除超过过期时间的记录。
func (s *oauthSessionStore) purgeExpiredLocked(now time.Time) {
	for state, session := range s.sessions {
		if !session.ExpiresAt.IsZero() && now.After(session.ExpiresAt) {
			delete(s.sessions, state)
		}
	}
}

// Register 注册一个新的 OAuth 会话。
// 会话初始状态为 pending（Status 为空字符串），设置创建时间和过期时间。
// 如果 state 或 provider 为空，则静默忽略。同时会清理过期的会话。
func (s *oauthSessionStore) Register(state, provider string) {
	state = strings.TrimSpace(state)
	provider = strings.ToLower(strings.TrimSpace(provider))
	if state == "" || provider == "" {
		return
	}
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	s.purgeExpiredLocked(now)
	s.sessions[state] = oauthSession{
		Provider:  provider,
		Status:    "",
		CreatedAt: now,
		ExpiresAt: now.Add(s.ttl),
	}
}

// SetError 将指定会话标记为错误状态。
// 如果会话不存在或 state 为空，则静默忽略。
// 错误消息为空时使用默认值 "Authentication failed"。
// 会重置会话的过期时间，给予前端足够时间读取错误信息。
func (s *oauthSessionStore) SetError(state, message string) {
	state = strings.TrimSpace(state)
	message = strings.TrimSpace(message)
	if state == "" {
		return
	}
	if message == "" {
		message = "Authentication failed"
	}
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	s.purgeExpiredLocked(now)
	session, ok := s.sessions[state]
	if !ok {
		return
	}
	session.Status = message
	session.ExpiresAt = now.Add(s.ttl)
	s.sessions[state] = session
}

// Complete 将指定会话标记为已完成并从存储中删除。
// 如果 state 为空或会话不存在，则静默忽略。
func (s *oauthSessionStore) Complete(state string) {
	state = strings.TrimSpace(state)
	if state == "" {
		return
	}
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	s.purgeExpiredLocked(now)
	delete(s.sessions, state)
}

// CompleteProvider 删除指定提供者的所有会话。
// 返回被删除的会话数量。用于批量清理某个提供者的所有 OAuth 流程。
func (s *oauthSessionStore) CompleteProvider(provider string) int {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		return 0
	}
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	s.purgeExpiredLocked(now)
	removed := 0
	for state, session := range s.sessions {
		if strings.EqualFold(session.Provider, provider) {
			delete(s.sessions, state)
			removed++
		}
	}
	return removed
}

// Get 获取指定 state 对应的 OAuth 会话信息。
// 返回会话对象和是否存在。如果会话已过期会被自动清理并返回不存在。
func (s *oauthSessionStore) Get(state string) (oauthSession, bool) {
	state = strings.TrimSpace(state)
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	s.purgeExpiredLocked(now)
	session, ok := s.sessions[state]
	return session, ok
}

// IsPending 检查指定 state 的会话是否处于 pending 状态。
// pending 状态意味着会话存在且 Status 为空（未被标记为错误或完成）。
// 如果指定了 provider，还会验证会话的提供者是否匹配。
func (s *oauthSessionStore) IsPending(state, provider string) bool {
	state = strings.TrimSpace(state)
	provider = strings.ToLower(strings.TrimSpace(provider))
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	s.purgeExpiredLocked(now)
	session, ok := s.sessions[state]
	if !ok {
		return false
	}
	if session.Status != "" {
		return false
	}
	if provider == "" {
		return true
	}
	return strings.EqualFold(session.Provider, provider)
}

// oauthSessions 是全局的 OAuth 会话存储实例，使用默认 TTL。
var oauthSessions = newOAuthSessionStore(oauthSessionTTL)

// RegisterOAuthSession 是 Register 的包级别便捷函数。
func RegisterOAuthSession(state, provider string) { oauthSessions.Register(state, provider) }

// SetOAuthSessionError 是 SetError 的包级别便捷函数。
func SetOAuthSessionError(state, message string) { oauthSessions.SetError(state, message) }

// CompleteOAuthSession 是 Complete 的包级别便捷函数。
func CompleteOAuthSession(state string) { oauthSessions.Complete(state) }

// CompleteOAuthSessionsByProvider 是 CompleteProvider 的包级别便捷函数。
func CompleteOAuthSessionsByProvider(provider string) int {
	return oauthSessions.CompleteProvider(provider)
}

// GetOAuthSession 是 Get 的包级别便捷函数，返回提供者名称、状态和是否存在。
func GetOAuthSession(state string) (provider string, status string, ok bool) {
	session, ok := oauthSessions.Get(state)
	if !ok {
		return "", "", false
	}
	return session.Provider, session.Status, true
}

// IsOAuthSessionPending 是 IsPending 的包级别便捷函数。
func IsOAuthSessionPending(state, provider string) bool {
	return oauthSessions.IsPending(state, provider)
}

// oauthSessionErrorWithCause 构造带原因的 OAuth 会话错误消息。
// 将主消息和原因错误拼接为 "message: cause" 格式。
func oauthSessionErrorWithCause(message string, cause error) string {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "Authentication failed"
	}
	if cause == nil {
		return message
	}
	detail := strings.TrimSpace(cause.Error())
	if detail == "" {
		return message
	}
	return message + ": " + detail
}

// ValidateOAuthState 验证 OAuth state 参数的格式安全性。
// 验证规则：
//   - 不能为空
//   - 长度不超过 maxOAuthStateLength
//   - 不能包含路径分隔符（/、\）
//   - 不能包含 ".."（防止路径遍历攻击）
//   - 只能包含字母、数字、连字符、下划线和点号
func ValidateOAuthState(state string) error {
	trimmed := strings.TrimSpace(state)
	if trimmed == "" {
		return fmt.Errorf("%w: empty", errInvalidOAuthState)
	}
	if len(trimmed) > maxOAuthStateLength {
		return fmt.Errorf("%w: too long", errInvalidOAuthState)
	}
	if strings.Contains(trimmed, "/") || strings.Contains(trimmed, "\\") {
		return fmt.Errorf("%w: contains path separator", errInvalidOAuthState)
	}
	if strings.Contains(trimmed, "..") {
		return fmt.Errorf("%w: contains '..'", errInvalidOAuthState)
	}
	for _, r := range trimmed {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_' || r == '.':
		default:
			return fmt.Errorf("%w: invalid character", errInvalidOAuthState)
		}
	}
	return nil
}

// NormalizeOAuthProvider 将各种 OAuth 提供者名称标准化为内部使用的规范名称。
// 支持的映射：
//   - anthropic/claude -> anthropic
//   - codex/openai -> codex
//   - gemini/google -> gemini
//   - antigravity/anti-gravity -> antigravity
//   - xai/x-ai/x.ai/grok -> xai
//
// 不支持的提供者返回 errUnsupportedOAuthFlow 错误。
func NormalizeOAuthProvider(provider string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "anthropic", "claude":
		return "anthropic", nil
	case "codex", "openai":
		return "codex", nil
	case "gemini", "google":
		return "gemini", nil
	case "antigravity", "anti-gravity":
		return "antigravity", nil
	case "xai", "x-ai", "x.ai", "grok":
		return "xai", nil
	default:
		return "", errUnsupportedOAuthFlow
	}
}

// oauthCallbackFilePayload 表示写入 OAuth 回调文件的 JSON 负载。
type oauthCallbackFilePayload struct {
	Code  string `json:"code"`  // OAuth 授权码
	State string `json:"state"` // OAuth state 参数
	Error string `json:"error"` // OAuth 错误信息（如有）
}

// WriteOAuthCallbackFile 将 OAuth 回调数据写入文件。
// 文件命名为 ".oauth-{provider}-{state}.oauth"，写入认证目录（authDir）。
// 文件权限为 0600（仅所有者可读写），确保安全性。
// 返回写入的文件路径和可能的错误。
func WriteOAuthCallbackFile(authDir, provider, state, code, errorMessage string) (string, error) {
	if strings.TrimSpace(authDir) == "" {
		return "", fmt.Errorf("auth dir is empty")
	}
	canonicalProvider, err := NormalizeOAuthProvider(provider)
	if err != nil {
		return "", err
	}
	if err := ValidateOAuthState(state); err != nil {
		return "", err
	}

	fileName := fmt.Sprintf(".oauth-%s-%s.oauth", canonicalProvider, state)
	filePath := filepath.Join(authDir, fileName)
	payload := oauthCallbackFilePayload{
		Code:  strings.TrimSpace(code),
		State: strings.TrimSpace(state),
		Error: strings.TrimSpace(errorMessage),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal oauth callback payload: %w", err)
	}
	if err := os.WriteFile(filePath, data, 0o600); err != nil {
		return "", fmt.Errorf("write oauth callback file: %w", err)
	}
	return filePath, nil
}

// WriteOAuthCallbackFileForPendingSession 为处于 pending 状态的 OAuth 会话写入回调文件。
// 首先验证会话是否处于 pending 状态，如果不是则返回 errOAuthSessionNotPending 错误。
// 验证通过后委托给 WriteOAuthCallbackFile 执行实际的文件写入。
func WriteOAuthCallbackFileForPendingSession(authDir, provider, state, code, errorMessage string) (string, error) {
	canonicalProvider, err := NormalizeOAuthProvider(provider)
	if err != nil {
		return "", err
	}
	if !IsOAuthSessionPending(state, canonicalProvider) {
		return "", errOAuthSessionNotPending
	}
	return WriteOAuthCallbackFile(authDir, canonicalProvider, state, code, errorMessage)
}
