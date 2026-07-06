// Package model - account_pool.go
// 该文件定义了账号池（Account Pool）数据模型及相关操作
//
// 主要结构体：
// - AccountPoolGroup：账号池分组，定义一组账号的公共配置和调度策略
// - PoolAccount：池账号，存储单个账号的凭据和状态信息
// - AccountPoolAuthFile：账号池认证文件，保存 JSON 凭据原文和文件级调度配置
//
// 常量定义：
// - 认证类型（AuthType）：api_key、official_oauth、cookie、service_account、custom_json
// - 分组来源（Source）：native（原生）、cliproxyapi（CLI 代理 API）
// - 调度策略（Strategy）：round_robin（轮询）、random（随机）、weighted（加权）、fill_first（优先填满）、least_used（最少使用）、success_rate（成功率优先）
//
// 核心功能：
// - 账号池分组和账号的增删改查
// - 账号凭据的脱敏摘要生成
// - 账号状态管理（启用/禁用/冷却中）
// - 账号使用统计（配额、请求数、成功/失败计数）
package model

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common"
	"gorm.io/gorm"
)

const (
	// AccountPoolAuthTypeAPIKey API Key 认证方式
	AccountPoolAuthTypeAPIKey = "api_key"
	// AccountPoolAuthTypeOfficialOAuth 官方 OAuth 认证方式
	AccountPoolAuthTypeOfficialOAuth = "official_oauth"
	// AccountPoolAuthTypeCookie Cookie 认证方式
	AccountPoolAuthTypeCookie = "cookie"
	// AccountPoolAuthTypeServiceAccount 服务账号认证方式
	AccountPoolAuthTypeServiceAccount = "service_account"
	// AccountPoolAuthTypeCustomJSON 自定义 JSON 认证方式
	AccountPoolAuthTypeCustomJSON = "custom_json"

	// AccountPoolGroupSourceNative 原生分组（手动创建）
	AccountPoolGroupSourceNative = "native"
	// AccountPoolGroupSourceCLIProxyAPI CLI 代理 API 来源
	AccountPoolGroupSourceCLIProxyAPI = "cliproxyapi"

	// AccountPoolStrategyRoundRobin 轮询调度策略
	AccountPoolStrategyRoundRobin = "round_robin"
	// AccountPoolStrategyRandom 随机调度策略
	AccountPoolStrategyRandom = "random"
	// AccountPoolStrategyWeighted 加权调度策略
	AccountPoolStrategyWeighted = "weighted"
	// AccountPoolStrategyFillFirst 优先填满调度策略
	AccountPoolStrategyFillFirst = "fill_first"
	// AccountPoolStrategyLeastUsed 最少使用调度策略
	AccountPoolStrategyLeastUsed = "least_used"
	// AccountPoolStrategySuccessRate 成功率优先调度策略
	AccountPoolStrategySuccessRate = "success_rate"

	// AccountPoolAuthFileFormatNative 原生 JSON 认证文件格式
	AccountPoolAuthFileFormatNative = "native"
	// AccountPoolAuthFileFormatSub2 sub2 导出的 JSON 包装格式
	AccountPoolAuthFileFormatSub2 = "sub2"
	// AccountPoolAuthFileFormatNewAPI NewAPI 导出的 JSON 包装格式
	AccountPoolAuthFileFormatNewAPI = "newapi"

	// AccountPoolDailyLimitTypeRequest 表示每日请求次数限制。
	AccountPoolDailyLimitTypeRequest = "daily_request"
	// AccountPoolDailyLimitTypeQuota 表示每日额度限制。
	AccountPoolDailyLimitTypeQuota = "daily_quota"
	// AccountPoolDailyLimitActionCooldown 表示每日限制耗尽后临时冷却到下一个每日窗口。
	AccountPoolDailyLimitActionCooldown = "cooldown"
	// AccountPoolDailyLimitActionDisable 表示每日限制耗尽后自动禁用账号，等待管理员或检测流程恢复。
	AccountPoolDailyLimitActionDisable = "disable"
	// AccountPoolGroupDailyRequestLimitStatusMessage 表示分组因当日请求次数耗尽而停止调度。
	AccountPoolGroupDailyRequestLimitStatusMessage = "账号池分组今日请求次数已用尽，次日自动恢复"
	// AccountPoolGroupDailyQuotaLimitStatusMessage 表示分组因当日额度耗尽而停止调度。
	AccountPoolGroupDailyQuotaLimitStatusMessage = "账号池分组今日额度已用尽，次日自动恢复"
	// PoolAccountDailyRequestLimitStatusMessage 表示账号因当日请求次数耗尽进入临时冷却。
	// 该状态由账号池每日窗口自动恢复逻辑清除，不能作为人工禁用原因长期保存。
	PoolAccountDailyRequestLimitStatusMessage = "账号池账号今日请求次数已用尽，次日自动恢复"
	// PoolAccountDailyQuotaLimitStatusMessage 表示账号因当日额度耗尽进入临时冷却。
	// 该状态由账号池每日窗口自动恢复逻辑清除，不能作为人工禁用原因长期保存。
	PoolAccountDailyQuotaLimitStatusMessage = "账号池账号今日额度已用尽，次日自动恢复"
	// PoolAccountDailyRequestLimitAutoDisabledStatusMessage 表示账号因当日请求次数耗尽被自动禁用。
	// 自动禁用是管理员显式选择的保护策略，每日窗口重置不会自动恢复该状态。
	PoolAccountDailyRequestLimitAutoDisabledStatusMessage = "账号池账号今日请求次数已用尽，已自动禁用"
	// PoolAccountDailyQuotaLimitAutoDisabledStatusMessage 表示账号因当日额度耗尽被自动禁用。
	// 自动禁用需要管理员手动启用或人工检测成功后恢复，避免高风险账号次日自动回流。
	PoolAccountDailyQuotaLimitAutoDisabledStatusMessage = "账号池账号今日额度已用尽，已自动禁用"

	// PoolAccountStateActionManualStatus 表示管理员手动修改账号启用/禁用状态。
	PoolAccountStateActionManualStatus = "manual_status"
	// PoolAccountStateActionManualClearCooldown 表示管理员手动清理冷却状态。
	PoolAccountStateActionManualClearCooldown = "manual_clear_cooldown"
	// PoolAccountStateActionRuntimeReset 表示管理员重置账号运行时统计和错误状态。
	PoolAccountStateActionRuntimeReset = "runtime_reset"
	// PoolAccountStateActionCheckSucceeded 表示人工检测成功并恢复账号健康状态。
	PoolAccountStateActionCheckSucceeded = "check_succeeded"
	// PoolAccountStateActionCheckFailed 表示人工检测失败并标记账号不可用。
	PoolAccountStateActionCheckFailed = "check_failed"
	// PoolAccountStateActionRelayError 表示 Relay 热路径遇到上游错误并更新账号状态。
	PoolAccountStateActionRelayError = "relay_error"
	// PoolAccountStateActionDailyLimitCooling 表示账号达到每日限制并进入次日冷却。
	PoolAccountStateActionDailyLimitCooling = "daily_limit_cooling"
	// PoolAccountStateActionDailyLimitRecovered 表示每日窗口重置后清理账号每日限制冷却。
	PoolAccountStateActionDailyLimitRecovered = "daily_limit_recovered"
	// PoolAccountStateActionDailyLimitDisabled 表示账号达到每日限制并按策略自动禁用。
	PoolAccountStateActionDailyLimitDisabled = "daily_limit_disabled"
	// PoolAccountStateActionRefreshSucceeded 表示凭据刷新成功并恢复账号状态。
	PoolAccountStateActionRefreshSucceeded = "refresh_succeeded"
	// PoolAccountStateActionRefreshFailed 表示自动刷新失败并标记账号不可用。
	PoolAccountStateActionRefreshFailed = "refresh_failed"
)

var (
	// ErrAccountPoolGroupDailyRequestLimitExceeded 表示账号池分组当天可调度请求次数已耗尽。
	ErrAccountPoolGroupDailyRequestLimitExceeded = errors.New("账号池分组今日请求次数已用尽")
	// ErrAccountPoolGroupDailyQuotaLimitExceeded 表示账号池分组当天可用配额已耗尽。
	ErrAccountPoolGroupDailyQuotaLimitExceeded = errors.New("账号池分组今日额度已用尽")
	// ErrPoolAccountDailyRequestLimitExceeded 表示单个账号当天可调度请求次数已耗尽。
	ErrPoolAccountDailyRequestLimitExceeded = errors.New("账号池账号今日请求次数已用尽")
	// ErrPoolAccountDailyQuotaLimitExceeded 表示单个账号当天可用配额已耗尽。
	ErrPoolAccountDailyQuotaLimitExceeded = errors.New("账号池账号今日额度已用尽")
)

// AccountPoolGroupSettings 保存历史版本写入 Settings JSON 的分组级调度设置。
// 新增组级限制应优先使用 AccountPoolGroup 的明确列；此结构只负责兼容旧数据。
type AccountPoolGroupSettings struct {
	MaxConcurrency int `json:"max_concurrency"` // 分组最大并发数，0 表示不限
}

