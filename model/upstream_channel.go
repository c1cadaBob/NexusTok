package model

import (
	"errors"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/c1cadaBob/NexusTok/common"
	"github.com/c1cadaBob/NexusTok/setting/ratio_setting"
	"gorm.io/gorm"
)

const (
	UpstreamKindKeyChannel   = "key_channel"
	UpstreamKindPlatformSite = "platform_site"

	PlatformNewAPI  = "newapi"
	PlatformSub2API = "sub2api"

	UpstreamAuthPassword    = "password"
	UpstreamAuthAccessToken = "access_token"
	UpstreamAuthAdminKey    = "admin_key"
	UpstreamAuthCookie      = "cookie"

	UpstreamSiteSyncIdle    = "idle"
	UpstreamSiteSyncRunning = "running"
	UpstreamSiteSyncSuccess = "success"
	UpstreamSiteSyncFailed  = "failed"

	UpstreamKeyStatusEnabled        = common.ChannelStatusEnabled
	UpstreamKeyStatusManualDisabled = common.ChannelStatusManuallyDisabled
	UpstreamKeyStatusAutoDisabled   = common.ChannelStatusAutoDisabled
	UpstreamKeyStatusMissing        = 4
)

const (
	UpstreamAvailabilityRoutable              = "routable"
	UpstreamAvailabilityCredentialUnavailable = "credential_unavailable"
	UpstreamAvailabilityModelsUnavailable     = "models_unavailable"
	UpstreamAvailabilitySnapshotOnly          = "snapshot_only"
	UpstreamAvailabilitySiteSyncUnavailable   = "site_sync_unavailable"
	UpstreamAvailabilityManualDisabled        = "manual_disabled"
	UpstreamAvailabilityUpstreamDisabled      = "upstream_disabled"
	UpstreamAvailabilityMissing               = "missing"
	UpstreamAvailabilityExpired               = "expired"
	UpstreamAvailabilityQuotaExhausted        = "quota_exhausted"
)

const (
	MinUpstreamKeyWeight       = 0
	MaxUpstreamKeyWeight       = 2000
	MaxUpstreamConversionRatio = 1000
)

type PlatformSiteCredential struct {
	AuthType       string `json:"auth_type,omitempty"`
	Username       string `json:"username,omitempty"`
	Password       string `json:"password,omitempty"`
	AccessToken    string `json:"access_token,omitempty"`
	RefreshToken   string `json:"refresh_token,omitempty"`
	TokenExpiresAt int64  `json:"token_expires_at,omitempty"`
	AdminKey       string `json:"admin_key,omitempty"`
	Cookie         string `json:"cookie,omitempty"`
}

type PlatformSiteAccount struct {
	ID        uint   `json:"id" gorm:"primaryKey"`
	ChannelID int    `json:"channel_id" gorm:"not null;uniqueIndex"`
	Platform  string `json:"platform" gorm:"type:varchar(32);not null;index"`
	BaseURL   string `json:"base_url" gorm:"type:varchar(1024);not null"`
	// RelayBaseURL 保存平台页面发现的实际转发地址，BaseURL 始终保留管理接口地址。
	RelayBaseURL string `json:"relay_base_url" gorm:"type:varchar(1024)"`
	// AuthType 对旧数据库允许为空，迁移和同步会为可解密的历史凭据补齐。
	AuthType              string  `json:"auth_type" gorm:"type:varchar(32);index"`
	CredentialCiphertext  string  `json:"-" gorm:"type:text;not null"`
	CredentialKeyVersion  string  `json:"-" gorm:"type:varchar(32);not null"`
	CredentialFingerprint string  `json:"credential_fingerprint" gorm:"type:varchar(128);index"`
	RechargeAmount        float64 `json:"recharge_amount"`
	CreditedAmount        float64 `json:"credited_amount"`
	ConversionRatio       float64 `json:"conversion_ratio"`
	Balance               float64 `json:"balance"`
	UsedQuota             int64   `json:"used_quota" gorm:"bigint"`
	LastSyncAt            int64   `json:"last_sync_at" gorm:"bigint;index"`
	SyncStatus            string  `json:"sync_status" gorm:"type:varchar(32);index"`
	LastSyncError         string  `json:"last_sync_error" gorm:"type:text"`
	ConsecutiveFailures   int     `json:"consecutive_failures"`
	DisabledAt            int64   `json:"disabled_at" gorm:"bigint"`
	DisabledReason        string  `json:"disabled_reason" gorm:"type:varchar(255)"`
}

