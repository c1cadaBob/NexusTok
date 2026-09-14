package model

import (
	"errors"
	"math"
	"strings"
	"time"

	"github.com/c1cadaBob/NexusTok/common"
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
	MinUpstreamKeyWeight       = 0
	MaxUpstreamKeyWeight       = 2000
	MaxUpstreamConversionRatio = 1000
)

type PlatformSiteCredential struct {
	Username    string `json:"username,omitempty"`
	Password    string `json:"password,omitempty"`
	AccessToken string `json:"access_token,omitempty"`
	AdminKey    string `json:"admin_key,omitempty"`
	Cookie      string `json:"cookie,omitempty"`
}

type PlatformSiteAccount struct {
	ID                    uint    `json:"id" gorm:"primaryKey"`
	ChannelID             int     `json:"channel_id" gorm:"not null;uniqueIndex"`
	Platform              string  `json:"platform" gorm:"type:varchar(32);not null;index"`
	BaseURL               string  `json:"base_url" gorm:"type:varchar(1024);not null"`
	AuthType              string  `json:"auth_type" gorm:"type:varchar(32);not null"`
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
	ID                uint       `json:"id" gorm:"primaryKey"`
	ChannelID         int        `json:"channel_id" gorm:"not null;index;uniqueIndex:idx_upstream_key_channel_external"`
	ExternalID        string     `json:"external_id" gorm:"type:varchar(255);not null;uniqueIndex:idx_upstream_key_channel_external"`
	Name              string     `json:"name" gorm:"type:varchar(255)"`
	SecretCiphertext  string     `json:"-" gorm:"type:text;not null"`
	SecretFingerprint string     `json:"secret_fingerprint" gorm:"type:varchar(128);index"`
	Models            string     `json:"models" gorm:"type:text"`
	KeyPriority       int64      `json:"key_priority" gorm:"bigint;index"`
	ConversionRatio   float64    `json:"conversion_ratio"`
	Weight            int        `json:"weight" gorm:"index"`
	WeightOverride    *int       `json:"weight_override" gorm:"index"`
	UsedQuota         int64      `json:"used_quota" gorm:"bigint"`
	RemainQuota       *int64     `json:"remain_quota" gorm:"bigint"`
	ExpiresAt         *time.Time `json:"expires_at"`
	Status            int        `json:"status" gorm:"index"`
	DisabledReason    string     `json:"disabled_reason" gorm:"type:varchar(255)"`
	LastSyncAt        int64      `json:"last_sync_at" gorm:"bigint;index"`
	LastUsedAt        int64      `json:"last_used_at" gorm:"bigint;index"`
	MissingSince      int64      `json:"missing_since" gorm:"bigint"`
	Secret            string     `json:"-" gorm:"-"`
}

type UpstreamKeyAbility struct {
	UpstreamKeyID uint   `json:"upstream_key_id" gorm:"primaryKey;autoIncrement:false"`
	Group         string `json:"group" gorm:"type:varchar(64);primaryKey;autoIncrement:false"`
	Model         string `json:"model" gorm:"type:varchar(255);primaryKey;autoIncrement:false"`
	Enabled       bool   `json:"enabled"`
}

func (credential PlatformSiteCredential) Fingerprint() string {
	return common.GenerateHMAC(strings.Join([]string{
		credential.Username,
		credential.Password,
		credential.AccessToken,
		credential.AdminKey,
		credential.Cookie,
	}, "\x00"))
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

func (key *UpstreamKey) EffectiveWeight() int {
	if key.ConversionRatio == 0 {
		return MaxUpstreamKeyWeight
	}
	if key.WeightOverride != nil {
		return max(MinUpstreamKeyWeight, min(MaxUpstreamKeyWeight, *key.WeightOverride))
	}
	return max(MinUpstreamKeyWeight, min(MaxUpstreamKeyWeight, key.Weight))
}

func (key *UpstreamKey) GetModels() []string {
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

func (key *UpstreamKey) IsRoutable(now time.Time) bool {
	if key.Status != UpstreamKeyStatusEnabled {
		return false
	}
	if key.MissingSince != 0 {
		return false
	}
	if key.ExpiresAt != nil && !key.ExpiresAt.After(now) {
		return false
	}
	return key.RemainQuota == nil || *key.RemainQuota > 0
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
	var account PlatformSiteAccount
	if err := DB.Select("sync_status", "disabled_at").
		Where("channel_id = ?", channelID).
		First(&account).Error; err != nil {
		return nil, err
	}
	if account.SyncStatus != UpstreamSiteSyncSuccess || account.DisabledAt != 0 {
		return nil, errors.New("platform site is not routable")
	}

	var key UpstreamKey
	if err := DB.Where("id = ? AND channel_id = ?", keyID, channelID).First(&key).Error; err != nil {
		return nil, err
	}
	if !key.IsRoutable(now) {
		return nil, errors.New("upstream key is not routable")
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

func DeleteUpstreamData(tx *gorm.DB, channelIDs []int) error {
	if tx == nil || len(channelIDs) == 0 {
		return nil
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
	if !tx.Migrator().HasTable(&PlatformSiteAccount{}) {
		return nil
	}
	return tx.Where("channel_id IN ?", channelIDs).Delete(&PlatformSiteAccount{}).Error
}
