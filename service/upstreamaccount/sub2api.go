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

// Sub2APIClient 负责从 sub2api 平台读取账号快照。
type Sub2APIClient struct {
	httpClient *http.Client
}

// NewSub2APIClient 创建 sub2api 客户端。
func NewSub2APIClient(client *http.Client) *Sub2APIClient {
	return &Sub2APIClient{httpClient: client}
}

type sub2APILoginResponse struct {
	AccessToken  string      `json:"access_token"`
	RefreshToken string      `json:"refresh_token"`
	ExpiresIn    int64       `json:"expires_in"`
	User         sub2APIUser `json:"user"`
	Requires2FA  bool        `json:"requires_2fa"`
	TempToken    string      `json:"temp_token"`
}

type sub2APIUser struct {
	ID       any     `json:"id"`
	Username string  `json:"username"`
	Email    string  `json:"email"`
	Balance  float64 `json:"balance"`
}

type sub2APIGroup struct {
	ID                 any     `json:"id"`
	Name               string  `json:"name"`
	Platform           string  `json:"platform"`
	Description        string  `json:"description"`
	RateMultiplier     float64 `json:"rate_multiplier"`
	PeakRateMultiplier float64 `json:"peak_rate_multiplier"`
}

type sub2APIKeyList struct {
	Items []sub2APIKey `json:"items"`
	Total int          `json:"total"`
}

type sub2APIKey struct {
	ID        any              `json:"id"`
	Name      string           `json:"name"`
	Key       string           `json:"key"`
	Status    sub2APIKeyStatus `json:"status"`
	GroupID   any              `json:"group_id"`
	Group     sub2APIGroup     `json:"group"`
	Models    []string         `json:"models"`
	Quota     float64          `json:"quota"`
	QuotaUsed float64          `json:"quota_used"`
}

// sub2APIKeyStatus 兼容 sub2api 不同版本的 API Key 状态字段。
//
// 早期 mock 与部分部署可能返回数字状态；当前真实 sub2api 返回 `active`、
// `inactive`、`quota_exhausted`、`expired` 等字符串枚举。这里在解析阶段统一
// 映射为 NexusTok 渠道账号状态，避免真实平台新增 key 后预览失败。
type sub2APIKeyStatus struct {
	value int
}

// UnmarshalJSON 支持数字、数字字符串和字符串枚举三类状态格式。
func (s *sub2APIKeyStatus) UnmarshalJSON(data []byte) error {
	raw := strings.TrimSpace(string(data))
	if raw == "" || raw == "null" {
		s.value = common.ChannelStatusEnabled
		return nil
	}
	var number int
	if err := common.Unmarshal(data, &number); err == nil {
		s.value = normalizeSub2APIKeyNumericStatus(number)
		return nil
	}
	var text string
	if err := common.Unmarshal(data, &text); err != nil {
		return err
	}
	s.value = normalizeSub2APIKeyTextStatus(text)
	return nil
}

func normalizeSub2APIKeyNumericStatus(status int) int {
	if status <= 0 {
		return common.ChannelStatusEnabled
	}
	return status
}

func normalizeSub2APIKeyTextStatus(status string) int {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "active", "enabled", "enable", "ok", "normal", "1":
		return common.ChannelStatusEnabled
	case "inactive", "disabled", "disable", "quota_exhausted", "expired", "deleted", "revoked", "2":
		return common.ChannelStatusManuallyDisabled
	case "auto_disabled", "auto-disabled", "error", "errored", "3":
		return common.ChannelStatusAutoDisabled
	default:
		return common.ChannelStatusManuallyDisabled
	}
}

type sub2APIUsageStats struct {
	TotalActualCost float64 `json:"total_actual_cost"`
	TotalCost       float64 `json:"total_cost"`
	TodayActualCost float64 `json:"today_actual_cost"`
}

