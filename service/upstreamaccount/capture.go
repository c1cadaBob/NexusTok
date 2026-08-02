package upstreamaccount

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/pkg/cachex"

	"github.com/samber/hot"
)

const (
	captureCacheNamespace = "upstream-account-capture"
	captureTTL            = 10 * time.Minute

	captureStatusPending   = "pending"
	captureStatusCompleted = "completed"
	captureStatusFailed    = "failed"
)

var captureSessionCache = cachex.NewHybridCache[CaptureSessionRecord](cachex.HybridCacheConfig[CaptureSessionRecord]{
	Namespace:    cachex.Namespace(captureCacheNamespace),
	Redis:        common.RDB,
	RedisCodec:   cachex.JSONCodec[CaptureSessionRecord]{},
	RedisEnabled: func() bool { return common.RedisEnabled && common.RDB != nil },
	Memory: func() *hot.HotCache[string, CaptureSessionRecord] {
		return hot.NewHotCache[string, CaptureSessionRecord](hot.LRU, 256).
			WithTTL(captureTTL).
			WithJanitor().
			Build()
	},
})

// CaptureSessionStartRequest 是管理员创建油猴采集会话的请求。
type CaptureSessionStartRequest struct {
	Platform  string `json:"platform"`
	BaseURL   string `json:"base_url"`
	ChannelID int    `json:"channel_id,omitempty"`
}

// CaptureSessionStartResult 返回给后台页面的安装信息。
type CaptureSessionStartResult struct {
	CaptureID     string `json:"capture_id"`
	ExpiresAt     int64  `json:"expires_at"`
	Platform      string `json:"platform"`
	BaseURL       string `json:"base_url"`
	Origin        string `json:"origin"`
	UserscriptURL string `json:"userscript_url"`
	LoginURL      string `json:"login_url"`
}

// CaptureSessionRecord 是短期缓存中的采集会话。
//
// 该记录可能临时持有目标站 access token / refresh token。它只保存到短 TTL 缓存，
// status 查询只返回脱敏摘要；真正预览时后端按 capture_id 取出登录态使用，避免把
// 明文 token 再回传到浏览器页面。
type CaptureSessionRecord struct {
	ID         string                    `json:"id"`
	Secret     string                    `json:"secret"`
	UserID     int                       `json:"user_id"`
	ChannelID  int                       `json:"channel_id,omitempty"`
	Platform   string                    `json:"platform"`
	BaseURL    string                    `json:"base_url"`
	Origin     string                    `json:"origin"`
	Status     string                    `json:"status"`
	Error      string                    `json:"error,omitempty"`
	ExpiresAt  int64                     `json:"expires_at"`
	UpdatedAt  int64                     `json:"updated_at"`
	Credential Credential                `json:"credential,omitempty"`
	Summary    *CaptureCredentialSummary `json:"summary,omitempty"`
}

// CaptureCredentialSummary 是安全返回给前端的采集摘要。
type CaptureCredentialSummary struct {
	Platform            string `json:"platform"`
	AuthMode            string `json:"auth_mode"`
	BaseURL             string `json:"base_url"`
	Origin              string `json:"origin"`
	UserID              string `json:"user_id,omitempty"`
	Username            string `json:"username,omitempty"`
	Email               string `json:"email,omitempty"`
	AccessTokenMasked   string `json:"access_token_masked,omitempty"`
	RefreshTokenPresent bool   `json:"refresh_token_present,omitempty"`
	ExpiresAt           int64  `json:"expires_at,omitempty"`
	CapturedAt          int64  `json:"captured_at,omitempty"`
	CaptureSource       string `json:"capture_source,omitempty"`
}

// CaptureSessionStatusResult 是后台页面轮询采集状态的响应。
type CaptureSessionStatusResult struct {
	CaptureID string                    `json:"capture_id"`
	Status    string                    `json:"status"`
	Message   string                    `json:"message,omitempty"`
	ExpiresAt int64                     `json:"expires_at"`
	Platform  string                    `json:"platform"`
	BaseURL   string                    `json:"base_url"`
	Origin    string                    `json:"origin"`
	Summary   *CaptureCredentialSummary `json:"summary,omitempty"`
}

