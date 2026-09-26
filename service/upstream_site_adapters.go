package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/c1cadaBob/NexusTok/common"
	"github.com/c1cadaBob/NexusTok/model"
	"golang.org/x/net/publicsuffix"
)

const defaultNewAPIQuotaPerUnit = 500000

var sub2APIAppConfigAPIBaseURLPattern = regexp.MustCompile(`(?i)["']api_base_url["']\s*:\s*["']([^"']+)["']`)

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
			return 0, false, fmt.Errorf("%w: Sub2API 额度字段无效", ErrPlatformSiteResponse)
		}
		quota, err := sub2APIQuotaToInternal(value)
		if err != nil {
			return 0, false, err
		}
		return quota, true, nil
	}
	return 0, false, nil
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
	if adapter.client != nil {
		session.Client = adapter.client
	}
	switch platformSiteCredentialAuthType(credential) {
	case model.UpstreamAuthPassword:
		payload, requestErr := loginNewAPIWithPassword(ctx, session, credential)
		if requestErr != nil {
			return nil, wrapPlatformSiteStage("NewAPI 登录", requestErr)
		}
		if loginRequiresInteractiveVerification(payload) {
			return nil, fmt.Errorf("%w: 需要完成上游二次验证", ErrPlatformSiteAuth)
		}
		if token := findToken(payload); token != "" {
			session.Headers.Set("Authorization", bearerToken(token))
		}
		if userID := findUserID(payload); userID != "" {
			setNewAPICompatUserHeaders(session.Headers, userID)
		}
	case model.UpstreamAuthAccessToken:
		if credential.Cookie != "" {
			headers.Set("Cookie", credential.Cookie)
		}
		if credential.RefreshToken != "" || credential.SessionID != "" {
			if err := refreshPlatformSiteSession(
				ctx,
				session,
				"/api/user/auth/refresh",
				&credential,
			); err != nil {
				return session, wrapPlatformSiteStage("NewAPI 刷新令牌", err)
			}
		} else if credential.AccessToken != "" {
			headers.Set("Authorization", bearerToken(credential.AccessToken))
			setNewAPICompatUserHeaders(headers, credential.UserID)
		} else {
			return nil, wrapPlatformSiteStage("NewAPI 认证", fmt.Errorf("%w: 缺少访问令牌", ErrPlatformSiteAuth))
		}
	case model.UpstreamAuthAdminKey:
		if credential.AdminKey == "" {
			return nil, wrapPlatformSiteStage("NewAPI 认证", fmt.Errorf("%w: 缺少 Admin Key", ErrPlatformSiteAuth))
		}
		headers.Set("Authorization", bearerToken(credential.AdminKey))
		headers.Set("x-api-key", credential.AdminKey)
		headers.Set("New-Api-Key", credential.AdminKey)
	case model.UpstreamAuthCookie:
		if credential.Cookie == "" {
			return nil, wrapPlatformSiteStage("NewAPI 认证", fmt.Errorf("%w: 缺少 Cookie", ErrPlatformSiteAuth))
		}
		headers.Set("Cookie", credential.Cookie)
		setNewAPICompatUserHeaders(headers, credential.UserID)
	default:
		return nil, wrapPlatformSiteStage("NewAPI 认证", fmt.Errorf("%w: 认证方式不受支持", ErrPlatformSiteAuth))
	}
	currentUser, err := fetchNewAPICurrentUser(ctx, session)
	if err != nil {
		if credential.AuthType != model.UpstreamAuthAdminKey {
			return nil, wrapPlatformSiteStage("NewAPI 当前用户", err)
		}
		if _, adminErr := fetchNewAPIAdminChannels(ctx, session); adminErr != nil {
			return nil, wrapPlatformSiteStage("NewAPI 管理接口", adminErr)
		}
	} else if expectedUserID := strings.TrimSpace(credential.UserID); expectedUserID != "" {
		actualUserID := strings.TrimSpace(findUserID(currentUser))
		if actualUserID != "" && actualUserID != expectedUserID {
			return nil, wrapPlatformSiteStage("NewAPI 用户身份", ErrPlatformSiteIdentity)
		}
	}
	return session, nil
}