// AccountPoolGroup 账号池分组模型
// 定义一组账号的公共配置，包括平台、认证类型、调度策略等
type AccountPoolGroup struct {
	Id                int     `json:"id"`                                                                          // 分组 ID
	Name              string  `json:"name" gorm:"type:varchar(255);index;not null"`                                // 分组名称
	Platform          string  `json:"platform" gorm:"type:varchar(64);index;not null"`                             // 平台标识（如 openai、claude）
	AuthType          string  `json:"auth_type" gorm:"type:varchar(64);index;not null"`                            // 认证类型
	Source            string  `json:"source" gorm:"type:varchar(64);default:'native';index"`                       // 分组来源
	ExternalKey       string  `json:"external_group_key" gorm:"column:external_group_key;type:varchar(255);index"` // 外部分组标识
	Status            int     `json:"status" gorm:"default:1;index"`                                               // 状态（1=启用，2=禁用）
	Strategy          string  `json:"strategy" gorm:"type:varchar(64);default:'round_robin'"`                      // 调度策略
	Models            string  `json:"models" gorm:"type:text"`                                                     // 支持的模型列表（逗号分隔）
	Group             string  `json:"group" gorm:"column:group;type:varchar(255);index"`                           // 关联的渠道分组
	ModelMapping      *string `json:"model_mapping" gorm:"type:text"`                                              // 模型映射
	Settings          string  `json:"settings" gorm:"type:text"`                                                   // 额外设置（JSON）
	MaxConcurrency    int     `json:"max_concurrency" gorm:"default:0"`                                            // 分组最大并发数（0=不限）
	RateLimitRpm      int     `json:"rate_limit_rpm" gorm:"default:0"`                                             // 分组每分钟最大请求数（0=不限）
	DailyRequestLimit int64   `json:"daily_request_limit" gorm:"bigint;default:0"`                                 // 分组每日最大请求数（0=不限）
	DailyQuotaLimit   int64   `json:"daily_quota_limit" gorm:"bigint;default:0"`                                   // 分组每日最大配额消耗（0=不限）
	DailyLimitAction  string  `json:"daily_limit_action" gorm:"type:varchar(32);default:'cooldown'"`               // 账号继承的每日限制耗尽处理策略
	DailyRequestCount int64   `json:"daily_request_count" gorm:"bigint;default:0"`                                 // 当日已分配请求数
	UsedQuota         int64   `json:"used_quota" gorm:"bigint;default:0"`                                          // 分组累计消耗配额
	DailyUsedQuota    int64   `json:"daily_used_quota" gorm:"bigint;default:0"`                                    // 当日已消耗配额
	DailyResetTime    int64   `json:"daily_reset_time" gorm:"bigint;default:0;index"`                              // 当日统计窗口开始时间
	CreatedTime       int64   `json:"created_time" gorm:"bigint"`                                                  // 创建时间
	UpdatedTime       int64   `json:"updated_time" gorm:"bigint"`                                                  // 更新时间

	Stats map[string]int64 `json:"stats,omitempty" gorm:"-"` // 统计信息（非持久化，运行时附加）
}

// GetSettings 解析账号池分组级调度设置。
// Settings 为空或 JSON 非法时返回零值配置，调用方应把零值视为“不限制”，
// 这样旧数据不会因为设置缺失而影响调度。
func (group *AccountPoolGroup) GetSettings() AccountPoolGroupSettings {
	settings := AccountPoolGroupSettings{}
	if group == nil || strings.TrimSpace(group.Settings) == "" {
		return settings
	}
	if err := common.UnmarshalJsonStr(group.Settings, &settings); err != nil {
		return AccountPoolGroupSettings{}
	}
	if settings.MaxConcurrency < 0 {
		settings.MaxConcurrency = 0
	}
	return settings
}

// GetMaxConcurrency 返回账号池分组最大并发限制。
// 返回 0 表示该分组不限制并发；大于 0 时，整个分组内所有账号共享该并发上限。
func (group *AccountPoolGroup) GetMaxConcurrency() int {
	if group == nil {
		return 0
	}
	if group.MaxConcurrency > 0 {
		return group.MaxConcurrency
	}
	return group.GetSettings().MaxConcurrency
}

// NormalizeAccountPoolDailyLimitAction 规范化每日限制耗尽后的处理策略。
// allowInherit 仅用于账号级配置：账号空值表示继承所属分组；分组级配置必须落到明确策略。
func NormalizeAccountPoolDailyLimitAction(action string, allowInherit bool) string {
	action = strings.ToLower(strings.TrimSpace(action))
	switch action {
	case AccountPoolDailyLimitActionCooldown:
		return AccountPoolDailyLimitActionCooldown
	case AccountPoolDailyLimitActionDisable:
		return AccountPoolDailyLimitActionDisable
	case "", "inherit":
		if allowInherit {
			return ""
		}
		return AccountPoolDailyLimitActionCooldown
	default:
		if allowInherit {
			return ""
		}
		return AccountPoolDailyLimitActionCooldown
	}
}

// GetDailyLimitAction 返回分组级每日限制耗尽处理策略。
// 历史数据中该字段可能为空；为了保持兼容，空值按“次日冷却自动恢复”处理。
func (group *AccountPoolGroup) GetDailyLimitAction() string {
	if group == nil {
		return AccountPoolDailyLimitActionCooldown
	}
	return NormalizeAccountPoolDailyLimitAction(group.DailyLimitAction, false)
}

// GetDailyLimitAction 返回账号实际生效的每日限制耗尽处理策略。
// 账号字段为空时继承分组；分组缺失或配置异常时回退到冷却，保证旧数据行为不变。
func (account *PoolAccount) GetDailyLimitAction(group *AccountPoolGroup) string {
	if account == nil {
		return AccountPoolDailyLimitActionCooldown
	}
	action := NormalizeAccountPoolDailyLimitAction(account.DailyLimitAction, true)
	if action != "" {
		return action
	}
	if group != nil {
		return group.GetDailyLimitAction()
	}
	if account.PoolGroupId > 0 {
		if loadedGroup, err := GetAccountPoolGroupById(account.PoolGroupId); err == nil && loadedGroup != nil {
			return loadedGroup.GetDailyLimitAction()
		}
	}
	return AccountPoolDailyLimitActionCooldown
}

// PoolAccount 池账号模型
// 存储单个账号的凭据、状态和使用统计信息
type PoolAccount struct {
	Id                 int     `json:"id"`                                                                  // 账号 ID
	PoolGroupId        int     `json:"pool_group_id" gorm:"index;not null"`                                 // 所属分组 ID
	Name               string  `json:"name" gorm:"type:varchar(255);index;not null"`                        // 账号名称
	Platform           string  `json:"platform" gorm:"type:varchar(64);index;not null"`                     // 平台标识
	AuthType           string  `json:"auth_type" gorm:"type:varchar(64);index;not null"`                    // 认证类型
	Credentials        string  `json:"credentials" gorm:"type:text;not null"`                               // 加密存储的凭据
	CredentialSummary  string  `json:"credential_summary" gorm:"type:text"`                                 // 凭据摘要（脱敏）
	CredentialProvider string  `json:"credential_provider" gorm:"type:varchar(64);index"`                   // 凭据提供方
	CredentialLabel    string  `json:"credential_label" gorm:"type:varchar(255)"`                           // 凭据标签
	CredentialMetadata string  `json:"credential_metadata" gorm:"type:text"`                                // 凭据元数据（JSON）
	CredentialAttrs    string  `json:"credential_attributes" gorm:"column:credential_attributes;type:text"` // 凭据属性（JSON）
	Status             int     `json:"status" gorm:"default:1;index"`                                       // 状态
	StatusMessage      string  `json:"status_message" gorm:"type:text"`                                     // 状态说明
	Schedulable        bool    `json:"schedulable" gorm:"default:true;index"`                               // 是否可调度
	Unavailable        bool    `json:"unavailable" gorm:"default:false;index"`                              // 是否不可用
	Models             string  `json:"models" gorm:"type:text"`                                             // 支持的模型
	Group              string  `json:"group" gorm:"column:group;type:varchar(255);index"`                   // 关联的渠道分组
	Priority           int64   `json:"priority" gorm:"bigint;default:0;index"`                              // 优先级
	Weight             int     `json:"weight" gorm:"default:1;index"`                                       // 权重
	MaxConcurrency     int     `json:"max_concurrency" gorm:"default:0"`                                    // 最大并发数（0=不限）
	RateLimitRpm       int     `json:"rate_limit_rpm" gorm:"default:0"`                                     // 每分钟最大请求数（0=不限）
	DailyRequestLimit  int64   `json:"daily_request_limit" gorm:"bigint;default:0"`                         // 每日最大请求数（0=不限）
	DailyQuotaLimit    int64   `json:"daily_quota_limit" gorm:"bigint;default:0"`                           // 每日最大配额消耗（0=不限）
	DailyLimitAction   string  `json:"daily_limit_action" gorm:"type:varchar(32);default:''"`               // 每日限制耗尽处理策略，空值表示继承分组
	DailyRequestCount  int64   `json:"daily_request_count" gorm:"bigint;default:0"`                         // 当日已分配请求数
	DailyUsedQuota     int64   `json:"daily_used_quota" gorm:"bigint;default:0"`                            // 当日已消耗配额
	DailyResetTime     int64   `json:"daily_reset_time" gorm:"bigint;default:0;index"`                      // 当日统计窗口开始时间
	Proxy              string  `json:"proxy" gorm:"type:text"`                                              // 代理地址
	BaseURL            *string `json:"base_url" gorm:"column:base_url;default:''"`                          // 自定义 API 基础 URL
	OpenAIOrganization *string `json:"openai_organization"`                                                 // OpenAI 组织 ID
	Other              string  `json:"other"`                                                               // 其他配置
	Setting            *string `json:"setting" gorm:"type:text"`                                            // 账号级设置
	OtherSettings      string  `json:"settings" gorm:"column:settings;type:text"`                           // 额外设置
	ModelMapping       *string `json:"model_mapping" gorm:"type:text"`                                      // 账号级模型映射
	ParamOverride      *string `json:"param_override" gorm:"type:text"`                                     // 参数覆盖
	HeaderOverride     *string `json:"header_override" gorm:"type:text"`                                    // 请求头覆盖
	StatusCodeMapping  *string `json:"status_code_mapping" gorm:"type:varchar(1024);default:''"`            // 状态码映射
	LastUsedTime       int64   `json:"last_used_time" gorm:"bigint;default:0;index"`                        // 最后使用时间
	UsedQuota          int64   `json:"used_quota" gorm:"bigint;default:0"`                                  // 已用配额
	RateLimitedUntil   int64   `json:"rate_limited_until" gorm:"bigint;default:0;index"`                    // 限流截止时间
	OverloadUntil      int64   `json:"overload_until" gorm:"bigint;default:0;index"`                        // 过载截止时间
	TempDisabledUntil  int64   `json:"temp_disabled_until" gorm:"bigint;default:0;index"`                   // 临时禁用截止时间
	DisabledReason     string  `json:"disabled_reason" gorm:"type:text"`                                    // 禁用原因
	LastError          string  `json:"last_error" gorm:"type:text"`                                         // 最近错误信息
	QuotaSnapshot      string  `json:"quota_snapshot" gorm:"type:text"`                                     // 配额快照
	ModelStates        string  `json:"model_states" gorm:"type:text"`                                       // 模型状态（JSON）
	LastCheckedTime    int64   `json:"last_checked_time" gorm:"bigint;default:0;index"`                     // 最后人工检测时间
	LastRefreshedTime  int64   `json:"last_refreshed_time" gorm:"bigint;default:0;index"`                   // 最后刷新时间
	NextRefreshTime    int64   `json:"next_refresh_time" gorm:"bigint;default:0;index"`                     // 下次刷新时间
	NextRetryTime      int64   `json:"next_retry_time" gorm:"bigint;default:0;index"`                       // 下次重试时间
	SuccessCount       int64   `json:"success_count" gorm:"bigint;default:0"`                               // 成功请求数
	FailedCount        int64   `json:"failed_count" gorm:"bigint;default:0"`                                // 失败请求数
	RecentRequests     string  `json:"recent_requests" gorm:"type:text"`                                    // 最近请求记录
	CreatedTime        int64   `json:"created_time" gorm:"bigint"`                                          // 创建时间
	UpdatedTime        int64   `json:"updated_time" gorm:"bigint"`                                          // 更新时间
}

