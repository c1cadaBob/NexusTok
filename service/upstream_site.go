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
	ErrUnsupportedPlatformSite = errors.New("unsupported upstream platform site")
	ErrPlatformSiteAuth        = errors.New("platform site authentication failed")
	ErrPlatformSiteHTTPStatus  = errors.New("platform site http status failed")
	ErrPlatformSiteResponse    = errors.New("platform site returned an invalid response")
	ErrPlatformSiteCredential  = errors.New("platform site credential unavailable")
	ErrPlatformSiteIdentity    = errors.New("platform site identity mismatch")
	ErrPlatformSiteAuthBundle  = errors.New("platform site auth bundle invalid")
	ErrPlatformSiteSecurity    = errors.New("platform site security verification required")
	ErrPlatformSiteTransport   = errors.New("platform site transport failed")
	ErrPlatformSiteCredentials = errors.New("platform site credentials invalid")
	ErrSub2APILoginRequest     = errors.New("sub2api login request failed")
	ErrSub2APILoginHTTPStatus  = errors.New("sub2api login http status failed")
	ErrSub2APILoginResponse    = errors.New("sub2api login response format failed")
	ErrSub2APILoginToken       = errors.New("sub2api login token missing")
	ErrSub2APILoginInteractive = errors.New("sub2api login requires interactive verification")
	ErrSub2APILoginEmail       = errors.New("sub2api login requires a valid email")
	ErrSub2APICurrentUser      = errors.New("sub2api current user request failed")
)

const (
	upstreamKeySyncErrorSecretUnavailable   = "credential_unavailable"
	upstreamKeySyncErrorModelsUnavailable   = "models_unavailable"
	upstreamKeySyncErrorInvalidData         = "invalid_data"
	platformSiteErrorCategoryAuthentication = "authentication"
	platformSiteErrorCategoryInteractive    = "interactive_verification"
	platformSiteErrorCategoryRouteMissing   = "route_missing"
	platformSiteErrorCategoryWAF            = "waf_blocked"
)

type PlatformSiteSession struct {
	BaseURL           string
	ModelBaseURL      string
	ManagementBaseURL string
	Client            *http.Client
	Headers           http.Header
	CredentialUpdate  *model.PlatformSiteCredential
}

type PlatformSiteAdapter interface {
	Platform() string
	Authenticate(context.Context, string, model.PlatformSiteCredential) (*PlatformSiteSession, error)
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
	UsedQuota         int64
	UsedQuotaSet      bool
	Models            []string
	Keys              []UpstreamKeySnapshot
	KeysComplete      bool
	AuthStatus        string
	AuthStatusReason  string
	ManagementBaseURL string
	RelayBaseURL      string
	Identity          *PlatformSiteIdentitySnapshot
	Groups            []PlatformSiteGroupSnapshot
	GroupsLoaded      bool
	Endpoint          *PlatformSiteEndpointSnapshot
	ResourceSyncs     []PlatformSiteResourceSyncSnapshot
}

type platformSiteResponseDiagnostics struct {
	statusCode    int
	initialURL    string
	finalURL      string
	contentType   string
	redirected    bool
	responseType  string
	errorCode     string
	errorReason   string
	errorCategory string
}

type PlatformSiteIdentitySnapshot struct {
	PlatformUserID    string
	Username          string
	Email             string
	DisplayName       string
	Role              string
	CurrentGroup      string
	Status            string
	QuotaUnit         string
	UpstreamUpdatedAt int64
	SourceEndpoint    string
}

type PlatformSiteGroupSnapshot struct {
	ExternalID        string
	Name              string
	Ratio             float64
	Available         bool
	Usable            bool
	SourceEndpoint    string
	UpstreamUpdatedAt int64
}

type PlatformSiteEndpointCapabilitySnapshot struct {
	Protocol   string
	HTTPMethod string
	Path       string
	Supported  bool
	SourceData string
}

