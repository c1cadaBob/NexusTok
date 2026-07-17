package upstreamaccount

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/c1cada/NexusTok/common"
)

// NewAPIClient 负责从 new-api 平台读取账号快照。
type NewAPIClient struct {
	httpClient *http.Client
}

// NewNewAPIClient 创建 new-api 客户端。
func NewNewAPIClient(client *http.Client) *NewAPIClient {
	return &NewAPIClient{httpClient: client}
}

type newAPIStatus struct {
	QuotaPerUnit float64 `json:"quota_per_unit"`
}

type newAPIUser struct {
	ID         any     `json:"id"`
	Username   string  `json:"username"`
	Email      string  `json:"email"`
	Group      string  `json:"group"`
	Quota      float64 `json:"quota"`
	UsedQuota  float64 `json:"used_quota"`
	Require2FA bool    `json:"require_2fa"`
}

type newAPIGroup struct {
	Ratio float64 `json:"ratio"`
	Desc  string  `json:"desc"`
}

type newAPITokenList struct {
	Items []newAPIToken `json:"items"`
	Total int           `json:"total"`
}

type newAPIToken struct {
	ID             any     `json:"id"`
	Name           string  `json:"name"`
	Key            string  `json:"key"`
	Group          string  `json:"group"`
	Status         int     `json:"status"`
	ModelLimits    string  `json:"model_limits"`
	RemainQuota    float64 `json:"remain_quota"`
	UsedQuota      float64 `json:"used_quota"`
	UnlimitedQuota bool    `json:"unlimited_quota"`
}

type newAPIRatioConfig struct {
	ModelRatio       map[string]float64 `json:"model_ratio"`
	CompletionRatio  map[string]float64 `json:"completion_ratio"`
	CacheRatio       map[string]float64 `json:"cache_ratio"`
	CreateCacheRatio map[string]float64 `json:"create_cache_ratio"`
	ModelPrice       map[string]float64 `json:"model_price"`
}

type newAPITokenKeysResponse struct {
	Keys map[string]string `json:"keys"`
}

// UnmarshalJSON 兼容 new-api 不同版本的批量 Key 响应。
//
// 当前 new-api controller 返回 `data.keys`，早期或兼容实现可能直接返回
// `data` 作为 `id -> key` 映射。同步链路只需要完整 Key 的短期后端快照，
// 因此这里统一归一化到 Keys，避免真实平台有 Key 时因 envelope 差异取不到密钥。
func (r *newAPITokenKeysResponse) UnmarshalJSON(data []byte) error {
	type wrappedTokenKeys struct {
		Keys map[string]string `json:"keys"`
	}
	var raw map[string]any
	if err := common.Unmarshal(data, &raw); err != nil {
		return err
	}
	if _, hasWrappedKeys := raw["keys"]; hasWrappedKeys {
		var wrapped wrappedTokenKeys
		if err := common.Unmarshal(data, &wrapped); err != nil {
			return err
		}
		if wrapped.Keys == nil {
			wrapped.Keys = map[string]string{}
		}
		r.Keys = wrapped.Keys
		return nil
	}
	var direct map[string]string
	if err := common.Unmarshal(data, &direct); err != nil {
		return err
	}
	r.Keys = direct
	return nil
}

