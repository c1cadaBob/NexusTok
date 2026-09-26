package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
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
	platformSiteAuthFlowTTL                              = 5 * time.Minute
	platformSiteAuthFlowMaxAttempts                      = 5
	platformSiteAuthFlowMaxCodeAttempts                  = 5
	PlatformSiteAuthFlowStatusAuthenticated              = model.PlatformSiteAuthStatusAuthenticated
	PlatformSiteAuthFlowStatusTwoFactorRequired          = model.PlatformSiteAuthStatusTwoFactorRequired
	PlatformSiteAuthFlowStatusSecureVerificationRequired = model.PlatformSiteAuthStatusSecureVerificationRequired
	PlatformSiteAuthFlowStatusCredentialsInvalid         = model.PlatformSiteAuthStatusCredentialsInvalid
	PlatformSiteAuthFlowStatusExpired                    = model.PlatformSiteAuthStatusExpired
	PlatformSiteAuthFlowStatusReauthRequired             = model.PlatformSiteAuthStatusReauthRequired
	PlatformSiteAuthFlowStatusSessionLimit               = model.PlatformSiteAuthStatusSessionLimit
	PlatformSiteAuthFlowStatusRateLimited                = model.PlatformSiteAuthStatusRateLimited
)

var (
	ErrPlatformSiteAuthFlowInvalid     = errors.New("平台站点认证流程无效")
	ErrPlatformSiteAuthFlowExpired     = errors.New("平台站点认证流程已过期")
	ErrPlatformSiteAuthFlowConsumed    = errors.New("平台站点认证流程已消费")
	ErrPlatformSiteAuthFlowRateLimited = errors.New("平台站点认证流程尝试次数过多")
	ErrPlatformSiteAuthFlowCodeInvalid = errors.New("平台站点二次验证码错误")
	ErrPlatformSiteAuthFlowStatus      = errors.New("平台站点认证状态需要人工处理")
)