type UpstreamKey struct {
	ID                      uint       `json:"id" gorm:"primaryKey"`
	RoutingKeyID            uint       `json:"routing_key_id" gorm:"index"`
	ChannelID               int        `json:"channel_id" gorm:"not null;index;uniqueIndex:,composite:channel_external,priority:1"`
	ExternalID              string     `json:"external_id" gorm:"type:varchar(255);not null;uniqueIndex:,composite:channel_external,priority:2"`
	Name                    string     `json:"name" gorm:"type:varchar(255)"`
	SecretCiphertext        string     `json:"-" gorm:"type:text;not null"`
	SecretFingerprint       string     `json:"secret_fingerprint" gorm:"type:varchar(128);index"`
	Models                  string     `json:"models" gorm:"type:text"`
	ModelsSynced            bool       `json:"models_synced"`
	AllowedModelsJSON       *string    `json:"-" gorm:"type:text;column:allowed_models"`
	KeyPriority             int64      `json:"key_priority" gorm:"bigint;index"`
	SourceConversionRatio   *float64   `json:"source_conversion_ratio"`
	ConversionRatio         float64    `json:"conversion_ratio"`
	ConversionRatioOverride *float64   `json:"conversion_ratio_override"`
	Weight                  int        `json:"weight" gorm:"index"`
	WeightOverride          *int       `json:"weight_override" gorm:"index"`
	UsedQuota               int64      `json:"used_quota" gorm:"bigint"`
	RemainQuota             *int64     `json:"remain_quota" gorm:"bigint"`
	ExpiresAt               *time.Time `json:"expires_at"`
	Status                  int        `json:"status" gorm:"index"`
	DisabledReason          string     `json:"disabled_reason" gorm:"type:varchar(255)"`
	LastSyncAt              int64      `json:"last_sync_at" gorm:"bigint;index"`
	LastUsedAt              int64      `json:"last_used_at" gorm:"bigint;index"`
	MissingSince            int64      `json:"missing_since" gorm:"bigint"`
	Secret                  string     `json:"-" gorm:"-"`
}

type UpstreamKeyAbility struct {
	UpstreamKeyID uint   `json:"upstream_key_id" gorm:"primaryKey;autoIncrement:false"`
	Group         string `json:"group" gorm:"type:varchar(64);primaryKey;autoIncrement:false"`
	Model         string `json:"model" gorm:"type:varchar(255);primaryKey;autoIncrement:false"`
	Enabled       bool   `json:"enabled"`
}

func (credential PlatformSiteCredential) Fingerprint() string {
	return common.GenerateHMAC(strings.Join([]string{
		credential.AuthType,
		credential.Username,
		credential.Password,
		credential.AccessToken,
		credential.RefreshToken,
		strconv.FormatInt(credential.TokenExpiresAt, 10),
		credential.AdminKey,
		credential.Cookie,
	}, "\x00"))
}

func IsPlatformSiteAuthType(authType string) bool {
	switch strings.ToLower(strings.TrimSpace(authType)) {
	case UpstreamAuthPassword, UpstreamAuthAccessToken, UpstreamAuthAdminKey, UpstreamAuthCookie:
		return true
	default:
		return false
	}
}

// NormalizeSub2APIRelayBaseURL 将页面发现的 OpenAI 兼容端点转换为转发适配器
// 使用的渠道基础地址。转发请求路径已经包含 /v1 前缀，而 Sub2API 页面配置
// 常见返回值本身以 /v1 结尾。
func NormalizeSub2APIRelayBaseURL(raw string) string {
	normalized := strings.TrimRight(strings.TrimSpace(raw), "/")
	if normalized == "" {
		return ""
	}
	parsed, err := url.Parse(normalized)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return normalized
	}
	path := strings.TrimRight(parsed.Path, "/")
	if path == "/v1" {
		parsed.Path = ""
		parsed.RawPath = ""
	} else if strings.HasSuffix(path, "/v1") {
		parsed.Path = strings.TrimSuffix(path, "/v1")
		parsed.RawPath = ""
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/")
}

