package model

import (
	"fmt"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"gorm.io/gorm"
)

const (
	AccountPoolAuthTypeAPIKey         = "api_key"
	AccountPoolAuthTypeOfficialOAuth  = "official_oauth"
	AccountPoolAuthTypeCookie         = "cookie"
	AccountPoolAuthTypeServiceAccount = "service_account"
	AccountPoolAuthTypeCustomJSON     = "custom_json"

	AccountPoolStrategyRoundRobin = "round_robin"
	AccountPoolStrategyWeighted   = "weighted"
	AccountPoolStrategyFillFirst  = "fill_first"
	AccountPoolStrategyLeastUsed  = "least_used"
)

type AccountPoolGroup struct {
	Id           int     `json:"id"`
	Name         string  `json:"name" gorm:"type:varchar(255);index;not null"`
	Platform     string  `json:"platform" gorm:"type:varchar(64);index;not null"`
	AuthType     string  `json:"auth_type" gorm:"type:varchar(64);index;not null"`
	Status       int     `json:"status" gorm:"default:1;index"`
	Strategy     string  `json:"strategy" gorm:"type:varchar(64);default:'round_robin'"`
	Models       string  `json:"models" gorm:"type:text"`
	Group        string  `json:"group" gorm:"column:group;type:varchar(255);index"`
	ModelMapping *string `json:"model_mapping" gorm:"type:text"`
	Settings     string  `json:"settings" gorm:"type:text"`
	CreatedTime  int64   `json:"created_time" gorm:"bigint"`
	UpdatedTime  int64   `json:"updated_time" gorm:"bigint"`

	Stats map[string]int64 `json:"stats,omitempty" gorm:"-"`
}

type PoolAccount struct {
	Id                 int     `json:"id"`
	PoolGroupId        int     `json:"pool_group_id" gorm:"index;not null"`
	Name               string  `json:"name" gorm:"type:varchar(255);index;not null"`
	Platform           string  `json:"platform" gorm:"type:varchar(64);index;not null"`
	AuthType           string  `json:"auth_type" gorm:"type:varchar(64);index;not null"`
	Credentials        string  `json:"credentials" gorm:"type:text;not null"`
	CredentialSummary  string  `json:"credential_summary" gorm:"type:text"`
	CredentialProvider string  `json:"credential_provider" gorm:"type:varchar(64);index"`
	CredentialLabel    string  `json:"credential_label" gorm:"type:varchar(255)"`
	CredentialMetadata string  `json:"credential_metadata" gorm:"type:text"`
	CredentialAttrs    string  `json:"credential_attributes" gorm:"column:credential_attributes;type:text"`
	Status             int     `json:"status" gorm:"default:1;index"`
	StatusMessage      string  `json:"status_message" gorm:"type:text"`
	Schedulable        bool    `json:"schedulable" gorm:"default:true;index"`
	Unavailable        bool    `json:"unavailable" gorm:"default:false;index"`
	Models             string  `json:"models" gorm:"type:text"`
	Group              string  `json:"group" gorm:"column:group;type:varchar(255);index"`
	Priority           int64   `json:"priority" gorm:"bigint;default:0;index"`
	Weight             int     `json:"weight" gorm:"default:1;index"`
	MaxConcurrency     int     `json:"max_concurrency" gorm:"default:0"`
	Proxy              string  `json:"proxy" gorm:"type:text"`
	BaseURL            *string `json:"base_url" gorm:"column:base_url;default:''"`
	OpenAIOrganization *string `json:"openai_organization"`
	Other              string  `json:"other"`
	Setting            *string `json:"setting" gorm:"type:text"`
	OtherSettings      string  `json:"settings" gorm:"column:settings;type:text"`
	ModelMapping       *string `json:"model_mapping" gorm:"type:text"`
	ParamOverride      *string `json:"param_override" gorm:"type:text"`
	HeaderOverride     *string `json:"header_override" gorm:"type:text"`
	StatusCodeMapping  *string `json:"status_code_mapping" gorm:"type:varchar(1024);default:''"`
	LastUsedTime       int64   `json:"last_used_time" gorm:"bigint;default:0;index"`
	UsedQuota          int64   `json:"used_quota" gorm:"bigint;default:0"`
	RateLimitedUntil   int64   `json:"rate_limited_until" gorm:"bigint;default:0;index"`
	OverloadUntil      int64   `json:"overload_until" gorm:"bigint;default:0;index"`
	TempDisabledUntil  int64   `json:"temp_disabled_until" gorm:"bigint;default:0;index"`
	DisabledReason     string  `json:"disabled_reason" gorm:"type:text"`
	LastError          string  `json:"last_error" gorm:"type:text"`
	QuotaSnapshot      string  `json:"quota_snapshot" gorm:"type:text"`
	ModelStates        string  `json:"model_states" gorm:"type:text"`
	LastRefreshedTime  int64   `json:"last_refreshed_time" gorm:"bigint;default:0;index"`
	NextRefreshTime    int64   `json:"next_refresh_time" gorm:"bigint;default:0;index"`
	NextRetryTime      int64   `json:"next_retry_time" gorm:"bigint;default:0;index"`
	SuccessCount       int64   `json:"success_count" gorm:"bigint;default:0"`
	FailedCount        int64   `json:"failed_count" gorm:"bigint;default:0"`
	RecentRequests     string  `json:"recent_requests" gorm:"type:text"`
	CreatedTime        int64   `json:"created_time" gorm:"bigint"`
	UpdatedTime        int64   `json:"updated_time" gorm:"bigint"`
}

