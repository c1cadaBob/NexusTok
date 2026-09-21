package model

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/c1cadaBob/NexusTok/common"
	"github.com/c1cadaBob/NexusTok/logger"

	"gorm.io/gorm"
)

const (
	RoutingKeySourceKeyChannel   = UpstreamKindKeyChannel
	RoutingKeySourcePlatformSite = UpstreamKindPlatformSite

	MinRoutingKeyPriority int64 = 0
	MaxRoutingKeyPriority int64 = 99
)

type RoutingKey struct {
	ID          uint   `json:"id" gorm:"primaryKey"`
	ChannelID   int    `json:"channel_id" gorm:"not null;index"`
	Source      string `json:"source" gorm:"type:varchar(32);not null;index"`
	SourceRefID uint   `json:"source_ref_id" gorm:"index"`
}

type ChannelKey struct {
	ID                uint    `json:"id" gorm:"primaryKey"`
	RoutingKeyID      uint    `json:"routing_key_id" gorm:"not null;index"`
	ChannelID         int     `json:"channel_id" gorm:"not null;index"`
	KeyIndex          int     `json:"key_index" gorm:"not null;index"`
	SecretCiphertext  string  `json:"-" gorm:"type:text;not null"`
	SecretFingerprint string  `json:"secret_fingerprint" gorm:"type:varchar(128);index"`
	Status            int     `json:"status" gorm:"index"`
	DisabledReason    string  `json:"disabled_reason" gorm:"type:varchar(255)"`
	DisabledTime      int64   `json:"disabled_time" gorm:"bigint"`
	KeyPriority       int64   `json:"key_priority" gorm:"bigint;index"`
	ConversionRatio   float64 `json:"conversion_ratio"`
	Weight            int     `json:"weight" gorm:"index"`
	WeightOverride    *int    `json:"weight_override" gorm:"index"`
	LastUsedAt        int64   `json:"last_used_at" gorm:"bigint;index"`
	Secret            string  `json:"-" gorm:"-"`
}

type RoutingKeySelection struct {
	KeyID             uint
	ChannelID         int
	Source            string
	SourceRefID       uint
	Secret            string
	Name              string
	KeyPriority       int64
	EffectivePriority int64
	Weight            int
	AutoWeight        int
	ConversionRatio   float64
	ChannelKeyID      uint
	UpstreamKeyID     uint
	MultiKeyIndex     int
}

func ValidateRoutingKeyPriority(priority int64) error {
	if priority < MinRoutingKeyPriority || priority > MaxRoutingKeyPriority {
		return fmt.Errorf("密钥优先级必须在 %d 到 %d 之间", MinRoutingKeyPriority, MaxRoutingKeyPriority)
	}
	return nil
}

func normalizeRoutingKeyPriority(priority int64) int64 {
	return max(MinRoutingKeyPriority, min(MaxRoutingKeyPriority, priority))
}

func validChannelKeyConversionRatio(ratio float64) bool {
	return !math.IsNaN(ratio) && !math.IsInf(ratio, 0) &&
		ratio >= 0 && ratio <= MaxUpstreamConversionRatio
}

func (key *ChannelKey) AutoWeight() int {
	if key == nil {
		return MinUpstreamKeyWeight
	}
	if key.ConversionRatio == 0 {
		return MaxUpstreamKeyWeight
	}
	weight, err := CalculateUpstreamKeyWeight(key.ConversionRatio)
	if err != nil {
		return MinUpstreamKeyWeight
	}
	return weight
}

func (key *ChannelKey) EffectiveWeight() int {
	if key == nil {
		return MinUpstreamKeyWeight
	}
	if key.ConversionRatio == 0 {
		return MaxUpstreamKeyWeight
	}
	if key.WeightOverride != nil {
		return max(MinUpstreamKeyWeight, min(MaxUpstreamKeyWeight, *key.WeightOverride))
	}
	if key.Weight > 0 {
		return max(MinUpstreamKeyWeight, min(MaxUpstreamKeyWeight, key.Weight))
	}
	return key.AutoWeight()
}

