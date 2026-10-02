package service

import (
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/c1cadaBob/NexusTok/common"
	"github.com/c1cadaBob/NexusTok/model"
	"github.com/c1cadaBob/NexusTok/pkg/cachex"

	"github.com/samber/hot"
)

const (
	platformSiteCaptureTTL            = 10 * time.Minute
	platformSiteCaptureStatusPending  = "pending"
	platformSiteCaptureStatusComplete = "completed"
	platformSiteCaptureStatusFailed   = "failed"
	platformSiteCaptureHelperVersion  = "1.5.0"
	platformSiteCaptureHandoffParam   = "nexustok_capture"

	PlatformSiteCaptureAuthAuto = "auto"
)

type PlatformSiteCaptureStartRequest struct {
	Platform  string `json:"platform"`
	BaseURL   string `json:"base_url"`
	AuthType  string `json:"auth_type"`
	ChannelID int    `json:"channel_id,omitempty"`
	ReturnURL string `json:"return_url,omitempty"`
}

type PlatformSiteCaptureStartResult struct {
	CaptureID             string `json:"capture_id"`
	ExpiresAt             int64  `json:"expires_at"`
	Platform              string `json:"platform"`
	BaseURL               string `json:"base_url"`
	AuthType              string `json:"auth_type"`
	Origin                string `json:"origin"`
	UserscriptURL         string `json:"userscript_url"`
	HelperInstallURL      string `json:"helper_install_url"`
	HandoffURL            string `json:"handoff_url"`
	LoginURL              string `json:"login_url"`
	HelperVersion         string `json:"helper_version"`
	HelperRequiredVersion string `json:"helper_required_version"`
	HelperStatusMessage   string `json:"helper_status_message,omitempty"`
	ReturnURL             string `json:"return_url,omitempty"`
}

type PlatformSiteCaptureCompleteRequest struct {
	CaptureSecret     string                          `json:"capture_secret"`
	CaptureSource     string                          `json:"capture_source,omitempty"`
	HelperVersion     string                          `json:"helper_version,omitempty"`
	Platform          string                          `json:"platform,omitempty"`
	AuthType          string                          `json:"auth_type,omitempty"`
	BaseURL           string                          `json:"base_url,omitempty"`
	ManagementBaseURL string                          `json:"management_base_url,omitempty"`
	RelayBaseURL      string                          `json:"relay_base_url,omitempty"`
	APIBaseURL        string                          `json:"api_base_url,omitempty"`
	Origin            string                          `json:"origin,omitempty"`
	AccessToken       string                          `json:"access_token,omitempty"`
	RefreshToken      string                          `json:"refresh_token,omitempty"`
	AdminKey          string                          `json:"admin_key,omitempty"`
	Cookie            string                          `json:"cookie,omitempty"`
	SessionID         string                          `json:"session_id,omitempty"`
	UserID            string                          `json:"user_id,omitempty"`
	Username          string                          `json:"username,omitempty"`
	Email             string                          `json:"email,omitempty"`
	TokenExpiresAt    int64                           `json:"token_expires_at,omitempty"`
	ExpiresIn         int64                           `json:"expires_in,omitempty"`
	AuthUser          map[string]any                  `json:"auth_user,omitempty"`
	Diagnostics       *PlatformSiteCaptureDiagnostics `json:"diagnostics,omitempty"`
	Error             string                          `json:"error,omitempty"`
}

type PlatformSiteCaptureSummary struct {
	Platform            string                          `json:"platform"`
	AuthType            string                          `json:"auth_type"`
	BaseURL             string                          `json:"base_url"`
	ManagementBaseURL   string                          `json:"management_base_url,omitempty"`
	RelayBaseURL        string                          `json:"relay_base_url,omitempty"`
	APIBaseURL          string                          `json:"api_base_url,omitempty"`
	Origin              string                          `json:"origin"`
	AccessTokenMasked   string                          `json:"access_token_masked,omitempty"`
	RefreshTokenPresent bool                            `json:"refresh_token_present,omitempty"`
	AdminKeyPresent     bool                            `json:"admin_key_present,omitempty"`
	CookiePresent       bool                            `json:"cookie_present,omitempty"`
	UserID              string                          `json:"user_id,omitempty"`
	Username            string                          `json:"username,omitempty"`
	Email               string                          `json:"email,omitempty"`
	TokenExpiresAt      int64                           `json:"token_expires_at,omitempty"`
	CapturedAt          int64                           `json:"captured_at,omitempty"`
	CaptureSource       string                          `json:"capture_source,omitempty"`
	Diagnostics         *PlatformSiteCaptureDiagnostics `json:"diagnostics,omitempty"`
}

type PlatformSiteCaptureDiagnostics struct {
	Source                       string   `json:"source,omitempty"`
	HelperVersion                string   `json:"helper_version,omitempty"`
	HelperRequiredVersion        string   `json:"helper_required_version,omitempty"`
	PageOrigin                   string   `json:"page_origin,omitempty"`
	APIBaseURLSeen               string   `json:"api_base_url_seen,omitempty"`
	LocalStorageKeys             []string `json:"local_storage_keys,omitempty"`
	SessionStorageKeys           []string `json:"session_storage_keys,omitempty"`
	AuthTokenPresent             bool     `json:"auth_token_present,omitempty"`
	AccessTokenPresent           bool     `json:"access_token_present,omitempty"`
	RefreshTokenPresent          bool     `json:"refresh_token_present,omitempty"`
	AdminKeyPresent              bool     `json:"admin_key_present,omitempty"`
	CookiePresent                bool     `json:"cookie_present,omitempty"`
	OAuthHashTokenPresent        bool     `json:"oauth_hash_token_present,omitempty"`
	AuthClientIDPresent          bool     `json:"auth_client_id_present,omitempty"`
	AuthUserVerified             bool     `json:"auth_user_verified,omitempty"`
	AdminKeyVerified             bool     `json:"admin_key_verified,omitempty"`
	AuthMePath                   string   `json:"auth_me_path,omitempty"`
	AdminVerificationPath        string   `json:"admin_verification_path,omitempty"`
	BrowserSessionRestorePath    string   `json:"browser_session_restore_path,omitempty"`
	BrowserSessionRestoreStatus  string   `json:"browser_session_restore_status,omitempty"`
	BrowserSessionRestoreMessage string   `json:"browser_session_restore_message,omitempty"`
	FailureStage                 string   `json:"failure_stage,omitempty"`
	FailureReason                string   `json:"failure_reason,omitempty"`
	VerificationEndpoints        []string `json:"verification_endpoints,omitempty"`
}

type PlatformSiteCaptureStatusResult struct {
	CaptureID             string                          `json:"capture_id"`
	Status                string                          `json:"status"`
	Message               string                          `json:"message,omitempty"`
	ExpiresAt             int64                           `json:"expires_at"`
	Platform              string                          `json:"platform"`
	BaseURL               string                          `json:"base_url"`
	AuthType              string                          `json:"auth_type"`
	Origin                string                          `json:"origin"`
	UserscriptURL         string                          `json:"userscript_url,omitempty"`
	HelperInstallURL      string                          `json:"helper_install_url,omitempty"`
	HandoffURL            string                          `json:"handoff_url,omitempty"`
	LoginURL              string                          `json:"login_url,omitempty"`
	HelperVersion         string                          `json:"helper_version,omitempty"`
	HelperRequiredVersion string                          `json:"helper_required_version,omitempty"`
	HelperStatusMessage   string                          `json:"helper_status_message,omitempty"`
	ReturnURL             string                          `json:"return_url,omitempty"`
	Summary               *PlatformSiteCaptureSummary     `json:"summary,omitempty"`
	Diagnostics           *PlatformSiteCaptureDiagnostics `json:"diagnostics,omitempty"`
}

type PlatformSiteCaptureResolution struct {
	Platform          string
	Credential        model.PlatformSiteCredential
	ManagementBaseURL string
	RelayBaseURL      string
	APIBaseURL        string
	ClaimToken        string
}

type platformSiteCaptureRecord struct {
	ID           string `json:"id"`
	Secret       string `json:"secret"`
	InstallToken string `json:"install_token"`
	UserID       int    `json:"user_id"`
	ChannelID    int    `json:"channel_id,omitempty"`
	Platform     string `json:"platform"`
	AuthType     string `json:"auth_type"`
	BaseURL      string `json:"base_url"`
	Origin       string `json:"origin"`
	ReturnURL    string `json:"return_url,omitempty"`
	ExpiresAt    int64  `json:"expires_at"`
	Status       string `json:"status"`
	Error        string `json:"error,omitempty"`
	UpdatedAt    int64  `json:"updated_at"`
	// Credential 仅用于读取升级前已经写入缓存的短期记录；新记录必须使用
	// CredentialCiphertext，避免 Redis 或进程内缓存保存明文登录态。
	Credential           model.PlatformSiteCredential    `json:"credential,omitempty"`
	CredentialCiphertext string                          `json:"credential_ciphertext,omitempty"`
	ManagementBaseURL    string                          `json:"management_base_url,omitempty"`
	RelayBaseURL         string                          `json:"relay_base_url,omitempty"`
	APIBaseURL           string                          `json:"api_base_url,omitempty"`
	Summary              *PlatformSiteCaptureSummary     `json:"summary,omitempty"`
	Diagnostics          *PlatformSiteCaptureDiagnostics `json:"diagnostics,omitempty"`
}

var (
	platformSiteCaptureCache = cachex.NewHybridCache[platformSiteCaptureRecord](
		cachex.HybridCacheConfig[platformSiteCaptureRecord]{
			Namespace:  cachex.Namespace("platform-site-capture"),
			Redis:      common.RDB,
			RedisCodec: cachex.JSONCodec[platformSiteCaptureRecord]{},
			RedisEnabled: func() bool {
				return common.RedisEnabled && common.RDB != nil
			},
			Memory: func() *hot.HotCache[string, platformSiteCaptureRecord] {
				return hot.NewHotCache[string, platformSiteCaptureRecord](hot.LRU, 256).
					WithTTL(platformSiteCaptureTTL).
					WithJanitor().
					Build()
			},
		},
	)
	platformSiteCaptureMu sync.Mutex
)