// AccountPoolAuthFile 账号池认证文件模型。
// 该模型把“导入的 JSON 文件”提升为一等管理对象：原文加密保存，列表和调度只使用
// 脱敏摘要及文件级配置。PoolAccount 仍是热路径实际调度对象，PoolAccountId 用来
// 关联由该文件生成的账号，后续编辑分组、代理、优先级时可同步更新到调度层。
type AccountPoolAuthFile struct {
	Id                 int     `json:"id"`                                                                  // 认证文件 ID
	Name               string  `json:"name" gorm:"type:varchar(255);index;not null"`                        // 文件显示名称
	SourcePlatform     string  `json:"source_platform" gorm:"type:varchar(64);index"`                       // 来源平台，如 sub2、newapi、native
	Format             string  `json:"format" gorm:"type:varchar(64);default:'native';index"`               // 解析格式标识
	Provider           string  `json:"provider" gorm:"type:varchar(64);index;not null"`                     // 凭据提供方，如 codex、xai
	Platform           string  `json:"platform" gorm:"type:varchar(64);index;not null"`                     // 本地调度平台，通常与 Provider 一致
	AuthType           string  `json:"auth_type" gorm:"type:varchar(64);index;not null"`                    // 认证类型
	PoolGroupId        int     `json:"pool_group_id" gorm:"index;not null"`                                 // 关联账号池分组
	PoolAccountId      int     `json:"pool_account_id" gorm:"index"`                                        // 由该文件生成的池账号
	Status             int     `json:"status" gorm:"default:1;index"`                                       // 状态（1=启用，2=禁用）
	FileDigest         string  `json:"file_digest" gorm:"type:varchar(64);uniqueIndex"`                     // 原始 JSON 内容 SHA256，用于去重
	EncryptedContent   string  `json:"-" gorm:"type:text;not null"`                                         // 加密后的认证文件原文
	CredentialSummary  string  `json:"credential_summary" gorm:"type:text"`                                 // 凭据摘要（脱敏）
	CredentialMetadata string  `json:"credential_metadata" gorm:"type:text"`                                // 解析出的元数据 JSON
	CredentialAttrs    string  `json:"credential_attributes" gorm:"column:credential_attributes;type:text"` // 解析出的属性 JSON
	AccountGroups      string  `json:"account_groups" gorm:"type:text"`                                     // 文件级调用分组，逗号分隔
	Models             string  `json:"models" gorm:"type:text"`                                             // 文件级模型限制，逗号分隔
	Proxy              string  `json:"proxy" gorm:"type:text"`                                              // 文件级代理
	BaseURL            *string `json:"base_url" gorm:"column:base_url;default:''"`                          // 文件级基础 URL 覆盖
	Priority           int64   `json:"priority" gorm:"bigint;default:0;index"`                              // 文件级优先级
	Weight             int     `json:"weight" gorm:"default:1;index"`                                       // 文件级权重
	MaxConcurrency     int     `json:"max_concurrency" gorm:"default:0"`                                    // 文件级最大并发数（0=不限）
	LastImportedTime   int64   `json:"last_imported_time" gorm:"bigint;default:0;index"`                    // 最近导入时间
	CreatedTime        int64   `json:"created_time" gorm:"bigint"`                                          // 创建时间
	UpdatedTime        int64   `json:"updated_time" gorm:"bigint"`                                          // 更新时间
}

// PoolAccountUsageLog 记录原生账号池账号在 Relay 热路径中的一次使用结果。
//
// 该表刻意使用普通列而不是复用通用日志里的 JSON admin_info：
// 1. 账号池管理页需要按账号池组、账号、渠道、模型、请求 ID 和成功状态稳定筛选；
// 2. SQLite、MySQL、PostgreSQL 对 JSON 查询能力和索引能力差异较大，普通列更容易跨库兼容；
// 3. 日志库可能与主业务库分离，因此这里保存账号、分组、渠道名称快照，查询时不依赖跨库关联。
type PoolAccountUsageLog struct {
	Id                  int    `json:"id"`
	CreatedAt           int64  `json:"created_at" gorm:"bigint;index:idx_pool_account_usage_created_at"`
	PoolGroupId         int    `json:"pool_group_id" gorm:"index:idx_pool_account_usage_group"`
	PoolGroupName       string `json:"pool_group_name" gorm:"type:varchar(255)"`
	PoolAccountId       int    `json:"pool_account_id" gorm:"index:idx_pool_account_usage_account"`
	PoolAccountName     string `json:"pool_account_name" gorm:"type:varchar(255)"`
	PoolAccountAuthType string `json:"pool_account_auth_type" gorm:"type:varchar(64);index"`
	ChannelId           int    `json:"channel_id" gorm:"index:idx_pool_account_usage_channel"`
	ChannelName         string `json:"channel_name" gorm:"type:varchar(255)"`
	ModelName           string `json:"model_name" gorm:"type:varchar(255);index"`
	UserId              int    `json:"user_id" gorm:"index"`
	Username            string `json:"username" gorm:"type:varchar(255);index"`
	TokenId             int    `json:"token_id" gorm:"index"`
	TokenName           string `json:"token_name" gorm:"type:varchar(255)"`
	Group               string `json:"group" gorm:"column:group;type:varchar(255);index"`
	Quota               int    `json:"quota" gorm:"default:0"`
	PromptTokens        int    `json:"prompt_tokens" gorm:"default:0"`
	CompletionTokens    int    `json:"completion_tokens" gorm:"default:0"`
	UseTime             int    `json:"use_time" gorm:"default:0"`
	IsStream            bool   `json:"is_stream" gorm:"default:false"`
	Success             bool   `json:"success" gorm:"index"`
	StatusCode          int    `json:"status_code" gorm:"default:0;index"`
	ErrorCode           string `json:"error_code" gorm:"type:varchar(128);index"`
	ErrorMessage        string `json:"error_message" gorm:"type:text"`
	RequestId           string `json:"request_id,omitempty" gorm:"type:varchar(64);index:idx_pool_account_usage_request_id"`
	UpstreamRequestId   string `json:"upstream_request_id,omitempty" gorm:"type:varchar(128);index:idx_pool_account_usage_upstream_request_id"`
	RetryIndex          int    `json:"retry_index" gorm:"default:0"`
}

// PoolAccountUsageLogRecord 描述一次账号池使用日志写入请求。
// 调用方只传递当前热路径已经持有的快照值，避免日志写入再反查主库并放大请求延迟。
type PoolAccountUsageLogRecord struct {
	PoolGroupId         int
	PoolGroupName       string
	PoolAccountId       int
	PoolAccountName     string
	PoolAccountAuthType string
	ChannelId           int
	ChannelName         string
	ModelName           string
	UserId              int
	Username            string
	TokenId             int
	TokenName           string
	Group               string
	Quota               int
	PromptTokens        int
	CompletionTokens    int
	UseTime             int
	IsStream            bool
	Success             bool
	StatusCode          int
	ErrorCode           string
	ErrorMessage        string
	RequestId           string
	UpstreamRequestId   string
	RetryIndex          int
}

// PoolAccountUsageLogFilter 是账号池使用日志的分页筛选条件。
// 仅使用普通等值和 LIKE 条件，保证三类数据库行为一致。
type PoolAccountUsageLogFilter struct {
	PoolGroupId       int
	PoolAccountId     int
	ChannelId         int
	UserId            int
	Success           *bool
	StartTimestamp    int64
	EndTimestamp      int64
	ModelName         string
	RequestId         string
	UpstreamRequestId string
	Search            string
	StartIdx          int
	Limit             int
}

