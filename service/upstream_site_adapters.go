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

	"github.com/c1cadaBob/NexusTok/model"
)

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
	session, err := newPlatformSiteSession(baseURL, headers)
	if err != nil {
		return nil, err
	}
	if adapter.client != nil {
		session.Client = adapter.client
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
	}
	if _, err := platformSiteRequest(ctx, session, http.MethodGet, "/api/user/self", nil, nil); err != nil {
		return nil, err
	}
	return session, nil
}

func (adapter *NewAPIAdapter) FetchSnapshot(ctx context.Context, session *PlatformSiteSession) (PlatformSiteSnapshot, error) {
	selfPayload, err := platformSiteRequest(ctx, session, http.MethodGet, "/api/user/self", nil, nil)
	if err != nil {
		return PlatformSiteSnapshot{}, err
	}
	self := firstRecord(selfPayload)
	snapshot := PlatformSiteSnapshot{
		Balance:   firstFloat(self, "quota", "balance", "money", "credit"),
		UsedQuota: firstInt64(self, "used_quota", "used", "used_quota_amount"),
	}
	if modelsPayload, requestErr := platformSiteRequest(ctx, session, http.MethodGet, "/api/user/models", nil, nil); requestErr == nil {
		snapshot.Models = stringsFromPayload(modelsPayload)
	}
	if len(snapshot.Models) == 0 {
		if groupsPayload, requestErr := platformSiteRequest(ctx, session, http.MethodGet, "/api/user/self/groups", nil, nil); requestErr == nil {
			snapshot.Models = modelsFromGroups(groupsPayload)
		}
	}

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
		itemModels := stringsFromPayload(token["models"])
		if len(itemModels) == 0 {
			itemModels = snapshot.Models
		}
		item := UpstreamKeySnapshot{
			ExternalID:         externalID,
			Name:               firstString(token, "name", "key_name", "token_name"),
			Group:              firstString(token, "group", "group_name"),
			Models:             itemModels,
			ConversionRatio:    firstFloat(token, "ratio", "rate", "multiplier", "group_ratio"),
			ConversionRatioSet: hasAnyField(token, "ratio", "rate", "multiplier", "group_ratio"),
			UsedQuota:          firstInt64(token, "used_quota", "used", "quota_used"),
			RemainQuota:        optionalInt64(token, "remain_quota", "remaining_quota", "quota"),
			ExpiresAt:          firstTime(token, "expired_time", "expires_at", "expire_at"),
			Disabled:           isUpstreamKeyDisabled(token),
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
	switch {
	case credential.AdminKey != "":
		headers.Set("Authorization", bearerToken(credential.AdminKey))
		headers.Set("x-api-key", credential.AdminKey)
	case credential.AccessToken != "":
		headers.Set("Authorization", bearerToken(credential.AccessToken))
	case credential.Cookie != "":
		headers.Set("Cookie", credential.Cookie)
	}
	session, err := newPlatformSiteSession(baseURL, headers)
	if err != nil {
		return nil, err
	}
	if adapter.client != nil {
		session.Client = adapter.client
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
	rates := map[string]float64{}
	if payload, requestErr := platformSiteRequest(ctx, session, http.MethodGet, "/api/v1/groups/rates", nil, nil); requestErr == nil {
		rates = parseGroupRates(payload)
	}
	if modelsPayload, requestErr := platformSiteRequest(ctx, session, http.MethodGet, "/v1/models", nil, nil); requestErr == nil {
		snapshot.Models = stringsFromPayload(modelsPayload)
	}
	if session.Headers.Get("x-api-key") != "" {
		snapshot.Keys, err = fetchSub2APIAdminKeys(ctx, session, rates, snapshot.Models)
	} else {
		snapshot.Keys, err = fetchSub2APIKeys(ctx, session, rates, snapshot.Models)
	}
	if err != nil {
		return PlatformSiteSnapshot{}, err
	}
	return snapshot, nil
}

func bearerToken(token string) string {
	return "Bearer " + strings.TrimSpace(token)
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
		case "disabled", "inactive", "revoked", "suspended", "expired", "error":
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

func fetchNewAPITokens(ctx context.Context, session *PlatformSiteSession) ([]map[string]any, error) {
	result := make([]map[string]any, 0)
	for page := 1; page <= upstreamSiteMaxPages; page++ {
		payload, err := platformSiteRequest(ctx, session, http.MethodGet, "/api/token/", url.Values{
			"p":    {fmt.Sprint(page)},
			"size": {fmt.Sprint(upstreamSitePageSize)},
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

func fetchSub2APIKeys(ctx context.Context, session *PlatformSiteSession, rates map[string]float64, models []string) ([]UpstreamKeySnapshot, error) {
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
			group := firstString(item, "group", "group_name")
			ratioKeysPresent := hasAnyField(item, "rate_multiplier", "ratio", "rate", "multiplier")
			ratio := firstFloat(item, "rate_multiplier", "ratio", "rate", "multiplier")
			_, groupRateSet := rates[group]
			if !ratioKeysPresent {
				ratio = rates[group]
			}
			keySnapshot := UpstreamKeySnapshot{
				ExternalID:         externalID,
				Name:               firstString(item, "name", "key_name"),
				Group:              group,
				Models:             stringsFromPayload(item["models"]),
				ConversionRatio:    ratio,
				ConversionRatioSet: ratioKeysPresent || groupRateSet,
				UsedQuota:          firstInt64(item, "quota_used", "used_quota", "used"),
				RemainQuota:        optionalInt64(item, "quota", "remain_quota", "remaining_quota"),
				ExpiresAt:          firstTime(item, "expires_at", "expired_at", "expire_at"),
				Disabled:           isUpstreamKeyDisabled(item),
			}
			secret := firstString(item, "key", "api_key", "token")
			if secret == "" || strings.Contains(secret, "*") {
				keySnapshot.SyncError = upstreamKeySyncErrorSecretUnavailable
				result = append(result, keySnapshot)
				continue
			}
			keySnapshot.Secret = secret
			result = append(result, keySnapshot)
		}
		if len(items) == 0 || len(items) < upstreamSitePageSize || page >= payloadPageCount(payload) {
			break
		}
	}
	for i := range result {
		if len(result[i].Models) == 0 {
			result[i].Models = models
		}
	}
	return result, nil
}

func fetchSub2APIAdminKeys(ctx context.Context, session *PlatformSiteSession, rates map[string]float64, models []string) ([]UpstreamKeySnapshot, error) {
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
			group := firstString(item, "group", "group_name")
			ratioKeysPresent := hasAnyField(item, "rate_multiplier", "ratio", "rate", "multiplier")
			ratio := firstFloat(item, "rate_multiplier", "ratio", "rate", "multiplier")
			_, groupRateSet := rates[group]
			if !ratioKeysPresent {
				ratio = rates[group]
			}
			itemModels := stringsFromPayload(item["models"])
			if len(itemModels) == 0 {
				itemModels = models
			}
			keySnapshot := UpstreamKeySnapshot{
				ExternalID:         id,
				Name:               firstString(item, "name", "account_name"),
				Group:              group,
				Models:             itemModels,
				ConversionRatio:    ratio,
				ConversionRatioSet: ratioKeysPresent || groupRateSet,
				UsedQuota:          firstInt64(item, "quota_used", "used_quota", "used"),
				RemainQuota:        optionalInt64(item, "quota", "remain_quota", "remaining_quota"),
				ExpiresAt:          firstTime(item, "expires_at", "expired_at", "expire_at"),
				Disabled:           isUpstreamKeyDisabled(item),
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