// FetchSnapshot 登录 sub2api 并读取当前账号可见的密钥、分组、倍率和余额。
func (c *Sub2APIClient) FetchSnapshot(ctx context.Context, credential Credential) (*Snapshot, error) {
	snapshot, challenge, err := c.BeginPreview(ctx, credential)
	if err != nil {
		return nil, err
	}
	if challenge != nil {
		return nil, fmt.Errorf("sub2api 账号启用了 2FA，请使用二阶段验证码完成上游账号同步")
	}
	return snapshot, nil
}

// BeginPreview 开始 sub2api 账号同步预览。
//
// 普通账号会直接返回完整后端快照；启用 2FA 的账号只返回短期 challenge。客户端生成
// 的 challenge 只包含 sub2api 登录接口返回的 temp_token，不包含正式 access_token；
// 调用方可以再额外挂载加密后的账号凭据，供 2FA 完成后落库复用。
func (c *Sub2APIClient) BeginPreview(ctx context.Context, credential Credential) (*Snapshot, *AuthChallengeRecord, error) {
	credential.BaseURL = normalizeSub2APIBaseURL(credential.BaseURL)
	api, err := newHTTPClient(credential.BaseURL, c.httpClient)
	if err != nil {
		return nil, nil, err
	}
	if snapshot, ok := c.fetchSnapshotWithSavedSession(ctx, api, credential.Session); ok {
		return snapshot, nil, nil
	}
	if credentialRequiresImportedSession(credential, AuthModeAccessToken) {
		return nil, nil, fmt.Errorf("sub2api Access Token 登录态不可用：请确认 token 未过期并具备读取分组、密钥和余额的权限")
	}
	login, err := c.login(ctx, api, credential)
	if err != nil {
		return nil, nil, err
	}
	if login.Requires2FA || strings.TrimSpace(login.TempToken) != "" {
		tempToken := strings.TrimSpace(login.TempToken)
		if tempToken == "" {
			return nil, nil, fmt.Errorf("sub2api 账号启用了 2FA，但登录响应缺少 temp_token")
		}
		return nil, &AuthChallengeRecord{
			Platform: PlatformSub2API,
			BaseURL:  api.baseURL,
			Email:    strings.TrimSpace(firstNonEmpty(credential.Email, credential.Username)),
			Sub2API: &Sub2APIChallengeData{
				TempToken: tempToken,
			},
		}, nil
	}
	snapshot, err := c.fetchSnapshotWithAuthenticatedSession(ctx, api, login.AccessToken, login.User)
	if err == nil {
		snapshot.AuthSession = buildSub2APIAuthenticatedSession(api.baseURL, login)
	}
	return snapshot, nil, err
}

// Complete2FA 使用已缓存的 sub2api temp_token 和管理员输入的验证码完成预览。
func (c *Sub2APIClient) Complete2FA(ctx context.Context, record AuthChallengeRecord, code string) (*Snapshot, error) {
	if record.Sub2API == nil || strings.TrimSpace(record.Sub2API.TempToken) == "" {
		return nil, fmt.Errorf("sub2api 二次验证会话无效，请重新同步上游账号")
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, fmt.Errorf("验证码不能为空")
	}
	api, err := newHTTPClient(record.BaseURL, c.httpClient)
	if err != nil {
		return nil, err
	}
	var envelope sub2APIEnvelope[sub2APILoginResponse]
	body := map[string]string{
		"temp_token": record.Sub2API.TempToken,
		"totp_code":  code,
	}
	if err := api.postJSON(ctx, "/api/v1/auth/login/2fa", nil, body, &envelope); err != nil {
		return nil, err
	}
	login, err := unwrapSub2API(envelope)
	if err != nil {
		return nil, fmt.Errorf("sub2api 2FA 验证失败：%w", err)
	}
	snapshot, err := c.fetchSnapshotWithAuthenticatedSession(ctx, api, login.AccessToken, login.User)
	if err == nil {
		snapshot.AuthSession = buildSub2APIAuthenticatedSession(api.baseURL, &login)
	}
	return snapshot, err
}

