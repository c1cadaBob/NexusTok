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
	platformSiteCaptureHelperVersion  = "1.2.0"
	platformSiteCaptureHandoffParam   = "nexustok_capture"

	PlatformSiteCaptureAuthAuto = "auto"
)

type PlatformSiteCaptureStartRequest struct {
	Platform  string `json:"platform"`
	BaseURL   string `json:"base_url"`
	AuthType  string `json:"auth_type"`
	ChannelID int    `json:"channel_id,omitempty"`
}

type PlatformSiteCaptureStartResult struct {
	CaptureID        string `json:"capture_id"`
	ExpiresAt        int64  `json:"expires_at"`
	Platform         string `json:"platform"`
	BaseURL          string `json:"base_url"`
	AuthType         string `json:"auth_type"`
	Origin           string `json:"origin"`
	UserscriptURL    string `json:"userscript_url"`
	HelperInstallURL string `json:"helper_install_url"`
	HandoffURL       string `json:"handoff_url"`
	LoginURL         string `json:"login_url"`
}

type PlatformSiteCaptureCompleteRequest struct {
	CaptureSecret     string         `json:"capture_secret"`
	CaptureSource     string         `json:"capture_source,omitempty"`
	HelperVersion     string         `json:"helper_version,omitempty"`
	Platform          string         `json:"platform,omitempty"`
	AuthType          string         `json:"auth_type,omitempty"`
	BaseURL           string         `json:"base_url,omitempty"`
	ManagementBaseURL string         `json:"management_base_url,omitempty"`
	RelayBaseURL      string         `json:"relay_base_url,omitempty"`
	APIBaseURL        string         `json:"api_base_url,omitempty"`
	Origin            string         `json:"origin,omitempty"`
	AccessToken       string         `json:"access_token,omitempty"`
	RefreshToken      string         `json:"refresh_token,omitempty"`
	AdminKey          string         `json:"admin_key,omitempty"`
	Cookie            string         `json:"cookie,omitempty"`
	UserID            string         `json:"user_id,omitempty"`
	Username          string         `json:"username,omitempty"`
	Email             string         `json:"email,omitempty"`
	TokenExpiresAt    int64          `json:"token_expires_at,omitempty"`
	ExpiresIn         int64          `json:"expires_in,omitempty"`
	AuthUser          map[string]any `json:"auth_user,omitempty"`
	Error             string         `json:"error,omitempty"`
}

type PlatformSiteCaptureSummary struct {
	Platform            string `json:"platform"`
	AuthType            string `json:"auth_type"`
	BaseURL             string `json:"base_url"`
	ManagementBaseURL   string `json:"management_base_url,omitempty"`
	RelayBaseURL        string `json:"relay_base_url,omitempty"`
	APIBaseURL          string `json:"api_base_url,omitempty"`
	Origin              string `json:"origin"`
	AccessTokenMasked   string `json:"access_token_masked,omitempty"`
	RefreshTokenPresent bool   `json:"refresh_token_present,omitempty"`
	AdminKeyPresent     bool   `json:"admin_key_present,omitempty"`
	CookiePresent       bool   `json:"cookie_present,omitempty"`
	UserID              string `json:"user_id,omitempty"`
	Username            string `json:"username,omitempty"`
	Email               string `json:"email,omitempty"`
	TokenExpiresAt      int64  `json:"token_expires_at,omitempty"`
	CapturedAt          int64  `json:"captured_at,omitempty"`
}

type PlatformSiteCaptureStatusResult struct {
	CaptureID        string                      `json:"capture_id"`
	Status           string                      `json:"status"`
	Message          string                      `json:"message,omitempty"`
	ExpiresAt        int64                       `json:"expires_at"`
	Platform         string                      `json:"platform"`
	BaseURL          string                      `json:"base_url"`
	AuthType         string                      `json:"auth_type"`
	Origin           string                      `json:"origin"`
	UserscriptURL    string                      `json:"userscript_url,omitempty"`
	HelperInstallURL string                      `json:"helper_install_url,omitempty"`
	HandoffURL       string                      `json:"handoff_url,omitempty"`
	LoginURL         string                      `json:"login_url,omitempty"`
	Summary          *PlatformSiteCaptureSummary `json:"summary,omitempty"`
}

type PlatformSiteCaptureResolution struct {
	Platform          string
	Credential        model.PlatformSiteCredential
	ManagementBaseURL string
	RelayBaseURL      string
	APIBaseURL        string
}

