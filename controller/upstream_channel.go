package controller

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/c1cadaBob/NexusTok/common"
	"github.com/c1cadaBob/NexusTok/constant"
	"github.com/c1cadaBob/NexusTok/model"
	"github.com/c1cadaBob/NexusTok/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type PlatformSiteInput struct {
	Platform        string   `json:"platform"`
	BaseURL         string   `json:"base_url"`
	AuthType        string   `json:"auth_type"`
	Username        string   `json:"username"`
	Password        string   `json:"password"`
	AccessToken     string   `json:"access_token"`
	RefreshToken    string   `json:"refresh_token"`
	AdminKey        string   `json:"admin_key"`
	Cookie          string   `json:"cookie"`
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
}

type UpstreamKeyResponse struct {
	ID                      uint     `json:"id"`
	ChannelID               int      `json:"channel_id"`
	ExternalID              string   `json:"external_id"`
	Name                    string   `json:"name"`
	KeyPreview              string   `json:"key_preview"`
	Models                  []string `json:"models"`
	ModelsSynced            bool     `json:"models_synced"`
	KeyPriority             int64    `json:"key_priority"`
	SourceConversionRatio   float64  `json:"source_conversion_ratio"`
	ConversionRatio         float64  `json:"conversion_ratio"`
	ConversionRatioOverride *float64 `json:"conversion_ratio_override"`
	Weight                  int      `json:"weight"`
	AutoWeight              int      `json:"auto_weight"`
	WeightOverride          *int     `json:"weight_override"`
	Status                  int      `json:"status"`
	DisabledReason          string   `json:"disabled_reason"`
	LastSyncAt              int64    `json:"last_sync_at"`
}

type UpstreamKeyPatchRequest struct {
	KeyPriority          *int64   `json:"key_priority"`
	ConversionRatio      *float64 `json:"conversion_ratio"`
	ClearConversionRatio bool     `json:"clear_conversion_ratio"`
	WeightOverride       *int     `json:"weight_override"`
	ClearWeight          bool     `json:"clear_weight"`
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
		return model.PlatformSiteCredential{}, 0, errors.New("认证方式不受支持")
	}
	if err := service.ValidatePlatformSiteURLForAdmin(input.BaseURL); err != nil {
		return model.PlatformSiteCredential{}, 0, err
	}
	credential := model.PlatformSiteCredential{
		Username:     strings.TrimSpace(input.Username),
		Password:     input.Password,
		AccessToken:  strings.TrimSpace(input.AccessToken),
		RefreshToken: strings.TrimSpace(input.RefreshToken),
		AdminKey:     strings.TrimSpace(input.AdminKey),
		Cookie:       input.Cookie,
	}
	switch input.AuthType {
	case model.UpstreamAuthPassword:
		if credential.Username == "" || credential.Password == "" {
			return model.PlatformSiteCredential{}, 0, errors.New("账号密码认证需要用户名和密码")
		}
	case model.UpstreamAuthAccessToken:
		if credential.AccessToken == "" {
			return model.PlatformSiteCredential{}, 0, errors.New("访问令牌不能为空")
		}
	case model.UpstreamAuthAdminKey:
		if credential.AdminKey == "" {
			return model.PlatformSiteCredential{}, 0, errors.New("Admin Key 不能为空")
		}
	case model.UpstreamAuthCookie:
		if credential.Cookie == "" {
			return model.PlatformSiteCredential{}, 0, errors.New("Cookie 不能为空")
		}
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
		if strings.TrimSpace(merged.AuthType) == "" {
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
					return decryptErr
				}
			} else {
				switch strings.ToLower(strings.TrimSpace(merged.AuthType)) {
				case model.UpstreamAuthPassword:
					if strings.TrimSpace(merged.Username) == "" {
						merged.Username = credential.Username
					}
					if merged.Password == "" {
						merged.Password = credential.Password
					}
				case model.UpstreamAuthAccessToken:
					if merged.AccessToken == "" {
						merged.AccessToken = credential.AccessToken
					}
					if merged.RefreshToken == "" {
						merged.RefreshToken = credential.RefreshToken
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
	return input != nil && (input.Username != "" || input.Password != "" || input.AccessToken != "" || input.RefreshToken != "" || input.AdminKey != "" || input.Cookie != "")
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

func redactBaseURL(raw string) string {
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return parsed.String()
	}
	return strings.TrimRight(raw, "/")
}

func platformSiteStatus(account *model.PlatformSiteAccount) UpstreamSiteStatusResponse {
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
	}
}

func getPlatformSiteChannel(channelID int) (*model.Channel, error) {
	var channel model.Channel
	if err := model.DB.Select("id", "upstream_kind", "balance_updated_time").First(&channel, "id = ?", channelID).Error; err != nil {
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
	return UpstreamKeyResponse{
		ID:                      key.ID,
		ChannelID:               key.ChannelID,
		ExternalID:              key.ExternalID,
		Name:                    key.Name,
		KeyPreview:              upstreamKeyPreview(key),
		Models:                  models,
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
	}
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
			if keys[index].ModelsSynced && keys[index].IsRoutable(time.Now()) {
				response.RoutableKeyCount++
			}
		}
	}
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
	var keys []model.UpstreamKey
	if err := model.DB.Where("channel_id = ?", channelID).Order("key_priority DESC, weight DESC, id ASC").Find(&keys).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	result := make([]UpstreamKeyResponse, 0, len(keys))
	for _, key := range keys {
		result = append(result, toUpstreamKeyResponse(&key))
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
	updates := map[string]any{}
	if request.KeyPriority != nil {
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
	if len(updates) == 0 {
		common.ApiError(c, errors.New("没有可更新的字段"))
		return
	}
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		return tx.Model(&key).Updates(updates).Error
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
	payload := map[string]any{"channel_id": channelID}
	task, created, err := service.EnqueueSystemTask(model.SystemTaskTypeUpstreamSync, payload)
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