type PlatformSiteAuthFlowStartRequest struct {
	Platform  string `json:"platform"`
	BaseURL   string `json:"base_url"`
	AuthType  string `json:"auth_type"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	ChannelID int    `json:"channel_id,omitempty"`
}

type PlatformSiteAuthFlowVerifyRequest struct {
	Code string `json:"code"`
}

type PlatformSiteAuthFlowResult struct {
	FlowID         string   `json:"flow_id"`
	Status         string   `json:"status"`
	ExpiresAt      int64    `json:"expires_at"`
	Platform       string   `json:"platform"`
	BaseURL        string   `json:"base_url"`
	ChannelID      int      `json:"channel_id,omitempty"`
	Methods        []string `json:"methods,omitempty"`
	Attempts       int      `json:"attempts,omitempty"`
	CodeAttempts   int      `json:"code_attempts,omitempty"`
	AuthType       string   `json:"auth_type,omitempty"`
	TokenType      string   `json:"token_type,omitempty"`
	TokenExpiresAt int64    `json:"token_expires_at,omitempty"`
	UserID         string   `json:"user_id,omitempty"`
	Username       string   `json:"username,omitempty"`
}

type PlatformSiteAuthFlowResolution struct {
	Platform   string
	BaseURL    string
	ChannelID  int
	Credential model.PlatformSiteCredential
}

type platformSiteAuthFlowRecord struct {
	FlowID       string   `json:"flow_id"`
	UserID       int      `json:"user_id"`
	ChannelID    int      `json:"channel_id,omitempty"`
	Platform     string   `json:"platform"`
	BaseURL      string   `json:"base_url"`
	Origin       string   `json:"origin"`
	ExpiresAt    int64    `json:"expires_at"`
	Status       string   `json:"status"`
	Attempts     int      `json:"attempts"`
	CodeAttempts int      `json:"code_attempts"`
	Consumed     bool     `json:"consumed"`
	AuthType     string   `json:"auth_type"`
	Methods      []string `json:"methods,omitempty"`
	Secret       string   `json:"secret"`
}

type platformSiteAuthFlowSecret struct {
	Credential model.PlatformSiteCredential `json:"credential"`
	FlowToken  string                       `json:"flow_token,omitempty"`
	TempToken  string                       `json:"temp_token,omitempty"`
}

var (
	platformSiteAuthFlowCache = cachex.NewHybridCache[platformSiteAuthFlowRecord](
		cachex.HybridCacheConfig[platformSiteAuthFlowRecord]{
			Namespace:  cachex.Namespace("platform-site-auth-flow"),
			Redis:      common.RDB,
			RedisCodec: cachex.JSONCodec[platformSiteAuthFlowRecord]{},
			RedisEnabled: func() bool {
				return common.RedisEnabled && common.RDB != nil
			},
			Memory: func() *hot.HotCache[string, platformSiteAuthFlowRecord] {
				return hot.NewHotCache[string, platformSiteAuthFlowRecord](hot.LRU, 512).
					WithTTL(platformSiteAuthFlowTTL).
					WithJanitor().
					Build()
			},
		},
	)
	platformSiteAuthFlowMu sync.Mutex
)

func StartPlatformSiteAuthFlow(ctx context.Context, userID int, request PlatformSiteAuthFlowStartRequest) (*PlatformSiteAuthFlowResult, error) {
	if userID <= 0 {
		return nil, ErrPlatformSiteAuthFlowInvalid
	}
	platform := strings.ToLower(strings.TrimSpace(request.Platform))
	if platform != model.PlatformNewAPI && platform != model.PlatformSub2API {
		return nil, fmt.Errorf("%w: 平台类型不受支持", ErrPlatformSiteAuthFlowInvalid)
	}
	if strings.ToLower(strings.TrimSpace(request.AuthType)) != model.UpstreamAuthPassword {
		return nil, fmt.Errorf("%w: 认证流程仅支持账号密码登录", ErrPlatformSiteAuthFlowInvalid)
	}
	baseURL, err := normalizePlatformSiteURL(request.BaseURL)
	if err != nil {
		return nil, err
	}
	if err := validatePlatformSiteURL(baseURL); err != nil {
		return nil, err
	}
	credential := model.PlatformSiteCredential{
		AuthType: model.UpstreamAuthPassword,
		Username: strings.TrimSpace(request.Username),
		Password: request.Password,
	}
	if credential.Username == "" || credential.Password == "" {
		return nil, fmt.Errorf("%w: 账号密码不能为空", ErrPlatformSiteAuth)
	}
	session, payload, err := authenticatePlatformSitePassword(ctx, platform, baseURL, credential)
	if err != nil {
		return nil, classifyPlatformSiteAuthFlowError(err)
	}
	status := PlatformSiteAuthFlowStatusAuthenticated
	methods := make([]string, 0, 2)
	secret := platformSiteAuthFlowSecret{Credential: credential}
	if platform == model.PlatformNewAPI {
		secret.FlowToken = firstString(firstRecord(payload), "flow_token", "verification_token")
		if secret.FlowToken == "" {
			secret.FlowToken = findFlowToken(payload)
		}
		if loginRequiresInteractiveVerification(payload) {
			status = PlatformSiteAuthFlowStatusTwoFactorRequired
			methods = newAPIAuthMethods(payload)
		}
	} else {
		secret.TempToken = findTempToken(payload)
		if loginRequiresInteractiveVerification(payload) || secret.TempToken != "" {
			status = PlatformSiteAuthFlowStatusTwoFactorRequired
			methods = []string{"totp"}
		}
	}
	capturePlatformSiteSessionCookie(session, &secret.Credential)
	if status == PlatformSiteAuthFlowStatusAuthenticated {
		if err := applyPlatformSiteLoginPayload(session, platform, &credential, payload); err != nil {
			return nil, classifyPlatformSiteAuthFlowError(err)
		}
		secret.Credential = credential
	}
	if status == PlatformSiteAuthFlowStatusTwoFactorRequired {
		if len(methods) == 0 {
			methods = []string{"totp"}
		}
	}
	record, err := savePlatformSiteAuthFlow(userID, request.ChannelID, platform, baseURL, status, methods, secret)
	if err != nil {
		return nil, err
	}
	return platformSiteAuthFlowResult(record), nil
}

func VerifyPlatformSiteAuthFlow(ctx context.Context, userID int, flowID string, request PlatformSiteAuthFlowVerifyRequest) (*PlatformSiteAuthFlowResult, error) {
	platformSiteAuthFlowMu.Lock()
	defer platformSiteAuthFlowMu.Unlock()
	record, secret, err := getPlatformSiteAuthFlow(flowID, userID)
	if err != nil {
		return nil, err
	}
	if record.Status != PlatformSiteAuthFlowStatusTwoFactorRequired {
		return nil, fmt.Errorf("%w: 当前流程无需二次验证", ErrPlatformSiteAuthFlowInvalid)
	}
	if record.CodeAttempts >= platformSiteAuthFlowMaxCodeAttempts {
		record.Status = PlatformSiteAuthFlowStatusRateLimited
		_ = savePlatformSiteAuthFlowRecord(record)
		return nil, ErrPlatformSiteAuthFlowRateLimited
	}
	code := strings.TrimSpace(request.Code)
	if len(code) != 6 {
		record.Attempts++
		record.CodeAttempts++
		if record.Attempts >= platformSiteAuthFlowMaxAttempts {
			record.Status = PlatformSiteAuthFlowStatusRateLimited
		}
		_ = savePlatformSiteAuthFlowRecord(record)
		return nil, ErrPlatformSiteAuthFlowCodeInvalid
	}
	session, err := newPlatformSiteSession(record.BaseURL, nil)
	if err != nil {
		return nil, err
	}
	var payload any
	if record.Platform == model.PlatformSub2API {
		setSub2APIBrowserHeaders(session)
		payload, err = verifySub2APILogin2FA(ctx, session, secret.TempToken, code)
	} else {
		payload, err = verifyNewAPILogin2FA(ctx, session, secret.FlowToken, code)
	}
	if err != nil {
		record.Attempts++
		record.CodeAttempts++
		if record.CodeAttempts >= platformSiteAuthFlowMaxCodeAttempts ||
			record.Attempts >= platformSiteAuthFlowMaxAttempts {
			record.Status = PlatformSiteAuthFlowStatusRateLimited
		}
		_ = savePlatformSiteAuthFlowRecord(record)
		return nil, ErrPlatformSiteAuthFlowCodeInvalid
	}
	credential := secret.Credential
	if err := applyPlatformSiteLoginPayload(session, record.Platform, &credential, payload); err != nil {
		record.Status = classifyAuthStatus(err)
		_ = savePlatformSiteAuthFlowRecord(record)
		return nil, err
	}
	secret.Credential = credential
	record.Status = PlatformSiteAuthFlowStatusAuthenticated
	record.AuthType = model.InferPlatformSiteAuthType(credential)
	record.Methods = nil
	record.Secret, err = encryptPlatformSiteAuthFlowSecret(secret)
	if err != nil {
		return nil, err
	}
	if err := savePlatformSiteAuthFlowRecord(record); err != nil {
		return nil, err
	}
	return platformSiteAuthFlowResult(record), nil
}

func ResolvePlatformSiteAuthFlow(userID int, flowID string, channelID int, platform, baseURL string) (PlatformSiteAuthFlowResolution, error) {
	platformSiteAuthFlowMu.Lock()
	defer platformSiteAuthFlowMu.Unlock()
	record, secret, err := getPlatformSiteAuthFlow(flowID, userID)
	if err != nil {
		return PlatformSiteAuthFlowResolution{}, err
	}
	if record.Status != PlatformSiteAuthFlowStatusAuthenticated || record.Consumed {
		return PlatformSiteAuthFlowResolution{}, ErrPlatformSiteAuthFlowInvalid
	}
	if record.ChannelID != 0 && channelID != 0 && record.ChannelID != channelID {
		record.Attempts++
		_ = savePlatformSiteAuthFlowRecord(record)
		return PlatformSiteAuthFlowResolution{}, ErrPlatformSiteAuthFlowInvalid
	}
	if platform != "" && !strings.EqualFold(platform, record.Platform) {
		record.Attempts++
		_ = savePlatformSiteAuthFlowRecord(record)
		return PlatformSiteAuthFlowResolution{}, ErrPlatformSiteAuthFlowInvalid
	}
	normalizedBaseURL, err := normalizePlatformSiteURL(baseURL)
	if err != nil || normalizedBaseURL != record.BaseURL {
		record.Attempts++
		_ = savePlatformSiteAuthFlowRecord(record)
		return PlatformSiteAuthFlowResolution{}, ErrPlatformSiteAuthFlowInvalid
	}
	origin, err := platformSiteOrigin(normalizedBaseURL)
	if err != nil || origin != record.Origin {
		record.Attempts++
		_ = savePlatformSiteAuthFlowRecord(record)
		return PlatformSiteAuthFlowResolution{}, ErrPlatformSiteAuthFlowInvalid
	}
	return PlatformSiteAuthFlowResolution{
		Platform:   record.Platform,
		BaseURL:    record.BaseURL,
		ChannelID:  record.ChannelID,
		Credential: secret.Credential,
	}, nil
}

func ConsumePlatformSiteAuthFlow(userID int, flowID string, channelID int) error {
	platformSiteAuthFlowMu.Lock()
	defer platformSiteAuthFlowMu.Unlock()
	record, err := getPlatformSiteAuthFlowRecord(flowID)
	if err != nil {
		return err
	}
	if record.UserID != userID ||
		(record.ChannelID != 0 && channelID != 0 && record.ChannelID != channelID) ||
		record.Status != PlatformSiteAuthFlowStatusAuthenticated {
		record.Attempts++
		_ = savePlatformSiteAuthFlowRecord(record)
		return ErrPlatformSiteAuthFlowInvalid
	}
	record.Consumed = true
	if err := savePlatformSiteAuthFlowRecord(record); err != nil {
		return err
	}
	_, err = platformSiteAuthFlowCache.DeleteMany([]string{flowID})
	return err
}

func DeletePlatformSiteAuthFlow(userID int, flowID string) error {
	platformSiteAuthFlowMu.Lock()
	defer platformSiteAuthFlowMu.Unlock()
	record, err := getPlatformSiteAuthFlowRecord(flowID)
	if err != nil {
		return err
	}
	if record.UserID != userID {
		return ErrPlatformSiteAuthFlowInvalid
	}
	_, err = platformSiteAuthFlowCache.DeleteMany([]string{flowID})
	return err
}

func authenticatePlatformSitePassword(ctx context.Context, platform, baseURL string, credential model.PlatformSiteCredential) (*PlatformSiteSession, any, error) {
	if platform != model.PlatformNewAPI && platform != model.PlatformSub2API {
		return nil, nil, ErrUnsupportedPlatformSite
	}
	session, err := newPlatformSiteSession(baseURL, make(http.Header))
	if err != nil {
		return nil, nil, err
	}
	if platform == model.PlatformSub2API {
		setSub2APIBrowserHeaders(session)
		payload, err := loginSub2APIWithPassword(ctx, session, credential)
		return session, payload, err
	}
	payload, err := loginNewAPIWithPassword(ctx, session, credential)
	return session, payload, err
}

func applyPlatformSiteLoginPayload(session *PlatformSiteSession, platform string, credential *model.PlatformSiteCredential, payload any) error {
	if session == nil || credential == nil {
		return ErrPlatformSiteAuthFlowInvalid
	}
	token := findToken(payload)
	if token == "" && platform != model.PlatformNewAPI {
		return ErrSub2APILoginToken
	}
	if token != "" {
		credential.AuthType = model.UpstreamAuthAccessToken
		credential.Username = ""
		credential.Password = ""
		credential.AccessToken = token
		credential.TokenType = firstNonEmptyString(firstString(firstRecord(payload), "token_type", "tokenType"), "Bearer")
		session.Headers.Set("Authorization", bearerToken(token))
	}
	if refresh := findRefreshToken(payload); refresh != "" {
		credential.RefreshToken = refresh
	}
	credential.TokenExpiresAt = findTokenExpiresAt(payload)
	credential.UserID = findUserID(payload)
	credential.SessionID = findSessionID(payload)
	credential.SessionCurrent = credential.SessionID != ""
	credential.LastAuthAt = common.GetTimestamp()
	credential.RefreshStatus = "active"
	credential.ReauthRequired = false
	credential.RefreshUncertain = false
	capturePlatformSiteSessionCookie(session, credential)
	if credential.UserID != "" && platform == model.PlatformNewAPI {
		setNewAPICompatUserHeaders(session.Headers, credential.UserID)
	}
	if token == "" && session.Client.Jar == nil {
		return ErrPlatformSiteAuth
	}
	if token == "" {
		credential.AuthType = model.UpstreamAuthCookie
		credential.Username = ""
		credential.Password = ""
	}
	return nil
}

func capturePlatformSiteSessionCookie(session *PlatformSiteSession, credential *model.PlatformSiteCredential) {
	if session == nil || credential == nil || session.Client == nil || session.Client.Jar == nil {
		return
	}
	parsed, err := url.Parse(session.BaseURL)
	if err != nil {
		return
	}
	cookies := session.Client.Jar.Cookies(parsed)
	if len(cookies) == 0 {
		return
	}
	values := make([]string, 0, len(cookies))
	for _, cookie := range cookies {
		if cookie == nil || strings.TrimSpace(cookie.Name) == "" {
			continue
		}
		values = append(values, cookie.Name+"="+cookie.Value)
	}
	if len(values) > 0 {
		credential.Cookie = strings.Join(values, "; ")
	}
}

func verifyNewAPILogin2FA(ctx context.Context, session *PlatformSiteSession, flowToken, code string) (any, error) {
	if strings.TrimSpace(flowToken) == "" {
		return nil, ErrPlatformSiteAuthFlowInvalid
	}
	body := map[string]string{"flow_token": flowToken, "code": code, "method": "2fa"}
	for _, path := range []string{"/api/user/login/2fa", "/api/user/login/verify"} {
		payload, err := platformSiteRequest(ctx, session, http.MethodPost, path, nil, body)
		if err == nil {
			return payload, nil
		}
	}
	return nil, ErrPlatformSiteAuth
}

func verifySub2APILogin2FA(ctx context.Context, session *PlatformSiteSession, tempToken, code string) (any, error) {
	if strings.TrimSpace(tempToken) == "" {
		return nil, ErrPlatformSiteAuthFlowInvalid
	}
	return platformSiteRequest(ctx, session, http.MethodPost, "/api/v1/auth/login/2fa", nil, map[string]string{
		"temp_token": tempToken,
		"totp_code":  code,
	})
}

func classifyPlatformSiteAuthFlowError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrPlatformSiteHTTPStatus) || errors.Is(err, ErrPlatformSiteAuth) {
		return fmt.Errorf("%w: %w", ErrPlatformSiteAuth, err)
	}
	return err
}

func classifyAuthStatus(err error) string {
	switch {
	case errors.Is(err, ErrPlatformSiteAuthFlowExpired):
		return PlatformSiteAuthFlowStatusExpired
	case errors.Is(err, ErrPlatformSiteAuth):
		return PlatformSiteAuthFlowStatusCredentialsInvalid
	default:
		return PlatformSiteAuthFlowStatusReauthRequired
	}
}

func savePlatformSiteAuthFlow(userID, channelID int, platform, baseURL, status string, methods []string, secret platformSiteAuthFlowSecret) (platformSiteAuthFlowRecord, error) {
	flowID, err := common.GenerateRandomCharsKey(48)
	if err != nil {
		return platformSiteAuthFlowRecord{}, err
	}
	origin, err := platformSiteOrigin(baseURL)
	if err != nil {
		return platformSiteAuthFlowRecord{}, err
	}
	record := platformSiteAuthFlowRecord{
		FlowID: flowID, UserID: userID, ChannelID: channelID, Platform: platform,
		BaseURL: baseURL, Origin: origin, ExpiresAt: time.Now().Add(platformSiteAuthFlowTTL).Unix(),
		Status: status, AuthType: model.UpstreamAuthPassword, Methods: methods,
	}
	record.AuthType = model.InferPlatformSiteAuthType(secret.Credential)
	record.Secret, err = encryptPlatformSiteAuthFlowSecret(secret)
	if err != nil {
		return platformSiteAuthFlowRecord{}, err
	}
	if err := platformSiteAuthFlowCache.SetWithTTL(flowID, record, platformSiteAuthFlowTTL); err != nil {
		return platformSiteAuthFlowRecord{}, err
	}
	return record, nil
}

func savePlatformSiteAuthFlowRecord(record platformSiteAuthFlowRecord) error {
	remaining := time.Until(time.Unix(record.ExpiresAt, 0))
	if remaining <= 0 {
		return ErrPlatformSiteAuthFlowExpired
	}
	return platformSiteAuthFlowCache.SetWithTTL(record.FlowID, record, remaining)
}

func getPlatformSiteAuthFlow(flowID string, userID int) (platformSiteAuthFlowRecord, platformSiteAuthFlowSecret, error) {
	record, err := getPlatformSiteAuthFlowRecord(flowID)
	if err != nil {
		return record, platformSiteAuthFlowSecret{}, err
	}
	if record.UserID != userID {
		return record, platformSiteAuthFlowSecret{}, ErrPlatformSiteAuthFlowInvalid
	}
	secret, err := decryptPlatformSiteAuthFlowSecret(record.Secret)
	if err != nil {
		return record, platformSiteAuthFlowSecret{}, ErrPlatformSiteAuthFlowInvalid
	}
	return record, secret, nil
}

func getPlatformSiteAuthFlowRecord(flowID string) (platformSiteAuthFlowRecord, error) {
	flowID = strings.TrimSpace(flowID)
	if flowID == "" {
		return platformSiteAuthFlowRecord{}, ErrPlatformSiteAuthFlowInvalid
	}
	record, found, err := platformSiteAuthFlowCache.Get(flowID)
	if err != nil {
		return record, err
	}
	if !found {
		return record, ErrPlatformSiteAuthFlowExpired
	}
	if record.Consumed {
		return record, ErrPlatformSiteAuthFlowConsumed
	}
	if record.ExpiresAt <= time.Now().Unix() {
		_, _ = platformSiteAuthFlowCache.DeleteMany([]string{flowID})
		return record, ErrPlatformSiteAuthFlowExpired
	}
	return record, nil
}

func platformSiteAuthFlowResult(record platformSiteAuthFlowRecord) *PlatformSiteAuthFlowResult {
	secret, _ := decryptPlatformSiteAuthFlowSecret(record.Secret)
	return &PlatformSiteAuthFlowResult{
		FlowID: record.FlowID, Status: record.Status, ExpiresAt: record.ExpiresAt,
		Platform: record.Platform, BaseURL: record.BaseURL, ChannelID: record.ChannelID,
		Methods: record.Methods, Attempts: record.Attempts, CodeAttempts: record.CodeAttempts,
		AuthType: record.AuthType, TokenType: secret.Credential.TokenType,
		TokenExpiresAt: secret.Credential.TokenExpiresAt, UserID: secret.Credential.UserID,
		Username: secret.Credential.Username,
	}
}

func encryptPlatformSiteAuthFlowSecret(secret platformSiteAuthFlowSecret) (string, error) {
	payload, err := common.Marshal(secret)
	if err != nil {
		return "", err
	}
	encrypted, err := common.EncryptUpstreamCredential(string(payload))
	if err != nil {
		return "", err
	}
	return encrypted, nil
}

func decryptPlatformSiteAuthFlowSecret(value string) (platformSiteAuthFlowSecret, error) {
	plaintext, err := common.DecryptUpstreamCredential(value)
	if err != nil {
		return platformSiteAuthFlowSecret{}, err
	}
	var secret platformSiteAuthFlowSecret
	if err := common.Unmarshal([]byte(plaintext), &secret); err != nil {
		return platformSiteAuthFlowSecret{}, err
	}
	return secret, nil
}

func findFlowToken(payload any) string {
	record := firstRecord(payload)
	for _, key := range []string{"flow_token", "verification_token", "flowToken", "verificationToken"} {
		if token := firstString(record, key); token != "" {
			return token
		}
	}
	for _, key := range []string{"data", "result", "auth_bundle"} {
		if nested, ok := record[key]; ok {
			if token := findFlowToken(nested); token != "" {
				return token
			}
		}
	}
	return ""
}

func findTempToken(payload any) string {
	record := firstRecord(payload)
	for _, key := range []string{"temp_token", "tempToken"} {
		if token := firstString(record, key); token != "" {
			return token
		}
	}
	for _, key := range []string{"data", "result"} {
		if nested, ok := record[key]; ok {
			if token := findTempToken(nested); token != "" {
				return token
			}
		}
	}
	return ""
}

func findSessionID(payload any) string {
	record := firstRecord(payload)
	if session, ok := record["session"]; ok {
		if sessionRecord := firstRecord(session); len(sessionRecord) > 0 {
			return firstString(sessionRecord, "sid", "id", "session_id")
		}
	}
	return firstString(record, "session_id", "sessionId", "sid")
}

func newAPIAuthMethods(payload any) []string {
	record := firstRecord(payload)
	value, ok := record["methods"]
	if !ok {
		return []string{"totp"}
	}
	methods := make([]string, 0)
	if items, ok := value.([]any); ok {
		for _, item := range items {
			if itemRecord, ok := item.(map[string]any); ok {
				if available, exists := itemRecord["available"].(bool); exists && !available {
					continue
				}
				if method := firstString(itemRecord, "method", "type", "name"); method != "" {
					methods = append(methods, method)
				}
			} else if method, ok := item.(string); ok && strings.TrimSpace(method) != "" {
				methods = append(methods, strings.TrimSpace(method))
			}
		}
	}
	if len(methods) == 0 {
		return []string{"totp"}
	}
	return uniqueStrings(methods)
}