func (group *AccountPoolGroup) BeforeCreate(tx *gorm.DB) error {
	_ = tx
	now := common.GetTimestamp()
	if group.CreatedTime == 0 {
		group.CreatedTime = now
	}
	group.UpdatedTime = now
	group.normalize()
	return nil
}

func (group *AccountPoolGroup) BeforeUpdate(tx *gorm.DB) error {
	_ = tx
	group.UpdatedTime = common.GetTimestamp()
	group.normalize()
	return nil
}

func (group *AccountPoolGroup) normalize() {
	if group.Status == 0 {
		group.Status = common.ChannelStatusEnabled
	}
	group.Platform = strings.ToLower(strings.TrimSpace(group.Platform))
	group.AuthType = strings.ToLower(strings.TrimSpace(group.AuthType))
	if group.AuthType == "" {
		group.AuthType = AccountPoolAuthTypeAPIKey
	}
	group.Strategy = strings.ToLower(strings.TrimSpace(group.Strategy))
	if group.Strategy == "" {
		group.Strategy = AccountPoolStrategyRoundRobin
	}
}

func (account *PoolAccount) BeforeCreate(tx *gorm.DB) error {
	_ = tx
	now := common.GetTimestamp()
	if account.CreatedTime == 0 {
		account.CreatedTime = now
	}
	account.UpdatedTime = now
	account.normalize()
	return nil
}

func (account *PoolAccount) BeforeUpdate(tx *gorm.DB) error {
	_ = tx
	account.UpdatedTime = common.GetTimestamp()
	account.normalize()
	return nil
}

func (account *PoolAccount) normalize() {
	if account.Status == 0 {
		account.Status = common.ChannelStatusEnabled
	}
	if account.Weight <= 0 {
		account.Weight = 1
	}
	account.Platform = strings.ToLower(strings.TrimSpace(account.Platform))
	account.AuthType = strings.ToLower(strings.TrimSpace(account.AuthType))
	if account.AuthType == "" {
		account.AuthType = AccountPoolAuthTypeAPIKey
	}
	account.CredentialProvider = strings.ToLower(strings.TrimSpace(account.CredentialProvider))
	if account.CredentialProvider == "" {
		account.CredentialProvider = account.Platform
	}
	account.CredentialLabel = strings.TrimSpace(account.CredentialLabel)
}

func (account *PoolAccount) GetWeight() int {
	if account == nil || account.Weight <= 0 {
		return 1
	}
	return account.Weight
}

func (account *PoolAccount) IsCoolingDown(now int64) bool {
	if account == nil {
		return true
	}
	return account.RateLimitedUntil > now || account.OverloadUntil > now || account.TempDisabledUntil > now || account.NextRetryTime > now
}

func (account *PoolAccount) GetDecryptedCredentials() (string, error) {
	if account == nil {
		return "", nil
	}
	return common.DecryptSensitiveString(account.Credentials)
}

func (account *PoolAccount) GetCredentialProvider() string {
	if account == nil {
		return ""
	}
	provider := strings.TrimSpace(account.CredentialProvider)
	if provider == "" {
		provider = account.Platform
	}
	return strings.ToLower(strings.TrimSpace(provider))
}

func (account *PoolAccount) GetCredentialLabel() string {
	if account == nil {
		return ""
	}
	label := strings.TrimSpace(account.CredentialLabel)
	if label == "" {
		label = strings.TrimSpace(account.Name)
	}
	return label
}