// FetchSnapshot 登录 new-api 并读取当前账号可见的密钥、分组、倍率和余额。
func (c *NewAPIClient) FetchSnapshot(ctx context.Context, credential Credential) (*Snapshot, error) {
	api, err := newHTTPClient(credential.BaseURL, c.httpClient)
	if err != nil {
		return nil, err
	}
	status, err := c.fetchStatus(ctx, api)
	if err != nil {
		return nil, err
	}
	user, headers, err := c.login(ctx, api, credential)
	if err != nil {
		return nil, err
	}
	if user.ID == nil {
		return nil, fmt.Errorf("new-api 登录响应缺少用户 ID")
	}
	headers.Set("New-Api-User", stringValue(user.ID))

	self, err := c.fetchSelf(ctx, api, headers)
	if err == nil {
		*user = self
	} else if strings.TrimSpace(user.Group) == "" {
		return nil, err
	}
	groups, groupRates, err := c.fetchGroups(ctx, api, headers)
	if err != nil {
		return nil, err
	}
	rates, warnings := c.fetchRatioConfig(ctx, api)
	keys, err := c.fetchTokens(ctx, api, headers, status.QuotaPerUnit, groupRates, rates)
	if err != nil {
		return nil, err
	}

	snapshot := &Snapshot{
		Platform: PlatformNewAPI,
		BaseURL:  api.baseURL,
		User: &UserSnapshot{
			ID:       stringValue(user.ID),
			Username: user.Username,
			Email:    user.Email,
			Group:    user.Group,
		},
		Balance:  buildNewAPIBalance(*user, status.QuotaPerUnit),
		Groups:   groups,
		Keys:     keys,
		Rates:    rates,
		Warnings: warnings,
	}
	ApplySuggestions(snapshot)
	return snapshot, nil
}

func (c *NewAPIClient) fetchStatus(ctx context.Context, api *httpClient) (*newAPIStatus, error) {
	var envelope newAPIEnvelope[newAPIStatus]
	if err := api.getJSON(ctx, "/api/status", nil, &envelope); err != nil {
		return nil, err
	}
	status, err := unwrapNewAPI(envelope)
	if err != nil {
		return nil, err
	}
	if status.QuotaPerUnit <= 0 {
		status.QuotaPerUnit = 500000
	}
	return &status, nil
}

func (c *NewAPIClient) login(ctx context.Context, api *httpClient, credential Credential) (*newAPIUser, http.Header, error) {
	username := strings.TrimSpace(credential.Username)
	if username == "" {
		username = strings.TrimSpace(credential.Email)
	}
	if username == "" || strings.TrimSpace(credential.Password) == "" {
		return nil, nil, fmt.Errorf("new-api 账号和密码不能为空")
	}
	body := map[string]string{
		"username": username,
		"password": credential.Password,
	}
	var envelope newAPIEnvelope[newAPIUser]
	if err := api.postJSON(ctx, "/api/user/login?turnstile=", nil, body, &envelope); err != nil {
		return nil, nil, err
	}
	user, err := unwrapNewAPI(envelope)
	if err != nil {
		return nil, nil, err
	}
	if user.Require2FA {
		return nil, nil, fmt.Errorf("new-api 账号启用了 2FA，当前预览接口暂不支持交互式二次验证")
	}
	headers := http.Header{}
	return &user, headers, nil
}

func (c *NewAPIClient) fetchSelf(ctx context.Context, api *httpClient, headers http.Header) (newAPIUser, error) {
	var envelope newAPIEnvelope[newAPIUser]
	if err := api.getJSON(ctx, "/api/user/self", headers, &envelope); err != nil {
		return newAPIUser{}, err
	}
	return unwrapNewAPI(envelope)
}

func (c *NewAPIClient) fetchGroups(ctx context.Context, api *httpClient, headers http.Header) ([]SyncedGroup, map[string]float64, error) {
	var envelope newAPIEnvelope[map[string]newAPIGroup]
	if err := api.getJSON(ctx, "/api/user/self/groups", headers, &envelope); err != nil {
		return nil, nil, err
	}
	rawGroups, err := unwrapNewAPI(envelope)
	if err != nil {
		return nil, nil, err
	}
	groups := make([]SyncedGroup, 0, len(rawGroups))
	groupRates := make(map[string]float64, len(rawGroups))
	for name, group := range rawGroups {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		ratio := group.Ratio
		groupRates[name] = ratio
		groups = append(groups, SyncedGroup{
			ID:          name,
			Name:        name,
			Ratio:       floatPtr(ratio),
			Description: group.Desc,
		})
	}
	return groups, groupRates, nil
}

