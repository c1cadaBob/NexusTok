package controller

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

	"github.com/c1cadaBob/NexusTok/common"
	"github.com/c1cadaBob/NexusTok/constant"
	"github.com/c1cadaBob/NexusTok/model"
	"github.com/c1cadaBob/NexusTok/service"
	"github.com/c1cadaBob/NexusTok/setting/ratio_setting"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type PlatformSiteInput struct {
	Platform        string   `json:"platform"`
	BaseURL         string   `json:"base_url"`
	RelayBaseURL    string   `json:"relay_base_url,omitempty"`
	AuthType        string   `json:"auth_type"`
	Username        string   `json:"username"`
	Password        string   `json:"password"`
	UserID          string   `json:"user_id,omitempty"`
	AccessToken     string   `json:"access_token"`
	RefreshToken    string   `json:"refresh_token,omitempty"`
	TokenExpiresAt  int64    `json:"-"`
	AdminKey        string   `json:"admin_key"`
	Cookie          string   `json:"cookie"`
	CaptureID       string   `json:"capture_id,omitempty"`
	RechargeAmount  *float64 `json:"recharge_amount"`
	CreditedAmount  *float64 `json:"credited_amount"`
	ConversionRatio *float64 `json:"conversion_ratio"`
}

type UpstreamSiteStatusResponse struct {
	ChannelID           int     `json:"channel_id"`
	Platform            string  `json:"platform"`
	BaseURL             string  `json:"base_url"`
	AuthType            string  `json:"auth_type"`
	RechargeAmount      float64 `json:"recharge_amount"`
	CreditedAmount      float64 `json:"credited_amount"`
	ConversionRatio     float64 `json:"conversion_ratio"`
	Balance             float64 `json:"balance"`
	UsedQuota           int64   `json:"used_quota"`
	BalanceUpdatedTime  int64   `json:"balance_updated_time"`
	SyncStatus          string  `json:"sync_status"`
	LastSyncAt          int64   `json:"last_sync_at"`
	LastSyncError       string  `json:"last_sync_error"`
	ConsecutiveFailures int     `json:"consecutive_failures"`
	KeyCount            int     `json:"key_count"`
	RoutableKeyCount    int     `json:"routable_key_count"`
	SnapshotUsable      bool    `json:"snapshot_usable,omitempty"`
	UsingLastSnapshot   bool    `json:"using_last_snapshot,omitempty"`
	CredentialAvailable bool    `json:"credential_available,omitempty"`
	NeedsCredentialSave bool    `json:"needs_credential_save,omitempty"`
	Routable            bool    `json:"routable"`
	AvailabilityReason  string  `json:"availability_reason,omitempty"`
}

type UpstreamKeyResponse struct {
	ID                      uint      `json:"id"`
	KeyID                   uint      `json:"key_id"`
	ChannelID               int       `json:"channel_id"`
	ExternalID              string    `json:"external_id"`
	Name                    string    `json:"name"`
	KeyPreview              string    `json:"key_preview"`
	Models                  []string  `json:"models"`
	AllowedModels           *[]string `json:"allowed_models,omitempty"`
	ModelsSynced            bool      `json:"models_synced"`
	KeyPriority             int64     `json:"key_priority"`
	SourceConversionRatio   float64   `json:"source_conversion_ratio"`
	ConversionRatio         float64   `json:"conversion_ratio"`
	ConversionRatioOverride *float64  `json:"conversion_ratio_override"`
	Weight                  int       `json:"weight"`
	AutoWeight              int       `json:"auto_weight"`
	WeightOverride          *int      `json:"weight_override"`
	Status                  int       `json:"status"`
	DisabledReason          string    `json:"disabled_reason"`
	LastSyncAt              int64     `json:"last_sync_at"`
	LastUsedAt              int64     `json:"last_used_at"`
	Routable                bool      `json:"routable"`
	AvailabilityReason      string    `json:"availability_reason,omitempty"`
	SnapshotOnly            bool      `json:"snapshot_only,omitempty"`
	CredentialUnavailable   bool      `json:"credential_unavailable,omitempty"`
	HealthStatus            string    `json:"health_status,omitempty"`
	HealthReason            string    `json:"health_reason,omitempty"`
	HealthSampleCount       int       `json:"health_sample_count,omitempty"`
	HealthSuccessCount      int       `json:"health_success_count,omitempty"`
	HealthFirstLatencyMs    int64     `json:"health_first_latency_ms,omitempty"`
}

