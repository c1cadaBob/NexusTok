package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/c1cadaBob/NexusTok/common"
	"github.com/c1cadaBob/NexusTok/logger"
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
	PlatformSiteAuthFlowStatusLoginAgreementRequired     = model.PlatformSiteAuthStatusLoginAgreementRequired
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
	if platform == model.PlatformSub2API {
		baseURL = normalizeSub2APIBaseURL(baseURL)
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
	siteClient, err := platformSiteHTTPClientForChannel(request.ChannelID)
	if err != nil {
		return nil, err
	}
	session, payload, err := authenticatePlatformSitePassword(ctx, platform, baseURL, credential, siteClient)
	if err != nil {
		setPasswordSessionMaterialsFromPayload(session, payload)
		cleanupPlatformSiteAuthFlowSession(ctx, platform, session)
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
	captureNewAPIRefreshCookie(session, &secret.Credential)
	if status == PlatformSiteAuthFlowStatusAuthenticated {
		markPlatformSiteAuthFlowSessionForCleanup(session, payload)
		defer cleanupPlatformSiteAuthFlowSession(ctx, platform, session)
		if err := applyPlatformSiteLoginPayload(session, platform, &credential, payload); err != nil {
			return nil, classifyPlatformSiteAuthFlowError(err)
		}
		if err := finalizePlatformSitePasswordCredential(
			ctx,
			platform,
			session,
			&credential,
			payload,
		); err != nil {
			clearPlatformSiteAuthFlowTemporaryCredential(session, &credential)
			return nil, classifyPlatformSiteAuthFlowError(err)
		}
		clearPlatformSiteAuthFlowSecretTemporaryMaterials(&secret)
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
		cleanupPlatformSiteAuthFlowSecret(ctx, &record, &secret)
		clearPlatformSiteAuthFlowSecretTemporaryMaterials(&secret)
		record.Status = PlatformSiteAuthFlowStatusRateLimited
		record.Secret, _ = encryptPlatformSiteAuthFlowSecret(secret)
		_ = savePlatformSiteAuthFlowRecord(record)
		return nil, ErrPlatformSiteAuthFlowRateLimited
	}
	code := strings.TrimSpace(request.Code)
	if len(code) != 6 {
		record.Attempts++
		record.CodeAttempts++
		if record.CodeAttempts >= platformSiteAuthFlowMaxCodeAttempts ||
			record.Attempts >= platformSiteAuthFlowMaxAttempts {
			record.Status = PlatformSiteAuthFlowStatusRateLimited
		}
		if record.Status == PlatformSiteAuthFlowStatusRateLimited {
			cleanupPlatformSiteAuthFlowSecret(ctx, &record, &secret)
			clearPlatformSiteAuthFlowSecretTemporaryMaterials(&secret)
		}
		record.Secret, _ = encryptPlatformSiteAuthFlowSecret(secret)
		_ = savePlatformSiteAuthFlowRecord(record)
		if record.Status == PlatformSiteAuthFlowStatusRateLimited {
			return nil, ErrPlatformSiteAuthFlowRateLimited
		}
		return nil, ErrPlatformSiteAuthFlowCodeInvalid
	}
	session, err := newPlatformSiteSession(record.BaseURL, nil)
	if err != nil {
		return nil, err
	}
	session.Platform = record.Platform
	siteClient, err := platformSiteHTTPClientForChannel(record.ChannelID)
	if err != nil {
		return nil, err
	}
	attachPlatformSiteHTTPClient(session, siteClient)
	if record.Platform == model.PlatformNewAPI {
		session.CredentialUpdate = &secret.Credential
		syncNewAPISessionHeaders(session, secret.Credential)
	} else {
		session.CredentialUpdate = &secret.Credential
	}
	var payload any
	if record.Platform == model.PlatformSub2API {
		prepareSub2APIManagementSession(ctx, session)
		payload, err = verifySub2APILogin2FA(ctx, session, secret.TempToken, code)
	} else {
		setNewAPIBrowserHeaders(session)
		payload, err = verifyNewAPILogin2FA(ctx, session, secret.FlowToken, code)
	}
	if err != nil {
		if persistErr := persistPlatformSiteAuthFlowCredential(&record, &secret, session); persistErr != nil {
			return nil, persistErr
		}
		if errors.Is(err, ErrSub2APILoginAgreement) {
			record.Status = classifyAuthStatus(err)
			if saveErr := savePlatformSiteAuthFlowRecord(record); saveErr != nil {
				return nil, saveErr
			}
			return nil, err
		}
		if !platformSiteAuthFlowCodeRetryable(err) {
			setTemporaryPasswordSessionMaterials(session, secret.Credential)
			cleanupPlatformSiteAuthFlowSession(ctx, record.Platform, session)
			clearPlatformSiteAuthFlowSecretTemporaryMaterials(&secret)
			record.Status = classifyAuthStatus(err)
			record.Secret, _ = encryptPlatformSiteAuthFlowSecret(secret)
			_ = savePlatformSiteAuthFlowRecord(record)
			return nil, classifyPlatformSiteAuthFlowError(err)
		}
		record.Attempts++
		record.CodeAttempts++
		if record.CodeAttempts >= platformSiteAuthFlowMaxCodeAttempts ||
			record.Attempts >= platformSiteAuthFlowMaxAttempts {
			record.Status = PlatformSiteAuthFlowStatusRateLimited
			setTemporaryPasswordSessionMaterials(session, secret.Credential)
			cleanupPlatformSiteAuthFlowSession(ctx, record.Platform, session)
			clearPlatformSiteAuthFlowSecretTemporaryMaterials(&secret)
		}
		record.Secret, _ = encryptPlatformSiteAuthFlowSecret(secret)
		_ = savePlatformSiteAuthFlowRecord(record)
		if record.Status == PlatformSiteAuthFlowStatusRateLimited {
			return nil, ErrPlatformSiteAuthFlowRateLimited
		}
		return nil, ErrPlatformSiteAuthFlowCodeInvalid
	}
	credential := secret.Credential
	markPlatformSiteAuthFlowSessionForCleanup(session, payload)
	defer cleanupPlatformSiteAuthFlowSession(ctx, record.Platform, session)
	if err := applyPlatformSiteLoginPayload(session, record.Platform, &credential, payload); err != nil {
		clearPlatformSiteAuthFlowTemporaryCredential(session, &credential)
		secret.Credential = credential
		if persistErr := persistPlatformSiteAuthFlowCredential(&record, &secret, session); persistErr != nil {
			return nil, persistErr
		}
		record.Status = classifyAuthStatus(err)
		clearPlatformSiteAuthFlowSecretTemporaryMaterials(&secret)
		record.Secret, _ = encryptPlatformSiteAuthFlowSecret(secret)
		_ = savePlatformSiteAuthFlowRecord(record)
		return nil, err
	}
	if err := finalizePlatformSitePasswordCredential(
		ctx,
		record.Platform,
		session,
		&credential,
		payload,
	); err != nil {
		clearPlatformSiteAuthFlowTemporaryCredential(session, &credential)
		secret.Credential = credential
		clearPlatformSiteAuthFlowSecretTemporaryMaterials(&secret)
		record.Status = classifyAuthStatus(err)
		record.Secret, _ = encryptPlatformSiteAuthFlowSecret(secret)
		_ = savePlatformSiteAuthFlowRecord(record)
		return nil, classifyPlatformSiteAuthFlowError(err)
	}
	secret.Credential = credential
	clearPlatformSiteAuthFlowSecretTemporaryMaterials(&secret)
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
	if err == nil && record.Platform == model.PlatformSub2API {
		normalizedBaseURL = normalizeSub2APIBaseURL(normalizedBaseURL)
	}
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
	record, secret, err := getPlatformSiteAuthFlow(flowID, userID)
	if err != nil {
		return err
	}
	cleanupPlatformSiteAuthFlowSecret(context.Background(), &record, &secret)
	clearPlatformSiteAuthFlowSecretTemporaryMaterials(&secret)
	_, err = platformSiteAuthFlowCache.DeleteMany([]string{flowID})
	return err
}

func authenticatePlatformSitePassword(
	ctx context.Context,
	platform,
	baseURL string,
	credential model.PlatformSiteCredential,
	client *http.Client,
) (*PlatformSiteSession, any, error) {
	if platform != model.PlatformNewAPI && platform != model.PlatformSub2API {
		return nil, nil, ErrUnsupportedPlatformSite
	}
	session, err := newPlatformSiteSession(baseURL, make(http.Header))
	if err != nil {
		return nil, nil, err
	}
	session.Platform = platform
	attachPlatformSiteHTTPClient(session, client)
	if platform == model.PlatformSub2API {
		prepareSub2APIManagementSession(ctx, session)
		payload, err := loginSub2APIWithPassword(ctx, session, credential)
		return session, payload, err
	}
	setNewAPIBrowserHeaders(session)
	payload, err := loginNewAPIWithPassword(ctx, session, credential)
	return session, payload, err
}

func markPlatformSiteAuthFlowSessionForCleanup(
	session *PlatformSiteSession,
	payload any,
) {
	if session == nil {
		return
	}
	setPasswordSessionMaterialsFromPayload(session, payload)
}

func finalizePlatformSitePasswordCredential(
	ctx context.Context,
	platform string,
	session *PlatformSiteSession,
	credential *model.PlatformSiteCredential,
	loginPayload any,
) error {
	if session == nil || credential == nil {
		return ErrPlatformSiteAuthFlowInvalid
	}
	var currentUser any
	var err error
	switch platform {
	case model.PlatformNewAPI:
		currentUser, err = fetchNewAPICurrentUser(ctx, session)
	case model.PlatformSub2API:
		currentUser, err = fetchSub2APICurrentUser(ctx, session)
	default:
		return ErrUnsupportedPlatformSite
	}
	if err != nil {
		if platformSiteRouteMissing(err) {
			loginUserID := strings.TrimSpace(findUserID(loginPayload))
			loginUsername := platformSiteUsernameFromRecord(
				firstNestedRecord(loginPayload, "user", "account", "profile"),
			)
			if loginUserID == "" && loginUsername == "" {
				return wrapPlatformSiteStage("平台站点当前用户", err)
			}
			if loginUserID != "" {
				credential.UserID = loginUserID
			}
			if loginUsername != "" {
				credential.Username = loginUsername
			}
			credential.AuthType = model.UpstreamAuthPassword
			credential.LastAuthAt = common.GetTimestamp()
			credential.RefreshStatus = "active"
			credential.ReauthRequired = false
			credential.RefreshUncertain = false
			clearPlatformSiteTemporaryCredential(credential)
			return nil
		}
		return wrapPlatformSiteStage("平台站点当前用户", err)
	}
	userID := strings.TrimSpace(findUserID(currentUser))
	username := platformSiteUsernameFromRecord(
		firstNestedRecord(currentUser, "user", "account", "profile"),
	)
	if userID == "" && username == "" {
		return wrapPlatformSiteStage(
			"平台站点当前用户",
			fmt.Errorf("%w: 当前用户响应缺少身份信息", ErrPlatformSiteAuth),
		)
	}
	if userID != "" {
		credential.UserID = userID
	}
	if username != "" {
		credential.Username = username
	}
	credential.AuthType = model.UpstreamAuthPassword
	credential.LastAuthAt = common.GetTimestamp()
	credential.RefreshStatus = "active"
	credential.ReauthRequired = false
	credential.RefreshUncertain = false
	clearPlatformSiteTemporaryCredential(credential)
	return nil
}

func clearPlatformSiteAuthFlowTemporaryCredential(
	session *PlatformSiteSession,
	credential *model.PlatformSiteCredential,
) {
	if credential != nil {
		clearPlatformSiteTemporaryCredential(credential)
	}
	if session != nil && session.CredentialUpdate != nil {
		clearPlatformSiteTemporaryCredential(session.CredentialUpdate)
	}
}

func clearPlatformSiteAuthFlowSecretTemporaryMaterials(
	secret *platformSiteAuthFlowSecret,
) {
	if secret == nil {
		return
	}
	clearPlatformSiteTemporaryCredential(&secret.Credential)
	secret.FlowToken = ""
	secret.TempToken = ""
}

func cleanupPlatformSiteAuthFlowSession(
	ctx context.Context,
	platform string,
	session *PlatformSiteSession,
) {
	if session == nil || !session.PasswordSession {
		return
	}
	adapter, err := adapterForPlatform(platform, session.Client)
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf(
			"platform site auth flow session cleanup adapter failed: platform=%s error=%s",
			platform,
			SafePlatformSiteSessionCleanupError(err),
		))
		return
	}
	cleanupCtx, cancel := context.WithTimeout(context.Background(), upstreamSiteRequestTimeout)
	defer cancel()
	if err := adapter.Cleanup(cleanupCtx, session); err != nil {
		logger.LogWarn(ctx, fmt.Sprintf(
			"platform site auth flow session cleanup failed: platform=%s error=%s",
			platform,
			SafePlatformSiteSessionCleanupError(err),
		))
	}
}

