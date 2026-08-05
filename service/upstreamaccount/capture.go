package upstreamaccount

import (
	"crypto/subtle"
	"encoding/base64"
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
	captureHelperVersion  = "1.2.0"
	captureHandoffParam   = "nexustok_capture"

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
	ReturnURL string `json:"return_url,omitempty"`
}

// CaptureSessionStartResult 返回给后台页面的安装信息。
type CaptureSessionStartResult struct {
	CaptureID         string `json:"capture_id"`
	ExpiresAt         int64  `json:"expires_at"`
	Platform          string `json:"platform"`
	BaseURL           string `json:"base_url"`
	ManagementBaseURL string `json:"management_base_url,omitempty"`
	RelayBaseURL      string `json:"relay_base_url,omitempty"`
	APIBaseURL        string `json:"api_base_url,omitempty"`
	Origin            string `json:"origin"`
	UserscriptURL     string `json:"userscript_url"`
	HelperInstallURL  string `json:"helper_install_url,omitempty"`
	HandoffURL        string `json:"handoff_url,omitempty"`
	HelperVersion     string `json:"helper_version,omitempty"`
	LoginURL          string `json:"login_url"`
	ReturnURL         string `json:"return_url,omitempty"`
}

// CaptureSessionRecord 是短期缓存中的采集会话。
//
// 该记录可能临时持有目标站 access token / refresh token。它只保存到短 TTL 缓存，
// status 查询只返回脱敏摘要；真正预览时后端按 capture_id 取出登录态使用，避免把
// 明文 token 再回传到浏览器页面。
type CaptureSessionRecord struct {
	ID                string                    `json:"id"`
	Secret            string                    `json:"secret"`
	InstallToken      string                    `json:"install_token,omitempty"`
	UserID            int                       `json:"user_id"`
	ChannelID         int                       `json:"channel_id,omitempty"`
	Platform          string                    `json:"platform"`
	BaseURL           string                    `json:"base_url"`
	ManagementBaseURL string                    `json:"management_base_url,omitempty"`
	RelayBaseURL      string                    `json:"relay_base_url,omitempty"`
	APIBaseURL        string                    `json:"api_base_url,omitempty"`
	Origin            string                    `json:"origin"`
	ReturnURL         string                    `json:"return_url,omitempty"`
	Status            string                    `json:"status"`
	Error             string                    `json:"error,omitempty"`
	ExpiresAt         int64                     `json:"expires_at"`
	UpdatedAt         int64                     `json:"updated_at"`
	Credential        Credential                `json:"credential,omitempty"`
	Summary           *CaptureCredentialSummary `json:"summary,omitempty"`
	Diagnostics       *CaptureDiagnostics       `json:"diagnostics,omitempty"`
}

// CaptureCredentialSummary 是安全返回给前端的采集摘要。
type CaptureCredentialSummary struct {
	Platform            string `json:"platform"`
	AuthMode            string `json:"auth_mode"`
	BaseURL             string `json:"base_url"`
	ManagementBaseURL   string `json:"management_base_url,omitempty"`
	RelayBaseURL        string `json:"relay_base_url,omitempty"`
	APIBaseURL          string `json:"api_base_url,omitempty"`
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
	PageOrigin                   string   `json:"page_origin,omitempty"`
	APIBaseURLSeen               string   `json:"api_base_url_seen,omitempty"`
	LocalStorageKeys             []string `json:"local_storage_keys,omitempty"`
	SessionStorageKeys           []string `json:"session_storage_keys,omitempty"`
	AuthTokenPresent             bool     `json:"auth_token_present,omitempty"`
	AccessTokenPresent           bool     `json:"access_token_present,omitempty"`
	RefreshTokenPresent          bool     `json:"refresh_token_present,omitempty"`
	OAuthHashTokenPresent        bool     `json:"oauth_hash_token_present,omitempty"`
	AuthClientIDPresent          bool     `json:"auth_client_id_present,omitempty"`
	AuthMePath                   string   `json:"auth_me_path,omitempty"`
	BrowserSessionRestorePath    string   `json:"browser_session_restore_path,omitempty"`
	BrowserSessionRestoreStatus  string   `json:"browser_session_restore_status,omitempty"`
	BrowserSessionRestoreMessage string   `json:"browser_session_restore_message,omitempty"`
}

