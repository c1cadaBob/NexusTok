package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"

	"github.com/gin-gonic/gin"
)

type channelAccountUpsertRequest struct {
	Name              string  `json:"name"`
	Key               string  `json:"key"`
	Status            *int    `json:"status"`
	Models            string  `json:"models"`
	Group             string  `json:"group"`
	Priority          *int64  `json:"priority"`
	Weight            *int    `json:"weight"`
	BaseURL           *string `json:"base_url"`
	OpenAIOrganization *string `json:"openai_organization"`
	Other             string  `json:"other"`
	Setting           *string `json:"setting"`
	OtherSettings     string  `json:"settings"`
	ModelMapping      *string `json:"model_mapping"`
	ParamOverride     *string `json:"param_override"`
	HeaderOverride    *string `json:"header_override"`
	StatusCodeMapping *string `json:"status_code_mapping"`
	MaxConcurrency    *int    `json:"max_concurrency"`
}

type channelAccountBatchRequest struct {
	Keys           string `json:"keys"`
	NamePrefix     string `json:"name_prefix"`
	Models         string `json:"models"`
	Group          string `json:"group"`
	Priority       int64  `json:"priority"`
	Weight         int    `json:"weight"`
	Status         int    `json:"status"`
	MaxConcurrency int    `json:"max_concurrency"`
}

type channelAccountStatusRequest struct {
	Status        int    `json:"status"`
	Reason        string `json:"reason"`
	ClearCooldown bool   `json:"clear_cooldown"`
}