func (account *PoolAccount) GetBaseURL(defaultBaseURL string) string {
	if account != nil && account.BaseURL != nil && strings.TrimSpace(*account.BaseURL) != "" {
		return *account.BaseURL
	}
	return defaultBaseURL
}

func (account *PoolAccount) GetModelMapping(defaultMapping string) string {
	if account != nil && account.ModelMapping != nil && strings.TrimSpace(*account.ModelMapping) != "" {
		return *account.ModelMapping
	}
	return defaultMapping
}

func (account *PoolAccount) GetStatusCodeMapping(defaultMapping string) string {
	if account != nil && account.StatusCodeMapping != nil && strings.TrimSpace(*account.StatusCodeMapping) != "" {
		return *account.StatusCodeMapping
	}
	return defaultMapping
}

func (account *PoolAccount) GetSetting(defaultSetting string) string {
	if account != nil && account.Setting != nil && strings.TrimSpace(*account.Setting) != "" {
		return *account.Setting
	}
	return defaultSetting
}

func (account *PoolAccount) GetOtherSettings(defaultSettings string) string {
	if account != nil && strings.TrimSpace(account.OtherSettings) != "" {
		return account.OtherSettings
	}
	return defaultSettings
}

func (account *PoolAccount) GetParamOverride(defaultOverride *string) *string {
	if account != nil && account.ParamOverride != nil && strings.TrimSpace(*account.ParamOverride) != "" {
		return account.ParamOverride
	}
	return defaultOverride
}

func (account *PoolAccount) GetHeaderOverride(defaultOverride *string) *string {
	if account != nil && account.HeaderOverride != nil && strings.TrimSpace(*account.HeaderOverride) != "" {
		return account.HeaderOverride
	}
	return defaultOverride
}

func NormalizeAccountPoolCredentialSummary(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	summary := map[string]string{}
	var obj map[string]interface{}
	if err := common.UnmarshalJsonStr(raw, &obj); err == nil && len(obj) > 0 {
		for _, key := range []string{"api_key", "access_token", "refresh_token", "account_id", "email", "project_id", "organization_id"} {
			if value, ok := obj[key]; ok && value != nil {
				text := strings.TrimSpace(fmt.Sprintf("%v", value))
				if text == "" {
					continue
				}
				if strings.Contains(key, "token") || strings.Contains(key, "key") {
					text = MaskTokenKey(text)
				}
				summary[key] = text
			}
		}
		if len(summary) > 0 {
			data, err := common.Marshal(summary)
			if err == nil {
				return string(data)
			}
		}
	}
	return MaskTokenKey(raw)
}

func GetAccountPoolGroupById(groupID int) (*AccountPoolGroup, error) {
	group := &AccountPoolGroup{}
	err := DB.Where("id = ?", groupID).First(group).Error
	return group, err
}

func GetAccountPoolGroups(page int, pageSize int, status int, search string) ([]*AccountPoolGroup, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	query := DB.Model(&AccountPoolGroup{})
	if status > 0 {
		query = query.Where("status = ?", status)
	}
	if strings.TrimSpace(search) != "" {
		like := "%" + strings.TrimSpace(search) + "%"
		query = query.Where("name LIKE ? OR platform LIKE ? OR auth_type LIKE ? OR models LIKE ?", like, like, like, like)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	groups := []*AccountPoolGroup{}
	err := query.Order("id DESC").Limit(pageSize).Offset((page - 1) * pageSize).Find(&groups).Error
	return groups, total, err
}

func AttachAccountPoolGroupStats(groups []*AccountPoolGroup) {
	groupIDs := make([]int, 0, len(groups))
	for _, group := range groups {
		if group != nil {
			groupIDs = append(groupIDs, group.Id)
		}
	}
	stats, err := CountPoolAccountsByGroupIDs(groupIDs)
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to count account pool group stats: %v", err))
		return
	}
	for _, group := range groups {
		if group == nil {
			continue
		}
		group.Stats = stats[group.Id]
	}
}

