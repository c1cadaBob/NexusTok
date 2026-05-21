package accountauth

import (
	"crypto/rand"
	"fmt"
	"strings"
	"sync"
	"time"
)

const defaultLoginSessionTTL = 15 * time.Minute

type LoginSession struct {
	SessionID       string
	AccountID       int
	Provider        string
	Mode            string
	Status          LoginSessionStatus
	StatusMessage   string
	PoolGroupID     int
	Name            string
	Options         LoginOptions
	State           string
	Verifier        string
	Challenge       string
	AuthorizeURL    string
	DeviceAuthID    string
	UserCode        string
	VerificationURL string
	ExpiresAt       time.Time
	PollInterval    time.Duration
	Account         *AccountCredential
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

var loginSessions = struct {
	sync.RWMutex
	items map[string]*LoginSession
}{
	items: map[string]*LoginSession{},
}

// SaveLoginSession 保存短生命周期登录会话，避免把 OAuth verifier 暴露给前端。
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

func SetLoginSessionAccountID(sessionID string, accountID int) {
	session, ok := GetLoginSession(sessionID)
	if !ok || session == nil {
		return
	}
	session.AccountID = accountID
	UpdateLoginSession(session)
}

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

func cleanupExpiredLoginSessionsLocked(now time.Time) {
	for id, session := range loginSessions.items {
		if session == nil || (!session.ExpiresAt.IsZero() && now.After(session.ExpiresAt.Add(time.Minute))) {
			delete(loginSessions.items, id)
		}
	}
}

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