// InferPlatformSiteAuthType 仅用于升级旧数据和修复缺少 AuthType 的记录。
// 正常同步必须使用 PlatformSiteAccount.AuthType，不再根据凭据字段动态选择认证方式。
func InferPlatformSiteAuthType(credential PlatformSiteCredential) string {
	if IsPlatformSiteAuthType(credential.AuthType) {
		return strings.ToLower(strings.TrimSpace(credential.AuthType))
	}
	switch {
	case strings.TrimSpace(credential.Username) != "" || credential.Password != "":
		return UpstreamAuthPassword
	case strings.TrimSpace(credential.AdminKey) != "":
		return UpstreamAuthAdminKey
	case strings.TrimSpace(credential.AccessToken) != "" || strings.TrimSpace(credential.RefreshToken) != "":
		return UpstreamAuthAccessToken
	case strings.TrimSpace(credential.Cookie) != "":
		return UpstreamAuthCookie
	default:
		return ""
	}
}

func PlatformSiteSnapshotUsable(account *PlatformSiteAccount) bool {
	if account == nil {
		return false
	}
	switch account.SyncStatus {
	case UpstreamSiteSyncSuccess:
		return true
	case UpstreamSiteSyncFailed, UpstreamSiteSyncRunning:
		return account.LastSyncAt > 0
	default:
		return false
	}
}

func PlatformSiteUsingLastSnapshot(account *PlatformSiteAccount) bool {
	if account == nil || account.LastSyncAt <= 0 {
		return false
	}
	return account.SyncStatus == UpstreamSiteSyncFailed ||
		account.SyncStatus == UpstreamSiteSyncRunning
}

func PlatformSiteCredentialAvailable(account *PlatformSiteAccount) bool {
	if account == nil || strings.TrimSpace(account.CredentialCiphertext) == "" {
		return false
	}
	_, err := DecryptPlatformSiteCredential(account.CredentialCiphertext)
	return err == nil
}

func (key *UpstreamKey) AvailabilityReason(now time.Time) string {
	if key == nil {
		return UpstreamAvailabilityCredentialUnavailable
	}
	if key.Status == UpstreamKeyStatusManualDisabled {
		return UpstreamAvailabilityManualDisabled
	}
	if key.Status == UpstreamKeyStatusMissing || key.MissingSince != 0 {
		return UpstreamAvailabilityMissing
	}
	if key.Status != UpstreamKeyStatusEnabled {
		switch strings.TrimSpace(key.DisabledReason) {
		case "credential_unavailable":
			return UpstreamAvailabilityCredentialUnavailable
		case "models_unavailable", "上游密钥模型能力读取失败":
			return UpstreamAvailabilityModelsUnavailable
		case "expired", "密钥已过期":
			return UpstreamAvailabilityExpired
		case "quota_exhausted", "密钥剩余额度不足":
			return UpstreamAvailabilityQuotaExhausted
		case "upstream_disabled", "上游平台已禁用":
			return UpstreamAvailabilityUpstreamDisabled
		}
		return UpstreamAvailabilityUpstreamDisabled
	}
	if !key.ModelsSynced || len(key.GetEffectiveModels()) == 0 {
		return UpstreamAvailabilityModelsUnavailable
	}
	if key.ExpiresAt != nil && !key.ExpiresAt.After(now) {
		return UpstreamAvailabilityExpired
	}
	if key.RemainQuota != nil && *key.RemainQuota <= 0 {
		return UpstreamAvailabilityQuotaExhausted
	}
	return UpstreamAvailabilityRoutable
}

func EncryptPlatformSiteCredential(credential PlatformSiteCredential) (string, error) {
	payload, err := common.Marshal(credential)
	if err != nil {
		return "", err
	}
	return common.EncryptUpstreamCredential(string(payload))
}