type UpstreamKeyPatchRequest struct {
	KeyPriority          *int64    `json:"key_priority"`
	ConversionRatio      *float64  `json:"conversion_ratio"`
	ClearConversionRatio bool      `json:"clear_conversion_ratio"`
	WeightOverride       *int      `json:"weight_override"`
	ClearWeight          bool      `json:"clear_weight"`
	AllowedModels        *[]string `json:"allowed_models"`
	ClearAllowedModels   bool      `json:"clear_allowed_models"`
}

type UpstreamKeyBatchStatusRequest struct {
	IDs    []uint `json:"ids"`
	Status int    `json:"status"`
}

func validatePlatformSiteInput(input *PlatformSiteInput, existing *model.PlatformSiteAccount) (model.PlatformSiteCredential, float64, error) {
	if input == nil && existing == nil {
		return model.PlatformSiteCredential{}, 0, errors.New("平台站点配置不能为空")
	}
	if input == nil {
		return model.PlatformSiteCredential{}, existing.ConversionRatio, nil
	}
	platform := strings.ToLower(strings.TrimSpace(input.Platform))
	if platform != model.PlatformNewAPI && platform != model.PlatformSub2API {
		return model.PlatformSiteCredential{}, 0, errors.New("平台站点仅支持 NewAPI 或 Sub2API")
	}
	input.AuthType = strings.ToLower(strings.TrimSpace(input.AuthType))
	if input.AuthType != model.UpstreamAuthPassword &&
		input.AuthType != model.UpstreamAuthAccessToken &&
		input.AuthType != model.UpstreamAuthAdminKey &&
		input.AuthType != model.UpstreamAuthCookie {
		if input.AuthType == service.PlatformSiteCaptureAuthAuto {
			return model.PlatformSiteCredential{}, 0, errors.New("自动配置需要先完成上游登录态采集")
		}
		return model.PlatformSiteCredential{}, 0, errors.New("认证方式不受支持")
	}
	if err := service.ValidatePlatformSiteURLForAdmin(input.BaseURL); err != nil {
		return model.PlatformSiteCredential{}, 0, err
	}
	if strings.TrimSpace(input.RelayBaseURL) != "" {
		if err := service.ValidatePlatformSiteURLForAdmin(input.RelayBaseURL); err != nil {
			return model.PlatformSiteCredential{}, 0, err
		}
	}
	credential := model.PlatformSiteCredential{
		AuthType:       input.AuthType,
		Username:       strings.TrimSpace(input.Username),
		Password:       input.Password,
		UserID:         strings.TrimSpace(input.UserID),
		AccessToken:    strings.TrimSpace(input.AccessToken),
		RefreshToken:   strings.TrimSpace(input.RefreshToken),
		TokenExpiresAt: input.TokenExpiresAt,
		AdminKey:       strings.TrimSpace(input.AdminKey),
		Cookie:         input.Cookie,
	}
	switch input.AuthType {
	case model.UpstreamAuthPassword:
		if credential.Username == "" || credential.Password == "" {
			return model.PlatformSiteCredential{}, 0, errors.New("账号密码认证需要用户名和密码")
		}
		credential.AdminKey = ""
		credential.Cookie = ""
	case model.UpstreamAuthAccessToken:
		if credential.AccessToken == "" {
			return model.PlatformSiteCredential{}, 0, errors.New("访问令牌不能为空")
		}
		credential.Username = ""
		credential.Password = ""
		credential.AdminKey = ""
		credential.Cookie = ""
	case model.UpstreamAuthAdminKey:
		if credential.AdminKey == "" {
			return model.PlatformSiteCredential{}, 0, errors.New("Admin Key 不能为空")
		}
		credential.Username = ""
		credential.Password = ""
		credential.UserID = ""
		credential.AccessToken = ""
		credential.RefreshToken = ""
		credential.TokenExpiresAt = 0
		credential.Cookie = ""
	case model.UpstreamAuthCookie:
		if credential.Cookie == "" {
			return model.PlatformSiteCredential{}, 0, errors.New("Cookie 不能为空")
		}
		credential.Username = ""
		credential.Password = ""
		credential.UserID = ""
		credential.AccessToken = ""
		credential.RefreshToken = ""
		credential.TokenExpiresAt = 0
		credential.AdminKey = ""
	}
	rechargeAmount := 0.0
	if input.RechargeAmount != nil {
		rechargeAmount = *input.RechargeAmount
	}
	creditedAmount := 0.0
	if input.CreditedAmount != nil {
		creditedAmount = *input.CreditedAmount
	}
	for _, value := range []float64{rechargeAmount, creditedAmount} {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return model.PlatformSiteCredential{}, 0, errors.New("充值金额、到账金额和转换倍率必须是有限的非负数")
		}
	}
	ratio := 1.0
	if input.ConversionRatio != nil {
		ratio = *input.ConversionRatio
	} else if creditedAmount > 0 {
		ratio = rechargeAmount / creditedAmount
	} else if rechargeAmount > 0 {
		return model.PlatformSiteCredential{}, 0, errors.New("到账金额必须大于 0")
	}
	if math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio < 0 {
		return model.PlatformSiteCredential{}, 0, errors.New("充值金额、到账金额和转换倍率必须是有限的非负数")
	}
	if ratio > model.MaxUpstreamConversionRatio {
		return model.PlatformSiteCredential{}, 0, errors.New("转换倍率超出允许范围")
	}
	return credential, ratio, nil
}

