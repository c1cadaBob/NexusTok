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
	"sort"
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
	// PoolAccountStateActionManualDelete 表示管理员手动删除账号。
	PoolAccountStateActionManualDelete = "manual_delete"
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

	// PoolAccountCheckTaskStatusQueued 表示账号检测任务已入队，等待后台 worker 执行。
	PoolAccountCheckTaskStatusQueued = "queued"
	// PoolAccountCheckTaskStatusRunning 表示账号检测任务正在执行。
	PoolAccountCheckTaskStatusRunning = "running"
	// PoolAccountCheckTaskStatusCompleted 表示账号检测任务已完成；其中仍可能包含检测失败的账号。
	PoolAccountCheckTaskStatusCompleted = "completed"
	// PoolAccountCheckTaskStatusFailed 表示账号检测任务因内部错误中断，未能完成全部账号检测。
	PoolAccountCheckTaskStatusFailed = "failed"

	// AccountPoolAutoCheckDefaultIntervalMinutes 表示分组自动检测的默认间隔。
	AccountPoolAutoCheckDefaultIntervalMinutes = 60
	// AccountPoolAutoCheckDefaultLimit 表示每次自动检测默认覆盖的账号数量上限。
	AccountPoolAutoCheckDefaultLimit = 100
	// AccountPoolAutoCheckMaxLimit 表示每次自动检测最多允许覆盖的账号数量。
	AccountPoolAutoCheckMaxLimit = 100

	// AccountPoolPreflightCheckModeOff 表示请求运行前不检查账号最近检测时间，保持旧调度行为。
	AccountPoolPreflightCheckModeOff = "off"
	// AccountPoolPreflightCheckModeWarmup 表示发现候选账号检测结果过期时异步创建后台检测任务，本次请求仍可继续调度。
	AccountPoolPreflightCheckModeWarmup = "warmup"
	// AccountPoolPreflightCheckModeRequireRecent 表示只允许最近检测结果仍在有效期内的账号进入调度候选集。
	AccountPoolPreflightCheckModeRequireRecent = "require_recent"
	// AccountPoolPreflightCheckDefaultFreshnessMinutes 表示运行前检测结果默认有效期，默认 24 小时。
	AccountPoolPreflightCheckDefaultFreshnessMinutes = 1440
	// AccountPoolPreflightCheckDefaultLimit 表示运行前预热任务默认最多检测的账号数量。
	AccountPoolPreflightCheckDefaultLimit = 20
	// AccountPoolPreflightCheckMaxLimit 表示运行前预热任务单次最多允许覆盖的账号数量。
	AccountPoolPreflightCheckMaxLimit = AccountPoolAutoCheckMaxLimit

	// AccountPoolNoAvailableActionFail 表示账号池短暂没有空闲账号时立即返回错误。
	AccountPoolNoAvailableActionFail = "fail"
	// AccountPoolNoAvailableActionWait 表示账号池短暂没有空闲账号时在安全超时内等待空闲槽位。
	AccountPoolNoAvailableActionWait = "wait"
	// AccountPoolNoAvailableDefaultWaitSeconds 表示等待策略的默认超时时间。
	AccountPoolNoAvailableDefaultWaitSeconds = 5
	// AccountPoolNoAvailableMaxWaitSeconds 表示等待策略允许配置的最大超时时间。
	AccountPoolNoAvailableMaxWaitSeconds = 30

	// AccountPoolTaskLimitActionFail 表示异步任务提交并发满时立即返回错误。
	AccountPoolTaskLimitActionFail = "fail"
	// AccountPoolTaskLimitActionWait 表示异步任务提交并发满时在安全超时内等待槽位。
	AccountPoolTaskLimitActionWait = "wait"
	// AccountPoolTaskLimitDefaultWaitSeconds 表示任务提交等待策略的默认超时时间。
	AccountPoolTaskLimitDefaultWaitSeconds = 5
	// AccountPoolTaskLimitMaxWaitSeconds 表示任务提交等待策略允许配置的最大超时时间。
	AccountPoolTaskLimitMaxWaitSeconds = 30
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
	AutoCheckEnabled  bool    `json:"auto_check_enabled" gorm:"default:false;index"`                               // 是否启用分组级账号可用性定时检测
	// AutoCheckIntervalMinutes 是自动检测间隔，单位为分钟；小于等于 0 时会回退到安全默认值。
	AutoCheckIntervalMinutes int `json:"auto_check_interval_minutes" gorm:"default:60"`
	// AutoCheckLimit 是单次自动检测最多覆盖的账号数，过大时会被限制到全局安全上限。
	AutoCheckLimit      int   `json:"auto_check_limit" gorm:"default:100"`
	AutoCheckLastTime   int64 `json:"auto_check_last_time" gorm:"bigint;default:0"`       // 最近一次自动检测任务创建时间
	AutoCheckNextTime   int64 `json:"auto_check_next_time" gorm:"bigint;default:0;index"` // 下次自动检测任务计划时间
	AutoCheckLastTaskId int   `json:"auto_check_last_task_id" gorm:"default:0"`           // 最近一次自动检测任务 ID
	// PreflightCheckMode 控制 Relay 热路径在选择账号前如何处理“检测结果过期”的账号。
	PreflightCheckMode string `json:"preflight_check_mode" gorm:"type:varchar(32);default:'off'"`
	// PreflightCheckFreshnessMinutes 是最近一次检测结果的有效期，单位分钟；小于等于 0 时回退到默认 24 小时。
	PreflightCheckFreshnessMinutes int `json:"preflight_check_freshness_minutes" gorm:"default:1440"`
	// PreflightCheckLimit 是运行前预热任务最多覆盖的账号数；预热只创建后台检测任务，不在热路径同步检测。
	PreflightCheckLimit int `json:"preflight_check_limit" gorm:"default:20"`
	// NoAvailableAction 控制分组或账号并发满时的处理方式。默认 fail 保持旧分组立即失败的行为。
	NoAvailableAction string `json:"no_available_action" gorm:"type:varchar(32);default:'fail'"`
	// NoAvailableWaitSeconds 是 wait 策略的最长等待秒数，调度层会限制到安全上限。
	NoAvailableWaitSeconds int `json:"no_available_wait_seconds" gorm:"default:0"`
	// TaskMaxConcurrency 限制同一账号池组内同一 platform + action 的异步任务提交并发。
	TaskMaxConcurrency int `json:"task_max_concurrency" gorm:"default:0"`
	// TaskRateLimitRpm 限制同一账号池组内同一 platform + action 的异步任务每分钟提交次数。
	TaskRateLimitRpm int `json:"task_rate_limit_rpm" gorm:"default:0"`
	// TaskLimitAction 控制任务提交并发满时立即失败还是短暂等待。RPM 超限始终立即失败。
	TaskLimitAction string `json:"task_limit_action" gorm:"type:varchar(32);default:'fail'"`
	// TaskLimitWaitSeconds 是任务提交并发满时 wait 策略的最长等待秒数。
	TaskLimitWaitSeconds int   `json:"task_limit_wait_seconds" gorm:"default:0"`
	CreatedTime          int64 `json:"created_time" gorm:"bigint"` // 创建时间
	UpdatedTime          int64 `json:"updated_time" gorm:"bigint"` // 更新时间

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