func DecryptPlatformSiteCredential(ciphertext string) (PlatformSiteCredential, error) {
	plaintext, err := common.DecryptUpstreamCredential(ciphertext)
	if err != nil {
		return PlatformSiteCredential{}, err
	}
	var credential PlatformSiteCredential
	if err := common.Unmarshal([]byte(plaintext), &credential); err != nil {
		return PlatformSiteCredential{}, err
	}
	return credential, nil
}

func CalculateUpstreamKeyWeight(conversionRatio float64) (int, error) {
	if math.IsNaN(conversionRatio) || math.IsInf(conversionRatio, 0) ||
		conversionRatio < 0 || conversionRatio > MaxUpstreamConversionRatio {
		return 0, errors.New("conversion ratio must be a finite non-negative number")
	}
	if conversionRatio >= 2 {
		return MinUpstreamKeyWeight, nil
	}
	weight := int(math.Round(1000 + (1-conversionRatio)/0.001))
	if weight < MinUpstreamKeyWeight {
		return MinUpstreamKeyWeight, nil
	}
	if weight > MaxUpstreamKeyWeight {
		return MaxUpstreamKeyWeight, nil
	}
	return weight, nil
}

func (key *UpstreamKey) AutoWeight() int {
	if key == nil {
		return MinUpstreamKeyWeight
	}
	weight, err := CalculateUpstreamKeyWeight(key.ConversionRatio)
	if err != nil {
		return MinUpstreamKeyWeight
	}
	return weight
}

func (key *UpstreamKey) EffectiveWeight() int {
	if key.ConversionRatio == 0 {
		return MaxUpstreamKeyWeight
	}
	if key.WeightOverride != nil {
		return max(MinUpstreamKeyWeight, min(MaxUpstreamKeyWeight, *key.WeightOverride))
	}
	return max(MinUpstreamKeyWeight, min(MaxUpstreamKeyWeight, key.AutoWeight()))
}

func (key *UpstreamKey) EffectiveSourceConversionRatio() float64 {
	if key == nil || key.SourceConversionRatio == nil {
		return 1
	}
	return *key.SourceConversionRatio
}

func CalculatePlatformKeyConversionRatio(siteRatio, sourceRatio float64) (float64, error) {
	if math.IsNaN(siteRatio) || math.IsInf(siteRatio, 0) || siteRatio < 0 ||
		siteRatio > MaxUpstreamConversionRatio {
		return 0, errors.New("site conversion ratio must be a finite non-negative number")
	}
	if math.IsNaN(sourceRatio) || math.IsInf(sourceRatio, 0) || sourceRatio < 0 ||
		sourceRatio > MaxUpstreamConversionRatio {
		return 0, errors.New("source conversion ratio must be a finite non-negative number")
	}
	ratio := siteRatio * sourceRatio
	if math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio < 0 ||
		ratio > MaxUpstreamConversionRatio {
		return 0, errors.New("effective conversion ratio exceeds the supported range")
	}
	return ratio, nil
}

func (key *UpstreamKey) GetModels() []string {
	if key == nil {
		return nil
	}
	if strings.TrimSpace(key.Models) == "" {
		return nil
	}
	models := strings.Split(key.Models, ",")
	result := make([]string, 0, len(models))
	for _, modelName := range models {
		modelName = strings.TrimSpace(modelName)
		if modelName != "" {
			result = append(result, modelName)
		}
	}
	return result
}