type PlatformSiteEndpointSnapshot struct {
	ManagementURL   string
	RelayURL        string
	ModelsURL       string
	PricingURL      string
	UsageURL        string
	TokenURL        string
	AdminURL        string
	OpenAIURL       string
	ClaudeURL       string
	GeminiURL       string
	ResponsesURL    string
	Source          string
	DiscoveryMethod string
	Enabled         bool
	Capabilities    []PlatformSiteEndpointCapabilitySnapshot
}

type PlatformSiteResourceSyncSnapshot struct {
	ResourceType                 string
	Status                       string
	SourceEndpoint               string
	RecordCount                  int
	FailureReason                string
	Partial                      bool
	RequiresSecurityVerification bool
}

type platformSiteHTTPStatusError struct {
	statusCode  int
	diagnostics platformSiteResponseDiagnostics
}

func (err *platformSiteHTTPStatusError) Error() string {
	return fmt.Sprintf("%s: HTTP %d", ErrPlatformSiteHTTPStatus, err.statusCode)
}

func (err *platformSiteHTTPStatusError) Unwrap() error {
	errs := []error{ErrPlatformSiteHTTPStatus}
	switch err.diagnostics.errorCategory {
	case platformSiteErrorCategoryAuthentication:
		errs = append(errs, ErrPlatformSiteCredentials)
	case platformSiteErrorCategoryInteractive, platformSiteErrorCategoryWAF:
		errs = append(errs, ErrPlatformSiteSecurity)
	}
	return errors.Join(errs...)
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
	switch err.category {
	case platformSiteErrorCategoryInteractive, platformSiteErrorCategoryWAF:
		return errors.Join(ErrPlatformSiteAuth, ErrPlatformSiteSecurity)
	case platformSiteErrorCategoryAuthentication:
		return errors.Join(ErrPlatformSiteAuth, ErrPlatformSiteCredentials)
	default:
		return ErrPlatformSiteAuth
	}
}

type platformSiteSecurityError struct {
	code        string
	statusCode  int
	diagnostics platformSiteResponseDiagnostics
}

func (err *platformSiteSecurityError) Error() string {
	if err == nil {
		return ""
	}
	return ErrPlatformSiteSecurity.Error()
}

func (err *platformSiteSecurityError) Unwrap() error {
	return ErrPlatformSiteSecurity
}