func (key *ChannelKey) LoadSecret() error {
	if key == nil {
		return errors.New("channel key is nil")
	}
	plaintext, err := common.DecryptUpstreamCredential(key.SecretCiphertext)
	if err != nil {
		return err
	}
	if strings.TrimSpace(plaintext) == "" {
		return errors.New("channel key secret unavailable")
	}
	key.Secret = plaintext
	return nil
}

func newRoutingKey(tx *gorm.DB, channelID int, source string, sourceRefID uint) (*RoutingKey, error) {
	if tx == nil {
		tx = DB
	}
	routingKey := &RoutingKey{
		ChannelID:   channelID,
		Source:      source,
		SourceRefID: sourceRefID,
	}
	if err := tx.Create(routingKey).Error; err != nil {
		return nil, err
	}
	return routingKey, nil
}

func EnsureRoutingKeyForUpstreamKey(tx *gorm.DB, key *UpstreamKey) error {
	if key == nil {
		return nil
	}
	if tx == nil {
		tx = DB
	}
	if tx == nil || !tx.Migrator().HasTable(&RoutingKey{}) {
		return nil
	}
	if key.RoutingKeyID != 0 {
		return nil
	}
	var routingKey RoutingKey
	err := tx.Where("source = ? AND source_ref_id = ?", RoutingKeySourcePlatformSite, key.ID).
		First(&routingKey).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		created, createErr := newRoutingKey(tx, key.ChannelID, RoutingKeySourcePlatformSite, key.ID)
		if createErr != nil {
			return createErr
		}
		routingKey = *created
	} else if err != nil {
		return err
	}
	key.RoutingKeyID = routingKey.ID
	return tx.Model(&UpstreamKey{}).Where("id = ?", key.ID).Update("routing_key_id", routingKey.ID).Error
}

func createChannelKey(tx *gorm.DB, channel *Channel, secret string, keyIndex int, status int, reason string, disabledTime int64) (*ChannelKey, error) {
	if tx == nil {
		tx = DB
	}
	if channel == nil {
		return nil, errors.New("channel is nil")
	}
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil, nil
	}
	if status == 0 {
		status = common.ChannelStatusEnabled
	}
	routingKey, err := newRoutingKey(tx, channel.Id, RoutingKeySourceKeyChannel, 0)
	if err != nil {
		return nil, err
	}
	ciphertext, err := common.EncryptUpstreamCredential(secret)
	if err != nil {
		return nil, err
	}
	ratio := channel.ConversionRatio
	if !validChannelKeyConversionRatio(ratio) {
		ratio = 1
	}
	key := &ChannelKey{
		RoutingKeyID:      routingKey.ID,
		ChannelID:         channel.Id,
		KeyIndex:          keyIndex,
		SecretCiphertext:  ciphertext,
		SecretFingerprint: common.GenerateHMAC(secret),
		Status:            status,
		DisabledReason:    reason,
		DisabledTime:      disabledTime,
		KeyPriority:       normalizeRoutingKeyPriority(channel.KeyPriority),
		ConversionRatio:   ratio,
	}
	if channel.KeyWeightOverride != nil && ratio != 0 {
		override := max(MinUpstreamKeyWeight, min(MaxUpstreamKeyWeight, *channel.KeyWeightOverride))
		key.WeightOverride = &override
	}
	key.Weight = key.AutoWeight()
	if err := tx.Create(key).Error; err != nil {
		return nil, err
	}
	if err := tx.Model(routingKey).Update("source_ref_id", key.ID).Error; err != nil {
		return nil, err
	}
	return key, nil
}

func channelKeyPlaintextsFromField(channel *Channel) []string {
	if channel == nil {
		return nil
	}
	keys := channel.GetKeys()
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key != "" {
			result = append(result, key)
		}
	}
	return result
}

func PlaintextKeysFromChannelField(channel *Channel) []string {
	return channelKeyPlaintextsFromField(channel)
}