func (adapter *NewAPIAdapter) FetchSnapshot(ctx context.Context, session *PlatformSiteSession) (PlatformSiteSnapshot, error) {
	quotaPerUnit := fetchNewAPIQuotaPerUnit(ctx, session)
	selfPayload, err := fetchNewAPICurrentUser(ctx, session)
	isAdminKey := session != nil && strings.TrimSpace(session.Headers.Get("x-api-key")) != ""
	if err != nil && !isAdminKey {
		return PlatformSiteSnapshot{}, wrapPlatformSiteStage("NewAPI 当前用户", err)
	}
	self := firstNestedRecord(selfPayload, "user", "account", "profile")
	usedQuota, usedQuotaSet, quotaErr := firstValidatedInternalQuota(
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
	if quotaErr != nil {
		return PlatformSiteSnapshot{}, quotaErr
	}
	balanceValue, _, balanceErr := firstValidatedNonNegativeFloat(
		self,
		"quota",
		"balance",
		"money",
		"credit",
	)
	if balanceErr != nil {
		return PlatformSiteSnapshot{}, balanceErr
	}
	snapshot := PlatformSiteSnapshot{
		Balance:      normalizeNewAPIQuota(balanceValue, quotaPerUnit),
		UsedQuota:    usedQuota,
		UsedQuotaSet: usedQuotaSet,
		KeysComplete: true,
	}
	if accountModels := fetchNewAPIModels(ctx, session); len(accountModels) > 0 {
		snapshot.Models = accountModels
		snapshot.ResourceSyncs = append(snapshot.ResourceSyncs,
			PlatformSiteResourceSyncSnapshot{
				ResourceType:   model.PlatformSiteResourceModels,
				Status:         model.PlatformSiteResourceStatusSuccess,
				SourceEndpoint: "/api/user/models",
				RecordCount:    len(accountModels),
			},
		)
	}
	if !isAdminKey || err == nil {
		snapshot.Identity = platformSiteIdentityFromRecord(
			self,
			fmt.Sprintf("%.0f", quotaPerUnit),
			"/api/user/self",
		)
		snapshot.Identity.SourceEndpoint = "/api/user/self"
	}
	snapshot.Endpoint = newAPIEndpointSnapshot(session)
	snapshot.ResourceSyncs = append(snapshot.ResourceSyncs,
		PlatformSiteResourceSyncSnapshot{
			ResourceType:   model.PlatformSiteResourceUsage,
			Status:         model.PlatformSiteResourceStatusSuccess,
			SourceEndpoint: "/api/status,/api/user/self",
			RecordCount:    1,
		},
	)
	if snapshot.Identity != nil {
		snapshot.ResourceSyncs = append(snapshot.ResourceSyncs, PlatformSiteResourceSyncSnapshot{
			ResourceType:   model.PlatformSiteResourceIdentity,
			Status:         model.PlatformSiteResourceStatusSuccess,
			SourceEndpoint: "/api/user/self",
			RecordCount:    1,
		})
	}
	if isAdminKey {
		adminChannels, adminErr := fetchNewAPIAdminChannels(ctx, session)
		if adminErr == nil {
			adminModels := make([]string, 0)
			for _, channel := range adminChannels {
				adminModels = append(
					adminModels,
					fetchNewAPIAdminChannelModels(ctx, session, channel)...,
				)
			}
			if len(adminModels) > 0 {
				snapshot.Models = uniqueStrings(append(snapshot.Models, adminModels...))
			}
			snapshot.ResourceSyncs = append(snapshot.ResourceSyncs, PlatformSiteResourceSyncSnapshot{
				ResourceType:   model.PlatformSiteResourceModels,
				Status:         model.PlatformSiteResourceStatusSuccess,
				SourceEndpoint: "/api/channel/,/api/channel/{id},/api/channel/fetch_models/{id}",
				RecordCount:    len(adminModels),
			})
		} else {
			snapshot.ResourceSyncs = append(snapshot.ResourceSyncs, PlatformSiteResourceSyncSnapshot{
				ResourceType:   model.PlatformSiteResourceModels,
				Status:         model.PlatformSiteResourceStatusStale,
				SourceEndpoint: "/api/channel/",
				FailureReason:  "管理员渠道接口不可用，保留最近成功模型快照",
				Partial:        true,
			})
		}
	}
	groupRates, groupSnapshots, groupsLoaded, groupEndpoint := fetchNewAPIGroupResources(ctx, session)
	snapshot.Groups = groupSnapshots
	snapshot.GroupsLoaded = groupsLoaded
	if groupsLoaded {
		snapshot.ResourceSyncs = append(snapshot.ResourceSyncs, PlatformSiteResourceSyncSnapshot{
			ResourceType:   model.PlatformSiteResourceGroups,
			Status:         model.PlatformSiteResourceStatusSuccess,
			SourceEndpoint: groupEndpoint,
			RecordCount:    len(groupSnapshots),
		})
	} else {
		snapshot.ResourceSyncs = append(snapshot.ResourceSyncs, PlatformSiteResourceSyncSnapshot{
			ResourceType:   model.PlatformSiteResourceGroups,
			Status:         model.PlatformSiteResourceStatusStale,
			SourceEndpoint: "/api/user/self/groups,/api/user/groups",
			FailureReason:  "上游分组接口不可用",
		})
	}
	pricingPayload, pricingErr := platformSiteRequest(ctx, session, http.MethodGet, "/api/pricing", nil, nil)
	if pricingErr == nil {
		applyNewAPIPricingResources(&snapshot, session, pricingPayload)
	}
	tokens, err := fetchNewAPITokens(ctx, session)
	if err != nil {
		return PlatformSiteSnapshot{}, wrapPlatformSiteStage("NewAPI 密钥分页", err)
	}
	revealedKeys, revealFailures := fetchNewAPITokenKeys(ctx, session, tokens)
	snapshot.Keys = make([]UpstreamKeySnapshot, 0, len(tokens))
	snapshot.KeysComplete = true
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
				item.SyncError = upstreamKeySyncErrorModelsUnavailable
				snapshot.KeysComplete = false
			}
		}
		snapshot.Keys = append(snapshot.Keys, item)
	}
	keysResourceStatus := model.PlatformSiteResourceStatusSuccess
	keysFailureReason := ""
	keysPartial := false
	if !snapshot.KeysComplete {
		keysResourceStatus = model.PlatformSiteResourceStatusPartial
		keysFailureReason = "部分密钥详情或模型能力读取失败，已保留最近成功快照"
		keysPartial = true
	}
	snapshot.ResourceSyncs = append(snapshot.ResourceSyncs, PlatformSiteResourceSyncSnapshot{
		ResourceType:   model.PlatformSiteResourceKeys,
		Status:         keysResourceStatus,
		SourceEndpoint: "/api/token/,/api/token/batch/keys",
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
			SourceEndpoint: "/api/user/models,/v1/models",
			FailureReason:  "账号级或密钥级模型目录接口不可用",
		})
	}
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
	if adapter.client != nil {
		session.Client = adapter.client
	}
	setSub2APIBrowserHeaders(session)
	if modelBaseURL, ok := discoverSub2APIModelBaseURL(ctx, session); ok {
		session.ModelBaseURL = modelBaseURL
	}
	if managementBaseURL, modelBaseURL, ok := discoverSub2APIManagementBaseURL(
		ctx,
		session,
	); ok {
		session.BaseURL = managementBaseURL
		session.ManagementBaseURL = managementBaseURL
		setSub2APIBrowserHeaders(session)
		session.ModelBaseURL = modelBaseURL
	} else {
		session.ManagementBaseURL = session.BaseURL
	}
	switch platformSiteCredentialAuthType(credential) {
	case model.UpstreamAuthPassword:
		payload, requestErr := loginSub2APIWithPassword(ctx, session, credential)
		if requestErr != nil {
			return nil, wrapPlatformSiteStage("Sub2API 登录", requestErr)
		}
		if loginRequiresInteractiveVerification(payload) {
			return nil, fmt.Errorf("%w: 需要完成上游二次验证", ErrPlatformSiteAuth)
		}
		if token := findToken(payload); token != "" {
			session.Headers.Set("Authorization", bearerToken(token))
		} else {
			return nil, wrapPlatformSiteStage("Sub2API 登录未返回访问令牌", ErrSub2APILoginToken)
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
		return nil, wrapPlatformSiteStage("Sub2API 当前用户", err)
	}
	if expectedUserID := strings.TrimSpace(credential.UserID); expectedUserID != "" {
		actualUserID := strings.TrimSpace(findUserID(currentUser))
		if actualUserID != "" && actualUserID != expectedUserID {
			return nil, wrapPlatformSiteStage("Sub2API 用户身份", ErrPlatformSiteIdentity)
		}
	}
	return session, nil
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
	usedQuota, usedQuotaSet, quotaErr := firstSub2APIQuota(
		me,
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
	)
	if quotaErr != nil {
		return PlatformSiteSnapshot{}, quotaErr
	}
	snapshot := PlatformSiteSnapshot{
		Balance:           firstFloat(me, "balance", "quota", "credit"),
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
		PlatformSiteResourceSyncSnapshot{
			ResourceType:   model.PlatformSiteResourceUsage,
			Status:         model.PlatformSiteResourceStatusSuccess,
			SourceEndpoint: "/api/v1/auth/me",
			RecordCount:    1,
		},
	)
	if payload, requestErr := platformSiteRequest(ctx, session, http.MethodGet, "/api/v1/user/profile", nil, nil); requestErr == nil {
		profile := firstNestedRecord(payload, "profile", "user", "account")
		if balance := firstFloat(profile, "balance", "quota", "credit"); balance > 0 {
			snapshot.Balance = balance
		}
		if !snapshot.UsedQuotaSet {
			if profileUsedQuota, profileUsedQuotaSet, profileQuotaErr := firstSub2APIQuota(
				profile,
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
			); profileQuotaErr != nil {
				return PlatformSiteSnapshot{}, profileQuotaErr
			} else if profileUsedQuotaSet {
				snapshot.UsedQuota = profileUsedQuota
				snapshot.UsedQuotaSet = true
			}
		}
	}
	usageSource := "/api/v1/usage/dashboard/stats"
	if !snapshot.UsedQuotaSet {
		if payload, requestErr := platformSiteRequest(ctx, session, http.MethodGet, "/api/v1/usage/stats", nil, nil); requestErr == nil {
			usage := firstNestedRecord(payload, "stats", "usage", "dashboard")
			if used, usedSet, usageQuotaErr := firstSub2APIQuota(usage, "total_actual_cost", "totalActualCost", "total_cost", "totalCost", "total_used", "totalUsed"); usageQuotaErr != nil {
				return PlatformSiteSnapshot{}, usageQuotaErr
			} else if usedSet {
				snapshot.UsedQuota = used
				snapshot.UsedQuotaSet = true
				usageSource = "/api/v1/usage/stats"
			}
		}
	}
	if !snapshot.UsedQuotaSet {
		if payload, requestErr := platformSiteRequest(ctx, session, http.MethodGet, "/api/v1/usage/dashboard/stats", nil, nil); requestErr == nil {
			usage := firstNestedRecord(payload, "stats", "usage", "dashboard")
			if used, usedSet, usageQuotaErr := firstSub2APIQuota(
				usage,
				"total_actual_cost",
				"totalActualCost",
				"total_cost",
				"totalCost",
				"total_used",
				"total_used_quota",
				"totalUsedQuota",
				"totalUsed",
				"used_quota",
				"quota_used",
				"quotaUsed",
				"used",
			); usageQuotaErr != nil {
				return PlatformSiteSnapshot{}, usageQuotaErr
			} else if usedSet {
				snapshot.UsedQuota = used
				snapshot.UsedQuotaSet = true
			} else if used, usedSet, usageQuotaErr := firstSub2APIQuota(
				usage,
				"today_actual_cost",
				"todayActualCost",
				"today_cost",
				"todayCost",
			); usageQuotaErr != nil {
				return PlatformSiteSnapshot{}, usageQuotaErr
			} else if usedSet {
				snapshot.UsedQuota = used
				snapshot.UsedQuotaSet = true
			}
		}
	}
	snapshot.ResourceSyncs = append(snapshot.ResourceSyncs, PlatformSiteResourceSyncSnapshot{
		ResourceType:   model.PlatformSiteResourceUsage,
		Status:         model.PlatformSiteResourceStatusSuccess,
		SourceEndpoint: usageSource,
		RecordCount:    1,
	})
	rates := map[string]float64{}
	groupPayload, groupLoaded := any(nil), false
	if payload, requestErr := platformSiteRequest(ctx, session, http.MethodGet, "/api/v1/groups/available", nil, nil); requestErr == nil {
		groupPayload = payload
		groupLoaded = true
		for key, value := range parseGroupRates(payload) {
			rates[key] = value
		}
	}
	if payload, requestErr := platformSiteRequest(ctx, session, http.MethodGet, "/api/v1/groups/rates", nil, nil); requestErr == nil {
		for key, value := range parseGroupRates(payload) {
			rates[key] = value
		}
	}
	snapshot.Groups = parsePlatformSiteGroups(groupPayload, "/api/v1/groups/available", rates)
	snapshot.GroupsLoaded = groupLoaded
	if groupLoaded {
		snapshot.ResourceSyncs = append(snapshot.ResourceSyncs, PlatformSiteResourceSyncSnapshot{
			ResourceType:   model.PlatformSiteResourceGroups,
			Status:         model.PlatformSiteResourceStatusSuccess,
			SourceEndpoint: "/api/v1/groups/available,/api/v1/groups/rates",
			RecordCount:    len(snapshot.Groups),
		})
	}
	if session.Headers.Get("x-api-key") != "" {
		snapshot.Keys, err = fetchSub2APIAdminKeys(ctx, session, rates)
	} else {
		snapshot.Keys, err = fetchSub2APIKeys(ctx, session, rates)
	}
	if err != nil {
		if errors.Is(err, ErrPlatformSiteSecurity) {
			snapshot.KeysComplete = false
			snapshot.AuthStatus = model.PlatformSiteAuthStatusSecureVerificationRequired
			snapshot.AuthStatusReason = "读取管理员密钥需要完成上游安全验证"
			snapshot.ResourceSyncs = append(snapshot.ResourceSyncs,
				PlatformSiteResourceSyncSnapshot{
					ResourceType:                 model.PlatformSiteResourceKeys,
					Status:                       model.PlatformSiteResourceStatusSecureVerificationRequired,
					SourceEndpoint:               "/api/v1/admin/accounts,/api/v1/admin/accounts/data",
					FailureReason:                "上游拒绝 Admin Key 的安全验证",
					Partial:                      true,
					RequiresSecurityVerification: true,
				},
				PlatformSiteResourceSyncSnapshot{
					ResourceType:                 model.PlatformSiteResourceModels,
					Status:                       model.PlatformSiteResourceStatusSecureVerificationRequired,
					SourceEndpoint:               "/v1/models",
					FailureReason:                "密钥资源未完成安全验证，保留最近成功模型快照",
					Partial:                      true,
					RequiresSecurityVerification: true,
				},
			)
			return snapshot, nil
		}
		return PlatformSiteSnapshot{}, wrapPlatformSiteStage("Sub2API 密钥分页", err)
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

func loginNewAPIWithPassword(ctx context.Context, session *PlatformSiteSession, credential model.PlatformSiteCredential) (any, error) {
	username := strings.TrimSpace(credential.Username)
	if username == "" || credential.Password == "" {
		return nil, fmt.Errorf("%w: 缺少账号密码", ErrPlatformSiteAuth)
	}
	payload, err := platformSiteRequest(
		ctx,
		session,
		http.MethodPost,
		"/api/user/login?turnstile=",
		nil,
		map[string]string{"username": username, "password": credential.Password},
	)
	if err != nil {
		return nil, err
	}
	return payload, nil
}

func loginSub2APIWithPassword(ctx context.Context, session *PlatformSiteSession, credential model.PlatformSiteCredential) (any, error) {
	username := strings.TrimSpace(credential.Username)
	if username == "" || credential.Password == "" {
		return nil, fmt.Errorf("%w: 缺少账号密码", ErrPlatformSiteAuth)
	}
	parsedEmail, err := mail.ParseAddress(username)
	if err != nil || parsedEmail.Address != username {
		return nil, ErrSub2APILoginEmail
	}
	payload, requestErr := platformSiteRequest(
		ctx,
		session,
		http.MethodPost,
		"/api/v1/auth/login",
		nil,
		map[string]string{"email": parsedEmail.Address, "password": credential.Password},
	)
	if requestErr != nil {
		return nil, classifySub2APILoginError(requestErr)
	}
	return payload, nil
}

func classifySub2APILoginError(err error) error {
	if err == nil {
		return nil
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
		if len(items) == 0 ||
			len(items) < upstreamSitePageSize ||
			page >= payloadPageCount(payload) {
			return channels, nil
		}
	}
	return nil, errors.New("NewAPI 管理渠道分页超过安全上限")
}

func fetchNewAPIAdminChannelModels(
	ctx context.Context,
	session *PlatformSiteSession,
	channel map[string]any,
) []string {
	models := modelsFromRecord(channel)
	channelID := firstString(channel, "id", "channel_id", "channelId")
	if channelID == "" {
		return uniqueStrings(models)
	}
	if payload, err := platformSiteRequest(
		ctx,
		session,
		http.MethodGet,
		"/api/channel/"+url.PathEscape(channelID),
		nil,
		nil,
	); err == nil {
		models = append(models, modelsFromRecord(firstRecord(payload))...)
	}
	if payload, err := platformSiteRequest(
		ctx,
		session,
		http.MethodGet,
		"/api/channel/fetch_models/"+url.PathEscape(channelID),
		nil,
		nil,
	); err == nil {
		models = append(models, stringsFromPayload(payload)...)
	}
	return uniqueStrings(models)
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
	if credential == nil ||
		(path != "/api/user/auth/refresh" && strings.TrimSpace(credential.RefreshToken) == "" &&
			strings.TrimSpace(credential.SessionID) == "") ||
		(path == "/api/v1/auth/refresh" && strings.TrimSpace(credential.RefreshToken) == "") {
		return fmt.Errorf("%w: 缺少刷新令牌", ErrPlatformSiteAuth)
	}
	if strings.TrimSpace(credential.AccessToken) != "" {
		session.Headers.Set("Authorization", bearerToken(credential.AccessToken))
	}
	isSub2API := path == "/api/v1/auth/refresh"
	isDashboardRefresh := !isSub2API &&
		(strings.TrimSpace(credential.SessionID) != "" ||
			(strings.TrimSpace(credential.Cookie) != "" && strings.TrimSpace(credential.RefreshToken) == ""))
	var body any
	if !isDashboardRefresh {
		body = map[string]string{"refresh_token": credential.RefreshToken}
	}
	payload, err := platformSiteRequest(ctx, session, http.MethodPost, path, nil, body)
	if err != nil {
		setPlatformSiteCredentialUpdate(session, *credential)
		if isSub2API {
			markPlatformSiteRefreshFailure(credential, platformSiteHTTPStatusIsUnauthorized(err))
		} else {
			markPlatformSiteRefreshFailure(credential, false)
		}
		setPlatformSiteCredentialUpdate(session, *credential)
		return fmt.Errorf("%w: 刷新会话失败", ErrPlatformSiteAuth)
	}
	if isDashboardRefresh {
		recognized, bundleErr := applyNewAPIDashboardAuthBundle(payload, credential)
		if recognized {
			if bundleErr != nil {
				markPlatformSiteRefreshFailure(credential, false)
				setPlatformSiteCredentialUpdate(session, *credential)
				return bundleErr
			}
			capturePlatformSiteSessionCookie(session, credential)
			session.Headers.Set("Authorization", bearerToken(credential.AccessToken))
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
		markPlatformSiteRefreshFailure(credential, false)
		setPlatformSiteCredentialUpdate(session, *credential)
		return fmt.Errorf("%w: 刷新会话响应不完整", ErrPlatformSiteAuth)
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
	session.Headers.Set("Authorization", bearerToken(accessToken))
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
	recognized := hasAnyField(data, "access_token", "token_type", "access_expires_at") ||
		(sessionOK && hasAnyField(session, "sid", "current"))
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
	credential.AuthType = model.UpstreamAuthAccessToken
	credential.Username = ""
	credential.Password = ""
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

func fetchNewAPIQuotaPerUnit(ctx context.Context, session *PlatformSiteSession) float64 {
	payload, err := platformSiteRequest(ctx, session, http.MethodGet, "/api/status", nil, nil)
	if err != nil {
		return defaultNewAPIQuotaPerUnit
	}
	quotaPerUnit := firstFloat(firstRecord(payload), "quota_per_unit", "quotaPerUnit")
	if quotaPerUnit <= 0 || math.IsNaN(quotaPerUnit) || math.IsInf(quotaPerUnit, 0) {
		return defaultNewAPIQuotaPerUnit
	}
	return quotaPerUnit
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
		Username:       firstString(record, "username", "user_name", "login"),
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
) (map[string]float64, []PlatformSiteGroupSnapshot, bool, string) {
	rates := make(map[string]float64)
	groups := make([]PlatformSiteGroupSnapshot, 0)
	loaded := false
	sources := make([]string, 0, 2)
	for _, path := range []string{"/api/user/self/groups", "/api/user/groups"} {
		payload, err := platformSiteRequest(ctx, session, http.MethodGet, path, nil, nil)
		if err != nil {
			if platformSiteRouteMissing(err) {
				continue
			}
			break
		}
		loaded = true
		sources = append(sources, path)
		for key, value := range parseGroupRates(payload) {
			rates[key] = value
		}
		groups = mergePlatformSiteGroups(groups, parsePlatformSiteGroups(payload, path, rates))
	}
	return rates, groups, loaded, strings.Join(sources, ",")
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
) {
	if snapshot == nil {
		return
	}
	record := firstRecord(payload)
	if len(record) == 0 {
		return
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
		groupID := firstString(record, "id", "group_id", "groupId", "external_id", "externalId")
		groupName := firstString(record, "name", "group_name", "groupName", "group")
		if groupID == "" {
			groupID = groupName
		}
		if groupID == "" {
			continue
		}
		ratioSet := hasAnyField(record, "rate_multiplier", "ratio", "rate", "multiplier", "group_ratio")
		ratio := firstFloat(record, "rate_multiplier", "ratio", "rate", "multiplier", "group_ratio")
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

func fetchNewAPIModels(ctx context.Context, session *PlatformSiteSession) []string {
	for _, path := range []string{"/api/user/models"} {
		payload, err := platformSiteRequest(ctx, session, http.MethodGet, path, nil, nil)
		if err != nil {
			continue
		}
		if models := uniqueStrings(append(stringsFromPayload(payload), modelsFromGroups(payload)...)); len(models) > 0 {
			return models
		}
	}
	return nil
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
			keySession.Headers = session.Headers.Clone()
			keySession.Headers.Del("x-api-key")
			keySession.Headers.Del("New-Api-Key")
			keySession.Headers.Del("Cookie")
			keySession.Headers.Set("Authorization", bearerToken(secret))
			keySession.Headers.Set("x-api-key", secret)
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

func fetchNewAPITokens(ctx context.Context, session *PlatformSiteSession) ([]map[string]any, error) {
	result := make([]map[string]any, 0)
	for page := 1; page <= upstreamSiteMaxPages; page++ {
		var payload any
		var err error
		for _, path := range []string{"/api/token/", "/api/token", "/api/tokens"} {
			payload, err = platformSiteRequest(ctx, session, http.MethodGet, path, url.Values{
				"p":         {fmt.Sprint(page)},
				"page":      {fmt.Sprint(page)},
				"page_size": {fmt.Sprint(upstreamSitePageSize)},
				"size":      {fmt.Sprint(upstreamSitePageSize)},
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
		if len(pageItems) == 0 || len(pageItems) < upstreamSitePageSize || page >= payloadPageCount(payload) {
			return result, nil
		}
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
	if len(ids) > 0 {
		payload, err := platformSiteRequest(ctx, session, http.MethodPost, "/api/token/batch/keys", nil, map[string]any{
			"ids": ids,
		})
		if err != nil {
			batchErr = err
		} else {
			for id, key := range stringsMapFromPayload(payload) {
				result[id] = key
			}
		}
	}
	var fallbackErr error
	for _, token := range tokens {
		externalID := firstString(token, "id", "token_id", "key_id")
		if externalID == "" || strings.TrimSpace(result[externalID]) != "" {
			continue
		}
		secret := firstString(token, "key", "token", "api_key")
		if secret != "" && !strings.Contains(secret, "*") {
			result[externalID] = secret
			continue
		}
		key, err := fetchNewAPITokenKey(ctx, session, externalID)
		if err != nil {
			fallbackErr = err
			failures[externalID] = err
			continue
		}
		result[externalID] = key
	}
	for _, token := range tokens {
		externalID := firstString(token, "id", "token_id", "key_id")
		if externalID == "" || strings.TrimSpace(result[externalID]) != "" {
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
	if err != nil && platformSiteRouteMissing(err) {
		payload, err = platformSiteRequest(ctx, session, http.MethodGet, path, nil, nil)
	}
	if err != nil {
		return "", err
	}
	key := stringFromPayload(payload)
	if key == "" {
		return "", fmt.Errorf("%w: 完整 Key 为空", ErrPlatformSiteResponse)
	}
	return key, nil
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
	for page := 1; page <= upstreamSiteMaxPages; page++ {
		payload, err := platformSiteRequest(ctx, session, http.MethodGet, "/api/v1/keys", url.Values{
			"page":      {fmt.Sprint(page)},
			"page_size": {fmt.Sprint(upstreamSitePageSize)},
		}, nil)
		if err != nil {
			return nil, err
		}
		items := recordsFromPayload(payload)
		for _, item := range items {
			externalID := firstString(item, "id", "key_id")
			if externalID == "" {
				return nil, fmt.Errorf("%w: Sub2API 密钥缺少外部 ID", ErrPlatformSiteResponse)
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
				return nil, quotaErr
			}
			remainQuota, remainErr := sub2APIRemainQuota(item)
			if remainErr != nil {
				return nil, remainErr
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
			secret := firstString(item, "key", "api_key", "token")
			if secret == "" || strings.Contains(secret, "*") {
				detail, detailErr := fetchSub2APIKeyDetail(ctx, session, externalID)
				if detailErr == nil {
					if len(keySnapshot.Models) == 0 {
						keySnapshot.Models = modelsFromRecord(detail)
						keySnapshot.ModelsSynced = len(keySnapshot.Models) > 0
					}
					secret = stringFromPayload(detail)
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
		if len(items) == 0 || len(items) < upstreamSitePageSize || page >= payloadPageCount(payload) {
			break
		}
	}
	return result, nil
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
	for page := 1; page <= upstreamSiteMaxPages; page++ {
		payload, err := platformSiteRequest(ctx, session, http.MethodGet, "/api/v1/admin/accounts", url.Values{
			"page":       {fmt.Sprint(page)},
			"page_size":  {fmt.Sprint(upstreamSitePageSize)},
			"sort_by":    {"name"},
			"sort_order": {"asc"},
			"type":       {"apikey"},
		}, nil)
		if err != nil {
			return nil, err
		}
		items := recordsFromPayload(payload)
		for _, item := range items {
			id := firstString(item, "id", "account_id")
			if id == "" {
				return nil, fmt.Errorf("%w: Sub2API 密钥缺少外部 ID", ErrPlatformSiteResponse)
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
				return nil, quotaErr
			}
			remainQuota, remainErr := sub2APIRemainQuota(item)
			if remainErr != nil {
				return nil, remainErr
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
			dataPayload, dataErr := platformSiteRequest(ctx, session, http.MethodGet, "/api/v1/admin/accounts/data", url.Values{
				"ids":             {id},
				"include_proxies": {"false"},
			}, nil)
			if dataErr != nil {
				if errors.Is(dataErr, ErrPlatformSiteSecurity) {
					return nil, dataErr
				}
				keySnapshot.SyncError = upstreamKeySyncErrorSecretUnavailable
				result = append(result, keySnapshot)
				continue
			}
			data := firstRecord(dataPayload)
			secret := nestedString(data, "accounts", "0", "credentials", "api_key")
			if secret == "" {
				secret = firstString(data, "key", "api_key", "token")
			}
			if secret == "" {
				secret = nestedString(item, "credentials", "api_key")
			}
			if secret == "" {
				secret = firstString(item, "key", "api_key", "token")
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
		if len(items) == 0 || len(items) < upstreamSitePageSize || page >= payloadPageCount(payload) {
			break
		}
	}
	return result, nil
}

func sub2APIRemainQuota(record map[string]any) (*int64, error) {
	if boolFromRecord(record, "unlimited", "unlimited_quota", "unlimitedQuota") {
		return nil, nil
	}
	if hasAnyField(record, "quota") {
		quota, quotaSet, err := firstValidatedNonNegativeFloat(record, "quota")
		if err != nil {
			return nil, err
		}
		if !quotaSet {
			return nil, nil
		}
		if quota <= 0 {
			return nil, nil
		}
		used, usedSet, err := firstValidatedNonNegativeFloat(
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
	if remain, ok, err := firstValidatedNonNegativeFloat(
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
	normalized, ok := discoverSub2APIPageModelBaseURL(ctx, session.Client, session.BaseURL)
	if !ok {
		return "", false
	}
	if validatePlatformSiteURL(normalized) != nil {
		return "", false
	}
	if !relatedPlatformSiteBaseURL(session.BaseURL, normalized) {
		return "", false
	}
	return normalized, true
}

func discoverSub2APIManagementBaseURL(
	ctx context.Context,
	session *PlatformSiteSession,
) (string, string, bool) {
	if session == nil || session.Client == nil {
		return "", "", false
	}
	originalBaseURL, err := normalizePlatformSiteURL(session.BaseURL)
	if err != nil {
		return "", "", false
	}
	managementBaseURL, ok := sub2APIManagementBaseURLCandidate(originalBaseURL)
	if !ok {
		return "", "", false
	}
	anonymousClient := *session.Client
	anonymousClient.Jar = nil
	modelBaseURL, ok := discoverSub2APIPageModelBaseURL(
		ctx,
		&anonymousClient,
		managementBaseURL,
	)
	if !ok || !samePlatformSiteOrigin(originalBaseURL, modelBaseURL) {
		return "", "", false
	}
	return managementBaseURL, modelBaseURL, true
}

func discoverSub2APIPageModelBaseURL(
	ctx context.Context,
	client *http.Client,
	pageBaseURL string,
) (string, bool) {
	if client == nil || strings.TrimSpace(pageBaseURL) == "" {
		return "", false
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, pageBaseURL, nil)
	if err != nil {
		return "", false
	}
	request.Header.Set("Accept", "text/html,application/xhtml+xml")
	response, err := client.Do(request)
	if err != nil {
		return "", false
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", false
	}
	contentType := strings.ToLower(response.Header.Get("Content-Type"))
	if !strings.Contains(contentType, "text/html") &&
		!strings.Contains(contentType, "application/xhtml+xml") {
		return "", false
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, upstreamSiteResponseLimit+1))
	if err != nil || len(data) > upstreamSiteResponseLimit {
		return "", false
	}
	match := sub2APIAppConfigAPIBaseURLPattern.FindStringSubmatch(string(data))
	if len(match) < 2 {
		return "", false
	}
	candidate := strings.TrimSpace(strings.ReplaceAll(match[1], `\/`, `/`))
	if decoded, err := url.QueryUnescape(candidate); err == nil {
		candidate = decoded
	}
	if parsedCandidate, err := url.Parse(candidate); err == nil && !parsedCandidate.IsAbs() {
		baseURL, baseErr := url.Parse(pageBaseURL)
		if baseErr != nil {
			return "", false
		}
		candidate = baseURL.ResolveReference(parsedCandidate).String()
	}
	normalized, err := normalizePlatformSiteURL(candidate)
	if err != nil {
		return "", false
	}
	return normalized, true
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

func payloadPageCount(payload any) int {
	record := firstRecord(payload)
	pages := firstInt64(record, "pages", "total_pages")
	if pages > 0 {
		if pages > int64(upstreamSiteMaxPages) {
			return upstreamSiteMaxPages + 1
		}
		return int(pages)
	}
	total := firstInt64(record, "total")
	pageSize := firstInt64(record, "page_size", "pageSize", "size")
	if total > 0 && pageSize > 0 {
		pageCount := (total-1)/pageSize + 1
		if pageCount > int64(upstreamSiteMaxPages) {
			return upstreamSiteMaxPages + 1
		}
		return int(pageCount)
	}
	return upstreamSiteMaxPages + 1
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
		ratioKeysPresent := hasAnyField(record, "rate_multiplier", "ratio", "rate", "multiplier")
		ratio := firstFloat(record, "rate_multiplier", "ratio", "rate", "multiplier")
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
				ratio := firstFloat(parsed, "rate_multiplier", "ratio", "rate", "multiplier")
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