func platformSiteSecurityErrorCode(err error) string {
	var securityErr *platformSiteSecurityError
	if !errors.As(err, &securityErr) {
		return ""
	}
	return securityErr.code
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
		return nil, errors.Join(ErrPlatformSiteTransport, err)
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
		var payload any
		if parsed, unmarshalErr := unmarshalPlatformSiteJSON(data); unmarshalErr == nil {
			payload = parsed
			diagnostics.errorCode, diagnostics.errorReason, diagnostics.errorCategory =
				platformSiteErrorMetadata(payload)
		}
		if code := platformSiteSecurityCode(payload, string(data)); code != "" {
			diagnostics.errorCode = sanitizePlatformSiteDiagnosticToken(code)
			diagnostics.errorCategory = platformSiteErrorCategoryInteractive
			return nil, &platformSiteSecurityError{
				code:        code,
				statusCode:  response.StatusCode,
				diagnostics: diagnostics,
			}
		}
		if diagnostics.errorCategory == "" {
			diagnostics.errorCategory = classifyPlatformSiteResponseCategory(
				response.StatusCode,
				diagnostics.responseType,
				data,
			)
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
	if code := platformSiteSecurityCode(payload, string(data)); code != "" {
		diagnostics.errorCode = sanitizePlatformSiteDiagnosticToken(code)
		diagnostics.errorCategory = platformSiteErrorCategoryInteractive
		return nil, &platformSiteSecurityError{
			code:        code,
			statusCode:  response.StatusCode,
			diagnostics: diagnostics,
		}
	}
	if object, ok := payload.(map[string]any); ok {
		if success, exists := object["success"].(bool); exists && !success {
			code, reason, category := platformSiteErrorMetadata(object)
			diagnostics.errorCode = code
			diagnostics.errorReason = reason
			diagnostics.errorCategory = category
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

func platformSiteSecurityCode(payload any, raw string) string {
	const requiredCode = "STEP_UP_ADMIN_API_KEY_FORBIDDEN"
	if strings.Contains(raw, requiredCode) {
		return requiredCode
	}
	var findCode func(any) string
	findCode = func(value any) string {
		switch typed := value.(type) {
		case map[string]any:
			for _, key := range []string{"code", "error_code", "errorCode", "reason"} {
				if text, ok := typed[key].(string); ok && strings.EqualFold(strings.TrimSpace(text), requiredCode) {
					return requiredCode
				}
			}
			for _, nested := range typed {
				if code := findCode(nested); code != "" {
					return code
				}
			}
		case []any:
			for _, nested := range typed {
				if code := findCode(nested); code != "" {
					return code
				}
			}
		}
		return ""
	}
	return findCode(payload)
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
	if code == "" {
		if numericCode, ok := firstOptionalFloat(record, "code", "status"); ok {
			code = sanitizePlatformSiteDiagnosticToken(
				strconv.FormatFloat(numericCode, 'f', -1, 64),
			)
		}
	}
	message := firstString(record, "message", "error", "detail")
	category = classifyPlatformSiteErrorCategory(code, reason, message)
	return code, reason, category
}

func classifyPlatformSiteErrorCategory(code, reason, message string) string {
	combined := strings.ToLower(strings.Join([]string{code, reason, message}, " "))
	switch {
	case strings.Contains(combined, "turnstile"),
		strings.Contains(combined, "captcha"),
		strings.Contains(combined, "challenge"),
		strings.Contains(combined, "verification_required"),
		strings.Contains(combined, "requires_verification"),
		strings.Contains(combined, "verify_required"),
		strings.Contains(combined, "browser_verification"):
		return platformSiteErrorCategoryInteractive
	case strings.Contains(combined, "cloudflare"),
		strings.Contains(combined, "cf-ray"),
		strings.Contains(combined, "waf"),
		strings.Contains(combined, "bot_detection"),
		strings.Contains(combined, "access_denied"):
		return platformSiteErrorCategoryWAF
	case code == "401",
		code == "403",
		strings.Contains(combined, "invalid_credentials"),
		strings.Contains(combined, "invalid_password"),
		strings.Contains(combined, "invalid username"),
		strings.Contains(combined, "password error"),
		strings.Contains(combined, "username_or_password"),
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

func classifyPlatformSiteResponseCategory(statusCode int, responseType string, data []byte) string {
	sample := bytes.TrimSpace(data)
	if len(sample) > 4096 {
		sample = sample[:4096]
	}
	combined := strings.ToLower(string(sample))
	switch {
	case strings.Contains(combined, "turnstile"),
		strings.Contains(combined, "captcha"),
		strings.Contains(combined, "challenge"),
		strings.Contains(combined, "verify you are human"),
		strings.Contains(combined, "browser verification"):
		return platformSiteErrorCategoryInteractive
	case strings.Contains(combined, "cloudflare"),
		strings.Contains(combined, "cf-ray"),
		strings.Contains(combined, "access denied"),
		strings.Contains(combined, "waf"):
		return platformSiteErrorCategoryWAF
	case statusCode == http.StatusUnauthorized,
		statusCode == http.StatusForbidden:
		return platformSiteErrorCategoryAuthentication
	case statusCode == http.StatusNotFound || statusCode == http.StatusMethodNotAllowed:
		return platformSiteErrorCategoryRouteMissing
	case responseType == "html" && statusCode == http.StatusForbidden:
		return platformSiteErrorCategoryWAF
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

func platformSiteErrorCategoryOf(err error) string {
	if err == nil {
		return ""
	}
	var statusErr *platformSiteHTTPStatusError
	if errors.As(err, &statusErr) {
		return statusErr.diagnostics.errorCategory
	}
	var businessErr *platformSiteBusinessError
	if errors.As(err, &businessErr) {
		return businessErr.category
	}
	var securityErr *platformSiteSecurityError
	if errors.As(err, &securityErr) {
		return securityErr.diagnostics.errorCategory
	}
	return ""
}

func platformSiteRouteMissing(err error) bool {
	return platformSiteErrorCategoryOf(err) == platformSiteErrorCategoryRouteMissing
}

func platformSiteInteractiveVerificationRequired(err error) bool {
	return errors.Is(err, ErrPlatformSiteSecurity) ||
		errors.Is(err, ErrSub2APILoginInteractive) ||
		platformSiteErrorCategoryOf(err) == platformSiteErrorCategoryInteractive ||
		platformSiteErrorCategoryOf(err) == platformSiteErrorCategoryWAF
}

func platformSiteResponseDiagnosticsFor(
	initialURL string,
	response *http.Response,
) platformSiteResponseDiagnostics {
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
	if mediaType == "text/html" || mediaType == "application/xhtml+xml" {
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
	var securityErr *platformSiteSecurityError
	if errors.As(err, &securityErr) {
		if summary := securityErr.diagnostics.summary(); summary != "" {
			return "（" + summary + "）"
		}
	}
	return ""
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
	lock := getUpstreamSiteLock(channelID)
	lock.mu.Lock()
	defer lock.mu.Unlock()

	var account model.PlatformSiteAccount
	if err := model.DB.Where("channel_id = ?", channelID).First(&account).Error; err != nil {
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
			err = authenticateErr
			if err == nil {
				var snapshot PlatformSiteSnapshot
				snapshot, err = adapter.FetchSnapshot(ctx, session)
				if err == nil {
					err = persistPlatformSiteSnapshot(ctx, &account, snapshot)
					if err != nil {
						err = wrapPlatformSiteStage("同步写库", err)
					}
					if err == nil && session.CredentialUpdate != nil {
						err = persistPlatformSiteCredential(&account, *session.CredentialUpdate)
						if err != nil {
							err = wrapPlatformSiteStage("凭据更新", err)
						}
					}
				}
			} else if session != nil && session.CredentialUpdate != nil {
				if updateErr := persistPlatformSiteCredential(&account, *session.CredentialUpdate); updateErr != nil {
					err = errors.Join(err, wrapPlatformSiteStage("凭据状态更新", updateErr))
				}
			}
		}
	}
	if err != nil {
		account.SyncStatus = model.UpstreamSiteSyncFailed
		account.LastSyncError = safeUpstreamError(err)
		failureUpdates := map[string]any{
			"sync_status":          account.SyncStatus,
			"last_sync_error":      account.LastSyncError,
			"consecutive_failures": account.ConsecutiveFailures + 1,
			"auth_status_reason":   account.LastSyncError,
		}
		if errors.Is(err, ErrPlatformSiteSecurity) ||
			errors.Is(err, ErrSub2APILoginInteractive) ||
			platformSiteErrorCategoryOf(err) == platformSiteErrorCategoryInteractive ||
			platformSiteErrorCategoryOf(err) == platformSiteErrorCategoryWAF {
			account.AuthStatus = model.PlatformSiteAuthStatusSecureVerificationRequired
			failureUpdates["auth_status"] = account.AuthStatus
		} else if errors.Is(err, ErrPlatformSiteCredentials) ||
			errors.Is(err, ErrSub2APILoginEmail) {
			account.AuthStatus = model.PlatformSiteAuthStatusCredentialsInvalid
			failureUpdates["auth_status"] = account.AuthStatus
		}
		account.ConsecutiveFailures++
		if updateErr := model.DB.Model(&account).Updates(failureUpdates).Error; updateErr != nil {
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
		return "Sub2API 登录需要交互验证" +
			platformSiteResponseDiagnosticSuffix(err) +
			"，请先在上游站点完成验证，或使用浏览器采集登录态"
	case errors.Is(err, ErrPlatformSiteSecurity):
		return "上游平台要求完成安全验证" + platformSiteResponseDiagnosticSuffix(err)
	case errors.Is(err, ErrPlatformSiteTransport):
		return "上游平台网络连接失败，已保留最近成功快照"
	case errors.Is(err, ErrSub2APILoginEmail):
		return "Sub2API 登录账号必须是合法邮箱，请使用上游账号邮箱或浏览器采集登录态"
	case errors.Is(err, ErrPlatformSiteCredentials):
		return "上游平台账号或密码错误"
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
			"，请检查管理端 URL、反向代理和登录 API 路径"
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
	usedQuota := snapshot.UsedQuota
	if !snapshot.UsedQuotaSet && snapshot.UsedQuota == 0 {
		var err error
		usedQuota, err = sumUpstreamKeyUsedQuota(snapshot.Keys)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrPlatformSiteResponse, err)
		}
	}
	if usedQuota < 0 {
		return fmt.Errorf("%w: 站点额度数据无效", ErrPlatformSiteResponse)
	}
	now := common.GetTimestamp()
	return model.DB.Transaction(func(tx *gorm.DB) error {
		var channel model.Channel
		if err := tx.First(&channel, "id = ?", account.ChannelID).Error; err != nil {
			return err
		}
		seen := make(map[string]struct{}, len(snapshot.Keys))
		keysPartial := false
		for _, item := range snapshot.Keys {
			if item.ExternalID == "" {
				return fmt.Errorf("%w: 上游密钥数据不完整", ErrPlatformSiteResponse)
			}
			// 密钥详情或模型能力读取失败时，当前轮次没有足够数据覆盖
			// 既有路由快照。保留旧记录和能力，等待下一次完整同步。
			if item.SyncError != "" {
				keysPartial = true
				continue
			}
			seen[item.ExternalID] = struct{}{}
			if item.Secret == "" {
				return fmt.Errorf("%w: 上游密钥数据不完整", ErrPlatformSiteResponse)
			}
			if item.UsedQuota < 0 {
				return fmt.Errorf("%w: 上游密钥额度数据无效", ErrPlatformSiteResponse)
			}
			models := uniqueStrings(item.Models)
			modelsSynced := item.ModelsSynced && len(models) > 0
			sourceRatio := item.SourceConversionRatio
			sourceRatioSet := item.SourceConversionRatioSet
			if !sourceRatioSet {
				sourceRatio = item.ConversionRatio
				sourceRatioSet = item.ConversionRatioSet || item.ConversionRatio > 0
			}
			if !sourceRatioSet {
				sourceRatio = 1
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
			var existing model.UpstreamKey
			findErr := tx.Where("channel_id = ? AND external_id = ?", account.ChannelID, item.ExternalID).First(&existing).Error
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
			} else if err := tx.Where("upstream_key_id = ?", existing.ID).Delete(&model.UpstreamKeyAbility{}).Error; err != nil {
				return err
			}
		}
		if snapshot.KeysComplete {
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
		}
		if keysPartial || !snapshot.KeysComplete {
			hasKeysResourceSync := false
			for index := range snapshot.ResourceSyncs {
				if snapshot.ResourceSyncs[index].ResourceType != model.PlatformSiteResourceKeys {
					continue
				}
				hasKeysResourceSync = true
				if snapshot.ResourceSyncs[index].Status == model.PlatformSiteResourceStatusSuccess {
					snapshot.ResourceSyncs[index].Status = model.PlatformSiteResourceStatusPartial
					snapshot.ResourceSyncs[index].FailureReason = "部分密钥详情或模型能力读取失败，已保留最近成功快照"
					snapshot.ResourceSyncs[index].Partial = true
				}
			}
			if !hasKeysResourceSync {
				snapshot.ResourceSyncs = append(snapshot.ResourceSyncs, PlatformSiteResourceSyncSnapshot{
					ResourceType:   model.PlatformSiteResourceKeys,
					Status:         model.PlatformSiteResourceStatusPartial,
					SourceEndpoint: "/api/token/,/api/token/batch/keys",
					RecordCount:    len(snapshot.Keys),
					FailureReason:  "部分密钥详情或模型能力读取失败，已保留最近成功快照",
					Partial:        true,
				})
			}
		}
		channel.Balance = snapshot.Balance
		channel.UsedQuota = usedQuota
		if err := model.RebuildPlatformSiteChannelModels(tx, account.ChannelID); err != nil {
			return err
		}
		if err := tx.Model(&channel).Select("balance", "used_quota", "balance_updated_time").Updates(map[string]any{
			"balance":              snapshot.Balance,
			"used_quota":           usedQuota,
			"balance_updated_time": now,
		}).Error; err != nil {
			return err
		}
		accountUpdates := map[string]any{
			"balance":    snapshot.Balance,
			"used_quota": usedQuota,
		}
		if snapshot.AuthStatus != "" {
			accountUpdates["auth_status"] = snapshot.AuthStatus
			accountUpdates["auth_status_reason"] = snapshot.AuthStatusReason
		} else {
			accountUpdates["auth_status"] = model.PlatformSiteAuthStatusAuthenticated
			accountUpdates["auth_status_reason"] = ""
		}
		managementBaseURL := strings.TrimRight(strings.TrimSpace(snapshot.ManagementBaseURL), "/")
		if managementBaseURL != "" && account.Platform == model.PlatformSub2API {
			accountUpdates["base_url"] = managementBaseURL
		}
		relayBaseURL := strings.TrimRight(strings.TrimSpace(snapshot.RelayBaseURL), "/")
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
		if err := persistPlatformSiteResources(tx, account, snapshot, now); err != nil {
			return err
		}
		if err := tx.Where("channel_id = ?", channel.Id).Delete(&model.Ability{}).Error; err != nil {
			return err
		}
		return nil
	})
}

func persistPlatformSiteResources(
	tx *gorm.DB,
	account *model.PlatformSiteAccount,
	snapshot PlatformSiteSnapshot,
	now int64,
) error {
	if tx == nil || account == nil {
		return errors.New("平台站点资源写入参数无效")
	}
	if !tx.Migrator().HasTable(&model.PlatformSiteIdentity{}) {
		return nil
	}
	if snapshot.Identity != nil {
		identity := *snapshot.Identity
		var existing model.PlatformSiteIdentity
		err := tx.Where("channel_id = ?", account.ChannelID).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			existing = model.PlatformSiteIdentity{ChannelID: account.ChannelID}
		} else if err != nil {
			return err
		}
		existing.PlatformUserID = identity.PlatformUserID
		existing.Username = identity.Username
		existing.Email = identity.Email
		existing.DisplayName = identity.DisplayName
		existing.Role = identity.Role
		existing.CurrentGroup = identity.CurrentGroup
		existing.Status = identity.Status
		existing.QuotaUnit = identity.QuotaUnit
		existing.Balance = snapshot.Balance
		existing.UsedQuota = snapshot.UsedQuota
		existing.CurrentValueAt = now
		existing.SnapshotValueAt = now
		existing.SourceEndpoint = identity.SourceEndpoint
		existing.UpstreamUpdatedAt = identity.UpstreamUpdatedAt
		existing.LastSyncAt = now
		if err := tx.Save(&existing).Error; err != nil {
			return err
		}
	}
	if snapshot.GroupsLoaded {
		if err := tx.Where("channel_id = ?", account.ChannelID).Delete(&model.PlatformSiteGroup{}).Error; err != nil {
			return err
		}
		for _, group := range snapshot.Groups {
			if strings.TrimSpace(group.ExternalID) == "" {
				return fmt.Errorf("%w: 平台分组缺少外部 ID", ErrPlatformSiteResponse)
			}
			if !isValidConversionRatio(group.Ratio) {
				return fmt.Errorf("%w: 平台分组倍率无效", ErrPlatformSiteResponse)
			}
			if err := tx.Create(&model.PlatformSiteGroup{
				ChannelID:         account.ChannelID,
				ExternalID:        group.ExternalID,
				Name:              group.Name,
				Ratio:             group.Ratio,
				Available:         group.Available,
				Usable:            group.Usable,
				SourceEndpoint:    group.SourceEndpoint,
				UpstreamUpdatedAt: group.UpstreamUpdatedAt,
				LastSyncAt:        now,
			}).Error; err != nil {
				return err
			}
		}
	}
	if snapshot.Endpoint != nil {
		endpoint := snapshot.Endpoint
		var existing model.PlatformSiteEndpoint
		err := tx.Where("channel_id = ?", account.ChannelID).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			existing = model.PlatformSiteEndpoint{ChannelID: account.ChannelID}
		} else if err != nil {
			return err
		}
		existing.ManagementURL = endpoint.ManagementURL
		existing.RelayURL = endpoint.RelayURL
		existing.ModelsURL = endpoint.ModelsURL
		existing.PricingURL = endpoint.PricingURL
		existing.UsageURL = endpoint.UsageURL
		existing.TokenURL = endpoint.TokenURL
		existing.AdminURL = endpoint.AdminURL
		existing.OpenAIURL = endpoint.OpenAIURL
		existing.ClaudeURL = endpoint.ClaudeURL
		existing.GeminiURL = endpoint.GeminiURL
		existing.ResponsesURL = endpoint.ResponsesURL
		existing.Source = endpoint.Source
		existing.DiscoveryMethod = endpoint.DiscoveryMethod
		existing.Enabled = endpoint.Enabled
		existing.LastConfirmedAt = now
		if err := tx.Save(&existing).Error; err != nil {
			return err
		}
		if err := tx.Where("endpoint_id = ?", existing.ID).Delete(&model.PlatformSiteEndpointCapability{}).Error; err != nil {
			return err
		}
		for _, capability := range endpoint.Capabilities {
			if err := tx.Create(&model.PlatformSiteEndpointCapability{
				EndpointID:      existing.ID,
				Protocol:        capability.Protocol,
				HTTPMethod:      capability.HTTPMethod,
				Path:            capability.Path,
				Supported:       capability.Supported,
				SourceData:      capability.SourceData,
				LastConfirmedAt: now,
			}).Error; err != nil {
				return err
			}
		}
	}
	for _, resource := range snapshot.ResourceSyncs {
		var existing model.PlatformSiteResourceSync
		err := tx.Where(
			"channel_id = ? AND resource_type = ?",
			account.ChannelID,
			resource.ResourceType,
		).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			existing = model.PlatformSiteResourceSync{
				ChannelID:    account.ChannelID,
				ResourceType: resource.ResourceType,
			}
		} else if err != nil {
			return err
		}
		existing.Status = resource.Status
		existing.AttemptedAt = now
		existing.SourceEndpoint = resource.SourceEndpoint
		existing.RecordCount = resource.RecordCount
		existing.FailureReason = resource.FailureReason
		existing.Partial = resource.Partial
		existing.RequiresSecurityVerification = resource.RequiresSecurityVerification
		existing.UsingSnapshot = resource.Status != model.PlatformSiteResourceStatusSuccess
		if resource.Status == model.PlatformSiteResourceStatusSuccess {
			existing.SucceededAt = now
			existing.UsingSnapshot = false
		}
		if err := tx.Save(&existing).Error; err != nil {
			return err
		}
	}
	return nil
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