func StartPlatformSiteCaptureSession(
	userID int,
	request PlatformSiteCaptureStartRequest,
	nexusBaseURL string,
) (*PlatformSiteCaptureStartResult, error) {
	if userID <= 0 {
		return nil, fmt.Errorf("管理员身份无效")
	}
	platform := strings.ToLower(strings.TrimSpace(request.Platform))
	authType := strings.ToLower(strings.TrimSpace(request.AuthType))
	if platform != model.PlatformNewAPI && platform != model.PlatformSub2API {
		return nil, errorsForCapture("平台站点类型不受支持")
	}
	if !isPlatformSiteCaptureAuthType(authType) {
		return nil, errorsForCapture("仅支持自动配置、Access Token、Admin Key 和 Cookie 采集")
	}
	baseURL, err := normalizePlatformSiteURL(request.BaseURL)
	if err != nil {
		return nil, err
	}
	if err := validatePlatformSiteURL(baseURL); err != nil {
		return nil, err
	}
	origin, err := platformSiteOrigin(baseURL)
	if err != nil {
		return nil, err
	}
	nexusBaseURL, err = normalizeCaptureNexusBaseURL(nexusBaseURL)
	if err != nil {
		return nil, err
	}
	returnURL, err := normalizePlatformSiteCaptureReturnURL(request.ReturnURL, nexusBaseURL)
	if err != nil {
		return nil, err
	}
	secret, err := common.GenerateRandomCharsKey(64)
	if err != nil {
		return nil, fmt.Errorf("生成采集会话密钥失败")
	}
	installToken, err := common.GenerateRandomCharsKey(64)
	if err != nil {
		return nil, fmt.Errorf("生成采集安装签名失败")
	}
	record := platformSiteCaptureRecord{
		ID:           common.GetUUID(),
		Secret:       secret,
		InstallToken: installToken,
		UserID:       userID,
		ChannelID:    request.ChannelID,
		Platform:     platform,
		AuthType:     authType,
		BaseURL:      baseURL,
		Origin:       origin,
		ReturnURL:    returnURL,
		ExpiresAt:    time.Now().Add(platformSiteCaptureTTL).Unix(),
		Status:       platformSiteCaptureStatusPending,
		UpdatedAt:    common.GetTimestamp(),
	}
	if err := platformSiteCaptureCache.SetWithTTL(record.ID, record, platformSiteCaptureTTL); err != nil {
		return nil, fmt.Errorf("保存采集会话失败")
	}
	userscriptURL := captureUserscriptURL(nexusBaseURL, record.ID, record.InstallToken)
	handoffURL := captureHandoffURL(record, nexusBaseURL)
	return &PlatformSiteCaptureStartResult{
		CaptureID:             record.ID,
		ExpiresAt:             record.ExpiresAt,
		Platform:              record.Platform,
		BaseURL:               record.BaseURL,
		AuthType:              record.AuthType,
		Origin:                record.Origin,
		UserscriptURL:         userscriptURL,
		HelperInstallURL:      strings.TrimRight(nexusBaseURL, "/") + "/api/channel/platform-site/capture-helper.user.js",
		HandoffURL:            handoffURL,
		LoginURL:              record.BaseURL,
		HelperVersion:         platformSiteCaptureHelperVersion,
		HelperRequiredVersion: platformSiteCaptureHelperVersion,
		HelperStatusMessage:   "请安装或更新 NexusTok Capture Helper 后继续。",
		ReturnURL:             record.ReturnURL,
	}, nil
}

func GetPlatformSiteCaptureStatus(
	userID int,
	captureID string,
	nexusBaseURL string,
) (*PlatformSiteCaptureStatusResult, error) {
	record, err := getPlatformSiteCaptureRecord(captureID)
	if err != nil {
		return nil, err
	}
	if record.UserID != userID {
		return nil, errorsForCapture("无权访问该采集会话")
	}
	return sanitizePlatformSiteCaptureRecord(record, nexusBaseURL), nil
}

func CompletePlatformSiteCaptureSession(
	captureID string,
	request PlatformSiteCaptureCompleteRequest,
) (*PlatformSiteCaptureStatusResult, error) {
	platformSiteCaptureMu.Lock()
	defer platformSiteCaptureMu.Unlock()

	record, err := getPlatformSiteCaptureRecord(captureID)
	if err != nil {
		return nil, err
	}
	if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(request.CaptureSecret)), []byte(record.Secret)) != 1 {
		return nil, errorsForCapture("采集会话密钥无效")
	}
	if record.Status == platformSiteCaptureStatusComplete {
		return nil, errorsForCapture("采集会话已完成，请重新创建采集会话")
	}
	captureSource := strings.ToLower(strings.TrimSpace(request.CaptureSource))
	if captureSource != "" && captureSource != "capture_helper" {
		return nil, errorsForCapture("采集来源不受支持")
	}
	if record.AuthType == PlatformSiteCaptureAuthAuto && captureSource != "capture_helper" {
		return nil, errorsForCapture("自动配置必须通过 Capture Helper 完成采集")
	}
	if (record.AuthType == PlatformSiteCaptureAuthAuto || captureSource == "capture_helper") &&
		strings.TrimSpace(request.HelperVersion) != platformSiteCaptureHelperVersion {
		return nil, errorsForCapture("采集助手版本不匹配")
	}
	diagnostics := sanitizePlatformSiteCaptureDiagnostics(request.Diagnostics)
	if diagnostics == nil {
		diagnostics = &PlatformSiteCaptureDiagnostics{}
	}
	diagnostics.Source = captureSource
	diagnostics.HelperVersion = strings.TrimSpace(request.HelperVersion)
	diagnostics.HelperRequiredVersion = platformSiteCaptureHelperVersion
	if platform := strings.ToLower(strings.TrimSpace(request.Platform)); platform != "" && platform != record.Platform {
		return nil, errorsForCapture("采集平台与会话不匹配")
	}
	if authType := strings.ToLower(strings.TrimSpace(request.AuthType)); authType != "" && authType != record.AuthType {
		if record.AuthType != PlatformSiteCaptureAuthAuto ||
			(authType != PlatformSiteCaptureAuthAuto && !isPlatformSiteScriptAuthType(authType)) {
			return nil, errorsForCapture("采集认证方式与会话不匹配")
		}
	}
	origin := strings.TrimRight(strings.TrimSpace(request.Origin), "/")
	if origin == "" {
		origin = record.Origin
	}
	if !strings.EqualFold(origin, record.Origin) {
		return nil, errorsForCapture("目标站来源不匹配")
	}
	if strings.TrimSpace(request.Error) != "" {
		record.Status = platformSiteCaptureStatusFailed
		record.Error = safePlatformSiteCaptureFailure(request.Error)
		diagnostics.FailureStage = firstNonEmptyCaptureString(
			diagnostics.FailureStage,
			"capture_helper",
		)
		diagnostics.FailureReason = record.Error
		record.Diagnostics = diagnostics
		record.UpdatedAt = common.GetTimestamp()
		if err := savePlatformSiteCaptureRecord(record); err != nil {
			return nil, err
		}
		return sanitizePlatformSiteCaptureRecord(record, ""), errorsForCapture(record.Error)
	}

	resolution, summary, err := buildPlatformSiteCaptureResolution(record, request)
	if err != nil {
		record.Status = platformSiteCaptureStatusFailed
		record.Error = safePlatformSiteCaptureFailure(err.Error())
		diagnostics.FailureStage = firstNonEmptyCaptureString(
			diagnostics.FailureStage,
			"server_validation",
		)
		diagnostics.FailureReason = record.Error
		record.Diagnostics = diagnostics
		record.UpdatedAt = common.GetTimestamp()
		_ = savePlatformSiteCaptureRecord(record)
		return sanitizePlatformSiteCaptureRecord(record, ""), err
	}
	record.Status = platformSiteCaptureStatusComplete
	record.Error = ""
	credentialCiphertext, err := model.EncryptPlatformSiteCredential(resolution.Credential)
	if err != nil {
		record.Status = platformSiteCaptureStatusFailed
		record.Error = "采集结果保存失败"
		diagnostics.FailureStage = "credential_encryption"
		diagnostics.FailureReason = record.Error
		record.Diagnostics = diagnostics
		record.UpdatedAt = common.GetTimestamp()
		_ = savePlatformSiteCaptureRecord(record)
		return sanitizePlatformSiteCaptureRecord(record, ""), fmt.Errorf("保存采集凭据失败")
	}
	record.Credential = model.PlatformSiteCredential{}
	record.CredentialCiphertext = credentialCiphertext
	record.ManagementBaseURL = resolution.ManagementBaseURL
	record.RelayBaseURL = resolution.RelayBaseURL
	record.APIBaseURL = resolution.APIBaseURL
	record.Diagnostics = diagnostics
	if summary != nil {
		summary.Diagnostics = diagnostics
	}
	record.Summary = summary
	record.UpdatedAt = common.GetTimestamp()
	if err := savePlatformSiteCaptureRecord(record); err != nil {
		return nil, fmt.Errorf("保存采集结果失败")
	}
	return sanitizePlatformSiteCaptureRecord(record, ""), nil
}