// PoolAccountStateLog 记录账号池账号状态的每一次人工或自动变更。
//
// 状态日志和使用日志分开保存：使用日志关注“某次请求是否成功”，状态日志关注“账号为何被禁用、
// 冷却、恢复或重置”。字段全部使用普通列，方便 SQLite、MySQL 和 PostgreSQL 统一筛选。
type PoolAccountStateLog struct {
	Id                   int    `json:"id"`
	CreatedAt            int64  `json:"created_at" gorm:"bigint;index:idx_pool_account_state_created_at"`
	PoolGroupId          int    `json:"pool_group_id" gorm:"index:idx_pool_account_state_group"`
	PoolGroupName        string `json:"pool_group_name" gorm:"type:varchar(255)"`
	PoolAccountId        int    `json:"pool_account_id" gorm:"index:idx_pool_account_state_account"`
	PoolAccountName      string `json:"pool_account_name" gorm:"type:varchar(255)"`
	PoolAccountAuthType  string `json:"pool_account_auth_type" gorm:"type:varchar(64);index"`
	Action               string `json:"action" gorm:"type:varchar(64);index"`
	Source               string `json:"source" gorm:"type:varchar(64);index"`
	Actor                string `json:"actor" gorm:"type:varchar(255);index"`
	Reason               string `json:"reason" gorm:"type:text"`
	BeforeStatus         int    `json:"before_status" gorm:"default:0"`
	AfterStatus          int    `json:"after_status" gorm:"default:0"`
	BeforeSchedulable    bool   `json:"before_schedulable" gorm:"default:false"`
	AfterSchedulable     bool   `json:"after_schedulable" gorm:"default:false"`
	BeforeUnavailable    bool   `json:"before_unavailable" gorm:"default:false"`
	AfterUnavailable     bool   `json:"after_unavailable" gorm:"default:false"`
	BeforeNextRetryTime  int64  `json:"before_next_retry_time" gorm:"bigint;default:0"`
	AfterNextRetryTime   int64  `json:"after_next_retry_time" gorm:"bigint;default:0"`
	BeforeStatusMessage  string `json:"before_status_message" gorm:"type:text"`
	AfterStatusMessage   string `json:"after_status_message" gorm:"type:text"`
	BeforeDisabledReason string `json:"before_disabled_reason" gorm:"type:text"`
	AfterDisabledReason  string `json:"after_disabled_reason" gorm:"type:text"`
	RequestId            string `json:"request_id,omitempty" gorm:"type:varchar(64);index:idx_pool_account_state_request_id"`
}

// PoolAccountStateLogRecord 描述一次账号状态变更日志写入请求。
// Before 可选；传入时会保存变更前快照，未传入则只保存变更后的账号当前状态。
type PoolAccountStateLogRecord struct {
	PoolAccountId int
	Action        string
	Source        string
	Actor         string
	Reason        string
	RequestId     string
	Before        *PoolAccount
}

// PoolAccountStateLogFilter 是账号状态日志的分页筛选条件。
type PoolAccountStateLogFilter struct {
	PoolGroupId    int
	PoolAccountId  int
	Action         string
	Source         string
	Actor          string
	StartTimestamp int64
	EndTimestamp   int64
	Search         string
	StartIdx       int
	Limit          int
}

// BeforeCreate GORM 钩子：创建前自动设置时间和规范化字段
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

// BeforeUpdate GORM 钩子：更新前自动设置更新时间和规范化字段
func (group *AccountPoolGroup) BeforeUpdate(tx *gorm.DB) error {
	_ = tx
	group.UpdatedTime = common.GetTimestamp()
	group.normalize()
	return nil
}

// normalize 规范化分组字段（小写化、去空格、设置默认值）
func (group *AccountPoolGroup) normalize() {
	if group.Status == 0 {
		group.Status = common.ChannelStatusEnabled
	}
	group.Platform = strings.ToLower(strings.TrimSpace(group.Platform))
	group.AuthType = strings.ToLower(strings.TrimSpace(group.AuthType))
	if group.AuthType == "" {
		group.AuthType = AccountPoolAuthTypeAPIKey
	}
	group.Source = strings.ToLower(strings.TrimSpace(group.Source))
	if group.Source == "" {
		group.Source = AccountPoolGroupSourceNative
	}
	group.ExternalKey = strings.TrimSpace(group.ExternalKey)
	group.Strategy = strings.ToLower(strings.TrimSpace(group.Strategy))
	if group.Strategy == "" {
		group.Strategy = AccountPoolStrategyRoundRobin
	}
	if group.MaxConcurrency < 0 {
		group.MaxConcurrency = 0
	}
	if group.RateLimitRpm < 0 {
		group.RateLimitRpm = 0
	}
	if group.DailyRequestLimit < 0 {
		group.DailyRequestLimit = 0
	}
	if group.DailyQuotaLimit < 0 {
		group.DailyQuotaLimit = 0
	}
	group.DailyLimitAction = NormalizeAccountPoolDailyLimitAction(group.DailyLimitAction, false)
	if group.DailyRequestCount < 0 {
		group.DailyRequestCount = 0
	}
	if group.UsedQuota < 0 {
		group.UsedQuota = 0
	}
	if group.DailyUsedQuota < 0 {
		group.DailyUsedQuota = 0
	}
	if group.DailyResetTime < 0 {
		group.DailyResetTime = 0
	}
}

// BeforeCreate GORM 钩子：创建前自动设置时间和规范化字段
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

// BeforeUpdate GORM 钩子：更新前自动设置更新时间和规范化字段
func (account *PoolAccount) BeforeUpdate(tx *gorm.DB) error {
	_ = tx
	account.UpdatedTime = common.GetTimestamp()
	account.normalize()
	return nil
}

// BeforeCreate GORM 钩子：创建认证文件前设置时间戳并规范化字段。
func (authFile *AccountPoolAuthFile) BeforeCreate(tx *gorm.DB) error {
	_ = tx
	now := common.GetTimestamp()
	if authFile.CreatedTime == 0 {
		authFile.CreatedTime = now
	}
	authFile.UpdatedTime = now
	if authFile.LastImportedTime == 0 {
		authFile.LastImportedTime = now
	}
	authFile.normalize()
	return nil
}

// BeforeUpdate GORM 钩子：更新认证文件前刷新时间戳并规范化字段。
func (authFile *AccountPoolAuthFile) BeforeUpdate(tx *gorm.DB) error {
	_ = tx
	authFile.UpdatedTime = common.GetTimestamp()
	authFile.normalize()
	return nil
}

// normalize 规范化认证文件字段，保证列表筛选和调度字段稳定。
func (authFile *AccountPoolAuthFile) normalize() {
	if authFile.Status == 0 {
		authFile.Status = common.ChannelStatusEnabled
	}
	authFile.Name = strings.TrimSpace(authFile.Name)
	authFile.SourcePlatform = strings.ToLower(strings.TrimSpace(authFile.SourcePlatform))
	authFile.Format = strings.ToLower(strings.TrimSpace(authFile.Format))
	if authFile.Format == "" {
		authFile.Format = AccountPoolAuthFileFormatNative
	}
	authFile.Provider = strings.ToLower(strings.TrimSpace(authFile.Provider))
	authFile.Platform = strings.ToLower(strings.TrimSpace(authFile.Platform))
	if authFile.Platform == "" {
		authFile.Platform = authFile.Provider
	}
	authFile.AuthType = strings.ToLower(strings.TrimSpace(authFile.AuthType))
	if authFile.AuthType == "" {
		authFile.AuthType = AccountPoolAuthTypeCustomJSON
	}
	if authFile.Weight <= 0 {
		authFile.Weight = 1
	}
	if authFile.MaxConcurrency < 0 {
		authFile.MaxConcurrency = 0
	}
	authFile.FileDigest = strings.TrimSpace(authFile.FileDigest)
	authFile.AccountGroups = normalizeAccountPoolCSV(authFile.AccountGroups)
	authFile.Models = normalizeAccountPoolCSV(authFile.Models)
	authFile.Proxy = strings.TrimSpace(authFile.Proxy)
}

// normalize 规范化账号字段（小写化、去空格、设置默认值）
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
	if account.MaxConcurrency < 0 {
		account.MaxConcurrency = 0
	}
	if account.RateLimitRpm < 0 {
		account.RateLimitRpm = 0
	}
	if account.DailyRequestLimit < 0 {
		account.DailyRequestLimit = 0
	}
	if account.DailyQuotaLimit < 0 {
		account.DailyQuotaLimit = 0
	}
	account.DailyLimitAction = NormalizeAccountPoolDailyLimitAction(account.DailyLimitAction, true)
	if account.DailyRequestCount < 0 {
		account.DailyRequestCount = 0
	}
	if account.UsedQuota < 0 {
		account.UsedQuota = 0
	}
	if account.DailyUsedQuota < 0 {
		account.DailyUsedQuota = 0
	}
	if account.DailyResetTime < 0 {
		account.DailyResetTime = 0
	}
	account.CredentialLabel = strings.TrimSpace(account.CredentialLabel)
}

// GetWeight 获取账号权重，最小为 1
func (account *PoolAccount) GetWeight() int {
	if account == nil || account.Weight <= 0 {
		return 1
	}
	return account.Weight
}

// IsCoolingDown 判断账号是否处于冷却状态
// 包含限流、过载、临时禁用、重试等待四种冷却条件
func (account *PoolAccount) IsCoolingDown(now int64) bool {
	if account == nil {
		return true
	}
	return account.RateLimitedUntil > now || account.OverloadUntil > now || account.TempDisabledUntil > now || account.NextRetryTime > now
}

