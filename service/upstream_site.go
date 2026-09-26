package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/c1cadaBob/NexusTok/common"
	"github.com/c1cadaBob/NexusTok/logger"
	"github.com/c1cadaBob/NexusTok/model"

	"gorm.io/gorm"
)

const (
	upstreamSiteRequestTimeout = 30 * time.Second
	upstreamSiteResponseLimit  = 2 << 20
	upstreamSitePageSize       = 100
	upstreamSiteMaxPages       = 100
)

var (
	ErrUnsupportedPlatformSite      = errors.New("unsupported upstream platform site")
	ErrPlatformSiteAuth             = errors.New("platform site authentication failed")
	ErrPlatformSiteHTTPStatus       = errors.New("platform site http status failed")
	ErrPlatformSiteResponse         = errors.New("platform site returned an invalid response")
	ErrPlatformSiteCredential       = errors.New("platform site credential unavailable")
	ErrPlatformSiteVerificationCode = errors.New("platform site verification code rejected")
	ErrSub2APILoginRequest          = errors.New("sub2api login request failed")
	ErrSub2APILoginHTTPStatus       = errors.New("sub2api login http status failed")
	ErrSub2APILoginResponse         = errors.New("sub2api login response format failed")
	ErrSub2APILoginToken            = errors.New("sub2api login token missing")
	ErrSub2APILoginInteractive      = errors.New("sub2api login requires interactive verification")
	ErrSub2APICurrentUser           = errors.New("sub2api current user request failed")
	ErrPlatformSiteVerification     = errors.New("platform site verification required")
)

const (
	upstreamKeySyncErrorSecretUnavailable = "credential_unavailable"
	upstreamKeySyncErrorModelsUnavailable = "models_unavailable"
	upstreamKeySyncErrorInvalidData       = "invalid_data"

	platformSiteErrorCategoryAuthentication = "authentication"
	platformSiteErrorCategoryInteractive    = "interactive_verification"
	platformSiteErrorCategoryRouteMissing   = "route_missing"
)

type PlatformSiteSession struct {
	BaseURL           string
	ModelBaseURL      string
	ManagementBaseURL string
	Client            *http.Client
	Headers           http.Header
	CredentialUpdate  *model.PlatformSiteCredential
	StageProgress     func(model.PlatformSiteSyncStages) error
}

type PlatformSitePendingContext struct {
	Kind      string                      `json:"kind"`
	Cookies   []PlatformSitePendingCookie `json:"cookies,omitempty"`
	TempToken string                      `json:"temp_token,omitempty"`
}

// PlatformSitePendingCookie 是交互验证期间暂存的 Cookie 最小字段集合。
// Cookie 只会出现在加密后的 Challenge 上下文中，不会进入 API 响应或日志。
type PlatformSitePendingCookie struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Path     string `json:"path,omitempty"`
	Domain   string `json:"domain,omitempty"`
	Expires  int64  `json:"expires,omitempty"`
	Secure   bool   `json:"secure,omitempty"`
	HttpOnly bool   `json:"http_only,omitempty"`
	SameSite string `json:"same_site,omitempty"`
}

type PlatformSiteVerificationRequired struct {
	Platform string
	BaseURL  string
	Pending  PlatformSitePendingContext
	Cause    error
}

func (err *PlatformSiteVerificationRequired) Error() string {
	return "上游平台需要在真实浏览器中完成交互验证"
}

func (err *PlatformSiteVerificationRequired) Unwrap() error {
	return errors.Join(ErrPlatformSiteVerification, err.Cause)
}

type PlatformSiteAdapter interface {
	Platform() string
	Authenticate(context.Context, string, model.PlatformSiteCredential) (*PlatformSiteSession, error)
	CompleteVerification(context.Context, string, model.PlatformSiteCredential, PlatformSitePendingContext, string) (*PlatformSiteSession, error)
	FetchSnapshot(context.Context, *PlatformSiteSession) (PlatformSiteSnapshot, error)
}

type UpstreamKeySnapshot struct {
	ExternalID               string
	Name                     string
	Secret                   string
	Group                    string
	Models                   []string
	ModelsSynced             bool
	SourceConversionRatio    float64
	SourceConversionRatioSet bool
	ConversionRatio          float64
	ConversionRatioSet       bool
	UsedQuota                int64
	UsedQuotaSet             bool
	RemainQuota              *int64
	ExpiresAt                *time.Time
	Disabled                 bool
	SyncError                string
}

type PlatformSiteSnapshot struct {
	Balance           float64
	BalanceSet        bool
	UsedQuota         int64
	UsedQuotaSet      bool
	Models            []string
	Keys              []UpstreamKeySnapshot
	ManagementBaseURL string
	RelayBaseURL      string
	SyncStages        model.PlatformSiteSyncStages
}

func platformSiteSyncStage(status, message string, err error) model.PlatformSiteSyncStage {
	stage := model.PlatformSiteSyncStage{
		Status:    status,
		UpdatedAt: common.GetTimestamp(),
	}
	if message != "" {
		stage.Error = common.MaskSensitiveInfo(message)
	}
	if err != nil {
		stage.Error = common.MaskSensitiveInfo(err.Error())
		stage.ResponseCategory = platformSiteErrorCategoryOf(err)
		stage.HTTPStatus, _ = platformSiteHTTPStatusCode(err)
		diagnostics, ok := platformSiteResponseDiagnosticsOf(err)
		if ok {
			stage.URL = diagnostics.finalURL
			if stage.URL == "" {
				stage.URL = diagnostics.initialURL
			}
			stage.ContentType = diagnostics.contentType
			stage.Redirected = diagnostics.redirected
			if stage.ResponseCategory == "" {
				stage.ResponseCategory = diagnostics.responseType
			}
		}
	}
	return stage
}

func persistPlatformSiteSyncStages(
	account *model.PlatformSiteAccount,
	stages *model.PlatformSiteSyncStages,
) error {
	if account == nil || stages == nil {
		return errors.New("平台同步阶段不能为空")
	}
	stages.UpdatedAt = common.GetTimestamp()
	raw, err := model.EncodePlatformSiteSyncStages(*stages)
	if err != nil {
		return err
	}
	account.SyncStages = raw
	return model.DB.Model(account).Update("sync_stages", raw).Error
}

func publishPlatformSiteStageProgress(
	session *PlatformSiteSession,
	stages model.PlatformSiteSyncStages,
) error {
	if session == nil || session.StageProgress == nil || stages.Version == 0 {
		return nil
	}
	return session.StageProgress(stages)
}

func updatePlatformSiteSyncStageFailure(stages *model.PlatformSiteSyncStages, err error) {
	if stages == nil || err == nil {
		return
	}
	stages.OverallStatus = model.PlatformSiteStageFailed
	for _, stage := range []model.PlatformSiteSyncStage{
		stages.Authentication,
		stages.CurrentUser,
		stages.BalanceUsage,
		stages.GroupsRates,
		stages.KeyPagination,
		stages.KeySecrets,
		stages.KeyModels,
		stages.Sub2APIEndpoints,
	} {
		if stage.Status == model.PlatformSiteStageFailed {
			return
		}
	}
	stages.Authentication = platformSiteSyncStage(model.PlatformSiteStageFailed, "", err)
}

