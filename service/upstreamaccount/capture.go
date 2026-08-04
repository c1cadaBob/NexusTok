package upstreamaccount

import (
	"crypto/subtle"
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
	ID           string                    `json:"id"`
	Secret       string                    `json:"secret"`
	InstallToken string                    `json:"install_token,omitempty"`
	UserID       int                       `json:"user_id"`
	ChannelID    int                       `json:"channel_id,omitempty"`
	Platform     string                    `json:"platform"`
	BaseURL      string                    `json:"base_url"`
	Origin       string                    `json:"origin"`
	Status       string                    `json:"status"`
	Error        string                    `json:"error,omitempty"`
	ExpiresAt    int64                     `json:"expires_at"`
	UpdatedAt    int64                     `json:"updated_at"`
	Credential   Credential                `json:"credential,omitempty"`
	Summary      *CaptureCredentialSummary `json:"summary,omitempty"`
	Diagnostics  *CaptureDiagnostics       `json:"diagnostics,omitempty"`
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

// CaptureDiagnostics 只记录排查登录态读取问题所需的非敏感信息。
//
// 这里禁止保存任何 token、Cookie 或 localStorage value，只保留 key 名、存在性
// 和最后尝试的校验接口。这样管理员可以判断脚本运行上下文是否正确，同时不会
// 因为错误诊断把第三方登录凭据写入页面响应或普通日志。
type CaptureDiagnostics struct {
	PageOrigin            string   `json:"page_origin,omitempty"`
	LocalStorageKeys      []string `json:"local_storage_keys,omitempty"`
	SessionStorageKeys    []string `json:"session_storage_keys,omitempty"`
	AuthTokenPresent      bool     `json:"auth_token_present,omitempty"`
	AccessTokenPresent    bool     `json:"access_token_present,omitempty"`
	RefreshTokenPresent   bool     `json:"refresh_token_present,omitempty"`
	OAuthHashTokenPresent bool     `json:"oauth_hash_token_present,omitempty"`
	AuthMePath            string   `json:"auth_me_path,omitempty"`
}

// CaptureSessionStatusResult 是后台页面轮询采集状态的响应。
type CaptureSessionStatusResult struct {
	CaptureID     string                    `json:"capture_id"`
	Status        string                    `json:"status"`
	Message       string                    `json:"message,omitempty"`
	ExpiresAt     int64                     `json:"expires_at"`
	Platform      string                    `json:"platform"`
	BaseURL       string                    `json:"base_url"`
	Origin        string                    `json:"origin"`
	UserscriptURL string                    `json:"userscript_url,omitempty"`
	LoginURL      string                    `json:"login_url,omitempty"`
	Summary       *CaptureCredentialSummary `json:"summary,omitempty"`
	Diagnostics   *CaptureDiagnostics       `json:"diagnostics,omitempty"`
}

// CaptureSessionCompleteRequest 是油猴脚本回传的登录态负载。
type CaptureSessionCompleteRequest struct {
	CaptureSecret string              `json:"capture_secret"`
	CaptureSource string              `json:"capture_source,omitempty"`
	Platform      string              `json:"platform,omitempty"`
	BaseURL       string              `json:"base_url,omitempty"`
	Origin        string              `json:"origin,omitempty"`
	AuthMode      string              `json:"auth_mode,omitempty"`
	UserID        string              `json:"user_id,omitempty"`
	Username      string              `json:"username,omitempty"`
	Email         string              `json:"email,omitempty"`
	AccessToken   string              `json:"access_token,omitempty"`
	RefreshToken  string              `json:"refresh_token,omitempty"`
	ExpiresAt     int64               `json:"expires_at,omitempty"`
	ExpiresIn     int64               `json:"expires_in,omitempty"`
	Hash          string              `json:"hash,omitempty"`
	LocalStorage  map[string]string   `json:"local_storage,omitempty"`
	AuthUser      map[string]any      `json:"auth_user,omitempty"`
	CapturedAt    int64               `json:"captured_at,omitempty"`
	UserAgent     string              `json:"user_agent,omitempty"`
	Error         string              `json:"error,omitempty"`
	Diagnostics   *CaptureDiagnostics `json:"diagnostics,omitempty"`
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
	installToken, err := common.GenerateRandomCharsKey(48)
	if err != nil {
		return nil, fmt.Errorf("生成脚本安装签名失败：%w", err)
	}
	expiresAt := time.Now().Add(captureTTL).Unix()
	record := CaptureSessionRecord{
		ID:           id,
		Secret:       secret,
		InstallToken: installToken,
		UserID:       userID,
		ChannelID:    req.ChannelID,
		Platform:     platform,
		BaseURL:      normalizedBaseURL,
		Origin:       origin,
		Status:       captureStatusPending,
		ExpiresAt:    expiresAt,
		UpdatedAt:    common.GetTimestamp(),
	}
	if err := captureSessionCache.SetWithTTL(id, record, captureTTL); err != nil {
		return nil, fmt.Errorf("保存采集会话失败：%w", err)
	}
	userscriptURL, loginURL := captureSessionLinks(record, nexusBaseURL)
	return &CaptureSessionStartResult{
		CaptureID:     id,
		ExpiresAt:     expiresAt,
		Platform:      platform,
		BaseURL:       normalizedBaseURL,
		Origin:        origin,
		UserscriptURL: userscriptURL,
		LoginURL:      loginURL,
	}, nil
}

// GetCaptureSessionStatus 返回当前管理员可见的采集状态。
func GetCaptureSessionStatus(userID int, captureID string, nexusBaseURL string) (*CaptureSessionStatusResult, error) {
	record, err := getCaptureRecordForUser(userID, captureID)
	if err != nil {
		return nil, err
	}
	return sanitizeCaptureRecord(record, nexusBaseURL), nil
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
		record.Diagnostics = sanitizeCaptureDiagnostics(req.Diagnostics)
		record.UpdatedAt = common.GetTimestamp()
		_ = captureSessionCache.SetWithTTL(record.ID, record, time.Until(time.Unix(record.ExpiresAt, 0)))
		return sanitizeCaptureRecord(record, ""), fmt.Errorf("%s", record.Error)
	}
	credential, summary, err := buildCredentialFromCapture(record, req)
	if err != nil {
		record.Status = captureStatusFailed
		record.Error = common.MaskSensitiveInfo(err.Error())
		record.Diagnostics = sanitizeCaptureDiagnostics(req.Diagnostics)
		record.UpdatedAt = common.GetTimestamp()
		_ = captureSessionCache.SetWithTTL(record.ID, record, time.Until(time.Unix(record.ExpiresAt, 0)))
		return sanitizeCaptureRecord(record, ""), err
	}
	record.Status = captureStatusCompleted
	record.Error = ""
	record.Credential = credential
	record.Summary = summary
	record.Diagnostics = sanitizeCaptureDiagnostics(req.Diagnostics)
	record.UpdatedAt = common.GetTimestamp()
	if err := captureSessionCache.SetWithTTL(record.ID, record, time.Until(time.Unix(record.ExpiresAt, 0))); err != nil {
		return nil, fmt.Errorf("保存采集结果失败：%w", err)
	}
	return sanitizeCaptureRecord(record, ""), nil
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
	return renderCaptureUserscript(record, nexusBaseURL)
}

