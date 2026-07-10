// Package oauth - generic.go
// 该文件实现了通用 OAuth 提供商（GenericOAuthProvider）
//
// 功能说明：
// - 支持任意符合 OAuth 2.0 标准的提供商
// - 可通过数据库配置自定义端点、字段映射、访问策略
// - 支持多种认证方式（参数传递、Basic Auth）
// - 支持访问策略（Access Policy）控制用户访问权限
//
// 访问策略系统：
// - 支持 and/or 逻辑组合
// - 支持 12 种操作符（eq、ne、gt、gte、lt、lte、in、not_in、contains、not_contains、exists、not_exists）
// - 支持嵌套策略组
// - 支持自定义拒绝消息模板
package oauth

import (
	"context"
	"encoding/base64"
	stdjson "encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/i18n"
	"github.com/c1cada/NexusTok/logger"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/tidwall/gjson"
)

// AuthStyle 定义客户端凭证的传递方式
const (
	AuthStyleAutoDetect = 0 // 自动检测：根据服务器响应自动选择认证方式
	AuthStyleInParams   = 1 // 参数传递：将 client_id 和 client_secret 作为 POST 参数发送
	AuthStyleInHeader   = 2 // Basic Auth：将 client_id:client_secret 编码为 Base64 放入 Authorization 头
)

// GenericOAuthProvider 实现通用 OAuth 提供商
// 支持任意符合 OAuth 2.0 标准的提供商，所有配置通过数据库动态管理
// 适用于非内置的 OAuth 提供商（如企业 SSO、自建认证服务等）
type GenericOAuthProvider struct {
	config *model.CustomOAuthProvider // 提供商配置，包含端点、密钥、字段映射等
}

// accessPolicy 表示访问策略的 JSON 结构
// 支持 and/or 逻辑组合和嵌套策略组，用于控制哪些用户可以通过此提供商登录
type accessPolicy struct {
	Logic      string            `json:"logic"`      // 逻辑运算符："and"（全部满足）或 "or"（任一满足），默认为 "and"
	Conditions []accessCondition `json:"conditions"` // 条件列表
	Groups     []accessPolicy    `json:"groups"`     // 嵌套的策略组，支持多层嵌套
}

// accessCondition 表示访问策略中的单个条件
// 通过字段路径、操作符和期望值来匹配用户信息
type accessCondition struct {
	Field string `json:"field"` // JSON 字段路径（支持 gjson 语法，如 "email"、"organizations.#.name"）
	Op    string `json:"op"`    // 操作符（eq、ne、gt、gte、lt、lte、in、not_in、contains、not_contains、exists、not_exists）
	Value any    `json:"value"` // 期望值（可以是字符串、数字、布尔值或数组）
}

// accessPolicyFailure 记录访问策略评估失败的详细信息
// 用于生成面向用户的拒绝消息模板
type accessPolicyFailure struct {
	Field    string // 失败的字段路径
	Op       string // 使用的操作符
	Expected any    // 期望的值
	Current  any    // 实际的值
}

// supportedAccessPolicyOps 定义了所有支持的访问策略操作符
var supportedAccessPolicyOps = []string{
	"eq",
	"ne",
	"gt",
	"gte",
	"lt",
	"lte",
	"in",
	"not_in",
	"contains",
	"not_contains",
	"exists",
	"not_exists",
}

// NewGenericOAuthProvider 根据数据库配置创建通用 OAuth 提供商实例
// 参数：
//   - config：自定义 OAuth 提供商的数据库配置（包含端点、密钥、字段映射等）
//
// 返回值：初始化好的 GenericOAuthProvider 实例
func NewGenericOAuthProvider(config *model.CustomOAuthProvider) *GenericOAuthProvider {
	return &GenericOAuthProvider{config: config}
}

// GetName 返回提供商的显示名称
func (p *GenericOAuthProvider) GetName() string {
	return p.config.Name
}

// IsEnabled 检查提供商是否已启用
func (p *GenericOAuthProvider) IsEnabled() bool {
	return p.config.Enabled
}

