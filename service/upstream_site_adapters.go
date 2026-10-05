package service

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"maps"
	"math"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/c1cadaBob/NexusTok/common"
	"github.com/c1cadaBob/NexusTok/model"
	"golang.org/x/net/publicsuffix"
)

const defaultNewAPIQuotaPerUnit = 500000

type sub2APIPageConfiguration struct {
	APIBaseURL      string `json:"api_base_url"`
	APIBaseURLCamel string `json:"apiBaseUrl"`
	CustomEndpoints []struct {
		Endpoint string `json:"endpoint"`
	} `json:"custom_endpoints"`
	CustomEndpointsCamel []struct {
		Endpoint string `json:"endpoint"`
	} `json:"customEndpoints"`
}

type sub2APIPageDiscovery struct {
	ManagementBaseURL    string
	ManagementAPIBaseURL string
	RelayBaseURL         string
	DeclaredRelayURLs    []string
	FinalPageURL         string
}

func sub2APIQuotaToInternal(value float64) (int64, error) {
	if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("%w: Sub2API 额度必须是有限的非负数", ErrPlatformSiteResponse)
	}
	quota, clamp := common.QuotaRoundChecked(value * common.QuotaPerUnit)
	if clamp != nil {
		return 0, fmt.Errorf("%w: Sub2API 额度超出允许范围", ErrPlatformSiteResponse)
	}
	return int64(quota), nil
}

func firstSub2APIQuota(record map[string]any, keys ...string) (int64, bool, error) {
	for _, key := range keys {
		raw, exists := record[key]
		if !exists || raw == nil {
			continue
		}
		value, ok := upstreamFloatValue(raw)
		if !ok {
			if isStructuredUpstreamValue(raw) {
				continue
			}
			return 0, false, fmt.Errorf(
				"%w: Sub2API 额度字段无效（字段：%s，类型：%T）",
				ErrPlatformSiteResponse,
				key,
				raw,
			)
		}
		quota, err := sub2APIQuotaToInternal(value)
		if err != nil {
			return 0, false, err
		}
		return quota, true, nil
	}
	return 0, false, nil
}

func firstSub2APIFiniteFloat(
	record map[string]any,
	keys ...string,
) (float64, bool, error) {
	for _, key := range keys {
		raw, exists := record[key]
		if !exists || raw == nil {
			continue
		}
		value, ok := upstreamFloatValue(raw)
		if !ok {
			if isStructuredUpstreamValue(raw) {
				continue
			}
			return 0, false, fmt.Errorf(
				"%w: Sub2API 数值字段无效（字段：%s，类型：%T）",
				ErrPlatformSiteResponse,
				key,
				raw,
			)
		}
		return value, true, nil
	}
	return 0, false, nil
}

func firstSub2APINonNegativeFloat(
	record map[string]any,
	keys ...string,
) (float64, bool, error) {
	value, found, err := firstSub2APIFiniteFloat(record, keys...)
	if err != nil {
		return 0, false, err
	}
	if found && value < 0 {
		return 0, false, fmt.Errorf("%w: Sub2API 数值必须是非负数", ErrPlatformSiteResponse)
	}
	return value, found, nil
}

func isStructuredUpstreamValue(value any) bool {
	switch value.(type) {
	case map[string]any, []any:
		return true
	default:
		return false
	}
}

func firstSub2APIUsedQuota(record map[string]any) (int64, bool, error) {
	return firstSub2APIQuota(
		record,
		"used_quota",
		"quota_used",
		"used",
		"usedQuota",
		"quotaUsed",
		"total_used",
		"total_used_quota",
		"totalUsedQuota",
		"totalUsed",
		"total_actual_cost",
		"totalActualCost",
		"total_cost",
		"totalCost",
	)
}

func firstValidatedFloat(record map[string]any, keys ...string) (float64, bool, error) {
	for _, key := range keys {
		raw, exists := record[key]
		if !exists || raw == nil {
			continue
		}
		value, ok := upstreamFloatValue(raw)
		if !ok {
			return 0, false, fmt.Errorf("%w: 上游数值字段无效", ErrPlatformSiteResponse)
		}
		return value, true, nil
	}
	return 0, false, nil
}

func firstValidatedNonNegativeFloat(
	record map[string]any,
	keys ...string,
) (float64, bool, error) {
	value, found, err := firstValidatedFloat(record, keys...)
	if err != nil {
		return 0, false, err
	}
	if found && value < 0 {
		return 0, false, fmt.Errorf("%w: 上游数值必须是非负数", ErrPlatformSiteResponse)
	}
	return value, found, nil
}

func firstValidatedInternalQuota(record map[string]any, keys ...string) (int64, bool, error) {
	for _, key := range keys {
		raw, exists := record[key]
		if !exists || raw == nil {
			continue
		}
		value, ok := upstreamFloatValue(raw)
		if !ok {
			return 0, false, fmt.Errorf("%w: 上游额度字段无效", ErrPlatformSiteResponse)
		}
		if value < 0 {
			return 0, false, fmt.Errorf("%w: 上游额度必须是有限的非负数", ErrPlatformSiteResponse)
		}
		quota, err := common.QuotaRoundStrict(value)
		if err != nil {
			return 0, false, fmt.Errorf("%w: 上游额度超出允许范围", ErrPlatformSiteResponse)
		}
		return int64(quota), true, nil
	}
	return 0, false, nil
}

type NewAPIAdapter struct {
	client *http.Client
}

func NewNewAPIAdapter(client *http.Client) *NewAPIAdapter {
	return &NewAPIAdapter{client: client}
}

func (adapter *NewAPIAdapter) Platform() string {
	return model.PlatformNewAPI
}

func (adapter *NewAPIAdapter) Authenticate(ctx context.Context, baseURL string, credential model.PlatformSiteCredential) (*PlatformSiteSession, error) {
	headers := make(http.Header)
	session, err := newPlatformSiteSession(baseURL, headers)
	if err != nil {
		return nil, err
	}
	session.Platform = model.PlatformNewAPI
	if adapter.client != nil {
		attachPlatformSiteHTTPClient(session, adapter.client)
	}
	setNewAPIBrowserHeaders(session)
	switch platformSiteCredentialAuthType(credential) {
	case model.UpstreamAuthAccessToken, model.UpstreamAuthCookie:
		// 让登录、当前用户和刷新响应中的 Set-Cookie 都能回写到同一份凭据。
		session.CredentialUpdate = &credential
	}
	var currentUser any
	switch platformSiteCredentialAuthType(credential) {
	case model.UpstreamAuthPassword:
		currentUser, err = authenticateNewAPIPasswordSession(ctx, session, &credential)
		if err != nil {
			return session, wrapPlatformSiteStage("NewAPI 认证", err)
		}
	case model.UpstreamAuthAccessToken:
		if credential.Cookie != "" {
			headers.Set("Cookie", credential.Cookie)
		}
		if credential.SessionID != "" {
			headers.Set("X-Auth-Session", credential.SessionID)
		}
		if credential.RefreshUncertain {
			setPlatformSiteCredentialUpdate(session, credential)
			return session, wrapPlatformSiteStage("NewAPI 刷新令牌", ErrPlatformSiteRefreshUncertain)
		} else if newAPIAccessTokenUsable(credential) {
			headers.Set("Authorization", bearerToken(credential.AccessToken))
			setNewAPICompatUserHeaders(headers, credential.UserID)
		} else if newAPIRefreshMaterial(credential) {
			if err := refreshPlatformSiteSession(
				ctx,
				session,
				"/api/user/auth/refresh",
				&credential,
			); err != nil {
				return session, wrapPlatformSiteStage("NewAPI 刷新令牌", err)
			}
		} else if hasLegacyNewAPIRefreshMaterial(credential) &&
			!newAPIAccessTokenUsable(credential) {
			setPlatformSiteCredentialUpdate(session, credential)
			return session, wrapPlatformSiteStage(
				"NewAPI 刷新令牌",
				errors.Join(ErrPlatformSiteAuth, ErrPlatformSiteRefreshUncertain),
			)
		} else if credential.AccessToken != "" {
			headers.Set("Authorization", bearerToken(credential.AccessToken))
			setNewAPICompatUserHeaders(headers, credential.UserID)
		} else {
			return session, wrapPlatformSiteStage("NewAPI 认证", fmt.Errorf("%w: 缺少访问令牌", ErrPlatformSiteAuth))
		}
	case model.UpstreamAuthAdminKey:
		if credential.AdminKey == "" {
			return session, wrapPlatformSiteStage("NewAPI 认证", fmt.Errorf("%w: 缺少 Admin Key", ErrPlatformSiteAuth))
		}
		headers.Set("Authorization", bearerToken(credential.AdminKey))
		headers.Set("x-api-key", credential.AdminKey)
		headers.Set("New-Api-Key", credential.AdminKey)
	case model.UpstreamAuthCookie:
		if credential.Cookie == "" {
			return session, wrapPlatformSiteStage("NewAPI 认证", fmt.Errorf("%w: 缺少 Cookie", ErrPlatformSiteAuth))
		}
		headers.Set("Cookie", credential.Cookie)
		setNewAPICompatUserHeaders(headers, credential.UserID)
	default:
		return session, wrapPlatformSiteStage("NewAPI 认证", fmt.Errorf("%w: 认证方式不受支持", ErrPlatformSiteAuth))
	}
	if currentUser == nil {
		currentUser, err = fetchNewAPICurrentUser(ctx, session)
		if err != nil {
			if credential.AuthType != model.UpstreamAuthAdminKey {
				return session, wrapPlatformSiteStage("NewAPI 当前用户", err)
			}
			if _, adminErr := fetchNewAPIAdminChannels(ctx, session); adminErr != nil {
				return session, wrapPlatformSiteStage("NewAPI 管理接口", adminErr)
			}
		}
	}
	if currentUser != nil {
		if expectedUserID := strings.TrimSpace(credential.UserID); expectedUserID != "" {
			actualUserID := strings.TrimSpace(findUserID(currentUser))
			if actualUserID != "" && actualUserID != expectedUserID {
				return session, wrapPlatformSiteStage("NewAPI 用户身份", ErrPlatformSiteIdentity)
			}
		}
		if userID := strings.TrimSpace(findUserID(currentUser)); userID != "" {
			credential.UserID = userID
		}
		if username := platformSiteUsernameFromRecord(firstNestedRecord(currentUser, "user", "account", "profile")); username != "" {
			credential.Username = username
		}
		credential.LastAuthAt = common.GetTimestamp()
		credential.RefreshStatus = "active"
		credential.ReauthRequired = false
		credential.RefreshUncertain = false
		if credential.AuthType == model.UpstreamAuthPassword {
			clearPlatformSiteTemporaryCredential(&credential)
			setPlatformSiteCredentialUpdate(session, credential)
		} else if session.CredentialUpdate != nil {
			credential.Cookie = mergePlatformSiteCookieHeaders(
				credential.Cookie,
				session.CredentialUpdate.Cookie,
			)
			session.CredentialUpdate = &credential
		}
	}
	return session, nil
}

func authenticateNewAPIPasswordSession(
	ctx context.Context,
	session *PlatformSiteSession,
	credential *model.PlatformSiteCredential,
) (any, error) {
	if credential == nil {
		return nil, fmt.Errorf("%w: 凭据为空", ErrPlatformSiteAuth)
	}
	credential.AuthType = model.UpstreamAuthPassword
	resetNewAPISessionBeforePasswordLogin(session, credential)
	credential.UserID = ""
	payload, err := loginNewAPIWithPassword(ctx, session, *credential)
	if err != nil {
		setPasswordSessionMaterialsFromPayload(session, nil)
		return nil, err
	}
	if loginRequiresInteractiveVerification(payload) {
		setPasswordSessionMaterialsFromPayload(session, payload)
		return nil, fmt.Errorf("%w: 需要完成上游二次验证", ErrPlatformSiteAuth)
	}
	if err := applyNewAPIPasswordLoginPayload(session, credential, payload); err != nil {
		if !session.PasswordSession {
			setPasswordSessionMaterialsFromPayload(session, payload)
		}
		return nil, err
	}
	return nil, nil
}

func newAPIAccessTokenUsable(credential model.PlatformSiteCredential) bool {
	return strings.TrimSpace(credential.AccessToken) != "" &&
		credential.TokenExpiresAt > common.GetTimestamp()
}

func newAPIRefreshMaterial(credential model.PlatformSiteCredential) bool {
	return hasNewAPIRefreshCookie(credential.Cookie) &&
		strings.TrimSpace(credential.AccessToken) != "" &&
		strings.TrimSpace(credential.SessionID) != ""
}

func hasLegacyNewAPIRefreshMaterial(credential model.PlatformSiteCredential) bool {
	return strings.TrimSpace(credential.RefreshToken) != "" ||
		strings.TrimSpace(credential.SessionID) != ""
}

func hasNewAPIRefreshCookie(cookieHeader string) bool {
	return newAPIRefreshCookieValue(cookieHeader) != ""
}