func (c *Sub2APIClient) fetchSnapshotWithSavedSession(ctx context.Context, api *httpClient, session *AuthenticatedSession) (*Snapshot, bool) {
	if api == nil || !authSessionMatches(session, PlatformSub2API, api.baseURL) || session.Sub2API == nil {
		return nil, false
	}
	token := strings.TrimSpace(session.Sub2API.AccessToken)
	if token == "" {
		return nil, false
	}
	if session.Sub2API.ExpiresAt > 0 && session.Sub2API.ExpiresAt <= common.GetTimestamp()+30 {
		return nil, false
	}
	snapshot, err := c.fetchSnapshotWithAuthenticatedSession(ctx, api, token, sub2APIUser{})
	if err != nil {
		return nil, false
	}
	snapshot.AuthSession = &AuthenticatedSession{
		Platform:   PlatformSub2API,
		BaseURL:    api.baseURL,
		AuthMode:   NormalizeAuthMode(firstNonEmpty(session.AuthMode, AuthModeAccessToken)),
		ImportedAt: session.ImportedAt,
		UpdatedAt:  common.GetTimestamp(),
		Sub2API: &Sub2APISessionData{
			AccessToken:  token,
			RefreshToken: session.Sub2API.RefreshToken,
			ExpiresAt:    session.Sub2API.ExpiresAt,
		},
	}
	return snapshot, true
}

func buildSub2APIAuthenticatedSession(baseURL string, login *sub2APILoginResponse) *AuthenticatedSession {
	if login == nil || strings.TrimSpace(login.AccessToken) == "" {
		return nil
	}
	expiresAt := int64(0)
	if login.ExpiresIn > 0 {
		expiresAt = common.GetTimestamp() + login.ExpiresIn
	}
	return &AuthenticatedSession{
		Platform:  PlatformSub2API,
		BaseURL:   baseURL,
		AuthMode:  AuthModePassword,
		UpdatedAt: common.GetTimestamp(),
		Sub2API: &Sub2APISessionData{
			AccessToken:  strings.TrimSpace(login.AccessToken),
			RefreshToken: strings.TrimSpace(login.RefreshToken),
			ExpiresAt:    expiresAt,
		},
	}
}

func (c *Sub2APIClient) fetchSnapshotWithAuthenticatedSession(ctx context.Context, api *httpClient, accessToken string, user sub2APIUser) (*Snapshot, error) {
	if strings.TrimSpace(accessToken) == "" {
		return nil, fmt.Errorf("sub2api 登录响应缺少 access_token")
	}
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+accessToken)

	if me, err := c.fetchMe(ctx, api, headers); err == nil {
		user = me
	}
	profile, profileErr := c.fetchProfile(ctx, api, headers)
	if profileErr == nil {
		user.Balance = profile.Balance
		if strings.TrimSpace(user.Email) == "" {
			user.Email = profile.Email
		}
		if strings.TrimSpace(user.Username) == "" {
			user.Username = profile.Username
		}
	}
	groups, groupRates, err := c.fetchGroups(ctx, api, headers)
	if err != nil {
		return nil, err
	}
	usage, warnings := c.fetchUsage(ctx, api, headers)
	keys, err := c.fetchKeys(ctx, api, headers, groupRates)
	if err != nil {
		return nil, err
	}

	snapshot := &Snapshot{
		Platform: PlatformSub2API,
		BaseURL:  api.baseURL,
		User: &UserSnapshot{
			ID:       stringValue(user.ID),
			Username: user.Username,
			Email:    user.Email,
		},
		Balance: buildSub2APIBalance(user, usage),
		Groups:  groups,
		Keys:    keys,
		Rates: &RateSnapshot{
			GroupRates: groupRates,
			Source:     "sub2api:groups",
		},
		Warnings: warnings,
	}
	ApplySuggestions(snapshot)
	return snapshot, nil
}

