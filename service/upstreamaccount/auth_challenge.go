package upstreamaccount

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/pkg/cachex"

	"github.com/samber/hot"
)

const (
	authChallengeCacheNamespace = "upstream-account-auth-challenge"
	authChallengeTTL            = 5 * time.Minute
	authChallengeTypeTOTP       = "totp"
)

var authChallengeCache = cachex.NewHybridCache[AuthChallengeRecord](cachex.HybridCacheConfig[AuthChallengeRecord]{
	Namespace:    cachex.Namespace(authChallengeCacheNamespace),
	Redis:        common.RDB,
	RedisCodec:   cachex.JSONCodec[AuthChallengeRecord]{},
	RedisEnabled: func() bool { return common.RedisEnabled && common.RDB != nil },
	Memory: func() *hot.HotCache[string, AuthChallengeRecord] {
		return hot.NewHotCache[string, AuthChallengeRecord](hot.LRU, 128).
			WithTTL(authChallengeTTL).
			WithJanitor().
			Build()
	},
})

var (
	authChallengeConsumeMu    sync.Mutex
	authChallengeConsumeLocks = map[string]*previewConsumeLock{}
)

// AuthChallengeRecord 是后端短期保存的二阶段登录上下文。
//
// 该结构不保存明文账号密码、完整 API Key 或正式 access token；如果前端选择“记住
// 上游登录”，这里只会保存加密后的上游凭据，供 2FA 完成后继续写入普通预览快照。
// new-api 2FA 仍只需要首次登录时目标站点写入的 pending session cookie；sub2api 2FA
// 仍只需要目标平台返回的短期 temp_token。
type AuthChallengeRecord struct {
	ID         string                `json:"id"`
	Platform   string                `json:"platform"`
	BaseURL    string                `json:"base_url"`
	Username   string                `json:"username,omitempty"`
	Email      string                `json:"email,omitempty"`
	ExpiresAt  int64                 `json:"expires_at"`
	Credential *StoredCredential     `json:"credential,omitempty"`
	NewAPI     *NewAPIChallengeData  `json:"new_api,omitempty"`
	Sub2API    *Sub2APIChallengeData `json:"sub2api,omitempty"`
}

// NewAPIChallengeData 保存 new-api 2FA 二阶段需要复用的上下文。
type NewAPIChallengeData struct {
	QuotaPerUnit float64            `json:"quota_per_unit"`
	Cookies      []StoredHTTPCookie `json:"cookies"`
}

// Sub2APIChallengeData 保存 sub2api 2FA 二阶段需要复用的上下文。
type Sub2APIChallengeData struct {
	TempToken string `json:"temp_token"`
}

// StoredHTTPCookie 是 http.Cookie 的可缓存子集。
//
// cookiejar.Jar 本身无法直接 JSON 编码；这里保存目标平台 session cookie 的必要字段，
// 二阶段验证时再按 Base URL 重建 jar。SameSite 不参与 new-api 当前登录判断，但保留
// 字段便于后续排查兼容平台行为。
type StoredHTTPCookie struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Path     string `json:"path,omitempty"`
	Domain   string `json:"domain,omitempty"`
	Expires  int64  `json:"expires,omitempty"`
	Secure   bool   `json:"secure,omitempty"`
	HttpOnly bool   `json:"http_only,omitempty"`
	SameSite string `json:"same_site,omitempty"`
}

func saveAuthChallenge(record AuthChallengeRecord) (*AuthChallenge, error) {
	if strings.TrimSpace(record.ID) == "" {
		record.ID = common.GetUUID()
	}
	record.Platform = NormalizePlatform(record.Platform)
	if record.ExpiresAt == 0 {
		record.ExpiresAt = time.Now().Add(authChallengeTTL).Unix()
	}
	if err := authChallengeCache.SetWithTTL(record.ID, record, authChallengeTTL); err != nil {
		return nil, fmt.Errorf("保存上游账号二次验证会话失败：%w", err)
	}
	return &AuthChallenge{
		ChallengeID: record.ID,
		Platform:    record.Platform,
		Type:        authChallengeTypeTOTP,
		ExpiresAt:   record.ExpiresAt,
		Username:    firstNonEmpty(record.Username, record.Email),
	}, nil
}