func CountPoolAccountsByGroupIDs(groupIDs []int) (map[int]map[string]int64, error) {
	result := make(map[int]map[string]int64)
	uniqueIDs := make([]int, 0, len(groupIDs))
	seen := map[int]bool{}
	for _, id := range groupIDs {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		uniqueIDs = append(uniqueIDs, id)
		result[id] = newPoolAccountStats()
	}
	if len(uniqueIDs) == 0 {
		return result, nil
	}
	now := common.GetTimestamp()
	var accounts []PoolAccount
	if err := DB.Select("pool_group_id", "status", "schedulable", "unavailable", "rate_limited_until", "overload_until", "temp_disabled_until", "next_retry_time").Where("pool_group_id IN ?", uniqueIDs).Find(&accounts).Error; err != nil {
		return result, err
	}
	for _, account := range accounts {
		stats := result[account.PoolGroupId]
		if stats == nil {
			stats = newPoolAccountStats()
			result[account.PoolGroupId] = stats
		}
		stats["total"]++
		if account.Status == common.ChannelStatusEnabled && account.Schedulable && !account.Unavailable {
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

func newPoolAccountStats() map[string]int64 {
	return map[string]int64{
		"total":    0,
		"enabled":  0,
		"disabled": 0,
		"cooldown": 0,
	}
}

func GetPoolAccountById(accountID int) (*PoolAccount, error) {
	account := &PoolAccount{}
	err := DB.Where("id = ?", accountID).First(account).Error
	return account, err
}

func GetPoolAccounts(groupID int, page int, pageSize int, status int, search string) ([]*PoolAccount, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	query := DB.Model(&PoolAccount{}).Where("pool_group_id = ?", groupID)
	if status > 0 {
		query = query.Where("status = ?", status)
	}
	if strings.TrimSpace(search) != "" {
		like := "%" + strings.TrimSpace(search) + "%"
		query = query.Where("name LIKE ? OR credential_summary LIKE ? OR models LIKE ?", like, like, like)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	accounts := []*PoolAccount{}
	err := query.Order("priority DESC").Order("id DESC").Limit(pageSize).Offset((page - 1) * pageSize).Find(&accounts).Error
	return accounts, total, err
}

func UpdatePoolAccountStatus(accountID int, status int, reason string, schedulable *bool) error {
	updates := map[string]interface{}{
		"status":          status,
		"disabled_reason": reason,
		"status_message":  reason,
	}
	if status == common.ChannelStatusEnabled {
		updates["rate_limited_until"] = 0
		updates["overload_until"] = 0
		updates["temp_disabled_until"] = 0
		updates["next_retry_time"] = 0
		updates["unavailable"] = false
		updates["last_error"] = ""
		updates["status_message"] = ""
		if schedulable == nil {
			updates["schedulable"] = true
		}
	}
	if schedulable != nil {
		updates["schedulable"] = *schedulable
	}
	return DB.Model(&PoolAccount{}).Where("id = ?", accountID).Updates(updates).Error
}

func UpdatePoolAccountErrorState(accountID int, updates map[string]interface{}) error {
	if accountID <= 0 || len(updates) == 0 {
		return nil
	}
	return DB.Model(&PoolAccount{}).Where("id = ?", accountID).Updates(updates).Error
}

func TouchPoolAccount(accountID int) {
	if accountID <= 0 {
		return
	}
	if err := DB.Model(&PoolAccount{}).Where("id = ?", accountID).Update("last_used_time", common.GetTimestamp()).Error; err != nil {
		common.SysLog(fmt.Sprintf("failed to update pool account last_used_time: account_id=%d, error=%v", accountID, err))
	}
}

func AddPoolAccountUsedQuota(accountID int, quota int64) {
	if accountID <= 0 || quota <= 0 {
		return
	}
	if err := DB.Model(&PoolAccount{}).Where("id = ?", accountID).Update("used_quota", gorm.Expr("used_quota + ?", quota)).Error; err != nil {
		common.SysLog(fmt.Sprintf("failed to update pool account used_quota: account_id=%d, quota=%d, error=%v", accountID, quota, err))
	}
}

func RecordPoolAccountRequest(accountID int, success bool, recentRequests string) {
	if accountID <= 0 {
		return
	}
	updates := map[string]interface{}{
		"recent_requests": recentRequests,
	}
	if success {
		updates["success_count"] = gorm.Expr("success_count + ?", 1)
		updates["unavailable"] = false
		updates["status_message"] = ""
	} else {
		updates["failed_count"] = gorm.Expr("failed_count + ?", 1)
	}
	if err := DB.Model(&PoolAccount{}).Where("id = ?", accountID).Updates(updates).Error; err != nil {
		common.SysLog(fmt.Sprintf("failed to update pool account request runtime: account_id=%d, error=%v", accountID, err))
	}
}