func markPlatformSiteSyncStagesUsingPrevious(
	stages *model.PlatformSiteSyncStages,
	account *model.PlatformSiteAccount,
) {
	if stages == nil || account == nil || account.LastSyncAt <= 0 {
		return
	}
	stagePointers := []*model.PlatformSiteSyncStage{
		&stages.Authentication,
		&stages.CurrentUser,
		&stages.BalanceUsage,
		&stages.GroupsRates,
		&stages.KeyPagination,
		&stages.KeySecrets,
		&stages.KeyModels,
		&stages.Sub2APIEndpoints,
	}
	for _, stage := range stagePointers {
		if stage.Status == model.PlatformSiteStageWarning ||
			stage.Status == model.PlatformSiteStageFailed ||
			stage.Status == model.PlatformSiteStageWaiting {
			stage.UsedPrevious = true
		}
	}
}

type platformSiteResponseDiagnostics struct {
	statusCode               int
	initialURL               string
	finalURL                 string
	contentType              string
	redirected               bool
	responseType             string
	errorCode                string
	errorReason              string
	errorCategory            string
	verificationCodeRejected bool
	pending                  *PlatformSitePendingContext
}

type platformSiteHTTPStatusError struct {
	statusCode  int
	diagnostics platformSiteResponseDiagnostics
}

func (err *platformSiteHTTPStatusError) Error() string {
	return fmt.Sprintf("%s: HTTP %d", ErrPlatformSiteHTTPStatus, err.statusCode)
}

func (err *platformSiteHTTPStatusError) Unwrap() error {
	return ErrPlatformSiteHTTPStatus
}

type platformSiteResponseError struct {
	diagnostics platformSiteResponseDiagnostics
}

func (err *platformSiteResponseError) Error() string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", ErrPlatformSiteResponse, err.diagnostics.summary())
}

func (err *platformSiteResponseError) Unwrap() error {
	return ErrPlatformSiteResponse
}

type platformSiteBusinessError struct {
	diagnostics platformSiteResponseDiagnostics
	code        string
	reason      string
	category    string
}

func (err *platformSiteBusinessError) Error() string {
	if err == nil {
		return ""
	}
	parts := []string{ErrPlatformSiteAuth.Error()}
	if err.category != "" {
		parts = append(parts, err.category)
	}
	if err.reason != "" {
		parts = append(parts, err.reason)
	} else if err.code != "" {
		parts = append(parts, err.code)
	}
	return strings.Join(parts, ": ")
}

func (err *platformSiteBusinessError) Unwrap() error {
	return ErrPlatformSiteAuth
}

type platformSiteStageError struct {
	stage string
	err   error
}

func (err *platformSiteStageError) Error() string {
	if err == nil {
		return ""
	}
	if err.err == nil {
		return err.stage
	}
	return err.stage + ": " + common.MaskSensitiveInfo(err.err.Error())
}

func (err *platformSiteStageError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.err
}

func wrapPlatformSiteStage(stage string, err error) error {
	if err == nil {
		return nil
	}
	return &platformSiteStageError{stage: stage, err: err}
}

type upstreamSiteSyncLock struct {
	mu sync.Mutex
}

var upstreamSiteLocks sync.Map

func adapterForPlatform(platform string) (PlatformSiteAdapter, error) {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case model.PlatformNewAPI:
		return NewNewAPIAdapter(nil), nil
	case model.PlatformSub2API:
		return NewSub2APIAdapter(nil), nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedPlatformSite, platform)
	}
}

func getUpstreamSiteLock(channelID int) *upstreamSiteSyncLock {
	value, _ := upstreamSiteLocks.LoadOrStore(channelID, &upstreamSiteSyncLock{})
	return value.(*upstreamSiteSyncLock)
}

func normalizePlatformSiteURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("站点地址不能为空")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("站点地址格式错误")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", errors.New("站点地址只允许使用 HTTP 或 HTTPS")
	}
	if parsed.User != nil {
		return "", errors.New("站点地址不允许包含用户信息")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func validatePlatformSiteURL(raw string) error {
	normalized, err := normalizePlatformSiteURL(raw)
	if err != nil {
		return err
	}
	protection := &common.SSRFProtection{
		AllowPrivateIp:         true,
		DomainFilterMode:       false,
		IpFilterMode:           false,
		ApplyIPFilterForDomain: true,
	}
	return protection.ValidateURL(normalized)
}

func validateDiscoveredPlatformSiteURL(baseURL, candidate string) error {
	normalizedCandidate, err := normalizePlatformSiteURL(candidate)
	if err != nil {
		return err
	}
	// 同一已配置来源只改变路径时不会产生新的网络目的地，避免因临时 DNS
	// 解析失败而丢弃合法的转发路径；跨主机发现仍必须完整执行 SSRF 校验。
	if samePlatformSiteOrigin(baseURL, normalizedCandidate) {
		return nil
	}
	return validatePlatformSiteURL(normalizedCandidate)
}

// ValidatePlatformSiteURLForAdmin validates an administrator-provided site
// address before it is persisted or used for outbound requests.
func ValidatePlatformSiteURLForAdmin(raw string) error {
	return validatePlatformSiteURL(raw)
}

func newPlatformSiteHTTPClient() (*http.Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	transport := http.DefaultTransport
	if defaultTransport, ok := http.DefaultTransport.(*http.Transport); ok && defaultTransport != nil {
		transport = defaultTransport.Clone()
	}
	client := &http.Client{
		Transport: transport,
		Jar:       jar,
		Timeout:   upstreamSiteRequestTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("平台站点重定向次数超过限制")
			}
			if err := validatePlatformSiteURL(req.URL.String()); err != nil {
				return fmt.Errorf("平台站点重定向被拒绝: %w", err)
			}
			return nil
		},
	}
	return client, nil
}

func newPlatformSiteSession(baseURL string, headers http.Header) (*PlatformSiteSession, error) {
	normalized, err := normalizePlatformSiteURL(baseURL)
	if err != nil {
		return nil, err
	}
	client, err := newPlatformSiteHTTPClient()
	if err != nil {
		return nil, err
	}
	if headers == nil {
		headers = make(http.Header)
	}
	return &PlatformSiteSession{BaseURL: normalized, Client: client, Headers: headers}, nil
}

func upstreamSiteURL(baseURL, path string, query url.Values) (string, error) {
	normalized, err := normalizePlatformSiteURL(baseURL)
	if err != nil {
		return "", err
	}
	joined := normalized + "/" + strings.TrimLeft(path, "/")
	if len(query) > 0 {
		joined += "?" + query.Encode()
	}
	return joined, nil
}