// GetDecryptedCredentials 解密并返回账号凭据
func (account *PoolAccount) GetDecryptedCredentials() (string, error) {
	if account == nil {
		return "", nil
	}
	return common.DecryptSensitiveString(account.Credentials)
}

// GetCredentialProvider 获取凭据提供方名称，未设置时回退到平台名
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

// GetCredentialLabel 获取凭据标签，未设置时回退到账号名称
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

// GetBaseURL 获取账号的自定义 API 基础 URL，未设置时返回默认值
func (account *PoolAccount) GetBaseURL(defaultBaseURL string) string {
	if account != nil && account.BaseURL != nil && strings.TrimSpace(*account.BaseURL) != "" {
		return *account.BaseURL
	}
	return defaultBaseURL
}

// GetModelMapping 获取账号的模型映射，未设置时返回默认值
func (account *PoolAccount) GetModelMapping(defaultMapping string) string {
	if account != nil && account.ModelMapping != nil && strings.TrimSpace(*account.ModelMapping) != "" {
		return *account.ModelMapping
	}
	return defaultMapping
}

// GetStatusCodeMapping 获取账号的状态码映射，未设置时返回默认值
func (account *PoolAccount) GetStatusCodeMapping(defaultMapping string) string {
	if account != nil && account.StatusCodeMapping != nil && strings.TrimSpace(*account.StatusCodeMapping) != "" {
		return *account.StatusCodeMapping
	}
	return defaultMapping
}

// GetSetting 获取账号的自定义设置，未设置时返回默认值
func (account *PoolAccount) GetSetting(defaultSetting string) string {
	if account != nil && account.Setting != nil && strings.TrimSpace(*account.Setting) != "" {
		return *account.Setting
	}
	return defaultSetting
}

// GetOtherSettings 获取账号的额外设置，未设置时返回默认值
func (account *PoolAccount) GetOtherSettings(defaultSettings string) string {
	if account != nil && strings.TrimSpace(account.OtherSettings) != "" {
		return account.OtherSettings
	}
	return defaultSettings
}

// GetParamOverride 获取账号的参数覆盖配置，未设置时返回默认值
func (account *PoolAccount) GetParamOverride(defaultOverride *string) *string {
	if account != nil && account.ParamOverride != nil && strings.TrimSpace(*account.ParamOverride) != "" {
		return account.ParamOverride
	}
	return defaultOverride
}

// GetHeaderOverride 获取账号的请求头覆盖配置，未设置时返回默认值
func (account *PoolAccount) GetHeaderOverride(defaultOverride *string) *string {
	if account != nil && account.HeaderOverride != nil && strings.TrimSpace(*account.HeaderOverride) != "" {
		return account.HeaderOverride
	}
	return defaultOverride
}

// NormalizeAccountPoolCredentialSummary 规范化凭据摘要
// 将凭据中的敏感字段（token、key）脱敏后生成摘要
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

// GetAccountPoolGroupById 根据 ID 获取账号池分组
func GetAccountPoolGroupById(groupID int) (*AccountPoolGroup, error) {
	group := &AccountPoolGroup{}
	err := DB.Where("id = ?", groupID).First(group).Error
	return group, err
}

// AccountPoolDailyWindowStart 返回账号池每日统计窗口的开始时间。
// 当前实现以服务器本地时区的当天零点为窗口起点，和系统现有“每日任务”语义保持一致。
func AccountPoolDailyWindowStart(now time.Time) int64 {
	if now.IsZero() {
		now = time.Now()
	}
	local := now.Local()
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
	return start.Unix()
}

// AccountPoolNextDailyWindowStart 返回下一次账号池每日统计窗口的开始时间。
// 每日请求数和每日配额耗尽时，账号会临时冷却到这个时间点，窗口切换后自动恢复调度资格。
func AccountPoolNextDailyWindowStart(now time.Time) int64 {
	windowStart := AccountPoolDailyWindowStart(now)
	return time.Unix(windowStart, 0).Local().AddDate(0, 0, 1).Unix()
}

// AccountPoolDailyLimitState 描述账号池每日限制在当前窗口内的状态。
// 该结构用于管理接口和前端展示，避免前端根据中文错误文本推断限制类型。
type AccountPoolDailyLimitState struct {
	Limited       bool   `json:"limited"`                   // 当前每日窗口是否已经达到限制
	LimitType     string `json:"limit_type,omitempty"`      // daily_request 或 daily_quota
	Reason        string `json:"reason,omitempty"`          // 管理员可直接阅读的状态原因
	WindowStart   int64  `json:"window_start"`              // 当前每日统计窗口开始时间
	NextResetTime int64  `json:"next_reset_time,omitempty"` // 下一次每日窗口开始时间
}

// AccountPoolGroupEffectiveDailyUsage 返回分组在当前每日窗口内用于展示和判断的有效用量。
// 如果数据库中仍保存上一窗口的计数，这里只在内存中归零，避免列表页展示过期的“已耗尽”状态；
// 热路径仍会通过 ResetAccountPoolGroupDailyUsageIfNeeded 执行真实重置，保证调度并发安全。
func AccountPoolGroupEffectiveDailyUsage(group *AccountPoolGroup, now time.Time) (int64, int64, int64) {
	if group == nil {
		return 0, 0, AccountPoolDailyWindowStart(now)
	}
	windowStart := AccountPoolDailyWindowStart(now)
	if group.DailyResetTime == 0 || group.DailyResetTime < windowStart {
		return 0, 0, windowStart
	}
	return group.DailyRequestCount, group.DailyUsedQuota, group.DailyResetTime
}

// AccountPoolGroupDailyLimitState 返回账号池分组的每日请求/额度限制状态。
// 每日请求限制优先于每日额度限制，与 SelectPoolAccount 进入候选筛选前的判断顺序保持一致。
func AccountPoolGroupDailyLimitState(group *AccountPoolGroup, now time.Time) AccountPoolDailyLimitState {
	windowStart := AccountPoolDailyWindowStart(now)
	state := AccountPoolDailyLimitState{
		WindowStart:   windowStart,
		NextResetTime: AccountPoolNextDailyWindowStart(now),
	}
	if group == nil {
		return state
	}
	dailyRequestCount, dailyUsedQuota, effectiveResetTime := AccountPoolGroupEffectiveDailyUsage(group, now)
	state.WindowStart = effectiveResetTime
	if group.DailyRequestLimit > 0 && dailyRequestCount >= group.DailyRequestLimit {
		state.Limited = true
		state.LimitType = AccountPoolDailyLimitTypeRequest
		state.Reason = AccountPoolGroupDailyRequestLimitStatusMessage
		return state
	}
	if group.DailyQuotaLimit > 0 && dailyUsedQuota >= group.DailyQuotaLimit {
		state.Limited = true
		state.LimitType = AccountPoolDailyLimitTypeQuota
		state.Reason = AccountPoolGroupDailyQuotaLimitStatusMessage
	}
	return state
}

