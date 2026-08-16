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

const defaultNewAPIQuotaPerUnit = 500000

// NewNewAPIClient 创建 new-api 客户端。
func NewNewAPIClient(client *http.Client) *NewAPIClient {
	return &NewAPIClient{httpClient: client}
}

type newAPIStatus struct {
	QuotaPerUnit float64 `json:"quota_per_unit"`
}

type newAPIUser struct {
	ID          any     `json:"id"`
	Username    string  `json:"username"`
	Email       string  `json:"email"`
	Group       string  `json:"group"`
	Quota       float64 `json:"quota"`
	UsedQuota   float64 `json:"used_quota"`
	Require2FA  bool    `json:"require_2fa"`
	AccessToken string  `json:"access_token"`
	Token       string  `json:"token"`
}

// UnmarshalJSON 兼容 new-api 及其二开站点的登录/用户响应结构。
//
// 主线 new-api 通常把用户字段直接放在 `data.id`、`data.username` 等位置；部分站点
// 会返回 `data.user.id`，同时把浏览器 access token 放在同一层。同步流程必须先拿到
// 目标站数字用户 ID 才能设置 `New-Api-User`，因此这里在解析层统一拉平这些常见变体。
func (u *newAPIUser) UnmarshalJSON(data []byte) error {
	type alias newAPIUser
	var raw struct {
		alias
		User        *alias `json:"user"`
		UserID      any    `json:"user_id"`
		UID         any    `json:"uid"`
		UserIDCamel any    `json:"userId"`
		DisplayName string `json:"display_name"`
		TokenType   string `json:"token_type"`
	}
	if err := common.Unmarshal(data, &raw); err != nil {
		return err
	}
	*u = newAPIUser(raw.alias)
	if raw.User != nil {
		mergeNewAPIUser(u, newAPIUser(*raw.User))
	}
	if u.ID == nil {
		u.ID = firstNonNil(raw.UserID, raw.UID, raw.UserIDCamel)
	}
	if strings.TrimSpace(u.Username) == "" {
		u.Username = strings.TrimSpace(raw.DisplayName)
	}
	return nil
}

type newAPIGroup struct {
	Ratio float64 `json:"ratio"`
	Desc  string  `json:"desc"`
}

type newAPITokenList struct {
	Items []newAPIToken `json:"items"`
	Total int           `json:"total"`
}