func (c *NewAPIClient) fetchRatioConfig(ctx context.Context, api *httpClient) (*RateSnapshot, []string) {
	var envelope newAPIEnvelope[newAPIRatioConfig]
	if err := api.getJSON(ctx, "/api/ratio_config", nil, &envelope); err != nil {
		return &RateSnapshot{Source: "new-api:ratio_config", Partial: true}, []string{"new-api 未暴露倍率配置，已仅使用分组倍率"}
	}
	raw, err := unwrapNewAPI(envelope)
	if err != nil {
		return &RateSnapshot{Source: "new-api:ratio_config", Partial: true}, []string{"new-api 倍率配置不可见：" + err.Error()}
	}
	return &RateSnapshot{
		ModelRatios:       raw.ModelRatio,
		CompletionRatios:  raw.CompletionRatio,
		CacheRatios:       raw.CacheRatio,
		CreateCacheRatios: raw.CreateCacheRatio,
		ModelPrices:       raw.ModelPrice,
		Source:            "new-api:ratio_config",
	}, nil
}

func (c *NewAPIClient) fetchTokens(ctx context.Context, api *httpClient, headers http.Header, quotaPerUnit float64, groupRates map[string]float64, rates *RateSnapshot) ([]SyncedKey, error) {
	var tokens []newAPIToken
	for page := 1; page <= 1000; page++ {
		path := "/api/token/?" + url.Values{
			"p":         {strconv.Itoa(page)},
			"page_size": {"100"},
		}.Encode()
		var envelope newAPIEnvelope[newAPITokenList]
		if err := api.getJSON(ctx, path, headers, &envelope); err != nil {
			return nil, err
		}
		pageData, err := unwrapNewAPI(envelope)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, pageData.Items...)
		if len(pageData.Items) == 0 || len(tokens) >= pageData.Total {
			break
		}
	}
	if len(tokens) == 0 {
		return nil, nil
	}
	fullKeys, err := c.fetchTokenKeys(ctx, api, headers, tokens)
	if err != nil {
		return nil, err
	}
	result := make([]SyncedKey, 0, len(tokens))
	for _, token := range tokens {
		externalID := stringValue(token.ID)
		key := strings.TrimSpace(fullKeys[externalID])
		if key == "" && !strings.Contains(token.Key, "*") {
			key = strings.TrimSpace(token.Key)
		}
		if key == "" {
			return nil, fmt.Errorf("new-api 密钥 %s 缺少完整 key，请稍后重试或检查目标平台完整 Key 读取权限", firstNonEmpty(token.Name, externalID))
		}
		groupRatio, hasGroupRatio := groupRates[token.Group]
		synced := SyncedKey{
			ExternalID:        externalID,
			Name:              token.Name,
			Key:               key,
			MaskedKey:         maskKey(firstNonEmpty(key, token.Key)),
			Status:            token.Status,
			GroupID:           token.Group,
			GroupName:         token.Group,
			Models:            splitModels(token.ModelLimits),
			QuotaLimitUSD:     quotaToUSD(token.RemainQuota+token.UsedQuota, quotaPerUnit),
			QuotaUsedUSD:      quotaToUSD(token.UsedQuota, quotaPerUnit),
			QuotaRemainingUSD: quotaToUSD(token.RemainQuota, quotaPerUnit),
			Unlimited:         token.UnlimitedQuota,
		}
		if hasGroupRatio {
			synced.GroupRatio = floatPtr(groupRatio)
		}
		if rates != nil {
			synced.ModelRatios = filterModelRatios(rates.ModelRatios, synced.Models)
		}
		result = append(result, synced)
	}
	return result, nil
}