// CaptureSessionCompleteRequest 是油猴脚本回传的登录态负载。
type CaptureSessionCompleteRequest struct {
	CaptureSecret string            `json:"capture_secret"`
	CaptureSource string            `json:"capture_source,omitempty"`
	Platform      string            `json:"platform,omitempty"`
	BaseURL       string            `json:"base_url,omitempty"`
	Origin        string            `json:"origin,omitempty"`
	AuthMode      string            `json:"auth_mode,omitempty"`
	UserID        string            `json:"user_id,omitempty"`
	Username      string            `json:"username,omitempty"`
	Email         string            `json:"email,omitempty"`
	AccessToken   string            `json:"access_token,omitempty"`
	RefreshToken  string            `json:"refresh_token,omitempty"`
	ExpiresAt     int64             `json:"expires_at,omitempty"`
	ExpiresIn     int64             `json:"expires_in,omitempty"`
	Hash          string            `json:"hash,omitempty"`
	LocalStorage  map[string]string `json:"local_storage,omitempty"`
	AuthUser      map[string]any    `json:"auth_user,omitempty"`
	CapturedAt    int64             `json:"captured_at,omitempty"`
	UserAgent     string            `json:"user_agent,omitempty"`
	Error         string            `json:"error,omitempty"`
}

// CredentialParseRequest 允许手动粘贴内容复用采集解析器。
type CredentialParseRequest struct {
	Platform string `json:"platform"`
	BaseURL  string `json:"base_url"`
	Raw      string `json:"raw"`
}

// CredentialParseResult 是手动粘贴登录态后的脱敏解析结果。
type CredentialParseResult struct {
	Summary *CaptureCredentialSummary `json:"summary"`
}

// StartCaptureSession 创建一次性油猴采集会话。
func StartCaptureSession(userID int, req CaptureSessionStartRequest, nexusBaseURL string) (*CaptureSessionStartResult, error) {
	platform := NormalizePlatform(req.Platform)
	if platform == "" {
		return nil, fmt.Errorf("上游平台不能为空")
	}
	if platform != PlatformNewAPI && platform != PlatformSub2API {
		return nil, fmt.Errorf("登录态采集暂仅支持 new-api 和 sub2api")
	}
	baseURL := req.BaseURL
	if platform == PlatformSub2API {
		baseURL = normalizeSub2APIBaseURL(baseURL)
	}
	normalizedBaseURL, err := normalizeBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	origin, err := originFromURL(normalizedBaseURL)
	if err != nil {
		return nil, err
	}
	nexusBaseURL = strings.TrimRight(strings.TrimSpace(nexusBaseURL), "/")
	if nexusBaseURL == "" {
		return nil, fmt.Errorf("NexusTok 地址不能为空")
	}
	id := common.GetUUID()
	secret, err := common.GenerateRandomCharsKey(48)
	if err != nil {
		return nil, fmt.Errorf("生成采集密钥失败：%w", err)
	}
	expiresAt := time.Now().Add(captureTTL).Unix()
	record := CaptureSessionRecord{
		ID:        id,
		Secret:    secret,
		UserID:    userID,
		ChannelID: req.ChannelID,
		Platform:  platform,
		BaseURL:   normalizedBaseURL,
		Origin:    origin,
		Status:    captureStatusPending,
		ExpiresAt: expiresAt,
		UpdatedAt: common.GetTimestamp(),
	}
	if err := captureSessionCache.SetWithTTL(id, record, captureTTL); err != nil {
		return nil, fmt.Errorf("保存采集会话失败：%w", err)
	}
	return &CaptureSessionStartResult{
		CaptureID:     id,
		ExpiresAt:     expiresAt,
		Platform:      platform,
		BaseURL:       normalizedBaseURL,
		Origin:        origin,
		UserscriptURL: nexusBaseURL + "/api/channel/upstream-account/capture-session/" + url.PathEscape(id) + "/userscript.user.js",
		LoginURL:      normalizedBaseURL,
	}, nil
}