func savePlatformSiteAccount(channelID int, input *PlatformSiteInput, existing *model.PlatformSiteAccount) error {
	if input != nil && existing != nil {
		merged := *input
		if strings.TrimSpace(merged.Platform) == "" {
			merged.Platform = existing.Platform
		}
		if strings.TrimSpace(merged.BaseURL) == "" {
			merged.BaseURL = existing.BaseURL
		}
		if strings.TrimSpace(merged.RelayBaseURL) == "" {
			merged.RelayBaseURL = existing.RelayBaseURL
		}
		if strings.TrimSpace(merged.AuthType) == "" {
			merged.AuthType = existing.AuthType
		}
		if strings.EqualFold(strings.TrimSpace(merged.AuthType), service.PlatformSiteCaptureAuthAuto) &&
			!hasCredentialInput(&merged) {
			merged.AuthType = existing.AuthType
		}
		if merged.RechargeAmount == nil {
			merged.RechargeAmount = &existing.RechargeAmount
		}
		if merged.CreditedAmount == nil {
			merged.CreditedAmount = &existing.CreditedAmount
		}
		if merged.ConversionRatio == nil &&
			input.RechargeAmount == nil &&
			input.CreditedAmount == nil {
			merged.ConversionRatio = &existing.ConversionRatio
		}
		if existing.CredentialCiphertext != "" {
			credential, decryptErr := model.DecryptPlatformSiteCredential(existing.CredentialCiphertext)
			if decryptErr != nil {
				if !hasPlatformSiteCredentialForAuthType(&merged) {
					return errors.New("平台凭据无法解密，请重新保存平台凭据")
				}
			} else {
				if strings.TrimSpace(merged.AuthType) == "" {
					merged.AuthType = model.InferPlatformSiteAuthType(credential)
				}
				switch strings.ToLower(strings.TrimSpace(merged.AuthType)) {
				case model.UpstreamAuthPassword:
					if strings.TrimSpace(merged.Username) == "" {
						merged.Username = credential.Username
					}
					if merged.Password == "" {
						merged.Password = credential.Password
					}
					if merged.AccessToken == "" {
						merged.AccessToken = credential.AccessToken
					}
					if merged.RefreshToken == "" {
						merged.RefreshToken = credential.RefreshToken
					}
					if merged.UserID == "" {
						merged.UserID = credential.UserID
					}
					merged.TokenExpiresAt = credential.TokenExpiresAt
				case model.UpstreamAuthAccessToken:
					if merged.AccessToken == "" {
						merged.AccessToken = credential.AccessToken
						merged.TokenExpiresAt = credential.TokenExpiresAt
					}
					if merged.RefreshToken == "" {
						merged.RefreshToken = credential.RefreshToken
					}
					if merged.UserID == "" {
						merged.UserID = credential.UserID
					}
				case model.UpstreamAuthAdminKey:
					if merged.AdminKey == "" {
						merged.AdminKey = credential.AdminKey
					}
				case model.UpstreamAuthCookie:
					if merged.Cookie == "" {
						merged.Cookie = credential.Cookie
					}
				}
			}
		}
		input = &merged
	}
	credential, ratio, err := validatePlatformSiteInput(input, existing)
	if err != nil {
		return err
	}
	account := model.PlatformSiteAccount{}
	if existing != nil {
		account = *existing
	}
	if input != nil {
		account.ChannelID = channelID
		account.Platform = strings.ToLower(strings.TrimSpace(input.Platform))
		account.BaseURL = strings.TrimRight(strings.TrimSpace(input.BaseURL), "/")
		account.RelayBaseURL = strings.TrimRight(strings.TrimSpace(input.RelayBaseURL), "/")
		account.AuthType = input.AuthType
		if existing != nil {
			account.SyncStatus = model.UpstreamSiteSyncIdle
			account.LastSyncError = ""
			account.DisabledAt = 0
			account.DisabledReason = ""
		}
		if input.RechargeAmount != nil {
			account.RechargeAmount = *input.RechargeAmount
		}
		if input.CreditedAmount != nil {
			account.CreditedAmount = *input.CreditedAmount
		}
		account.ConversionRatio = ratio
		if existing == nil || hasCredentialInput(input) {
			ciphertext, encryptErr := model.EncryptPlatformSiteCredential(credential)
			if encryptErr != nil {
				return encryptErr
			}
			account.CredentialCiphertext = ciphertext
			account.CredentialKeyVersion = "v1"
			account.CredentialFingerprint = credential.Fingerprint()
		}
	}
	if account.ChannelID == 0 {
		account.ChannelID = channelID
	}
	if account.SyncStatus == "" {
		account.SyncStatus = model.UpstreamSiteSyncIdle
	}
	return model.DB.Save(&account).Error
}