func ReplaceChannelKeysFromPlaintext(tx *gorm.DB, channel *Channel, secrets []string) error {
	if tx == nil {
		tx = DB
	}
	if channel == nil {
		return errors.New("channel is nil")
	}
	return tx.Transaction(func(tx *gorm.DB) error {
		var existing []ChannelKey
		if err := tx.Where("channel_id = ?", channel.Id).Find(&existing).Error; err != nil {
			return err
		}
		routingIDs := make([]uint, 0, len(existing))
		for _, key := range existing {
			routingIDs = append(routingIDs, key.RoutingKeyID)
		}
		if len(existing) > 0 {
			if err := tx.Where("channel_id = ?", channel.Id).Delete(&ChannelKey{}).Error; err != nil {
				return err
			}
		}
		if len(routingIDs) > 0 {
			if tx.Migrator().HasTable(&RoutingKeyHealth{}) {
				if err := tx.Where("routing_key_id IN ?", routingIDs).Delete(&RoutingKeyHealth{}).Error; err != nil {
					return err
				}
			}
			if err := tx.Where("id IN ?", routingIDs).Delete(&RoutingKey{}).Error; err != nil {
				return err
			}
		}
		for index, secret := range secrets {
			if _, err := createChannelKey(tx, channel, secret, index, common.ChannelStatusEnabled, "", 0); err != nil {
				return err
			}
		}
		return updateChannelKeySummary(tx, channel.Id)
	})
}

func AppendChannelKeysFromPlaintext(tx *gorm.DB, channel *Channel, secrets []string) error {
	if tx == nil {
		tx = DB
	}
	if channel == nil {
		return errors.New("channel is nil")
	}
	return tx.Transaction(func(tx *gorm.DB) error {
		existing, err := LoadChannelKeys(tx, channel.Id, false)
		if err != nil {
			return err
		}
		seen := make(map[string]struct{}, len(existing)+len(secrets))
		maxIndex := -1
		for index := range existing {
			key := &existing[index]
			if key.SecretFingerprint != "" {
				seen[key.SecretFingerprint] = struct{}{}
			}
			if key.KeyIndex > maxIndex {
				maxIndex = key.KeyIndex
			}
		}
		nextIndex := maxIndex + 1
		for _, secret := range secrets {
			secret = strings.TrimSpace(secret)
			if secret == "" {
				continue
			}
			fingerprint := common.GenerateHMAC(secret)
			if _, duplicate := seen[fingerprint]; duplicate {
				continue
			}
			seen[fingerprint] = struct{}{}
			if _, err := createChannelKey(tx, channel, secret, nextIndex, common.ChannelStatusEnabled, "", 0); err != nil {
				return err
			}
			nextIndex++
		}
		return updateChannelKeySummary(tx, channel.Id)
	})
}

func SyncChannelKeysFromLegacyField(tx *gorm.DB, channel *Channel) error {
	if tx == nil {
		tx = DB
	}
	if channel == nil || channel.UpstreamKind == UpstreamKindPlatformSite || strings.TrimSpace(channel.Key) == "" {
		return nil
	}
	if !tx.Migrator().HasTable(&ChannelKey{}) || !tx.Migrator().HasTable(&RoutingKey{}) {
		return nil
	}
	var count int64
	if err := tx.Model(&ChannelKey{}).Where("channel_id = ?", channel.Id).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return tx.Model(&Channel{}).Where("id = ?", channel.Id).Update("key", "").Error
	}
	keys := channelKeyPlaintextsFromField(channel)
	for index, key := range keys {
		status := common.ChannelStatusEnabled
		reason := ""
		disabledTime := int64(0)
		if channel.ChannelInfo.MultiKeyStatusList != nil {
			if savedStatus, ok := channel.ChannelInfo.MultiKeyStatusList[index]; ok {
				status = savedStatus
			}
		}
		if status != common.ChannelStatusEnabled {
			if channel.ChannelInfo.MultiKeyDisabledReason != nil {
				reason = channel.ChannelInfo.MultiKeyDisabledReason[index]
			}
			if channel.ChannelInfo.MultiKeyDisabledTime != nil {
				disabledTime = channel.ChannelInfo.MultiKeyDisabledTime[index]
			}
		}
		if _, err := createChannelKey(tx, channel, key, index, status, reason, disabledTime); err != nil {
			return err
		}
	}
	if err := updateChannelKeySummary(tx, channel.Id); err != nil {
		return err
	}
	return tx.Model(&Channel{}).Where("id = ?", channel.Id).Update("key", "").Error
}