// GetCaptureSessionStatus 返回当前管理员可见的采集状态。
func GetCaptureSessionStatus(userID int, captureID string) (*CaptureSessionStatusResult, error) {
	record, err := getCaptureRecordForUser(userID, captureID)
	if err != nil {
		return nil, err
	}
	return sanitizeCaptureRecord(record), nil
}

// ResolveCaptureCredential 将已完成的采集会话转换为预览请求可用的临时凭据。
func ResolveCaptureCredential(userID int, captureID string) (Credential, error) {
	record, err := getCaptureRecordForUser(userID, captureID)
	if err != nil {
		return Credential{}, err
	}
	if record.Status != captureStatusCompleted || record.Credential.Session == nil {
		return Credential{}, fmt.Errorf("登录态采集尚未完成，请先在目标站执行油猴脚本")
	}
	return record.Credential, nil
}

// CompleteCaptureSession 接收油猴脚本回传的目标站登录态。
func CompleteCaptureSession(captureID string, req CaptureSessionCompleteRequest) (*CaptureSessionStatusResult, error) {
	record, err := getCaptureRecord(captureID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.CaptureSecret) == "" || strings.TrimSpace(req.CaptureSecret) != record.Secret {
		return nil, fmt.Errorf("采集会话密钥无效")
	}
	if record.Status == captureStatusCompleted {
		return nil, fmt.Errorf("采集会话已完成，请重新创建会话后再提交")
	}
	payloadOrigin := strings.TrimRight(strings.TrimSpace(req.Origin), "/")
	if payloadOrigin == "" {
		payloadOrigin, _ = originFromURL(req.BaseURL)
	}
	if payloadOrigin != record.Origin {
		return nil, fmt.Errorf("目标站来源不匹配：期望 %s，实际 %s", record.Origin, payloadOrigin)
	}
	if strings.TrimSpace(req.Error) != "" {
		record.Status = captureStatusFailed
		record.Error = common.MaskSensitiveInfo(req.Error)
		record.UpdatedAt = common.GetTimestamp()
		_ = captureSessionCache.SetWithTTL(record.ID, record, time.Until(time.Unix(record.ExpiresAt, 0)))
		return sanitizeCaptureRecord(record), fmt.Errorf("%s", record.Error)
	}
	credential, summary, err := buildCredentialFromCapture(record, req)
	if err != nil {
		record.Status = captureStatusFailed
		record.Error = common.MaskSensitiveInfo(err.Error())
		record.UpdatedAt = common.GetTimestamp()
		_ = captureSessionCache.SetWithTTL(record.ID, record, time.Until(time.Unix(record.ExpiresAt, 0)))
		return sanitizeCaptureRecord(record), err
	}
	record.Status = captureStatusCompleted
	record.Error = ""
	record.Credential = credential
	record.Summary = summary
	record.UpdatedAt = common.GetTimestamp()
	if err := captureSessionCache.SetWithTTL(record.ID, record, time.Until(time.Unix(record.ExpiresAt, 0))); err != nil {
		return nil, fmt.Errorf("保存采集结果失败：%w", err)
	}
	return sanitizeCaptureRecord(record), nil
}