// NormalizeAccountPoolAutoCheckIntervalMinutes 规范化分组自动检测间隔。
// 自动检测调度器按分钟级扫描，间隔必须为正数；旧数据或错误请求写入 0/负数时，
// 统一回退到默认值，避免任务被每一轮扫描反复创建。
func NormalizeAccountPoolAutoCheckIntervalMinutes(minutes int) int {
	if minutes <= 0 {
		return AccountPoolAutoCheckDefaultIntervalMinutes
	}
	return minutes
}

// NormalizeAccountPoolAutoCheckLimit 规范化单次自动检测账号数量。
// 自动检测复用后台检测任务队列，单任务需要受固定上限保护，避免一个大分组长期占用
// worker 并拖慢管理员手动检测。
func NormalizeAccountPoolAutoCheckLimit(limit int) int {
	if limit <= 0 {
		return AccountPoolAutoCheckDefaultLimit
	}
	if limit > AccountPoolAutoCheckMaxLimit {
		return AccountPoolAutoCheckMaxLimit
	}
	return limit
}

// NormalizeAccountPoolPreflightCheckMode 规范化运行前检测策略。
// 空值和未知值都回退到 off，确保历史分组或异常请求不会意外改变 Relay 热路径准入规则。
func NormalizeAccountPoolPreflightCheckMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case AccountPoolPreflightCheckModeWarmup:
		return AccountPoolPreflightCheckModeWarmup
	case AccountPoolPreflightCheckModeRequireRecent:
		return AccountPoolPreflightCheckModeRequireRecent
	default:
		return AccountPoolPreflightCheckModeOff
	}
}

// NormalizeAccountPoolPreflightCheckFreshnessMinutes 规范化运行前检测结果有效期。
// 该值只用于判断 last_checked_time 是否足够新；非法值统一回退到 24 小时，避免误把所有账号过滤掉。
func NormalizeAccountPoolPreflightCheckFreshnessMinutes(minutes int) int {
	if minutes <= 0 {
		return AccountPoolPreflightCheckDefaultFreshnessMinutes
	}
	return minutes
}