func newAPIRefreshCookieValue(cookieHeader string) string {
	for _, part := range strings.Split(cookieHeader, ";") {
		name, value, found := strings.Cut(strings.TrimSpace(part), "=")
		if found &&
			strings.EqualFold(strings.TrimSpace(name), "new_api_refresh") &&
			strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func newAPISessionInvalid(err error) bool {
	return errors.Is(err, ErrPlatformSiteCredentials) &&
		!errors.Is(err, ErrPlatformSiteSecurity) &&
		!platformSiteSessionLimit(err)
}

func newAPIPasswordFallbackAllowed(err error) bool {
	return platformSiteHTTPStatusIsUnauthorized(err) && newAPISessionInvalid(err)
}

func resetNewAPISessionBeforePasswordLogin(
	session *PlatformSiteSession,
	credential *model.PlatformSiteCredential,
) {
	if session == nil || credential == nil {
		return
	}
	clearPlatformSitePasswordSession(session)
	credential.AccessToken = ""
	credential.RefreshToken = ""
	credential.TokenExpiresAt = 0
	credential.TokenType = ""
	credential.SessionID = ""
	credential.SessionCurrent = false
	credential.AdminKey = ""
	credential.Cookie = ""
	credential.RefreshStatus = ""
	credential.ReauthRequired = false
	credential.RefreshUncertain = false
}

func applyNewAPIPasswordLoginPayload(
	session *PlatformSiteSession,
	credential *model.PlatformSiteCredential,
	payload any,
) error {
	if session == nil || credential == nil {
		return fmt.Errorf("%w: 登录会话不可用", ErrPlatformSiteAuth)
	}
	tempCredential := model.PlatformSiteCredential{
		AuthType: model.UpstreamAuthPassword,
		Username: credential.Username,
		Password: credential.Password,
	}
	recognized, bundleErr := applyNewAPIDashboardAuthBundle(payload, credential, true)
	if recognized {
		if bundleErr != nil {
			return bundleErr
		}
		capturePlatformSiteSessionCookie(session, credential)
		captureNewAPIRefreshCookie(session, credential)
		syncNewAPISessionHeaders(session, *credential)
		session.Headers.Set("Authorization", bearerToken(credential.AccessToken))
		setNewAPICompatUserHeaders(session.Headers, credential.UserID)
		setTemporaryPasswordSessionMaterials(session, *credential)
		clearPlatformSiteTemporaryCredential(credential)
		return nil
	}

	token := findToken(payload)
	if token != "" {
		tempCredential.AccessToken = token
		tempCredential.TokenType = firstNonEmptyString(findTokenType(payload), "Bearer")
		session.Headers.Set("Authorization", bearerToken(token))
	}
	if refreshToken := findRefreshToken(payload); refreshToken != "" {
		tempCredential.RefreshToken = refreshToken
	}
	tempCredential.TokenExpiresAt = findTokenExpiresAt(payload)
	if userID := findUserID(payload); userID != "" {
		tempCredential.UserID = userID
	}
	if sessionID := findSessionID(payload); sessionID != "" {
		tempCredential.SessionID = sessionID
		tempCredential.SessionCurrent = true
	}
	capturePlatformSiteSessionCookie(session, &tempCredential)
	captureNewAPIRefreshCookie(session, &tempCredential)
	syncNewAPISessionHeaders(session, tempCredential)
	setNewAPICompatUserHeaders(session.Headers, tempCredential.UserID)
	setTemporaryPasswordSessionMaterials(session, tempCredential)
	credential.UserID = tempCredential.UserID
	clearPlatformSiteTemporaryCredential(credential)
	return nil
}

func (adapter *NewAPIAdapter) FetchSnapshot(ctx context.Context, session *PlatformSiteSession) (PlatformSiteSnapshot, error) {
	quotaPerUnit, quotaStatusErr := fetchNewAPIQuotaPerUnit(ctx, session)
	selfPayload, selfErr := fetchNewAPICurrentUser(ctx, session)
	isAdminKey := session != nil && strings.TrimSpace(session.Headers.Get("x-api-key")) != ""
	if selfErr != nil && !isAdminKey {
		return PlatformSiteSnapshot{}, wrapPlatformSiteStage("NewAPI 当前用户", selfErr)
	}
	self := firstNestedRecord(selfPayload, "user", "account", "profile")
	usedQuota, usedQuotaSet, usedQuotaErr := firstValidatedInternalQuota(
		self,
		"used_quota",
		"used",
		"used_quota_amount",
		"quota_used",
		"usedQuota",
		"total_used",
		"totalUsedQuota",
		"total_actual_cost",
		"totalActualCost",
	)
	balanceValue, balanceSet, balanceErr := firstValidatedNonNegativeFloat(
		self,
		"quota",
		"balance",
		"money",
		"credit",
	)
	if balanceErr != nil {
		return PlatformSiteSnapshot{}, balanceErr
	}
	if usedQuotaErr != nil {
		usedQuota = 0
		usedQuotaSet = false
	}
	snapshot := PlatformSiteSnapshot{
		Balance:           normalizeNewAPIQuota(balanceValue, quotaPerUnit),
		BalanceSet:        selfErr == nil && balanceSet,
		UsedQuota:         usedQuota,
		UsedQuotaSet:      usedQuotaSet,
		KeysComplete:      true,
		ManagementBaseURL: strings.TrimRight(firstNonEmptyString(session.ManagementBaseURL, session.BaseURL), "/"),
		RelayBaseURL:      strings.TrimRight(firstNonEmptyString(session.ModelBaseURL, session.BaseURL), "/"),
	}
	if selfErr == nil {
		snapshot.Identity = platformSiteIdentityFromRecord(
			self,
			fmt.Sprintf("%.0f", quotaPerUnit),
			"/api/user/self",
		)
		snapshot.Identity.SourceEndpoint = "/api/user/self"
	}
	snapshot.Endpoint = newAPIEndpointSnapshot(session)
	usageResource := PlatformSiteResourceSyncSnapshot{
		ResourceType:   model.PlatformSiteResourceUsage,
		Status:         model.PlatformSiteResourceStatusSuccess,
		SourceEndpoint: "/api/status,/api/user/self",
		RecordCount:    1,
	}
	usageErr := errors.Join(quotaStatusErr, selfErr, usedQuotaErr)
	if usageErr != nil {
		usageResource = newAPIResourceSyncFailure(
			model.PlatformSiteResourceUsage,
			"/api/status,/api/user/self",
			usageErr,
			true,
		)
	}
	snapshot.ResourceSyncs = append(snapshot.ResourceSyncs, usageResource)
	if snapshot.Identity != nil {
		snapshot.ResourceSyncs = append(snapshot.ResourceSyncs, PlatformSiteResourceSyncSnapshot{
			ResourceType:   model.PlatformSiteResourceIdentity,
			Status:         model.PlatformSiteResourceStatusSuccess,
			SourceEndpoint: "/api/user/self",
			RecordCount:    1,
		})
	} else if selfErr != nil {
		snapshot.ResourceSyncs = append(snapshot.ResourceSyncs, newAPIResourceSyncFailure(
			model.PlatformSiteResourceIdentity,
			"/api/user/self",
			selfErr,
			true,
		))
	}
	if isAdminKey {
		adminChannels, adminErr := fetchNewAPIAdminChannels(ctx, session)
		if adminErr == nil {
			adminModels := make([]string, 0)
			var adminModelErr error
			for _, channel := range adminChannels {
				channelModels, channelErr := fetchNewAPIAdminChannelModels(ctx, session, channel)
				adminModels = append(adminModels, channelModels...)
				if channelErr != nil && adminModelErr == nil {
					adminModelErr = channelErr
				}
			}
			if len(adminModels) > 0 {
				snapshot.Models = uniqueStrings(append(snapshot.Models, adminModels...))
			}
			if adminModelErr != nil {
				snapshot.ResourceSyncs = append(snapshot.ResourceSyncs, newAPIResourceSyncFailure(
					model.PlatformSiteResourceModels,
					"/api/channel/,/api/channel/{id},/api/channel/fetch_models/{id}",
					adminModelErr,
					true,
				))
			} else {
				snapshot.ResourceSyncs = append(snapshot.ResourceSyncs, PlatformSiteResourceSyncSnapshot{
					ResourceType:   model.PlatformSiteResourceModels,
					Status:         model.PlatformSiteResourceStatusSuccess,
					SourceEndpoint: "/api/channel/,/api/channel/{id},/api/channel/fetch_models/{id}",
					RecordCount:    len(adminModels),
				})
			}
		} else {
			snapshot.ResourceSyncs = append(snapshot.ResourceSyncs, newAPIResourceSyncFailure(
				model.PlatformSiteResourceModels,
				"/api/channel/",
				adminErr,
				true,
			))
		}
	}
	groupRates, groupSnapshots, groupsLoaded, groupEndpoint, groupErr := fetchNewAPIGroupResources(ctx, session)
	snapshot.Groups = groupSnapshots
	snapshot.GroupsLoaded = groupsLoaded
	if groupsLoaded && (groupErr == nil || platformSiteRouteMissing(groupErr)) {
		snapshot.ResourceSyncs = append(snapshot.ResourceSyncs, PlatformSiteResourceSyncSnapshot{
			ResourceType:   model.PlatformSiteResourceGroups,
			Status:         model.PlatformSiteResourceStatusSuccess,
			SourceEndpoint: groupEndpoint,
			RecordCount:    len(groupSnapshots),
		})
	} else if groupErr != nil {
		snapshot.ResourceSyncs = append(snapshot.ResourceSyncs, newAPIResourceSyncFailure(
			model.PlatformSiteResourceGroups,
			"/api/user/self/groups,/api/user/groups,/api/groupPro/selectable",
			groupErr,
			groupsLoaded,
		))
	}
	ratioConfigPayload, ratioConfigErr := platformSiteRequest(
		ctx,
		session,
		http.MethodGet,
		"/api/ratio_config",
		nil,
		nil,
	)
	if ratioConfigErr != nil {
		snapshot.ResourceSyncs = append(snapshot.ResourceSyncs, newAPIResourceSyncFailure(
			model.PlatformSiteResourceEndpoints,
			"/api/ratio_config",
			ratioConfigErr,
			false,
		))
	} else {
		snapshot.ResourceSyncs = append(snapshot.ResourceSyncs, PlatformSiteResourceSyncSnapshot{
			ResourceType:   model.PlatformSiteResourceEndpoints,
			Status:         model.PlatformSiteResourceStatusSuccess,
			SourceEndpoint: "/api/ratio_config",
			RecordCount:    len(recordsFromPayload(ratioConfigPayload)),
		})
	}
	pricingPayload, pricingErr := platformSiteRequest(ctx, session, http.MethodGet, "/api/pricing", nil, nil)
	if pricingErr == nil {
		if !applyNewAPIPricingResources(&snapshot, session, pricingPayload) {
			snapshot.ResourceSyncs = append(snapshot.ResourceSyncs, newAPIResourceSyncFailure(
				model.PlatformSiteResourceEndpoints,
				"/api/pricing",
				fmt.Errorf("%w: NewAPI 价格响应为空", ErrPlatformSiteResponse),
				false,
			))
		}
	} else {
		snapshot.ResourceSyncs = append(snapshot.ResourceSyncs, newAPIResourceSyncFailure(
			model.PlatformSiteResourceEndpoints,
			"/api/pricing",
			pricingErr,
			false,
		))
	}
	tokens, tokensErr := fetchNewAPITokens(ctx, session)
	if tokensErr != nil {
		snapshot.KeysComplete = false
		tokenResource := newAPIResourceSyncFailure(
			model.PlatformSiteResourceKeys,
			"/api/token/,/api/token,/api/tokens",
			errors.Join(ErrPlatformSiteResource, tokensErr),
			true,
		)
		snapshot.ResourceSyncs = append(snapshot.ResourceSyncs,
			tokenResource,
			PlatformSiteResourceSyncSnapshot{
				ResourceType:   model.PlatformSiteResourceModels,
				Status:         model.PlatformSiteResourceStatusStale,
				SourceEndpoint: "/api/user/models,/v1/models",
				FailureReason:  "密钥分页失败，保留最近成功模型快照",
				Partial:        true,
			},
		)
		normalizePlatformSiteResourceSyncs(&snapshot)
		return snapshot, wrapPlatformSiteStage(
			"NewAPI 密钥分页",
			errors.Join(ErrPlatformSiteResource, tokensErr),
		)
	}
	revealedKeys, revealFailures := fetchNewAPITokenKeys(ctx, session, tokens)
	snapshot.Keys = make([]UpstreamKeySnapshot, 0, len(tokens))
	snapshot.KeysComplete = true
	keysRequireSecurityVerification := false
	for _, token := range tokens {
		externalID := firstString(token, "id", "token_id", "key_id")
		if externalID == "" {
			return PlatformSiteSnapshot{}, fmt.Errorf("%w: NewAPI 密钥缺少外部 ID", ErrPlatformSiteResponse)
		}
		group := firstString(token, "group", "group_name")
		ratioKeysPresent := hasAnyField(token, "ratio", "rate", "multiplier", "group_ratio")
		ratio := firstFloat(token, "ratio", "rate", "multiplier", "group_ratio")
		_, groupRateSet := groupRates[group]
		if !ratioKeysPresent {
			ratio = groupRates[group]
		}
		itemModels := modelsFromRecord(token)
		keyUsedQuota, keyUsedQuotaSet, quotaErr := firstValidatedInternalQuota(
			token,
			"used_quota",
			"used",
			"quota_used",
			"usedQuota",
			"total_used",
			"totalUsedQuota",
		)
		if quotaErr != nil {
			return PlatformSiteSnapshot{}, quotaErr
		}
		remainQuota, remainErr := newAPIRemainQuota(token)
		if remainErr != nil {
			return PlatformSiteSnapshot{}, remainErr
		}
		item := UpstreamKeySnapshot{
			ExternalID:               externalID,
			Name:                     firstString(token, "name", "key_name", "token_name"),
			Group:                    group,
			Models:                   itemModels,
			ModelsSynced:             len(itemModels) > 0,
			SourceConversionRatio:    ratio,
			SourceConversionRatioSet: ratioKeysPresent || groupRateSet,
			ConversionRatio:          ratio,
			ConversionRatioSet:       ratioKeysPresent || groupRateSet,
			UsedQuota:                keyUsedQuota,
			UsedQuotaSet:             keyUsedQuotaSet,
			RemainQuota:              remainQuota,
			ExpiresAt:                firstTime(token, "expired_time", "expires_at", "expire_at"),
			Disabled:                 isUpstreamKeyDisabled(token),
		}
		secret := strings.TrimSpace(revealedKeys[externalID])
		if secret == "" {
			secret = firstString(token, "key", "token", "api_key")
		}
		if secret == "" || strings.Contains(secret, "*") {
			if revealErr := revealFailures[externalID]; revealErr != nil {
				if errors.Is(revealErr, ErrPlatformSiteSecurity) {
					keysRequireSecurityVerification = true
				}
				item.SyncError = upstreamKeySyncErrorSecretUnavailable
				snapshot.KeysComplete = false
				snapshot.Keys = append(snapshot.Keys, item)
				continue
			}
		}
		if secret == "" || strings.Contains(secret, "*") {
			item.SyncError = upstreamKeySyncErrorSecretUnavailable
			snapshot.KeysComplete = false
			snapshot.Keys = append(snapshot.Keys, item)
			continue
		}
		item.Secret = secret
		if !item.ModelsSynced {
			models, modelsErr := fetchModelsForSecret(ctx, session, secret)
			if modelsErr == nil && len(models) > 0 {
				item.Models = models
				item.ModelsSynced = true
			} else {
				if errors.Is(modelsErr, ErrPlatformSiteSecurity) {
					keysRequireSecurityVerification = true
				}
				item.SyncError = upstreamKeySyncErrorModelsUnavailable
				snapshot.KeysComplete = false
			}
		}
		snapshot.Keys = append(snapshot.Keys, item)
	}
	accountModels, accountModelsErr := fetchNewAPIModels(ctx, session)
	if accountModelsErr == nil {
		snapshot.Models = uniqueStrings(append(snapshot.Models, accountModels...))
		snapshot.ResourceSyncs = append(snapshot.ResourceSyncs, PlatformSiteResourceSyncSnapshot{
			ResourceType:   model.PlatformSiteResourceModels,
			Status:         model.PlatformSiteResourceStatusSuccess,
			SourceEndpoint: "/api/user/models,/api/user/available_models,/api/user/available_model/",
			RecordCount:    len(accountModels),
		})
	} else {
		snapshot.ResourceSyncs = append(snapshot.ResourceSyncs,
			newAPIResourceSyncFailure(
				model.PlatformSiteResourceModels,
				"/api/user/models,/api/user/available_models,/api/user/available_model/",
				accountModelsErr,
				false,
			),
		)
	}
	keysResourceStatus := model.PlatformSiteResourceStatusSuccess
	keysFailureReason := ""
	keysPartial := false
	if !snapshot.KeysComplete {
		keysResourceStatus = model.PlatformSiteResourceStatusPartial
		keysFailureReason = "部分密钥详情或模型能力读取失败，已保留最近成功快照"
		keysPartial = true
	}
	if keysRequireSecurityVerification {
		keysResourceStatus = model.PlatformSiteResourceStatusSecureVerificationRequired
		keysFailureReason = "读取密钥或模型能力需要完成上游安全验证，已保留最近成功快照"
		snapshot.AuthStatus = model.PlatformSiteAuthStatusSecureVerificationRequired
		snapshot.AuthStatusReason = keysFailureReason
	}
	snapshot.ResourceSyncs = append(snapshot.ResourceSyncs, PlatformSiteResourceSyncSnapshot{
		ResourceType:                 model.PlatformSiteResourceKeys,
		Status:                       keysResourceStatus,
		SourceEndpoint:               "/api/token/,/api/token/batch/keys",
		RecordCount:                  len(snapshot.Keys),
		FailureReason:                keysFailureReason,
		Partial:                      keysPartial,
		RequiresSecurityVerification: keysRequireSecurityVerification,
	})
	if models := uniqueStrings(modelsFromKeys(snapshot.Keys)); len(models) > 0 {
		snapshot.Models = uniqueStrings(append(snapshot.Models, models...))
		modelsStatus := model.PlatformSiteResourceStatusSuccess
		modelsFailureReason := ""
		if !snapshot.KeysComplete {
			modelsStatus = model.PlatformSiteResourceStatusStale
			modelsFailureReason = "密钥资源未完整同步，保留最近成功模型快照"
		}
		if keysRequireSecurityVerification {
			modelsStatus = model.PlatformSiteResourceStatusSecureVerificationRequired
			modelsFailureReason = "密钥资源需要完成上游安全验证，保留最近成功模型快照"
		}
		snapshot.ResourceSyncs = append(snapshot.ResourceSyncs, PlatformSiteResourceSyncSnapshot{
			ResourceType:                 model.PlatformSiteResourceModels,
			Status:                       modelsStatus,
			SourceEndpoint:               "/v1/models",
			RecordCount:                  len(models),
			FailureReason:                modelsFailureReason,
			RequiresSecurityVerification: keysRequireSecurityVerification,
		})
	} else {
		modelsStatus := model.PlatformSiteResourceStatusStale
		modelsFailureReason := "账号级或密钥级模型目录接口不可用"
		if keysRequireSecurityVerification {
			modelsStatus = model.PlatformSiteResourceStatusSecureVerificationRequired
			modelsFailureReason = "密钥资源需要完成上游安全验证，保留最近成功模型快照"
		}
		snapshot.ResourceSyncs = append(snapshot.ResourceSyncs, PlatformSiteResourceSyncSnapshot{
			ResourceType:                 model.PlatformSiteResourceModels,
			Status:                       modelsStatus,
			SourceEndpoint:               "/api/user/models,/v1/models",
			FailureReason:                modelsFailureReason,
			RequiresSecurityVerification: keysRequireSecurityVerification,
		})
	}
	normalizePlatformSiteResourceSyncs(&snapshot)
	return snapshot, nil
}

type Sub2APIAdapter struct {
	client *http.Client
}

func NewSub2APIAdapter(client *http.Client) *Sub2APIAdapter {
	return &Sub2APIAdapter{client: client}
}

func (adapter *Sub2APIAdapter) Platform() string {
	return model.PlatformSub2API
}

func (adapter *Sub2APIAdapter) Authenticate(ctx context.Context, baseURL string, credential model.PlatformSiteCredential) (*PlatformSiteSession, error) {
	headers := make(http.Header)
	baseURL = normalizeSub2APIBaseURL(baseURL)
	session, err := newPlatformSiteSession(baseURL, headers)
	if err != nil {
		return nil, err
	}
	session.Platform = model.PlatformSub2API
	if adapter.client != nil {
		attachPlatformSiteHTTPClient(session, adapter.client)
	}
	if platformSiteCredentialAuthType(credential) == model.UpstreamAuthPassword {
		resetSub2APISessionBeforePasswordLogin(session, &credential)
	}
	prepareSub2APIManagementSession(ctx, session)
	loginIdentity := strings.TrimSpace(credential.Username)
	loginIdentityIsEmail := strings.Contains(loginIdentity, "@")
	switch platformSiteCredentialAuthType(credential) {
	case model.UpstreamAuthPassword:
		payload, requestErr := loginSub2APIWithPassword(ctx, session, credential)
		if requestErr != nil {
			setPasswordSessionMaterialsFromPayload(session, nil)
			return session, wrapPlatformSiteStage("Sub2API 登录", requestErr)
		}
		if loginRequiresInteractiveVerification(payload) {
			setPasswordSessionMaterialsFromPayload(session, payload)
			return session, fmt.Errorf("%w: 需要完成上游二次验证", ErrPlatformSiteAuth)
		}
		if token := findToken(payload); token != "" {
			session.Headers.Set("Authorization", bearerToken(token))
		} else {
			setPasswordSessionMaterialsFromPayload(session, payload)
			return session, wrapPlatformSiteStage("Sub2API 登录未返回访问令牌", ErrSub2APILoginToken)
		}
		if err := applySub2APIPasswordLoginPayload(session, &credential, payload); err != nil {
			if !session.PasswordSession {
				setPasswordSessionMaterialsFromPayload(session, payload)
			}
			return session, wrapPlatformSiteStage("Sub2API 登录响应", err)
		}
	case model.UpstreamAuthAccessToken:
		if credential.Cookie != "" {
			headers.Set("Cookie", credential.Cookie)
		}
		if credential.RefreshToken != "" {
			if err := refreshPlatformSiteSession(
				ctx,
				session,
				"/api/v1/auth/refresh",
				&credential,
			); err != nil {
				return session, wrapPlatformSiteStage("Sub2API 刷新令牌", err)
			}
		} else if credential.AccessToken != "" {
			headers.Set("Authorization", bearerToken(credential.AccessToken))
		} else {
			return nil, wrapPlatformSiteStage("Sub2API 认证", fmt.Errorf("%w: 缺少访问令牌", ErrPlatformSiteAuth))
		}
	case model.UpstreamAuthAdminKey:
		if credential.AdminKey == "" {
			return nil, wrapPlatformSiteStage("Sub2API 认证", fmt.Errorf("%w: 缺少 Admin Key", ErrPlatformSiteAuth))
		}
		headers.Set("Authorization", bearerToken(credential.AdminKey))
		headers.Set("x-api-key", credential.AdminKey)
	case model.UpstreamAuthCookie:
		if credential.Cookie == "" {
			return nil, wrapPlatformSiteStage("Sub2API 认证", fmt.Errorf("%w: 缺少 Cookie", ErrPlatformSiteAuth))
		}
		headers.Set("Cookie", credential.Cookie)
	default:
		return nil, wrapPlatformSiteStage("Sub2API 认证", fmt.Errorf("%w: 认证方式不受支持", ErrPlatformSiteAuth))
	}
	currentUser, err := fetchSub2APICurrentUser(ctx, session)
	if err != nil {
		return session, wrapPlatformSiteStage("Sub2API 当前用户", err)
	}
	if expectedUserID := strings.TrimSpace(credential.UserID); expectedUserID != "" {
		actualUserID := strings.TrimSpace(findUserID(currentUser))
		if actualUserID != "" && actualUserID != expectedUserID {
			return session, wrapPlatformSiteStage("Sub2API 用户身份", ErrPlatformSiteIdentity)
		}
	}
	if userID := strings.TrimSpace(findUserID(currentUser)); userID != "" {
		credential.UserID = userID
	}
	if username := platformSiteUsernameFromRecord(firstNestedRecord(currentUser, "user", "account", "profile")); username != "" {
		if platformSiteCredentialAuthType(credential) != model.UpstreamAuthPassword ||
			!loginIdentityIsEmail {
			credential.Username = username
		} else {
			credential.Username = loginIdentity
		}
	}
	credential.LastAuthAt = common.GetTimestamp()
	credential.RefreshStatus = "active"
	credential.ReauthRequired = false
	credential.RefreshUncertain = false
	if credential.AuthType == model.UpstreamAuthPassword {
		clearPlatformSiteTemporaryCredential(&credential)
		setPlatformSiteCredentialUpdate(session, credential)
	}
	return session, nil
}

func prepareSub2APIManagementSession(ctx context.Context, session *PlatformSiteSession) {
	if session == nil {
		return
	}
	session.Platform = model.PlatformSub2API
	session.BaseURL = normalizeSub2APIBaseURL(session.BaseURL)
	session.ManagementBaseURL = session.BaseURL
	setSub2APIBrowserHeaders(session)
	if managementBaseURL, requestBaseURL, modelBaseURL, ok := discoverSub2APIManagementBaseURL(
		ctx,
		session,
	); ok {
		session.BaseURL = requestBaseURL
		session.ManagementBaseURL = managementBaseURL
		setSub2APIBrowserHeaders(session)
		session.ModelBaseURL = modelBaseURL
	}
}

func platformSiteCredentialAuthType(credential model.PlatformSiteCredential) string {
	switch authType := strings.ToLower(strings.TrimSpace(credential.AuthType)); authType {
	case model.UpstreamAuthPassword, model.UpstreamAuthAccessToken,
		model.UpstreamAuthAdminKey, model.UpstreamAuthCookie:
		return authType
	default:
		return ""
	}
}

func (adapter *Sub2APIAdapter) FetchSnapshot(ctx context.Context, session *PlatformSiteSession) (PlatformSiteSnapshot, error) {
	mePayload, err := fetchSub2APICurrentUser(ctx, session)
	if err != nil {
		return PlatformSiteSnapshot{}, wrapPlatformSiteStage("Sub2API 当前用户", err)
	}
	me := firstNestedRecord(mePayload, "user", "account", "profile")
	usedQuota, usedQuotaSet, quotaErr := firstSub2APIUsedQuota(me)
	if quotaErr != nil {
		return PlatformSiteSnapshot{}, wrapPlatformSiteStage("Sub2API 当前用户用量字段", quotaErr)
	}
	balance, balanceSet, balanceErr := firstSub2APIFiniteFloat(
		me,
		"balance",
		"quota",
		"credit",
	)
	if balanceErr != nil {
		return PlatformSiteSnapshot{}, wrapPlatformSiteStage("Sub2API 当前用户余额字段", balanceErr)
	}
	snapshot := PlatformSiteSnapshot{
		Balance:           balance,
		BalanceSet:        balanceSet,
		UsedQuota:         usedQuota,
		UsedQuotaSet:      usedQuotaSet,
		ManagementBaseURL: strings.TrimRight(strings.TrimSpace(session.ManagementBaseURL), "/"),
		RelayBaseURL:      strings.TrimRight(strings.TrimSpace(session.ModelBaseURL), "/"),
		KeysComplete:      true,
		Identity:          platformSiteIdentityFromRecord(me, "quota", "/api/v1/auth/me"),
		Endpoint:          sub2APIEndpointSnapshot(session),
	}
	snapshot.ResourceSyncs = append(snapshot.ResourceSyncs,
		PlatformSiteResourceSyncSnapshot{
			ResourceType:   model.PlatformSiteResourceIdentity,
			Status:         model.PlatformSiteResourceStatusSuccess,
			SourceEndpoint: "/api/v1/auth/me",
			RecordCount:    1,
		},
	)

	var profileErr error
	if payload, requestErr := platformSiteRequest(ctx, session, http.MethodGet, "/api/v1/user/profile", nil, nil); requestErr == nil {
		profile := firstNestedRecord(payload, "profile", "user", "account")
		if profileBalance, profileBalanceSet, profileBalanceErr := firstSub2APIFiniteFloat(
			profile,
			"balance",
			"quota",
			"credit",
		); profileBalanceErr == nil && profileBalanceSet {
			snapshot.Balance = profileBalance
			snapshot.BalanceSet = true
		} else if profileBalanceErr != nil {
			profileErr = profileBalanceErr
		}
		if !snapshot.UsedQuotaSet {
			if profileUsedQuota, profileUsedQuotaSet, profileQuotaErr := firstSub2APIUsedQuota(profile); profileQuotaErr == nil && profileUsedQuotaSet {
				snapshot.UsedQuota = profileUsedQuota
				snapshot.UsedQuotaSet = true
			} else if profileQuotaErr != nil && profileErr == nil {
				profileErr = profileQuotaErr
			}
		}
	} else {
		profileErr = requestErr
	}

	rates := map[string]float64{}
	groupPayload, groupLoaded := any(nil), false
	groupErr := error(nil)
	if payload, requestErr := platformSiteRequest(ctx, session, http.MethodGet, "/api/v1/groups/available", nil, nil); requestErr != nil {
		groupErr = requestErr
	} else {
		groupPayload = payload
		groupLoaded = true
		for key, value := range parseGroupRates(payload) {
			rates[key] = value
		}
	}
	groupRatesErr := error(nil)
	if groupErr == nil {
		if payload, requestErr := platformSiteRequest(ctx, session, http.MethodGet, "/api/v1/groups/rates", nil, nil); requestErr != nil {
			groupRatesErr = requestErr
		} else {
			for key, value := range parseGroupRates(payload) {
				rates[key] = value
			}
		}
	}
	snapshot.Groups = parsePlatformSiteGroups(groupPayload, "/api/v1/groups/available", rates)
	snapshot.GroupsLoaded = groupLoaded
	if groupErr != nil {
		snapshot.KeysComplete = false
		if errors.Is(groupErr, ErrPlatformSiteSecurity) {
			snapshot.AuthStatus = model.PlatformSiteAuthStatusSecureVerificationRequired
			snapshot.AuthStatusReason = "读取分组需要完成上游安全验证"
		}
		snapshot.ResourceSyncs = append(snapshot.ResourceSyncs,
			sub2APIResourceSyncFailure(
				model.PlatformSiteResourceGroups,
				"/api/v1/groups/available",
				groupErr,
				false,
			),
		)
		return snapshot, wrapPlatformSiteStage(
			"Sub2API 分组",
			errors.Join(ErrPlatformSiteResource, groupErr),
		)
	}
	groupsStatus := model.PlatformSiteResourceStatusSuccess
	groupsFailureReason := ""
	groupsPartial := false
	if groupRatesErr != nil {
		groupsStatus = model.PlatformSiteResourceStatusPartial
		groupsFailureReason = "分组倍率接口不可用，已保留可用分组并继续同步密钥"
		groupsPartial = true
		if errors.Is(groupRatesErr, ErrPlatformSiteSecurity) {
			groupsStatus = model.PlatformSiteResourceStatusSecureVerificationRequired
			groupsFailureReason = "分组倍率接口需要完成上游安全验证"
			snapshot.AuthStatus = model.PlatformSiteAuthStatusSecureVerificationRequired
			snapshot.AuthStatusReason = groupsFailureReason
		}
	}
	snapshot.ResourceSyncs = append(snapshot.ResourceSyncs, PlatformSiteResourceSyncSnapshot{
		ResourceType:                 model.PlatformSiteResourceGroups,
		Status:                       groupsStatus,
		SourceEndpoint:               "/api/v1/groups/available,/api/v1/groups/rates",
		RecordCount:                  len(snapshot.Groups),
		FailureReason:                groupsFailureReason,
		Partial:                      groupsPartial,
		RequiresSecurityVerification: errors.Is(groupRatesErr, ErrPlatformSiteSecurity),
	})

	usageSource := "/api/v1/auth/me"
	usageErrors := make([]error, 0, 3)
	if profileErr != nil {
		usageErrors = append(usageErrors, profileErr)
	}
	dashboardUsedSet := false
	dashboardPayload, dashboardErr := platformSiteRequest(
		ctx,
		session,
		http.MethodGet,
		"/api/v1/usage/dashboard/stats",
		nil,
		nil,
	)
	if dashboardErr != nil {
		usageErrors = append(usageErrors, dashboardErr)
	} else {
		usage := firstNestedRecord(dashboardPayload, "stats", "usage", "dashboard")
		if used, usedSet, usageQuotaErr := firstSub2APIUsedQuota(usage); usageQuotaErr != nil {
			usageErrors = append(usageErrors, usageQuotaErr)
		} else if usedSet {
			dashboardUsedSet = true
			if !snapshot.UsedQuotaSet {
				snapshot.UsedQuota = used
				snapshot.UsedQuotaSet = true
				usageSource = "/api/v1/usage/dashboard/stats"
			}
		} else {
			usageErrors = append(usageErrors, fmt.Errorf("%w: Sub2API Dashboard 未返回累计用量", ErrPlatformSiteResponse))
		}
	}

	shouldReadUsageStats := !dashboardUsedSet &&
		(dashboardErr == nil || platformSiteRouteMissing(dashboardErr))
	if shouldReadUsageStats {
		statsPayload, statsErr := platformSiteRequest(
			ctx,
			session,
			http.MethodGet,
			"/api/v1/usage/stats",
			nil,
			nil,
		)
		if statsErr != nil {
			usageErrors = append(usageErrors, statsErr)
		} else {
			usage := firstNestedRecord(statsPayload, "stats", "usage", "dashboard")
			if used, usedSet, usageQuotaErr := firstSub2APIUsedQuota(usage); usageQuotaErr != nil {
				usageErrors = append(usageErrors, usageQuotaErr)
			} else if usedSet && !snapshot.UsedQuotaSet {
				snapshot.UsedQuota = used
				snapshot.UsedQuotaSet = true
				usageSource = "/api/v1/usage/stats"
			} else if !usedSet {
				usageErrors = append(usageErrors, fmt.Errorf("%w: Sub2API Usage Stats 未返回累计用量", ErrPlatformSiteResponse))
			}
		}
	}
	usageStatus := model.PlatformSiteResourceStatusSuccess
	usageFailureReason := ""
	usagePartial := false
	usageSecurityRequired := false
	if !snapshot.UsedQuotaSet {
		usageStatus = model.PlatformSiteResourceStatusPartial
		usageFailureReason = "账号累计用量字段缺失，持久化阶段将按本轮密钥已用量回退"
		usagePartial = true
		if len(usageErrors) > 0 {
			usageFailureReason += "：" + platformSiteResourceFailureReason(usageErrors[0])
		}
	}
	for _, usageErr := range usageErrors {
		if errors.Is(usageErr, ErrPlatformSiteSecurity) {
			usageStatus = model.PlatformSiteResourceStatusSecureVerificationRequired
			usageFailureReason = "账号用量接口需要完成上游安全验证"
			usagePartial = true
			usageSecurityRequired = true
			snapshot.AuthStatus = model.PlatformSiteAuthStatusSecureVerificationRequired
			snapshot.AuthStatusReason = usageFailureReason
			break
		}
	}
	snapshot.ResourceSyncs = append(snapshot.ResourceSyncs, PlatformSiteResourceSyncSnapshot{
		ResourceType:                 model.PlatformSiteResourceUsage,
		Status:                       usageStatus,
		SourceEndpoint:               usageSource + ",/api/v1/usage/dashboard/stats,/api/v1/usage/stats",
		RecordCount:                  1,
		FailureReason:                usageFailureReason,
		Partial:                      usagePartial,
		RequiresSecurityVerification: usageSecurityRequired,
	})

	var keyErr error
	if session.Headers.Get("x-api-key") != "" {
		snapshot.Keys, keyErr = fetchSub2APIAdminKeys(ctx, session, rates)
	} else {
		snapshot.Keys, keyErr = fetchSub2APIKeys(ctx, session, rates)
	}
	if keyErr != nil {
		snapshot.KeysComplete = false
		keyResourceEndpoint := "/api/v1/keys,/api/v1/keys/{id}"
		if session.Headers.Get("x-api-key") != "" {
			keyResourceEndpoint = "/api/v1/admin/accounts,/api/v1/admin/accounts/data"
		}
		keyResource := sub2APIResourceSyncFailure(
			model.PlatformSiteResourceKeys,
			keyResourceEndpoint,
			keyErr,
			len(snapshot.Keys) > 0,
		)
		snapshot.ResourceSyncs = append(snapshot.ResourceSyncs, keyResource)
		if errors.Is(keyErr, ErrPlatformSiteSecurity) {
			snapshot.AuthStatus = model.PlatformSiteAuthStatusSecureVerificationRequired
			snapshot.AuthStatusReason = "读取密钥需要完成上游安全验证"
			snapshot.ResourceSyncs = append(snapshot.ResourceSyncs, PlatformSiteResourceSyncSnapshot{
				ResourceType:                 model.PlatformSiteResourceModels,
				Status:                       model.PlatformSiteResourceStatusSecureVerificationRequired,
				SourceEndpoint:               "/v1/models",
				FailureReason:                "密钥资源未完成安全验证，保留最近成功模型快照",
				Partial:                      true,
				RequiresSecurityVerification: true,
			})
		}
		return snapshot, wrapPlatformSiteStage(
			"Sub2API 密钥分页",
			errors.Join(ErrPlatformSiteResource, keyErr),
		)
	}
	snapshot.KeysComplete = true
	keysResourceStatus := model.PlatformSiteResourceStatusSuccess
	keysFailureReason := ""
	keysPartial := false
	for _, key := range snapshot.Keys {
		if key.SyncError != "" {
			snapshot.KeysComplete = false
			keysResourceStatus = model.PlatformSiteResourceStatusPartial
			keysFailureReason = "部分密钥详情或模型能力读取失败，已保留最近成功快照"
			keysPartial = true
			break
		}
	}
	snapshot.ResourceSyncs = append(snapshot.ResourceSyncs, PlatformSiteResourceSyncSnapshot{
		ResourceType:   model.PlatformSiteResourceKeys,
		Status:         keysResourceStatus,
		SourceEndpoint: "/api/v1/keys,/api/v1/admin/accounts",
		RecordCount:    len(snapshot.Keys),
		FailureReason:  keysFailureReason,
		Partial:        keysPartial,
	})
	if models := uniqueStrings(modelsFromKeys(snapshot.Keys)); len(models) > 0 {
		snapshot.Models = uniqueStrings(append(snapshot.Models, models...))
		modelsStatus := model.PlatformSiteResourceStatusSuccess
		modelsFailureReason := ""
		if !snapshot.KeysComplete {
			modelsStatus = model.PlatformSiteResourceStatusStale
			modelsFailureReason = "密钥资源未完整同步，保留最近成功模型快照"
		}
		snapshot.ResourceSyncs = append(snapshot.ResourceSyncs, PlatformSiteResourceSyncSnapshot{
			ResourceType:   model.PlatformSiteResourceModels,
			Status:         modelsStatus,
			SourceEndpoint: "/v1/models",
			RecordCount:    len(models),
			FailureReason:  modelsFailureReason,
		})
	} else {
		snapshot.ResourceSyncs = append(snapshot.ResourceSyncs, PlatformSiteResourceSyncSnapshot{
			ResourceType:   model.PlatformSiteResourceModels,
			Status:         model.PlatformSiteResourceStatusStale,
			SourceEndpoint: "/v1/models",
			FailureReason:  "密钥级模型目录接口不可用",
		})
	}
	return snapshot, nil
}

func (adapter *NewAPIAdapter) Cleanup(ctx context.Context, session *PlatformSiteSession) error {
	if session == nil || !session.PasswordSession {
		return nil
	}
	credentialUpdate := session.CredentialUpdate
	session.CredentialUpdate = nil
	defer func() {
		clearPlatformSitePasswordSession(session)
		session.CredentialUpdate = credentialUpdate
	}()

	token := strings.TrimSpace(session.TemporaryToken)
	sessionID := strings.TrimSpace(session.TemporarySession)
	if token == "" && sessionID == "" && strings.TrimSpace(session.TemporaryCookie) == "" {
		return fmt.Errorf("%w: NewAPI 本轮会话材料为空", ErrPlatformSiteSessionCleanup)
	}

	setNewAPIBrowserHeaders(session)
	session.Headers.Del("Cookie")
	session.Headers.Del("Authorization")
	session.Headers.Del("X-Auth-Session")
	if session.TemporaryCookie != "" {
		session.Headers.Set("Cookie", session.TemporaryCookie)
	}
	if token != "" {
		session.Headers.Set("Authorization", bearerToken(token))
	}
	if sessionID != "" {
		session.Headers.Set("X-Auth-Session", sessionID)
	}

	logoutErr := error(nil)
	_, logoutErr = platformSiteRequest(
		ctx,
		session,
		http.MethodPost,
		"/api/user/auth/logout",
		nil,
		nil,
	)

	shouldDeleteSID := sessionID != ""
	if logoutErr != nil {
		if !isIdempotentPlatformSiteCleanupError(logoutErr) {
			// 5xx 和网络错误仍继续尝试精确 SID 删除，但保留脱敏错误供
			// 同步层记录告警，不能影响已经读取的资源快照。
			if !shouldDeleteSID {
				return errors.Join(ErrPlatformSiteSessionCleanup, logoutErr)
			}
		}
	}

	if !shouldDeleteSID {
		if logoutErr != nil && !isIdempotentPlatformSiteCleanupError(logoutErr) {
			return errors.Join(ErrPlatformSiteSessionCleanup, logoutErr)
		}
		return nil
	}

	session.Headers.Del("Cookie")
	if session.TemporaryCookie != "" {
		session.Headers.Set("Cookie", session.TemporaryCookie)
	}
	if token != "" {
		session.Headers.Set("Authorization", bearerToken(token))
	} else {
		session.Headers.Del("Authorization")
	}
	session.Headers.Set("X-Auth-Session", sessionID)
	_, deleteErr := platformSiteRequest(
		ctx,
		session,
		http.MethodDelete,
		"/api/user/sessions/"+url.PathEscape(sessionID),
		nil,
		nil,
	)
	if deleteErr != nil {
		if !isIdempotentPlatformSiteCleanupError(deleteErr) {
			if logoutErr != nil {
				return errors.Join(
					ErrPlatformSiteSessionCleanup,
					logoutErr,
					deleteErr,
				)
			}
			return errors.Join(ErrPlatformSiteSessionCleanup, deleteErr)
		}
	}
	if logoutErr != nil && !isIdempotentPlatformSiteCleanupError(logoutErr) {
		return errors.Join(ErrPlatformSiteSessionCleanup, logoutErr)
	}
	return nil
}

func isIdempotentPlatformSiteCleanupError(err error) bool {
	if err == nil {
		return true
	}
	if platformSiteErrorCodeOf(err) == "AUTH_SESSION_MISMATCH" {
		return true
	}
	status, ok := platformSiteHTTPStatusCode(err)
	return ok && slices.Contains([]int{
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusMethodNotAllowed,
	}, status)
}

func (adapter *Sub2APIAdapter) Cleanup(ctx context.Context, session *PlatformSiteSession) error {
	if session == nil || !session.PasswordSession {
		return nil
	}
	credentialUpdate := session.CredentialUpdate
	session.CredentialUpdate = nil
	defer func() {
		clearPlatformSitePasswordSession(session)
		session.CredentialUpdate = credentialUpdate
	}()

	refreshToken := strings.TrimSpace(session.TemporaryRefresh)
	if refreshToken == "" {
		return fmt.Errorf(
			"%w: Sub2API 登录响应未返回 Refresh Token",
			ErrPlatformSiteSessionCleanup,
		)
	}

	session.Headers.Del("Authorization")
	session.Headers.Del("Cookie")
	session.Headers.Del("X-Auth-Session")
	setSub2APIBrowserHeaders(session)
	_, err := platformSiteRequest(
		ctx,
		session,
		http.MethodPost,
		"/api/v1/auth/logout",
		nil,
		map[string]string{"refresh_token": refreshToken},
	)
	if err != nil {
		return errors.Join(ErrPlatformSiteSessionCleanup, err)
	}
	return nil
}

func resetSub2APISessionBeforePasswordLogin(
	session *PlatformSiteSession,
	credential *model.PlatformSiteCredential,
) {
	if session == nil || credential == nil {
		return
	}
	clearPlatformSitePasswordSession(session)
	clearPlatformSiteTemporaryCredential(credential)
	credential.AuthType = model.UpstreamAuthPassword
	credential.RefreshStatus = ""
	credential.ReauthRequired = false
	credential.RefreshUncertain = false
}

func applySub2APIPasswordLoginPayload(
	session *PlatformSiteSession,
	credential *model.PlatformSiteCredential,
	payload any,
) error {
	if session == nil || credential == nil {
		return fmt.Errorf("%w: 登录会话不可用", ErrPlatformSiteAuth)
	}
	token := findToken(payload)
	if token == "" {
		return ErrSub2APILoginToken
	}
	tempCredential := model.PlatformSiteCredential{
		AuthType:       model.UpstreamAuthPassword,
		Username:       credential.Username,
		Password:       credential.Password,
		AccessToken:    token,
		RefreshToken:   findRefreshToken(payload),
		TokenType:      firstNonEmptyString(findTokenType(payload), "Bearer"),
		TokenExpiresAt: findTokenExpiresAt(payload),
		UserID:         findUserID(payload),
	}
	if sessionID := findSessionID(payload); sessionID != "" {
		tempCredential.SessionID = sessionID
		tempCredential.SessionCurrent = true
	}
	session.Headers.Set("Authorization", bearerToken(token))
	capturePlatformSiteSessionCookie(session, &tempCredential)
	setTemporaryPasswordSessionMaterials(session, tempCredential)
	credential.UserID = tempCredential.UserID
	clearPlatformSiteTemporaryCredential(credential)
	return nil
}

func loginNewAPIWithPassword(ctx context.Context, session *PlatformSiteSession, credential model.PlatformSiteCredential) (any, error) {
	identity := strings.TrimSpace(credential.Username)
	password := credential.Password
	if identity == "" || password == "" {
		return nil, fmt.Errorf("%w: 缺少账号密码", ErrPlatformSiteAuth)
	}

	encryptionConfig, encryptionErr := fetchNewAPIPasswordEncryptionConfig(ctx, session)
	if encryptionErr != nil && !platformSiteRouteMissing(encryptionErr) {
		return nil, fmt.Errorf("%w: NewAPI 密码加密配置读取失败: %w", ErrPlatformSiteAuth, encryptionErr)
	}
	encryptedPassword := ""
	encryptionKeyID := ""
	if encryptionErr == nil && encryptionConfig != nil && encryptionConfig.enabled {
		encryptedPassword, encryptionErr = encryptNewAPIPassword(password, encryptionConfig)
		if encryptionErr != nil {
			return nil, fmt.Errorf("%w: %w", ErrPlatformSiteAuth, encryptionErr)
		}
		encryptionKeyID = encryptionConfig.keyID
	}

	email := ""
	if strings.Contains(identity, "@") {
		email = identity
	}
	bodies := []map[string]string{{
		"username": identity,
	}}
	if email != "" {
		bodies = uniqueStringMaps(append(bodies,
			map[string]string{
				"email": email,
			},
			map[string]string{
				"username": identity,
				"email":    email,
			},
		))
	}
	for _, body := range bodies {
		if encryptedPassword != "" {
			body["password_encrypted"] = encryptedPassword
			body["encryption_key_id"] = encryptionKeyID
		} else {
			body["password"] = password
		}
	}
	var lastErr error
	for _, body := range bodies {
		payload, err := platformSiteRequest(
			ctx,
			session,
			http.MethodPost,
			"/api/user/login?turnstile=",
			nil,
			body,
		)
		if err == nil {
			return payload, nil
		}
		lastErr = err
		if !platformSiteLoginCredentialRetryAllowed(err) {
			return nil, err
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("%w: 登录失败", ErrPlatformSiteAuth)
	}
	return nil, lastErr
}

func loginSub2APIWithPassword(ctx context.Context, session *PlatformSiteSession, credential model.PlatformSiteCredential) (any, error) {
	identity := strings.TrimSpace(credential.Username)
	password := credential.Password
	if identity == "" || password == "" {
		return nil, fmt.Errorf("%w: 缺少账号密码", ErrPlatformSiteAuth)
	}

	email := ""
	if strings.Contains(identity, "@") {
		email = identity
	}
	bodies := []map[string]string{{"username": identity, "password": password}}
	if email != "" {
		bodies = []map[string]string{
			{"email": email, "password": password},
			{"username": identity, "password": password},
		}
	}
	loginPaths := []string{"/api/v1/auth/login", "/auth/login"}
	selectedPath := 0
	routeFallbackAllowed := true
	var lastErr error
	for _, body := range bodies {
		for {
			payload, requestErr := platformSiteRequest(
				ctx,
				session,
				http.MethodPost,
				loginPaths[selectedPath],
				nil,
				body,
			)
			if requestErr == nil {
				return payload, nil
			}
			lastErr = classifySub2APILoginError(requestErr)
			if platformSiteLoginAgreementRequired(lastErr) {
				agreedRevision := sub2APILoginAgreementRevision(ctx, session)
				if agreedRevision == "" {
					return nil, lastErr
				}
				agreedBody := make(map[string]string, len(body)+1)
				maps.Copy(agreedBody, body)
				agreedBody["agreed_revision"] = agreedRevision
				agreedPayload, agreedErr := platformSiteRequest(
					ctx,
					session,
					http.MethodPost,
					loginPaths[selectedPath],
					nil,
					agreedBody,
				)
				if agreedErr == nil {
					return agreedPayload, nil
				}
				return nil, classifySub2APILoginError(agreedErr)
			}
			if platformSiteRouteMissing(lastErr) &&
				routeFallbackAllowed &&
				selectedPath+1 < len(loginPaths) {
				selectedPath++
				routeFallbackAllowed = false
				continue
			}
			routeFallbackAllowed = false
			if sub2APILoginCredentialRetryAllowed(lastErr) {
				break
			}
			return nil, lastErr
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("%w: 登录失败", ErrSub2APILoginRequest)
	}
	return nil, lastErr
}

func sub2APILoginAgreementRevision(ctx context.Context, session *PlatformSiteSession) string {
	if session == nil || session.Platform != model.PlatformSub2API {
		return ""
	}
	payload, err := platformSiteRequest(
		ctx,
		session,
		http.MethodGet,
		"/api/v1/settings/public",
		nil,
		nil,
	)
	if err != nil {
		return ""
	}
	record := firstRecord(payload)
	enabled, ok := record["login_agreement_enabled"].(bool)
	if !ok || !enabled {
		return ""
	}
	revision, ok := record["login_agreement_revision"].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(revision)
}

func platformSiteLoginCredentialRetryAllowed(err error) bool {
	if err == nil ||
		platformSiteLoginAgreementRequired(err) ||
		!errors.Is(err, ErrPlatformSiteCredentials) ||
		errors.Is(err, ErrPlatformSiteSecurity) ||
		errors.Is(err, ErrPlatformSitePermission) ||
		platformSiteSessionLimit(err) {
		return false
	}
	var statusErr *platformSiteHTTPStatusError
	if errors.As(err, &statusErr) && statusErr.statusCode != http.StatusUnauthorized {
		return false
	}
	return true
}

func sub2APILoginCredentialRetryAllowed(err error) bool {
	if !platformSiteLoginCredentialRetryAllowed(err) {
		return false
	}
	statusCode, hasStatus := platformSiteHTTPStatusCode(err)
	return hasStatus && statusCode == http.StatusUnauthorized
}

func classifySub2APILoginError(err error) error {
	if err == nil {
		return nil
	}
	if platformSiteLoginAgreementRequired(err) {
		return errors.Join(
			ErrSub2APILoginAgreement,
			ErrSub2APILoginHTTPStatus,
			err,
		)
	}
	if platformSiteInteractiveVerificationRequired(err) {
		return errors.Join(
			ErrSub2APILoginInteractive,
			ErrPlatformSiteSecurity,
			fmt.Errorf("%w: %w", ErrSub2APILoginHTTPStatus, err),
		)
	}
	if errors.Is(err, ErrPlatformSiteCredentials) {
		return errors.Join(ErrPlatformSiteCredentials, ErrSub2APILoginHTTPStatus, err)
	}
	if errors.Is(err, ErrPlatformSiteHTTPStatus) {
		return fmt.Errorf("%w: %w", ErrSub2APILoginHTTPStatus, err)
	}
	if errors.Is(err, ErrPlatformSiteResponse) {
		return fmt.Errorf("%w: %w", ErrSub2APILoginResponse, err)
	}
	if errors.Is(err, ErrPlatformSiteAuth) {
		return fmt.Errorf("%w: %w", ErrSub2APILoginHTTPStatus, err)
	}
	return fmt.Errorf("%w: %w", ErrSub2APILoginRequest, err)
}

func setSub2APIBrowserHeaders(session *PlatformSiteSession) {
	if session == nil {
		return
	}
	parsed, err := url.Parse(session.BaseURL)
	if err != nil {
		return
	}
	origin := parsed.Scheme + "://" + parsed.Host
	session.Headers.Set("Origin", origin)
	session.Headers.Set("Referer", origin+"/login")
	session.Headers.Set("User-Agent", "NexusTok-UpstreamSite/1.0")
	session.Headers.Set("X-Requested-With", "XMLHttpRequest")
}

func setNewAPIBrowserHeaders(session *PlatformSiteSession) {
	if session == nil {
		return
	}
	parsed, err := url.Parse(session.BaseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return
	}
	origin := parsed.Scheme + "://" + parsed.Host
	referer := strings.TrimRight(session.BaseURL, "/") + "/login"
	session.Headers.Set("Origin", origin)
	session.Headers.Set("Referer", referer)
	session.Headers.Set("User-Agent", "NexusTok-UpstreamSite/1.0")
	session.Headers.Set("X-Requested-With", "XMLHttpRequest")
}

func fetchNewAPICurrentUser(ctx context.Context, session *PlatformSiteSession) (any, error) {
	var lastErr error
	for _, path := range []string{"/api/user/self", "/api/user/me", "/api/user/profile", "/api/user/info"} {
		payload, err := platformSiteRequest(ctx, session, http.MethodGet, path, nil, nil)
		if err == nil {
			return payload, nil
		}
		lastErr = err
		if !platformSiteRouteMissing(err) {
			break
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("%w: NewAPI 当前用户接口不可用", ErrPlatformSiteAuth)
	}
	return nil, lastErr
}

func newAPIResourceSyncFailure(
	resourceType string,
	sourceEndpoint string,
	err error,
	partial bool,
) PlatformSiteResourceSyncSnapshot {
	status := model.PlatformSiteResourceStatusFailed
	requiresSecurityVerification := false
	if platformSiteErrorCategoryOf(err) == platformSiteErrorCategoryRouteMissing {
		status = model.PlatformSiteResourceStatusStale
	}
	if errors.Is(err, ErrPlatformSiteSecurity) {
		status = model.PlatformSiteResourceStatusSecureVerificationRequired
		requiresSecurityVerification = true
	}
	return PlatformSiteResourceSyncSnapshot{
		ResourceType:                 resourceType,
		Status:                       status,
		SourceEndpoint:               sourceEndpoint,
		FailureReason:                platformSiteResourceFailureReason(err),
		Partial:                      partial,
		RequiresSecurityVerification: requiresSecurityVerification,
	}
}

func sub2APIResourceSyncFailure(
	resourceType string,
	sourceEndpoint string,
	err error,
	partial bool,
) PlatformSiteResourceSyncSnapshot {
	status := model.PlatformSiteResourceStatusFailed
	if platformSiteRouteMissing(err) {
		status = model.PlatformSiteResourceStatusStale
	}
	requiresSecurityVerification := errors.Is(err, ErrPlatformSiteSecurity)
	if requiresSecurityVerification {
		status = model.PlatformSiteResourceStatusSecureVerificationRequired
	}
	return PlatformSiteResourceSyncSnapshot{
		ResourceType:                 resourceType,
		Status:                       status,
		SourceEndpoint:               sourceEndpoint,
		FailureReason:                platformSiteResourceFailureReason(err),
		Partial:                      partial,
		RequiresSecurityVerification: requiresSecurityVerification,
	}
}

func platformSiteResourceFailureReason(err error) string {
	if err == nil {
		return ""
	}
	diagnostics := platformSiteResponseDiagnosticSuffix(err) + platformSiteStageDiagnosticSuffix(err)
	switch {
	case errors.Is(err, ErrPlatformSiteSecurity):
		return "上游平台资源需要完成安全验证" + diagnostics
	case errors.Is(err, ErrPlatformSitePermission):
		return "上游平台资源权限不足" + diagnostics
	case platformSiteRouteMissing(err):
		return "上游平台资源路由不存在" + diagnostics
	case errors.Is(err, ErrPlatformSiteTransport):
		return "上游平台资源网络请求失败" + diagnostics
	case errors.Is(err, ErrPlatformSiteCredentials):
		return "上游平台资源会话已失效" + diagnostics
	case errors.Is(err, ErrPlatformSiteResponse):
		return "上游平台资源响应无效" + diagnostics
	default:
		return "上游平台资源同步失败" + diagnostics
	}
}

func fetchNewAPIAdminChannels(
	ctx context.Context,
	session *PlatformSiteSession,
) ([]map[string]any, error) {
	channels := make([]map[string]any, 0)
	for page := 1; page <= upstreamSiteMaxPages; page++ {
		payload, err := platformSiteRequest(ctx, session, http.MethodGet, "/api/channel/", url.Values{
			"p":         {fmt.Sprint(page)},
			"page_size": {fmt.Sprint(upstreamSitePageSize)},
		}, nil)
		if err != nil {
			return nil, err
		}
		items := recordsFromPayload(payload)
		channels = append(channels, items...)
		if !platformSitePageHasMore(payload, page, len(items), upstreamSiteMaxPages) {
			return channels, nil
		}
	}
	return nil, errors.New("NewAPI 管理渠道分页超过安全上限")
}

func fetchNewAPIAdminChannelModels(
	ctx context.Context,
	session *PlatformSiteSession,
	channel map[string]any,
) ([]string, error) {
	models := modelsFromRecord(channel)
	channelID := firstString(channel, "id", "channel_id", "channelId")
	if channelID == "" {
		return uniqueStrings(models), nil
	}
	payload, err := platformSiteRequest(
		ctx,
		session,
		http.MethodGet,
		"/api/channel/"+url.PathEscape(channelID),
		nil,
		nil,
	)
	if err == nil {
		models = append(models, modelsFromRecord(firstRecord(payload))...)
	} else if !platformSiteRouteMissing(err) {
		return uniqueStrings(models), err
	}
	payload, err = platformSiteRequest(
		ctx,
		session,
		http.MethodGet,
		"/api/channel/fetch_models/"+url.PathEscape(channelID),
		nil,
		nil,
	)
	if err == nil {
		models = append(models, stringsFromPayload(payload)...)
		return uniqueStrings(models), nil
	}
	if platformSiteRouteMissing(err) {
		return uniqueStrings(models), nil
	}
	return uniqueStrings(models), err
}

func fetchSub2APICurrentUser(ctx context.Context, session *PlatformSiteSession) (any, error) {
	var lastErr error
	for _, path := range []string{"/api/v1/auth/me", "/api/v1/user/profile"} {
		payload, err := platformSiteRequest(ctx, session, http.MethodGet, path, nil, nil)
		if err == nil {
			return payload, nil
		}
		lastErr = err
		if !platformSiteRouteMissing(err) {
			break
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("%w: Sub2API 当前用户接口不可用", ErrPlatformSiteAuth)
	}
	return nil, errors.Join(ErrSub2APICurrentUser, lastErr)
}

func bearerToken(token string) string {
	return "Bearer " + strings.TrimSpace(token)
}

func refreshPlatformSiteSession(
	ctx context.Context,
	session *PlatformSiteSession,
	path string,
	credential *model.PlatformSiteCredential,
) error {
	isSub2API := path == "/api/v1/auth/refresh"
	isDashboardRefresh := path == "/api/user/auth/refresh"
	if credential == nil {
		return fmt.Errorf("%w: 缺少刷新凭据", ErrPlatformSiteAuth)
	}
	if session == nil || session.Headers == nil {
		return fmt.Errorf("%w: 平台站点会话不可用", ErrPlatformSiteAuth)
	}
	if isSub2API && strings.TrimSpace(credential.RefreshToken) == "" {
		return fmt.Errorf("%w: 缺少刷新令牌", ErrPlatformSiteAuth)
	}
	if isDashboardRefresh {
		captureNewAPIRefreshCookie(session, credential)
		if !hasNewAPIRefreshCookie(credential.Cookie) ||
			strings.TrimSpace(credential.AccessToken) == "" ||
			strings.TrimSpace(credential.SessionID) == "" {
			return errors.Join(
				ErrPlatformSiteAuth,
				ErrPlatformSiteRefreshUncertain,
				errors.New("缺少 Dashboard Refresh 会话材料"),
			)
		}
	}
	if strings.TrimSpace(credential.Cookie) != "" {
		session.Headers.Set("Cookie", credential.Cookie)
	}
	if strings.TrimSpace(credential.AccessToken) != "" {
		session.Headers.Set("Authorization", bearerToken(credential.AccessToken))
	}
	if isDashboardRefresh {
		setNewAPIBrowserHeaders(session)
	}
	if isDashboardRefresh && strings.TrimSpace(credential.SessionID) != "" {
		session.Headers.Set("X-Auth-Session", credential.SessionID)
	}
	previousRefreshCookie := newAPIRefreshCookieValue(credential.Cookie)
	var body any
	if !isDashboardRefresh {
		body = map[string]string{"refresh_token": credential.RefreshToken}
	}
	payload, err := platformSiteRequest(ctx, session, http.MethodPost, path, nil, body)
	if err != nil {
		capturePlatformSiteSessionCookie(session, credential)
		captureNewAPIRefreshCookie(session, credential)
		syncNewAPISessionHeaders(session, *credential)
		uncertain := true
		if isSub2API {
			uncertain = !platformSiteHTTPStatusIsUnauthorized(err)
		} else if newAPIPasswordFallbackAllowed(err) {
			uncertain = false
		}
		markPlatformSiteRefreshFailure(credential, uncertain)
		setPlatformSiteCredentialUpdate(session, *credential)
		refreshErr := fmt.Errorf("%w: 刷新会话失败", ErrPlatformSiteAuth)
		if uncertain {
			return errors.Join(refreshErr, ErrPlatformSiteRefreshUncertain, err)
		}
		return errors.Join(refreshErr, err)
	}
	if isDashboardRefresh {
		recognized, bundleErr := applyNewAPIDashboardAuthBundle(
			payload,
			credential,
			credential.AuthType == model.UpstreamAuthPassword,
		)
		if recognized {
			if bundleErr != nil {
				markPlatformSiteRefreshFailure(credential, true)
				setPlatformSiteCredentialUpdate(session, *credential)
				return errors.Join(bundleErr, ErrPlatformSiteRefreshUncertain)
			}
			capturePlatformSiteSessionCookie(session, credential)
			captureNewAPIRefreshCookie(session, credential)
			currentRefreshCookie := newAPIRefreshCookieValue(credential.Cookie)
			if currentRefreshCookie == "" ||
				(previousRefreshCookie != "" && currentRefreshCookie == previousRefreshCookie) {
				markPlatformSiteRefreshFailure(credential, true)
				setPlatformSiteCredentialUpdate(session, *credential)
				return errors.Join(
					ErrPlatformSiteRefreshUncertain,
					fmt.Errorf("%w: 刷新响应未返回轮换后的 Refresh Cookie", ErrPlatformSiteAuth),
				)
			}
			syncNewAPISessionHeaders(session, *credential)
			session.Headers.Set("Authorization", bearerToken(credential.AccessToken))
			setNewAPICompatUserHeaders(session.Headers, credential.UserID)
			updatedCredential := *credential
			session.CredentialUpdate = &updatedCredential
			return nil
		}
	}
	accessToken := findToken(payload)
	refreshToken := findRefreshToken(payload)
	if isSub2API && (accessToken == "" || refreshToken == "" || findTokenExpiresAt(payload) <= common.GetTimestamp()) {
		markPlatformSiteRefreshFailure(credential, true)
		setPlatformSiteCredentialUpdate(session, *credential)
		return fmt.Errorf("%w: 刷新会话响应不完整", ErrPlatformSiteAuth)
	}
	if !isSub2API && (accessToken == "" || refreshToken == "") {
		markPlatformSiteRefreshFailure(credential, true)
		setPlatformSiteCredentialUpdate(session, *credential)
		return errors.Join(
			fmt.Errorf("%w: 刷新会话响应不完整", ErrPlatformSiteAuth),
			ErrPlatformSiteRefreshUncertain,
		)
	}
	credential.AccessToken = accessToken
	credential.RefreshToken = refreshToken
	credential.TokenExpiresAt = findTokenExpiresAt(payload)
	credential.TokenType = findTokenType(payload)
	if credential.TokenType == "" {
		credential.TokenType = "Bearer"
	}
	credential.LastAuthAt = common.GetTimestamp()
	credential.RefreshStatus = "active"
	credential.ReauthRequired = false
	credential.RefreshUncertain = false
	if userID := findUserID(payload); userID != "" {
		credential.UserID = userID
	}
	if sessionID := findSessionID(payload); sessionID != "" {
		credential.SessionID = sessionID
		credential.SessionCurrent = true
	}
	capturePlatformSiteSessionCookie(session, credential)
	captureNewAPIRefreshCookie(session, credential)
	syncNewAPISessionHeaders(session, *credential)
	session.Headers.Set("Authorization", bearerToken(accessToken))
	setNewAPICompatUserHeaders(session.Headers, credential.UserID)
	updatedCredential := *credential
	session.CredentialUpdate = &updatedCredential
	return nil
}

func setPlatformSiteCredentialUpdate(session *PlatformSiteSession, credential model.PlatformSiteCredential) {
	if session == nil {
		return
	}
	updatedCredential := credential
	session.CredentialUpdate = &updatedCredential
}

func setTemporaryPasswordSessionMaterials(
	session *PlatformSiteSession,
	credential model.PlatformSiteCredential,
) {
	if session == nil {
		return
	}
	session.TemporaryToken = strings.TrimSpace(credential.AccessToken)
	session.TemporaryRefresh = strings.TrimSpace(credential.RefreshToken)
	session.TemporarySession = strings.TrimSpace(credential.SessionID)
	session.TemporaryCookie = mergePlatformSiteCookieHeaders(
		credential.Cookie,
		platformSiteJarCookieHeader(session),
	)
	session.PasswordSession = session.TemporaryToken != "" ||
		session.TemporaryRefresh != "" ||
		session.TemporarySession != "" ||
		session.TemporaryCookie != ""
}

func setPasswordSessionMaterialsFromPayload(
	session *PlatformSiteSession,
	payload any,
) {
	if session == nil {
		return
	}
	setTemporaryPasswordSessionMaterials(session, model.PlatformSiteCredential{
		AccessToken:    findToken(payload),
		RefreshToken:   findRefreshToken(payload),
		TokenType:      firstNonEmptyString(findTokenType(payload), "Bearer"),
		TokenExpiresAt: findTokenExpiresAt(payload),
		SessionID:      findSessionID(payload),
		Cookie:         platformSiteJarCookieHeader(session),
	})
}

func clearPlatformSiteTemporaryCredential(credential *model.PlatformSiteCredential) {
	if credential == nil {
		return
	}
	credential.AccessToken = ""
	credential.RefreshToken = ""
	credential.TokenExpiresAt = 0
	credential.TokenType = ""
	credential.SessionID = ""
	credential.SessionCurrent = false
	credential.AdminKey = ""
	credential.Cookie = ""
}

func clearPlatformSiteTemporarySession(session *PlatformSiteSession) {
	if session == nil {
		return
	}
	session.Headers.Del("Authorization")
	session.Headers.Del("Cookie")
	session.Headers.Del("X-Auth-Session")
	clearNewAPICompatUserHeaders(session.Headers)
	session.PasswordSession = false
	session.TemporaryToken = ""
	session.TemporaryRefresh = ""
	session.TemporarySession = ""
	session.TemporaryCookie = ""
}

func clearPlatformSitePasswordSession(session *PlatformSiteSession) {
	if session == nil {
		return
	}
	clearPlatformSiteTemporarySession(session)
	if session.Client == nil {
		return
	}
	jar, err := cookiejar.New(nil)
	if err == nil {
		session.Client.Jar = jar
	}
}

func clearNewAPICompatUserHeaders(headers http.Header) {
	if headers == nil {
		return
	}
	for _, name := range []string{
		"New-API-User",
		"X-ModelFlare-User",
		"Veloera-User",
		"X-Api-User",
		"voapi-user",
		"User-id",
		"Rix-Api-User",
		"neo-api-user",
	} {
		headers.Del(name)
	}
}

func capturePlatformSiteCredentialCookie(session *PlatformSiteSession, rawURL string) {
	if session == nil || session.CredentialUpdate == nil {
		return
	}
	capturePlatformSiteSessionCookie(session, session.CredentialUpdate, rawURL)
	captureNewAPIRefreshCookie(session, session.CredentialUpdate)
}

func syncNewAPISessionHeaders(session *PlatformSiteSession, credential model.PlatformSiteCredential) {
	if session == nil {
		return
	}
	if strings.TrimSpace(credential.SessionID) != "" {
		session.Headers.Set("X-Auth-Session", credential.SessionID)
	} else {
		session.Headers.Del("X-Auth-Session")
	}
	jarCookie := platformSiteJarCookieHeader(session)
	mergedCookie := mergePlatformSiteCookieHeaders(credential.Cookie, jarCookie)
	if mergedCookie != "" {
		session.Headers.Set("Cookie", mergedCookie)
	} else {
		session.Headers.Del("Cookie")
	}
}

func platformSiteJarCookieHeader(session *PlatformSiteSession) string {
	if session == nil {
		return ""
	}
	return platformSiteJarCookieHeaderForURL(session, session.BaseURL)
}

func platformSiteJarCookieHeaderForURL(session *PlatformSiteSession, rawURL string) string {
	if session == nil || session.Client == nil || session.Client.Jar == nil {
		return ""
	}
	if strings.TrimSpace(rawURL) == "" {
		rawURL = session.BaseURL
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	cookies := session.Client.Jar.Cookies(parsed)
	return platformSiteCookieHeaderFromCookies(cookies)
}

func platformSiteCookieHeaderFromCookies(cookies []*http.Cookie) string {
	parts := make([]string, 0, len(cookies))
	for _, cookie := range cookies {
		if cookie == nil || strings.TrimSpace(cookie.Name) == "" {
			continue
		}
		parts = append(parts, cookie.Name+"="+cookie.Value)
	}
	return strings.Join(parts, "; ")
}

func platformSiteRequestCookieHeader(session *PlatformSiteSession, rawURL string) string {
	if session == nil {
		return ""
	}
	jarCookie := platformSiteJarCookieHeaderForURL(session, rawURL)
	if jarCookie == "" {
		return mergePlatformSiteCookieHeaders(session.Headers.Get("Cookie"))
	}
	jarNames := make(map[string]struct{})
	for _, part := range strings.Split(jarCookie, ";") {
		name, _, found := strings.Cut(strings.TrimSpace(part), "=")
		name = strings.TrimSpace(name)
		if found && name != "" {
			jarNames[strings.ToLower(name)] = struct{}{}
		}
	}
	explicitCookie := mergePlatformSiteCookieHeaders(session.Headers.Get("Cookie"))
	parts := make([]string, 0, len(strings.Split(jarCookie, ";"))+len(strings.Split(explicitCookie, ";")))
	if jarCookie != "" {
		parts = append(parts, jarCookie)
	}
	for _, part := range strings.Split(explicitCookie, ";") {
		name, value, found := strings.Cut(strings.TrimSpace(part), "=")
		name = strings.TrimSpace(name)
		if !found || name == "" {
			continue
		}
		if _, exists := jarNames[strings.ToLower(name)]; exists {
			continue
		}
		parts = append(parts, name+"="+strings.TrimSpace(value))
	}
	return strings.Join(parts, "; ")
}

func mergePlatformSiteCookieHeaders(values ...string) string {
	cookies := make(map[string]string)
	order := make([]string, 0)
	for _, header := range values {
		for _, part := range strings.Split(header, ";") {
			name, value, found := strings.Cut(strings.TrimSpace(part), "=")
			name = strings.TrimSpace(name)
			if !found || name == "" {
				continue
			}
			canonicalName := strings.ToLower(name)
			if _, exists := cookies[canonicalName]; !exists {
				order = append(order, canonicalName)
			}
			cookies[canonicalName] = name + "=" + strings.TrimSpace(value)
		}
	}
	parts := make([]string, 0, len(order))
	for _, canonicalName := range order {
		parts = append(parts, cookies[canonicalName])
	}
	return strings.Join(parts, "; ")
}

func captureNewAPIRefreshCookie(session *PlatformSiteSession, credential *model.PlatformSiteCredential) {
	if session == nil || credential == nil {
		return
	}
	refreshURL, err := upstreamSiteURL(session.BaseURL, "/api/user/auth/refresh", nil)
	if err != nil {
		return
	}
	jarCookie := platformSiteJarCookieHeaderForURL(session, refreshURL)
	mergedCookie := mergePlatformSiteCookieHeaders(credential.Cookie, jarCookie)
	if mergedCookie != "" {
		credential.Cookie = mergedCookie
	}
}

func markPlatformSiteRefreshFailure(credential *model.PlatformSiteCredential, uncertain bool) {
	if credential == nil {
		return
	}
	credential.ReauthRequired = true
	credential.RefreshUncertain = uncertain
	if uncertain {
		credential.RefreshStatus = "uncertain_rotation"
	} else {
		credential.RefreshStatus = "reauth_required"
	}
}

func platformSiteHTTPStatusIsUnauthorized(err error) bool {
	status, ok := platformSiteHTTPStatusCode(err)
	return ok && status == http.StatusUnauthorized
}

func applyNewAPIDashboardAuthBundle(
	payload any,
	credential *model.PlatformSiteCredential,
	preservePassword bool,
) (bool, error) {
	record, ok := payload.(map[string]any)
	if !ok {
		return false, nil
	}
	data, ok := record["data"].(map[string]any)
	if !ok {
		return false, nil
	}
	session, sessionOK := data["session"].(map[string]any)
	_, sessionFieldPresent := data["session"]
	recognized := hasAnyField(data, "access_token", "token_type", "access_expires_at") ||
		sessionFieldPresent
	if !recognized {
		return false, nil
	}
	success, successOK := record["success"].(bool)
	token := firstString(data, "access_token")
	tokenType := firstString(data, "token_type")
	expiresAt, expiresAtOK := upstreamFloatValue(data["access_expires_at"])
	sessionID := firstString(session, "sid")
	sessionCurrent, currentOK := session["current"].(bool)
	user, userOK := data["user"].(map[string]any)
	if !successOK || !success ||
		token == "" ||
		tokenType != "Bearer" ||
		!expiresAtOK ||
		expiresAt <= float64(common.GetTimestamp()) ||
		sessionID == "" ||
		!sessionOK ||
		!currentOK ||
		!sessionCurrent ||
		!userOK ||
		findUserID(user) == "" {
		return true, ErrPlatformSiteAuthBundle
	}
	if credential == nil {
		return true, ErrPlatformSiteAuthBundle
	}
	bundleUserID := findUserID(user)
	if credential.UserID != "" && credential.UserID != bundleUserID {
		return true, ErrPlatformSiteIdentity
	}
	if !preservePassword {
		credential.AuthType = model.UpstreamAuthAccessToken
		credential.Username = ""
		credential.Password = ""
	}
	credential.UserID = bundleUserID
	credential.AccessToken = token
	credential.RefreshToken = ""
	credential.TokenType = tokenType
	credential.TokenExpiresAt = upstreamInt64Value(expiresAt)
	credential.SessionID = sessionID
	credential.SessionCurrent = true
	credential.LastAuthAt = common.GetTimestamp()
	credential.RefreshStatus = "active"
	credential.ReauthRequired = false
	credential.RefreshUncertain = false
	return true, nil
}

func findToken(payload any) string {
	record := firstRecord(payload)
	if token := firstString(record, "access_token", "accessToken", "auth_token", "authToken", "id_token", "idToken", "token", "jwt"); token != "" {
		return token
	}
	for _, key := range []string{"data", "result", "auth", "session", "user", "account", "auth_bundle"} {
		if nested, ok := record[key]; ok {
			if token := findToken(nested); token != "" {
				return token
			}
		}
	}
	return ""
}

func findRefreshToken(payload any) string {
	record := firstRecord(payload)
	if token := firstString(record, "refresh_token", "refreshToken", "refresh"); token != "" {
		return token
	}
	for _, key := range []string{"data", "result", "auth_bundle"} {
		if nested, ok := record[key]; ok {
			if token := findRefreshToken(nested); token != "" {
				return token
			}
		}
	}
	return ""
}

func findTokenType(payload any) string {
	record := firstRecord(payload)
	if tokenType := firstString(record, "token_type", "tokenType"); tokenType != "" {
		return tokenType
	}
	for _, key := range []string{"data", "result", "auth", "session", "auth_bundle"} {
		if nested, ok := record[key]; ok {
			if tokenType := findTokenType(nested); tokenType != "" {
				return tokenType
			}
		}
	}
	return ""
}

func findUserID(payload any) string {
	record := firstRecord(payload)
	if userID := firstString(record, "id", "user_id", "userId", "uid"); userID != "" {
		return userID
	}
	for _, key := range []string{"data", "result", "user", "account", "profile", "auth"} {
		if nested, ok := record[key]; ok {
			if userID := findUserID(nested); userID != "" {
				return userID
			}
		}
	}
	return ""
}

func setNewAPICompatUserHeaders(headers http.Header, userID string) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return
	}
	for _, name := range []string{
		"New-API-User",
		"X-ModelFlare-User",
		"Veloera-User",
		"X-Api-User",
		"voapi-user",
		"User-id",
		"Rix-Api-User",
		"neo-api-user",
	} {
		headers.Set(name, userID)
	}
}

func findTokenExpiresAt(payload any) int64 {
	record := firstRecord(payload)
	for _, key := range []string{"access_expires_at", "token_expires_at", "expires_at"} {
		value := firstFloat(record, key)
		if value > 0 && !math.IsInf(value, 0) && !math.IsNaN(value) {
			return int64(value)
		}
	}
	if expiresIn := firstFloat(record, "expires_in"); expiresIn > 0 &&
		!math.IsInf(expiresIn, 0) && !math.IsNaN(expiresIn) {
		return time.Now().Add(time.Duration(expiresIn) * time.Second).Unix()
	}
	for _, key := range []string{"data", "result", "auth_bundle"} {
		if nested, ok := record[key]; ok {
			if value := findTokenExpiresAt(nested); value > 0 {
				return value
			}
		}
	}
	return 0
}

func loginRequiresInteractiveVerification(payload any) bool {
	switch value := payload.(type) {
	case map[string]any:
		for _, key := range []string{
			"require_2fa",
			"requires_2fa",
			"verification_required",
			"requires_verification",
		} {
			if required, ok := value[key].(bool); ok && required {
				return true
			}
		}
		for _, key := range []string{"flow_token", "verification_token"} {
			if token, ok := value[key].(string); ok && strings.TrimSpace(token) != "" {
				return true
			}
		}
		for _, key := range []string{"data", "auth_bundle", "result"} {
			if nested, ok := value[key]; ok && loginRequiresInteractiveVerification(nested) {
				return true
			}
		}
	case []any:
		for _, item := range value {
			if loginRequiresInteractiveVerification(item) {
				return true
			}
		}
	}
	return false
}

func hasAnyField(record map[string]any, keys ...string) bool {
	for _, key := range keys {
		if _, exists := record[key]; exists {
			return true
		}
	}
	return false
}

func isUpstreamKeyDisabled(record map[string]any) bool {
	for _, key := range []string{"disabled", "revoked", "suspended"} {
		if disabled, ok := record[key].(bool); ok && disabled {
			return true
		}
	}
	if enabled, ok := record["enabled"].(bool); ok && !enabled {
		return true
	}
	for _, key := range []string{"status", "state"} {
		status := strings.ToLower(strings.TrimSpace(firstString(record, key)))
		switch status {
		case "2", "3", "disabled", "inactive", "revoked", "suspended", "expired", "error", "deleted", "quota_exhausted":
			return true
		}
	}
	return false
}

func firstRecord(payload any) map[string]any {
	payload = unwrapPlatformData(payload)
	if record, ok := payload.(map[string]any); ok {
		return record
	}
	return map[string]any{}
}

func firstNestedRecord(payload any, nestedKeys ...string) map[string]any {
	record := firstRecord(payload)
	for _, key := range nestedKeys {
		nested, ok := record[key]
		if !ok {
			continue
		}
		nestedRecord := firstRecord(nested)
		if len(nestedRecord) == 0 {
			continue
		}
		merged := make(map[string]any, len(record)+len(nestedRecord))
		maps.Copy(merged, record)
		maps.Copy(merged, nestedRecord)
		return merged
	}
	return record
}

func stringFromPayload(payload any) string {
	payload = unwrapPlatformData(payload)
	if value, ok := payload.(string); ok {
		return strings.TrimSpace(value)
	}
	return firstString(firstRecord(payload), "key", "token", "api_key", "value")
}

func stringsFromPayload(payload any) []string {
	payload = unwrapPlatformData(payload)
	switch value := payload.(type) {
	case string:
		parts := strings.FieldsFunc(value, func(r rune) bool {
			return r == ',' || r == '\n'
		})
		return uniqueStrings(parts)
	case []any:
		result := make([]string, 0, len(value))
		for _, item := range value {
			if text, ok := item.(string); ok {
				result = append(result, text)
				continue
			}
			if record, ok := item.(map[string]any); ok {
				if text := firstString(record, "id", "name", "model"); text != "" {
					result = append(result, text)
				}
			}
		}
		return uniqueStrings(result)
	case map[string]any:
		for _, key := range []string{"data", "items", "models", "list"} {
			if nested, ok := value[key]; ok {
				return stringsFromPayload(nested)
			}
		}
	}
	return nil
}

func fetchNewAPIQuotaPerUnit(ctx context.Context, session *PlatformSiteSession) (float64, error) {
	payload, err := platformSiteRequest(ctx, session, http.MethodGet, "/api/status", nil, nil)
	if err != nil {
		return defaultNewAPIQuotaPerUnit, err
	}
	quotaPerUnit := firstFloat(firstRecord(payload), "quota_per_unit", "quotaPerUnit")
	if quotaPerUnit <= 0 || math.IsNaN(quotaPerUnit) || math.IsInf(quotaPerUnit, 0) {
		return defaultNewAPIQuotaPerUnit, fmt.Errorf("%w: NewAPI 额度单位无效", ErrPlatformSiteResponse)
	}
	return quotaPerUnit, nil
}

func normalizeNewAPIQuota(value float64, quotaPerUnit float64) float64 {
	if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	if quotaPerUnit <= 1 || math.IsNaN(quotaPerUnit) || math.IsInf(quotaPerUnit, 0) {
		return value
	}
	return value / quotaPerUnit
}

func fetchNewAPIGroupRates(ctx context.Context, session *PlatformSiteSession) map[string]float64 {
	for _, path := range []string{"/api/user/self/groups", "/api/user/groups"} {
		payload, err := platformSiteRequest(ctx, session, http.MethodGet, path, nil, nil)
		if err != nil {
			if platformSiteRouteMissing(err) {
				continue
			}
			break
		}
		if rates := parseGroupRates(payload); len(rates) > 0 {
			return rates
		}
	}
	return map[string]float64{}
}

func platformSiteIdentityFromRecord(
	record map[string]any,
	quotaUnit string,
	sourceEndpoint string,
) *PlatformSiteIdentitySnapshot {
	identity := &PlatformSiteIdentitySnapshot{
		PlatformUserID: firstString(record, "id", "user_id", "userId", "uid"),
		Username:       platformSiteUsernameFromRecord(record),
		Email:          firstString(record, "email", "mail"),
		DisplayName:    firstString(record, "display_name", "displayName", "nickname", "name"),
		Role:           firstString(record, "role", "role_name", "roleName"),
		CurrentGroup:   firstString(record, "group", "group_name", "groupName", "current_group", "currentGroup"),
		Status:         firstString(record, "status", "state", "account_status", "accountStatus"),
		QuotaUnit: firstNonEmptyString(
			firstString(record, "quota_unit", "quotaUnit", "unit"),
			quotaUnit,
		),
		UpstreamUpdatedAt: firstInt64(record, "updated_at", "updatedAt", "last_updated_at", "lastUpdatedAt"),
		SourceEndpoint:    sourceEndpoint,
	}
	return identity
}

func platformSiteUsernameFromRecord(record map[string]any) string {
	if record == nil {
		return ""
	}
	return firstNonEmptyString(
		firstString(record, "username", "user_name", "login"),
		firstString(record, "email", "mail"),
	)
}

func platformSiteEndpointURL(baseURL, path string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	path = "/" + strings.TrimLeft(strings.TrimSpace(path), "/")
	if baseURL == "" {
		return ""
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return baseURL + path
	}
	basePath := strings.TrimRight(parsed.Path, "/")
	if strings.HasSuffix(basePath, "/v1") && strings.HasPrefix(path, "/v1/") {
		path = strings.TrimPrefix(path, "/v1")
	}
	parsed.Path = strings.TrimRight(basePath, "/") + path
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/")
}

func platformSiteModelsURL(baseURL string) string {
	normalized := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if normalized == "" {
		return ""
	}
	if parsed, err := url.Parse(normalized); err == nil &&
		strings.HasSuffix(strings.TrimRight(parsed.Path, "/"), "/v1") {
		return platformSiteEndpointURL(normalized, "/models")
	}
	return platformSiteEndpointURL(normalized, "/v1/models")
}

func newAPIEndpointSnapshot(session *PlatformSiteSession) *PlatformSiteEndpointSnapshot {
	if session == nil {
		return nil
	}
	managementURL := firstNonEmptyString(session.ManagementBaseURL, session.BaseURL)
	relayURL := firstNonEmptyString(session.ModelBaseURL, session.BaseURL)
	return &PlatformSiteEndpointSnapshot{
		ManagementURL:   managementURL,
		RelayURL:        relayURL,
		ModelsURL:       platformSiteModelsURL(relayURL),
		PricingURL:      platformSiteEndpointURL(managementURL, "/api/pricing"),
		UsageURL:        platformSiteEndpointURL(managementURL, "/api/user/self"),
		TokenURL:        platformSiteEndpointURL(managementURL, "/api/token/"),
		AdminURL:        platformSiteEndpointURL(managementURL, "/api/channel/"),
		OpenAIURL:       relayURL,
		ClaudeURL:       relayURL,
		GeminiURL:       relayURL,
		ResponsesURL:    relayURL,
		Source:          "NewAPI",
		DiscoveryMethod: "configured_base_url",
		Enabled:         managementURL != "",
		Capabilities:    newAPIEndpointCapabilities(),
	}
}

func sub2APIEndpointSnapshot(session *PlatformSiteSession) *PlatformSiteEndpointSnapshot {
	if session == nil {
		return nil
	}
	managementURL := firstNonEmptyString(session.ManagementBaseURL, session.BaseURL)
	relayURL := firstNonEmptyString(session.ModelBaseURL, session.BaseURL)
	return &PlatformSiteEndpointSnapshot{
		ManagementURL:   managementURL,
		RelayURL:        relayURL,
		ModelsURL:       platformSiteModelsURL(relayURL),
		UsageURL:        platformSiteEndpointURL(managementURL, "/api/v1/usage/stats"),
		TokenURL:        platformSiteEndpointURL(managementURL, "/api/v1/keys"),
		AdminURL:        platformSiteEndpointURL(managementURL, "/api/v1/admin/accounts"),
		OpenAIURL:       relayURL,
		ClaudeURL:       relayURL,
		GeminiURL:       relayURL,
		ResponsesURL:    relayURL,
		Source:          "Sub2API",
		DiscoveryMethod: "page_api_base_url",
		Enabled:         managementURL != "",
		Capabilities:    sub2APIEndpointCapabilities(),
	}
}

func newAPIEndpointCapabilities() []PlatformSiteEndpointCapabilitySnapshot {
	entries := []struct {
		protocol string
		method   string
		path     string
	}{
		{"management", http.MethodGet, "/api/status"},
		{"management", http.MethodGet, "/api/user/self"},
		{"management", http.MethodGet, "/api/user/self/groups"},
		{"management", http.MethodGet, "/api/user/groups"},
		{"management", http.MethodGet, "/api/user/models"},
		{"management", http.MethodGet, "/api/user/available_models"},
		{"management", http.MethodGet, "/api/user/available_model/"},
		{"management", http.MethodGet, "/api/groupPro/selectable"},
		{"management", http.MethodGet, "/api/pricing"},
		{"management", http.MethodGet, "/api/token/"},
		{"management", http.MethodGet, "/api/token/{id}/key"},
		{"management", http.MethodPost, "/api/token/batch/keys"},
		{"admin", http.MethodGet, "/api/channel/"},
		{"admin", http.MethodGet, "/api/channel/{id}"},
		{"admin", http.MethodGet, "/api/channel/fetch_models/{id}"},
		{"openai", http.MethodGet, "/v1/models"},
	}
	result := make([]PlatformSiteEndpointCapabilitySnapshot, 0, len(entries))
	for _, entry := range entries {
		result = append(result, PlatformSiteEndpointCapabilitySnapshot{
			Protocol:   entry.protocol,
			HTTPMethod: entry.method,
			Path:       entry.path,
			Supported:  true,
			SourceData: "route",
		})
	}
	return result
}

func sub2APIEndpointCapabilities() []PlatformSiteEndpointCapabilitySnapshot {
	entries := []struct {
		protocol string
		method   string
		path     string
	}{
		{"management", http.MethodGet, "/api/v1/auth/me"},
		{"management", http.MethodGet, "/api/v1/user/profile"},
		{"management", http.MethodGet, "/api/v1/usage/stats"},
		{"management", http.MethodGet, "/api/v1/usage/dashboard/stats"},
		{"management", http.MethodGet, "/api/v1/groups/available"},
		{"management", http.MethodGet, "/api/v1/groups/rates"},
		{"management", http.MethodGet, "/api/v1/keys"},
		{"management", http.MethodGet, "/api/v1/keys/{id}"},
		{"openai", http.MethodGet, "/v1/models"},
	}
	result := make([]PlatformSiteEndpointCapabilitySnapshot, 0, len(entries))
	for _, entry := range entries {
		result = append(result, PlatformSiteEndpointCapabilitySnapshot{
			Protocol:   entry.protocol,
			HTTPMethod: entry.method,
			Path:       entry.path,
			Supported:  true,
			SourceData: "route",
		})
	}
	return result
}

func fetchNewAPIGroupResources(
	ctx context.Context,
	session *PlatformSiteSession,
) (map[string]float64, []PlatformSiteGroupSnapshot, bool, string, error) {
	rates := make(map[string]float64)
	groups := make([]PlatformSiteGroupSnapshot, 0)
	for _, path := range []string{"/api/user/self/groups", "/api/user/groups"} {
		payload, err := platformSiteRequest(ctx, session, http.MethodGet, path, nil, nil)
		if err != nil {
			if platformSiteRouteMissing(err) {
				continue
			}
			return rates, groups, false, path, err
		}
		for key, value := range parseGroupRates(payload) {
			rates[key] = value
		}
		groups = mergePlatformSiteGroups(groups, parsePlatformSiteGroups(payload, path, rates))
		return rates, groups, true, path, nil
	}
	selectableRates, selectableGroups, selectableErr := fetchNewAPISelectableGroups(ctx, session)
	if selectableErr == nil {
		return selectableRates, selectableGroups, true, "/api/groupPro/selectable", nil
	}
	return rates, groups, false, "/api/groupPro/selectable", selectableErr
}

func fetchNewAPISelectableGroups(
	ctx context.Context,
	session *PlatformSiteSession,
) (map[string]float64, []PlatformSiteGroupSnapshot, error) {
	const pageSize = 1000

	rates := make(map[string]float64)
	groups := make([]PlatformSiteGroupSnapshot, 0)
	for page := range upstreamSiteMaxPages {
		payload, err := platformSiteRequest(
			ctx,
			session,
			http.MethodGet,
			"/api/groupPro/selectable",
			url.Values{
				"p":        {fmt.Sprint(page)},
				"pageSize": {fmt.Sprint(pageSize)},
			},
			nil,
		)
		if err != nil {
			return rates, groups, err
		}

		rawItems := unwrapPlatformData(payload)
		items, ok := rawItems.([]any)
		if !ok {
			return rates, groups, fmt.Errorf(
				"%w: APIyi 可选分组响应不是数组",
				ErrPlatformSiteResponse,
			)
		}
		for _, item := range items {
			record, ok := item.(map[string]any)
			if !ok {
				return rates, groups, fmt.Errorf(
					"%w: APIyi 可选分组记录无效",
					ErrPlatformSiteResponse,
				)
			}
			name := firstString(record, "name")
			ratio, found, ratioErr := firstValidatedNonNegativeFloat(record, "convert_ratio")
			if ratioErr != nil || !found || !isValidConversionRatio(ratio) || name == "" {
				return rates, groups, fmt.Errorf(
					"%w: APIyi 可选分组字段无效",
					ErrPlatformSiteResponse,
				)
			}
		}

		for key, value := range parseGroupRates(payload) {
			rates[key] = value
		}
		groups = mergePlatformSiteGroups(
			groups,
			parsePlatformSiteGroups(payload, "/api/groupPro/selectable", rates),
		)
		if len(items) < pageSize {
			return rates, groups, nil
		}
	}
	return rates, groups, errors.New("APIyi 可选分组分页超过安全上限")
}

func mergePlatformSiteGroups(
	left []PlatformSiteGroupSnapshot,
	right []PlatformSiteGroupSnapshot,
) []PlatformSiteGroupSnapshot {
	byID := make(map[string]int, len(left)+len(right))
	result := append([]PlatformSiteGroupSnapshot(nil), left...)
	for index := range result {
		byID[result[index].ExternalID] = index
	}
	for _, group := range right {
		if index, ok := byID[group.ExternalID]; ok {
			if group.Name != "" {
				result[index].Name = group.Name
			}
			if group.Ratio != 0 || result[index].Ratio == 0 {
				result[index].Ratio = group.Ratio
			}
			result[index].Available = result[index].Available || group.Available
			result[index].Usable = result[index].Usable || group.Usable
			continue
		}
		byID[group.ExternalID] = len(result)
		result = append(result, group)
	}
	return result
}

func applyNewAPIPricingResources(
	snapshot *PlatformSiteSnapshot,
	_ *PlatformSiteSession,
	payload any,
) bool {
	if snapshot == nil {
		return false
	}
	record := make(map[string]any)
	if envelope, ok := payload.(map[string]any); ok {
		maps.Copy(record, envelope)
		if dataRecord, ok := envelope["data"].(map[string]any); ok {
			maps.Copy(record, dataRecord)
		}
	}
	if len(record) == 0 {
		return false
	}
	rates := parseGroupRates(record["group_ratio"])
	usableGroups := map[string]any{}
	if value, ok := record["usable_group"].(map[string]any); ok {
		usableGroups = value
	}
	for groupID, ratio := range rates {
		group := PlatformSiteGroupSnapshot{
			ExternalID:     groupID,
			Name:           groupID,
			Ratio:          ratio,
			Available:      true,
			Usable:         len(usableGroups) == 0,
			SourceEndpoint: "/api/pricing",
		}
		if _, ok := usableGroups[groupID]; ok {
			group.Usable = true
		}
		snapshot.Groups = mergePlatformSiteGroups(snapshot.Groups, []PlatformSiteGroupSnapshot{group})
	}
	for groupID := range usableGroups {
		if slices.ContainsFunc(snapshot.Groups, func(group PlatformSiteGroupSnapshot) bool {
			return group.ExternalID == groupID
		}) {
			continue
		}
		snapshot.Groups = append(snapshot.Groups, PlatformSiteGroupSnapshot{
			ExternalID:     groupID,
			Name:           groupID,
			Ratio:          firstGroupRate(rates, groupID),
			Available:      true,
			Usable:         true,
			SourceEndpoint: "/api/pricing",
		})
	}
	if len(snapshot.Groups) > 0 {
		snapshot.GroupsLoaded = true
	}
	if snapshot.Endpoint == nil {
		snapshot.Endpoint = &PlatformSiteEndpointSnapshot{Enabled: true, Source: "NewAPI"}
	}
	if supported, ok := record["supported_endpoint"]; ok {
		snapshot.Endpoint.Capabilities = append(
			snapshot.Endpoint.Capabilities,
			newAPIPricingEndpointCapabilities(supported)...,
		)
	}
	snapshot.ResourceSyncs = append(snapshot.ResourceSyncs, PlatformSiteResourceSyncSnapshot{
		ResourceType:   model.PlatformSiteResourceEndpoints,
		Status:         model.PlatformSiteResourceStatusSuccess,
		SourceEndpoint: "/api/pricing",
		RecordCount:    len(snapshot.Endpoint.Capabilities),
	})
	return true
}

func newAPIPricingEndpointCapabilities(payload any) []PlatformSiteEndpointCapabilitySnapshot {
	result := make([]PlatformSiteEndpointCapabilitySnapshot, 0)
	appendRecord := func(protocol string, record map[string]any) {
		path := firstString(record, "path", "url", "endpoint", "route")
		if path == "" {
			return
		}
		method := firstNonEmptyString(
			firstString(record, "method", "http_method", "httpMethod"),
			http.MethodPost,
		)
		sourceData, err := common.Marshal(record)
		if err != nil {
			sourceData = nil
		}
		result = append(result, PlatformSiteEndpointCapabilitySnapshot{
			Protocol:   protocol,
			HTTPMethod: strings.ToUpper(method),
			Path:       path,
			Supported:  true,
			SourceData: string(sourceData),
		})
	}
	switch value := unwrapPlatformData(payload).(type) {
	case map[string]any:
		for protocol, raw := range value {
			switch typed := raw.(type) {
			case map[string]any:
				appendRecord(protocol, typed)
			case string:
				result = append(result, PlatformSiteEndpointCapabilitySnapshot{
					Protocol:   protocol,
					HTTPMethod: http.MethodPost,
					Path:       typed,
					Supported:  true,
					SourceData: "pricing.supported_endpoint",
				})
			}
		}
	case []any:
		for _, raw := range value {
			if record, ok := raw.(map[string]any); ok {
				appendRecord(firstString(record, "protocol", "type", "name"), record)
			}
		}
	}
	return result
}

func parsePlatformSiteGroups(
	payload any,
	endpoint string,
	rates map[string]float64,
) []PlatformSiteGroupSnapshot {
	unwrapped := unwrapPlatformData(payload)
	records := recordsFromPayload(unwrapped)
	if record, ok := unwrapped.(map[string]any); ok &&
		!hasAnyField(record, "id", "group_id", "groupId", "name", "group", "items", "list", "groups") {
		records = make([]map[string]any, 0, len(record))
		for key, value := range record {
			switch typed := value.(type) {
			case map[string]any:
				copyRecord := make(map[string]any, len(typed)+1)
				maps.Copy(copyRecord, typed)
				if !hasAnyField(copyRecord, "id", "group_id", "groupId") {
					copyRecord["id"] = key
				}
				records = append(records, copyRecord)
			default:
				records = append(records, map[string]any{"id": key, "ratio": value})
			}
		}
	}
	result := make([]PlatformSiteGroupSnapshot, 0, len(records))
	for _, record := range records {
		groupID := firstString(
			record,
			"id",
			"group_id",
			"groupId",
			"external_id",
			"externalId",
			"name",
		)
		groupName := firstString(
			record,
			"display_name",
			"displayName",
			"name",
			"group_name",
			"groupName",
			"group",
		)
		if groupID == "" {
			groupID = groupName
		}
		if groupID == "" {
			continue
		}
		ratioSet := hasAnyField(
			record,
			"rate_multiplier",
			"ratio",
			"rate",
			"multiplier",
			"group_ratio",
			"convert_ratio",
		)
		ratio := firstFloat(
			record,
			"rate_multiplier",
			"ratio",
			"rate",
			"multiplier",
			"group_ratio",
			"convert_ratio",
		)
		if !ratioSet {
			ratio = firstGroupRate(rates, groupID, groupName)
			if !hasGroupRate(rates, groupID, groupName) {
				ratio = 1
			}
		}
		if !isValidConversionRatio(ratio) {
			continue
		}
		result = append(result, PlatformSiteGroupSnapshot{
			ExternalID:        groupID,
			Name:              firstNonEmptyString(groupName, groupID),
			Ratio:             ratio,
			Available:         firstBoolOrDefault(record, true, "available", "enabled", "active"),
			Usable:            firstBoolOrDefault(record, true, "usable", "allowed", "selectable"),
			SourceEndpoint:    endpoint,
			UpstreamUpdatedAt: firstInt64(record, "updated_at", "updatedAt"),
		})
	}
	return result
}

func hasGroupRate(rates map[string]float64, values ...string) bool {
	for _, value := range values {
		if _, ok := rates[strings.TrimSpace(value)]; ok && strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func firstBoolOrDefault(record map[string]any, fallback bool, keys ...string) bool {
	if !hasAnyField(record, keys...) {
		return fallback
	}
	return boolFromRecord(record, keys...)
}

func fetchNewAPIModels(ctx context.Context, session *PlatformSiteSession) ([]string, error) {
	var lastErr error
	for _, path := range []string{
		"/api/user/models",
		"/api/user/available_models",
		"/api/user/available_model/",
	} {
		payload, err := platformSiteRequest(ctx, session, http.MethodGet, path, nil, nil)
		if err != nil {
			lastErr = err
			if !platformSiteRouteMissing(err) {
				return nil, err
			}
			continue
		}
		models := uniqueStrings(append(stringsFromPayload(payload), modelsFromGroups(payload)...))
		if len(models) > 0 {
			return models, nil
		}
		return []string{}, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("%w: NewAPI 模型接口不可用", ErrPlatformSiteResponse)
	}
	return nil, lastErr
}

func fetchSub2APIModels(ctx context.Context, session *PlatformSiteSession) []string {
	return nil
}

func modelsFromKeys(keys []UpstreamKeySnapshot) []string {
	models := make([]string, 0)
	for _, key := range keys {
		if !key.ModelsSynced {
			continue
		}
		models = append(models, key.Models...)
	}
	return uniqueStrings(models)
}

func modelsFromRecord(record map[string]any) []string {
	for _, key := range []string{
		"models",
		"model_limits",
		"modelLimits",
		"model_limit",
		"modelLimit",
		"allowed_models",
		"allowedModels",
		"model_ids",
		"modelIds",
		"model_names",
		"modelNames",
		"supported_models",
		"supportedModels",
	} {
		if value, ok := record[key]; ok {
			if models := stringsFromPayload(value); len(models) > 0 {
				return models
			}
		}
	}
	return nil
}

func fetchModelsForSecret(ctx context.Context, session *PlatformSiteSession, secret string) ([]string, error) {
	if session == nil || strings.TrimSpace(secret) == "" {
		return nil, errors.New("上游密钥为空")
	}
	bases := uniqueStrings([]string{session.ModelBaseURL, session.BaseURL})
	var lastErr error
	for _, baseURL := range bases {
		paths := modelEndpointPaths(baseURL)
		for _, path := range paths {
			keySession := *session
			keySession.BaseURL = baseURL
			keySession.ManagementBaseURL = ""
			keySession.CredentialUpdate = nil
			keySession.Headers = make(http.Header)
			keySession.Headers.Set("Authorization", bearerToken(secret))
			keySession.Headers.Set("x-api-key", secret)
			if session.Client != nil {
				client := *session.Client
				client.Jar = nil
				keySession.Client = &client
			}
			payload, err := platformSiteRequest(ctx, &keySession, http.MethodGet, path, nil, nil)
			if err != nil {
				lastErr = err
				if !platformSiteRouteMissing(err) {
					return nil, err
				}
				continue
			}
			models := stringsFromPayload(payload)
			if len(models) > 0 {
				return models, nil
			}
			lastErr = fmt.Errorf("%w: 子密钥模型列表为空", ErrPlatformSiteResponse)
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("%w: 子密钥模型列表为空", ErrPlatformSiteResponse)
}

func modelsFromGroups(payload any) []string {
	records := recordsFromPayload(payload)
	models := make([]string, 0)
	for _, record := range records {
		if values, ok := record["models"]; ok {
			models = append(models, stringsFromPayload(values)...)
		}
	}
	return uniqueStrings(models)
}

func optionalInt64(record map[string]any, keys ...string) *int64 {
	for _, key := range keys {
		if _, ok := record[key]; !ok {
			continue
		}
		value := firstInt64(record, key)
		return &value
	}
	return nil
}

func optionalInt64FromFloat(value float64) *int64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return nil
	}
	converted := int64(value)
	return &converted
}

func boolFromRecord(record map[string]any, keys ...string) bool {
	for _, key := range keys {
		switch value := record[key].(type) {
		case bool:
			return value
		case string:
			parsed, err := strconv.ParseBool(strings.TrimSpace(value))
			if err == nil {
				return parsed
			}
		case float64:
			return value != 0
		case int:
			return value != 0
		case int64:
			return value != 0
		}
	}
	return false
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func uniqueStringMaps(items []map[string]string) []map[string]string {
	result := make([]map[string]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if len(item) == 0 {
			continue
		}
		keys := make([]string, 0, len(item))
		for key := range item {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			value := strings.TrimSpace(item[key])
			if value != "" {
				parts = append(parts, key+"="+value)
			}
		}
		signature := strings.Join(parts, ";")
		if signature == "" {
			continue
		}
		if _, ok := seen[signature]; ok {
			continue
		}
		seen[signature] = struct{}{}
		result = append(result, item)
	}
	return result
}

func firstGroupRate(rates map[string]float64, values ...string) float64 {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if ratio, ok := rates[value]; ok {
			return ratio
		}
	}
	return 0
}

func sub2APIGroupIdentifiers(item map[string]any) (string, string) {
	groupID := firstString(item, "group_id", "groupId")
	groupName := firstString(item, "group_name", "groupName")
	switch group := item["group"].(type) {
	case string:
		if groupName == "" {
			groupName = strings.TrimSpace(group)
		}
	case map[string]any:
		if groupID == "" {
			groupID = firstString(group, "id", "group_id", "groupId")
		}
		if groupName == "" {
			groupName = firstString(group, "name", "group", "group_name", "groupName")
		}
	}
	return groupID, groupName
}

type newAPITokenPageInfo struct {
	responsePage    int64
	responsePageSet bool
	pageSize        int64
	pageSizeSet     bool
	total           int64
	totalSet        bool
}

func newAPITokenPageInfoFromPayload(payload any) newAPITokenPageInfo {
	record := firstRecord(payload)
	responsePage, responsePageSet := firstOptionalInt64(record, "page")
	pageSize, pageSizeSet := firstOptionalInt64(record, "page_size", "pageSize", "size")
	total, totalSet := firstOptionalInt64(record, "total")
	return newAPITokenPageInfo{
		responsePage:    responsePage,
		responsePageSet: responsePageSet && responsePage >= 0,
		pageSize:        pageSize,
		pageSizeSet:     pageSizeSet && pageSize > 0,
		total:           total,
		totalSet:        totalSet && total >= 0,
	}
}

func (info newAPITokenPageInfo) hasMore(
	requestedPage int,
	itemCount int,
	oneBased bool,
) bool {
	if itemCount == 0 {
		return false
	}
	pageSize := int64(upstreamSitePageSize)
	if info.pageSizeSet {
		pageSize = info.pageSize
	}
	if pageSize <= 0 {
		return itemCount >= upstreamSitePageSize
	}
	if info.totalSet {
		responsePage := int64(requestedPage)
		if info.responsePageSet {
			responsePage = info.responsePage
		}
		var offset int64
		const maxInt64 = int64(^uint64(0) >> 1)
		if oneBased {
			if responsePage < 1 {
				return itemCount >= int(pageSize)
			}
			pageIndex := responsePage - 1
			if pageIndex > maxInt64/pageSize {
				return itemCount >= int(pageSize)
			}
			offset = pageIndex * pageSize
		} else if responsePage >= 0 {
			if responsePage > maxInt64/pageSize {
				return itemCount >= int(pageSize)
			}
			offset = responsePage * pageSize
		} else {
			return itemCount >= int(pageSize)
		}
		if int64(itemCount) > maxInt64-offset {
			return itemCount >= int(pageSize)
		}
		loadedThrough := offset + int64(itemCount)
		if info.total >= loadedThrough {
			return loadedThrough < info.total
		}
	}
	return itemCount >= int(pageSize)
}

func fetchNewAPITokens(ctx context.Context, session *PlatformSiteSession) ([]map[string]any, error) {
	result := make([]map[string]any, 0)
	page := 1
	for fetchedPages := 0; fetchedPages < upstreamSiteResourceMaxPages; fetchedPages++ {
		var payload any
		var err error
		for _, path := range []string{"/api/token/", "/api/token", "/api/tokens"} {
			payload, err = platformSiteRequest(ctx, session, http.MethodGet, path, url.Values{
				"p":         {fmt.Sprint(page)},
				"page_size": {fmt.Sprint(upstreamSitePageSize)},
			}, nil)
			if err == nil {
				break
			}
			if !platformSiteRouteMissing(err) {
				break
			}
		}
		if err != nil {
			return nil, err
		}
		pageItems := recordsFromPayload(payload)
		result = append(result, pageItems...)
		pageInfo := newAPITokenPageInfoFromPayload(payload)
		if !pageInfo.hasMore(page, len(pageItems), true) {
			return result, nil
		}
		page++
	}
	return nil, errors.New("NewAPI 密钥分页超过安全上限")
}

func fetchNewAPITokenKeys(ctx context.Context, session *PlatformSiteSession, tokens []map[string]any) (map[string]string, map[string]error) {
	result := make(map[string]string)
	failures := make(map[string]error)
	ids := make([]any, 0, len(tokens))
	for _, token := range tokens {
		externalID, upstreamID := upstreamIdentifier(token, "id", "token_id", "key_id")
		secret := firstString(token, "key", "token", "api_key")
		if externalID != "" && (secret == "" || strings.Contains(secret, "*")) {
			ids = append(ids, upstreamID)
		}
	}
	var batchErr error
	batchRouteMissing := false
	if len(ids) > 0 {
		payload, err := platformSiteRequest(ctx, session, http.MethodPost, "/api/token/batch/keys", nil, map[string]any{
			"ids": ids,
		})
		if err != nil {
			batchErr = err
			batchRouteMissing = platformSiteRouteMissing(err)
		} else {
			for id, key := range stringsMapFromPayload(payload) {
				if isCompleteNewAPITokenKey(key) {
					result[id] = strings.TrimSpace(key)
				}
			}
		}
	}
	var fallbackErr error
	for _, token := range tokens {
		externalID := firstString(token, "id", "token_id", "key_id")
		if externalID == "" || isCompleteNewAPITokenKey(result[externalID]) {
			continue
		}
		secret := firstString(token, "key", "token", "api_key")
		if secret != "" && !strings.Contains(secret, "*") {
			result[externalID] = secret
			continue
		}
		if batchErr != nil && !batchRouteMissing {
			failures[externalID] = batchErr
			continue
		}
		key, err := fetchNewAPITokenKey(ctx, session, externalID)
		if err != nil {
			fallbackErr = err
			failures[externalID] = err
			continue
		}
		if isCompleteNewAPITokenKey(key) {
			result[externalID] = strings.TrimSpace(key)
		}
	}
	for _, token := range tokens {
		externalID := firstString(token, "id", "token_id", "key_id")
		if externalID == "" || isCompleteNewAPITokenKey(result[externalID]) {
			continue
		}
		secret := firstString(token, "key", "token", "api_key")
		if secret != "" && !strings.Contains(secret, "*") {
			continue
		}
		switch {
		case fallbackErr != nil:
			failures[externalID] = fallbackErr
		case batchErr != nil:
			failures[externalID] = batchErr
		default:
			failures[externalID] = fmt.Errorf("%w: 完整 Key 未返回", ErrPlatformSiteResponse)
		}
	}
	return result, failures
}

func upstreamIdentifier(record map[string]any, keys ...string) (string, any) {
	for _, key := range keys {
		value, exists := record[key]
		if !exists {
			continue
		}
		switch typed := value.(type) {
		case string:
			trimmed := strings.TrimSpace(typed)
			if trimmed != "" {
				return trimmed, trimmed
			}
		case float64:
			if math.IsNaN(typed) || math.IsInf(typed, 0) {
				continue
			}
			if typed == math.Trunc(typed) {
				integerID := int64(typed)
				return strconv.FormatInt(integerID, 10), integerID
			}
			text := strconv.FormatFloat(typed, 'f', -1, 64)
			return text, text
		case int:
			return strconv.Itoa(typed), typed
		case int64:
			return strconv.FormatInt(typed, 10), typed
		case uint:
			text := strconv.FormatUint(uint64(typed), 10)
			return text, typed
		case uint64:
			text := strconv.FormatUint(typed, 10)
			return text, typed
		}
	}
	return "", nil
}

func fetchNewAPITokenKey(ctx context.Context, session *PlatformSiteSession, externalID string) (string, error) {
	path := "/api/token/" + url.PathEscape(externalID) + "/key"
	payload, err := platformSiteRequest(ctx, session, http.MethodPost, path, nil, nil)
	if err != nil {
		if !platformSiteRouteMissing(err) {
			return "", err
		}
		payload, err = platformSiteRequest(ctx, session, http.MethodGet, path, nil, nil)
		if err != nil {
			return "", err
		}
	}
	key := stringFromPayload(payload)
	if !isCompleteNewAPITokenKey(key) {
		return "", fmt.Errorf("%w: 完整 Key 为空", ErrPlatformSiteResponse)
	}
	return key, nil
}

func isCompleteNewAPITokenKey(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && !strings.Contains(value, "*")
}

func stringsMapFromPayload(payload any) map[string]string {
	payload = unwrapPlatformData(payload)
	if record, ok := payload.(map[string]any); ok {
		for _, field := range []string{"keys", "data", "items", "list", "records", "rows"} {
			if nested, exists := record[field]; exists {
				candidate := stringsMapFromPayload(nested)
				if len(candidate) > 0 {
					return candidate
				}
			}
		}
		result := make(map[string]string, len(record))
		for key, value := range record {
			switch typed := value.(type) {
			case string:
				result[strings.TrimSpace(key)] = strings.TrimSpace(typed)
			case float64:
				result[strings.TrimSpace(key)] = strconv.FormatFloat(typed, 'f', -1, 64)
			}
		}
		if len(result) > 0 {
			return result
		}
	}
	result := make(map[string]string)
	for _, record := range recordsFromPayload(payload) {
		id := firstString(record, "id", "token_id", "key_id")
		key := firstString(record, "key", "token", "api_key", "value")
		if id != "" && key != "" {
			result[id] = key
		}
	}
	return result
}

func newAPIRemainQuota(record map[string]any) (*int64, error) {
	if boolFromRecord(record, "unlimited_quota", "unlimitedQuota", "unlimited") {
		return nil, nil
	}
	if remain, found, err := firstValidatedInternalQuota(
		record,
		"remain_quota",
		"remaining_quota",
	); err != nil {
		return nil, err
	} else if found {
		return &remain, nil
	}
	if !hasAnyField(record, "quota") {
		return nil, nil
	}
	quota, found, err := firstValidatedInternalQuota(record, "quota")
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	return &quota, nil
}

func fetchSub2APIKeys(ctx context.Context, session *PlatformSiteSession, rates map[string]float64) ([]UpstreamKeySnapshot, error) {
	result := make([]UpstreamKeySnapshot, 0)
	for page := 1; page <= upstreamSiteResourceMaxPages; page++ {
		payload, err := platformSiteRequest(ctx, session, http.MethodGet, "/api/v1/keys", url.Values{
			"page":      {fmt.Sprint(page)},
			"page_size": {fmt.Sprint(upstreamSitePageSize)},
		}, nil)
		if err != nil {
			return result, err
		}
		items := recordsFromPayload(payload)
		for _, item := range items {
			externalID := firstString(item, "id", "key_id")
			if externalID == "" {
				return result, fmt.Errorf("%w: Sub2API 密钥缺少外部 ID", ErrPlatformSiteResponse)
			}
			groupID, groupName := sub2APIGroupIdentifiers(item)
			ratioKeysPresent := hasAnyField(item, "rate_multiplier", "ratio", "rate", "multiplier")
			ratio := firstFloat(item, "rate_multiplier", "ratio", "rate", "multiplier")
			_, groupRateSet := rates[groupID]
			if !groupRateSet {
				_, groupRateSet = rates[groupName]
			}
			if !ratioKeysPresent {
				ratio = firstGroupRate(rates, groupID, groupName)
			}
			itemModels := modelsFromRecord(item)
			keyUsedQuota, keyUsedQuotaSet, quotaErr := firstSub2APIQuota(
				item,
				"quota_used",
				"used_quota",
				"used",
				"usedQuota",
				"quotaUsed",
				"total_used",
				"total_used_quota",
				"totalUsedQuota",
				"totalUsed",
			)
			if quotaErr != nil {
				return result, quotaErr
			}
			remainQuota, remainErr := sub2APIRemainQuota(item)
			if remainErr != nil {
				return result, remainErr
			}
			keySnapshot := UpstreamKeySnapshot{
				ExternalID:               externalID,
				Name:                     firstString(item, "name", "key_name"),
				Group:                    firstNonEmptyString(groupName, groupID),
				Models:                   itemModels,
				ModelsSynced:             len(itemModels) > 0,
				SourceConversionRatio:    ratio,
				SourceConversionRatioSet: ratioKeysPresent || groupRateSet,
				ConversionRatio:          ratio,
				ConversionRatioSet:       ratioKeysPresent || groupRateSet,
				UsedQuota:                keyUsedQuota,
				UsedQuotaSet:             keyUsedQuotaSet,
				RemainQuota:              remainQuota,
				ExpiresAt:                firstTime(item, "expires_at", "expired_at", "expire_at"),
				Disabled:                 isUpstreamKeyDisabled(item),
			}
			secret := nestedString(item, "credentials", "api_key")
			if secret == "" {
				secret = firstString(item, "key", "api_key", "token")
			}
			if secret == "" || strings.Contains(secret, "*") {
				detail, detailErr := fetchSub2APIKeyDetail(ctx, session, externalID)
				if detailErr == nil {
					if len(keySnapshot.Models) == 0 {
						keySnapshot.Models = modelsFromRecord(detail)
						keySnapshot.ModelsSynced = len(keySnapshot.Models) > 0
					}
					secret = stringFromPayload(detail)
				} else if errors.Is(detailErr, ErrPlatformSiteSecurity) {
					return result, detailErr
				}
			}
			if secret == "" || strings.Contains(secret, "*") {
				keySnapshot.SyncError = upstreamKeySyncErrorSecretUnavailable
				result = append(result, keySnapshot)
				continue
			}
			keySnapshot.Secret = secret
			if !keySnapshot.ModelsSynced {
				models, modelsErr := fetchModelsForSecret(ctx, session, secret)
				if modelsErr == nil && len(models) > 0 {
					keySnapshot.Models = models
					keySnapshot.ModelsSynced = true
				} else {
					keySnapshot.SyncError = upstreamKeySyncErrorModelsUnavailable
				}
			}
			result = append(result, keySnapshot)
		}
		if !platformSitePageHasMore(payload, page, len(items), upstreamSiteResourceMaxPages) {
			return result, nil
		}
	}
	return result, errors.New("Sub2API 密钥分页超过安全上限")
}

func fetchSub2APIKeyDetail(
	ctx context.Context,
	session *PlatformSiteSession,
	externalID string,
) (map[string]any, error) {
	if strings.TrimSpace(externalID) == "" {
		return nil, errors.New("Sub2API 密钥 ID 为空")
	}
	payload, err := platformSiteRequest(
		ctx,
		session,
		http.MethodGet,
		"/api/v1/keys/"+url.PathEscape(externalID),
		nil,
		nil,
	)
	if err != nil {
		return nil, err
	}
	record := firstRecord(payload)
	if len(record) == 0 {
		return nil, fmt.Errorf("%w: Sub2API 密钥详情为空", ErrPlatformSiteResponse)
	}
	return record, nil
}

func fetchSub2APIAdminKeys(ctx context.Context, session *PlatformSiteSession, rates map[string]float64) ([]UpstreamKeySnapshot, error) {
	result := make([]UpstreamKeySnapshot, 0)
	for page := 1; page <= upstreamSiteResourceMaxPages; page++ {
		payload, err := platformSiteRequest(ctx, session, http.MethodGet, "/api/v1/admin/accounts", url.Values{
			"page":       {fmt.Sprint(page)},
			"page_size":  {fmt.Sprint(upstreamSitePageSize)},
			"sort_by":    {"name"},
			"sort_order": {"asc"},
			"type":       {"apikey"},
		}, nil)
		if err != nil {
			return result, err
		}
		items := recordsFromPayload(payload)
		for _, item := range items {
			id := firstString(item, "id", "account_id")
			if id == "" {
				return result, fmt.Errorf("%w: Sub2API 密钥缺少外部 ID", ErrPlatformSiteResponse)
			}
			groupID, groupName := sub2APIGroupIdentifiers(item)
			ratioKeysPresent := hasAnyField(item, "rate_multiplier", "ratio", "rate", "multiplier")
			ratio := firstFloat(item, "rate_multiplier", "ratio", "rate", "multiplier")
			_, groupRateSet := rates[groupID]
			if !groupRateSet {
				_, groupRateSet = rates[groupName]
			}
			if !ratioKeysPresent {
				ratio = firstGroupRate(rates, groupID, groupName)
			}
			itemModels := modelsFromRecord(item)
			keyUsedQuota, keyUsedQuotaSet, quotaErr := firstSub2APIQuota(
				item,
				"quota_used",
				"used_quota",
				"used",
				"usedQuota",
				"quotaUsed",
				"total_used",
				"total_used_quota",
				"totalUsedQuota",
				"totalUsed",
			)
			if quotaErr != nil {
				return result, quotaErr
			}
			remainQuota, remainErr := sub2APIRemainQuota(item)
			if remainErr != nil {
				return result, remainErr
			}
			keySnapshot := UpstreamKeySnapshot{
				ExternalID:               id,
				Name:                     firstString(item, "name", "account_name"),
				Group:                    firstNonEmptyString(groupName, groupID),
				Models:                   itemModels,
				ModelsSynced:             len(itemModels) > 0,
				SourceConversionRatio:    ratio,
				SourceConversionRatioSet: ratioKeysPresent || groupRateSet,
				ConversionRatio:          ratio,
				ConversionRatioSet:       ratioKeysPresent || groupRateSet,
				UsedQuota:                keyUsedQuota,
				UsedQuotaSet:             keyUsedQuotaSet,
				RemainQuota:              remainQuota,
				ExpiresAt:                firstTime(item, "expires_at", "expired_at", "expire_at"),
				Disabled:                 isUpstreamKeyDisabled(item),
			}
			secret := nestedString(item, "credentials", "api_key")
			if secret == "" {
				secret = firstString(item, "key", "api_key", "token")
			}
			if secret == "" || strings.Contains(secret, "*") {
				dataPayload, dataErr := platformSiteRequest(ctx, session, http.MethodGet, "/api/v1/admin/accounts/data", url.Values{
					"ids":             {id},
					"include_proxies": {"false"},
				}, nil)
				if dataErr != nil {
					if errors.Is(dataErr, ErrPlatformSiteSecurity) {
						return result, dataErr
					}
					keySnapshot.SyncError = upstreamKeySyncErrorSecretUnavailable
					result = append(result, keySnapshot)
					continue
				}
				data := firstRecord(dataPayload)
				secret = nestedString(data, "accounts", "0", "credentials", "api_key")
				if secret == "" {
					secret = firstString(data, "key", "api_key", "token")
				}
			}
			if secret == "" || strings.Contains(secret, "*") {
				keySnapshot.SyncError = upstreamKeySyncErrorSecretUnavailable
				result = append(result, keySnapshot)
				continue
			}
			keySnapshot.Secret = secret
			if !keySnapshot.ModelsSynced {
				models, modelsErr := fetchModelsForSecret(ctx, session, secret)
				if modelsErr == nil && len(models) > 0 {
					keySnapshot.Models = models
					keySnapshot.ModelsSynced = true
				} else {
					keySnapshot.SyncError = upstreamKeySyncErrorModelsUnavailable
				}
			}
			result = append(result, keySnapshot)
		}
		if !platformSitePageHasMore(payload, page, len(items), upstreamSiteResourceMaxPages) {
			return result, nil
		}
	}
	return result, errors.New("Sub2API 管理密钥分页超过安全上限")
}

func sub2APIRemainQuota(record map[string]any) (*int64, error) {
	if boolFromRecord(record, "unlimited", "unlimited_quota", "unlimitedQuota") {
		return nil, nil
	}
	if hasAnyField(record, "quota") {
		quota, quotaSet, err := firstSub2APINonNegativeFloat(record, "quota")
		if err != nil {
			return nil, err
		}
		if !quotaSet {
			return nil, nil
		}
		if quota <= 0 {
			return nil, nil
		}
		used, usedSet, err := firstSub2APINonNegativeFloat(
			record,
			"quota_used",
			"used_quota",
			"used",
			"usedQuota",
			"quotaUsed",
			"total_used",
			"total_used_quota",
			"totalUsedQuota",
			"totalUsed",
		)
		if err != nil {
			return nil, err
		}
		if !usedSet {
			used = 0
		}
		remain := quota - used
		if remain < 0 {
			remain = 0
		}
		converted, err := sub2APIQuotaToInternal(remain)
		if err != nil {
			return nil, err
		}
		return &converted, nil
	}
	if remain, ok, err := firstSub2APINonNegativeFloat(
		record,
		"remain_quota",
		"remaining_quota",
		"remainQuota",
		"remainingQuota",
	); err != nil {
		return nil, err
	} else if ok {
		converted, err := sub2APIQuotaToInternal(remain)
		if err != nil {
			return nil, err
		}
		return &converted, nil
	}
	return nil, nil
}

func modelEndpointPaths(baseURL string) []string {
	normalized, err := normalizePlatformSiteURL(baseURL)
	if err != nil {
		return []string{"/v1/models", "/models"}
	}
	parsed, err := url.Parse(normalized)
	if err != nil {
		return []string{"/v1/models", "/models"}
	}
	path := strings.TrimRight(parsed.EscapedPath(), "/")
	if path == "/v1" || strings.HasSuffix(path, "/v1") {
		return []string{"/models", "/v1/models"}
	}
	return []string{"/v1/models", "/models"}
}

func normalizeSub2APIBaseURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return raw
	}
	switch strings.TrimRight(parsed.EscapedPath(), "/") {
	case "", "/", "/v1", "/login", "/dashboard", "/register", "/setup", "/home":
		parsed.Path = ""
		parsed.RawPath = ""
		parsed.RawQuery = ""
		parsed.Fragment = ""
		return strings.TrimRight(parsed.String(), "/")
	default:
		return raw
	}
}

func discoverSub2APIModelBaseURL(ctx context.Context, session *PlatformSiteSession) (string, bool) {
	if session == nil || session.Client == nil || strings.TrimSpace(session.BaseURL) == "" {
		return "", false
	}
	discovery, ok := discoverSub2APIPageConfiguration(
		ctx,
		session.Client,
		session.BaseURL,
	)
	if !ok {
		return "", false
	}
	normalized := discovery.RelayBaseURL
	if !sub2APIPageDeclaredRelayURLAllowed(
		session.BaseURL,
		discovery.FinalPageURL,
		normalized,
		discovery.DeclaredRelayURLs...,
	) {
		return "", false
	}
	return normalized, true
}

func discoverSub2APIManagementBaseURL(
	ctx context.Context,
	session *PlatformSiteSession,
) (string, string, string, bool) {
	if session == nil || session.Client == nil {
		return "", "", "", false
	}
	originalBaseURL, err := normalizePlatformSiteURL(session.BaseURL)
	if err != nil {
		return "", "", "", false
	}
	anonymousClient := *session.Client
	anonymousClient.Jar = nil
	discovery, ok := discoverSub2APIPageConfiguration(
		ctx,
		&anonymousClient,
		originalBaseURL,
	)
	modelBaseURL := discovery.RelayBaseURL
	finalPageURL := discovery.FinalPageURL
	if ok &&
		validatePlatformSiteURL(finalPageURL) == nil &&
		sub2APIPageDeclaredRelayURLAllowed(
			originalBaseURL,
			finalPageURL,
			modelBaseURL,
			discovery.DeclaredRelayURLs...,
		) {
		requestBaseURL := originalBaseURL
		if samePlatformSiteOrigin(finalPageURL, modelBaseURL) {
			requestBaseURL = platformSiteOriginBaseURL(finalPageURL)
		}
		return originalBaseURL, requestBaseURL, modelBaseURL, true
	}

	managementBaseURL, ok := sub2APIManagementBaseURLCandidate(originalBaseURL)
	if !ok {
		return "", "", "", false
	}
	discovery, ok = discoverSub2APIPageConfiguration(
		ctx,
		&anonymousClient,
		managementBaseURL,
	)
	modelBaseURL = discovery.RelayBaseURL
	finalPageURL = discovery.FinalPageURL
	if !ok ||
		validatePlatformSiteURL(finalPageURL) != nil ||
		!sub2APIPageDeclaredRelayURLAllowed(
			originalBaseURL,
			finalPageURL,
			modelBaseURL,
			discovery.DeclaredRelayURLs...,
		) {
		return "", "", "", false
	}
	requestBaseURL := managementBaseURL
	if samePlatformSiteOrigin(finalPageURL, modelBaseURL) {
		requestBaseURL = platformSiteOriginBaseURL(finalPageURL)
	}
	return managementBaseURL, requestBaseURL, modelBaseURL, true
}

func discoverSub2APIPageModelBaseURL(
	ctx context.Context,
	client *http.Client,
	pageBaseURL string,
) (string, string, bool) {
	discovery, ok := discoverSub2APIPageConfiguration(ctx, client, pageBaseURL)
	if !ok {
		return "", "", false
	}
	return discovery.RelayBaseURL, discovery.FinalPageURL, true
}

func discoverSub2APIPageConfiguration(
	ctx context.Context,
	client *http.Client,
	pageBaseURL string,
) (sub2APIPageDiscovery, bool) {
	if client == nil || strings.TrimSpace(pageBaseURL) == "" {
		return sub2APIPageDiscovery{}, false
	}
	initialPageURL, err := normalizePlatformSiteURL(pageBaseURL)
	if err != nil {
		return sub2APIPageDiscovery{}, false
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, pageBaseURL, nil)
	if err != nil {
		return sub2APIPageDiscovery{}, false
	}
	request.Header.Set("Accept", "text/html,application/xhtml+xml")
	anonymousClient := *client
	anonymousClient.Jar = nil
	previousCheckRedirect := client.CheckRedirect
	anonymousClient.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= 5 ||
			validatePlatformSiteURL(request.URL.String()) != nil ||
			!samePlatformSiteOrigin(initialPageURL, request.URL.String()) {
			return http.ErrUseLastResponse
		}
		if previousCheckRedirect != nil {
			return previousCheckRedirect(request, via)
		}
		return nil
	}
	response, err := anonymousClient.Do(request)
	if err != nil {
		return sub2APIPageDiscovery{}, false
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return sub2APIPageDiscovery{}, false
	}
	contentType := strings.ToLower(response.Header.Get("Content-Type"))
	if !strings.Contains(contentType, "text/html") &&
		!strings.Contains(contentType, "application/xhtml+xml") {
		return sub2APIPageDiscovery{}, false
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, upstreamSiteResponseLimit+1))
	if err != nil || len(data) > upstreamSiteResponseLimit {
		return sub2APIPageDiscovery{}, false
	}
	finalPageURL := pageBaseURL
	if response.Request != nil && response.Request.URL != nil {
		finalPageURL = response.Request.URL.String()
	}
	finalPageURL, err = normalizePlatformSiteURL(finalPageURL)
	if err != nil {
		return sub2APIPageDiscovery{}, false
	}
	configurations := parseSub2APIPageConfigurations(data)
	for _, configuration := range configurations {
		discovery := sub2APIPageDiscovery{
			ManagementBaseURL: platformSiteOriginBaseURL(finalPageURL),
			FinalPageURL:      finalPageURL,
		}
		discovery.DeclaredRelayURLs = resolveSub2APICustomEndpoints(
			configuration,
			finalPageURL,
		)
		apiRaw := firstNonEmptyString(
			configuration.APIBaseURL,
			configuration.APIBaseURLCamel,
		)
		if apiRaw != "" {
			discovery.ManagementAPIBaseURL = resolveSub2APIPageURL(apiRaw, finalPageURL)
		}
		discovery.RelayBaseURL = firstNonEmptyString(
			discovery.ManagementAPIBaseURL,
			firstStringValue(discovery.DeclaredRelayURLs),
		)
		if discovery.RelayBaseURL != "" {
			return discovery, true
		}
	}

	return sub2APIPageDiscovery{}, false
}

func parseSub2APIPageConfigurations(data []byte) []sub2APIPageConfiguration {
	source := html.UnescapeString(string(data))
	configurations := make([]sub2APIPageConfiguration, 0, 4)
	seen := make(map[string]struct{})

	appendConfiguration := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" || len(raw) > 256<<10 {
			return
		}
		var configuration sub2APIPageConfiguration
		if err := common.Unmarshal([]byte(raw), &configuration); err != nil {
			return
		}
		if strings.TrimSpace(configuration.APIBaseURL) == "" &&
			strings.TrimSpace(configuration.APIBaseURLCamel) == "" &&
			len(configuration.CustomEndpoints) == 0 &&
			len(configuration.CustomEndpointsCamel) == 0 {
			return
		}
		if _, exists := seen[raw]; exists {
			return
		}
		seen[raw] = struct{}{}
		configurations = append(configurations, configuration)
	}

	hasAppConfigMarker := false
	for _, marker := range []string{"window.__APP_CONFIG__", "__APP_CONFIG__"} {
		offset := 0
		for offset < len(source) && len(configurations) < 16 {
			index := strings.Index(source[offset:], marker)
			if index < 0 {
				break
			}
			hasAppConfigMarker = true
			index += offset + len(marker)
			if objectStart := strings.IndexByte(source[index:min(len(source), index+4096)], '{'); objectStart >= 0 {
				start := index + objectStart
				if object := balancedJSONFragment(source, start); object != "" {
					appendConfiguration(object)
				}
			}
			offset = index + 1
		}
	}
	if hasAppConfigMarker && len(configurations) > 0 {
		return configurations
	}

	for offset := 0; offset < len(source) && len(configurations) < 16; {
		index := strings.IndexAny(source[offset:], "{[")
		if index < 0 {
			break
		}
		start := offset + index
		if source[start] == '{' {
			appendConfiguration(balancedJSONFragment(source, start))
		}
		offset = start + 1
	}
	return configurations
}

func balancedJSONFragment(source string, start int) string {
	if start < 0 || start >= len(source) || source[start] != '{' {
		return ""
	}
	depth := 0
	inString := false
	escaped := false
	for index := start; index < len(source); index++ {
		char := source[index]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if char == '\\' {
				escaped = true
				continue
			}
			if char == '"' {
				inString = false
			}
			continue
		}
		switch char {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return source[start : index+1]
			}
		}
	}
	return ""
}