func ListChannelAccounts(c *gin.Context) {
	channelID, ok := parseChannelIDParam(c)
	if !ok {
		return
	}
	if !ensureChannelExists(c, channelID) {
		return
	}
	pageInfo := common.GetPageQuery(c)
	status, _ := strconv.Atoi(c.Query("status"))
	accounts, total, err := model.GetChannelAccounts(channelID, pageInfo.GetPage(), pageInfo.GetPageSize(), status, c.Query("search"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	items := make([]gin.H, 0, len(accounts))
	for _, account := range accounts {
		items = append(items, channelAccountResponse(account))
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	stats, _ := model.CountChannelAccountsByStatus(channelID)
	common.ApiSuccess(c, gin.H{
		"accounts": pageInfo,
		"stats":    stats,
	})
}

func CreateChannelAccount(c *gin.Context) {
	channelID, ok := parseChannelIDParam(c)
	if !ok {
		return
	}
	if !ensureChannelExists(c, channelID) {
		return
	}
	var req channelAccountUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	account, err := buildChannelAccountFromRequest(channelID, req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.DB.Create(account).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, channelAccountResponse(account))
}

func BatchCreateChannelAccounts(c *gin.Context) {
	channelID, ok := parseChannelIDParam(c)
	if !ok {
		return
	}
	if !ensureChannelExists(c, channelID) {
		return
	}
	var req channelAccountBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	created, skipped, err := createChannelAccountsFromKeys(channelID, req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"created": created,
		"skipped": skipped,
	})
}

func GetChannelAccount(c *gin.Context) {
	channelID, accountID, ok := parseChannelAccountParams(c)
	if !ok {
		return
	}
	account, err := model.GetChannelAccountById(channelID, accountID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, channelAccountResponse(account))
}

func UpdateChannelAccount(c *gin.Context) {
	channelID, accountID, ok := parseChannelAccountParams(c)
	if !ok {
		return
	}
	account, err := model.GetChannelAccountById(channelID, accountID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var req channelAccountUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	updates := channelAccountUpdateMap(req)
	if len(updates) == 0 {
		common.ApiSuccess(c, channelAccountResponse(account))
		return
	}
	if err := model.DB.Model(account).Updates(updates).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	updated, err := model.GetChannelAccountById(channelID, accountID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, channelAccountResponse(updated))
}

func DeleteChannelAccount(c *gin.Context) {
	channelID, accountID, ok := parseChannelAccountParams(c)
	if !ok {
		return
	}
	if err := model.DB.Where("channel_id = ? AND id = ?", channelID, accountID).Delete(&model.ChannelAccount{}).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func UpdateChannelAccountStatus(c *gin.Context) {
	channelID, accountID, ok := parseChannelAccountParams(c)
	if !ok {
		return
	}
	var req channelAccountStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if req.ClearCooldown && req.Status == 0 {
		err := model.UpdateChannelAccountErrorState(channelID, accountID, map[string]interface{}{
			"rate_limited_until":  0,
			"overload_until":      0,
			"temp_disabled_until": 0,
			"disabled_reason":     req.Reason,
		})
		if err != nil {
			common.ApiError(c, err)
			return
		}
		common.ApiSuccess(c, nil)
		return
	}
	if req.Status <= 0 {
		common.ApiErrorMsg(c, "status is required")
		return
	}
	if err := model.UpdateChannelAccountStatus(channelID, accountID, req.Status, req.Reason); err != nil {
		common.ApiError(c, err)
		return
	}
	if req.ClearCooldown {
		_ = model.UpdateChannelAccountErrorState(channelID, accountID, map[string]interface{}{
			"rate_limited_until":  0,
			"overload_until":      0,
			"temp_disabled_until": 0,
		})
	}
	common.ApiSuccess(c, nil)
}

func ImportMultiKeyToChannelAccounts(c *gin.Context) {
	channelID, ok := parseChannelIDParam(c)
	if !ok {
		return
	}
	channel, err := model.GetChannelById(channelID, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var req channelAccountBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	keys := strings.Join(channel.GetKeys(), "\n")
	if strings.TrimSpace(req.Keys) == "" {
		req.Keys = keys
	}
	if strings.TrimSpace(req.NamePrefix) == "" {
		req.NamePrefix = channel.Name
	}
	if strings.TrimSpace(req.Models) == "" {
		req.Models = channel.Models
	}
	if strings.TrimSpace(req.Group) == "" {
		req.Group = channel.Group
	}
	if req.Priority == 0 {
		req.Priority = channel.GetPriority()
	}
	if req.Weight == 0 {
		req.Weight = channel.GetWeight()
	}
	created, skipped, err := createChannelAccountsFromKeys(channelID, req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"created": created,
		"skipped": skipped,
	})
}

func parseChannelIDParam(c *gin.Context) (int, bool) {
	channelID, err := strconv.Atoi(c.Param("id"))
	if err != nil || channelID <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid channel id"})
		return 0, false
	}
	return channelID, true
}

func parseChannelAccountParams(c *gin.Context) (int, int, bool) {
	channelID, ok := parseChannelIDParam(c)
	if !ok {
		return 0, 0, false
	}
	accountID, err := strconv.Atoi(c.Param("account_id"))
	if err != nil || accountID <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid account id"})
		return 0, 0, false
	}
	return channelID, accountID, true
}

func ensureChannelExists(c *gin.Context, channelID int) bool {
	if _, err := model.GetChannelById(channelID, false); err != nil {
		common.ApiError(c, err)
		return false
	}
	return true
}

func buildChannelAccountFromRequest(channelID int, req channelAccountUpsertRequest) (*model.ChannelAccount, error) {
	if strings.TrimSpace(req.Key) == "" {
		return nil, fmt.Errorf("key is required")
	}
	status := common.ChannelStatusEnabled
	if req.Status != nil && *req.Status > 0 {
		status = *req.Status
	}
	priority := int64(0)
	if req.Priority != nil {
		priority = *req.Priority
	}
	weight := 0
	if req.Weight != nil {
		weight = *req.Weight
	}
	maxConcurrency := 0
	if req.MaxConcurrency != nil {
		maxConcurrency = *req.MaxConcurrency
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "账号"
	}
	return &model.ChannelAccount{
		ChannelId:          channelID,
		Name:               name,
		Key:                strings.TrimSpace(req.Key),
		Status:             status,
		Models:             strings.TrimSpace(req.Models),
		Group:              strings.TrimSpace(req.Group),
		Priority:           priority,
		Weight:             weight,
		BaseURL:            req.BaseURL,
		OpenAIOrganization: req.OpenAIOrganization,
		Other:              req.Other,
		Setting:            req.Setting,
		OtherSettings:      req.OtherSettings,
		ModelMapping:       req.ModelMapping,
		ParamOverride:      req.ParamOverride,
		HeaderOverride:     req.HeaderOverride,
		StatusCodeMapping:  req.StatusCodeMapping,
		MaxConcurrency:     maxConcurrency,
	}, nil
}

func channelAccountUpdateMap(req channelAccountUpsertRequest) map[string]interface{} {
	updates := map[string]interface{}{}
	if strings.TrimSpace(req.Name) != "" {
		updates["name"] = strings.TrimSpace(req.Name)
	}
	if strings.TrimSpace(req.Key) != "" {
		updates["key"] = strings.TrimSpace(req.Key)
	}
	if req.Status != nil && *req.Status > 0 {
		updates["status"] = *req.Status
	}
	updates["models"] = strings.TrimSpace(req.Models)
	updates["group"] = strings.TrimSpace(req.Group)
	updates["other"] = req.Other
	updates["settings"] = req.OtherSettings
	if req.Priority != nil {
		updates["priority"] = *req.Priority
	}
	if req.Weight != nil {
		updates["weight"] = *req.Weight
	}
	if req.BaseURL != nil {
		updates["base_url"] = *req.BaseURL
	}
	if req.OpenAIOrganization != nil {
		updates["openai_organization"] = *req.OpenAIOrganization
	}
	if req.Setting != nil {
		updates["setting"] = *req.Setting
	}
	if req.ModelMapping != nil {
		updates["model_mapping"] = *req.ModelMapping
	}
	if req.ParamOverride != nil {
		updates["param_override"] = *req.ParamOverride
	}
	if req.HeaderOverride != nil {
		updates["header_override"] = *req.HeaderOverride
	}
	if req.StatusCodeMapping != nil {
		updates["status_code_mapping"] = *req.StatusCodeMapping
	}
	if req.MaxConcurrency != nil {
		updates["max_concurrency"] = *req.MaxConcurrency
	}
	return updates
}

func createChannelAccountsFromKeys(channelID int, req channelAccountBatchRequest) (int, int, error) {
	keys := splitImportKeys(req.Keys)
	if len(keys) == 0 {
		return 0, 0, fmt.Errorf("keys is required")
	}
	existingKeys, err := getExistingChannelAccountKeys(channelID)
	if err != nil {
		return 0, 0, err
	}
	status := req.Status
	if status <= 0 {
		status = common.ChannelStatusEnabled
	}
	namePrefix := strings.TrimSpace(req.NamePrefix)
	if namePrefix == "" {
		namePrefix = "账号"
	}
	accounts := make([]model.ChannelAccount, 0, len(keys))
	skipped := 0
	for _, key := range keys {
		if existingKeys[key] {
			skipped++
			continue
		}
		existingKeys[key] = true
		accounts = append(accounts, model.ChannelAccount{
			ChannelId:      channelID,
			Name:           fmt.Sprintf("%s %d", namePrefix, len(accounts)+1),
			Key:            key,
			Status:         status,
			Models:         strings.TrimSpace(req.Models),
			Group:          strings.TrimSpace(req.Group),
			Priority:       req.Priority,
			Weight:         req.Weight,
			MaxConcurrency: req.MaxConcurrency,
		})
	}
	if len(accounts) == 0 {
		return 0, skipped, nil
	}
	if err := model.DB.Create(&accounts).Error; err != nil {
		return 0, skipped, err
	}
	return len(accounts), skipped, nil
}

func getExistingChannelAccountKeys(channelID int) (map[string]bool, error) {
	var accounts []model.ChannelAccount
	if err := model.DB.Select("key").Where("channel_id = ?", channelID).Find(&accounts).Error; err != nil {
		return nil, err
	}
	result := map[string]bool{}
	for _, account := range accounts {
		key := strings.TrimSpace(account.Key)
		if key != "" {
			result[key] = true
		}
	}
	return result, nil
}

func splitImportKeys(keys string) []string {
	lines := strings.Split(keys, "\n")
	result := make([]string, 0, len(lines))
	seen := map[string]bool{}
	for _, line := range lines {
		key := strings.TrimSpace(line)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, key)
	}
	return result
}

func channelAccountResponse(account *model.ChannelAccount) gin.H {
	if account == nil {
		return gin.H{}
	}
	return gin.H{
		"id":                   account.Id,
		"channel_id":           account.ChannelId,
		"name":                 account.Name,
		"key":                  account.GetMaskedKey(),
		"status":               account.Status,
		"models":               account.Models,
		"group":                account.Group,
		"priority":             account.Priority,
		"weight":               account.Weight,
		"last_used_time":       account.LastUsedTime,
		"used_quota":           account.UsedQuota,
		"base_url":             account.BaseURL,
		"openai_organization":  account.OpenAIOrganization,
		"other":                account.Other,
		"setting":              account.Setting,
		"settings":             account.OtherSettings,
		"model_mapping":        account.ModelMapping,
		"param_override":       account.ParamOverride,
		"header_override":      account.HeaderOverride,
		"status_code_mapping":  account.StatusCodeMapping,
		"rate_limited_until":   account.RateLimitedUntil,
		"overload_until":       account.OverloadUntil,
		"temp_disabled_until":  account.TempDisabledUntil,
		"disabled_reason":      account.DisabledReason,
		"last_error":           account.LastError,
		"max_concurrency":      account.MaxConcurrency,
		"created_time":         account.CreatedTime,
	}
}
