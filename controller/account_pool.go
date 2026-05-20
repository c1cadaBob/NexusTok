package controller

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/service"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type accountPoolGroupUpsertRequest struct {
	Name         string  `json:"name"`
	Platform     string  `json:"platform"`
	AuthType     string  `json:"auth_type"`
	Status       *int    `json:"status"`
	Strategy     string  `json:"strategy"`
	Models       string  `json:"models"`
	Group        string  `json:"group"`
	ModelMapping *string `json:"model_mapping"`
	Settings     string  `json:"settings"`
}

type poolAccountUpsertRequest struct {
	Name               string  `json:"name"`
	Platform           string  `json:"platform"`
	AuthType           string  `json:"auth_type"`
	Credentials        string  `json:"credentials"`
	Status             *int    `json:"status"`
	Schedulable        *bool   `json:"schedulable"`
	Models             string  `json:"models"`
	Group              string  `json:"group"`
	Priority           *int64  `json:"priority"`
	Weight             *int    `json:"weight"`
	MaxConcurrency     *int    `json:"max_concurrency"`
	Proxy              string  `json:"proxy"`
	BaseURL            *string `json:"base_url"`
	OpenAIOrganization *string `json:"openai_organization"`
	Other              string  `json:"other"`
	Setting            *string `json:"setting"`
	OtherSettings      string  `json:"settings"`
	ModelMapping       *string `json:"model_mapping"`
	ParamOverride      *string `json:"param_override"`
	HeaderOverride     *string `json:"header_override"`
	StatusCodeMapping  *string `json:"status_code_mapping"`
}

type poolAccountBatchRequest struct {
	Credentials    string `json:"credentials"`
	Keys           string `json:"keys"`
	NamePrefix     string `json:"name_prefix"`
	Platform       string `json:"platform"`
	AuthType       string `json:"auth_type"`
	Models         string `json:"models"`
	Group          string `json:"group"`
	Priority       int64  `json:"priority"`
	Weight         int    `json:"weight"`
	Status         int    `json:"status"`
	MaxConcurrency int    `json:"max_concurrency"`
}

type poolAccountStatusRequest struct {
	Status        int    `json:"status"`
	Reason        string `json:"reason"`
	ClearCooldown bool   `json:"clear_cooldown"`
	Schedulable   *bool  `json:"schedulable"`
}

type accountPoolCodexOAuthStartRequest struct {
	PoolGroupId int    `json:"pool_group_id"`
	Proxy       string `json:"proxy"`
}

type accountPoolCodexOAuthCompleteRequest struct {
	PoolGroupId int    `json:"pool_group_id"`
	Input       string `json:"input"`
	Name        string `json:"name"`
	Proxy       string `json:"proxy"`
}