// ResetAccountPoolGroupDailyUsageIfNeeded 在每日窗口切换时重置分组日用量。
// 使用条件更新避免并发请求重复覆盖新窗口中的计数；返回值表示本次是否实际执行了重置。
func ResetAccountPoolGroupDailyUsageIfNeeded(groupID int, now time.Time) (bool, error) {
	if groupID <= 0 {
		return false, nil
	}
	windowStart := AccountPoolDailyWindowStart(now)
	result := DB.Model(&AccountPoolGroup{}).
		Where("id = ? AND (daily_reset_time = 0 OR daily_reset_time < ?)", groupID, windowStart).
		Updates(map[string]interface{}{
			"daily_request_count": 0,
			"daily_used_quota":    0,
			"daily_reset_time":    windowStart,
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// ReserveAccountPoolGroupRequest 在请求真正拿到账号前预占一次分组每日请求额度。
// 这里的“请求数”表示分组当天被调度的次数，用于防止热路径在并发下突破每日请求上限。
func ReserveAccountPoolGroupRequest(groupID int) error {
	if groupID <= 0 {
		return nil
	}
	if _, err := ResetAccountPoolGroupDailyUsageIfNeeded(groupID, time.Now()); err != nil {
		return err
	}
	var limit int64
	if err := DB.Model(&AccountPoolGroup{}).Where("id = ?", groupID).Select("daily_request_limit").Find(&limit).Error; err != nil {
		return err
	}
	if limit <= 0 {
		return DB.Model(&AccountPoolGroup{}).
			Where("id = ?", groupID).
			Update("daily_request_count", gorm.Expr("daily_request_count + ?", 1)).Error
	}
	result := DB.Model(&AccountPoolGroup{}).
		Where("id = ? AND daily_request_count < daily_request_limit", groupID).
		Update("daily_request_count", gorm.Expr("daily_request_count + ?", 1))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrAccountPoolGroupDailyRequestLimitExceeded
	}
	return nil
}

// CheckAccountPoolGroupDailyQuotaLimit 检查账号池分组每日配额是否仍可调度。
// daily_quota_limit 为 0 表示不限制；达到上限后，热路径停止选择该分组账号，
// 防止已耗尽额度的账号池组继续进入上游调用。
func CheckAccountPoolGroupDailyQuotaLimit(groupID int) error {
	if groupID <= 0 {
		return nil
	}
	if _, err := ResetAccountPoolGroupDailyUsageIfNeeded(groupID, time.Now()); err != nil {
		return err
	}
	var group AccountPoolGroup
	if err := DB.Select("id", "daily_quota_limit", "daily_used_quota").Where("id = ?", groupID).First(&group).Error; err != nil {
		return err
	}
	if group.DailyQuotaLimit > 0 && group.DailyUsedQuota >= group.DailyQuotaLimit {
		return ErrAccountPoolGroupDailyQuotaLimitExceeded
	}
	return nil
}

// AddAccountPoolGroupUsedQuota 累加账号池分组的累计配额和当日配额。
// 请求数在选号时预占；这里仅记录实际结算出的配额消耗，避免无 usage 的失败请求污染额度。
func AddAccountPoolGroupUsedQuota(groupID int, quota int64) {
	if groupID <= 0 || quota <= 0 {
		return
	}
	if _, err := ResetAccountPoolGroupDailyUsageIfNeeded(groupID, time.Now()); err != nil {
		common.SysLog(fmt.Sprintf("failed to reset account pool group daily usage before quota update: group_id=%d, error=%v", groupID, err))
	}
	updates := map[string]interface{}{
		"used_quota":       gorm.Expr("used_quota + ?", quota),
		"daily_used_quota": gorm.Expr("daily_used_quota + ?", quota),
	}
	if err := DB.Model(&AccountPoolGroup{}).Where("id = ?", groupID).Updates(updates).Error; err != nil {
		common.SysLog(fmt.Sprintf("failed to update account pool group used quota: group_id=%d, quota=%d, error=%v", groupID, quota, err))
	}
}

// ResetPoolAccountDailyUsageIfNeeded 在每日窗口切换时重置单个账号的日用量。
// 这里不修改账号的启用状态，只清空账号池自身的日统计和临时日限额提示；
// 其它风控、失败冷却或人工禁用状态仍由对应字段独立控制。
func ResetPoolAccountDailyUsageIfNeeded(accountID int, now time.Time) (bool, error) {
	if accountID <= 0 {
		return false, nil
	}
	windowStart := AccountPoolDailyWindowStart(now)
	result := DB.Model(&PoolAccount{}).
		Where("id = ? AND (daily_reset_time = 0 OR daily_reset_time < ?)", accountID, windowStart).
		Updates(map[string]interface{}{
			"daily_request_count": 0,
			"daily_used_quota":    0,
			"daily_reset_time":    windowStart,
		})
	if result.Error != nil {
		return false, result.Error
	}
	reset := result.RowsAffected > 0
	if reset {
		if err := ClearPoolAccountDailyLimitCooling(accountID); err != nil {
			return true, err
		}
	}
	return reset, nil
}

// PoolAccountDailyLimitError 返回账号当前是否已经达到账号级每日请求或每日配额限制。
// 调用前会尝试切换每日窗口，保证跨天后的账号能自动恢复参与调度。
func PoolAccountDailyLimitError(account *PoolAccount, now time.Time) error {
	if account == nil {
		return nil
	}
	if reset, err := ResetPoolAccountDailyUsageIfNeeded(account.Id, now); err != nil {
		return err
	} else if reset {
		account.DailyRequestCount = 0
		account.DailyUsedQuota = 0
		account.DailyResetTime = AccountPoolDailyWindowStart(now)
		if IsPoolAccountDailyLimitCooling(account) {
			account.Unavailable = false
			account.StatusMessage = ""
			account.LastError = ""
			account.DisabledReason = ""
			account.NextRetryTime = 0
		}
	}
	if account.DailyRequestLimit > 0 && account.DailyRequestCount >= account.DailyRequestLimit {
		return ErrPoolAccountDailyRequestLimitExceeded
	}
	if account.DailyQuotaLimit > 0 && account.DailyUsedQuota >= account.DailyQuotaLimit {
		return ErrPoolAccountDailyQuotaLimitExceeded
	}
	return nil
}

// PoolAccountDailyLimitReason 将账号每日请求/额度错误转换成可展示的临时冷却原因。
// 返回空字符串表示该错误不属于每日限制，不应由每日窗口恢复逻辑处理。
func PoolAccountDailyLimitReason(limitErr error) string {
	switch {
	case errors.Is(limitErr, ErrPoolAccountDailyRequestLimitExceeded):
		return PoolAccountDailyRequestLimitStatusMessage
	case errors.Is(limitErr, ErrPoolAccountDailyQuotaLimitExceeded):
		return PoolAccountDailyQuotaLimitStatusMessage
	default:
		return ""
	}
}

// PoolAccountDailyLimitDisabledReason 将账号每日请求/额度错误转换成自动禁用原因。
// 自动禁用原因和冷却原因刻意分离，避免每日窗口重置误清需要管理员确认的禁用状态。
func PoolAccountDailyLimitDisabledReason(limitErr error) string {
	switch {
	case errors.Is(limitErr, ErrPoolAccountDailyRequestLimitExceeded):
		return PoolAccountDailyRequestLimitAutoDisabledStatusMessage
	case errors.Is(limitErr, ErrPoolAccountDailyQuotaLimitExceeded):
		return PoolAccountDailyQuotaLimitAutoDisabledStatusMessage
	default:
		return ""
	}
}

// IsPoolAccountDailyLimitCooling 判断账号当前不可用状态是否由每日请求/额度耗尽策略写入。
// 该判断只识别账号池自己写入的固定状态文本，避免误清人工禁用或真实上游错误。
func IsPoolAccountDailyLimitCooling(account *PoolAccount) bool {
	if account == nil {
		return false
	}
	return isPoolAccountDailyLimitCoolingReason(account.StatusMessage) || isPoolAccountDailyLimitCoolingReason(account.DisabledReason)
}

func isPoolAccountDailyLimitCoolingReason(reason string) bool {
	reason = strings.TrimSpace(reason)
	return reason == PoolAccountDailyRequestLimitStatusMessage || reason == PoolAccountDailyQuotaLimitStatusMessage
}

// IsPoolAccountDailyLimitDisabled 判断账号是否已经按每日限制策略被自动禁用。
// 该状态不会随每日窗口自动恢复；人工检测成功或管理员手动启用会走既有恢复流程。
func IsPoolAccountDailyLimitDisabled(account *PoolAccount) bool {
	if account == nil {
		return false
	}
	return isPoolAccountDailyLimitDisabledReason(account.StatusMessage) || isPoolAccountDailyLimitDisabledReason(account.DisabledReason)
}

func isPoolAccountDailyLimitDisabledReason(reason string) bool {
	reason = strings.TrimSpace(reason)
	return reason == PoolAccountDailyRequestLimitAutoDisabledStatusMessage || reason == PoolAccountDailyQuotaLimitAutoDisabledStatusMessage
}

// MarkPoolAccountDailyLimitExceeded 按账号实际策略处理每日请求/额度耗尽。
// 默认策略是冷却到下一日；账号可覆盖为自动禁用，未覆盖时继承所属分组配置。
func MarkPoolAccountDailyLimitExceeded(accountID int, limitErr error, now time.Time) error {
	if accountID <= 0 {
		return nil
	}
	account, err := GetPoolAccountById(accountID)
	if err != nil {
		return err
	}
	if account.GetDailyLimitAction(nil) == AccountPoolDailyLimitActionDisable {
		return MarkPoolAccountDailyLimitDisabled(accountID, limitErr)
	}
	return MarkPoolAccountDailyLimitCooling(accountID, limitErr, now)
}

// MarkPoolAccountDailyLimitCooling 将账号标记为“今日限制耗尽，次日自动恢复”的临时冷却状态。
// 该状态不会改写 status 或 schedulable，只通过 unavailable 和 next_retry_time 阻止继续调度；
// 每日窗口重置时 ClearPoolAccountDailyLimitCooling 会只清理这种固定原因的状态。
func MarkPoolAccountDailyLimitCooling(accountID int, limitErr error, now time.Time) error {
	if accountID <= 0 {
		return nil
	}
	reason := PoolAccountDailyLimitReason(limitErr)
	if reason == "" {
		return nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	before, _ := GetPoolAccountById(accountID)
	recoverAt := AccountPoolNextDailyWindowStart(now)
	shouldRecord := before == nil || !IsPoolAccountDailyLimitCooling(before) || before.NextRetryTime != recoverAt
	if err := DB.Model(&PoolAccount{}).Where("id = ?", accountID).Updates(map[string]interface{}{
		"unavailable":     true,
		"status_message":  reason,
		"last_error":      reason,
		"disabled_reason": reason,
		"next_retry_time": recoverAt,
	}).Error; err != nil {
		return err
	}
	if shouldRecord {
		RecordPoolAccountStateLog(PoolAccountStateLogRecord{
			PoolAccountId: accountID,
			Action:        PoolAccountStateActionDailyLimitCooling,
			Source:        "daily_limit",
			Reason:        reason,
			Before:        before,
		})
	}
	return nil
}

// MarkPoolAccountDailyLimitDisabled 将账号标记为“今日限制耗尽，自动禁用”。
// 自动禁用会改写 status 和 schedulable，避免高风险账号在下一个每日窗口自动回到调度池。
func MarkPoolAccountDailyLimitDisabled(accountID int, limitErr error) error {
	if accountID <= 0 {
		return nil
	}
	reason := PoolAccountDailyLimitDisabledReason(limitErr)
	if reason == "" {
		return nil
	}
	before, _ := GetPoolAccountById(accountID)
	shouldRecord := before == nil || !IsPoolAccountDailyLimitDisabled(before)
	if err := DB.Model(&PoolAccount{}).Where("id = ?", accountID).Updates(map[string]interface{}{
		"status":              common.ChannelStatusAutoDisabled,
		"schedulable":         false,
		"unavailable":         true,
		"status_message":      reason,
		"last_error":          reason,
		"disabled_reason":     reason,
		"next_retry_time":     0,
		"rate_limited_until":  0,
		"overload_until":      0,
		"temp_disabled_until": 0,
	}).Error; err != nil {
		return err
	}
	if shouldRecord {
		RecordPoolAccountStateLog(PoolAccountStateLogRecord{
			PoolAccountId: accountID,
			Action:        PoolAccountStateActionDailyLimitDisabled,
			Source:        "daily_limit",
			Reason:        reason,
			Before:        before,
		})
	}
	return nil
}

// ClearPoolAccountDailyLimitCooling 清理由每日请求/额度耗尽策略写入的临时冷却状态。
// 只有状态原因仍然是固定的每日限制文本时才会清理，避免跨天恢复误伤其它故障状态。
func ClearPoolAccountDailyLimitCooling(accountID int) error {
	if accountID <= 0 {
		return nil
	}
	before, err := GetPoolAccountById(accountID)
	if err != nil || !IsPoolAccountDailyLimitCooling(before) {
		return err
	}
	reasons := []string{PoolAccountDailyRequestLimitStatusMessage, PoolAccountDailyQuotaLimitStatusMessage}
	result := DB.Model(&PoolAccount{}).
		Where("id = ? AND (status_message IN ? OR disabled_reason IN ?)", accountID, reasons, reasons).
		Updates(map[string]interface{}{
			"unavailable":     false,
			"status_message":  "",
			"last_error":      "",
			"disabled_reason": "",
			"next_retry_time": 0,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		RecordPoolAccountStateLog(PoolAccountStateLogRecord{
			PoolAccountId: accountID,
			Action:        PoolAccountStateActionDailyLimitRecovered,
			Source:        "daily_limit",
			Reason:        "每日窗口已重置，清理账号每日限制冷却状态",
			Before:        before,
		})
	}
	return nil
}

// ReservePoolAccountRequest 在账号被真正选中前预占一次账号级每日请求额度。
// 该计数表示账号当天进入 Relay 热路径的次数；即使未配置上限也会记录实际使用次数，
// 便于前端展示“今日请求数 / 无限制”。
func ReservePoolAccountRequest(accountID int) error {
	if accountID <= 0 {
		return nil
	}
	if _, err := ResetPoolAccountDailyUsageIfNeeded(accountID, time.Now()); err != nil {
		return err
	}
	var limit int64
	if err := DB.Model(&PoolAccount{}).Where("id = ?", accountID).Select("daily_request_limit").Find(&limit).Error; err != nil {
		return err
	}
	if limit <= 0 {
		return DB.Model(&PoolAccount{}).
			Where("id = ?", accountID).
			Update("daily_request_count", gorm.Expr("daily_request_count + ?", 1)).Error
	}
	result := DB.Model(&PoolAccount{}).
		Where("id = ? AND daily_request_count < daily_request_limit", accountID).
		Update("daily_request_count", gorm.Expr("daily_request_count + ?", 1))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrPoolAccountDailyRequestLimitExceeded
	}
	return nil
}

// ReleasePoolAccountRequest 回滚一次尚未进入上游调用的账号级请求预占。
// 仅用于选号后续环节失败的内部补偿；请求已经交给上游后，无论成功失败都不应回滚。
func ReleasePoolAccountRequest(accountID int) {
	if accountID <= 0 {
		return
	}
	if err := DB.Model(&PoolAccount{}).
		Where("id = ? AND daily_request_count > 0", accountID).
		Update("daily_request_count", gorm.Expr("daily_request_count - ?", 1)).Error; err != nil {
		common.SysLog(fmt.Sprintf("failed to rollback pool account daily request: account_id=%d, error=%v", accountID, err))
	}
}

// CheckPoolAccountDailyQuotaLimit 检查单个账号每日配额是否仍可调度。
// daily_quota_limit 为 0 表示不限制；达到上限后，调度层会跳过该账号并尝试其它账号。
func CheckPoolAccountDailyQuotaLimit(accountID int) error {
	if accountID <= 0 {
		return nil
	}
	if _, err := ResetPoolAccountDailyUsageIfNeeded(accountID, time.Now()); err != nil {
		return err
	}
	var account PoolAccount
	if err := DB.Select("id", "daily_quota_limit", "daily_used_quota").Where("id = ?", accountID).First(&account).Error; err != nil {
		return err
	}
	if account.DailyQuotaLimit > 0 && account.DailyUsedQuota >= account.DailyQuotaLimit {
		return ErrPoolAccountDailyQuotaLimitExceeded
	}
	return nil
}

// GetAccountPoolGroups 分页查询账号池分组列表
// 支持按状态筛选和关键词搜索（名称、平台、认证类型、模型）
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

// AttachAccountPoolGroupStats 为分组列表附加账号统计信息
// 统计每个分组下的账号总数、启用数、禁用数、冷却中数
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

// CountPoolAccountsByGroupIDs 批量统计多个分组的账号状态
// 返回 map[groupId]map[statusKey]count 的嵌套映射
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
		if account.Unavailable {
			stats["unavailable"]++
		}
		if account.Status != common.ChannelStatusEnabled || !account.Schedulable {
			stats["disabled"]++
			continue
		}
		if account.IsCoolingDown(now) {
			stats["cooldown"]++
			continue
		}
		if !account.Unavailable {
			stats["enabled"]++
		} else {
			stats["disabled"]++
		}
	}
	return result, nil
}

// newPoolAccountStats 创建空的池账号统计映射
func newPoolAccountStats() map[string]int64 {
	return map[string]int64{
		"total":       0,
		"enabled":     0,
		"disabled":    0,
		"cooldown":    0,
		"unavailable": 0,
	}
}

// GetPoolAccountById 根据 ID 获取池账号
func GetPoolAccountById(accountID int) (*PoolAccount, error) {
	account := &PoolAccount{}
	err := DB.Where("id = ?", accountID).First(account).Error
	return account, err
}

// GetAccountPoolAuthFileById 根据 ID 获取认证文件。
func GetAccountPoolAuthFileById(authFileID int) (*AccountPoolAuthFile, error) {
	authFile := &AccountPoolAuthFile{}
	err := DB.Where("id = ?", authFileID).First(authFile).Error
	return authFile, err
}

// GetAccountPoolAuthFiles 分页查询原生账号池认证文件。
// 支持按状态、账号池分组、提供方和关键词筛选，便于后台把“文件”和“生成账号”
// 分开管理，同时仍能定位到实际调度对象。
func GetAccountPoolAuthFiles(page int, pageSize int, status int, poolGroupID int, provider string, search string) ([]*AccountPoolAuthFile, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	query := DB.Model(&AccountPoolAuthFile{})
	if status > 0 {
		query = query.Where("status = ?", status)
	}
	if poolGroupID > 0 {
		query = query.Where("pool_group_id = ?", poolGroupID)
	}
	if strings.TrimSpace(provider) != "" {
		query = query.Where("provider = ? OR platform = ?", strings.ToLower(strings.TrimSpace(provider)), strings.ToLower(strings.TrimSpace(provider)))
	}
	if strings.TrimSpace(search) != "" {
		like := "%" + strings.TrimSpace(search) + "%"
		query = query.Where("name LIKE ? OR provider LIKE ? OR platform LIKE ? OR source_platform LIKE ? OR credential_summary LIKE ? OR account_groups LIKE ?", like, like, like, like, like, like)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	authFiles := []*AccountPoolAuthFile{}
	err := query.Order("id DESC").Limit(pageSize).Offset((page - 1) * pageSize).Find(&authFiles).Error
	return authFiles, total, err
}

// GetPoolAccounts 分页查询池账号列表
// 支持按状态筛选和关键词搜索（名称、凭据摘要、模型）
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

func normalizeAccountPoolCSV(value string) string {
	parts := splitAccountPoolCSV(value)
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ",")
}

func splitAccountPoolCSV(value string) []string {
	seen := map[string]struct{}{}
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '\n' || r == ';'
	})
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.Join(strings.Fields(strings.TrimSpace(part)), " ")
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}