func LoadChannelKeys(tx *gorm.DB, channelID int, includeSecret bool) ([]ChannelKey, error) {
	if tx == nil {
		tx = DB
	}
	if tx == nil || !tx.Migrator().HasTable(&ChannelKey{}) {
		return nil, nil
	}
	var keys []ChannelKey
	if err := tx.Where("channel_id = ?", channelID).Order("key_index ASC, id ASC").Find(&keys).Error; err != nil {
		return nil, err
	}
	if includeSecret {
		for index := range keys {
			if err := keys[index].LoadSecret(); err != nil {
				return nil, err
			}
		}
	}
	return keys, nil
}

func CountChannelKeys(channelID int) (int, error) {
	if DB == nil || !DB.Migrator().HasTable(&ChannelKey{}) {
		return 0, nil
	}
	var count int64
	if err := DB.Model(&ChannelKey{}).Where("channel_id = ?", channelID).Count(&count).Error; err != nil {
		return 0, err
	}
	return int(count), nil
}

func updateChannelKeySummary(tx *gorm.DB, channelID int) error {
	if tx == nil {
		tx = DB
	}
	var keys []ChannelKey
	if err := tx.Where("channel_id = ?", channelID).Order("key_index ASC, id ASC").Find(&keys).Error; err != nil {
		return err
	}
	info := ChannelInfo{}
	var existing Channel
	if err := tx.Select("channel_info").First(&existing, "id = ?", channelID).Error; err == nil {
		info = existing.ChannelInfo
	}
	info.MultiKeySize = len(keys)
	if len(keys) > 1 {
		info.IsMultiKey = true
	}
	if len(keys) == 0 {
		info.IsMultiKey = false
	}
	info.MultiKeyStatusList = make(map[int]int)
	info.MultiKeyDisabledReason = make(map[int]string)
	info.MultiKeyDisabledTime = make(map[int]int64)
	for _, key := range keys {
		if key.Status != common.ChannelStatusEnabled {
			info.MultiKeyStatusList[key.KeyIndex] = key.Status
			if key.DisabledReason != "" {
				info.MultiKeyDisabledReason[key.KeyIndex] = key.DisabledReason
			}
			if key.DisabledTime > 0 {
				info.MultiKeyDisabledTime[key.KeyIndex] = key.DisabledTime
			}
		}
	}
	return tx.Model(&Channel{}).
		Where("id = ?", channelID).
		Updates(map[string]any{"channel_info": info, "key": ""}).Error
}

func ReindexChannelKeys(tx *gorm.DB, channelID int) error {
	keys, err := LoadChannelKeys(tx, channelID, false)
	if err != nil {
		return err
	}
	for index := range keys {
		if keys[index].KeyIndex == index {
			continue
		}
		if err := tx.Model(&keys[index]).Update("key_index", index).Error; err != nil {
			return err
		}
	}
	return updateChannelKeySummary(tx, channelID)
}