func platformSiteRequest(
	ctx context.Context,
	session *PlatformSiteSession,
	method string,
	path string,
	query url.Values,
	body any,
) (any, error) {
	if session == nil || session.Client == nil {
		return nil, errors.New("平台站点会话不可用")
	}
	target, err := upstreamSiteURL(session.BaseURL, path, query)
	if err != nil {
		return nil, err
	}
	var reader io.Reader
	if body != nil {
		payload, marshalErr := common.Marshal(body)
		if marshalErr != nil {
			return nil, marshalErr
		}
		reader = strings.NewReader(string(payload))
	}
	request, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	for name, values := range session.Headers {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}
	response, err := session.Client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, upstreamSiteResponseLimit+1))
	if err != nil {
		return nil, err
	}
	if len(data) > upstreamSiteResponseLimit {
		return nil, errors.New("平台站点响应体超过限制")
	}
	diagnostics := platformSiteResponseDiagnosticsFor(target, response)
	diagnostics.responseType = platformSiteResponseType(response.Header.Get("Content-Type"), data)
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		if payload, unmarshalErr := unmarshalPlatformSiteJSON(data); unmarshalErr == nil {
			diagnostics.errorCode, diagnostics.errorReason, diagnostics.errorCategory =
				platformSiteErrorMetadata(payload)
			diagnostics.verificationCodeRejected = platformSiteVerificationCodeMetadata(payload)
			if tempToken := findTemporaryToken(payload); tempToken != "" {
				diagnostics.pending = &PlatformSitePendingContext{
					Kind:      "sub2api_temp_token",
					TempToken: tempToken,
				}
			}
		}
		return nil, &platformSiteHTTPStatusError{
			statusCode:  response.StatusCode,
			diagnostics: diagnostics,
		}
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]any{}, nil
	}
	var payload any
	if err := common.Unmarshal(data, &payload); err != nil {
		return nil, &platformSiteResponseError{diagnostics: diagnostics}
	}
	if object, ok := payload.(map[string]any); ok {
		if success, exists := object["success"].(bool); exists && !success {
			code, reason, category := platformSiteErrorMetadata(object)
			diagnostics.errorCode = code
			diagnostics.errorReason = reason
			diagnostics.errorCategory = category
			diagnostics.verificationCodeRejected = platformSiteVerificationCodeMetadata(object)
			if tempToken := findTemporaryToken(object); tempToken != "" {
				diagnostics.pending = &PlatformSitePendingContext{
					Kind:      "sub2api_temp_token",
					TempToken: tempToken,
				}
			}
			return nil, &platformSiteBusinessError{
				diagnostics: diagnostics,
				code:        code,
				reason:      reason,
				category:    category,
			}
		}
		if code := firstFloat(object, "code"); code != 0 && code != 200 {
			errorCode, reason, category := platformSiteErrorMetadata(object)
			diagnostics.errorCode = errorCode
			diagnostics.errorReason = reason
			diagnostics.errorCategory = category
			diagnostics.verificationCodeRejected = platformSiteVerificationCodeMetadata(object)
			if tempToken := findTemporaryToken(object); tempToken != "" {
				diagnostics.pending = &PlatformSitePendingContext{
					Kind:      "sub2api_temp_token",
					TempToken: tempToken,
				}
			}
			return nil, &platformSiteBusinessError{
				diagnostics: diagnostics,
				code:        errorCode,
				reason:      reason,
				category:    category,
			}
		}
		if code := firstString(object, "code"); code != "" &&
			code != "0" && code != "200" && !strings.EqualFold(code, "success") {
			errorCode, reason, category := platformSiteErrorMetadata(object)
			diagnostics.errorCode = errorCode
			diagnostics.errorReason = reason
			diagnostics.errorCategory = category
			diagnostics.verificationCodeRejected = platformSiteVerificationCodeMetadata(object)
			if tempToken := findTemporaryToken(object); tempToken != "" {
				diagnostics.pending = &PlatformSitePendingContext{
					Kind:      "sub2api_temp_token",
					TempToken: tempToken,
				}
			}
			return nil, &platformSiteBusinessError{
				diagnostics: diagnostics,
				code:        errorCode,
				reason:      reason,
				category:    category,
			}
		}
	}
	return payload, nil
}

func unmarshalPlatformSiteJSON(data []byte) (any, error) {
	var payload any
	if err := common.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func platformSiteErrorMetadata(payload any) (code, reason, category string) {
	record, ok := payload.(map[string]any)
	if !ok {
		return "", "", ""
	}
	code = sanitizePlatformSiteDiagnosticToken(firstString(
		record,
		"code",
		"error_code",
		"errorCode",
		"status",
	))
	reason = sanitizePlatformSiteDiagnosticToken(firstString(
		record,
		"reason",
		"error_reason",
		"errorReason",
	))
	message := firstString(record, "message", "error")
	category = classifyPlatformSiteErrorCategory(code, reason, message)
	return code, reason, category
}

func platformSiteVerificationCodeMetadata(payload any) bool {
	record, ok := payload.(map[string]any)
	if !ok {
		return false
	}
	combined := strings.ToLower(strings.Join([]string{
		firstString(record, "code", "error_code", "errorCode", "status"),
		firstString(record, "reason", "error_reason", "errorReason"),
		firstString(record, "message", "error"),
	}, " "))
	return strings.Contains(combined, "otp") ||
		strings.Contains(combined, "2fa") ||
		strings.Contains(combined, "two_factor") ||
		strings.Contains(combined, "verification_code") ||
		strings.Contains(combined, "invalid_code") ||
		strings.Contains(combined, "code_invalid") ||
		strings.Contains(combined, "invalid code") ||
		strings.Contains(combined, "invalid verification") ||
		strings.Contains(combined, "verification code")
}

func classifyPlatformSiteErrorCategory(code, reason, message string) string {
	combined := strings.ToLower(strings.Join([]string{code, reason, message}, " "))
	switch {
	case strings.Contains(combined, "turnstile"),
		strings.Contains(combined, "captcha"),
		strings.Contains(combined, "challenge"),
		strings.Contains(combined, "verification_required"),
		strings.Contains(combined, "requires_verification"),
		strings.Contains(combined, "verify_required"):
		return platformSiteErrorCategoryInteractive
	case code == "401",
		code == "403",
		strings.Contains(combined, "invalid_credentials"),
		strings.Contains(combined, "invalid_password"),
		strings.Contains(combined, "authentication_failed"),
		strings.Contains(combined, "unauthorized"),
		strings.Contains(combined, "login_failed"):
		return platformSiteErrorCategoryAuthentication
	case code == "404",
		code == "405",
		strings.Contains(combined, "route_not_found"),
		strings.Contains(combined, "endpoint_not_found"),
		strings.Contains(combined, "method_not_allowed"):
		return platformSiteErrorCategoryRouteMissing
	default:
		return ""
	}
}

func sanitizePlatformSiteDiagnosticToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 96 {
		return ""
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '_' || char == '-' || char == '.' {
			continue
		}
		return ""
	}
	return value
}

func unwrapPlatformData(payload any) any {
	object, ok := payload.(map[string]any)
	if !ok {
		return payload
	}
	if data, exists := object["data"]; exists {
		return data
	}
	return payload
}

func recordsFromPayload(payload any) []map[string]any {
	payload = unwrapPlatformData(payload)
	switch value := payload.(type) {
	case []any:
		records := make([]map[string]any, 0, len(value))
		for _, item := range value {
			if record, ok := item.(map[string]any); ok {
				records = append(records, record)
			}
		}
		return records
	case map[string]any:
		for _, key := range []string{"items", "data", "list", "records", "rows", "tokens", "keys", "accounts"} {
			if items, ok := value[key]; ok {
				return recordsFromPayload(items)
			}
		}
		return []map[string]any{value}
	default:
		return nil
	}
}

func firstString(record map[string]any, keys ...string) string {
	for _, key := range keys {
		switch value := record[key].(type) {
		case string:
			if strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		case float64:
			return strconv.FormatFloat(value, 'f', -1, 64)
		case int:
			return strconv.Itoa(value)
		case int64:
			return strconv.FormatInt(value, 10)
		}
	}
	return ""
}

func firstFloat(record map[string]any, keys ...string) float64 {
	for _, key := range keys {
		if value, ok := upstreamFloatValue(record[key]); ok {
			return value
		}
	}
	return 0
}

