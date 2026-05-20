package model

import (
	"fmt"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"gorm.io/gorm"
)

type ChannelAccount struct {
	Id                int     `json:"id"`
	ChannelId         int     `json:"channel_id" gorm:"index;not null"`
	Name              string  `json:"name" gorm:"type:varchar(255);index"`
	Key               string  `json:"key" gorm:"type:text;not null"`
	Status            int     `json:"status" gorm:"default:1;index"`
	Models            string  `json:"models" gorm:"type:text"`
	Group             string  `json:"group" gorm:"column:group;type:varchar(255);index"`
	Priority          int64   `json:"priority" gorm:"bigint;default:0;index"`
	Weight            int     `json:"weight" gorm:"default:0;index"`
	LastUsedTime      int64   `json:"last_used_time" gorm:"bigint;default:0;index"`
	UsedQuota         int64   `json:"used_quota" gorm:"bigint;default:0"`
	BaseURL           *string `json:"base_url" gorm:"column:base_url;default:''"`
	OpenAIOrganization *string `json:"openai_organization"`
	Other              string  `json:"other"`
	Setting            *string `json:"setting" gorm:"type:text"`
	OtherSettings      string  `json:"settings" gorm:"column:settings;type:text"`
	ModelMapping       *string `json:"model_mapping" gorm:"type:text"`
	ParamOverride      *string `json:"param_override" gorm:"type:text"`
	HeaderOverride     *string `json:"header_override" gorm:"type:text"`
	StatusCodeMapping  *string `json:"status_code_mapping" gorm:"type:varchar(1024);default:''"`
	RateLimitedUntil   int64   `json:"rate_limited_until" gorm:"bigint;default:0;index"`
	OverloadUntil      int64   `json:"overload_until" gorm:"bigint;default:0;index"`
	TempDisabledUntil  int64   `json:"temp_disabled_until" gorm:"bigint;default:0;index"`
	DisabledReason     string  `json:"disabled_reason" gorm:"type:text"`
	LastError           string  `json:"last_error" gorm:"type:text"`
	MaxConcurrency     int     `json:"max_concurrency" gorm:"default:0"`
	CreatedTime        int64   `json:"created_time" gorm:"bigint"`
}

type ChannelAccountSortOptions struct {
	SortBy    string
	SortOrder string
}

func (account *ChannelAccount) BeforeCreate(tx *gorm.DB) error {
	_ = tx
	if account.CreatedTime == 0 {
		account.CreatedTime = common.GetTimestamp()
	}
	if account.Status == 0 {
		account.Status = common.ChannelStatusEnabled
	}
	return nil
}

func (account *ChannelAccount) GetWeight() int {
	if account == nil || account.Weight <= 0 {
		return 1
	}
	return account.Weight
}

func (account *ChannelAccount) GetMaskedKey() string {
	if account == nil || account.Key == "" {
		return ""
	}
	return MaskTokenKey(account.Key)
}

func (account *ChannelAccount) IsCoolingDown(now int64) bool {
	if account == nil {
		return true
	}
	return account.RateLimitedUntil > now || account.OverloadUntil > now || account.TempDisabledUntil > now
}

func (account *ChannelAccount) GetBaseURL(defaultBaseURL string) string {
	if account != nil && account.BaseURL != nil && strings.TrimSpace(*account.BaseURL) != "" {
		return *account.BaseURL
	}
	return defaultBaseURL
}

func (account *ChannelAccount) GetModelMapping(defaultMapping string) string {
	if account != nil && account.ModelMapping != nil && strings.TrimSpace(*account.ModelMapping) != "" {
		return *account.ModelMapping
	}
	return defaultMapping
}

func (account *ChannelAccount) GetStatusCodeMapping(defaultMapping string) string {
	if account != nil && account.StatusCodeMapping != nil && strings.TrimSpace(*account.StatusCodeMapping) != "" {
		return *account.StatusCodeMapping
	}
	return defaultMapping
}

func (account *ChannelAccount) GetSetting(defaultSetting string) string {
	if account != nil && account.Setting != nil && strings.TrimSpace(*account.Setting) != "" {
		return *account.Setting
	}
	return defaultSetting
}

func (account *ChannelAccount) GetOtherSettings(defaultSettings string) string {
	if account != nil && strings.TrimSpace(account.OtherSettings) != "" {
		return account.OtherSettings
	}
	return defaultSettings
}

func (account *ChannelAccount) GetParamOverride(defaultOverride *string) *string {
	if account != nil && account.ParamOverride != nil && strings.TrimSpace(*account.ParamOverride) != "" {
		return account.ParamOverride
	}
	return defaultOverride
}

func (account *ChannelAccount) GetHeaderOverride(defaultOverride *string) *string {
	if account != nil && account.HeaderOverride != nil && strings.TrimSpace(*account.HeaderOverride) != "" {
		return account.HeaderOverride
	}
	return defaultOverride
}

func GetChannelAccountById(channelID int, accountID int) (*ChannelAccount, error) {
	account := &ChannelAccount{}
	err := DB.Where("channel_id = ? AND id = ?", channelID, accountID).First(account).Error
	return account, err
}