func ResolvePlatformSiteCapture(
	userID int,
	captureID string,
	channelID int,
	platform string,
	authType string,
) (PlatformSiteCaptureResolution, error) {
	platformSiteCaptureMu.Lock()
	defer platformSiteCaptureMu.Unlock()

	record, err := getPlatformSiteCaptureRecord(captureID)
	if err != nil {
		return PlatformSiteCaptureResolution{}, err
	}
	if record.UserID != userID {
		return PlatformSiteCaptureResolution{}, errorsForCapture("无权访问该采集会话")
	}
	if record.Status != platformSiteCaptureStatusComplete {
		return PlatformSiteCaptureResolution{}, errorsForCapture("采集会话尚未完成")
	}
	if record.ChannelID != 0 && channelID != 0 && record.ChannelID != channelID {
		return PlatformSiteCaptureResolution{}, errorsForCapture("采集会话未绑定当前渠道")
	}
	if normalized := strings.ToLower(strings.TrimSpace(platform)); normalized != "" && normalized != record.Platform {
		return PlatformSiteCaptureResolution{}, errorsForCapture("采集平台与渠道不匹配")
	}
	credential, err := decryptPlatformSiteCaptureCredential(record)
	if err != nil {
		return PlatformSiteCaptureResolution{}, errorsForCapture("采集凭据不可用，请重新采集")
	}
	if normalized := strings.ToLower(strings.TrimSpace(authType)); normalized != "" &&
		normalized != record.AuthType &&
		!(record.AuthType == PlatformSiteCaptureAuthAuto && normalized == credential.AuthType) {
		return PlatformSiteCaptureResolution{}, errorsForCapture("采集认证方式与渠道不匹配")
	}
	claimToken, err := common.GenerateRandomCharsKey(48)
	if err != nil {
		return PlatformSiteCaptureResolution{}, errorsForCapture("生成采集会话占用凭据失败")
	}
	remaining := time.Until(time.Unix(record.ExpiresAt, 0))
	if remaining <= 0 {
		return PlatformSiteCaptureResolution{}, errorsForCapture("采集会话已过期")
	}
	claimed, err := platformSiteCaptureCache.TryClaim(record.ID, claimToken, remaining)
	if err != nil {
		return PlatformSiteCaptureResolution{}, errorsForCapture("采集会话暂时不可用")
	}
	if !claimed {
		return PlatformSiteCaptureResolution{}, errorsForCapture("采集会话正在被其他请求使用")
	}
	return PlatformSiteCaptureResolution{
		Platform:          record.Platform,
		Credential:        credential,
		ManagementBaseURL: record.ManagementBaseURL,
		RelayBaseURL:      record.RelayBaseURL,
		APIBaseURL:        record.APIBaseURL,
		ClaimToken:        claimToken,
	}, nil
}

func ConsumePlatformSiteCapture(userID int, captureID string, channelID int, claimTokens ...string) error {
	platformSiteCaptureMu.Lock()
	defer platformSiteCaptureMu.Unlock()

	record, err := getPlatformSiteCaptureRecord(captureID)
	if err != nil {
		return err
	}
	if record.UserID != userID {
		return errorsForCapture("无权访问该采集会话")
	}
	if record.ChannelID != 0 && channelID != 0 && record.ChannelID != channelID {
		return errorsForCapture("采集会话未绑定当前渠道")
	}
	claimToken := ""
	if len(claimTokens) > 0 {
		claimToken = strings.TrimSpace(claimTokens[0])
	}
	if claimToken == "" {
		return errorsForCapture("采集会话占用凭据无效")
	}
	claimed, err := platformSiteCaptureCache.ClaimMatches(captureID, claimToken)
	if err != nil {
		return err
	}
	if !claimed {
		return errorsForCapture("采集会话占用凭据无效")
	}
	_, err = platformSiteCaptureCache.DeleteMany([]string{captureID})
	if err != nil {
		return err
	}
	_, err = platformSiteCaptureCache.ReleaseClaim(captureID, claimToken)
	return err
}

func ReleasePlatformSiteCaptureClaim(captureID, claimToken string) error {
	_, err := platformSiteCaptureCache.ReleaseClaim(captureID, claimToken)
	return err
}

func RenderPlatformSiteCaptureUserscript(captureID, installToken, nexusBaseURL string) (string, error) {
	record, err := getPlatformSiteCaptureRecord(captureID)
	if err != nil {
		return "", err
	}
	if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(installToken)), []byte(record.InstallToken)) != 1 {
		return "", errorsForCapture("采集安装签名无效")
	}
	nexusBaseURL, err = normalizeCaptureNexusBaseURL(nexusBaseURL)
	if err != nil {
		return "", err
	}
	return renderPlatformSiteCaptureScript(nexusBaseURL, record.BaseURL, record.Platform, record.AuthType, record.ID), nil
}

func RenderPlatformSiteCaptureHelper(nexusBaseURL string) (string, error) {
	nexusBaseURL, err := normalizeCaptureNexusBaseURL(nexusBaseURL)
	if err != nil {
		return "", err
	}
	return renderPlatformSiteCaptureScript(nexusBaseURL, "", "", "", ""), nil
}

func getPlatformSiteCaptureRecord(captureID string) (platformSiteCaptureRecord, error) {
	captureID = strings.TrimSpace(captureID)
	if captureID == "" {
		return platformSiteCaptureRecord{}, errorsForCapture("采集会话 ID 不能为空")
	}
	record, found, err := platformSiteCaptureCache.Get(captureID)
	if err != nil {
		return platformSiteCaptureRecord{}, errorsForCapture("采集会话不可用")
	}
	if !found || record.ID == "" {
		return platformSiteCaptureRecord{}, errorsForCapture("采集会话不存在或已过期")
	}
	if record.ExpiresAt <= time.Now().Unix() {
		_, _ = platformSiteCaptureCache.DeleteMany([]string{captureID})
		_ = platformSiteCaptureCache.DeleteClaim(captureID)
		return platformSiteCaptureRecord{}, errorsForCapture("采集会话已过期")
	}
	return record, nil
}

func savePlatformSiteCaptureRecord(record platformSiteCaptureRecord) error {
	remaining := time.Until(time.Unix(record.ExpiresAt, 0))
	if remaining <= 0 {
		return errorsForCapture("采集会话已过期")
	}
	return platformSiteCaptureCache.SetWithTTL(record.ID, record, remaining)
}

func decryptPlatformSiteCaptureCredential(
	record platformSiteCaptureRecord,
) (model.PlatformSiteCredential, error) {
	if strings.TrimSpace(record.CredentialCiphertext) != "" {
		return model.DecryptPlatformSiteCredential(record.CredentialCiphertext)
	}
	if record.Credential.AuthType != "" ||
		record.Credential.Username != "" ||
		record.Credential.Password != "" ||
		record.Credential.AccessToken != "" ||
		record.Credential.RefreshToken != "" ||
		record.Credential.AdminKey != "" ||
		record.Credential.Cookie != "" {
		return record.Credential, nil
	}
	return model.PlatformSiteCredential{}, errorsForCapture("采集凭据不存在")
}

func sanitizePlatformSiteCaptureRecord(
	record platformSiteCaptureRecord,
	nexusBaseURL string,
) *PlatformSiteCaptureStatusResult {
	result := &PlatformSiteCaptureStatusResult{
		CaptureID:             record.ID,
		Status:                record.Status,
		ExpiresAt:             record.ExpiresAt,
		Platform:              record.Platform,
		BaseURL:               record.BaseURL,
		AuthType:              record.AuthType,
		Origin:                record.Origin,
		HelperVersion:         platformSiteCaptureHelperVersion,
		HelperRequiredVersion: platformSiteCaptureHelperVersion,
		HelperStatusMessage:   "请安装或更新 NexusTok Capture Helper 后继续。",
		ReturnURL:             record.ReturnURL,
		Message:               record.Error,
		Summary:               record.Summary,
		Diagnostics:           record.Diagnostics,
	}
	if strings.TrimSpace(nexusBaseURL) == "" {
		return result
	}
	if normalized, err := normalizeCaptureNexusBaseURL(nexusBaseURL); err == nil {
		result.UserscriptURL = captureUserscriptURL(normalized, record.ID, record.InstallToken)
		result.HelperInstallURL = strings.TrimRight(normalized, "/") + "/api/channel/platform-site/capture-helper.user.js"
		result.HandoffURL = captureHandoffURL(record, normalized)
	}
	result.LoginURL = record.BaseURL
	return result
}

func sanitizePlatformSiteCaptureDiagnostics(
	diagnostics *PlatformSiteCaptureDiagnostics,
) *PlatformSiteCaptureDiagnostics {
	if diagnostics == nil {
		return nil
	}
	sanitized := &PlatformSiteCaptureDiagnostics{
		Source:                       sanitizePlatformSiteDiagnosticToken(diagnostics.Source),
		HelperVersion:                sanitizePlatformSiteDiagnosticToken(diagnostics.HelperVersion),
		HelperRequiredVersion:        sanitizePlatformSiteDiagnosticToken(diagnostics.HelperRequiredVersion),
		PageOrigin:                   sanitizePlatformSiteDiagnosticURL(diagnostics.PageOrigin),
		APIBaseURLSeen:               sanitizePlatformSiteDiagnosticURL(diagnostics.APIBaseURLSeen),
		AuthTokenPresent:             diagnostics.AuthTokenPresent,
		AccessTokenPresent:           diagnostics.AccessTokenPresent,
		RefreshTokenPresent:          diagnostics.RefreshTokenPresent,
		AdminKeyPresent:              diagnostics.AdminKeyPresent,
		CookiePresent:                diagnostics.CookiePresent,
		OAuthHashTokenPresent:        diagnostics.OAuthHashTokenPresent,
		AuthClientIDPresent:          diagnostics.AuthClientIDPresent,
		AuthUserVerified:             diagnostics.AuthUserVerified,
		AdminKeyVerified:             diagnostics.AdminKeyVerified,
		AuthMePath:                   sanitizePlatformSiteDiagnosticPath(diagnostics.AuthMePath),
		AdminVerificationPath:        sanitizePlatformSiteDiagnosticPath(diagnostics.AdminVerificationPath),
		BrowserSessionRestorePath:    sanitizePlatformSiteDiagnosticPath(diagnostics.BrowserSessionRestorePath),
		BrowserSessionRestoreStatus:  sanitizePlatformSiteDiagnosticToken(diagnostics.BrowserSessionRestoreStatus),
		BrowserSessionRestoreMessage: sanitizePlatformSiteDiagnosticToken(diagnostics.BrowserSessionRestoreMessage),
		FailureStage:                 sanitizePlatformSiteDiagnosticToken(diagnostics.FailureStage),
		VerificationEndpoints:        sanitizePlatformSiteDiagnosticPaths(diagnostics.VerificationEndpoints),
		LocalStorageKeys:             sanitizePlatformSiteDiagnosticKeys(diagnostics.LocalStorageKeys),
		SessionStorageKeys:           sanitizePlatformSiteDiagnosticKeys(diagnostics.SessionStorageKeys),
	}
	if strings.TrimSpace(diagnostics.FailureReason) != "" {
		sanitized.FailureReason = safePlatformSiteCaptureFailure(diagnostics.FailureReason)
	}
	if sanitized.HelperRequiredVersion == "" {
		sanitized.HelperRequiredVersion = platformSiteCaptureHelperVersion
	}
	return sanitized
}

func sanitizePlatformSiteDiagnosticURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/")
}

func sanitizePlatformSiteDiagnosticPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.ContainsAny(value, "\r\n") || len(value) > 512 {
		return ""
	}
	return value
}

func sanitizePlatformSiteDiagnosticPaths(values []string) []string {
	result := make([]string, 0, min(len(values), 16))
	for _, value := range values {
		if sanitized := sanitizePlatformSiteDiagnosticPath(value); sanitized != "" {
			result = append(result, sanitized)
		}
		if len(result) >= 16 {
			break
		}
	}
	return uniqueStrings(result)
}

func sanitizePlatformSiteDiagnosticKeys(values []string) []string {
	result := make([]string, 0, min(len(values), 64))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len(value) > 128 || strings.ContainsAny(value, "\r\n") {
			continue
		}
		result = append(result, value)
		if len(result) >= 64 {
			break
		}
	}
	return uniqueStrings(result)
}

func firstNonEmptyCaptureString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func buildPlatformSiteCaptureResolution(
	record platformSiteCaptureRecord,
	request PlatformSiteCaptureCompleteRequest,
) (PlatformSiteCaptureResolution, *PlatformSiteCaptureSummary, error) {
	managementBaseURL, err := firstCaptureURL(
		request.ManagementBaseURL,
		request.BaseURL,
		record.BaseURL,
	)
	if err != nil {
		return PlatformSiteCaptureResolution{}, nil, err
	}
	relayBaseURL, err := firstCaptureURL(request.RelayBaseURL, request.APIBaseURL)
	if err != nil {
		return PlatformSiteCaptureResolution{}, nil, err
	}
	apiBaseURL, err := firstCaptureURL(request.APIBaseURL)
	if err != nil {
		return PlatformSiteCaptureResolution{}, nil, err
	}
	for _, candidate := range []string{managementBaseURL, relayBaseURL, apiBaseURL} {
		if candidate == "" {
			continue
		}
		if !captureRelatedPlatformURL(record.BaseURL, candidate) {
			return PlatformSiteCaptureResolution{}, nil, errorsForCapture("采集页面地址不属于目标站点")
		}
	}
	if relayBaseURL == "" && apiBaseURL != "" {
		relayBaseURL = apiBaseURL
	}
	authType := record.AuthType
	strictAuthType := authType != PlatformSiteCaptureAuthAuto
	if authType == PlatformSiteCaptureAuthAuto {
		authType = selectPlatformSiteCaptureAuthType(request)
		if authType == "" {
			return PlatformSiteCaptureResolution{}, nil, errorsForCapture("自动配置未采集到可用登录态")
		}
	} else if !isPlatformSiteScriptAuthType(authType) {
		return PlatformSiteCaptureResolution{}, nil, errorsForCapture("采集认证方式不受支持")
	}
	diagnostics := request.Diagnostics
	if record.AuthType == PlatformSiteCaptureAuthAuto {
		if diagnostics == nil {
			return PlatformSiteCaptureResolution{}, nil, errorsForCapture("自动配置缺少认证验证诊断")
		}
		switch authType {
		case model.UpstreamAuthAccessToken, model.UpstreamAuthCookie:
			if diagnostics == nil || !diagnostics.AuthUserVerified {
				return PlatformSiteCaptureResolution{}, nil, errorsForCapture("采集登录态未通过当前用户接口验证")
			}
		case model.UpstreamAuthAdminKey:
			if diagnostics == nil || !diagnostics.AdminKeyVerified {
				return PlatformSiteCaptureResolution{}, nil, errorsForCapture("采集 Admin Key 未通过管理权限验证")
			}
		}
	}
	credential := model.PlatformSiteCredential{AuthType: authType}
	switch authType {
	case model.UpstreamAuthAccessToken:
		accessToken := strings.TrimSpace(request.AccessToken)
		if accessToken == "" {
			return PlatformSiteCaptureResolution{}, nil, errorsForCapture("未采集到 Access Token")
		}
		if strictAuthType && (strings.TrimSpace(request.AdminKey) != "" || strings.TrimSpace(request.Cookie) != "") {
			return PlatformSiteCaptureResolution{}, nil, errorsForCapture("采集结果不是 Access Token")
		}
		userID := captureUserID(request)
		if userID != "" && !isNumericCaptureUserID(userID) {
			return PlatformSiteCaptureResolution{}, nil, errorsForCapture("NewAPI 用户 ID 必须是数字")
		}
		credential.AccessToken = accessToken
		credential.RefreshToken = strings.TrimSpace(request.RefreshToken)
		credential.UserID = userID
		credential.TokenExpiresAt = captureTokenExpiresAt(request)
		credential.Username = strings.TrimSpace(request.Username)
		credential.SessionID = strings.TrimSpace(request.SessionID)
		credential.SessionCurrent = credential.SessionID != ""
	case model.UpstreamAuthAdminKey:
		adminKey := strings.TrimSpace(request.AdminKey)
		if adminKey == "" {
			return PlatformSiteCaptureResolution{}, nil, errorsForCapture("未采集到 Admin Key")
		}
		if strictAuthType && (strings.TrimSpace(request.AccessToken) != "" ||
			strings.TrimSpace(request.RefreshToken) != "" ||
			strings.TrimSpace(request.Cookie) != "") {
			return PlatformSiteCaptureResolution{}, nil, errorsForCapture("采集结果不是 Admin Key")
		}
		credential.AdminKey = adminKey
		credential.UserID = captureUserID(request)
		credential.Username = strings.TrimSpace(request.Username)
	case model.UpstreamAuthCookie:
		cookie := strings.TrimSpace(request.Cookie)
		if cookie == "" {
			return PlatformSiteCaptureResolution{}, nil, errorsForCapture("未读取到可用 Cookie，HttpOnly Cookie 无法通过浏览器脚本采集")
		}
		if strictAuthType && (strings.TrimSpace(request.AccessToken) != "" ||
			strings.TrimSpace(request.RefreshToken) != "" ||
			strings.TrimSpace(request.AdminKey) != "") {
			return PlatformSiteCaptureResolution{}, nil, errorsForCapture("采集结果不是 Cookie")
		}
		credential.Cookie = cookie
		credential.UserID = captureUserID(request)
		credential.Username = strings.TrimSpace(request.Username)
	default:
		return PlatformSiteCaptureResolution{}, nil, errorsForCapture("采集认证方式不受支持")
	}
	summary := &PlatformSiteCaptureSummary{
		Platform:            record.Platform,
		AuthType:            authType,
		BaseURL:             record.BaseURL,
		ManagementBaseURL:   managementBaseURL,
		RelayBaseURL:        relayBaseURL,
		APIBaseURL:          apiBaseURL,
		Origin:              record.Origin,
		AccessTokenMasked:   maskPlatformSiteCaptureToken(credential.AccessToken),
		RefreshTokenPresent: strings.TrimSpace(credential.RefreshToken) != "",
		AdminKeyPresent:     strings.TrimSpace(credential.AdminKey) != "",
		CookiePresent:       strings.TrimSpace(credential.Cookie) != "",
		UserID:              credential.UserID,
		Username:            strings.TrimSpace(request.Username),
		Email:               strings.TrimSpace(request.Email),
		TokenExpiresAt:      credential.TokenExpiresAt,
		CapturedAt:          common.GetTimestamp(),
		CaptureSource:       strings.TrimSpace(request.CaptureSource),
		Diagnostics:         sanitizePlatformSiteCaptureDiagnostics(request.Diagnostics),
	}
	return PlatformSiteCaptureResolution{
		Platform:          record.Platform,
		Credential:        credential,
		ManagementBaseURL: managementBaseURL,
		RelayBaseURL:      relayBaseURL,
		APIBaseURL:        apiBaseURL,
	}, summary, nil
}

func isPlatformSiteScriptAuthType(authType string) bool {
	switch strings.ToLower(strings.TrimSpace(authType)) {
	case model.UpstreamAuthAccessToken, model.UpstreamAuthAdminKey, model.UpstreamAuthCookie:
		return true
	default:
		return false
	}
}

func isPlatformSiteCaptureAuthType(authType string) bool {
	authType = strings.ToLower(strings.TrimSpace(authType))
	return authType == PlatformSiteCaptureAuthAuto || isPlatformSiteScriptAuthType(authType)
}

func selectPlatformSiteCaptureAuthType(request PlatformSiteCaptureCompleteRequest) string {
	if strings.TrimSpace(request.AccessToken) != "" {
		return model.UpstreamAuthAccessToken
	}
	if strings.TrimSpace(request.AdminKey) != "" {
		return model.UpstreamAuthAdminKey
	}
	if strings.TrimSpace(request.Cookie) != "" {
		return model.UpstreamAuthCookie
	}
	return ""
}