// NormalizeAccountPoolPreflightCheckLimit 规范化运行前预热任务账号数量上限。
// 预热复用后台检测任务队列，限制单次覆盖数量可以避免某个请求把大分组全部投入检测。
func NormalizeAccountPoolPreflightCheckLimit(limit int) int {
	if limit <= 0 {
		return AccountPoolPreflightCheckDefaultLimit
	}
	if limit > AccountPoolPreflightCheckMaxLimit {
		return AccountPoolPreflightCheckMaxLimit
	}
	return limit
}

// GetAutoCheckIntervalMinutes 返回分组自动检测间隔。
// 历史数据或 API 请求可能写入 0/负数；这里统一回退到默认 60 分钟，避免调度器出现
// 立即重复创建任务的忙循环。
func (group *AccountPoolGroup) GetAutoCheckIntervalMinutes() int {
	if group == nil {
		return AccountPoolAutoCheckDefaultIntervalMinutes
	}
	return NormalizeAccountPoolAutoCheckIntervalMinutes(group.AutoCheckIntervalMinutes)
}

// GetAutoCheckLimit 返回单次自动检测账号数量上限。
// 自动检测最终会创建后台检测任务，单任务最多检测 100 个账号；此处和任务层保持同一上限，
// 防止定时扫描在大分组上长时间占用 worker。
func (group *AccountPoolGroup) GetAutoCheckLimit() int {
	if group == nil {
		return AccountPoolAutoCheckDefaultLimit
	}
	return NormalizeAccountPoolAutoCheckLimit(group.AutoCheckLimit)
}

// GetPreflightCheckMode 返回分组运行前检测策略。
// 默认 off 保持旧行为；只有管理员显式选择 warmup 或 require_recent 时，调度层才会读取 last_checked_time。
func (group *AccountPoolGroup) GetPreflightCheckMode() string {
	if group == nil {
		return AccountPoolPreflightCheckModeOff
	}
	return NormalizeAccountPoolPreflightCheckMode(group.PreflightCheckMode)
}

// GetPreflightCheckFreshnessMinutes 返回最近检测结果有效期。
// 运行前检测只依赖已存在的检测时间，不会在热路径同步刷新凭据；该窗口用于定义“近期”的业务边界。
func (group *AccountPoolGroup) GetPreflightCheckFreshnessMinutes() int {
	if group == nil {
		return AccountPoolPreflightCheckDefaultFreshnessMinutes
	}
	return NormalizeAccountPoolPreflightCheckFreshnessMinutes(group.PreflightCheckFreshnessMinutes)
}

// GetPreflightCheckLimit 返回运行前预热任务账号数量上限。
// warmup 和 require_recent 模式发现过期账号时都会复用该上限创建后台检测任务。
func (group *AccountPoolGroup) GetPreflightCheckLimit() int {
	if group == nil {
		return AccountPoolPreflightCheckDefaultLimit
	}
	return NormalizeAccountPoolPreflightCheckLimit(group.PreflightCheckLimit)
}

// NormalizeAccountPoolNoAvailableAction 规范化无空闲账号处理策略。
// 默认 fail 保持历史行为；wait 只用于短暂并发满场景，不表示完整任务队列。
func NormalizeAccountPoolNoAvailableAction(action string) string {
	action = strings.ToLower(strings.TrimSpace(action))
	if action == AccountPoolNoAvailableActionWait {
		return AccountPoolNoAvailableActionWait
	}
	return AccountPoolNoAvailableActionFail
}

// NormalizeAccountPoolNoAvailableWaitSeconds 规范化无空闲账号等待超时时间。
// 等待会占用当前 Relay 请求连接，因此必须有保守上限，避免误配置造成请求长时间堆积。
func NormalizeAccountPoolNoAvailableWaitSeconds(seconds int) int {
	if seconds <= 0 {
		return AccountPoolNoAvailableDefaultWaitSeconds
	}
	if seconds > AccountPoolNoAvailableMaxWaitSeconds {
		return AccountPoolNoAvailableMaxWaitSeconds
	}
	return seconds
}

// GetNoAvailableAction 返回账号池组的无空闲账号处理策略。
// 旧数据字段为空时按 fail 处理，确保升级后不会改变已有渠道的延迟和错误行为。
func (group *AccountPoolGroup) GetNoAvailableAction() string {
	if group == nil {
		return AccountPoolNoAvailableActionFail
	}
	return NormalizeAccountPoolNoAvailableAction(group.NoAvailableAction)
}

