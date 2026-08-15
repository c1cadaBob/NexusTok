package upstreamaccount

import (
	"context"
	"errors"
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

type sub2APIRefreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	TokenType    string `json:"token_type"`
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

// UnmarshalJSON 兼容 sub2api 不同版本的密钥列表 envelope。
//
// 生产站点里常见的形态包括 `items/total`、`data/total`、`list/count`，
// 也有接口直接把 data 返回成数组。这里统一归一化，避免分页字段差异导致同步不到 key。
func (l *sub2APIKeyList) UnmarshalJSON(data []byte) error {
	var direct []sub2APIKey
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
		var items []sub2APIKey
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

// UnmarshalJSON 兼容 sub2api 的 models 数组、字符串或空值三种形态。
func (k *sub2APIKey) UnmarshalJSON(data []byte) error {
	var raw struct {
		ID        any              `json:"id"`
		Name      string           `json:"name"`
		Key       string           `json:"key"`
		Status    sub2APIKeyStatus `json:"status"`
		GroupID   any              `json:"group_id"`
		Group     sub2APIGroup     `json:"group"`
		Models    any              `json:"models"`
		Quota     float64          `json:"quota"`
		QuotaUsed float64          `json:"quota_used"`
	}
	if err := common.Unmarshal(data, &raw); err != nil {
		return err
	}
	*k = sub2APIKey{
		ID:        raw.ID,
		Name:      raw.Name,
		Key:       raw.Key,
		Status:    raw.Status,
		GroupID:   raw.GroupID,
		Group:     raw.Group,
		Quota:     raw.Quota,
		QuotaUsed: raw.QuotaUsed,
	}
	if raw.Models != nil {
		k.Models = splitStringValues([]string{modelsValueToCSV(raw.Models)})
	}
	return nil
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
	TotalActualCost *float64 `json:"total_actual_cost"`
	TotalCost       *float64 `json:"total_cost"`
	TodayActualCost *float64 `json:"today_actual_cost"`
}

type sub2APISnapshotStage string

const (
	sub2APIStageAuthMe  sub2APISnapshotStage = "auth_me"
	sub2APIStageProfile sub2APISnapshotStage = "profile"
	sub2APIStageGroups  sub2APISnapshotStage = "groups"
	sub2APIStageKeys    sub2APISnapshotStage = "keys"
	sub2APIStageRefresh sub2APISnapshotStage = "refresh"
)

// sub2APISnapshotError 保留快照读取失败的具体阶段，避免把管理接口 404、
// 分组权限不足、密钥列表失败等问题全部误报为 Access Token 不可用。
type sub2APISnapshotError struct {
	Stage sub2APISnapshotStage
	Err   error
}

func (e *sub2APISnapshotError) Error() string {
	if e == nil {
		return ""
	}
	label := sub2APIStageMessage(e.Stage)
	var httpErr *upstreamHTTPError
	if errors.As(e.Err, &httpErr) {
		path := firstNonEmpty(httpErr.Path, sub2APIStagePath(e.Stage))
		method := firstNonEmpty(httpErr.Method, http.MethodGet)
		return fmt.Sprintf("%s：%s %s 返回 status=%d, body=%s", label, method, path, httpErr.StatusCode, httpErr.Body)
	}
	if e.Err != nil {
		return fmt.Sprintf("%s：%v", label, e.Err)
	}
	return label
}

func (e *sub2APISnapshotError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func wrapSub2APISnapshotError(stage sub2APISnapshotStage, err error) error {
	if err == nil {
		return nil
	}
	return &sub2APISnapshotError{Stage: stage, Err: err}
}

func sub2APIStageMessage(stage sub2APISnapshotStage) string {
	switch stage {
	case sub2APIStageAuthMe:
		return "Sub2API Access Token 校验失败"
	case sub2APIStageProfile:
		return "Sub2API 已登录，但读取用户详情失败"
	case sub2APIStageGroups:
		return "Sub2API 已登录，但读取分组失败"
	case sub2APIStageKeys:
		return "Sub2API 已登录，但读取密钥失败"
	case sub2APIStageRefresh:
		return "Sub2API Access Token 刷新失败"
	default:
		return "Sub2API 同步失败"
	}
}

func sub2APIStagePath(stage sub2APISnapshotStage) string {
	switch stage {
	case sub2APIStageAuthMe:
		return "/api/v1/auth/me"
	case sub2APIStageProfile:
		return "/api/v1/user/profile"
	case sub2APIStageGroups:
		return "/api/v1/groups/available"
	case sub2APIStageKeys:
		return "/api/v1/keys"
	case sub2APIStageRefresh:
		return "/api/v1/auth/refresh"
	default:
		return ""
	}
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
	managementBaseURL, relayBaseURL, err := c.resolveSub2APIBaseURLs(ctx, credential)
	if err != nil {
		return nil, nil, err
	}
	credential.BaseURL = managementBaseURL
	credential.ManagementBaseURL = managementBaseURL
	credential.RelayBaseURL = relayBaseURL

	api, err := newHTTPClient(managementBaseURL, c.httpClient)
	if err != nil {
		return nil, nil, err
	}
	if snapshot, ok, err := c.fetchSnapshotWithSavedSession(ctx, api, credential.Session, relayBaseURL); ok || err != nil {
		return snapshot, nil, err
	}
	if credentialRequiresImportedSession(credential, AuthModeAccessToken) {
		return nil, nil, fmt.Errorf("sub2api Access Token 登录态不可用：请确认 token 未过期并具备读取分组、密钥和余额的权限")
	}
	login, err := c.login(ctx, api, credential)
	if err != nil {
		return nil, nil, friendlySub2APIEndpointError(err)
	}
	if login.Requires2FA || strings.TrimSpace(login.TempToken) != "" {
		tempToken := strings.TrimSpace(login.TempToken)
		if tempToken == "" {
			return nil, nil, fmt.Errorf("sub2api 账号启用了 2FA，但登录响应缺少 temp_token")
		}
		return nil, &AuthChallengeRecord{
			Platform:     PlatformSub2API,
			BaseURL:      api.baseURL,
			RelayBaseURL: relayBaseURL,
			Email:        strings.TrimSpace(firstNonEmpty(credential.Email, credential.Username)),
			Sub2API: &Sub2APIChallengeData{
				TempToken: tempToken,
			},
		}, nil
	}
	snapshot, err := c.fetchSnapshotWithAuthenticatedSession(ctx, api, login.AccessToken, login.User, relayBaseURL)
	if err == nil {
		snapshot.AuthSession = buildSub2APIAuthenticatedSession(api.baseURL, login)
	}
	return snapshot, nil, friendlySub2APIEndpointError(err)
}

// resolveSub2APIBaseURLs 为账号同步拆出两个不同用途的地址。
//
// managementBaseURL 始终用于登录、auth/me、分组、密钥等管理接口；relayBaseURL
// 仅用于最终创建 NexusTok 渠道时的模型请求地址。aiapipay.com 这类站点会在页面
// 配置中声明 relay API 域名，但管理接口仍留在面板域名，因此不能再用 relay 覆盖
// HTTP client 的 baseURL。
func (c *Sub2APIClient) resolveSub2APIBaseURLs(ctx context.Context, credential Credential) (string, string, error) {
	managementCandidate := firstNonEmpty(credential.ManagementBaseURL, credential.BaseURL)
	managementBaseURL, err := normalizeBaseURL(normalizeSub2APIBaseURL(managementCandidate))
	if err != nil {
		return "", "", err
	}

	relayBaseURL := strings.TrimSpace(credential.RelayBaseURL)
	if isLikelyAPISubdomain(managementBaseURL) {
		if recoveredManagementBaseURL, ok := c.recoverSub2APIManagementBaseURLFromRelay(ctx, managementBaseURL); ok {
			if relayBaseURL == "" {
				relayBaseURL = managementBaseURL
			}
			managementBaseURL = recoveredManagementBaseURL
		}
	}

	if relayBaseURL == "" {
		if discoveredRelayBaseURL, ok := c.discoverSub2APIRelayBaseURLFromPanel(ctx, managementBaseURL); ok {
			relayBaseURL = discoveredRelayBaseURL
		}
	}
	if relayBaseURL == "" {
		relayBaseURL = managementBaseURL
	} else {
		relayBaseURL, err = normalizeBaseURL(normalizeSub2APIBaseURL(relayBaseURL))
		if err != nil {
			return "", "", err
		}
		if err := validateRelatedAPIBaseURL(managementBaseURL, relayBaseURL); err != nil {
			return "", "", err
		}
	}
	return managementBaseURL, relayBaseURL, nil
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
	relayBaseURL := firstNonEmpty(record.RelayBaseURL, record.BaseURL)
	snapshot, err := c.fetchSnapshotWithAuthenticatedSession(ctx, api, login.AccessToken, login.User, relayBaseURL)
	if err == nil {
		snapshot.AuthSession = buildSub2APIAuthenticatedSession(api.baseURL, &login)
	}
	return snapshot, err
}

func (c *Sub2APIClient) fetchSnapshotWithSavedSession(ctx context.Context, api *httpClient, session *AuthenticatedSession, relayBaseURL string) (*Snapshot, bool, error) {
	if api == nil || !authSessionMatches(session, PlatformSub2API, api.baseURL) || session.Sub2API == nil {
		return nil, false, nil
	}
	token := strings.TrimSpace(session.Sub2API.AccessToken)
	if token == "" {
		return nil, false, nil
	}
	refreshToken := strings.TrimSpace(session.Sub2API.RefreshToken)
	expiresAt := normalizeUnixSeconds(session.Sub2API.ExpiresAt)
	if expiresAt > 0 && expiresAt <= common.GetTimestamp()+30 {
		if refreshToken == "" {
			return nil, true, fmt.Errorf("Sub2API Access Token 已过期，当前登录态没有 refresh_token，请重新使用油猴脚本采集。")
		}
		refreshed, err := c.refreshAccessToken(ctx, api, refreshToken)
		if err != nil {
			return nil, true, friendlySub2APIEndpointError(wrapSub2APISnapshotError(sub2APIStageRefresh, err))
		}
		token = refreshed.AccessToken
		refreshToken = firstNonEmpty(refreshed.RefreshToken, refreshToken)
		expiresAt = refreshed.ExpiresAt
	}
	snapshot, err := c.fetchSnapshotWithAuthenticatedSession(ctx, api, token, sub2APIUser{}, relayBaseURL)
	if err != nil {
		if !isSub2APIAuthFailure(err) {
			return nil, true, friendlySub2APIEndpointError(err)
		}
		if refreshToken == "" {
			return nil, true, fmt.Errorf("Sub2API Access Token 不可用，当前登录态没有 refresh_token，请重新使用油猴脚本采集。")
		}
		refreshed, refreshErr := c.refreshAccessToken(ctx, api, refreshToken)
		if refreshErr != nil {
			return nil, true, fmt.Errorf("Sub2API Access Token 不可用且 refresh_token 刷新失败：%w", friendlySub2APIEndpointError(wrapSub2APISnapshotError(sub2APIStageRefresh, refreshErr)))
		}
		token = refreshed.AccessToken
		refreshToken = firstNonEmpty(refreshed.RefreshToken, refreshToken)
		expiresAt = refreshed.ExpiresAt
		snapshot, err = c.fetchSnapshotWithAuthenticatedSession(ctx, api, token, sub2APIUser{}, relayBaseURL)
		if err != nil {
			return nil, true, friendlySub2APIEndpointError(err)
		}
	}
	snapshot.AuthSession = &AuthenticatedSession{
		Platform:   PlatformSub2API,
		BaseURL:    api.baseURL,
		AuthMode:   NormalizeAuthMode(firstNonEmpty(session.AuthMode, AuthModeAccessToken)),
		ImportedAt: session.ImportedAt,
		UpdatedAt:  common.GetTimestamp(),
		Sub2API: &Sub2APISessionData{
			AccessToken:  token,
			RefreshToken: refreshToken,
			ExpiresAt:    expiresAt,
		},
	}
	return snapshot, true, nil
}

func isSub2APIAuthFailure(err error) bool {
	if err == nil {
		return false
	}
	var snapshotErr *sub2APISnapshotError
	if !errors.As(err, &snapshotErr) || snapshotErr.Stage != sub2APIStageAuthMe {
		return false
	}
	var httpErr *upstreamHTTPError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode == http.StatusUnauthorized || httpErr.StatusCode == http.StatusForbidden
	}
	message := strings.ToLower(snapshotErr.Err.Error())
	return strings.Contains(message, "token") ||
		strings.Contains(message, "unauthorized") ||
		strings.Contains(message, "forbidden") ||
		strings.Contains(message, "expired") ||
		strings.Contains(message, "无效") ||
		strings.Contains(message, "过期") ||
		strings.Contains(message, "未登录") ||
		strings.Contains(message, "未授权")
}

type refreshedSub2APIToken struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    int64
}

func (c *Sub2APIClient) refreshAccessToken(ctx context.Context, api *httpClient, refreshToken string) (*refreshedSub2APIToken, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return nil, fmt.Errorf("sub2api refresh_token 为空")
	}
	var envelope sub2APIEnvelope[sub2APIRefreshResponse]
	if err := api.postJSON(ctx, "/api/v1/auth/refresh", nil, map[string]string{"refresh_token": refreshToken}, &envelope); err != nil {
		return nil, err
	}
	data, err := unwrapSub2API(envelope)
	if err != nil {
		return nil, err
	}
	accessToken := strings.TrimSpace(data.AccessToken)
	if accessToken == "" {
		return nil, fmt.Errorf("sub2api refresh 响应缺少 access_token")
	}
	expiresAt := int64(0)
	if data.ExpiresIn > 0 {
		expiresAt = common.GetTimestamp() + data.ExpiresIn
	}
	return &refreshedSub2APIToken{
		AccessToken:  accessToken,
		RefreshToken: strings.TrimSpace(data.RefreshToken),
		ExpiresAt:    expiresAt,
	}, nil
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

func (c *Sub2APIClient) fetchSnapshotWithAuthenticatedSession(ctx context.Context, api *httpClient, accessToken string, user sub2APIUser, relayBaseURL string) (*Snapshot, error) {
	if strings.TrimSpace(accessToken) == "" {
		return nil, fmt.Errorf("sub2api 登录响应缺少 access_token")
	}
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+accessToken)

	me, err := c.fetchMe(ctx, api, headers)
	if err != nil {
		return nil, wrapSub2APISnapshotError(sub2APIStageAuthMe, err)
	}
	user = me

	warnings := make([]string, 0, 2)
	profile, profileErr := c.fetchProfile(ctx, api, headers)
	if profileErr == nil {
		user.Balance = profile.Balance
		if strings.TrimSpace(user.Email) == "" {
			user.Email = profile.Email
		}
		if strings.TrimSpace(user.Username) == "" {
			user.Username = profile.Username
		}
	} else {
		warnings = append(warnings, "sub2api 用户详情不可用，已使用 auth/me 返回的基础信息")
	}
	groups, groupRates, err := c.fetchGroups(ctx, api, headers)
	if err != nil {
		return nil, wrapSub2APISnapshotError(sub2APIStageGroups, err)
	}
	usage, usageWarnings := c.fetchUsage(ctx, api, headers)
	warnings = append(warnings, usageWarnings...)
	keys, err := c.fetchKeys(ctx, api, headers, groupRates)
	if err != nil {
		return nil, wrapSub2APISnapshotError(sub2APIStageKeys, err)
	}
	relayBaseURL = normalizeSyncMetadataBaseURL(PlatformSub2API, firstNonEmpty(relayBaseURL, api.baseURL))

	snapshot := &Snapshot{
		Platform:          PlatformSub2API,
		BaseURL:           relayBaseURL,
		ManagementBaseURL: api.baseURL,
		RelayBaseURL:      relayBaseURL,
		User: &UserSnapshot{
			ID:       stringValue(user.ID),
			Username: user.Username,
			Email:    user.Email,
		},
		Groups: groups,
		Keys:   keys,
		Rates: &RateSnapshot{
			GroupRates: groupRates,
			Source:     "sub2api:groups",
		},
		Warnings: warnings,
	}
	// 账号级 usage 接口在不同 Sub2API 版本中并不稳定；如果累计字段缺失，
	// 仍然使用同一轮同步拿到的密钥已用量做近似回退，并明确标记 partial。
	// 这样创建、手动刷新和系统同步不会因为一个可选统计接口缺失而把已用量
	// 伪装成 0。
	snapshot.Balance = buildSub2APIBalance(user, usage, keys)
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
	case "/login", "/dashboard", "/register", "/setup", "/home":
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

func friendlySub2APIEndpointError(err error) error {
	if err == nil {
		return nil
	}
	var snapshotErr *sub2APISnapshotError
	if errors.As(err, &snapshotErr) {
		return err
	}
	message := err.Error()
	if strings.Contains(message, "status=410") && strings.Contains(message, "endpoint_migrated") {
		return fmt.Errorf("目标站要求使用已发布 API 端点，请检查 Sub2API 页面配置中的 API 端点")
	}
	return err
}

func (c *Sub2APIClient) login(ctx context.Context, api *httpClient, credential Credential) (*sub2APILoginResponse, error) {
	email := strings.TrimSpace(credential.Email)
	username := strings.TrimSpace(credential.Username)
	if email == "" && strings.Contains(username, "@") {
		email = username
	}
	identity := firstNonEmpty(email, username)
	if identity == "" || strings.TrimSpace(credential.Password) == "" {
		return nil, fmt.Errorf("sub2api 邮箱和密码不能为空")
	}
	bodies := uniqueStringMaps([]map[string]string{
		{
			"email":    firstNonEmpty(email, identity),
			"password": credential.Password,
		},
		{
			"username": firstNonEmpty(username, identity),
			"password": credential.Password,
		},
		{
			"email":    firstNonEmpty(email, identity),
			"username": firstNonEmpty(username, identity),
			"password": credential.Password,
		},
	})
	var lastErr error
	for _, body := range bodies {
		var envelope sub2APIEnvelope[sub2APILoginResponse]
		if err := api.postJSON(ctx, "/api/v1/auth/login", nil, body, &envelope); err != nil {
			lastErr = err
			continue
		}
		login, err := unwrapSub2API(envelope)
		if err != nil {
			lastErr = err
			continue
		}
		return &login, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("sub2api 登录失败")
	}
	return nil, lastErr
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
			ExternalID:   stringValue(key.ID),
			Name:         key.Name,
			Key:          key.Key,
			MaskedKey:    maskKey(key.Key),
			Status:       key.Status.value,
			GroupID:      groupID,
			GroupName:    groupName,
			Models:       key.Models,
			QuotaUsedUSD: floatPtr(key.QuotaUsed),
			Unlimited:    key.Quota == 0,
		}
		if key.Quota > 0 {
			synced.QuotaLimitUSD = floatPtr(key.Quota)
			synced.QuotaRemainingUSD = floatPtr(quotaRemaining)
		}
		if hasRatio {
			synced.GroupRatio = floatPtr(groupRatio)
		}
		result = append(result, synced)
	}
	return result, nil
}