func firstCaptureURL(values ...string) (string, error) {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		normalized, err := normalizePlatformSiteURL(value)
		if err != nil {
			return "", errorsForCapture("采集页面地址格式错误")
		}
		if err := validatePlatformSiteURL(normalized); err != nil {
			return "", errorsForCapture("采集页面地址不受支持")
		}
		return normalized, nil
	}
	return "", nil
}

func captureUserID(request PlatformSiteCaptureCompleteRequest) string {
	if userID := strings.TrimSpace(request.UserID); userID != "" {
		return userID
	}
	return firstNumericCaptureValue(request.AuthUser, 0, "id", "user_id", "userId", "uid", "sub")
}

func firstNumericCaptureValue(value any, depth int, names ...string) string {
	if value == nil || depth > 5 {
		return ""
	}
	switch item := value.(type) {
	case map[string]any:
		for key, child := range item {
			for _, name := range names {
				if strings.EqualFold(key, name) {
					text := strings.TrimSpace(fmt.Sprint(child))
					if isNumericCaptureUserID(text) {
						return text
					}
				}
			}
			if found := firstNumericCaptureValue(child, depth+1, names...); found != "" {
				return found
			}
		}
	case []any:
		for _, child := range item {
			if found := firstNumericCaptureValue(child, depth+1, names...); found != "" {
				return found
			}
		}
	}
	return ""
}

func captureTokenExpiresAt(request PlatformSiteCaptureCompleteRequest) int64 {
	if request.TokenExpiresAt > 0 {
		return normalizeCaptureUnixSeconds(request.TokenExpiresAt)
	}
	if request.ExpiresIn > 0 {
		return time.Now().Add(time.Duration(request.ExpiresIn) * time.Second).Unix()
	}
	return 0
}

func normalizeCaptureUnixSeconds(value int64) int64 {
	if value > 1_000_000_000_000 {
		return value / 1000
	}
	return value
}

func isNumericCaptureUserID(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func captureRelatedPlatformURL(baseURL, candidate string) bool {
	if validatePlatformSiteURL(baseURL) != nil || validatePlatformSiteURL(candidate) != nil {
		return false
	}
	base, baseErr := url.Parse(baseURL)
	other, otherErr := url.Parse(candidate)
	if baseErr != nil || otherErr != nil {
		return false
	}
	if !strings.EqualFold(base.Scheme, other.Scheme) ||
		platformSiteEffectivePort(base) != platformSiteEffectivePort(other) {
		return false
	}
	if strings.EqualFold(base.Hostname(), other.Hostname()) {
		return true
	}
	baseHost := normalizePlatformHostname(base.Hostname())
	otherHost := normalizePlatformHostname(other.Hostname())
	return baseHost == "api."+otherHost || otherHost == "api."+baseHost
}

func platformSiteOrigin(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errorsForCapture("目标站地址格式错误")
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}

func normalizeCaptureNexusBaseURL(raw string) (string, error) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", errorsForCapture("NexusTok 地址格式错误")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func normalizePlatformSiteCaptureReturnURL(raw, nexusBaseURL string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	target, err := url.Parse(raw)
	if err != nil || target.Scheme == "" || target.Host == "" ||
		(target.Scheme != "http" && target.Scheme != "https") {
		return "", errorsForCapture("采集返回地址格式错误")
	}
	base, err := url.Parse(nexusBaseURL)
	if err != nil || !strings.EqualFold(target.Scheme, base.Scheme) ||
		!strings.EqualFold(target.Host, base.Host) {
		return "", errorsForCapture("采集返回地址必须属于 NexusTok")
	}
	target.RawQuery = ""
	target.Fragment = ""
	return strings.TrimRight(target.String(), "/"), nil
}

func captureUserscriptURL(nexusBaseURL, captureID, installToken string) string {
	return strings.TrimRight(nexusBaseURL, "/") +
		"/api/channel/platform-site/capture-session/" +
		url.PathEscape(captureID) +
		"/userscript.user.js?install_token=" + url.QueryEscape(installToken)
}

func captureHandoffURL(record platformSiteCaptureRecord, nexusBaseURL string) string {
	payload, err := common.Marshal(map[string]any{
		"capture_id":     record.ID,
		"capture_secret": record.Secret,
		"platform":       record.Platform,
		"auth_type":      record.AuthType,
		"base_url":       record.BaseURL,
		"origin":         record.Origin,
		"return_url":     record.ReturnURL,
		"helper_version": platformSiteCaptureHelperVersion,
		"complete_url": strings.TrimRight(nexusBaseURL, "/") +
			"/api/channel/platform-site/capture-session/" + url.PathEscape(record.ID) + "/complete",
		"expires_at": record.ExpiresAt,
	})
	if err != nil {
		return record.BaseURL
	}
	return record.BaseURL + "?" + platformSiteCaptureHandoffParam + "=" +
		base64.RawURLEncoding.EncodeToString(payload)
}

func maskPlatformSiteCaptureToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= 6 {
		return "***"
	}
	return value[:3] + "..." + value[len(value)-3:]
}

func safePlatformSiteCaptureFailure(value string) string {
	lower := strings.ToLower(value)
	switch {
	case strings.Contains(lower, "automatic") ||
		strings.Contains(lower, "auto") ||
		strings.Contains(value, "自动配置"):
		return "自动配置未采集到可用登录态"
	case strings.Contains(lower, "cookie"):
		return "未读取到可用 Cookie"
	case strings.Contains(lower, "admin"):
		return "未读取到可用 Admin Key"
	case strings.Contains(lower, "token"):
		return "未读取到可用 Access Token"
	case strings.Contains(lower, "expired"):
		return "采集会话已过期"
	default:
		return "上游登录态采集失败"
	}
}

func errorsForCapture(message string) error {
	return fmt.Errorf("%s", message)
}

