// Package controller - account_pool.go
// 该文件实现了账号池管理的 API 控制器
//
// 账号池功能用于集中管理多个 AI 服务提供商的账号：
// - 账号池分组（AccountPoolGroup）：按平台/认证方式分组管理账号
// - 池账号（PoolAccount）：具体的账号凭证和配置
//
// 主要 API：
// - 分组管理：创建、查询、更新、删除账号池分组
// - 账号管理：创建、查询、更新、删除池账号
// - 批量导入：支持批量导入多个账号
// - OAuth 登录：支持通过 OAuth 流程添加账号
// - 凭证刷新：支持刷新 OAuth 凭证
//
// 架构说明：
// - Controller 层处理 HTTP 请求和响应
// - 业务逻辑委托给 service 层
// - 数据持久化委托给 model 层
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
	"github.com/c1cada/NexusTok/service/accountauth"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// accountPoolGroupUpsertRequest 账号池分组创建/更新请求
type accountPoolGroupUpsertRequest struct {
	Name         string  `json:"name"`          // 分组名称
	Platform     string  `json:"platform"`      // 平台标识（如 openai, anthropic）
	AuthType     string  `json:"auth_type"`     // 认证类型（api_key, oauth 等）
	Status       *int    `json:"status"`        // 状态（启用/禁用）
	Strategy     string  `json:"strategy"`      // 调度策略（round_robin, random 等）
	Models       string  `json:"models"`        // 支持的模型列表
	Group        string  `json:"group"`         // 用户组
	ModelMapping *string `json:"model_mapping"` // 模型映射
	Settings     string  `json:"settings"`      // 其他配置
}

// poolAccountUpsertRequest 池账号创建/更新请求
type poolAccountUpsertRequest struct {
	Name               string  `json:"name"`                // 账号名称
	Platform           string  `json:"platform"`            // 平台标识
	AuthType           string  `json:"auth_type"`           // 认证类型
	Credentials        string  `json:"credentials"`         // 凭证（API Key 或 OAuth Token）
	Status             *int    `json:"status"`              // 状态
	Schedulable        *bool   `json:"schedulable"`         // 是否可调度
	Models             string  `json:"models"`              // 支持的模型
	Group              string  `json:"group"`               // 用户组
	Priority           *int64  `json:"priority"`            // 优先级
	Weight             *int    `json:"weight"`              // 权重
	MaxConcurrency     *int    `json:"max_concurrency"`     // 最大并发数
	Proxy              string  `json:"proxy"`               // 代理地址
	BaseURL            *string `json:"base_url"`            // 基础 URL
	OpenAIOrganization *string `json:"openai_organization"` // OpenAI 组织 ID
	Other              string  `json:"other"`               // 其他配置
	Setting            *string `json:"setting"`             // 设置
	OtherSettings      string  `json:"settings"`            // 其他设置
	ModelMapping       *string `json:"model_mapping"`       // 模型映射
	ParamOverride      *string `json:"param_override"`      // 参数覆盖
	HeaderOverride     *string `json:"header_override"`     // 请求头覆盖
	StatusCodeMapping  *string `json:"status_code_mapping"` // 状态码映射
}

// poolAccountBatchRequest 池账号批量创建请求
type poolAccountBatchRequest struct {
	Credentials    string `json:"credentials"`     // 批量凭证（每行一个）
	Keys           string `json:"keys"`            // 批量密钥（兼容旧格式）
	NamePrefix     string `json:"name_prefix"`     // 名称前缀
	Platform       string `json:"platform"`        // 平台标识
	AuthType       string `json:"auth_type"`       // 认证类型
	Models         string `json:"models"`          // 支持的模型
	Group          string `json:"group"`           // 用户组
	Priority       int64  `json:"priority"`        // 优先级
	Weight         int    `json:"weight"`          // 权重
	Status         int    `json:"status"`          // 状态
	MaxConcurrency int    `json:"max_concurrency"` // 最大并发数
}

// poolAccountStatusRequest 池账号状态更新请求
type poolAccountStatusRequest struct {
	Status        int    `json:"status"`         // 新状态
	Reason        string `json:"reason"`         // 状态变更原因
	ClearCooldown bool   `json:"clear_cooldown"` // 是否清除冷却时间
	Schedulable   *bool  `json:"schedulable"`    // 是否可调度
}