func GetChannelAccounts(channelID int, page int, pageSize int, status int, search string) ([]*ChannelAccount, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	query := DB.Model(&ChannelAccount{}).Where("channel_id = ?", channelID)
	if status > 0 {
		query = query.Where("status = ?", status)
	}
	if strings.TrimSpace(search) != "" {
		like := "%" + strings.TrimSpace(search) + "%"
		query = query.Where("name LIKE ? OR models LIKE ? OR "+commonKeyCol+" LIKE ?", like, like, like)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	accounts := []*ChannelAccount{}
	err := query.Order("priority DESC").Order("id DESC").Limit(pageSize).Offset((page - 1) * pageSize).Find(&accounts).Error
	return accounts, total, err
}

func CountChannelAccountsByStatus(channelID int) (map[string]int64, error) {
	now := common.GetTimestamp()
	result := map[string]int64{
		"total":    0,
		"enabled":  0,
		"disabled": 0,
		"cooldown": 0,
	}
	var accounts []ChannelAccount
	if err := DB.Select("status", "rate_limited_until", "overload_until", "temp_disabled_until").Where("channel_id = ?", channelID).Find(&accounts).Error; err != nil {
		return result, err
	}
	result["total"] = int64(len(accounts))
	for _, account := range accounts {
		if account.Status == common.ChannelStatusEnabled {
			result["enabled"]++
			if account.IsCoolingDown(now) {
				result["cooldown"]++
			}
		} else {
			result["disabled"]++
		}
	}
	return result, nil
}

func CountChannelAccountsByChannelIDs(channelIDs []int) (map[int]map[string]int64, error) {
	result := make(map[int]map[string]int64)
	uniqueIDs := make([]int, 0, len(channelIDs))
	seen := make(map[int]bool)
	for _, channelID := range channelIDs {
		if channelID <= 0 || seen[channelID] {
			continue
		}
		seen[channelID] = true
		uniqueIDs = append(uniqueIDs, channelID)
		result[channelID] = map[string]int64{
			"total":    0,
			"enabled":  0,
			"disabled": 0,
			"cooldown": 0,
		}
	}
	if len(uniqueIDs) == 0 {
		return result, nil
	}

	now := common.GetTimestamp()
	var accounts []ChannelAccount
	if err := DB.Select("channel_id", "status", "rate_limited_until", "overload_until", "temp_disabled_until").Where("channel_id IN ?", uniqueIDs).Find(&accounts).Error; err != nil {
		return result, err
	}
	for _, account := range accounts {
		stats := result[account.ChannelId]
		if stats == nil {
			stats = map[string]int64{
				"total":    0,
				"enabled":  0,
				"disabled": 0,
				"cooldown": 0,
			}
			result[account.ChannelId] = stats
		}
		stats["total"]++
		if account.Status == common.ChannelStatusEnabled {
			stats["enabled"]++
			if account.IsCoolingDown(now) {
				stats["cooldown"]++
			}
		} else {
			stats["disabled"]++
		}
	}
	return result, nil
}

func AttachChannelAccountStats(channels []*Channel) {
	channelIDs := make([]int, 0, len(channels))
	for _, channel := range channels {
		if channel == nil {
			continue
		}
		channelIDs = append(channelIDs, channel.Id)
	}
	stats, err := CountChannelAccountsByChannelIDs(channelIDs)
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to count channel account stats: %v", err))
		return
	}
	for _, channel := range channels {
		if channel == nil {
			continue
		}
		channelStats := stats[channel.Id]
		if channelStats == nil {
			continue
		}
		if channelStats["total"] > 0 || channel.IsAccountPoolEnabled() {
			channel.ChannelAccountStats = channelStats
		}
	}
}

func UpdateChannelAccountStatus(channelID int, accountID int, status int, reason string) error {
	update := map[string]interface{}{
		"status":          status,
		"disabled_reason": reason,
	}
	if status == common.ChannelStatusEnabled {
		update["rate_limited_until"] = 0
		update["overload_until"] = 0
		update["temp_disabled_until"] = 0
		update["last_error"] = ""
	}
	return DB.Model(&ChannelAccount{}).Where("channel_id = ? AND id = ?", channelID, accountID).Updates(update).Error
}

func TouchChannelAccount(accountID int) {
	if accountID <= 0 {
		return
	}
	if err := DB.Model(&ChannelAccount{}).Where("id = ?", accountID).Update("last_used_time", common.GetTimestamp()).Error; err != nil {
		common.SysLog(fmt.Sprintf("failed to update channel account last_used_time: account_id=%d, error=%v", accountID, err))
	}
}

func AddChannelAccountUsedQuota(accountID int, quota int64) {
	if accountID <= 0 || quota == 0 {
		return
	}
	if err := DB.Model(&ChannelAccount{}).Where("id = ?", accountID).Update("used_quota", gorm.Expr("used_quota + ?", quota)).Error; err != nil {
		common.SysLog(fmt.Sprintf("failed to update channel account used_quota: account_id=%d, quota=%d, error=%v", accountID, quota, err))
	}
}

func UpdateChannelAccountErrorState(channelID int, accountID int, updates map[string]interface{}) error {
	if accountID <= 0 || len(updates) == 0 {
		return nil
	}
	query := DB.Model(&ChannelAccount{}).Where("id = ?", accountID)
	if channelID > 0 {
		query = query.Where("channel_id = ?", channelID)
	}
	return query.Updates(updates).Error
}