func cleanupPlatformSiteAuthFlowSecret(
	ctx context.Context,
	record *platformSiteAuthFlowRecord,
	secret *platformSiteAuthFlowSecret,
) {
	if record == nil || secret == nil {
		return
	}
	siteClient, err := platformSiteHTTPClientForChannel(record.ChannelID)
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf(
			"platform site auth flow secret cleanup adapter failed: platform=%s error=%s",
			record.Platform,
			SafePlatformSiteSessionCleanupError(err),
		))
		return
	}
	session, err := newPlatformSiteSession(record.BaseURL, nil)
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf(
			"platform site auth flow secret cleanup session failed: platform=%s error=%s",
			record.Platform,
			SafePlatformSiteSessionCleanupError(err),
		))
		return
	}
	session.Platform = record.Platform
	attachPlatformSiteHTTPClient(session, siteClient)
	session.CredentialUpdate = &secret.Credential
	if record.Platform == model.PlatformNewAPI {
		setNewAPIBrowserHeaders(session)
		if token := strings.TrimSpace(secret.Credential.AccessToken); token != "" {
			session.Headers.Set("Authorization", bearerToken(token))
		}
		syncNewAPISessionHeaders(session, secret.Credential)
	} else {
		setSub2APIBrowserHeaders(session)
	}
	setTemporaryPasswordSessionMaterials(session, secret.Credential)
	cleanupPlatformSiteAuthFlowSession(ctx, record.Platform, session)
}