// GetAllowedModels 返回管理员配置的允许模型列表。第二个返回值表示是否
// 配置过限制，用于区分 NULL（未限制）和 []（明确禁止全部模型）。
func (key *UpstreamKey) GetAllowedModels() ([]string, bool, error) {
	if key == nil || key.AllowedModelsJSON == nil ||
		strings.TrimSpace(*key.AllowedModelsJSON) == "" {
		return nil, false, nil
	}
	var models []string
	if err := common.Unmarshal([]byte(*key.AllowedModelsJSON), &models); err != nil {
		return nil, true, err
	}
	normalized := make([]string, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, modelName := range models {
		modelName = strings.TrimSpace(modelName)
		if modelName == "" {
			continue
		}
		if _, exists := seen[modelName]; exists {
			continue
		}
		seen[modelName] = struct{}{}
		normalized = append(normalized, modelName)
	}
	return normalized, true, nil
}

func (key *UpstreamKey) GetEffectiveModels() []string {
	if key == nil {
		return nil
	}
	allowed, configured, err := key.GetAllowedModels()
	if err != nil || !configured {
		return key.GetModels()
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, modelName := range allowed {
		allowedSet[modelName] = struct{}{}
		allowedSet[ratio_setting.RoutingMatchModelName(modelName)] = struct{}{}
	}
	effective := make([]string, 0, len(allowed))
	for _, modelName := range key.GetModels() {
		if _, exists := allowedSet[modelName]; exists {
			effective = append(effective, modelName)
			continue
		}
		if _, exists := allowedSet[ratio_setting.RoutingMatchModelName(modelName)]; exists {
			effective = append(effective, modelName)
		}
	}
	return effective
}

func (key *UpstreamKey) AllowsModel(modelName string) bool {
	if key == nil {
		return false
	}
	allowed, configured, err := key.GetAllowedModels()
	if err != nil || !configured {
		return true
	}
	normalizedModel := ratio_setting.RoutingMatchModelName(modelName)
	for _, allowedModel := range allowed {
		if allowedModel == modelName ||
			ratio_setting.RoutingMatchModelName(allowedModel) == normalizedModel {
			return true
		}
	}
	return false
}

func (key *UpstreamKey) IsRoutable(now time.Time) bool {
	return key != nil && key.AvailabilityReason(now) == UpstreamAvailabilityRoutable
}

func (key *UpstreamKey) LoadSecret() error {
	if key == nil {
		return errors.New("upstream key is nil")
	}
	credential, err := DecryptPlatformSiteCredential(key.SecretCiphertext)
	if err != nil {
		return err
	}
	if credential.AccessToken == "" {
		return errors.New("upstream key secret unavailable")
	}
	key.Secret = credential.AccessToken
	return nil
}

func GetRoutableUpstreamKeyByID(channelID int, keyID uint, group, modelName string, now time.Time) (*UpstreamKey, error) {
	var channel Channel
	if err := DB.Select("id", "status", "upstream_kind").
		Where("id = ?", channelID).
		First(&channel).Error; err != nil {
		return nil, err
	}
	if channel.UpstreamKind != UpstreamKindPlatformSite ||
		channel.Status != common.ChannelStatusEnabled {
		return nil, errors.New("platform site is not routable")
	}

	var account PlatformSiteAccount
	if err := DB.Select("sync_status", "last_sync_at").
		Where("channel_id = ?", channelID).
		First(&account).Error; err != nil {
		return nil, err
	}
	if !PlatformSiteSnapshotUsable(&account) {
		return nil, errors.New("platform site is not routable")
	}

	var key UpstreamKey
	if err := DB.Where("id = ? AND channel_id = ?", keyID, channelID).First(&key).Error; err != nil {
		return nil, err
	}
	if key.RoutingKeyID == 0 {
		if err := EnsureRoutingKeyForUpstreamKey(nil, &key); err != nil {
			return nil, err
		}
	}
	if !key.IsRoutable(now) {
		return nil, errors.New("upstream key is not routable")
	}
	if !key.ModelsSynced {
		return nil, errors.New("upstream key model capability is not synchronized")
	}
	if strings.TrimSpace(modelName) != "" && !upstreamKeySupportsModel(&key, group, modelName) {
		return nil, errors.New("upstream key does not support the test model")
	}
	if err := key.LoadSecret(); err != nil {
		return nil, err
	}
	return &key, nil
}

func UpdateUpstreamKeyLastUsed(keyID uint, unixTime int64) error {
	if keyID == 0 || unixTime <= 0 {
		return nil
	}
	return DB.Model(&UpstreamKey{}).Where("id = ?", keyID).Update("last_used_at", unixTime).Error
}

func RebuildPlatformSiteChannelModels(tx *gorm.DB, channelID int) error {
	if tx == nil {
		tx = DB
	}
	if channelID <= 0 {
		return errors.New("channel ID is invalid")
	}
	var keys []UpstreamKey
	if err := tx.Where("channel_id = ?", channelID).Order("id ASC").Find(&keys).Error; err != nil {
		return err
	}
	seen := make(map[string]struct{})
	models := make([]string, 0)
	for _, key := range keys {
		if !key.ModelsSynced {
			continue
		}
		for _, modelName := range key.GetEffectiveModels() {
			if _, exists := seen[modelName]; exists {
				continue
			}
			seen[modelName] = struct{}{}
			models = append(models, modelName)
		}
	}
	return tx.Model(&Channel{}).
		Where("id = ?", channelID).
		Update("models", strings.Join(models, ",")).Error
}

func PersistPlatformSiteKeyModels(tx *gorm.DB, channelID int, keyID uint, models []string) error {
	if tx == nil {
		tx = DB
	}
	if channelID <= 0 || keyID == 0 {
		return errors.New("平台站点密钥参数无效")
	}
	normalized := make([]string, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, modelName := range models {
		modelName = strings.TrimSpace(modelName)
		if modelName == "" {
			continue
		}
		if _, exists := seen[modelName]; exists {
			continue
		}
		seen[modelName] = struct{}{}
		normalized = append(normalized, modelName)
	}
	return tx.Transaction(func(tx *gorm.DB) error {
		var key UpstreamKey
		if err := tx.Where("id = ? AND channel_id = ?", keyID, channelID).First(&key).Error; err != nil {
			return err
		}
		updates := map[string]any{
			"models":        strings.Join(normalized, ","),
			"models_synced": true,
			"last_sync_at":  common.GetTimestamp(),
		}
		key.Models = strings.Join(normalized, ",")
		key.ModelsSynced = true
		if key.Status != UpstreamKeyStatusManualDisabled {
			if len(key.GetEffectiveModels()) > 0 {
				updates["status"] = UpstreamKeyStatusEnabled
				updates["disabled_reason"] = ""
			} else {
				updates["status"] = UpstreamKeyStatusAutoDisabled
				updates["disabled_reason"] = "models_unavailable"
			}
		}
		if err := tx.Model(&key).Updates(updates).Error; err != nil {
			return err
		}
		if err := tx.Where("upstream_key_id = ?", key.ID).Delete(&UpstreamKeyAbility{}).Error; err != nil {
			return err
		}
		for _, modelName := range normalized {
			if err := tx.Create(&UpstreamKeyAbility{
				UpstreamKeyID: key.ID,
				Model:         modelName,
				Enabled:       true,
			}).Error; err != nil {
				return err
			}
		}
		return RebuildPlatformSiteChannelModels(tx, channelID)
	})
}

func DeleteUpstreamData(tx *gorm.DB, channelIDs []int) error {
	if tx == nil || len(channelIDs) == 0 {
		return nil
	}
	if tx.Migrator().HasTable(&RoutingKeyHealth{}) {
		if err := tx.Where("channel_id IN ?", channelIDs).Delete(&RoutingKeyHealth{}).Error; err != nil {
			return err
		}
	}
	if tx.Migrator().HasTable(&ChannelKey{}) {
		if err := tx.Where("channel_id IN ?", channelIDs).Delete(&ChannelKey{}).Error; err != nil {
			return err
		}
	}
	if tx.Migrator().HasTable(&UpstreamKey{}) {
		var keys []UpstreamKey
		if err := tx.Where("channel_id IN ?", channelIDs).Find(&keys).Error; err != nil {
			return err
		}
		keyIDs := make([]uint, 0, len(keys))
		for _, key := range keys {
			keyIDs = append(keyIDs, key.ID)
		}
		if len(keyIDs) > 0 && tx.Migrator().HasTable(&UpstreamKeyAbility{}) {
			if err := tx.Where("upstream_key_id IN ?", keyIDs).Delete(&UpstreamKeyAbility{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("channel_id IN ?", channelIDs).Delete(&UpstreamKey{}).Error; err != nil {
			return err
		}
	}
	if tx.Migrator().HasTable(&RoutingKey{}) {
		if err := tx.Where("channel_id IN ?", channelIDs).Delete(&RoutingKey{}).Error; err != nil {
			return err
		}
	}
	if !tx.Migrator().HasTable(&PlatformSiteAccount{}) {
		return nil
	}
	return tx.Where("channel_id IN ?", channelIDs).Delete(&PlatformSiteAccount{}).Error
}
