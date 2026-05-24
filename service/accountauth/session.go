// session.go 实现了账号认证登录会话的内存存储管理。
// 登录会话用于跟踪 OAuth 和 Device Code 认证流程的状态，
// 保存 PKCE verifier、state 等敏感参数，避免将其暴露给前端。
// 所有会话存储在进程内存中，带自动过期清理机制。
package accountauth

import (
	"crypto/rand"
	"fmt"
	"strings"
	"sync"
	"time"
)

// defaultLoginSessionTTL 是登录会话的默认存活时间（15 分钟）。
// 超过此时间的会话将被自动清理。
const defaultLoginSessionTTL = 15 * time.Minute

// LoginSession 表示一个登录认证流程的会话状态。
// 包含提供者信息、认证模式、PKCE 参数、设备授权信息、
// 凭证结果等完整的流程上下文。
type LoginSession struct {
	SessionID       string              // 会话唯一标识（随机生成的十六进制字符串）
	AccountID       int                 // 关联的账号 ID（认证完成后填充）
	Provider        string              // 认证提供者名称（如 "codex"）
	Mode            string              // 认证模式："oauth" 或 "device"
	Status          LoginSessionStatus  // 会话状态
	StatusMessage   string              // 状态附加消息（如错误信息）
	PoolGroupID     int                 // 账号池分组 ID
	Name            string              // 用户指定的账号名称
	Options         LoginOptions        // 登录选项（代理、端口等）
	State           string              // OAuth state 参数（防 CSRF）
	Verifier        string              // PKCE code_verifier
	Challenge       string              // PKCE code_challenge
	AuthorizeURL    string              // OAuth 授权页面 URL
	DeviceAuthID    string              // Device 流程的设备授权 ID
	UserCode        string              // Device 流程的用户验证码
	VerificationURL string              // Device 流程的用户验证页面 URL
	ExpiresAt       time.Time           // 会话过期时间
	PollInterval    time.Duration       // Device 流程的轮询间隔
	Account         *AccountCredential  // 认证成功后的账号凭证
	CreatedAt       time.Time           // 会话创建时间
	UpdatedAt       time.Time           // 会话最后更新时间
}

// loginSessions 是全局的登录会话存储，使用 sync.RWMutex 保证并发安全。
// 存储结构为 sessionID -> LoginSession 的映射表。
var loginSessions = struct {
	sync.RWMutex
	items map[string]*LoginSession
}{
	items: map[string]*LoginSession{},
}

// SaveLoginSession 保存短生命周期登录会话，避免把 OAuth verifier 暴露给前端。
// 如果会话没有 SessionID，会自动生成一个；如果状态为空，默认设为 Pending；
// 如果过期时间未设置，默认设为当前时间 + 15 分钟。
// 保存前会自动清理过期的会话。返回保存的会话副本。
//
// 参数：
//   - session: 待保存的登录会话
//
// 返回：
//   - *LoginSession: 保存后的会话副本
//   - error: 参数校验错误
func SaveLoginSession(session *LoginSession) (*LoginSession, error) {
	if session == nil {
		return nil, fmt.Errorf("登录会话为空")
	}
	if strings.TrimSpace(session.Provider) == "" || strings.TrimSpace(session.Mode) == "" {
		return nil, fmt.Errorf("登录会话缺少 provider 或 mode")
	}
	now := time.Now()
	if session.SessionID == "" {
		id, err := randomHex(16)
		if err != nil {
			return nil, err
		}
		session.SessionID = id
	}
	session.Provider = normalizeProvider(session.Provider)
	session.Mode = strings.ToLower(strings.TrimSpace(session.Mode))
	if session.Status == "" {
		session.Status = LoginSessionPending
	}
	if session.CreatedAt.IsZero() {
		session.CreatedAt = now
	}
	session.UpdatedAt = now
	if session.ExpiresAt.IsZero() {
		session.ExpiresAt = now.Add(defaultLoginSessionTTL)
	}
	loginSessions.Lock()
	defer loginSessions.Unlock()
	cleanupExpiredLoginSessionsLocked(now)
	copySession := *session
	loginSessions.items[session.SessionID] = &copySession
	return &copySession, nil
}

// GetLoginSession 根据会话 ID 获取登录会话。
// 查找前会自动清理过期会话。返回会话的副本。
//
// 参数：
//   - sessionID: 会话 ID
//
// 返回：
//   - *LoginSession: 会话副本
//   - bool: 是否找到有效会话
func GetLoginSession(sessionID string) (*LoginSession, bool) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, false
	}
	now := time.Now()
	loginSessions.Lock()
	defer loginSessions.Unlock()
	cleanupExpiredLoginSessionsLocked(now)
	session, ok := loginSessions.items[sessionID]
	if !ok || session == nil {
		return nil, false
	}
	copySession := *session
	return &copySession, true
}