// CaptureSessionStatusResult 是后台页面轮询采集状态的响应。
type CaptureSessionStatusResult struct {
	CaptureID         string                    `json:"capture_id"`
	Status            string                    `json:"status"`
	Message           string                    `json:"message,omitempty"`
	ExpiresAt         int64                     `json:"expires_at"`
	Platform          string                    `json:"platform"`
	BaseURL           string                    `json:"base_url"`
	ManagementBaseURL string                    `json:"management_base_url,omitempty"`
	RelayBaseURL      string                    `json:"relay_base_url,omitempty"`
	APIBaseURL        string                    `json:"api_base_url,omitempty"`
	Origin            string                    `json:"origin"`
	UserscriptURL     string                    `json:"userscript_url,omitempty"`
	HelperInstallURL  string                    `json:"helper_install_url,omitempty"`
	HandoffURL        string                    `json:"handoff_url,omitempty"`
	HelperVersion     string                    `json:"helper_version,omitempty"`
	LoginURL          string                    `json:"login_url,omitempty"`
	ReturnURL         string                    `json:"return_url,omitempty"`
	Summary           *CaptureCredentialSummary `json:"summary,omitempty"`
	Diagnostics       *CaptureDiagnostics       `json:"diagnostics,omitempty"`
}

// CaptureSessionCompleteRequest 是油猴脚本回传的登录态负载。
type CaptureSessionCompleteRequest struct {
	CaptureSecret     string              `json:"capture_secret"`
	CaptureSource     string              `json:"capture_source,omitempty"`
	Platform          string              `json:"platform,omitempty"`
	BaseURL           string              `json:"base_url,omitempty"`
	ManagementBaseURL string              `json:"management_base_url,omitempty"`
	RelayBaseURL      string              `json:"relay_base_url,omitempty"`
	APIBaseURL        string              `json:"api_base_url,omitempty"`
	Origin            string              `json:"origin,omitempty"`
	AuthMode          string              `json:"auth_mode,omitempty"`
	UserID            string              `json:"user_id,omitempty"`
	Username          string              `json:"username,omitempty"`
	Email             string              `json:"email,omitempty"`
	AccessToken       string              `json:"access_token,omitempty"`
	RefreshToken      string              `json:"refresh_token,omitempty"`
	ExpiresAt         int64               `json:"expires_at,omitempty"`
	ExpiresIn         int64               `json:"expires_in,omitempty"`
	Hash              string              `json:"hash,omitempty"`
	LocalStorage      map[string]string   `json:"local_storage,omitempty"`
	AuthUser          map[string]any      `json:"auth_user,omitempty"`
	CapturedAt        int64               `json:"captured_at,omitempty"`
	UserAgent         string              `json:"user_agent,omitempty"`
	Error             string              `json:"error,omitempty"`
	Diagnostics       *CaptureDiagnostics `json:"diagnostics,omitempty"`
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
func StartCaptureSession(userID int, req CaptureSessionStartRequest, nexusBaseURL string, allowedReturnBaseURLs ...string) (*CaptureSessionStartResult, error) {
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
	returnURL := normalizeCaptureReturnURL(req.ReturnURL, nexusBaseURL, allowedReturnBaseURLs...)
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
		ID:                id,
		Secret:            secret,
		InstallToken:      installToken,
		UserID:            userID,
		ChannelID:         req.ChannelID,
		Platform:          platform,
		BaseURL:           normalizedBaseURL,
		ManagementBaseURL: normalizedBaseURL,
		Origin:            origin,
		ReturnURL:         returnURL,
		Status:            captureStatusPending,
		ExpiresAt:         expiresAt,
		UpdatedAt:         common.GetTimestamp(),
	}
	if err := captureSessionCache.SetWithTTL(id, record, captureTTL); err != nil {
		return nil, fmt.Errorf("保存采集会话失败：%w", err)
	}
	userscriptURL, loginURL := captureSessionLinks(record, nexusBaseURL)
	helperInstallURL := captureHelperInstallURL(nexusBaseURL)
	handoffURL := captureHandoffURL(record, userscriptURL)
	return &CaptureSessionStartResult{
		CaptureID:         id,
		ExpiresAt:         expiresAt,
		Platform:          platform,
		BaseURL:           normalizedBaseURL,
		ManagementBaseURL: normalizedBaseURL,
		Origin:            origin,
		UserscriptURL:     userscriptURL,
		HelperInstallURL:  helperInstallURL,
		HandoffURL:        handoffURL,
		HelperVersion:     captureHelperVersion,
		LoginURL:          loginURL,
		ReturnURL:         returnURL,
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
	if summary != nil {
		record.ManagementBaseURL = strings.TrimSpace(summary.ManagementBaseURL)
		record.RelayBaseURL = strings.TrimSpace(summary.RelayBaseURL)
		record.APIBaseURL = strings.TrimSpace(summary.APIBaseURL)
	}
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

// RenderCaptureHelperUserscript 生成稳定版自动采集助手脚本。
//
// 稳定助手不内置任何 capture_secret 或上游登录态，只负责在管理员主动从 NexusTok
// 创建会话后，从当前页 URL fragment/sessionStorage 读取一次性 handoff 参数，再加载
// 对应会话的签名脚本执行真实采集。这样脚本可以安装一次长期复用，同时不会把固定的
// NexusTok 管理权限或第三方账号凭据暴露给任意站点。
func RenderCaptureHelperUserscript(nexusBaseURL string) (string, error) {
	nexusBaseURL = strings.TrimRight(strings.TrimSpace(nexusBaseURL), "/")
	if nexusBaseURL == "" {
		return "", fmt.Errorf("NexusTok 地址不能为空")
	}
	connectHost := "*"
	if u, err := url.Parse(nexusBaseURL); err == nil && u.Hostname() != "" {
		connectHost = u.Hostname()
	}
	configBytes, err := common.Marshal(map[string]any{
		"nexusBaseURL": nexusBaseURL,
		"version":      captureHelperVersion,
		"paramName":    captureHandoffParam,
		"storageKey":   "nexustok_upstream_capture_handoff",
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`// ==UserScript==
// @name         NexusTok Upstream Login Capture Helper
// @namespace    https://github.com/c1cadaBob/NexusTok
// @version      %s
// @description  Stable helper for NexusTok upstream account automatic capture.
// @match        http://*/*
// @match        https://*/*
// @run-at       document-start
// @grant        GM_xmlhttpRequest
// @grant        GM_addStyle
// @grant        unsafeWindow
// @connect      %s
// ==/UserScript==

(function () {
  'use strict';
  const pageWindow = typeof unsafeWindow !== 'undefined' ? unsafeWindow : window;
  const config = %s;
  const readyEvent = 'nexustok-upstream-capture-helper-ready';
  const panelId = 'nexustok-upstream-capture-helper-panel';

  function text(value) {
    return value == null ? '' : String(value);
  }

  function markReady() {
    try {
      pageWindow.__NEXUSTOK_UPSTREAM_CAPTURE_HELPER_VERSION__ = config.version;
      pageWindow.dispatchEvent(new CustomEvent(readyEvent, {
        detail: { version: config.version },
      }));
    } catch (_) {}
  }

  function parseJSON(value) {
    try {
      return value ? JSON.parse(value) : null;
    } catch (_) {
      return null;
    }
  }

  function decodeBase64URL(value) {
    const normalized = text(value).replace(/-/g, '+').replace(/_/g, '/');
    const padded = normalized + '='.repeat((4 - normalized.length %% 4) %% 4);
    try {
      return decodeURIComponent(escape(atob(padded)));
    } catch (_) {
      try { return atob(padded); } catch (__) { return ''; }
    }
  }

  function findHandoffToken() {
    const name = config.paramName || 'nexustok_capture';
    try {
      const url = new URL(pageWindow.location.href);
      const queryValue = url.searchParams.get(name);
      if (queryValue) return { value: queryValue, source: 'query' };
      const rawHash = text(url.hash || '').replace(/^#/, '');
      const hashParams = new URLSearchParams(rawHash);
      const hashValue = hashParams.get(name);
      if (hashValue) return { value: hashValue, source: 'hash' };
      const matched = rawHash.match(new RegExp('(?:^|[?&])' + name + '=([^&]+)'));
      if (matched && matched[1]) return { value: decodeURIComponent(matched[1]), source: 'hash' };
    } catch (_) {}
    return { value: '', source: '' };
  }

  function clearHandoffFromURL(source) {
    const name = config.paramName || 'nexustok_capture';
    try {
      const url = new URL(pageWindow.location.href);
      if (source === 'query') {
        url.searchParams.delete(name);
      }
      if (source === 'hash') {
        const rawHash = text(url.hash || '').replace(/^#/, '');
        const hashParams = new URLSearchParams(rawHash);
        if (hashParams.has(name)) {
          hashParams.delete(name);
          url.hash = hashParams.toString();
        } else if (rawHash.includes(name + '=')) {
          url.hash = '';
        }
      }
      pageWindow.history.replaceState(pageWindow.history.state, '', url.toString());
    } catch (_) {}
  }

  function readStoredHandoff() {
    try {
      return parseJSON(pageWindow.sessionStorage.getItem(config.storageKey));
    } catch (_) {
      return null;
    }
  }

  function writeStoredHandoff(payload) {
    try {
      pageWindow.sessionStorage.setItem(config.storageKey, JSON.stringify(payload));
    } catch (_) {}
  }

  function removeStoredHandoff() {
    try {
      pageWindow.sessionStorage.removeItem(config.storageKey);
    } catch (_) {}
  }

  function readHandoff() {
    const token = findHandoffToken();
    if (token.value) {
      const payload = parseJSON(decodeBase64URL(token.value));
      if (payload && typeof payload === 'object') {
        writeStoredHandoff(payload);
        clearHandoffFromURL(token.source);
        return payload;
      }
    }
    return readStoredHandoff();
  }

  function showStatus(message, tone) {
    if (!document.body) return;
    if (typeof GM_addStyle === 'function') {
      GM_addStyle('#' + panelId + '{position:fixed;right:16px;bottom:16px;z-index:2147483647;max-width:360px;border-radius:8px;background:white;color:#111827;padding:10px 12px;font:12px system-ui;box-shadow:0 8px 24px rgba(0,0,0,.18)}#' + panelId + '[data-tone=error]{border-left:4px solid #dc2626}#' + panelId + '[data-tone=info]{border-left:4px solid #2563eb}');
    }
    let panel = document.getElementById(panelId);
    if (!panel) {
      panel = document.createElement('div');
      panel.id = panelId;
      document.body.appendChild(panel);
    }
    panel.textContent = message;
    panel.dataset.tone = tone || 'info';
  }

  function handoffExpired(payload) {
    const expiresAt = Number.parseInt(text(payload && payload.expiresAt), 10);
    return Number.isFinite(expiresAt) && expiresAt > 0 && Math.floor(Date.now() / 1000) >= expiresAt;
  }

  function loadSessionScript(payload) {
    return new Promise((resolve, reject) => {
      if (!payload || !payload.userscriptURL) {
        reject(new Error('NexusTok capture handoff is missing userscriptURL.'));
        return;
      }
      GM_xmlhttpRequest({
        method: 'GET',
        url: payload.userscriptURL,
        headers: { Accept: 'application/javascript,text/javascript,*/*' },
        onload: (res) => {
          const source = text(res.responseText || '');
          if (res.status < 200 || res.status >= 300 || !source.trimStart().startsWith('// ==UserScript==')) {
            reject(new Error('NexusTok capture script could not be loaded. Create a new capture session and try again.'));
            return;
          }
          try {
            // 这里执行的是当前 NexusTok 会话生成的短时脚本；脚本内仍会校验 capture_secret、
            // origin 和 TTL。稳定助手本身不保存任何长期密钥。
            eval(source + '\n//# sourceURL=nexustok-upstream-capture-session.user.js');
            resolve();
          } catch (error) {
            reject(error);
          }
        },
        onerror: () => reject(new Error('Cannot connect to NexusTok to load the capture script.')),
      });
    });
  }

  async function boot() {
    markReady();
    const payload = readHandoff();
    if (!payload) return;
    if (payload.origin && pageWindow.location.origin !== payload.origin) return;
    if (handoffExpired(payload)) {
      removeStoredHandoff();
      showStatus('NexusTok capture session expired. Create a new session.', 'error');
      return;
    }
    try {
      await loadSessionScript(payload);
    } catch (error) {
      showStatus(error && error.message ? error.message : String(error), 'error');
    }
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', boot, { once: true });
    markReady();
  } else {
    boot();
  }
})();`, captureHelperVersion, connectHost, string(configBytes)), nil
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
		"returnURL":     record.ReturnURL,
		"expiresAt":     record.ExpiresAt,
		"autoRun":       true,
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`// ==UserScript==
// @name         NexusTok Upstream Login Capture
// @namespace    https://github.com/c1cadaBob/NexusTok
// @version      1.1.1
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
  let captureStarted = false;
  let captureCompleted = false;
  let lastFailurePostAt = 0;
  let retryTimer = 0;

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

  function cookieValue(name) {
    if (typeof document === 'undefined') return '';
    try {
      const prefix = name + '=';
      for (const item of document.cookie.split(';')) {
        const trimmed = item.trim();
        if (!trimmed.startsWith(prefix)) continue;
        return decodeURIComponent(trimmed.slice(prefix.length)).trim();
      }
    } catch (_) {}
    return '';
  }

  function appConfigAPIBaseURL() {
    try {
      const value = pageWindow.__APP_CONFIG__ && pageWindow.__APP_CONFIG__.api_base_url;
      return text(value || '').trim();
    } catch (_) {
      return '';
    }
  }

  function normalizeAuthClientID(value) {
    const trimmed = text(value).trim();
    return trimmed && trimmed.length <= 128 ? trimmed : '';
  }

  function readSub2APIAuthClientIDSync() {
    return normalizeAuthClientID(
      directStorageValue(['sub2api_auth_client_id']) ||
      cookieValue('sub2api_auth_client_id')
    );
  }

  function readIndexedDBValue(dbName, storeName, key) {
    if (typeof indexedDB === 'undefined') return Promise.resolve('');
    return new Promise((resolve) => {
      let settled = false;
      const finish = (value) => {
        if (settled) return;
        settled = true;
        resolve(normalizeAuthClientID(value));
      };
      let request;
      try {
        // 不指定版本，避免读取时触发目标站 IndexedDB 升级或因版本更高而失败。
        request = indexedDB.open(dbName);
      } catch (_) {
        finish('');
        return;
      }
      request.onerror = () => finish('');
      request.onblocked = () => finish('');
      request.onsuccess = () => {
        const db = request.result;
        let transaction;
        try {
          if (!db.objectStoreNames.contains(storeName)) {
            db.close();
            finish('');
            return;
          }
          transaction = db.transaction(storeName, 'readonly');
          const getRequest = transaction.objectStore(storeName).get(key);
          getRequest.onsuccess = () => finish(getRequest.result);
          getRequest.onerror = () => finish('');
          transaction.oncomplete = () => db.close();
          transaction.onerror = () => { db.close(); finish(''); };
          transaction.onabort = () => { db.close(); finish(''); };
        } catch (_) {
          try { db.close(); } catch (__) {}
          finish('');
        }
      };
    });
  }

  async function readSub2APIAuthClientID() {
    const direct = readSub2APIAuthClientIDSync();
    if (direct) return direct;
    return readIndexedDBValue('sub2api-auth-coordination', 'values', 'sub2api_auth_client_id');
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

  function collectSub2APIDiagnostics(authMePath, restoreInfo) {
    let localStorageKeys = [];
    let sessionStorageKeys = [];
    try { localStorageKeys = storageKeyNames(pageWindow.localStorage); } catch (_) {}
    try { sessionStorageKeys = storageKeyNames(pageWindow.sessionStorage); } catch (_) {}
    const restore = restoreInfo || {};
    return {
      page_origin: text(pageWindow.location && pageWindow.location.origin),
      api_base_url_seen: appConfigAPIBaseURL(),
      local_storage_keys: localStorageKeys,
      session_storage_keys: sessionStorageKeys,
      auth_token_present: hasStorageValue(['auth_token']),
      access_token_present: hasStorageValue(['access_token', 'token', 'jwt', 'sub2api_auth_token']) ||
        Boolean(pageStateValue(['access_token', 'auth_token', 'token', 'jwt'])),
      refresh_token_present: hasStorageValue(['refresh_token', 'refreshToken', 'rt', 'sub2api_refresh_token']) ||
        Boolean(pageStateValue(['refresh_token', 'refreshtoken', 'rt'])),
      oauth_hash_token_present: hasHashToken(),
      auth_client_id_present: Boolean(restore.authClientIDPresent || readSub2APIAuthClientIDSync()),
      auth_me_path: text(authMePath || ''),
      browser_session_restore_path: text(restore.path || ''),
      browser_session_restore_status: text(restore.status || 'not_attempted'),
      browser_session_restore_message: text(restore.message || ''),
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

  function normalizeSessionRestoreData(data) {
    const source = data && typeof data === 'object' && data.data !== undefined ? data.data : data;
    if (!source || typeof source !== 'object') {
      return { authenticated: false, accessToken: '', refreshToken: '', expiresIn: 0, user: {} };
    }
    const accessToken = text(source.access_token || source.auth_token || source.token || '').replace(/^Bearer\s+/i, '').trim();
    const authenticated = source.authenticated === true || Boolean(accessToken);
    const refreshToken = text(source.refresh_token || source.rt || '').trim();
    const expiresIn = Number.parseInt(text(source.expires_in || source.expiresIn || ''), 10);
    const user = source.user && typeof source.user === 'object' ? source.user : {};
    return {
      authenticated,
      accessToken,
      refreshToken,
      expiresIn: Number.isFinite(expiresIn) && expiresIn > 0 ? expiresIn : 0,
      user,
    };
  }

  async function restoreSub2APIBrowserSession() {
    const authClientID = await readSub2APIAuthClientID();
    const restoreInfo = {
      status: 'not_attempted',
      path: '',
      message: '',
      authClientIDPresent: Boolean(authClientID),
    };
    const headers = { 'Content-Type': 'application/json' };
    if (authClientID) headers['X-Sub2API-Auth-Client'] = authClientID;
    try {
      const result = await readFirstJSON(
        ['/api/v1/auth/session/restore', '/api/auth/session/restore', '/auth/session/restore'],
        { method: 'POST', body: '{}', headers },
        'sub2api browser session restore endpoint',
        'sub2_session_restore'
      );
      const restored = normalizeSessionRestoreData(result.data);
      restoreInfo.path = result.path;
      if (!restored.authenticated) {
        restoreInfo.status = 'unauthenticated';
        restoreInfo.message = 'The target browser session is not authenticated.';
        return { state: null, restoreInfo };
      }
      if (!restored.accessToken) {
        restoreInfo.status = 'failed';
        restoreInfo.message = 'The target browser session restore response did not include access_token.';
        return { state: null, restoreInfo };
      }
      restoreInfo.status = 'authenticated';
      restoreInfo.message = 'The target browser session restored an access token.';
      return {
        state: {
          accessToken: restored.accessToken,
          refreshToken: restored.refreshToken,
          expiresAt: '',
          expiresIn: restored.expiresIn,
          authUser: restored.user,
          localStorage: {
            auth_token: restored.accessToken,
            access_token: restored.accessToken,
            refresh_token: restored.refreshToken,
            token_expires_at: '',
          },
          captureSource: 'browser_session_restore',
        },
        restoreInfo,
      };
    } catch (error) {
      restoreInfo.status = 'failed';
      restoreInfo.message = error && error.message ? error.message : String(error);
      const attempts = error && Array.isArray(error.attempts) ? error.attempts : [];
      if (attempts.length > 0) {
        restoreInfo.path = attempts.slice(0, 3).map((item) => item.path).join(', ');
      }
      return { state: null, restoreInfo };
    }
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
      expiresIn: 0,
      authUser,
      localStorage: {
        auth_token: accessToken,
        access_token: accessToken,
        refresh_token: refreshToken,
        token_expires_at: expiresAt,
      },
      captureSource: 'userscript',
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
    const relayBaseURL = appConfigAPIBaseURL();
    const managementBaseURL = config.baseURL;
    let state = readSub2APILoginState();
    let diagnostics = state.diagnostics || collectSub2APIDiagnostics('');
    let authUser = state.authUser || {};
    let accessToken = state.accessToken;
    if (!accessToken) {
      const restored = await restoreSub2APIBrowserSession();
      if (restored.state && restored.state.accessToken) {
        state = restored.state;
        accessToken = state.accessToken;
        authUser = state.authUser || {};
        diagnostics = collectSub2APIDiagnostics('', restored.restoreInfo);
      } else {
        diagnostics = collectSub2APIDiagnostics('', restored.restoreInfo);
      }
    }
    if (!accessToken) {
      if (await pageLooksLikeNewAPI()) {
        const error = new Error('This page looks like a new-api/NexusTok site, but this capture session was created as sub2api. Open the real sub2api upstream site and make sure its login state is available there.');
        error.diagnostics = diagnostics;
        throw error;
      }
      const restoreStatus = diagnostics.browser_session_restore_status || 'not_attempted';
      const restoreMessage = diagnostics.browser_session_restore_message || '';
      let message = 'sub2api access token was not found in the target page context.';
      if (restoreStatus === 'unauthenticated') {
        message = 'sub2api browser session restore returned unauthenticated. Log in to the upstream sub2api site, wait for the console to finish loading, then run the userscript again.';
      } else if (restoreStatus === 'failed') {
        message = 'sub2api browser session restore failed. The site may use a custom API path, or the login session may be unavailable to the browser page.';
      }
      if (restoreMessage) message += ' ' + restoreMessage;
      const error = new Error(message);
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
      base_url: managementBaseURL,
      management_base_url: managementBaseURL,
      relay_base_url: relayBaseURL,
      access_token: accessToken,
      refresh_token: state.refreshToken,
      api_base_url: relayBaseURL,
      expires_in: state.expiresIn || (Number.isFinite(expiresIn) && expiresIn > 0 ? expiresIn : 0),
      expires_at: normalizeExpiresAt(state.expiresAt),
      auth_user: authUser,
      hash: pageWindow.location.hash || '',
      local_storage: state.localStorage,
      capture_source: state.captureSource || 'userscript',
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

  function captureExpired() {
    const expiresAt = Number.parseInt(text(config.expiresAt || ''), 10);
    return Number.isFinite(expiresAt) && expiresAt > 0 && Math.floor(Date.now() / 1000) >= expiresAt;
  }

  function scheduleRetry(message) {
    if (captureCompleted || captureExpired()) return;
    if (retryTimer) window.clearTimeout(retryTimer);
    setStatus(message || 'Waiting for upstream login...', 'info');
    retryTimer = window.setTimeout(() => {
      captureStarted = false;
      runCapture();
    }, 3000);
  }

  function returnToNexusTok() {
    const target = text(config.returnURL || '').trim();
    if (!target) return;
    try {
      const url = new URL(target, window.location.href);
      url.searchParams.set('upstream_capture_id', config.captureID);
      window.setTimeout(() => {
        window.location.href = url.toString();
      }, 700);
    } catch (_) {
      window.setTimeout(() => {
        window.location.href = target;
      }, 700);
    }
  }

  async function runCapture() {
    if (captureStarted || captureCompleted) return;
    if (captureExpired()) {
      setStatus('NexusTok capture session expired. Create a new session in NexusTok.', 'error');
      return;
    }
    captureStarted = true;
    try {
      setStatus('Capturing upstream login...', 'info');
      const captured = config.platform === 'new-api' ? await captureNewAPI() : await captureSub2API();
      const managementBaseURL = captured.management_base_url || captured.base_url || config.baseURL;
      const relayBaseURL = captured.relay_base_url || captured.api_base_url || '';
      const payload = {
        ...captured,
        capture_secret: config.captureSecret,
        capture_source: captured.capture_source || 'userscript',
        origin: pageWindow.location.origin,
        base_url: managementBaseURL,
        management_base_url: managementBaseURL,
        relay_base_url: relayBaseURL,
        api_base_url: relayBaseURL,
        captured_at: Math.floor(Date.now() / 1000),
        user_agent: navigator.userAgent,
      };
      await postToNexusTok(payload);
      captureCompleted = true;
      setStatus('Captured. Returning to NexusTok...', 'success');
      returnToNexusTok();
    } catch (error) {
      const message = error && error.message ? error.message : String(error);
      const relayBaseURL = config.platform === 'sub2api' ? appConfigAPIBaseURL() : '';
      const payload = {
        capture_secret: config.captureSecret,
        capture_source: 'userscript',
        origin: pageWindow.location.origin,
        base_url: config.baseURL,
        management_base_url: config.baseURL,
        relay_base_url: relayBaseURL,
        api_base_url: relayBaseURL,
        platform: config.platform,
        captured_at: Math.floor(Date.now() / 1000),
        error: message,
        diagnostics: error && error.diagnostics
          ? error.diagnostics
          : collectSub2APIDiagnostics(''),
      };
      const now = Date.now();
      if (now - lastFailurePostAt > 15000) {
        lastFailurePostAt = now;
        try { await postToNexusTok(payload); } catch (_) {}
      }
      captureStarted = false;
      scheduleRetry('Waiting for upstream login. If you are not logged in, finish login on this page.');
    }
  }

  function mount() {
    if (document.getElementById(panelId)) return;
    GM_addStyle('#' + buttonId + '{position:fixed;right:16px;bottom:16px;z-index:2147483647;border:0;border-radius:8px;background:#111827;color:white;padding:10px 12px;font:13px system-ui;box-shadow:0 8px 24px rgba(0,0,0,.24);cursor:pointer}#' + panelId + '{position:fixed;right:16px;bottom:60px;z-index:2147483647;max-width:360px;border-radius:8px;background:white;color:#111827;padding:10px 12px;font:12px system-ui;box-shadow:0 8px 24px rgba(0,0,0,.18)}#' + panelId + '[data-tone=success]{border-left:4px solid #16a34a}#' + panelId + '[data-tone=error]{border-left:4px solid #dc2626}#' + panelId + '[data-tone=info]{border-left:4px solid #2563eb}');
    setStatus('NexusTok capture helper is ready.', 'info');
    if (config.autoRun !== false) {
      window.setTimeout(runCapture, 800);
    } else {
      const button = document.createElement('button');
      button.id = buttonId;
      button.type = 'button';
      button.textContent = 'Send login to NexusTok';
      button.addEventListener('click', runCapture);
      document.body.appendChild(button);
    }
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
	helperInstallURL := captureHelperInstallURL(nexusBaseURL)
	handoffURL := captureHandoffURL(record, userscriptURL)
	managementBaseURL := firstNonEmpty(record.ManagementBaseURL, record.BaseURL)
	relayBaseURL := firstNonEmpty(record.RelayBaseURL, record.APIBaseURL)
	if record.Summary != nil {
		managementBaseURL = firstNonEmpty(record.Summary.ManagementBaseURL, record.Summary.BaseURL, managementBaseURL)
		relayBaseURL = firstNonEmpty(record.Summary.RelayBaseURL, record.Summary.APIBaseURL, relayBaseURL)
	}
	baseURL := managementBaseURL
	return &CaptureSessionStatusResult{
		CaptureID:         record.ID,
		Status:            record.Status,
		Message:           message,
		ExpiresAt:         record.ExpiresAt,
		Platform:          record.Platform,
		BaseURL:           baseURL,
		ManagementBaseURL: managementBaseURL,
		RelayBaseURL:      relayBaseURL,
		APIBaseURL:        relayBaseURL,
		Origin:            record.Origin,
		UserscriptURL:     userscriptURL,
		HelperInstallURL:  helperInstallURL,
		HandoffURL:        handoffURL,
		HelperVersion:     captureHelperVersion,
		LoginURL:          loginURL,
		ReturnURL:         record.ReturnURL,
		Summary:           record.Summary,
		Diagnostics:       sanitizeCaptureDiagnostics(record.Diagnostics),
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
	limitText := func(text string, max int) string {
		text = strings.TrimSpace(common.MaskSensitiveInfo(text))
		if text == "" || max <= 0 {
			return ""
		}
		if len(text) > max {
			return text[:max] + "..."
		}
		return text
	}
	sanitized := &CaptureDiagnostics{
		PageOrigin:                   limitText(value.PageOrigin, 256),
		APIBaseURLSeen:               limitText(value.APIBaseURLSeen, 256),
		AuthTokenPresent:             value.AuthTokenPresent,
		AccessTokenPresent:           value.AccessTokenPresent,
		RefreshTokenPresent:          value.RefreshTokenPresent,
		OAuthHashTokenPresent:        value.OAuthHashTokenPresent,
		AuthClientIDPresent:          value.AuthClientIDPresent,
		AuthMePath:                   limitText(value.AuthMePath, 256),
		BrowserSessionRestorePath:    limitText(value.BrowserSessionRestorePath, 256),
		BrowserSessionRestoreStatus:  limitText(value.BrowserSessionRestoreStatus, 64),
		BrowserSessionRestoreMessage: limitText(value.BrowserSessionRestoreMessage, 512),
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

func captureHelperInstallURL(nexusBaseURL string) string {
	nexusBaseURL = strings.TrimRight(strings.TrimSpace(nexusBaseURL), "/")
	if nexusBaseURL == "" {
		return ""
	}
	return nexusBaseURL + "/api/channel/upstream-account/capture-helper.user.js"
}

func captureHandoffURL(record CaptureSessionRecord, userscriptURL string) string {
	baseURL := strings.TrimSpace(record.BaseURL)
	if baseURL == "" || strings.TrimSpace(userscriptURL) == "" {
		return baseURL
	}
	payloadBytes, err := common.Marshal(map[string]any{
		"captureID":     record.ID,
		"userscriptURL": userscriptURL,
		"returnURL":     record.ReturnURL,
		"expiresAt":     record.ExpiresAt,
		"origin":        record.Origin,
		"platform":      record.Platform,
		"baseURL":       record.BaseURL,
		"helperVersion": captureHelperVersion,
	})
	if err != nil {
		return baseURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return baseURL
	}
	params := url.Values{}
	params.Set(captureHandoffParam, base64.RawURLEncoding.EncodeToString(payloadBytes))
	parsed.Fragment = params.Encode()
	return parsed.String()
}

// normalizeCaptureReturnURL 只允许 userscript 回跳到当前 NexusTok 站点。
//
// userscript 安装链接是一次性敏感入口，回跳地址由后台页面提交；这里仍按
// NexusTok 外部访问地址做同源约束，避免第三方目标站借 capture session 把管理员
// 浏览器重定向到无关站点。无法安全解析时回退到 NexusTok 首页。
func normalizeCaptureReturnURL(raw string, nexusBaseURL string, allowedBaseURLs ...string) string {
	nexusBaseURL = strings.TrimRight(strings.TrimSpace(nexusBaseURL), "/")
	if nexusBaseURL == "" {
		return ""
	}
	base, err := url.Parse(nexusBaseURL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return nexusBaseURL
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nexusBaseURL
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nexusBaseURL
	}
	resolved := base.ResolveReference(parsed)
	if captureReturnURLSameOrigin(resolved, base) {
		return resolved.String()
	}
	for _, allowed := range allowedBaseURLs {
		allowed = strings.TrimRight(strings.TrimSpace(allowed), "/")
		if allowed == "" {
			continue
		}
		allowedURL, err := url.Parse(allowed)
		if err != nil || allowedURL.Scheme == "" || allowedURL.Host == "" {
			continue
		}
		if captureReturnURLSameOrigin(resolved, allowedURL) {
			return resolved.String()
		}
	}
	return nexusBaseURL
}

// captureReturnURLSameOrigin 判断两个 URL 是否属于同源回跳目标。
//
// 采集脚本的下载和 complete 回调仍由签名 URL 与 capture_secret 控制；return_url
// 只决定采集成功后浏览器回到哪个后台页面。这里允许后端感知到的 NexusTok 地址，
// 也允许控制器从 Origin/Referer 推导出的前端地址，以兼容本地 dev server 和反代。
func captureReturnURLSameOrigin(left *url.URL, right *url.URL) bool {
	if left == nil || right == nil {
		return false
	}
	return strings.EqualFold(left.Scheme, right.Scheme) && strings.EqualFold(left.Host, right.Host)
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
		managementBaseURL, relayBaseURL, err := resolveCaptureSub2APIBaseURLs(record, req)
		if err != nil {
			return Credential{}, nil, err
		}
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
			Platform:          PlatformSub2API,
			BaseURL:           managementBaseURL,
			ManagementBaseURL: managementBaseURL,
			RelayBaseURL:      relayBaseURL,
			Username:          strings.TrimSpace(firstNonEmpty(req.Username, valueFromAny(req.AuthUser, "username"))),
			Email:             strings.TrimSpace(firstNonEmpty(req.Email, valueFromAny(req.AuthUser, "email"))),
			AuthMode:          AuthModeAccessToken,
			AccessToken:       accessToken,
			RefreshToken:      refreshToken,
			ExpiresAt:         expiresAt,
		}
		prepared, err := prepareAccessTokenCredential(credential)
		if err != nil {
			return Credential{}, nil, err
		}
		return prepared, &CaptureCredentialSummary{
			Platform:            PlatformSub2API,
			AuthMode:            AuthModeAccessToken,
			BaseURL:             managementBaseURL,
			ManagementBaseURL:   managementBaseURL,
			RelayBaseURL:        relayBaseURL,
			APIBaseURL:          relayBaseURL,
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
			payload.APIBaseURL = strings.TrimSpace(payload.APIBaseURL)
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
