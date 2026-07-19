// Package controller - channel_account.go
// 该文件实现了渠道账号管理的 API 控制器
//
// 渠道账号（Channel Account）是多密钥渠道中的单个账号
// 每个账号有独立的 API 密钥、配置和状态
//
// 主要 API：
// - 账号列表：查询渠道下的所有账号
// - 创建账号：添加新的 API 密钥
// - 批量创建：批量导入多个 API 密钥
// - 更新账号：修改账号配置
// - 删除账号：移除账号
// - 状态管理：启用/禁用账号、清除冷却时间
package controller

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/service"
	"github.com/c1cada/NexusTok/service/authz"
	"github.com/c1cada/NexusTok/service/upstreamaccount"

	"github.com/gin-gonic/gin"
)

// channelAccountUpsertRequest 渠道账号创建/更新请求
type channelAccountUpsertRequest struct {
	Name               string  `json:"name"`                // 账号名称
	Key                string  `json:"key"`                 // API 密钥
	Status             *int    `json:"status"`              // 状态
	Models             string  `json:"models"`              // 支持的模型
	Group              string  `json:"group"`               // 用户组
	Priority           *int64  `json:"priority"`            // 优先级
	Weight             *int    `json:"weight"`              // 权重
	BaseURL            *string `json:"base_url"`            // 基础 URL
	OpenAIOrganization *string `json:"openai_organization"` // OpenAI 组织 ID
	Other              string  `json:"other"`               // 其他配置
	Setting            *string `json:"setting"`             // 设置
	OtherSettings      string  `json:"settings"`            // 其他设置
	ModelMapping       *string `json:"model_mapping"`       // 模型映射
	ParamOverride      *string `json:"param_override"`      // 参数覆盖
	HeaderOverride     *string `json:"header_override"`     // 请求头覆盖
	StatusCodeMapping  *string `json:"status_code_mapping"` // 状态码映射
	MaxConcurrency     *int    `json:"max_concurrency"`     // 最大并发数
}

// channelAccountBatchRequest 渠道账号批量创建请求
type channelAccountBatchRequest struct {
	Keys           string `json:"keys"`            // 批量密钥（每行一个）
	NamePrefix     string `json:"name_prefix"`     // 名称前缀
	Models         string `json:"models"`          // 支持的模型
	Group          string `json:"group"`           // 用户组
	Priority       int64  `json:"priority"`        // 优先级
	Weight         int    `json:"weight"`          // 权重
	Status         int    `json:"status"`          // 状态
	MaxConcurrency int    `json:"max_concurrency"` // 最大并发数
}