func upstreamFloatValue(value any) (float64, bool) {
	var parsed float64
	switch typed := value.(type) {
	case float64:
		parsed = typed
	case float32:
		parsed = float64(typed)
	case int:
		parsed = float64(typed)
	case int8:
		parsed = float64(typed)
	case int16:
		parsed = float64(typed)
	case int32:
		parsed = float64(typed)
	case int64:
		parsed = float64(typed)
	case uint:
		parsed = float64(typed)
	case uint8:
		parsed = float64(typed)
	case uint16:
		parsed = float64(typed)
	case uint32:
		parsed = float64(typed)
	case uint64:
		parsed = float64(typed)
	case json.Number:
		value, err := typed.Float64()
		if err != nil {
			return 0, false
		}
		parsed = value
	case string:
		value, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err != nil {
			return 0, false
		}
		parsed = value
	default:
		return 0, false
	}
	if math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0, false
	}
	return parsed, true
}

func firstOptionalFloat(record map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		value, exists := record[key]
		if !exists || value == nil {
			continue
		}
		if parsed, ok := upstreamFloatValue(value); ok {
			return parsed, true
		}
	}
	return 0, false
}

func upstreamInt64Value(value float64) int64 {
	const maxInt64 = int64(^uint64(0) >> 1)
	const minInt64 = -maxInt64 - 1
	if value >= float64(maxInt64) {
		return maxInt64
	}
	if value <= float64(minInt64) {
		return minInt64
	}
	return int64(value)
}

func firstInt64(record map[string]any, keys ...string) int64 {
	value := firstFloat(record, keys...)
	return upstreamInt64Value(value)
}

func firstOptionalInt64(record map[string]any, keys ...string) (int64, bool) {
	value, ok := firstOptionalFloat(record, keys...)
	if !ok {
		return 0, false
	}
	return upstreamInt64Value(value), true
}

func firstTime(record map[string]any, keys ...string) *time.Time {
	for _, key := range keys {
		value, exists := record[key]
		if !exists || value == nil {
			continue
		}
		switch parsed := value.(type) {
		case float64:
			if result := unixTimestamp(parsed); result != nil {
				return result
			}
		case string:
			text := strings.TrimSpace(parsed)
			if text == "" || text == "0" {
				continue
			}
			if unix, err := strconv.ParseInt(text, 10, 64); err == nil {
				if unix > 100_000_000_000 {
					unix /= 1000
				}
				if unix > 0 {
					result := time.Unix(unix, 0).UTC()
					return &result
				}
			}
			if parsedTime, err := time.Parse(time.RFC3339, text); err == nil {
				return &parsedTime
			}
		}
	}
	return nil
}

func unixTimestamp(value float64) *time.Time {
	if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		return nil
	}
	if value > 100_000_000_000 {
		value /= 1000
	}
	const maxInt64 = int64(^uint64(0) >> 1)
	if value >= float64(maxInt64) {
		return nil
	}
	result := time.Unix(int64(value), 0).UTC()
	return &result
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func syncPlatformSite(ctx context.Context, channelID int) error {
	return syncPlatformSiteWithSource(ctx, channelID, platformSiteChallengeBackground, 0)
}