// ParseCredentialDraft 解析管理员手动粘贴的登录态，并只返回脱敏摘要。
//
// 该接口用于页面即时校验，不持久化任何凭据，也不会把 AT/RT/Cookie 明文返回给前端。
// 真正同步仍应走 preview/refresh 接口，由后端重新解析并加密保存凭据。
func ParseCredentialDraft(req CredentialParseRequest) (*CredentialParseResult, error) {
	platform := NormalizePlatform(req.Platform)
	if platform != PlatformNewAPI && platform != PlatformSub2API {
		return nil, fmt.Errorf("登录态解析暂仅支持 new-api 和 sub2api")
	}
	baseURL := req.BaseURL
	if platform == PlatformSub2API {
		baseURL = normalizeSub2APIBaseURL(baseURL)
	}
	normalizedBaseURL, err := normalizeBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	origin, err := originFromURL(normalizedBaseURL)
	if err != nil {
		return nil, err
	}
	payload, err := parseCredentialRawPayload(platform, normalizedBaseURL, req.Raw)
	if err != nil {
		return nil, err
	}
	_, summary, err := buildCredentialFromCapture(CaptureSessionRecord{
		Platform: platform,
		BaseURL:  normalizedBaseURL,
		Origin:   origin,
	}, payload)
	if err != nil {
		return nil, err
	}
	return &CredentialParseResult{Summary: summary}, nil
}

