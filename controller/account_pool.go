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
	"io"
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
	Name              string  `json:"name"`                // 分组名称
	Platform          string  `json:"platform"`            // 平台标识（如 openai, anthropic）
	AuthType          string  `json:"auth_type"`           // 认证类型（api_key, oauth 等）
	Status            *int    `json:"status"`              // 状态（启用/禁用）
	Strategy          string  `json:"strategy"`            // 调度策略（round_robin、random、weighted、fill_first、least_used、success_rate 等）
	Models            string  `json:"models"`              // 支持的模型列表
	Group             string  `json:"group"`               // 用户组
	ModelMapping      *string `json:"model_mapping"`       // 模型映射
	Settings          string  `json:"settings"`            // 其他配置
	MaxConcurrency    *int    `json:"max_concurrency"`     // 分组最大并发数，0 表示不限
	RateLimitRpm      *int    `json:"rate_limit_rpm"`      // 分组每分钟最大请求数，0 表示不限
	DailyRequestLimit *int64  `json:"daily_request_limit"` // 分组每日最大请求数，0 表示不限
	DailyQuotaLimit   *int64  `json:"daily_quota_limit"`   // 分组每日最大配额消耗，0 表示不限
	DailyLimitAction  string  `json:"daily_limit_action"`  // 账号继承的每日限制耗尽处理策略
	// AutoCheckEnabled 控制是否由后台定时任务为该分组创建账号可用性检测任务。
	AutoCheckEnabled *bool `json:"auto_check_enabled"`
	// AutoCheckIntervalMinutes 是自动检测间隔，单位分钟；小于等于 0 时由模型层回退默认值。
	AutoCheckIntervalMinutes *int `json:"auto_check_interval_minutes"`
	// AutoCheckLimit 是单次自动检测最多覆盖的账号数；超过全局上限时由模型层截断。
	AutoCheckLimit *int `json:"auto_check_limit"`
	// PreflightCheckMode 控制 Relay 选号前是否根据最近检测结果过滤或预热账号。
	PreflightCheckMode string `json:"preflight_check_mode"`
	// PreflightCheckFreshnessMinutes 是 last_checked_time 的有效窗口，单位分钟。
	PreflightCheckFreshnessMinutes *int `json:"preflight_check_freshness_minutes"`
	// PreflightCheckLimit 是运行前预热任务最多覆盖的账号数。
	PreflightCheckLimit *int `json:"preflight_check_limit"`
	// NoAvailableAction 控制分组或账号并发满时立即失败还是短暂等待。
	NoAvailableAction string `json:"no_available_action"`
	// NoAvailableWaitSeconds 是等待策略的最长等待秒数。
	NoAvailableWaitSeconds *int `json:"no_available_wait_seconds"`
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
	RateLimitRpm       *int    `json:"rate_limit_rpm"`      // 每分钟最大请求数，0 表示不限
	DailyRequestLimit  *int64  `json:"daily_request_limit"` // 每日最大请求数，0 表示不限
	DailyQuotaLimit    *int64  `json:"daily_quota_limit"`   // 每日最大配额消耗，0 表示不限
	DailyLimitAction   string  `json:"daily_limit_action"`  // 每日限制耗尽处理策略，空值表示继承分组
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
	Credentials       string `json:"credentials"`         // 批量凭证（每行一个）
	Keys              string `json:"keys"`                // 批量密钥（兼容旧格式）
	NamePrefix        string `json:"name_prefix"`         // 名称前缀
	Platform          string `json:"platform"`            // 平台标识
	AuthType          string `json:"auth_type"`           // 认证类型
	Models            string `json:"models"`              // 支持的模型
	Group             string `json:"group"`               // 用户组
	Priority          int64  `json:"priority"`            // 优先级
	Weight            int    `json:"weight"`              // 权重
	Status            int    `json:"status"`              // 状态
	MaxConcurrency    int    `json:"max_concurrency"`     // 最大并发数
	RateLimitRpm      int    `json:"rate_limit_rpm"`      // 每分钟最大请求数，0 表示不限
	DailyRequestLimit int64  `json:"daily_request_limit"` // 每日最大请求数，0 表示不限
	DailyQuotaLimit   int64  `json:"daily_quota_limit"`   // 每日最大配额消耗，0 表示不限
	DailyLimitAction  string `json:"daily_limit_action"`  // 每日限制耗尽处理策略，空值表示继承分组
}

// poolAccountAttachRequest 描述把已有凭证或其他账号组账号添加到当前账号组的请求。
// auth_file_ids 用于从“凭证”列表批量选择；source_group_id 用于复用另一个账号组的同一批账号。
type poolAccountAttachRequest struct {
	AuthFileIDs   []int `json:"auth_file_ids"`   // 待添加的认证文件 ID 列表
	SourceGroupID int   `json:"source_group_id"` // 可选，复制该账号组下的账号到当前组
	SkipExisting  *bool `json:"skip_existing"`   // 是否跳过目标组中已存在的同源凭证，默认 true
}

// poolAccountStatusRequest 池账号状态更新请求
type poolAccountStatusRequest struct {
	Status        int    `json:"status"`         // 新状态
	Reason        string `json:"reason"`         // 状态变更原因
	ClearCooldown bool   `json:"clear_cooldown"` // 是否清除冷却时间
	Schedulable   *bool  `json:"schedulable"`    // 是否可调度
}

// poolAccountBatchStatusRequest 池账号批量状态更新请求。
// account_ids 必须来自当前分组；接口会逐个校验归属，避免跨组误操作。
type poolAccountBatchStatusRequest struct {
	AccountIDs    []int  `json:"account_ids"`    // 待更新的账号 ID 列表
	Status        int    `json:"status"`         // 新状态；仅清冷却时可为 0
	Reason        string `json:"reason"`         // 状态变更原因
	ClearCooldown bool   `json:"clear_cooldown"` // 是否同步清除冷却和临时不可用状态
	Schedulable   *bool  `json:"schedulable"`    // 是否可调度
}

// poolAccountBatchStatusItem 描述批量状态操作中单个账号的结果。
type poolAccountBatchStatusItem struct {
	AccountID   int    `json:"account_id"`
	AccountName string `json:"account_name,omitempty"`
	Success     bool   `json:"success"`
	Skipped     bool   `json:"skipped"`
	Message     string `json:"message,omitempty"`
}

// poolAccountBatchStatusResult 汇总批量状态操作结果。
type poolAccountBatchStatusResult struct {
	Total   int                           `json:"total"`
	Updated int                           `json:"updated"`
	Skipped int                           `json:"skipped"`
	Failed  int                           `json:"failed"`
	Items   []*poolAccountBatchStatusItem `json:"items"`
}

// poolAccountBatchDeleteRequest 池账号批量删除请求。
// account_ids 必须来自当前分组；接口逐个校验归属，避免跨组误删。
type poolAccountBatchDeleteRequest struct {
	AccountIDs []int  `json:"account_ids"` // 待删除的账号 ID 列表
	Reason     string `json:"reason"`      // 删除原因，用于状态审计日志
}

// poolAccountBatchDeleteItem 描述批量删除操作中单个账号的结果。
type poolAccountBatchDeleteItem struct {
	AccountID   int    `json:"account_id"`
	AccountName string `json:"account_name,omitempty"`
	Success     bool   `json:"success"`
	Skipped     bool   `json:"skipped"`
	Message     string `json:"message,omitempty"`
}

// poolAccountBatchDeleteResult 汇总批量删除操作结果。
type poolAccountBatchDeleteResult struct {
	Total   int                           `json:"total"`
	Deleted int                           `json:"deleted"`
	Skipped int                           `json:"skipped"`
	Failed  int                           `json:"failed"`
	Items   []*poolAccountBatchDeleteItem `json:"items"`
}

// poolAccountBatchExportRequest 池账号导出请求。
// account_ids 为空时导出当前分组全部账号；非空时只导出当前分组中匹配的账号。
type poolAccountBatchExportRequest struct {
	AccountIDs []int `json:"account_ids"` // 可选的账号 ID 列表
}

// poolAccountBatchExportResult 是账号池安全导出结果。
// 导出结果只包含调度、状态、统计和脱敏凭证摘要，不包含明文凭据、OAuth 元数据、
// 代理地址、请求覆盖配置等可能包含敏感信息的字段。
type poolAccountBatchExportResult struct {
	ExportedAt              int64                    `json:"exported_at"`
	Format                  string                   `json:"format"`
	PoolGroup               gin.H                    `json:"pool_group"`
	Total                   int                      `json:"total"`
	Exported                int                      `json:"exported"`
	Skipped                 int                      `json:"skipped"`
	SkippedAccountIDs       []int                    `json:"skipped_account_ids,omitempty"`
	CredentialsExported     bool                     `json:"credentials_exported"`
	SensitiveFieldsRedacted []string                 `json:"sensitive_fields_redacted"`
	Accounts                []*poolAccountExportItem `json:"accounts"`
}

// poolAccountStateLogAuditExportResult 是账号池状态审计日志的安全导出结果。
// 状态日志只包含账号状态快照和操作原因，不包含凭证明文；这里仍使用独立导出 DTO，
// 避免未来模型字段增加时被导出接口意外透出。
type poolAccountStateLogAuditExportResult struct {
	ExportedAt              int64                            `json:"exported_at"`
	Format                  string                           `json:"format"`
	Total                   int64                            `json:"total"`
	Exported                int                              `json:"exported"`
	Limit                   int                              `json:"limit"`
	Filters                 gin.H                            `json:"filters"`
	SensitiveFieldsRedacted []string                         `json:"sensitive_fields_redacted"`
	Logs                    []*poolAccountStateLogExportItem `json:"logs"`
}