func syncPlatformSiteWithSource(
	ctx context.Context,
	channelID int,
	source string,
	createdBy int,
) error {
	lock := getUpstreamSiteLock(channelID)
	lock.mu.Lock()
	defer lock.mu.Unlock()

	var account model.PlatformSiteAccount
	if err := model.DB.Where("channel_id = ?", channelID).First(&account).Error; err != nil {
		return err
	}
	if source == platformSiteChallengeBackground &&
		account.SyncStatus == model.UpstreamSiteSyncWaitingVerification {
		if challenge, found := GetPlatformSiteChallengeStatus(channelID); found {
			return &PlatformSiteWaitingVerificationError{Result: challenge}
		}
		account.LastSyncError = "上游交互验证 Challenge 已过期，请手动重新同步"
		_ = model.DB.Model(&account).Update("last_sync_error", account.LastSyncError).Error
		return &PlatformSiteWaitingVerificationError{
			Result: &PlatformSiteChallengeResult{
				Status: platformSiteChallengeStatusWaiting,
			},
		}
	}
	stages := model.NewPlatformSiteSyncStages(common.GetTimestamp())
	if err := persistPlatformSiteSyncStages(&account, &stages); err != nil {
		return err
	}
	account.SyncStatus = model.UpstreamSiteSyncRunning
	account.LastSyncError = ""
	if err := model.DB.Model(&account).Updates(map[string]any{
		"sync_status":     account.SyncStatus,
		"last_sync_error": "",
	}).Error; err != nil {
		return err
	}

	var credential model.PlatformSiteCredential
	credential, err := model.DecryptPlatformSiteCredential(account.CredentialCiphertext)
	if err != nil {
		err = wrapPlatformSiteStage("凭据解密", errors.Join(ErrPlatformSiteCredential, err))
	}
	if err == nil {
		authType := strings.ToLower(strings.TrimSpace(account.AuthType))
		if !model.IsPlatformSiteAuthType(authType) {
			authType = model.InferPlatformSiteAuthType(credential)
			if authType != "" {
				account.AuthType = authType
				if updateErr := model.DB.Model(&account).Update("auth_type", authType).Error; updateErr != nil {
					err = wrapPlatformSiteStage("认证方式修复", updateErr)
				}
			}
		}
		if err == nil && !model.IsPlatformSiteAuthType(authType) {
			err = wrapPlatformSiteStage("认证方式", errors.New("平台站点认证方式未配置"))
		}
		credential.AuthType = authType
	}
	if err == nil {
		var adapter PlatformSiteAdapter
		adapter, err = adapterForPlatform(account.Platform)
		if err != nil {
			err = wrapPlatformSiteStage("适配器选择", err)
		}
		if err == nil {
			session, authenticateErr := adapter.Authenticate(ctx, account.BaseURL, credential)
			if session != nil && session.CredentialUpdate != nil {
				if credentialErr := persistPlatformSiteCredential(&account, *session.CredentialUpdate); credentialErr != nil {
					err = wrapPlatformSiteStage("凭据更新", credentialErr)
				} else {
					session.CredentialUpdate = nil
				}
			}
			if err == nil {
				err = authenticateErr
			}
			if err != nil && session != nil && session.CredentialUpdate != nil {
				if credentialErr := persistPlatformSiteCredential(&account, *session.CredentialUpdate); credentialErr != nil {
					err = errors.Join(err, wrapPlatformSiteStage("凭据更新", credentialErr))
				}
				session.CredentialUpdate = nil
			}
			if err != nil {
				var verificationErr *PlatformSiteVerificationRequired
				if errors.As(err, &verificationErr) {
					challengeResult, challengeErr := HandlePlatformSiteVerification(
						ctx,
						&account,
						verificationErr,
						source,
						createdBy,
					)
					if challengeErr == nil {
						stages.OverallStatus = model.PlatformSiteStageWaiting
						stages.Authentication = platformSiteSyncStage(
							model.PlatformSiteStageWaiting,
							"等待管理员在真实浏览器中完成上游验证",
							verificationErr,
						)
						if stageErr := persistPlatformSiteSyncStages(&account, &stages); stageErr != nil {
							return stageErr
						}
						account.SyncStatus = model.UpstreamSiteSyncWaitingVerification
						account.LastSyncError = "等待上游交互验证"
						if updateErr := model.DB.Model(&account).Updates(map[string]any{
							"sync_status":     account.SyncStatus,
							"last_sync_error": account.LastSyncError,
						}).Error; updateErr != nil {
							return updateErr
						}
						return &PlatformSiteWaitingVerificationError{Result: challengeResult}
					}
					err = challengeErr
				}
			}
			if err == nil {
				stages.Authentication = platformSiteSyncStage(
					model.PlatformSiteStageSuccess,
					"",
					nil,
				)
				stages.CurrentUser = platformSiteSyncStage(
					model.PlatformSiteStageSuccess,
					"",
					nil,
				)
				if stageErr := persistPlatformSiteSyncStages(&account, &stages); stageErr != nil {
					err = wrapPlatformSiteStage("阶段状态", stageErr)
				}
			}
			if session != nil && session.StageProgress == nil {
				session.StageProgress = func(progress model.PlatformSiteSyncStages) error {
					if progress.Version == 0 {
						return nil
					}
					progress.Authentication = stages.Authentication
					if progress.CurrentUser.Status == model.PlatformSiteStagePending {
						progress.CurrentUser = stages.CurrentUser
					}
					stages = progress
					visibleStages := stages
					markPlatformSiteSyncStagesUsingPrevious(&visibleStages, &account)
					return persistPlatformSiteSyncStages(&account, &visibleStages)
				}
			}
			if err == nil {
				var snapshot PlatformSiteSnapshot
				hasSnapshotStages := false
				snapshot, err = adapter.FetchSnapshot(ctx, session)
				hasSnapshotStages = snapshot.SyncStages.Version > 0
				if hasSnapshotStages {
					fetchedStages := snapshot.SyncStages
					fetchedStages.Authentication = stages.Authentication
					if fetchedStages.CurrentUser.Status == model.PlatformSiteStagePending {
						fetchedStages.CurrentUser = stages.CurrentUser
					}
					stages = fetchedStages
					if stageErr := persistPlatformSiteSyncStages(&account, &stages); stageErr != nil {
						err = wrapPlatformSiteStage("阶段状态", stageErr)
					}
				}
				if err == nil {
					err = persistPlatformSiteSnapshot(ctx, &account, snapshot)
					if err != nil {
						err = wrapPlatformSiteStage("同步写库", err)
					}
				}
				if err == nil {
					if !hasSnapshotStages {
						stages.Authentication = platformSiteSyncStage(model.PlatformSiteStageSuccess, "", nil)
						stages.CurrentUser = platformSiteSyncStage(model.PlatformSiteStageSuccess, "", nil)
						stages.BalanceUsage = platformSiteSyncStage(model.PlatformSiteStageSuccess, "", nil)
						stages.GroupsRates = platformSiteSyncStage(model.PlatformSiteStageSuccess, "", nil)
						stages.KeyPagination = platformSiteSyncStage(model.PlatformSiteStageSuccess, "", nil)
						stages.KeySecrets = platformSiteSyncStage(model.PlatformSiteStageSuccess, "", nil)
						stages.KeyModels = platformSiteSyncStage(model.PlatformSiteStageSuccess, "", nil)
						if account.Platform == model.PlatformSub2API {
							stages.Sub2APIEndpoints = platformSiteSyncStage(model.PlatformSiteStageSuccess, "", nil)
						} else {
							stages.Sub2APIEndpoints = platformSiteSyncStage(model.PlatformSiteStageSkipped, "", nil)
						}
					}
					stages.OverallStatus = model.PlatformSiteStageSuccess
					markPlatformSiteSyncStagesUsingPrevious(&stages, &account)
					if stageErr := persistPlatformSiteSyncStages(&account, &stages); stageErr != nil {
						err = wrapPlatformSiteStage("阶段状态", stageErr)
					}
				}
			}
		}
	}
	if err != nil {
		if waiting, ok := err.(*PlatformSiteWaitingVerificationError); ok {
			return waiting
		}
		updatePlatformSiteSyncStageFailure(&stages, err)
		markPlatformSiteSyncStagesUsingPrevious(&stages, &account)
		_ = persistPlatformSiteSyncStages(&account, &stages)
		account.SyncStatus = model.UpstreamSiteSyncFailed
		account.LastSyncError = safeUpstreamError(err)
		account.ConsecutiveFailures++
		if updateErr := model.DB.Model(&account).Updates(map[string]any{
			"sync_status":          account.SyncStatus,
			"last_sync_error":      account.LastSyncError,
			"consecutive_failures": account.ConsecutiveFailures,
		}).Error; updateErr != nil {
			return errors.Join(err, updateErr)
		}
		model.InitChannelCache()
		logger.LogWarn(ctx, fmt.Sprintf("upstream site sync failed: channel_id=%d platform=%s error=%s", channelID, account.Platform, safeUpstreamError(err)))
		return err
	}
	now := common.GetTimestamp()
	if err := model.DB.Model(&account).Updates(map[string]any{
		"sync_status":          model.UpstreamSiteSyncSuccess,
		"last_sync_at":         now,
		"last_sync_error":      "",
		"consecutive_failures": 0,
		"disabled_at":          0,
		"disabled_reason":      "",
	}).Error; err != nil {
		return err
	}
	model.InitChannelCache()
	return nil
}

func persistPlatformSiteCredential(account *model.PlatformSiteAccount, credential model.PlatformSiteCredential) error {
	if account == nil {
		return errors.New("平台站点不存在")
	}
	credential.AuthType = account.AuthType
	ciphertext, err := model.EncryptPlatformSiteCredential(credential)
	if err != nil {
		return err
	}
	return model.DB.Model(account).Updates(map[string]any{
		"credential_ciphertext":  ciphertext,
		"credential_key_version": "v1",
		"credential_fingerprint": credential.Fingerprint(),
	}).Error
}

func safeUpstreamError(err error) string {
	return SafePlatformSiteError(err)
}

func SafePlatformSiteError(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, ErrPlatformSiteCredential):
		return "平台凭据无法解密，请重新保存平台凭据"
	case errors.Is(err, ErrSub2APILoginInteractive):
		label := "上游交互验证"
		if platformSiteErrorContains(err, "turnstile") {
			label = "Turnstile 交互验证"
		}
		return "Sub2API 登录需要" + label +
			platformSiteResponseDiagnosticSuffix(err) +
			"。后台不会自动绕过验证码或安全验证，请先在上游站点完成验证，或使用浏览器采集 Access Token/Cookie"
	case errors.Is(err, ErrPlatformSiteVerification):
		return "上游平台需要管理员在真实浏览器中完成交互验证后重新提交验证码"
	case errors.Is(err, ErrSub2APILoginRequest):
		return "Sub2API 登录请求失败"
	case errors.Is(err, ErrSub2APILoginHTTPStatus):
		if statusCode, ok := platformSiteHTTPStatusCode(err); ok {
			return fmt.Sprintf(
				"Sub2API 登录 HTTP 状态失败（HTTP %d%s）",
				statusCode,
				platformSiteResponseDiagnosticSuffix(err),
			)
		}
		return "Sub2API 登录 HTTP 状态失败"
	case errors.Is(err, ErrSub2APILoginResponse):
		return "Sub2API 登录响应格式错误" +
			platformSiteResponseDiagnosticSuffix(err) +
			"。请检查管理端 URL、反向代理和登录 API 路径；如站点需要交互验证，请使用浏览器采集 Access Token 或 Cookie"
	case errors.Is(err, ErrSub2APILoginToken):
		return "Sub2API 登录未返回访问令牌"
	case errors.Is(err, ErrSub2APICurrentUser):
		return "Sub2API 当前用户接口失败"
	}
	var stageErr *platformSiteStageError
	stage := ""
	if errors.As(err, &stageErr) {
		stage = "（" + stageErr.stage + "）"
	}
	switch {
	case errors.Is(err, ErrPlatformSiteAuth):
		return "上游平台认证失败" + stage
	case errors.Is(err, ErrPlatformSiteResponse):
		return "上游平台响应无效" + stage
	case errors.Is(err, ErrUnsupportedPlatformSite):
		return "不支持的平台站点类型" + stage
	default:
		return "上游平台同步失败" + stage
	}
}