func DeleteChannelKey(tx *gorm.DB, channelID int, routingKeyID uint) error {
	if tx == nil {
		tx = DB
	}
	return tx.Transaction(func(tx *gorm.DB) error {
		var key ChannelKey
		if err := tx.Where("channel_id = ? AND routing_key_id = ?", channelID, routingKeyID).First(&key).Error; err != nil {
			return err
		}
		if err := tx.Delete(&key).Error; err != nil {
			return err
		}
		if tx.Migrator().HasTable(&RoutingKeyHealth{}) {
			if err := tx.Where("routing_key_id = ?", routingKeyID).Delete(&RoutingKeyHealth{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("id = ?", routingKeyID).Delete(&RoutingKey{}).Error; err != nil {
			return err
		}
		return ReindexChannelKeys(tx, channelID)
	})
}

func UpdateChannelKeySecret(tx *gorm.DB, channelKeyID uint, secret string) error {
	if tx == nil {
		tx = DB
	}
	secret = strings.TrimSpace(secret)
	if channelKeyID == 0 || secret == "" {
		return nil
	}
	ciphertext, err := common.EncryptUpstreamCredential(secret)
	if err != nil {
		return err
	}
	return tx.Model(&ChannelKey{}).Where("id = ?", channelKeyID).Updates(map[string]any{
		"secret_ciphertext":  ciphertext,
		"secret_fingerprint": common.GenerateHMAC(secret),
	}).Error
}

func UpdateRoutingKeyStatus(channelID int, routingKeyID uint, status int, reason string) bool {
	if routingKeyID == 0 {
		return false
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		var routingKey RoutingKey
		if err := tx.Where("id = ? AND channel_id = ?", routingKeyID, channelID).First(&routingKey).Error; err != nil {
			return err
		}
		now := common.GetTimestamp()
		switch routingKey.Source {
		case RoutingKeySourceKeyChannel:
			updates := map[string]any{
				"status": status,
			}
			if status == common.ChannelStatusEnabled {
				updates["disabled_reason"] = ""
				updates["disabled_time"] = int64(0)
			} else {
				updates["disabled_reason"] = reason
				updates["disabled_time"] = now
			}
			if err := tx.Model(&ChannelKey{}).
				Where("routing_key_id = ? AND channel_id = ?", routingKeyID, channelID).
				Updates(updates).Error; err != nil {
				return err
			}
			if err := updateChannelKeySummary(tx, channelID); err != nil {
				return err
			}
			return updateKeyChannelAvailability(tx, channelID, status, reason)
		case RoutingKeySourcePlatformSite:
			updates := map[string]any{
				"status": status,
			}
			if status == common.ChannelStatusEnabled {
				updates["disabled_reason"] = ""
			} else {
				updates["disabled_reason"] = reason
			}
			return tx.Model(&UpstreamKey{}).
				Where("routing_key_id = ? AND channel_id = ?", routingKeyID, channelID).
				Updates(updates).Error
		default:
			return fmt.Errorf("unsupported routing key source %s", routingKey.Source)
		}
	})
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to update routing key status: channel_id=%d key_id=%d status=%d error=%v", channelID, routingKeyID, status, err))
		return false
	}
	return true
}

func updateKeyChannelAvailability(tx *gorm.DB, channelID int, status int, reason string) error {
	var keys []ChannelKey
	if err := tx.Where("channel_id = ?", channelID).Find(&keys).Error; err != nil {
		return err
	}
	if len(keys) == 0 {
		return nil
	}
	hasEnabled := false
	for _, key := range keys {
		if key.Status == common.ChannelStatusEnabled {
			hasEnabled = true
			break
		}
	}
	channelStatus := common.ChannelStatusEnabled
	statusReason := ""
	if !hasEnabled {
		channelStatus = common.ChannelStatusAutoDisabled
		statusReason = "All keys are disabled"
	} else if status != common.ChannelStatusEnabled {
		return nil
	}
	var channel Channel
	if err := tx.Select("id", "status", "other_info").First(&channel, "id = ?", channelID).Error; err != nil {
		return err
	}
	if channel.Status == channelStatus {
		return nil
	}
	info := channel.GetOtherInfo()
	if statusReason == "" {
		statusReason = reason
	}
	info["status_reason"] = statusReason
	info["status_time"] = common.GetTimestamp()
	channel.SetOtherInfo(info)
	if err := tx.Model(&Channel{}).Where("id = ?", channelID).Updates(map[string]any{
		"status":     channelStatus,
		"other_info": channel.OtherInfo,
	}).Error; err != nil {
		return err
	}
	return UpdateAbilityStatus(channelID, channelStatus == common.ChannelStatusEnabled)
}

func GetRoutableKeyByID(channel *Channel, routingKeyID uint, group, modelName string) (*RoutingKeySelection, error) {
	if channel == nil {
		return nil, errors.New("channel is nil")
	}
	if routingKeyID == 0 {
		return nil, errors.New("key_id is required")
	}
	var routingKey RoutingKey
	if err := DB.Where("id = ? AND channel_id = ?", routingKeyID, channel.Id).First(&routingKey).Error; err != nil {
		return nil, err
	}
	switch routingKey.Source {
	case RoutingKeySourceKeyChannel:
		var key ChannelKey
		if err := DB.Where("routing_key_id = ? AND channel_id = ?", routingKeyID, channel.Id).First(&key).Error; err != nil {
			return nil, err
		}
		if key.Status != common.ChannelStatusEnabled {
			return nil, errors.New("channel key is not routable")
		}
		if strings.TrimSpace(modelName) != "" && !channelSupportsModel(channel, modelName) {
			return nil, errors.New("channel key does not support the test model")
		}
		if err := key.LoadSecret(); err != nil {
			return nil, err
		}
		return channelKeySelection(channel, &key), nil
	case RoutingKeySourcePlatformSite:
		var key UpstreamKey
		if err := DB.Where("routing_key_id = ? AND channel_id = ?", routingKeyID, channel.Id).First(&key).Error; err != nil {
			return nil, err
		}
		selected, err := GetRoutableUpstreamKeyByID(channel.Id, key.ID, group, modelName, time.Now())
		if err != nil {
			return nil, err
		}
		return upstreamKeySelection(channel, selected), nil
	default:
		return nil, fmt.Errorf("unsupported routing key source %s", routingKey.Source)
	}
}

func GetRoutingKeyForModelFetch(channel *Channel, routingKeyID uint) (*RoutingKeySelection, error) {
	if channel == nil {
		return nil, errors.New("channel is nil")
	}
	if routingKeyID == 0 {
		return nil, errors.New("key_id is required")
	}
	var routingKey RoutingKey
	if err := DB.Where("id = ? AND channel_id = ?", routingKeyID, channel.Id).First(&routingKey).Error; err != nil {
		return nil, err
	}
	switch routingKey.Source {
	case RoutingKeySourceKeyChannel:
		var key ChannelKey
		if err := DB.Where("routing_key_id = ? AND channel_id = ?", routingKeyID, channel.Id).First(&key).Error; err != nil {
			return nil, err
		}
		if key.Status != common.ChannelStatusEnabled {
			return nil, errors.New("channel key is not enabled")
		}
		if err := key.LoadSecret(); err != nil {
			return nil, errors.New("channel key credential is unavailable")
		}
		return channelKeySelection(channel, &key), nil
	case RoutingKeySourcePlatformSite:
		var key UpstreamKey
		if err := DB.Where("routing_key_id = ? AND channel_id = ?", routingKeyID, channel.Id).First(&key).Error; err != nil {
			return nil, err
		}
		if key.Status != UpstreamKeyStatusEnabled || key.MissingSince != 0 {
			return nil, errors.New("upstream key is not available")
		}
		if key.ExpiresAt != nil && !key.ExpiresAt.After(time.Now()) {
			return nil, errors.New("upstream key is expired")
		}
		if key.RemainQuota != nil && *key.RemainQuota <= 0 {
			return nil, errors.New("upstream key quota is exhausted")
		}
		if err := key.LoadSecret(); err != nil {
			return nil, errors.New("upstream key credential is unavailable")
		}
		return upstreamKeySelection(channel, &key), nil
	default:
		return nil, fmt.Errorf("unsupported routing key source %s", routingKey.Source)
	}
}

func SelectRoutableKeyByIDForRefresh(channel *Channel, routingKeyID uint) (*Channel, error) {
	if channel == nil {
		return nil, errors.New("channel is nil")
	}
	selection, err := GetRoutableKeyByID(channel, routingKeyID, "", "")
	if err != nil {
		return nil, err
	}
	channelCopy := *channel
	selectionCopy := *selection
	channelCopy.SelectedRoutingKey = &selectionCopy
	if selectionCopy.Source == RoutingKeySourceKeyChannel {
		channelCopy.SelectedUpstreamKey = &UpstreamKey{
			RoutingKeyID:    selectionCopy.KeyID,
			ChannelID:       selectionCopy.ChannelID,
			Secret:          selectionCopy.Secret,
			KeyPriority:     selectionCopy.KeyPriority,
			ConversionRatio: selectionCopy.ConversionRatio,
			Weight:          selectionCopy.Weight,
		}
	}
	return &channelCopy, nil
}

func TouchRoutingKeyLastUsed(routingKeyID uint, unixTime int64) error {
	if routingKeyID == 0 || unixTime <= 0 {
		return nil
	}
	var routingKey RoutingKey
	if err := DB.Where("id = ?", routingKeyID).First(&routingKey).Error; err != nil {
		return err
	}
	switch routingKey.Source {
	case RoutingKeySourceKeyChannel:
		return DB.Model(&ChannelKey{}).Where("routing_key_id = ?", routingKeyID).Update("last_used_at", unixTime).Error
	case RoutingKeySourcePlatformSite:
		return DB.Model(&UpstreamKey{}).Where("routing_key_id = ?", routingKeyID).Update("last_used_at", unixTime).Error
	default:
		return nil
	}
}

func GetChannelCredential(channel *Channel) (string, error) {
	if channel == nil {
		return "", errors.New("channel is nil")
	}
	if channel.SelectedRoutingKey != nil {
		if strings.TrimSpace(channel.SelectedRoutingKey.Secret) == "" {
			return "", errors.New("selected routing key secret unavailable")
		}
		return channel.SelectedRoutingKey.Secret, nil
	}
	if channel.UpstreamKind == UpstreamKindPlatformSite {
		selected := SelectRoutableKey(channel, "", "")
		if selected != nil && selected.SelectedRoutingKey != nil && selected.SelectedRoutingKey.Secret != "" {
			return selected.SelectedRoutingKey.Secret, nil
		}
		return "", errors.New("no routable platform key available")
	}
	selected := SelectRoutableKey(channel, "", "")
	if selected != nil && selected.SelectedRoutingKey != nil && selected.SelectedRoutingKey.Secret != "" {
		return selected.SelectedRoutingKey.Secret, nil
	}
	if strings.TrimSpace(channel.Key) != "" {
		if err := SyncChannelKeysFromLegacyField(nil, channel); err == nil {
			selected = SelectRoutableKey(channel, "", "")
			if selected != nil && selected.SelectedRoutingKey != nil && selected.SelectedRoutingKey.Secret != "" {
				return selected.SelectedRoutingKey.Secret, nil
			}
		}
		keys := channelKeyPlaintextsFromField(channel)
		if len(keys) > 0 {
			return keys[0], nil
		}
	}
	return "", errors.New("no channel key available")
}

func ChannelKeyPreviews(keys []ChannelKey) []string {
	previews := make([]string, 0, len(keys))
	for index := range keys {
		key := &keys[index]
		preview := ""
		if key.Secret != "" {
			preview = MaskTokenKey(key.Secret)
		} else if key.SecretFingerprint != "" {
			preview = key.SecretFingerprint
			if len(preview) > 10 {
				preview = preview[:10] + "..."
			}
		}
		previews = append(previews, preview)
	}
	return previews
}

func sortedUniqueEffectivePriorities(candidates []routingKeyRouteCandidate) []int64 {
	unique := make(map[int64]struct{}, len(candidates))
	for _, candidate := range candidates {
		unique[candidate.key.EffectivePriority] = struct{}{}
	}
	values := make([]int64, 0, len(unique))
	for priority := range unique {
		values = append(values, priority)
	}
	sort.Slice(values, func(i, j int) bool {
		return values[i] > values[j]
	})
	return values
}

func logZeroWeightRoutingFallback(channelID int, effectivePriority int64) {
	logger.LogWarn(nil, "routing key weighted selection fallback to equal probability: channel_id=%d effective_priority=%d", channelID, effectivePriority)
}