// channelAccountStatusRequest 渠道账号状态更新请求
type channelAccountStatusRequest struct {
	Status        int    `json:"status"`         // 新状态
	Reason        string `json:"reason"`         // 状态变更原因
	ClearCooldown bool   `json:"clear_cooldown"` // 是否清除冷却时间
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
		items = append(items, channelAccountResponseForContext(c, account))
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
	if err := syncChannelAccountCapabilitiesIfNeeded(channelID); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, channelAccountResponseForContext(c, account))
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
	if created > 0 {
		if err := syncChannelAccountCapabilitiesIfNeeded(channelID); err != nil {
			common.ApiError(c, err)
			return
		}
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
	common.ApiSuccess(c, channelAccountResponseForContext(c, account))
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
	req, requestData, ok := readChannelAccountUpsertRequest(c)
	if !ok {
		return
	}
	if channelAccountHasSensitiveChanges(account, req, requestData) &&
		!channelAccountCanSensitiveWrite(c) {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "insufficient permission for sensitive channel account fields",
		})
		return
	}
	if channelAccountHasUnknownFields(requestData) && !channelAccountCanSensitiveWrite(c) {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "insufficient permission for unknown channel account fields",
		})
		return
	}
	updates := channelAccountUpdateMap(account, req, requestData)
	if len(updates) == 0 {
		common.ApiSuccess(c, channelAccountResponseForContext(c, account))
		return
	}
	if err := model.DB.Model(account).Updates(updates).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	if channelAccountUpdatesAffectCapabilities(updates) {
		if err := syncChannelAccountCapabilitiesIfNeeded(channelID); err != nil {
			common.ApiError(c, err)
			return
		}
	}
	updated, err := model.GetChannelAccountById(channelID, accountID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, channelAccountResponseForContext(c, updated))
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
	if err := syncChannelAccountCapabilitiesIfNeeded(channelID); err != nil {
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
		if err := syncChannelAccountCapabilitiesIfNeeded(channelID); err != nil {
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
	if err := syncChannelAccountCapabilitiesIfNeeded(channelID); err != nil {
		common.ApiError(c, err)
		return
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
	if created > 0 {
		if err := syncChannelAccountCapabilitiesIfNeeded(channelID); err != nil {
			common.ApiError(c, err)
			return
		}
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

func readChannelAccountUpsertRequest(c *gin.Context) (channelAccountUpsertRequest, map[string]any, bool) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		common.ApiError(c, err)
		return channelAccountUpsertRequest{}, nil, false
	}
	var requestData map[string]any
	if err := common.Unmarshal(body, &requestData); err != nil {
		common.ApiError(c, err)
		return channelAccountUpsertRequest{}, nil, false
	}
	var req channelAccountUpsertRequest
	if err := common.Unmarshal(body, &req); err != nil {
		common.ApiError(c, err)
		return channelAccountUpsertRequest{}, nil, false
	}
	return req, requestData, true
}

func channelAccountCanSensitiveWrite(c *gin.Context) bool {
	return authz.Can(c.GetInt("id"), c.GetInt("role"), authz.ChannelAccountSensitiveWrite)
}

// channelAccountHasSensitiveChanges 判断更新请求是否触碰凭证或高级上游配置。
//
// 该函数只在 PUT 更新路径使用：普通 `channel_account.write` 允许维护名称、模型、分组、
// 优先级、权重和最大并发；key、上游地址、组织、provider settings、请求覆盖、模型映射、
// 状态码映射和 status 仍要求 `channel_account.sensitive_write`。比较时使用原始 JSON
// 字段集合区分“字段缺失”和“字段显式设置为空值/零值”。
func channelAccountHasSensitiveChanges(account *model.ChannelAccount, req channelAccountUpsertRequest, requestData map[string]any) bool {
	if account == nil {
		return true
	}
	if _, ok := requestData["key"]; ok && strings.TrimSpace(req.Key) != account.Key {
		return true
	}
	if _, ok := requestData["status"]; ok && req.Status != nil && *req.Status != account.Status {
		return true
	}
	if _, ok := requestData["base_url"]; ok && !equalOptionalStringPtr(req.BaseURL, account.BaseURL) {
		return true
	}
	if _, ok := requestData["openai_organization"]; ok && !equalOptionalStringPtr(req.OpenAIOrganization, account.OpenAIOrganization) {
		return true
	}
	if _, ok := requestData["other"]; ok && req.Other != account.Other {
		return true
	}
	if _, ok := requestData["setting"]; ok && !equalOptionalStringPtr(req.Setting, account.Setting) {
		return true
	}
	if _, ok := requestData["settings"]; ok && req.OtherSettings != account.OtherSettings {
		return true
	}
	if _, ok := requestData["model_mapping"]; ok && !equalOptionalStringPtr(req.ModelMapping, account.ModelMapping) {
		return true
	}
	if _, ok := requestData["param_override"]; ok && !equalOptionalStringPtr(req.ParamOverride, account.ParamOverride) {
		return true
	}
	if _, ok := requestData["header_override"]; ok && !equalOptionalStringPtr(req.HeaderOverride, account.HeaderOverride) {
		return true
	}
	if _, ok := requestData["status_code_mapping"]; ok && !equalOptionalStringPtr(req.StatusCodeMapping, account.StatusCodeMapping) {
		return true
	}
	return false
}

func channelAccountHasUnknownFields(requestData map[string]any) bool {
	for field := range requestData {
		if _, ok := channelAccountKnownFields[field]; !ok {
			return true
		}
	}
	return false
}

var channelAccountKnownFields = map[string]struct{}{
	"name":                {},
	"key":                 {},
	"status":              {},
	"models":              {},
	"group":               {},
	"priority":            {},
	"weight":              {},
	"base_url":            {},
	"openai_organization": {},
	"other":               {},
	"setting":             {},
	"settings":            {},
	"model_mapping":       {},
	"param_override":      {},
	"header_override":     {},
	"status_code_mapping": {},
	"max_concurrency":     {},
}

// syncChannelAccountCapabilitiesIfNeeded 在渠道账号变化后重建账号池渠道能力。
//
// 同步渠道的路由能力来自每个启用 key 的真实模型/分组组合，而不是渠道顶层
// models/group 的简单笛卡尔积。账号新增、删除、模型/分组修改或启停后，如果不立即
// 重建 `abilities`，Relay 会继续命中过期能力，表现为本地保存成功但请求没有按最新
// 优先级、权重和可用 key 降级。非账号池渠道会在 model 层快速返回，不影响普通渠道。
func syncChannelAccountCapabilitiesIfNeeded(channelID int) error {
	if channelID <= 0 {
		return nil
	}
	if err := model.SyncChannelAccountPoolCapabilities(channelID, nil); err != nil {
		return err
	}
	model.InitChannelCache()
	service.ResetProxyClientCache()
	return nil
}

func channelAccountUpdatesAffectCapabilities(updates map[string]interface{}) bool {
	for field := range updates {
		switch field {
		case "models", "group", "priority", "weight", "status":
			return true
		}
	}
	return false
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

// channelAccountUpdateMap 只更新本次请求显式提交的字段。
//
// 渠道账号编辑页支持普通管理员只维护名称、模型、分组、优先级、权重和并发限制。
// 因此 PUT 请求不能沿用“空字符串就是清空”的全量表单语义，否则只改一个字段时会把
// models/group/settings 等字段误覆盖为空。requestData 来自原始 JSON，用来判断字段是否
// 真实出现；key 仍保持“空值不写入”，避免编辑时空密钥覆盖已有凭证。
func channelAccountUpdateMap(account *model.ChannelAccount, req channelAccountUpsertRequest, requestData map[string]any) map[string]interface{} {
	updates := map[string]interface{}{}
	if _, ok := requestData["name"]; ok && strings.TrimSpace(req.Name) != "" {
		updates["name"] = strings.TrimSpace(req.Name)
	}
	if _, ok := requestData["key"]; ok && strings.TrimSpace(req.Key) != "" {
		updates["key"] = strings.TrimSpace(req.Key)
	}
	if _, ok := requestData["status"]; ok && req.Status != nil && *req.Status > 0 {
		updates["status"] = *req.Status
	}
	if _, ok := requestData["models"]; ok {
		updates["models"] = strings.TrimSpace(req.Models)
	}
	if _, ok := requestData["group"]; ok {
		updates["group"] = strings.TrimSpace(req.Group)
	}
	if _, ok := requestData["other"]; ok {
		updates["other"] = req.Other
	}
	if _, ok := requestData["settings"]; ok {
		settings := req.OtherSettings
		if account != nil {
			settings = upstreamaccount.PreserveAccountSyncMetadata(account.OtherSettings, req.OtherSettings)
		}
		updates["settings"] = settings
	}
	if _, ok := requestData["priority"]; ok && req.Priority != nil {
		updates["priority"] = *req.Priority
	}
	if _, ok := requestData["weight"]; ok && req.Weight != nil {
		updates["weight"] = *req.Weight
	}
	if _, ok := requestData["base_url"]; ok {
		updates["base_url"] = optionalStringValue(req.BaseURL)
	}
	if _, ok := requestData["openai_organization"]; ok {
		// ChannelAccount.OpenAIOrganization 未显式声明 gorm column，GORM 默认列名为
		// open_ai_organization；请求和响应仍沿用 API 字段 openai_organization。
		updates["open_ai_organization"] = optionalStringValue(req.OpenAIOrganization)
	}
	if _, ok := requestData["setting"]; ok {
		updates["setting"] = optionalStringValue(req.Setting)
	}
	if _, ok := requestData["model_mapping"]; ok {
		updates["model_mapping"] = optionalStringValue(req.ModelMapping)
	}
	if _, ok := requestData["param_override"]; ok {
		updates["param_override"] = optionalStringValue(req.ParamOverride)
	}
	if _, ok := requestData["header_override"]; ok {
		updates["header_override"] = optionalStringValue(req.HeaderOverride)
	}
	if _, ok := requestData["status_code_mapping"]; ok {
		updates["status_code_mapping"] = optionalStringValue(req.StatusCodeMapping)
	}
	if _, ok := requestData["max_concurrency"]; ok && req.MaxConcurrency != nil {
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

// channelAccountResponseForContext 根据当前管理员权限返回渠道账号响应。
//
// `channel_account.read` 只应支持列表查看和运行态运维，因此基础响应保留脱敏 key、
// 状态、模型、分组、权重、冷却和配额等页面需要的字段。私有上游地址、组织 ID、请求
// 覆盖、模型映射、provider settings 和原始错误详情只给具备
// `channel_account.sensitive_write` 的用户，避免把只读运维权限扩大为凭证配置读取权限。
func channelAccountResponseForContext(c *gin.Context, account *model.ChannelAccount) gin.H {
	userID := c.GetInt("id")
	role := c.GetInt("role")
	return channelAccountResponse(account, authz.Can(userID, role, authz.ChannelAccountSensitiveWrite))
}

func channelAccountResponse(account *model.ChannelAccount, includeSensitive bool) gin.H {
	if account == nil {
		return gin.H{}
	}
	response := gin.H{
		"id":                  account.Id,
		"channel_id":          account.ChannelId,
		"name":                account.Name,
		"key":                 account.GetMaskedKey(),
		"status":              account.Status,
		"models":              account.Models,
		"group":               account.Group,
		"priority":            account.Priority,
		"weight":              account.Weight,
		"last_used_time":      account.LastUsedTime,
		"used_quota":          account.UsedQuota,
		"rate_limited_until":  account.RateLimitedUntil,
		"overload_until":      account.OverloadUntil,
		"temp_disabled_until": account.TempDisabledUntil,
		"disabled_reason":     account.DisabledReason,
		"max_concurrency":     account.MaxConcurrency,
		"created_time":        account.CreatedTime,
	}
	if metadata := upstreamaccount.ReadAccountSyncDisplayMetadata(account.OtherSettings); metadata.KeyGroupID != "" ||
		metadata.KeyGroupName != "" ||
		metadata.GroupRatio != nil ||
		len(metadata.ModelRatios) > 0 ||
		metadata.EffectiveRatio > 0 ||
		metadata.RatioConversion > 0 ||
		metadata.RatioConversionConfig != nil {
		response["key_group_id"] = metadata.KeyGroupID
		response["key_group_name"] = metadata.KeyGroupName
		response["group_ratio"] = metadata.GroupRatio
		response["model_ratios"] = metadata.ModelRatios
		response["effective_ratio"] = metadata.EffectiveRatio
		response["ratio_conversion"] = metadata.RatioConversion
		response["ratio_conversion_config"] = metadata.RatioConversionConfig
	}
	if includeSensitive {
		response["base_url"] = account.BaseURL
		response["openai_organization"] = account.OpenAIOrganization
		response["other"] = account.Other
		response["setting"] = account.Setting
		response["settings"] = account.OtherSettings
		response["model_mapping"] = account.ModelMapping
		response["param_override"] = account.ParamOverride
		response["header_override"] = account.HeaderOverride
		response["status_code_mapping"] = account.StatusCodeMapping
		response["last_error"] = account.LastError
	}
	return response
}