func platformSiteHTTPStatusCode(err error) (int, bool) {
	var statusErr *platformSiteHTTPStatusError
	if errors.As(err, &statusErr) && statusErr.statusCode > 0 {
		return statusErr.statusCode, true
	}
	var businessErr *platformSiteBusinessError
	if errors.As(err, &businessErr) && businessErr.diagnostics.statusCode > 0 {
		return businessErr.diagnostics.statusCode, true
	}
	return 0, false
}

func platformSiteResponseDiagnosticsOf(err error) (platformSiteResponseDiagnostics, bool) {
	if err == nil {
		return platformSiteResponseDiagnostics{}, false
	}
	var statusErr *platformSiteHTTPStatusError
	if errors.As(err, &statusErr) {
		return statusErr.diagnostics, true
	}
	var responseErr *platformSiteResponseError
	if errors.As(err, &responseErr) {
		return responseErr.diagnostics, true
	}
	var businessErr *platformSiteBusinessError
	if errors.As(err, &businessErr) {
		return businessErr.diagnostics, true
	}
	return platformSiteResponseDiagnostics{}, false
}

func platformSiteErrorContains(err error, value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return false
	}
	var statusErr *platformSiteHTTPStatusError
	if errors.As(err, &statusErr) {
		return strings.Contains(strings.ToLower(statusErr.diagnostics.errorCode), value) ||
			strings.Contains(strings.ToLower(statusErr.diagnostics.errorReason), value)
	}
	var businessErr *platformSiteBusinessError
	if errors.As(err, &businessErr) {
		return strings.Contains(strings.ToLower(businessErr.code), value) ||
			strings.Contains(strings.ToLower(businessErr.reason), value)
	}
	return false
}

func platformSiteResponseDiagnosticsFor(initialURL string, response *http.Response) platformSiteResponseDiagnostics {
	diagnostics := platformSiteResponseDiagnostics{
		initialURL: sanitizePlatformSiteURL(initialURL),
	}
	if response != nil {
		diagnostics.statusCode = response.StatusCode
		diagnostics.contentType = safePlatformSiteContentType(response.Header.Get("Content-Type"))
		diagnostics.finalURL = diagnostics.initialURL
		if response.Request != nil && response.Request.URL != nil {
			diagnostics.finalURL = sanitizePlatformSiteURL(response.Request.URL.String())
			diagnostics.redirected = response.Request.URL.String() != initialURL
		}
	}
	return diagnostics
}

func sanitizePlatformSiteURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func safePlatformSiteContentType(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	mediaType, _, err := mime.ParseMediaType(raw)
	if err != nil {
		mediaType = strings.TrimSpace(strings.SplitN(raw, ";", 2)[0])
	}
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	if mediaType == "" {
		return ""
	}
	if len(mediaType) > 128 {
		return mediaType[:128]
	}
	return mediaType
}

func platformSiteResponseType(contentType string, data []byte) string {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return "empty"
	}
	mediaType := safePlatformSiteContentType(contentType)
	if mediaType == "text/html" || mediaType == "application/xhtml+xml" ||
		strings.HasPrefix(mediaType, "text/html") {
		return "html"
	}
	sample := trimmed
	if len(sample) > 512 {
		sample = sample[:512]
	}
	lowerSample := bytes.ToLower(sample)
	for _, prefix := range [][]byte{
		[]byte("<!doctype html"),
		[]byte("<html"),
		[]byte("<head"),
		[]byte("<body"),
	} {
		if bytes.HasPrefix(lowerSample, prefix) {
			return "html"
		}
	}
	if strings.HasPrefix(mediaType, "text/plain") {
		return "plain_text"
	}
	var payload any
	if err := common.Unmarshal(trimmed, &payload); err == nil {
		return "json"
	}
	return "invalid_json"
}

func (diagnostics platformSiteResponseDiagnostics) summary() string {
	parts := make([]string, 0, 7)
	if diagnostics.responseType != "" {
		parts = append(parts, "响应类型："+diagnostics.responseType)
	}
	if diagnostics.statusCode > 0 {
		parts = append(parts, fmt.Sprintf("HTTP %d", diagnostics.statusCode))
	}
	if diagnostics.contentType != "" {
		parts = append(parts, "Content-Type："+diagnostics.contentType)
	}
	if diagnostics.finalURL != "" {
		parts = append(parts, "地址："+diagnostics.finalURL)
	}
	if diagnostics.initialURL != "" && diagnostics.finalURL != "" {
		if diagnostics.redirected {
			parts = append(parts, "发生重定向")
		} else {
			parts = append(parts, "未发生重定向")
		}
	}
	if diagnostics.errorReason != "" {
		parts = append(parts, "错误原因："+diagnostics.errorReason)
	} else if diagnostics.errorCode != "" {
		parts = append(parts, "错误码："+diagnostics.errorCode)
	}
	return strings.Join(parts, "，")
}

func platformSiteResponseDiagnosticSuffix(err error) string {
	var responseErr *platformSiteResponseError
	if errors.As(err, &responseErr) {
		if summary := responseErr.diagnostics.summary(); summary != "" {
			return "（" + summary + "）"
		}
		return ""
	}
	var statusErr *platformSiteHTTPStatusError
	if errors.As(err, &statusErr) {
		if summary := statusErr.diagnostics.summary(); summary != "" {
			return "（" + summary + "）"
		}
	}
	var businessErr *platformSiteBusinessError
	if errors.As(err, &businessErr) {
		if summary := businessErr.diagnostics.summary(); summary != "" {
			return "（" + summary + "）"
		}
	}
	return ""
}

