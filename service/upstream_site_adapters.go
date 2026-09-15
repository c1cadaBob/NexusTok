package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/c1cadaBob/NexusTok/model"
)

const defaultNewAPIQuotaPerUnit = 1

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
			return nil, err
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
		if strings.TrimSpace(credential.Username) == "" || credential.Password == "" {
			return nil, fmt.Errorf("%w: 缺少账号密码", ErrPlatformSiteAuth)
		}
		payload, requestErr := platformSiteRequest(ctx, session, http.MethodPost, "/api/user/login", nil, map[string]string{
			"username": strings.TrimSpace(credential.Username),
			"password": credential.Password,
		})
		if requestErr != nil {
			return nil, requestErr
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
	if _, err := platformSiteRequest(ctx, session, http.MethodGet, "/api/user/self", nil, nil); err != nil {
		return nil, err
	}
	return session, nil
}

func (adapter *NewAPIAdapter) FetchSnapshot(ctx context.Context, session *PlatformSiteSession) (PlatformSiteSnapshot, error) {
	quotaPerUnit := fetchNewAPIQuotaPerUnit(ctx, session)
	selfPayload, err := platformSiteRequest(ctx, session, http.MethodGet, "/api/user/self", nil, nil)
	if err != nil {
		return PlatformSiteSnapshot{}, err
	}
	self := firstRecord(selfPayload)
	snapshot := PlatformSiteSnapshot{
		Balance:   normalizeNewAPIQuota(firstFloat(self, "quota", "balance", "money", "credit"), quotaPerUnit),
		UsedQuota: firstInt64(self, "used_quota", "used", "used_quota_amount"),
	}
	groupRates := fetchNewAPIGroupRates(ctx, session)
	tokens, err := fetchNewAPITokens(ctx, session)
	if err != nil {
		return PlatformSiteSnapshot{}, err
	}
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
			RemainQuota:              optionalInt64(token, "remain_quota", "remaining_quota", "quota"),
			ExpiresAt:                firstTime(token, "expired_time", "expires_at", "expire_at"),
			Disabled:                 isUpstreamKeyDisabled(token),
		}
		secret := firstString(token, "key", "token", "api_key")
		if secret == "" || strings.Contains(secret, "*") {
			revealed, revealErr := platformSiteRequest(ctx, session, http.MethodPost, "/api/token/"+externalID+"/key", nil, nil)
			if revealErr != nil {
				item.SyncError = upstreamKeySyncErrorSecretUnavailable
				snapshot.Keys = append(snapshot.Keys, item)
				continue
			}
			secret = firstString(firstRecord(revealed), "key", "token", "api_key")
			if secret == "" {
				secret = stringFromPayload(revealed)
			}
		}
		if secret == "" {
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
			"/api/v1/auth/refresh",
			&credential,
		); err != nil {
			return nil, err
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
		if strings.TrimSpace(credential.Username) == "" || credential.Password == "" {
			return nil, fmt.Errorf("%w: 缺少账号密码", ErrPlatformSiteAuth)
		}
		payload, requestErr := platformSiteRequest(ctx, session, http.MethodPost, "/api/v1/auth/login", nil, map[string]string{
			"username": strings.TrimSpace(credential.Username),
			"email":    strings.TrimSpace(credential.Username),
			"password": credential.Password,
		})
		if requestErr != nil {
			return nil, requestErr
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
	if _, err := platformSiteRequest(ctx, session, http.MethodGet, "/api/v1/auth/me", nil, nil); err != nil {
		return nil, err
	}
	return session, nil
}

func (adapter *Sub2APIAdapter) FetchSnapshot(ctx context.Context, session *PlatformSiteSession) (PlatformSiteSnapshot, error) {
	mePayload, err := platformSiteRequest(ctx, session, http.MethodGet, "/api/v1/auth/me", nil, nil)
	if err != nil {
		return PlatformSiteSnapshot{}, err
	}
	me := firstRecord(mePayload)
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
		return PlatformSiteSnapshot{}, err
	}
	return snapshot, nil
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
	if userID := firstString(record, "id", "user_id", "userId"); userID != "" {
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

func stringFromPayload(payload any) string {
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
	keySession := *session
	keySession.Headers = session.Headers.Clone()
	keySession.Headers.Del("x-api-key")
	keySession.Headers.Del("New-Api-Key")
	keySession.Headers.Del("Cookie")
	keySession.Headers.Set("Authorization", bearerToken(secret))
	payload, err := platformSiteRequest(ctx, &keySession, http.MethodGet, "/v1/models", nil, nil)
	if err != nil {
		return nil, err
	}
	models := stringsFromPayload(payload)
	if len(models) == 0 {
		return nil, fmt.Errorf("%w: 子密钥模型列表为空", ErrPlatformSiteResponse)
	}
	return models, nil
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

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
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
		payload, err := platformSiteRequest(ctx, session, http.MethodGet, "/api/token/", url.Values{
			"p":         {fmt.Sprint(page)},
			"page_size": {fmt.Sprint(upstreamSitePageSize)},
			"size":      {fmt.Sprint(upstreamSitePageSize)},
		}, nil)
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
				RemainQuota:              optionalInt64(item, "quota", "remain_quota", "remaining_quota"),
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
				RemainQuota:              optionalInt64(item, "quota", "remain_quota", "remaining_quota"),
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