// GetNoAvailableWaitSeconds 返回 wait 策略的最长等待秒数。
// 即使数据库保存了异常值，调度层也只会使用规范化后的安全范围。
func (group *AccountPoolGroup) GetNoAvailableWaitSeconds() int {
	if group == nil {
		return AccountPoolNoAvailableDefaultWaitSeconds
	}
	return NormalizeAccountPoolNoAvailableWaitSeconds(group.NoAvailableWaitSeconds)
}

// NormalizeAccountPoolTaskLimitAction 规范化异步任务提交并发满时的处理策略。
// 默认 fail 保持历史行为；wait 只表示在当前 HTTP 请求内短暂等待任务提交槽位，
// 不代表任务已经进入持久化队列，也不会把账号占用延长到上游异步任务完成。
func NormalizeAccountPoolTaskLimitAction(action string) string {
	action = strings.ToLower(strings.TrimSpace(action))
	if action == AccountPoolTaskLimitActionWait {
		return AccountPoolTaskLimitActionWait
	}
	return AccountPoolTaskLimitActionFail
}

// NormalizeAccountPoolTaskLimitWaitSeconds 规范化任务提交等待超时时间。
// 等待会占用当前 Relay 请求连接，因此必须保持保守上限，避免误配置造成连接堆积。
func NormalizeAccountPoolTaskLimitWaitSeconds(seconds int) int {
	if seconds <= 0 {
		return AccountPoolTaskLimitDefaultWaitSeconds
	}
	if seconds > AccountPoolTaskLimitMaxWaitSeconds {
		return AccountPoolTaskLimitMaxWaitSeconds
	}
	return seconds
}

// GetTaskMaxConcurrency 返回异步任务提交级最大并发限制。
// 返回 0 表示不限制；该限制按账号池分组和任务 platform + action 维度生效。
func (group *AccountPoolGroup) GetTaskMaxConcurrency() int {
	if group == nil || group.TaskMaxConcurrency <= 0 {
		return 0
	}
	return group.TaskMaxConcurrency
}

// GetTaskRateLimitRpm 返回异步任务提交级每分钟频率限制。
// 返回 0 表示不限制；达到上限时立即返回 429，不等待下一个自然分钟窗口。
func (group *AccountPoolGroup) GetTaskRateLimitRpm() int {
	if group == nil || group.TaskRateLimitRpm <= 0 {
		return 0
	}
	return group.TaskRateLimitRpm
}

// GetTaskLimitAction 返回异步任务提交并发满时的处理策略。
// 旧数据字段为空时按 fail 处理，确保升级后不会改变已有任务提交延迟。
func (group *AccountPoolGroup) GetTaskLimitAction() string {
	if group == nil {
		return AccountPoolTaskLimitActionFail
	}
	return NormalizeAccountPoolTaskLimitAction(group.TaskLimitAction)
}