func consumeAuthChallenge(challengeID string) (*AuthChallengeRecord, error) {
	challengeID = strings.TrimSpace(challengeID)
	if challengeID == "" {
		return nil, fmt.Errorf("challenge_id 不能为空")
	}

	unlock := lockAuthChallengeConsume(challengeID)
	defer unlock()

	record, found, err := authChallengeCache.GetAndDelete(challengeID)
	if err != nil {
		return nil, err
	}
	if !found || record.ExpiresAt < time.Now().Unix() {
		return nil, fmt.Errorf("二次验证会话不存在或已过期，请重新同步上游账号")
	}
	return &record, nil
}

func lockAuthChallengeConsume(challengeID string) func() {
	authChallengeConsumeMu.Lock()
	lock := authChallengeConsumeLocks[challengeID]
	if lock == nil {
		lock = &previewConsumeLock{}
		authChallengeConsumeLocks[challengeID] = lock
	}
	lock.refs++
	authChallengeConsumeMu.Unlock()

	lock.mu.Lock()
	return func() {
		lock.mu.Unlock()
		authChallengeConsumeMu.Lock()
		lock.refs--
		if lock.refs == 0 && authChallengeConsumeLocks[challengeID] == lock {
			delete(authChallengeConsumeLocks, challengeID)
		}
		authChallengeConsumeMu.Unlock()
	}
}

func storeCookiesFromJar(api *httpClient) []StoredHTTPCookie {
	if api == nil || api.client == nil || api.client.Jar == nil {
		return nil
	}
	baseURL, err := url.Parse(api.baseURL)
	if err != nil {
		return nil
	}
	cookies := api.client.Jar.Cookies(baseURL)
	stored := make([]StoredHTTPCookie, 0, len(cookies))
	for _, cookie := range cookies {
		if cookie == nil || strings.TrimSpace(cookie.Name) == "" {
			continue
		}
		sameSite := ""
		switch cookie.SameSite {
		case http.SameSiteDefaultMode:
			sameSite = "default"
		case http.SameSiteLaxMode:
			sameSite = "lax"
		case http.SameSiteStrictMode:
			sameSite = "strict"
		case http.SameSiteNoneMode:
			sameSite = "none"
		}
		stored = append(stored, StoredHTTPCookie{
			Name:     cookie.Name,
			Value:    cookie.Value,
			Path:     cookie.Path,
			Domain:   cookie.Domain,
			Expires:  cookie.Expires.Unix(),
			Secure:   cookie.Secure,
			HttpOnly: cookie.HttpOnly,
			SameSite: sameSite,
		})
	}
	return stored
}

func restoreCookiesToJar(api *httpClient, stored []StoredHTTPCookie) error {
	if api == nil || api.client == nil || api.client.Jar == nil {
		return fmt.Errorf("上游平台二次验证会话不可用，请重新同步")
	}
	baseURL, err := url.Parse(api.baseURL)
	if err != nil {
		return err
	}
	cookies := make([]*http.Cookie, 0, len(stored))
	for _, item := range stored {
		if strings.TrimSpace(item.Name) == "" {
			continue
		}
		cookie := &http.Cookie{
			Name:     item.Name,
			Value:    item.Value,
			Path:     item.Path,
			Domain:   item.Domain,
			Secure:   item.Secure,
			HttpOnly: item.HttpOnly,
		}
		if item.Expires > 0 {
			cookie.Expires = time.Unix(item.Expires, 0)
		}
		switch item.SameSite {
		case "default":
			cookie.SameSite = http.SameSiteDefaultMode
		case "lax":
			cookie.SameSite = http.SameSiteLaxMode
		case "strict":
			cookie.SameSite = http.SameSiteStrictMode
		case "none":
			cookie.SameSite = http.SameSiteNoneMode
		}
		cookies = append(cookies, cookie)
	}
	if len(cookies) == 0 {
		return fmt.Errorf("上游平台二次验证会话已失效，请重新同步")
	}
	api.client.Jar.SetCookies(baseURL, cookies)
	return nil
}