// RenderCaptureUserscriptWithInstallToken 通过短时安装签名生成油猴脚本。
//
// 该入口用于浏览器/Tampermonkey 直接打开 `.user.js` 链接。浏览器导航无法稳定携带
// NexusTok-User 自定义头，因此不能再依赖后台登录态鉴权；安全边界收敛到一次性
// install_token、capture session TTL 和脚本内的 capture_secret。
func RenderCaptureUserscriptWithInstallToken(captureID string, installToken string, nexusBaseURL string) (string, error) {
	record, err := getCaptureRecord(captureID)
	if err != nil {
		return "", err
	}
	if err := verifyCaptureInstallToken(record, installToken); err != nil {
		return "", err
	}
	return renderCaptureUserscript(record, nexusBaseURL)
}

func renderCaptureUserscript(record CaptureSessionRecord, nexusBaseURL string) (string, error) {
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
// @version      1.1.0
// @description  Capture logged-in new-api/sub2api credentials for NexusTok upstream account sync.
// @match        %s
// @run-at       document-idle
// @grant        GM_xmlhttpRequest
// @grant        GM_addStyle
// @grant        unsafeWindow
// @connect      %s
// ==/UserScript==

(function () {
  'use strict';
  // Tampermonkey 默认把脚本放在隔离沙箱中；登录态却属于目标页面上下文。
  // 优先使用 unsafeWindow，才能读取 sub2api 页面真正的 localStorage/sessionStorage。
  const pageWindow = typeof unsafeWindow !== 'undefined' ? unsafeWindow : window;
  const pageFetch = typeof pageWindow.fetch === 'function'
    ? pageWindow.fetch.bind(pageWindow)
    : window.fetch.bind(window);
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

  function isNumericUserID(value) {
    return /^\d+$/.test(text(value).trim());
  }

  function normalizeNewAPIUserID(value) {
    const trimmed = text(value).trim();
    return isNumericUserID(trimmed) ? trimmed : '';
  }

  function storageItems() {
    const items = [];
    for (const storage of [pageWindow.localStorage, pageWindow.sessionStorage]) {
      try {
        for (let i = 0; i < storage.length; i += 1) {
          const key = storage.key(i) || '';
          items.push({ storage, key, value: storage.getItem(key) || '' });
        }
      } catch (_) {}
    }
    return items;
  }

  function directStorageValue(keys) {
    for (const storage of [pageWindow.localStorage, pageWindow.sessionStorage]) {
      for (const key of keys) {
        try {
          const value = text(storage.getItem(key) || '').trim();
          if (value) return value;
        } catch (_) {}
      }
    }
    return '';
  }

  function deepStorageValue(keys, names, keyPattern) {
    for (const item of storageItems()) {
      if (keys.length > 0 && !keys.includes(item.key)) continue;
      if (keyPattern && !keyPattern.test(item.key)) continue;
      const parsed = parseJSON(item.value);
      const found = parsed ? findValueDeep(parsed, names, 0) : '';
      if (found) return found;
    }
    return '';
  }

  function pageStateValue(names) {
    const stateNames = ['__INITIAL_STATE__', '__APP_STATE__', '__NUXT__', '__NEXT_DATA__', '__PINIA__'];
    for (const stateName of stateNames) {
      try {
        const found = findValueDeep(pageWindow[stateName], names, 0);
        if (found) return found;
      } catch (_) {}
    }
    return '';
  }

  function normalizeTokenCandidate(value, names) {
    const raw = text(value).trim();
    if (!raw) return '';
    const parsed = parseJSON(raw);
    if (parsed) {
      return text(findValueDeep(parsed, names, 0)).trim();
    }
    return raw.replace(/^Bearer\s+/i, '').trim();
  }

  function directStorageToken(keys, names) {
    return normalizeTokenCandidate(directStorageValue(keys), names);
  }

  function storageKeyNames(storage) {
    const keys = [];
    try {
      for (let index = 0; index < storage.length && keys.length < 64; index += 1) {
        const key = text(storage.key(index) || '').trim();
        if (key && key.length <= 128) keys.push(key);
      }
    } catch (_) {}
    return keys;
  }

  function hasStorageValue(keys) {
    for (const storage of [pageWindow.localStorage, pageWindow.sessionStorage]) {
      for (const key of keys) {
        try {
          if (text(storage.getItem(key) || '').trim()) return true;
        } catch (_) {}
      }
    }
    return false;
  }

  function hasHashToken() {
    const params = parseHashParams();
    return Boolean(
      params.get('access_token') ||
      params.get('auth_token') ||
      params.get('token') ||
      params.get('refresh_token') ||
      params.get('rt')
    );
  }

  function collectSub2APIDiagnostics(authMePath) {
    let localStorageKeys = [];
    let sessionStorageKeys = [];
    try { localStorageKeys = storageKeyNames(pageWindow.localStorage); } catch (_) {}
    try { sessionStorageKeys = storageKeyNames(pageWindow.sessionStorage); } catch (_) {}
    return {
      page_origin: text(pageWindow.location && pageWindow.location.origin),
      local_storage_keys: localStorageKeys,
      session_storage_keys: sessionStorageKeys,
      auth_token_present: hasStorageValue(['auth_token']),
      access_token_present: hasStorageValue(['access_token', 'token', 'jwt', 'sub2api_auth_token']) ||
        Boolean(pageStateValue(['access_token', 'auth_token', 'token', 'jwt'])),
      refresh_token_present: hasStorageValue(['refresh_token', 'refreshToken', 'rt', 'sub2api_refresh_token']) ||
        Boolean(pageStateValue(['refresh_token', 'refreshtoken', 'rt'])),
      oauth_hash_token_present: hasHashToken(),
      auth_me_path: text(authMePath || ''),
    };
  }

  function parseHashParams() {
    const rawHash = text(pageWindow.location.hash || '').replace(/^#/, '');
    const candidates = [rawHash];
    if (rawHash.includes('?')) candidates.push(rawHash.slice(rawHash.indexOf('?') + 1));
    if (rawHash.includes('&')) candidates.push(rawHash.slice(rawHash.indexOf('&') + 1));
    for (const candidate of candidates) {
      const params = new URLSearchParams(candidate);
      if (params.get('access_token') || params.get('auth_token') || params.get('refresh_token')) {
        return params;
      }
    }
    return new URLSearchParams(rawHash);
  }

  function tokenFromHashParam(...names) {
    const params = parseHashParams();
    for (const name of names) {
      const value = text(params.get(name) || '').trim();
      if (value) return value;
    }
    return '';
  }

  function guessNewAPIUserID() {
    const directUID = text(pageWindow.localStorage.getItem('uid') || pageWindow.sessionStorage.getItem('uid') || '').trim();
    if (isNumericUserID(directUID)) return directUID;
    const keys = ['uid', 'user', 'user_info', 'userInfo', 'auth', 'auth_user', 'new-api-user', 'New-Api-User'];
    for (const storage of [pageWindow.localStorage, pageWindow.sessionStorage]) {
      for (const key of keys) {
        const raw = storage.getItem(key);
        if (!raw) continue;
        const parsed = parseJSON(raw);
        const found = parsed ? findValueDeep(parsed, ['id', 'userid', 'user_id'], 0) : '';
        const normalizedFound = normalizeNewAPIUserID(found);
        if (normalizedFound) return normalizedFound;
        if (isNumericUserID(raw)) return raw.trim();
      }
      for (let i = 0; i < storage.length; i += 1) {
        const key = storage.key(i) || '';
        if (!/user|auth|profile|self/i.test(key)) continue;
        const parsed = parseJSON(storage.getItem(key));
        const found = parsed ? findValueDeep(parsed, ['id', 'userid', 'user_id'], 0) : '';
        const normalizedFound = normalizeNewAPIUserID(found);
        if (normalizedFound) return normalizedFound;
      }
    }
    return '';
  }

  function promptForNewAPIUserID(reason) {
    const message = (reason ? reason + '\n\n' : '') +
      'New-Api-User must be the numeric user ID from the upstream new-api site. ' +
      'Username/email values such as c1cada or linuxdo-... cannot be used here.';
    const value = window.prompt(message);
    const normalized = normalizeNewAPIUserID(value);
    if (!normalized) {
      throw new Error('New-Api-User must be a numeric target-site user ID. Username/email cannot be used.');
    }
    return normalized;
  }

  function cleanPathPrefix(pathname) {
    const commonPageSegments = new Set([
      'login', 'register', 'dashboard', 'console', 'playground', 'token', 'tokens',
      'channel', 'channels', 'setting', 'settings', 'models', 'pricing', 'wallet',
      'topup', 'logs', 'about', 'home', 'panel', 'admin',
    ]);
    const parts = text(pathname).split('/').filter(Boolean);
    while (parts.length > 0 && commonPageSegments.has(parts[parts.length - 1].toLowerCase())) {
      parts.pop();
    }
    if (parts.length === 0) return '';
    return '/' + parts.join('/');
  }

  function candidateAPIPrefixes() {
    const prefixes = new Set(['']);
    for (const raw of [config.baseURL, pageWindow.location.href]) {
      try {
        const parsed = new URL(raw, pageWindow.location.origin);
        const prefix = cleanPathPrefix(parsed.pathname);
        if (prefix && prefix !== '/api') {
          prefixes.add(prefix.endsWith('/api') ? prefix.slice(0, -4) : prefix);
          const firstSegment = '/' + prefix.split('/').filter(Boolean)[0];
          if (firstSegment !== '/') prefixes.add(firstSegment);
        }
      } catch (_) {}
    }
    return Array.from(prefixes);
  }

  function joinPath(prefix, path) {
    if (/^https?:\/\//i.test(path)) return path;
    const left = text(prefix).replace(/\/+$/, '');
    const right = text(path).replace(/^\/+/, '');
    return (left ? left : '') + '/' + right;
  }

  let discoveredAPIPathsPromise = null;

  async function discoverAPIPaths(kind) {
    if (!discoveredAPIPathsPromise) {
      discoveredAPIPathsPromise = (async () => {
        const sources = new Set();
        for (const script of Array.from(document.scripts || [])) {
          if (script.src) sources.add(script.src);
        }
        for (const entry of performance.getEntriesByType ? performance.getEntriesByType('resource') : []) {
          if (entry && entry.name) sources.add(entry.name);
        }
        const sameOriginScripts = Array.from(sources)
          .map((src) => {
            try { return new URL(src, pageWindow.location.href); } catch (_) { return null; }
          })
          .filter((src) => src && src.origin === pageWindow.location.origin && /\.js(?:$|\?)/i.test(src.href))
          .slice(0, 12);
        const discovered = { self: new Set(), token: new Set() };
        const patterns = [
          { key: 'self', regex: /["'](\/[^"']*api\/user\/self[^"']*)["']/g },
          { key: 'token', regex: /["'](\/[^"']*api\/user\/token[^"']*)["']/g },
        ];
        for (const scriptURL of sameOriginScripts) {
          try {
            const res = await pageFetch(scriptURL.href, { credentials: 'include', cache: 'force-cache' });
            if (!res.ok) continue;
            const body = await res.text();
            for (const pattern of patterns) {
              pattern.regex.lastIndex = 0;
              let match;
              while ((match = pattern.regex.exec(body)) !== null) {
                const path = text(match[1]).replace(/\\u0026/g, '&').split('?')[0];
                if (path.startsWith('/')) discovered[pattern.key].add(path);
              }
            }
          } catch (_) {}
        }
        return {
          self: Array.from(discovered.self),
          token: Array.from(discovered.token),
        };
      })();
    }
    const paths = await discoveredAPIPathsPromise;
    return paths[kind] || [];
  }

  async function withCandidatePaths(paths, kind) {
    const discovered = await discoverAPIPaths(kind);
    const prefixes = candidateAPIPrefixes();
    const result = new Set();
    for (const path of [...paths, ...discovered]) {
      for (const prefix of prefixes) {
        result.add(joinPath(prefix, path));
      }
    }
    return Array.from(result);
  }

  function envelopeFailed(data) {
    return data && typeof data === 'object' &&
      (data.success === false || (typeof data.code === 'number' && data.code > 0));
  }

  function responseMessage(data) {
    if (!data || typeof data !== 'object') return '';
    return text(data.message || data.error || data.msg || '');
  }

  async function readJSON(path, options) {
    const response = await pageFetch(path, {
      credentials: 'include',
      cache: 'no-store',
      ...options,
      headers: {
        Accept: 'application/json',
        ...(options && options.headers ? options.headers : {}),
      },
    });
    const rawBody = await response.text();
    const data = parseJSON(rawBody) || {};
    if (!response.ok || envelopeFailed(data)) {
      const message = responseMessage(data) || ('HTTP ' + response.status);
      const error = new Error(message);
      error.status = response.status;
      error.path = path;
      throw error;
    }
    return data && typeof data === 'object' && data.data !== undefined ? data.data : data;
  }

  function summarizeAttempts(attempts) {
    if (attempts.length === 0) return 'no request was made';
    return attempts.slice(0, 6).map((item) => item.path + ' -> ' + item.message).join('; ');
  }

  async function readFirstJSON(paths, options, label, kind) {
    const candidates = await withCandidatePaths(paths, kind);
    const attempts = [];
    for (const path of candidates) {
      try {
        return { data: await readJSON(path, options), path };
      } catch (error) {
        attempts.push({
          path,
          status: error && error.status ? error.status : 0,
          message: error && error.message ? error.message : String(error),
        });
      }
    }
    const onlyNotFound = attempts.length > 0 && attempts.every((item) => item.status === 404);
    const error = new Error(
      onlyNotFound
        ? label + ' was not found on this site. The site may use a custom API prefix or may not be a standard new-api deployment. Tried: ' + summarizeAttempts(attempts)
        : label + ' failed. Tried: ' + summarizeAttempts(attempts)
    );
    error.attempts = attempts;
    throw error;
  }

  function looksLikeMissingUserID(error) {
    const message = error && error.message ? error.message : String(error);
    return /New-Api-User|user id|user_id|用户\s*ID|用户ID|未提供|not provided|mismatch/i.test(message);
  }

  function newAPIHeaders(userID) {
    const headers = {};
    const normalized = normalizeNewAPIUserID(userID);
    if (normalized) headers['New-Api-User'] = normalized;
    return headers;
  }

  async function pageLooksLikeNewAPI() {
    if (normalizeNewAPIUserID(directStorageValue(['uid', 'new-api-user', 'New-Api-User']))) {
      return true;
    }
    const storedUserID = deepStorageValue(
      ['user', 'user_info', 'userInfo', 'auth', 'auth_user'],
      ['id', 'userid', 'user_id'],
      /user|auth|profile|self/i
    );
    if (normalizeNewAPIUserID(storedUserID)) return true;
    const discovered = await discoverAPIPaths('self');
    if (discovered.some((path) => /\/api\/user\/self/.test(path))) return true;
    try {
      const response = await pageFetch('/api/status', { credentials: 'include', cache: 'no-store' });
      const headerVersion = text(response.headers.get('X-New-Api-Version') || response.headers.get('x-new-api-version') || '');
      if (headerVersion) return true;
      const rawBody = await response.text();
      const body = parseJSON(rawBody) || {};
      const data = body.data || body;
      if (data && typeof data === 'object' && (data.quota_per_unit || data.self_use_mode_enabled || data.server_address)) {
        return true;
      }
    } catch (_) {}
    return false;
  }

  function readSub2APILoginState() {
    const accessToken = normalizeTokenCandidate(
      tokenFromHashParam('access_token', 'auth_token', 'token') ||
      directStorageToken(
        ['auth_token', 'access_token', 'token', 'jwt', 'sub2api_auth_token'],
        ['access_token', 'auth_token', 'token', 'jwt']
      ) ||
      deepStorageValue(
        ['auth', 'auth_user', 'token_info', 'tokenInfo', 'session', 'user'],
        ['access_token', 'auth_token', 'token', 'jwt'],
        /auth|token|session|user/i
      ) ||
      pageStateValue(['access_token', 'auth_token', 'token', 'jwt']),
      ['access_token', 'auth_token', 'token', 'jwt']
    );
    const refreshToken = normalizeTokenCandidate(
      tokenFromHashParam('refresh_token', 'rt') ||
      directStorageToken(
        ['refresh_token', 'refreshToken', 'rt', 'sub2api_refresh_token'],
        ['refresh_token', 'refreshtoken', 'rt']
      ) ||
      deepStorageValue(
        ['auth', 'auth_user', 'token_info', 'tokenInfo', 'session', 'user'],
        ['refresh_token', 'refreshtoken', 'rt'],
        /auth|token|session|user/i
      ) ||
      pageStateValue(['refresh_token', 'refreshtoken', 'rt']),
      ['refresh_token', 'refreshtoken', 'rt']
    );
    const expiresAt = text(
      tokenFromHashParam('expires_at', 'expiresAt') ||
      directStorageValue(['token_expires_at', 'expires_at', 'expiresAt', 'access_token_expires_at']) ||
      deepStorageValue(
        ['auth', 'auth_user', 'token_info', 'tokenInfo', 'session', 'user'],
        ['token_expires_at', 'expires_at', 'expiresat', 'access_token_expires_at'],
        /auth|token|session|user/i
      )
    ).trim();
    const storedAuthUser =
      parseJSON(directStorageValue(['auth_user', 'user', 'user_info', 'userInfo']));
    const pageAuthUser = pageWindow.__AUTH_USER__;
    const authUser =
      (storedAuthUser && typeof storedAuthUser === 'object' ? storedAuthUser : null) ||
      (pageAuthUser && typeof pageAuthUser === 'object' ? pageAuthUser : {}) ||
      {};
    return {
      accessToken,
      refreshToken,
      expiresAt,
      authUser,
      localStorage: {
        auth_token: accessToken,
        access_token: accessToken,
        refresh_token: refreshToken,
        token_expires_at: expiresAt,
      },
      diagnostics: collectSub2APIDiagnostics(''),
    };
  }

  async function captureNewAPI() {
    let userID = guessNewAPIUserID();
    const selfPaths = ['/api/user/self', '/api/user/me', '/api/user/profile', '/api/user/info'];
    const tokenPaths = ['/api/user/token', '/api/user/access_token', '/api/user/access-token'];
    let selfResult;
    try {
      selfResult = await readFirstJSON(
        selfPaths,
        { headers: newAPIHeaders(userID) },
        'new-api user profile endpoint',
        'self'
      );
    } catch (error) {
      if (!userID || looksLikeMissingUserID(error)) {
        userID = promptForNewAPIUserID(error && error.message ? error.message : '');
        selfResult = await readFirstJSON(
          selfPaths,
          { headers: newAPIHeaders(userID) },
          'new-api user profile endpoint',
          'self'
        );
      } else {
        throw error;
      }
    }
    const self = selfResult.data || {};
    const resolvedUserID = normalizeNewAPIUserID(self.id || self.user_id || userID);
    if (!resolvedUserID) {
      userID = promptForNewAPIUserID('The target site did not return a numeric user ID from ' + selfResult.path + '.');
    }
    const finalUserID = normalizeNewAPIUserID(resolvedUserID || userID);
    if (!finalUserID) throw new Error('New-Api-User must be a numeric target-site user ID.');
    const tokenResult = await readFirstJSON(
      tokenPaths,
      { headers: newAPIHeaders(finalUserID) },
      'new-api access token endpoint',
      'token'
    );
    const token = tokenResult.data;
    const accessToken = typeof token === 'string' ? token : text(token.access_token || token.token || '');
    if (!accessToken) throw new Error('new-api /api/user/token did not return access_token');
    return {
      platform: 'new-api',
      auth_mode: 'access_token',
      user_id: finalUserID,
      username: text(self.username || ''),
      email: text(self.email || ''),
      access_token: accessToken,
      auth_user: self,
    };
  }

  async function captureSub2API() {
    const params = parseHashParams();
    const state = readSub2APILoginState();
    const diagnostics = state.diagnostics || collectSub2APIDiagnostics('');
    let authUser = state.authUser || {};
    const accessToken = state.accessToken;
    if (!accessToken) {
      if (await pageLooksLikeNewAPI()) {
        const error = new Error('This page looks like a new-api/NexusTok site, but this capture session was created as sub2api. Open the real sub2api upstream site and make sure its login state is available there.');
        error.diagnostics = diagnostics;
        throw error;
      }
      const error = new Error(
        'sub2api access token was not found in the target page context. ' +
        'Open the logged-in sub2api site in the same browser profile. ' +
        'If localStorage.auth_token is null, the login state is stored under another origin or was cleared.'
      );
      error.diagnostics = diagnostics;
      throw error;
    }
    try {
      const meResult = await readFirstJSON(
        ['/api/v1/auth/me', '/api/auth/me', '/auth/me'],
        { headers: { Authorization: 'Bearer ' + accessToken } },
        'sub2api current user endpoint',
        'sub2_me'
      );
      const me = meResult.data;
      diagnostics.auth_me_path = meResult.path;
      authUser = { ...(authUser || {}), ...(me || {}) };
    } catch (error) {
      diagnostics.auth_me_path = 'failed';
      const message = error && error.message ? error.message : String(error);
      const wrapped = new Error('sub2api access token is invalid or expired: ' + message);
      wrapped.diagnostics = diagnostics;
      throw wrapped;
    }
    const expiresIn = Number.parseInt(text(params.get('expires_in')), 10);
    return {
      platform: 'sub2api',
      auth_mode: 'access_token',
      access_token: accessToken,
      refresh_token: state.refreshToken,
      expires_in: Number.isFinite(expiresIn) && expiresIn > 0 ? expiresIn : 0,
      expires_at: normalizeExpiresAt(state.expiresAt),
      auth_user: authUser,
      hash: pageWindow.location.hash || '',
      local_storage: state.localStorage,
      diagnostics,
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
      const captured = config.platform === 'new-api' ? await captureNewAPI() : await captureSub2API();
      const payload = {
        ...captured,
        capture_secret: config.captureSecret,
        capture_source: 'userscript',
        origin: pageWindow.location.origin,
        base_url: config.baseURL,
        captured_at: Math.floor(Date.now() / 1000),
        user_agent: navigator.userAgent,
      };
      await postToNexusTok(payload);
      setStatus('Captured. Return to NexusTok and click Sync Keys to validate, preview, and save.', 'success');
    } catch (error) {
      const message = error && error.message ? error.message : String(error);
      const payload = {
        capture_secret: config.captureSecret,
        capture_source: 'userscript',
        origin: pageWindow.location.origin,
        base_url: config.baseURL,
        platform: config.platform,
        captured_at: Math.floor(Date.now() / 1000),
        error: message,
        diagnostics: error && error.diagnostics
          ? error.diagnostics
          : collectSub2APIDiagnostics(''),
      };
      try { await postToNexusTok(payload); } catch (_) {}
      setStatus('Capture failed: ' + message, 'error');
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

func verifyCaptureInstallToken(record CaptureSessionRecord, installToken string) error {
	expected := strings.TrimSpace(record.InstallToken)
	actual := strings.TrimSpace(installToken)
	if expected == "" {
		return fmt.Errorf("脚本安装链接已失效，请重新创建采集会话")
	}
	if actual == "" {
		return fmt.Errorf("脚本安装链接缺少 install_token，请从 NexusTok 页面重新复制安装链接")
	}
	if subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) != 1 {
		return fmt.Errorf("脚本安装链接无效或已过期，请重新创建采集会话")
	}
	return nil
}

func sanitizeCaptureRecord(record CaptureSessionRecord, nexusBaseURL string) *CaptureSessionStatusResult {
	message := ""
	if record.Status == captureStatusFailed {
		message = record.Error
	}
	userscriptURL, loginURL := captureSessionLinks(record, nexusBaseURL)
	return &CaptureSessionStatusResult{
		CaptureID:     record.ID,
		Status:        record.Status,
		Message:       message,
		ExpiresAt:     record.ExpiresAt,
		Platform:      record.Platform,
		BaseURL:       record.BaseURL,
		Origin:        record.Origin,
		UserscriptURL: userscriptURL,
		LoginURL:      loginURL,
		Summary:       record.Summary,
		Diagnostics:   sanitizeCaptureDiagnostics(record.Diagnostics),
	}
}

// sanitizeCaptureDiagnostics 限制诊断字段只能包含安全的 key 名和布尔值。
//
// userscript 是在第三方站点页面中运行的，任何诊断扩展都必须保持“只说明读取
// 结果，不回传读取内容”的不变量；这里再次清理长度和 key 数量，防止恶意页面
// 构造超大诊断负载占用缓存。
func sanitizeCaptureDiagnostics(value *CaptureDiagnostics) *CaptureDiagnostics {
	if value == nil {
		return nil
	}
	sanitized := &CaptureDiagnostics{
		PageOrigin:            strings.TrimSpace(value.PageOrigin),
		AuthTokenPresent:      value.AuthTokenPresent,
		AccessTokenPresent:    value.AccessTokenPresent,
		RefreshTokenPresent:   value.RefreshTokenPresent,
		OAuthHashTokenPresent: value.OAuthHashTokenPresent,
		AuthMePath:            strings.TrimSpace(value.AuthMePath),
	}
	limitKeys := func(keys []string) []string {
		limited := make([]string, 0, min(len(keys), 64))
		for _, key := range keys {
			key = strings.TrimSpace(key)
			if key == "" || len(key) > 128 {
				continue
			}
			limited = append(limited, key)
			if len(limited) >= 64 {
				break
			}
		}
		return limited
	}
	sanitized.LocalStorageKeys = limitKeys(value.LocalStorageKeys)
	sanitized.SessionStorageKeys = limitKeys(value.SessionStorageKeys)
	return sanitized
}

func captureSessionLinks(record CaptureSessionRecord, nexusBaseURL string) (string, string) {
	nexusBaseURL = strings.TrimRight(strings.TrimSpace(nexusBaseURL), "/")
	userscriptURL := ""
	if nexusBaseURL != "" && strings.TrimSpace(record.ID) != "" && strings.TrimSpace(record.InstallToken) != "" {
		rawURL := nexusBaseURL + "/api/channel/upstream-account/capture-session/" + url.PathEscape(record.ID) + "/userscript.user.js"
		if parsed, err := url.Parse(rawURL); err == nil {
			query := parsed.Query()
			query.Set("install_token", record.InstallToken)
			parsed.RawQuery = query.Encode()
			userscriptURL = parsed.String()
		} else {
			userscriptURL = rawURL + "?install_token=" + url.QueryEscape(record.InstallToken)
		}
	}
	return userscriptURL, record.BaseURL
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
		accessToken := normalizeImportedBearerToken(firstNonEmpty(
			req.AccessToken,
			req.LocalStorage["auth_token"],
			req.LocalStorage["access_token"],
			req.LocalStorage["token"],
			req.LocalStorage["jwt"],
			tokenFromHash(req.Hash, "access_token"),
			tokenFromHash(req.Hash, "auth_token"),
			tokenFromHash(req.Hash, "token"),
		))
		refreshToken := strings.TrimSpace(firstNonEmpty(
			req.RefreshToken,
			req.LocalStorage["refresh_token"],
			req.LocalStorage["refreshToken"],
			req.LocalStorage["rt"],
			tokenFromHash(req.Hash, "refresh_token"),
			tokenFromHash(req.Hash, "rt"),
		))
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
	if raw := firstNonEmpty(
		req.LocalStorage["token_expires_at"],
		req.LocalStorage["expires_at"],
		req.LocalStorage["expiresAt"],
		req.LocalStorage["access_token_expires_at"],
	); strings.TrimSpace(raw) != "" {
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