func platformSiteAuthFlowCodeRetryable(err error) bool {
	if err == nil || errors.Is(err, ErrSub2APILoginAgreement) {
		return false
	}
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "invalid verification code") ||
		strings.Contains(lower, "invalid code") ||
		strings.Contains(err.Error(), "验证码错误") {
		return true
	}
	status, ok := platformSiteHTTPStatusCode(err)
	return ok && (status == http.StatusBadRequest || status == http.StatusUnauthorized)
}

func applyPlatformSiteLoginPayload(session *PlatformSiteSession, platform string, credential *model.PlatformSiteCredential, payload any) error {
	if session == nil || credential == nil {
		return ErrPlatformSiteAuthFlowInvalid
	}
	if platform == model.PlatformNewAPI {
		recognized, bundleErr := applyNewAPIDashboardAuthBundle(payload, credential, true)
		if recognized {
			if bundleErr != nil {
				return bundleErr
			}
			capturePlatformSiteSessionCookie(session, credential)
			captureNewAPIRefreshCookie(session, credential)
			syncNewAPISessionHeaders(session, *credential)
			session.Headers.Set("Authorization", bearerToken(credential.AccessToken))
			setTemporaryPasswordSessionMaterials(session, *credential)
			setNewAPICompatUserHeaders(session.Headers, credential.UserID)
			return nil
		}
	}
	token := findToken(payload)
	if token == "" && platform != model.PlatformNewAPI {
		return ErrSub2APILoginToken
	}
	preservePassword := credential.AuthType == model.UpstreamAuthPassword
	if token != "" {
		if !preservePassword {
			credential.AuthType = model.UpstreamAuthAccessToken
			credential.Username = ""
			credential.Password = ""
		}
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
	if token == "" && !preservePassword {
		credential.AuthType = model.UpstreamAuthCookie
		credential.Username = ""
		credential.Password = ""
	}
	setTemporaryPasswordSessionMaterials(session, *credential)
	return nil
}

func capturePlatformSiteSessionCookie(
	session *PlatformSiteSession,
	credential *model.PlatformSiteCredential,
	rawURLs ...string,
) {
	if session == nil || credential == nil || session.Client == nil || session.Client.Jar == nil {
		return
	}
	rawURL := session.LastRequestURL
	if len(rawURLs) > 0 && strings.TrimSpace(rawURLs[0]) != "" {
		rawURL = rawURLs[0]
	}
	jarCookie := platformSiteJarCookieHeaderForURL(session, rawURL)
	if jarCookie == "" {
		return
	}
	credential.Cookie = mergePlatformSiteCookieHeaders(credential.Cookie, jarCookie)
}

func persistPlatformSiteAuthFlowCredential(
	record *platformSiteAuthFlowRecord,
	secret *platformSiteAuthFlowSecret,
	session *PlatformSiteSession,
) error {
	if record == nil || secret == nil || session == nil || session.CredentialUpdate == nil {
		return nil
	}
	secret.Credential.Cookie = mergePlatformSiteCookieHeaders(
		secret.Credential.Cookie,
		session.CredentialUpdate.Cookie,
	)
	encrypted, err := encryptPlatformSiteAuthFlowSecret(*secret)
	if err != nil {
		return err
	}
	record.Secret = encrypted
	return savePlatformSiteAuthFlowRecord(*record)
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
		if !platformSiteRouteMissing(err) {
			return nil, err
		}
	}
	return nil, ErrPlatformSiteAuth
}