// RenderCaptureUserscript 生成只匹配目标站的 Tampermonkey 脚本。
func RenderCaptureUserscript(userID int, captureID string, nexusBaseURL string) (string, error) {
	record, err := getCaptureRecordForUser(userID, captureID)
	if err != nil {
		return "", err
	}
	nexusBaseURL = strings.TrimRight(strings.TrimSpace(nexusBaseURL), "/")
	if nexusBaseURL == "" {
		return "", fmt.Errorf("NexusTok 地址不能为空")
	}
	completeURL := nexusBaseURL + "/api/channel/upstream-account/capture-session/" + url.PathEscape(record.ID) + "/complete"
	connectHost := "*"
	if u, err := url.Parse(nexusBaseURL); err == nil && u.Hostname() != "" {
		connectHost = u.Hostname()
	}
	matchURL := record.Origin + "/*"
	if u, err := url.Parse(record.Origin); err == nil {
		matchURL = u.Scheme + "://" + u.Host + "/*"
	}
	configBytes, err := common.Marshal(map[string]any{
		"captureID":     record.ID,
		"captureSecret": record.Secret,
		"platform":      record.Platform,
		"baseURL":       record.BaseURL,
		"origin":        record.Origin,
		"completeURL":   completeURL,
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`// ==UserScript==
// @name         NexusTok Upstream Login Capture
// @namespace    https://github.com/c1cadaBob/NexusTok
// @version      1.0.0
// @description  Capture logged-in new-api/sub2api credentials for NexusTok upstream account sync.
// @match        %s
// @grant        GM_xmlhttpRequest
// @grant        GM_addStyle
// @connect      %s
// ==/UserScript==

(function () {
  'use strict';
  const config = %s;
  const buttonId = 'nexustok-upstream-capture-button';
  const panelId = 'nexustok-upstream-capture-panel';

  function text(value) {
    return value == null ? '' : String(value);
  }

  function parseJSON(value) {
    try {
      return value ? JSON.parse(value) : null;
    } catch (_) {
      return null;
    }
  }

  function normalizeExpiresAt(value) {
    const parsed = Number.parseInt(text(value), 10);
    if (!Number.isFinite(parsed) || parsed <= 0) return 0;
    return parsed > 1000000000000 ? Math.floor(parsed / 1000) : parsed;
  }

  function findValueDeep(value, names, depth) {
    if (!value || depth > 4) return '';
    if (Array.isArray(value)) {
      for (const item of value) {
        const found = findValueDeep(item, names, depth + 1);
        if (found) return found;
      }
      return '';
    }
    if (typeof value === 'object') {
      for (const [key, child] of Object.entries(value)) {
        if (names.includes(key.toLowerCase()) && child != null && typeof child !== 'object') {
          return text(child).trim();
        }
        const found = findValueDeep(child, names, depth + 1);
        if (found) return found;
      }
    }
    return '';
  }

  function guessNewAPIUserID() {
    const keys = ['user', 'user_info', 'userInfo', 'auth', 'auth_user', 'new-api-user', 'New-Api-User'];
    for (const storage of [window.localStorage, window.sessionStorage]) {
      for (const key of keys) {
        const raw = storage.getItem(key);
        if (!raw) continue;
        const parsed = parseJSON(raw);
        const found = parsed ? findValueDeep(parsed, ['id', 'userid', 'user_id'], 0) : '';
        if (found) return found;
        if (/^\d+$/.test(raw.trim())) return raw.trim();
      }
      for (let i = 0; i < storage.length; i += 1) {
        const key = storage.key(i) || '';
        if (!/user|auth|profile|self/i.test(key)) continue;
        const parsed = parseJSON(storage.getItem(key));
        const found = parsed ? findValueDeep(parsed, ['id', 'userid', 'user_id'], 0) : '';
        if (found) return found;
      }
    }
    return '';
  }

  async function readJSON(path, options) {
    const response = await fetch(path, {
      credentials: 'include',
      cache: 'no-store',
      ...options,
      headers: {
        Accept: 'application/json',
        ...(options && options.headers ? options.headers : {}),
      },
    });
    const data = await response.json().catch(() => ({}));
    if (!response.ok || data.success === false || data.code > 0) {
      throw new Error(data.message || data.error || ('HTTP ' + response.status));
    }
    return data.data !== undefined ? data.data : data;
  }

  async function captureNewAPI() {
    let userID = guessNewAPIUserID();
    if (!userID) {
      userID = window.prompt('New-Api-User / User ID');
    }
    userID = text(userID).trim();
    if (!userID) throw new Error('New-Api-User / User ID is required');
    const headers = { 'New-Api-User': userID };
    const self = await readJSON('/api/user/self', { headers });
    const resolvedUserID = text(self.id || userID).trim();
    const token = await readJSON('/api/user/token', { headers: { 'New-Api-User': resolvedUserID } });
    return {
      platform: 'new-api',
      auth_mode: 'access_token',
      user_id: resolvedUserID,
      username: text(self.username || ''),
      email: text(self.email || ''),
      access_token: typeof token === 'string' ? token : text(token.access_token || token.token || ''),
      auth_user: self,
    };
  }

  function captureSub2API() {
    const params = new URLSearchParams(window.location.hash.replace(/^#/, ''));
    const authUser = parseJSON(localStorage.getItem('auth_user')) || null;
    const expiresIn = Number.parseInt(text(params.get('expires_in')), 10);
    return {
      platform: 'sub2api',
      auth_mode: 'access_token',
      access_token: text(params.get('access_token') || localStorage.getItem('auth_token') || ''),
      refresh_token: text(params.get('refresh_token') || localStorage.getItem('refresh_token') || ''),
      expires_in: Number.isFinite(expiresIn) && expiresIn > 0 ? expiresIn : 0,
      expires_at: normalizeExpiresAt(localStorage.getItem('token_expires_at')),
      auth_user: authUser,
      hash: window.location.hash || '',
      local_storage: {
        auth_token: text(localStorage.getItem('auth_token') || ''),
        refresh_token: text(localStorage.getItem('refresh_token') || ''),
        token_expires_at: text(localStorage.getItem('token_expires_at') || ''),
      },
    };
  }

  function postToNexusTok(payload) {
    return new Promise((resolve, reject) => {
      GM_xmlhttpRequest({
        method: 'POST',
        url: config.completeURL,
        headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
        data: JSON.stringify(payload),
        onload: (res) => {
          let body = {};
          try { body = JSON.parse(res.responseText || '{}'); } catch (_) {}
          if (res.status >= 200 && res.status < 300 && body.success !== false) resolve(body);
          else reject(new Error(body.message || ('NexusTok HTTP ' + res.status)));
        },
        onerror: () => reject(new Error('Cannot connect to NexusTok')),
      });
    });
  }

  function setStatus(message, tone) {
    let panel = document.getElementById(panelId);
    if (!panel) {
      panel = document.createElement('div');
      panel.id = panelId;
      document.body.appendChild(panel);
    }
    panel.textContent = message;
    panel.dataset.tone = tone || 'info';
  }

  async function runCapture() {
    try {
      setStatus('Capturing upstream login...', 'info');
      const captured = config.platform === 'new-api' ? await captureNewAPI() : captureSub2API();
      const payload = {
        ...captured,
        capture_secret: config.captureSecret,
        capture_source: 'userscript',
        origin: window.location.origin,
        base_url: config.baseURL,
        captured_at: Math.floor(Date.now() / 1000),
        user_agent: navigator.userAgent,
      };
      await postToNexusTok(payload);
      setStatus('Captured. Return to NexusTok to preview and save.', 'success');
    } catch (error) {
      const payload = {
        capture_secret: config.captureSecret,
        capture_source: 'userscript',
        origin: window.location.origin,
        base_url: config.baseURL,
        platform: config.platform,
        captured_at: Math.floor(Date.now() / 1000),
        error: error && error.message ? error.message : String(error),
      };
      try { await postToNexusTok(payload); } catch (_) {}
      setStatus(payload.error, 'error');
    }
  }

  function mount() {
    if (document.getElementById(buttonId)) return;
    GM_addStyle('#' + buttonId + '{position:fixed;right:16px;bottom:16px;z-index:2147483647;border:0;border-radius:8px;background:#111827;color:white;padding:10px 12px;font:13px system-ui;box-shadow:0 8px 24px rgba(0,0,0,.24);cursor:pointer}#' + panelId + '{position:fixed;right:16px;bottom:60px;z-index:2147483647;max-width:360px;border-radius:8px;background:white;color:#111827;padding:10px 12px;font:12px system-ui;box-shadow:0 8px 24px rgba(0,0,0,.18)}#' + panelId + '[data-tone=success]{border-left:4px solid #16a34a}#' + panelId + '[data-tone=error]{border-left:4px solid #dc2626}#' + panelId + '[data-tone=info]{border-left:4px solid #2563eb}');
    const button = document.createElement('button');
    button.id = buttonId;
    button.type = 'button';
    button.textContent = 'Send login to NexusTok';
    button.addEventListener('click', runCapture);
    document.body.appendChild(button);
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', mount);
  else mount();
})();`, matchURL, connectHost, string(configBytes)), nil
}

func getCaptureRecordForUser(userID int, captureID string) (CaptureSessionRecord, error) {
	record, err := getCaptureRecord(captureID)
	if err != nil {
		return CaptureSessionRecord{}, err
	}
	if record.UserID != userID {
		return CaptureSessionRecord{}, fmt.Errorf("采集会话不存在或无权访问")
	}
	return record, nil
}

func getCaptureRecord(captureID string) (CaptureSessionRecord, error) {
	captureID = strings.TrimSpace(captureID)
	if captureID == "" {
		return CaptureSessionRecord{}, fmt.Errorf("capture_id 不能为空")
	}
	record, found, err := captureSessionCache.Get(captureID)
	if err != nil {
		return CaptureSessionRecord{}, err
	}
	if !found || record.ExpiresAt < time.Now().Unix() {
		return CaptureSessionRecord{}, fmt.Errorf("采集会话不存在或已过期")
	}
	return record, nil
}

func sanitizeCaptureRecord(record CaptureSessionRecord) *CaptureSessionStatusResult {
	message := ""
	if record.Status == captureStatusFailed {
		message = record.Error
	}
	return &CaptureSessionStatusResult{
		CaptureID: record.ID,
		Status:    record.Status,
		Message:   message,
		ExpiresAt: record.ExpiresAt,
		Platform:  record.Platform,
		BaseURL:   record.BaseURL,
		Origin:    record.Origin,
		Summary:   record.Summary,
	}
}

func buildCredentialFromCapture(record CaptureSessionRecord, req CaptureSessionCompleteRequest) (Credential, *CaptureCredentialSummary, error) {
	capturedAt := req.CapturedAt
	if capturedAt <= 0 {
		capturedAt = common.GetTimestamp()
	}
	switch record.Platform {
	case PlatformNewAPI:
		userID := strings.TrimSpace(firstNonEmpty(req.UserID, valueFromAny(req.AuthUser, "id"), valueFromAny(req.AuthUser, "user_id")))
		accessToken := normalizeImportedBearerToken(req.AccessToken)
		if userID == "" {
			return Credential{}, nil, fmt.Errorf("new-api 采集结果缺少 New-Api-User / User ID")
		}
		if accessToken == "" {
			return Credential{}, nil, fmt.Errorf("new-api 采集结果缺少 access_token")
		}
		credential := Credential{
			Platform:    PlatformNewAPI,
			BaseURL:     record.BaseURL,
			Username:    strings.TrimSpace(req.Username),
			Email:       strings.TrimSpace(req.Email),
			AuthMode:    AuthModeAccessToken,
			UserID:      userID,
			AccessToken: accessToken,
		}
		prepared, err := prepareAccessTokenCredential(credential)
		if err != nil {
			return Credential{}, nil, err
		}
		return prepared, &CaptureCredentialSummary{
			Platform:          PlatformNewAPI,
			AuthMode:          AuthModeAccessToken,
			BaseURL:           record.BaseURL,
			Origin:            record.Origin,
			UserID:            userID,
			Username:          strings.TrimSpace(req.Username),
			Email:             strings.TrimSpace(req.Email),
			AccessTokenMasked: maskSecret(accessToken),
			CapturedAt:        capturedAt,
			CaptureSource:     firstNonEmpty(req.CaptureSource, "userscript"),
		}, nil
	case PlatformSub2API:
		accessToken := normalizeImportedBearerToken(firstNonEmpty(req.AccessToken, req.LocalStorage["auth_token"], tokenFromHash(req.Hash, "access_token")))
		refreshToken := strings.TrimSpace(firstNonEmpty(req.RefreshToken, req.LocalStorage["refresh_token"], tokenFromHash(req.Hash, "refresh_token")))
		expiresAt := normalizeCaptureExpiresAt(req)
		if accessToken == "" {
			return Credential{}, nil, fmt.Errorf("sub2api 采集结果缺少 access_token")
		}
		credential := Credential{
			Platform:     PlatformSub2API,
			BaseURL:      record.BaseURL,
			Username:     strings.TrimSpace(firstNonEmpty(req.Username, valueFromAny(req.AuthUser, "username"))),
			Email:        strings.TrimSpace(firstNonEmpty(req.Email, valueFromAny(req.AuthUser, "email"))),
			AuthMode:     AuthModeAccessToken,
			AccessToken:  accessToken,
			RefreshToken: refreshToken,
			ExpiresAt:    expiresAt,
		}
		prepared, err := prepareAccessTokenCredential(credential)
		if err != nil {
			return Credential{}, nil, err
		}
		return prepared, &CaptureCredentialSummary{
			Platform:            PlatformSub2API,
			AuthMode:            AuthModeAccessToken,
			BaseURL:             record.BaseURL,
			Origin:              record.Origin,
			Username:            credential.Username,
			Email:               credential.Email,
			AccessTokenMasked:   maskSecret(accessToken),
			RefreshTokenPresent: refreshToken != "",
			ExpiresAt:           expiresAt,
			CapturedAt:          capturedAt,
			CaptureSource:       firstNonEmpty(req.CaptureSource, "userscript"),
		}, nil
	default:
		return Credential{}, nil, fmt.Errorf("不支持的采集平台：%s", record.Platform)
	}
}

func parseCredentialRawPayload(platform string, baseURL string, raw string) (CaptureSessionCompleteRequest, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return CaptureSessionCompleteRequest{}, fmt.Errorf("登录态内容不能为空")
	}
	payload := CaptureSessionCompleteRequest{
		Platform:      platform,
		BaseURL:       baseURL,
		Origin:        mustOriginFromURL(baseURL),
		AuthMode:      AuthModeAccessToken,
		CaptureSource: "manual",
		CapturedAt:    common.GetTimestamp(),
	}
	if strings.HasPrefix(trimmed, "{") {
		if err := common.UnmarshalJsonStr(trimmed, &payload); err == nil {
			payload.Platform = firstNonEmpty(payload.Platform, platform)
			payload.BaseURL = firstNonEmpty(payload.BaseURL, baseURL)
			payload.Origin = firstNonEmpty(payload.Origin, mustOriginFromURL(baseURL))
			payload.CaptureSource = firstNonEmpty(payload.CaptureSource, "manual")
			if payload.AccessToken != "" || payload.UserID != "" || payload.Hash != "" || len(payload.LocalStorage) > 0 {
				return payload, nil
			}
		}
		var object map[string]any
		if err := common.UnmarshalJsonStr(trimmed, &object); err != nil {
			return CaptureSessionCompleteRequest{}, fmt.Errorf("解析登录态 JSON 失败：%w", err)
		}
		payload.AccessToken = firstNonEmpty(valueFromAny(object, "access_token"), valueFromAny(object, "auth_token"), valueFromAny(object, "token"))
		payload.RefreshToken = firstNonEmpty(valueFromAny(object, "refresh_token"), valueFromAny(object, "rt"))
		payload.UserID = firstNonEmpty(valueFromAny(object, "user_id"), valueFromAny(object, "userid"), valueFromAny(object, "New-Api-User"))
		payload.Username = valueFromAny(object, "username")
		payload.Email = valueFromAny(object, "email")
		payload.Hash = valueFromAny(object, "hash")
		payload.AuthUser = object
		return payload, nil
	}
	if strings.HasPrefix(trimmed, "#") || strings.Contains(trimmed, "access_token=") {
		payload.Hash = trimmed
		payload.AccessToken = tokenFromHash(trimmed, "access_token")
		payload.RefreshToken = tokenFromHash(trimmed, "refresh_token")
		if rawExpiresIn := tokenFromHash(trimmed, "expires_in"); rawExpiresIn != "" {
			payload.ExpiresIn, _ = strconv.ParseInt(rawExpiresIn, 10, 64)
		}
		return payload, nil
	}
	payload.AccessToken = trimmed
	return payload, nil
}