// poolAccountStateLogExportItem 是单条状态审计日志的导出快照。
type poolAccountStateLogExportItem struct {
	ID                   int    `json:"id"`
	CreatedAt            int64  `json:"created_at"`
	PoolGroupID          int    `json:"pool_group_id"`
	PoolGroupName        string `json:"pool_group_name"`
	PoolAccountID        int    `json:"pool_account_id"`
	PoolAccountName      string `json:"pool_account_name"`
	PoolAccountAuthType  string `json:"pool_account_auth_type"`
	Action               string `json:"action"`
	Source               string `json:"source"`
	Actor                string `json:"actor"`
	Reason               string `json:"reason"`
	BeforeStatus         int    `json:"before_status"`
	AfterStatus          int    `json:"after_status"`
	BeforeSchedulable    bool   `json:"before_schedulable"`
	AfterSchedulable     bool   `json:"after_schedulable"`
	BeforeUnavailable    bool   `json:"before_unavailable"`
	AfterUnavailable     bool   `json:"after_unavailable"`
	BeforeNextRetryTime  int64  `json:"before_next_retry_time"`
	AfterNextRetryTime   int64  `json:"after_next_retry_time"`
	BeforeStatusMessage  string `json:"before_status_message"`
	AfterStatusMessage   string `json:"after_status_message"`
	BeforeDisabledReason string `json:"before_disabled_reason"`
	AfterDisabledReason  string `json:"after_disabled_reason"`
	RequestID            string `json:"request_id,omitempty"`
}

// poolAccountExportItem 是单个账号的安全导出快照。
// 这里有意不复用 PoolAccount 模型，避免未来模型新增敏感字段时被导出接口意外透出。
type poolAccountExportItem struct {
	ID                   int     `json:"id"`
	PoolGroupID          int     `json:"pool_group_id"`
	Name                 string  `json:"name"`
	Platform             string  `json:"platform"`
	AuthType             string  `json:"auth_type"`
	CredentialSummary    string  `json:"credential_summary"`
	CredentialProvider   string  `json:"credential_provider"`
	CredentialLabel      string  `json:"credential_label"`
	Status               int     `json:"status"`
	StatusMessage        string  `json:"status_message"`
	Schedulable          bool    `json:"schedulable"`
	Unavailable          bool    `json:"unavailable"`
	Models               string  `json:"models"`
	Group                string  `json:"group"`
	Priority             int64   `json:"priority"`
	Weight               int     `json:"weight"`
	MaxConcurrency       int     `json:"max_concurrency"`
	RateLimitRpm         int     `json:"rate_limit_rpm"`
	DailyRequestLimit    int64   `json:"daily_request_limit"`
	DailyQuotaLimit      int64   `json:"daily_quota_limit"`
	DailyLimitAction     string  `json:"daily_limit_action"`
	DailyRequestCount    int64   `json:"daily_request_count"`
	DailyUsedQuota       int64   `json:"daily_used_quota"`
	DailyResetTime       int64   `json:"daily_reset_time"`
	ProxyConfigured      bool    `json:"proxy_configured"`
	BaseURLConfigured    bool    `json:"base_url_configured"`
	OpenAIOrganization   *string `json:"openai_organization,omitempty"`
	HasOtherSettings     bool    `json:"has_other_settings"`
	HasModelMapping      bool    `json:"has_model_mapping"`
	HasParamOverride     bool    `json:"has_param_override"`
	HasHeaderOverride    bool    `json:"has_header_override"`
	HasStatusCodeMapping bool    `json:"has_status_code_mapping"`
	LastUsedTime         int64   `json:"last_used_time"`
	UsedQuota            int64   `json:"used_quota"`
	RateLimitedUntil     int64   `json:"rate_limited_until"`
	OverloadUntil        int64   `json:"overload_until"`
	TempDisabledUntil    int64   `json:"temp_disabled_until"`
	DisabledReason       string  `json:"disabled_reason"`
	LastError            string  `json:"last_error"`
	LastCheckedTime      int64   `json:"last_checked_time"`
	LastRefreshedTime    int64   `json:"last_refreshed_time"`
	NextRefreshTime      int64   `json:"next_refresh_time"`
	NextRetryTime        int64   `json:"next_retry_time"`
	SuccessCount         int64   `json:"success_count"`
	FailedCount          int64   `json:"failed_count"`
	CreatedTime          int64   `json:"created_time"`
	UpdatedTime          int64   `json:"updated_time"`
}

// poolAccountCheckRequest 池账号人工检测请求。
// account_ids 用于批量检测指定账号；为空时按 limit 检测当前分组前 N 个账号。
type poolAccountCheckRequest struct {
	AccountIDs []int `json:"account_ids"` // 指定检测的账号 ID 列表
	Limit      int   `json:"limit"`       // 未指定账号 ID 时的最大检测数量
}

