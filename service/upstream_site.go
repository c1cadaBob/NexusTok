package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
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
	ErrPlatformSiteResponse    = errors.New("platform site returned an invalid response")
	ErrPlatformSiteCredential  = errors.New("platform site credential unavailable")
	ErrSub2APILoginRequest     = errors.New("sub2api login request failed")
	ErrSub2APILoginHTTPStatus  = errors.New("sub2api login http status failed")
	ErrSub2APILoginResponse    = errors.New("sub2api login response format failed")
	ErrSub2APILoginToken       = errors.New("sub2api login token missing")
	ErrSub2APICurrentUser      = errors.New("sub2api current user request failed")
)

const (
	upstreamKeySyncErrorSecretUnavailable = "credential_unavailable"
	upstreamKeySyncErrorModelsUnavailable = "models_unavailable"
	upstreamKeySyncErrorInvalidData       = "invalid_data"
)

type PlatformSiteSession struct {
	BaseURL          string
	ModelBaseURL     string
	Client           *http.Client
	Headers          http.Header
	CredentialUpdate *model.PlatformSiteCredential
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
	RemainQuota              *int64
	ExpiresAt                *time.Time
	Disabled                 bool
	SyncError                string
}

type PlatformSiteSnapshot struct {
	Balance   float64
	UsedQuota int64
	Models    []string
	Keys      []UpstreamKeySnapshot
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
	parsed, _ := url.Parse(normalized)
	if parsed.Scheme != "https" && !common.GetEnvOrDefaultBool("NEXUSTOK_ALLOW_HTTP_UPSTREAM_SITES", false) {
		return errors.New("平台站点默认必须使用 HTTPS")
	}
	protection := &common.SSRFProtection{
		AllowPrivateIp:         false,
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
	client := GetSSRFProtectedHTTPClient()
	if client == nil {
		client = GetHttpClient()
	}
	if client == nil {
		client = &http.Client{}
	}
	clone := *client
	clone.Jar = jar
	clone.Timeout = upstreamSiteRequestTimeout
	clone.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("平台站点重定向次数超过限制")
		}
		if err := validatePlatformSiteURL(req.URL.String()); err != nil {
			return fmt.Errorf("平台站点重定向被拒绝: %w", err)
		}
		return nil
	}
	return &clone, nil
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
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("%w: HTTP %d", ErrPlatformSiteAuth, response.StatusCode)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return map[string]any{}, nil
	}
	var payload any
	if err := common.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("%w: JSON", ErrPlatformSiteResponse)
	}
	if object, ok := payload.(map[string]any); ok {
		if success, exists := object["success"].(bool); exists && !success {
			return nil, fmt.Errorf("%w: %s", ErrPlatformSiteAuth, firstString(object, "message", "error"))
		}
		if code := firstFloat(object, "code"); code != 0 && code != 200 {
			return nil, fmt.Errorf("%w: %s", ErrPlatformSiteAuth, firstString(object, "message", "error"))
		}
		if code := firstString(object, "code"); code != "" &&
			code != "0" && code != "200" && !strings.EqualFold(code, "success") {
			return nil, fmt.Errorf("%w: %s", ErrPlatformSiteAuth, firstString(object, "message", "error"))
		}
	}
	return payload, nil
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
		switch value := record[key].(type) {
		case float64:
			return value
		case int:
			return float64(value)
		case int64:
			return float64(value)
		case string:
			parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
			if err == nil {
				return parsed
			}
		}
	}
	return 0
}

func firstInt64(record map[string]any, keys ...string) int64 {
	value := firstFloat(record, keys...)
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
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
			}
		}
	}
	if err != nil {
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
	case errors.Is(err, ErrSub2APILoginRequest):
		return "Sub2API 登录请求失败"
	case errors.Is(err, ErrSub2APILoginHTTPStatus):
		return "Sub2API 登录 HTTP 状态失败"
	case errors.Is(err, ErrSub2APILoginResponse):
		return "Sub2API 登录响应格式错误"
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
	now := common.GetTimestamp()
	return model.DB.Transaction(func(tx *gorm.DB) error {
		var channel model.Channel
		if err := tx.First(&channel, "id = ?", account.ChannelID).Error; err != nil {
			return err
		}
		seen := make(map[string]struct{}, len(snapshot.Keys))
		allModels := make([]string, 0)
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
					continue
				}
				if findErr != nil {
					return findErr
				}
				updates := map[string]any{
					"last_sync_at":  now,
					"missing_since": 0,
					"models_synced": false,
					"name":          item.Name,
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
				if err := tx.Where("upstream_key_id = ?", existing.ID).Delete(&model.UpstreamKeyAbility{}).Error; err != nil {
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
			if modelsSynced {
				allModels = append(allModels, item.Models...)
			}
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
		allModels = uniqueStrings(allModels)
		channel.Models = strings.Join(allModels, ",")
		channel.Balance = snapshot.Balance
		channel.UsedQuota = snapshot.UsedQuota
		if err := tx.Model(&channel).Select("models", "balance", "used_quota", "balance_updated_time").Updates(map[string]any{
			"models":               channel.Models,
			"balance":              snapshot.Balance,
			"used_quota":           snapshot.UsedQuota,
			"balance_updated_time": now,
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(account).Updates(map[string]any{
			"balance":    snapshot.Balance,
			"used_quota": snapshot.UsedQuota,
		}).Error; err != nil {
			return err
		}
		if err := tx.Where("channel_id = ?", channel.Id).Delete(&model.Ability{}).Error; err != nil {
			return err
		}
		return nil
	})
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