func resolveSub2APICustomEndpoints(
	configuration sub2APIPageConfiguration,
	finalPageURL string,
) []string {
	rawEndpoints := make([]string, 0,
		len(configuration.CustomEndpoints)+len(configuration.CustomEndpointsCamel),
	)
	for _, endpoint := range configuration.CustomEndpoints {
		rawEndpoints = append(rawEndpoints, endpoint.Endpoint)
	}
	for _, endpoint := range configuration.CustomEndpointsCamel {
		rawEndpoints = append(rawEndpoints, endpoint.Endpoint)
	}
	result := make([]string, 0, len(rawEndpoints))
	for _, raw := range rawEndpoints {
		if !sub2APIRelayURLHasNoCredentialsOrDecorations(raw) {
			continue
		}
		resolved := resolveSub2APIPageURL(raw, finalPageURL)
		if validatePlatformSiteURL(resolved) != nil {
			continue
		}
		result = append(result, resolved)
	}
	return uniqueStrings(result)
}

func resolveSub2APIPageURL(raw, finalPageURL string) string {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, `\/`, `/`))
	if raw == "" {
		return ""
	}
	if !sub2APIRelayURLHasNoCredentialsOrDecorations(raw) {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if !parsed.IsAbs() {
		baseURL, baseErr := url.Parse(finalPageURL)
		if baseErr != nil {
			return ""
		}
		raw = baseURL.ResolveReference(parsed).String()
	}
	normalized, err := normalizePlatformSiteURL(raw)
	if err != nil {
		return ""
	}
	return normalized
}