const platformSiteCaptureScriptTemplate = `// ==UserScript==
// @name         NexusTok Platform Site Capture
// @namespace    https://github.com/c1cadaBob/NexusTok
// @version      __NEXUSTOK_HELPER_VERSION__
// @description  Capture a verified NewAPI or Sub2API browser login state for NexusTok.
// @match        __NEXUSTOK_MATCH__
// @run-at       document-start
// @grant        GM_xmlhttpRequest
// @grant        GM_cookie
// @grant        unsafeWindow
// @connect      __NEXUSTOK_CONNECT__
// ==/UserScript==

(function () {
  'use strict';
  const config = __NEXUSTOK_CONFIG__;
  const pageWindow = typeof unsafeWindow !== 'undefined' ? unsafeWindow : window;
  const readyEvent = 'nexustok-upstream-capture-helper-ready';
  const platformStrategies = {
    newapi: {
      candidates: ['dashboard_refresh', 'access_token', 'admin_key', 'cookie'],
      mePaths: ['/api/user/self', '/api/user/me', '/api/user/profile', '/api/user/info'],
    },
    sub2api: {
      candidates: ['auth_token', 'auth_user', 'refresh_token', 'browser_restore', 'access_token', 'admin_key', 'cookie'],
      mePaths: ['/api/v1/auth/me', '/api/auth/me', '/auth/me'],
    },
  };
  const text = (value) => value == null ? '' : String(value).trim();

  function parseJSON(value) {
    try { return value ? JSON.parse(value) : null; } catch (_) { return null; }
  }

  function storageKeys(storage) {
    const keys = [];
    try {
      for (let index = 0; index < storage.length && keys.length < 64; index += 1) {
        const key = text(storage.key(index));
        if (key && key.length <= 128) keys.push(key);
      }
    } catch (_) {}
    return keys;
  }

  function diagnosticsBase(source) {
    return {
      source: text(source),
      helper_version: config.version || '__NEXUSTOK_HELPER_VERSION__',
      helper_required_version: '__NEXUSTOK_HELPER_VERSION__',
      page_origin: text(pageWindow.location && pageWindow.location.origin),
      api_base_url_seen: '',
      local_storage_keys: storageKeys(pageWindow.localStorage),
      session_storage_keys: storageKeys(pageWindow.sessionStorage),
      auth_token_present: false,
      access_token_present: false,
      refresh_token_present: false,
      admin_key_present: false,
      cookie_present: false,
      oauth_hash_token_present: false,
      auth_client_id_present: false,
      auth_user_verified: false,
      admin_key_verified: false,
      auth_me_path: '',
      admin_verification_path: '',
      browser_session_restore_path: '',
      browser_session_restore_status: 'not_attempted',
      browser_session_restore_message: '',
      failure_stage: '',
      failure_reason: '',
      verification_endpoints: [],
    };
  }

  function markAttempt(diagnostics, path) {
    if (!diagnostics || !path) return;
    diagnostics.verification_endpoints = Array.from(
      new Set([...(diagnostics.verification_endpoints || []), text(path)])
    ).slice(0, 16);
  }

  function mergeDiagnostics(target, source) {
    if (!target || !source) return target;
    for (const key of [
      'auth_token_present',
      'access_token_present',
      'refresh_token_present',
      'admin_key_present',
      'cookie_present',
      'oauth_hash_token_present',
      'auth_client_id_present',
      'auth_user_verified',
      'admin_key_verified',
    ]) {
      target[key] = Boolean(target[key] || source[key]);
    }
    for (const key of [
      'api_base_url_seen',
      'auth_me_path',
      'admin_verification_path',
      'browser_session_restore_path',
      'browser_session_restore_status',
      'browser_session_restore_message',
    ]) {
      if (!text(target[key]) && text(source[key])) target[key] = text(source[key]);
    }
    for (const key of [
      'local_storage_keys',
      'session_storage_keys',
      'verification_endpoints',
    ]) {
      target[key] = Array.from(
        new Set([...(target[key] || []), ...(source[key] || [])])
      ).slice(0, key === 'verification_endpoints' ? 16 : 64);
    }
    return target;
  }

  function nestedValue(value, names, depth) {
    if (!value || depth > 5) return '';
    if (Array.isArray(value)) {
      for (const item of value) {
        const found = nestedValue(item, names, depth + 1);
        if (found) return found;
      }
      return '';
    }
    if (typeof value !== 'object') return '';
    for (const [key, child] of Object.entries(value)) {
      const normalizedKey = key.toLowerCase();
      if (
        names.some((name) => normalizedKey === String(name).toLowerCase()) &&
        child != null &&
        typeof child !== 'object'
      ) {
        const found = text(child).replace(/^Bearer\s+/i, '');
        if (found) return found;
      }
      const nested = nestedValue(child, names, depth + 1);
      if (nested) return nested;
    }
    return '';
  }

  function readStorage(name) {
    for (const storage of [pageWindow.localStorage, pageWindow.sessionStorage]) {
      try {
        const raw = storage.getItem(name);
        if (text(raw)) return raw;
      } catch (_) {}
    }
    return '';
  }

  function readHashValue(names) {
    try {
      const rawHash = text(pageWindow.location.hash || '').replace(/^#/, '');
      const candidates = [rawHash];
      const queryIndex = rawHash.indexOf('?');
      if (queryIndex >= 0) candidates.push(rawHash.slice(queryIndex + 1));
      for (const candidate of candidates) {
        const params = new URLSearchParams(candidate);
        for (const name of names) {
          const value = text(params.get(name));
          if (value) return value;
        }
      }
    } catch (_) {}
    return '';
  }

  function readDeepStorageValue(names) {
    for (const storage of [pageWindow.localStorage, pageWindow.sessionStorage]) {
      try {
        for (let index = 0; index < storage.length && index < 128; index += 1) {
          const key = text(storage.key(index));
          if (!/(auth|token|session|user)/i.test(key)) continue;
          const parsed = parseJSON(storage.getItem(key));
          const value = nestedValue(parsed, names, 0);
          if (value) return value;
        }
      } catch (_) {}
    }
    return '';
  }

  function readNamed(names, nestedNames) {
    for (const name of names) {
      const raw = readStorage(name);
      if (!raw) continue;
      const parsed = parseJSON(raw);
      const value = parsed ? nestedValue(parsed, nestedNames, 0) : text(raw);
      if (value) return value;
    }
    try {
      const appConfig = pageWindow.__APP_CONFIG__ || {};
      const value = nestedValue(appConfig, nestedNames, 0);
      if (value) return value;
    } catch (_) {}
    for (const stateName of [
      '__INITIAL_STATE__',
      '__APP_STATE__',
      '__NUXT__',
      '__NEXT_DATA__',
      '__PINIA__',
    ]) {
      try {
        const value = nestedValue(pageWindow[stateName], nestedNames, 0);
        if (value) return value;
      } catch (_) {}
    }
    return '';
  }

  function readStructured(names) {
    for (const name of names) {
      const raw = readStorage(name);
      if (!raw) continue;
      const parsed = parseJSON(raw);
      if (parsed && typeof parsed === 'object') return parsed;
    }
    try {
      const appConfig = pageWindow.__APP_CONFIG__ || {};
      for (const name of names) {
        const value = appConfig[name];
        if (value && typeof value === 'object') return value;
      }
    } catch (_) {}
    return null;
  }

  function readCookieHeader(targetURL) {
    let cookieURL = window.location.origin;
    try {
      const parsed = new URL(text(targetURL || ''), window.location.href);
      if (
        parsed.protocol === window.location.protocol &&
        (parsed.host === window.location.host ||
          parsed.host === 'api.' + window.location.host ||
          window.location.host === 'api.' + parsed.host)
      ) {
        cookieURL = parsed.origin;
      }
    } catch (_) {}
    let visibleCookies = '';
    try {
      visibleCookies = text(document.cookie);
    } catch (_) {}
    if (cookieURL === window.location.origin && visibleCookies) {
      return Promise.resolve(visibleCookies);
    }
    if (typeof GM_cookie !== 'object' || typeof GM_cookie.list !== 'function') {
      return Promise.resolve(visibleCookies);
    }
    return new Promise((resolve) => {
      try {
        GM_cookie.list({ url: cookieURL }, (cookies, error) => {
          if (error || !Array.isArray(cookies)) {
            resolve(visibleCookies);
            return;
          }
          const values = cookies
            .filter((cookie) => cookie && text(cookie.name))
            .map((cookie) => text(cookie.name) + '=' + text(cookie.value));
          resolve(values.join('; ') || visibleCookies);
        });
      } catch (_) {
        resolve(visibleCookies);
      }
    });
  }

  function readUserID(value) {
    const raw = text(value);
    return /^\d+$/.test(raw) ? raw : '';
  }

  function userIDFromToken(token) {
    try {
      const parts = text(token).split('.');
      if (parts.length < 2) return '';
      const normalized = parts[1].replace(/-/g, '+').replace(/_/g, '/');
      const padded = normalized + '='.repeat((4 - normalized.length % 4) % 4);
      const payload = parseJSON(decodeURIComponent(escape(atob(padded)))) || {};
      return readUserID(nestedValue(payload, ['id', 'user_id', 'userid', 'uid', 'sub'], 0));
    } catch (_) {
      return '';
    }
  }

  function normalizeExpiresAt(value) {
    const parsed = Number(value);
    if (!Number.isFinite(parsed) || parsed <= 0) return 0;
    return parsed > 1000000000000 ? Math.floor(parsed / 1000) : Math.floor(parsed);
  }

  function tokenFromResponse(payload) {
    return nestedValue(payload, ['access_token', 'accessToken', 'auth_token', 'authToken', 'token', 'jwt'], 0)
      .replace(/^Bearer\s+/i, '');
  }

  function refreshTokenFromResponse(payload) {
    return nestedValue(payload, ['refresh_token', 'refreshToken', 'refresh'], 0);
  }

  function expiryFromResponse(payload) {
    const expiresAt = normalizeExpiresAt(
      nestedValue(payload, ['token_expires_at', 'access_expires_at', 'expires_at', 'expiresAt'], 0)
    );
    if (expiresAt > 0) return expiresAt;
    const expiresIn = Number(nestedValue(payload, ['expires_in', 'expiresIn'], 0));
    return Number.isFinite(expiresIn) && expiresIn > 0
      ? Math.floor(Date.now() / 1000) + Math.floor(expiresIn)
      : 0;
  }

  function decodeHandoff(value) {
    try {
      const normalized = value.replace(/-/g, '+').replace(/_/g, '/');
      const padded = normalized + '='.repeat((4 - normalized.length % 4) % 4);
      return parseJSON(decodeURIComponent(escape(atob(padded)))) || null;
    } catch (_) {
      return null;
    }
  }

  function handoff() {
    const current = new URL(window.location.href);
    const encoded = current.searchParams.get('nexustok_capture');
    if (encoded) {
      const value = decodeHandoff(encoded);
      current.searchParams.delete('nexustok_capture');
      try { history.replaceState({}, document.title, current.toString()); } catch (_) {}
      if (value) {
        try {
          pageWindow.sessionStorage.setItem(
            'nexustok_platform_site_capture_handoff',
            JSON.stringify(value)
          );
        } catch (_) {}
      }
      return value;
    }
    try {
      return parseJSON(pageWindow.sessionStorage.getItem(
        'nexustok_platform_site_capture_handoff'
      ));
    } catch (_) {
      return null;
    }
  }

  function clearStoredHandoff() {
    try {
      pageWindow.sessionStorage.removeItem('nexustok_platform_site_capture_handoff');
    } catch (_) {}
  }

  function apiBaseURL(payload) {
    const candidates = [
      payload.api_base_url,
      pageWindow.__APP_CONFIG__ && pageWindow.__APP_CONFIG__.api_base_url,
      pageWindow.__APP_CONFIG__ && pageWindow.__APP_CONFIG__.apiBaseUrl,
      readStorage('api_base_url'),
    ];
    for (const value of candidates) {
      try {
        const parsed = new URL(text(value), window.location.origin);
        if (parsed.protocol === window.location.protocol &&
            (parsed.host === window.location.host ||
             parsed.host === 'api.' + window.location.host ||
             window.location.host === 'api.' + parsed.host)) {
          return parsed.origin;
        }
      } catch (_) {}
    }
    return window.location.origin;
  }

  function isRelatedUpstreamURL(rawURL) {
    try {
      const page = new URL(window.location.href);
      const target = new URL(text(rawURL), page.href);
      if (target.protocol !== page.protocol || target.port !== page.port) return false;
      if (target.hostname === page.hostname) return true;
      return target.hostname === 'api.' + page.hostname ||
        page.hostname === 'api.' + target.hostname;
    } catch (_) {
      return false;
    }
  }

  function gmJSONRequest(url, options) {
    return new Promise((resolve, reject) => {
      const requestOptions = options || {};
      const headers = {
        Accept: 'application/json',
        ...(requestOptions.headers || {}),
      };
      const cookie = text(requestOptions.cookie);
      if (cookie) headers.Cookie = cookie;
      const request = {
        method: text(requestOptions.method) || 'GET',
        url,
        headers,
        data: requestOptions.body,
        withCredentials: false,
        timeout: 15000,
        onload: (response) => {
          const parsed = parseJSON(response.responseText);
          if (
            response.status < 200 ||
            response.status >= 300 ||
            !responseSucceeded(parsed)
          ) {
            const error = new Error('upstream request failed');
            error.requestCompleted = true;
            error.status = response.status;
            reject(error);
            return;
          }
          resolve(parsed || {});
        },
        onerror: () => reject(new Error('upstream request unavailable')),
        ontimeout: () => reject(new Error('upstream request timed out')),
      };
      try {
        GM_xmlhttpRequest(request);
      } catch (_) {
        reject(new Error('upstream request unavailable'));
      }
    });
  }

  async function jsonRequest(url, options) {
    const requestOptions = options || {};
    const cookie = text(requestOptions.cookie);
    if (
      cookie &&
      typeof GM_xmlhttpRequest === 'function' &&
      isRelatedUpstreamURL(url)
    ) {
      try {
        return await gmJSONRequest(url, requestOptions);
      } catch (error) {
        if (error && error.requestCompleted) throw error;
      }
    }
    const fetchOptions = { ...requestOptions };
    delete fetchOptions.cookie;
    delete fetchOptions.headers;
    const response = await fetch(url, {
      ...fetchOptions,
      credentials: 'include',
      headers: { Accept: 'application/json', ...(requestOptions.headers || {}) },
    });
    const body = await response.text();
    const parsed = parseJSON(body);
    if (!response.ok || !responseSucceeded(parsed)) {
      const error = new Error('upstream request failed');
      error.status = response.status;
      throw error;
    }
    return parsed || {};
  }

  function responseSucceeded(payload) {
    if (!payload || typeof payload !== 'object') return true;
    if (payload.success === false) return false;
    const code = payload.code;
    if (typeof code === 'number') return code === 0 || code === 200;
    if (typeof code === 'string' && code.trim()) {
      const normalized = code.trim().toLowerCase();
      return normalized === '0' || normalized === '200' || normalized === 'success';
    }
    return true;
  }

  async function restoreSub2APIBrowserSession(apiBase) {
    const diagnostics = diagnosticsBase('browser_session_restore');
    let clientID = readNamed(
      ['sub2api_auth_client_id'],
      ['sub2api_auth_client_id']
    );
    diagnostics.auth_client_id_present = Boolean(clientID);
    const paths = [
      '/api/v1/auth/session/restore',
      '/api/auth/session/restore',
      '/auth/session/restore',
    ];
    for (const path of paths) {
      try {
        markAttempt(diagnostics, path);
        const restored = await jsonRequest(new URL(path, apiBase).toString(), {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            ...(clientID ? { 'X-Sub2API-Auth-Client': clientID } : {}),
          },
          body: '{}',
        });
        const accessToken = tokenFromResponse(restored);
        if (!accessToken) continue;
        diagnostics.browser_session_restore_path = path;
        diagnostics.browser_session_restore_status = 'authenticated';
        return {
          accessToken,
          refreshToken: refreshTokenFromResponse(restored),
          expiresAt: expiryFromResponse(restored),
          authUser: restored && (restored.data || restored.user || restored),
          diagnostics,
        };
      } catch (_) {}
    }
    diagnostics.browser_session_restore_status = 'failed';
    diagnostics.browser_session_restore_message = '浏览器会话恢复接口未返回可用访问令牌';
    return { diagnostics };
  }

  async function readNewAPIAuthBundle(apiBase) {
    try {
      const cookie = await readCookieHeader(apiBase);
      const payload = await jsonRequest(
        new URL('/api/user/auth/refresh', apiBase).toString(),
        {
          method: 'POST',
          headers: { Accept: 'application/json' },
          cookie,
        }
      );
      const data = payload && payload.data;
      const session = data && data.session;
      if (
        !payload ||
        payload.success !== true ||
        !data ||
        data.token_type !== 'Bearer' ||
        !session ||
        session.current !== true
      ) {
        return null;
      }
      const accessToken = text(data.access_token).replace(/^Bearer\s+/i, '');
      const expiresAt = normalizeExpiresAt(data.access_expires_at);
      if (!accessToken || !session.sid || !expiresAt || expiresAt <= Math.floor(Date.now() / 1000)) {
        return null;
      }
      return {
        accessToken,
        refreshToken: '',
        expiresAt,
        authUser: data.user || {},
        sessionID: text(session.sid),
        cookie,
      };
    } catch (_) {
      return null;
    }
  }

  async function send(payload) {
    const body = JSON.stringify(payload);
    if (typeof GM_xmlhttpRequest === 'function') {
      return new Promise((resolve, reject) => {
        GM_xmlhttpRequest({
          method: 'POST',
          url: payload.complete_url,
          headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
          data: body,
          withCredentials: false,
          onload: (response) => {
            const result = parseJSON(response.responseText) || {};
            if (response.status >= 200 && response.status < 300) resolve(result);
            else reject(new Error('HTTP ' + response.status));
          },
          onerror: () => reject(new Error('capture callback failed')),
        });
      });
    }
    return jsonRequest(payload.complete_url, {
      method: 'POST',
      credentials: 'omit',
      headers: { 'Content-Type': 'application/json' },
      body,
    });
  }

  async function verifyCurrentUser(result, platform, accessToken, cookie, sessionID) {
    const diagnostics = result.diagnostics || diagnosticsBase('verification');
    const strategy = platformStrategies[platform] || platformStrategies.newapi;
    const headers = {};
    if (accessToken) headers.Authorization = 'Bearer ' + accessToken;
    if (sessionID) headers['X-Auth-Session'] = sessionID;
    let validatedUser = null;
    for (const mePath of strategy.mePaths) {
      try {
        markAttempt(diagnostics, mePath);
        const me = await jsonRequest(
          new URL(mePath, result.api_base_url).toString(),
          { headers, cookie }
        );
        validatedUser = me && (me.data || me.user || me);
        const userID = nestedValue(validatedUser, ['id', 'user_id', 'userid', 'uid', 'sub'], 0);
        const username = nestedValue(
          validatedUser,
          ['username', 'user_name', 'login', 'email', 'mail'],
          0
        );
        if (!userID && !username) {
          validatedUser = null;
          continue;
        }
        diagnostics.auth_me_path = mePath;
        break;
      } catch (_) {}
    }
    if (!validatedUser) throw new Error('current user validation failed');
    diagnostics.auth_user_verified = true;
    result.auth_user = validatedUser;
    const id = nestedValue(validatedUser, ['id', 'user_id', 'userid', 'uid', 'sub'], 0);
    if (/^\d+$/.test(text(id))) result.user_id = text(id);
    if (!result.user_id && platform === 'newapi' && accessToken) {
      result.user_id = userIDFromToken(accessToken);
    }
    result.username = text(
      nestedValue(validatedUser, ['username', 'user_name', 'login'], 0)
    ) || text(nestedValue(validatedUser, ['email', 'mail'], 0));
    result.email = text(nestedValue(validatedUser, ['email', 'mail'], 0));
    return validatedUser;
  }

  async function fillCookieCredential(result, platform) {
    result.cookie = await readCookieHeader(result.api_base_url);
    result.diagnostics.cookie_present = Boolean(result.cookie);
    if (!result.cookie) throw new Error('cookie is not readable; HttpOnly cookie cannot be captured');
    await verifyCurrentUser(result, platform, '', result.cookie, '');
  }

  async function fillAdminKeyCredential(result, platform) {
    result.admin_key = readNamed(
      ['admin_key', 'adminKey', 'admin_token', 'adminToken', 'x-api-key', 'New-Api-Key'],
      ['admin_key', 'adminkey', 'admin_token', 'admintoken', 'x-api-key', 'new-api-key']
    );
    result.diagnostics.admin_key_present = Boolean(result.admin_key);
    if (!result.admin_key) throw new Error('admin key was not found in an explicitly named field');
    const headers = {
      Authorization: 'Bearer ' + result.admin_key,
      'x-api-key': result.admin_key,
      'New-Api-Key': result.admin_key,
    };
    const paths = platform === 'newapi'
      ? ['/api/channel/?p=1&page_size=1', '/api/channel/']
      : ['/api/v1/admin/accounts', '/api/v1/admin/dashboard'];
    for (const path of paths) {
      try {
        markAttempt(result.diagnostics, path);
        await jsonRequest(new URL(path, result.api_base_url).toString(), { headers });
        result.diagnostics.admin_key_verified = true;
        result.diagnostics.admin_verification_path = path;
        return;
      } catch (_) {}
    }
    throw new Error('admin key validation failed');
  }

  async function readTokenCredential(result, platform, source) {
    const apiBase = result.api_base_url;
    let accessToken = '';
    let refreshToken = '';
    let expiresAt = 0;
    let sessionID = '';
    let cookie = '';
    let storedAuthUser = null;
    if (source === 'dashboard_refresh') {
      const bundle = await readNewAPIAuthBundle(apiBase);
      if (!bundle) throw new Error('NewAPI Dashboard refresh session is unavailable');
      accessToken = bundle.accessToken;
      refreshToken = bundle.refreshToken;
      expiresAt = bundle.expiresAt;
      sessionID = bundle.sessionID || '';
      cookie = bundle.cookie || '';
      storedAuthUser = bundle.authUser;
      markAttempt(result.diagnostics, '/api/user/auth/refresh');
    } else if (source === 'browser_restore') {
      const restored = await restoreSub2APIBrowserSession(apiBase);
      Object.assign(result.diagnostics, restored.diagnostics || {});
      if (!restored.accessToken) throw new Error('Sub2API browser session restore did not return an access token');
      accessToken = restored.accessToken;
      refreshToken = restored.refreshToken || '';
      expiresAt = restored.expiresAt || 0;
      storedAuthUser = restored.authUser;
    } else {
      accessToken = readHashValue(
        ['auth_token', 'access_token', 'token', 'jwt']
      ) || readNamed(
        ['auth_token', 'access_token', 'accessToken', 'token', 'jwt'],
        ['auth_token', 'access_token', 'accesstoken', 'token', 'jwt']
      ) || readDeepStorageValue(
        ['access_token', 'auth_token', 'token', 'jwt']
      );
      refreshToken = readHashValue(
        ['refresh_token', 'refreshToken', 'rt']
      ) || readNamed(
        ['refresh_token', 'refreshToken', 'sub2api_refresh_token'],
        ['refresh_token', 'refreshtoken', 'rt']
      ) || readDeepStorageValue(
        ['refresh_token', 'refreshtoken', 'rt']
      );
      expiresAt = normalizeExpiresAt(readHashValue(
        ['token_expires_at', 'tokenExpiresAt', 'expires_at', 'expiresAt']
      ) || readNamed(
        ['token_expires_at', 'tokenExpiresAt', 'expires_at', 'expiresAt'],
        ['token_expires_at', 'tokenexpiresat', 'expires_at', 'expiresat']
      ) || readDeepStorageValue(
        ['token_expires_at', 'tokenexpiresat', 'expires_at', 'expiresat']
      ));
      storedAuthUser = readStructured([
        'auth_user',
        'authUser',
        'current_user',
        'currentUser',
        'user',
      ]);
      if (source === 'auth_user' && !storedAuthUser) {
        throw new Error('Sub2API auth_user login state was not found');
      }
      if (source === 'auth_user' && !accessToken) {
        accessToken = tokenFromResponse(storedAuthUser);
      }
      if (!accessToken && platform === 'sub2api' && refreshToken) {
        const refreshed = await jsonRequest(new URL('/api/v1/auth/refresh', apiBase).toString(), {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ refresh_token: refreshToken }),
        });
        markAttempt(result.diagnostics, '/api/v1/auth/refresh');
        accessToken = tokenFromResponse(refreshed);
        refreshToken = refreshTokenFromResponse(refreshed) || refreshToken;
        expiresAt = expiryFromResponse(refreshed) || expiresAt;
      }
    }
    result.diagnostics.auth_token_present = Boolean(
      source === 'auth_token' && accessToken
    );
    result.diagnostics.access_token_present = Boolean(accessToken);
    result.diagnostics.refresh_token_present = Boolean(refreshToken);
    if (!accessToken) throw new Error('access token was not found in browser login state');
    const storedUserID = nestedValue(
      storedAuthUser,
      ['id', 'user_id', 'userid', 'uid', 'sub'],
      0
    );
    if (/^\d+$/.test(text(storedUserID))) result.user_id = text(storedUserID);
    if (storedAuthUser && typeof storedAuthUser === 'object') {
      result.auth_user = storedAuthUser;
    }
    result.diagnostics.cookie_present = Boolean(cookie);
    await verifyCurrentUser(result, platform, accessToken, cookie, sessionID);
    result.access_token = accessToken;
    result.refresh_token = refreshToken;
    if (expiresAt > 0) result.token_expires_at = expiresAt;
    result.session_id = sessionID;
    result.cookie = cookie;
  }

  async function fillSelectedCredential(result, platform, candidate) {
    if (candidate === 'dashboard_refresh' ||
        candidate === 'auth_token' ||
        candidate === 'auth_user' ||
        candidate === 'refresh_token' ||
        candidate === 'browser_restore' ||
        candidate === 'access_token') {
      await readTokenCredential(result, platform, candidate);
      return;
    }
    if (candidate === 'admin_key') {
      await fillAdminKeyCredential(result, platform);
      return;
    }
    if (candidate === 'cookie') {
      await fillCookieCredential(result, platform);
      return;
    }
    throw new Error('unsupported capture candidate');
  }

  async function collect(payload) {
    if (!payload || !payload.capture_secret || payload.origin !== window.location.origin) {
      throw new Error('capture handoff is invalid');
    }
    const platform = text(payload.platform).toLowerCase();
    const authType = text(payload.auth_type).toLowerCase();
    const result = {
      capture_secret: payload.capture_secret,
      complete_url: payload.complete_url,
      capture_source: 'capture_helper',
      helper_version: config.version || '__NEXUSTOK_HELPER_VERSION__',
      platform,
      auth_type: authType,
      base_url: window.location.origin,
      origin: window.location.origin,
      api_base_url: apiBaseURL(payload),
      diagnostics: diagnosticsBase('capture_helper'),
    };
    result.diagnostics.api_base_url_seen = result.api_base_url;
    if (authType === 'auto') {
      let selected = null;
      const strategy = platformStrategies[platform] || platformStrategies.newapi;
      const aggregateDiagnostics = result.diagnostics;
      for (const candidateType of strategy.candidates) {
        const candidate = {
          ...result,
          auth_type: candidateType === 'admin_key' || candidateType === 'cookie'
            ? candidateType
            : 'access_token',
          diagnostics: { ...result.diagnostics, verification_endpoints: [] },
        };
        try {
          await fillSelectedCredential(candidate, platform, candidateType);
          mergeDiagnostics(aggregateDiagnostics, candidate.diagnostics);
          candidate.diagnostics = aggregateDiagnostics;
          selected = candidate;
          break;
        } catch (error) {
          mergeDiagnostics(aggregateDiagnostics, candidate.diagnostics);
          aggregateDiagnostics.failure_stage = candidateType;
          aggregateDiagnostics.failure_reason = text(error && error.message);
        }
      }
      if (!selected) {
        aggregateDiagnostics.failure_stage = 'automatic_candidates_exhausted';
        aggregateDiagnostics.failure_reason = 'automatic capture failed';
        const failure = new Error('automatic capture failed');
        failure.captureDiagnostics = aggregateDiagnostics;
        throw failure;
      }
      Object.assign(result, selected);
    } else if (authType === 'access_token' || authType === 'admin_key' || authType === 'cookie') {
      await fillSelectedCredential(result, platform, authType);
    } else {
      throw new Error('unsupported capture auth type');
    }
    const safePayload = { ...result };
    delete safePayload.complete_url;
    await send({ ...safePayload, complete_url: payload.complete_url });
    clearStoredHandoff();
    try {
      if (window.opener && !window.opener.closed) {
        window.opener.postMessage({
          type: 'nexustok-upstream-capture-completed',
          capture_id: payload.capture_id,
          captureID: payload.capture_id,
        }, '*');
      }
    } catch (_) {}
    const returnURL = text(payload.return_url);
    if (returnURL) {
      try {
        const returnTarget = new URL(returnURL, window.location.href);
        returnTarget.searchParams.set('platform_site_capture_id', payload.capture_id);
        window.setTimeout(() => window.location.replace(returnTarget.toString()), 700);
      } catch (_) {}
    }
  }

  function markReady() {
    const readyPayload = {
      type: 'nexustok-upstream-capture-helper-ready',
      version: config.version || '__NEXUSTOK_HELPER_VERSION__',
      helper_version: config.version || '__NEXUSTOK_HELPER_VERSION__',
      capture_id: text(config.capture_id || ''),
    };
    try {
        window.dispatchEvent(new CustomEvent(readyEvent, {
          detail: readyPayload,
        }));
    } catch (_) {}
    try {
      if (window.opener && !window.opener.closed) {
        window.opener.postMessage(readyPayload, '*');
      }
    } catch (_) {}
  }

  const payload = handoff();
  markReady();
  if (!payload) {
    return;
  }
  collect(payload).catch((error) => {
    const requestedAuthType = text(payload.auth_type).toLowerCase();
    const errorMessage = text(error && error.message).toLowerCase();
    const safeMessage = requestedAuthType === 'auto'
      ? 'automatic capture failed'
      : errorMessage.includes('cookie')
        ? 'cookie capture failed'
        : errorMessage.includes('admin')
        ? 'admin key capture failed'
        : 'access token capture failed';
    const diagnostics = error && error.captureDiagnostics
      ? error.captureDiagnostics
      : Object.assign(
          diagnosticsBase('capture_helper'),
          { failure_stage: 'capture', failure_reason: safeMessage }
        );
    diagnostics.failure_stage = diagnostics.failure_stage || 'capture';
    diagnostics.failure_reason = diagnostics.failure_reason || safeMessage;
    send({
      capture_secret: payload.capture_secret,
      complete_url: payload.complete_url,
      capture_source: 'capture_helper',
      helper_version: config.version || '__NEXUSTOK_HELPER_VERSION__',
      platform: payload.platform,
      auth_type: payload.auth_type,
      origin: window.location.origin,
      diagnostics,
      error: safeMessage,
    }).catch(() => {});
  });
})();`

