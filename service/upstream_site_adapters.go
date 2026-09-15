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
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/c1cadaBob/NexusTok/model"
	"golang.org/x/net/publicsuffix"
)

const defaultNewAPIQuotaPerUnit = 500000

var sub2APIAppConfigAPIBaseURLPattern = regexp.MustCompile(`(?i)["']api_base_url["']\s*:\s*["']([^"']+)["']`)

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
	if credential.RefreshToken != "" && credential.Username == "" &&
		credential.AdminKey == "" && credential.Cookie == "" {
		if err := refreshPlatformSiteSession(
			ctx,
			session,
			"/api/user/auth/refresh",
			&credential,
		); err != nil {
			return nil, wrapPlatformSiteStage("NewAPI 刷新令牌", err)
		}
	}
	if credential.AccessToken != "" {
		headers.Set("Authorization", bearerToken(credential.AccessToken))
	}
	if credential.AdminKey != "" {
		headers.Set("Authorization", bearerToken(credential.AdminKey))
		headers.Set("x-api-key", credential.AdminKey)
		headers.Set("New-Api-Key", credential.AdminKey)
	}
	if credential.Cookie != "" {
		headers.Set("Cookie", credential.Cookie)
	}
	if credential.AccessToken == "" && credential.AdminKey == "" && credential.Cookie == "" {
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
		if refreshToken := findRefreshToken(payload); refreshToken != "" {
			updatedCredential := credential
			updatedCredential.AccessToken = ""
			updatedCredential.RefreshToken = refreshToken
			updatedCredential.TokenExpiresAt = findTokenExpiresAt(payload)
			session.CredentialUpdate = &updatedCredential
		}
	}
	if _, err := fetchNewAPICurrentUser(ctx, session); err != nil {
		return nil, wrapPlatformSiteStage("NewAPI 当前用户", err)
	}
	return session, nil
}

func (adapter *NewAPIAdapter) FetchSnapshot(ctx context.Context, session *PlatformSiteSession) (PlatformSiteSnapshot, error) {
	quotaPerUnit := fetchNewAPIQuotaPerUnit(ctx, session)
	selfPayload, err := fetchNewAPICurrentUser(ctx, session)
	if err != nil {
		return PlatformSiteSnapshot{}, wrapPlatformSiteStage("NewAPI 当前用户", err)
	}
	self := firstNestedRecord(selfPayload, "user", "account", "profile")
	snapshot := PlatformSiteSnapshot{
		Balance:   normalizeNewAPIQuota(firstFloat(self, "quota", "balance", "money", "credit"), quotaPerUnit),
		UsedQuota: firstInt64(self, "used_quota", "used", "used_quota_amount"),
	}
	groupRates := fetchNewAPIGroupRates(ctx, session)
	tokens, err := fetchNewAPITokens(ctx, session)
	if err != nil {
		return PlatformSiteSnapshot{}, wrapPlatformSiteStage("NewAPI 密钥分页", err)
	}
	revealedKeys, revealFailures := fetchNewAPITokenKeys(ctx, session, tokens)
	snapshot.Keys = make([]UpstreamKeySnapshot, 0, len(tokens))
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
			UsedQuota:                firstInt64(token, "used_quota", "used", "quota_used"),
			RemainQuota:              newAPIRemainQuota(token),
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
				snapshot.Keys = append(snapshot.Keys, item)
				continue
			}
		}
		if secret == "" || strings.Contains(secret, "*") {
			item.SyncError = upstreamKeySyncErrorSecretUnavailable
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
			}
		}
		snapshot.Keys = append(snapshot.Keys, item)
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
	if modelBaseURL, ok := discoverSub2APIModelBaseURL(ctx, session); ok {
		session.ModelBaseURL = modelBaseURL
	}
	if credential.RefreshToken != "" && credential.Username == "" &&
		credential.AdminKey == "" && credential.Cookie == "" {
		if err := refreshPlatformSiteSession(
			ctx,
			session,
			"/api/v1/auth/refresh",
			&credential,
		); err != nil {
			return nil, wrapPlatformSiteStage("Sub2API 刷新令牌", err)
		}
	}
	switch {
	case credential.AdminKey != "":
		headers.Set("Authorization", bearerToken(credential.AdminKey))
		headers.Set("x-api-key", credential.AdminKey)
	case credential.AccessToken != "":
		headers.Set("Authorization", bearerToken(credential.AccessToken))
	case credential.Cookie != "":
		headers.Set("Cookie", credential.Cookie)
	}
	if credential.AdminKey == "" && credential.AccessToken == "" && credential.Cookie == "" {
		payload, requestErr := loginSub2APIWithPassword(ctx, session, credential)
		if requestErr != nil {
			return nil, wrapPlatformSiteStage("Sub2API 登录", requestErr)
		}
		if loginRequiresInteractiveVerification(payload) {
			return nil, fmt.Errorf("%w: 需要完成上游二次验证", ErrPlatformSiteAuth)
		}
		if token := findToken(payload); token != "" {
			session.Headers.Set("Authorization", bearerToken(token))
		}
		if refreshToken := findRefreshToken(payload); refreshToken != "" {
			updatedCredential := credential
			updatedCredential.AccessToken = ""
			updatedCredential.RefreshToken = refreshToken
			updatedCredential.TokenExpiresAt = findTokenExpiresAt(payload)
			session.CredentialUpdate = &updatedCredential
		}
	}
	if _, err := fetchSub2APICurrentUser(ctx, session); err != nil {
		return nil, wrapPlatformSiteStage("Sub2API 当前用户", err)
	}
	return session, nil
}