func persistPlatformSiteSnapshot(_ context.Context, account *model.PlatformSiteAccount, snapshot PlatformSiteSnapshot) error {
	if account == nil {
		return errors.New("平台站点不存在")
	}
	ratio := account.ConversionRatio
	if math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio < 0 || ratio > model.MaxUpstreamConversionRatio {
		return errors.New("平台站点转换倍率超出允许范围")
	}
	if math.IsNaN(snapshot.Balance) || math.IsInf(snapshot.Balance, 0) ||
		snapshot.UsedQuota < 0 {
		return fmt.Errorf("%w: 站点额度数据无效", ErrPlatformSiteResponse)
	}
	for _, key := range snapshot.Keys {
		if key.UsedQuota < 0 {
			return fmt.Errorf("%w: 密钥额度数据无效", ErrPlatformSiteResponse)
		}
	}
	balanceAvailable := snapshot.BalanceSet || snapshot.Balance != 0
	usedQuota := snapshot.UsedQuota
	usedQuotaAvailable := snapshot.UsedQuotaSet || snapshot.UsedQuota != 0
	if !usedQuotaAvailable && snapshot.UsedQuota == 0 {
		hasKeyUsage := false
		for _, key := range snapshot.Keys {
			if key.UsedQuotaSet {
				hasKeyUsage = true
				break
			}
		}
		if hasKeyUsage {
			var err error
			usedQuota, err = sumUpstreamKeyUsedQuota(snapshot.Keys)
			if err != nil {
				return fmt.Errorf("%w: %v", ErrPlatformSiteResponse, err)
			}
			usedQuotaAvailable = true
		}
	}
	if usedQuotaAvailable && usedQuota < 0 {
		return fmt.Errorf("%w: 站点额度数据无效", ErrPlatformSiteResponse)
	}
	now := common.GetTimestamp()
	return model.DB.Transaction(func(tx *gorm.DB) error {
		var channel model.Channel
		if err := tx.First(&channel, "id = ?", account.ChannelID).Error; err != nil {
			return err
		}
		seen := make(map[string]struct{}, len(snapshot.Keys))
		for _, item := range snapshot.Keys {
			if item.ExternalID == "" {
				return fmt.Errorf("%w: 上游密钥数据不完整", ErrPlatformSiteResponse)
			}
			seen[item.ExternalID] = struct{}{}
			if item.SyncError != "" {
				var existing model.UpstreamKey
				findErr := tx.Where("channel_id = ? AND external_id = ?", account.ChannelID, item.ExternalID).First(&existing).Error
				if errors.Is(findErr, gorm.ErrRecordNotFound) {
					sourceRatio := item.SourceConversionRatio
					if !item.SourceConversionRatioSet {
						sourceRatio = item.ConversionRatio
					}
					if !isValidConversionRatio(sourceRatio) {
						sourceRatio = 1
					}
					effectiveRatio, ratioErr := model.CalculatePlatformKeyConversionRatio(
						ratio,
						sourceRatio,
					)
					if ratioErr != nil {
						return ratioErr
					}
					weight, weightErr := model.CalculateUpstreamKeyWeight(effectiveRatio)
					if weightErr != nil {
						return weightErr
					}
					existing = model.UpstreamKey{
						ChannelID:             account.ChannelID,
						ExternalID:            item.ExternalID,
						Name:                  item.Name,
						UsedQuota:             item.UsedQuota,
						SourceConversionRatio: &sourceRatio,
						ConversionRatio:       effectiveRatio,
						Weight:                weight,
						ModelsSynced:          false,
						Status:                model.UpstreamKeyStatusAutoDisabled,
						DisabledReason:        upstreamKeySyncErrorReason(item.SyncError),
						LastSyncAt:            now,
					}
					if item.Secret != "" {
						ciphertext, encryptErr := model.EncryptPlatformSiteCredential(
							model.PlatformSiteCredential{AccessToken: item.Secret},
						)
						if encryptErr != nil {
							return encryptErr
						}
						existing.SecretCiphertext = ciphertext
						existing.SecretFingerprint = common.GenerateHMAC(item.Secret)
					}
					if err := tx.Create(&existing).Error; err != nil {
						return err
					}
					if err := model.EnsureRoutingKeyForUpstreamKey(tx, &existing); err != nil {
						return err
					}
					continue
				}
				if findErr != nil {
					return findErr
				}
				updates := map[string]any{
					"last_sync_at":  now,
					"missing_since": 0,
					"name":          item.Name,
				}
				if item.UsedQuotaSet {
					updates["used_quota"] = item.UsedQuota
				}
				if item.Secret != "" {
					ciphertext, encryptErr := model.EncryptPlatformSiteCredential(
						model.PlatformSiteCredential{AccessToken: item.Secret},
					)
					if encryptErr != nil {
						return encryptErr
					}
					updates["secret_ciphertext"] = ciphertext
					updates["secret_fingerprint"] = common.GenerateHMAC(item.Secret)
				}
				if existing.Status != model.UpstreamKeyStatusManualDisabled {
					updates["status"] = model.UpstreamKeyStatusAutoDisabled
					updates["disabled_reason"] = upstreamKeySyncErrorReason(item.SyncError)
				}
				if err := tx.Model(&existing).Updates(updates).Error; err != nil {
					return err
				}
				if err := model.EnsureRoutingKeyForUpstreamKey(tx, &existing); err != nil {
					return err
				}
				continue
			}
			if item.Secret == "" {
				return fmt.Errorf("%w: 上游密钥数据不完整", ErrPlatformSiteResponse)
			}
			if item.UsedQuota < 0 {
				return fmt.Errorf("%w: 上游密钥额度数据无效", ErrPlatformSiteResponse)
			}
			models := uniqueStrings(item.Models)
			modelsSynced := item.ModelsSynced && len(models) > 0
			var existing model.UpstreamKey
			findErr := tx.Where("channel_id = ? AND external_id = ?", account.ChannelID, item.ExternalID).First(&existing).Error
			sourceRatio := item.SourceConversionRatio
			sourceRatioSet := item.SourceConversionRatioSet
			if !sourceRatioSet {
				sourceRatio = item.ConversionRatio
				sourceRatioSet = item.ConversionRatioSet || item.ConversionRatio > 0
			}
			if !sourceRatioSet {
				if findErr == nil {
					sourceRatio = existing.EffectiveSourceConversionRatio()
				} else {
					sourceRatio = 1
				}
			}
			effectiveRatio, ratioErr := model.CalculatePlatformKeyConversionRatio(ratio, sourceRatio)
			if ratioErr != nil {
				return ratioErr
			}
			weight, err := model.CalculateUpstreamKeyWeight(effectiveRatio)
			if err != nil {
				return err
			}
			ciphertext, err := model.EncryptPlatformSiteCredential(model.PlatformSiteCredential{AccessToken: item.Secret})
			if err != nil {
				return err
			}
			if errors.Is(findErr, gorm.ErrRecordNotFound) {
				existing = model.UpstreamKey{
					ChannelID:   account.ChannelID,
					ExternalID:  item.ExternalID,
					KeyPriority: 0,
					Status:      model.UpstreamKeyStatusEnabled,
				}
			} else if findErr != nil {
				return findErr
			}
			previousModels := existing.Models
			existing.Name = item.Name
			existing.SecretCiphertext = ciphertext
			existing.SecretFingerprint = common.GenerateHMAC(item.Secret)
			if !modelsSynced && previousModels != "" {
				existing.Models = previousModels
			} else {
				existing.Models = strings.Join(models, ",")
			}
			existing.ModelsSynced = modelsSynced
			sourceRatioValue := sourceRatio
			existing.SourceConversionRatio = &sourceRatioValue
			if existing.ConversionRatioOverride != nil {
				existing.ConversionRatio = *existing.ConversionRatioOverride
				overrideWeight, overrideErr := model.CalculateUpstreamKeyWeight(existing.ConversionRatio)
				if overrideErr != nil {
					return overrideErr
				}
				existing.Weight = overrideWeight
			} else {
				existing.ConversionRatio = effectiveRatio
				existing.Weight = weight
			}
			if existing.ConversionRatio == 0 {
				existing.WeightOverride = nil
			}
			existing.UsedQuota = item.UsedQuota
			existing.RemainQuota = item.RemainQuota
			existing.ExpiresAt = item.ExpiresAt
			if existing.Status != model.UpstreamKeyStatusManualDisabled {
				switch {
				case item.Disabled:
					existing.Status = model.UpstreamKeyStatusAutoDisabled
					existing.DisabledReason = "上游平台已禁用"
				case !modelsSynced:
					existing.Status = model.UpstreamKeyStatusAutoDisabled
					existing.DisabledReason = upstreamKeySyncErrorModelsUnavailable
				case item.ExpiresAt != nil && !item.ExpiresAt.After(time.Unix(now, 0)):
					existing.Status = model.UpstreamKeyStatusAutoDisabled
					existing.DisabledReason = "密钥已过期"
				case item.RemainQuota != nil && *item.RemainQuota <= 0:
					existing.Status = model.UpstreamKeyStatusAutoDisabled
					existing.DisabledReason = "密钥剩余额度不足"
				default:
					existing.Status = model.UpstreamKeyStatusEnabled
					existing.DisabledReason = ""
				}
			}
			existing.LastSyncAt = now
			existing.MissingSince = 0
			if err := tx.Save(&existing).Error; err != nil {
				return err
			}
			if err := model.EnsureRoutingKeyForUpstreamKey(tx, &existing); err != nil {
				return err
			}
			if modelsSynced {
				if err := tx.Where("upstream_key_id = ?", existing.ID).Delete(&model.UpstreamKeyAbility{}).Error; err != nil {
					return err
				}
				for _, modelName := range models {
					if err := tx.Create(&model.UpstreamKeyAbility{
						UpstreamKeyID: existing.ID,
						Group:         "",
						Model:         modelName,
						Enabled:       true,
					}).Error; err != nil {
						return err
					}
				}
			}
		}
		var existingKeys []model.UpstreamKey
		if err := tx.Where("channel_id = ?", account.ChannelID).Find(&existingKeys).Error; err != nil {
			return err
		}
		for _, key := range existingKeys {
			if _, exists := seen[key.ExternalID]; !exists {
				if err := tx.Model(&key).Updates(map[string]any{
					"status":          model.UpstreamKeyStatusMissing,
					"disabled_reason": "同步结果中未返回",
					"missing_since":   now,
				}).Error; err != nil {
					return err
				}
			}
		}
		if err := model.RebuildPlatformSiteChannelModels(tx, account.ChannelID); err != nil {
			return err
		}
		channelUpdates := make(map[string]any)
		if balanceAvailable {
			channel.Balance = snapshot.Balance
			channelUpdates["balance"] = snapshot.Balance
		}
		if usedQuotaAvailable {
			channel.UsedQuota = usedQuota
			channelUpdates["used_quota"] = usedQuota
		}
		if len(channelUpdates) > 0 {
			channelUpdates["balance_updated_time"] = now
		}
		if len(channelUpdates) > 0 {
			if err := tx.Model(&channel).Select("balance", "used_quota", "balance_updated_time").Updates(channelUpdates).Error; err != nil {
				return err
			}
		}
		accountUpdates := make(map[string]any)
		if balanceAvailable {
			accountUpdates["balance"] = snapshot.Balance
		}
		if usedQuotaAvailable {
			accountUpdates["used_quota"] = usedQuota
		}
		if len(accountUpdates) == 0 {
			accountUpdates = make(map[string]any)
		}
		managementBaseURL := strings.TrimRight(strings.TrimSpace(snapshot.ManagementBaseURL), "/")
		if managementBaseURL != "" &&
			(validateDiscoveredPlatformSiteURL(account.BaseURL, managementBaseURL) != nil ||
				!relatedPlatformSiteBaseURL(account.BaseURL, managementBaseURL)) {
			managementBaseURL = ""
		}
		if managementBaseURL != "" && account.Platform == model.PlatformSub2API {
			accountUpdates["base_url"] = managementBaseURL
		}
		relayBaseURL := strings.TrimRight(strings.TrimSpace(snapshot.RelayBaseURL), "/")
		if relayBaseURL != "" &&
			(validateDiscoveredPlatformSiteURL(account.BaseURL, relayBaseURL) != nil ||
				!relatedPlatformSiteBaseURL(account.BaseURL, relayBaseURL)) {
			relayBaseURL = ""
		}
		if relayBaseURL == "" && account.Platform == model.PlatformSub2API {
			relayBaseURL = strings.TrimRight(strings.TrimSpace(account.RelayBaseURL), "/")
		}
		if relayBaseURL != "" && account.Platform == model.PlatformSub2API {
			accountUpdates["relay_base_url"] = relayBaseURL
			channelBaseURL := model.NormalizeSub2APIRelayBaseURL(relayBaseURL)
			if channelBaseURL == "" {
				channelBaseURL = relayBaseURL
			}
			if err := tx.Model(&channel).Update("base_url", channelBaseURL).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(account).Updates(accountUpdates).Error; err != nil {
			return err
		}
		if err := tx.Where("channel_id = ?", channel.Id).Delete(&model.Ability{}).Error; err != nil {
			return err
		}
		return nil
	})
}

func sumUpstreamKeyUsedQuota(keys []UpstreamKeySnapshot) (int64, error) {
	const maxInt64 = int64(^uint64(0) >> 1)
	total := int64(0)
	for _, key := range keys {
		if !key.UsedQuotaSet {
			continue
		}
		if total > maxInt64-key.UsedQuota {
			return 0, errors.New("密钥已用额度汇总超出范围")
		}
		total += key.UsedQuota
	}
	return total, nil
}

func upstreamKeySyncErrorReason(reason string) string {
	switch reason {
	case upstreamKeySyncErrorSecretUnavailable:
		return upstreamKeySyncErrorSecretUnavailable
	case upstreamKeySyncErrorModelsUnavailable:
		return upstreamKeySyncErrorModelsUnavailable
	case upstreamKeySyncErrorInvalidData:
		return upstreamKeySyncErrorInvalidData
	default:
		return "上游密钥同步失败"
	}
}

// SyncUpstreamSite performs one read-only synchronization for a platform site.
func SyncUpstreamSite(ctx context.Context, channelID int) error {
	return syncPlatformSite(ctx, channelID)
}

// SyncUpstreamSiteManual 执行绑定当前管理员的手动平台站点同步。
func SyncUpstreamSiteManual(ctx context.Context, channelID int, userID int) error {
	return syncPlatformSiteWithSource(ctx, channelID, platformSiteChallengeManual, userID)
}

// SyncAllUpstreamSites synchronizes every configured platform site. A single
// site failure is returned in the summary but does not stop other sites.
func SyncAllUpstreamSites(ctx context.Context, progress func(processed, total int)) (int, int, error) {
	return SyncUpstreamSites(ctx, 0, progress)
}

// SyncUpstreamSites synchronizes either one site or all configured sites when
// channelID is zero.
func SyncUpstreamSites(ctx context.Context, channelID int, progress func(processed, total int)) (int, int, error) {
	var accounts []model.PlatformSiteAccount
	query := model.DB
	if channelID > 0 {
		query = query.Where("channel_id = ?", channelID)
	}
	if err := query.Find(&accounts).Error; err != nil {
		return 0, 0, err
	}
	successCount := 0
	failureCount := 0
	var firstErr error
	for index, account := range accounts {
		if err := SyncUpstreamSite(ctx, account.ChannelID); err != nil {
			failureCount++
			if firstErr == nil {
				firstErr = err
			}
		} else {
			successCount++
		}
		if progress != nil {
			progress(index+1, len(accounts))
		}
		if ctx.Err() != nil {
			return successCount, failureCount, ctx.Err()
		}
	}
	return successCount, failureCount, firstErr
}