// GetConfig 返回提供商的数据库配置对象
func (p *GenericOAuthProvider) GetConfig() *model.CustomOAuthProvider {
	return p.config
}

// ExchangeToken 使用授权码向提供商的 Token 端点交换访问令牌
// 支持三种认证方式：参数传递（AuthStyleInParams）、Basic Auth（AuthStyleInHeader）、自动检测
// 支持两种响应格式：JSON 和 URL-encoded（兼容 GitHub 等旧式提供商）
func (p *GenericOAuthProvider) ExchangeToken(ctx context.Context, code string, c *gin.Context) (*OAuthToken, error) {
	if code == "" {
		return nil, NewOAuthError(i18n.MsgOAuthInvalidCode, nil)
	}

	logger.LogDebug(ctx, "[OAuth-Generic-%s] ExchangeToken: code=%s...", p.config.Slug, code[:min(len(code), 10)])

	redirectUri := fmt.Sprintf("%s/oauth/%s", system_setting.ServerAddress, p.config.Slug)
	values := url.Values{}
	values.Set("grant_type", "authorization_code")
	values.Set("code", code)
	values.Set("redirect_uri", redirectUri)

	// Determine auth style
	authStyle := p.config.AuthStyle
	if authStyle == AuthStyleAutoDetect {
		// Default to params style for most OAuth servers
		authStyle = AuthStyleInParams
	}

	var req *http.Request
	var err error

	if authStyle == AuthStyleInParams {
		values.Set("client_id", p.config.ClientId)
		values.Set("client_secret", p.config.ClientSecret)
	}

	if err := validateConfiguredOAuthEndpointURL(p.config.TokenEndpoint); err != nil {
		logger.LogError(ctx, fmt.Sprintf("[OAuth-Generic-%s] ExchangeToken endpoint blocked: %s", p.config.Slug, err.Error()))
		return nil, NewOAuthErrorWithRaw(i18n.MsgOAuthConnectFailed, map[string]any{"Provider": p.config.Name}, err.Error())
	}

	req, err = http.NewRequestWithContext(ctx, "POST", p.config.TokenEndpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	if authStyle == AuthStyleInHeader {
		// Basic Auth
		credentials := base64.StdEncoding.EncodeToString([]byte(p.config.ClientId + ":" + p.config.ClientSecret))
		req.Header.Set("Authorization", "Basic "+credentials)
	}

	logger.LogDebug(ctx, "[OAuth-Generic-%s] ExchangeToken: token_endpoint=%s, redirect_uri=%s, auth_style=%d",
		p.config.Slug, p.config.TokenEndpoint, redirectUri, authStyle)

	client := newConfiguredOAuthHTTPClient(20 * time.Second)
	res, err := client.Do(req)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("[OAuth-Generic-%s] ExchangeToken error: %s", p.config.Slug, err.Error()))
		return nil, NewOAuthErrorWithRaw(i18n.MsgOAuthConnectFailed, map[string]any{"Provider": p.config.Name}, err.Error())
	}
	defer res.Body.Close()

	logger.LogDebug(ctx, "[OAuth-Generic-%s] ExchangeToken response status: %d", p.config.Slug, res.StatusCode)

	body, err := io.ReadAll(res.Body)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("[OAuth-Generic-%s] ExchangeToken read body error: %s", p.config.Slug, err.Error()))
		return nil, err
	}

	bodyStr := string(body)
	logger.LogDebug(ctx, "[OAuth-Generic-%s] ExchangeToken response body: %s", p.config.Slug, bodyStr[:min(len(bodyStr), 500)])

	// Try to parse as JSON first
	var tokenResponse struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		Scope        string `json:"scope"`
		IDToken      string `json:"id_token"`
		Error        string `json:"error"`
		ErrorDesc    string `json:"error_description"`
	}

	if err := common.Unmarshal(body, &tokenResponse); err != nil {
		// Try to parse as URL-encoded (some OAuth servers like GitHub return this format)
		parsedValues, parseErr := url.ParseQuery(bodyStr)
		if parseErr != nil {
			logger.LogError(ctx, fmt.Sprintf("[OAuth-Generic-%s] ExchangeToken parse error: %s", p.config.Slug, err.Error()))
			return nil, err
		}
		tokenResponse.AccessToken = parsedValues.Get("access_token")
		tokenResponse.TokenType = parsedValues.Get("token_type")
		tokenResponse.Scope = parsedValues.Get("scope")
	}

	if tokenResponse.Error != "" {
		logger.LogError(ctx, fmt.Sprintf("[OAuth-Generic-%s] ExchangeToken OAuth error: %s - %s",
			p.config.Slug, tokenResponse.Error, tokenResponse.ErrorDesc))
		return nil, NewOAuthErrorWithRaw(i18n.MsgOAuthTokenFailed, map[string]any{"Provider": p.config.Name}, tokenResponse.ErrorDesc)
	}

	if tokenResponse.AccessToken == "" {
		logger.LogError(ctx, fmt.Sprintf("[OAuth-Generic-%s] ExchangeToken failed: empty access token", p.config.Slug))
		return nil, NewOAuthError(i18n.MsgOAuthTokenFailed, map[string]any{"Provider": p.config.Name})
	}

	logger.LogDebug(ctx, "[OAuth-Generic-%s] ExchangeToken success: scope=%s", p.config.Slug, tokenResponse.Scope)

	return &OAuthToken{
		AccessToken:  tokenResponse.AccessToken,
		TokenType:    tokenResponse.TokenType,
		RefreshToken: tokenResponse.RefreshToken,
		ExpiresIn:    tokenResponse.ExpiresIn,
		Scope:        tokenResponse.Scope,
		IDToken:      tokenResponse.IDToken,
	}, nil
}