func hasCredentialInput(input *PlatformSiteInput) bool {
	return input != nil && (input.Username != "" || input.Password != "" || input.UserID != "" ||
		input.AccessToken != "" || input.RefreshToken != "" ||
		input.AdminKey != "" || input.Cookie != "")
}

func hasPlatformSiteCredentialForAuthType(input *PlatformSiteInput) bool {
	if input == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(input.AuthType)) {
	case model.UpstreamAuthPassword:
		return strings.TrimSpace(input.Username) != "" && input.Password != ""
	case model.UpstreamAuthAccessToken:
		return strings.TrimSpace(input.AccessToken) != ""
	case model.UpstreamAuthAdminKey:
		return strings.TrimSpace(input.AdminKey) != ""
	case model.UpstreamAuthCookie:
		return input.Cookie != ""
	default:
		return false
	}
}

func applyPlatformSiteCapture(userID, channelID int, input *PlatformSiteInput) (string, error) {
	if input == nil {
		return "", nil
	}
	captureID := strings.TrimSpace(input.CaptureID)
	if captureID == "" {
		return "", nil
	}
	if input.Username != "" || input.Password != "" || input.UserID != "" ||
		input.AccessToken != "" || input.RefreshToken != "" ||
		input.AdminKey != "" || input.Cookie != "" {
		return "", errors.New("采集会话不能与手动凭据同时提交")
	}
	resolution, err := service.ResolvePlatformSiteCapture(
		userID,
		captureID,
		channelID,
		input.Platform,
		input.AuthType,
	)
	if err != nil {
		return "", err
	}
	input.Platform = strings.ToLower(strings.TrimSpace(input.Platform))
	if input.Platform == "" {
		input.Platform = resolution.Platform
	}
	input.AuthType = resolution.Credential.AuthType
	input.Username = resolution.Credential.Username
	input.Password = resolution.Credential.Password
	input.UserID = resolution.Credential.UserID
	input.AccessToken = resolution.Credential.AccessToken
	input.RefreshToken = resolution.Credential.RefreshToken
	input.TokenExpiresAt = resolution.Credential.TokenExpiresAt
	input.AdminKey = resolution.Credential.AdminKey
	input.Cookie = resolution.Credential.Cookie
	if resolution.ManagementBaseURL != "" {
		input.BaseURL = resolution.ManagementBaseURL
	}
	if resolution.RelayBaseURL != "" {
		input.RelayBaseURL = resolution.RelayBaseURL
	}
	return captureID, nil
}

func redactBaseURL(raw string) string {
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return parsed.String()
	}
	return strings.TrimRight(raw, "/")
}

func platformSiteStatus(account *model.PlatformSiteAccount) UpstreamSiteStatusResponse {
	credentialAvailable := model.PlatformSiteCredentialAvailable(account)
	snapshotUsable := model.PlatformSiteSnapshotUsable(account)
	usingLastSnapshot := model.PlatformSiteUsingLastSnapshot(account)
	availabilityReason := model.UpstreamAvailabilityRoutable
	switch {
	case !credentialAvailable:
		availabilityReason = model.UpstreamAvailabilityCredentialUnavailable
	case !snapshotUsable:
		availabilityReason = model.UpstreamAvailabilitySiteSyncUnavailable
	case usingLastSnapshot:
		availabilityReason = model.UpstreamAvailabilitySnapshotOnly
	}
	return UpstreamSiteStatusResponse{
		ChannelID:           account.ChannelID,
		Platform:            account.Platform,
		BaseURL:             redactBaseURL(account.BaseURL),
		AuthType:            account.AuthType,
		RechargeAmount:      account.RechargeAmount,
		CreditedAmount:      account.CreditedAmount,
		ConversionRatio:     account.ConversionRatio,
		Balance:             account.Balance,
		UsedQuota:           account.UsedQuota,
		SyncStatus:          account.SyncStatus,
		LastSyncAt:          account.LastSyncAt,
		LastSyncError:       account.LastSyncError,
		ConsecutiveFailures: account.ConsecutiveFailures,
		SnapshotUsable:      snapshotUsable,
		UsingLastSnapshot:   usingLastSnapshot,
		CredentialAvailable: credentialAvailable,
		NeedsCredentialSave: !credentialAvailable,
		AvailabilityReason:  availabilityReason,
	}
}