// accountPoolAuthFileImportRequest 原生认证文件导入请求。
// content 是 JSON 文件原文，系统会加密保存原文并生成关联 PoolAccount；其余字段用于
// 覆盖 JSON 中的文件级配置，方便 sub2/newapi 等包装格式缺少本地调度字段时补齐。
type accountPoolAuthFileImportRequest struct {
	Name           string   `json:"name"`            // 文件显示名称
	Content        string   `json:"content"`         // JSON 认证文件原文
	PoolGroupID    int      `json:"pool_group_id"`   // 指定账号池分组
	GroupName      string   `json:"group_name"`      // 自动创建分组时使用的名称
	Provider       string   `json:"provider"`        // 覆盖 provider
	Platform       string   `json:"platform"`        // 覆盖本地平台
	AuthType       string   `json:"auth_type"`       // 覆盖认证类型
	AccountGroup   string   `json:"account_group"`   // 单一调用分组
	AccountGroups  []string `json:"account_groups"`  // 多调用分组
	Models         string   `json:"models"`          // 模型限制
	Proxy          string   `json:"proxy"`           // 文件级代理
	BaseURL        *string  `json:"base_url"`        // 基础 URL
	Priority       *int64   `json:"priority"`        // 优先级
	Weight         *int     `json:"weight"`          // 权重
	MaxConcurrency *int     `json:"max_concurrency"` // 最大并发数
	Status         *int     `json:"status"`          // 状态
	SkipDuplicates *bool    `json:"skip_duplicates"` // 批量导入时是否跳过重复文件
}

// accountPoolAuthFileUpdateRequest 原生认证文件更新请求。
// content 为空时只修改文件级调度字段；content 非空时重新解析凭据并更新关联账号凭证。
type accountPoolAuthFileUpdateRequest struct {
	Name           *string  `json:"name"`
	Content        *string  `json:"content"`
	PoolGroupID    *int     `json:"pool_group_id"`
	GroupName      *string  `json:"group_name"`
	Provider       *string  `json:"provider"`
	Platform       *string  `json:"platform"`
	AuthType       *string  `json:"auth_type"`
	AccountGroup   *string  `json:"account_group"`
	AccountGroups  []string `json:"account_groups"`
	Models         *string  `json:"models"`
	Proxy          *string  `json:"proxy"`
	BaseURL        *string  `json:"base_url"`
	Priority       *int64   `json:"priority"`
	Weight         *int     `json:"weight"`
	MaxConcurrency *int     `json:"max_concurrency"`
	Status         *int     `json:"status"`
}

// accountPoolCodexOAuthStartRequest Codex OAuth 开始请求
type accountPoolCodexOAuthStartRequest struct {
	PoolGroupId int    `json:"pool_group_id"` // 账号池分组 ID
	Proxy       string `json:"proxy"`         // 代理地址
}

// accountPoolCodexOAuthCompleteRequest Codex OAuth 完成请求
type accountPoolCodexOAuthCompleteRequest struct {
	PoolGroupId int    `json:"pool_group_id"` // 账号池分组 ID
	SessionId   string `json:"session_id"`    // 会话 ID
	Input       string `json:"input"`         // 用户输入（如授权码）
	Name        string `json:"name"`          // 账号名称
	Proxy       string `json:"proxy"`         // 代理地址
}