// GetUserInfo 使用访问令牌从提供商的 UserInfo 端点获取用户信息
// 通过配置中的字段映射（UserIdField、UsernameField 等）提取用户数据
// 支持 gjson 路径语法（如 "user.id"、"emails.#(primary==true).value"）
// 如果配置了访问策略（Access Policy），还会评估用户是否满足访问条件
func (p *GenericOAuthProvider) GetUserInfo(ctx context.Context, token *OAuthToken) (*OAuthUser, error) {
	logger.LogDebug(ctx, "[OAuth-Generic-%s] GetUserInfo: fetching user info from %s", p.config.Slug, p.config.UserInfoEndpoint)

	if err := validateConfiguredOAuthEndpointURL(p.config.UserInfoEndpoint); err != nil {
		logger.LogError(ctx, fmt.Sprintf("[OAuth-Generic-%s] GetUserInfo endpoint blocked: %s", p.config.Slug, err.Error()))
		return nil, NewOAuthErrorWithRaw(i18n.MsgOAuthConnectFailed, map[string]any{"Provider": p.config.Name}, err.Error())
	}

	req, err := http.NewRequestWithContext(ctx, "GET", p.config.UserInfoEndpoint, nil)
	if err != nil {
		return nil, err
	}

	// Set authorization header
	tokenType := normalizeAuthorizationTokenType(token.TokenType)
	req.Header.Set("Authorization", fmt.Sprintf("%s %s", tokenType, token.AccessToken))
	req.Header.Set("Accept", "application/json")

	client := newConfiguredOAuthHTTPClient(20 * time.Second)
	res, err := client.Do(req)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("[OAuth-Generic-%s] GetUserInfo error: %s", p.config.Slug, err.Error()))
		return nil, NewOAuthErrorWithRaw(i18n.MsgOAuthConnectFailed, map[string]any{"Provider": p.config.Name}, err.Error())
	}
	defer res.Body.Close()

	logger.LogDebug(ctx, "[OAuth-Generic-%s] GetUserInfo response status: %d", p.config.Slug, res.StatusCode)

	if res.StatusCode != http.StatusOK {
		logger.LogError(ctx, fmt.Sprintf("[OAuth-Generic-%s] GetUserInfo failed: status=%d", p.config.Slug, res.StatusCode))
		return nil, NewOAuthError(i18n.MsgOAuthGetUserErr, nil)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("[OAuth-Generic-%s] GetUserInfo read body error: %s", p.config.Slug, err.Error()))
		return nil, err
	}

	bodyStr := string(body)
	logger.LogDebug(ctx, "[OAuth-Generic-%s] GetUserInfo response body: %s", p.config.Slug, bodyStr[:min(len(bodyStr), 500)])

	// Extract fields using gjson (supports JSONPath-like syntax)
	userId := gjson.Get(bodyStr, p.config.UserIdField).String()
	username := gjson.Get(bodyStr, p.config.UsernameField).String()
	displayName := gjson.Get(bodyStr, p.config.DisplayNameField).String()
	email := gjson.Get(bodyStr, p.config.EmailField).String()

	// If user ID field returns a number, convert it
	if userId == "" {
		// Try to get as number
		userIdNum := gjson.Get(bodyStr, p.config.UserIdField)
		if userIdNum.Exists() {
			userId = userIdNum.Raw
			// Remove quotes if present
			userId = strings.Trim(userId, "\"")
		}
	}

	if userId == "" {
		logger.LogError(ctx, fmt.Sprintf("[OAuth-Generic-%s] GetUserInfo failed: empty user ID (field: %s)", p.config.Slug, p.config.UserIdField))
		return nil, NewOAuthError(i18n.MsgOAuthUserInfoEmpty, map[string]any{"Provider": p.config.Name})
	}

	logger.LogDebug(ctx, "[OAuth-Generic-%s] GetUserInfo success: id=%s, username=%s, name=%s, email=%s",
		p.config.Slug, userId, username, displayName, email)

	policyRaw := strings.TrimSpace(p.config.AccessPolicy)
	if policyRaw != "" {
		policy, err := parseAccessPolicy(policyRaw)
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("[OAuth-Generic-%s] invalid access policy: %s", p.config.Slug, err.Error()))
			return nil, NewOAuthErrorWithRaw(i18n.MsgOAuthGetUserErr, nil, "invalid access policy configuration")
		}
		allowed, failure := evaluateAccessPolicy(bodyStr, policy)
		if !allowed {
			message := renderAccessDeniedMessage(p.config.AccessDeniedMessage, p.config.Name, bodyStr, failure)
			logger.LogWarn(ctx, fmt.Sprintf("[OAuth-Generic-%s] access denied by policy: field=%s op=%s expected=%v current=%v",
				p.config.Slug, failure.Field, failure.Op, failure.Expected, failure.Current))
			return nil, &AccessDeniedError{Message: message}
		}
	}

	return &OAuthUser{
		ProviderUserID: userId,
		Username:       username,
		DisplayName:    displayName,
		Email:          email,
		Extra: map[string]any{
			"provider": p.config.Slug,
		},
	}, nil
}