func getPlatformSiteChannel(channelID int) (*model.Channel, error) {
	var channel model.Channel
	if err := model.DB.Select("id", "status", "upstream_kind", "balance_updated_time").First(&channel, "id = ?", channelID).Error; err != nil {
		return nil, err
	}
	if channel.UpstreamKind != model.UpstreamKindPlatformSite {
		return nil, errors.New("该渠道不是平台站点渠道")
	}
	return &channel, nil
}

func toUpstreamKeyResponse(key *model.UpstreamKey) UpstreamKeyResponse {
	models := key.GetModels()
	if models == nil {
		models = []string{}
	}
	allowedModels, configured, _ := key.GetAllowedModels()
	var allowedModelsResponse *[]string
	if configured {
		allowedModelsResponse = &allowedModels
	}
	return UpstreamKeyResponse{
		ID:                      key.ID,
		KeyID:                   key.RoutingKeyID,
		ChannelID:               key.ChannelID,
		ExternalID:              key.ExternalID,
		Name:                    key.Name,
		KeyPreview:              upstreamKeyPreview(key),
		Models:                  models,
		AllowedModels:           allowedModelsResponse,
		KeyPriority:             key.KeyPriority,
		SourceConversionRatio:   key.EffectiveSourceConversionRatio(),
		ConversionRatio:         key.ConversionRatio,
		ConversionRatioOverride: key.ConversionRatioOverride,
		Weight:                  key.EffectiveWeight(),
		AutoWeight:              key.AutoWeight(),
		WeightOverride:          key.WeightOverride,
		ModelsSynced:            key.ModelsSynced,
		Status:                  key.Status,
		DisabledReason:          key.DisabledReason,
		LastSyncAt:              key.LastSyncAt,
		LastUsedAt:              key.LastUsedAt,
	}
}

func toPlatformSiteKeyResponse(
	key *model.UpstreamKey,
	account *model.PlatformSiteAccount,
	channel *model.Channel,
) UpstreamKeyResponse {
	response := toUpstreamKeyResponse(key)
	response.SnapshotOnly = model.PlatformSiteUsingLastSnapshot(account)
	fillHealth := func(reason string) {
		health := model.GetRoutingKeyHealthSummary(
			key.RoutingKeyID,
			key.Status,
			reason,
			time.Now(),
		)
		response.HealthStatus = health.Status
		response.HealthReason = health.Reason
		response.HealthSampleCount = health.SampleCount
		response.HealthSuccessCount = health.SuccessCount
		response.HealthFirstLatencyMs = health.FirstLatencyMs
	}
	if channel == nil || channel.Status != common.ChannelStatusEnabled {
		response.Routable = false
		response.AvailabilityReason = model.UpstreamAvailabilityManualDisabled
		fillHealth(response.AvailabilityReason)
		return response
	}
	if !model.PlatformSiteSnapshotUsable(account) {
		response.Routable = false
		response.AvailabilityReason = model.UpstreamAvailabilitySiteSyncUnavailable
		fillHealth(response.AvailabilityReason)
		return response
	}
	response.AvailabilityReason = key.AvailabilityReason(time.Now())
	fillHealth(response.AvailabilityReason)
	if response.AvailabilityReason != model.UpstreamAvailabilityRoutable {
		response.CredentialUnavailable =
			response.AvailabilityReason == model.UpstreamAvailabilityCredentialUnavailable
		return response
	}
	if response.HealthStatus == model.RoutingKeyHealthDisabled ||
		response.HealthStatus == model.RoutingKeyHealthInvalid {
		response.Routable = false
		return response
	}
	if err := key.LoadSecret(); err != nil {
		response.Routable = false
		response.CredentialUnavailable = true
		response.AvailabilityReason = model.UpstreamAvailabilityCredentialUnavailable
		return response
	}
	response.Routable = true
	return response
}

func upstreamKeyPreview(key *model.UpstreamKey) string {
	if key == nil {
		return ""
	}
	if key.Secret != "" {
		return model.MaskTokenKey(key.Secret)
	}
	credential, err := model.DecryptPlatformSiteCredential(key.SecretCiphertext)
	if err != nil || credential.AccessToken == "" {
		return ""
	}
	return model.MaskTokenKey(credential.AccessToken)
}