// UnmarshalJSON 兼容 new-api 及其分支在 token 列表接口上的分页字段差异。
//
// 官方接口通常返回 `data.items` 和 `data.total`，部分兼容站点会把列表放在
// `data.data`、`data.list`、`data.records`，甚至直接让 `data` 成为数组。这里在
// 解析层统一成 Items/Total，避免同步流程把字段名差异误判成“没有密钥”。
func (l *newAPITokenList) UnmarshalJSON(data []byte) error {
	var direct []newAPIToken
	if err := common.Unmarshal(data, &direct); err == nil {
		l.Items = direct
		l.Total = len(direct)
		return nil
	}
	var raw map[string]any
	if err := common.Unmarshal(data, &raw); err != nil {
		return err
	}
	for _, field := range []string{"items", "data", "list", "records", "rows"} {
		value, ok := raw[field]
		if !ok {
			continue
		}
		payload, err := common.Marshal(value)
		if err != nil {
			return err
		}
		var items []newAPIToken
		if err := common.Unmarshal(payload, &items); err == nil {
			l.Items = items
			break
		}
	}
	l.Total = intValueFromRaw(raw, "total", "count", "total_count", "totalCount")
	if l.Total <= 0 {
		l.Total = len(l.Items)
	}
	return nil
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

// UnmarshalJSON 兼容 token 模型字段的数组和字符串两种返回形态。
//
// new-api 主线使用 model_limits 字符串；一些二开站点会返回 models 数组或逗号分隔
// 字符串。这里仍落到 ModelLimits，后续沿用 splitModels 去重和清理空白。
func (t *newAPIToken) UnmarshalJSON(data []byte) error {
	type alias newAPIToken
	var raw struct {
		alias
		Models any `json:"models"`
	}
	if err := common.Unmarshal(data, &raw); err != nil {
		return err
	}
	*t = newAPIToken(raw.alias)
	if strings.TrimSpace(t.ModelLimits) == "" && raw.Models != nil {
		t.ModelLimits = modelsValueToCSV(raw.Models)
	}
	return nil
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
	snapshot, challenge, err := c.BeginPreview(ctx, credential)
	if err != nil {
		return nil, err
	}
	if challenge != nil {
		return nil, fmt.Errorf("new-api 账号启用了 2FA，请使用二阶段验证码完成上游账号同步")
	}
	return snapshot, nil
}

// BeginPreview 开始 new-api 账号同步预览。
//
// 普通账号会直接返回完整后端快照；启用 2FA 的账号会返回 challenge 记录，调用方需要
// 先短期缓存该记录，再等待管理员输入验证码后调用 Complete2FA。该方法不会保存密码。
func (c *NewAPIClient) BeginPreview(ctx context.Context, credential Credential) (*Snapshot, *AuthChallengeRecord, error) {
	api, err := newHTTPClient(credential.BaseURL, c.httpClient)
	if err != nil {
		return nil, nil, err
	}
	quotaPerUnit, statusWarnings := c.fetchStatusBestEffort(ctx, api)
	if snapshot, ok, sessionErr := c.fetchSnapshotWithSavedSession(ctx, api, quotaPerUnit, credential.Session); ok || sessionErr != nil {
		appendSnapshotWarnings(snapshot, statusWarnings...)
		return snapshot, nil, sessionErr
	}
	if credentialRequiresImportedSession(credential, AuthModeSessionCookie) {
		return nil, nil, fmt.Errorf("new-api Cookie 登录态不可用：请确认 Cookie 未过期；如果目标站要求 New-Api-User 头，请填写 user_id / New-Api-User 后重试")
	}
	if credentialRequiresImportedSession(credential, AuthModeAccessToken) {
		return nil, nil, fmt.Errorf("new-api Access Token 登录态不可用：请确认 token 未过期，并且同时填写 user_id / New-Api-User 后重试")
	}
	user, headers, needs2FA, err := c.login(ctx, api, credential)
	if err != nil {
		return nil, nil, err
	}
	if needs2FA {
		return nil, &AuthChallengeRecord{
			Platform: PlatformNewAPI,
			BaseURL:  api.baseURL,
			Username: strings.TrimSpace(firstNonEmpty(credential.Username, credential.Email)),
			NewAPI: &NewAPIChallengeData{
				QuotaPerUnit: quotaPerUnit,
				Cookies:      storeCookiesFromJar(api),
			},
		}, nil
	}
	snapshot, err := c.fetchSnapshotWithAuthenticatedSession(ctx, api, quotaPerUnit, user, headers)
	appendSnapshotWarnings(snapshot, statusWarnings...)
	return snapshot, nil, err
}

// Complete2FA 使用已缓存的 new-api pending session 和管理员输入的验证码完成预览。
func (c *NewAPIClient) Complete2FA(ctx context.Context, record AuthChallengeRecord, code string) (*Snapshot, error) {
	if record.NewAPI == nil {
		return nil, fmt.Errorf("new-api 二次验证会话无效，请重新同步上游账号")
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, fmt.Errorf("验证码不能为空")
	}
	api, err := newHTTPClient(record.BaseURL, c.httpClient)
	if err != nil {
		return nil, err
	}
	if err := restoreCookiesToJar(api, record.NewAPI.Cookies); err != nil {
		return nil, err
	}
	quotaPerUnit := record.NewAPI.QuotaPerUnit
	var statusWarnings []string
	if quotaPerUnit <= 0 {
		quotaPerUnit, statusWarnings = c.fetchStatusBestEffort(ctx, api)
	}
	var envelope newAPIEnvelope[newAPIUser]
	if err := api.postJSON(ctx, "/api/user/login/2fa", nil, map[string]string{"code": code}, &envelope); err != nil {
		return nil, err
	}
	user, err := unwrapNewAPI(envelope)
	if err != nil {
		return nil, fmt.Errorf("new-api 2FA 验证失败：%w", err)
	}
	headers := http.Header{}
	snapshot, err := c.fetchSnapshotWithAuthenticatedSession(ctx, api, quotaPerUnit, &user, headers)
	appendSnapshotWarnings(snapshot, statusWarnings...)
	return snapshot, err
}

func (c *NewAPIClient) fetchSnapshotWithSavedSession(ctx context.Context, api *httpClient, quotaPerUnit float64, session *AuthenticatedSession) (*Snapshot, bool, error) {
	if api == nil || !authSessionMatches(session, PlatformNewAPI, api.baseURL) || session.NewAPI == nil {
		return nil, false, nil
	}
	userID := strings.TrimSpace(session.NewAPI.UserID)
	accessToken := normalizeImportedBearerToken(session.NewAPI.AccessToken)
	if accessToken != "" {
		if userID == "" {
			return nil, true, fmt.Errorf("new-api Access Token 登录态缺少 user_id / New-Api-User，无法读取账号快照")
		}
		headers := http.Header{}
		headers.Set("Authorization", newAPIAuthorizationHeader(accessToken))
		headers.Set("New-Api-User", userID)
		user := &newAPIUser{ID: userID, AccessToken: accessToken}
		snapshot, err := c.fetchSnapshotWithAuthenticatedSession(ctx, api, quotaPerUnit, user, headers)
		if err != nil {
			return nil, true, fmt.Errorf("new-api Access Token 登录态不可用：%w", err)
		}
		snapshot.AuthSession = &AuthenticatedSession{
			Platform:   PlatformNewAPI,
			BaseURL:    api.baseURL,
			AuthMode:   NormalizeAuthMode(firstNonEmpty(session.AuthMode, AuthModeAccessToken)),
			ImportedAt: session.ImportedAt,
			UpdatedAt:  common.GetTimestamp(),
			NewAPI: &NewAPISessionData{
				UserID:      userID,
				AccessToken: accessToken,
			},
		}
		return snapshot, true, nil
	}
	if err := restoreCookiesToJar(api, session.NewAPI.Cookies); err != nil {
		return nil, true, fmt.Errorf("new-api Cookie 登录态恢复失败：%w", err)
	}
	user := &newAPIUser{ID: userID}
	if userID == "" {
		self, err := c.fetchSelf(ctx, api, http.Header{})
		if err != nil || stringValue(self.ID) == "" {
			return nil, true, fmt.Errorf("new-api Cookie 登录态不可用，无法读取当前用户：%w", err)
		}
		user = &self
	}
	snapshot, err := c.fetchSnapshotWithAuthenticatedSession(ctx, api, quotaPerUnit, user, http.Header{})
	if err != nil {
		return nil, true, fmt.Errorf("new-api Cookie 登录态不可用：%w", err)
	}
	if snapshot.AuthSession != nil {
		snapshot.AuthSession.AuthMode = NormalizeAuthMode(firstNonEmpty(session.AuthMode, AuthModeSessionCookie))
		snapshot.AuthSession.ImportedAt = session.ImportedAt
	}
	return snapshot, true, nil
}

func (c *NewAPIClient) fetchSnapshotWithAuthenticatedSession(ctx context.Context, api *httpClient, quotaPerUnit float64, user *newAPIUser, headers http.Header) (*Snapshot, error) {
	if headers == nil {
		headers = http.Header{}
	}
	if user == nil {
		user = &newAPIUser{}
	}
	if auth := newAPIAuthorizationHeader(user.accessToken()); auth != "" && strings.TrimSpace(headers.Get("Authorization")) == "" {
		headers.Set("Authorization", auth)
	}
	if userID := stringValue(user.ID); userID != "" {
		headers.Set("New-Api-User", userID)
	}

	self, err := c.fetchSelf(ctx, api, headers)
	if err == nil {
		mergeNewAPIUser(&self, *user)
		*user = self
		if userID := stringValue(user.ID); userID != "" {
			headers.Set("New-Api-User", userID)
		}
	} else if stringValue(user.ID) == "" {
		return nil, fmt.Errorf("new-api 登录响应缺少用户 ID，且读取当前用户失败：%w", err)
	} else if strings.TrimSpace(user.Group) == "" {
		return nil, err
	}
	if userID := stringValue(user.ID); userID == "" {
		return nil, fmt.Errorf("new-api 登录响应和当前用户响应均缺少用户 ID")
	}
	groups, groupRates, groupWarnings := c.fetchGroupsBestEffort(ctx, api, headers)
	warnings := append([]string{}, groupWarnings...)
	rates, ratioWarnings := c.fetchRatioConfig(ctx, api)
	warnings = append(warnings, ratioWarnings...)
	keys, tokenWarnings, err := c.fetchTokens(ctx, api, headers, quotaPerUnit, groupRates, rates)
	if err != nil {
		return nil, err
	}
	warnings = append(warnings, tokenWarnings...)

	snapshot := &Snapshot{
		Platform: PlatformNewAPI,
		BaseURL:  api.baseURL,
		User: &UserSnapshot{
			ID:       stringValue(user.ID),
			Username: user.Username,
			Email:    user.Email,
			Group:    user.Group,
		},
		Balance:  buildNewAPIBalance(*user, quotaPerUnit),
		Groups:   groups,
		Keys:     keys,
		Rates:    rates,
		Warnings: warnings,
	}
	snapshot.AuthSession = buildNewAPIAuthenticatedSession(api, snapshot)
	if token := user.accessToken(); token != "" {
		if snapshot.AuthSession == nil {
			snapshot.AuthSession = &AuthenticatedSession{
				Platform:  PlatformNewAPI,
				BaseURL:   api.baseURL,
				AuthMode:  AuthModePassword,
				UpdatedAt: common.GetTimestamp(),
				NewAPI: &NewAPISessionData{
					UserID: stringValue(user.ID),
				},
			}
		}
		if snapshot.AuthSession.NewAPI == nil {
			snapshot.AuthSession.NewAPI = &NewAPISessionData{UserID: stringValue(user.ID)}
		}
		snapshot.AuthSession.NewAPI.AccessToken = normalizeImportedBearerToken(token)
	}
	ApplySuggestions(snapshot)
	return snapshot, nil
}

func buildNewAPIAuthenticatedSession(api *httpClient, snapshot *Snapshot) *AuthenticatedSession {
	if api == nil || snapshot == nil || snapshot.User == nil {
		return nil
	}
	userID := strings.TrimSpace(snapshot.User.ID)
	if userID == "" {
		return nil
	}
	cookies := storeCookiesFromJar(api)
	if len(cookies) == 0 {
		return nil
	}
	return &AuthenticatedSession{
		Platform:  PlatformNewAPI,
		BaseURL:   api.baseURL,
		AuthMode:  AuthModePassword,
		UpdatedAt: common.GetTimestamp(),
		NewAPI: &NewAPISessionData{
			UserID:  userID,
			Cookies: cookies,
		},
	}
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
		status.QuotaPerUnit = defaultNewAPIQuotaPerUnit
	}
	return &status, nil
}