// UpdatePoolAccountStatus 更新池账号状态
// 启用时自动清除限流、过载、临时禁用等状态
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

// UpdatePoolAccountErrorState 更新池账号的错误状态字段
func UpdatePoolAccountErrorState(accountID int, updates map[string]interface{}) error {
	if accountID <= 0 || len(updates) == 0 {
		return nil
	}
	return DB.Model(&PoolAccount{}).Where("id = ?", accountID).Updates(updates).Error
}

// TouchPoolAccount 更新池账号的最后使用时间
func TouchPoolAccount(accountID int) {
	if accountID <= 0 {
		return
	}
	if err := DB.Model(&PoolAccount{}).Where("id = ?", accountID).Update("last_used_time", common.GetTimestamp()).Error; err != nil {
		common.SysLog(fmt.Sprintf("failed to update pool account last_used_time: account_id=%d, error=%v", accountID, err))
	}
}

// AddPoolAccountUsedQuota 增加池账号的累计配额和当日配额。
// 账号级每日配额限制依赖 daily_used_quota；每次结算前先确保日窗口已切换，
// 避免跨天后的用量继续累加到旧窗口。
func AddPoolAccountUsedQuota(accountID int, quota int64) {
	if accountID <= 0 || quota <= 0 {
		return
	}
	if _, err := ResetPoolAccountDailyUsageIfNeeded(accountID, time.Now()); err != nil {
		common.SysLog(fmt.Sprintf("failed to reset pool account daily usage before quota update: account_id=%d, error=%v", accountID, err))
	}
	updates := map[string]interface{}{
		"used_quota":       gorm.Expr("used_quota + ?", quota),
		"daily_used_quota": gorm.Expr("daily_used_quota + ?", quota),
	}
	if err := DB.Model(&PoolAccount{}).Where("id = ?", accountID).Updates(updates).Error; err != nil {
		common.SysLog(fmt.Sprintf("failed to update pool account used_quota: account_id=%d, quota=%d, error=%v", accountID, quota, err))
	}
	var account PoolAccount
	if err := DB.Select("id", "daily_quota_limit", "daily_used_quota").
		Where("id = ?", accountID).First(&account).Error; err != nil {
		common.SysLog(fmt.Sprintf("failed to check pool account daily quota after quota update: account_id=%d, error=%v", accountID, err))
		return
	}
	if account.DailyQuotaLimit > 0 && account.DailyUsedQuota >= account.DailyQuotaLimit {
		if err := MarkPoolAccountDailyLimitExceeded(accountID, ErrPoolAccountDailyQuotaLimitExceeded, time.Now()); err != nil {
			common.SysLog(fmt.Sprintf("failed to mark pool account daily quota exceeded: account_id=%d, error=%v", accountID, err))
		}
	}
}