func GetUpstreamSiteStatus(c *gin.Context) {
	channelID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	channel, err := getPlatformSiteChannel(channelID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var account model.PlatformSiteAccount
	if err := model.DB.Where("channel_id = ?", channelID).First(&account).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	response := platformSiteStatus(&account)
	response.BalanceUpdatedTime = channel.BalanceUpdatedTime
	var keys []model.UpstreamKey
	if err := model.DB.Where("channel_id = ?", channelID).Find(&keys).Error; err == nil {
		response.KeyCount = len(keys)
		for index := range keys {
			if channel.Status == common.ChannelStatusEnabled &&
				model.PlatformSiteSnapshotUsable(&account) &&
				keys[index].IsRoutable(time.Now()) &&
				keys[index].LoadSecret() == nil {
				response.RoutableKeyCount++
			}
		}
	}
	response.Routable = channel.Status == common.ChannelStatusEnabled &&
		response.SnapshotUsable &&
		response.RoutableKeyCount > 0
	common.ApiSuccess(c, response)
}

func GetUpstreamKeys(c *gin.Context) {
	channelID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if _, err := getPlatformSiteChannel(channelID); err != nil {
		common.ApiError(c, err)
		return
	}
	var account model.PlatformSiteAccount
	if err := model.DB.Where("channel_id = ?", channelID).First(&account).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	channel, err := getPlatformSiteChannel(channelID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var keys []model.UpstreamKey
	if err := model.DB.Where("channel_id = ?", channelID).Order("key_priority DESC, weight DESC, id ASC").Find(&keys).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	result := make([]UpstreamKeyResponse, 0, len(keys))
	for index := range keys {
		if err := model.EnsureRoutingKeyForUpstreamKey(nil, &keys[index]); err != nil {
			common.ApiError(c, err)
			return
		}
		result = append(result, toPlatformSiteKeyResponse(&keys[index], &account, channel))
	}
	common.ApiSuccess(c, gin.H{"items": result, "total": len(result)})
}

func PatchUpstreamKey(c *gin.Context) {
	channelID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if _, err := getPlatformSiteChannel(channelID); err != nil {
		common.ApiError(c, err)
		return
	}
	keyID, err := strconv.ParseUint(c.Param("keyId"), 10, 64)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var request UpstreamKeyPatchRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiError(c, err)
		return
	}
	var key model.UpstreamKey
	if err := model.DB.Where("id = ? AND channel_id = ?", keyID, channelID).First(&key).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	if request.ConversionRatio != nil && request.ClearConversionRatio {
		common.ApiError(c, errors.New("不能同时设置和清除密钥倍率覆盖"))
		return
	}
	if request.AllowedModels != nil && request.ClearAllowedModels {
		common.ApiError(c, errors.New("不能同时设置和清除密钥允许模型"))
		return
	}
	updates := map[string]any{}
	if request.KeyPriority != nil {
		if err := model.ValidateRoutingKeyPriority(*request.KeyPriority); err != nil {
			common.ApiError(c, err)
			return
		}
		updates["key_priority"] = *request.KeyPriority
	}
	effectiveRatio := key.ConversionRatio
	if request.ConversionRatio != nil {
		weight, weightErr := model.CalculateUpstreamKeyWeight(*request.ConversionRatio)
		if weightErr != nil {
			common.ApiError(c, weightErr)
			return
		}
		updates["conversion_ratio_override"] = *request.ConversionRatio
		updates["conversion_ratio"] = *request.ConversionRatio
		updates["weight"] = weight
		effectiveRatio = *request.ConversionRatio
	} else if request.ClearConversionRatio {
		var account model.PlatformSiteAccount
		if err := model.DB.Where("channel_id = ?", channelID).First(&account).Error; err != nil {
			common.ApiError(c, err)
			return
		}
		autoRatio, ratioErr := model.CalculatePlatformKeyConversionRatio(
			account.ConversionRatio,
			key.EffectiveSourceConversionRatio(),
		)
		if ratioErr != nil {
			common.ApiError(c, ratioErr)
			return
		}
		weight, weightErr := model.CalculateUpstreamKeyWeight(autoRatio)
		if weightErr != nil {
			common.ApiError(c, weightErr)
			return
		}
		updates["conversion_ratio_override"] = nil
		updates["conversion_ratio"] = autoRatio
		updates["weight"] = weight
		effectiveRatio = autoRatio
	}
	if effectiveRatio == 0 && request.WeightOverride != nil {
		common.ApiError(c, errors.New("免费密钥的权重固定为 2000，不允许修改"))
		return
	}
	if effectiveRatio == 0 {
		updates["weight"] = model.MaxUpstreamKeyWeight
		updates["weight_override"] = nil
	} else if request.ClearWeight {
		updates["weight_override"] = nil
	} else if request.WeightOverride != nil {
		if *request.WeightOverride < model.MinUpstreamKeyWeight || *request.WeightOverride > model.MaxUpstreamKeyWeight {
			common.ApiError(c, errors.New("密钥权重必须在 0 到 2000 之间"))
			return
		}
		updates["weight_override"] = *request.WeightOverride
	}
	if request.AllowedModels != nil {
		normalizedModels := make([]string, 0, len(*request.AllowedModels))
		seenModels := make(map[string]struct{}, len(*request.AllowedModels))
		realModels := make(map[string]struct{})
		for _, modelName := range key.GetModels() {
			realModels[modelName] = struct{}{}
			realModels[ratio_setting.RoutingMatchModelName(modelName)] = struct{}{}
		}
		for _, modelName := range *request.AllowedModels {
			modelName = strings.TrimSpace(modelName)
			if modelName == "" {
				continue
			}
			if _, exists := realModels[modelName]; !exists {
				if _, exists := realModels[ratio_setting.RoutingMatchModelName(modelName)]; !exists {
					common.ApiError(c, fmt.Errorf("密钥允许模型不存在于当前同步模型中: %s", modelName))
					return
				}
			}
			if _, exists := seenModels[modelName]; exists {
				continue
			}
			seenModels[modelName] = struct{}{}
			normalizedModels = append(normalizedModels, modelName)
		}
		encoded, marshalErr := common.Marshal(normalizedModels)
		if marshalErr != nil {
			common.ApiError(c, marshalErr)
			return
		}
		updates["allowed_models"] = string(encoded)
	}
	if request.ClearAllowedModels {
		updates["allowed_models"] = nil
	}
	if len(updates) == 0 {
		common.ApiError(c, errors.New("没有可更新的字段"))
		return
	}
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&key).Updates(updates).Error; err != nil {
			return err
		}
		return model.RebuildPlatformSiteChannelModels(tx, channelID)
	}); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "channel.upstream_key_update", map[string]any{
		"channel_id": channelID,
		"key_id":     key.ID,
		"fields":     keysOfMap(updates),
	})
	_ = model.DB.First(&key, "id = ?", key.ID).Error
	if err := model.EnsureRoutingKeyForUpstreamKey(nil, &key); err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	common.ApiSuccess(c, toUpstreamKeyResponse(&key))
}