func (c *NewAPIClient) fetchStatusBestEffort(ctx context.Context, api *httpClient) (float64, []string) {
	status, err := c.fetchStatus(ctx, api)
	if err != nil {
		return defaultNewAPIQuotaPerUnit, []string{
			"new-api /api/status 不可用，已使用默认额度换算：" + common.MaskSensitiveInfo(err.Error()),
		}
	}
	return status.QuotaPerUnit, nil
}

func (c *NewAPIClient) login(ctx context.Context, api *httpClient, credential Credential) (*newAPIUser, http.Header, bool, error) {
	username := strings.TrimSpace(credential.Username)
	email := strings.TrimSpace(credential.Email)
	if email == "" && strings.Contains(username, "@") {
		email = username
	}
	identity := firstNonEmpty(username, email)
	if identity == "" || strings.TrimSpace(credential.Password) == "" {
		return nil, nil, false, fmt.Errorf("new-api 账号和密码不能为空")
	}
	bodies := uniqueStringMaps([]map[string]string{
		{
			"username": identity,
			"password": credential.Password,
		},
		{
			"email":    firstNonEmpty(email, identity),
			"password": credential.Password,
		},
		{
			"username": identity,
			"email":    firstNonEmpty(email, identity),
			"password": credential.Password,
		},
	})
	var lastErr error
	for _, body := range bodies {
		var envelope newAPIEnvelope[newAPIUser]
		if err := api.postJSON(ctx, "/api/user/login?turnstile=", nil, body, &envelope); err != nil {
			lastErr = err
			continue
		}
		user, err := unwrapNewAPI(envelope)
		if err != nil {
			lastErr = err
			continue
		}
		if user.Require2FA {
			return &user, nil, true, nil
		}
		headers := http.Header{}
		if auth := newAPIAuthorizationHeader(user.accessToken()); auth != "" {
			headers.Set("Authorization", auth)
		}
		return &user, headers, false, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("new-api 登录失败")
	}
	return nil, nil, false, lastErr
}