func buildSub2APIBalance(user sub2APIUser, usage *sub2APIUsageStats, keys []SyncedKey) *BalanceSnapshot {
	balance := user.Balance
	result := &BalanceSnapshot{
		BalanceUSD: floatPtr(balance),
		RawBalance: floatPtr(balance),
		Source:     "sub2api:user/profile",
	}
	if usage == nil {
		result.Partial = true
		result.MissingUsedValue = true
		attachSub2APIKeyUsageFallback(result, keys)
		return result
	}
	var used *float64
	switch {
	case usage.TotalActualCost != nil && finiteNonNegativeFloat(*usage.TotalActualCost):
		used = usage.TotalActualCost
	case usage.TotalCost != nil && finiteNonNegativeFloat(*usage.TotalCost):
		used = usage.TotalCost
	default:
		result.Partial = true
		result.MissingUsedValue = true
		attachSub2APIKeyUsageFallback(result, keys)
		return result
	}
	value := *used
	result.UsedUSD = &value
	result.RawUsed = &value
	return result
}

func attachSub2APIKeyUsageFallback(result *BalanceSnapshot, keys []SyncedKey) {
	if result == nil {
		return
	}
	var keyUsed float64
	hasKeyUsed := false
	for i := range keys {
		if !finiteNonNegativeFloatPtr(keys[i].QuotaUsedUSD) {
			continue
		}
		keyUsed += *keys[i].QuotaUsedUSD
		hasKeyUsed = true
	}
	if !hasKeyUsed {
		return
	}
	value := keyUsed
	result.UsedUSD = &value
	result.RawUsed = &value
	result.Source = "sub2api:keys"
}

func optionalPositiveFloat(value float64) *float64 {
	if value <= 0 {
		return nil
	}
	return floatPtr(value)
}