func BatchUpdateUpstreamKeyStatus(c *gin.Context) {
	channelID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if _, err := getPlatformSiteChannel(channelID); err != nil {
		common.ApiError(c, err)
		return
	}
	var request UpstreamKeyBatchStatusRequest
	if err := c.ShouldBindJSON(&request); err != nil || len(request.IDs) == 0 {
		common.ApiError(c, errors.New("密钥 ID 不能为空"))
		return
	}
	if request.Status != model.UpstreamKeyStatusEnabled && request.Status != model.UpstreamKeyStatusManualDisabled {
		common.ApiError(c, errors.New("密钥状态不受支持"))
		return
	}
	result := model.DB.Model(&model.UpstreamKey{}).
		Where("channel_id = ? AND id IN ?", channelID, request.IDs).
		Updates(map[string]any{"status": request.Status, "disabled_reason": ""})
	if result.Error != nil {
		common.ApiError(c, result.Error)
		return
	}
	recordManageAudit(c, "channel.upstream_key_status_update", map[string]any{
		"channel_id": channelID,
		"count":      result.RowsAffected,
		"status":     request.Status,
	})
	common.ApiSuccess(c, gin.H{"updated": result.RowsAffected})
}

func SyncUpstreamSiteNow(c *gin.Context) {
	channelID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if _, err := getPlatformSiteChannel(channelID); err != nil {
		common.ApiError(c, err)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	err = service.SyncUpstreamSite(ctx, channelID)
	recordManageAudit(c, "channel.upstream_sync", map[string]any{
		"channel_id": channelID,
		"success":    err == nil,
	})
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": safeUpstreamErrorForResponse(err)})
		return
	}
	GetUpstreamSiteStatus(c)
}

func safeUpstreamErrorForResponse(err error) string {
	return service.SafePlatformSiteError(err)
}

func EnqueueUpstreamSiteSync(c *gin.Context) {
	channelID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if _, err := getPlatformSiteChannel(channelID); err != nil {
		common.ApiError(c, err)
		return
	}
	task, created, err := service.EnqueueUpstreamSiteSync(channelID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"task": task.ToResponse(), "created": created})
}