// IsUserIDTaken 检查提供商用户 ID 是否已被系统中的其他账号关联
// 查询 OAuth 绑定表（user_oauth_bindings）确认唯一性
func (p *GenericOAuthProvider) IsUserIDTaken(providerUserID string) bool {
	return model.IsProviderUserIdTaken(p.config.Id, providerUserID)
}

// FillUserByProviderID 通过提供商用户 ID 查找关联的系统用户
// 从 OAuth 绑定表中查找匹配记录，然后填充完整的用户信息
func (p *GenericOAuthProvider) FillUserByProviderID(user *model.User, providerUserID string) error {
	foundUser, err := model.GetUserByOAuthBinding(p.config.Id, providerUserID)
	if err != nil {
		return err
	}
	*user = *foundUser
	return nil
}

// SetProviderUserID 对于通用提供商为空操作
// 通用提供商的 OAuth 绑定通过 user_oauth_bindings 表管理
// 具体绑定逻辑在 OAuth 控制器中完成
func (p *GenericOAuthProvider) SetProviderUserID(user *model.User, providerUserID string) {
	// For generic providers, we store the binding in user_oauth_bindings table
	// This is handled separately in the OAuth controller
}

// GetProviderPrefix 返回自动生成用户名时使用的前缀
// 格式为 "{slug}_"，例如 "custom_sso_"
func (p *GenericOAuthProvider) GetProviderPrefix() string {
	return p.config.Slug + "_"
}