func renderPlatformSiteCaptureScript(
	nexusBaseURL,
	targetBaseURL,
	platform,
	authType,
	captureID string,
) string {
	match := "http://*/*\n// @match        https://*/*"
	if targetBaseURL != "" {
		if parsed, err := url.Parse(targetBaseURL); err == nil && parsed.Scheme != "" && parsed.Host != "" {
			match = parsed.Scheme + "://" + parsed.Host + "/*"
		}
	}
	connectHosts := make([]string, 0, 4)
	addConnectHost := func(raw string) {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Hostname() == "" {
			return
		}
		host := strings.ToLower(parsed.Hostname())
		for _, existing := range connectHosts {
			if existing == host {
				return
			}
		}
		connectHosts = append(connectHosts, host)
		if strings.HasPrefix(host, "api.") {
			rootHost := strings.TrimPrefix(host, "api.")
			for _, existing := range connectHosts {
				if existing == rootHost {
					return
				}
			}
			connectHosts = append(connectHosts, rootHost)
			return
		}
		apiHost := "api." + host
		for _, existing := range connectHosts {
			if existing == apiHost {
				return
			}
		}
		connectHosts = append(connectHosts, apiHost)
	}
	addConnectHost(nexusBaseURL)
	addConnectHost(targetBaseURL)
	if len(connectHosts) == 0 {
		connectHosts = append(connectHosts, "localhost")
	}
	connectHost := strings.Join(connectHosts, "\n// @connect      ")
	config, _ := common.Marshal(map[string]any{
		"version":    platformSiteCaptureHelperVersion,
		"nexus_base": nexusBaseURL,
		"platform":   platform,
		"auth_type":  authType,
		"capture_id": captureID,
	})
	script := strings.ReplaceAll(platformSiteCaptureScriptTemplate, "__NEXUSTOK_MATCH__", match)
	script = strings.ReplaceAll(script, "__NEXUSTOK_CONNECT__", connectHost)
	script = strings.ReplaceAll(script, "__NEXUSTOK_HELPER_VERSION__", platformSiteCaptureHelperVersion)
	script = strings.ReplaceAll(script, "__NEXUSTOK_CONFIG__", string(config))
	return script
}