// GetTaskLimitWaitSeconds 返回任务提交并发等待超时时间。
// 即使数据库保存异常值，调度层也只使用规范化后的安全范围。
func (group *AccountPoolGroup) GetTaskLimitWaitSeconds() int {
	if group == nil {
		return AccountPoolTaskLimitDefaultWaitSeconds
	}
	return NormalizeAccountPoolTaskLimitWaitSeconds(group.TaskLimitWaitSeconds)
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
	AuthFileId         int     `json:"auth_file_id" gorm:"index;default:0"`                                 // 来源认证文件 ID；同一凭证分配到多个账号组时，各组内调度实例共享该 ID
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

// PoolAccountCheckTask 记录一次后台账号可用性检测任务。
//
// 任务表存放在主业务库中，而不是 LOG_DB：后台检测任务需要被管理页面轮询查询，并且
// 任务状态会随着 worker 执行持续更新。ResultsJSON 只保存脱敏后的检测结果列表，
// 不保存账号凭证原文；AccountIds 使用逗号分隔 ID，避免依赖数据库专用 JSON 类型。
type PoolAccountCheckTask struct {
	Id            int    `json:"id"`
	PoolGroupId   int    `json:"pool_group_id" gorm:"index:idx_pool_account_check_task_group"`
	PoolGroupName string `json:"pool_group_name" gorm:"type:varchar(255)"`
	Status        string `json:"status" gorm:"type:varchar(32);index:idx_pool_account_check_task_status"`
	Actor         string `json:"actor" gorm:"type:varchar(255);index"`
	RequestId     string `json:"request_id,omitempty" gorm:"type:varchar(64);index"`
	AccountIds    string `json:"account_ids" gorm:"type:text"`
	Total         int    `json:"total" gorm:"default:0"`
	Checked       int    `json:"checked" gorm:"default:0"`
	Success       int    `json:"success" gorm:"default:0"`
	Failed        int    `json:"failed" gorm:"default:0"`
	Skipped       int    `json:"skipped" gorm:"default:0"`
	Message       string `json:"message" gorm:"type:text"`
	ResultsJSON   string `json:"-" gorm:"column:results_json;type:text"`
	StartedTime   int64  `json:"started_time" gorm:"bigint;default:0"`
	FinishedTime  int64  `json:"finished_time" gorm:"bigint;default:0"`
	CreatedTime   int64  `json:"created_time" gorm:"bigint;index:idx_pool_account_check_task_created"`
	UpdatedTime   int64  `json:"updated_time" gorm:"bigint"`
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
	RequestId      string
	StartTimestamp int64
	EndTimestamp   int64
	Search         string
	StartIdx       int
	Limit          int
	// MaxLimit 允许调用方为导出等只读场景提高单次读取上限；为空时保持普通列表接口最多 100 条。
	MaxLimit int
}

// PoolAccountStateLogAuditBucket 是状态日志按动作、来源或操作者聚合后的统计项。
type PoolAccountStateLogAuditBucket struct {
	Key      string `json:"key"`
	Total    int64  `json:"total"`
	LatestAt int64  `json:"latest_at"`
}

// PoolAccountStateLogAuditAccountRef 是批量审计摘要中的账号引用。
// 只返回账号 ID 和名称，避免把状态日志列表扩展成凭据或运行时配置导出通道。
type PoolAccountStateLogAuditAccountRef struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

// PoolAccountStateLogBulkAuditSummary 汇总同一批状态变更操作。
//
// 批量启用、禁用、清冷却、批量删除和后台检测任务都会为多个账号写入多条状态日志；
// 这里按 request_id 优先聚合。没有 request_id 的历史日志按分钟、动作、来源、分组和原因做保守聚合，
// 只返回影响账号数大于 1 的摘要，避免把普通单账号操作误展示成批量事件。
type PoolAccountStateLogBulkAuditSummary struct {
	Action         string                                `json:"action"`
	Source         string                                `json:"source"`
	Actor          string                                `json:"actor"`
	Reason         string                                `json:"reason"`
	RequestId      string                                `json:"request_id,omitempty"`
	PoolGroupId    int                                   `json:"pool_group_id"`
	PoolGroupName  string                                `json:"pool_group_name"`
	AccountCount   int                                   `json:"account_count"`
	FirstAt        int64                                 `json:"first_at"`
	LastAt         int64                                 `json:"last_at"`
	SampleAccounts []*PoolAccountStateLogAuditAccountRef `json:"sample_accounts"`
}

// PoolAccountStateLogAuditSummary 是账号池状态日志的审计概览。
//
// 该结构只基于已脱敏的状态日志生成，用于管理页面快速回答：
// 最近发生了哪些类型的操作、来自哪些系统来源、是否存在批量操作，以及影响账号规模。
type PoolAccountStateLogAuditSummary struct {
	GeneratedAt          int64                                  `json:"generated_at"`
	Total                int64                                  `json:"total"`
	ManualTotal          int64                                  `json:"manual_total"`
	AutomaticTotal       int64                                  `json:"automatic_total"`
	AffectedAccounts     int64                                  `json:"affected_accounts"`
	ActionStats          []*PoolAccountStateLogAuditBucket      `json:"action_stats"`
	SourceStats          []*PoolAccountStateLogAuditBucket      `json:"source_stats"`
	ActorStats           []*PoolAccountStateLogAuditBucket      `json:"actor_stats"`
	RecentBulkOperations []*PoolAccountStateLogBulkAuditSummary `json:"recent_bulk_operations"`
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
	group.AutoCheckIntervalMinutes = NormalizeAccountPoolAutoCheckIntervalMinutes(group.AutoCheckIntervalMinutes)
	group.AutoCheckLimit = NormalizeAccountPoolAutoCheckLimit(group.AutoCheckLimit)
	if group.AutoCheckLastTime < 0 {
		group.AutoCheckLastTime = 0
	}
	if group.AutoCheckNextTime < 0 {
		group.AutoCheckNextTime = 0
	}
	if group.AutoCheckLastTaskId < 0 {
		group.AutoCheckLastTaskId = 0
	}
	group.PreflightCheckMode = NormalizeAccountPoolPreflightCheckMode(group.PreflightCheckMode)
	group.PreflightCheckFreshnessMinutes = NormalizeAccountPoolPreflightCheckFreshnessMinutes(group.PreflightCheckFreshnessMinutes)
	group.PreflightCheckLimit = NormalizeAccountPoolPreflightCheckLimit(group.PreflightCheckLimit)
	group.NoAvailableAction = NormalizeAccountPoolNoAvailableAction(group.NoAvailableAction)
	group.NoAvailableWaitSeconds = NormalizeAccountPoolNoAvailableWaitSeconds(group.NoAvailableWaitSeconds)
	if group.TaskMaxConcurrency < 0 {
		group.TaskMaxConcurrency = 0
	}
	if group.TaskRateLimitRpm < 0 {
		group.TaskRateLimitRpm = 0
	}
	group.TaskLimitAction = NormalizeAccountPoolTaskLimitAction(group.TaskLimitAction)
	group.TaskLimitWaitSeconds = NormalizeAccountPoolTaskLimitWaitSeconds(group.TaskLimitWaitSeconds)
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

// BeforeCreate GORM 钩子：创建后台检测任务前设置时间戳并规范化状态。
func (task *PoolAccountCheckTask) BeforeCreate(tx *gorm.DB) error {
	_ = tx
	now := common.GetTimestamp()
	if task.CreatedTime == 0 {
		task.CreatedTime = now
	}
	task.UpdatedTime = now
	task.normalize()
	return nil
}

// BeforeUpdate GORM 钩子：更新后台检测任务前刷新更新时间并规范化计数字段。
func (task *PoolAccountCheckTask) BeforeUpdate(tx *gorm.DB) error {
	_ = tx
	task.UpdatedTime = common.GetTimestamp()
	task.normalize()
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

// normalize 规范化后台检测任务字段，避免旧数据或部分更新写入非法状态和负数统计。
func (task *PoolAccountCheckTask) normalize() {
	task.PoolGroupName = strings.TrimSpace(task.PoolGroupName)
	task.Status = strings.ToLower(strings.TrimSpace(task.Status))
	switch task.Status {
	case PoolAccountCheckTaskStatusQueued, PoolAccountCheckTaskStatusRunning, PoolAccountCheckTaskStatusCompleted, PoolAccountCheckTaskStatusFailed:
	default:
		task.Status = PoolAccountCheckTaskStatusQueued
	}
	task.Actor = strings.TrimSpace(task.Actor)
	task.RequestId = strings.TrimSpace(task.RequestId)
	task.AccountIds = normalizeAccountPoolCSV(task.AccountIds)
	task.Message = strings.TrimSpace(task.Message)
	if task.Total < 0 {
		task.Total = 0
	}
	if task.Checked < 0 {
		task.Checked = 0
	}
	if task.Success < 0 {
		task.Success = 0
	}
	if task.Failed < 0 {
		task.Failed = 0
	}
	if task.Skipped < 0 {
		task.Skipped = 0
	}
	if task.StartedTime < 0 {
		task.StartedTime = 0
	}
	if task.FinishedTime < 0 {
		task.FinishedTime = 0
	}
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
		linkedAuthFiles := DB.Model(&PoolAccount{}).
			Select("auth_file_id").
			Where("pool_group_id = ? AND auth_file_id > ?", poolGroupID, 0)
		query = query.Where("pool_group_id = ? OR id IN (?)", poolGroupID, linkedAuthFiles)
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
	before := record.Before
	if err != nil {
		after = nil
	}
	if after == nil && before == nil {
		common.SysLog(fmt.Sprintf("failed to load pool account state after update: account_id=%d, error=%v", record.PoolAccountId, err))
		return
	}
	// 删除账号后无法再读取 after 快照；此时使用调用方传入的 before 快照保留账号归属、
	// 名称和变更前状态，after 字段保持零值，便于审计页面明确识别“账号已被删除”。
	snapshot := after
	if snapshot == nil {
		snapshot = before
	}
	groupName := ""
	if snapshot.PoolGroupId > 0 {
		if group, groupErr := GetAccountPoolGroupById(snapshot.PoolGroupId); groupErr == nil && group != nil {
			groupName = group.Name
		}
	}
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
		PoolGroupId:         snapshot.PoolGroupId,
		PoolGroupName:       groupName,
		PoolAccountId:       snapshot.Id,
		PoolAccountName:     snapshot.Name,
		PoolAccountAuthType: snapshot.AuthType,
		Action:              action,
		Source:              source,
		Actor:               strings.TrimSpace(record.Actor),
		Reason:              strings.TrimSpace(record.Reason),
		RequestId:           strings.TrimSpace(record.RequestId),
	}
	if after != nil {
		log.AfterStatus = after.Status
		log.AfterSchedulable = after.Schedulable
		log.AfterUnavailable = after.Unavailable
		log.AfterNextRetryTime = after.NextRetryTime
		log.AfterStatusMessage = after.StatusMessage
		log.AfterDisabledReason = after.DisabledReason
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

func poolAccountStateLogDB() (*gorm.DB, error) {
	logDB := LOG_DB
	if logDB == nil {
		logDB = DB
	}
	if logDB == nil {
		return nil, gorm.ErrInvalidDB
	}
	return logDB, nil
}

func applyPoolAccountStateLogFilter(query *gorm.DB, filter PoolAccountStateLogFilter) *gorm.DB {
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
	if strings.TrimSpace(filter.RequestId) != "" {
		query = query.Where("request_id = ?", strings.TrimSpace(filter.RequestId))
	}
	if filter.StartTimestamp > 0 {
		query = query.Where("created_at >= ?", filter.StartTimestamp)
	}
	if filter.EndTimestamp > 0 {
		query = query.Where("created_at <= ?", filter.EndTimestamp)
	}
	if strings.TrimSpace(filter.Search) != "" {
		like := "%" + strings.TrimSpace(filter.Search) + "%"
		query = query.Where("(pool_group_name LIKE ? OR pool_account_name LIKE ? OR action LIKE ? OR source LIKE ? OR actor LIKE ? OR request_id LIKE ? OR reason LIKE ? OR after_status_message LIKE ? OR after_disabled_reason LIKE ?)", like, like, like, like, like, like, like, like, like)
	}
	return query
}

func normalizePoolAccountStateLogLimit(filter PoolAccountStateLogFilter) int {
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	maxLimit := filter.MaxLimit
	if maxLimit <= 0 {
		maxLimit = 100
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	return limit
}

// GetPoolAccountStateLogs 分页查询账号池账号状态变更日志。
func GetPoolAccountStateLogs(filter PoolAccountStateLogFilter) ([]*PoolAccountStateLog, int64, error) {
	logDB, err := poolAccountStateLogDB()
	if err != nil {
		return nil, 0, err
	}
	limit := normalizePoolAccountStateLogLimit(filter)
	query := applyPoolAccountStateLogFilter(logDB.Model(&PoolAccountStateLog{}), filter)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	logs := []*PoolAccountStateLog{}
	err = query.Order("id DESC").Limit(limit).Offset(filter.StartIdx).Find(&logs).Error
	return logs, total, err
}

// GetPoolAccountStateLogAuditSummary 聚合账号池状态日志的审计概览。
func GetPoolAccountStateLogAuditSummary(filter PoolAccountStateLogFilter) (*PoolAccountStateLogAuditSummary, error) {
	logDB, err := poolAccountStateLogDB()
	if err != nil {
		return nil, err
	}
	summary := &PoolAccountStateLogAuditSummary{
		GeneratedAt: common.GetTimestamp(),
	}
	if err := applyPoolAccountStateLogFilter(logDB.Model(&PoolAccountStateLog{}), filter).Count(&summary.Total).Error; err != nil {
		return nil, err
	}
	if err := applyPoolAccountStateLogFilter(logDB.Model(&PoolAccountStateLog{}), filter).Where("source = ?", "admin").Count(&summary.ManualTotal).Error; err != nil {
		return nil, err
	}
	if err := applyPoolAccountStateLogFilter(logDB.Model(&PoolAccountStateLog{}), filter).Where("source <> ?", "admin").Count(&summary.AutomaticTotal).Error; err != nil {
		return nil, err
	}
	if err := applyPoolAccountStateLogFilter(logDB.Model(&PoolAccountStateLog{}), filter).Distinct("pool_account_id").Count(&summary.AffectedAccounts).Error; err != nil {
		return nil, err
	}
	summary.ActionStats, err = getPoolAccountStateLogBuckets(logDB, filter, "action", false, 20)
	if err != nil {
		return nil, err
	}
	summary.SourceStats, err = getPoolAccountStateLogBuckets(logDB, filter, "source", false, 20)
	if err != nil {
		return nil, err
	}
	summary.ActorStats, err = getPoolAccountStateLogBuckets(logDB, filter, "actor", true, 20)
	if err != nil {
		return nil, err
	}
	summary.RecentBulkOperations, err = getPoolAccountStateLogBulkSummaries(filter, 8)
	if err != nil {
		return nil, err
	}
	return summary, nil
}

type poolAccountStateLogBucketRow struct {
	BucketKey string `gorm:"column:bucket_key"`
	Total     int64  `gorm:"column:total"`
	LatestAt  int64  `gorm:"column:latest_at"`
}

func getPoolAccountStateLogBuckets(logDB *gorm.DB, filter PoolAccountStateLogFilter, column string, excludeEmpty bool, limit int) ([]*PoolAccountStateLogAuditBucket, error) {
	if limit <= 0 {
		limit = 20
	}
	query := applyPoolAccountStateLogFilter(logDB.Model(&PoolAccountStateLog{}), filter)
	if excludeEmpty {
		query = query.Where(column+" <> ?", "")
	}
	rows := []*poolAccountStateLogBucketRow{}
	selectClause := fmt.Sprintf("%s AS bucket_key, COUNT(*) AS total, MAX(created_at) AS latest_at", column)
	if err := query.Select(selectClause).Group(column).Order("COUNT(*) DESC").Order("MAX(created_at) DESC").Limit(limit).Scan(&rows).Error; err != nil {
		return nil, err
	}
	buckets := make([]*PoolAccountStateLogAuditBucket, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		buckets = append(buckets, &PoolAccountStateLogAuditBucket{
			Key:      row.BucketKey,
			Total:    row.Total,
			LatestAt: row.LatestAt,
		})
	}
	return buckets, nil
}

type poolAccountStateLogBulkAccumulator struct {
	summary     *PoolAccountStateLogBulkAuditSummary
	accountSeen map[int]bool
}

func getPoolAccountStateLogBulkSummaries(filter PoolAccountStateLogFilter, limit int) ([]*PoolAccountStateLogBulkAuditSummary, error) {
	logs, _, err := GetPoolAccountStateLogs(PoolAccountStateLogFilter{
		PoolGroupId:    filter.PoolGroupId,
		PoolAccountId:  filter.PoolAccountId,
		Action:         filter.Action,
		Source:         filter.Source,
		Actor:          filter.Actor,
		RequestId:      filter.RequestId,
		StartTimestamp: filter.StartTimestamp,
		EndTimestamp:   filter.EndTimestamp,
		Search:         filter.Search,
		Limit:          1000,
		MaxLimit:       1000,
	})
	if err != nil {
		return nil, err
	}
	grouped := map[string]*poolAccountStateLogBulkAccumulator{}
	for _, log := range logs {
		if log == nil {
			continue
		}
		key := poolAccountStateLogBulkKey(log)
		acc := grouped[key]
		if acc == nil {
			acc = &poolAccountStateLogBulkAccumulator{
				summary: &PoolAccountStateLogBulkAuditSummary{
					Action:         log.Action,
					Source:         log.Source,
					Actor:          log.Actor,
					Reason:         log.Reason,
					RequestId:      log.RequestId,
					PoolGroupId:    log.PoolGroupId,
					PoolGroupName:  log.PoolGroupName,
					FirstAt:        log.CreatedAt,
					LastAt:         log.CreatedAt,
					SampleAccounts: make([]*PoolAccountStateLogAuditAccountRef, 0, 6),
				},
				accountSeen: map[int]bool{},
			}
			grouped[key] = acc
		}
		acc.add(log)
	}
	summaries := make([]*PoolAccountStateLogBulkAuditSummary, 0, len(grouped))
	for _, acc := range grouped {
		if acc == nil || acc.summary == nil || acc.summary.AccountCount <= 1 {
			continue
		}
		summaries = append(summaries, acc.summary)
	}
	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].LastAt == summaries[j].LastAt {
			return summaries[i].AccountCount > summaries[j].AccountCount
		}
		return summaries[i].LastAt > summaries[j].LastAt
	})
	if limit > 0 && len(summaries) > limit {
		summaries = summaries[:limit]
	}
	return summaries, nil
}

func poolAccountStateLogBulkKey(log *PoolAccountStateLog) string {
	if strings.TrimSpace(log.RequestId) != "" {
		return strings.Join([]string{log.RequestId, log.Action, log.Source}, "|")
	}
	return fmt.Sprintf("%s|%s|%s|%s|%d|%d", log.Action, log.Source, log.Actor, log.Reason, log.PoolGroupId, log.CreatedAt/60)
}

func (acc *poolAccountStateLogBulkAccumulator) add(log *PoolAccountStateLog) {
	if acc == nil || acc.summary == nil || log == nil {
		return
	}
	if log.CreatedAt < acc.summary.FirstAt {
		acc.summary.FirstAt = log.CreatedAt
	}
	if log.CreatedAt > acc.summary.LastAt {
		acc.summary.LastAt = log.CreatedAt
	}
	if log.PoolAccountId <= 0 || acc.accountSeen[log.PoolAccountId] {
		return
	}
	acc.accountSeen[log.PoolAccountId] = true
	acc.summary.AccountCount = len(acc.accountSeen)
	if len(acc.summary.SampleAccounts) < 6 {
		acc.summary.SampleAccounts = append(acc.summary.SampleAccounts, &PoolAccountStateLogAuditAccountRef{
			Id:   log.PoolAccountId,
			Name: log.PoolAccountName,
		})
	}
}