// GetProviderId 返回提供商的数据库 ID
// 用于 OAuth 绑定表的关联查询
func (p *GenericOAuthProvider) GetProviderId() int {
	return p.config.Id
}

// normalizeAuthorizationTokenType 标准化 Authorization 头的令牌类型
// 空值或 "Bearer"（不区分大小写）统一返回 "Bearer"
func normalizeAuthorizationTokenType(tokenType string) string {
	tokenType = strings.TrimSpace(tokenType)
	if tokenType == "" || strings.EqualFold(tokenType, "Bearer") {
		return "Bearer"
	}
	return tokenType
}

// IsGenericProvider 标识此提供商为通用提供商（非内置）
// 用于在 OAuth 控制器中区分处理逻辑
func (p *GenericOAuthProvider) IsGenericProvider() bool {
	return true
}

// parseAccessPolicy 解析访问策略的 JSON 字符串
// 返回验证后的策略对象，解析或验证失败时返回错误
func parseAccessPolicy(raw string) (*accessPolicy, error) {
	var policy accessPolicy
	if err := common.UnmarshalJsonStr(raw, &policy); err != nil {
		return nil, err
	}
	if err := validateAccessPolicy(&policy); err != nil {
		return nil, err
	}
	return &policy, nil
}

// validateAccessPolicy 验证访问策略的结构合法性
// 检查逻辑运算符是否有效、是否至少有一个条件或子组
// 递归验证嵌套的策略组
func validateAccessPolicy(policy *accessPolicy) error {
	if policy == nil {
		return errors.New("policy is nil")
	}

	logic := strings.ToLower(strings.TrimSpace(policy.Logic))
	if logic == "" {
		logic = "and"
	}
	if !lo.Contains([]string{"and", "or"}, logic) {
		return fmt.Errorf("unsupported policy logic: %s", logic)
	}
	policy.Logic = logic

	if len(policy.Conditions) == 0 && len(policy.Groups) == 0 {
		return errors.New("policy requires at least one condition or group")
	}

	for index := range policy.Conditions {
		if err := validateAccessCondition(&policy.Conditions[index], index); err != nil {
			return err
		}
	}

	for index := range policy.Groups {
		if err := validateAccessPolicy(&policy.Groups[index]); err != nil {
			return fmt.Errorf("invalid policy group[%d]: %w", index, err)
		}
	}

	return nil
}

// validateAccessPolicy 验证单个访问条件的合法性
// 检查字段路径非空、操作符是否在支持列表中
// 对于 in/not_in 操作符，额外检查值是否为数组类型
func validateAccessCondition(condition *accessCondition, index int) error {
	if condition == nil {
		return fmt.Errorf("condition[%d] is nil", index)
	}

	condition.Field = strings.TrimSpace(condition.Field)
	if condition.Field == "" {
		return fmt.Errorf("condition[%d].field is required", index)
	}

	condition.Op = normalizePolicyOp(condition.Op)
	if !lo.Contains(supportedAccessPolicyOps, condition.Op) {
		return fmt.Errorf("condition[%d].op is unsupported: %s", index, condition.Op)
	}

	if lo.Contains([]string{"in", "not_in"}, condition.Op) {
		if _, ok := condition.Value.([]any); !ok {
			return fmt.Errorf("condition[%d].value must be an array for op %s", index, condition.Op)
		}
	}

	return nil
}