func mustOriginFromURL(raw string) string {
	origin, _ := originFromURL(raw)
	return origin
}

func normalizeCaptureExpiresAt(req CaptureSessionCompleteRequest) int64 {
	if req.ExpiresIn > 0 {
		return common.GetTimestamp() + req.ExpiresIn
	}
	if req.ExpiresAt > 0 {
		return normalizeUnixSeconds(req.ExpiresAt)
	}
	if raw := req.LocalStorage["token_expires_at"]; strings.TrimSpace(raw) != "" {
		value, _ := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		return normalizeUnixSeconds(value)
	}
	if raw := tokenFromHash(req.Hash, "expires_in"); strings.TrimSpace(raw) != "" {
		value, _ := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if value > 0 {
			return common.GetTimestamp() + value
		}
	}
	return 0
}

func tokenFromHash(hash string, key string) string {
	hash = strings.TrimPrefix(strings.TrimSpace(hash), "#")
	if hash == "" {
		return ""
	}
	values, err := url.ParseQuery(hash)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(values.Get(key))
}

func valueFromAny(values map[string]any, key string) string {
	if len(values) == 0 {
		return ""
	}
	if value, ok := values[key]; ok {
		return strings.TrimSpace(fmt.Sprint(value))
	}
	for k, value := range values {
		if strings.EqualFold(k, key) {
			return strings.TrimSpace(fmt.Sprint(value))
		}
		if nested, ok := value.(map[string]any); ok {
			if found := valueFromAny(nested, key); found != "" {
				return found
			}
		}
	}
	return ""
}

func originFromURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("目标站地址无效：%s", raw)
	}
	return u.Scheme + "://" + u.Host, nil
}

func maskSecret(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= 10 {
		return value[:1] + "***"
	}
	return value[:6] + "..." + value[len(value)-4:]
}