// FindOAuthLoginSession 通过提供者、分组 ID 和 state 参数反查 OAuth 登录会话。
// 用于 OAuth 回调时通过 state 参数找回对应的会话。
//
// 参数：
//   - provider: 提供者名称
//   - poolGroupID: 账号池分组 ID（为 0 时不参与匹配）
//   - state: OAuth state 参数
//
// 返回：
//   - *LoginSession: 找到的会话副本
//   - bool: 是否找到匹配的会话
func FindOAuthLoginSession(provider string, poolGroupID int, state string) (*LoginSession, bool) {
	provider = normalizeProvider(provider)
	state = strings.TrimSpace(state)
	if provider == "" || state == "" {
		return nil, false
	}
	now := time.Now()
	loginSessions.Lock()
	defer loginSessions.Unlock()
	cleanupExpiredLoginSessionsLocked(now)
	for _, session := range loginSessions.items {
		if session == nil || session.Provider != provider || session.Mode != "oauth" || session.State != state {
			continue
		}
		if poolGroupID > 0 && session.PoolGroupID != poolGroupID {
			continue
		}
		copySession := *session
		return &copySession, true
	}
	return nil, false
}

// UpdateLoginSession 更新已有的登录会话。
// 自动更新 UpdatedAt 时间戳。保存的是会话副本。
func UpdateLoginSession(session *LoginSession) {
	if session == nil || strings.TrimSpace(session.SessionID) == "" {
		return
	}
	session.UpdatedAt = time.Now()
	loginSessions.Lock()
	defer loginSessions.Unlock()
	copySession := *session
	loginSessions.items[session.SessionID] = &copySession
}

// SetLoginSessionAccountID 将账号 ID 关联到登录会话。
// 用于认证完成后将会话与新建的账号关联。
func SetLoginSessionAccountID(sessionID string, accountID int) {
	session, ok := GetLoginSession(sessionID)
	if !ok || session == nil {
		return
	}
	session.AccountID = accountID
	UpdateLoginSession(session)
}

// CancelLoginSession 取消登录会话。
// 将会话状态设为 Cancelled，返回是否成功取消。
func CancelLoginSession(sessionID string) bool {
	session, ok := GetLoginSession(sessionID)
	if !ok {
		return false
	}
	session.Status = LoginSessionCancelled
	session.StatusMessage = "cancelled"
	UpdateLoginSession(session)
	return true
}

// LoginSessionPublicView 将内部登录会话转换为前端可展示的视图对象。
// 过滤掉敏感信息（如 verifier、challenge），仅保留前端需要的字段。
// 时间字段转换为 Unix 时间戳（秒）。
func LoginSessionPublicView(session *LoginSession) *LoginSessionView {
	if session == nil {
		return nil
	}
	return &LoginSessionView{
		SessionID:       session.SessionID,
		AccountID:       session.AccountID,
		Provider:        session.Provider,
		Mode:            session.Mode,
		Status:          session.Status,
		StatusMessage:   session.StatusMessage,
		PoolGroupID:     session.PoolGroupID,
		Name:            session.Name,
		AuthorizeURL:    session.AuthorizeURL,
		VerificationURL: session.VerificationURL,
		UserCode:        session.UserCode,
		ExpiresAt:       session.ExpiresAt.Unix(),
		PollInterval:    int64(session.PollInterval.Seconds()),
		CreatedAt:       session.CreatedAt.Unix(),
		UpdatedAt:       session.UpdatedAt.Unix(),
		Account:         session.Account,
	}
}

// cleanupExpiredLoginSessionsLocked 清理过期的登录会话。
// 在持有写锁的情况下调用。过期时间超过 1 分钟的会话会被删除。
func cleanupExpiredLoginSessionsLocked(now time.Time) {
	for id, session := range loginSessions.items {
		if session == nil || (!session.ExpiresAt.IsZero() && now.After(session.ExpiresAt.Add(time.Minute))) {
			delete(loginSessions.items, id)
		}
	}
}

// randomHex 生成指定字节数的随机十六进制字符串。
// 用于生成会话 ID、OAuth state 等安全随机值。
func randomHex(nBytes int) (string, error) {
	if nBytes <= 0 {
		return "", fmt.Errorf("invalid random bytes length")
	}
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", b), nil
}