func ListAccountPoolGroups(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	status, _ := strconv.Atoi(c.Query("status"))
	groups, total, err := model.GetAccountPoolGroups(pageInfo.GetPage(), pageInfo.GetPageSize(), status, c.Query("search"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.AttachAccountPoolGroupStats(groups)
	items := make([]gin.H, 0, len(groups))
	for _, group := range groups {
		items = append(items, accountPoolGroupResponse(group))
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

func ListAccountPoolGroupOptions(c *gin.Context) {
	var groups []*model.AccountPoolGroup
	if err := model.DB.Where("status = ?", common.ChannelStatusEnabled).Order("id DESC").Find(&groups).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	model.AttachAccountPoolGroupStats(groups)
	items := make([]gin.H, 0, len(groups))
	for _, group := range groups {
		items = append(items, gin.H{
			"id":        group.Id,
			"name":      group.Name,
			"platform":  group.Platform,
			"auth_type": group.AuthType,
			"strategy":  group.Strategy,
			"stats":     group.Stats,
		})
	}
	common.ApiSuccess(c, items)
}

func CreateAccountPoolGroup(c *gin.Context) {
	var req accountPoolGroupUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	group, err := buildAccountPoolGroupFromRequest(req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.DB.Create(group).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, accountPoolGroupResponse(group))
}

func GetAccountPoolGroup(c *gin.Context) {
	groupID, ok := parsePoolGroupIDParam(c)
	if !ok {
		return
	}
	group, err := model.GetAccountPoolGroupById(groupID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.AttachAccountPoolGroupStats([]*model.AccountPoolGroup{group})
	common.ApiSuccess(c, accountPoolGroupResponse(group))
}

func UpdateAccountPoolGroup(c *gin.Context) {
	groupID, ok := parsePoolGroupIDParam(c)
	if !ok {
		return
	}
	group, err := model.GetAccountPoolGroupById(groupID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var req accountPoolGroupUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	updates := accountPoolGroupUpdateMap(req)
	if len(updates) == 0 {
		common.ApiSuccess(c, accountPoolGroupResponse(group))
		return
	}
	if err := model.DB.Model(group).Updates(updates).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	updated, err := model.GetAccountPoolGroupById(groupID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, accountPoolGroupResponse(updated))
}

func DeleteAccountPoolGroup(c *gin.Context) {
	groupID, ok := parsePoolGroupIDParam(c)
	if !ok {
		return
	}
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("pool_group_id = ?", groupID).Delete(&model.PoolAccount{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", groupID).Delete(&model.AccountPoolGroup{}).Error
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func ListPoolAccounts(c *gin.Context) {
	groupID, ok := parsePoolGroupIDParam(c)
	if !ok {
		return
	}
	if !ensureAccountPoolGroupExists(c, groupID) {
		return
	}
	pageInfo := common.GetPageQuery(c)
	status, _ := strconv.Atoi(c.Query("status"))
	accounts, total, err := model.GetPoolAccounts(groupID, pageInfo.GetPage(), pageInfo.GetPageSize(), status, c.Query("search"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	items := make([]gin.H, 0, len(accounts))
	for _, account := range accounts {
		items = append(items, poolAccountResponse(account))
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	stats, _ := model.CountPoolAccountsByGroupIDs([]int{groupID})
	common.ApiSuccess(c, gin.H{
		"accounts": pageInfo,
		"stats":    stats[groupID],
	})
}

func CreatePoolAccount(c *gin.Context) {
	groupID, ok := parsePoolGroupIDParam(c)
	if !ok {
		return
	}
	group, err := model.GetAccountPoolGroupById(groupID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var req poolAccountUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	account, err := buildPoolAccountFromRequest(group, req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.DB.Create(account).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, poolAccountResponse(account))
}

func BatchCreatePoolAccounts(c *gin.Context) {
	groupID, ok := parsePoolGroupIDParam(c)
	if !ok {
		return
	}
	group, err := model.GetAccountPoolGroupById(groupID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var req poolAccountBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	created, skipped, err := createPoolAccountsFromCredentials(group, req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"created": created,
		"skipped": skipped,
	})
}

func GetPoolAccount(c *gin.Context) {
	accountID, ok := parsePoolAccountIDParam(c)
	if !ok {
		return
	}
	account, err := model.GetPoolAccountById(accountID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, poolAccountResponse(account))
}

func UpdatePoolAccount(c *gin.Context) {
	accountID, ok := parsePoolAccountIDParam(c)
	if !ok {
		return
	}
	account, err := model.GetPoolAccountById(accountID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var req poolAccountUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	updates, err := poolAccountUpdateMap(req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if len(updates) == 0 {
		common.ApiSuccess(c, poolAccountResponse(account))
		return
	}
	if err := model.DB.Model(account).Updates(updates).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	updated, err := model.GetPoolAccountById(accountID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, poolAccountResponse(updated))
}

func DeletePoolAccount(c *gin.Context) {
	accountID, ok := parsePoolAccountIDParam(c)
	if !ok {
		return
	}
	if err := model.DB.Where("id = ?", accountID).Delete(&model.PoolAccount{}).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func UpdatePoolAccountStatus(c *gin.Context) {
	accountID, ok := parsePoolAccountIDParam(c)
	if !ok {
		return
	}
	var req poolAccountStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if req.ClearCooldown && req.Status == 0 {
		err := model.UpdatePoolAccountErrorState(accountID, map[string]interface{}{
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
	if err := model.UpdatePoolAccountStatus(accountID, req.Status, req.Reason, req.Schedulable); err != nil {
		common.ApiError(c, err)
		return
	}
	if req.ClearCooldown {
		_ = model.UpdatePoolAccountErrorState(accountID, map[string]interface{}{
			"rate_limited_until":  0,
			"overload_until":      0,
			"temp_disabled_until": 0,
		})
	}
	common.ApiSuccess(c, nil)
}

func StartAccountPoolCodexOAuth(c *gin.Context) {
	var req accountPoolCodexOAuthStartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if req.PoolGroupId <= 0 {
		common.ApiErrorMsg(c, "pool_group_id is required")
		return
	}
	if !ensureAccountPoolGroupExists(c, req.PoolGroupId) {
		return
	}
	flow, err := service.CreateCodexOAuthAuthorizationFlow()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	session := sessions.Default(c)
	session.Set(accountPoolCodexOAuthSessionKey(req.PoolGroupId, "state"), flow.State)
	session.Set(accountPoolCodexOAuthSessionKey(req.PoolGroupId, "verifier"), flow.Verifier)
	session.Set(accountPoolCodexOAuthSessionKey(req.PoolGroupId, "proxy"), strings.TrimSpace(req.Proxy))
	session.Set(accountPoolCodexOAuthSessionKey(req.PoolGroupId, "created_at"), time.Now().Unix())
	_ = session.Save()
	common.ApiSuccess(c, gin.H{"authorize_url": flow.AuthorizeURL})
}

func CompleteAccountPoolCodexOAuth(c *gin.Context) {
	var req accountPoolCodexOAuthCompleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if req.PoolGroupId <= 0 {
		common.ApiErrorMsg(c, "pool_group_id is required")
		return
	}
	group, err := model.GetAccountPoolGroupById(req.PoolGroupId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	code, state, err := parseCodexAuthorizationInput(req.Input)
	if err != nil || strings.TrimSpace(code) == "" || strings.TrimSpace(state) == "" {
		common.ApiErrorMsg(c, "解析授权信息失败，请检查输入格式")
		return
	}
	session := sessions.Default(c)
	expectedState, _ := session.Get(accountPoolCodexOAuthSessionKey(req.PoolGroupId, "state")).(string)
	verifier, _ := session.Get(accountPoolCodexOAuthSessionKey(req.PoolGroupId, "verifier")).(string)
	sessionProxy, _ := session.Get(accountPoolCodexOAuthSessionKey(req.PoolGroupId, "proxy")).(string)
	if strings.TrimSpace(expectedState) == "" || strings.TrimSpace(verifier) == "" {
		common.ApiErrorMsg(c, "oauth flow not started or session expired")
		return
	}
	if state != expectedState {
		common.ApiErrorMsg(c, "state mismatch")
		return
	}
	proxy := strings.TrimSpace(req.Proxy)
	if proxy == "" {
		proxy = sessionProxy
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	tokenRes, err := service.ExchangeCodexAuthorizationCodeWithProxy(ctx, code, verifier, proxy)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	account, err := buildCodexPoolAccount(group, req.Name, proxy, tokenRes)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.DB.Create(account).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	session.Delete(accountPoolCodexOAuthSessionKey(req.PoolGroupId, "state"))
	session.Delete(accountPoolCodexOAuthSessionKey(req.PoolGroupId, "verifier"))
	session.Delete(accountPoolCodexOAuthSessionKey(req.PoolGroupId, "proxy"))
	session.Delete(accountPoolCodexOAuthSessionKey(req.PoolGroupId, "created_at"))
	_ = session.Save()
	common.ApiSuccess(c, poolAccountResponse(account))
}

func RefreshPoolAccountCredential(c *gin.Context) {
	accountID, ok := parsePoolAccountIDParam(c)
	if !ok {
		return
	}
	account, err := model.GetPoolAccountById(accountID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !strings.EqualFold(account.Platform, "codex") || account.AuthType != model.AccountPoolAuthTypeOfficialOAuth {
		common.ApiErrorMsg(c, "only Codex official OAuth account refresh is supported")
		return
	}
	raw, err := account.GetDecryptedCredentials()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var oauthKey service.CodexOAuthKey
	if err := common.UnmarshalJsonStr(raw, &oauthKey); err != nil {
		common.ApiError(c, err)
		return
	}
	if strings.TrimSpace(oauthKey.RefreshToken) == "" {
		common.ApiErrorMsg(c, "refresh_token is required")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	tokenRes, err := service.RefreshCodexOAuthTokenWithProxy(ctx, oauthKey.RefreshToken, account.Proxy)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := updateCodexPoolAccountCredential(account, &oauthKey, tokenRes); err != nil {
		common.ApiError(c, err)
		return
	}
	updated, err := model.GetPoolAccountById(accountID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, poolAccountResponse(updated))
}

func parsePoolGroupIDParam(c *gin.Context) (int, bool) {
	groupID, err := strconv.Atoi(c.Param("id"))
	if err != nil || groupID <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid pool group id"})
		return 0, false
	}
	return groupID, true
}

func parsePoolAccountIDParam(c *gin.Context) (int, bool) {
	accountID, err := strconv.Atoi(c.Param("account_id"))
	if err != nil || accountID <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid pool account id"})
		return 0, false
	}
	return accountID, true
}

func ensureAccountPoolGroupExists(c *gin.Context, groupID int) bool {
	if _, err := model.GetAccountPoolGroupById(groupID); err != nil {
		common.ApiError(c, err)
		return false
	}
	return true
}

func buildAccountPoolGroupFromRequest(req accountPoolGroupUpsertRequest) (*model.AccountPoolGroup, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	platform := strings.ToLower(strings.TrimSpace(req.Platform))
	if platform == "" {
		return nil, fmt.Errorf("platform is required")
	}
	authType := strings.ToLower(strings.TrimSpace(req.AuthType))
	if authType == "" {
		authType = model.AccountPoolAuthTypeAPIKey
	}
	strategy := strings.ToLower(strings.TrimSpace(req.Strategy))
	if strategy == "" {
		strategy = model.AccountPoolStrategyRoundRobin
	}
	status := common.ChannelStatusEnabled
	if req.Status != nil && *req.Status > 0 {
		status = *req.Status
	}
	return &model.AccountPoolGroup{
		Name:         name,
		Platform:     platform,
		AuthType:     authType,
		Status:       status,
		Strategy:     strategy,
		Models:       strings.TrimSpace(req.Models),
		Group:        strings.TrimSpace(req.Group),
		ModelMapping: req.ModelMapping,
		Settings:     strings.TrimSpace(req.Settings),
	}, nil
}

func accountPoolGroupUpdateMap(req accountPoolGroupUpsertRequest) map[string]interface{} {
	updates := map[string]interface{}{}
	if strings.TrimSpace(req.Name) != "" {
		updates["name"] = strings.TrimSpace(req.Name)
	}
	if strings.TrimSpace(req.Platform) != "" {
		updates["platform"] = strings.ToLower(strings.TrimSpace(req.Platform))
	}
	if strings.TrimSpace(req.AuthType) != "" {
		updates["auth_type"] = strings.ToLower(strings.TrimSpace(req.AuthType))
	}
	if req.Status != nil && *req.Status > 0 {
		updates["status"] = *req.Status
	}
	if strings.TrimSpace(req.Strategy) != "" {
		updates["strategy"] = strings.ToLower(strings.TrimSpace(req.Strategy))
	}
	updates["models"] = strings.TrimSpace(req.Models)
	updates["group"] = strings.TrimSpace(req.Group)
	updates["settings"] = strings.TrimSpace(req.Settings)
	if req.ModelMapping != nil {
		updates["model_mapping"] = *req.ModelMapping
	}
	return updates
}

func buildPoolAccountFromRequest(group *model.AccountPoolGroup, req poolAccountUpsertRequest) (*model.PoolAccount, error) {
	if group == nil {
		return nil, fmt.Errorf("account pool group is required")
	}
	if strings.TrimSpace(req.Credentials) == "" {
		return nil, fmt.Errorf("credentials is required")
	}
	encrypted, summary, err := encryptAccountPoolCredentials(req.Credentials)
	if err != nil {
		return nil, err
	}
	status := common.ChannelStatusEnabled
	if req.Status != nil && *req.Status > 0 {
		status = *req.Status
	}
	schedulable := true
	if req.Schedulable != nil {
		schedulable = *req.Schedulable
	}
	priority := int64(0)
	if req.Priority != nil {
		priority = *req.Priority
	}
	weight := 1
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
	platform := strings.ToLower(strings.TrimSpace(req.Platform))
	if platform == "" {
		platform = group.Platform
	}
	authType := strings.ToLower(strings.TrimSpace(req.AuthType))
	if authType == "" {
		authType = group.AuthType
	}
	return &model.PoolAccount{
		PoolGroupId:        group.Id,
		Name:               name,
		Platform:           platform,
		AuthType:           authType,
		Credentials:        encrypted,
		CredentialSummary:  summary,
		Status:             status,
		Schedulable:        schedulable,
		Models:             strings.TrimSpace(req.Models),
		Group:              strings.TrimSpace(req.Group),
		Priority:           priority,
		Weight:             weight,
		MaxConcurrency:     maxConcurrency,
		Proxy:              strings.TrimSpace(req.Proxy),
		BaseURL:            req.BaseURL,
		OpenAIOrganization: req.OpenAIOrganization,
		Other:              req.Other,
		Setting:            req.Setting,
		OtherSettings:      req.OtherSettings,
		ModelMapping:       req.ModelMapping,
		ParamOverride:      req.ParamOverride,
		HeaderOverride:     req.HeaderOverride,
		StatusCodeMapping:  req.StatusCodeMapping,
	}, nil
}

func poolAccountUpdateMap(req poolAccountUpsertRequest) (map[string]interface{}, error) {
	updates := map[string]interface{}{}
	if strings.TrimSpace(req.Name) != "" {
		updates["name"] = strings.TrimSpace(req.Name)
	}
	if strings.TrimSpace(req.Platform) != "" {
		updates["platform"] = strings.ToLower(strings.TrimSpace(req.Platform))
	}
	if strings.TrimSpace(req.AuthType) != "" {
		updates["auth_type"] = strings.ToLower(strings.TrimSpace(req.AuthType))
	}
	if strings.TrimSpace(req.Credentials) != "" {
		encrypted, summary, err := encryptAccountPoolCredentials(req.Credentials)
		if err != nil {
			return nil, err
		}
		updates["credentials"] = encrypted
		updates["credential_summary"] = summary
	}
	if req.Status != nil && *req.Status > 0 {
		updates["status"] = *req.Status
	}
	if req.Schedulable != nil {
		updates["schedulable"] = *req.Schedulable
	}
	updates["models"] = strings.TrimSpace(req.Models)
	updates["group"] = strings.TrimSpace(req.Group)
	updates["proxy"] = strings.TrimSpace(req.Proxy)
	updates["other"] = req.Other
	updates["settings"] = req.OtherSettings
	if req.Priority != nil {
		updates["priority"] = *req.Priority
	}
	if req.Weight != nil {
		updates["weight"] = *req.Weight
	}
	if req.MaxConcurrency != nil {
		updates["max_concurrency"] = *req.MaxConcurrency
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
	return updates, nil
}

func createPoolAccountsFromCredentials(group *model.AccountPoolGroup, req poolAccountBatchRequest) (int, int, error) {
	raw := req.Credentials
	if strings.TrimSpace(raw) == "" {
		raw = req.Keys
	}
	credentials := splitImportKeys(raw)
	if len(credentials) == 0 {
		return 0, 0, fmt.Errorf("credentials is required")
	}
	existing, err := getExistingPoolAccountSummaries(group.Id)
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
	platform := strings.ToLower(strings.TrimSpace(req.Platform))
	if platform == "" {
		platform = group.Platform
	}
	authType := strings.ToLower(strings.TrimSpace(req.AuthType))
	if authType == "" {
		authType = group.AuthType
	}
	weight := req.Weight
	if weight <= 0 {
		weight = 1
	}
	accounts := make([]model.PoolAccount, 0, len(credentials))
	skipped := 0
	for _, credential := range credentials {
		encrypted, summary, err := encryptAccountPoolCredentials(credential)
		if err != nil {
			return 0, skipped, err
		}
		if existing[summary] {
			skipped++
			continue
		}
		existing[summary] = true
		accounts = append(accounts, model.PoolAccount{
			PoolGroupId:       group.Id,
			Name:              fmt.Sprintf("%s %d", namePrefix, len(accounts)+1),
			Platform:          platform,
			AuthType:          authType,
			Credentials:       encrypted,
			CredentialSummary: summary,
			Status:            status,
			Schedulable:       true,
			Models:            strings.TrimSpace(req.Models),
			Group:             strings.TrimSpace(req.Group),
			Priority:          req.Priority,
			Weight:            weight,
			MaxConcurrency:    req.MaxConcurrency,
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

func getExistingPoolAccountSummaries(groupID int) (map[string]bool, error) {
	var accounts []model.PoolAccount
	if err := model.DB.Select("credential_summary").Where("pool_group_id = ?", groupID).Find(&accounts).Error; err != nil {
		return nil, err
	}
	result := map[string]bool{}
	for _, account := range accounts {
		summary := strings.TrimSpace(account.CredentialSummary)
		if summary != "" {
			result[summary] = true
		}
	}
	return result, nil
}

func encryptAccountPoolCredentials(raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", fmt.Errorf("credentials is required")
	}
	encrypted, err := common.EncryptSensitiveString(raw)
	if err != nil {
		return "", "", err
	}
	return encrypted, model.NormalizeAccountPoolCredentialSummary(raw), nil
}

func accountPoolGroupResponse(group *model.AccountPoolGroup) gin.H {
	if group == nil {
		return gin.H{}
	}
	return gin.H{
		"id":            group.Id,
		"name":          group.Name,
		"platform":      group.Platform,
		"auth_type":     group.AuthType,
		"status":        group.Status,
		"strategy":      group.Strategy,
		"models":        group.Models,
		"group":         group.Group,
		"model_mapping": group.ModelMapping,
		"settings":      group.Settings,
		"created_time":  group.CreatedTime,
		"updated_time":  group.UpdatedTime,
		"stats":         group.Stats,
	}
}

func poolAccountResponse(account *model.PoolAccount) gin.H {
	if account == nil {
		return gin.H{}
	}
	return gin.H{
		"id":                  account.Id,
		"pool_group_id":       account.PoolGroupId,
		"name":                account.Name,
		"platform":            account.Platform,
		"auth_type":           account.AuthType,
		"credential_summary":  account.CredentialSummary,
		"status":              account.Status,
		"schedulable":         account.Schedulable,
		"models":              account.Models,
		"group":               account.Group,
		"priority":            account.Priority,
		"weight":              account.Weight,
		"max_concurrency":     account.MaxConcurrency,
		"proxy":               account.Proxy,
		"base_url":            account.BaseURL,
		"openai_organization": account.OpenAIOrganization,
		"other":               account.Other,
		"setting":             account.Setting,
		"settings":            account.OtherSettings,
		"model_mapping":       account.ModelMapping,
		"param_override":      account.ParamOverride,
		"header_override":     account.HeaderOverride,
		"status_code_mapping": account.StatusCodeMapping,
		"last_used_time":      account.LastUsedTime,
		"used_quota":          account.UsedQuota,
		"rate_limited_until":  account.RateLimitedUntil,
		"overload_until":      account.OverloadUntil,
		"temp_disabled_until": account.TempDisabledUntil,
		"disabled_reason":     account.DisabledReason,
		"last_error":          account.LastError,
		"quota_snapshot":      account.QuotaSnapshot,
		"model_states":        account.ModelStates,
		"created_time":        account.CreatedTime,
		"updated_time":        account.UpdatedTime,
	}
}

func accountPoolCodexOAuthSessionKey(groupID int, field string) string {
	return fmt.Sprintf("account_pool_codex_oauth_%s_%d", field, groupID)
}

func buildCodexPoolAccount(group *model.AccountPoolGroup, name string, proxy string, tokenRes *service.CodexOAuthTokenResult) (*model.PoolAccount, error) {
	if tokenRes == nil {
		return nil, fmt.Errorf("token result is empty")
	}
	accountID, ok := service.ExtractCodexAccountIDFromJWT(tokenRes.AccessToken)
	if !ok {
		return nil, fmt.Errorf("failed to extract account_id from access_token")
	}
	email, _ := service.ExtractEmailFromJWT(tokenRes.AccessToken)
	oauthKey := service.CodexOAuthKey{
		AccessToken:  tokenRes.AccessToken,
		RefreshToken: tokenRes.RefreshToken,
		AccountID:    accountID,
		LastRefresh:  time.Now().Format(time.RFC3339),
		Expired:      tokenRes.ExpiresAt.Format(time.RFC3339),
		Email:        email,
		Type:         "codex",
	}
	encoded, err := common.Marshal(oauthKey)
	if err != nil {
		return nil, err
	}
	encrypted, summary, err := encryptAccountPoolCredentials(string(encoded))
	if err != nil {
		return nil, err
	}
	accountName := strings.TrimSpace(name)
	if accountName == "" {
		accountName = email
	}
	if accountName == "" {
		accountName = accountID
	}
	return &model.PoolAccount{
		PoolGroupId:       group.Id,
		Name:              accountName,
		Platform:          "codex",
		AuthType:          model.AccountPoolAuthTypeOfficialOAuth,
		Credentials:       encrypted,
		CredentialSummary: summary,
		Status:            common.ChannelStatusEnabled,
		Schedulable:       true,
		Weight:            1,
		Proxy:             strings.TrimSpace(proxy),
	}, nil
}

func updateCodexPoolAccountCredential(account *model.PoolAccount, oauthKey *service.CodexOAuthKey, tokenRes *service.CodexOAuthTokenResult) error {
	oauthKey.AccessToken = tokenRes.AccessToken
	oauthKey.RefreshToken = tokenRes.RefreshToken
	oauthKey.LastRefresh = time.Now().Format(time.RFC3339)
	oauthKey.Expired = tokenRes.ExpiresAt.Format(time.RFC3339)
	if strings.TrimSpace(oauthKey.Type) == "" {
		oauthKey.Type = "codex"
	}
	if strings.TrimSpace(oauthKey.AccountID) == "" {
		if accountID, ok := service.ExtractCodexAccountIDFromJWT(oauthKey.AccessToken); ok {
			oauthKey.AccountID = accountID
		}
	}
	if strings.TrimSpace(oauthKey.Email) == "" {
		if email, ok := service.ExtractEmailFromJWT(oauthKey.AccessToken); ok {
			oauthKey.Email = email
		}
	}
	encoded, err := common.Marshal(oauthKey)
	if err != nil {
		return err
	}
	encrypted, summary, err := encryptAccountPoolCredentials(string(encoded))
	if err != nil {
		return err
	}
	return model.DB.Model(account).Updates(map[string]interface{}{
		"credentials":        encrypted,
		"credential_summary": summary,
		"schedulable":        true,
		"last_error":         "",
	}).Error
}