func sub2APIRelayURLHasNoCredentialsOrDecorations(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(strings.ReplaceAll(raw, `\/`, `/`)))
	if err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	return parsed.Scheme == "" || parsed.Scheme == "http" || parsed.Scheme == "https"
}

func firstStringValue(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func sub2APIPageDeclaredRelayURLAllowed(
	managementBaseURL string,
	finalPageURL string,
	relayBaseURL string,
	declaredRelayURLs ...string,
) bool {
	normalizedRelay, err := normalizePlatformSiteURL(relayBaseURL)
	if err != nil {
		return false
	}
	if samePlatformSiteOrigin(finalPageURL, normalizedRelay) ||
		samePlatformSiteOrigin(managementBaseURL, normalizedRelay) ||
		sub2APIDirectAPIOriginRelation(managementBaseURL, normalizedRelay) {
		return true
	}
	strictRelay, strictErr := strictSub2APIRelayURL(normalizedRelay)
	if strictErr != nil {
		return false
	}
	for _, declared := range declaredRelayURLs {
		normalizedDeclared, declaredErr := strictSub2APIRelayURL(declared)
		if declaredErr == nil &&
			sameStrictSub2APIRelayURL(
				normalizedDeclared,
				strictRelay,
			) {
			return true
		}
	}
	return false
}

func strictSub2APIRelayURL(raw string) (*url.URL, error) {
	if !sub2APIRelayURLHasNoCredentialsOrDecorations(raw) {
		return nil, errors.New("Sub2API Relay URL 包含不支持的字段")
	}
	normalized, err := normalizePlatformSiteURL(raw)
	if err != nil {
		return nil, err
	}
	if err := validatePlatformSiteURL(normalized); err != nil {
		return nil, err
	}
	parsed, err := url.Parse(normalized)
	if err != nil {
		return nil, err
	}
	return parsed, nil
}

func sameStrictSub2APIRelayURL(left, right *url.URL) bool {
	if left == nil || right == nil ||
		!strings.EqualFold(left.Scheme, right.Scheme) ||
		!strings.EqualFold(left.Hostname(), right.Hostname()) ||
		platformSiteEffectivePort(left) != platformSiteEffectivePort(right) {
		return false
	}
	leftPath := strings.TrimRight(left.EscapedPath(), "/")
	rightPath := strings.TrimRight(right.EscapedPath(), "/")
	return leftPath == rightPath
}

func platformSiteOriginBaseURL(raw string) string {
	normalized, err := normalizePlatformSiteURL(raw)
	if err != nil {
		return ""
	}
	parsed, err := url.Parse(normalized)
	if err != nil {
		return ""
	}
	parsed.Path = ""
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/")
}

func sub2APIDirectAPIOriginRelation(left string, right string) bool {
	leftNormalized, leftErr := normalizePlatformSiteURL(left)
	rightNormalized, rightErr := normalizePlatformSiteURL(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	leftURL, leftErr := url.Parse(leftNormalized)
	rightURL, rightErr := url.Parse(rightNormalized)
	if leftErr != nil || rightErr != nil ||
		!strings.EqualFold(leftURL.Scheme, rightURL.Scheme) ||
		platformSiteEffectivePort(leftURL) != platformSiteEffectivePort(rightURL) {
		return false
	}
	leftHost := normalizePlatformHostname(leftURL.Hostname())
	rightHost := normalizePlatformHostname(rightURL.Hostname())
	return leftHost == "api."+rightHost || rightHost == "api."+leftHost
}

func sub2APIManagementBaseURLCandidate(raw string) (string, bool) {
	normalized, err := normalizePlatformSiteURL(raw)
	if err != nil {
		return "", false
	}
	parsed, err := url.Parse(normalized)
	if err != nil {
		return "", false
	}
	host := normalizePlatformHostname(parsed.Hostname())
	firstLabel, remainder, found := strings.Cut(host, ".")
	if !found || firstLabel != "api" || remainder == "" {
		return "", false
	}
	port := parsed.Port()
	parsed.Host = remainder
	if port != "" {
		parsed.Host += ":" + port
	}
	parsed.Path = ""
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	candidate := strings.TrimRight(parsed.String(), "/")
	if validatePlatformSiteURL(candidate) != nil {
		return "", false
	}
	return candidate, true
}

func samePlatformSiteOrigin(left string, right string) bool {
	leftNormalized, leftErr := normalizePlatformSiteURL(left)
	rightNormalized, rightErr := normalizePlatformSiteURL(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	leftURL, leftErr := url.Parse(leftNormalized)
	rightURL, rightErr := url.Parse(rightNormalized)
	if leftErr != nil || rightErr != nil {
		return false
	}
	if !strings.EqualFold(leftURL.Scheme, rightURL.Scheme) ||
		!strings.EqualFold(leftURL.Hostname(), rightURL.Hostname()) {
		return false
	}
	return platformSiteEffectivePort(leftURL) == platformSiteEffectivePort(rightURL)
}

func platformSiteEffectivePort(value *url.URL) string {
	if port := value.Port(); port != "" {
		return port
	}
	switch strings.ToLower(value.Scheme) {
	case "https":
		return "443"
	case "http":
		return "80"
	default:
		return ""
	}
}

func relatedPlatformSiteBaseURL(left string, right string) bool {
	leftURL, leftErr := normalizePlatformSiteURL(left)
	rightURL, rightErr := normalizePlatformSiteURL(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	leftParsed, _ := url.Parse(leftURL)
	rightParsed, _ := url.Parse(rightURL)
	leftHost := normalizePlatformHostname(leftParsed.Hostname())
	rightHost := normalizePlatformHostname(rightParsed.Hostname())
	if leftHost == "" || rightHost == "" {
		return false
	}
	if leftHost == rightHost {
		return true
	}
	if isLocalOrIPPlatformHost(leftHost) || isLocalOrIPPlatformHost(rightHost) {
		return false
	}
	leftDomain, leftOK := registrablePlatformDomain(leftHost)
	rightDomain, rightOK := registrablePlatformDomain(rightHost)
	return leftOK && rightOK && leftDomain == rightDomain
}

func normalizePlatformHostname(host string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
}

func isLocalOrIPPlatformHost(host string) bool {
	host = normalizePlatformHostname(host)
	return host == "localhost" || net.ParseIP(host) != nil
}

func registrablePlatformDomain(host string) (string, bool) {
	host = normalizePlatformHostname(host)
	if host == "" || isLocalOrIPPlatformHost(host) {
		return "", false
	}
	domain, err := publicsuffix.EffectiveTLDPlusOne(host)
	if err != nil {
		return "", false
	}
	return strings.ToLower(domain), true
}

func payloadPageCount(payload any, maxPages int) int {
	if maxPages <= 0 {
		maxPages = upstreamSiteMaxPages
	}
	record := firstRecord(payload)
	pages := firstInt64(record, "pages", "total_pages")
	if pages > 0 {
		if pages > int64(maxPages) {
			return maxPages + 1
		}
		return int(pages)
	}
	total := firstInt64(record, "total")
	pageSize := firstInt64(record, "page_size", "pageSize", "size")
	if total > 0 && pageSize > 0 {
		pageCount := (total-1)/pageSize + 1
		if pageCount > int64(maxPages) {
			return maxPages + 1
		}
		return int(pageCount)
	}
	return maxPages + 1
}

func platformSitePageHasMore(payload any, page, itemCount, maxPages int) bool {
	if itemCount == 0 {
		return false
	}
	pageCount := payloadPageCount(payload, maxPages)
	if pageCount <= maxPages {
		return page < pageCount
	}
	return itemCount >= upstreamSitePageSize
}

func nestedString(value map[string]any, path ...string) string {
	var current any = value
	for _, part := range path {
		switch typed := current.(type) {
		case map[string]any:
			current = typed[part]
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(typed) {
				return ""
			}
			current = typed[index]
		default:
			return ""
		}
	}
	text, _ := current.(string)
	return strings.TrimSpace(text)
}

func parseGroupRates(payload any) map[string]float64 {
	rates := make(map[string]float64)
	records := recordsFromPayload(payload)
	for _, record := range records {
		name := firstString(record, "name", "group", "group_name", "id")
		ratioKeysPresent := hasAnyField(
			record,
			"rate_multiplier",
			"ratio",
			"rate",
			"multiplier",
			"convert_ratio",
		)
		ratio := firstFloat(
			record,
			"rate_multiplier",
			"ratio",
			"rate",
			"multiplier",
			"convert_ratio",
		)
		if name != "" && ratioKeysPresent && isValidConversionRatio(ratio) {
			rates[name] = ratio
		}
	}
	if record := firstRecord(payload); len(record) > 0 {
		for key, value := range record {
			switch parsed := value.(type) {
			case float64:
				if isValidConversionRatio(parsed) {
					rates[key] = parsed
				}
			case map[string]any:
				ratio := firstFloat(
					parsed,
					"rate_multiplier",
					"ratio",
					"rate",
					"multiplier",
					"convert_ratio",
				)
				if isValidConversionRatio(ratio) {
					rates[key] = ratio
					if name := firstString(parsed, "name", "group", "group_name", "id"); name != "" {
						rates[name] = ratio
					}
				}
			case string:
				if ratio, err := strconv.ParseFloat(strings.TrimSpace(parsed), 64); err == nil && isValidConversionRatio(ratio) {
					rates[key] = ratio
				}
			}
		}
	}
	return rates
}

func isValidConversionRatio(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) &&
		value >= 0 && value <= model.MaxUpstreamConversionRatio
}