type platformSiteCaptureRecord struct {
	ID                string                       `json:"id"`
	Secret            string                       `json:"secret"`
	InstallToken      string                       `json:"install_token"`
	UserID            int                          `json:"user_id"`
	ChannelID         int                          `json:"channel_id,omitempty"`
	Platform          string                       `json:"platform"`
	AuthType          string                       `json:"auth_type"`
	BaseURL           string                       `json:"base_url"`
	Origin            string                       `json:"origin"`
	ExpiresAt         int64                        `json:"expires_at"`
	Status            string                       `json:"status"`
	Error             string                       `json:"error,omitempty"`
	UpdatedAt         int64                        `json:"updated_at"`
	Credential        model.PlatformSiteCredential `json:"credential"`
	ManagementBaseURL string                       `json:"management_base_url,omitempty"`
	RelayBaseURL      string                       `json:"relay_base_url,omitempty"`
	APIBaseURL        string                       `json:"api_base_url,omitempty"`
	Summary           *PlatformSiteCaptureSummary  `json:"summary,omitempty"`
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
		return nil, errorsForCapture("仅支持自动配置、账号密码缓存登录态、Access Token、Admin Key 和 Cookie 采集")
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
		CaptureID:        record.ID,
		ExpiresAt:        record.ExpiresAt,
		Platform:         record.Platform,
		BaseURL:          record.BaseURL,
		AuthType:         record.AuthType,
		Origin:           record.Origin,
		UserscriptURL:    userscriptURL,
		HelperInstallURL: strings.TrimRight(nexusBaseURL, "/") + "/api/channel/platform-site/capture-helper.user.js",
		HandoffURL:       handoffURL,
		LoginURL:         record.BaseURL,
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
	if captureSource == "capture_helper" &&
		strings.TrimSpace(request.HelperVersion) != platformSiteCaptureHelperVersion {
		return nil, errorsForCapture("采集助手版本不匹配")
	}
	if platform := strings.ToLower(strings.TrimSpace(request.Platform)); platform != "" && platform != record.Platform {
		return nil, errorsForCapture("采集平台与会话不匹配")
	}
	if authType := strings.ToLower(strings.TrimSpace(request.AuthType)); authType != "" && authType != record.AuthType {
		if record.AuthType == model.UpstreamAuthPassword && authType == model.UpstreamAuthAccessToken {
			// 密码模式只允许浏览器采集结果提供缓存登录态，不切换认证方式。
		} else if record.AuthType != PlatformSiteCaptureAuthAuto ||
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
		record.UpdatedAt = common.GetTimestamp()
		_ = savePlatformSiteCaptureRecord(record)
		return sanitizePlatformSiteCaptureRecord(record, ""), err
	}
	record.Status = platformSiteCaptureStatusComplete
	record.Error = ""
	record.Credential = resolution.Credential
	record.ManagementBaseURL = resolution.ManagementBaseURL
	record.RelayBaseURL = resolution.RelayBaseURL
	record.APIBaseURL = resolution.APIBaseURL
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
	if normalized := strings.ToLower(strings.TrimSpace(authType)); normalized != "" && normalized != record.AuthType {
		if normalized != PlatformSiteCaptureAuthAuto &&
			!(normalized == model.UpstreamAuthPassword && record.AuthType == model.UpstreamAuthPassword) &&
			!(record.AuthType == PlatformSiteCaptureAuthAuto && normalized == record.Credential.AuthType) {
			return PlatformSiteCaptureResolution{}, errorsForCapture("采集认证方式与渠道不匹配")
		}
	}
	return PlatformSiteCaptureResolution{
		Platform:          record.Platform,
		Credential:        record.Credential,
		ManagementBaseURL: record.ManagementBaseURL,
		RelayBaseURL:      record.RelayBaseURL,
		APIBaseURL:        record.APIBaseURL,
	}, nil
}