func (adapter *Sub2APIAdapter) FetchSnapshot(ctx context.Context, session *PlatformSiteSession) (PlatformSiteSnapshot, error) {
	mePayload, err := fetchSub2APICurrentUser(ctx, session)
	if err != nil {
		return PlatformSiteSnapshot{}, wrapPlatformSiteStage("Sub2API 当前用户", err)
	}
	me := firstNestedRecord(mePayload, "user", "account", "profile")
	snapshot := PlatformSiteSnapshot{
		Balance:   firstFloat(me, "balance", "quota", "credit"),
		UsedQuota: firstInt64(me, "used_quota", "quota_used", "used"),
	}
	if payload, requestErr := platformSiteRequest(ctx, session, http.MethodGet, "/api/v1/user/profile", nil, nil); requestErr == nil {
		profile := firstRecord(payload)
		if balance := firstFloat(profile, "balance", "quota", "credit"); balance > 0 {
			snapshot.Balance = balance
		}
	}
	if payload, requestErr := platformSiteRequest(ctx, session, http.MethodGet, "/api/v1/usage/dashboard/stats", nil, nil); requestErr == nil {
		usage := firstRecord(payload)
		if used := firstInt64(usage, "total_actual_cost", "total_cost", "today_actual_cost"); used > 0 {
			snapshot.UsedQuota = used
		}
	}
	rates := map[string]float64{}
	if payload, requestErr := platformSiteRequest(ctx, session, http.MethodGet, "/api/v1/groups/available", nil, nil); requestErr == nil {
		for key, value := range parseGroupRates(payload) {
			rates[key] = value
		}
	}
	if payload, requestErr := platformSiteRequest(ctx, session, http.MethodGet, "/api/v1/groups/rates", nil, nil); requestErr == nil {
		for key, value := range parseGroupRates(payload) {
			rates[key] = value
		}
	}
	if session.Headers.Get("x-api-key") != "" {
		snapshot.Keys, err = fetchSub2APIAdminKeys(ctx, session, rates)
	} else {
		snapshot.Keys, err = fetchSub2APIKeys(ctx, session, rates)
	}
	if err != nil {
		return PlatformSiteSnapshot{}, wrapPlatformSiteStage("Sub2API 密钥分页", err)
	}
	return snapshot, nil
}