// evaluateAccessPolicy 评估访问策略是否通过
// 根据逻辑运算符（and/or）组合所有条件和子组的评估结果
// 返回值：
//   - bool：策略是否通过
//   - *accessPolicyFailure：失败时返回第一个失败的条件详情
func evaluateAccessPolicy(body string, policy *accessPolicy) (bool, *accessPolicyFailure) {
	if policy == nil {
		return true, nil
	}

	logic := strings.ToLower(strings.TrimSpace(policy.Logic))
	if logic == "" {
		logic = "and"
	}

	hasAny := len(policy.Conditions) > 0 || len(policy.Groups) > 0
	if !hasAny {
		return true, nil
	}

	if logic == "or" {
		var firstFailure *accessPolicyFailure
		for _, cond := range policy.Conditions {
			ok, failure := evaluateAccessCondition(body, cond)
			if ok {
				return true, nil
			}
			if firstFailure == nil {
				firstFailure = failure
			}
		}
		for _, group := range policy.Groups {
			ok, failure := evaluateAccessPolicy(body, &group)
			if ok {
				return true, nil
			}
			if firstFailure == nil {
				firstFailure = failure
			}
		}
		return false, firstFailure
	}

	for _, cond := range policy.Conditions {
		ok, failure := evaluateAccessCondition(body, cond)
		if !ok {
			return false, failure
		}
	}
	for _, group := range policy.Groups {
		ok, failure := evaluateAccessPolicy(body, &group)
		if !ok {
			return false, failure
		}
	}
	return true, nil
}

// evaluateAccessCondition 评估单个访问条件
// 从 JSON 响应体中提取字段值，根据操作符与期望值比较
// 支持 12 种操作符：exists/not_exists/eq/ne/gt/gte/lt/lte/in/not_in/contains/not_contains
func evaluateAccessCondition(body string, cond accessCondition) (bool, *accessPolicyFailure) {
	path := cond.Field
	op := cond.Op
	result := gjson.Get(body, path)
	current := gjsonResultToValue(result)
	failure := &accessPolicyFailure{
		Field:    path,
		Op:       op,
		Expected: cond.Value,
		Current:  current,
	}

	switch op {
	case "exists":
		return result.Exists(), failure
	case "not_exists":
		return !result.Exists(), failure
	case "eq":
		return compareAny(current, cond.Value) == 0, failure
	case "ne":
		return compareAny(current, cond.Value) != 0, failure
	case "gt":
		return compareAny(current, cond.Value) > 0, failure
	case "gte":
		return compareAny(current, cond.Value) >= 0, failure
	case "lt":
		return compareAny(current, cond.Value) < 0, failure
	case "lte":
		return compareAny(current, cond.Value) <= 0, failure
	case "in":
		return valueInSlice(current, cond.Value), failure
	case "not_in":
		return !valueInSlice(current, cond.Value), failure
	case "contains":
		return containsValue(current, cond.Value), failure
	case "not_contains":
		return !containsValue(current, cond.Value), failure
	default:
		return false, failure
	}
}

// normalizePolicyOp 标准化策略操作符（转小写、去空格）
func normalizePolicyOp(op string) string {
	return strings.ToLower(strings.TrimSpace(op))
}

// gjsonResultToValue 将 gjson.Result 转换为 Go 原生类型
// 递归处理嵌套的 JSON 对象和数组，用于策略条件比较
func gjsonResultToValue(result gjson.Result) any {
	if !result.Exists() {
		return nil
	}
	if result.IsArray() {
		arr := result.Array()
		values := make([]any, 0, len(arr))
		for _, item := range arr {
			values = append(values, gjsonResultToValue(item))
		}
		return values
	}
	switch result.Type {
	case gjson.Null:
		return nil
	case gjson.True:
		return true
	case gjson.False:
		return false
	case gjson.Number:
		return result.Num
	case gjson.String:
		return result.String()
	case gjson.JSON:
		var data any
		if err := common.UnmarshalJsonStr(result.Raw, &data); err == nil {
			return data
		}
		return result.Raw
	default:
		return result.Value()
	}
}

// compareAny 通用比较函数，支持数值和字符串比较
// 优先尝试数值比较（toFloat），失败则回退到字符串比较
// 返回值：-1（小于）、0（等于）、1（大于）
func compareAny(left any, right any) int {
	if lf, ok := toFloat(left); ok {
		if rf, ok2 := toFloat(right); ok2 {
			switch {
			case lf < rf:
				return -1
			case lf > rf:
				return 1
			default:
				return 0
			}
		}
	}

	ls := strings.TrimSpace(fmt.Sprint(left))
	rs := strings.TrimSpace(fmt.Sprint(right))
	switch {
	case ls < rs:
		return -1
	case ls > rs:
		return 1
	default:
		return 0
	}
}