// RecordPoolAccountRequest 记录池账号的请求结果
// 成功时增加成功计数并清除不可用状态，失败时增加失败计数
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

// RecordPoolAccountUsageLog 写入账号池账号的一次使用日志。
// 写入失败不应影响 Relay 主流程，因此只记录系统日志并返回。
func RecordPoolAccountUsageLog(record PoolAccountUsageLogRecord) {
	if record.PoolAccountId <= 0 {
		return
	}
	logDB := LOG_DB
	if logDB == nil {
		logDB = DB
	}
	if logDB == nil {
		return
	}
	log := &PoolAccountUsageLog{
		CreatedAt:           common.GetTimestamp(),
		PoolGroupId:         record.PoolGroupId,
		PoolGroupName:       record.PoolGroupName,
		PoolAccountId:       record.PoolAccountId,
		PoolAccountName:     record.PoolAccountName,
		PoolAccountAuthType: record.PoolAccountAuthType,
		ChannelId:           record.ChannelId,
		ChannelName:         record.ChannelName,
		ModelName:           record.ModelName,
		UserId:              record.UserId,
		Username:            record.Username,
		TokenId:             record.TokenId,
		TokenName:           record.TokenName,
		Group:               record.Group,
		Quota:               record.Quota,
		PromptTokens:        record.PromptTokens,
		CompletionTokens:    record.CompletionTokens,
		UseTime:             record.UseTime,
		IsStream:            record.IsStream,
		Success:             record.Success,
		StatusCode:          record.StatusCode,
		ErrorCode:           strings.TrimSpace(record.ErrorCode),
		ErrorMessage:        strings.TrimSpace(record.ErrorMessage),
		RequestId:           strings.TrimSpace(record.RequestId),
		UpstreamRequestId:   strings.TrimSpace(record.UpstreamRequestId),
		RetryIndex:          record.RetryIndex,
	}
	if err := logDB.Create(log).Error; err != nil {
		common.SysLog(fmt.Sprintf("failed to record pool account usage log: account_id=%d, error=%v", record.PoolAccountId, err))
	}
}

// GetPoolAccountUsageLogs 分页查询账号池使用日志。
// 管理员页面直接消费该接口，避免从通用日志的 JSON other 字段反向解析账号池信息。
func GetPoolAccountUsageLogs(filter PoolAccountUsageLogFilter) ([]*PoolAccountUsageLog, int64, error) {
	logDB := LOG_DB
	if logDB == nil {
		logDB = DB
	}
	if logDB == nil {
		return nil, 0, gorm.ErrInvalidDB
	}
	if filter.Limit <= 0 {
		filter.Limit = 20
	}
	if filter.Limit > 100 {
		filter.Limit = 100
	}
	query := logDB.Model(&PoolAccountUsageLog{})
	if filter.PoolGroupId > 0 {
		query = query.Where("pool_group_id = ?", filter.PoolGroupId)
	}
	if filter.PoolAccountId > 0 {
		query = query.Where("pool_account_id = ?", filter.PoolAccountId)
	}
	if filter.ChannelId > 0 {
		query = query.Where("channel_id = ?", filter.ChannelId)
	}
	if filter.UserId > 0 {
		query = query.Where("user_id = ?", filter.UserId)
	}
	if filter.Success != nil {
		query = query.Where("success = ?", *filter.Success)
	}
	if filter.StartTimestamp > 0 {
		query = query.Where("created_at >= ?", filter.StartTimestamp)
	}
	if filter.EndTimestamp > 0 {
		query = query.Where("created_at <= ?", filter.EndTimestamp)
	}
	if strings.TrimSpace(filter.ModelName) != "" {
		query = query.Where("model_name = ?", strings.TrimSpace(filter.ModelName))
	}
	if strings.TrimSpace(filter.RequestId) != "" {
		query = query.Where("request_id = ?", strings.TrimSpace(filter.RequestId))
	}
	if strings.TrimSpace(filter.UpstreamRequestId) != "" {
		query = query.Where("upstream_request_id = ?", strings.TrimSpace(filter.UpstreamRequestId))
	}
	if strings.TrimSpace(filter.Search) != "" {
		like := "%" + strings.TrimSpace(filter.Search) + "%"
		query = query.Where("pool_group_name LIKE ? OR pool_account_name LIKE ? OR channel_name LIKE ? OR model_name LIKE ? OR username LIKE ? OR token_name LIKE ? OR error_message LIKE ?", like, like, like, like, like, like, like)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	logs := []*PoolAccountUsageLog{}
	err := query.Order("id DESC").Limit(filter.Limit).Offset(filter.StartIdx).Find(&logs).Error
	return logs, total, err
}

// RecordPoolAccountStateLog 写入账号池账号状态变更日志。
// 日志写入失败不应影响热路径、检测任务或管理员操作，因此失败时只写系统日志。
func RecordPoolAccountStateLog(record PoolAccountStateLogRecord) {
	if record.PoolAccountId <= 0 {
		return
	}
	logDB := LOG_DB
	if logDB == nil {
		logDB = DB
	}
	if logDB == nil {
		return
	}
	after, err := GetPoolAccountById(record.PoolAccountId)
	if err != nil || after == nil {
		common.SysLog(fmt.Sprintf("failed to load pool account state after update: account_id=%d, error=%v", record.PoolAccountId, err))
		return
	}
	groupName := ""
	if after.PoolGroupId > 0 {
		if group, groupErr := GetAccountPoolGroupById(after.PoolGroupId); groupErr == nil && group != nil {
			groupName = group.Name
		}
	}
	before := record.Before
	action := strings.TrimSpace(record.Action)
	if action == "" {
		action = "unknown"
	}
	source := strings.TrimSpace(record.Source)
	if source == "" {
		source = "system"
	}
	log := &PoolAccountStateLog{
		CreatedAt:           common.GetTimestamp(),
		PoolGroupId:         after.PoolGroupId,
		PoolGroupName:       groupName,
		PoolAccountId:       after.Id,
		PoolAccountName:     after.Name,
		PoolAccountAuthType: after.AuthType,
		Action:              action,
		Source:              source,
		Actor:               strings.TrimSpace(record.Actor),
		Reason:              strings.TrimSpace(record.Reason),
		RequestId:           strings.TrimSpace(record.RequestId),
		AfterStatus:         after.Status,
		AfterSchedulable:    after.Schedulable,
		AfterUnavailable:    after.Unavailable,
		AfterNextRetryTime:  after.NextRetryTime,
		AfterStatusMessage:  after.StatusMessage,
		AfterDisabledReason: after.DisabledReason,
	}
	if before != nil {
		log.BeforeStatus = before.Status
		log.BeforeSchedulable = before.Schedulable
		log.BeforeUnavailable = before.Unavailable
		log.BeforeNextRetryTime = before.NextRetryTime
		log.BeforeStatusMessage = before.StatusMessage
		log.BeforeDisabledReason = before.DisabledReason
	}
	if err := logDB.Create(log).Error; err != nil {
		common.SysLog(fmt.Sprintf("failed to record pool account state log: account_id=%d, action=%s, error=%v", record.PoolAccountId, action, err))
	}
}

// GetPoolAccountStateLogs 分页查询账号池账号状态变更日志。
func GetPoolAccountStateLogs(filter PoolAccountStateLogFilter) ([]*PoolAccountStateLog, int64, error) {
	logDB := LOG_DB
	if logDB == nil {
		logDB = DB
	}
	if logDB == nil {
		return nil, 0, gorm.ErrInvalidDB
	}
	if filter.Limit <= 0 {
		filter.Limit = 20
	}
	if filter.Limit > 100 {
		filter.Limit = 100
	}
	query := logDB.Model(&PoolAccountStateLog{})
	if filter.PoolGroupId > 0 {
		query = query.Where("pool_group_id = ?", filter.PoolGroupId)
	}
	if filter.PoolAccountId > 0 {
		query = query.Where("pool_account_id = ?", filter.PoolAccountId)
	}
	if strings.TrimSpace(filter.Action) != "" {
		query = query.Where("action = ?", strings.TrimSpace(filter.Action))
	}
	if strings.TrimSpace(filter.Source) != "" {
		query = query.Where("source = ?", strings.TrimSpace(filter.Source))
	}
	if strings.TrimSpace(filter.Actor) != "" {
		query = query.Where("actor = ?", strings.TrimSpace(filter.Actor))
	}
	if filter.StartTimestamp > 0 {
		query = query.Where("created_at >= ?", filter.StartTimestamp)
	}
	if filter.EndTimestamp > 0 {
		query = query.Where("created_at <= ?", filter.EndTimestamp)
	}
	if strings.TrimSpace(filter.Search) != "" {
		like := "%" + strings.TrimSpace(filter.Search) + "%"
		query = query.Where("(pool_group_name LIKE ? OR pool_account_name LIKE ? OR action LIKE ? OR source LIKE ? OR actor LIKE ? OR reason LIKE ? OR after_status_message LIKE ? OR after_disabled_reason LIKE ?)", like, like, like, like, like, like, like, like)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	logs := []*PoolAccountStateLog{}
	err := query.Order("id DESC").Limit(filter.Limit).Offset(filter.StartIdx).Find(&logs).Error
	return logs, total, err
}