func normalizeSub2APIBaseURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	u, err := url.Parse(trimmed)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return raw
	}
	switch strings.TrimRight(u.EscapedPath(), "/") {
	case "/login", "/dashboard", "/register", "/setup":
		// 用户通常会复制 sub2api 的前端页面地址作为测试地址；后端接口实际固定在
		// 同站点的 /api/v1 下。只剥离明确的前端路由，避免破坏带反向代理 API
		// 前缀的部署地址。
		u.Path = ""
		u.RawPath = ""
		u.RawQuery = ""
		u.Fragment = ""
		return strings.TrimRight(u.String(), "/")
	default:
		return raw
	}
}

func (c *Sub2APIClient) login(ctx context.Context, api *httpClient, credential Credential) (*sub2APILoginResponse, error) {
	email := strings.TrimSpace(credential.Email)
	if email == "" {
		email = strings.TrimSpace(credential.Username)
	}
	if email == "" || strings.TrimSpace(credential.Password) == "" {
		return nil, fmt.Errorf("sub2api 邮箱和密码不能为空")
	}
	var envelope sub2APIEnvelope[sub2APILoginResponse]
	body := map[string]string{
		"email":    email,
		"password": credential.Password,
	}
	if err := api.postJSON(ctx, "/api/v1/auth/login", nil, body, &envelope); err != nil {
		return nil, err
	}
	login, err := unwrapSub2API(envelope)
	if err != nil {
		return nil, err
	}
	return &login, nil
}

func (c *Sub2APIClient) fetchMe(ctx context.Context, api *httpClient, headers http.Header) (sub2APIUser, error) {
	var envelope sub2APIEnvelope[sub2APIUser]
	if err := api.getJSON(ctx, "/api/v1/auth/me", headers, &envelope); err != nil {
		return sub2APIUser{}, err
	}
	return unwrapSub2API(envelope)
}

func (c *Sub2APIClient) fetchProfile(ctx context.Context, api *httpClient, headers http.Header) (sub2APIUser, error) {
	var envelope sub2APIEnvelope[sub2APIUser]
	if err := api.getJSON(ctx, "/api/v1/user/profile", headers, &envelope); err != nil {
		return sub2APIUser{}, err
	}
	return unwrapSub2API(envelope)
}

func (c *Sub2APIClient) fetchGroups(ctx context.Context, api *httpClient, headers http.Header) ([]SyncedGroup, map[string]float64, error) {
	var envelope sub2APIEnvelope[[]sub2APIGroup]
	if err := api.getJSON(ctx, "/api/v1/groups/available", headers, &envelope); err != nil {
		return nil, nil, err
	}
	rawGroups, err := unwrapSub2API(envelope)
	if err != nil {
		return nil, nil, err
	}
	groups := make([]SyncedGroup, 0, len(rawGroups))
	groupRates := make(map[string]float64, len(rawGroups))
	for _, group := range rawGroups {
		id := stringValue(group.ID)
		ratio := group.RateMultiplier
		if ratio <= 0 {
			ratio = 1
		}
		if id != "" {
			groupRates[id] = ratio
		}
		if group.Name != "" {
			groupRates[group.Name] = ratio
		}
		groups = append(groups, SyncedGroup{
			ID:          id,
			Name:        group.Name,
			Platform:    group.Platform,
			Ratio:       floatPtr(ratio),
			PeakRatio:   optionalPositiveFloat(group.PeakRateMultiplier),
			Description: group.Description,
		})
	}
	if userRates := c.fetchUserGroupRates(ctx, api, headers); len(userRates) > 0 {
		for key, ratio := range userRates {
			groupRates[key] = ratio
			for i := range groups {
				if groups[i].ID == key || groups[i].Name == key {
					groups[i].Ratio = floatPtr(ratio)
				}
			}
		}
	}
	return groups, groupRates, nil
}