// toFloat 将任意类型转换为 float64
// 支持所有数值类型（int/uint/float 各种位宽）、json.Number 和字符串
// 返回值：转换后的浮点数和是否成功
func toFloat(v any) (float64, bool) {
	switch value := v.(type) {
	case float64:
		return value, true
	case float32:
		return float64(value), true
	case int:
		return float64(value), true
	case int8:
		return float64(value), true
	case int16:
		return float64(value), true
	case int32:
		return float64(value), true
	case int64:
		return float64(value), true
	case uint:
		return float64(value), true
	case uint8:
		return float64(value), true
	case uint16:
		return float64(value), true
	case uint32:
		return float64(value), true
	case uint64:
		return float64(value), true
	case stdjson.Number:
		n, err := value.Float64()
		if err == nil {
			return n, true
		}
	case string:
		n, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err == nil {
			return n, true
		}
	}
	return 0, false
}

// valueInSlice 检查当前值是否存在于期望值列表中
// 用于 in/not_in 操作符的评估
func valueInSlice(current any, expected any) bool {
	list, ok := expected.([]any)
	if !ok {
		return false
	}
	return lo.ContainsBy(list, func(item any) bool {
		return compareAny(current, item) == 0
	})
}

// containsValue 检查当前值是否包含期望值
// 对于字符串类型使用 strings.Contains，对于数组类型使用 lo.ContainsBy
// 用于 contains/not_contains 操作符的评估
func containsValue(current any, expected any) bool {
	switch value := current.(type) {
	case string:
		target := strings.TrimSpace(fmt.Sprint(expected))
		return strings.Contains(value, target)
	case []any:
		return lo.ContainsBy(value, func(item any) bool {
			return compareAny(item, expected) == 0
		})
	}
	return false
}

// renderAccessDeniedMessage 渲染访问拒绝消息模板
// 支持以下模板变量：
//   - {{provider}}：提供商名称
//   - {{field}}：失败的字段路径
//   - {{op}}：使用的操作符
//   - {{required}}：期望值
//   - {{current}}：当前值
//   - {{current.xxx}}：从用户 JSON 中提取的字段值（支持 gjson 路径）
//   - {{required.xxx}}：从策略条件中提取的字段值
func renderAccessDeniedMessage(template string, providerName string, body string, failure *accessPolicyFailure) string {
	defaultMessage := "Access denied: your account does not meet this provider's access requirements."
	message := strings.TrimSpace(template)
	if message == "" {
		return defaultMessage
	}

	if failure == nil {
		failure = &accessPolicyFailure{}
	}

	replacements := map[string]string{
		"{{provider}}": providerName,
		"{{field}}":    failure.Field,
		"{{op}}":       failure.Op,
		"{{required}}": fmt.Sprint(failure.Expected),
		"{{current}}":  fmt.Sprint(failure.Current),
	}

	for key, value := range replacements {
		message = strings.ReplaceAll(message, key, value)
	}

	currentPattern := regexp.MustCompile(`\{\{current\.([^}]+)\}\}`)
	message = currentPattern.ReplaceAllStringFunc(message, func(token string) string {
		match := currentPattern.FindStringSubmatch(token)
		if len(match) != 2 {
			return ""
		}
		path := strings.TrimSpace(match[1])
		if path == "" {
			return ""
		}
		return strings.TrimSpace(gjson.Get(body, path).String())
	})

	requiredPattern := regexp.MustCompile(`\{\{required\.([^}]+)\}\}`)
	message = requiredPattern.ReplaceAllStringFunc(message, func(token string) string {
		match := requiredPattern.FindStringSubmatch(token)
		if len(match) != 2 {
			return ""
		}
		path := strings.TrimSpace(match[1])
		if failure.Field == path {
			return fmt.Sprint(failure.Expected)
		}
		return ""
	})

	return strings.TrimSpace(message)
}