func verifySub2APILogin2FA(ctx context.Context, session *PlatformSiteSession, tempToken, code string) (any, error) {
	if strings.TrimSpace(tempToken) == "" {
		return nil, ErrPlatformSiteAuthFlowInvalid
	}
	body := map[string]string{
		"temp_token": tempToken,
		"totp_code":  code,
	}
	if agreedRevision := sub2APILoginAgreementRevision(ctx, session); agreedRevision != "" {
		body["agreed_revision"] = agreedRevision
	}
	payload, err := platformSiteRequest(
		ctx,
		session,
		http.MethodPost,
		"/api/v1/auth/login/2fa",
		nil,
		body,
	)
	if err != nil && platformSiteLoginAgreementRequired(err) {
		return nil, classifySub2APILoginError(err)
	}
	return payload, err
}

func classifyPlatformSiteAuthFlowError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrSub2APILoginAgreement) {
		return err
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
	case errors.Is(err, ErrSub2APILoginAgreement):
		return PlatformSiteAuthFlowStatusLoginAgreementRequired
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
		if secret, decryptErr := decryptPlatformSiteAuthFlowSecret(record.Secret); decryptErr == nil {
			cleanupPlatformSiteAuthFlowSecret(context.Background(), &record, &secret)
			clearPlatformSiteAuthFlowSecretTemporaryMaterials(&secret)
		}
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