// poolAccountCheckTaskCleanupRequest 检测任务历史清理请求。
// statuses 只接受 completed/failed；queued/running 即使传入也会被 service 层忽略，避免
// 管理员清理历史时删除仍在队列或正在执行的检测任务。
type poolAccountCheckTaskCleanupRequest struct {
	PoolGroupID     int      `json:"pool_group_id"`    // 可选，限制只清理某个账号池分组
	BeforeTimestamp int64    `json:"before_timestamp"` // 可选，默认清理 7 天前完成的终态任务
	Statuses        []string `json:"statuses"`         // 可选，默认 completed + failed
	Limit           int      `json:"limit"`            // 可选，单次最多由 service 层限制
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

const poolAccountBatchOperationLimit = 100

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
	updates, err := accountPoolGroupUpdateMap(req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
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

func AttachPoolAccountsToGroup(c *gin.Context) {
	groupID, ok := parsePoolGroupIDParam(c)
	if !ok {
		return
	}
	var req poolAccountAttachRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	skipExisting := true
	if req.SkipExisting != nil {
		skipExisting = *req.SkipExisting
	}
	result, err := service.AttachAccountPoolAccounts(service.AccountPoolAttachAccountsOptions{
		TargetGroupID: groupID,
		AuthFileIDs:   req.AuthFileIDs,
		SourceGroupID: req.SourceGroupID,
		SkipExisting:  skipExisting,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
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
	before, err := model.GetPoolAccountById(accountID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Where("id = ?", accountID).Delete(&model.PoolAccount{})
		if result.Error != nil {
			return result.Error
		}
		return detachAccountPoolAuthFilesForDeletedAccount(tx, before)
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordPoolAccountStateFromController(c, accountID, model.PoolAccountStateActionManualDelete, "管理员删除账号", before)
	common.ApiSuccess(c, nil)
}

func UpdatePoolAccountStatus(c *gin.Context) {
	accountID, ok := parsePoolAccountIDParam(c)
	if !ok {
		return
	}
	before, err := model.GetPoolAccountById(accountID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var req poolAccountStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if req.ClearCooldown && req.Status == 0 {
		if err := clearPoolAccountCooldownState(accountID, req.Reason); err != nil {
			common.ApiError(c, err)
			return
		}
		recordPoolAccountStateFromController(c, accountID, model.PoolAccountStateActionManualClearCooldown, req.Reason, before)
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
		_ = clearPoolAccountCooldownState(accountID, req.Reason)
	}
	recordPoolAccountStateFromController(c, accountID, model.PoolAccountStateActionManualStatus, req.Reason, before)
	common.ApiSuccess(c, nil)
}

func BatchUpdatePoolAccountStatus(c *gin.Context) {
	groupID, ok := parsePoolGroupIDParam(c)
	if !ok {
		return
	}
	if !ensureAccountPoolGroupExists(c, groupID) {
		return
	}
	var req poolAccountBatchStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	accountIDs := normalizePoolAccountBatchOperationIDs(req.AccountIDs)
	if len(accountIDs) == 0 {
		common.ApiErrorMsg(c, "account_ids is required")
		return
	}
	if len(accountIDs) > poolAccountBatchOperationLimit {
		common.ApiErrorMsg(c, fmt.Sprintf("account_ids cannot exceed %d", poolAccountBatchOperationLimit))
		return
	}
	if req.Status <= 0 && !req.ClearCooldown {
		common.ApiErrorMsg(c, "status is required")
		return
	}

	accounts, err := loadPoolAccountsForBatchOperation(groupID, accountIDs)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	result := applyPoolAccountBatchStatus(c, accounts, accountIDs, req)
	common.ApiSuccess(c, result)
}

func BatchDeletePoolAccounts(c *gin.Context) {
	groupID, ok := parsePoolGroupIDParam(c)
	if !ok {
		return
	}
	if !ensureAccountPoolGroupExists(c, groupID) {
		return
	}
	var req poolAccountBatchDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	accountIDs := normalizePoolAccountBatchOperationIDs(req.AccountIDs)
	if len(accountIDs) == 0 {
		common.ApiErrorMsg(c, "account_ids is required")
		return
	}
	if len(accountIDs) > poolAccountBatchOperationLimit {
		common.ApiErrorMsg(c, fmt.Sprintf("account_ids cannot exceed %d", poolAccountBatchOperationLimit))
		return
	}

	accounts, err := loadPoolAccountsForBatchOperation(groupID, accountIDs)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	result := applyPoolAccountBatchDelete(c, groupID, accounts, accountIDs, req)
	common.ApiSuccess(c, result)
}

func BatchExportPoolAccounts(c *gin.Context) {
	groupID, ok := parsePoolGroupIDParam(c)
	if !ok {
		return
	}
	group, err := model.GetAccountPoolGroupById(groupID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var req poolAccountBatchExportRequest
	if !bindOptionalPoolAccountExportRequest(c, &req) {
		return
	}
	accountIDs := normalizePoolAccountBatchOperationIDs(req.AccountIDs)
	accounts, skippedAccountIDs, total, err := loadPoolAccountsForBatchExport(groupID, accountIDs)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.AttachAccountPoolGroupStats([]*model.AccountPoolGroup{group})
	result := buildPoolAccountBatchExportResult(group, accounts, skippedAccountIDs, total)
	common.ApiSuccess(c, result)
}

func ListAccountPoolProviders(c *gin.Context) {
	common.ApiSuccess(c, accountauth.DefaultManager().Providers())
}

func GetAccountPoolHealth(c *gin.Context) {
	poolGroupID, _ := strconv.Atoi(c.Query("pool_group_id"))
	abnormalLimit, _ := strconv.Atoi(c.Query("abnormal_limit"))
	auditLimit, _ := strconv.Atoi(c.Query("audit_limit"))
	summary, err := model.GetAccountPoolHealthSummary(model.AccountPoolHealthOptions{
		PoolGroupID:   poolGroupID,
		AbnormalLimit: abnormalLimit,
		AuditLimit:    auditLimit,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, summary)
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

func ListAccountPoolStateLogs(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	logs, total, err := model.GetPoolAccountStateLogs(accountPoolStateLogFilterFromQuery(c, pageInfo.GetStartIdx(), pageInfo.GetPageSize()))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(logs)
	common.ApiSuccess(c, pageInfo)
}

func GetAccountPoolStateLogAuditSummary(c *gin.Context) {
	summary, err := model.GetPoolAccountStateLogAuditSummary(accountPoolStateLogFilterFromQuery(c, 0, 0))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, summary)
}

func ExportAccountPoolStateLogs(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	if limit <= 0 {
		limit = 1000
	}
	if limit > 1000 {
		limit = 1000
	}
	filter := accountPoolStateLogFilterFromQuery(c, 0, limit)
	filter.MaxLimit = 1000
	logs, total, err := model.GetPoolAccountStateLogs(filter)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, buildPoolAccountStateLogAuditExportResult(filter, logs, total, limit))
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
	deletedAccountsBefore := make([]*model.PoolAccount, 0)
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var authFile model.AccountPoolAuthFile
		if err := tx.Where("id = ?", authFileID).First(&authFile).Error; err != nil {
			return err
		}
		if deleteAccount {
			accounts, err := loadAccountsLinkedToAuthFile(tx, &authFile)
			if err != nil {
				return err
			}
			deletedAccountsBefore = accounts
		}
		if err := tx.Where("id = ?", authFileID).Delete(&model.AccountPoolAuthFile{}).Error; err != nil {
			return err
		}
		if deleteAccount && len(deletedAccountsBefore) > 0 {
			accountIDs := make([]int, 0, len(deletedAccountsBefore))
			for _, account := range deletedAccountsBefore {
				accountIDs = append(accountIDs, account.Id)
			}
			if err := tx.Where("id IN ?", accountIDs).Delete(&model.PoolAccount{}).Error; err != nil {
				return err
			}
		}
		if !deleteAccount {
			return tx.Model(&model.PoolAccount{}).
				Where("auth_file_id = ?", authFileID).
				Update("auth_file_id", 0).Error
		}
		return nil
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	for _, account := range deletedAccountsBefore {
		recordPoolAccountStateFromController(c, account.Id, model.PoolAccountStateActionManualDelete, "删除认证文件时同步删除关联账号", account)
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
	before, getErr := model.GetPoolAccountById(accountID)
	if getErr != nil {
		common.ApiError(c, getErr)
		return
	}
	err := model.UpdatePoolAccountErrorState(accountID, map[string]interface{}{
		"unavailable":         false,
		"status_message":      "",
		"last_error":          "",
		"quota_snapshot":      "",
		"model_states":        "",
		"recent_requests":     "",
		"last_checked_time":   0,
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
	recordPoolAccountStateFromController(c, accountID, model.PoolAccountStateActionRuntimeReset, "重置账号运行时状态", before)
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
	recordPoolAccountStateFromController(c, accountID, model.PoolAccountStateActionRefreshSucceeded, "管理员手动刷新账号凭据成功", account)
	updated, err := model.GetPoolAccountById(accountID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, poolAccountResponse(updated))
}

func CheckPoolAccount(c *gin.Context) {
	accountID, ok := parsePoolAccountIDParam(c)
	if !ok {
		return
	}
	result, err := service.CheckPoolAccount(c.Request.Context(), accountID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordPoolAccountCheckStateFromController(c, result)
	common.ApiSuccess(c, result)
}

func CheckPoolAccountsInGroup(c *gin.Context) {
	groupID, ok := parsePoolGroupIDParam(c)
	if !ok {
		return
	}
	var req poolAccountCheckRequest
	if !bindOptionalPoolAccountCheckRequest(c, &req) {
		return
	}
	limit := req.Limit
	if limit <= 0 {
		limit, _ = strconv.Atoi(c.DefaultQuery("limit", "100"))
	}
	var result *service.AccountPoolBatchCheckResult
	var err error
	if len(req.AccountIDs) > 0 {
		result, err = service.CheckPoolAccountsByIDs(c.Request.Context(), groupID, req.AccountIDs)
	} else {
		result, err = service.CheckPoolAccountsInGroup(c.Request.Context(), groupID, limit)
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	for _, item := range result.Items {
		recordPoolAccountCheckStateFromController(c, item)
	}
	common.ApiSuccess(c, result)
}

func StartPoolAccountCheckTask(c *gin.Context) {
	groupID, ok := parsePoolGroupIDParam(c)
	if !ok {
		return
	}
	var req poolAccountCheckRequest
	if !bindOptionalPoolAccountCheckRequest(c, &req) {
		return
	}
	limit := req.Limit
	if limit <= 0 {
		limit, _ = strconv.Atoi(c.DefaultQuery("limit", "100"))
	}
	task, err := service.StartPoolAccountCheckTask(service.AccountPoolCheckTaskOptions{
		PoolGroupID: groupID,
		AccountIDs:  req.AccountIDs,
		Limit:       limit,
		Actor:       strings.TrimSpace(c.GetString("username")),
		RequestID:   strings.TrimSpace(c.GetString(common.RequestIdKey)),
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, task)
}

func GetPoolAccountCheckTask(c *gin.Context) {
	taskID, ok := parsePoolAccountCheckTaskIDParam(c)
	if !ok {
		return
	}
	task, err := service.GetPoolAccountCheckTask(taskID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, task)
}

func ListPoolAccountCheckTasks(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	poolGroupID, _ := strconv.Atoi(c.Query("pool_group_id"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tasks, total, err := service.ListPoolAccountCheckTasks(service.AccountPoolCheckTaskFilter{
		PoolGroupID:    poolGroupID,
		Status:         c.Query("status"),
		Actor:          c.Query("actor"),
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
		Search:         c.Query("search"),
		StartIdx:       pageInfo.GetStartIdx(),
		Limit:          pageInfo.GetPageSize(),
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(tasks)
	common.ApiSuccess(c, pageInfo)
}

func CleanupPoolAccountCheckTasks(c *gin.Context) {
	var req poolAccountCheckTaskCleanupRequest
	if !bindOptionalPoolAccountCheckTaskCleanupRequest(c, &req) {
		return
	}
	deleted, err := service.CleanupPoolAccountCheckTasks(service.AccountPoolCheckTaskRetentionOptions{
		PoolGroupID:     req.PoolGroupID,
		BeforeTimestamp: req.BeforeTimestamp,
		Statuses:        req.Statuses,
		Limit:           req.Limit,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"deleted": deleted})
}

func recordPoolAccountStateFromController(c *gin.Context, accountID int, action string, reason string, before *model.PoolAccount) {
	if accountID <= 0 {
		return
	}
	actor := ""
	requestID := ""
	if c != nil {
		actor = strings.TrimSpace(c.GetString("username"))
		requestID = strings.TrimSpace(c.GetString(common.RequestIdKey))
	}
	model.RecordPoolAccountStateLog(model.PoolAccountStateLogRecord{
		PoolAccountId: accountID,
		Action:        action,
		Source:        "admin",
		Actor:         actor,
		Reason:        reason,
		RequestId:     requestID,
		Before:        before,
	})
}

func recordPoolAccountCheckStateFromController(c *gin.Context, result *service.AccountPoolCheckResult) {
	if result == nil || !result.Checked || result.AccountID <= 0 {
		return
	}
	action := model.PoolAccountStateActionCheckFailed
	if result.Success {
		action = model.PoolAccountStateActionCheckSucceeded
	}
	recordPoolAccountStateFromController(c, result.AccountID, action, result.Message, nil)
}

func bindOptionalPoolAccountCheckRequest(c *gin.Context, req *poolAccountCheckRequest) bool {
	if c == nil || c.Request == nil || req == nil {
		return true
	}
	if c.Request.Body == nil || c.Request.ContentLength == 0 {
		return true
	}
	if err := common.DecodeJson(c.Request.Body, req); err != nil && err != io.EOF {
		common.ApiError(c, err)
		return false
	}
	return true
}

func bindOptionalPoolAccountCheckTaskCleanupRequest(c *gin.Context, req *poolAccountCheckTaskCleanupRequest) bool {
	if c == nil || c.Request == nil || req == nil {
		return true
	}
	if c.Request.Body == nil || c.Request.ContentLength == 0 {
		return true
	}
	if err := common.DecodeJson(c.Request.Body, req); err != nil && err != io.EOF {
		common.ApiError(c, err)
		return false
	}
	return true
}

func bindOptionalPoolAccountExportRequest(c *gin.Context, req *poolAccountBatchExportRequest) bool {
	if c == nil || c.Request == nil || req == nil {
		return true
	}
	if c.Request.Body == nil || c.Request.ContentLength == 0 {
		return true
	}
	if err := common.DecodeJson(c.Request.Body, req); err != nil && err != io.EOF {
		common.ApiError(c, err)
		return false
	}
	return true
}

func normalizePoolAccountBatchOperationIDs(accountIDs []int) []int {
	if len(accountIDs) == 0 {
		return nil
	}
	seen := map[int]bool{}
	result := make([]int, 0, len(accountIDs))
	for _, accountID := range accountIDs {
		if accountID <= 0 || seen[accountID] {
			continue
		}
		seen[accountID] = true
		result = append(result, accountID)
	}
	return result
}

func loadPoolAccountsForBatchOperation(groupID int, accountIDs []int) (map[int]*model.PoolAccount, error) {
	accounts := []*model.PoolAccount{}
	if err := model.DB.
		Where("pool_group_id = ? AND id IN ?", groupID, accountIDs).
		Order("id ASC").
		Find(&accounts).Error; err != nil {
		return nil, err
	}
	result := make(map[int]*model.PoolAccount, len(accounts))
	for _, account := range accounts {
		if account == nil {
			continue
		}
		result[account.Id] = account
	}
	return result, nil
}

func loadPoolAccountsForBatchExport(groupID int, accountIDs []int) ([]*model.PoolAccount, []int, int, error) {
	accounts := []*model.PoolAccount{}
	if len(accountIDs) == 0 {
		if err := model.DB.
			Where("pool_group_id = ?", groupID).
			Order("id ASC").
			Find(&accounts).Error; err != nil {
			return nil, nil, 0, err
		}
		return accounts, nil, len(accounts), nil
	}
	accountMap, err := loadPoolAccountsForBatchOperation(groupID, accountIDs)
	if err != nil {
		return nil, nil, 0, err
	}
	result := make([]*model.PoolAccount, 0, len(accountMap))
	skippedAccountIDs := make([]int, 0)
	for _, accountID := range accountIDs {
		account := accountMap[accountID]
		if account == nil {
			skippedAccountIDs = append(skippedAccountIDs, accountID)
			continue
		}
		result = append(result, account)
	}
	return result, skippedAccountIDs, len(accountIDs), nil
}

func buildPoolAccountBatchExportResult(group *model.AccountPoolGroup, accounts []*model.PoolAccount, skippedAccountIDs []int, total int) *poolAccountBatchExportResult {
	items := make([]*poolAccountExportItem, 0, len(accounts))
	for _, account := range accounts {
		items = append(items, poolAccountExportItemFromAccount(account))
	}
	return &poolAccountBatchExportResult{
		ExportedAt:          common.GetTimestamp(),
		Format:              "nexustok_account_pool_safe_export_v1",
		PoolGroup:           accountPoolGroupResponse(group),
		Total:               total,
		Exported:            len(items),
		Skipped:             len(skippedAccountIDs),
		SkippedAccountIDs:   skippedAccountIDs,
		CredentialsExported: false,
		SensitiveFieldsRedacted: []string{
			"credentials",
			"credential_metadata",
			"credential_attributes",
			"proxy",
			"base_url",
			"other",
			"setting",
			"settings",
			"model_mapping",
			"param_override",
			"header_override",
			"status_code_mapping",
			"quota_snapshot",
			"model_states",
			"recent_requests",
		},
		Accounts: items,
	}
}

func accountPoolStateLogFilterFromQuery(c *gin.Context, startIdx int, limit int) model.PoolAccountStateLogFilter {
	poolGroupID, _ := strconv.Atoi(c.Query("pool_group_id"))
	poolAccountID, _ := strconv.Atoi(c.Query("pool_account_id"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	return model.PoolAccountStateLogFilter{
		PoolGroupId:    poolGroupID,
		PoolAccountId:  poolAccountID,
		Action:         c.Query("action"),
		Source:         c.Query("source"),
		Actor:          c.Query("actor"),
		RequestId:      c.Query("request_id"),
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
		Search:         c.Query("search"),
		StartIdx:       startIdx,
		Limit:          limit,
	}
}

func buildPoolAccountStateLogAuditExportResult(filter model.PoolAccountStateLogFilter, logs []*model.PoolAccountStateLog, total int64, limit int) *poolAccountStateLogAuditExportResult {
	items := make([]*poolAccountStateLogExportItem, 0, len(logs))
	for _, log := range logs {
		items = append(items, poolAccountStateLogExportItemFromLog(log))
	}
	return &poolAccountStateLogAuditExportResult{
		ExportedAt: common.GetTimestamp(),
		Format:     "nexustok_account_pool_state_audit_export_v1",
		Total:      total,
		Exported:   len(items),
		Limit:      limit,
		Filters: gin.H{
			"pool_group_id":   filter.PoolGroupId,
			"pool_account_id": filter.PoolAccountId,
			"action":          strings.TrimSpace(filter.Action),
			"source":          strings.TrimSpace(filter.Source),
			"actor":           strings.TrimSpace(filter.Actor),
			"request_id":      strings.TrimSpace(filter.RequestId),
			"start_timestamp": filter.StartTimestamp,
			"end_timestamp":   filter.EndTimestamp,
			"search":          strings.TrimSpace(filter.Search),
		},
		SensitiveFieldsRedacted: []string{
			"credentials",
			"credential_metadata",
			"credential_attributes",
			"proxy",
			"base_url",
			"other",
			"setting",
			"settings",
			"model_mapping",
			"param_override",
			"header_override",
			"status_code_mapping",
			"quota_snapshot",
			"model_states",
			"recent_requests",
		},
		Logs: items,
	}
}

func poolAccountStateLogExportItemFromLog(log *model.PoolAccountStateLog) *poolAccountStateLogExportItem {
	if log == nil {
		return &poolAccountStateLogExportItem{}
	}
	return &poolAccountStateLogExportItem{
		ID:                   log.Id,
		CreatedAt:            log.CreatedAt,
		PoolGroupID:          log.PoolGroupId,
		PoolGroupName:        log.PoolGroupName,
		PoolAccountID:        log.PoolAccountId,
		PoolAccountName:      log.PoolAccountName,
		PoolAccountAuthType:  log.PoolAccountAuthType,
		Action:               log.Action,
		Source:               log.Source,
		Actor:                log.Actor,
		Reason:               log.Reason,
		BeforeStatus:         log.BeforeStatus,
		AfterStatus:          log.AfterStatus,
		BeforeSchedulable:    log.BeforeSchedulable,
		AfterSchedulable:     log.AfterSchedulable,
		BeforeUnavailable:    log.BeforeUnavailable,
		AfterUnavailable:     log.AfterUnavailable,
		BeforeNextRetryTime:  log.BeforeNextRetryTime,
		AfterNextRetryTime:   log.AfterNextRetryTime,
		BeforeStatusMessage:  log.BeforeStatusMessage,
		AfterStatusMessage:   log.AfterStatusMessage,
		BeforeDisabledReason: log.BeforeDisabledReason,
		AfterDisabledReason:  log.AfterDisabledReason,
		RequestID:            log.RequestId,
	}
}

func poolAccountExportItemFromAccount(account *model.PoolAccount) *poolAccountExportItem {
	if account == nil {
		return &poolAccountExportItem{}
	}
	return &poolAccountExportItem{
		ID:                   account.Id,
		PoolGroupID:          account.PoolGroupId,
		Name:                 account.Name,
		Platform:             account.Platform,
		AuthType:             account.AuthType,
		CredentialSummary:    account.CredentialSummary,
		CredentialProvider:   account.CredentialProvider,
		CredentialLabel:      account.CredentialLabel,
		Status:               account.Status,
		StatusMessage:        account.StatusMessage,
		Schedulable:          account.Schedulable,
		Unavailable:          account.Unavailable,
		Models:               account.Models,
		Group:                account.Group,
		Priority:             account.Priority,
		Weight:               account.Weight,
		MaxConcurrency:       account.MaxConcurrency,
		RateLimitRpm:         account.RateLimitRpm,
		DailyRequestLimit:    account.DailyRequestLimit,
		DailyQuotaLimit:      account.DailyQuotaLimit,
		DailyLimitAction:     model.NormalizeAccountPoolDailyLimitAction(account.DailyLimitAction, true),
		DailyRequestCount:    account.DailyRequestCount,
		DailyUsedQuota:       account.DailyUsedQuota,
		DailyResetTime:       account.DailyResetTime,
		ProxyConfigured:      strings.TrimSpace(account.Proxy) != "",
		BaseURLConfigured:    account.BaseURL != nil && strings.TrimSpace(*account.BaseURL) != "",
		OpenAIOrganization:   account.OpenAIOrganization,
		HasOtherSettings:     strings.TrimSpace(account.Other) != "" || strings.TrimSpace(account.OtherSettings) != "" || (account.Setting != nil && strings.TrimSpace(*account.Setting) != ""),
		HasModelMapping:      account.ModelMapping != nil && strings.TrimSpace(*account.ModelMapping) != "",
		HasParamOverride:     account.ParamOverride != nil && strings.TrimSpace(*account.ParamOverride) != "",
		HasHeaderOverride:    account.HeaderOverride != nil && strings.TrimSpace(*account.HeaderOverride) != "",
		HasStatusCodeMapping: account.StatusCodeMapping != nil && strings.TrimSpace(*account.StatusCodeMapping) != "",
		LastUsedTime:         account.LastUsedTime,
		UsedQuota:            account.UsedQuota,
		RateLimitedUntil:     account.RateLimitedUntil,
		OverloadUntil:        account.OverloadUntil,
		TempDisabledUntil:    account.TempDisabledUntil,
		DisabledReason:       account.DisabledReason,
		LastError:            account.LastError,
		LastCheckedTime:      account.LastCheckedTime,
		LastRefreshedTime:    account.LastRefreshedTime,
		NextRefreshTime:      account.NextRefreshTime,
		NextRetryTime:        account.NextRetryTime,
		SuccessCount:         account.SuccessCount,
		FailedCount:          account.FailedCount,
		CreatedTime:          account.CreatedTime,
		UpdatedTime:          account.UpdatedTime,
	}
}

func applyPoolAccountBatchDelete(c *gin.Context, groupID int, accounts map[int]*model.PoolAccount, accountIDs []int, req poolAccountBatchDeleteRequest) *poolAccountBatchDeleteResult {
	result := &poolAccountBatchDeleteResult{
		Total: len(accountIDs),
		Items: make([]*poolAccountBatchDeleteItem, 0, len(accountIDs)),
	}
	for _, accountID := range accountIDs {
		account := accounts[accountID]
		if account == nil {
			result.Skipped++
			result.Items = append(result.Items, &poolAccountBatchDeleteItem{
				AccountID: accountID,
				Skipped:   true,
				Message:   "account not found in group",
			})
			continue
		}
		item := applySinglePoolAccountBatchDelete(c, groupID, account, req)
		result.Items = append(result.Items, item)
		if item.Success {
			result.Deleted++
		} else if item.Skipped {
			result.Skipped++
		} else {
			result.Failed++
		}
	}
	return result
}

func applySinglePoolAccountBatchDelete(c *gin.Context, groupID int, account *model.PoolAccount, req poolAccountBatchDeleteRequest) *poolAccountBatchDeleteItem {
	item := &poolAccountBatchDeleteItem{
		AccountID:   account.Id,
		AccountName: account.Name,
	}
	before := *account
	deleted := false
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Where("id = ? AND pool_group_id = ?", account.Id, groupID).Delete(&model.PoolAccount{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		deleted = true
		return detachAccountPoolAuthFilesForDeletedAccount(tx, account)
	})
	if err != nil {
		item.Message = err.Error()
		return item
	}
	if !deleted {
		item.Skipped = true
		item.Message = "account not found in group"
		return item
	}
	recordPoolAccountStateFromController(c, account.Id, model.PoolAccountStateActionManualDelete, poolAccountBatchDeleteReason(req), &before)
	item.Success = true
	return item
}

func loadAccountsLinkedToAuthFile(tx *gorm.DB, authFile *model.AccountPoolAuthFile) ([]*model.PoolAccount, error) {
	if tx == nil || authFile == nil || authFile.Id <= 0 {
		return nil, nil
	}
	var accounts []*model.PoolAccount
	query := tx.Where("auth_file_id = ?", authFile.Id)
	if authFile.PoolAccountId > 0 {
		query = query.Or("id = ?", authFile.PoolAccountId)
	}
	if err := query.Order("id ASC").Find(&accounts).Error; err != nil {
		return nil, err
	}
	seen := map[int]struct{}{}
	result := make([]*model.PoolAccount, 0, len(accounts))
	for _, account := range accounts {
		if account == nil || account.Id <= 0 {
			continue
		}
		if _, ok := seen[account.Id]; ok {
			continue
		}
		seen[account.Id] = struct{}{}
		result = append(result, account)
	}
	return result, nil
}

func detachAccountPoolAuthFilesForDeletedAccount(tx *gorm.DB, account *model.PoolAccount) error {
	if tx == nil || account == nil || account.Id <= 0 {
		return nil
	}
	authFileIDs := make([]int, 0, 2)
	if account.AuthFileId > 0 {
		authFileIDs = append(authFileIDs, account.AuthFileId)
	}
	if account.AuthFileId <= 0 {
		var authFile model.AccountPoolAuthFile
		if err := tx.Select("id").Where("pool_account_id = ?", account.Id).First(&authFile).Error; err == nil {
			authFileIDs = append(authFileIDs, authFile.Id)
		} else if err != gorm.ErrRecordNotFound {
			return err
		}
	}
	for _, authFileID := range authFileIDs {
		var remaining model.PoolAccount
		err := tx.Select("id").
			Where("auth_file_id = ?", authFileID).
			Order("id ASC").
			First(&remaining).Error
		if err == nil {
			// 同一凭证仍被其他账号组使用时，只把认证文件的主账号指针切到仍存在的实例。
			// 这样删除某个组内账号不会把凭证页记录置灰，也不会影响其他组继续调度。
			if err := tx.Model(&model.AccountPoolAuthFile{}).
				Where("id = ? AND pool_account_id = ?", authFileID, account.Id).
				Update("pool_account_id", remaining.Id).Error; err != nil {
				return err
			}
			continue
		}
		if err != gorm.ErrRecordNotFound {
			return err
		}
		// 删除池账号时仍保留认证文件的加密原文，避免把管理员导入的凭据备份和来源记录一并误删。
		// 只有该凭证已经没有其他组内调度实例时，才清空主账号 ID 并禁用认证文件。
		if err := tx.Model(&model.AccountPoolAuthFile{}).
			Where("id = ? OR pool_account_id = ?", authFileID, account.Id).
			Updates(map[string]interface{}{
				"pool_account_id": 0,
				"status":          common.ChannelStatusManuallyDisabled,
			}).Error; err != nil {
			return err
		}
	}
	return nil
}

func applyPoolAccountBatchStatus(c *gin.Context, accounts map[int]*model.PoolAccount, accountIDs []int, req poolAccountBatchStatusRequest) *poolAccountBatchStatusResult {
	result := &poolAccountBatchStatusResult{
		Total: len(accountIDs),
		Items: make([]*poolAccountBatchStatusItem, 0, len(accountIDs)),
	}
	for _, accountID := range accountIDs {
		account := accounts[accountID]
		if account == nil {
			result.Skipped++
			result.Items = append(result.Items, &poolAccountBatchStatusItem{
				AccountID: accountID,
				Skipped:   true,
				Message:   "account not found in group",
			})
			continue
		}
		item := applySinglePoolAccountBatchStatus(c, account, req)
		result.Items = append(result.Items, item)
		if item.Success {
			result.Updated++
		} else if item.Skipped {
			result.Skipped++
		} else {
			result.Failed++
		}
	}
	return result
}

func applySinglePoolAccountBatchStatus(c *gin.Context, account *model.PoolAccount, req poolAccountBatchStatusRequest) *poolAccountBatchStatusItem {
	item := &poolAccountBatchStatusItem{
		AccountID:   account.Id,
		AccountName: account.Name,
	}
	before := *account
	reason := poolAccountBatchStatusReason(req)
	action := model.PoolAccountStateActionManualStatus
	if req.ClearCooldown && req.Status == 0 {
		action = model.PoolAccountStateActionManualClearCooldown
		if err := clearPoolAccountCooldownState(account.Id, reason); err != nil {
			item.Message = err.Error()
			return item
		}
		recordPoolAccountStateFromController(c, account.Id, action, reason, &before)
		item.Success = true
		return item
	}
	if err := model.UpdatePoolAccountStatus(account.Id, req.Status, reason, req.Schedulable); err != nil {
		item.Message = err.Error()
		return item
	}
	if req.ClearCooldown {
		_ = clearPoolAccountCooldownState(account.Id, reason)
	}
	recordPoolAccountStateFromController(c, account.Id, action, reason, &before)
	item.Success = true
	return item
}

func poolAccountBatchDeleteReason(req poolAccountBatchDeleteRequest) string {
	reason := strings.TrimSpace(req.Reason)
	if reason != "" {
		return reason
	}
	return "批量删除账号"
}

func poolAccountBatchStatusReason(req poolAccountBatchStatusRequest) string {
	reason := strings.TrimSpace(req.Reason)
	if reason != "" {
		return reason
	}
	if req.ClearCooldown && req.Status == 0 {
		return "批量清理账号冷却状态"
	}
	return "批量修改账号状态"
}

func clearPoolAccountCooldownState(accountID int, reason string) error {
	// 清冷却是管理员恢复账号参与调度的显式操作，因此同时清理所有临时冷却时间、
	// next_retry_time 和临时不可用标记；如果账号仍处于手动禁用或自动禁用状态，
	// status/schedulable 会继续阻止它进入调度候选集，不会被该操作绕过。
	return model.UpdatePoolAccountErrorState(accountID, map[string]interface{}{
		"rate_limited_until":  0,
		"overload_until":      0,
		"temp_disabled_until": 0,
		"next_retry_time":     0,
		"unavailable":         false,
		"last_error":          "",
		"status_message":      "",
		"disabled_reason":     strings.TrimSpace(reason),
	})
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

func parsePoolAccountCheckTaskIDParam(c *gin.Context) (int, bool) {
	taskID, err := strconv.Atoi(c.Param("check_task_id"))
	if err != nil || taskID <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid account check task id"})
		return 0, false
	}
	return taskID, true
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
	maxConcurrency := 0
	if req.MaxConcurrency != nil && *req.MaxConcurrency > 0 {
		maxConcurrency = *req.MaxConcurrency
	}
	rateLimitRpm := 0
	if req.RateLimitRpm != nil && *req.RateLimitRpm > 0 {
		rateLimitRpm = *req.RateLimitRpm
	}
	dailyRequestLimit := int64(0)
	if req.DailyRequestLimit != nil && *req.DailyRequestLimit > 0 {
		dailyRequestLimit = *req.DailyRequestLimit
	}
	dailyQuotaLimit := int64(0)
	if req.DailyQuotaLimit != nil && *req.DailyQuotaLimit > 0 {
		dailyQuotaLimit = *req.DailyQuotaLimit
	}
	dailyLimitAction := model.NormalizeAccountPoolDailyLimitAction(req.DailyLimitAction, false)
	autoCheckEnabled := false
	if req.AutoCheckEnabled != nil {
		autoCheckEnabled = *req.AutoCheckEnabled
	}
	autoCheckIntervalMinutes := model.NormalizeAccountPoolAutoCheckIntervalMinutes(0)
	if req.AutoCheckIntervalMinutes != nil {
		autoCheckIntervalMinutes = *req.AutoCheckIntervalMinutes
		autoCheckIntervalMinutes = model.NormalizeAccountPoolAutoCheckIntervalMinutes(autoCheckIntervalMinutes)
	}
	autoCheckLimit := model.NormalizeAccountPoolAutoCheckLimit(0)
	if req.AutoCheckLimit != nil {
		autoCheckLimit = *req.AutoCheckLimit
		autoCheckLimit = model.NormalizeAccountPoolAutoCheckLimit(autoCheckLimit)
	}
	preflightCheckMode := model.NormalizeAccountPoolPreflightCheckMode(req.PreflightCheckMode)
	preflightCheckFreshnessMinutes := model.NormalizeAccountPoolPreflightCheckFreshnessMinutes(0)
	if req.PreflightCheckFreshnessMinutes != nil {
		preflightCheckFreshnessMinutes = model.NormalizeAccountPoolPreflightCheckFreshnessMinutes(*req.PreflightCheckFreshnessMinutes)
	}
	preflightCheckLimit := model.NormalizeAccountPoolPreflightCheckLimit(0)
	if req.PreflightCheckLimit != nil {
		preflightCheckLimit = model.NormalizeAccountPoolPreflightCheckLimit(*req.PreflightCheckLimit)
	}
	noAvailableAction := model.NormalizeAccountPoolNoAvailableAction(req.NoAvailableAction)
	noAvailableWaitSeconds := model.NormalizeAccountPoolNoAvailableWaitSeconds(0)
	if req.NoAvailableWaitSeconds != nil {
		noAvailableWaitSeconds = model.NormalizeAccountPoolNoAvailableWaitSeconds(*req.NoAvailableWaitSeconds)
	}
	settings := accountPoolGroupRequestSettings(req)
	return &model.AccountPoolGroup{
		Name:                           name,
		Platform:                       platform,
		AuthType:                       authType,
		Source:                         model.AccountPoolGroupSourceNative,
		Status:                         status,
		Strategy:                       strategy,
		Models:                         strings.TrimSpace(req.Models),
		Group:                          strings.TrimSpace(req.Group),
		ModelMapping:                   req.ModelMapping,
		Settings:                       settings,
		MaxConcurrency:                 maxConcurrency,
		RateLimitRpm:                   rateLimitRpm,
		DailyRequestLimit:              dailyRequestLimit,
		DailyQuotaLimit:                dailyQuotaLimit,
		DailyLimitAction:               dailyLimitAction,
		AutoCheckEnabled:               autoCheckEnabled,
		AutoCheckIntervalMinutes:       autoCheckIntervalMinutes,
		AutoCheckLimit:                 autoCheckLimit,
		PreflightCheckMode:             preflightCheckMode,
		PreflightCheckFreshnessMinutes: preflightCheckFreshnessMinutes,
		PreflightCheckLimit:            preflightCheckLimit,
		NoAvailableAction:              noAvailableAction,
		NoAvailableWaitSeconds:         noAvailableWaitSeconds,
	}, nil
}

func accountPoolGroupUpdateMap(req accountPoolGroupUpsertRequest) (map[string]interface{}, error) {
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
	updates["settings"] = accountPoolGroupRequestSettings(req)
	if req.MaxConcurrency != nil {
		maxConcurrency := 0
		if *req.MaxConcurrency > 0 {
			maxConcurrency = *req.MaxConcurrency
		}
		updates["max_concurrency"] = maxConcurrency
	}
	if req.RateLimitRpm != nil {
		rateLimitRpm := 0
		if *req.RateLimitRpm > 0 {
			rateLimitRpm = *req.RateLimitRpm
		}
		updates["rate_limit_rpm"] = rateLimitRpm
	}
	if req.DailyRequestLimit != nil {
		dailyRequestLimit := int64(0)
		if *req.DailyRequestLimit > 0 {
			dailyRequestLimit = *req.DailyRequestLimit
		}
		updates["daily_request_limit"] = dailyRequestLimit
	}
	if req.DailyQuotaLimit != nil {
		dailyQuotaLimit := int64(0)
		if *req.DailyQuotaLimit > 0 {
			dailyQuotaLimit = *req.DailyQuotaLimit
		}
		updates["daily_quota_limit"] = dailyQuotaLimit
	}
	if strings.TrimSpace(req.DailyLimitAction) != "" {
		updates["daily_limit_action"] = model.NormalizeAccountPoolDailyLimitAction(req.DailyLimitAction, false)
	}
	if req.AutoCheckEnabled != nil {
		updates["auto_check_enabled"] = *req.AutoCheckEnabled
	}
	if req.AutoCheckIntervalMinutes != nil {
		updates["auto_check_interval_minutes"] = model.NormalizeAccountPoolAutoCheckIntervalMinutes(*req.AutoCheckIntervalMinutes)
	}
	if req.AutoCheckLimit != nil {
		updates["auto_check_limit"] = model.NormalizeAccountPoolAutoCheckLimit(*req.AutoCheckLimit)
	}
	if strings.TrimSpace(req.PreflightCheckMode) != "" {
		updates["preflight_check_mode"] = model.NormalizeAccountPoolPreflightCheckMode(req.PreflightCheckMode)
	}
	if req.PreflightCheckFreshnessMinutes != nil {
		updates["preflight_check_freshness_minutes"] = model.NormalizeAccountPoolPreflightCheckFreshnessMinutes(*req.PreflightCheckFreshnessMinutes)
	}
	if req.PreflightCheckLimit != nil {
		updates["preflight_check_limit"] = model.NormalizeAccountPoolPreflightCheckLimit(*req.PreflightCheckLimit)
	}
	if strings.TrimSpace(req.NoAvailableAction) != "" {
		updates["no_available_action"] = model.NormalizeAccountPoolNoAvailableAction(req.NoAvailableAction)
	}
	if req.NoAvailableWaitSeconds != nil {
		updates["no_available_wait_seconds"] = model.NormalizeAccountPoolNoAvailableWaitSeconds(*req.NoAvailableWaitSeconds)
	}
	if req.ModelMapping != nil {
		updates["model_mapping"] = *req.ModelMapping
	}
	return updates, nil
}

// accountPoolGroupRequestSettings 返回保存到分组 settings 的 JSON 字符串。
// 历史版本曾把 max_concurrency 写在 settings JSON 中；新版本已有明确列。
// 当请求显式携带 max_concurrency 时，新列应成为唯一来源，否则用户把新字段设为 0
// 想取消限制时，旧 settings.max_concurrency 会继续兜底生效，导致页面语义和调度行为不一致。
func accountPoolGroupRequestSettings(req accountPoolGroupUpsertRequest) string {
	settings := strings.TrimSpace(req.Settings)
	if req.MaxConcurrency == nil {
		return settings
	}
	return removeAccountPoolGroupSetting(settings, "max_concurrency")
}

// removeAccountPoolGroupSetting 从 settings JSON 对象中移除指定键。
// settings 是面向高级用户的扩展配置，可能为空或包含历史手写内容；解析失败时保持原文，
// 避免因为兼容清理逻辑阻断其他字段的保存。
func removeAccountPoolGroupSetting(settings string, key string) string {
	settings = strings.TrimSpace(settings)
	if settings == "" || strings.TrimSpace(key) == "" {
		return settings
	}
	values := map[string]interface{}{}
	if err := common.UnmarshalJsonStr(settings, &values); err != nil {
		return settings
	}
	if _, ok := values[key]; !ok {
		return settings
	}
	delete(values, key)
	if len(values) == 0 {
		return ""
	}
	encoded, err := common.Marshal(values)
	if err != nil {
		return settings
	}
	return string(encoded)
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
	if req.MaxConcurrency != nil && *req.MaxConcurrency > 0 {
		maxConcurrency = *req.MaxConcurrency
	}
	rateLimitRpm := 0
	if req.RateLimitRpm != nil && *req.RateLimitRpm > 0 {
		rateLimitRpm = *req.RateLimitRpm
	}
	dailyRequestLimit := int64(0)
	if req.DailyRequestLimit != nil && *req.DailyRequestLimit > 0 {
		dailyRequestLimit = *req.DailyRequestLimit
	}
	dailyQuotaLimit := int64(0)
	if req.DailyQuotaLimit != nil && *req.DailyQuotaLimit > 0 {
		dailyQuotaLimit = *req.DailyQuotaLimit
	}
	dailyLimitAction := model.NormalizeAccountPoolDailyLimitAction(req.DailyLimitAction, true)
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
		RateLimitRpm:       rateLimitRpm,
		DailyRequestLimit:  dailyRequestLimit,
		DailyQuotaLimit:    dailyQuotaLimit,
		DailyLimitAction:   dailyLimitAction,
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
		maxConcurrency := 0
		if *req.MaxConcurrency > 0 {
			maxConcurrency = *req.MaxConcurrency
		}
		updates["max_concurrency"] = maxConcurrency
	}
	if req.RateLimitRpm != nil {
		rateLimitRpm := 0
		if *req.RateLimitRpm > 0 {
			rateLimitRpm = *req.RateLimitRpm
		}
		updates["rate_limit_rpm"] = rateLimitRpm
	}
	if req.DailyRequestLimit != nil {
		dailyRequestLimit := int64(0)
		if *req.DailyRequestLimit > 0 {
			dailyRequestLimit = *req.DailyRequestLimit
		}
		updates["daily_request_limit"] = dailyRequestLimit
	}
	if req.DailyQuotaLimit != nil {
		dailyQuotaLimit := int64(0)
		if *req.DailyQuotaLimit > 0 {
			dailyQuotaLimit = *req.DailyQuotaLimit
		}
		updates["daily_quota_limit"] = dailyQuotaLimit
	}
	if strings.TrimSpace(req.DailyLimitAction) != "" {
		updates["daily_limit_action"] = model.NormalizeAccountPoolDailyLimitAction(req.DailyLimitAction, true)
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
	maxConcurrency := req.MaxConcurrency
	if maxConcurrency < 0 {
		maxConcurrency = 0
	}
	rateLimitRpm := req.RateLimitRpm
	if rateLimitRpm < 0 {
		rateLimitRpm = 0
	}
	dailyRequestLimit := req.DailyRequestLimit
	if dailyRequestLimit < 0 {
		dailyRequestLimit = 0
	}
	dailyQuotaLimit := req.DailyQuotaLimit
	if dailyQuotaLimit < 0 {
		dailyQuotaLimit = 0
	}
	dailyLimitAction := model.NormalizeAccountPoolDailyLimitAction(req.DailyLimitAction, true)
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
			MaxConcurrency:    maxConcurrency,
			RateLimitRpm:      rateLimitRpm,
			DailyRequestLimit: dailyRequestLimit,
			DailyQuotaLimit:   dailyQuotaLimit,
			DailyLimitAction:  dailyLimitAction,
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
	now := time.Now()
	dailyRequestCount, dailyUsedQuota, dailyResetTime := model.AccountPoolGroupEffectiveDailyUsage(group, now)
	dailyLimitState := model.AccountPoolGroupDailyLimitState(group, now)
	return gin.H{
		"id":                                group.Id,
		"name":                              group.Name,
		"platform":                          group.Platform,
		"auth_type":                         group.AuthType,
		"source":                            group.Source,
		"external_group_key":                group.ExternalKey,
		"status":                            group.Status,
		"strategy":                          group.Strategy,
		"models":                            group.Models,
		"group":                             group.Group,
		"model_mapping":                     group.ModelMapping,
		"settings":                          group.Settings,
		"max_concurrency":                   group.GetMaxConcurrency(),
		"rate_limit_rpm":                    group.RateLimitRpm,
		"daily_request_limit":               group.DailyRequestLimit,
		"daily_quota_limit":                 group.DailyQuotaLimit,
		"daily_limit_action":                group.GetDailyLimitAction(),
		"daily_request_count":               dailyRequestCount,
		"used_quota":                        group.UsedQuota,
		"daily_used_quota":                  dailyUsedQuota,
		"daily_reset_time":                  dailyResetTime,
		"daily_limit_state":                 dailyLimitState,
		"auto_check_enabled":                group.AutoCheckEnabled,
		"auto_check_interval_minutes":       group.GetAutoCheckIntervalMinutes(),
		"auto_check_limit":                  group.GetAutoCheckLimit(),
		"auto_check_last_time":              group.AutoCheckLastTime,
		"auto_check_next_time":              group.AutoCheckNextTime,
		"auto_check_last_task_id":           group.AutoCheckLastTaskId,
		"preflight_check_mode":              group.GetPreflightCheckMode(),
		"preflight_check_freshness_minutes": group.GetPreflightCheckFreshnessMinutes(),
		"preflight_check_limit":             group.GetPreflightCheckLimit(),
		"no_available_action":               group.GetNoAvailableAction(),
		"no_available_wait_seconds":         group.GetNoAvailableWaitSeconds(),
		"created_time":                      group.CreatedTime,
		"updated_time":                      group.UpdatedTime,
		"stats":                             group.Stats,
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
	now := time.Now()
	dailyRequestCount, dailyUsedQuota, dailyResetTime := model.AccountPoolGroupEffectiveDailyUsage(group, now)
	dailyLimitState := model.AccountPoolGroupDailyLimitState(group, now)
	return gin.H{
		"id":                                group.Id,
		"name":                              group.Name,
		"platform":                          group.Platform,
		"auth_type":                         group.AuthType,
		"source":                            group.Source,
		"external_group_key":                group.ExternalKey,
		"strategy":                          group.Strategy,
		"max_concurrency":                   group.GetMaxConcurrency(),
		"rate_limit_rpm":                    group.RateLimitRpm,
		"daily_request_limit":               group.DailyRequestLimit,
		"daily_quota_limit":                 group.DailyQuotaLimit,
		"daily_limit_action":                group.GetDailyLimitAction(),
		"daily_request_count":               dailyRequestCount,
		"used_quota":                        group.UsedQuota,
		"daily_used_quota":                  dailyUsedQuota,
		"daily_reset_time":                  dailyResetTime,
		"daily_limit_state":                 dailyLimitState,
		"auto_check_enabled":                group.AutoCheckEnabled,
		"auto_check_interval_minutes":       group.GetAutoCheckIntervalMinutes(),
		"auto_check_limit":                  group.GetAutoCheckLimit(),
		"auto_check_last_time":              group.AutoCheckLastTime,
		"auto_check_next_time":              group.AutoCheckNextTime,
		"auto_check_last_task_id":           group.AutoCheckLastTaskId,
		"preflight_check_mode":              group.GetPreflightCheckMode(),
		"preflight_check_freshness_minutes": group.GetPreflightCheckFreshnessMinutes(),
		"preflight_check_limit":             group.GetPreflightCheckLimit(),
		"no_available_action":               group.GetNoAvailableAction(),
		"no_available_wait_seconds":         group.GetNoAvailableWaitSeconds(),
		"stats":                             group.Stats,
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

var accountPoolSubscriptionTypeMetadataPaths = [][]string{
	{"plan_type"},
	{"planType"},
	{"chatgpt_plan_type"},
	{"chatgptPlanType"},
	{"account_type"},
	{"accountType"},
	{"subscription_type"},
	{"subscriptionType"},
	{"subscription_plan"},
	{"subscriptionPlan"},
	{"tier_id"},
	{"tierId"},
	{"account", "plan_type"},
	{"account", "planType"},
	{"entitlement", "subscription_plan"},
	{"entitlement", "subscriptionPlan"},
	{"subscription", "plan_type"},
	{"subscription", "planType"},
	{"subscription", "type"},
	{"extra", "plan_type"},
	{"extra", "planType"},
	{"extra", "chatgpt_plan_type"},
	{"extra", "chatgptPlanType"},
	{"extra", "account", "plan_type"},
	{"extra", "entitlement", "subscription_plan"},
	{"extra", "subscription", "plan_type"},
	{"extra", "tier_id"},
	{"extra", "tierId"},
}

// accountPoolAuthFileSubscriptionType 从认证文件元数据中派生账号订阅类型。
// 该值只用于管理页展示，不作为权限或调度判定的唯一依据。这里优先读取导入文件中
// 已存在的 plan_type / subscription_plan / tier_id 等字段；如果是 Codex/OpenAI
// OAuth 凭据且元数据缺少显式字段，则尝试从 JWT payload 的 chatgpt_plan_type 中补全。
func accountPoolAuthFileSubscriptionType(authFile *model.AccountPoolAuthFile) string {
	if authFile == nil {
		return ""
	}
	metadata := parseAccountPoolAuthFileMetadata(authFile.CredentialMetadata)
	if value := accountPoolSubscriptionTypeFromMetadata(metadata); value != "" {
		return value
	}
	if accountPoolAuthFileLooksLikeCodex(authFile) {
		for _, path := range [][]string{
			{"id_token"},
			{"access_token"},
			{"token_data", "id_token"},
			{"token_data", "access_token"},
			{"tokenData", "idToken"},
			{"tokenData", "accessToken"},
			{"token", "id_token"},
			{"token", "access_token"},
		} {
			token := accountPoolMetadataStringAtPath(metadata, path...)
			if planType, ok := service.ExtractCodexPlanTypeFromJWT(token); ok {
				return normalizeAccountPoolSubscriptionType(planType)
			}
		}
	}
	// 兼容历史摘要：如果后续摘要中加入非敏感 plan_type，这里无需再改前端。
	summary := parseAccountPoolAuthFileMetadata(authFile.CredentialSummary)
	return accountPoolSubscriptionTypeFromMetadata(summary)
}

func parseAccountPoolAuthFileMetadata(raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var metadata map[string]any
	if err := common.UnmarshalJsonStr(raw, &metadata); err != nil {
		return nil
	}
	return metadata
}

func accountPoolSubscriptionTypeFromMetadata(metadata map[string]any) string {
	if len(metadata) == 0 {
		return ""
	}
	for _, path := range accountPoolSubscriptionTypeMetadataPaths {
		if value := normalizeAccountPoolSubscriptionType(accountPoolMetadataStringAtPath(metadata, path...)); value != "" {
			return value
		}
	}
	return ""
}

func accountPoolMetadataStringAtPath(metadata map[string]any, path ...string) string {
	if len(metadata) == 0 || len(path) == 0 {
		return ""
	}
	var current any = metadata
	for _, key := range path {
		obj, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		value, ok := obj[key]
		if !ok || value == nil {
			return ""
		}
		current = value
	}
	switch value := current.(type) {
	case string:
		return strings.TrimSpace(value)
	case float64:
		return strings.TrimSpace(strconv.FormatFloat(value, 'f', -1, 64))
	case int:
		return strconv.Itoa(value)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", value))
	}
}

func accountPoolAuthFileLooksLikeCodex(authFile *model.AccountPoolAuthFile) bool {
	for _, value := range []string{authFile.Provider, authFile.Platform} {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "codex", "openai":
			return true
		}
	}
	return false
}

func normalizeAccountPoolSubscriptionType(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	value = strings.Trim(value, `"'`)
	value = strings.ReplaceAll(value, "-", "_")
	value = strings.ReplaceAll(value, " ", "_")
	if value == "" || value == "<nil>" {
		return ""
	}
	switch value {
	case "chatgptplus", "chatgpt_plus", "plus":
		return "plus"
	case "chatgptpro", "chatgpt_pro", "professional", "pro":
		return "pro"
	case "chatgptteam", "chatgpt_team", "team":
		return "team"
	case "self_serve_business", "self_serve_business_usage_based", "business":
		return "business"
	case "edu", "education", "chatgpt_k12", "k_12", "k12":
		return "k12"
	case "google_one_free", "aistudio_free", "free":
		return "free"
	case "google_ai_pro", "aistudio_paid":
		return "pro"
	case "gcp_enterprise", "enterprise":
		return "enterprise"
	default:
		return value
	}
}

func accountPoolAuthFileUsageRefs(authFile *model.AccountPoolAuthFile) ([]int, []string, []int) {
	if authFile == nil || authFile.Id <= 0 || model.DB == nil {
		return nil, nil, nil
	}
	var accounts []model.PoolAccount
	query := model.DB.Select("id", "pool_group_id").Where("auth_file_id = ?", authFile.Id)
	if authFile.PoolAccountId > 0 {
		query = query.Or("id = ?", authFile.PoolAccountId)
	}
	if err := query.Order("id ASC").Find(&accounts).Error; err != nil {
		return fallbackAccountPoolAuthFileUsageRefs(authFile)
	}
	groupSeen := map[int]struct{}{}
	accountSeen := map[int]struct{}{}
	groupIDs := make([]int, 0, len(accounts)+1)
	accountIDs := make([]int, 0, len(accounts))
	for _, account := range accounts {
		if account.Id > 0 {
			if _, ok := accountSeen[account.Id]; !ok {
				accountSeen[account.Id] = struct{}{}
				accountIDs = append(accountIDs, account.Id)
			}
		}
		if account.PoolGroupId > 0 {
			if _, ok := groupSeen[account.PoolGroupId]; !ok {
				groupSeen[account.PoolGroupId] = struct{}{}
				groupIDs = append(groupIDs, account.PoolGroupId)
			}
		}
	}
	if len(groupIDs) == 0 && authFile.PoolGroupId > 0 {
		groupIDs = append(groupIDs, authFile.PoolGroupId)
	}
	if len(accountIDs) == 0 && authFile.PoolAccountId > 0 {
		accountIDs = append(accountIDs, authFile.PoolAccountId)
	}
	groupNames := accountPoolGroupNamesByIDs(groupIDs)
	return groupIDs, groupNames, accountIDs
}

func fallbackAccountPoolAuthFileUsageRefs(authFile *model.AccountPoolAuthFile) ([]int, []string, []int) {
	groupIDs := []int{}
	accountIDs := []int{}
	if authFile.PoolGroupId > 0 {
		groupIDs = append(groupIDs, authFile.PoolGroupId)
	}
	if authFile.PoolAccountId > 0 {
		accountIDs = append(accountIDs, authFile.PoolAccountId)
	}
	return groupIDs, accountPoolGroupNamesByIDs(groupIDs), accountIDs
}

func accountPoolGroupNamesByIDs(groupIDs []int) []string {
	if len(groupIDs) == 0 || model.DB == nil {
		return nil
	}
	var groups []model.AccountPoolGroup
	if err := model.DB.Select("id", "name").Where("id IN ?", groupIDs).Find(&groups).Error; err != nil {
		names := make([]string, 0, len(groupIDs))
		for _, groupID := range groupIDs {
			names = append(names, fmt.Sprintf("#%d", groupID))
		}
		return names
	}
	nameByID := map[int]string{}
	for _, group := range groups {
		nameByID[group.Id] = group.Name
	}
	names := make([]string, 0, len(groupIDs))
	for _, groupID := range groupIDs {
		if name := strings.TrimSpace(nameByID[groupID]); name != "" {
			names = append(names, name)
			continue
		}
		names = append(names, fmt.Sprintf("#%d", groupID))
	}
	return names
}

func accountPoolAuthFileResponse(authFile *model.AccountPoolAuthFile) gin.H {
	if authFile == nil {
		return gin.H{}
	}
	groups := splitAccountPoolAuthFileGroups(authFile.AccountGroups)
	poolGroupIDs, poolGroupNames, poolAccountIDs := accountPoolAuthFileUsageRefs(authFile)
	accountGroup := ""
	if len(groups) > 0 {
		accountGroup = groups[0]
	}
	return gin.H{
		"id":                 authFile.Id,
		"name":               authFile.Name,
		"source_platform":    authFile.SourcePlatform,
		"format":             authFile.Format,
		"provider":           authFile.Provider,
		"platform":           authFile.Platform,
		"auth_type":          authFile.AuthType,
		"pool_group_id":      authFile.PoolGroupId,
		"pool_account_id":    authFile.PoolAccountId,
		"pool_group_ids":     poolGroupIDs,
		"pool_group_names":   poolGroupNames,
		"pool_account_ids":   poolAccountIDs,
		"status":             authFile.Status,
		"file_digest":        authFile.FileDigest,
		"credential_summary": authFile.CredentialSummary,
		"subscription_type":  accountPoolAuthFileSubscriptionType(authFile),
		"account_group":      accountGroup,
		"account_groups":     groups,
		"models":             authFile.Models,
		"proxy":              authFile.Proxy,
		"base_url":           authFile.BaseURL,
		"priority":           authFile.Priority,
		"weight":             authFile.Weight,
		"max_concurrency":    authFile.MaxConcurrency,
		"last_imported_time": authFile.LastImportedTime,
		"created_time":       authFile.CreatedTime,
		"updated_time":       authFile.UpdatedTime,
	}
}

func poolAccountResponse(account *model.PoolAccount) gin.H {
	if account == nil {
		return gin.H{}
	}
	return gin.H{
		"id":                  account.Id,
		"pool_group_id":       account.PoolGroupId,
		"auth_file_id":        account.AuthFileId,
		"name":                account.Name,
		"platform":            account.Platform,
		"auth_type":           account.AuthType,
		"credential_summary":  account.CredentialSummary,
		"credential_provider": account.CredentialProvider,
		"credential_label":    account.CredentialLabel,
		"status":              account.Status,
		"status_message":      account.StatusMessage,
		"schedulable":         account.Schedulable,
		"unavailable":         account.Unavailable,
		"models":              account.Models,
		"group":               account.Group,
		"priority":            account.Priority,
		"weight":              account.Weight,
		"max_concurrency":     account.MaxConcurrency,
		"rate_limit_rpm":      account.RateLimitRpm,
		"daily_request_limit": account.DailyRequestLimit,
		"daily_quota_limit":   account.DailyQuotaLimit,
		"daily_limit_action":  model.NormalizeAccountPoolDailyLimitAction(account.DailyLimitAction, true),
		"daily_request_count": account.DailyRequestCount,
		"daily_used_quota":    account.DailyUsedQuota,
		"daily_reset_time":    account.DailyResetTime,
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
		"last_checked_time":   account.LastCheckedTime,
		"last_refreshed_time": account.LastRefreshedTime,
		"next_refresh_time":   account.NextRefreshTime,
		"next_retry_time":     account.NextRetryTime,
		"success_count":       account.SuccessCount,
		"failed_count":        account.FailedCount,
		"recent_requests":     account.RecentRequests,
		"runtime":             accountauth.RuntimeView(account),
		"created_time":        account.CreatedTime,
		"updated_time":        account.UpdatedTime,
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