func keysOfMap(values map[string]any) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	return result
}

func ensurePlatformSiteChannel(channel *model.Channel, input *PlatformSiteInput) error {
	if channel == nil || channel.UpstreamKind != model.UpstreamKindPlatformSite {
		return nil
	}
	if channel.Type != constant.ChannelTypeSub2API && channel.Type != constant.ChannelTypeNewAPI {
		return errors.New("平台站点渠道类型必须是 NewAPI 或 Sub2API")
	}
	if input == nil {
		var account model.PlatformSiteAccount
		findErr := model.DB.Where("channel_id = ?", channel.Id).First(&account).Error
		if findErr != nil && !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return findErr
		}
		if errors.Is(findErr, gorm.ErrRecordNotFound) {
			return errors.New("平台站点配置不能为空")
		}
		return nil
	}
	platform := strings.ToLower(strings.TrimSpace(input.Platform))
	if platform == "" && channel.Id > 0 {
		var account model.PlatformSiteAccount
		if err := model.DB.Where("channel_id = ?", channel.Id).First(&account).Error; err != nil {
			return err
		}
		platform = account.Platform
	}
	switch channel.Type {
	case constant.ChannelTypeNewAPI:
		if platform != model.PlatformNewAPI {
			return errors.New("NewAPI 渠道必须使用 NewAPI 平台")
		}
	case constant.ChannelTypeSub2API:
		if platform != model.PlatformSub2API {
			return errors.New("Sub2API 渠道必须使用 Sub2API 平台")
		}
	}
	return nil
}

func StartPlatformSiteCapture(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32<<10)
	var request service.PlatformSiteCaptureStartRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorMsg(c, "无效的采集请求")
		return
	}
	result, err := service.StartPlatformSiteCaptureSession(
		c.GetInt("id"),
		request,
		externalRequestBaseURL(c),
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func GetPlatformSiteCaptureStatus(c *gin.Context) {
	result, err := service.GetPlatformSiteCaptureStatus(
		c.GetInt("id"),
		c.Param("captureID"),
		externalRequestBaseURL(c),
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func GetPlatformSiteCaptureUserscript(c *gin.Context) {
	script, err := service.RenderPlatformSiteCaptureUserscript(
		c.Param("captureID"),
		c.Query("install_token"),
		externalRequestBaseURL(c),
	)
	if err != nil {
		c.Header("Cache-Control", "no-store")
		c.String(http.StatusForbidden, "capture script unavailable")
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Content-Type", "application/javascript; charset=utf-8")
	c.String(http.StatusOK, script)
}

func GetPlatformSiteCaptureHelper(c *gin.Context) {
	script, err := service.RenderPlatformSiteCaptureHelper(externalRequestBaseURL(c))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Content-Type", "application/javascript; charset=utf-8")
	c.String(http.StatusOK, script)
}

func CompletePlatformSiteCapture(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 256<<10)
	var request service.PlatformSiteCaptureCompleteRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorMsg(c, "无效的采集回调")
		return
	}
	result, err := service.CompletePlatformSiteCaptureSession(c.Param("captureID"), request)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func externalRequestBaseURL(c *gin.Context) string {
	scheme := "http"
	host := ""
	if c.Request != nil {
		host = strings.TrimSpace(c.Request.Host)
		if c.Request.TLS != nil {
			scheme = "https"
		}
	}

	// 只有 Gin 已经根据可信代理配置解析出不同的客户端地址时，才采用
	// X-Forwarded-*。否则这些请求头可能来自普通客户端，不能用来拼接
	// Capture 回调地址。
	if c.Request != nil && c.RemoteIP() != "" && c.ClientIP() != c.RemoteIP() {
		forwardedScheme := strings.TrimSpace(strings.Split(c.GetHeader("X-Forwarded-Proto"), ",")[0])
		forwardedHost := strings.TrimSpace(strings.Split(c.GetHeader("X-Forwarded-Host"), ",")[0])
		if isSafeExternalRequestScheme(forwardedScheme) &&
			isSafeExternalRequestHost(forwardedHost) {
			scheme = strings.ToLower(forwardedScheme)
			host = forwardedHost
		}
	}
	if !isSafeExternalRequestScheme(scheme) || !isSafeExternalRequestHost(host) {
		return ""
	}
	return strings.ToLower(scheme) + "://" + host
}

func isSafeExternalRequestScheme(value string) bool {
	return value == "http" || value == "https"
}

func isSafeExternalRequestHost(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "/?#") {
		return false
	}
	parsed, err := url.Parse("//" + value)
	if err != nil || parsed.Host == "" || parsed.User != nil {
		return false
	}
	return parsed.Host == value
}