func (c *NewAPIClient) fetchSelf(ctx context.Context, api *httpClient, headers http.Header) (newAPIUser, error) {
	var lastErr error
	for _, path := range []string{"/api/user/self", "/api/user/me", "/api/user/profile", "/api/user/info"} {
		var envelope newAPIEnvelope[newAPIUser]
		if err := api.getJSON(ctx, path, headers, &envelope); err != nil {
			lastErr = err
			continue
		}
		user, err := unwrapNewAPI(envelope)
		if err != nil {
			lastErr = err
			continue
		}
		return user, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("new-api 当前用户接口不可用")
	}
	return newAPIUser{}, lastErr
}

func (c *NewAPIClient) fetchGroups(ctx context.Context, api *httpClient, headers http.Header) ([]SyncedGroup, map[string]float64, error) {
	var envelope newAPIEnvelope[map[string]newAPIGroup]
	var lastErr error
	for _, path := range []string{"/api/user/self/groups", "/api/user/groups"} {
		if err := api.getJSON(ctx, path, headers, &envelope); err != nil {
			lastErr = err
			continue
		}
		rawGroups, err := unwrapNewAPI(envelope)
		if err != nil {
			lastErr = err
			continue
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
	if lastErr == nil {
		lastErr = fmt.Errorf("new-api 分组接口不可用")
	}
	return nil, nil, lastErr
}

func (c *NewAPIClient) fetchGroupsBestEffort(ctx context.Context, api *httpClient, headers http.Header) ([]SyncedGroup, map[string]float64, []string) {
	groups, groupRates, err := c.fetchGroups(ctx, api, headers)
	if err != nil {
		return nil, map[string]float64{}, []string{
			"new-api 分组接口不可用，已仅按密钥自身分组信息导入：" + common.MaskSensitiveInfo(err.Error()),
		}
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

func (c *NewAPIClient) fetchTokens(ctx context.Context, api *httpClient, headers http.Header, quotaPerUnit float64, groupRates map[string]float64, rates *RateSnapshot) ([]SyncedKey, []string, error) {
	var tokens []newAPIToken
	for page := 1; page <= 1000; page++ {
		query := url.Values{
			"p":         {strconv.Itoa(page)},
			"page_size": {"100"},
		}.Encode()
		pageData, err := c.fetchTokenPage(ctx, api, headers, query)
		if err != nil {
			return nil, nil, err
		}
		tokens = append(tokens, pageData.Items...)
		if len(pageData.Items) == 0 || len(tokens) >= pageData.Total {
			break
		}
	}
	if len(tokens) == 0 {
		return nil, nil, nil
	}
	fullKeys, revealFailures := c.fetchTokenKeys(ctx, api, headers, tokens)
	warnings := make([]string, 0, len(revealFailures))
	result := make([]SyncedKey, 0, len(tokens))
	for _, token := range tokens {
		externalID := stringValue(token.ID)
		key := strings.TrimSpace(fullKeys[externalID])
		if key == "" && !strings.Contains(token.Key, "*") {
			key = strings.TrimSpace(token.Key)
		}
		if key == "" {
			name := firstNonEmpty(token.Name, externalID, token.Key)
			if failure := revealFailures[externalID]; failure != nil {
				warnings = append(warnings, fmt.Sprintf("new-api 密钥 %s 完整 Key 读取失败，已跳过该密钥：%s", name, common.MaskSensitiveInfo(failure.Error())))
			} else {
				warnings = append(warnings, fmt.Sprintf("new-api 密钥 %s 缺少完整 key，已跳过该密钥", name))
			}
			continue
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
		if token.UnlimitedQuota {
			// 无限额度不能把上游接口返回的 0 误显示成“剩余 0”。
			// 已用量仍然有效，但额度上限和剩余量保持缺失，由前端展示为“-”。
			synced.QuotaLimitUSD = nil
			synced.QuotaRemainingUSD = nil
		}
		if rates != nil {
			synced.ModelRatios = filterModelRatios(rates.ModelRatios, synced.Models)
		}
		result = append(result, synced)
	}
	if len(result) == 0 {
		return nil, warnings, fmt.Errorf("读取 new-api 完整 Key 失败：未读取到任何可同步的完整 Key，请检查目标平台完整 Key 读取权限")
	}
	return result, warnings, nil
}

func (c *NewAPIClient) fetchTokenPage(ctx context.Context, api *httpClient, headers http.Header, query string) (newAPITokenList, error) {
	var lastErr error
	for _, basePath := range []string{"/api/token/", "/api/token", "/api/tokens"} {
		path := appendQueryString(basePath, query)
		var envelope newAPIEnvelope[newAPITokenList]
		if err := api.getJSON(ctx, path, headers, &envelope); err != nil {
			lastErr = err
			continue
		}
		pageData, err := unwrapNewAPI(envelope)
		if err != nil {
			lastErr = err
			continue
		}
		return pageData, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("new-api 密钥列表接口不可用")
	}
	return newAPITokenList{}, lastErr
}

func (c *NewAPIClient) fetchTokenKeys(ctx context.Context, api *httpClient, headers http.Header, tokens []newAPIToken) (map[string]string, map[string]error) {
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
	failures := map[string]error{}
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
			failures[externalID] = err
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
			failures[externalID] = fmt.Errorf("读取 new-api 完整 Key 失败：%w", fallbackErr)
			continue
		}
		if batchErr != nil {
			failures[externalID] = fmt.Errorf("读取 new-api 完整 Key 失败：%w", batchErr)
			continue
		}
		failures[externalID] = fmt.Errorf("读取 new-api 完整 Key 失败：token %s 未返回完整 key", externalID)
	}
	return normalized, failures
}

func (c *NewAPIClient) fetchTokenKey(ctx context.Context, api *httpClient, headers http.Header, externalID string) (string, error) {
	var envelope newAPIEnvelope[struct {
		Key string `json:"key"`
	}]
	path := "/api/token/" + url.PathEscape(externalID) + "/key"
	if err := api.postJSON(ctx, path, headers, nil, &envelope); err != nil {
		var getEnvelope newAPIEnvelope[struct {
			Key string `json:"key"`
		}]
		if getErr := api.getJSON(ctx, path, headers, &getEnvelope); getErr != nil {
			return "", err
		}
		envelope = getEnvelope
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

func mergeNewAPIUser(target *newAPIUser, fallback newAPIUser) {
	if target == nil {
		return
	}
	if target.ID == nil {
		target.ID = fallback.ID
	}
	if strings.TrimSpace(target.Username) == "" {
		target.Username = fallback.Username
	}
	if strings.TrimSpace(target.Email) == "" {
		target.Email = fallback.Email
	}
	if strings.TrimSpace(target.Group) == "" {
		target.Group = fallback.Group
	}
	if target.Quota == 0 {
		target.Quota = fallback.Quota
	}
	if target.UsedQuota == 0 {
		target.UsedQuota = fallback.UsedQuota
	}
	if strings.TrimSpace(target.AccessToken) == "" {
		target.AccessToken = fallback.AccessToken
	}
	if strings.TrimSpace(target.Token) == "" {
		target.Token = fallback.Token
	}
}

func (u newAPIUser) accessToken() string {
	return firstNonEmpty(u.AccessToken, u.Token)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func newAPIAuthorizationHeader(raw string) string {
	token := normalizeImportedBearerToken(raw)
	if token == "" {
		return ""
	}
	return "Bearer " + token
}

func appendSnapshotWarnings(snapshot *Snapshot, warnings ...string) {
	if snapshot == nil || len(warnings) == 0 {
		return
	}
	for _, warning := range warnings {
		warning = strings.TrimSpace(warning)
		if warning == "" {
			continue
		}
		snapshot.Warnings = append(snapshot.Warnings, warning)
	}
}

func appendQueryString(path string, query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return path
	}
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return path + separator + query
}