func loginNewAPIWithPassword(ctx context.Context, session *PlatformSiteSession, credential model.PlatformSiteCredential) (any, error) {
	username := strings.TrimSpace(credential.Username)
	if username == "" || credential.Password == "" {
		return nil, fmt.Errorf("%w: 缺少账号密码", ErrPlatformSiteAuth)
	}
	email := ""
	if strings.Contains(username, "@") {
		email = username
	}
	bodies := uniqueStringMaps([]map[string]string{
		{"username": username, "password": credential.Password},
		{"email": firstNonEmptyString(email, username), "password": credential.Password},
		{"username": username, "email": firstNonEmptyString(email, username), "password": credential.Password},
	})
	var lastErr error
	for _, body := range bodies {
		payload, err := platformSiteRequest(ctx, session, http.MethodPost, "/api/user/login?turnstile=", nil, body)
		if err == nil {
			return payload, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("%w: NewAPI 登录失败", ErrPlatformSiteAuth)
	}
	return nil, lastErr
}

func loginSub2APIWithPassword(ctx context.Context, session *PlatformSiteSession, credential model.PlatformSiteCredential) (any, error) {
	username := strings.TrimSpace(credential.Username)
	if username == "" || credential.Password == "" {
		return nil, fmt.Errorf("%w: 缺少账号密码", ErrPlatformSiteAuth)
	}
	email := ""
	if strings.Contains(username, "@") {
		email = username
	}
	bodies := uniqueStringMaps([]map[string]string{
		{"email": firstNonEmptyString(email, username), "password": credential.Password},
		{"username": username, "password": credential.Password},
		{"email": firstNonEmptyString(email, username), "username": username, "password": credential.Password},
	})
	var lastErr error
	for _, body := range bodies {
		payload, err := platformSiteRequest(ctx, session, http.MethodPost, "/api/v1/auth/login", nil, body)
		if err == nil {
			return payload, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("%w: Sub2API 登录失败", ErrPlatformSiteAuth)
	}
	return nil, lastErr
}

func fetchNewAPICurrentUser(ctx context.Context, session *PlatformSiteSession) (any, error) {
	var lastErr error
	for _, path := range []string{"/api/user/self", "/api/user/me", "/api/user/profile", "/api/user/info"} {
		payload, err := platformSiteRequest(ctx, session, http.MethodGet, path, nil, nil)
		if err == nil {
			return payload, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("%w: NewAPI 当前用户接口不可用", ErrPlatformSiteAuth)
	}
	return nil, lastErr
}

func fetchSub2APICurrentUser(ctx context.Context, session *PlatformSiteSession) (any, error) {
	var lastErr error
	for _, path := range []string{"/api/v1/auth/me", "/api/v1/user/profile"} {
		payload, err := platformSiteRequest(ctx, session, http.MethodGet, path, nil, nil)
		if err == nil {
			return payload, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("%w: Sub2API 当前用户接口不可用", ErrPlatformSiteAuth)
	}
	return nil, lastErr
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
	if credential == nil || strings.TrimSpace(credential.RefreshToken) == "" {
		return fmt.Errorf("%w: 缺少刷新令牌", ErrPlatformSiteAuth)
	}
	if strings.TrimSpace(credential.AccessToken) != "" {
		session.Headers.Set("Authorization", bearerToken(credential.AccessToken))
	}
	payload, err := platformSiteRequest(ctx, session, http.MethodPost, path, nil, map[string]string{
		"refresh_token": credential.RefreshToken,
	})
	if err != nil {
		return fmt.Errorf("%w: 刷新会话失败", ErrPlatformSiteAuth)
	}
	accessToken := findToken(payload)
	refreshToken := findRefreshToken(payload)
	if accessToken == "" || refreshToken == "" {
		return fmt.Errorf("%w: 刷新会话响应不完整", ErrPlatformSiteAuth)
	}
	credential.AccessToken = accessToken
	credential.RefreshToken = refreshToken
	credential.TokenExpiresAt = findTokenExpiresAt(payload)
	session.Headers.Set("Authorization", bearerToken(accessToken))
	updatedCredential := *credential
	session.CredentialUpdate = &updatedCredential
	return nil
}

func findToken(payload any) string {
	record := firstRecord(payload)
	if token := firstString(record, "access_token", "token", "accessToken", "jwt"); token != "" {
		return token
	}
	if data, ok := record["data"]; ok {
		return findToken(data)
	}
	return ""
}

func findRefreshToken(payload any) string {
	record := firstRecord(payload)
	if token := firstString(record, "refresh_token", "refreshToken"); token != "" {
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

func findUserID(payload any) string {
	record := firstRecord(payload)
	if userID := firstString(record, "id", "user_id", "userId", "uid"); userID != "" {
		return userID
	}
	for _, key := range []string{"data", "result", "user", "account"} {
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
			continue
		}
		if rates := parseGroupRates(payload); len(rates) > 0 {
			return rates
		}
	}
	return map[string]float64{}
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
	if err != nil {
		payload, err = platformSiteRequest(ctx, session, http.MethodGet, path, nil, nil)
		if err != nil {
			return "", err
		}
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
		if keys, exists := record["keys"]; exists {
			payload = unwrapPlatformData(keys)
			record, _ = payload.(map[string]any)
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

func newAPIRemainQuota(record map[string]any) *int64 {
	if boolFromRecord(record, "unlimited_quota", "unlimitedQuota", "unlimited") {
		return nil
	}
	if remain := optionalInt64(record, "remain_quota", "remaining_quota"); remain != nil {
		return remain
	}
	if !hasAnyField(record, "quota") {
		return nil
	}
	return optionalInt64(record, "quota")
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
				UsedQuota:                firstInt64(item, "quota_used", "used_quota", "used"),
				RemainQuota:              sub2APIRemainQuota(item),
				ExpiresAt:                firstTime(item, "expires_at", "expired_at", "expire_at"),
				Disabled:                 isUpstreamKeyDisabled(item),
			}
			secret := firstString(item, "key", "api_key", "token")
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
				UsedQuota:                firstInt64(item, "quota_used", "used_quota", "used"),
				RemainQuota:              sub2APIRemainQuota(item),
				ExpiresAt:                firstTime(item, "expires_at", "expired_at", "expire_at"),
				Disabled:                 isUpstreamKeyDisabled(item),
			}
			dataPayload, dataErr := platformSiteRequest(ctx, session, http.MethodGet, "/api/v1/admin/accounts/data", url.Values{
				"ids":             {id},
				"include_proxies": {"false"},
			}, nil)
			if dataErr != nil {
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

func sub2APIRemainQuota(record map[string]any) *int64 {
	if boolFromRecord(record, "unlimited", "unlimited_quota", "unlimitedQuota") {
		return nil
	}
	if hasAnyField(record, "quota") {
		quota := firstFloat(record, "quota")
		if quota <= 0 {
			return nil
		}
		used := firstFloat(record, "quota_used", "used_quota", "used")
		remain := quota - used
		if remain < 0 {
			remain = 0
		}
		return optionalInt64FromFloat(remain)
	}
	return optionalInt64(record, "remain_quota", "remaining_quota")
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
	case "/login", "/dashboard", "/register", "/setup", "/home":
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
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, session.BaseURL, nil)
	if err != nil {
		return "", false
	}
	request.Header.Set("Accept", "text/html,application/xhtml+xml")
	response, err := session.Client.Do(request)
	if err != nil {
		return "", false
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
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
	if decoded, err := url.QueryUnescape(candidate); err == nil && strings.HasPrefix(decoded, "http") {
		candidate = decoded
	}
	normalized, err := normalizePlatformSiteURL(normalizeSub2APIBaseURL(candidate))
	if err != nil || validatePlatformSiteURL(normalized) != nil {
		return "", false
	}
	if !relatedPlatformSiteBaseURL(session.BaseURL, normalized) {
		return "", false
	}
	return normalized, true
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