// accountPoolProviderLoginRequest 账号池提供商登录请求
type accountPoolProviderLoginRequest struct {
	SessionId    string            `json:"session_id"`    // 会话 ID
	Input        string            `json:"input"`         // 用户输入
	Name         string            `json:"name"`          // 账号名称
	Proxy        string            `json:"proxy"`         // 代理地址
	NoBrowser    bool              `json:"no_browser"`    // 是否不自动打开浏览器
	ProjectID    string            `json:"project_id"`    // 项目 ID
	CallbackPort int               `json:"callback_port"` // 回调端口
	Metadata     map[string]string `json:"metadata"`      // 元数据
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
	if err := model.DB.
		Where("status = ? AND (source = ? OR source = '')", common.ChannelStatusEnabled, model.AccountPoolGroupSourceNative).
		Order("id DESC").
		Find(&groups).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	model.AttachAccountPoolGroupStats(groups)
	items := make([]gin.H, 0, len(groups))
	for _, group := range groups {
		if item, ok := accountPoolGroupOptionResponse(group); ok {
			items = append(items, item)
		}
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

func ListAccountPoolProviders(c *gin.Context) {
	common.ApiSuccess(c, accountauth.DefaultManager().Providers())
}

func ListAccountPoolAuthFiles(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	status, _ := strconv.Atoi(c.Query("status"))
	poolGroupID, _ := strconv.Atoi(c.Query("pool_group_id"))
	authFiles, total, err := model.GetAccountPoolAuthFiles(pageInfo.GetPage(), pageInfo.GetPageSize(), status, poolGroupID, c.Query("provider"), c.Query("search"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	items := make([]gin.H, 0, len(authFiles))
	for _, authFile := range authFiles {
		items = append(items, accountPoolAuthFileResponse(authFile))
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

func ListAccountPoolUsageLogs(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	poolGroupID, _ := strconv.Atoi(c.Query("pool_group_id"))
	poolAccountID, _ := strconv.Atoi(c.Query("pool_account_id"))
	channelID, _ := strconv.Atoi(c.Query("channel_id"))
	userID, _ := strconv.Atoi(c.Query("user_id"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	success := parseAccountPoolUsageSuccess(c.Query("success"))
	logs, total, err := model.GetPoolAccountUsageLogs(model.PoolAccountUsageLogFilter{
		PoolGroupId:       poolGroupID,
		PoolAccountId:     poolAccountID,
		ChannelId:         channelID,
		UserId:            userID,
		Success:           success,
		StartTimestamp:    startTimestamp,
		EndTimestamp:      endTimestamp,
		ModelName:         c.Query("model_name"),
		RequestId:         c.Query("request_id"),
		UpstreamRequestId: c.Query("upstream_request_id"),
		Search:            c.Query("search"),
		StartIdx:          pageInfo.GetStartIdx(),
		Limit:             pageInfo.GetPageSize(),
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(logs)
	common.ApiSuccess(c, pageInfo)
}

func CreateAccountPoolAuthFile(c *gin.Context) {
	var req accountPoolAuthFileImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	result, err := service.ImportAccountPoolAuthFile(service.AccountPoolAuthFileImportOptions{
		Name:           req.Name,
		Content:        req.Content,
		PoolGroupID:    req.PoolGroupID,
		GroupName:      req.GroupName,
		Provider:       req.Provider,
		Platform:       req.Platform,
		AuthType:       req.AuthType,
		AccountGroups:  mergeAccountPoolAuthFileGroups(req.AccountGroups, req.AccountGroup),
		Models:         req.Models,
		Proxy:          req.Proxy,
		BaseURL:        req.BaseURL,
		Priority:       req.Priority,
		Weight:         req.Weight,
		MaxConcurrency: req.MaxConcurrency,
		Status:         req.Status,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, accountPoolAuthFileResultResponse(result))
}

func ImportAccountPoolAuthFiles(c *gin.Context) {
	var req accountPoolAuthFileImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	skipDuplicates := true
	if req.SkipDuplicates != nil {
		skipDuplicates = *req.SkipDuplicates
	}
	result, err := service.ImportAccountPoolAuthFiles(service.AccountPoolAuthFileBatchImportOptions{
		AccountPoolAuthFileImportOptions: service.AccountPoolAuthFileImportOptions{
			Name:           req.Name,
			Content:        req.Content,
			PoolGroupID:    req.PoolGroupID,
			GroupName:      req.GroupName,
			Provider:       req.Provider,
			Platform:       req.Platform,
			AuthType:       req.AuthType,
			AccountGroups:  mergeAccountPoolAuthFileGroups(req.AccountGroups, req.AccountGroup),
			Models:         req.Models,
			Proxy:          req.Proxy,
			BaseURL:        req.BaseURL,
			Priority:       req.Priority,
			Weight:         req.Weight,
			MaxConcurrency: req.MaxConcurrency,
			Status:         req.Status,
		},
		SkipDuplicates: skipDuplicates,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, accountPoolAuthFileBatchImportResponse(result))
}

func GetAccountPoolAuthFile(c *gin.Context) {
	authFileID, ok := parseAccountPoolAuthFileIDParam(c)
	if !ok {
		return
	}
	authFile, err := model.GetAccountPoolAuthFileById(authFileID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, accountPoolAuthFileResponse(authFile))
}

func UpdateAccountPoolAuthFile(c *gin.Context) {
	authFileID, ok := parseAccountPoolAuthFileIDParam(c)
	if !ok {
		return
	}
	var req accountPoolAuthFileUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	opts := service.AccountPoolAuthFileUpdateOptions{
		Name:           req.Name,
		Content:        req.Content,
		PoolGroupID:    req.PoolGroupID,
		GroupName:      req.GroupName,
		Provider:       req.Provider,
		Platform:       req.Platform,
		AuthType:       req.AuthType,
		Models:         req.Models,
		Proxy:          req.Proxy,
		BaseURL:        req.BaseURL,
		Priority:       req.Priority,
		Weight:         req.Weight,
		MaxConcurrency: req.MaxConcurrency,
		Status:         req.Status,
	}
	if req.AccountGroups != nil || req.AccountGroup != nil {
		groups := mergeAccountPoolAuthFileGroups(req.AccountGroups, stringPointerValue(req.AccountGroup))
		opts.AccountGroups = &groups
	}
	result, err := service.UpdateAccountPoolAuthFile(authFileID, opts)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, accountPoolAuthFileResultResponse(result))
}

func DeleteAccountPoolAuthFile(c *gin.Context) {
	authFileID, ok := parseAccountPoolAuthFileIDParam(c)
	if !ok {
		return
	}
	deleteAccount := strings.ToLower(strings.TrimSpace(c.DefaultQuery("delete_account", "true"))) != "false"
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var authFile model.AccountPoolAuthFile
		if err := tx.Where("id = ?", authFileID).First(&authFile).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ?", authFileID).Delete(&model.AccountPoolAuthFile{}).Error; err != nil {
			return err
		}
		if deleteAccount && authFile.PoolAccountId > 0 {
			if err := tx.Where("id = ?", authFile.PoolAccountId).Delete(&model.PoolAccount{}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func StartAccountPoolProviderOAuth(c *gin.Context) {
	groupID, ok := parsePoolGroupIDParam(c)
	if !ok {
		return
	}
	group, err := model.GetAccountPoolGroupById(groupID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	provider, ok := getAccountPoolProvider(c)
	if !ok {
		return
	}
	var req accountPoolProviderLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	result, err := provider.StartOAuth(c.Request.Context(), group, accountauth.LoginStartRequest{
		PoolGroupID: groupID,
		Name:        strings.TrimSpace(req.Name),
		Options:     accountPoolLoginOptions(req),
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func CompleteAccountPoolProviderOAuth(c *gin.Context) {
	groupID, ok := parsePoolGroupIDParam(c)
	if !ok {
		return
	}
	group, err := model.GetAccountPoolGroupById(groupID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	provider, ok := getAccountPoolProvider(c)
	if !ok {
		return
	}
	var req accountPoolProviderLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	credential, err := provider.CompleteOAuth(c.Request.Context(), group, accountauth.LoginCompleteRequest{
		SessionID:   strings.TrimSpace(req.SessionId),
		PoolGroupID: groupID,
		Name:        strings.TrimSpace(req.Name),
		Input:       strings.TrimSpace(req.Input),
		Options:     accountPoolLoginOptions(req),
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	account, err := createPoolAccountFromCredential(group, credential)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	accountauth.SetLoginSessionAccountID(req.SessionId, account.Id)
	common.ApiSuccess(c, poolAccountResponse(account))
}

func StartAccountPoolProviderDevice(c *gin.Context) {
	groupID, ok := parsePoolGroupIDParam(c)
	if !ok {
		return
	}
	group, err := model.GetAccountPoolGroupById(groupID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	provider, ok := getAccountPoolProvider(c)
	if !ok {
		return
	}
	var req accountPoolProviderLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	options := accountPoolLoginOptions(req)
	result, err := provider.StartDevice(c.Request.Context(), group, accountauth.LoginStartRequest{
		PoolGroupID: groupID,
		Name:        strings.TrimSpace(req.Name),
		Options:     options,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	startPoolDeviceLoginWorker(provider, group.Id, strings.TrimSpace(req.Name), options, result.SessionID)
	common.ApiSuccess(c, result)
}

func GetAccountPoolLoginSession(c *gin.Context) {
	session, ok := accountauth.GetLoginSession(c.Param("session_id"))
	if !ok {
		common.ApiErrorMsg(c, "login session not found")
		return
	}
	common.ApiSuccess(c, accountauth.LoginSessionPublicView(session))
}

func CancelAccountPoolLoginSession(c *gin.Context) {
	if !accountauth.CancelLoginSession(c.Param("session_id")) {
		common.ApiErrorMsg(c, "login session not found")
		return
	}
	common.ApiSuccess(c, nil)
}

func ResetPoolAccountRuntime(c *gin.Context) {
	accountID, ok := parsePoolAccountIDParam(c)
	if !ok {
		return
	}
	err := model.UpdatePoolAccountErrorState(accountID, map[string]interface{}{
		"unavailable":         false,
		"status_message":      "",
		"last_error":          "",
		"quota_snapshot":      "",
		"model_states":        "",
		"recent_requests":     "",
		"success_count":       0,
		"failed_count":        0,
		"rate_limited_until":  0,
		"overload_until":      0,
		"temp_disabled_until": 0,
		"next_retry_time":     0,
		"disabled_reason":     "",
	})
	if err != nil {
		common.ApiError(c, err)
		return
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
	group, err := model.GetAccountPoolGroupById(req.PoolGroupId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	provider, err := accountauth.DefaultManager().MustProvider("codex")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	result, err := provider.StartOAuth(c.Request.Context(), group, accountauth.LoginStartRequest{
		PoolGroupID: req.PoolGroupId,
		Options: accountauth.LoginOptions{
			Proxy: strings.TrimSpace(req.Proxy),
		},
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	session := sessions.Default(c)
	session.Set(accountPoolCodexOAuthSessionKey(req.PoolGroupId, "session_id"), result.SessionID)
	_ = session.Save()
	common.ApiSuccess(c, gin.H{"authorize_url": result.AuthorizeURL, "session_id": result.SessionID})
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
	session := sessions.Default(c)
	sessionID := strings.TrimSpace(req.SessionId)
	if sessionID == "" {
		sessionID, _ = session.Get(accountPoolCodexOAuthSessionKey(req.PoolGroupId, "session_id")).(string)
	}
	provider, err := accountauth.DefaultManager().MustProvider("codex")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	proxy := strings.TrimSpace(req.Proxy)
	credential, err := provider.CompleteOAuth(c.Request.Context(), group, accountauth.LoginCompleteRequest{
		SessionID:   sessionID,
		PoolGroupID: req.PoolGroupId,
		Name:        strings.TrimSpace(req.Name),
		Input:       strings.TrimSpace(req.Input),
		Options: accountauth.LoginOptions{
			Proxy: proxy,
		},
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	account, err := createPoolAccountFromCredential(group, credential)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	accountauth.SetLoginSessionAccountID(sessionID, account.Id)
	session.Delete(accountPoolCodexOAuthSessionKey(req.PoolGroupId, "session_id"))
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
	providerName := account.GetCredentialProvider()
	provider, err := accountauth.DefaultManager().MustProvider(providerName)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	credential, err := provider.Refresh(ctx, account)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := updatePoolAccountCredential(account, credential); err != nil {
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

func parseAccountPoolAuthFileIDParam(c *gin.Context) (int, bool) {
	authFileID, err := strconv.Atoi(c.Param("auth_file_id"))
	if err != nil || authFileID <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid auth file id"})
		return 0, false
	}
	return authFileID, true
}

func parseAccountPoolUsageSuccess(value string) *bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "success", "succeeded":
		result := true
		return &result
	case "false", "0", "failed", "failure", "error":
		result := false
		return &result
	default:
		return nil
	}
}

func ensureAccountPoolGroupExists(c *gin.Context, groupID int) bool {
	if _, err := model.GetAccountPoolGroupById(groupID); err != nil {
		common.ApiError(c, err)
		return false
	}
	return true
}

func getAccountPoolProvider(c *gin.Context) (accountauth.Provider, bool) {
	providerName := strings.TrimSpace(c.Param("provider"))
	if providerName == "" {
		common.ApiErrorMsg(c, "provider is required")
		return nil, false
	}
	provider, err := accountauth.DefaultManager().MustProvider(providerName)
	if err != nil {
		common.ApiError(c, err)
		return nil, false
	}
	return provider, true
}

func accountPoolLoginOptions(req accountPoolProviderLoginRequest) accountauth.LoginOptions {
	return accountauth.LoginOptions{
		NoBrowser:    req.NoBrowser,
		ProjectID:    strings.TrimSpace(req.ProjectID),
		CallbackPort: req.CallbackPort,
		Proxy:        strings.TrimSpace(req.Proxy),
		Metadata:     req.Metadata,
	}
}

func startPoolDeviceLoginWorker(provider accountauth.Provider, groupID int, name string, options accountauth.LoginOptions, sessionID string) {
	if provider == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	go func() {
		group, err := model.GetAccountPoolGroupById(groupID)
		if err != nil {
			markLoginSessionFailed(sessionID, err)
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 16*time.Minute)
		defer cancel()
		credential, err := provider.CompleteDevice(ctx, group, accountauth.LoginCompleteRequest{
			SessionID:   sessionID,
			PoolGroupID: groupID,
			Name:        name,
			Options:     options,
		})
		if err != nil {
			markLoginSessionFailed(sessionID, err)
			return
		}
		account, err := createPoolAccountFromCredential(group, credential)
		if err != nil {
			markLoginSessionFailed(sessionID, err)
			return
		}
		accountauth.SetLoginSessionAccountID(sessionID, account.Id)
	}()
}

func markLoginSessionFailed(sessionID string, err error) {
	session, ok := accountauth.GetLoginSession(sessionID)
	if !ok || session == nil || err == nil {
		return
	}
	session.Status = accountauth.LoginSessionFailed
	session.StatusMessage = err.Error()
	accountauth.UpdateLoginSession(session)
}

func createPoolAccountFromCredential(group *model.AccountPoolGroup, credential *accountauth.AccountCredential) (*model.PoolAccount, error) {
	if group == nil {
		return nil, fmt.Errorf("account pool group is required")
	}
	if credential == nil {
		return nil, fmt.Errorf("credential is required")
	}
	encrypted, summary, err := encryptAccountPoolCredentials(credential.Credentials)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(credential.Summary) != "" {
		summary = credential.Summary
	}
	account := &model.PoolAccount{
		PoolGroupId:        group.Id,
		Name:               credential.Label,
		Platform:           credential.Provider,
		AuthType:           credential.AuthType,
		Credentials:        encrypted,
		CredentialSummary:  summary,
		CredentialProvider: credential.Provider,
		CredentialLabel:    credential.Label,
		CredentialMetadata: accountauth.MetadataToJSON(credential.Metadata),
		CredentialAttrs:    accountauth.AttributesToJSON(credential.Attributes),
		Status:             common.ChannelStatusEnabled,
		Schedulable:        true,
		Weight:             1,
		LastRefreshedTime:  timestampOrZero(credential.LastRefreshedAt),
		NextRefreshTime:    timestampOrZero(credential.NextRefreshAt),
	}
	if account.Name == "" {
		account.Name = credential.Provider + " 账号"
	}
	if err := model.DB.Create(account).Error; err != nil {
		return nil, err
	}
	return account, nil
}

func updatePoolAccountCredential(account *model.PoolAccount, credential *accountauth.AccountCredential) error {
	if account == nil || credential == nil {
		return fmt.Errorf("account and credential are required")
	}
	encrypted, summary, err := encryptAccountPoolCredentials(credential.Credentials)
	if err != nil {
		return err
	}
	if strings.TrimSpace(credential.Summary) != "" {
		summary = credential.Summary
	}
	updates := map[string]interface{}{
		"credentials":           encrypted,
		"credential_summary":    summary,
		"credential_provider":   credential.Provider,
		"credential_label":      credential.Label,
		"credential_metadata":   accountauth.MetadataToJSON(credential.Metadata),
		"credential_attributes": accountauth.AttributesToJSON(credential.Attributes),
		"schedulable":           true,
		"unavailable":           false,
		"last_error":            "",
		"status_message":        "",
		"last_refreshed_time":   timestampOrZero(credential.LastRefreshedAt),
		"next_refresh_time":     timestampOrZero(credential.NextRefreshAt),
	}
	if strings.TrimSpace(credential.Provider) != "" {
		updates["platform"] = credential.Provider
	}
	if strings.TrimSpace(credential.AuthType) != "" {
		updates["auth_type"] = credential.AuthType
	}
	if strings.TrimSpace(credential.Label) != "" {
		updates["name"] = credential.Label
	}
	return model.DB.Model(account).Updates(updates).Error
}

func timestampOrZero(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

func syncCLIProxyGroupsForList(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	if err := service.SyncCLIProxyAccountGroups(ctx); err != nil {
		common.SysLog(service.AccountPoolSidecarUnavailableError(err).Error())
	}
}

func attachCLIProxyGroupStats(c *gin.Context, groups []*model.AccountPoolGroup) {
	hasCLIProxyGroup := false
	for _, group := range groups {
		if service.IsCLIProxyAccountPoolGroup(group) {
			hasCLIProxyGroup = true
			break
		}
	}
	if !hasCLIProxyGroup {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	stats, err := service.CLIProxyGroupStats(ctx)
	if err != nil {
		common.SysLog(service.AccountPoolSidecarUnavailableError(err).Error())
		return
	}
	for _, group := range groups {
		if !service.IsCLIProxyAccountPoolGroup(group) {
			continue
		}
		groupKey := strings.TrimSpace(group.ExternalKey)
		if groupKey == "" {
			groupKey = strings.TrimSpace(group.Name)
		}
		if groupStats := stats[groupKey]; groupStats != nil {
			group.Stats = groupStats
		}
	}
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
		Source:       model.AccountPoolGroupSourceNative,
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
		"id":                 group.Id,
		"name":               group.Name,
		"platform":           group.Platform,
		"auth_type":          group.AuthType,
		"source":             group.Source,
		"external_group_key": group.ExternalKey,
		"status":             group.Status,
		"strategy":           group.Strategy,
		"models":             group.Models,
		"group":              group.Group,
		"model_mapping":      group.ModelMapping,
		"settings":           group.Settings,
		"created_time":       group.CreatedTime,
		"updated_time":       group.UpdatedTime,
		"stats":              group.Stats,
	}
}

// accountPoolGroupOptionResponse 构造渠道表单可选择的账号池组响应。
// 原生账号池是当前唯一的账号池运行目标：渠道绑定分组后，Relay 会在本地数据库中
// 查询该组下的 PoolAccount 并完成调度。这里不再同步或过滤外部 Sidecar 分组，只要求
// 分组启用且至少包含一个账号，避免用户选到空组后保存了不可运行的渠道。
func accountPoolGroupOptionResponse(group *model.AccountPoolGroup) (gin.H, bool) {
	if group == nil || group.Status != common.ChannelStatusEnabled {
		return nil, false
	}
	source := strings.TrimSpace(group.Source)
	if source != "" && !strings.EqualFold(source, model.AccountPoolGroupSourceNative) {
		return nil, false
	}
	stats := group.Stats
	if stats == nil || stats["total"] <= 0 {
		return nil, false
	}
	return gin.H{
		"id":                 group.Id,
		"name":               group.Name,
		"platform":           group.Platform,
		"auth_type":          group.AuthType,
		"source":             group.Source,
		"external_group_key": group.ExternalKey,
		"strategy":           group.Strategy,
		"stats":              group.Stats,
	}, true
}

func accountPoolAuthFileResultResponse(result *service.AccountPoolAuthFileImportResult) gin.H {
	if result == nil {
		return gin.H{}
	}
	return gin.H{
		"auth_file": accountPoolAuthFileResponse(result.AuthFile),
		"account":   poolAccountResponse(result.Account),
		"group":     accountPoolGroupResponse(result.Group),
	}
}

func accountPoolAuthFileBatchImportResponse(result *service.AccountPoolAuthFileBatchImportResult) gin.H {
	if result == nil {
		return gin.H{}
	}
	items := make([]gin.H, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, accountPoolAuthFileResultResponse(item))
	}
	errors := make([]gin.H, 0, len(result.Errors))
	for _, item := range result.Errors {
		errors = append(errors, gin.H{
			"index":   item.Index,
			"name":    item.Name,
			"message": item.Message,
		})
	}
	return gin.H{
		"created": result.Created,
		"skipped": result.Skipped,
		"failed":  result.Failed,
		"items":   items,
		"errors":  errors,
	}
}

func accountPoolAuthFileResponse(authFile *model.AccountPoolAuthFile) gin.H {
	if authFile == nil {
		return gin.H{}
	}
	groups := splitAccountPoolAuthFileGroups(authFile.AccountGroups)
	accountGroup := ""
	if len(groups) > 0 {
		accountGroup = groups[0]
	}
	return gin.H{
		"id":                    authFile.Id,
		"name":                  authFile.Name,
		"source_platform":       authFile.SourcePlatform,
		"format":                authFile.Format,
		"provider":              authFile.Provider,
		"platform":              authFile.Platform,
		"auth_type":             authFile.AuthType,
		"pool_group_id":         authFile.PoolGroupId,
		"pool_account_id":       authFile.PoolAccountId,
		"status":                authFile.Status,
		"file_digest":           authFile.FileDigest,
		"credential_summary":    authFile.CredentialSummary,
		"credential_metadata":   authFile.CredentialMetadata,
		"credential_attributes": authFile.CredentialAttrs,
		"account_group":         accountGroup,
		"account_groups":        groups,
		"models":                authFile.Models,
		"proxy":                 authFile.Proxy,
		"base_url":              authFile.BaseURL,
		"priority":              authFile.Priority,
		"weight":                authFile.Weight,
		"max_concurrency":       authFile.MaxConcurrency,
		"last_imported_time":    authFile.LastImportedTime,
		"created_time":          authFile.CreatedTime,
		"updated_time":          authFile.UpdatedTime,
	}
}

func poolAccountResponse(account *model.PoolAccount) gin.H {
	if account == nil {
		return gin.H{}
	}
	return gin.H{
		"id":                    account.Id,
		"pool_group_id":         account.PoolGroupId,
		"name":                  account.Name,
		"platform":              account.Platform,
		"auth_type":             account.AuthType,
		"credential_summary":    account.CredentialSummary,
		"credential_provider":   account.CredentialProvider,
		"credential_label":      account.CredentialLabel,
		"credential_metadata":   account.CredentialMetadata,
		"credential_attributes": account.CredentialAttrs,
		"status":                account.Status,
		"status_message":        account.StatusMessage,
		"schedulable":           account.Schedulable,
		"unavailable":           account.Unavailable,
		"models":                account.Models,
		"group":                 account.Group,
		"priority":              account.Priority,
		"weight":                account.Weight,
		"max_concurrency":       account.MaxConcurrency,
		"proxy":                 account.Proxy,
		"base_url":              account.BaseURL,
		"openai_organization":   account.OpenAIOrganization,
		"other":                 account.Other,
		"setting":               account.Setting,
		"settings":              account.OtherSettings,
		"model_mapping":         account.ModelMapping,
		"param_override":        account.ParamOverride,
		"header_override":       account.HeaderOverride,
		"status_code_mapping":   account.StatusCodeMapping,
		"last_used_time":        account.LastUsedTime,
		"used_quota":            account.UsedQuota,
		"rate_limited_until":    account.RateLimitedUntil,
		"overload_until":        account.OverloadUntil,
		"temp_disabled_until":   account.TempDisabledUntil,
		"disabled_reason":       account.DisabledReason,
		"last_error":            account.LastError,
		"quota_snapshot":        account.QuotaSnapshot,
		"model_states":          account.ModelStates,
		"last_refreshed_time":   account.LastRefreshedTime,
		"next_refresh_time":     account.NextRefreshTime,
		"next_retry_time":       account.NextRetryTime,
		"success_count":         account.SuccessCount,
		"failed_count":          account.FailedCount,
		"recent_requests":       account.RecentRequests,
		"runtime":               accountauth.RuntimeView(account),
		"created_time":          account.CreatedTime,
		"updated_time":          account.UpdatedTime,
	}
}

func accountPoolCodexOAuthSessionKey(groupID int, field string) string {
	return fmt.Sprintf("account_pool_codex_oauth_%s_%d", field, groupID)
}

func mergeAccountPoolAuthFileGroups(groups []string, single string) []string {
	values := append([]string{}, groups...)
	if strings.TrimSpace(single) != "" {
		values = append(values, single)
	}
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		for _, part := range splitAccountPoolAuthFileGroups(value) {
			if _, ok := seen[part]; ok {
				continue
			}
			seen[part] = struct{}{}
			result = append(result, part)
		}
	}
	return result
}

func splitAccountPoolAuthFileGroups(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '\n' || r == ';'
	})
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.Join(strings.Fields(strings.TrimSpace(part)), " ")
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}

func stringPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
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