func (c *NewAPIClient) fetchTokenKeys(ctx context.Context, api *httpClient, headers http.Header, tokens []newAPIToken) (map[string]string, error) {
	ids := make([]any, 0, len(tokens))
	for _, token := range tokens {
		// new-api 有些兼容版本会在列表接口直接返回完整 Key；只有列表 Key 为空或
		// 明确为脱敏值时才需要调用敏感 reveal 接口，避免目标平台限流影响不必要的同步。
		if token.ID != nil && (strings.TrimSpace(token.Key) == "" || strings.Contains(token.Key, "*")) {
			ids = append(ids, token.ID)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	normalized := map[string]string{}
	var batchErr error
	var envelope newAPIEnvelope[newAPITokenKeysResponse]
	if err := api.postJSON(ctx, "/api/token/batch/keys", headers, map[string]any{"ids": ids}, &envelope); err != nil {
		batchErr = err
	} else {
		data, err := unwrapNewAPI(envelope)
		if err != nil {
			batchErr = err
		} else {
			for id, key := range data.Keys {
				normalized[strings.TrimSpace(id)] = key
			}
		}
	}

	var fallbackErr error
	for _, token := range tokens {
		externalID := stringValue(token.ID)
		if externalID == "" || strings.TrimSpace(normalized[externalID]) != "" {
			continue
		}
		key, err := c.fetchTokenKey(ctx, api, headers, externalID)
		if err != nil {
			fallbackErr = err
			continue
		}
		normalized[externalID] = key
	}
	for _, token := range tokens {
		externalID := stringValue(token.ID)
		if externalID == "" || strings.TrimSpace(normalized[externalID]) != "" || !strings.Contains(token.Key, "*") {
			continue
		}
		if fallbackErr != nil {
			return nil, fmt.Errorf("读取 new-api 完整 Key 失败：%w", fallbackErr)
		}
		if batchErr != nil {
			return nil, fmt.Errorf("读取 new-api 完整 Key 失败：%w", batchErr)
		}
		return nil, fmt.Errorf("读取 new-api 完整 Key 失败：token %s 未返回完整 key", externalID)
	}
	if len(normalized) == 0 {
		if fallbackErr != nil {
			return nil, fmt.Errorf("读取 new-api 完整 Key 失败：%w", fallbackErr)
		}
		if batchErr != nil {
			return nil, fmt.Errorf("读取 new-api 完整 Key 失败：%w", batchErr)
		}
	}
	return normalized, nil
}

func (c *NewAPIClient) fetchTokenKey(ctx context.Context, api *httpClient, headers http.Header, externalID string) (string, error) {
	var envelope newAPIEnvelope[struct {
		Key string `json:"key"`
	}]
	if err := api.postJSON(ctx, "/api/token/"+url.PathEscape(externalID)+"/key", headers, nil, &envelope); err != nil {
		return "", err
	}
	data, err := unwrapNewAPI(envelope)
	if err != nil {
		return "", err
	}
	key := strings.TrimSpace(data.Key)
	if key == "" {
		return "", fmt.Errorf("new-api token %s 返回空 key", externalID)
	}
	return key, nil
}

func buildNewAPIBalance(user newAPIUser, quotaPerUnit float64) *BalanceSnapshot {
	balance := quotaToUSD(user.Quota, quotaPerUnit)
	used := quotaToUSD(user.UsedQuota, quotaPerUnit)
	return &BalanceSnapshot{
		BalanceUSD:   balance,
		UsedUSD:      used,
		RawBalance:   floatPtr(user.Quota),
		RawUsed:      floatPtr(user.UsedQuota),
		QuotaPerUnit: floatPtr(quotaPerUnit),
		Source:       "new-api:user/self",
		Partial:      balance == nil || used == nil,
	}
}

func filterModelRatios(ratios map[string]float64, models []string) map[string]float64 {
	if len(ratios) == 0 {
		return nil
	}
	if len(models) == 0 {
		return ratios
	}
	filtered := map[string]float64{}
	for _, model := range models {
		if ratio, ok := ratios[model]; ok {
			filtered[model] = ratio
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