func ConsumePlatformSiteCapture(userID int, captureID string, channelID int) error {
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
	_, err = platformSiteCaptureCache.DeleteMany([]string{captureID})
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

func sanitizePlatformSiteCaptureRecord(
	record platformSiteCaptureRecord,
	nexusBaseURL string,
) *PlatformSiteCaptureStatusResult {
	result := &PlatformSiteCaptureStatusResult{
		CaptureID: record.ID,
		Status:    record.Status,
		ExpiresAt: record.ExpiresAt,
		Platform:  record.Platform,
		BaseURL:   record.BaseURL,
		AuthType:  record.AuthType,
		Origin:    record.Origin,
		Message:   record.Error,
		Summary:   record.Summary,
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
	if authType == model.UpstreamAuthPassword {
		authType = model.UpstreamAuthAccessToken
	} else if authType == PlatformSiteCaptureAuthAuto {
		authType = selectPlatformSiteCaptureAuthType(request)
		if authType == "" {
			return PlatformSiteCaptureResolution{}, nil, errorsForCapture("自动配置未采集到可用登录态")
		}
	} else if !isPlatformSiteScriptAuthType(authType) {
		return PlatformSiteCaptureResolution{}, nil, errorsForCapture("采集认证方式不受支持")
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
	return authType == PlatformSiteCaptureAuthAuto ||
		authType == model.UpstreamAuthPassword ||
		isPlatformSiteScriptAuthType(authType)
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
// @description  Capture an explicitly selected NewAPI or Sub2API login state for NexusTok.
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
  const readyEvent = 'nexustok-platform-site-capture-ready';
  const text = (value) => value == null ? '' : String(value).trim();

  function parseJSON(value) {
    try { return value ? JSON.parse(value) : null; } catch (_) { return null; }
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

  function readCookieHeader() {
    try {
      const visibleCookies = text(document.cookie);
      if (visibleCookies) return Promise.resolve(visibleCookies);
    } catch (_) {}
    if (typeof GM_cookie !== 'object' || typeof GM_cookie.list !== 'function') {
      return Promise.resolve('');
    }
    return new Promise((resolve) => {
      try {
        GM_cookie.list({ url: window.location.origin }, (cookies, error) => {
          if (error || !Array.isArray(cookies)) {
            resolve('');
            return;
          }
          const values = cookies
            .filter((cookie) => cookie && text(cookie.name))
            .map((cookie) => text(cookie.name) + '=' + text(cookie.value));
          resolve(values.join('; '));
        });
      } catch (_) {
        resolve('');
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

  async function jsonRequest(url, options) {
    const response = await fetch(url, {
      credentials: 'include',
      headers: { Accept: 'application/json', ...(options && options.headers || {}) },
      ...(options || {}),
    });
    const body = await response.text();
    const parsed = parseJSON(body);
    if (!response.ok) throw new Error('HTTP ' + response.status);
    return parsed || {};
  }

  async function restoreSub2APIBrowserSession(apiBase) {
    let clientID = readNamed(
      ['sub2api_auth_client_id'],
      ['sub2api_auth_client_id']
    );
    const paths = [
      '/api/v1/auth/session/restore',
      '/api/auth/session/restore',
      '/auth/session/restore',
    ];
    for (const path of paths) {
      try {
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
        return {
          accessToken,
          refreshToken: refreshTokenFromResponse(restored),
          expiresAt: expiryFromResponse(restored),
          authUser: restored && (restored.data || restored.user || restored),
        };
      } catch (_) {}
    }
    return null;
  }

  async function readNewAPIAuthBundle(apiBase) {
    try {
      const response = await fetch(new URL('/api/user/auth/refresh', apiBase).toString(), {
        method: 'POST',
        credentials: 'include',
        headers: { Accept: 'application/json' },
      });
      if (response.status === 404 || response.status === 405 || !response.ok) {
        return null;
      }
      const payload = parseJSON(await response.text());
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

  async function fillCookieCredential(result) {
    result.cookie = await readCookieHeader();
    if (!result.cookie) throw new Error('cookie is not readable; HttpOnly cookie cannot be captured');
  }

  function fillAdminKeyCredential(result) {
    result.admin_key = readNamed(
      ['admin_key', 'adminKey', 'admin_token', 'adminToken', 'x-api-key'],
      ['admin_key', 'adminkey', 'admin_token', 'admintoken', 'x-api-key']
    );
    if (!result.admin_key) throw new Error('admin key was not found in an explicitly named field');
  }

  async function fillAccessTokenCredential(result, platform) {
    const apiBase = result.api_base_url;
    let accessToken = readHashValue(
      ['auth_token', 'access_token', 'token', 'jwt']
    ) || readNamed(
      ['auth_token', 'access_token', 'accessToken', 'token', 'jwt'],
      ['auth_token', 'access_token', 'accesstoken', 'token', 'jwt']
    ) || readDeepStorageValue(
      ['access_token', 'auth_token', 'token', 'jwt']
    );
    let refreshToken = readHashValue(
      ['refresh_token', 'refreshToken', 'rt']
    ) || readNamed(
      ['refresh_token', 'refreshToken'],
      ['refresh_token', 'refreshtoken']
    ) || readDeepStorageValue(
      ['refresh_token', 'refreshtoken', 'rt']
    );
    let expiresAt = normalizeExpiresAt(readHashValue(
      ['token_expires_at', 'tokenExpiresAt', 'expires_at', 'expiresAt']
    ) || readNamed(
      ['token_expires_at', 'tokenExpiresAt', 'expires_at', 'expiresAt'],
      ['token_expires_at', 'tokenexpiresat', 'expires_at', 'expiresat']
    ) || readDeepStorageValue(
      ['token_expires_at', 'tokenexpiresat', 'expires_at', 'expiresat']
    ));
    const storedAuthUser = readStructured([
      'auth_user',
      'authUser',
      'current_user',
      'currentUser',
      'user',
    ]);
    const storedUserID = nestedValue(
      storedAuthUser,
      ['id', 'user_id', 'userid', 'uid', 'sub'],
      0
    );
    if (/^\d+$/.test(text(storedUserID))) {
      result.user_id = text(storedUserID);
    }
    if (!accessToken && platform === 'newapi') {
      const bundle = await readNewAPIAuthBundle(apiBase);
      if (bundle) {
        accessToken = bundle.accessToken;
        refreshToken = bundle.refreshToken;
        expiresAt = bundle.expiresAt;
        result.auth_user = bundle.authUser;
      }
    }
    if (!accessToken && platform === 'sub2api') {
      const restored = await restoreSub2APIBrowserSession(apiBase);
      if (restored) {
        accessToken = restored.accessToken;
        refreshToken = restored.refreshToken;
        expiresAt = restored.expiresAt;
        result.auth_user = restored.authUser;
      }
    }
    if (platform === 'sub2api' && refreshToken && expiresAt > 0 && expiresAt < Math.floor(Date.now() / 1000) + 300) {
      try {
        const refreshed = await jsonRequest(new URL('/api/v1/auth/refresh', apiBase).toString(), {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ refresh_token: refreshToken }),
        });
        accessToken = tokenFromResponse(refreshed) || accessToken;
        refreshToken = refreshTokenFromResponse(refreshed) || refreshToken;
        expiresAt = expiryFromResponse(refreshed) || expiresAt;
      } catch (_) {}
    }
    if (!accessToken) throw new Error('access token was not found in an explicitly named field');
    const authHeaders = { Authorization: 'Bearer ' + accessToken };
    let validatedUser = null;
    const mePaths = platform === 'sub2api'
      ? ['/api/v1/auth/me', '/api/auth/me']
      : ['/api/user/self', '/api/user/me', '/api/user/profile', '/api/user/info'];
    for (const mePath of mePaths) {
      try {
        const me = await jsonRequest(new URL(mePath, apiBase).toString(), { headers: authHeaders });
        validatedUser = me && (me.data || me.user || me);
        const id = nestedValue(validatedUser, ['id', 'user_id', 'userid', 'uid', 'sub'], 0);
        if (/^\d+$/.test(text(id))) result.user_id = text(id);
        break;
      } catch (_) {}
    }
    if (!validatedUser) throw new Error('access token validation failed');
    result.auth_user = validatedUser;
    if (!result.user_id && platform === 'newapi') {
      result.user_id = userIDFromToken(accessToken);
    }
    result.access_token = accessToken;
    result.refresh_token = refreshToken;
    if (expiresAt > 0) result.token_expires_at = expiresAt;
    result.username = text(nestedValue(
      result.auth_user,
      ['username', 'user_name', 'display_name', 'name'],
      0
    ));
    result.email = text(nestedValue(result.auth_user, ['email', 'mail'], 0));
    delete result.auth_user;
  }

  async function fillSelectedCredential(result, platform, authType) {
    if (authType === 'access_token') {
      await fillAccessTokenCredential(result, platform);
      return;
    }
    if (authType === 'admin_key') {
      fillAdminKeyCredential(result);
      return;
    }
    if (authType === 'cookie') {
      await fillCookieCredential(result);
      return;
    }
    throw new Error('unsupported capture auth type');
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
      helper_version: config.version || '1.2.0',
      platform,
      auth_type: authType,
      base_url: window.location.origin,
      origin: window.location.origin,
      api_base_url: apiBaseURL(payload),
    };
    if (authType === 'auto') {
      let selected = null;
      for (const candidateType of ['access_token', 'admin_key', 'cookie']) {
        const candidate = { ...result, auth_type: candidateType };
        try {
          await fillSelectedCredential(candidate, platform, candidateType);
          selected = candidate;
          break;
        } catch (_) {}
      }
      if (!selected) throw new Error('automatic capture failed');
      Object.assign(result, selected);
    } else if (authType === 'password') {
      result.auth_type = 'access_token';
      await fillSelectedCredential(result, platform, 'access_token');
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
        window.opener.postMessage({ type: 'nexustok-platform-site-capture-completed' }, '*');
      }
    } catch (_) {}
  }

  function markReady() {
    try {
        window.dispatchEvent(new CustomEvent(readyEvent, { detail: { version: config.version || '1.2.0' } }));
    } catch (_) {}
  }

  const payload = handoff();
  if (!payload) {
    markReady();
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
    send({
      capture_secret: payload.capture_secret,
      complete_url: payload.complete_url,
      capture_source: 'capture_helper',
      helper_version: config.version || '1.2.0',
      platform: payload.platform,
      auth_type: payload.auth_type,
      origin: window.location.origin,
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
	connectHost := "*"
	if parsed, err := url.Parse(nexusBaseURL); err == nil && parsed.Hostname() != "" {
		connectHost = parsed.Hostname()
	}
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
