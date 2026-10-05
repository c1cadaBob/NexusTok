package service

import (
	"context"
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
	platformSiteCaptureHelperVersion  = "1.7.0"
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
	BridgeURL             string `json:"capture_bridge_url"`
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
	RelayBaseURLSeen             string   `json:"relay_base_url_seen,omitempty"`
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
	BridgeURL             string                          `json:"capture_bridge_url,omitempty"`
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
	bridgeURL := captureBridgeURL(nexusBaseURL, record.ID, record.InstallToken)
	handoffURL := captureHandoffURL(record, nexusBaseURL)
	return &PlatformSiteCaptureStartResult{
		CaptureID:             record.ID,
		ExpiresAt:             record.ExpiresAt,
		Platform:              record.Platform,
		BaseURL:               record.BaseURL,
		AuthType:              record.AuthType,
		Origin:                record.Origin,
		UserscriptURL:         userscriptURL,
		BridgeURL:             bridgeURL,
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
	return CompletePlatformSiteCaptureSessionContext(context.Background(), captureID, request)
}

func CompletePlatformSiteCaptureSessionContext(
	ctx context.Context,
	captureID string,
	request PlatformSiteCaptureCompleteRequest,
) (*PlatformSiteCaptureStatusResult, error) {
	record, err := getPlatformSiteCaptureRecord(captureID)
	if err != nil {
		return nil, err
	}
	if err := validatePlatformSiteCaptureCompletionIdentity(record, request); err != nil {
		return nil, err
	}
	if strings.TrimSpace(request.Error) == "" {
		for _, candidate := range []string{
			request.ManagementBaseURL,
			request.BaseURL,
			request.RelayBaseURL,
			request.APIBaseURL,
		} {
			if strings.TrimSpace(candidate) == "" {
				continue
			}
			if !sub2APIRelayURLHasNoCredentialsOrDecorations(candidate) {
				return nil, errorsForCapture("采集页面地址格式错误")
			}
		}
		if strings.TrimSpace(request.RelayBaseURL) != "" {
			if _, err := strictSub2APIRelayURL(request.RelayBaseURL); err != nil {
				return nil, errorsForCapture("Relay 地址格式错误")
			}
		}
	}
	relayURL, err := firstCaptureURL(request.RelayBaseURL)
	if err != nil {
		return nil, err
	}
	verifiedExternalRelay := false
	if strings.TrimSpace(request.Error) == "" &&
		relayURL != "" &&
		!captureRelatedPlatformURL(record.BaseURL, relayURL) {
		if record.Platform == model.PlatformSub2API {
			if !sub2APIRelayDeclaredByPage(ctx, record.BaseURL, relayURL) {
				return nil, errorsForCapture("采集页面地址不属于目标站点")
			}
			verifiedExternalRelay = true
		} else {
			return nil, errorsForCapture("采集页面地址不属于目标站点")
		}
	}

	platformSiteCaptureMu.Lock()
	defer platformSiteCaptureMu.Unlock()

	record, err = getPlatformSiteCaptureRecord(captureID)
	if err != nil {
		return nil, err
	}
	captureSource := strings.ToLower(strings.TrimSpace(request.CaptureSource))
	if err := validatePlatformSiteCaptureCompletionIdentity(record, request); err != nil {
		return nil, err
	}
	diagnostics := sanitizePlatformSiteCaptureDiagnostics(request.Diagnostics)
	if diagnostics == nil {
		diagnostics = &PlatformSiteCaptureDiagnostics{}
	}
	diagnostics.Source = captureSource
	diagnostics.HelperVersion = strings.TrimSpace(request.HelperVersion)
	diagnostics.HelperRequiredVersion = platformSiteCaptureHelperVersion
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

	resolution, summary, err := buildPlatformSiteCaptureResolution(
		record,
		request,
		verifiedExternalRelay,
	)
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

func validatePlatformSiteCaptureCompletionIdentity(
	record platformSiteCaptureRecord,
	request PlatformSiteCaptureCompleteRequest,
) error {
	if subtle.ConstantTimeCompare(
		[]byte(strings.TrimSpace(request.CaptureSecret)),
		[]byte(record.Secret),
	) != 1 {
		return errorsForCapture("采集会话密钥无效")
	}
	if record.Status == platformSiteCaptureStatusComplete {
		return errorsForCapture("采集会话已完成，请重新创建采集会话")
	}
	captureSource := strings.ToLower(strings.TrimSpace(request.CaptureSource))
	if captureSource != "" &&
		captureSource != "capture_helper" &&
		captureSource != "capture_bridge" {
		return errorsForCapture("采集来源不受支持")
	}
	if record.AuthType == PlatformSiteCaptureAuthAuto &&
		captureSource != "capture_helper" &&
		captureSource != "capture_bridge" {
		return errorsForCapture("自动配置必须通过 Capture Helper 完成采集")
	}
	if captureSource == "capture_bridge" && record.AuthType != PlatformSiteCaptureAuthAuto {
		return errorsForCapture("页面桥接只能用于自动配置")
	}
	if (record.AuthType == PlatformSiteCaptureAuthAuto ||
		captureSource == "capture_helper" ||
		captureSource == "capture_bridge") &&
		strings.TrimSpace(request.HelperVersion) != platformSiteCaptureHelperVersion {
		return errorsForCapture("采集助手版本不匹配")
	}
	if platform := strings.ToLower(strings.TrimSpace(request.Platform)); platform != "" &&
		platform != record.Platform {
		return errorsForCapture("采集平台与会话不匹配")
	}
	if authType := strings.ToLower(strings.TrimSpace(request.AuthType)); authType != "" &&
		authType != record.AuthType &&
		(record.AuthType != PlatformSiteCaptureAuthAuto ||
			(authType != PlatformSiteCaptureAuthAuto && !isPlatformSiteScriptAuthType(authType))) {
		return errorsForCapture("采集认证方式与会话不匹配")
	}
	origin := strings.TrimRight(strings.TrimSpace(request.Origin), "/")
	if origin == "" {
		origin = record.Origin
	}
	if !strings.EqualFold(origin, record.Origin) {
		return errorsForCapture("目标站来源不匹配")
	}
	return nil
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
	return renderPlatformSiteCaptureScript(
		nexusBaseURL,
		record.BaseURL,
		record.Platform,
		record.AuthType,
		record.ID,
		"userscript",
	), nil
}

func RenderPlatformSiteCaptureBridge(captureID, installToken, nexusBaseURL string) (string, error) {
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
	return renderPlatformSiteCaptureScript(
		nexusBaseURL,
		record.BaseURL,
		record.Platform,
		record.AuthType,
		record.ID,
		"bridge",
	), nil
}

func RenderPlatformSiteCaptureHelper(nexusBaseURL string) (string, error) {
	nexusBaseURL, err := normalizeCaptureNexusBaseURL(nexusBaseURL)
	if err != nil {
		return "", err
	}
	return renderPlatformSiteCaptureScript(nexusBaseURL, "", "", "", "", "userscript"), nil
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
		result.BridgeURL = captureBridgeURL(normalized, record.ID, record.InstallToken)
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
		RelayBaseURLSeen:             sanitizePlatformSiteDiagnosticURL(diagnostics.RelayBaseURLSeen),
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
	verifiedExternalRelay bool,
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
	for _, candidate := range []string{managementBaseURL, apiBaseURL} {
		if candidate == "" {
			continue
		}
		if !captureRelatedPlatformURL(record.BaseURL, candidate) {
			return PlatformSiteCaptureResolution{}, nil, errorsForCapture("采集页面地址不属于目标站点")
		}
	}
	if relayBaseURL != "" &&
		!captureRelatedPlatformURL(record.BaseURL, relayBaseURL) &&
		(record.Platform != model.PlatformSub2API || !verifiedExternalRelay) {
		return PlatformSiteCaptureResolution{}, nil, errorsForCapture("Relay 地址未由目标站公开配置声明")
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

func sub2APIRelayDeclaredByPage(
	ctx context.Context,
	managementBaseURL string,
	relayBaseURL string,
) bool {
	normalizedRelay, err := normalizePlatformSiteURL(relayBaseURL)
	if err != nil {
		return false
	}
	client, err := newPlatformSiteHTTPClient()
	if err != nil {
		return false
	}
	discovery, ok := discoverSub2APIPageConfiguration(ctx, client, managementBaseURL)
	if !ok {
		return false
	}
	for _, declared := range discovery.DeclaredRelayURLs {
		if normalized, normalizeErr := normalizePlatformSiteURL(declared); normalizeErr == nil &&
			strings.EqualFold(normalized, normalizedRelay) {
			return true
		}
	}
	return false
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

func captureBridgeURL(nexusBaseURL, captureID, installToken string) string {
	return strings.TrimRight(nexusBaseURL, "/") +
		"/api/channel/platform-site/capture-session/" +
		url.PathEscape(captureID) +
		"/bridge.js?install_token=" + url.QueryEscape(installToken)
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
// @grant        GM_addStyle
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
      relay_base_url_seen: '',
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
      'relay_base_url_seen',
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

  function directStorageValue(names) {
    for (const name of names) {
      const value = readStorage(name);
      if (value) return value;
    }
    return '';
  }

  function isNumericUserID(value) {
    return /^\d+$/.test(text(value));
  }

  function normalizeNewAPIUserID(value) {
    let normalized = text(value);
    const parsed = parseJSON(normalized);
    if (parsed !== null && typeof parsed !== 'object') normalized = text(parsed);
    normalized = normalized.replace(/^["']|["']$/g, '').trim();
    return isNumericUserID(normalized) ? normalized : '';
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
      const value = parsed !== null
        ? (typeof parsed === 'object' ? nestedValue(parsed, nestedNames, 0) : text(parsed))
        : text(raw);
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
      '__AUTH_USER__',
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
    try {
      const authUser = pageWindow.__AUTH_USER__;
      if (
        authUser &&
        typeof authUser === 'object' &&
        names.some((name) => String(name).toLowerCase() === 'auth_user')
      ) {
        return authUser;
      }
    } catch (_) {}
    return null;
  }

  function readNewAPIAccessToken() {
    return text(
      readHashValue(['access_token', 'auth_token', 'token', 'jwt']) ||
      directStorageValue([
        'new_api_access_token',
        'new-api-access-token',
        'auth_token',
        'access_token',
        'token',
        'jwt',
      ]) ||
      readNamed(
        ['auth_token', 'access_token', 'accessToken', 'token', 'jwt'],
        ['access_token', 'auth_token', 'token', 'jwt']
      ) ||
      readDeepStorageValue(['access_token', 'auth_token', 'token', 'jwt'])
    ).replace(/^Bearer\s+/i, '').trim();
  }

  function guessNewAPIUserID() {
    for (const storage of [pageWindow.localStorage, pageWindow.sessionStorage]) {
      for (const key of ['uid', 'new-api-user', 'New-Api-User']) {
        try {
          const raw = storage.getItem(key) || '';
          const direct = normalizeNewAPIUserID(raw);
          if (direct) return direct;
        } catch (_) {}
      }
      for (const key of ['user', 'user_info', 'userInfo', 'auth', 'auth_user']) {
        try {
          const raw = storage.getItem(key) || '';
          if (!raw) continue;
          const parsed = parseJSON(raw);
          const direct = normalizeNewAPIUserID(raw);
          if (direct) return direct;
          const nested = normalizeNewAPIUserID(
            parsed && typeof parsed === 'object'
              ? nestedValue(parsed, ['id', 'userid', 'user_id', 'uid'], 0)
              : ''
          );
          if (nested) return nested;
        } catch (_) {}
      }
      try {
        for (let index = 0; index < storage.length && index < 128; index += 1) {
          const key = text(storage.key(index));
          if (!/user|auth|profile|self/i.test(key)) continue;
          const parsed = parseJSON(storage.getItem(key));
          const nested = normalizeNewAPIUserID(
            parsed && typeof parsed === 'object'
              ? nestedValue(parsed, ['id', 'userid', 'user_id', 'uid'], 0)
              : ''
          );
          if (nested) return nested;
        }
      } catch (_) {}
    }
    for (const stateName of [
      '__AUTH_USER__',
      '__INITIAL_STATE__',
      '__APP_STATE__',
      '__NUXT__',
      '__NEXT_DATA__',
      '__PINIA__',
    ]) {
      try {
        const nested = normalizeNewAPIUserID(
          nestedValue(
            pageWindow[stateName],
            ['id', 'userid', 'user_id', 'uid'],
            0
          )
        );
        if (nested) return nested;
      } catch (_) {}
    }
    return normalizeNewAPIUserID(
      readDeepStorageValue(['id', 'userid', 'user_id', 'uid']) ||
      userIDFromToken(readNewAPIAccessToken())
    );
  }

  function newAPIHeaders(userID, accessToken) {
    const headers = {};
    const token = text(accessToken).replace(/^Bearer\s+/i, '').trim();
    if (token) headers.Authorization = 'Bearer ' + token;
    const normalizedUserID = normalizeNewAPIUserID(userID);
    if (normalizedUserID) headers['New-Api-User'] = normalizedUserID;
    return headers;
  }

  function readVisibleCookie(name) {
    const expectedName = text(name);
    if (!expectedName) return '';
    try {
      for (const part of text(document.cookie).split(';')) {
        const separator = part.indexOf('=');
        if (separator <= 0 || text(part.slice(0, separator)) !== expectedName) {
          continue;
        }
        const value = text(part.slice(separator + 1));
        return value.length <= 256 ? value : '';
      }
    } catch (_) {}
    return '';
  }

  function readNamedCookie(name, targetURL) {
    const expectedName = text(name);
    const visibleValue = readVisibleCookie(expectedName);
    if (visibleValue) return Promise.resolve(visibleValue);
    if (typeof GM_cookie !== 'object' || typeof GM_cookie.list !== 'function') {
      return Promise.resolve('');
    }
    let cookieURL = window.location.origin;
    try {
      const parsed = new URL(text(targetURL || ''), window.location.href);
      if (
        parsed.protocol === window.location.protocol &&
        (
          parsed.host === window.location.host ||
          parsed.host === 'api.' + window.location.host ||
          window.location.host === 'api.' + parsed.host
        )
      ) {
        cookieURL = parsed.origin;
      }
    } catch (_) {}
    return new Promise((resolve) => {
      try {
        GM_cookie.list({ url: cookieURL }, (cookies, error) => {
          if (error || !Array.isArray(cookies)) {
            resolve('');
            return;
          }
          const cookie = cookies.find(
            (item) => item && text(item.name) === expectedName
          );
          const value = cookie ? text(cookie.value) : '';
          resolve(value.length <= 256 ? value : '');
        });
      } catch (_) {
        resolve('');
      }
    });
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
    const mergeCookieHeaders = (headers) => {
      const values = new Map();
      for (const header of headers) {
        for (const part of text(header).split(';')) {
          const separator = part.indexOf('=');
          if (separator <= 0) continue;
          const name = text(part.slice(0, separator));
          if (!name) continue;
          values.set(name, name + '=' + text(part.slice(separator + 1)));
        }
      }
      return Array.from(values.values()).join('; ');
    };
    if (typeof GM_cookie !== 'object' || typeof GM_cookie.list !== 'function') {
      return Promise.resolve(mergeCookieHeaders([visibleCookies]));
    }
    return new Promise((resolve) => {
      try {
        GM_cookie.list({ url: cookieURL }, (cookies, error) => {
          if (error || !Array.isArray(cookies)) {
            resolve(mergeCookieHeaders([visibleCookies]));
            return;
          }
          const values = cookies
            .filter((cookie) => cookie && text(cookie.name))
            .map((cookie) => text(cookie.name) + '=' + text(cookie.value));
          resolve(mergeCookieHeaders([visibleCookies, values.join('; ')]));
        });
      } catch (_) {
        resolve(mergeCookieHeaders([visibleCookies]));
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
    const tokenNames = [
      'access_token',
      'accessToken',
      'auth_token',
      'authToken',
      'token',
      'jwt',
    ];
    if (typeof payload === 'string') {
      return text(payload).replace(/^Bearer\s+/i, '');
    }
    const direct = nestedValue(payload, tokenNames, 0);
    if (direct) return direct.replace(/^Bearer\s+/i, '');
    if (payload && typeof payload === 'object') {
      const data = payload.data;
      if (typeof data === 'string') {
        return text(data).replace(/^Bearer\s+/i, '');
      }
      if (data && typeof data === 'object') {
        const nested = nestedValue(data, tokenNames, 0);
        if (nested) return nested.replace(/^Bearer\s+/i, '');
      }
    }
    return '';
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

  function handoffFromURL(rawURL) {
    try {
      const current = new URL(text(rawURL), window.location.href);
      const rawHash = text(current.hash || '').replace(/^#/, '');
      const hashParams = new URLSearchParams(rawHash);
      const encoded =
        current.searchParams.get('nexustok_capture') ||
        hashParams.get('nexustok_capture');
      return encoded ? decodeHandoff(encoded) : null;
    } catch (_) {
      return null;
    }
  }

  function handoff() {
    const current = new URL(window.location.href);
    const handoffParam = 'nexustok_capture';
    const rawHash = text(current.hash || '').replace(/^#/, '');
    const hashParams = new URLSearchParams(rawHash);
    const encoded = current.searchParams.get(handoffParam) || hashParams.get(handoffParam);
    if (encoded) {
      const value = decodeHandoff(encoded);
      current.searchParams.delete(handoffParam);
      if (hashParams.has(handoffParam)) {
        hashParams.delete(handoffParam);
        current.hash = hashParams.toString() ? '#' + hashParams.toString() : '';
      }
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

  function bridgeTargetOrigin() {
    try {
      return new URL(text(config.nexus_base), window.location.href).origin;
    } catch (_) {
      return '';
    }
  }

  function requestBridgeHandoff() {
    if (
      config.transport !== 'bridge' ||
      !window.opener ||
      window.opener.closed
    ) {
      return Promise.resolve(null);
    }
    const targetOrigin = bridgeTargetOrigin();
    if (!targetOrigin) return Promise.resolve(null);
    return new Promise((resolve) => {
      let settled = false;
      let timeoutID = 0;
      const cleanup = () => {
        window.removeEventListener('message', handleMessage);
        if (timeoutID) window.clearTimeout(timeoutID);
      };
      const finish = (value) => {
        if (settled) return;
        settled = true;
        cleanup();
        resolve(value);
      };
      const handleMessage = (event) => {
        const data = event.data || {};
        if (
          event.source !== window.opener ||
          event.origin !== targetOrigin ||
          data.type !== 'nexustok-upstream-capture-bridge-handoff' ||
          text(data.capture_id || data.captureID) !== text(config.capture_id)
        ) {
          return;
        }
        const value =
          data.handoff && typeof data.handoff === 'object'
            ? data.handoff
            : handoffFromURL(data.handoff_url || data.handoffURL);
        finish(value);
      };
      window.addEventListener('message', handleMessage);
      timeoutID = window.setTimeout(() => finish(null), 3000);
      try {
        window.opener.postMessage(
          {
            type: 'nexustok-upstream-capture-bridge-request',
            capture_id: config.capture_id,
            captureID: config.capture_id,
          },
          targetOrigin
        );
      } catch (_) {
        finish(null);
      }
    });
  }

  function clearStoredHandoff() {
    try {
      pageWindow.sessionStorage.removeItem('nexustok_platform_site_capture_handoff');
    } catch (_) {}
  }

  function permanentCaptureError(message, status) {
    const error = new Error(text(message));
    error.capturePermanent = true;
    if (status) error.status = status;
    return error;
  }

  function isPermanentCaptureError(error) {
    return Boolean(error && error.capturePermanent);
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

  function safeConfiguredEndpoint(raw) {
    try {
      const parsed = new URL(text(raw), window.location.href);
      if (
        (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') ||
        parsed.username ||
        parsed.password ||
        parsed.port &&
          (!/^\d+$/.test(parsed.port) ||
            Number(parsed.port) < 1 ||
            Number(parsed.port) > 65535)
      ) {
        return '';
      }
      const hostname = text(parsed.hostname).toLowerCase();
      if (
        !hostname ||
        hostname === 'localhost' ||
        hostname === 'localhost.localdomain' ||
        hostname === 'metadata.google.internal' ||
        hostname === 'instance-data.ec2.internal' ||
        /^(127\.|10\.|192\.168\.|169\.254\.)/.test(hostname) ||
        /^172\.(1[6-9]|2\d|3[01])\./.test(hostname) ||
        hostname === '::1' ||
        hostname.startsWith('fc') ||
        hostname.startsWith('fd') ||
        hostname.startsWith('fe80:')
      ) {
        return '';
      }
      parsed.search = '';
      parsed.hash = '';
      parsed.pathname = parsed.pathname.replace(/\/+$/, '');
      return parsed.origin + (parsed.pathname === '/' ? '' : parsed.pathname);
    } catch (_) {
      return '';
    }
  }

  function configuredEndpointValues(value) {
    return configuredEndpointValuesAtDepth(value, 0);
  }

  function configuredEndpointValuesAtDepth(value, depth) {
    if (!value || depth > 4) return [];
    if (typeof value === 'string') {
      const parsed = parseJSON(value);
      return parsed && parsed !== value
        ? configuredEndpointValuesAtDepth(parsed, depth + 1)
        : [value];
    }
    if (Array.isArray(value)) {
      return value.flatMap((item) => configuredEndpointValuesAtDepth(item, depth + 1));
    }
    if (typeof value !== 'object') return [];
    const values = [];
    const endpoint = value.endpoint || value.url || value.base_url || value.baseUrl;
    if (endpoint) values.push(endpoint);
    for (const key of [
      'data',
      'config',
      'public_config',
      'publicConfig',
      'settings',
      'value',
    ]) {
      if (!Object.prototype.hasOwnProperty.call(value, key)) continue;
      values.push(...configuredEndpointValuesAtDepth(value[key], depth + 1));
    }
    return values;
  }

  function configuredRelayBaseURL(payload) {
    const values = [];
    const appendConfig = (configuration, depth) => {
      if (!configuration || depth > 4) return;
      if (typeof configuration === 'string') {
        const parsed = parseJSON(configuration);
        if (parsed !== null && parsed !== configuration) {
          appendConfig(parsed, depth + 1);
        }
        return;
      }
      if (Array.isArray(configuration) || typeof configuration !== 'object') return;
      values.push(
        ...configuredEndpointValues(configuration.custom_endpoints),
        ...configuredEndpointValues(configuration.customEndpoints)
      );
      for (const key of [
        'data',
        'config',
        'public_config',
        'publicConfig',
        'settings',
        'value',
      ]) {
        if (Object.prototype.hasOwnProperty.call(configuration, key)) {
          appendConfig(configuration[key], depth + 1);
        }
      }
    };
    appendConfig(payload, 0);
    try { appendConfig(pageWindow.__APP_CONFIG__, 0); } catch (_) {}
    for (const stateName of [
      '__INITIAL_STATE__',
      '__APP_STATE__',
      '__NUXT__',
      '__NEXT_DATA__',
      '__PINIA__',
    ]) {
      try { appendConfig(pageWindow[stateName], 0); } catch (_) {}
    }
    for (const storageName of [
      'custom_endpoints',
      'customEndpoints',
      'app_config',
      'appConfig',
      'public_settings',
      'publicSettings',
      'settings',
    ]) {
      for (const storage of [pageWindow.localStorage, pageWindow.sessionStorage]) {
        try {
          const raw = storage.getItem(storageName);
          if (!raw) continue;
          appendConfig(raw, 0);
        } catch (_) {}
      }
    }
    for (const value of values) {
      const normalized = safeConfiguredEndpoint(value);
      if (normalized) return normalized;
    }
    return '';
  }

  function apiBasePathPrefix(pathname) {
    const normalized = cleanAPIPathPrefix(pathname);
    for (const suffix of ['/api/v1', '/api']) {
      if (normalized === suffix) return '';
      if (normalized.endsWith(suffix)) {
        return normalized.slice(0, -suffix.length) || '';
      }
    }
    return normalized;
  }

  function cleanAPIPathPrefix(pathname) {
    const pageSegments = new Set([
      'login',
      'register',
      'dashboard',
      'console',
      'playground',
      'token',
      'tokens',
      'channel',
      'channels',
      'setting',
      'settings',
      'models',
      'pricing',
      'wallet',
      'topup',
      'logs',
      'about',
      'home',
      'panel',
      'admin',
      'sign-in',
      'sign-up',
    ]);
    const parts = text(pathname).split('/').filter(Boolean);
    while (
      parts.length > 0 &&
      pageSegments.has(parts[parts.length - 1].toLowerCase())
    ) {
      parts.pop();
    }
    if (parts.length === 0) return '';
    return '/' + parts.join('/');
  }

  function candidateAPIPrefixes(payload) {
    const prefixes = new Set(['']);
    const candidates = [
      payload && payload.base_url,
      payload && payload.api_base_url,
      pageWindow.location && pageWindow.location.href,
      pageWindow.__APP_CONFIG__ && pageWindow.__APP_CONFIG__.api_base_url,
      pageWindow.__APP_CONFIG__ && pageWindow.__APP_CONFIG__.apiBaseUrl,
      readStorage('api_base_url'),
    ];
    for (const raw of candidates) {
      try {
        const parsed = new URL(text(raw), window.location.href);
        const prefix = apiBasePathPrefix(parsed.pathname);
        if (!prefix || prefix === '/api' || prefix.includes('/api/')) continue;
        prefixes.add(prefix);
        const firstSegment = '/' + prefix.split('/').filter(Boolean)[0];
        if (firstSegment && firstSegment !== '/') prefixes.add(firstSegment);
      } catch (_) {}
    }
    return Array.from(prefixes);
  }

  function joinAPIPath(prefix, path) {
    if (/^https?:\/\//i.test(path)) return path;
    const left = text(prefix).replace(/\/+$/, '');
    const right = text(path).replace(/^\/+/, '');
    return (left ? left : '') + '/' + right;
  }

  function discoveredAPIPathPattern(kind) {
    switch (kind) {
      case 'self':
        return /["'](\/[^"']*api\/user\/self[^"']*)["']/g;
      case 'token':
        return /["'](\/[^"']*api\/user\/(?:token|access_token|access-token)[^"']*)["']/g;
      case 'newapi_refresh':
        return /["'](\/[^"']*api\/user\/auth\/refresh[^"']*)["']/g;
      case 'sub2_me':
        return /["'](\/[^"']*(?:api\/v1\/)?auth\/me[^"']*)["']/g;
      case 'sub2_session_restore':
        return /["'](\/[^"']*session\/restore[^"']*)["']/g;
      case 'sub2_refresh':
        return /["'](\/[^"']*(?:api\/v1\/)?auth\/refresh[^"']*)["']/g;
      default:
        return null;
    }
  }

  const discoveredAPIPathsPromises = {};

  async function discoverAPIPaths(kind) {
    if (!discoveredAPIPathsPromises[kind]) {
      discoveredAPIPathsPromises[kind] = (async () => {
        const sources = new Set();
        try {
          for (const script of Array.from(document.scripts || [])) {
            if (script.src) sources.add(script.src);
          }
        } catch (_) {}
        try {
          const entries = pageWindow.performance &&
            typeof pageWindow.performance.getEntriesByType === 'function'
            ? pageWindow.performance.getEntriesByType('resource')
            : [];
          for (const entry of entries) {
            if (entry && entry.name) sources.add(entry.name);
          }
        } catch (_) {}
        const sameOriginScripts = Array.from(sources)
          .map((source) => {
            try {
              return new URL(source, pageWindow.location.href);
            } catch (_) {
              return null;
            }
          })
          .filter(
            (source) =>
              source &&
              source.origin === pageWindow.location.origin &&
              /\.js(?:$|\?)/i.test(source.href)
          )
          .slice(0, 16);
        const found = new Set();
        const pattern = discoveredAPIPathPattern(kind);
        if (!pattern) return [];
        for (const scriptURL of sameOriginScripts) {
          const controller = typeof AbortController === 'function'
            ? new AbortController()
            : null;
          const timeoutID = window.setTimeout(
            () => controller && controller.abort(),
            2000
          );
          try {
            const response = await fetch(scriptURL.href, {
              credentials: 'omit',
              cache: 'force-cache',
              ...(controller ? { signal: controller.signal } : {}),
            });
            if (!response.ok) continue;
            const body = await response.text();
            pattern.lastIndex = 0;
            let match;
            while ((match = pattern.exec(body)) !== null) {
              const path = text(match[1])
                .replace(/\\u0026/g, '&')
                .split('?')[0];
              if (path.startsWith('/')) found.add(path);
            }
          } catch (_) {}
          finally {
            window.clearTimeout(timeoutID);
          }
        }
        return Array.from(found).slice(0, 16);
      })();
    }
    return discoveredAPIPathsPromises[kind];
  }

  async function expandedAPIPaths(paths, kind, payload) {
    const prefixes = candidateAPIPrefixes(payload || {});
    const result = new Set();
    for (const path of paths) {
      result.add(path);
      for (const prefix of prefixes) {
        if (prefix) result.add(joinAPIPath(prefix, path));
      }
    }
    let discovered = [];
    try {
      discovered = await Promise.race([
        discoverAPIPaths(kind),
        new Promise((resolve) => {
          window.setTimeout(() => resolve([]), 2500);
        }),
      ]);
    } catch (_) {}
    for (const path of discovered || []) {
      result.add(path);
      for (const prefix of prefixes) {
        if (prefix) result.add(joinAPIPath(prefix, path));
      }
    }
    return Array.from(result);
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
      const pageOrigin = text(pageWindow.location && pageWindow.location.origin);
      if (
        pageOrigin &&
        !Object.prototype.hasOwnProperty.call(headers, 'Origin') &&
        !Object.prototype.hasOwnProperty.call(headers, 'origin')
      ) {
        headers.Origin = pageOrigin;
      }
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
        const payload = await gmJSONRequest(url, requestOptions);
        return payload;
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

  function readIndexedDBValue(dbName, storeName, key) {
    if (typeof indexedDB === 'undefined') return Promise.resolve('');
    return new Promise((resolve) => {
      let settled = false;
      const valueFromRecord = (value, depth) => {
        if (value == null || depth > 3) return '';
        if (typeof value === 'string') {
          const parsed = parseJSON(value);
          if (parsed !== null && typeof parsed === 'object') {
            return valueFromRecord(parsed, depth + 1);
          }
          return text(value);
        }
        if (typeof value !== 'object') return '';
        for (const field of [
          'value',
          'client_id',
          'clientId',
          'auth_client_id',
          'authClientId',
          'authClientID',
          'data',
        ]) {
          if (!Object.prototype.hasOwnProperty.call(value, field)) continue;
          const found = valueFromRecord(value[field], depth + 1);
          if (found) return found;
        }
        return '';
      };
      const finish = (value) => {
        if (settled) return;
        settled = true;
        const normalized = valueFromRecord(value, 0).trim();
        resolve(normalized && normalized.length <= 128 ? normalized : '');
      };
      let request;
      try {
        request = indexedDB.open(dbName);
      } catch (_) {
        finish('');
        return;
      }
      request.onerror = () => finish('');
      request.onblocked = () => finish('');
      request.onsuccess = () => {
        const db = request.result;
        try {
          if (!db.objectStoreNames.contains(storeName)) {
            db.close();
            finish('');
            return;
          }
          const transaction = db.transaction(storeName, 'readonly');
          const getRequest = transaction.objectStore(storeName).get(key);
          getRequest.onsuccess = () => finish(getRequest.result);
          getRequest.onerror = () => finish('');
          transaction.oncomplete = () => db.close();
          transaction.onerror = () => {
            db.close();
            finish('');
          };
          transaction.onabort = () => {
            db.close();
            finish('');
          };
        } catch (_) {
          try { db.close(); } catch (__) {}
          finish('');
        }
      };
    });
  }

  async function restoreSub2APIBrowserSession(result) {
    const apiBase = result.api_base_url;
    const diagnostics = diagnosticsBase('browser_session_restore');
    let clientID = readNamed(
      ['sub2api_auth_client_id'],
      ['sub2api_auth_client_id']
    );
    if (!clientID) {
      clientID = await readNamedCookie('sub2api_auth_client_id', apiBase);
    }
    if (!clientID) {
      clientID = await readIndexedDBValue(
        'sub2api-auth-coordination',
        'values',
        'sub2api_auth_client_id'
      );
    }
    diagnostics.auth_client_id_present = Boolean(clientID);
    if (!clientID) {
      diagnostics.browser_session_restore_status = 'not_attempted';
      diagnostics.browser_session_restore_message = '未发现浏览器会话恢复 Client ID';
      return { diagnostics };
    }
    let discoveredPaths = [];
    try {
      discoveredPaths = await discoverAPIPaths('sub2_session_restore');
    } catch (_) {}
    if (!Array.isArray(discoveredPaths) || discoveredPaths.length === 0) {
      diagnostics.browser_session_restore_status = 'not_attempted';
      diagnostics.browser_session_restore_message = '未从同源 JavaScript 发现浏览器会话恢复接口';
      return { diagnostics };
    }
    const candidatePaths = await expandedAPIPaths(
      discoveredPaths,
      'sub2_session_restore',
      result
    );
    for (const path of candidatePaths) {
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

  async function readNewAPIAuthBundle(result) {
    const apiBase = result.api_base_url;
    const cookie = await readCookieHeader(apiBase);
    const userID = guessNewAPIUserID();
    const candidatePaths = await expandedAPIPaths(
      ['/api/user/auth/refresh'],
      'newapi_refresh',
      result
    );
    for (const path of candidatePaths) {
      try {
        const payload = await jsonRequest(
          new URL(path, apiBase).toString(),
          {
            method: 'POST',
            headers: {
              Accept: 'application/json',
              ...newAPIHeaders(userID, ''),
            },
            cookie,
          }
        );
        const data = payload && payload.data;
        const session = data && data.session;
        const authUser = data && data.user;
        const hasUserIdentity = Boolean(
          authUser &&
          typeof authUser === 'object' &&
          (
            nestedValue(authUser, ['id', 'user_id', 'userid', 'uid'], 0) ||
            nestedValue(authUser, ['username', 'user_name', 'login', 'email', 'mail'], 0)
          )
        );
        if (
          !payload ||
          payload.success !== true ||
          !data ||
          data.token_type !== 'Bearer' ||
          !session ||
          session.current !== true ||
          !hasUserIdentity
        ) {
          continue;
        }
        const accessToken = text(data.access_token).replace(/^Bearer\s+/i, '');
        const expiresAt = normalizeExpiresAt(data.access_expires_at);
        if (!accessToken || !session.sid || !expiresAt || expiresAt <= Math.floor(Date.now() / 1000)) {
          continue;
        }
        return {
          accessToken,
          refreshToken: '',
          expiresAt,
          authUser,
          sessionID: text(session.sid),
          cookie,
          path,
        };
      } catch (_) {}
    }
    return null;
  }

  async function send(payload) {
    if (config.transport === 'bridge') {
      if (!window.opener || window.opener.closed) {
        throw new Error('bridge opener is unavailable');
      }
      const returnURL = text(payload.return_url || '');
      let targetOrigin = '';
      if (returnURL) {
        try {
          targetOrigin = new URL(returnURL, window.location.href).origin;
        } catch (_) {}
      }
      if (!targetOrigin) {
        targetOrigin = text(config.nexus_base);
        try {
          targetOrigin = new URL(targetOrigin, window.location.href).origin;
        } catch (_) {
          targetOrigin = '';
        }
      }
      if (!targetOrigin) throw new Error('bridge target origin is unavailable');
      const messagePayload = { ...payload };
      delete messagePayload.complete_url;
      return await new Promise((resolve, reject) => {
        let settled = false;
        const cleanup = () => {
          window.removeEventListener('message', handleMessage);
          if (timeoutID) window.clearTimeout(timeoutID);
        };
        const finish = (callback, value) => {
          if (settled) return;
          settled = true;
          cleanup();
          callback(value);
        };
        const handleMessage = (event) => {
          const data = event.data || {};
          if (event.source !== window.opener || event.origin !== targetOrigin) return;
          if (data.type !== 'nexustok-upstream-capture-bridge-ack') return;
          if (text(data.capture_id || data.captureID) !== text(payload.capture_id)) return;
          if (data.success) {
            finish(resolve, data);
          } else {
            finish(reject, new Error('capture bridge callback was rejected'));
          }
        };
        const timeoutID = window.setTimeout(() => {
          finish(reject, new Error('capture bridge callback timed out'));
        }, 30000);
        window.addEventListener('message', handleMessage);
        try {
          window.opener.postMessage({
            type: 'nexustok-upstream-capture-bridge-result',
            capture_id: payload.capture_id,
            captureID: payload.capture_id,
            helper_version: payload.helper_version,
            payload: {
              ...messagePayload,
              capture_source: 'capture_bridge',
            },
          }, targetOrigin);
        } catch (_) {
          finish(reject, new Error('capture bridge callback failed'));
        }
      });
    }
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
            if (
              response.status >= 200 &&
              response.status < 300 &&
              responseSucceeded(result)
            ) {
              resolve(result);
            }
            else reject(permanentCaptureError('HTTP ' + response.status, response.status));
          },
          onerror: () => reject(permanentCaptureError('capture callback failed')),
          ontimeout: () => reject(permanentCaptureError('capture callback timed out')),
        });
      });
    }
    try {
      return await jsonRequest(payload.complete_url, {
        method: 'POST',
        credentials: 'omit',
        headers: { 'Content-Type': 'application/json' },
        body,
      });
    } catch (error) {
      throw permanentCaptureError(
        error && error.message ? error.message : 'capture callback failed',
        error && error.status ? error.status : 0
      );
    }
  }

  async function verifyCurrentUser(result, platform, accessToken, cookie, sessionID) {
    const diagnostics = result.diagnostics || diagnosticsBase('verification');
    const strategy = platformStrategies[platform] || platformStrategies.newapi;
    const userID = platform === 'newapi'
      ? normalizeNewAPIUserID(result.user_id || guessNewAPIUserID())
      : '';
    const headers = platform === 'newapi'
      ? newAPIHeaders(userID, accessToken)
      : {};
    if (platform !== 'newapi' && text(accessToken)) {
      headers.Authorization = 'Bearer ' + text(accessToken).replace(/^Bearer\s+/i, '').trim();
    }
    if (sessionID) headers['X-Auth-Session'] = sessionID;
    let validatedUser = null;
    const candidatePaths = await expandedAPIPaths(
      strategy.mePaths,
      platform === 'sub2api' ? 'sub2_me' : 'self',
      result
    );
    for (const mePath of candidatePaths) {
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
    const id = nestedValue(validatedUser, ['id', 'user_id', 'userid', 'uid', 'sub'], 0);
    if (/^\d+$/.test(text(id))) result.user_id = text(id);
    if (!result.user_id && platform === 'newapi' && accessToken) {
      result.user_id = userIDFromToken(accessToken);
    }
    if (!result.user_id && platform === 'newapi') {
      result.user_id = userID;
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
    if (!result.cookie && platform !== 'newapi') {
      throw new Error('cookie is not readable; HttpOnly cookie cannot be captured');
    }
    await verifyCurrentUser(result, platform, '', result.cookie, '');
    if (platform === 'newapi' && result.user_id) {
      const tokenPaths = await expandedAPIPaths(
        ['/api/user/token', '/api/user/access_token', '/api/user/access-token'],
        'token',
        result
      );
      for (const path of tokenPaths) {
        try {
          markAttempt(result.diagnostics, path);
          const tokenResult = await jsonRequest(
            new URL(path, result.api_base_url).toString(),
            {
              headers: newAPIHeaders(result.user_id, ''),
              cookie: result.cookie,
            }
          );
          const accessToken = tokenFromResponse(tokenResult);
          if (accessToken) {
            result.access_token = accessToken.replace(/^Bearer\s+/i, '').trim();
            result.auth_type = 'access_token';
            result.diagnostics.access_token_present = true;
            return;
          }
        } catch (_) {}
      }
    }
    if (!result.cookie) {
      throw new Error('cookie is not readable and no NewAPI access token could be issued');
    }
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
      const bundle = await readNewAPIAuthBundle(result);
      if (!bundle) throw new Error('NewAPI Dashboard refresh session is unavailable');
      accessToken = bundle.accessToken;
      refreshToken = bundle.refreshToken;
      expiresAt = bundle.expiresAt;
      sessionID = bundle.sessionID || '';
      cookie = bundle.cookie || '';
      storedAuthUser = bundle.authUser;
      markAttempt(result.diagnostics, bundle.path || '/api/user/auth/refresh');
    } else if (source === 'browser_restore') {
      const restored = await restoreSub2APIBrowserSession(result);
      Object.assign(result.diagnostics, restored.diagnostics || {});
      if (!restored.accessToken) throw new Error('Sub2API browser session restore did not return an access token');
      accessToken = restored.accessToken;
      refreshToken = restored.refreshToken || '';
      expiresAt = restored.expiresAt || 0;
      storedAuthUser = restored.authUser;
    } else {
      const hashAccessToken = readHashValue(
        ['auth_token', 'access_token', 'token', 'jwt']
      );
      const hashRefreshToken = readHashValue(
        ['refresh_token', 'refreshToken', 'rt']
      );
      const hashExpiresAt = readHashValue(
        ['token_expires_at', 'tokenExpiresAt', 'expires_at', 'expiresAt']
      );
      if (hashAccessToken || hashRefreshToken || hashExpiresAt) {
        result.diagnostics.oauth_hash_token_present = true;
      }
      if (source !== 'refresh_token') {
        accessToken = hashAccessToken || readNamed(
          ['auth_token', 'access_token', 'accessToken', 'token', 'jwt'],
          ['auth_token', 'access_token', 'accesstoken', 'token', 'jwt']
        ) || readDeepStorageValue(
          ['access_token', 'auth_token', 'token', 'jwt']
        );
      }
      refreshToken = hashRefreshToken || readNamed(
        ['refresh_token', 'refreshToken', 'sub2api_refresh_token'],
        ['refresh_token', 'refreshtoken', 'rt']
      ) || readDeepStorageValue(
        ['refresh_token', 'refreshtoken', 'rt']
      );
      expiresAt = normalizeExpiresAt(hashExpiresAt || readNamed(
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
      if (
        platform === 'sub2api' &&
        refreshToken &&
        (source === 'refresh_token' || !accessToken)
      ) {
        const refreshPaths = await expandedAPIPaths(
          ['/api/v1/auth/refresh', '/api/auth/refresh', '/auth/refresh'],
          'sub2_refresh',
          result
        );
        let refreshedAccessToken = '';
        let refreshedRefreshToken = '';
        let refreshedExpiresAt = 0;
        let refreshError = null;
        let refreshPath = '';
        for (const path of refreshPaths) {
          try {
            markAttempt(result.diagnostics, path);
            const refreshed = await jsonRequest(new URL(path, apiBase).toString(), {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify({ refresh_token: refreshToken }),
            });
            const candidateAccessToken = tokenFromResponse(refreshed);
            if (!candidateAccessToken) {
              refreshError = new Error('Sub2API refresh response did not return an access token');
              continue;
            }
            refreshedAccessToken = candidateAccessToken;
            refreshedRefreshToken = refreshTokenFromResponse(refreshed) || refreshToken;
            refreshedExpiresAt = expiryFromResponse(refreshed) || expiresAt;
            refreshPath = path;
            break;
          } catch (error) {
            refreshError = error;
          }
        }
        if (!refreshedAccessToken) {
          throw refreshError || new Error('Sub2API refresh endpoint unavailable');
        }
        markAttempt(result.diagnostics, refreshPath);
        accessToken = refreshedAccessToken;
        refreshToken = refreshedRefreshToken;
        expiresAt = refreshedExpiresAt;
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
    if (platform === 'newapi' && !result.user_id) {
      result.user_id = guessNewAPIUserID();
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

  const panelId = 'nexustok-upstream-capture-helper-panel';
  const buttonId = 'nexustok-upstream-capture-helper-button';
  let runtimeConfig = null;
  let captureStarted = false;
  let captureCompleted = false;
  let captureStopped = false;
  let retryTimer = 0;
  let styleMounted = false;

  function mountStatusStyle() {
    if (styleMounted || typeof GM_addStyle !== 'function') return;
    styleMounted = true;
    GM_addStyle(
      '#' + panelId + '{position:fixed;right:16px;bottom:64px;z-index:2147483647;max-width:360px;border-radius:8px;background:#fff;color:#111827;padding:10px 12px;font:12px system-ui;box-shadow:0 8px 24px rgba(0,0,0,.18);line-height:1.45}' +
      '#' + panelId + '[data-tone=success]{border-left:4px solid #16a34a}' +
      '#' + panelId + '[data-tone=error]{border-left:4px solid #dc2626}' +
      '#' + panelId + '[data-tone=info]{border-left:4px solid #2563eb}' +
      '#' + buttonId + '{position:fixed;right:16px;bottom:16px;z-index:2147483647;border:0;border-radius:8px;background:#111827;color:#fff;padding:10px 12px;font:13px system-ui;box-shadow:0 8px 24px rgba(0,0,0,.24);cursor:pointer}' +
      '#' + buttonId + ':disabled{opacity:.65;cursor:default}'
    );
  }

  function showStatus(message, tone, buttonLabel) {
    if (!document.body) return;
    mountStatusStyle();
    let panel = document.getElementById(panelId);
    if (!panel) {
      panel = document.createElement('div');
      panel.id = panelId;
      document.body.appendChild(panel);
    }
    panel.textContent = text(message);
    panel.dataset.tone = tone || 'info';
    let button = document.getElementById(buttonId);
    if (!button) {
      button = document.createElement('button');
      button.id = buttonId;
      button.type = 'button';
      button.addEventListener('click', () => runCapture(true));
      document.body.appendChild(button);
    }
    button.textContent = buttonLabel || '重新采集';
    button.disabled = captureStarted || captureCompleted || captureStopped;
  }

  function capturePayloadValue(payload, snakeName, camelName) {
    return payload && (payload[snakeName] || payload[camelName]);
  }

  function handoffExpired(payload) {
    const expiresAt = Number.parseInt(
      text(capturePayloadValue(payload, 'expires_at', 'expiresAt')),
      10
    );
    return Number.isFinite(expiresAt) &&
      expiresAt > 0 &&
      Math.floor(Date.now() / 1000) >= expiresAt;
  }

  function scheduleRetry() {
    if (
      captureCompleted ||
      captureStopped ||
      !runtimeConfig ||
      handoffExpired(runtimeConfig)
    ) return;
    if (retryTimer) window.clearTimeout(retryTimer);
    showStatus('正在等待上游登录。完成登录后会自动继续，也可以点击“重新采集”。', 'info');
    retryTimer = window.setTimeout(() => {
      captureStarted = false;
      void runCapture(false);
    }, 3000);
  }

  function safeCaptureFailureMessage(error) {
    const message = text(error && error.message).toLowerCase();
    if (message.includes('expired') || message.includes('过期')) {
      return '采集会话已过期，请回到 NexusTok 创建新的采集会话。';
    }
    return '暂未发现已验证的上游登录态，请完成登录后等待自动重试。';
  }

  function stopCapture(message) {
    captureStarted = false;
    captureStopped = true;
    runtimeConfig = null;
    if (retryTimer) {
      window.clearTimeout(retryTimer);
      retryTimer = 0;
    }
    clearStoredHandoff();
    showStatus(message, 'error', '请重新创建采集会话');
  }

  function permanentCaptureFailureMessage(error) {
    const message = text(error && error.message).toLowerCase();
    if (message.includes('expired') || message.includes('过期')) {
      return '采集会话已过期，请回到 NexusTok 重新创建采集会话。';
    }
    if (message.includes('callback') || message.includes('http ')) {
      return '采集回传服务不可用，请回到 NexusTok 重新创建采集会话。';
    }
    return '采集会话不可用，请回到 NexusTok 重新创建采集会话。';
  }

  async function runCapture(manual) {
    if (!runtimeConfig || captureStarted || captureCompleted || captureStopped) return;
    if (handoffExpired(runtimeConfig)) {
      stopCapture('采集会话已过期，请回到 NexusTok 重新创建采集会话。');
      return;
    }
    captureStarted = true;
    showStatus('正在验证并采集上游登录态...', 'info');
    try {
      await collect(runtimeConfig);
      captureCompleted = true;
      if (retryTimer) window.clearTimeout(retryTimer);
      showStatus('采集完成，正在返回 NexusTok...', 'success');
    } catch (error) {
      captureStarted = false;
      if (isPermanentCaptureError(error)) {
        stopCapture(permanentCaptureFailureMessage(error));
        return;
      }
      showStatus(
        manual ? safeCaptureFailureMessage(error) : '正在等待上游登录...',
        'info'
      );
      scheduleRetry();
    }
  }

  async function collect(payload) {
    if (!payload || !payload.capture_secret || payload.origin !== window.location.origin) {
      throw permanentCaptureError('capture handoff is invalid');
    }
    const platform = text(payload.platform).toLowerCase();
    const authType = text(payload.auth_type).toLowerCase();
    const result = {
      capture_id: text(payload.capture_id),
      capture_secret: payload.capture_secret,
      complete_url: payload.complete_url,
      capture_source: config.transport === 'bridge'
        ? 'capture_bridge'
        : 'capture_helper',
      helper_version: config.version || '__NEXUSTOK_HELPER_VERSION__',
      platform,
      auth_type: authType,
      base_url: text(payload.management_base_url || payload.base_url) || window.location.origin,
      management_base_url: text(payload.management_base_url || payload.base_url) || window.location.origin,
      origin: window.location.origin,
      api_base_url: apiBaseURL(payload),
      relay_base_url: '',
      diagnostics: diagnosticsBase(
        config.transport === 'bridge' ? 'capture_bridge' : 'capture_helper'
      ),
    };
    result.diagnostics.api_base_url_seen = result.api_base_url;
    result.relay_base_url = configuredRelayBaseURL(payload);
    result.diagnostics.relay_base_url_seen = result.relay_base_url;
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
    delete safePayload.auth_user;
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

  function markReady(payload) {
    const readyPayload = {
      type: 'nexustok-upstream-capture-helper-ready',
      version: config.version || '__NEXUSTOK_HELPER_VERSION__',
      helper_version: config.version || '__NEXUSTOK_HELPER_VERSION__',
      capture_id: text(
        capturePayloadValue(payload, 'capture_id', 'captureID') ||
        config.capture_id ||
        ''
      ),
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

  async function boot() {
    let payload = handoff();
    if (!payload && config.transport === 'bridge') {
      payload = await requestBridgeHandoff();
    }
    markReady(payload);
    if (!payload) {
      if (config.transport === 'bridge') {
        showStatus(
          window.opener && !window.opener.closed
            ? '未收到 NexusTok 采集参数，请回到管理页重新运行页面桥接。'
            : '页面桥接需要从 NexusTok 管理页打开的上游页面，请回到管理页重新创建采集会话。',
          'error',
          '请重新打开采集页面'
        );
      }
      return;
    }
    runtimeConfig = {
      ...payload,
      capture_secret: capturePayloadValue(payload, 'capture_secret', 'captureSecret'),
      complete_url: capturePayloadValue(payload, 'complete_url', 'completeURL'),
      capture_id: capturePayloadValue(payload, 'capture_id', 'captureID'),
      expires_at: capturePayloadValue(payload, 'expires_at', 'expiresAt'),
      helper_version: capturePayloadValue(payload, 'helper_version', 'helperVersion'),
    };
    const normalizedPayload = runtimeConfig;
    const expectedHelperVersion = text(
      normalizedPayload.helper_version
    );
    if (expectedHelperVersion && expectedHelperVersion !== config.version) {
      stopCapture('当前采集助手版本过旧，请回到 NexusTok 安装或更新 NexusTok Capture Helper。');
      return;
    }
    const expectedOrigin = text(normalizedPayload.origin);
    if (expectedOrigin && pageWindow.location.origin !== expectedOrigin) {
      stopCapture('当前页面不是采集会话指定的上游站点，请重新创建采集会话。');
      return;
    }
    if (handoffExpired(normalizedPayload)) {
      stopCapture('采集会话已过期，请回到 NexusTok 重新创建采集会话。');
      return;
    }
    if (
      !normalizedPayload.capture_secret ||
      !normalizedPayload.complete_url ||
      !normalizedPayload.capture_id
    ) {
      stopCapture('采集参数无效，请回到 NexusTok 重新创建采集会话。');
      return;
    }
    showStatus('NexusTok 采集助手已就绪，正在等待上游登录。', 'info');
    window.setTimeout(() => void runCapture(false), 800);
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', boot, { once: true });
    markReady();
  } else {
    void boot();
  }
})();`

func renderPlatformSiteCaptureScript(
	nexusBaseURL,
	targetBaseURL,
	platform,
	authType,
	captureID,
	transport string,
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
		"transport":  transport,
	})
	script := strings.ReplaceAll(platformSiteCaptureScriptTemplate, "__NEXUSTOK_MATCH__", match)
	script = strings.ReplaceAll(script, "__NEXUSTOK_CONNECT__", connectHost)
	script = strings.ReplaceAll(script, "__NEXUSTOK_HELPER_VERSION__", platformSiteCaptureHelperVersion)
	script = strings.ReplaceAll(script, "__NEXUSTOK_CONFIG__", string(config))
	return script
}