func (c *Sub2APIClient) fetchUserGroupRates(ctx context.Context, api *httpClient, headers http.Header) map[string]float64 {
	var envelope sub2APIEnvelope[map[string]float64]
	if err := api.getJSON(ctx, "/api/v1/groups/rates", headers, &envelope); err != nil {
		return nil
	}
	rates, err := unwrapSub2API(envelope)
	if err != nil {
		return nil
	}
	return rates
}

func (c *Sub2APIClient) fetchUsage(ctx context.Context, api *httpClient, headers http.Header) (*sub2APIUsageStats, []string) {
	var envelope sub2APIEnvelope[sub2APIUsageStats]
	if err := api.getJSON(ctx, "/api/v1/usage/dashboard/stats", headers, &envelope); err != nil {
		return nil, []string{"sub2api 未返回账号累计用量，已仅同步余额和 key 维度用量"}
	}
	usage, err := unwrapSub2API(envelope)
	if err != nil {
		return nil, []string{"sub2api 用量统计不可用：" + err.Error()}
	}
	return &usage, nil
}

func (c *Sub2APIClient) fetchKeys(ctx context.Context, api *httpClient, headers http.Header, groupRates map[string]float64) ([]SyncedKey, error) {
	var keys []sub2APIKey
	for page := 1; page <= 1000; page++ {
		path := "/api/v1/keys?" + url.Values{
			"page":      {strconv.Itoa(page)},
			"page_size": {"100"},
		}.Encode()
		var envelope sub2APIEnvelope[sub2APIKeyList]
		if err := api.getJSON(ctx, path, headers, &envelope); err != nil {
			return nil, err
		}
		pageData, err := unwrapSub2API(envelope)
		if err != nil {
			return nil, err
		}
		keys = append(keys, pageData.Items...)
		if len(pageData.Items) == 0 || len(keys) >= pageData.Total {
			break
		}
	}
	result := make([]SyncedKey, 0, len(keys))
	for _, key := range keys {
		groupID := stringValue(key.GroupID)
		if groupID == "" {
			groupID = stringValue(key.Group.ID)
		}
		groupName := key.Group.Name
		groupRatio, hasRatio := groupRates[groupID]
		if !hasRatio && groupName != "" {
			groupRatio, hasRatio = groupRates[groupName]
		}
		quotaRemaining := key.Quota - key.QuotaUsed
		if quotaRemaining < 0 {
			quotaRemaining = 0
		}
		synced := SyncedKey{
			ExternalID:        stringValue(key.ID),
			Name:              key.Name,
			Key:               key.Key,
			MaskedKey:         maskKey(key.Key),
			Status:            key.Status.value,
			GroupID:           groupID,
			GroupName:         groupName,
			Models:            key.Models,
			QuotaUsedUSD:      floatPtr(key.QuotaUsed),
			QuotaRemainingUSD: floatPtr(quotaRemaining),
			Unlimited:         key.Quota == 0,
		}
		if key.Quota > 0 {
			synced.QuotaLimitUSD = floatPtr(key.Quota)
		}
		if hasRatio {
			synced.GroupRatio = floatPtr(groupRatio)
		}
		result = append(result, synced)
	}
	return result, nil
}

func buildSub2APIBalance(user sub2APIUser, usage *sub2APIUsageStats) *BalanceSnapshot {
	balance := user.Balance
	result := &BalanceSnapshot{
		BalanceUSD: floatPtr(balance),
		RawBalance: floatPtr(balance),
		Source:     "sub2api:user/profile",
	}
	if usage == nil {
		result.Partial = true
		result.MissingUsedValue = true
		return result
	}
	result.UsedUSD = floatPtr(usage.TotalActualCost)
	result.RawUsed = floatPtr(usage.TotalActualCost)
	return result
}

func optionalPositiveFloat(value float64) *float64 {
	if value <= 0 {
		return nil
	}
	return floatPtr(value)
}
